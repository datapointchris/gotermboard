package gotermboard

import (
	"context"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermstyle"
	"golang.org/x/term"
)

// Board is one view, as this package draws it: sources to keep fresh, and the
// frame their last good reads make now.
type Board interface {
	Sources() []Refresher
	Groupings() []string
	Frame(now time.Time, grouping string) Frame
}

// Keyed is a Board with keys of its own. They are read ahead of the table's, so a
// view can add a key or take one over, and the footer and the ? overlay list them.
type Keyed interface {
	Actions() []Action
}

// TailBoard is a Board read from its newest row, as a log is. Its cursor starts on
// the last row, and stays on the last row as rows land while it is there.
type TailBoard interface {
	Tails() bool
}

// CursorBoard is a Board told which key the cursor is on, for a read that follows it.
type CursorBoard interface {
	CursorOn(key string)
}

// MovingBoard is a Board whose strip moves between its frames, as a diagram of work in motion
// does. Only the strip redraws at this pace, so the table and its change cue wait for their
// frame. Only the tab showing runs it, and a reader over the board holds it.
type MovingBoard interface {
	// MoveEvery is how long the strip holds each drawing. Zero or less holds it still, and the
	// next tick asks again, so a board with nothing moving spends nothing.
	MoveEvery() time.Duration
	// StripAt is the whole strip at now. It stands in for Frame.Strip on every draw, so a line
	// holding still is returned as it is. A strip that changes its height redraws the table.
	StripAt(now time.Time) []Line
}

// PanelFrom is the narrowest screen that gets the side panel.
const PanelFrom = 160

// tickEvery redraws the ages and starts what is due.
const tickEvery = time.Second

// Model is the Bubble Tea program of one view. Tabs holds one per tab.
type Model struct {
	board  Board
	pal    gotermstyle.Palette
	ctx    context.Context
	cancel context.CancelFunc

	frame    Frame
	width    int
	height   int
	grouping int

	display  display
	cursor   string
	cursorAt int
	top      int
	placed   bool
	panelOff bool
	// panelHasText says this frame holds text for the side panel. It is the frame's answer
	// rather than the cursor's line's, so the table never jumps as the cursor moves.
	panelHasText bool
	filtering    bool
	// filterLine is the filter as it is typed.
	filterLine typedLine
	overlay    bool
	said       string
	// asking is the question a key put on the footer, until enter or esc closes it, and answer
	// is what is typed under it.
	asking *Question
	answer typedLine
	// scrolled holds top where ScrollBy put it, until the cursor moves or a key changes what
	// shows, so a row's detail taller than the screen can be read through.
	scrolled bool
	// hidden is set while another tab shows. A hidden model starts no read.
	hidden bool
	// hosted are the keys of whatever holds this model, ahead of the table's.
	hosted []Action
	cue    changeCue
	// toldCursor is the key a CursorBoard last heard.
	toldCursor string
	// reading is the full-screen Reader over the board, while one is open.
	reading *Reader
	// moveClock numbers the strip's clock. A hide moves it on, so a beat sent before the hide
	// is dropped. moving says a beat is on its way.
	moveClock int
	moving    bool
	// underStrip is what the last draw put under the strip, and stripOnly says the next draw
	// keeps it, because only a move came in since.
	underStrip []string
	stripOnly  bool
	// charts is the frame's Charts laid out at the screen's size, painted.
	charts []string
}

// NewModel is a model reading board, sized until the terminal says otherwise.
func NewModel(board Board, pal gotermstyle.Palette) *Model {
	ctx, cancel := context.WithCancel(context.Background())
	m := newModel(board, pal, ctx, cancel)
	m.rebuild(time.Now())
	return m
}

func newModel(board Board, pal gotermstyle.Palette, ctx context.Context, cancel context.CancelFunc) *Model {
	return &Model{
		board: board, pal: pal, ctx: ctx, cancel: cancel, width: 80, height: 24, cue: newChangeCue(),
		display: display{folded: map[string]bool{}, opened: map[string]bool{}, matched: newRowMatches()},
	}
}

// Once is one frame at a size, drawn from what the board's sources already hold.
func Once(board Board, pal gotermstyle.Palette, width, height int) string {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := newModel(board, pal, ctx, cancel)
	m.width, m.height = width, height
	m.rebuild(time.Now())
	m.tellCursor()
	return m.screen()
}

// OnceSize is the size one printed frame draws at: COLUMNS by LINES, then the terminal's
// own size, then the fallback.
func OnceSize(fallbackWidth, fallbackHeight int) (width, height int) {
	width, height = fallbackWidth, fallbackHeight
	if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		width, height = w, h
	}
	if w, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && w > 0 {
		width = w
	}
	if h, err := strconv.Atoi(os.Getenv("LINES")); err == nil && h > 0 {
		height = h
	}
	return width, height
}

type (
	tickMsg      time.Time
	refreshedMsg struct{}
	// moveMsg is one beat of the strip's clock, for the model that started it.
	moveMsg struct {
		model *Model
		clock int
		at    time.Time
	}
)

func tick() tea.Cmd {
	return tea.Tick(tickEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Init starts every source's first read and the clock.
func (m *Model) Init() tea.Cmd { return tea.Batch(tick(), m.reveal(time.Now())) }

// reveal draws the last good reads, with no change cue, and starts what is due. The reads
// claim before the frame is built, so the frame sees the quiet last until they return.
func (m *Model) reveal(now time.Time) tea.Cmd {
	m.hidden = false
	m.cue.hush(now)
	reads := m.dueReads(now)
	m.rebuild(now)
	return tea.Batch(reads, m.nextMove())
}

// hide stops what runs only while the tab shows. A read under way runs to its end.
func (m *Model) hide() {
	m.hidden, m.moving = true, false
	m.moveClock++
}

// nextMove waits out one drawing of the strip, where the board asks for one and nothing
// else is waiting.
func (m *Model) nextMove() tea.Cmd {
	board, ok := m.board.(MovingBoard)
	if !ok || m.hidden || m.moving || m.reading != nil {
		return nil
	}
	every := board.MoveEvery()
	if every <= 0 {
		return nil
	}
	m.moving = true
	clock := m.moveClock
	return tea.Tick(every, func(at time.Time) tea.Msg { return moveMsg{model: m, clock: clock, at: at} })
}

// moved draws the strip at this beat and leaves the table as it stands. A reader over the
// board ends the clock, and the key that closes it starts another.
func (m *Model) moved(at time.Time) tea.Cmd {
	m.moving = false
	if m.reading != nil {
		return nil
	}
	height := len(m.frame.Strip)
	m.drawStrip(at)
	if len(m.frame.Strip) == height {
		m.stripOnly = true
	} else {
		m.follow()
	}
	return m.nextMove()
}

// layOutCharts lays the frame's Charts out at the screen's size, so a beat of the strip reuses them.
func (m *Model) layOutCharts() {
	m.charts = nil
	if m.frame.Charts == nil {
		return
	}
	for _, line := range m.frame.Charts(m.width, m.height) {
		m.charts = append(m.charts, ansi.Truncate(line.Paint(m.pal), m.width, "…"))
	}
}

type screenHeights struct{ strip, charts, body, footer int }

// heights shares the screen's lines out in the order they draw. The table keeps a line, so the
// charts give theirs up first.
func (m *Model) heights() screenHeights {
	h := screenHeights{strip: len(m.frame.Strip), footer: len(m.footer())}
	h.charts = min(len(m.charts), max(0, m.height-h.strip-h.footer-1))
	h.body = max(1, m.height-h.strip-h.charts-h.footer)
	return h
}

// drawStrip puts a moving board's strip in place of its frame's.
func (m *Model) drawStrip(now time.Time) {
	if board, ok := m.board.(MovingBoard); ok {
		m.frame.Strip = board.StripAt(now)
	}
}

func (m *Model) dueReads(now time.Time) tea.Cmd {
	if m.hidden {
		return nil
	}
	var cmds []tea.Cmd
	for _, source := range m.board.Sources() {
		if source.Claim(now) {
			cmds = append(cmds, func() tea.Msg {
				source.Refresh(m.ctx)
				return refreshedMsg{}
			})
		}
	}
	return tea.Batch(cmds...)
}

// Update is Bubble Tea's.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.update(msg)
	m.tellCursor()
	return m, cmd
}

// tellCursor says where the cursor is to a board that follows it, once per move.
func (m *Model) tellCursor() {
	board, ok := m.board.(CursorBoard)
	if !ok || m.cursor == m.toldCursor {
		return
	}
	m.toldCursor = m.cursor
	board.CursorOn(m.cursor)
}

func (m *Model) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if !m.hidden {
			m.layOutCharts()
		}
		m.follow()
		if m.reading != nil {
			m.reading.resize(m.width, m.height)
		}
	case tickMsg:
		now := time.Time(msg)
		m.rebuild(now)
		return tea.Batch(tick(), m.dueReads(now), m.nextMove())
	case moveMsg:
		if msg.model == m && msg.clock == m.moveClock {
			return m.moved(msg.at)
		}
	case refreshedMsg:
		m.rebuild(time.Now())
	case tea.PasteMsg:
		switch m.mode() {
		case readingMode:
			m.reading.paste(msg.Content)
		case askingMode:
			if m.asking.Typed {
				m.answer.paste(msg.Content)
			}
		case filteringMode:
			m.filterLine.paste(msg.Content)
			m.setFilter(m.filterLine.String())
		}
	case tea.KeyPressMsg:
		// A footer that grew, as a question or a long answer makes it, keeps the cursor in view.
		footer := len(m.footer())
		cmd := m.pressed(msg)
		if len(m.footer()) != footer {
			m.follow()
		}
		return tea.Batch(cmd, m.nextMove())
	}
	return nil
}

// Action is one row of the key table the handler, the footer and the ? overlay
// all read.
type Action struct {
	Keys   []string
	Label  string
	Footer string
	Run    func(*Model) tea.Cmd
	// Applies says whether this view can use the key now. Nil means always.
	// A board's key hides the table's action for it even while Applies is false.
	Applies func(*Model) bool
}

func actionTable() []Action {
	moved := func(by func(*Model) int) func(*Model) tea.Cmd {
		return func(m *Model) tea.Cmd {
			m.move(by(m))
			return nil
		}
	}
	halfScreen := func(direction int) func(*Model) tea.Cmd {
		return func(m *Model) tea.Cmd {
			if m.onOpenText() {
				m.ScrollBy(direction * m.HalfScreen())
			} else {
				m.move(direction * m.HalfScreen())
			}
			return nil
		}
	}
	scrolled := func(lines int) func(*Model) tea.Cmd {
		return func(m *Model) tea.Cmd {
			m.ScrollBy(lines)
			return nil
		}
	}
	jumped := func(direction int) func(*Model) tea.Cmd {
		return func(m *Model) tea.Cmd {
			m.jump(direction)
			return nil
		}
	}
	return []Action{
		{Keys: []string{"j", "down"}, Label: "move down", Footer: "j/k move", Run: moved(func(*Model) int { return 1 })},
		{Keys: []string{"k", "up"}, Label: "move up", Run: moved(func(*Model) int { return -1 })},
		{Keys: []string{"]"}, Label: "jump to the next group", Footer: "[/] jump", Run: jumped(1), Applies: (*Model).hasGroups},
		{Keys: []string{"["}, Label: "jump to the previous group", Run: jumped(-1), Applies: (*Model).hasGroups},
		{Keys: []string{"g", "home"}, Label: "go to the top", Run: moved(func(m *Model) int { return -len(m.nodes()) })},
		{Keys: []string{"G", "end"}, Label: "go to the bottom", Run: moved(func(m *Model) int { return len(m.nodes()) })},
		{Keys: []string{"ctrl+d", "pgdown"}, Label: "half a screen down the open text, or down the rows", Run: halfScreen(1)},
		{Keys: []string{"ctrl+u", "pgup"}, Label: "half a screen up the open text, or up the rows", Run: halfScreen(-1)},
		{Keys: []string{"enter"}, Label: "fold a group, or open a row, a tall document full screen", Footer: "enter open", Run: (*Model).toggle, Applies: (*Model).cursorToggles},
		{Keys: []string{"space"}, Label: "fold a group, or open or close a row's text in place", Footer: "space fold", Run: (*Model).toggleInPlace, Applies: (*Model).cursorToggles},
		// The way to the other keys and the way out sit ahead of what a narrow footer cuts.
		{Keys: []string{"?"}, Label: "show these keys and every read", Footer: "? keys", Run: func(m *Model) tea.Cmd {
			m.overlay = true
			return nil
		}},
		{Keys: []string{"q", "ctrl+c"}, Label: "quit", Footer: "q quit", Run: func(m *Model) tea.Cmd {
			m.cancel()
			return tea.Quit
		}},
		{Keys: []string{"J"}, Label: "scroll the open text down a line", Footer: "J/K scroll", Run: scrolled(1), Applies: (*Model).onOpenText},
		{Keys: []string{"K"}, Label: "scroll the open text up a line", Run: scrolled(-1), Applies: (*Model).onOpenText},
		{
			Keys: []string{"v"}, Label: "cycle the grouping", Footer: "v group", Run: (*Model).nextGrouping,
			Applies: func(m *Model) bool { return len(m.board.Groupings()) > 1 },
		},
		{
			Keys: []string{"f"}, Label: "show or hide the summaries under rows and groups", Footer: "f summaries",
			Run: func(m *Model) tea.Cmd {
				m.display.under = !m.display.under
				m.displayChanged()
				return nil
			},
			Applies: func(m *Model) bool {
				_, widths := m.laidOut()
				return anyUnder(m.frame.Groups, widths, 0)
			},
		},
		{
			Keys: []string{"p"}, Label: "show or hide the side panel", Footer: "p panel",
			Run: func(m *Model) tea.Cmd {
				m.panelOff = !m.panelOff
				return nil
			},
			Applies: (*Model).panelFits,
		},
		{Keys: []string{"/"}, Label: "filter the rows, matching their detail too", Footer: "/ filter", Run: func(m *Model) tea.Cmd {
			m.filtering = true
			m.filterLine.set(m.display.filter)
			return nil
		}},
		{Keys: []string{"R"}, Label: "read every source again now", Footer: "R read", Run: func(m *Model) tea.Cmd {
			for _, source := range m.board.Sources() {
				source.Force()
			}
			m.said = "reading every source again"
			return m.dueReads(time.Now())
		}},
	}
}

// tableActions is what holds this model's keys, then the table's own.
func (m *Model) tableActions() []Action {
	return append(slices.Clone(m.hosted), actionTable()...)
}

// actions is every key this view can use now: the handler, the footer and the
// overlay all read it, so a key the view cannot use is in none of them.
func (m *Model) actions() []Action {
	return slices.DeleteFunc(m.boardThenTableActions(), func(action Action) bool {
		return action.Applies != nil && !action.Applies(m)
	})
}

// boardThenTableActions is the board's keys, then the table's that none of them took.
// An action losing its first key also loses its place in the footer, where the
// board's stands.
func (m *Model) boardThenTableActions() []Action {
	keyed, ok := m.board.(Keyed)
	if !ok {
		return m.tableActions()
	}
	own := keyed.Actions()
	taken := map[string]bool{}
	for _, action := range own {
		for _, key := range action.Keys {
			taken[key] = true
		}
	}
	all := slices.Clone(own)
	for _, action := range m.tableActions() {
		left := slices.DeleteFunc(slices.Clone(action.Keys), func(key string) bool { return taken[key] })
		if len(left) == 0 {
			continue
		}
		if taken[action.Keys[0]] {
			action.Footer = ""
		}
		action.Keys = left
		all = append(all, action)
	}
	return all
}

// Cursor is the key of the group or row the cursor is on, for a board's own keys.
func (m *Model) Cursor() string { return m.cursor }

// Toggle is what enter does, for a board's key that takes enter and acts only on some
// rows: a row's Document that runs past the screen opens on a Reader, open in place or
// not, and anything else folds or opens in place. Bind it to enter alone and leave space
// to the table, which folds in place however tall the text. A row open in place that turns
// tall closes only on that space.
func (m *Model) Toggle() tea.Cmd { return m.toggle() }

// Reveal scrolls the cursor's row and what opens under it into view, for a board's key
// that opens text under a row.
func (m *Model) Reveal() { m.revealDetail() }

// Say puts text where the footer says what the last key did.
func (m *Model) Say(text string) { m.said = text }

// Restart puts the cursor back at the top and reads every source now, for a board's
// key that changes what the view shows.
func (m *Model) Restart() tea.Cmd {
	m.cursor, m.cursorAt, m.top, m.placed, m.scrolled = "", 0, 0, false, false
	for _, source := range m.board.Sources() {
		source.Force()
	}
	return m.reveal(time.Now())
}

// Rebuild is for a key that changed what the board shows from what it already holds.
func (m *Model) Rebuild() {
	now := time.Now()
	m.cue.hush(now)
	m.rebuild(now)
}

// ToggleEveryRow opens every showing row's detail, or closes them all where every
// one is open already, so a whole group reads as one document.
func (m *Model) ToggleEveryRow() tea.Cmd {
	var rows []string
	allOpen := true
	for _, n := range m.nodes() {
		if n.kind == nodeRow && n.toggles {
			rows = append(rows, n.key)
			allOpen = allOpen && n.open
		}
	}
	for _, key := range rows {
		m.display.opened[key] = !allOpen
	}
	m.displayChanged()
	return nil
}

// displayChanged is for a key that changes what shows. It ends any scroll through the text,
// which would otherwise leave the cursor off the screen, and brings the cursor into view.
func (m *Model) displayChanged() {
	m.scrolled = false
	m.follow()
}

// ScrollBy moves the screen over the table by lines and leaves the cursor, for
// reading through a row's detail taller than the screen. Moving the cursor ends it.
func (m *Model) ScrollBy(lines int) {
	m.scrolled = true
	m.top += lines
	m.follow()
}

// HalfScreen is half the table's height in lines.
func (m *Model) HalfScreen() int { return max(1, m.bodyHeight()/2) }

// Read opens document on a full-screen Reader over the board, until esc or space closes it.
// The reader's top line names the groups holding the cursor's line, then title.
func (m *Model) Read(title string, document Document) {
	m.reading = NewReader(title, document, m.pal)
	m.reading.path = m.cursorPath()
	m.reading.back = func() { m.reading = nil }
	m.reading.quit = func() tea.Cmd {
		m.cancel()
		return tea.Quit
	}
	m.reading.resize(m.width, m.height)
}

// Tall says document runs past the screen, so enter reads it on a Reader.
func (m *Model) Tall(document Document) bool {
	return len(document(m.tableWidth())) >= m.bodyHeight()
}

// mode is what takes the keys, a paste and the footer's first lines.
type mode int

const (
	tableMode mode = iota
	readingMode
	askingMode
	overlayMode
	filteringMode
)

// mode is the one open over the table. Each opens only from the table's keys, so at most one is.
func (m *Model) mode() mode {
	switch {
	case m.reading != nil:
		return readingMode
	case m.asking != nil:
		return askingMode
	case m.overlay:
		return overlayMode
	case m.filtering:
		return filteringMode
	}
	return tableMode
}

func (m *Model) pressed(key tea.KeyPressMsg) tea.Cmd {
	switch m.mode() {
	case readingMode:
		return m.reading.pressed(key)
	case askingMode:
		return m.answering(key)
	case overlayMode:
		m.overlay = false
		return nil
	case filteringMode:
		m.typed(key)
		return nil
	}
	m.said = ""
	name := key.String()
	if name == "esc" && m.display.filter != "" {
		m.setFilter("")
		return nil
	}
	for _, action := range m.actions() {
		if slices.Contains(action.Keys, name) {
			return action.Run(m)
		}
	}
	return nil
}

// typed is a key while the filter is typed: enter keeps it, and esc clears it.
func (m *Model) typed(key tea.KeyPressMsg) {
	switch m.filterLine.press(key) {
	case lineEntered:
		m.filtering = false
	case lineCanceled:
		m.filtering = false
		m.setFilter("")
	default:
		if text := m.filterLine.String(); text != m.display.filter {
			m.setFilter(text)
		}
	}
}

func (m *Model) setFilter(filter string) {
	m.display.filter = filter
	m.place()
	m.displayChanged()
}

// toggle is enter: a row's Document that runs past the screen opens on a Reader, open in
// place or not, and anything else folds or opens in place.
func (m *Model) toggle() tea.Cmd {
	if n, ok := m.selected(); ok && n.kind == nodeRow && n.row.Document != nil && m.Tall(n.row.Document) {
		m.Read(n.row.title(), n.row.Document)
		return nil
	}
	return m.toggleInPlace()
}

// toggleInPlace is space: it folds the group or opens the row under the cursor, however
// tall its text, and does nothing where that would change nothing.
func (m *Model) toggleInPlace() tea.Cmd {
	n, ok := m.selected()
	if !ok || !n.toggles {
		return nil
	}
	switch n.kind {
	case nodeGroup:
		m.display.folded[n.key] = n.open
	case nodeRow:
		m.display.opened[n.key] = !n.open
	}
	// A cursor still looking for its starting line stays on the line this key acted on.
	m.placed = true
	m.revealDetail()
	return nil
}

// cursorToggles says enter and space would change something on the cursor's line.
func (m *Model) cursorToggles() bool {
	n, ok := m.selected()
	return ok && n.toggles
}

// title is the row's words, for a Reader to name what it holds.
func (r Row) title() string {
	if r.ReaderTitle != "" {
		return r.ReaderTitle
	}
	if len(r.Span) > 0 {
		return r.Span.Text()
	}
	texts := make([]string, 0, len(r.Cells))
	for _, cell := range r.Cells {
		texts = append(texts, cell.Text)
	}
	return strings.Join(texts, "  ")
}

// revealDetail scrolls the cursor's row and what opens under it into view, so a row
// opened near the screen's foot shows its text, stopping with the row at the top.
func (m *Model) revealDetail() {
	m.displayChanged()
	pinned, lines := m.table()
	at := slices.IndexFunc(lines, func(l drawn) bool { return l.key != "" && l.key == m.cursor })
	if at < 0 {
		return
	}
	end := at
	for end+1 < len(lines) && lines[end+1].key == "" {
		end++
	}
	h := m.scrollHeight(pinned)
	if end >= m.top+h {
		m.top = min(at, end-h+1)
	}
	m.clearOfBand(lines, at, h)
}

// cursorPath is the words of each group the sticky band would name over the cursor's line,
// outermost first.
func (m *Model) cursorPath() []Line {
	n, ok := m.selected()
	if !ok {
		return nil
	}
	nodes := m.nodes()
	var path []Line
	for at := n.parent; at >= 0; at = nodes[at].parent {
		if nodes[at].names {
			path = append(path, groupWords(nodes[at].group))
		}
	}
	slices.Reverse(path)
	return path
}

func (m *Model) nextGrouping() tea.Cmd {
	if groupings := m.board.Groupings(); len(groupings) > 0 {
		m.grouping = (m.grouping + 1) % len(groupings)
		m.said = "grouped by " + groupings[m.grouping]
		m.Rebuild()
		m.displayChanged()
	}
	return nil
}

func (m *Model) currentGrouping() string {
	if groupings := m.board.Groupings(); len(groupings) > 0 {
		return groupings[m.grouping]
	}
	return ""
}

func (m *Model) rebuild(now time.Time) {
	wasLast := m.tails() && m.placed && m.onLastRow()
	m.frame = m.board.Frame(now, m.currentGrouping())
	m.display.matched.newFrame()
	m.drawStrip(now)
	m.layOutCharts()
	m.panelHasText = anyPanelText(m.frame.Groups)
	m.cue.mark(m.frame.Groups, now)
	if settled := m.cue.quietFrom.Add(settleFor); m.cue.quiet && !now.Before(settled) && !m.readsUnderWayFrom(settled) {
		m.cue.quiet = false
	}
	m.place()
	if wasLast {
		m.moveToLastRow()
	}
	m.follow()
}

// readsUnderWayFrom reports whether a read that started by then has not returned yet.
func (m *Model) readsUnderWayFrom(at time.Time) bool {
	return slices.ContainsFunc(m.board.Sources(), func(source Refresher) bool {
		status := source.Status()
		return status.Reading && !status.TriedAt.After(at)
	})
}

func (m *Model) tails() bool {
	board, ok := m.board.(TailBoard)
	return ok && board.Tails()
}

func (m *Model) onLastRow() bool {
	rows := selectableOf(m.nodes())
	return len(rows) > 0 && rows[len(rows)-1].key == m.cursor
}

func (m *Model) moveToLastRow() {
	if rows := selectableOf(m.nodes()); len(rows) > 0 {
		m.cursorAt = len(rows) - 1
		m.cursor = rows[m.cursorAt].key
	}
}

func (m *Model) nodes() []node {
	nodes, _ := m.laidOut()
	return nodes
}

// laidOut is what shows of the frame at the table's width, and the width each column takes.
func (m *Model) laidOut() ([]node, []int) {
	return m.display.nodes(m.frame.Groups, m.frame.Columns, m.tableWidth())
}

func selectableOf(nodes []node) []node {
	return slices.DeleteFunc(slices.Clone(nodes), func(n node) bool { return !n.selectable() })
}

// place puts the cursor on its key, or on where it starts the first time there is such a
// line, or on the line it was on where its row is gone.
func (m *Model) place() {
	rows := selectableOf(m.nodes())
	if len(rows) == 0 {
		m.cursor, m.cursorAt = "", 0
		return
	}
	if !m.placed {
		if i := slices.IndexFunc(rows, func(n node) bool { return n.key != "" && n.key == m.frame.StartAt }); i >= 0 {
			m.cursor, m.cursorAt, m.placed = rows[i].key, i, true
			return
		}
		if i := m.startingLine(rows); i >= 0 {
			m.cursor, m.cursorAt, m.placed = rows[i].key, i, true
			return
		}
	}
	if i := slices.IndexFunc(rows, func(n node) bool { return n.key == m.cursor }); i >= 0 {
		m.cursorAt = i
		return
	}
	m.cursorAt = max(0, min(m.cursorAt, len(rows)-1))
	m.cursor = rows[m.cursorAt].key
}

// startingLine is the first line holding cells, or the last on a board read from its
// newest, and otherwise the first with anything to read. The cursor starts past a group
// with no Panel or Detail, because the panel would only repeat its label.
func (m *Model) startingLine(rows []node) int {
	holdsCells := func(n node) bool {
		if n.kind == nodeGroup {
			return len(n.group.Cells) > 0 && n.group.hasPanelText()
		}
		return len(n.row.Cells) > 0
	}
	i := slices.IndexFunc(rows, holdsCells)
	if m.tails() {
		for at := len(rows) - 1; at >= 0; at-- {
			if holdsCells(rows[at]) {
				return at
			}
		}
	}
	if i >= 0 {
		return i
	}
	return slices.IndexFunc(rows, func(n node) bool { return n.kind == nodeRow || n.group.hasPanelText() })
}

func (m *Model) selected() (node, bool) {
	rows := selectableOf(m.nodes())
	if m.cursorAt < 0 || m.cursorAt >= len(rows) {
		return node{}, false
	}
	return rows[m.cursorAt], true
}

func (m *Model) move(by int) {
	rows := selectableOf(m.nodes())
	if len(rows) == 0 {
		return
	}
	m.cursorAt = max(0, min(m.cursorAt+by, len(rows)-1))
	m.cursor = rows[m.cursorAt].key
	m.placed = true
	m.scrolled = false
	m.follow()
}

// jump moves to the next or previous group at the cursor's depth or shallower. Going
// back from inside a group lands on its own heading first, as Vim's [[ does.
func (m *Model) jump(direction int) {
	rows := selectableOf(m.nodes())
	if m.cursorAt < 0 || m.cursorAt >= len(rows) {
		return
	}
	level := rows[m.cursorAt].depth
	if rows[m.cursorAt].kind == nodeRow {
		level--
	}
	for at := m.cursorAt + direction; at >= 0 && at < len(rows); at += direction {
		if rows[at].kind == nodeGroup && rows[at].depth <= level {
			m.move(at - m.cursorAt)
			return
		}
	}
}

func (m *Model) hasGroups() bool {
	return slices.ContainsFunc(m.nodes(), func(n node) bool { return n.kind == nodeGroup && n.selectable() })
}

// onOpenText says the cursor's row or group shows text under it, which the half-screen
// keys and J and K then read through.
func (m *Model) onOpenText() bool {
	n, ok := m.selected()
	if !ok {
		return false
	}
	if n.kind == nodeGroup {
		return n.group.Document != nil
	}
	return n.open
}

// panelFits says the screen has room for the side panel and this frame has text for it.
func (m *Model) panelFits() bool { return m.width >= PanelFrom && m.panelHasText }

// panelWidth grows with the screen up to ReadingWidth. A frame with nothing for the panel to
// say gives the table its width.
func (m *Model) panelWidth() int {
	if m.panelOff || !m.panelFits() {
		return 0
	}
	return min(ReadingWidth, m.width*3/10)
}

func (m *Model) tableWidth() int {
	if p := m.panelWidth(); p > 0 {
		return m.width - p - 3
	}
	return m.width
}

func (m *Model) bodyHeight() int { return m.heights().body }

// table is the laid-out table: the lines that stay on screen, and those that scroll.
func (m *Model) table() (pinned, scrolling []drawn) {
	nodes, widths := m.laidOut()
	return splitPinned(draw(m.pal, m.frame.Columns, nodes, widths, m.tableWidth(), m.cursor))
}

// scrollHeight is what the lines that stay on screen leave for those that scroll.
func (m *Model) scrollHeight(pinned []drawn) int { return max(1, m.bodyHeight()-len(pinned)) }

func (m *Model) follow() {
	pinned, lines := m.table()
	h := m.scrollHeight(pinned)
	at := slices.IndexFunc(lines, func(l drawn) bool { return l.key != "" && l.key == m.cursor })
	if at >= 0 && !m.scrolled {
		if at < m.top {
			m.top = at
		}
		if at >= m.top+h {
			m.top = at - h + 1
		}
	}
	m.top = max(0, min(m.top, len(lines)-h))
	if at >= 0 && !m.scrolled {
		m.clearOfBand(lines, at, h)
	}
}

// clearOfBand moves the screen up until the line at sits below the band, which is no cursor stop.
func (m *Model) clearOfBand(lines []drawn, at, height int) {
	for m.top > 0 && at < m.top+len(stickyBand(lines, m.top, height)) {
		m.top--
	}
}

// footer is an open question and the keys that answer it, or else the filter being typed, what
// the last key did, or the filter kept, then the keys. What a key did shows until the next key,
// so a kept filter never hides a refusal.
func (m *Model) footer() []string {
	if m.mode() == askingMode {
		lines, keys := m.questionLines()
		return append(lines, ansi.Truncate(keys, m.width, "…"))
	}
	var lines []string
	switch {
	case m.mode() == filteringMode:
		lines = []string{m.filterLine.draw("/", m.width)}
	case m.said != "":
		lines = m.wrapped(m.said)
	case m.display.filter != "":
		lines = []string{ansi.Truncate("filter (esc clears): "+m.display.filter, m.width, "…")}
	}
	var keys []string
	for _, action := range m.actions() {
		if action.Footer != "" {
			keys = append(keys, action.Footer)
		}
	}
	return append(lines, ansi.Truncate(strings.Join(keys, "  "), m.width, "…"))
}

// View is Bubble Tea's.
func (m *Model) View() tea.View {
	v := tea.NewView(m.screen())
	v.AltScreen = true
	return v
}

func (m *Model) screen() string {
	if m.reading != nil {
		return m.reading.screen()
	}
	if !m.stripOnly || m.underStrip == nil {
		m.underStrip = m.drawUnderStrip()
	}
	m.stripOnly = false
	charts := m.charts[:m.heights().charts]
	out := make([]string, 0, len(m.frame.Strip)+len(charts)+len(m.underStrip))
	for _, line := range m.frame.Strip {
		out = append(out, ansi.Truncate(line.Paint(m.pal), m.width, "…"))
	}
	out = append(out, charts...)
	return strings.Join(append(out, m.underStrip...), "\n")
}

// drawUnderStrip is the table or the keys overlay, then the footer.
func (m *Model) drawUnderStrip() []string {
	var out []string
	h := m.bodyHeight()
	if m.overlay {
		out = append(out, m.keysOverlay(h)...)
		return append(out, m.footer()...)
	}
	pinned, lines := m.table()
	for _, line := range pinned {
		out = append(out, line.text)
	}
	h = m.scrollHeight(pinned)
	shown := lines[min(m.top, len(lines)):min(m.top+h, len(lines))]
	band := stickyBand(lines, m.top, h)
	panel := m.panelLines(h)
	tw := m.tableWidth()
	for i := range h {
		left := ""
		switch {
		case i < len(band):
			left = band[i]
		case i < len(shown):
			left = shown[i].text
		}
		if m.panelWidth() == 0 {
			out = append(out, left)
			continue
		}
		left += strings.Repeat(" ", max(tw-ansi.StringWidth(left), 0))
		right := ""
		if i < len(panel) {
			right = panel[i]
		}
		out = append(out, strings.TrimRight(left+" │ "+right, " "))
	}
	return append(out, m.footer()...)
}

func (m *Model) panelLines(height int) []string {
	width := m.panelWidth()
	if width == 0 {
		return nil
	}
	n, ok := m.selected()
	if !ok {
		return nil
	}
	lines := m.panelText(n, width)
	if len(lines) > height {
		lines = lines[:height]
	}
	return lines
}

// panelText is a node's panelSource, and a group's label and summary where it holds neither.
func (m *Model) panelText(n node, width int) []string {
	var authored Document
	var cells []Cell
	switch n.kind {
	case nodeGroup:
		authored, cells = n.group.panelSource()
		if len(cells) == 0 {
			cells = []Cell{C(groupLabel(n.group).Text(), Bold), C(n.group.Summary.Text(), "")}
		}
	case nodeRow:
		authored, cells = n.row.panelSource()
	}
	var lines []string
	if authored != nil {
		for _, line := range authored(width) {
			for _, piece := range wrapWords(line, width) {
				lines = append(lines, piece.Paint(m.pal))
			}
		}
		return lines
	}
	for _, c := range cells {
		for _, piece := range wrap(c.Text, width) {
			lines = append(lines, m.pal.Paint(c.Code, piece))
		}
	}
	return lines
}

func (m *Model) keysOverlay(height int) []string {
	lines := []string{m.pal.Bold("Keys, any key closes this")}
	for _, action := range m.actions() {
		lines = append(lines, "  "+fit(strings.Join(action.Keys, " "), 16, false)+"  "+action.Label)
	}
	lines = append(lines, m.readLines()...)
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines[:height]
}

// readLines is every source's standing, which the strip names only when one is late or
// failing.
func (m *Model) readLines() []string {
	sources := m.board.Sources()
	if len(sources) == 0 {
		return nil
	}
	now := time.Now()
	lines := []string{"", m.pal.Bold("Reads")}
	for _, source := range sources {
		status := source.Status()
		cell := readCell(status, now)
		line := "  " + fit(status.Name, 16, false) + "  " + m.pal.Paint(cell.Code, fit(cell.Text, 16, false)) +
			"  every " + fit(status.Every, 8, false) + "  took " + fit(status.Took, 8, false)
		switch {
		case status.Fault != "":
			line += "  " + m.pal.Paint(Red, status.Fault)
		case status.Note != "":
			line += "  " + status.Note
		}
		lines = append(lines, ansi.Truncate(line, m.width, "…"))
	}
	return lines
}
