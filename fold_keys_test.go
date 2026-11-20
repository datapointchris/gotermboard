package gotermboard

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/datapointchris/gotermstyle"
)

// outline is what the table shows: each group and row by its key, and each row's open text.
func outline(m *Model) []string {
	var lines []string
	for _, n := range m.nodes() {
		switch n.kind {
		case nodeGroup, nodeRow:
			lines = append(lines, n.key)
		case nodeDetail:
			lines = append(lines, n.cell.Text)
		}
	}
	return lines
}

func TestTheFoldKeysOpenAndFoldTheWholeTableOrWhatIsUnderTheCursor(t *testing.T) {
	row := func(key string) Row {
		return Row{Key: key, Cells: []Cell{C(key, "")}, Detail: []Cell{C(key+" text", "")}}
	}
	group := func(key string, groups ...Group) Group {
		return Group{Key: key, Title: Line{C(key, "")}, Rows: []Row{row(key + "-row")}, Groups: groups}
	}
	frame := Frame{StartAt: "a", Columns: []Column{{Title: "NAME"}}, Groups: []Group{
		group("a", group("a1", group("a2"))),
		group("b"),
	}}
	m := NewModel(fixedBoard{frame}, gotermstyle.NewPalette(false))
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	press := func(keys string) {
		for _, key := range keys {
			m.Update(typedKey(string(key)))
		}
	}
	every := []string{"a", "a-row", "a-row text", "a1", "a1-row", "a1-row text", "a2", "a2-row", "a2-row text", "b", "b-row", "b-row text"}
	for _, step := range []struct {
		on, keys string
		want     []string
	}{
		{"a2-row", "zR", every},
		{"a2-row", "zM", []string{"a", "b"}},
		{"a", "zO", []string{"a", "a-row", "a-row text", "a1", "a1-row", "a1-row text", "a2", "a2-row", "a2-row text", "b"}},
		{"a1", "zC", []string{"a", "a-row", "a-row text", "a1", "b"}},
		{"b", "zO", []string{"a", "a-row", "a-row text", "a1", "b", "b-row", "b-row text"}},
		{"a1", "zO", every},
	} {
		if !m.MoveCursorTo(step.on) {
			t.Fatalf("%s should show before %s: %q", step.on, step.keys, outline(m))
		}
		press(step.keys)
		if got := outline(m); !slices.Equal(got, step.want) {
			t.Errorf("%s on %s should show\n%q, not\n%q", step.keys, step.on, step.want, got)
		}
	}
	press("zM")
	if m.Cursor() != "a" {
		t.Errorf("zM should take the cursor from a1 to a, the group that took its line in, not %q", m.Cursor())
	}
}

func TestZWaitsOnTheFooterAndAKeyNoFoldKeyEndsTheWaitLeavingTheTable(t *testing.T) {
	frame := Frame{StartAt: "first", Columns: []Column{{Title: "NAME"}}, Groups: []Group{{Key: "g", Title: Line{C("g", "")}, Rows: []Row{
		{Key: "first", Cells: []Cell{C("first", "")}, Detail: []Cell{C("first text", "")}},
		{Key: "second", Cells: []Cell{C("second", "")}},
	}}}}
	m := NewModel(fixedBoard{frame}, gotermstyle.NewPalette(false))
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	before := outline(m)
	m.Update(typedKey("z"))
	if footer := m.footer(); !strings.HasPrefix(footer[len(footer)-1], "z…  R open all") {
		t.Errorf("the footer should show the z waiting and the keys after it: %q", footer)
	}
	m.Update(typedKey("j"))
	if got := outline(m); !slices.Equal(got, before) || m.Cursor() != "first" {
		t.Errorf("j after z should leave the table as it was, %q on first, not %q on %q", before, got, m.Cursor())
	}
	if footer := m.footer(); strings.Contains(footer[len(footer)-1], "z…") {
		t.Errorf("the wait should end with the key after z: %q", footer)
	}
}
