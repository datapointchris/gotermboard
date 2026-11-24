package gotermboard

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermboard/terminalcontrol"
)

// Question is what a key asks on the footer's lines, until enter or a choice's key answers it or
// esc takes it away.
type Question struct {
	// Listing draws above Text, a line each, for a question about several things that names
	// each one. Each line is cut at the screen's edge. While the question is open the table
	// keeps one line, and a listing longer than the rest of the screen ends on how many more
	// it held.
	Listing []Line
	// Text draws in bold as written, so its own punctuation ends it.
	Text string
	// Typed takes one line typed under the question as the answer. Without it enter alone
	// answers, for an act that takes no text, and Answer is handed an empty string.
	Typed bool
	// Choices are the answers a single key picks, drawn under Text a line each, as a menu. A key
	// naming one closes the question and hands Answer that Key, and every other key but esc and
	// ctrl+c does nothing, enter included. Typed is ignored where Choices are set.
	Choices []Choice
	// Answer runs on enter with what was typed, trimmed, or on a choice's key with that key. The
	// question has closed by then, so it can Say what it did, ask another question, or return a
	// command such as an editor.
	Answer func(typed string) tea.Cmd
	// Cancel, where set, runs on esc or ctrl+c, so a board can say that nothing was done.
	Cancel func() tea.Cmd
}

// Choice is one answer of a Question's menu.
type Choice struct {
	// Key is the key that picks it, as Bubble Tea names a press, such as "n" or "N".
	Key string
	// Label says what picking it does, drawn after the key and cut at the screen's edge.
	Label string
}

// Ask puts q on the footer's lines. Until enter or esc every key and paste goes to it, so q
// and ? type themselves and tab changes no tab.
func (m *Model) Ask(q Question) {
	m.asking, m.answer = &q, typedLine{}
}

// typed says the question takes a line typed under it.
func (q *Question) typed() bool { return q.Typed && len(q.Choices) == 0 }

// answering is a key while a question is open.
func (m *Model) answering(key tea.KeyPressMsg) tea.Cmd {
	q := m.asking
	if len(q.Choices) > 0 {
		return m.choosing(key)
	}
	switch m.answer.press(key) {
	case lineEntered:
		m.asking = nil
		typed := ""
		if q.typed() {
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

// choosing is a key while a question with Choices is open: the key naming a choice answers, esc
// and ctrl+c cancel, and every other key does nothing, so a reflex enter picks nothing.
func (m *Model) choosing(key tea.KeyPressMsg) tea.Cmd {
	q := m.asking
	pressed := key.String()
	if pressed == "esc" || pressed == "ctrl+c" {
		m.asking = nil
		if q.Cancel != nil {
			return q.Cancel()
		}
		return nil
	}
	for _, choice := range q.Choices {
		if choice.Key != pressed {
			continue
		}
		m.asking = nil
		if q.Answer != nil {
			return q.Answer(choice.Key)
		}
		return nil
	}
	return nil
}

// questionLines is the open question in bold, then its choices or the line being typed, and the
// keys that answer. The question's text takes the room its choices and a line of the listing
// leave, so the whole of what is confirmed shows, and the listing gives way to both.
func (m *Model) questionLines(room int) (lines []string, keys string) {
	below := len(m.asking.Choices) + min(len(m.asking.Listing), 1)
	if m.asking.typed() {
		below++
	}
	for _, line := range m.wrapped(m.asking.Text, max(room-below, footerTextLines), "") {
		lines = append(lines, m.painter.bold(line))
	}
	keys = "enter confirms  esc cancels"
	switch {
	case len(m.asking.Choices) > 0:
		picks := make([]string, len(m.asking.Choices))
		for i, choice := range m.asking.Choices {
			lines = append(lines, ansi.Truncate("  "+m.painter.bold(choice.Key)+"  "+terminalcontrol.Strip(choice.Label), m.width, "…"))
			picks[i] = choice.Key
		}
		keys = strings.Join(picks, " ") + " chooses  esc cancels"
	case m.asking.typed():
		lines = append(lines, m.answer.draw("› ", m.width))
		keys = "enter answers  esc cancels  ctrl+u clears"
	}
	return append(m.listingLines(room-len(lines)), lines...), keys
}

// listingLines is the open question's Listing in room lines at most, the last saying how many
// more there were where it held more than that.
func (m *Model) listingLines(room int) []string {
	listing := m.asking.Listing
	room = max(room, 1)
	cut := len(listing) > room
	if cut {
		listing = listing[:room-1]
	}
	lines := make([]string, 0, room)
	for _, line := range listing {
		lines = append(lines, ansi.Truncate(m.painter.line(line), m.width, "…"))
	}
	if cut {
		lines = append(lines, ansi.Truncate(strconv.Itoa(len(m.asking.Listing)-len(listing))+" more", m.width, "…"))
	}
	return lines
}

// footerTextLines bounds the lines what a key did may take, so a long one never pushes every row
// off the screen, and the least a question's own text gets. The whole of what a key did opens on
// a Reader with saidReadKey, which the cut line names.
const footerTextLines = 4

// saidReadKey opens the whole of what the last key said on a Reader, while the footer cuts it.
const saidReadKey = "ctrl+o"

// wrappedLines is text wrapped to width, its terminal controls out first, because the wrap keeps
// every escape sequence it meets.
func wrappedLines(text string, width int) []string {
	return strings.Split(ansi.Wrap(terminalcontrol.Strip(text), width, "/"), "\n")
}

// wrapped is text wrapped to the screen in most lines, at least one, the last ending in … and
// then tail where more was left.
func (m *Model) wrapped(text string, most int, tail string) []string {
	lines := wrappedLines(text, m.width)
	most = max(most, 1)
	if len(lines) > most {
		lines = lines[:most]
		tail = "…" + tail
		last := ansi.Truncate(lines[most-1], max(m.width-ansi.StringWidth(tail), 0), "")
		lines[most-1] = ansi.Truncate(last+tail, m.width, "…")
	}
	return lines
}

// saidCut says the footer cuts said short.
func (m *Model) saidCut(said string) bool {
	return len(wrappedLines(said, m.width)) > footerTextLines
}
