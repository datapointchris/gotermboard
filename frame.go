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
	// item says the cell is one of an Items line's, which the draw may leave out.
	item bool
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

// Items is a line of a Group's Below that the draw cuts to the items that fit, ending in
// +N for the rest, rather than letting the screen's edge cut one in half. The width it
// has moves with the side panel, so only the draw knows it. The filter and the change cue
// read every item, shown or not.
func Items(code string, items ...string) Line {
	line := make(Line, 0, len(items))
	for i, item := range items {
		if i < len(items)-1 {
			item += ", "
		}
		line = append(line, Cell{Text: item, Code: code, item: true})
	}
	return line
}

// elided is an Items line within room. Any other line stays whole, for the edge to cut.
func (l Line) elided(room int) Line {
	if len(l) == 0 || !l[0].item || l.width() <= room {
		return l
	}
	more := func(kept int) Line { return Line{C("+"+strconv.Itoa(len(l)-kept), l[0].Code)} }
	for kept := len(l) - 1; kept > 0; kept-- {
		if l[:kept].width()+more(kept).width() <= room {
			return append(slices.Clone(l[:kept]), more(kept)...)
		}
	}
	return more(0)
}

func (l Line) paint(pal gotermstyle.Palette) string {
	var b strings.Builder
	for _, c := range l {
		b.WriteString(pal.Paint(c.Code, c.Text))
	}
	return b.String()
}

// emboldened is a color in bold. The color leads, because each code opens with a reset
// that would undo a bold set before it.
func emboldened(code string) string { return code + Bold }

// emboldened is the line in bold over each cell's own color.
func (l Line) emboldened() Line {
	out := slices.Clone(l)
	for i := range out {
		out[i].Code = emboldened(out[i].Code)
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
	// Panel is what the side panel shows in Detail's place, each line wrapping at
	// spaces with its cells' own colors.
	Panel []Line
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
	// Under is the group's first line, wrapping on a └─ row under Below while those are on.
	Under Cell
	// Paragraphs wrap under Below whether the group is folded or not, so a view can open
	// a group's text in place. The cursor moves past them too.
	Paragraphs []Cell
	// Summary is drawn past the title while the group is folded.
	Summary Line
	Folded  bool
	// Gap draws a blank line after the group.
	Gap bool
	// Still keeps the header open with no fold mark, and the cursor passes over it. At
	// the top of a frame it stays on screen with the column titles while the table scrolls.
	Still bool
	// Heading says the title only labels what sits under it, so the filter looks past it.
	Heading bool
	// FilterOnly keeps the group out of the table until a filter is typed, so a search
	// reaches past what a view shows.
	FilterOnly bool
	Rows       []Row
	Groups     []Group
	Detail     []Cell
	// Panel is what the side panel shows in Detail's place, as Row.Panel has it.
	Panel []Line
	// Text is more the filter matches beyond what the group draws.
	Text string
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
	// StartAt is the key the cursor starts on, where that key is showing. Without one it
	// starts on the first line holding cells.
	StartAt string
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
	// wraps says an Under line runs on past the screen's edge, where a row's is cut.
	wraps bool
}

func (n node) selectable() bool { return (n.kind == nodeGroup && !n.group.Still) || n.kind == nodeRow }

// staysOnScreen says the node can hold its place above a table that scrolls.
func (n node) staysOnScreen() bool {
	return n.kind == nodeBlank || (n.kind == nodeGroup && n.group.Still && n.depth == 0)
}

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
	own := g.Text + " " + g.Under.Text
	if !g.Heading {
		own += " " + g.Title.Text()
	}
	if d.contains(own) || slices.ContainsFunc(g.Below, func(l Line) bool { return d.contains(l.Text()) }) {
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
	if d.filter != "" || g.Still {
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
		if groups[i].Under.Text != "" || slices.ContainsFunc(groups[i].Rows, hasUnder) || anyUnder(groups[i].Groups) {
			return true
		}
	}
	return false
}

func (d display) walk(groups []Group, depth int, out []node) []node {
	for i := range groups {
		g := &groups[i]
		if g.FilterOnly && d.filter == "" {
			continue
		}
		if d.filter != "" && !d.groupMatches(g) {
			continue
		}
		open := d.isOpen(g)
		out = append(out, node{kind: nodeGroup, key: g.Key, depth: depth, group: g, open: open})
		if d.under && g.Under.Text != "" {
			out = append(out, node{kind: nodeUnder, depth: depth + 1, cell: g.Under, wraps: true})
		}
		for _, c := range g.Paragraphs {
			out = append(out, node{kind: nodeDetail, depth: depth + 2, cell: c})
		}
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

// drawn is one laid-out line and the key the cursor lands on there. pinned says it stays
// on screen above the lines that scroll.
type drawn struct {
	text   string
	key    string
	pinned bool
}

// splitPinned is the leading lines that stay on screen, and those that scroll.
func splitPinned(lines []drawn) (pinned, scrolling []drawn) {
	at := slices.IndexFunc(lines, func(l drawn) bool { return !l.pinned })
	if at < 0 {
		return lines, nil
	}
	return lines[:at], lines[at:]
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
		lines = append(lines, drawn{text: ansi.Truncate(" "+first+columnTitles(pal, columns, widths), width, "…"), pinned: true})
	}
	pinning := true
	for _, n := range nodes {
		pinning = pinning && n.staysOnScreen()
		from := len(lines)
		wantsTitles := n.kind == nodeGroup && n.depth == 0 && n.open && n.group.holdsRows() && !titled
		titled = titled || wantsTitles
		lines = append(lines, drawNode(pal, n, columns, widths, width, cursor, wantsTitles)...)
		for i := from; i < len(lines); i++ {
			lines[i].pinned = pinning
		}
	}
	return lines
}

// drawNode is the lines one node takes.
func drawNode(pal gotermstyle.Palette, n node, columns []Column, widths []int, width int, cursor string, wantsTitles bool) []drawn {
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
		text = pointer + drawGroup(pal, n, columns, widths, wantsTitles, selected)
		wrapped = n.group.WrapLast
	case nodeRow:
		text = pointer + drawRow(pal, n, columns, widths, selected)
		wrapped = n.row.WrapLast
	case nodeUnder:
		return underLines(pal, n, width)
	case nodeDetail:
		var lines []drawn
		for _, piece := range wrap(n.cell.Text, max(width-indentOf(n.depth), 20)) {
			lines = append(lines, drawn{text: strings.Repeat(" ", indentOf(n.depth)) + pal.Paint(n.cell.Code, piece)})
		}
		return lines
	}
	lines := []drawn{{text: ansi.Truncate(text, width, "…"), key: selectableKey(n)}}
	lines = append(lines, continuedLines(pal, wrapped, widths)...)
	if n.kind == nodeGroup {
		lines = append(lines, belowLines(pal, n, width)...)
	}
	return lines
}

// underLines is a first line on its └─ row, running on under it where it wraps.
func underLines(pal gotermstyle.Palette, n node, width int) []drawn {
	lead := strings.Repeat(" ", indentOf(n.depth)) + "└─ "
	if !n.wraps {
		return []drawn{{text: ansi.Truncate(lead+pal.Paint(n.cell.Code, n.cell.Text), width, "…")}}
	}
	room := max(width-ansi.StringWidth(lead), 20)
	var lines []drawn
	for i, piece := range wrap(n.cell.Text, room) {
		if i == 1 {
			lead = strings.Repeat(" ", ansi.StringWidth(lead))
		}
		lines = append(lines, drawn{text: lead + pal.Paint(n.cell.Code, piece)})
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
		lines = append(lines, drawn{text: ansi.Truncate(indent+line.elided(width-len(indent)).paint(pal), width, "…")})
	}
	return lines
}

func drawGroup(pal gotermstyle.Palette, n node, columns []Column, widths []int, titles, selected bool) string {
	fold := "▸"
	switch {
	case n.group.Still:
		fold = " "
	case n.open:
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
		if (!n.open || n.group.Still) && len(n.group.Summary) > 0 {
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
		code = emboldened(code)
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

// wrapWords breaks a line at spaces to width, each cell keeping its color. A word wider
// than width is cut.
func wrapWords(line Line, width int) []Line {
	if width < 2 || line.width() <= width {
		return []Line{line}
	}
	var out []Line
	var current Line
	room := width
	finish := func() {
		current[len(current)-1].Text = strings.TrimRight(current[len(current)-1].Text, " ")
		out = append(out, current)
		current, room = nil, width
	}
	for _, c := range line {
		for _, word := range strings.SplitAfter(c.Text, " ") {
			if word == "" {
				continue
			}
			text := strings.TrimRight(word, " ")
			if ansi.StringWidth(text) > room && len(current) > 0 {
				finish()
			}
			spaces := word[len(text):]
			for w := ansi.StringWidth(text); w > width; w = ansi.StringWidth(text) {
				out = append(out, Line{C(ansi.Cut(text, 0, width), c.Code)})
				text = ansi.Cut(text, width, w)
			}
			current = append(current, C(text+spaces, c.Code))
			room = max(room-ansi.StringWidth(text+spaces), 0)
		}
	}
	if len(current) > 0 {
		finish()
	}
	return out
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
