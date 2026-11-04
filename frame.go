package gotermboard

import (
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermstyle"
)

// The codes a view colors a cell with, all in ANSI 0-15 so they follow the theme.
const (
	Red     = gotermstyle.CodeRed
	Yellow  = gotermstyle.CodeYellow
	Green   = gotermstyle.CodeGreen
	Cyan    = gotermstyle.CodeCyan
	Blue    = gotermstyle.CodeBlue
	Magenta = gotermstyle.CodeMagenta
	White   = gotermstyle.CodeWhite
	Bold    = gotermstyle.CodeBold

	BoldRed     = "\033[1;91m"
	BoldYellow  = "\033[1;93m"
	BoldGreen   = "\033[1;92m"
	BoldCyan    = "\033[1;96m"
	BoldBlue    = "\033[1;94m"
	BoldMagenta = "\033[1;95m"
	Reverse     = "\033[7m"
)

// columnGap is the two spaces between columns, as go-gh's table printer has them.
const columnGap = "  "

// Cell is text and its color. Ticks says the text follows the clock, as an age does,
// so its changing is no change to the row, though its color changing still is.
type Cell struct {
	Text  string
	Code  string
	Ticks bool
}

// C is a Cell.
func C(text, code string) Cell { return Cell{Text: text, Code: code} }

// Ticking is a Cell whose text follows the clock.
func Ticking(text, code string) Cell { return Cell{Text: text, Code: code, Ticks: true} }

// Line is cells drawn one after another.
type Line []Cell

// Text is the line with its colors out.
func (l Line) Text() string {
	var b strings.Builder
	for _, c := range l {
		b.WriteString(c.Text)
	}
	return b.String()
}

func (l Line) width() int { return ansi.StringWidth(l.Text()) }

func (l Line) paint(pal gotermstyle.Palette) string {
	var b strings.Builder
	for _, c := range l {
		b.WriteString(pal.Paint(c.Code, c.Text))
	}
	return b.String()
}

// emboldened is the line in bold over each cell's own color.
func (l Line) emboldened() Line {
	out := slices.Clone(l)
	for i := range out {
		out[i].Code = Bold + out[i].Code
	}
	return out
}

// Column is one column of the table. The last is unpadded, so the screen's edge
// cuts it first. Max caps its width, where one long value must not take the row.
type Column struct {
	Title string
	Max   int
	Right bool
}

// Row is one line of the table: Cells on the columns, or Span across them.
type Row struct {
	Key    string
	Symbol Cell
	Cells  []Cell
	Span   Line
	// WrapLast is the last column as cells of their own colors. It continues on
	// further lines at that column, where a cell in Cells is cut at the screen's edge.
	WrapLast Line
	// Chevron draws ▸ or ▾ in Symbol's place, for a row whose Detail is the point of it.
	Chevron bool
	// Under is the row's first line, drawn on a └─ row while those are on.
	Under Cell
	// Detail is what the side panel shows while the cursor is on the row, and
	// what enter opens under the row. Each cell is a paragraph that wraps.
	Detail []Cell
	// Text is more the filter matches beyond what the row draws.
	Text string
	// Marked draws MarkGlyph in the row's indent and its first cell in MarkCode. No
	// column moves for it.
	Marked bool
	// changed is set while the row wears the change cue.
	changed bool
}

// The mark a Marked row draws.
const (
	MarkGlyph = "◆"
	MarkCode  = BoldMagenta
)

// Group is a header carrying its count, with rows and groups under it.
type Group struct {
	Key   string
	Title Line
	// Count is drawn after the title as " · n", where Counted says to.
	Count   int
	Counted bool
	// Cells draw on the columns past the first, which holds the title, so a group's
	// totals line up with the rows under it.
	Cells []Cell
	// WrapLast is the last column as Row.WrapLast has it.
	WrapLast Line
	// Below are lines drawn under the header whether the group is folded or not. The
	// cursor moves past them as one node.
	Below []Line
	// Summary is drawn past the title while the group is folded.
	Summary Line
	Folded  bool
	// Gap draws a blank line after the group.
	Gap    bool
	Rows   []Row
	Groups []Group
	Detail []Cell
	// changed is set while the header wears the change cue.
	changed bool
}

func (g *Group) holdsRows() bool {
	return len(g.Rows) > 0 || slices.ContainsFunc(g.Groups, func(child Group) bool { return child.holdsRows() })
}

// Frame is one screen of a view, before it is laid out to a size.
type Frame struct {
	Strip   []Line
	Columns []Column
	Groups  []Group
}

// nodeKind is what one laid-out line of the table is.
type nodeKind int

const (
	nodeGroup nodeKind = iota
	nodeRow
	nodeUnder
	nodeDetail
	nodeBlank
)

// node is one line of the table as folds and the filter leave it.
type node struct {
	kind  nodeKind
	key   string
	depth int
	group *Group
	row   *Row
	cell  Cell
	open  bool
}

func (n node) selectable() bool { return n.kind == nodeGroup || n.kind == nodeRow }

func (n node) wearsCue() bool {
	switch n.kind {
	case nodeGroup:
		return n.group.changed
	case nodeRow:
		return n.row.changed
	}
	return false
}

// display is what the folds, the filter and the toggles leave showing.
type display struct {
	folded map[string]bool
	opened map[string]bool
	under  bool
	filter string
}

func (d display) contains(text string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(d.filter))
}

func (d display) matches(r *Row) bool {
	if d.filter == "" {
		return true
	}
	var b strings.Builder
	b.WriteString(r.Span.Text() + " " + r.Under.Text + " " + r.Text)
	for _, c := range r.Cells {
		b.WriteString(" " + c.Text)
	}
	for _, c := range r.Detail {
		b.WriteString(" " + c.Text)
	}
	return d.contains(b.String())
}

func (d display) groupMatches(g *Group) bool {
	if d.contains(g.Title.Text()) || slices.ContainsFunc(g.Below, func(l Line) bool { return d.contains(l.Text()) }) {
		return true
	}
	for i := range g.Rows {
		if d.matches(&g.Rows[i]) {
			return true
		}
	}
	for i := range g.Groups {
		if d.groupMatches(&g.Groups[i]) {
			return true
		}
	}
	return false
}

func (d display) isOpen(g *Group) bool {
	if d.filter != "" {
		return true
	}
	if folded, set := d.folded[g.Key]; set {
		return !folded
	}
	return !g.Folded
}

// anyUnder says whether any row, folded or not, has a first line to show.
func anyUnder(groups []Group) bool {
	for i := range groups {
		hasUnder := func(r Row) bool { return r.Under.Text != "" }
		if slices.ContainsFunc(groups[i].Rows, hasUnder) || anyUnder(groups[i].Groups) {
			return true
		}
	}
	return false
}

func (d display) walk(groups []Group, depth int, out []node) []node {
	for i := range groups {
		g := &groups[i]
		if d.filter != "" && !d.groupMatches(g) {
			continue
		}
		open := d.isOpen(g)
		out = append(out, node{kind: nodeGroup, key: g.Key, depth: depth, group: g, open: open})
		if open {
			for j := range g.Rows {
				r := &g.Rows[j]
				if !d.matches(r) {
					continue
				}
				out = append(out, node{kind: nodeRow, key: r.Key, depth: depth + 1, row: r, open: d.opened[r.Key]})
				if d.under && r.Under.Text != "" {
					out = append(out, node{kind: nodeUnder, depth: depth + 1, cell: r.Under})
				}
				if d.opened[r.Key] {
					for _, c := range r.Detail {
						out = append(out, node{kind: nodeDetail, depth: depth + 2, cell: c})
					}
				}
			}
			out = d.walk(g.Groups, depth+1, out)
		}
		if g.Gap {
			out = append(out, node{kind: nodeBlank})
		}
	}
	return out
}

// indentOf is where a depth's text starts, past the pointer's one column.
func indentOf(depth int) int { return 1 + 2*depth }

// layout is the width each column takes for what shows, within width. The last
// column takes what is left and is never padded.
func layout(columns []Column, nodes []node, width int) []int {
	widths := make([]int, len(columns))
	for i, c := range columns {
		widths[i] = ansi.StringWidth(c.Title)
	}
	for _, n := range nodes {
		switch {
		case n.kind == nodeGroup && len(n.group.Cells) > 0:
			widths[0] = max(widths[0], indentOf(n.depth)+2+groupLabel(n.group).width())
			for i, c := range n.group.Cells {
				if i+1 < len(widths) {
					widths[i+1] = max(widths[i+1], ansi.StringWidth(c.Text))
				}
			}
		case n.kind == nodeGroup && n.depth == 0:
			widths[0] = max(widths[0], indentOf(0)+2+groupLabel(n.group).width())
		case n.kind == nodeRow && len(n.row.Cells) > 0:
			for i, c := range n.row.Cells {
				w := ansi.StringWidth(c.Text)
				if i == 0 {
					w += indentOf(n.depth)
					if symbol := rowSymbol(n); symbol.Text != "" {
						w += ansi.StringWidth(symbol.Text) + 1
					}
				}
				widths[i] = max(widths[i], w)
			}
		}
	}
	for i, c := range columns {
		if c.Max > 0 {
			widths[i] = min(widths[i], c.Max)
		}
	}
	return squeeze(widths, width)
}

// lastColumnFloor is what the last column keeps before the screen's edge cuts
// the rest.
const lastColumnFloor = 24

// squeeze takes columns from the widest until the last keeps its floor.
func squeeze(widths []int, width int) []int {
	last := len(widths) - 1
	if last < 0 {
		return widths
	}
	fixed := func() int {
		sum := 0
		for _, w := range widths[:last] {
			sum += w + len(columnGap)
		}
		return sum
	}
	for fixed()+lastColumnFloor > width {
		widest := 0
		for i, w := range widths[:last] {
			if w > widths[widest] {
				widest = i
			}
		}
		if widths[widest] <= 6 {
			break
		}
		widths[widest]--
	}
	widths[last] = max(width-fixed(), 0)
	return widths
}

func groupLabel(g *Group) Line {
	label := slices.Clone(g.Title)
	if g.Counted {
		code := ""
		if len(g.Title) > 0 {
			code = g.Title[0].Code
		}
		label = append(label, C(" · "+strconv.Itoa(g.Count), code))
	}
	return label
}

// fit is text cut to width with an ellipsis, then padded to it.
func fit(text string, width int, right bool) string {
	text = ansi.Truncate(text, width, "…")
	pad := strings.Repeat(" ", max(width-ansi.StringWidth(text), 0))
	if right {
		return pad + text
	}
	return text + pad
}

// drawn is one laid-out line and the key the cursor lands on there.
type drawn struct {
	text string
	key  string
}

// draw lays the nodes out to width. The first group holding rows and left open
// carries the one header row, so every group shares its columns. A group with cells
// of its own has no room for the titles, so they take a line of their own at the top.
func draw(pal gotermstyle.Palette, columns []Column, nodes []node, width int, cursor string) []drawn {
	widths := layout(columns, nodes, width)
	titled := false
	lines := make([]drawn, 0, len(nodes))
	if slices.ContainsFunc(nodes, func(n node) bool { return n.kind == nodeGroup && len(n.group.Cells) > 0 }) {
		titled = true
		first := pal.Bold(fit(columns[0].Title, max(widths[0]-1, 0), false))
		lines = append(lines, drawn{text: ansi.Truncate(" "+first+columnTitles(pal, columns, widths), width, "…")})
	}
	for _, n := range nodes {
		pointer := " "
		selected := n.selectable() && n.key == cursor
		switch {
		case selected:
			pointer = pal.Bold("›")
		case n.wearsCue():
			pointer = pal.Paint(CueCode, CueGlyph)
		}
		var text string
		var wrapped Line
		switch n.kind {
		case nodeBlank:
			text = ""
		case nodeGroup:
			wantsTitles := n.depth == 0 && n.open && n.group.holdsRows() && !titled
			titled = titled || wantsTitles
			text = pointer + drawGroup(pal, n, columns, widths, wantsTitles, selected)
			wrapped = n.group.WrapLast
		case nodeRow:
			text = pointer + drawRow(pal, n, columns, widths, selected)
			wrapped = n.row.WrapLast
		case nodeUnder:
			text = " " + strings.Repeat(" ", indentOf(n.depth)-1) + "└─ " + pal.Paint(n.cell.Code, n.cell.Text)
		case nodeDetail:
			for _, piece := range wrap(n.cell.Text, max(width-indentOf(n.depth), 20)) {
				lines = append(lines, drawn{text: strings.Repeat(" ", indentOf(n.depth)) + pal.Paint(n.cell.Code, piece)})
			}
			continue
		}
		lines = append(lines, drawn{text: ansi.Truncate(text, width, "…"), key: selectableKey(n)})
		lines = append(lines, continuedLines(pal, wrapped, widths)...)
		if n.kind == nodeGroup {
			lines = append(lines, belowLines(pal, n, width)...)
		}
	}
	return lines
}

func selectableKey(n node) string {
	if n.selectable() {
		return n.key
	}
	return ""
}

// columnTitles is every title past the first column's, on the shared columns.
func columnTitles(pal gotermstyle.Palette, columns []Column, widths []int) string {
	var b strings.Builder
	for i := 1; i < len(columns); i++ {
		title := columns[i].Title
		if i == len(columns)-1 {
			b.WriteString(columnGap + pal.Bold(title))
			break
		}
		b.WriteString(columnGap + pal.Bold(fit(title, widths[i], columns[i].Right)))
	}
	return b.String()
}

// columnCells draws cells on the columns from one on. wrapped takes the last column
// where it is set, and shows there as much of itself as that column holds.
func columnCells(pal gotermstyle.Palette, cells []Cell, from int, wrapped Line, columns []Column, widths []int) string {
	last := len(widths) - 1
	var b strings.Builder
	at := from
	for _, c := range cells {
		if at > last || (at == last && len(wrapped) > 0) {
			break
		}
		if at == last {
			b.WriteString(columnGap + pal.Paint(c.Code, ansi.Truncate(c.Text, widths[at], "…")))
		} else {
			b.WriteString(columnGap + pal.Paint(c.Code, fit(c.Text, widths[at], columns[at].Right)))
		}
		at++
	}
	if len(wrapped) == 0 {
		return b.String()
	}
	for ; at < last; at++ {
		b.WriteString(columnGap + strings.Repeat(" ", widths[at]))
	}
	if pieces := wrapLine(wrapped, widths[last]); len(pieces) > 0 {
		b.WriteString(columnGap + pieces[0].paint(pal))
	}
	return b.String()
}

// continuedLines is what a wrapping last column holds past its first line, each
// starting at that column.
func continuedLines(pal gotermstyle.Palette, wrapped Line, widths []int) []drawn {
	if len(wrapped) == 0 {
		return nil
	}
	last := len(widths) - 1
	start := 0
	for _, w := range widths[:last] {
		start += w + len(columnGap)
	}
	pieces := wrapLine(wrapped, widths[last])
	lines := make([]drawn, 0, len(pieces))
	for _, piece := range pieces[min(1, len(pieces)):] {
		lines = append(lines, drawn{text: strings.Repeat(" ", start) + piece.paint(pal)})
	}
	return lines
}

// wrapLine breaks a line into lines of at most width, each cell keeping its color.
func wrapLine(line Line, width int) []Line {
	if width < 2 {
		return []Line{line}
	}
	var out []Line
	var current Line
	room := width
	for _, c := range line {
		for text := c.Text; text != ""; {
			w := ansi.StringWidth(text)
			if w <= room {
				current = append(current, C(text, c.Code))
				room -= w
				break
			}
			current = append(current, C(ansi.Cut(text, 0, room), c.Code))
			text = ansi.Cut(text, room, w)
			out = append(out, current)
			current, room = nil, width
		}
	}
	if len(current) > 0 {
		out = append(out, current)
	}
	return out
}

// belowLines are the lines a group keeps under its header, at its title's column.
func belowLines(pal gotermstyle.Palette, n node, width int) []drawn {
	lines := make([]drawn, 0, len(n.group.Below))
	indent := strings.Repeat(" ", indentOf(n.depth)+2)
	for _, line := range n.group.Below {
		lines = append(lines, drawn{text: ansi.Truncate(indent+line.paint(pal), width, "…")})
	}
	return lines
}

func drawGroup(pal gotermstyle.Palette, n node, columns []Column, widths []int, titles, selected bool) string {
	fold := "▸"
	if n.open {
		fold = "▾"
	}
	head := strings.Repeat(" ", indentOf(n.depth)-1) + fold + " "
	label := groupLabel(n.group)
	if n.group.changed {
		label = label.emboldened()
	}
	if selected {
		label = Line{C(label.Text(), Reverse)}
	}
	if len(n.group.Cells) > 0 || len(n.group.WrapLast) > 0 {
		room := max(widths[0]-1-ansi.StringWidth(head), 1)
		painted := ansi.Truncate(label.paint(pal), room, "…")
		out := head + painted + strings.Repeat(" ", max(room-ansi.StringWidth(painted), 0))
		return out + columnCells(pal, n.group.Cells, 1, n.group.WrapLast, columns, widths)
	}
	if n.depth > 0 {
		out := head + label.paint(pal)
		if !n.open && len(n.group.Summary) > 0 {
			out += "  " + n.group.Summary.paint(pal)
		}
		return out
	}
	pad := strings.Repeat(" ", max(widths[0]-1-ansi.StringWidth(head)-label.width(), 0))
	out := head + label.paint(pal) + pad
	switch {
	case titles:
		out += columnTitles(pal, columns, widths)
	case !n.open || !n.group.holdsRows():
		if len(n.group.Summary) > 0 {
			out += columnGap + n.group.Summary.paint(pal)
		}
	}
	return out
}

func drawRow(pal gotermstyle.Palette, n node, columns []Column, widths []int, selected bool) string {
	r := n.row
	indent := strings.Repeat(" ", indentOf(n.depth)-1)
	if r.Marked && len(indent) > 0 {
		indent = indent[1:] + pal.Paint(MarkCode, MarkGlyph)
	}
	if len(r.Cells) == 0 {
		span := r.Span
		if r.changed {
			span = span.emboldened()
		}
		if selected {
			span = Line{C(span.Text(), Reverse)}
		}
		return indent + span.paint(pal)
	}
	symbol := ""
	used := indentOf(n.depth)
	if cell := rowSymbol(n); cell.Text != "" {
		symbol = pal.Paint(cell.Code, cell.Text) + " "
		used += ansi.StringWidth(cell.Text) + 1
	}
	first := r.Cells[0]
	text := fit(first.Text, max(widths[0]-used, 1), false)
	code := first.Code
	if r.Marked {
		code = MarkCode
	}
	if r.changed {
		code = Bold + code
	}
	if selected {
		code = Reverse
	}
	return indent + symbol + pal.Paint(code, text) + columnCells(pal, r.Cells[1:], 1, r.WrapLast, columns, widths)
}

// rowSymbol is what a row draws before its first cell: its chevron, or its symbol.
func rowSymbol(n node) Cell {
	switch {
	case !n.row.Chevron:
		return n.row.Symbol
	case n.open:
		return C("▾", "")
	}
	return C("▸", "")
}

// wrap breaks text at spaces to width, so every line can be painted on its own.
func wrap(text string, width int) []string {
	var out []string
	for _, paragraph := range strings.Split(text, "\n") {
		out = append(out, strings.Split(ansi.Wrap(paragraph, width, "/"), "\n")...)
	}
	return out
}

// Bar is a progress bar: done in green, the work under way in yellow, and the rest
// plain.
func Bar(done, underWay, open, cells int) Line {
	filled := barFilled(done, open, cells)
	empty := cells - filled
	way := 0
	if open > 0 && underWay > 0 {
		way = min(max((underWay*empty+open/2)/open, 1), empty)
	}
	return Line{
		C(strings.Repeat("▰", filled), Green),
		C(strings.Repeat("▱", way), Yellow),
		C(strings.Repeat("▱", empty-way), ""),
	}
}

func barFilled(done, open, cells int) int {
	total := done + open
	if done == 0 || total == 0 {
		return 0
	}
	ceiling := cells
	if open > 0 {
		ceiling--
	}
	return min(max((done*cells+total/2)/total, 1), ceiling)
}
