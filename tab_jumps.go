package gotermboard

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

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
// because a row naming three things would otherwise list each twice.
func (t *Tabs) EntityPage(name string) error {
	at := slices.Index(t.names, name)
	if at < 0 {
		return fmt.Errorf("no tab %q: the tabs are %s", name, strings.Join(t.names, ", "))
	}
	if _, ok := t.models[at].board.(EntityBoard); !ok {
		return fmt.Errorf("tab %q shows no entity, so it cannot be the entity page", name)
	}
	t.page, t.hasPage = at, true
	t.hostActions()
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
	switch len(refs) {
	case 0:
		return nil
	case 1:
		return t.jump(jumpTarget{ref: refs[0], tab: t.page})
	}
	listing := make([]Line, 0, len(refs))
	for i, ref := range refs {
		listing = append(listing, Line{C(strconv.Itoa(i+1), Bold), C("  "+ref.Kind+" "+ref.ID, "")})
	}
	openedNothing := func() tea.Cmd {
		m.Say("opened nothing")
		return nil
	}
	m.Ask(Question{
		Listing: listing, Text: fmt.Sprintf("open 1 to %d:", len(refs)), Typed: true, Cancel: openedNothing,
		Answer: func(typed string) tea.Cmd {
			n, err := strconv.Atoi(typed)
			if err != nil || n < 1 || n > len(refs) {
				return openedNothing()
			}
			return t.jump(jumpTarget{ref: refs[n-1], tab: t.page})
		},
	})
	return nil
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
	switch len(targets) {
	case 0:
		return nil
	case 1:
		return t.jump(targets[0])
	}
	listing := make([]Line, 0, len(targets))
	for i, target := range targets {
		listing = append(listing, Line{C(strconv.Itoa(i+1), Bold), C("  "+target.where(t)+"  ", Cyan), C(target.ref.Kind+" "+target.ref.ID, "")})
	}
	wentNowhere := func() tea.Cmd {
		m.Say("went nowhere")
		return nil
	}
	m.Ask(Question{
		Listing: listing, Text: fmt.Sprintf("go to 1 to %d:", len(targets)), Typed: true, Cancel: wentNowhere,
		Answer: func(typed string) tea.Cmd {
			n, err := strconv.Atoi(typed)
			if err != nil || n < 1 || n > len(targets) {
				return wentNowhere()
			}
			return t.jump(targets[n-1])
		},
	})
	return nil
}

// jump goes to target. A program outside is handed the terminal. A tab is shown at the entity,
// with the tab it left pushed for esc, and its cursor goes where the board says.
func (t *Tabs) jump(target jumpTarget) tea.Cmd {
	if outside := target.outside; outside != nil {
		return t.active().openOutside(outside, target.ref)
	}
	step := backStep{tab: t.at}
	if t.hasPage && t.at == t.page {
		step.page = t.pageShows
	}
	t.back = append(t.back, step)
	return t.showEntity(target.tab, target.ref)
}

// showEntity tells the tab at at the entity, then shows it, with its cursor where the board says.
func (t *Tabs) showEntity(at int, ref EntityRef) tea.Cmd {
	to := t.models[at]
	whole := to.board.(EntityBoard).ShowEntity(ref)
	to.toTop()
	if !whole {
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
// entity page to itself changed what the page shows, so going back opens it on that again.
func (t *Tabs) goBack() tea.Cmd {
	from := t.back[len(t.back)-1]
	t.back = t.back[:len(t.back)-1]
	if from.page != nil && (t.pageShows == nil || *from.page != *t.pageShows) {
		return t.showEntity(from.tab, *from.page)
	}
	return t.show(from.tab)
}

// seek is the entity a jump came for, which the cursor goes to once a line names it, and the
// tab it went to, which the footer names where none does once the reads land.
type seek struct {
	ref EntityRef
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

// seekEntity puts the cursor on the first line naming what a jump came for, opening the groups
// that hold it and clearing a filter that hides it. Where no line names it and no read is under
// way, the footer says so, and the cursor stays where the tab starts. A read that failed is no
// answer, so the footer then says the tab may hold it.
func (m *Model) seekEntity() {
	if m.seeking == nil {
		return
	}
	key, holding, found := entityLine(m.frame.Groups, m.seeking.ref, nil)
	if !found {
		sources := m.board.Sources()
		if slices.ContainsFunc(sources, func(source Refresher) bool { return source.Status().Reading }) {
			return
		}
		ref := m.seeking.ref
		m.said = fmt.Sprintf("%s holds no %s %s", m.seeking.tab, ref.Kind, ref.ID)
		if slices.ContainsFunc(sources, func(source Refresher) bool { return source.Status().Fault != "" }) {
			m.said = fmt.Sprintf("%s could not read, so it may still hold %s %s", m.seeking.tab, ref.Kind, ref.ID)
		}
		m.seeking = nil
		return
	}
	m.seeking = nil
	for _, group := range holding {
		m.display.folded[group] = false
	}
	if !m.MoveCursorTo(key) && m.display.filter != "" {
		m.display.filter = ""
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
