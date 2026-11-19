package gotermboard

import (
	"maps"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/datapointchris/gotermstyle"
)

func TestARunOpensOnTheTabAndDisplaySettingsTheLastRunLeft(t *testing.T) {
	path := filepath.Join(t.TempDir(), "display.json")
	frame := Frame{Columns: []Column{{Title: "NAME"}}, Groups: []Group{{Key: "g", Groups: []Group{{Key: "inner", Rows: []Row{
		{Key: "r", Cells: []Cell{C("x", "")}, Under: C("the first line", ""), Detail: []Cell{C("more", "")}},
		{Key: "s1", Fold: "s", Cells: []Cell{C("y", "")}},
		{Key: "s2", Fold: "s", Cells: []Cell{C("z", "")}},
	}}}}}, FoldRow: func([]Row) Row { return Row{Cells: []Cell{C("two", "")}} }}
	open := func(on string) *Tabs {
		tabs, err := NewTabs([]Tab{{Name: "one", Board: groupedBoard{fixedBoard{frame}}}, {Name: "two", Board: groupedBoard{fixedBoard{frame}}}}, gotermstyle.NewPalette(false), on)
		if err != nil {
			t.Fatal(err)
		}
		tabs.RemembersDisplayIn(path)
		tabs.Update(tea.WindowSizeMsg{Width: PanelFrom, Height: 20})
		tabs.Init()
		return tabs
	}
	first := open("")
	// p, f, v and Z, then space opens the row the cursor starts on, g goes to the top group and
	// space folds it, and tab moves to the second tab.
	keys := []tea.KeyPressMsg{typedKey("p"), typedKey("f"), typedKey("v"), typedKey(RawRowsKey), spaceKey, typedKey("g"), spaceKey, tabKey}
	for _, key := range keys {
		runReads(press(first, key))
	}
	left := first.models[0]
	if !left.panelOff || !left.display.under || left.currentGrouping() != "two" || !left.display.raw || !left.display.opened["r"] ||
		len(left.display.folded) == 0 || first.at != 1 {
		t.Fatalf("the keys did not set every setting: panel off %v, summaries %v, grouping %q, raw rows %v, opened %v, folded %v, tab %d",
			left.panelOff, left.display.under, left.currentGrouping(), left.display.raw, left.display.opened, left.display.folded, first.at)
	}
	again := open("")
	one, two := again.models[0], again.models[1]
	if again.at != 1 {
		t.Errorf("opened on tab %d rather than the second, the last one moved to", again.at)
	}
	if !one.panelOff || !one.display.under || one.currentGrouping() != "two" || !one.display.raw ||
		!maps.Equal(one.display.opened, left.display.opened) || !maps.Equal(one.display.folded, left.display.folded) {
		t.Errorf("tab one opened on panel off %v, summaries %v, grouping %q, raw rows %v, opened %v, folded %v",
			one.panelOff, one.display.under, one.currentGrouping(), one.display.raw, one.display.opened, one.display.folded)
	}
	if two.panelOff || two.display.under || two.currentGrouping() != "one" || two.display.raw || len(two.display.folded)+len(two.display.opened) != 0 {
		t.Errorf("tab two was never changed, yet opened on panel off %v, summaries %v, grouping %q, raw rows %v, opened %v, folded %v",
			two.panelOff, two.display.under, two.currentGrouping(), two.display.raw, two.display.opened, two.display.folded)
	}
	if named := open("one"); named.at != 0 {
		t.Errorf("a tab named at open gave way to the remembered one")
	}
}

// Several programs share one remembered file, so each answers for itself which tab it reopens on.
func TestAProgramReopensOnItsOwnTabAndNeverTheEntityPage(t *testing.T) {
	tabs, err := NewTabs([]Tab{{Name: "one", Board: &thingsBoard{}}, {Name: "two", Board: &thingsBoard{}}, {Name: "page", Board: &pageBoard{}}}, gotermstyle.NewPalette(false), "two")
	if err != nil {
		t.Fatal(err)
	}
	if err := tabs.EntityPage("page"); err != nil {
		t.Fatal(err)
	}
	tabs.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	for _, step := range []struct{ moved, want string }{{"opened on two", "two"}, {"moved to the page", "two"}, {"moved to one", "one"}} {
		if got := tabs.ReopensOn(); got != step.want {
			t.Errorf("%s, it reopens on %q, want %q", step.moved, got, step.want)
		}
		press(tabs, tabKey)
	}
}
