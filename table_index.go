package gotermboard

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// lineIndex says where every laid-out node sits among the table's lines, measured from what wraps
// and never from painting, so the cursor, the scroll and the draw each cost what the screen shows.
// It is built with its layout and dropped with it.
type lineIndex struct {
	// head is the lines ahead of every node: the column titles on a line of their own, and the
	// rule a board's Look.Rules puts under them.
	head int
	// starts[i] is the lines ahead of node i, head among them. starts[len(nodes)] is every line.
	starts []int
	// pinned is the leading lines that stay on screen above those that scroll.
	pinned int
	// titlesAt is the node whose header carries the column titles, and -1 where none does.
	titlesAt int
	// owners[i] is how many nodes holding node i name themselves, which is how tall a band over
	// one of its lines can grow.
	owners []int32
	// stops is each node the cursor lands on, in order, and stopAt the first stop holding a key.
	stops  []int
	stopAt map[string]int
	// hasGroups says some group is a cursor stop, and anyUnder that some row or group, folded or
	// not, has a summary to draw at these widths.
	hasGroups, anyUnder bool
	// documents is each open document's lines at this width, laid out once and unpainted.
	documents map[int][]Line
}

// total is every line of the table, and scrolling those that scroll.
func (ix *lineIndex) total() int     { return ix.starts[len(ix.starts)-1] }
func (ix *lineIndex) scrolling() int { return ix.total() - ix.pinned }

// nodeAt is the node drawing the table's line, and len(nodes) past the last line.
func (ix *lineIndex) nodeAt(line int) int {
	return sort.Search(len(ix.starts)-1, func(i int) bool { return ix.starts[i+1] > line })
}

// index measures the layout's nodes. groups are the frame's, which the f key's answer reads whole.
func (k *keptLayout) index(groups []Group) *lineIndex {
	ix := &lineIndex{
		starts: make([]int, len(k.nodes)+1), owners: make([]int32, len(k.nodes)), titlesAt: -1,
		stopAt: make(map[string]int, len(k.nodes)), anyUnder: anyUnder(groups, k.widths, 0),
	}
	titled := false
	if len(k.columns) > 0 && (k.look.Rules || titlesTakeOwnLine(k.nodes)) {
		titled = true
		ix.head = 1
		if k.look.Rules {
			ix.head = 2
		}
	}
	at := ix.head
	ix.pinned = ix.head
	for i := range k.nodes {
		n := &k.nodes[i]
		if !titled && n.kind == nodeGroup && n.depth == 0 && n.open && n.drawsRows {
			titled, ix.titlesAt = true, i
		}
		if n.parent >= 0 {
			ix.owners[i] = ix.owners[n.parent]
			if k.nodes[n.parent].names {
				ix.owners[i]++
			}
		}
		if n.selectable() {
			if _, held := ix.stopAt[n.key]; !held {
				ix.stopAt[n.key] = len(ix.stops)
			}
			ix.stops = append(ix.stops, i)
			ix.hasGroups = ix.hasGroups || n.kind == nodeGroup
		}
		ix.starts[i] = at
		at += k.height(ix, i)
		if n.pinned {
			ix.pinned = at
		}
	}
	ix.starts[len(k.nodes)] = at
	return ix
}

// titlesTakeOwnLine says a group has cells of its own, which leave its header no room for the
// column titles.
func titlesTakeOwnLine(nodes []node) bool {
	for i := range nodes {
		if nodes[i].kind == nodeGroup && len(nodes[i].group.Cells) > 0 {
			return true
		}
	}
	return false
}

// height is how many lines drawNode gives node i, case by case as drawNodeBody draws them. It lays
// an open document out, because only its own layout says how long it is, and keeps the lines.
func (k *keptLayout) height(ix *lineIndex, i int) int {
	n := k.nodes[i]
	switch n.kind {
	case nodeGroup:
		g := n.group
		h := len(g.Below)
		if g.Overline != RuleNone {
			h++
		}
		if g.Underline != RuleNone {
			h++
		}
		if len(g.Span) == 0 {
			return h + 1 + continuedCount(g.WrapLast, k.widths)
		}
		h += len(wrapWords(g.Span, spanRoom(n, k.width)))
		if len(g.Cells) > 0 || len(g.WrapLast) > 0 || len(g.Beside) > 0 {
			h += 1 + continuedCount(g.WrapLast, k.widths)
		}
		return h
	case nodeRow:
		h := 1 + continuedCount(n.row.WrapLast, k.widths)
		if len(n.row.Second) > 0 {
			h++
		}
		return h
	case nodeUnder:
		if !n.wraps {
			return 1
		}
		return len(wrap(n.cell.Text, underRoom(n, k.width)))
	case nodeDetail:
		return len(wrap(n.cell.Text, detailRoom(n, k.width)))
	case nodeDocument:
		lines := n.document(documentRoom(n, k.width))
		if ix.documents == nil {
			ix.documents = map[int][]Line{}
		}
		ix.documents[i] = lines
		return len(lines)
	}
	return 1
}

// continuedCount is how many lines a wrapping last column takes past its first.
func continuedCount(wrapped Line, widths []int) int {
	if len(wrapped) == 0 {
		return 0
	}
	return max(len(wrapLine(wrapped, widths[len(widths)-1]))-1, 0)
}

// The width each kind of text wraps in. The measure and the paint both ask these, so a node's
// height and its lines come from one wrap.
func spanRoom(n node, width int) int     { return max(width-spanColumn(n.depth, n.group), 20) }
func detailRoom(n node, width int) int   { return max(width-indentOf(n.depth), 20) }
func documentRoom(n node, width int) int { return max(width-n.column, 20) }

func underRoom(n node, width int) int {
	lead := indentOf(n.depth) + underMarkWidth
	if n.column >= underMarkWidth {
		lead = n.column
	}
	return max(width-lead, 20)
}

// underMarkWidth is the cells a summary's └─ and its space take.
const underMarkWidth = 3

// keyLine is the scrolling line the cursor lands on at node i, past a group's overline.
func (k *keptLayout) keyLine(i int) int {
	at := k.ix.starts[i] - k.ix.pinned
	if n := k.nodes[i]; n.kind == nodeGroup && n.group.Overline != RuleNone {
		at++
	}
	return at
}

// cursorSpan is the scrolling lines the cursor keeps in view on key: the line it lands on, through
// a row's Second. ok is false where no line the table draws holds key.
func (k *keptLayout) cursorSpan(key string) (at, end int, ok bool) {
	stop, held := k.ix.stopAt[key]
	if key == "" || !held {
		return 0, 0, false
	}
	i := k.ix.stops[stop]
	at = k.keyLine(i)
	end = at
	if n := k.nodes[i]; n.kind == nodeRow && len(n.row.Second) > 0 {
		end = k.ix.starts[i+1] - k.ix.pinned - 1
	}
	return at, end, true
}

// lastOpenedLine is the last scrolling line under the stop before the next keyed stop's own line, which
// is what opens under a row and its group's rule or gap.
func (k *keptLayout) lastOpenedLine(stop int) int {
	for next := stop + 1; next < len(k.ix.stops); next++ {
		if i := k.ix.stops[next]; k.nodes[i].key != "" {
			return k.keyLine(i) - 1
		}
	}
	return k.ix.scrolling() - 1
}

// bandOver is the node whose owners the band names with the table scrolled to top, and how many of
// them it names, the innermost where they do not all fit. Each owner the band adds covers one
// more line, so this looks again until the lines it covers need no more. It can end short of what
// it covers, where a sibling arrives under the band, because a line named by the wrong owner reads
// worse than a line named by none.
func (ix *lineIndex) bandOver(top, height int) (over, count int) {
	room := min(bandMost, height/bandPerLines)
	covered := 0
	for {
		over, count = -1, 0
		if at := ix.pinned + top + covered; at < ix.total() {
			over = ix.nodeAt(at)
			count = min(int(ix.owners[over]), room)
		}
		if count <= covered {
			return over, count
		}
		covered = count
	}
}

// paintHook hears each node a draw paints, with the lines the index measured for it and the lines
// it painted. Tests set it, to hold the measure to the paint and to count what a draw paints.
var paintHook func(n node, measured, painted int)

// tablePainter paints the nodes a range of the table's lines cuts, and nothing else.
type tablePainter struct {
	p      painter
	k      *keptLayout
	cursor string
	look   cursorLook
}

func (m *Model) tablePainter(k *keptLayout) tablePainter {
	look := m.painter.cursorLook()
	look.bold = k.look.BoldCursor
	return tablePainter{p: m.painter, k: k, cursor: m.cursor, look: look}
}

// node is the lines node i draws, held to the height the index measured for it. A measure that
// disagreed with the paint would otherwise move every line under the node.
func (t tablePainter) node(i int) []drawn {
	lines := drawNode(t.p, t.k.nodes[i], t.k.columns, t.k.widths, t.k.width, t.cursor, t.look, i == t.k.ix.titlesAt)
	measured := t.k.ix.starts[i+1] - t.k.ix.starts[i]
	if paintHook != nil {
		paintHook(t.k.nodes[i], measured, len(lines))
	}
	for len(lines) < measured {
		lines = append(lines, drawn{})
	}
	return lines[:measured]
}

// lines is the table's lines from from up to to, counted from its first, the column titles' own
// line among them. It paints each node the range cuts and keeps the lines inside it. An open
// document paints only the lines that show.
func (t tablePainter) lines(from, to int) []string {
	ix := t.k.ix
	from, to = max(from, 0), min(to, ix.total())
	if from >= to {
		return nil
	}
	out := make([]string, 0, to-from)
	if from < ix.head {
		out = append(out, titleLines(t.p, t.k.columns, t.k.widths, t.k.width, t.k.look)[from:min(to, ix.head)]...)
	}
	for i := ix.nodeAt(max(from, ix.head)); i < len(t.k.nodes) && ix.starts[i] < to; i++ {
		start := ix.starts[i]
		low, high := max(from-start, 0), min(to, ix.starts[i+1])-start
		if low >= high {
			continue
		}
		if document, kept := ix.documents[i]; kept {
			if paintHook != nil {
				paintHook(t.k.nodes[i], len(document), len(document))
			}
			for _, line := range document[low:high] {
				out = append(out, documentLine(t.p, t.k.nodes[i], line, t.k.width))
			}
			continue
		}
		for _, line := range t.node(i)[low:high] {
			out = append(out, line.text)
		}
	}
	return out
}

// band is the lines covering the top of those that scroll: the first line of each owner of the
// first line the band leaves showing, outermost first.
func (t tablePainter) band(top, height int) []string {
	over, count := t.k.ix.bandOver(top, height)
	if count == 0 {
		return nil
	}
	band := make([]string, count)
	at := t.k.nodes[over].parent
	for i := count - 1; i >= 0; i-- {
		for !t.k.nodes[at].names {
			at = t.k.nodes[at].parent
		}
		band[i] = firstLine(t.node(at), t.k.nodes[at])
		at = t.k.nodes[at].parent
	}
	return band
}

// titleLines is the column titles on a line of their own, and the rule a board's Look.Rules puts
// under them.
func titleLines(p painter, columns []Column, widths []int, width int, board Look) []string {
	first := p.bold(fit(columns[0].Title, max(widths[0]-1, 0), false))
	lines := []string{cut(" "+first+columnTitles(p, columns, widths), width)}
	if board.Rules {
		lines = append(lines, RuleThin.across(p, width, ""))
	}
	return lines
}

// documentLine is one line of a document at its node's column.
func documentLine(p painter, n node, line Line, width int) string {
	return cut(strings.Repeat(" ", n.column)+p.line(line), width)
}

// narrow says r is a glyph this package draws that takes one cell and joins no rune beside it: the
// box-drawing, block and braille ranges, and the single marks named here. A glyph left out is
// still measured right, by ansi.StringWidth.
func narrow(r rune) bool {
	if (r >= 0x2500 && r <= 0x259f) || (r >= 0x2800 && r <= 0x28ff) {
		return true
	}
	return strings.ContainsRune("·•…›↪▪▰▱▸▾◆●◦☐☑", r)
}

// counted is the cells text takes where it holds only color codes, printable ASCII and narrow
// glyphs, each of which is one cell or none. ok is false for any other text.
func counted(text string) (width int, ok bool) {
	for i := 0; i < len(text); {
		switch c := text[i]; {
		case c >= 0x20 && c <= 0x7e:
			width++
			i++
		case c == '\033':
			end := sgrEnd(text, i)
			if end == 0 {
				return 0, false
			}
			i = end
		case c >= utf8.RuneSelf:
			r, size := utf8.DecodeRuneInString(text[i:])
			if !narrow(r) {
				return 0, false
			}
			width++
			i += size
		default:
			return 0, false
		}
	}
	return width, true
}

// textWidth is the cells text takes, painted or not. Counting spares the grapheme walk that
// ansi.StringWidth runs, which the layout would pay over every cell of every row and the draw over
// every line it pads or cuts.
func textWidth(text string) int {
	if width, ok := counted(text); ok {
		return width
	}
	return ansi.StringWidth(text)
}

// cut is text cut to width with an ellipsis. ansi.Truncate returns text that fits as it came, so
// text counted to fit is not handed to it.
func cut(text string, width int) string {
	if counted, ok := counted(text); ok && counted <= width {
		return text
	}
	return ansi.Truncate(text, width, "…")
}
