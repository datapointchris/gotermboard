package gotermboard

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermstyle"

	"github.com/datapointchris/gotermboard/printedframe"
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

// LastRowBoard is a Board told whether the cursor sits on the last row the table shows, once
// each time that changes. The filter and the folds decide which row that is, so a board holding
// back rows that land while the reader is scrolled back, as less +F stops following once the
// reader scrolls, lets them in when the reader is back at the end of what shows. A table with
// no row to sit on counts as on its last, because nothing in it is scrolled back from.
type LastRowBoard interface {
	CursorOnLastRow(on bool)
}

// TwoLineBoard is a Board whose rows' Under summaries start shown. f still hides them.
type TwoLineBoard interface {
	TwoLines() bool
}

// LookBoard is a Board drawn in a Look of its own rather than the default, whose zero Look
// draws nothing differently.
type LookBoard interface {
	// Look is how the board draws on a screen this wide, read again as the screen changes size.
	Look(width int) Look
}

// Look is how a board's screen is laid out where it does not take the default.
type Look struct {
	// Sides is the blank columns at each side of the screen, and Ends the blank lines above
	// and below it. A screen too small for them keeps a column and a line to draw in.
	Sides, Ends int
	// Spread fills the table's width with its columns at their own widths, dealing what they
	// leave out between them from the left, rather than the last column taking it. A column
	// with no title continues the one before it and takes none of the spread. A table whose
	// rows or groups set WrapLast keeps the slack in its last column, where the text wraps.
	Spread bool
	// Rules puts the column titles on a line of their own with a rule under it, and a rule
	// above the footer.
	Rules bool
	// BoldCursor draws the cursor's row bold over the theme's selection, as it is where there
	// is none.
	BoldCursor bool
	// NoRowChevrons leaves a row's chevron cell blank, for a board whose every row opens, where
	// a chevron tells no row apart. A group's chevron still draws, and no column moves.
	NoRowChevrons bool
}

// CursorBoard is a Board told which key the cursor is on, for a read that follows it.
type CursorBoard interface {
	CursorOn(key string)
}

// ReceivingBoard is a Board handed every message the model does not handle itself, such as the
// one a command its key started sends when it ends. Receive runs with the model, so it can Say
// what came of the command, Rebuild, or return the next command. Tabs hands such a message to
// every tab, because the tab whose key started the command may be hidden by the time it ends,
// so a board ignores a message of a type it does not own. A message whose type names where it
// goes is the exception: a HandedBack reaches only the board whose model handed the terminal
// over, and a ClaudeSessionOutcome only the board of the tab showing.
type ReceivingBoard interface {
	Receive(m *Model, msg tea.Msg) tea.Cmd
}

// BusyBoard is a Board with work under way, such as a command its key started. The footer
// names it after a spinner, above what the last key did, until UnderWay is empty again. A key
// never clears it.
type BusyBoard interface {
	UnderWay() string
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

// PanelFrom is the narrowest width inside a board's Look margin that gets the side panel. On
// a board with no Look, that is the screen's width.
const PanelFrom = 160

// tickEvery redraws the ages and starts what is due.
const tickEvery = time.Second

// Model is the Bubble Tea program of one view. Tabs holds one per tab.
type Model struct {
	board   Board
	painter painter
	ctx     context.Context
	cancel  context.CancelFunc

	frame Frame
	// drawnFrom is what the reads behind frame said when it was built.
	drawnFrom readings
	// width and height are what the board draws in, inside its Look's margin, and margin is
	// that margin: Sides and Ends as the screen's size leaves them.
	width    int
	height   int
	margin   Look
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
	// hosted are the keys of whatever holds this model, ahead of the table's. ahead are its keys
	// ahead of the board's own, which take their keys only while they apply.
	hosted []Action
	ahead  []Action
	// seeking is what a jump came for, until a line names it or the reads say none does.
	seeking *seek
	cue     changeCue
	// toldCursor is the key a CursorBoard last heard.
	toldCursor string
	// toldLastRow is what a LastRowBoard last heard, once lastRowTold says it heard anything.
	toldLastRow, lastRowTold bool
	// layout is the frame's layout and its line index, which a new frame drops. nodesHint is how
	// many nodes the last walk made, which sizes the next walk's slice.
	layout    *keptLayout
	nodesHint int
	// frameCount numbers the frames taken, so an answer kept about one frame knows a newer one.
	frameCount uint64
	// looked is the last frame's answer to which line names what the tabs are locked to.
	looked lookedFor
	// footerDrawn is how many lines the footer under the strip took at the last draw.
	footerDrawn int
	// reading is the full-screen Reader over the board, while one is open.
	reading *Reader
	// moveClock numbers the strip's clock. A hide moves it on, so a beat sent before the hide
	// is dropped. moving says a beat is on its way.
	moveClock int
	moving    bool
	// spinning says the spinner's next beat is on its way, and spinAt is the drawing it shows.
	spinning bool
	spinAt   int
	// underStrip is what the last draw put under the strip, and stripOnly says the next draw
	// keeps it, because only a move came in since.
	underStrip []string
	stripOnly  bool
	// charts is the frame's Charts laid out at the screen's size. Each draw paints them, so a
	// reload recolors them as it does the rest.
	charts []Line
}

// NewModel is a model reading board, sized until the terminal says otherwise. It draws in
// the TUI theme file gotermstyle's tuitheme finds, and reads it again on SIGUSR2 while it runs
// as the program. A file that will not load at start leaves ANSI 0-15, and on a reload keeps
// the colors on screen. The footer says why.
func NewModel(board Board, pal gotermstyle.Palette) *Model {
	ctx, cancel := context.WithCancel(context.Background())
	theme, err := loadTheme()
	m := newModel(board, painter{pal: pal, theme: theme}, ctx, cancel)
	m.sayThemeFault(err)
	m.rebuild(time.Now())
	return m
}

func newModel(board Board, p painter, ctx context.Context, cancel context.CancelFunc) *Model {
	twoLines, ok := board.(TwoLineBoard)
	m := &Model{
		board: board, painter: p, ctx: ctx, cancel: cancel, cue: newChangeCue(),
		display: display{folded: map[string]bool{}, opened: map[string]bool{}, matched: newRowMatches(), under: ok && twoLines.TwoLines()},
	}
	m.resize(80, 24)
	return m
}

// look is the board's Look at the screen's width, and the zero Look for a board with none.
func (m *Model) look() Look {
	if board, ok := m.board.(LookBoard); ok {
		return board.Look(m.width + 2*m.margin.Sides)
	}
	return Look{}
}

// resize takes a screen of width by height, and draws inside the margin the board's Look asks
// for, less what the screen cannot spare.
func (m *Model) resize(width, height int) {
	m.margin = Look{}
	if board, ok := m.board.(LookBoard); ok {
		look := board.Look(width)
		m.margin = Look{Sides: min(max(look.Sides, 0), max((width-1)/2, 0)), Ends: min(max(look.Ends, 0), max((height-1)/2, 0))}
	}
	m.width, m.height = width-2*m.margin.Sides, height-2*m.margin.Ends
}

// framed is the screen's lines inside the margin.
func (m *Model) framed(lines string) string {
	if m.margin == (Look{}) {
		return lines
	}
	side := strings.Repeat(" ", m.margin.Sides)
	var b strings.Builder
	b.WriteString(strings.Repeat("\n", m.margin.Ends))
	for i, line := range strings.Split(lines, "\n") {
		if i > 0 {
			b.WriteString("\n")
		}
		if line != "" {
			b.WriteString(side + line)
		}
	}
	b.WriteString(strings.Repeat("\n", m.margin.Ends))
	return b.String()
}

// sayThemeFault puts why the theme file would not load on the footer.
func (m *Model) sayThemeFault(err error) {
	if err != nil {
		m.said = err.Error()
	}
}

// Once is one frame at a size, drawn from what the board's sources already hold.
func Once(board Board, pal gotermstyle.Palette, width, height int) string {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	theme, err := loadTheme()
	m := newModel(board, painter{pal: pal, theme: theme}, ctx, cancel)
	m.sayThemeFault(err)
	m.resize(width, height)
	m.rebuild(time.Now())
	m.tellCursor()
	return m.screen()
}

// OnceSize is the size one printed frame draws at: COLUMNS by LINES, then the terminal's
// own size, then the fallback. It is [printedframe.Size].
func OnceSize(fallbackWidth, fallbackHeight int) (width, height int) {
	return printedframe.Size(fallbackWidth, fallbackHeight)
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
	// spinMsg is one beat of the spinner beside a busy board's work, for the model that started
	// it, on the strip's clock, which a hide moves on.
	spinMsg struct {
		model *Model
		clock int
	}
)

// spinner is the drawings the footer turns through beside work under way, each one cell wide.
var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// SpinEvery is how long a spinner holds each drawing: the footer's beside work under way, and
// one a board turns on its own clock.
const SpinEvery = 100 * time.Millisecond

// SpinnerAt is the spinner's drawing at now, one cell wide, for a board that turns one on its
// own clock, such as a MovingBoard's strip while a read runs. It turns through the footer's
// drawings at the footer's pace.
func SpinnerAt(now time.Time) string {
	return spinner[now.UnixMilli()/SpinEvery.Milliseconds()%int64(len(spinner))]
}

func tick() tea.Cmd {
	return tea.Tick(tickEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Init starts every source's first read and the clock.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(tick(), m.reveal(time.Now()), m.painter.theme.watch())
}

// reveal draws the last good reads, with no change cue, and starts what is due. The reads
// claim before the frame is built, so the frame sees the quiet last until they return.
func (m *Model) reveal(now time.Time) tea.Cmd {
	m.hidden = false
	m.cue.hush(now)
	reads := m.dueReads(now)
	m.rebuild(now)
	m.tellCursor()
	return tea.Batch(reads, m.nextMove())
}

// hide stops what runs only while the tab shows. A read under way runs to its end. A jump's
// search ends, so a return later never moves the cursor under the reader.
func (m *Model) hide() {
	m.hidden, m.moving, m.spinning = true, false, false
	m.moveClock++
	m.seeking = nil
}

func (m *Model) underWay() string {
	if board, ok := m.board.(BusyBoard); ok {
		return board.UnderWay()
	}
	return ""
}

// nextSpin waits out one drawing of the spinner, while the board has work under way and the
// footer shows.
func (m *Model) nextSpin() tea.Cmd {
	if m.hidden || m.spinning || m.reading != nil || m.underWay() == "" {
		return nil
	}
	m.spinning = true
	clock := m.moveClock
	return tea.Tick(SpinEvery, func(time.Time) tea.Msg { return spinMsg{model: m, clock: clock} })
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
	if m.frame.Charts != nil {
		m.charts = m.frame.Charts(m.width, m.height)
	}
}

type screenHeights struct{ strip, charts, body, footer int }

// heights shares the screen's lines out in the order they draw. The table keeps a line, so the
// charts give theirs up first.
func (m *Model) heights() screenHeights { return m.heightsOver(len(m.footer())) }

// heightsOver is heights over a footer of this many lines, for a draw that has built its footer.
func (m *Model) heightsOver(footer int) screenHeights {
	h := screenHeights{strip: len(m.frame.Strip), footer: footer}
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
	return m, tea.Batch(cmd, m.nextSpin())
}

// tellCursor says where the cursor is to a board that follows it, once per move, and whether it
// sits on the last row to a board that asks, once per change.
func (m *Model) tellCursor() {
	// A hidden tab takes no key, so its cursor holds still, and reveal tells the board on its return.
	if m.hidden {
		return
	}
	if board, ok := m.board.(LastRowBoard); ok {
		k := m.kept()
		on := len(k.ix.stops) == 0 || k.nodes[k.ix.stops[len(k.ix.stops)-1]].key == m.cursor
		if !m.lastRowTold || on != m.toldLastRow {
			m.lastRowTold, m.toldLastRow = true, on
			board.CursorOnLastRow(on)
		}
	}
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
		m.resize(msg.Width, msg.Height)
		// A hidden tab keeps the size and lays out when it shows, where reveal rebuilds it.
		if !m.hidden {
			m.layOutCharts()
			m.follow()
		}
		if m.reading != nil {
			m.reading.resize(m.width, m.height)
		}
	case tickMsg:
		now := time.Time(msg)
		m.rebuildKeeping(now, true)
		return tea.Batch(tick(), m.dueReads(now), m.nextMove())
	case moveMsg:
		if msg.model == m && msg.clock == m.moveClock {
			return m.moved(msg.at)
		}
	case spinMsg:
		if msg.model == m && msg.clock == m.moveClock {
			m.spinning, m.stripOnly = false, false
			m.spinAt = (m.spinAt + 1) % len(spinner)
		}
	case refreshedMsg:
		m.rebuildKeeping(time.Now(), true)
		m.relayReading()
	case themeLoadedMsg:
		return m.painter.theme.reloaded(msg, m.sayThemeFault)
	case tea.PasteMsg:
		switch m.mode() {
		case readingMode:
			m.reading.paste(msg.Content)
		case askingMode:
			if m.asking.typed() {
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
	case HandedBack:
		if msg.model == m {
			return m.receive(msg)
		}
	case ClaudeSessionOutcome:
		if !m.hidden {
			m.said = msg.Said
			return m.receive(msg)
		}
	default:
		return m.receive(msg)
	}
	return nil
}

// receive hands msg to a ReceivingBoard, then runs a HandedBack's then, so what the board
// reads again is due before then says what came of the program. An open reader, an open
// document and the side panel read their documents again after the board's Receive, because
// what the board received may be more of one.
func (m *Model) receive(msg tea.Msg) tea.Cmd {
	footer := 0
	if !m.hidden {
		footer = len(m.footer())
	}
	var cmds []tea.Cmd
	if board, ok := m.board.(ReceivingBoard); ok {
		cmds = append(cmds, board.Receive(m, msg))
		m.relayReading()
		m.rereadDocuments()
	}
	if back, ok := msg.(HandedBack); ok && back.then != nil {
		cmds = append(cmds, back.then(m, back.Err, back.Stderr))
	}
	if !m.hidden && len(m.footer()) != footer {
		m.follow()
	}
	return tea.Batch(cmds...)
}

// TableKeys is every key the built-in table binds. A board's key hides the table's, so a board
// taking keys from its own config asks this which ones would take a key from the table, and
// Tabs.Keys where it is one of several tabs.
func TableKeys() []string {
	var keys []string
	for _, action := range actionTable() {
		keys = append(keys, action.Keys...)
	}
	return keys
}

// actionTable is the universal rows, then the keys only a Model binds.
func actionTable() []Action {
	return append(modelActions(), []Action{
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
			Applies: func(m *Model) bool { return m.kept().ix.anyUnder },
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
	}...)
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

// boardThenTableActions is clearing a kept filter, then the holder's keys, while each applies,
// then the board's keys, then the table's, each less the keys one before it took.
func (m *Model) boardThenTableActions() []Action {
	var own []Action
	if keyed, ok := m.board.(Keyed); ok {
		own = keyed.Actions()
	}
	return keysTakenInOrder(applyingTo(m, filterClearing()), applyingTo(m, m.ahead), own, m.tableActions())
}

// filterClearing is esc while a filter is kept, ahead of every other esc, because a filter is the
// nearest level to back out of. The filter's own line names the key, so it has no footer.
func filterClearing() []Action {
	return []Action{{
		Keys: []string{"esc"}, Label: "clear the filter",
		Run: func(m *Model) tea.Cmd {
			m.setFilter("")
			return nil
		},
		Applies: func(m *Model) bool { return m.display.filter != "" },
	}}
}

// Cursor is the key of the group or row the cursor is on, for a board's own keys.
func (m *Model) Cursor() string { return m.cursor }

// MoveCursorTo puts the cursor on the group or row keyed key, for a board's key that acts on
// the cursor's row and moves on, and says whether key shows. A row in a folded group, or one
// the filter hides, does not. A key that changed what the board holds calls Rebuild first.
func (m *Model) MoveCursorTo(key string) bool {
	at, shows := m.kept().ix.stopAt[key]
	if key == "" || !shows {
		return false
	}
	m.cursor, m.cursorAt, m.placed, m.scrolled = key, at, true, false
	m.follow()
	return true
}

// CloseInPlace closes the text open under the cursor's row, as the table's space does, and
// says whether any was open. A board that binds space to a row verb, such as a mark, calls it
// first, so space still closes a row open in place.
func (m *Model) CloseInPlace() bool {
	n, ok := m.selected()
	if !ok || n.kind != nodeRow || !n.open {
		return false
	}
	m.display.open(n.key, false)
	m.placed = true
	m.revealDetail()
	return true
}

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
	m.toTop()
	return m.reveal(time.Now())
}

// toTop puts the cursor back at the top and has every source read at the next reveal.
func (m *Model) toTop() {
	m.cursor, m.cursorAt, m.top, m.placed, m.scrolled = "", 0, 0, false, false
	for _, source := range m.board.Sources() {
		source.Force()
	}
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
		m.display.open(key, !allOpen)
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
	m.reading = newReader(title, document, m.painter)
	m.reading.path = m.cursorPath()
	m.reading.back = func() { m.reading = nil }
	m.reading.quit = func() tea.Cmd {
		m.cancel()
		return tea.Quit
	}
	m.reading.resize(m.width, m.height)
}

// Follow opens document on a full-screen Reader over the board at its last line, and keeps it
// there as the document grows, as tail -f and less +F do, until esc or space closes it.
// Scrolling up holds the reader where it is, and G follows again. Lines a board's command
// streams into the document show as each message the board receives lands. closed runs as
// esc or space closes the reader, so the board can stop what feeds it.
func (m *Model) Follow(title string, document Document, closed func()) {
	m.Read(title, document)
	m.reading.follows = true
	m.reading.scrollTo(len(m.reading.rows))
	m.reading.back = func() {
		m.reading = nil
		if closed != nil {
			closed()
		}
	}
}

// relayReading lays an open reader out again after a read lands, keeping its place, so text a
// source fills after enter shows without closing it. A reader a row opened reads that row's
// Document in the new frame.
func (m *Model) relayReading() {
	if m.reading == nil {
		return
	}
	if n, ok := m.selected(); ok && m.reading.row != "" && n.kind == nodeRow && n.row.Key == m.reading.row && n.row.Document != nil {
		m.reading.document = n.row.Document
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
	cmd, _ := RunKey(m, m.actions(), key.String())
	return cmd
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
// place or not, and so does one the side panel draws, since opening it in place would only
// draw it twice. Anything else folds or opens in place.
func (m *Model) toggle() tea.Cmd {
	if n, ok := m.selected(); ok && n.kind == nodeRow && n.row.Document != nil && (m.panelDrawsDocumentOf(n.row) || m.Tall(n.row.Document)) {
		m.Read(n.row.title(), n.row.Document)
		m.reading.row = n.row.Key
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
		m.display.fold(n.key, n.open)
	case nodeRow:
		m.display.open(n.key, !n.open)
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
	want := -1
	if revealHook != nil {
		want = revealHook(m)
	}
	k := m.kept()
	if at, _, ok := k.cursorSpan(m.cursor); ok {
		h := m.scrollHeight(k.ix.pinned)
		if end := k.lastOpenedLine(k.ix.stopAt[m.cursor]); end >= m.top+h {
			m.top = min(at, end-h+1)
		}
		m.clearOfBand(k, at, h)
	}
	if revealHook != nil && m.top != want {
		panic(fmt.Sprintf("a reveal scrolled to line %d, and the whole table's draw scrolls to %d", m.top, want))
	}
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

// readings is what a frame's reads said when it was built, which says whether a line missing
// from it is an answer.
type readings struct {
	// landed says every source had read, and none was under way or failing.
	landed           bool
	reading, failing bool
	// due is when the first source comes due again, and zero where none does.
	due time.Time
}

func (m *Model) readNow() readings {
	r := readings{landed: true}
	for _, source := range m.board.Sources() {
		status := source.Status()
		r.reading = r.reading || status.Reading
		r.failing = r.failing || status.Fault != ""
		r.landed = r.landed && !status.Reading && status.Fault == "" && !status.ReadAt.IsZero()
		if due := status.TriedAt.Add(status.every); status.every > 0 && (r.due.IsZero() || due.Before(r.due)) {
			r.due = due
		}
	}
	return r
}

// finding is what a tab's last frame says of one entity.
type finding int

const (
	// found is a line naming it.
	found finding = iota
	// holdsNone is no line naming it, in a frame drawn from reads that had all landed, none
	// of them due since.
	holdsNone
	// unanswered is no line naming it yet: a read was under way or had never run, or one
	// has come due since.
	unanswered
	// readFailed is no line naming it, where a read behind the frame failed, so it may still
	// hold it.
	readFailed
)

// lookFor is the key of the first line in the last frame naming ref, the groups holding it,
// and whether the frame's answer holds at now. A jump's search and the lock's tab line both
// ask it.
func (m *Model) lookFor(ref EntityRef, now time.Time) (string, []string, finding) {
	if m.looked.frame != m.frameCount || m.looked.ref != ref || !m.looked.asked {
		m.looked = lookedFor{frame: m.frameCount, ref: ref, asked: true}
		m.looked.key, m.looked.holding, m.looked.found = entityLine(m.frame.Groups, ref, nil)
	}
	if m.looked.found {
		return m.looked.key, m.looked.holding, found
	}
	switch r := m.drawnFrom; {
	case r.landed && (r.due.IsZero() || now.Before(r.due)):
		return "", nil, holdsNone
	case r.failing && !r.reading:
		return "", nil, readFailed
	}
	return "", nil, unanswered
}

// lookedFor is one frame's answer to which line names ref, which the tab line under a lock asks of
// every hidden tab on every draw.
type lookedFor struct {
	frame   uint64
	ref     EntityRef
	asked   bool
	key     string
	holding []string
	found   bool
}

// cursorShows says the cursor is on a line the table draws.
func (m *Model) cursorShows() bool {
	if !m.placed {
		return false
	}
	_, shows := m.kept().ix.stopAt[m.cursor]
	return shows
}

// rebuild reads the board's frame and lays it out as new. Rebuild, a tab's return and a new
// grouping come through here, because each says what the board shows may have changed.
func (m *Model) rebuild(now time.Time) { m.rebuildKeeping(now, false) }

// rebuildKeeping reads the board's frame. Where mayKeep holds and the frame carries the last
// frame's Revision, the last frame's Columns and Groups stand, with their layout, the filter's
// answers and the change cues, and only the Strip, Charts and StartAt are taken. The clock and a
// read that landed ask this way. A new grouping comes through Rebuild, which never keeps.
func (m *Model) rebuildKeeping(now time.Time, mayKeep bool) {
	wasLast := m.tails() && m.placed && m.onLastRow()
	grouping := m.currentGrouping()
	next := m.board.Frame(now, grouping)
	same := mayKeep && next.Revision != 0 && next.Revision == m.frame.Revision
	m.drawnFrom = m.readNow()
	if same {
		next.Columns, next.Groups = m.frame.Columns, m.frame.Groups
		m.frame = next
	} else {
		m.frame = next
		m.frameCount++
		m.layout = nil
		m.display.matched.newFrame()
		m.panelHasText = anyPanelText(m.frame.Groups)
	}
	m.drawStrip(now)
	m.layOutCharts()
	// A kept frame's cues stand until the first of them has run its time. Nothing else can move one,
	// because a row lights only where its cells differ from the frame before.
	if !same || m.cue.due(now) {
		m.cue.mark(m.frame.Groups, now)
	}
	if settled := m.cue.quietFrom.Add(settleFor); m.cue.quiet && !now.Before(settled) && !m.readsUnderWayFrom(settled) {
		m.cue.quiet = false
	}
	m.place()
	if wasLast {
		m.moveToLastRow()
	}
	m.seekEntity()
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
	k := m.kept()
	return len(k.ix.stops) > 0 && k.stop(len(k.ix.stops)-1).key == m.cursor
}

func (m *Model) moveToLastRow() {
	if k := m.kept(); len(k.ix.stops) > 0 {
		m.cursorAt = len(k.ix.stops) - 1
		m.cursor = k.stop(m.cursorAt).key
	}
}

func (m *Model) nodes() []node { return m.kept().nodes }

// laidOut is what shows of the frame at the table's width, the columns with their gaps, and
// the width each column takes. One key or draw asks it many times, and walking and measuring
// a table of a thousand rows takes tens of milliseconds, so it answers again from the layout it
// kept until the frame, the width, the Look or what the reader folded, opened or filtered moves.
func (m *Model) laidOut() ([]node, []Column, []int) {
	k := m.kept()
	return k.nodes, k.columns, k.widths
}

// kept is the frame's layout and its line index, built again only where what it was laid out for
// has moved.
func (m *Model) kept() *keptLayout {
	d := m.display
	d.panelShown = m.panelWidth() > 0
	width, look := m.tableWidth(), m.look()
	if kept := m.layout; kept != nil && kept.fits(d, width, look) {
		return kept
	}
	nodes, columns, widths := d.nodes(m.frame.Groups, m.frame.Columns, width, look, m.nodesHint)
	m.nodesHint = len(nodes)
	k := &keptLayout{
		width: width, look: look, under: d.under, panelShown: d.panelShown, filter: d.filter, locked: refOf(d.lockedTo()),
		toggles: d.toggles, nodes: nodes, columns: columns, widths: widths,
	}
	k.ix = k.index(m.frame.Groups)
	m.layout = k
	return k
}

// keptLayout is one frame's layout, its line index, and what it was laid out for beside the frame.
type keptLayout struct {
	width             int
	look              Look
	under, panelShown bool
	filter            string
	locked            EntityRef
	// toggles is the display's count of folds and opens when this was laid out.
	toggles uint64
	nodes   []node
	columns []Column
	widths  []int
	ix      *lineIndex
	// panel is the side panel's document for the cursor's stop, read once and kept unpainted.
	panel keptPanel
}

// rereadDocuments has the next draw call each document the layout holds again. An open
// document's lines sit in the line index, so a layout holding one is dropped whole.
func (m *Model) rereadDocuments() {
	switch {
	case m.layout == nil:
	case len(m.layout.ix.documents) > 0:
		m.layout = nil
	default:
		m.layout.panel = keptPanel{}
	}
}

func (k *keptLayout) fits(d display, width int, look Look) bool {
	return k.width == width && k.look == look && k.under == d.under && k.panelShown == d.panelShown &&
		k.filter == d.filter && k.locked == refOf(d.lockedTo()) && k.toggles == d.toggles
}

// stop is the node at the cursor's stop at.
func (k *keptLayout) stop(at int) *node { return &k.nodes[k.ix.stops[at]] }

// refOf is the entity ref points at, and the zero ref for nil.
func refOf(ref *EntityRef) EntityRef {
	if ref == nil {
		return EntityRef{}
	}
	return *ref
}

// panelDrawsDocumentOf says the side panel is drawn and draws the row's own Document.
func (m *Model) panelDrawsDocumentOf(r *Row) bool {
	return m.panelWidth() > 0 && r.panelDrawsDocument()
}

// place puts the cursor on its key, or on where it starts the first time there is such a
// line, or on the line it was on where its row is gone.
func (m *Model) place() {
	k := m.kept()
	if len(k.ix.stops) == 0 {
		m.cursor, m.cursorAt = "", 0
		return
	}
	if !m.placed {
		if i, shows := k.ix.stopAt[m.frame.StartAt]; shows && m.frame.StartAt != "" {
			m.cursor, m.cursorAt, m.placed = m.frame.StartAt, i, true
			return
		}
		if i := m.startingLine(k); i >= 0 {
			m.cursor, m.cursorAt, m.placed = k.stop(i).key, i, true
			return
		}
	}
	if i, shows := k.ix.stopAt[m.cursor]; shows {
		m.cursorAt = i
		return
	}
	m.cursorAt = max(0, min(m.cursorAt, len(k.ix.stops)-1))
	m.cursor = k.stop(m.cursorAt).key
}

// startingLine is the first line holding cells, or the last on a board read from its
// newest, and otherwise the first with anything to read. The cursor starts past a group
// with no Panel or Detail, because the panel would only repeat its label.
func (m *Model) startingLine(k *keptLayout) int {
	holdsCells := func(stop int) bool {
		n := k.nodes[stop]
		if n.kind == nodeGroup {
			return len(n.group.Cells) > 0 && n.group.hasPanelText()
		}
		return len(n.row.Cells) > 0
	}
	if m.tails() {
		for at := len(k.ix.stops) - 1; at >= 0; at-- {
			if holdsCells(k.ix.stops[at]) {
				return at
			}
		}
	}
	if i := slices.IndexFunc(k.ix.stops, holdsCells); i >= 0 {
		return i
	}
	return slices.IndexFunc(k.ix.stops, func(stop int) bool {
		return k.nodes[stop].kind == nodeRow || k.nodes[stop].group.hasPanelText()
	})
}

func (m *Model) selected() (node, bool) {
	k := m.kept()
	if m.cursorAt < 0 || m.cursorAt >= len(k.ix.stops) {
		return node{}, false
	}
	return *k.stop(m.cursorAt), true
}

func (m *Model) move(by int) {
	k := m.kept()
	if len(k.ix.stops) == 0 {
		return
	}
	m.cursorAt = max(0, min(m.cursorAt+by, len(k.ix.stops)-1))
	m.cursor = k.stop(m.cursorAt).key
	m.placed = true
	m.scrolled = false
	m.follow()
}

// jump moves to the next or previous group at the cursor's depth or shallower. Going
// back from inside a group lands on its own heading first, as Vim's [[ does.
func (m *Model) jump(direction int) {
	k := m.kept()
	if m.cursorAt < 0 || m.cursorAt >= len(k.ix.stops) {
		return
	}
	level := k.stop(m.cursorAt).depth
	if k.stop(m.cursorAt).kind == nodeRow {
		level--
	}
	for at := m.cursorAt + direction; at >= 0 && at < len(k.ix.stops); at += direction {
		if k.stop(at).kind == nodeGroup && k.stop(at).depth <= level {
			m.move(at - m.cursorAt)
			return
		}
	}
}

func (m *Model) hasGroups() bool { return m.kept().ix.hasGroups }

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

// scrollHeight is what pinned lines that stay on screen leave for those that scroll.
func (m *Model) scrollHeight(pinned int) int { return max(1, m.bodyHeight()-pinned) }

// follow keeps the cursor's line on screen, clear of the band. It reads the line from the index,
// so it paints nothing.
func (m *Model) follow() {
	want := -1
	if followHook != nil {
		want = followHook(m)
	}
	k := m.kept()
	h := m.scrollHeight(k.ix.pinned)
	at, end, ok := k.cursorSpan(m.cursor)
	if ok && !m.scrolled {
		if at < m.top {
			m.top = at
		}
		if end >= m.top+h {
			m.top = min(at, end-h+1)
		}
	}
	m.top = max(0, min(m.top, k.ix.scrolling()-h))
	if ok && !m.scrolled {
		m.clearOfBand(k, at, h)
	}
	if followHook != nil && m.top != want {
		panic(fmt.Sprintf("the cursor's follow scrolled to line %d, and the whole table's draw scrolls to %d", m.top, want))
	}
}

// clearOfBand moves the screen up until the line at sits below the band, which is no cursor stop.
func (m *Model) clearOfBand(k *keptLayout, at, height int) {
	for m.top > 0 {
		if _, band := k.ix.bandOver(m.top, height); at >= m.top+band {
			return
		}
		m.top--
	}
}

// The hooks a test sets to hold this draw to the whole table's draw. followHook and revealHook
// answer where the whole table's draw would scroll to, asked before this one moves, and drawHook
// hears every draw under the strip.
var (
	followHook func(m *Model) int
	revealHook func(m *Model) int
	drawHook   func(m *Model, body int, footer, drew []string)
)

// footer is the work under way, then an open question and the keys that answer it, or else the
// filter being typed, what the last key did, or the filter kept, then the keys. What a key did
// shows until the next key, so a kept filter never hides a refusal.
func (m *Model) footer() []string {
	var lines []string
	if m.look().Rules {
		lines = append(lines, RuleThin.across(m.painter, m.width, ""))
	}
	if work := m.underWay(); work != "" {
		lines = append(lines, ansi.Truncate(m.painter.cell(C(spinner[m.spinAt], Yellow))+" "+work, m.width, "…"))
	}
	if m.mode() == askingMode {
		asked, keys := m.questionLines(m.height - len(m.frame.Strip) - len(lines) - 2)
		return append(append(lines, asked...), ansi.Truncate(keys, m.width, "…"))
	}
	switch {
	case m.mode() == filteringMode:
		lines = append(lines, m.filterLine.draw("/", m.width))
	case m.said != "":
		lines = append(lines, m.wrapped(m.said)...)
	case m.display.filter != "":
		lines = append(lines, ansi.Truncate("filter (esc clears): "+m.display.filter, m.width, "…"))
	}
	if locked := m.lockLine(); locked != "" {
		lines = append(lines, ansi.Truncate(m.painter.cell(C(LockGlyph, LockCode))+" "+locked, m.width, "…"))
	}
	return append(lines, ansi.Truncate(strings.Join(FooterHints(m.actions()), "  "), m.width, "…"))
}

// View is Bubble Tea's.
func (m *Model) View() tea.View {
	v := tea.NewView(m.screen())
	v.AltScreen = true
	return v
}

func (m *Model) screen() string { return m.framed(m.inside()) }

// inside is the screen within its margin.
func (m *Model) inside() string {
	if m.reading != nil {
		return m.reading.screen()
	}
	if !m.stripOnly || m.underStrip == nil {
		footer := m.footer()
		m.footerDrawn = len(footer)
		m.underStrip = m.drawUnderStrip(m.heightsOver(len(footer)).body, footer)
	}
	m.stripOnly = false
	charts := m.charts[:m.heightsOver(m.footerDrawn).charts]
	out := make([]string, 0, len(m.frame.Strip)+len(charts)+len(m.underStrip))
	for _, line := range slices.Concat(m.frame.Strip, charts) {
		out = append(out, ansi.Truncate(m.painter.line(line), m.width, "…"))
	}
	return strings.Join(append(out, m.underStrip...), "\n")
}

// drawUnderStrip is the table or the keys overlay in body lines, then the footer. It paints three
// ranges of the table: the lines that stay on screen, the band over the top of those that scroll,
// and the lines the band leaves showing.
func (m *Model) drawUnderStrip(body int, footer []string) []string {
	out := make([]string, 0, body+len(footer))
	if m.overlay {
		out = append(out, m.keysOverlay(body)...)
		return append(out, footer...)
	}
	k := m.kept()
	t := m.tablePainter(k)
	out = append(out, t.lines(0, k.ix.pinned)...)
	h := max(1, body-k.ix.pinned)
	band := t.band(m.top, h)
	shown := t.lines(k.ix.pinned+m.top+len(band), k.ix.pinned+m.top+h)
	panelWidth := m.panelWidth()
	panel := m.panelLines(h)
	for i := range h {
		left := ""
		switch {
		case i < len(band):
			left = band[i]
		case i-len(band) < len(shown):
			left = shown[i-len(band)]
		}
		if panelWidth == 0 {
			out = append(out, left)
			continue
		}
		left += strings.Repeat(" ", max(k.width-textWidth(left), 0))
		right := ""
		if i < len(panel) {
			right = panel[i]
		}
		out = append(out, strings.TrimRight(left+" │ "+right, " "))
	}
	out = append(out, footer...)
	if drawHook != nil {
		drawHook(m, body, footer, out)
	}
	return out
}

// panelLines is the side panel's first height lines. It paints no line past them.
func (m *Model) panelLines(height int) []string {
	width := m.panelWidth()
	if width == 0 {
		return nil
	}
	n, ok := m.selected()
	if !ok {
		return nil
	}
	k := m.kept()
	at := k.ix.stops[m.cursorAt]
	if p := &k.panel; !p.held || p.node != at || p.width != width {
		*p = keptPanel{node: at, width: width, held: true}
		if authored, _ := n.panelSource(); authored != nil {
			p.lines = authored(width)
		}
	}
	return m.panelTextOf(n, k.panel.lines, width, height)
}

// keptPanel is the side panel's document as the node at one cursor stop gave it at one width,
// unpainted, so a theme reload recolors it. It is dropped with its layout.
type keptPanel struct {
	node, width int
	lines       []Line
	held        bool
}

// panelSource is the node's own text for the side panel: a Document, or cells where it has none.
func (n node) panelSource() (Document, []Cell) {
	switch n.kind {
	case nodeGroup:
		return n.group.panelSource()
	case nodeRow:
		return n.row.panelSource()
	}
	return nil, nil
}

// panelTextOf is the side panel's first most lines for a node whose Document gave document, or
// whose cells stand in where it has no Document.
func (m *Model) panelTextOf(n node, document []Line, width, most int) []string {
	authored, cells := n.panelSource()
	if n.kind == nodeGroup && len(cells) == 0 {
		cells = []Cell{C(groupLabel(n.group).Text(), Bold), C(n.group.Summary.Text(), "")}
	}
	var lines []string
	if n.kind == nodeRow && n.row.panelDrawsDocument() && n.row.ReaderTitle != "" {
		for _, piece := range wrapWords(Line{C(n.row.ReaderTitle, Bold)}, width) {
			lines = append(lines, m.painter.line(piece))
		}
	}
	if authored != nil {
		for _, line := range document {
			if len(lines) >= most {
				break
			}
			for _, piece := range wrapWords(line, width) {
				lines = append(lines, m.painter.line(piece))
			}
		}
		return lines[:min(len(lines), most)]
	}
	for _, c := range cells {
		if len(lines) >= most {
			break
		}
		for _, piece := range wrap(c.Text, width) {
			lines = append(lines, m.painter.cell(c.withText(piece)))
		}
	}
	return lines[:min(len(lines), most)]
}

func (m *Model) keysOverlay(height int) []string {
	lines := []string{m.painter.bold("Keys, any key closes this")}
	for _, action := range m.actions() {
		lines = append(lines, keyLines(action, m.width)...)
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
	lines := []string{"", m.painter.bold("Reads")}
	for _, source := range sources {
		status := source.Status()
		cell := readCell(status, now)
		line := "  " + fit(status.Name, 16, false) + "  " + m.painter.cell(cell.withText(fit(cell.Text, 16, false))) +
			"  every " + fit(status.Every, 8, false) + "  took " + fit(status.Took, 8, false)
		switch {
		case status.Fault != "":
			line += "  " + m.painter.cell(C(status.Fault, Red))
		case status.Note != "":
			line += "  " + status.Note
		}
		lines = append(lines, ansi.Truncate(line, m.width, "…"))
	}
	return lines
}
