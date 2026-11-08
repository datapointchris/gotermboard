package gotermboard

import (
	"fmt"
	"slices"
	"strconv"

	tea "charm.land/bubbletea/v2"
)

// LockKey locks every tab to what the cursor's row names, and lifts the lock from any tab. It is
// cross-filtering, as Tableau's brushing and linking or Grafana's ad hoc filters are.
const LockKey = "*"

// lockActions is LockKey, where there are tabs to lock across.
func (t *Tabs) lockActions() []Action {
	if len(t.models) < 2 {
		return nil
	}
	return []Action{{
		Keys: []string{LockKey}, Label: "lock every tab to what the row names, or lift the lock", Footer: LockKey + " lock",
		Run: t.pickLock,
		Applies: func(m *Model) bool {
			return t.locked != nil || len(m.CursorEntities()) > 0
		},
	}}
}

func (t *Tabs) pickLock(m *Model) tea.Cmd {
	if t.locked != nil {
		t.lock(nil)
		return nil
	}
	refs := m.CursorEntities()
	if len(refs) == 1 {
		t.lock(&refs[0])
		return nil
	}
	listing := make([]Line, 0, len(refs))
	for i, ref := range refs {
		listing = append(listing, Line{C(strconv.Itoa(i+1), Bold), C("  "+ref.Kind+" "+ref.ID, "")})
	}
	lockedNothing := func() tea.Cmd {
		m.Say("locked nothing")
		return nil
	}
	m.Ask(Question{
		Listing: listing, Text: fmt.Sprintf("lock to 1 to %d:", len(refs)), Typed: true, Cancel: lockedNothing,
		Answer: func(typed string) tea.Cmd {
			n, err := strconv.Atoi(typed)
			if err != nil || n < 1 || n > len(refs) {
				return lockedNothing()
			}
			t.lock(&refs[n-1])
			return nil
		},
	})
	return nil
}

// lock keeps only the lines naming ref in every tab, and nil lifts it. The tab showing goes to
// the first line naming it, as a jump's search does.
func (t *Tabs) lock(ref *EntityRef) {
	t.locked = ref
	for _, model := range t.models {
		model.display.locked = ref
	}
	shown := t.active()
	shown.Rebuild()
	if ref != nil {
		shown.seeking = &seek{ref: *ref, tab: t.names[t.at]}
		shown.seekEntity()
	}
	shown.displayChanged()
}

// holdsNothingLocked says a tab other than the one showing has nothing of what is locked, so the
// tab line leaves it out. Only a frame built from reads that had all landed answers that. A tab
// never shown, or last drawn while a read was under way or failing, may still hold it, so it
// stays. A hidden tab builds no frames, so a read landing after it hid does not change that.
func (t *Tabs) holdsNothingLocked(at int) bool {
	m := t.models[at]
	if t.locked == nil || at == t.at || !m.whole {
		return false
	}
	_, _, holds := entityLine(m.frame.Groups, *t.locked, nil)
	return !holds
}

// listed is each tab the tab line names, in the order tab cycles them.
func (t *Tabs) listed() []int {
	at := make([]int, 0, len(t.models))
	for i := range t.models {
		if !t.holdsNothingLocked(i) {
			at = append(at, i)
		}
	}
	return at
}

// nextListed is the tab by steps along the tab line from the one showing, skipping what a lock
// leaves out of it.
func (t *Tabs) nextListed(by int) int {
	listed := t.listed()
	here := slices.Index(listed, t.at)
	return listed[((here+by)%len(listed)+len(listed))%len(listed)]
}

// showLocked is show, with the cursor sent to the first line naming what is locked once the
// tab's reads land.
func (t *Tabs) showLocked(at int) tea.Cmd {
	if t.locked != nil {
		t.models[at].seeking = &seek{ref: *t.locked, tab: t.names[at]}
	}
	return t.show(at)
}

// lockLine is the footer's word on the lock, while one holds.
func (m *Model) lockLine() string {
	if m.display.locked == nil {
		return ""
	}
	return fmt.Sprintf("locked to %s %s · %s lifts", m.display.locked.Kind, m.display.locked.ID, LockKey)
}
