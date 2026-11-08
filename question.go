package gotermboard

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Question is what a key asks on the footer's lines, until enter answers it or esc takes it away.
type Question struct {
	// Text draws in bold as written, so its own punctuation ends it.
	Text string
	// Typed takes one line typed under the question as the answer. Without it enter alone
	// answers, for an act that takes no text, and Answer is handed an empty string.
	Typed bool
	// Answer runs on enter with what was typed, trimmed. The question has closed by then, so it
	// can Say what it did, ask another question, or return a command such as an editor.
	Answer func(typed string) tea.Cmd
	// Cancel, where set, runs on esc or ctrl+c, so a board can say that nothing was done.
	Cancel func() tea.Cmd
}

// Ask puts q on the footer's lines. Until enter or esc every key and paste goes to it, so q
// and ? type themselves and tab changes no tab.
func (m *Model) Ask(q Question) {
	m.asking, m.answer = &q, typedLine{}
}

// answering is a key while a question is open.
func (m *Model) answering(key tea.KeyPressMsg) tea.Cmd {
	q := m.asking
	switch m.answer.press(key) {
	case lineEntered:
		m.asking = nil
		typed := ""
		if q.Typed {
			typed = strings.TrimSpace(m.answer.String())
		}
		if q.Answer != nil {
			return q.Answer(typed)
		}
	case lineCanceled:
		m.asking = nil
		if q.Cancel != nil {
			return q.Cancel()
		}
	}
	return nil
}

// questionLines is the open question in bold, then the line being typed, and the keys that answer.
func (m *Model) questionLines() (lines []string, keys string) {
	for _, line := range m.wrapped(m.asking.Text) {
		lines = append(lines, m.pal.Bold(line))
	}
	if !m.asking.Typed {
		return lines, "enter confirms  esc cancels"
	}
	return append(lines, m.answer.draw("› ", m.width)), "enter answers  esc cancels  ctrl+u clears"
}

// footerTextLines bounds the lines a question or what a key did may take, so a long one never
// pushes every row off the screen.
const footerTextLines = 4

// wrapped is text wrapped to the screen, stopping at footerTextLines lines with the last one
// ending in … where more was left.
func (m *Model) wrapped(text string) []string {
	lines := strings.Split(ansi.Wrap(text, m.width, "/"), "\n")
	if len(lines) > footerTextLines {
		lines = lines[:footerTextLines]
		lines[footerTextLines-1] = ansi.Truncate(lines[footerTextLines-1]+"…", m.width, "…")
	}
	return lines
}
