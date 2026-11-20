package gotermboard

import (
	"context"
	"fmt"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
)

// LockKey locks every tab to what the cursor's row names, and lifts the lock from any tab. It is
// cross-filtering, as Tableau's brushing and linking or Grafana's ad hoc filters are.
const LockKey = "*"

// LockCover answers what a lock on ref keeps beside the lines naming ref itself, such as each run,
// commit and issue a flow touched. CoverLocks registers one for a kind.
type LockCover func(ctx context.Context, ref EntityRef) ([]EntityRef, error)

// lockCoverWait bounds one LockCover answer.
const lockCoverWait = 30 * time.Second

// tabLock is what LockKey locked every tab to, the one value Tabs and each of its tabs read.
type tabLock struct {
	ref *EntityRef
	// covers is what ref's kind's LockCover answered for it, and nil until it answers.
	covers map[EntityRef]bool
	// answers counts the covers that landed, so a frame's answer to which line the lock keeps is
	// worked out again after one.
	answers int
	// asking says a LockCover answer is under way, so a tab shown meanwhile asks no second one.
	asking bool
	// coverNote is the footer's word on covers: under way, how many, or why the answer failed.
	coverNote string
}

// keeps says entities name what is locked: the thing itself, or a thing its cover answered.
func (l *tabLock) keeps(entities []EntityRef) bool {
	for _, entity := range entities {
		if entity == *l.ref || l.covers[entity] {
			return true
		}
	}
	return false
}

// lift ends the lock and what its cover answered.
func (l *tabLock) lift() {
	*l = tabLock{answers: l.answers + 1}
}

// lockCoveredMsg is a LockCover's answer for ref.
type lockCoveredMsg struct {
	ref    EntityRef
	covers []EntityRef
	err    error
}

// CoverLocks has a lock on a thing of kind keep, beside the lines naming it, every line naming
// what cover answers for it. The cover is asked as the lock is set and each time a tab shows, so
// what it covers follows a thing that grows while it is locked.
func (t *Tabs) CoverLocks(kind string, cover LockCover) {
	if t.covers == nil {
		t.covers = map[string]LockCover{}
	}
	t.covers[kind] = cover
}

// askCover asks the locked thing's kind's LockCover off the screen's loop, where one is
// registered and no answer is under way.
func (t *Tabs) askCover() tea.Cmd {
	if t.lock.ref == nil || t.lock.asking {
		return nil
	}
	cover := t.covers[t.lock.ref.Kind]
	if cover == nil {
		return nil
	}
	ref, ctx := *t.lock.ref, t.ctx
	t.lock.asking = true
	if t.lock.covers == nil {
		t.lock.coverNote = "reading what it covers"
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, lockCoverWait)
		defer cancel()
		covers, err := cover(ctx, ref)
		return lockCoveredMsg{ref: ref, covers: covers, err: err}
	}
}

// lockCovered keeps what a cover answered while the lock it was asked for holds. A failed answer
// keeps what an earlier one covered, and the footer says why.
func (t *Tabs) lockCovered(msg lockCoveredMsg) tea.Cmd {
	if t.lock.ref == nil || *t.lock.ref != msg.ref {
		return nil
	}
	t.lock.asking = false
	if msg.err != nil {
		t.lock.coverNote = "what it covers did not read: " + msg.err.Error()
		return nil
	}
	t.lock.covers = make(map[EntityRef]bool, len(msg.covers))
	for _, ref := range msg.covers {
		t.lock.covers[ref] = true
	}
	t.lock.answers++
	t.lock.coverNote = fmt.Sprintf("and the %d it covers", len(t.lock.covers))
	t.seekLocked(t.at)
	shown := t.active()
	shown.Rebuild()
	shown.displayChanged()
	return nil
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
		return t.lockTo(nil)
	}
	refs := m.CursorEntities()
	return askByNumber(m, "lock to", "locked nothing", refLabels(refs), func(n int) tea.Cmd {
		return t.lockTo(&refs[n])
	})
}

// lockTo keeps only the lines naming ref in every tab, and those naming what its kind's cover
// answers, and nil lifts it.
func (t *Tabs) lockTo(ref *EntityRef) tea.Cmd {
	t.lock.lift()
	t.lock.ref = ref
	cmd := t.askCover()
	t.seekLocked(t.at)
	shown := t.active()
	shown.Rebuild()
	shown.displayChanged()
	return cmd
}

// seekLocked sends the cursor of the tab at at to the first line the lock keeps, once its reads
// land, where no other search is under way and the line it is on does not show.
func (t *Tabs) seekLocked(at int) {
	m := t.models[at]
	if ref := m.display.narrowsTo(); ref != nil && m.seeking == nil && !m.cursorShows() {
		m.seeking = &seek{ref: *ref, tab: t.names[at], byLock: true}
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
	_, _, f := m.lookFor(*ref, true, now)
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
	if note := m.display.lock.coverNote; note != "" {
		return fmt.Sprintf("locked to %s %s, %s · %s lifts", ref.Kind, ref.ID, note, LockKey)
	}
	return fmt.Sprintf("locked to %s %s · %s lifts", ref.Kind, ref.ID, LockKey)
}
