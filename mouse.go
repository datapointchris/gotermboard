package gotermboard

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// mouseMode asks the terminal for presses, drags, releases and the wheel. Taking the mouse takes
// the terminal's own selection too, so selecting gives it back inside the program.
const mouseMode = tea.MouseModeCellMotion

// wheelLines is how far one notch of the mouse wheel scrolls.
const wheelLines = 3

// multiClickWithin is how soon a press on the same cell makes a double or a triple click.
const multiClickWithin = 400 * time.Millisecond

// wheelBy is the lines a notch of the wheel scrolls, down positive, and false for any other button.
func wheelBy(button tea.MouseButton) (int, bool) {
	switch button {
	case tea.MouseWheelDown:
		return wheelLines, true
	case tea.MouseWheelUp:
		return -wheelLines, true
	}
	return 0, false
}

type cellAt struct{ x, y int }

// columns is a run of screen columns, the right one not in it.
type columns struct{ left, right int }

func (c columns) shift(by int) columns { return columns{c.left + by, c.right + by} }

func (c columns) clamp(x int) int { return max(c.left, min(x, c.right-1)) }

// selecting is the mouse's selection of the text on screen, as a terminal's own is: a drag copies
// what it covers on release, a double click the word under it and a triple click its line. Each
// keeps to the columns of the part of the screen it started in, so a drag in a side panel copies
// none of the table beside it.
type selecting struct {
	// shown is the screen as last drawn, which is what a selection copies.
	shown     string
	from, to  cellAt
	within    columns
	dragging  bool
	pressedOn cellAt
	pressedAt time.Time
	presses   int
}

// pointed is what the mouse did: chose text to copy, or let a click go on the cell it pressed.
type pointed struct {
	text    string
	chose   bool
	clicked bool
	at      cellAt
}

func copying(text string, chose bool) pointed { return pointed{text: text, chose: chose} }

// mouse takes a press, a drag or a release. region is the columns of the part of the screen
// holding a cell. It returns the text a release, a double or a triple click chose, or the cell a
// single click let go of without a drag.
func (s *selecting) mouse(msg tea.MouseMsg, now time.Time, region func(x, y int) columns) pointed {
	m := msg.Mouse()
	at := cellAt{m.X, m.Y}
	switch msg.(type) {
	case tea.MouseClickMsg:
		if m.Button != tea.MouseLeft {
			return pointed{}
		}
		if at == s.pressedOn && now.Sub(s.pressedAt) < multiClickWithin {
			s.presses = s.presses%3 + 1
		} else {
			s.presses = 1
		}
		s.pressedOn, s.pressedAt = at, now
		s.within = region(at.x, at.y)
		at.x = s.within.clamp(at.x)
		s.from, s.to, s.dragging = at, at, s.presses == 1
		switch s.presses {
		case 2:
			return copying(s.word(at))
		case 3:
			return copying(chosen(cellText(s.line(at.y), s.within)))
		}
	case tea.MouseMotionMsg:
		if s.dragging {
			s.to = cellAt{s.within.clamp(at.x), at.y}
		}
	case tea.MouseReleaseMsg:
		if !s.dragging {
			return pointed{}
		}
		s.dragging = false
		s.to = cellAt{s.within.clamp(at.x), at.y}
		if s.from == s.to {
			return pointed{clicked: true, at: s.from}
		}
		return copying(s.covered())
	}
	return pointed{}
}

func chosen(text string) (string, bool) { return text, strings.TrimSpace(text) != "" }

func (s *selecting) line(y int) string {
	lines := strings.Split(s.shown, "\n")
	if y < 0 || y >= len(lines) {
		return ""
	}
	return lines[y]
}

// ends is the drag's first and last cell in reading order.
func (s *selecting) ends() (cellAt, cellAt) {
	if s.to.y < s.from.y || s.to.y == s.from.y && s.to.x < s.from.x {
		return s.to, s.from
	}
	return s.from, s.to
}

// spanOn is the columns the drag covers on screen line y, and false on a line it misses.
func (s *selecting) spanOn(y int) (columns, bool) {
	first, last := s.ends()
	if y < first.y || y > last.y {
		return columns{}, false
	}
	c := s.within
	if y == first.y {
		c.left = first.x
	}
	if y == last.y {
		c.right = last.x + 1
	}
	return c, true
}

// covered is the text the drag covers, a line of the screen to a line.
func (s *selecting) covered() (string, bool) {
	first, last := s.ends()
	out := make([]string, 0, last.y-first.y+1)
	for y := first.y; y <= last.y; y++ {
		c, _ := s.spanOn(y)
		out = append(out, cellText(s.line(y), c))
	}
	return chosen(strings.Join(out, "\n"))
}

// cellText is the plain text in columns c of a drawn line, without the spaces that pad it.
func cellText(line string, c columns) string {
	return strings.TrimRight(ansi.Strip(ansi.Cut(line, c.left, c.right)), " ")
}

// word is the run of cells around at that holds no break, inside the columns the press is in.
func (s *selecting) word(at cellAt) (string, bool) {
	type cell struct {
		text        string
		left, right int
	}
	var cells []cell
	plain := ansi.Strip(s.line(at.y))
	for x := 0; plain != ""; {
		cluster, width := ansi.FirstGraphemeCluster(plain, ansi.GraphemeWidth)
		plain = plain[len(cluster):]
		cells = append(cells, cell{cluster, x, x + width})
		x += width
	}
	under := -1
	for i, c := range cells {
		if c.left <= at.x && at.x < c.right {
			under = i
		}
	}
	if under < 0 || breaksWord(cells[under].text) {
		return "", false
	}
	first, last := under, under
	for first > 0 && cells[first-1].left >= s.within.left && !breaksWord(cells[first-1].text) {
		first--
	}
	for last < len(cells)-1 && cells[last+1].right <= s.within.right && !breaksWord(cells[last+1].text) {
		last++
	}
	var b strings.Builder
	for _, c := range cells[first : last+1] {
		b.WriteString(c.text)
	}
	return chosen(b.String())
}

// wordBreaks end the word a double click copies, beside space and the box-drawing rules: the
// brackets, quotes and commas that sit around an id or a path in text.
const wordBreaks = "()[]{}<>\"'`,;"

func breaksWord(cluster string) bool {
	r := []rune(cluster)[0]
	return unicode.IsSpace(r) || strings.ContainsRune(wordBreaks, r) || r >= 0x2500 && r <= 0x257F
}

// paint keeps screen as the text a selection copies, and shows a drag on it in reverse video
// while the button is held.
func (s *selecting) paint(screen string) string {
	s.shown = screen
	if !s.dragging || s.from == s.to {
		return screen
	}
	lines := strings.Split(screen, "\n")
	for y := range lines {
		if c, ok := s.spanOn(y); ok {
			lines[y] = reversed(lines[y], c)
		}
	}
	return strings.Join(lines, "\n")
}

// reversed is line with columns c plain in reverse video, padded to fill them, and the rest of the
// line as it was drawn. The cut past c re-sends every style the line set before it.
func reversed(line string, c columns) string {
	left := ansi.Truncate(line, c.left, "")
	left += strings.Repeat(" ", max(c.left-ansi.StringWidth(left), 0))
	mid := ansi.Strip(ansi.Cut(line, c.left, c.right))
	mid += strings.Repeat(" ", max(c.right-c.left-ansi.StringWidth(mid), 0))
	return left + ansi.ResetStyle + ansi.Style{}.Reverse(true).Styled(mid) + ansi.TruncateLeft(line, c.right, "")
}

// copiedSaid is what a footer says of text that the mouse copied.
func copiedSaid(text string) string {
	if lines := strings.Count(text, "\n") + 1; lines > 1 {
		return fmt.Sprintf("copied %d lines", lines)
	}
	return "copied " + text
}
