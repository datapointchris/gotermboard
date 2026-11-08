package gotermboard

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// question is what a key asked on the footer's lines, and what enter does with the answer.
type question struct {
	text string
	// typing says the answer is a line typed under the question. Without it enter alone answers.
	typing bool
	typed  []rune
	answer func(typed string) tea.Cmd
}

// Ask puts text on the footer's lines as a question, and takes one line typed under it as the
// answer. enter hands answer what was typed, trimmed, and esc or ctrl+c takes the question
// away with answer never called. Until then every key and paste goes to the question. The
// text is drawn as written, so its own punctuation ends it.
func (m *Model) Ask(text string, answer func(typed string) tea.Cmd) {
	m.asking = &question{text: text, typing: true, answer: answer}
}

// Confirm puts text on the footer's lines as a question enter alone answers, for a key whose act
// takes no text. esc or ctrl+c takes it away with yes never called.
func (m *Model) Confirm(text string, yes func() tea.Cmd) {
	m.asking = &question{text: text, answer: func(string) tea.Cmd { return yes() }}
}

// answering is a key while a question is open. The question closes before its answer runs, so
// the answer can ask another.
func (m *Model) answering(key tea.KeyPressMsg) tea.Cmd {
	q := m.asking
	switch key.String() {
	case "enter":
		m.asking = nil
		return q.answer(strings.TrimSpace(string(q.typed)))
	case "esc", "ctrl+c":
		m.asking = nil
	case "backspace":
		if n := len(q.typed); n > 0 {
			q.typed = q.typed[:n-1]
		}
	case "ctrl+u":
		q.typed = nil
	default:
		if q.typing && key.Mod&^tea.ModShift == 0 {
			q.typed = append(q.typed, []rune(key.Text)...)
		}
	}
	return nil
}

// pasteAnswer adds a paste to what is typed as one line, so a newline in it answers nothing.
func (m *Model) pasteAnswer(content string) {
	if m.asking.typing {
		m.asking.typed = append(m.asking.typed, []rune(strings.Join(strings.Fields(content), " "))...)
	}
}

// questionLines is the open question in bold, then the line being typed, and the keys that answer.
func (m *Model) questionLines() (lines []string, keys string) {
	for _, line := range m.wrapped(m.asking.text) {
		lines = append(lines, m.pal.Bold(line))
	}
	if !m.asking.typing {
		return lines, "enter confirms  esc cancels"
	}
	return append(lines, m.typingLine("› ", string(m.asking.typed))), "enter answers  esc cancels  ctrl+u clears"
}

// footerTextLines bounds the lines a question or what a key did may take, so a long one never
// pushes every row off the screen.
const footerTextLines = 4

// wrapped is text wrapped to the screen rather than cut, so a long key or reason stays whole.
func (m *Model) wrapped(text string) []string {
	lines := strings.Split(ansi.Wrap(text, m.width, "/"), "\n")
	if len(lines) > footerTextLines {
		lines = lines[:footerTextLines]
		lines[footerTextLines-1] = ansi.Truncate(lines[footerTextLines-1]+"…", m.width, "…")
	}
	return lines
}

// typingLine is text being typed after prefix, with the cursor at its end. It is cut from the
// left, so the end being typed stays on the screen.
func (m *Model) typingLine(prefix, text string) string {
	text += "█"
	if over := ansi.StringWidth(prefix+text) - m.width; over > 0 {
		text = ansi.TruncateLeft(text, over+1, "…")
	}
	return prefix + text
}
