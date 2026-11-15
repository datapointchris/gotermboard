package gotermboard

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermstyle"
)

func foldingFrame() Frame {
	more := []Cell{C("more text", "")}
	return Frame{
		Strip:   []Line{{C("BOARD", "")}},
		Columns: []Column{{Title: "NAME"}, {Title: "WHAT"}},
		Groups: []Group{{Key: "g", Title: Line{C("G", "")}, Rows: []Row{
			{Key: "alone", Cells: []Cell{C("alone", ""), C("single", "")}},
			{Key: "b1", Fold: "b", Cells: []Cell{C("b1", ""), C("first", "")}},
			{Key: "b2", Fold: "b", Cells: []Cell{C("b2", ""), C("second", "")}, Detail: more},
			{Key: "b3", Fold: "b", Cells: []Cell{C("b3", ""), C("third", "")}},
			{Key: "c", Fold: "c", Cells: []Cell{C("c", ""), C("by itself", "")}},
		}}},
		FoldRow: func(rows []Row) Row {
			return Row{Cells: []Cell{C(strconv.Itoa(len(rows))+" rows", ""), C(rows[0].Key+" to "+rows[len(rows)-1].Key, "")}}
		},
	}
}

func TestRowsSharingAFoldKeyDrawAsOneRowThatOpensInPlaceAndZShowsTheRawRows(t *testing.T) {
	frame := foldingFrame()
	m := NewModel(fixedBoard{frame}, gotermstyle.NewPalette(false))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	has := func(step string, want ...string) {
		t.Helper()
		screen := m.screen()
		for _, w := range want {
			if !strings.Contains(screen, w) {
				t.Errorf("%s: no %q in:\n%s", step, w, screen)
			}
		}
	}
	lacks := func(step string, unwanted ...string) {
		t.Helper()
		screen := m.screen()
		for _, u := range unwanted {
			if strings.Contains(screen, u) {
				t.Errorf("%s: %q drawn in:\n%s", step, u, screen)
			}
		}
	}

	has("folded", "BOARD  ·  rows folded", " ▸ 3 rows", "b1 to b3", "    c ", "by itself", "Z raw rows")
	lacks("folded", "first", "third")
	lines := strings.Split(m.screen(), "\n")
	whatAt := func(text string) int {
		at := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, text) })
		before, _, _ := strings.Cut(lines[at], text)
		return ansi.StringWidth(before)
	}
	if fold, row := whatAt("b1 to b3"), whatAt("single"); fold != row {
		t.Errorf("the fold's cell sits at column %d and a row's at %d", fold, row)
	}

	m.MoveCursorTo(frame.Groups[0].Rows[0].Key)
	m.Update(downKey)
	m.Update(spaceKey)
	has("space on the fold", " ▾ 3 rows", "      b1", "first", "third")
	m.Update(downKey)
	m.Update(downKey)
	m.Update(spaceKey)
	has("space on a row the fold holds", "▾ b2", "more text")

	m.Update(typedKey(RawRowsKey))
	has("Z", "BOARD  ·  raw rows", "   b1", "Z fold rows")
	lacks("Z", "3 rows")
	if m.Cursor() != "b2" {
		t.Errorf("Z moved the cursor off b2 to %q", m.Cursor())
	}
	m.Update(typedKey(RawRowsKey))
	m.MoveCursorTo("alone")
	m.Update(downKey)
	m.Update(enterKey)
	has("enter on the fold closes it", " ▸ 3 rows")
	m.Update(typedKey(RawRowsKey))
	if m.Cursor() != "b1" {
		t.Errorf("Z on the fold's row put the cursor on %q rather than its first row", m.Cursor())
	}
	m.Update(typedKey(RawRowsKey))

	m.setFilter("third")
	has("a filter", "  3 rows", "b3", "third")
	lacks("a filter", "first", "▸ 3 rows")

	noFoldRow := frame
	noFoldRow.FoldRow = nil
	unkeyed := noFoldRow
	unkeyed.Groups = slices.Clone(noFoldRow.Groups)
	unkeyed.Groups[0].Rows = slices.Clone(noFoldRow.Groups[0].Rows)
	for i := range unkeyed.Groups[0].Rows {
		unkeyed.Groups[0].Rows[i].Fold = ""
	}
	pal := gotermstyle.NewPalette(true)
	if got, want := Once(fixedBoard{noFoldRow}, pal, 80, 20), Once(fixedBoard{unkeyed}, pal, 80, 20); got != want {
		t.Errorf("fold keys with no FoldRow drew\n%s\nand rows with none drew\n%s", got, want)
	}
}

// A stream read newest first, whose FoldRow starts from a copy of a row: a read lands a row beside
// the one the cursor is on, and a later read forms a second fold of one key ahead of the one open.
func TestAReadThatFoldsTheCursorsRowOrFormsAFoldAheadKeepsTheCursorAndTheOpenFold(t *testing.T) {
	rows := func(keys ...string) []Row {
		out := make([]Row, 0, len(keys))
		for _, key := range keys {
			out = append(out, Row{Key: key, Fold: key[:1], Cells: []Cell{C(key, "")}})
		}
		return out
	}
	board := &fixedBoard{Frame{Columns: []Column{{Title: "NAME"}}, FoldRow: func(rows []Row) Row {
		row := rows[0]
		row.Cells = []Cell{C(strconv.Itoa(len(rows))+" of "+rows[0].Fold, "")}
		return row
	}}}
	read := func(m *Model, keys ...string) {
		board.frame.Groups = []Group{{Key: "g", Rows: rows(keys...)}}
		if m != nil {
			m.Rebuild()
		}
	}
	read(nil, "p1", "a1", "q1")
	m := NewModel(board, gotermstyle.NewPalette(false))
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m.MoveCursorTo("a1")
	read(m, "n1", "n2", "a2", "a1", "p1", "q1")
	fold := m.Cursor()
	if !strings.Contains(m.screen(), "▌ ▸ 2 of a") || fold == "a2" {
		t.Fatalf("the cursor should sit on the fold that took a1 in, keyed apart from a2, and sits on %q:\n%s", fold, m.screen())
	}
	m.Update(spaceKey)
	read(m, "a3", "n1", "n2", "a2", "a1", "p1", "q1")
	read(m, "a4", "a3", "n1", "n2", "a2", "a1", "p1", "q1")
	if m.Cursor() != fold || !strings.Contains(m.screen(), "▌ ▾ 2 of a") || !strings.Contains(m.screen(), "  ▸ 2 of a") {
		t.Errorf("a fold of a forming ahead took the open fold's key or cursor, now on %q:\n%s", m.Cursor(), m.screen())
	}
}
