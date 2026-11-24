package gotermboard_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermboard"
	"github.com/datapointchris/gotermboard/markdown"
	"github.com/datapointchris/gotermboard/printedtable"
	"github.com/datapointchris/gotermstyle"
)

// textBoard draws a caller's text wherever a test puts it, under bold column titles. clean adds
// a row whose columns the text's row lines up with.
type textBoard struct {
	cell     gotermboard.Cell
	clean    bool
	header   gotermboard.Line
	entities []gotermboard.EntityRef
	actions  []gotermboard.Action
	busy     string
	sources  []gotermboard.Refresher
}

func (b textBoard) Sources() []gotermboard.Refresher { return b.sources }
func (b textBoard) Groupings() []string              { return nil }
func (b textBoard) Actions() []gotermboard.Action    { return b.actions }
func (b textBoard) UnderWay() string                 { return b.busy }

func (b textBoard) Frame(time.Time, string) gotermboard.Frame {
	cell := b.cell
	if cell.Text == "" {
		cell = gotermboard.C("row", "")
	}
	rows := []gotermboard.Row{{Key: "r", Cells: []gotermboard.Cell{cell, gotermboard.C("X9", "")}, Entities: b.entities}}
	if b.clean {
		rows = append(rows, gotermboard.Row{Key: "clean", Cells: []gotermboard.Cell{gotermboard.C("beforeafter", ""), gotermboard.C("X9", "")}})
	}
	return gotermboard.Frame{
		Columns: []gotermboard.Column{{Title: "NAME"}, {Title: "STATE"}},
		Groups:  []gotermboard.Group{{Key: "g", Title: b.header, Rows: rows}},
	}
}

// acting is a board whose s key runs act.
func acting(act func(m *gotermboard.Model)) textBoard {
	return textBoard{actions: []gotermboard.Action{{Keys: []string{"s"}, Run: func(m *gotermboard.Model) tea.Cmd {
		act(m)
		return nil
	}}}}
}

func key(text string) tea.KeyPressMsg { return tea.KeyPressMsg{Code: []rune(text)[0], Text: text} }

// controlsOn is each control a painted screen holds past the SGR colors a palette adds and the
// newlines between its lines.
func controlsOn(screen string) []string {
	var found []string
	for i := 0; i < len(screen); {
		if strings.HasPrefix(screen[i:], "\x1b[") {
			j := i + 2
			for j < len(screen) && (screen[j] >= '0' && screen[j] <= '9' || screen[j] == ';') {
				j++
			}
			if j < len(screen) && screen[j] == 'm' {
				i = j + 1
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(screen[i:])
		if r != '\n' && (r < 0x20 || r >= 0x7f && r < 0xa0 || r == utf8.RuneError && size == 1) {
			found = append(found, fmt.Sprintf("%q at byte %d", screen[i:i+size], i))
		}
		i += size
	}
	return found
}

func TestACallersTextReachesTheTerminalWithNoControlAtEverySinkAndTheBoardsColorsStillDraw(t *testing.T) {
	pal := gotermstyle.NewPalette(true)
	const width, height = 100, 20
	pressed := func(board gotermboard.Board, msgs ...tea.Msg) string {
		m := gotermboard.NewModel(board, pal)
		m.Update(tea.WindowSizeMsg{Width: width, Height: height})
		for _, msg := range msgs {
			m.Update(msg)
		}
		return m.View().Content
	}
	controls := []struct{ name, text string }{
		{"ESC and a byte, which resets the terminal", "\x1bc"},
		{"CSI moving up a line and erasing it", "\x1b[1A\x1b[2K"},
		{"OSC 52 setting the clipboard", "\x1b]52;c;ZWNobyBoaQ==\a"},
		{"DCS asking the terminal to answer", "\x1bP+q544e\x1b\\"},
		{"BEL", "\a"},
		{"BS over the word before", "\b\b\b\b\b\b"},
		{"CR back to the line's start", "\r"},
		{"CSI's C1 code point", "\u009b"},
		{"a CSI that a BS cuts short, which x/ansi reads on into the next letter", "\x1b[1\b"},
	}
	// Each sink draws text and names the color the board puts on it or beside it.
	// A pasted sink reads a CR as a line break, which a typed line joins with a space.
	sinks := []struct {
		name   string
		pasted bool
		draw   func(text string) (screen, own string)
	}{
		{"a row cell", false, func(text string) (string, string) {
			screen := gotermboard.Once(textBoard{cell: gotermboard.C(text, gotermboard.Red), clean: true}, pal, width, height)
			var at []int
			for _, line := range strings.Split(ansi.Strip(screen), "\n") {
				if i := strings.Index(line, "X9"); i >= 0 {
					at = append(at, ansi.StringWidth(line[:i]))
				}
			}
			if len(at) != 2 || at[0] != at[1] {
				t.Errorf("a row cell holding %q puts its next column at %v, out of line with a clean row's", text, at)
			}
			return screen, gotermboard.Red
		}},
		{"a group header", false, func(text string) (string, string) {
			board := textBoard{header: gotermboard.Line{gotermboard.C(text, gotermboard.Magenta)}}
			return gotermboard.Once(board, pal, width, height), gotermboard.Magenta
		}},
		{"a markdown body", false, func(text string) (string, string) {
			renderer := markdown.New(nil)
			reader := gotermboard.NewReader("doc", func(w int) []gotermboard.Line { return renderer.Lines("*"+text+"*", w) }, pal)
			return reader.Once(width), "\x1b[3m"
		}},
		{"what a key said", false, func(text string) (string, string) {
			return pressed(acting(func(m *gotermboard.Model) { m.Say(text) }), key("s")), gotermstyle.CodeBold
		}},
		{"a question's text", false, func(text string) (string, string) {
			ask := acting(func(m *gotermboard.Model) { m.Ask(gotermboard.Question{Text: text, Typed: true}) })
			return pressed(ask, key("s")), gotermstyle.CodeBold
		}},
		{"a question's choice", false, func(text string) (string, string) {
			ask := acting(func(m *gotermboard.Model) {
				m.Ask(gotermboard.Question{Text: "pick:", Choices: []gotermboard.Choice{{Key: "n", Label: text}}})
			})
			return pressed(ask, key("s")), gotermstyle.CodeBold
		}},
		{"a busy line", false, func(text string) (string, string) {
			return gotermboard.Once(textBoard{busy: text}, pal, width, height), gotermboard.Yellow
		}},
		{"a lock's id", false, func(text string) (string, string) {
			board := textBoard{entities: []gotermboard.EntityRef{{Kind: "thing", ID: text}}}
			tabs, err := gotermboard.NewTabs([]gotermboard.Tab{{Name: "one", Board: board}, {Name: "two", Board: board}}, pal, "")
			if err != nil {
				t.Fatal(err)
			}
			tabs.Update(tea.WindowSizeMsg{Width: width, Height: height})
			tabs.Init()
			tabs.Update(key(gotermboard.LockKey))
			return tabs.View().Content, gotermboard.LockCode
		}},
		{"a read's note", false, func(text string) (string, string) {
			source := gotermboard.NewSource("hosts", time.Minute, func(context.Context) (string, string, error) { return "read", text, nil })
			source.Refresh(context.Background())
			return pressed(textBoard{sources: []gotermboard.Refresher{source}}, key("?")), gotermstyle.CodeBold
		}},
		{"a printed table", false, func(text string) (string, string) {
			table := printedtable.Table{Columns: []printedtable.Column{{Title: text}}, Rows: [][]printedtable.Cell{{printedtable.C(text, gotermstyle.CodeRed)}}}
			return strings.Join(table.Lines(pal), "\n"), gotermstyle.CodeRed
		}},
		{"a reader's search, pasted and entered", true, func(text string) (string, string) {
			reader := gotermboard.NewReader("doc", func(int) []gotermboard.Line { return []gotermboard.Line{{gotermboard.C("body", "")}} }, pal)
			for _, msg := range []tea.Msg{tea.WindowSizeMsg{Width: width, Height: height}, key("/"), tea.PasteMsg{Content: text}, tea.KeyPressMsg{Code: tea.KeyEnter}} {
				reader.Update(msg)
			}
			return reader.View().Content, gotermstyle.CodeBold
		}},
	}
	for _, sink := range sinks {
		for _, control := range controls {
			screen, own := sink.draw("before" + control.text + "after")
			if found := controlsOn(screen); len(found) > 0 {
				t.Errorf("%s holding %s reached the terminal with %s:\n%q", sink.name, control.name, strings.Join(found, ", "), screen)
			}
			words := "beforeafter"
			if sink.pasted && control.text == "\r" {
				words = "before after"
			}
			if !strings.Contains(ansi.Strip(screen), words) {
				t.Errorf("%s holding %s should draw the words around it and none of the sequence:\n%s", sink.name, control.name, ansi.Strip(screen))
			}
			if !strings.Contains(screen, own) {
				t.Errorf("%s holding %s lost the board's own color %q:\n%q", sink.name, control.name, own, screen)
			}
		}
	}
}
