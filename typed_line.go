package gotermboard

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// typedLine is one line being typed and edited from its end: a question's answer, the filter,
// the reader's search. What enter and esc then do is each caller's own.
type typedLine struct{ runes []rune }

// lineEnd is how a key left a typed line.
type lineEnd int

const (
	lineOpen lineEnd = iota
	lineEntered
	lineCanceled
)

// press edits the line with key, or ends it: enter enters it, and esc or ctrl+c cancels it.
func (l *typedLine) press(key tea.KeyPressMsg) lineEnd {
	switch key.String() {
	case "enter":
		return lineEntered
	case "esc", "ctrl+c":
		return lineCanceled
	case "backspace":
		if n := len(l.runes); n > 0 {
			l.runes = l.runes[:n-1]
		}
	case "ctrl+u":
		l.runes = nil
	default:
		if key.Mod&^tea.ModShift == 0 {
			l.runes = append(l.runes, []rune(key.Text)...)
		}
	}
	return lineOpen
}

// paste adds a paste as one line, so a newline in it enters nothing.
func (l *typedLine) paste(content string) {
	l.runes = append(l.runes, []rune(strings.Join(strings.Fields(content), " "))...)
}

func (l *typedLine) set(text string) { l.runes = []rune(text) }

func (l *typedLine) String() string { return string(l.runes) }

// draw is the line after prefix with the cursor at its end, in width cells. It is cut from the
// left, so the end being typed stays on the screen.
func (l *typedLine) draw(prefix string, width int) string {
	text := l.String() + "█"
	if over := ansi.StringWidth(prefix+text) - width; over > 0 {
		text = ansi.TruncateLeft(text, over+1, "…")
	}
	return prefix + text
}
