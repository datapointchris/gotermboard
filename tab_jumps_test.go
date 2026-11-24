package gotermboard

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/datapointchris/gotermstyle"
)

// thingsBoard draws a row naming each thing it holds, shows the kind thing, binds esc itself,
// and counts each time its tab hides.
type thingsBoard struct {
	ids     []string
	sources []Refresher
	escaped bool
	hidden  int
}

func (b *thingsBoard) Hidden()              { b.hidden++ }
func (b *thingsBoard) Sources() []Refresher { return b.sources }
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

// openingBoard is a thingsBoard whose o shows the cursor's thing in the tab named to.
type openingBoard struct {
	thingsBoard
	to string
}

func (b *openingBoard) Actions() []Action {
	return []Action{{Keys: []string{"o"}, Label: "open the thing", Run: func(m *Model) tea.Cmd {
		return GoTo(b.to, m.CursorEntities()[0])
	}}}
}

// narrowingBoard is an openingBoard that shows thing 1 alone until a jump asks for any thing, and
// narrows back once told the reader left its tab.
type narrowingBoard struct{ openingBoard }

func (b *narrowingBoard) ShowEntity(EntityRef) (whole bool) {
	b.ids = []string{"1", "2", "3"}
	return false
}

func (b *narrowingBoard) Hidden() {
	b.hidden++
	b.ids = []string{"1"}
}

func TestEscAlongChainedJumpsFindsTheRowOnABoardThatNarrowsOnceTheReaderLeaves(t *testing.T) {
	src := &openingBoard{thingsBoard: thingsBoard{ids: []string{"1", "2"}}, to: "inbox"}
	inbox := &narrowingBoard{openingBoard{thingsBoard: thingsBoard{ids: []string{"1"}}, to: "far"}}
	far := &thingsBoard{ids: []string{"2"}}
	tabs, err := NewTabs([]Tab{{Name: "src", Board: src}, {Name: "inbox", Board: inbox}, {Name: "far", Board: far}}, gotermstyle.NewPalette(false), "")
	if err != nil {
		t.Fatal(err)
	}
	tabs.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	tabs.active().reveal(time.Now())
	press(tabs, downKey)
	where := func() string { return tabs.names[tabs.at] + " " + tabs.active().Cursor() }

	tabs.Update(press(tabs, typedKey("o"))())
	tabs.Update(press(tabs, typedKey("o"))())
	press(tabs, tea.KeyPressMsg{Code: tea.KeyEscape})
	if got := where(); got != "inbox row-2" || inbox.hidden != 0 {
		t.Fatalf("esc should find the row the second jump left, on a board not yet told it hid, not %q (hidden %d)", got, inbox.hidden)
	}

	tabs.Update(press(tabs, typedKey("o"))())
	press(tabs, tabKey)
	if where() != "src row-2" || inbox.hidden != 1 || len(inbox.ids) != 1 {
		t.Errorf("tab should drop the way back and tell inbox once that the reader left, not %d times", inbox.hidden)
	}
}

func TestABoardsKeyGoesToTheNamedTabOnItsThingAndEscComesBack(t *testing.T) {
	one, two := &openingBoard{thingsBoard: thingsBoard{ids: []string{"1", "2"}}, to: "two"}, &thingsBoard{ids: []string{"3", "2"}}
	tabs, err := NewTabs([]Tab{{Name: "one", Board: one}, {Name: "two", Board: two}}, gotermstyle.NewPalette(false), "")
	if err != nil {
		t.Fatal(err)
	}
	tabs.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	tabs.active().reveal(time.Now())
	press(tabs, downKey)

	tabs.Update(press(tabs, typedKey("o"))())
	if got := tabs.names[tabs.at] + " " + tabs.active().Cursor(); got != "two row-2" {
		t.Fatalf("o should land on two's row naming thing 2, not %q", got)
	}
	press(tabs, tea.KeyPressMsg{Code: tea.KeyEscape})
	if got := tabs.names[tabs.at] + " " + tabs.active().Cursor(); got != "one row-2" {
		t.Fatalf("esc should go back to the row o left, not %q", got)
	}

	one.to = "three"
	tabs.Update(press(tabs, typedKey("o"))())
	if got := tabs.View().Content; tabs.at != 0 || !strings.Contains(got, `no tab "three"`) {
		t.Errorf("a tab that is not there should leave the cursor where it is and say so:\n%s", got)
	}

	one.to = "one"
	tabs.Update(press(tabs, typedKey("o"))())
	if tabs.at != 0 || one.hidden != 0 {
		t.Errorf("a go-to inside the tab showing should keep it shown, not hide it (hidden %d times)", one.hidden)
	}
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
	if one.hidden != 0 || two.hidden != 1 {
		t.Errorf("esc back should tell two once and one, which the jump came back to, never, not %d and %d", two.hidden, one.hidden)
	}
	press(tabs, tea.KeyPressMsg{Code: tea.KeyEscape})
	if !one.escaped || tabs.at != 0 {
		t.Error("with no jump to go back along, esc should be the board's own")
	}

	press(tabs, tabKey)
	if tabs.at != 1 || one.hidden != 1 || two.hidden != 1 {
		t.Errorf("tab should tell one once and leave two shown, not %d and %d", one.hidden, two.hidden)
	}
}
