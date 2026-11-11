package gotermboard

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// KeyAction is one row of a key table, for a screen of type H. The key handler, the footer and
// the ? overlay all read the same rows, so a key bound in one shows in the other two. A Model's
// rows are Action, and a Reader's are KeyAction[*Reader]. A host that draws its own screen builds
// its table from UniversalActions and rows of its own, reads what applies now through
// ActionsNow, and reads those rows through RunKey, FooterHints and BySection.
type KeyAction[H any] struct {
	Keys []string
	// Section heads the row's group in a ? overlay that groups its rows. A Model's overlay lists
	// its rows in one run.
	Section string
	Label   string
	// Footer is the footer's words for the key, or empty to leave it to the ? overlay.
	Footer string
	Run    func(H) tea.Cmd
	// Applies says whether the screen can use the key now. Nil means always.
	Applies func(H) bool
}

// Action is one row of a Model's key table. A board's key hides the table's action for it even
// while Applies is false.
type Action = KeyAction[*Model]

// Host is a screen of rows that UniversalActions act on: a host that draws its own screen, or a
// Model. It gains no method, because each host's build would break on the release that added
// one. A universal key needing more of a host asks for it through an interface of its own, and
// binds only on a host that implements it.
type Host interface {
	// MoveBy moves the cursor by rows, stopping on the first and the last. g and G move by 1<<30
	// rows, so the cursor plus rows stays inside an int on a 32-bit build.
	MoveBy(rows int) tea.Cmd
	// HalfScreen is half the list's height in lines.
	HalfScreen() int
	// ScrollBy moves the text open under the cursor's row by lines, and leaves the cursor on it.
	ScrollBy(lines int)
	// OnOpenText says the cursor's row shows text open under it, which the half-screen keys and
	// J and K then read through.
	OnOpenText() bool
	// JumpGroup goes to the next group at the cursor's depth or shallower, or the previous where
	// direction is negative, as Vim's ]] and [[ move by section.
	JumpGroup(direction int)
	// HasGroups says the screen has groups for [ and ] to jump between.
	HasGroups() bool
	// Toggles says enter and space would change something on the cursor's line.
	Toggles() bool
	// Open is enter: it opens the row's text full screen where it runs past the screen, or a
	// side panel already shows it, and otherwise does what ToggleInPlace does.
	Open() tea.Cmd
	// ToggleInPlace is space: it folds or opens the cursor's group, or opens or closes its row's
	// text in place however tall.
	ToggleInPlace() tea.Cmd
	// ShowKeys opens the ? overlay.
	ShowKeys()
	// Quit ends the program.
	Quit() tea.Cmd
}

// Sections UniversalActions files its rows under.
const (
	SectionMoving     = "Moving"
	SectionOpening    = "Opening"
	SectionEverywhere = "Everywhere"
)

// pastEitherEnd is a move longer than any list, which MoveBy stops at its first or last row.
const pastEitherEnd = 1 << 30

// UniversalActions is the keys every board binds, and a host's table starts from, acting on a
// Host: moving by rows, to either end and by half a screen, scrolling the open text, jumping
// between groups, opening a row, the ? overlay and quitting. Each binds where its Applies holds.
// esc stays the host's, because what it backs out of is the host's. They are in the order a
// footer names them, with the way to the other keys and the way out ahead of what a narrow
// footer cuts.
func UniversalActions[H Host]() []KeyAction[H] {
	return universalActions[H]("fold a group, or open a row, full screen where its document is tall", "show these keys")
}

// universalActions is UniversalActions with enter's label and ?'s, which a Model says more of.
func universalActions[H Host](openLabel, keysLabel string) []KeyAction[H] {
	moved := func(rows int) func(H) tea.Cmd {
		return func(h H) tea.Cmd { return h.MoveBy(rows) }
	}
	halfScreen := func(direction int) func(H) tea.Cmd {
		return func(h H) tea.Cmd {
			if h.OnOpenText() {
				h.ScrollBy(direction * h.HalfScreen())
				return nil
			}
			return h.MoveBy(direction * h.HalfScreen())
		}
	}
	scrolled := func(lines int) func(H) tea.Cmd {
		return func(h H) tea.Cmd {
			h.ScrollBy(lines)
			return nil
		}
	}
	jumped := func(direction int) func(H) tea.Cmd {
		return func(h H) tea.Cmd {
			h.JumpGroup(direction)
			return nil
		}
	}
	hasGroups := func(h H) bool { return h.HasGroups() }
	toggles := func(h H) bool { return h.Toggles() }
	onOpenText := func(h H) bool { return h.OnOpenText() }
	return []KeyAction[H]{
		{Keys: []string{"j", "down"}, Section: SectionMoving, Label: "move down", Footer: "j/k move", Run: moved(1)},
		{Keys: []string{"k", "up"}, Section: SectionMoving, Label: "move up", Run: moved(-1)},
		{Keys: []string{"]"}, Section: SectionMoving, Label: "jump to the next group", Footer: "[/] jump", Run: jumped(1), Applies: hasGroups},
		{Keys: []string{"["}, Section: SectionMoving, Label: "jump to the previous group", Run: jumped(-1), Applies: hasGroups},
		{Keys: []string{"g", "home"}, Section: SectionMoving, Label: "go to the top", Run: moved(-pastEitherEnd)},
		{Keys: []string{"G", "end"}, Section: SectionMoving, Label: "go to the bottom", Run: moved(pastEitherEnd)},
		{Keys: []string{"ctrl+d", "pgdown"}, Section: SectionMoving, Label: "half a screen down the open text, or down the rows", Run: halfScreen(1)},
		{Keys: []string{"ctrl+u", "pgup"}, Section: SectionMoving, Label: "half a screen up the open text, or up the rows", Run: halfScreen(-1)},
		{
			Keys: []string{"enter"}, Section: SectionOpening, Label: openLabel,
			Footer: "enter open", Run: func(h H) tea.Cmd { return h.Open() }, Applies: toggles,
		},
		{
			Keys: []string{"space"}, Section: SectionOpening, Label: "fold a group, or open or close a row's text in place",
			Footer: "space fold", Run: func(h H) tea.Cmd { return h.ToggleInPlace() }, Applies: toggles,
		},
		{Keys: []string{"?"}, Section: SectionEverywhere, Label: keysLabel, Footer: "? keys", Run: func(h H) tea.Cmd {
			h.ShowKeys()
			return nil
		}},
		{Keys: []string{"q", "ctrl+c"}, Section: SectionEverywhere, Label: "quit", Footer: "q quit", Run: func(h H) tea.Cmd { return h.Quit() }},
		{Keys: []string{"J"}, Section: SectionMoving, Label: "scroll the open text down a line", Footer: "J/K scroll", Run: scrolled(1), Applies: onOpenText},
		{Keys: []string{"K"}, Section: SectionMoving, Label: "scroll the open text up a line", Run: scrolled(-1), Applies: onOpenText},
	}
}

// ActionsNow is every key host can use now from layers of rows: each layer's rows that apply,
// less the keys a layer before it took. A host's key handler, footer and ? overlay all read it,
// so a key the screen cannot use is in none of them. A host's own rows go in a layer ahead of
// UniversalActions, so its key wins where it applies, and the universal row acts everywhere else.
// The rows are in the footer's order, which splits a section where layers share it, so an
// overlay that heads each section reads BySection(ActionsNow(...)).
func ActionsNow[H any](host H, layers ...[]KeyAction[H]) []KeyAction[H] {
	applying := make([][]KeyAction[H], 0, len(layers))
	for _, layer := range layers {
		applying = append(applying, applyingTo(host, layer))
	}
	return keysTakenInOrder(applying...)
}

// RunKey runs the first of actions bound to key on host, and says whether one was.
func RunKey[H any](host H, actions []KeyAction[H], key string) (tea.Cmd, bool) {
	for _, action := range actions {
		if slices.Contains(action.Keys, key) {
			return action.Run(host), true
		}
	}
	return nil, false
}

// FooterHints is the footer's words for actions in their order, leaving out each row that
// leaves its key to the ? overlay.
func FooterHints[H any](actions []KeyAction[H]) []string {
	var hints []string
	for _, action := range actions {
		if action.Footer != "" {
			hints = append(hints, action.Footer)
		}
	}
	return hints
}

// KeySection is one section of a ? overlay: its heading and its rows.
type KeySection[H any] struct {
	Section string
	Actions []KeyAction[H]
}

// BySection is actions under their sections, each section where its first row stands and its
// rows in their order.
func BySection[H any](actions []KeyAction[H]) []KeySection[H] {
	var sections []KeySection[H]
	at := map[string]int{}
	for _, action := range actions {
		i, seen := at[action.Section]
		if !seen {
			i = len(sections)
			at[action.Section] = i
			sections = append(sections, KeySection[H]{Section: action.Section})
		}
		sections[i].Actions = append(sections[i].Actions, action)
	}
	return sections
}

// keyLines is an overlay's lines for one row: its keys in a column, then its label, wrapped to
// width under its own column so a long label reads whole rather than running off the screen.
func keyLines[H any](action KeyAction[H], width int) []string {
	keys := "  " + fit(strings.Join(action.Keys, " "), 16, false) + "  "
	under := strings.Repeat(" ", len(keys))
	label := wrap(action.Label, max(20, width-len(keys)))
	lines := []string{keys + label[0]}
	for _, more := range label[1:] {
		lines = append(lines, under+more)
	}
	return lines
}

// applyingTo is the rows of actions that apply to host now.
func applyingTo[H any](host H, actions []KeyAction[H]) []KeyAction[H] {
	return slices.DeleteFunc(slices.Clone(actions), func(action KeyAction[H]) bool {
		return action.Applies != nil && !action.Applies(host)
	})
}

// keysTakenInOrder is each layer's actions less the keys a layer before it took. An action losing
// every key goes, and one losing its first key also loses its place in the footer, where the
// earlier layer's stands.
func keysTakenInOrder[H any](layers ...[]KeyAction[H]) []KeyAction[H] {
	taken := map[string]bool{}
	var all []KeyAction[H]
	for _, layer := range layers {
		var took []string
		for _, action := range layer {
			left := slices.DeleteFunc(slices.Clone(action.Keys), func(key string) bool { return taken[key] })
			if len(left) == 0 {
				continue
			}
			if taken[action.Keys[0]] {
				action.Footer = ""
			}
			action.Keys = left
			all = append(all, action)
			took = append(took, left...)
		}
		for _, key := range took {
			taken[key] = true
		}
	}
	return all
}

// modelHost is a Model as UniversalActions act on it.
type modelHost struct{ m *Model }

func (h modelHost) MoveBy(rows int) tea.Cmd {
	h.m.move(rows)
	return nil
}

func (h modelHost) HalfScreen() int         { return h.m.HalfScreen() }
func (h modelHost) ScrollBy(lines int)      { h.m.ScrollBy(lines) }
func (h modelHost) OnOpenText() bool        { return h.m.onOpenText() }
func (h modelHost) JumpGroup(direction int) { h.m.jump(direction) }
func (h modelHost) HasGroups() bool         { return h.m.hasGroups() }
func (h modelHost) Toggles() bool           { return h.m.cursorToggles() }
func (h modelHost) Open() tea.Cmd           { return h.m.toggle() }
func (h modelHost) ToggleInPlace() tea.Cmd  { return h.m.toggleInPlace() }
func (h modelHost) ShowKeys()               { h.m.overlay = true }

func (h modelHost) Quit() tea.Cmd {
	h.m.cancel()
	return tea.Quit
}

// modelActions is the universal rows as a Model's, with what only a Model's say: its side panel
// can already show a row's document, and its overlay lists every read.
func modelActions() []Action {
	universal := universalActions[modelHost](
		"fold a group, or open a row, full screen where its document is tall or the panel draws it",
		"show these keys and every read",
	)
	actions := make([]Action, 0, len(universal))
	for _, row := range universal {
		action := Action{Keys: row.Keys, Section: row.Section, Label: row.Label, Footer: row.Footer}
		action.Run = func(m *Model) tea.Cmd { return row.Run(modelHost{m}) }
		if row.Applies != nil {
			action.Applies = func(m *Model) bool { return row.Applies(modelHost{m}) }
		}
		actions = append(actions, action)
	}
	return actions
}
