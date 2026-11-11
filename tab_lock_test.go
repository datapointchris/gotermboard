package gotermboard

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/datapointchris/gotermstyle"
)

func newThingTabs(t *testing.T, boards ...*thingsBoard) *Tabs {
	t.Helper()
	var list []Tab
	for i, board := range boards {
		list = append(list, Tab{Name: []string{"one", "two", "three", "four"}[i], Board: board})
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
	tabs := newThingTabs(t, &thingsBoard{ids: []string{"1", "2", "3"}}, &thingsBoard{ids: []string{"3", "2"}})
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
	late := NewSource("late", time.Hour, func(context.Context) (int, string, error) { return 0, "", nil })
	threes := NewSource("threes", time.Minute, func(context.Context) (int, string, error) { return 0, "", nil })
	threes.Refresh(t.Context())
	three := &thingsBoard{ids: []string{"3"}, sources: []Refresher{threes}}
	tabs := newThingTabs(t, &thingsBoard{ids: []string{"1", "2"}}, &thingsBoard{ids: []string{"2"}},
		three, &thingsBoard{sources: []Refresher{late}})
	for range 4 {
		press(tabs, tea.KeyPressMsg{Code: tea.KeyTab})
	}
	late.Refresh(t.Context())
	press(tabs, downKey)

	press(tabs, typedKey(LockKey))
	if line := tabs.tabLine(); !strings.Contains(line, "two") || strings.Contains(line, "three") || !strings.Contains(line, "four") {
		t.Fatalf("locked to thing 2, the tab line should name two, leave out three, and keep four, drawn before its read landed: %q", line)
	}
	press(tabs, tea.KeyPressMsg{Code: tea.KeyTab})
	press(tabs, tea.KeyPressMsg{Code: tea.KeyTab})
	press(tabs, tea.KeyPressMsg{Code: tea.KeyTab})
	if tabs.at != 0 {
		t.Errorf("tab should cycle one, two and four, and three times lands back on one, not on %s", tabs.names[tabs.at])
	}
	if listed := tabs.listed(time.Now().Add(2 * time.Minute)); !slices.Contains(listed, 2) {
		t.Errorf("three's reads come due a minute on, and a hidden tab never reads, so it should be listed again then: %v", listed)
	}

	three.ids = []string{"3", "2"}
	tabs.show(2)
	tabs.show(0)
	if line := tabs.tabLine(); !strings.Contains(line, "three") {
		t.Errorf("three was shown holding thing 2, so the tab line should name it from that frame: %q", line)
	}
	three.ids = []string{"3"}
	tabs.show(2)
	tabs.show(0)

	press(tabs, typedKey(LockKey))
	if line := tabs.tabLine(); !strings.Contains(line, "three") {
		t.Errorf("lifting the lock should list three again: %q", line)
	}
}

// pageBoard is an entity page, showing the thing it was opened on as a row naming each thing
// linked to it.
type pageBoard struct {
	shows  EntityRef
	linked map[string][]string
}

func (b *pageBoard) Sources() []Refresher { return nil }
func (b *pageBoard) Groupings() []string  { return nil }
func (b *pageBoard) Frame(time.Time, string) Frame {
	group := Group{Key: "linked", Title: Line{C("LINKED TO "+b.shows.ID, Bold)}}
	for _, id := range b.linked[b.shows.ID] {
		group.Rows = append(group.Rows, Row{Key: "row-" + id, Cells: []Cell{C("row "+id, "")}, Entities: []EntityRef{{Kind: "thing", ID: id}}})
	}
	return Frame{Columns: []Column{{Title: "NAME"}}, Groups: []Group{group}}
}
func (b *pageBoard) EntityKinds() []string { return []string{"thing"} }
func (b *pageBoard) ShowEntity(ref EntityRef) bool {
	b.shows = ref
	return true
}

// A host opens its page on the thing a flag names, the reader opens another from a row of it,
// locks to a third, and jumps to a list tab the lock would hide.
func TestThePageOpenedOnAThingComesBackToItOnTheRowLeftAndShowsWholeUnderALock(t *testing.T) {
	page := &pageBoard{linked: map[string][]string{"a": {"b", "c", "d"}, "c": {"e"}}}
	tabs, err := NewTabs([]Tab{{Name: "list", Board: &thingsBoard{ids: []string{"b", "c"}}}, {Name: "page", Board: page}}, gotermstyle.NewPalette(false), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := tabs.EntityPage("page"); err != nil {
		t.Fatal(err)
	}
	if err := tabs.OpenPage(EntityRef{Kind: "thing", ID: "a"}); err != nil {
		t.Fatal(err)
	}
	tabs.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	tabs.active().reveal(time.Now())
	press(tabs, downKey)

	press(tabs, typedKey(PageKey))
	if page.shows.ID != "c" {
		t.Fatalf("i on the row naming thing c should open the page on c, not %q", page.shows.ID)
	}
	press(tabs, tea.KeyPressMsg{Code: tea.KeyEscape})
	if got := page.shows.ID + " " + tabs.active().Cursor(); got != "a row-c" {
		t.Fatalf("esc should open the page on a again, on the row i left, not %q", got)
	}

	press(tabs, upKey)
	press(tabs, typedKey(LockKey))
	if got := tabs.View().Content; !strings.Contains(got, LockGlyph+" row b") || !strings.Contains(got, "row d") {
		t.Fatalf("locked to thing b, the page should mark its row and still show every row:\n%s", got)
	}
	press(tabs, downKey)
	press(tabs, typedKey(JumpKey))
	if got := tabs.names[tabs.at] + " " + tabs.active().Cursor(); got != "list row-c" || tabs.lock.ref != nil {
		t.Fatalf("a jump to the list's row naming c, which the lock on b hides, should lift the lock and land there, not on %q (lock %v)", got, tabs.lock.ref)
	}
	if got := tabs.View().Content; !strings.Contains(got, "the lock on thing b hid thing c here, so it lifted") {
		t.Errorf("the footer should say the lock lifted and why:\n%s", got)
	}
}
