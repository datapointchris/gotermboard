package gotermboard

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/datapointchris/gotermstyle"
)

// thingsBoard draws a row naming each thing it holds, shows the kind thing, and binds esc itself.
type thingsBoard struct {
	ids     []string
	escaped bool
}

func (b *thingsBoard) Sources() []Refresher { return nil }
func (b *thingsBoard) Groupings() []string  { return nil }
func (b *thingsBoard) Frame(time.Time, string) Frame {
	group := Group{Key: "things", Title: Line{C("THINGS", Bold)}}
	for _, id := range b.ids {
		group.Rows = append(group.Rows, Row{Key: "row-" + id, Cells: []Cell{C("row "+id, "")}, Entities: []EntityRef{{Kind: "thing", ID: id}}})
	}
	return Frame{Columns: []Column{{Title: "NAME"}}, Groups: []Group{group}}
}
func (b *thingsBoard) EntityKinds() []string             { return []string{"thing"} }
func (b *thingsBoard) ShowEntity(EntityRef) (whole bool) { return false }
func (b *thingsBoard) Actions() []Action {
	return []Action{{Keys: []string{"esc"}, Label: "back inside the board", Run: func(*Model) tea.Cmd {
		b.escaped = true
		return nil
	}}}
}

func TestAJumpLandsOnTheRowNamingItsEntityAndEscReturnsToTheRowItLeft(t *testing.T) {
	one, two := &thingsBoard{ids: []string{"1", "2", "3"}}, &thingsBoard{ids: []string{"3", "2"}}
	tabs, err := NewTabs([]Tab{{Name: "one", Board: one}, {Name: "two", Board: two}}, gotermstyle.NewPalette(false), "")
	if err != nil {
		t.Fatal(err)
	}
	tabs.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	tabs.active().reveal(time.Now())
	press(tabs, downKey)

	press(tabs, typedKey(JumpKey))
	if got := tabs.names[tabs.at] + " " + tabs.active().Cursor(); got != "two row-2" {
		t.Fatalf("the jump should land on two's row naming thing 2, not %q", got)
	}
	if got := tabs.View().Content; !strings.Contains(got, "esc back") {
		t.Errorf("the footer should offer the way back:\n%s", got)
	}

	press(tabs, tea.KeyPressMsg{Code: tea.KeyEscape})
	if got := tabs.names[tabs.at] + " " + tabs.active().Cursor(); got != "one row-2" || two.escaped {
		t.Fatalf("esc should go back to one's row ahead of two's own esc, not %q (two's esc ran: %v)", got, two.escaped)
	}
	press(tabs, tea.KeyPressMsg{Code: tea.KeyEscape})
	if !one.escaped || tabs.at != 0 {
		t.Error("with no jump to go back along, esc should be the board's own")
	}
}
