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
// read and moves no strip, and builds a frame only where its board's Receive rebuilds it. On
// its return it draws its last good read with that read's age, and whatever is due reads at
// once.
type Tabs struct {
	names   []string
	models  []*Model
	painter painter
	at      int
	// reopens is the tab a run started again opens on, which ReopensOn names.
	reopens int
	// back is each step a jump left, newest last, which esc returns to. A cycle empties it,
	// because the reader then left the trail the jumps made.
	back    []backStep
	outside []Outside
	// page is the tab EntityPage names, and pageShows what it was last opened on.
	page      int
	hasPage   bool
	pageShows *EntityRef
	// lock is what LockKey locked every tab to, which each tab's display shares.
	lock *tabLock
	// covers is each kind's LockCover, which CoverLocks registers.
	covers map[string]LockCover
	// ctx ends with the program, and bounds what a LockCover reads.
	ctx context.Context
	// openNamed says NewTabs was told which tab to open on, which a remembered tab gives way to.
	openNamed bool
	// memory is what RemembersDisplayIn keeps, and nil without it.
	memory *rememberedDisplays
	// selecting is the mouse's selection over the whole screen, tab line and all.
	selecting selecting
}

// backStep is the tab a jump left, the line its cursor was on, and what the entity page showed
// where it left the page, so esc puts both back.
type backStep struct {
	tab    int
	cursor string
	page   *EntityRef
}

// NewTabs is a program of these tabs, opening on the one named open, or the first where
// open is empty. A name no tab has is an error naming the ones there are. Every tab draws in
// the one TUI theme file gotermstyle's tuitheme finds, read again on SIGUSR2.
func NewTabs(tabs []Tab, pal gotermstyle.Palette, open string) (*Tabs, error) {
	if len(tabs) == 0 {
		return nil, errors.New("a board needs at least one tab")
	}
	theme, themeErr := loadTheme()
	t := &Tabs{painter: painter{pal: pal, theme: theme}}
	for _, tab := range tabs {
		t.names = append(t.names, tab.Name)
	}
	if open != "" {
		t.openNamed = true
		t.at = slices.Index(t.names, open)
		if t.at < 0 {
			return nil, fmt.Errorf("no tab %q: the tabs are %s", open, strings.Join(t.names, ", "))
		}
	}
	t.reopens = t.at
	ctx, cancel := context.WithCancel(context.Background())
	t.ctx = ctx
	t.models = hiddenModels(tabs, t.painter, ctx, cancel)
	t.lock = &tabLock{}
	for _, model := range t.models {
		model.display.lock = t.lock
	}
	t.hostActions()
	t.active().sayThemeFault(themeErr)
	return t, nil
}

// hostActions gives each tab's model the keys Tabs binds: esc back ahead of the board's own keys,
// and the rest beside the table's.
func (t *Tabs) hostActions() {
	for _, model := range t.models {
		model.ahead, model.hosted = t.backActions(), t.besideTable()
	}
}

// besideTable is the keys Tabs binds beside the table's, in the order they take a key.
func (t *Tabs) besideTable() []Action {
	return slices.Concat(t.cycleActions(), t.jumpActions(), t.pageActions(), t.lockActions())
}

// Keys is every key Tabs binds beside the table's, each where it has something to do, which a
// board's own key would hide. A board taking keys from its own config asks this and TableKeys
// which ones it would take.
func (t *Tabs) Keys() []string {
	var keys []string
	for _, action := range t.besideTable() {
		keys = append(keys, action.Keys...)
	}
	return keys
}

// hiddenModels share one context, so quitting from any tab ends every tab's reads, and one
// painter's theme, so a reload recolors every tab.
func hiddenModels(tabs []Tab, p painter, ctx context.Context, cancel context.CancelFunc) []*Model {
	models := make([]*Model, 0, len(tabs))
	for _, tab := range tabs {
		model := newModel(tab.Board, p, ctx, cancel)
		model.hidden = true
		models = append(models, model)
	}
	return models
}

func (t *Tabs) active() *Model { return t.models[t.at] }

// ReopensOn names the tab a run of this program started again should open on: the one showing,
// or the one the reader left for the entity page, which opened with nothing on it would show
// nothing. It is the tab RemembersDisplayIn keeps, for this program alone, where several share
// the file.
func (t *Tabs) ReopensOn() string { return t.names[t.reopens] }

// cycleActions are tab's keys, which join each view's one action table. A single tab has
// nothing to cycle, so it has none.
func (t *Tabs) cycleActions() []Action {
	if len(t.models) < 2 {
		return nil
	}
	cycle := func(by int) func(*Model) tea.Cmd {
		return func(*Model) tea.Cmd {
			t.back = nil
			return t.show(t.nextListed(by))
		}
	}
	return []Action{
		{Keys: []string{"tab"}, Label: "go to the next tab", Footer: "tab next view", Run: cycle(1)},
		{Keys: []string{"shift+tab"}, Label: "go to the previous tab", Run: cycle(-1)},
	}
}

// show hides the tab showing and shows the one at at, from its last good reads. A search set for
// it survives the hide, and under a lock one with none goes to the first line naming the thing
// where the line its cursor is on does not show. Every way onto a tab comes through here.
func (t *Tabs) show(at int) tea.Cmd {
	at = (at + len(t.models)) % len(t.models)
	seeking := t.models[at].seeking
	t.active().hide()
	t.at = at
	t.active().seeking = seeking
	t.seekLocked(at)
	// The entity page shows what a row named, so a run opening on it would show nothing.
	if !t.hasPage || at != t.page {
		t.reopens = at
		t.memory.showing(t.names[at])
	}
	return tea.Batch(t.active().reveal(time.Now()), t.askCover())
}

// tabLineHeight is what the line naming the tabs takes from each view's screen.
func (t *Tabs) tabLineHeight() int {
	if len(t.models) < 2 {
		return 0
	}
	return 1
}

// Init is Bubble Tea's. One clock runs, and it goes to whichever tab is showing.
func (t *Tabs) Init() tea.Cmd {
	return tea.Batch(tick(), t.active().reveal(time.Now()), t.painter.theme.watch())
}

// Update is Bubble Tea's. A size goes to every tab, and so does any message this package does
// not own, which is a ReceivingBoard's. A key, a paste and the clocks go to the tab showing.
// A message that changed what RemembersDisplayIn keeps writes it.
func (t *Tabs) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := t.update(msg)
	return t, tea.Batch(cmd, t.memory.written(t.active()))
}

func (t *Tabs) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		msg.Height -= t.tabLineHeight()
		for _, model := range t.models {
			model.Update(msg)
		}
		return nil
	case tickMsg, refreshedMsg, moveMsg, spinMsg, tea.KeyPressMsg, tea.PasteMsg, tea.MouseWheelMsg:
		_, cmd := t.active().Update(msg)
		return cmd
	case tea.MouseClickMsg, tea.MouseMotionMsg, tea.MouseReleaseMsg:
		model := t.active()
		p := t.selecting.mouse(msg.(tea.MouseMsg), time.Now(), func(x, y int) columns {
			return model.regionOnScreen(x, y, t.tabLineHeight())
		})
		if p.clicked && t.tabLineHeight() > 0 && p.at.y == model.margin.Ends {
			return t.clickedTab(p.at.x - model.margin.Sides)
		}
		cmd := model.pointedAt(p, t.tabLineHeight())
		model.tellCursor()
		return cmd
	case themeLoadedMsg:
		return t.painter.theme.reloaded(msg, t.active().sayThemeFault)
	case goToMsg:
		return t.goTo(msg)
	case lockCoveredMsg:
		return t.lockCovered(msg)
	}
	cmds := make([]tea.Cmd, 0, len(t.models))
	for _, model := range t.models {
		_, cmd := model.Update(msg)
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
}

// View is Bubble Tea's. The tab line sits inside the showing tab's margin.
func (t *Tabs) View() tea.View {
	inside := t.active().inside()
	if t.tabLineHeight() > 0 {
		inside = t.tabLine() + "\n" + inside
	}
	v := tea.NewView(t.selecting.paint(t.active().framed(inside)))
	v.AltScreen = true
	v.MouseMode = mouseMode
	return v
}

// tabLine names the tabs, each padded by a space and two spaces apart. It marks the current
// tab by color and weight rather than a box, as a tmux status line marks its window: bold in
// ui.warning, and the rest in ui.muted, so it reads like a tmux bar drawn from the same theme.
// Where there is no color, brackets take the current tab's padding, so no name moves. Under a
// lock it leaves out each tab holding nothing locked, and names the lock after the tabs.
func (t *Tabs) tabLine() string {
	parts := make([]string, 0, len(t.names)+1)
	for _, i := range t.listed(time.Now()) {
		name := t.names[i]
		switch {
		case i != t.at:
			parts = append(parts, t.painter.cell(Cell{Text: " " + name + " ", Role: "ui.muted"}))
		case t.painter.on():
			parts = append(parts, t.painter.cell(Cell{Text: " " + name + " ", Code: Bold, Role: "ui.warning"}))
		default:
			parts = append(parts, "["+name+"]")
		}
	}
	if ref := t.lock.ref; ref != nil {
		parts = append(parts, t.painter.cell(C(LockGlyph+" "+ref.Kind+" "+ref.ID, LockCode)))
	}
	return ansi.Truncate(strings.Join(parts, "  "), t.active().width, "…")
}

// clickedTab shows the tab whose name the tab line draws at x, measured as tabLine lays them out.
func (t *Tabs) clickedTab(x int) tea.Cmd {
	left := 0
	for _, i := range t.listed(time.Now()) {
		right := left + ansi.StringWidth(t.names[i]) + 2
		if x >= left && x < right {
			if i == t.at {
				return nil
			}
			t.back = nil
			return t.show(i)
		}
		left = right + 2
	}
	return nil
}
