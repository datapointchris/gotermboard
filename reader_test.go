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
			rows = append(rows, Line{Cell{Text: "section " + strconv.Itoa(i), Section: true}})
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

func TestEnterReadsATallDocumentFullScreenAndEscReturnsToTheRow(t *testing.T) {
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

	m.Update(downKey)
	m.Update(enterKey)
	if m.reading != nil {
		t.Fatal("a short row opened the reader rather than in place")
	}
}
