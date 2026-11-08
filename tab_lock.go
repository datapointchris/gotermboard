package gotermboard

import (
	"fmt"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
)

// LockKey locks every tab to what the cursor's row names, and lifts the lock from any tab. It is
// cross-filtering, as Tableau's brushing and linking or Grafana's ad hoc filters are.
const LockKey = "*"

// tabLock is what LockKey locked every tab to, the one value Tabs and each of its tabs read.
type tabLock struct {
	ref *EntityRef
}

// lockActions is LockKey, where there are tabs to lock across.
func (t *Tabs) lockActions() []Action {
	if len(t.models) < 2 {
		return nil
	}
	return []Action{{
		Keys: []string{LockKey}, Label: "lock every tab to what the row names, or lift the lock", Footer: LockKey + " lock",
		Run: t.pickLock,
		Applies: func(m *Model) bool {
			return t.lock.ref != nil || len(m.CursorEntities()) > 0
		},
	}}
}

func (t *Tabs) pickLock(m *Model) tea.Cmd {
	if t.lock.ref != nil {
		t.lockTo(nil)
		return nil
	}
	refs := m.CursorEntities()
	return askByNumber(m, "lock to", "locked nothing", refLabels(refs), func(n int) tea.Cmd {
		t.lockTo(&refs[n])
		return nil
	})
}

// lockTo keeps only the lines naming ref in every tab, and nil lifts it.
func (t *Tabs) lockTo(ref *EntityRef) {
	t.lock.ref = ref
	t.seekLocked(t.at)
	shown := t.active()
	shown.Rebuild()
	shown.displayChanged()
}

// seekLocked sends the cursor of the tab at at to the first line naming what is locked, once its
// reads land, where no other search is under way and the line it is on does not show.
func (t *Tabs) seekLocked(at int) {
	m := t.models[at]
	if ref := m.display.narrowsTo(); ref != nil && m.seeking == nil && !m.cursorShows() {
		m.seeking = &seek{ref: *ref, tab: t.names[at]}
	}
}

// holdsNothingLocked says a tab other than the one showing has nothing of what is locked, so the
// tab line leaves it out. Its last frame answers that only as lookFor's holdsNone, because a
// hidden tab reads nothing and keeps the frame it was last drawn with. The entity page hides
// nothing under a lock, so it stays.
func (t *Tabs) holdsNothingLocked(at int, now time.Time) bool {
	m := t.models[at]
	ref := m.display.narrowsTo()
	if ref == nil || at == t.at {
		return false
	}
	_, _, f := m.lookFor(*ref, now)
	return f == holdsNone
}

// listed is each tab the tab line names at now, in the order tab cycles them.
func (t *Tabs) listed(now time.Time) []int {
	at := make([]int, 0, len(t.models))
	for i := range t.models {
		if !t.holdsNothingLocked(i, now) {
			at = append(at, i)
		}
	}
	return at
}

// nextListed is the tab by steps along the tab line from the one showing, skipping what a lock
// leaves out of it.
func (t *Tabs) nextListed(by int) int {
	listed := t.listed(time.Now())
	here := slices.Index(listed, t.at)
	return listed[((here+by)%len(listed)+len(listed))%len(listed)]
}

// lockLine is the footer's word on the lock, while one holds.
func (m *Model) lockLine() string {
	ref := m.display.lockedTo()
	if ref == nil {
		return ""
	}
	return fmt.Sprintf("locked to %s %s · %s lifts", ref.Kind, ref.ID, LockKey)
}
