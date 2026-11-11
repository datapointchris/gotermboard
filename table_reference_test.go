package gotermboard

import (
	"flag"
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// This file is the whole-table draw the culled draw is held to. It paints every node and reads the
// cursor's line, the band and the screen off the painted lines. TestMain's hooks hold the culled
// draw to it, so a change to how the table scrolls or bands changes both.

// splitPinned is the leading lines that stay on screen, and those that scroll.
func splitPinned(lines []drawn) (pinned, scrolling []drawn) {
	at := slices.IndexFunc(lines, func(l drawn) bool { return !l.pinned })
	if at < 0 {
		return lines, nil
	}
	return lines[:at], lines[at:]
}

// stickyBand is the lines covering the top of those that scroll: the owners of the first line
// the band leaves showing, the innermost where they do not all fit. Each owner the band adds
// covers one more line, so this looks again until the lines it covers need no more. It can
// end short of what it covers, where a sibling arrives under the band, because a line named
// by the wrong owner reads worse than a line named by none.
func stickyBand(lines []drawn, top, height int) []string {
	room := min(bandMost, height/bandPerLines)
	covered := 0
	for {
		var band []string
		if at := top + covered; at < len(lines) {
			band = lines[at].owners
			band = band[max(len(band)-room, 0):]
		}
		if len(band) <= covered {
			return band
		}
		covered = len(band)
	}
}

// draw lays the nodes out to width. The first group holding rows and left open
// carries the one header row, so every group shares its columns. A group with cells
// of its own has no room for the titles, so they take a line of their own at the top. A
// board's Look.Rules puts them there too, and adds a rule under them.
func draw(p painter, columns []Column, nodes []node, widths []int, width int, cursor string, look cursorLook, board Look) []drawn {
	titled := false
	lines := make([]drawn, 0, len(nodes))
	ownLine := board.Rules || slices.ContainsFunc(nodes, func(n node) bool { return n.kind == nodeGroup && len(n.group.Cells) > 0 })
	if ownLine && len(columns) > 0 {
		titled = true
		first := p.bold(fit(columns[0].Title, max(widths[0]-1, 0), false))
		lines = append(lines, drawn{text: ansi.Truncate(" "+first+columnTitles(p, columns, widths), width, "…"), pinned: true})
		if board.Rules {
			lines = append(lines, drawn{text: RuleThin.across(p, width, ""), pinned: true})
		}
	}
	inside := make([][]string, len(nodes))
	for i, n := range nodes {
		from := len(lines)
		wantsTitles := n.kind == nodeGroup && n.depth == 0 && n.open && n.drawsRows && !titled
		titled = titled || wantsTitles
		lines = append(lines, drawNode(p, n, columns, widths, width, cursor, look, wantsTitles)...)
		var owners []string
		if n.parent >= 0 {
			owners = inside[n.parent]
		}
		for j := from; j < len(lines); j++ {
			lines[j].pinned = n.pinned
			lines[j].owners = owners
		}
		inside[i] = owners
		if n.names {
			inside[i] = append(slices.Clip(owners), firstLine(lines[from:], n))
		}
	}
	return lines
}

// wholeTable is the laid-out table painted whole: the lines that stay on screen, and those that scroll.
func wholeTable(m *Model) (pinned, scrolling []drawn) {
	nodes, columns, widths := m.laidOut()
	look := m.look()
	cursor := m.painter.cursorLook()
	cursor.bold = look.BoldCursor
	return splitPinned(draw(m.painter, columns, nodes, widths, m.tableWidth(), m.cursor, cursor, look))
}

// wholeFollow is where following the cursor through the painted table leaves the top.
func wholeFollow(m *Model) int {
	top := m.top
	pinned, lines := wholeTable(m)
	h := max(1, m.bodyHeight()-len(pinned))
	at := slices.IndexFunc(lines, func(l drawn) bool { return l.key != "" && l.key == m.cursor })
	if at >= 0 && !m.scrolled {
		end := at
		for end+1 < len(lines) && lines[end+1].continues {
			end++
		}
		if at < top {
			top = at
		}
		if end >= top+h {
			top = min(at, end-h+1)
		}
	}
	top = max(0, min(top, len(lines)-h))
	if at >= 0 && !m.scrolled {
		top = wholeClearOfBand(lines, top, at, h)
	}
	return top
}

func wholeClearOfBand(lines []drawn, top, at, height int) int {
	for top > 0 && at < top+len(stickyBand(lines, top, height)) {
		top--
	}
	return top
}

// wholeReveal is where revealing through the painted table leaves the top, after displayChanged.
func wholeReveal(m *Model) int {
	top := m.top
	pinned, lines := wholeTable(m)
	at := slices.IndexFunc(lines, func(l drawn) bool { return l.key != "" && l.key == m.cursor })
	if at < 0 {
		return top
	}
	end := at
	for end+1 < len(lines) && lines[end+1].key == "" {
		end++
	}
	h := max(1, m.bodyHeight()-len(pinned))
	if end >= top+h {
		top = min(at, end-h+1)
	}
	return wholeClearOfBand(lines, top, at, h)
}

// wholeUnderStrip is the whole table's draw in body lines, then the footer.
func wholeUnderStrip(m *Model, body int, footer []string) []string {
	var out []string
	pinned, lines := wholeTable(m)
	for _, line := range pinned {
		out = append(out, line.text)
	}
	h := max(1, body-len(pinned))
	shown := lines[min(m.top, len(lines)):min(m.top+h, len(lines))]
	band := stickyBand(lines, m.top, h)
	var panel []string
	if n, ok := m.selected(); ok && m.panelWidth() > 0 {
		panel = m.panelText(n, m.panelWidth(), 1<<30)
		panel = panel[:min(len(panel), h)]
	}
	tw := m.tableWidth()
	for i := range h {
		left := ""
		switch {
		case i < len(band):
			left = band[i]
		case i < len(shown):
			left = shown[i].text
		}
		if m.panelWidth() == 0 {
			out = append(out, left)
			continue
		}
		left += strings.Repeat(" ", max(tw-ansi.StringWidth(left), 0))
		right := ""
		if i < len(panel) {
			right = panel[i]
		}
		out = append(out, strings.TrimRight(left+" │ "+right, " "))
	}
	return append(out, footer...)
}

// held counts what the reference checked, so a test can say its boards reached each kind of screen.
var held struct{ draws, follows, reveals, nodes, scrolled, banded, pinned, panels int }

// holdToTheWholeTable sets the hooks: every node a test paints, every follow, every reveal and every
// draw is compared with the whole table's, and any difference panics.
func holdToTheWholeTable() {
	paintHook = func(n node, measured, painted int) {
		held.nodes++
		if measured != painted {
			panic(fmt.Sprintf("a node of kind %d measures %d lines and paints %d", n.kind, measured, painted))
		}
	}
	followHook = func(m *Model) int {
		held.follows++
		return wholeFollow(m)
	}
	revealHook = func(m *Model) int {
		held.reveals++
		return wholeReveal(m)
	}
	drawHook = func(m *Model, body int, footer, drew []string) {
		paint := paintHook
		paintHook = nil
		want := wholeUnderStrip(m, body, footer)
		paintHook = paint
		held.draws++
		if m.top > 0 {
			held.scrolled++
		}
		if k := m.kept(); k.ix.pinned > 0 {
			held.pinned++
		}
		if _, band := m.kept().ix.bandOver(m.top, max(1, body-m.kept().ix.pinned)); band > 0 {
			held.banded++
		}
		if m.panelWidth() > 0 {
			held.panels++
		}
		if !slices.Equal(drew, want) {
			for i := range max(len(drew), len(want)) {
				var got, whole string
				if i < len(drew) {
					got = drew[i]
				}
				if i < len(want) {
					whole = want[i]
				}
				if got != whole {
					panic(fmt.Sprintf("line %d of the draw differs from the whole table's, with the top at %d:\n  drew  %q\n  whole %q", i, m.top, got, whole))
				}
			}
		}
	}
}

// benchmarking says this run times something, where the reference would be what is timed.
func benchmarking() bool {
	bench := flag.Lookup("test.bench")
	return bench != nil && bench.Value.String() != ""
}

// panelText is a node's panelSource read now, under a row's ReaderTitle where it is the row's
// Document, and a group's label and summary where it holds neither.
func (m *Model) panelText(n node, width, most int) []string {
	var document []Line
	if authored, _ := n.panelSource(); authored != nil {
		document = authored(width)
	}
	return m.panelTextOf(n, document, width, most)
}
