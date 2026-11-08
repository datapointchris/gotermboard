package gotermboard

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/datapointchris/gotermstyle"
)

func typedKey(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: []rune(text)[0], Text: text}
}

// wrappedDocument is sixty sentences wrapped to the width, a heading before every tenth.
func wrappedDocument(width int) []Line {
	var rows []Line
	for i := range 60 {
		if i%10 == 0 {
			rows = append(rows, Line{Cell{Text: "section " + strconv.Itoa(i), Section: 1}})
		}
		sentence := "sentence number " + strconv.Itoa(i) + " has words"
		for len(sentence) > width {
			rows = append(rows, Line{C(sentence[:width], "")})
			sentence = sentence[width:]
		}
		rows = append(rows, Line{C(sentence, "")})
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
	if !strings.Contains(r.screen(), Reverse+"number 37") {
		t.Fatalf("the match is not reversed on screen:\n%s", r.screen())
	}

	first := r.rows[r.top].Text()
	r.resize(12, 10)
	if !strings.HasPrefix(first, strings.TrimSpace(r.rows[r.top].Text())) {
		t.Fatalf("after narrowing, the top row is %q, want the start of %q", r.rows[r.top].Text(), first)
	}
}

func TestEnterReadsTallTextAndTextThePanelShowsFullScreenAndSpaceFoldsItInPlace(t *testing.T) {
	short := func(int) []Line { return []Line{{C("short text", "")}} }
	frame := Frame{Columns: []Column{{Title: "ROW"}}, Groups: []Group{{Rows: []Row{
		{Key: "tall", Cells: []Cell{C("the tall row", "")}, Document: wrappedDocument},
		{Key: "short", Cells: []Cell{C("the short row", "")}, Document: short},
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
		t.Error("a short row the side panel shows should open the reader rather than draw it twice")
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
	if got := m.screen(); !strings.Contains(got, "› ▸") {
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
}
