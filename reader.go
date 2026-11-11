package gotermboard

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermstyle"

	"github.com/datapointchris/gotermboard/breadcrumb"
)

// ReadingWidth is the widest a reader lays text out, because a longer line is hard to
// follow back to its start. A wider screen keeps the rest as margin. A view that lays
// the same text out in its table reads this, so the two wrap alike.
const ReadingWidth = 100

// hitContext is how many rows a search leaves above the row it lands on.
const hitContext = 2

// Reader is a full screen over one Document. It lays out every row at the width and draws
// the window that fits, because Bubble Tea keeps only the last lines of a taller view.
// It runs as a program of its own, or a Model opens one with Read.
type Reader struct {
	// CopyText is what y copies. Nil copies the text as drawn.
	CopyText func() string
	// EditFile is the file e opens in the editor, after which the text is laid out again.
	// Empty leaves e unbound.
	EditFile string

	title    string
	document Document
	painter  painter
	// path is what holds the row that opened this reader, outermost first.
	path []Line
	// row is the key of the row whose Document this reads, so a read that lands lays out the
	// new frame's. Empty, the reader keeps the Document it opened with.
	row string
	// back is what esc does once nothing is left to clear. Nil means esc stays put.
	back func()
	quit func() tea.Cmd
	// follows keeps the reader on the document's last line as the document grows, while that
	// line shows.
	follows bool

	width, height int
	rows          []Line
	sections      []int
	top           int
	overlay       bool
	said          string

	searching    bool
	searchedFrom int
	// searchLine is the search as it is typed, and query the one the hits are for.
	searchLine typedLine
	query      string
	hits       []int
	hit        int
}

// NewReader is a Reader over document, sized until the terminal says otherwise. It draws in
// the TUI theme file gotermstyle's tuitheme finds, and reads it again on SIGUSR2 while it runs
// as the program.
func NewReader(title string, document Document, pal gotermstyle.Palette) *Reader {
	theme, err := loadTheme()
	r := newReader(title, document, painter{pal: pal, theme: theme})
	r.sayThemeFault(err)
	return r
}

// sayThemeFault puts why the theme file would not load on the footer.
func (r *Reader) sayThemeFault(err error) {
	if err != nil {
		r.said = err.Error()
	}
}

func newReader(title string, document Document, p painter) *Reader {
	r := &Reader{title: title, document: document, painter: p, quit: func() tea.Cmd { return tea.Quit }}
	r.resize(80, 24)
	return r
}

// ClosesWith is for a program drawing the reader inside a screen of its own, as Model.Read
// does over a board: esc and space call back once nothing is left to clear, where on a reader
// running as the program they do nothing.
func (r *Reader) ClosesWith(back func()) { r.back = back }

// Once is the whole document at width, for output nobody scrolls.
func (r *Reader) Once(width int) string {
	var b strings.Builder
	for _, line := range r.document(width) {
		b.WriteString(r.painter.line(line))
		b.WriteByte('\n')
	}
	return b.String()
}

// Init is Bubble Tea's.
func (r *Reader) Init() tea.Cmd { return r.painter.theme.watch() }

// Update is Bubble Tea's.
func (r *Reader) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		r.resize(msg.Width, msg.Height)
	case themeLoadedMsg:
		return r, r.painter.theme.reloaded(msg, r.sayThemeFault)
	case tea.PasteMsg:
		r.paste(msg.Content)
	case tea.KeyPressMsg:
		cmd := r.pressed(msg)
		return r, cmd
	case editedMsg:
		if msg.err != nil {
			r.said = "the editor failed: " + LastLine(msg.stderr, msg.err)
		}
		r.resize(r.width, r.height)
	}
	return r, nil
}

type editedMsg struct {
	err    error
	stderr []byte
}

// edit hands the terminal to the editor. A Reader runs as a program of its own too, with no
// Model to hand over through.
func (r *Reader) edit() tea.Cmd {
	return handOver(&handedOver{cmd: EditorCommand(r.EditFile)}, func(err error, stderr []byte) tea.Msg { return editedMsg{err, stderr} })
}

func (r *Reader) copyToClipboard() tea.Cmd {
	var text string
	if r.CopyText != nil {
		text = r.CopyText()
	} else {
		lines := make([]string, 0, len(r.rows))
		for _, row := range r.rows {
			lines = append(lines, row.Text())
		}
		text = strings.Join(lines, "\n")
	}
	r.said = "copied"
	return tea.SetClipboard(text)
}

// View is Bubble Tea's.
func (r *Reader) View() tea.View {
	v := tea.NewView(r.screen())
	v.AltScreen = true
	return v
}

// resize lays the text out again and keeps the same words at the top of the screen. A reader
// that follows and showed the last line shows the last line again.
func (r *Reader) resize(width, height int) {
	above := lettersIn(r.rows[:min(r.top, len(r.rows))])
	toEnd := r.follows && r.atEnd()
	r.width, r.height = width, height
	r.rows = r.document(min(max(width-2, 1), ReadingWidth))
	r.sections = r.sections[:0]
	for i, row := range r.rows {
		if slices.ContainsFunc(row, opensSection) {
			r.sections = append(r.sections, i)
		}
	}
	r.scrollTo(rowAfterLetters(r.rows, above))
	if toEnd {
		r.scrollTo(len(r.rows))
	}
	r.findHits()
	r.hit = max(slices.IndexFunc(r.hits, func(row int) bool { return row >= r.top }), 0)
}

func opensSection(c Cell) bool { return c.Section > 0 }

// headingsInView is the words of each heading still open at the top row, outermost first.
// A heading on the top row closes those it ends and is not named, as a table's band names
// only what has scrolled off, so the top line never repeats the row under it.
func (r *Reader) headingsInView() []Line {
	var open []Line
	var depths []int
	for _, at := range r.sections {
		if at > r.top {
			break
		}
		row := r.rows[at]
		from := slices.IndexFunc(row, opensSection)
		depth := row[from].Section
		for len(depths) > 0 && depths[len(depths)-1] >= depth {
			depths, open = depths[:len(depths)-1], open[:len(open)-1]
		}
		if at < r.top {
			depths, open = append(depths, depth), append(open, row[from:])
		}
	}
	return open
}

// topLine is the path of what opened the reader, its title, then the section in view.
func (r *Reader) topLine() string {
	segments := slices.Clone(r.path)
	if r.title != "" {
		segments = append(segments, Line{C(r.title, Bold)})
	}
	margin := r.margin()
	return margin + r.pathLine(append(segments, r.headingsInView()...), r.width-len(margin))
}

// pathLine draws segments on one line within room, cut as [breadcrumb.Fit] cuts them.
func (r *Reader) pathLine(segments []Line, room int) string {
	widths := make([]int, len(segments))
	for i, segment := range segments {
		widths[i] = segment.width()
	}
	dropped, widths := breadcrumb.Fit(widths, ansi.StringWidth(" › "), room)
	painted := make([]string, 0, len(widths))
	for i, segment := range segments[dropped:] {
		painted = append(painted, ansi.Truncate(r.painter.line(segment), widths[i], "…"))
	}
	return strings.Join(painted, r.painter.cell(C(" › ", RuleCode)))
}

func lettersIn(rows []Line) int {
	count := 0
	for _, row := range rows {
		for _, c := range row.Text() {
			if unicode.IsLetter(c) || unicode.IsDigit(c) {
				count++
			}
		}
	}
	return count
}

func rowAfterLetters(rows []Line, letters int) int {
	for i := range rows {
		if letters <= 0 {
			return i
		}
		letters -= lettersIn(rows[i : i+1])
	}
	return len(rows)
}

// bodyHeight is what the top line and the footer leave for the text.
func (r *Reader) bodyHeight() int { return max(r.height-2, 1) }

func (r *Reader) margin() string { return strings.Repeat(" ", max((r.width-ReadingWidth)/2, 1)) }

func (r *Reader) scrollTo(top int) {
	r.top = max(min(top, len(r.rows)-r.bodyHeight()), 0)
}

// atEnd says the document's last line shows.
func (r *Reader) atEnd() bool { return r.top >= len(r.rows)-r.bodyHeight() }

func readerActions() []KeyAction[*Reader] {
	by := func(lines func(*Reader) int) func(*Reader) tea.Cmd {
		return func(r *Reader) tea.Cmd {
			r.scrollTo(r.top + lines(r))
			return nil
		}
	}
	one := func(lines int) func(*Reader) int { return func(*Reader) int { return lines } }
	half := func(direction int) func(*Reader) int {
		return func(r *Reader) int { return direction * max(r.bodyHeight()/2, 1) }
	}
	all := func(direction int) func(*Reader) int {
		return func(r *Reader) int { return direction * len(r.rows) }
	}
	hasSections := func(r *Reader) bool { return len(r.sections) > 0 }
	hasHits := func(r *Reader) bool { return len(r.hits) > 0 }
	return []KeyAction[*Reader]{
		{Keys: []string{"j", "down"}, Label: "scroll down a line", Footer: "j/k scroll", Run: by(one(1))},
		{Keys: []string{"k", "up"}, Label: "scroll up a line", Run: by(one(-1))},
		{Keys: []string{"ctrl+d", "pgdown"}, Label: "half a screen down", Run: by(half(1))},
		{Keys: []string{"ctrl+u", "pgup"}, Label: "half a screen up", Run: by(half(-1))},
		{Keys: []string{"g", "home"}, Label: "go to the top", Run: by(all(-1))},
		{Keys: []string{"G", "end"}, Label: "go to the bottom", Run: by(all(1))},
		{Keys: []string{"]"}, Label: "jump to the next section", Footer: "[/] section", Run: func(r *Reader) tea.Cmd {
			if at := slices.IndexFunc(r.sections, func(row int) bool { return row > r.top }); at >= 0 {
				r.scrollTo(r.sections[at])
			}
			return nil
		}, Applies: hasSections},
		{Keys: []string{"["}, Label: "jump to the previous section", Run: func(r *Reader) tea.Cmd {
			for _, row := range slices.Backward(r.sections) {
				if row < r.top {
					r.scrollTo(row)
					break
				}
			}
			return nil
		}, Applies: hasSections},
		{Keys: []string{"/"}, Label: "search the text", Footer: "/ search", Run: func(r *Reader) tea.Cmd {
			r.searching, r.searchedFrom = true, r.top
			r.searchLine.set("")
			r.setQuery("")
			return nil
		}},
		{Keys: []string{"n"}, Label: "go to the next match", Run: func(r *Reader) tea.Cmd {
			r.landOnHit(r.hit + 1)
			return nil
		}, Applies: hasHits},
		{Keys: []string{"N"}, Label: "go to the previous match", Run: func(r *Reader) tea.Cmd {
			r.landOnHit(r.hit - 1)
			return nil
		}, Applies: hasHits},
		{Keys: []string{"y"}, Label: "copy the text", Footer: "y copy", Run: (*Reader).copyToClipboard},
		{Keys: []string{"e"}, Label: "edit the file in $EDITOR", Footer: "e edit", Run: (*Reader).edit, Applies: func(r *Reader) bool {
			return r.EditFile != ""
		}},
		{Keys: []string{"?"}, Label: "show these keys", Footer: "? keys", Run: func(r *Reader) tea.Cmd {
			r.overlay = true
			return nil
		}},
		{Keys: []string{"space"}, Label: "go back", Run: func(r *Reader) tea.Cmd {
			r.back()
			return nil
		}, Applies: func(r *Reader) bool { return r.back != nil }},
		{Keys: []string{"esc"}, Label: "clear the search, or go back", Footer: "esc/space back", Run: func(r *Reader) tea.Cmd {
			if r.query != "" {
				r.setQuery("")
			} else if r.back != nil {
				r.back()
			}
			return nil
		}, Applies: func(r *Reader) bool { return r.back != nil || r.query != "" }},
		{Keys: []string{"q", "ctrl+c"}, Label: "quit", Footer: "q quit", Run: func(r *Reader) tea.Cmd { return r.quit() }},
	}
}

// actions is the Reader's key table as it applies now, which the handler, the footer and the ?
// overlay read.
func (r *Reader) actions() []KeyAction[*Reader] { return ActionsNow(r, readerActions()) }

func (r *Reader) pressed(key tea.KeyPressMsg) tea.Cmd {
	if r.overlay {
		r.overlay = false
		return nil
	}
	if r.searching {
		r.typed(key)
		return nil
	}
	r.said = ""
	cmd, _ := RunKey(r, r.actions(), key.String())
	return cmd
}

// typed is a key while the search is typed: enter keeps it, and esc clears it and goes back to
// where it began.
func (r *Reader) typed(key tea.KeyPressMsg) {
	switch r.searchLine.press(key) {
	case lineEntered:
		r.searching = false
	case lineCanceled:
		r.searching = false
		r.setQuery("")
		r.scrollTo(r.searchedFrom)
	default:
		if query := r.searchLine.String(); query != r.query {
			r.setQuery(query)
		}
	}
}

func (r *Reader) paste(content string) {
	if r.searching {
		r.searchLine.paste(content)
		r.setQuery(r.searchLine.String())
	}
}

// setQuery lands on the first match below where the search began, as a pager's
// incremental search does.
func (r *Reader) setQuery(query string) {
	r.query = query
	r.findHits()
	if len(r.hits) == 0 {
		if r.searching {
			r.scrollTo(r.searchedFrom)
		}
		return
	}
	r.landOnHit(max(slices.IndexFunc(r.hits, func(row int) bool { return row >= r.searchedFrom }), 0))
}

func (r *Reader) landOnHit(hit int) {
	if len(r.hits) == 0 {
		return
	}
	r.hit = (hit + len(r.hits)) % len(r.hits)
	r.scrollTo(r.hits[r.hit] - hitContext)
}

func (r *Reader) findHits() {
	r.hits = r.hits[:0]
	if r.query == "" {
		return
	}
	for i, row := range r.rows {
		if slices.Contains(r.matchedIn(row.Text()), true) {
			r.hits = append(r.hits, i)
		}
	}
}

// matchedIn says which runes of text are part of a match. A query with no capital in it
// ignores case, as ripgrep's --smart-case does.
func (r *Reader) matchedIn(text string) []bool {
	fold := func(s string) []rune { return []rune(s) }
	if !strings.ContainsFunc(r.query, unicode.IsUpper) {
		fold = func(s string) []rune {
			runes := []rune(s)
			for i, c := range runes {
				runes[i] = unicode.ToLower(c)
			}
			return runes
		}
	}
	runes, query := fold(text), fold(r.query)
	matched := make([]bool, len(runes))
	if len(query) == 0 {
		return matched
	}
	for i := 0; i+len(query) <= len(runes); i++ {
		if slices.Equal(runes[i:i+len(query)], query) {
			for j := range query {
				matched[i+j] = true
			}
			i += len(query) - 1
		}
	}
	return matched
}

// marked is row with each match in the theme's match code, its other cells as they were.
func (r *Reader) marked(row Line) Line {
	if r.query == "" {
		return row
	}
	matched := r.matchedIn(row.Text())
	if !slices.Contains(matched, true) {
		return row
	}
	match := r.painter.matchCode()
	var out Line
	at := 0
	for _, cell := range row {
		runes := []rune(cell.Text)
		for start := 0; start < len(runes); {
			end := start + 1
			for end < len(runes) && matched[at+end] == matched[at+start] {
				end++
			}
			piece := cell.withText(string(runes[start:end]))
			if matched[at+start] {
				piece = C(piece.Text, match)
			}
			out = append(out, piece)
			start = end
		}
		at += len(runes)
	}
	return out
}

func (r *Reader) screen() string {
	body := r.bodyHeight()
	lines := make([]string, 0, body+2)
	lines = append(lines, r.topLine())
	if r.overlay {
		lines = append(lines, r.painter.bold("Keys, any key closes this"))
		for _, action := range r.actions() {
			lines = append(lines, keyLines(action, r.width)...)
		}
		lines = lines[:min(len(lines), body+1)]
	} else {
		margin := r.margin()
		for _, row := range r.rows[r.top:min(r.top+body, len(r.rows))] {
			lines = append(lines, ansi.Truncate(margin+r.painter.line(r.marked(row)), r.width, "…"))
		}
	}
	for len(lines) < body+1 {
		lines = append(lines, "")
	}
	return strings.Join(append(lines, ansi.Truncate(r.footer(), r.width, "…")), "\n")
}

func (r *Reader) footer() string {
	parts := []string{fmt.Sprintf("%d/%d", min(r.top+r.bodyHeight(), len(r.rows)), len(r.rows))}
	switch {
	case r.follows && r.atEnd():
		parts = append(parts, "following")
	case r.follows:
		parts = append(parts, "G follows")
	}
	switch {
	case r.searching:
		// The search being typed goes last, so a narrow screen cuts it from the left and keeps the cursor.
		if hits := r.hitsSaid(); hits != "" {
			parts = append(parts, hits)
		}
		lead := strings.Join(parts, "  ") + "  "
		return lead + r.searchLine.draw("/", r.width-ansi.StringWidth(lead))
	case r.query != "":
		parts = append(parts, "/"+r.query, r.hitsSaid(), "n/N match", "esc clears")
	case r.said != "":
		parts = append(parts, r.said)
	default:
		parts = append(parts, FooterHints(r.actions())...)
	}
	return strings.Join(parts, "  ")
}

func (r *Reader) hitsSaid() string {
	switch {
	case r.query == "":
		return ""
	case len(r.hits) == 0:
		return "no match"
	}
	return fmt.Sprintf("%d of %d", r.hit+1, len(r.hits))
}
