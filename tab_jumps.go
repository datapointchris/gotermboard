package gotermboard

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// EntityRef names one thing a row or a group is about, by a kind and an id every view agrees on,
// as a Backstage entity reference or a Kubernetes object reference does. The package gives no
// kind a meaning, so a caller names its own.
type EntityRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// EntityBoard is a Board that shows some kinds of entity, so a row in another tab naming one can
// jump here.
type EntityBoard interface {
	// EntityKinds is the kinds a jump lands on here.
	EntityKinds() []string
	// ShowEntity is told the entity a jump came for, before its tab shows, so the board can
	// change what it reads. It says whether the board now shows the entity whole, as a page
	// given to one thing does. Where it does not, the cursor goes to the first group or row
	// naming the entity once the reads land. Either way the tab starts at its top and reads
	// every source at once, as Model.Restart does.
	ShowEntity(ref EntityRef) (whole bool)
}

// Outside is a program that shows one kind of entity whole, such as a pull request tool opened
// on one pull request. A jump to it hands it the terminal and pushes nothing, because quitting
// it comes back to the row the jump left.
type Outside struct {
	Kind string
	// Name names it among the places a row goes, as a tab's name does.
	Name string
	// Command is the program opened on one entity's id, which the caller's own runner starts,
	// or nil where it cannot open that id. It ends as Model.HandOver's program does: the end
	// of its stderr is kept, the board whose row it left receives the HandedBack, and a
	// failure says its last line.
	Command func(id string) tea.ExecCommand
}

// JumpKey goes to what the cursor's row names, where another tab or a program outside shows it.
const JumpKey = "t"

// OpensOutside adds programs that a row's entity can jump to, after the tabs that show its kind.
func (t *Tabs) OpensOutside(programs ...Outside) {
	t.outside = append(t.outside, programs...)
	t.hostActions()
}

// jumpTarget is one place an entity goes: a tab, or a program outside.
type jumpTarget struct {
	ref     EntityRef
	tab     int
	outside *Outside
}

func (j jumpTarget) where(t *Tabs) string {
	if j.outside != nil {
		return j.outside.Name
	}
	return t.names[j.tab]
}

// jumpTargets is every place the cursor's entities go from the tab showing, in the order the row
// names them: each other tab that shows the kind, then each program outside. A tab is never its
// own target, because the row is already there. The entity page is PageKey's, so t leaves it out.
func (t *Tabs) jumpTargets(m *Model) []jumpTarget {
	var targets []jumpTarget
	for _, ref := range m.CursorEntities() {
		for at, model := range t.models {
			if t.hasPage && at == t.page {
				continue
			}
			if board, ok := model.board.(EntityBoard); ok && at != t.at && slices.Contains(board.EntityKinds(), ref.Kind) {
				targets = append(targets, jumpTarget{ref: ref, tab: at})
			}
		}
		for i := range t.outside {
			if t.outside[i].Kind == ref.Kind {
				targets = append(targets, jumpTarget{ref: ref, tab: -1, outside: &t.outside[i]})
			}
		}
	}
	return targets
}

// canJump says some row could go somewhere: a program outside shows a kind, or a tab does with
// another tab to jump from.
func (t *Tabs) canJump() bool {
	return len(t.outside) > 0 || len(t.models) > 1 && slices.ContainsFunc(t.models, func(m *Model) bool {
		_, ok := m.board.(EntityBoard)
		return ok
	})
}

// jumpActions is t, which goes straight to the one place the cursor's row names, or asks which
// where it names several. Where no row could go anywhere, Tabs binds no t.
func (t *Tabs) jumpActions() []Action {
	if !t.canJump() {
		return nil
	}
	return []Action{{
		Keys: []string{JumpKey}, Label: "go to what the row names, in another tab or program; esc comes back", Footer: "t go to",
		Run:     t.pickJump,
		Applies: func(m *Model) bool { return len(t.jumpTargets(m)) > 0 },
	}}
}

// PageKey opens the entity page on what the cursor's row names, from any tab, the page's own
// included.
const PageKey = "i"

// EntityPage names the tab that shows any one entity whole, as Backstage gives each entity one
// page. Its board is an EntityBoard. PageKey goes there, and JumpKey leaves it out of its list,
// because a row naming three things would otherwise list each twice. A lock marks the lines on
// the page naming what is locked and hides none, because the page already shows one thing whole.
func (t *Tabs) EntityPage(name string) error {
	at := slices.Index(t.names, name)
	if at < 0 {
		return fmt.Errorf("no tab %q: the tabs are %s", name, strings.Join(t.names, ", "))
	}
	if _, ok := t.models[at].board.(EntityBoard); !ok {
		return fmt.Errorf("tab %q shows no entity, so it cannot be the entity page", name)
	}
	t.page, t.hasPage = at, true
	t.models[at].display.wholePage = true
	t.hostActions()
	return nil
}

// OpenPage opens the program on the entity page showing ref, as PageKey would from a row naming
// it, so esc from the page opened on another thing comes back to ref. It is for before the
// program starts, after EntityPage.
func (t *Tabs) OpenPage(ref EntityRef) error {
	if !t.hasPage {
		return errors.New("no tab is the entity page, so EntityPage has to name one first")
	}
	t.models[t.page].board.(EntityBoard).ShowEntity(ref)
	t.at, t.pageShows = t.page, &ref
	return nil
}

// pageRefs is what the cursor's row names that the entity page shows.
func (t *Tabs) pageRefs(m *Model) []EntityRef {
	if !t.hasPage {
		return nil
	}
	kinds := t.models[t.page].board.(EntityBoard).EntityKinds()
	var refs []EntityRef
	for _, ref := range m.CursorEntities() {
		if slices.Contains(kinds, ref.Kind) {
			refs = append(refs, ref)
		}
	}
	return refs
}

// pageActions is PageKey, which opens the page on the one thing the row names, or asks which
// where it names several.
func (t *Tabs) pageActions() []Action {
	if !t.hasPage {
		return nil
	}
	return []Action{{
		Keys: []string{PageKey}, Label: "open everything known about what the row names, in " + t.names[t.page] + "; esc comes back",
		Footer:  PageKey + " " + t.names[t.page],
		Run:     t.pickPage,
		Applies: func(m *Model) bool { return len(t.pageRefs(m)) > 0 },
	}}
}

func (t *Tabs) pickPage(m *Model) tea.Cmd {
	refs := t.pageRefs(m)
	return askByNumber(m, "open", "opened nothing", refLabels(refs), func(n int) tea.Cmd {
		return t.jump(jumpTarget{ref: refs[n], tab: t.page})
	})
}

// askByNumber acts on the one choice there is, or lists them by number and acts on the one
// typed. ask is the question's verb, and nothing what the footer says to any other answer.
func askByNumber(m *Model, ask, nothing string, labels []Line, pick func(int) tea.Cmd) tea.Cmd {
	switch len(labels) {
	case 0:
		return nil
	case 1:
		return pick(0)
	}
	listing := make([]Line, 0, len(labels))
	for i, label := range labels {
		listing = append(listing, append(Line{C(strconv.Itoa(i+1), Bold), C("  ", "")}, label...))
	}
	cancel := func() tea.Cmd {
		m.Say(nothing)
		return nil
	}
	m.Ask(Question{
		Listing: listing, Text: fmt.Sprintf("%s 1 to %d:", ask, len(labels)), Typed: true, Cancel: cancel,
		Answer: func(typed string) tea.Cmd {
			n, err := strconv.Atoi(typed)
			if err != nil || n < 1 || n > len(labels) {
				return cancel()
			}
			return pick(n - 1)
		},
	})
	return nil
}

// refLabels is each ref as a numbered choice reads it.
func refLabels(refs []EntityRef) []Line {
	labels := make([]Line, 0, len(refs))
	for _, ref := range refs {
		labels = append(labels, Line{C(ref.Kind+" "+ref.ID, "")})
	}
	return labels
}

// backActions is esc while a jump brought the reader here. It sits ahead of the board's own keys,
// so the tab a jump opened is that tab's top, and esc there goes back rather than deeper into it.
func (t *Tabs) backActions() []Action {
	return []Action{{
		Keys: []string{"esc"}, Label: "go back to the tab the jump left, on the row it left", Footer: "esc back",
		Run:     func(*Model) tea.Cmd { return t.goBack() },
		Applies: func(*Model) bool { return len(t.back) > 0 },
	}}
}

func (t *Tabs) pickJump(m *Model) tea.Cmd {
	targets := t.jumpTargets(m)
	labels := make([]Line, 0, len(targets))
	for _, target := range targets {
		labels = append(labels, Line{C(target.where(t)+"  ", Cyan), C(target.ref.Kind+" "+target.ref.ID, "")})
	}
	return askByNumber(m, "go to", "went nowhere", labels, func(n int) tea.Cmd { return t.jump(targets[n]) })
}

// jump goes to target. A program outside is handed the terminal. A tab is shown at the entity,
// with the tab it left pushed for esc, and its cursor goes where the board says.
func (t *Tabs) jump(target jumpTarget) tea.Cmd {
	if outside := target.outside; outside != nil {
		return t.active().openOutside(outside, target.ref)
	}
	step := backStep{tab: t.at, cursor: t.active().Cursor()}
	if t.hasPage && t.at == t.page {
		step.page = t.pageShows
	}
	t.back = append(t.back, step)
	return t.showEntity(target.tab, target.ref, "")
}

// showEntity tells the tab at at the entity, then shows it. Its cursor goes to the line keyed
// backTo where that is set, and otherwise where the board says.
func (t *Tabs) showEntity(at int, ref EntityRef, backTo string) tea.Cmd {
	to := t.models[at]
	whole := to.board.(EntityBoard).ShowEntity(ref)
	to.toTop()
	switch {
	case backTo != "":
		to.seeking = &seek{key: backTo}
	case !whole:
		to.seeking = &seek{ref: ref, tab: t.names[at]}
	}
	if t.hasPage && at == t.page {
		t.pageShows = &ref
	}
	return t.show(at)
}

// openOutside hands the terminal to the program outside on ref, as HandOver does.
func (m *Model) openOutside(outside *Outside, ref EntityRef) tea.Cmd {
	program := outside.Command(ref.ID)
	if program == nil {
		m.Say(fmt.Sprintf("%s cannot open %s %s", outside.Name, ref.Kind, ref.ID))
		return nil
	}
	return m.handOverProgram(&runnerHandedOver{ExecCommand: program}, func(m *Model, err error, stderr []byte) tea.Cmd {
		if err != nil {
			m.Say(fmt.Sprintf("%s on %s %s: %s", outside.Name, ref.Kind, ref.ID, LastLine(stderr, err)))
		}
		return nil
	})
}

// runnerHandedOver is a program its caller's own runner starts, as Bubble Tea's Exec runs it for
// handOver. What it writes to stderr reaches the terminal, and the end of it is kept.
type runnerHandedOver struct {
	tea.ExecCommand
	tail stderrTail
}

// SetStderr is tea.ExecCommand's.
func (r *runnerHandedOver) SetStderr(w io.Writer) {
	if w == nil {
		r.ExecCommand.SetStderr(&r.tail)
		return
	}
	r.ExecCommand.SetStderr(io.MultiWriter(w, &r.tail))
}

func (r *runnerHandedOver) kept() []byte { return r.tail.kept() }

// goBack shows the tab the newest jump left, whose cursor stayed where it was. A jump from the
// entity page to itself changed what the page shows, so going back opens it on that again, with
// the cursor on the line it left once the reads land.
func (t *Tabs) goBack() tea.Cmd {
	from := t.back[len(t.back)-1]
	t.back = t.back[:len(t.back)-1]
	if from.page != nil && (t.pageShows == nil || *from.page != *t.pageShows) {
		return t.showEntity(from.tab, *from.page, from.cursor)
	}
	return t.show(from.tab)
}

// seek is what the cursor goes to once the reads land: the first line naming the entity a jump
// or the lock came for, or the line keyed key that esc comes back to. tab is the tab it went to,
// which the footer names where no line names the entity.
type seek struct {
	ref EntityRef
	key string
	tab string
}

// CursorEntities is what the group or row under the cursor names.
func (m *Model) CursorEntities() []EntityRef {
	n, ok := m.selected()
	switch {
	case !ok:
		return nil
	case n.kind == nodeGroup:
		return n.group.Entities
	}
	return n.row.Entities
}

// seekEntity puts the cursor where seeking says once the reads land. For an entity that is the
// first line naming it, opening the groups that hold it, and clearing a filter or lifting a lock
// on another thing that hides it, as the footer then says. Where no line names it, the footer
// says so, and the cursor stays where the tab starts. A read that failed is no answer, so the
// footer then says the tab may hold it.
func (m *Model) seekEntity() {
	s := m.seeking
	if s == nil {
		return
	}
	if s.key != "" {
		if !m.MoveCursorTo(s.key) && !m.drawnFrom.landed {
			return
		}
		m.seeking = nil
		return
	}
	key, holding, f := m.lookFor(s.ref, time.Now())
	switch f {
	case unanswered:
		return
	case holdsNone:
		m.said = fmt.Sprintf("%s holds no %s %s", s.tab, s.ref.Kind, s.ref.ID)
	case readFailed:
		m.said = fmt.Sprintf("%s could not read, so it may still hold %s %s", s.tab, s.ref.Kind, s.ref.ID)
	}
	m.seeking = nil
	if f != found {
		return
	}
	for _, group := range holding {
		m.display.fold(group, false)
	}
	if m.MoveCursorTo(key) {
		return
	}
	if m.display.filter != "" {
		m.display.filter = ""
		if m.MoveCursorTo(key) {
			return
		}
	}
	if locked := m.display.narrowsTo(); locked != nil && *locked != s.ref {
		m.said = fmt.Sprintf("the lock on %s %s hid %s %s here, so it lifted", locked.Kind, locked.ID, s.ref.Kind, s.ref.ID)
		m.display.lock.ref = nil
		m.MoveCursorTo(key)
	}
}

// entityLine is the key of the first group or row naming ref, in the order the table draws them,
// and the keys of the groups holding it.
func entityLine(groups []Group, ref EntityRef, holding []string) (string, []string, bool) {
	for i := range groups {
		g := &groups[i]
		if slices.Contains(g.Entities, ref) {
			return g.Key, holding, true
		}
		inside := append(slices.Clone(holding), g.Key)
		for j := range g.Rows {
			if slices.Contains(g.Rows[j].Entities, ref) {
				return g.Rows[j].Key, inside, true
			}
		}
		if key, held, found := entityLine(g.Groups, ref, inside); found {
			return key, held, true
		}
	}
	return "", nil, false
}
