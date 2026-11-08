package gotermboard

import (
	"fmt"
	"slices"
	"strconv"

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
	// Command is the program opened on one entity's id, which Bubble Tea's Exec runs.
	Command func(id string) tea.ExecCommand
}

// JumpKey goes to what the cursor's row names, where another tab or a program outside shows it.
const JumpKey = "t"

// OpensOutside adds programs that a row's entity can jump to, after the tabs that show its kind.
func (t *Tabs) OpensOutside(programs ...Outside) {
	t.outside = append(t.outside, programs...)
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
// own target, because the row is already there.
func (t *Tabs) jumpTargets(m *Model) []jumpTarget {
	var targets []jumpTarget
	for _, ref := range m.CursorEntities() {
		for at, model := range t.models {
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

// jumpActions is t, which goes straight to the one place the cursor's row names, or asks which
// where it names several.
func (t *Tabs) jumpActions() []Action {
	return []Action{{
		Keys: []string{JumpKey}, Label: "go to what the row names, in another tab or program; esc comes back", Footer: "t go to",
		Run:     t.pickJump,
		Applies: func(m *Model) bool { return len(t.jumpTargets(m)) > 0 },
	}}
}

// backActions is esc while a jump brought the reader here. It sits ahead of the board's own keys,
// so the tab a jump opened is that tab's top, and esc there goes back rather than deeper into it.
func (t *Tabs) backActions() []Action {
	return []Action{{
		Keys: []string{"esc"}, Label: "go back to the tab the jump left, on the row it left", Footer: "esc back",
		Run:     func(*Model) tea.Cmd { return t.goBack() },
		Applies: func(m *Model) bool { return len(t.back) > 0 && m.display.filter == "" },
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

// JumpTo goes to ref in the tab or the program outside named where, as JumpKey does, for a
// board's own key. Esc at that tab's top comes back. It says on the footer where nothing named
// where shows ref's kind.
func (m *Model) JumpTo(where string, ref EntityRef) tea.Cmd {
	if m.tabs != nil {
		for _, target := range m.tabs.jumpTargets(m) {
			if target.ref.Kind == ref.Kind && target.where(m.tabs) == where {
				target.ref = ref
				return m.tabs.jump(target)
			}
		}
	}
	m.Say(fmt.Sprintf("no tab or program named %s shows a %s here", where, ref.Kind))
	return nil
}

// jump goes to target. A program outside is handed the terminal. A tab is shown at the entity,
// with the tab it left pushed for esc, and its cursor goes where the board says.
func (t *Tabs) jump(target jumpTarget) tea.Cmd {
	from := t.active()
	if outside := target.outside; outside != nil {
		return tea.Exec(outside.Command(target.ref.ID), func(err error) tea.Msg {
			return outsideEnded{model: from, name: outside.Name, err: err}
		})
	}
	to := t.models[target.tab]
	whole := to.board.(EntityBoard).ShowEntity(target.ref)
	t.back = append(t.back, t.at)
	from.hide()
	t.at = target.tab
	if !whole {
		to.seeking = &seek{ref: target.ref, none: fmt.Sprintf("%s holds no %s %s", t.names[target.tab], target.ref.Kind, target.ref.ID)}
	}
	return to.Restart()
}

// goBack shows the tab the newest jump left, whose cursor stayed where it was.
func (t *Tabs) goBack() tea.Cmd {
	from := t.back[len(t.back)-1]
	t.back = t.back[:len(t.back)-1]
	return t.show(from)
}

// outsideEnded is what a program a jump handed the terminal to sends once it exits, for the model
// whose row it left.
type outsideEnded struct {
	model *Model
	name  string
	err   error
}

// seek is the entity a jump came for, which the cursor goes to once a line names it, and what the
// footer says where none does once the reads land.
type seek struct {
	ref  EntityRef
	none string
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
// way, the footer says so and the cursor stays where the tab starts.
func (m *Model) seekEntity() {
	if m.seeking == nil {
		return
	}
	key, holding, found := entityLine(m.frame.Groups, m.seeking.ref, nil)
	if !found {
		if !slices.ContainsFunc(m.board.Sources(), func(source Refresher) bool { return source.Status().Reading }) {
			m.said, m.seeking = m.seeking.none, nil
		}
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
