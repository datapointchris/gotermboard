package gotermboard

// nodeParts is what a node draws, part by part, before any of it is painted. The line index sums
// its lines and drawNode paints it, so a node's height and its lines come from one plan.
type nodeParts struct {
	overline, underline bool
	// span is a Span group's Span, wrapped.
	span []Line
	// head is the line holding the node's cells, or the one line a blank, a rule or an unwrapped
	// summary draws.
	head bool
	// continued is a wrapping last column past its first line.
	continued []Line
	// second is a row's Second, on a line of its own.
	second bool
	// below is what a group keeps under its header.
	below []Line
	// pieces is a detail's text or a wrapping summary's, wrapped.
	pieces   []string
	document []Line
}

func (p nodeParts) lines() int {
	n := len(p.span) + len(p.continued) + len(p.below) + len(p.pieces) + len(p.document)
	for _, drawn := range [...]bool{p.overline, p.head, p.second, p.underline} {
		if drawn {
			n++
		}
	}
	return n
}

// partsOf is node n's parts at width, its columns at widths. It lays an open document out,
// because only its own layout says how long it is.
func partsOf(n node, widths []int, width int) nodeParts {
	switch n.kind {
	case nodeGroup:
		g := n.group
		parts := nodeParts{overline: n.overlined(), underline: g.Underline != RuleNone, below: g.Below, head: true}
		if len(g.Span) > 0 {
			parts.span = wrapWords(g.Span, spanRoom(n, width))
			parts.head = len(g.Cells) > 0 || len(g.WrapLast) > 0 || len(g.Beside) > 0
		}
		if parts.head {
			parts.continued = continuedPieces(g.WrapLast, widths)
		}
		return parts
	case nodeRow:
		return nodeParts{head: true, continued: continuedPieces(n.row.WrapLast, widths), second: len(n.row.Second) > 0}
	case nodeUnder:
		if n.wraps {
			return nodeParts{pieces: wrap(n.cell.Text, underRoom(n, width))}
		}
	case nodeDetail:
		return nodeParts{pieces: wrap(n.cell.Text, detailRoom(n, width))}
	case nodeDocument:
		return nodeParts{document: n.document(documentRoom(n, width))}
	}
	return nodeParts{head: true}
}

// overlined says the node is a group drawing a rule above its header.
func (n node) overlined() bool { return n.kind == nodeGroup && n.group.Overline != RuleNone }

// continuedPieces is a wrapping last column past its first line.
func continuedPieces(wrapped Line, widths []int) []Line {
	if len(wrapped) == 0 {
		return nil
	}
	pieces := wrapLine(wrapped, widths[len(widths)-1])
	return pieces[min(1, len(pieces)):]
}

// The width each kind of text wraps in.
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
