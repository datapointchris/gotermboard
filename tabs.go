package gotermboard

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermstyle"
)

// Tab is one named view of a program.
type Tab struct {
	Name  string
	Board Board
}

// Tabs is the Bubble Tea program of one Model per tab, the Elm Architecture's
// composition. Each tab keeps its own cursor, folds and filter. A hidden tab starts no
// read, builds no frame and moves no strip. On its return it draws its last good read with that read's
// age, and whatever is due reads at once.
type Tabs struct {
	names  []string
	models []*Model
	pal    gotermstyle.Palette
	at     int
}

// NewTabs is a program of these tabs, opening on the one named open, or the first where
// open is empty. A name no tab has is an error naming the ones there are.
func NewTabs(tabs []Tab, pal gotermstyle.Palette, open string) (*Tabs, error) {
	if len(tabs) == 0 {
		return nil, errors.New("a board needs at least one tab")
	}
	t := &Tabs{pal: pal}
	for _, tab := range tabs {
		t.names = append(t.names, tab.Name)
	}
	if open != "" {
		t.at = slices.Index(t.names, open)
		if t.at < 0 {
			return nil, fmt.Errorf("no tab %q: the tabs are %s", open, strings.Join(t.names, ", "))
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.models = hiddenModels(tabs, pal, ctx, cancel)
	for _, model := range t.models {
		model.hosted = t.cycleActions()
	}
	return t, nil
}

// hiddenModels share one context, so quitting from any tab ends every tab's reads.
func hiddenModels(tabs []Tab, pal gotermstyle.Palette, ctx context.Context, cancel context.CancelFunc) []*Model {
	models := make([]*Model, 0, len(tabs))
	for _, tab := range tabs {
		model := newModel(tab.Board, pal, ctx, cancel)
		model.hidden = true
		models = append(models, model)
	}
	return models
}

func (t *Tabs) active() *Model { return t.models[t.at] }

// cycleActions are tab's keys, which join each view's one action table. A single tab has
// nothing to cycle, so it has none.
func (t *Tabs) cycleActions() []Action {
	if len(t.models) < 2 {
		return nil
	}
	return []Action{
		{Keys: []string{"tab"}, Label: "go to the next tab", Footer: "tab next view", Run: func(*Model) tea.Cmd { return t.show(t.at + 1) }},
		{Keys: []string{"shift+tab"}, Label: "go to the previous tab", Run: func(*Model) tea.Cmd { return t.show(t.at - 1) }},
	}
}

func (t *Tabs) show(at int) tea.Cmd {
	t.active().hide()
	t.at = (at + len(t.models)) % len(t.models)
	return t.active().reveal(time.Now())
}

// tabLineHeight is what the line naming the tabs takes from each view's screen.
func (t *Tabs) tabLineHeight() int {
	if len(t.models) < 2 {
		return 0
	}
	return 1
}

// Init is Bubble Tea's. One clock runs, and it goes to whichever tab is showing.
func (t *Tabs) Init() tea.Cmd { return tea.Batch(tick(), t.active().reveal(time.Now())) }

// Update is Bubble Tea's. A size goes to every tab, and the rest to the one showing.
func (t *Tabs) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		size.Height -= t.tabLineHeight()
		for _, model := range t.models {
			model.Update(size)
		}
		return t, nil
	}
	_, cmd := t.active().Update(msg)
	return t, cmd
}

// View is Bubble Tea's.
func (t *Tabs) View() tea.View {
	screen := t.active().screen()
	if t.tabLineHeight() > 0 {
		screen = t.tabLine() + "\n" + screen
	}
	v := tea.NewView(screen)
	v.AltScreen = true
	return v
}

// The tab line draws as a tmux status line draws its windows: the current tab bold in an
// accent and the rest in the theme's dim foreground, which bright black carries.
const (
	currentTabCode = BoldYellow
	otherTabCode   = "\033[0;90m"
)

// tabLine names the tabs, each padded by a space and two spaces apart. Where there is no
// color, brackets take the current tab's padding, so no name moves.
func (t *Tabs) tabLine() string {
	parts := make([]string, 0, len(t.names))
	for i, name := range t.names {
		switch {
		case i != t.at:
			parts = append(parts, t.pal.Paint(otherTabCode, " "+name+" "))
		case t.pal.On():
			parts = append(parts, t.pal.Paint(currentTabCode, " "+name+" "))
		default:
			parts = append(parts, "["+name+"]")
		}
	}
	return ansi.Truncate(strings.Join(parts, "  "), t.active().width, "…")
}
