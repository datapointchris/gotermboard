package gotermboard

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermstyle"
)

func TestTheMouseCopiesADragKeptToTheTableOrThePanelAndADoubleClickedWord(t *testing.T) {
	said := []Cell{C("panel words here", "")}
	frame := Frame{Columns: []Column{{Title: ""}}, Groups: []Group{{Key: "g", Title: Line{C("group", "")}, Rows: []Row{
		{Key: "a", Cells: []Cell{C("alpha-1 beta", Green)}, Detail: said},
		{Key: "b", Cells: []Cell{C("gamma delta", "")}, Detail: said},
	}}}}
	tabs, err := NewTabs([]Tab{{Name: "one", Board: fixedBoard{frame}}, {Name: "two", Board: fixedBoard{frame}}}, gotermstyle.NewPalette(true), "")
	if err != nil {
		t.Fatal(err)
	}
	tabs.Update(tea.WindowSizeMsg{Width: 200, Height: 12})
	tabs.active().reveal(time.Now())
	lines := strings.Split(ansi.Strip(tabs.View().Content), "\n")
	find := func(text string) cellAt {
		for y, line := range lines {
			if at := strings.Index(line, text); at >= 0 {
				return cellAt{ansi.StringWidth(line[:at]), y}
			}
		}
		t.Fatalf("%q is not on the screen:\n%s", text, strings.Join(lines, "\n"))
		return cellAt{}
	}
	copied := func(cmd tea.Cmd) string {
		if cmd == nil {
			return ""
		}
		return fmt.Sprint(cmd())
	}
	drag := func(from, to cellAt) string {
		tabs.Update(tea.MouseClickMsg{X: from.x, Y: from.y, Button: tea.MouseLeft})
		tabs.Update(tea.MouseMotionMsg{X: to.x, Y: to.y, Button: tea.MouseLeft})
		if !strings.Contains(tabs.View().Content, "\x1b[7m") {
			t.Errorf("a drag should show what it covers in reverse video while the button is held")
		}
		return copied(tabs.update(tea.MouseReleaseMsg{X: to.x, Y: to.y}))
	}

	beta, gamma := find("beta"), find("gamma")
	if got := drag(beta, cellAt{gamma.x + 4, gamma.y}); !strings.HasPrefix(got, "beta\n") || !strings.HasSuffix(got, "gamma") || strings.ContainsAny(got, "│") {
		t.Errorf("a drag in the table should copy from its first cell to its last and none of the panel beside it: %q", got)
	}
	if strings.Contains(tabs.View().Content, "\x1b[7m") {
		t.Errorf("the release should take the highlight away, as tmux's copy does")
	}
	if panel := find("panel"); drag(find("here"), cellAt{0, panel.y}) != "panel words h" {
		t.Errorf("a drag that starts in the panel should keep to its columns as it wanders over the table")
	}

	alpha := find("alpha")
	alpha.x += 2
	tabs.Update(tea.MouseClickMsg{X: alpha.x, Y: alpha.y, Button: tea.MouseLeft})
	tabs.Update(tea.MouseReleaseMsg{X: alpha.x, Y: alpha.y})
	if got := copied(tabs.update(tea.MouseClickMsg{X: alpha.x, Y: alpha.y, Button: tea.MouseLeft})); got != "alpha-1" {
		t.Errorf("a double click should copy the word under it, a hyphen and all: %q", got)
	}
	if !strings.Contains(ansi.Strip(tabs.View().Content), "copied alpha-1") {
		t.Errorf("the footer should say what was copied")
	}
}
