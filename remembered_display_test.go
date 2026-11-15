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
	}}}}}}
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
	// p, f and v, then space opens the row the cursor starts on, g goes to the top group and
	// space folds it, and tab moves to the second tab.
	for _, key := range []tea.KeyPressMsg{typedKey("p"), typedKey("f"), typedKey("v"), spaceKey, typedKey("g"), spaceKey, tabKey} {
		runReads(press(first, key))
	}
	left := first.models[0]
	if !left.panelOff || !left.display.under || left.currentGrouping() != "two" || !left.display.opened["r"] || len(left.display.folded) == 0 || first.at != 1 {
		t.Fatalf("the keys did not set every setting: panel off %v, summaries %v, grouping %q, opened %v, folded %v, tab %d",
			left.panelOff, left.display.under, left.currentGrouping(), left.display.opened, left.display.folded, first.at)
	}
	again := open("")
	one, two := again.models[0], again.models[1]
	if again.at != 1 {
		t.Errorf("opened on tab %d rather than the second, the last one moved to", again.at)
	}
	if !one.panelOff || !one.display.under || one.currentGrouping() != "two" ||
		!maps.Equal(one.display.opened, left.display.opened) || !maps.Equal(one.display.folded, left.display.folded) {
		t.Errorf("tab one opened on panel off %v, summaries %v, grouping %q, opened %v, folded %v",
			one.panelOff, one.display.under, one.currentGrouping(), one.display.opened, one.display.folded)
	}
	if two.panelOff || two.display.under || two.currentGrouping() != "one" || len(two.display.folded)+len(two.display.opened) != 0 {
		t.Errorf("tab two was never changed, yet opened on panel off %v, summaries %v, grouping %q, opened %v, folded %v",
			two.panelOff, two.display.under, two.currentGrouping(), two.display.opened, two.display.folded)
	}
	if named := open("one"); named.at != 0 {
		t.Errorf("a tab named at open gave way to the remembered one")
	}
}
