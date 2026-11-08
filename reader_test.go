package gotermboard

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/datapointchris/gotermstyle"
)

func typedKey(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: []rune(text)[0], Text: text}
}

// sentenceCode is how wrappedDocument's sentences draw: bold in the built-in match color, as
// a bold code span does, so a match the search marks has to differ from that to show.
var sentenceCode = emboldened(Yellow)

// wrappedDocument is sixty sentences wrapped to the width, a heading before every tenth.
func wrappedDocument(width int) []Line {
	var rows []Line
	for i := range 60 {
		if i%10 == 0 {
			rows = append(rows, Line{Cell{Text: "section " + strconv.Itoa(i), Section: 1}})
		}
		sentence := "sentence number " + strconv.Itoa(i) + " has words"
		for len(sentence) > width {
			rows = append(rows, Line{C(sentence[:width], sentenceCode)})
			sentence = sentence[width:]
		}
		rows = append(rows, Line{C(sentence, sentenceCode)})
	}
	return rows
}

func TestReaderJumpsSearchesAndKeepsItsPlaceOnAResize(t *testing.T) {
	r := NewReader("a title", wrappedDocument, gotermstyle.NewPalette(true))
	r.resize(80, 10)

	r.pressed(typedKey("]"))
	if got := r.rows[r.top].Text(); got != "section 10" {
		t.Fatalf("] landed on %q, want the next heading", got)
	}

	r.pressed(typedKey("/"))
	for _, c := range "NUMBER 37" {
		r.pressed(typedKey(string(c)))
	}
	if strings.Contains(r.screen(), "sentence number 37") {
		t.Fatal("a query with capitals matched lowercase text")
	}
	r.pressed(tea.KeyPressMsg{Code: tea.KeyEscape})

	r.pressed(typedKey("/"))
	for _, c := range "number 37" {
		r.pressed(typedKey(string(c)))
	}
	r.pressed(tea.KeyPressMsg{Code: tea.KeyEnter})
	at := slices.IndexFunc(r.rows, func(row Line) bool { return strings.Contains(row.Text(), "number 37 ") })
	matched := slices.IndexFunc(r.marked(r.rows[at]), func(c Cell) bool { return c.Text == "number 37" })
	if matched < 0 || r.marked(r.rows[at])[matched].Code == sentenceCode {
		t.Fatalf("the match should draw unlike the bold match-colored text around it: %+v", r.marked(r.rows[at]))
	}

	first := r.rows[r.top].Text()
	r.resize(12, 10)
	if !strings.HasPrefix(first, strings.TrimSpace(r.rows[r.top].Text())) {
		t.Fatalf("after narrowing, the top row is %q, want the start of %q", r.rows[r.top].Text(), first)
	}
}

func TestEnterReadsTallTextAndTextThePanelDrawsFullScreenAndSpaceFoldsItInPlace(t *testing.T) {
	short := func(int) []Line { return []Line{{C("short text", "")}} }
	frame := Frame{Columns: []Column{{Title: "ROW"}}, Groups: []Group{{Rows: []Row{
		{Key: "tall", Cells: []Cell{C("the tall row", "")}, Document: wrappedDocument},
		{Key: "short", Cells: []Cell{C("the short row", "")}, Document: short},
		{Key: "paneled", Cells: []Cell{C("the paneled row", "")}, Document: short, Panel: func(int) []Line { return []Line{{C("its facts", "")}} }},
	}}}}
	m := NewModel(fixedBoard{frame}, gotermstyle.NewPalette(false))
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 12})

	m.Update(enterKey)
	if m.reading == nil || !strings.Contains(m.screen(), "the tall row") || !strings.Contains(m.screen(), "section 0") {
		t.Fatalf("enter on a tall row did not open the reader:\n%s", m.screen())
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.reading != nil {
		t.Fatal("esc left the reader open")
	}
	m.Update(enterKey)
	m.Update(spaceKey)
	if m.reading != nil {
		t.Fatal("space left the reader open")
	}

	m.Update(spaceKey)
	if m.reading != nil || !strings.Contains(m.screen(), "section 0") {
		t.Fatalf("space on a tall row should open it in place:\n%s", m.screen())
	}
	m.Update(spaceKey)
	if got := m.screen(); strings.Contains(got, "section 0") {
		t.Fatalf("space should close a row open in place:\n%s", got)
	}

	m.Update(downKey)
	m.Update(enterKey)
	if m.reading != nil {
		t.Fatal("a short row opened the reader rather than in place")
	}

	m.Update(tea.WindowSizeMsg{Width: PanelFrom, Height: 12})
	m.Update(enterKey)
	if m.reading == nil {
		t.Fatal("a short row the side panel draws should open the reader rather than draw it twice")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m.Update(downKey)
	m.Update(enterKey)
	if m.reading != nil {
		t.Error("a short row whose Panel the side panel draws should open its Document in place")
	}
}

// lateBoard's one tall row ends on tail as it stood when the frame was built.
type lateBoard struct{ tail string }

func (b *lateBoard) Sources() []Refresher { return nil }
func (b *lateBoard) Groupings() []string  { return nil }
func (b *lateBoard) Frame(time.Time, string) Frame {
	tail := b.tail
	document := func(width int) []Line { return append(wrappedDocument(width), Line{C(tail, "")}) }
	return Frame{Columns: []Column{{Title: "ROW"}}, Groups: []Group{{Rows: []Row{
		{Key: "tall", Cells: []Cell{C("the tall row", "")}, Document: document},
	}}}}
}

func TestAnOpenReaderShowsTheTextAReadBringsWhereItStands(t *testing.T) {
	board := &lateBoard{tail: "the comments are unread"}
	m := NewModel(board, gotermstyle.NewPalette(false))
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 12})
	m.Update(enterKey)
	m.Update(typedKey("G"))
	board.tail = "89 points"
	m.Update(refreshedMsg{})
	if got := m.screen(); m.reading == nil || !strings.Contains(got, "89 points") {
		t.Fatalf("the reader at the document's end should show what the read brought:\n%s", got)
	}
}

// streamingBoard adds a line to its log for each lineLanded it receives.
type streamingBoard struct {
	fixedBoard
	log *[]string
}

type lineLanded string

func (b streamingBoard) Receive(_ *Model, msg tea.Msg) tea.Cmd {
	if line, ok := msg.(lineLanded); ok {
		*b.log = append(*b.log, string(line))
	}
	return nil
}

func TestAFollowingReaderKeepsToTheLastLineUntilScrolledUpAndGFollowsAgain(t *testing.T) {
	log := []string{}
	document := func(int) []Line {
		lines := make([]Line, len(log))
		for i, text := range log {
			lines[i] = Line{C(text, "")}
		}
		return lines
	}
	m := NewModel(streamingBoard{fixedBoard{Frame{Columns: []Column{{Title: "ROW"}}}}, &log}, gotermstyle.NewPalette(false))
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	m.Follow("output", document)
	for i := range 30 {
		m.Update(lineLanded("line " + strconv.Itoa(i)))
	}
	if got := m.screen(); !strings.Contains(got, "line 29") || !strings.Contains(got, "following") {
		t.Fatalf("a following reader should show the newest line and say it follows:\n%s", got)
	}
	m.Update(typedKey("k"))
	m.Update(lineLanded("line 30"))
	if got := m.screen(); strings.Contains(got, "line 30") || !strings.Contains(got, "G follows") {
		t.Fatalf("a reader scrolled up should hold its place and say G follows:\n%s", got)
	}
	m.Update(typedKey("G"))
	m.Update(lineLanded("line 31"))
	if got := m.screen(); !strings.Contains(got, "line 31") {
		t.Errorf("G should follow the newest line again:\n%s", got)
	}
}

func TestTheReadersTopLineNamesWhatHeldTheRowThenTheSectionScrolledInto(t *testing.T) {
	frame := Frame{Columns: []Column{{Title: "ROW"}}, Groups: []Group{{Key: "outer", Title: Line{C("OUTER", "")}, Rows: []Row{
		{Key: "tall", Cells: []Cell{C("the tall row", "")}, Document: wrappedDocument},
	}}}}
	m := NewModel(fixedBoard{frame}, gotermstyle.NewPalette(false))
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 12})
	m.Update(downKey)
	m.Update(enterKey)
	topLine := func() string { return strings.Split(m.screen(), "\n")[0] }
	if got := topLine(); !strings.Contains(got, "OUTER › the tall row") || strings.Contains(got, "section") {
		t.Fatalf("the top line should name the group and the row, and no section yet: %q", got)
	}
	m.Update(typedKey("]"))
	if got := topLine(); strings.Contains(got, "section") {
		t.Errorf("a heading on the top row should close the one before it and stay off the top line: %q", got)
	}
	m.Update(downKey)
	if got := topLine(); !strings.HasSuffix(strings.TrimSpace(got), "› section 10") {
		t.Errorf("the top line should name the section scrolled into: %q", got)
	}
}

func TestClosingATallRowScrolledThroughWithThirtyRowsBelowItLeavesTheCursorOnScreen(t *testing.T) {
	rows := []Row{{Key: "tall", Cells: []Cell{C("the tall row", "")}, Document: wrappedDocument}}
	for range 30 {
		rows = append(rows, Row{Key: "row" + strconv.Itoa(len(rows)), Cells: []Cell{C("a row", "")}})
	}
	m := NewModel(fixedBoard{Frame{Columns: []Column{{Title: "ROW"}}, Groups: []Group{{Rows: rows}}}}, gotermstyle.NewPalette(false))
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 12})
	m.Update(spaceKey)
	m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if strings.Contains(m.screen(), "section 0") {
		t.Fatalf("ctrl+d should have scrolled through the row's text:\n%s", m.screen())
	}
	m.Update(spaceKey)
	if got := m.screen(); !strings.Contains(got, "▌ ▸") {
		t.Fatalf("closing the row should leave the cursor on screen:\n%s", got)
	}
}

func TestARowThatStartsOpenFoldsToItsLineOnSpaceAndReadsFullScreenOnEnter(t *testing.T) {
	frame := Frame{Columns: []Column{{Title: "ROW"}}, Groups: []Group{{Rows: []Row{
		{Key: "newest", Cells: []Cell{C("the first line", "")}, Document: wrappedDocument, StartsOpen: true},
	}}}}
	m := NewModel(fixedBoard{frame}, gotermstyle.NewPalette(false))
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 12})
	if got := m.screen(); !strings.Contains(got, "▾the first line") || !strings.Contains(got, "section 0") {
		t.Fatalf("the row should start open:\n%s", got)
	}
	m.Update(enterKey)
	if m.reading == nil {
		t.Fatal("enter on an open tall row should read it full screen")
	}
	m.Update(spaceKey)
	m.Update(spaceKey)
	if got := m.screen(); !strings.Contains(got, "▸the first line") || strings.Contains(got, "section 0") {
		t.Fatalf("space should fold the row to its first line:\n%s", got)
	}

	frame.Groups[0].Rows[0].ReaderTitle = "the newest entry"
	m = NewModel(fixedBoard{frame}, gotermstyle.NewPalette(false))
	m.Update(tea.WindowSizeMsg{Width: PanelFrom, Height: 12})
	if got := m.screen(); !strings.Contains(got, "▸the first line") || !strings.Contains(got, "│ the newest entry") || strings.Count(got, "section 0") != 1 {
		t.Errorf("beside the panel, the row should start folded and the panel draw it once under its title:\n%s", got)
	}
}
