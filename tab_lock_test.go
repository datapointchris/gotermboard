package gotermboard

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/datapointchris/gotermstyle"
)

func newThingTabs(t *testing.T, tabs ...[]string) *Tabs {
	t.Helper()
	var list []Tab
	for i, ids := range tabs {
		list = append(list, Tab{Name: []string{"one", "two", "three"}[i], Board: &thingsBoard{ids: ids}})
	}
	program, err := NewTabs(list, gotermstyle.NewPalette(false), "")
	if err != nil {
		t.Fatal(err)
	}
	program.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	program.active().reveal(time.Now())
	return program
}

func TestALockKeepsOnlyTheRowsNamingTheLockedThingUntilItLifts(t *testing.T) {
	tabs := newThingTabs(t, []string{"1", "2", "3"}, []string{"3", "2"})
	press(tabs, downKey)

	press(tabs, typedKey(LockKey))
	got := tabs.View().Content
	if strings.Contains(got, "row 1") || strings.Contains(got, "row 3") || !strings.Contains(got, LockGlyph+" row 2") {
		t.Fatalf("a lock on thing 2 should leave only its row, marked:\n%s", got)
	}

	press(tabs, typedKey(LockKey))
	if got := tabs.View().Content; !strings.Contains(got, "row 1") || strings.Contains(got, LockGlyph) {
		t.Errorf("lifting the lock should bring every row back unmarked:\n%s", got)
	}
}

func TestTheTabLineLeavesOutATabHoldingNothingLockedAndTabSkipsIt(t *testing.T) {
	tabs := newThingTabs(t, []string{"1", "2"}, []string{"2"}, []string{"3"})
	press(tabs, tea.KeyPressMsg{Code: tea.KeyTab})
	press(tabs, tea.KeyPressMsg{Code: tea.KeyTab})
	press(tabs, tea.KeyPressMsg{Code: tea.KeyTab})
	press(tabs, downKey)

	press(tabs, typedKey(LockKey))
	if line := tabs.tabLine(); !strings.Contains(line, "two") || strings.Contains(line, "three") {
		t.Fatalf("locked to thing 2, the tab line should name two and leave out three: %q", line)
	}
	press(tabs, tea.KeyPressMsg{Code: tea.KeyTab})
	press(tabs, tea.KeyPressMsg{Code: tea.KeyTab})
	if tabs.at != 0 {
		t.Errorf("tab should cycle one and two only, and twice lands back on one, not on %s", tabs.names[tabs.at])
	}

	press(tabs, typedKey(LockKey))
	if line := tabs.tabLine(); !strings.Contains(line, "three") {
		t.Errorf("lifting the lock should list three again: %q", line)
	}
}
