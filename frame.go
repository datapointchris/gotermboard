package gotermboard

import (
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermstyle"
	"github.com/datapointchris/gotermstyle/tuitheme"
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
	Text string
	Code string
	// Role is the TUI theme role the cell draws in, such as syntax.keyword, with Code's
	// attributes over it. A board, its tabs and a Reader resolve it in their live theme wherever
	// they draw the cell, so a reload recolors it. Line.Paint resolves it in the built-in theme.
	Role  string
	Ticks bool
	// Section says this cell starts the words of a heading, where a Reader's [ and ] land, and
	// how deep the heading sits: 1 is outermost, and 0 is no heading. A heading closes every
	// open one at its depth or deeper, which is the path a Reader's top line draws.
	Section int
	// item says the cell is one of an Items line's, which the draw may leave out.
	item bool
}

// C is a Cell.
func C(text, code string) Cell { return Cell{Text: text, Code: code} }

// Plain says the cell names neither a Code nor a Role, so it draws in the terminal's own text
// color. A caller tinting a Document's uncolored words tests this rather than Code alone.
func (c Cell) Plain() bool { return c.Code == "" && c.Role == "" }

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

// width is the cells the line takes. Cells that count add up, and a line holding any other text is
// measured as one run, because a grapheme can span two cells' texts.
func (l Line) width() int {
	width := 0
	for _, c := range l {
		cells, ok := counted(c.Text)
		if !ok {
			return ansi.StringWidth(l.Text())
		}
		width += cells
	}
	return width
}

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
	more := func(kept int) Line { return Line{l[0].withText("+" + strconv.Itoa(len(l)-kept))} }
	for kept := len(l) - 1; kept > 0; kept-- {
		if l[:kept].width()+more(kept).width() <= room {
			return append(slices.Clone(l[:kept]), more(kept)...)
		}
	}
	return more(0)
}

// Paint is the line as text in its cells' colors, so a TallyBar can print outside a board. A
// cell's Role draws in the built-in theme.
func (l Line) Paint(pal gotermstyle.Palette) string { return l.paint(pal, builtInTheme) }

// paint is the line as text in its cells' colors, each Role drawn in theme.
func (l Line) paint(pal gotermstyle.Palette, theme *tuitheme.Theme) string {
	var b strings.Builder
	for _, c := range l {
		b.WriteString(pal.Paint(c.code(theme), c.Text))
	}
	return b.String()
}

// withText is a piece of the cell. It copies the whole cell, so a piece keeps every field.
func (c Cell) withText(text string) Cell {
	c.Text = text
	return c
}

// code is what the cell draws in: its Role in theme, then its Code. The role's code leads,
// because it opens with a reset that would undo an attribute set before it.
func (c Cell) code(theme *tuitheme.Theme) string {
	if c.Role == "" {
		return c.Code
	}
	return theme.Code(c.Role) + c.Code
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
	// Min holds the column at least this wide, for what a group's Beside fits into.
	Min   int
	Max   int
	Right bool
	// gap is the spaces the layout leaves ahead of the column, where Look.Spread deals it more
	// than columnGap's two.
	gap int
}

// spacer is the blank ahead of the column.
func (c Column) spacer() string {
	if c.gap == 0 {
		return columnGap
	}
	return strings.Repeat(" ", c.gap)
}

// start is where column at starts, past the columns ahead of it and their gaps.
func start(columns []Column, widths []int, at int) int {
	sum := 0
	for i := range at {
		sum += widths[i] + len(columns[i+1].spacer())
	}
	return sum
}

// Document is text that lays itself out at a width, for what only the draw can place:
// hanging bullets, tables and boxes of code. Its lines are drawn as they come.
type Document func(width int) []Line

// Row is one line of the table: Cells on the columns, or Span across them.
type Row struct {
	Key    string
	Symbol Cell
	Cells  []Cell
	Span   Line
	// WrapLast is the last column as cells of their own colors. It continues on
	// further lines at that column, where a cell in Cells is cut at the screen's edge.
	WrapLast Line
	// Under is the row's summary, drawn on a └─ row while those are on.
	Under Cell
	// Second is a line the row draws under its first, always. Its first cell sits in the first
	// column under the row's first cell, and the rest draw one after another from the second
	// column, cut at the table's edge. The cursor's row wears its look across both lines. A
	// row's Under draws after it.
	Second Line
	// UnderWhereCut draws Under only where the last column is too narrow for the row's cell
	// there, for a summary that repeats that cell. Only the draw knows the column's width.
	UnderWhereCut bool
	// Detail is what the side panel shows while the cursor is on the row, and what opens
	// under the row. Each cell is a paragraph that wraps. A row holding Detail or a
	// Document draws a chevron in its indent.
	Detail []Cell
	// Document is what opens under the row and the side panel shows, in Detail's place.
	Document Document
	// ReaderTitle heads the Reader enter opens on a tall Document, and the side panel where
	// it draws the Document. Empty, the row's own words head the Reader.
	ReaderTitle string
	// StartsOpen shows the row's Detail or Document under it until a key closes it, so a
	// view can draw its newest entry whole and still fold it to its first line. A row whose
	// Document the side panel draws starts folded while the panel shows, since the panel
	// draws it whole beside the table.
	StartsOpen bool
	// Panel is what the side panel shows in place of both, each line wrapping at spaces
	// with its cells' own colors. Without one, the panel draws the Document.
	Panel Document
	// Text is more the filter matches beyond what the row draws.
	Text string
	// Marked draws MarkGlyph in the row's indent and its first cell in MarkCode. No
	// column moves for it.
	Marked bool
	// Entities is what the row is about, which JumpKey lists and a jump's cursor looks for.
	Entities []EntityRef
	// changed is set while the row wears the change cue.
	changed bool
}

// opens says the row holds text the toggle shows under it.
func (r *Row) opens() bool { return len(r.Detail) > 0 || r.Document != nil }

// lead is what the row takes of the first column ahead of its first cell: the indent at its
// depth, and its Symbol and a space.
func (r *Row) lead(depth int) int {
	lead := indentOf(depth)
	if r.Symbol.Text != "" {
		lead += textWidth(r.Symbol.Text) + 1
	}
	return lead
}

// cellRoom is the width the draw gives the row's cell in column at.
func (r *Row) cellRoom(depth, at int, widths []int) int {
	if at > 0 {
		return widths[at]
	}
	return max(widths[0]-r.lead(depth), 1)
}

// showsUnder says the row at depth has a summary to draw at these column widths.
func (r *Row) showsUnder(depth int, widths []int) bool {
	if r.Under.Text == "" {
		return false
	}
	if !r.UnderWhereCut {
		return true
	}
	last := len(widths) - 1
	return last >= 0 && len(r.WrapLast) == 0 && len(r.Cells) > last &&
		textWidth(r.Cells[last].Text) > r.cellRoom(depth, last, widths)
}

// panelSource is the row's own text for the side panel: its Panel or else its Document, and
// the Detail that shows where it holds neither.
func (r *Row) panelSource() (Document, []Cell) {
	if r.Panel != nil {
		return r.Panel, r.Detail
	}
	return r.Document, r.Detail
}

// panelDrawsDocument says the side panel draws the row's own Document, which a Panel would
// replace.
func (r *Row) panelDrawsDocument() bool { return r.Panel == nil && r.Document != nil }

// hasPanelText says the side panel has something of the row's own to show.
func (r *Row) hasPanelText() bool {
	authored, cells := r.panelSource()
	return authored != nil || len(cells) > 0
}

// The mark a Marked row draws.
const (
	MarkGlyph = "◆"
	MarkCode  = BoldMagenta
)

// The mark a row naming what the tabs are locked to draws in MarkGlyph's cell, in its place.
const (
	LockGlyph = "●"
	LockCode  = BoldCyan
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
	// Span draws whole past the title on the header's line, wrapping under itself, so a
	// long value never sets the first column's width. The cells then take a line of their
	// own beneath it, and Beside sits ahead of them in the first column.
	Span Line
	// Beside is what the first column holds on a Span group's line of cells, under the
	// Span's start. An Items line there is cut to fit.
	Beside Line
	// Under is the group's summary, wrapping on a └─ row under Below while those are on.
	Under Cell
	// Document draws under Below whether the group is folded or not, so a view can open
	// a group's text in place. The cursor moves past it too.
	Document Document
	// Summary is drawn past the title while the group is folded.
	Summary Line
	Folded  bool
	// Gap draws a blank line after the group.
	Gap bool
	// Rule draws a thin line after the group, in place of Gap's blank one.
	Rule bool
	// BlankAboveRule draws a blank line between the group's last line and its Rule.
	BlankAboveRule bool
	// BetweenRows draws a blank line and then a rule of this weight between each two of the
	// group's rows, so rows of more than one line read as blocks.
	BetweenRows RuleWeight
	// Tone is the code the group's overline and underline draw in, where not the rule gray.
	Tone string
	// Overline draws a line across the table above the header.
	Overline RuleWeight
	// Underline draws a line across the table under the header, ahead of Below.
	Underline RuleWeight
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
	Panel Document
	// Text is more the filter matches beyond what the group draws.
	Text string
	// Entities is what the group is about, as Row.Entities has it.
	Entities []EntityRef
	// changed is set while the header wears the change cue.
	changed bool
}

// panelSource is the group's own text for the side panel. The label and summary the panel
// falls back on repeat what the table already draws, so they are not part of it.
func (g *Group) panelSource() (Document, []Cell) { return g.Panel, g.Detail }

// hasPanelText says the side panel has something of the group's own to show.
func (g *Group) hasPanelText() bool {
	authored, cells := g.panelSource()
	return authored != nil || len(cells) > 0
}

func (g *Group) cursorLands() bool { return !g.Still }

// anyPanelText says a row, or a group the cursor can land on, anywhere in the frame, shown or
// not, has text of its own for the side panel.
func anyPanelText(groups []Group) bool {
	for i := range groups {
		g := &groups[i]
		rowSays := func(r Row) bool { return r.hasPanelText() }
		if (g.cursorLands() && g.hasPanelText()) || slices.ContainsFunc(g.Rows, rowSays) || anyPanelText(g.Groups) {
			return true
		}
	}
	return false
}

// Frame is one screen of a view, before it is laid out to a size.
type Frame struct {
	Strip []Line
	// Charts is drawn under the strip at the screen's width, for what has to be laid out at the
	// width it gets, as a chart is. It is handed the screen's height too, so a board can keep it to
	// a share of a short screen. A moving strip leaves it as the frame drew it.
	Charts  func(width, height int) []Line
	Columns []Column
	Groups  []Group
	// StartAt is the key the cursor starts on, where that key is showing. Without one it
	// starts on the first line holding cells, or the last on a TailBoard, past any group with
	// no panel text. With no such line it starts on the first line with text to read.
	StartAt string
}

// nodeKind is what one laid-out line of the table is.
type nodeKind int

const (
	nodeGroup nodeKind = iota
	nodeRow
	nodeUnder
	nodeDetail
	nodeDocument
	nodeBlank
	nodeRule
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
	// toggles says folding or opening the node changes what shows, so it draws a chevron, unless
	// unmarked says the board's Look draws none beside a row.
	toggles  bool
	unmarked bool
	// drawsRows says the table draws a row somewhere under a group, so the group's Summary
	// gives way to the rows and the column titles are of use on its header.
	drawsRows bool
	// wraps says an Under line runs on past the screen's edge, where a row's is cut.
	wraps bool
	// document is what a nodeDocument draws.
	document Document
	// weight is how heavy a nodeRule draws.
	weight RuleWeight
	// column is where an Under line's text or a document starts, where that is not its depth's.
	column int
	// parent is the index of the node this one sits inside, and -1 at the top. A node's Under
	// line and its rule or gap are its own lines, so they sit where it sits.
	parent int
	// pinned says the node and every one before it stay on screen above the table that scrolls.
	pinned bool
	// names says the node scrolls and has words to name what sits inside it, so the sticky band
	// and a reader's path both name it.
	names bool
	// locked says the row names what the tabs are locked to, so it wears LockGlyph.
	locked bool
}

func (n node) selectable() bool {
	return (n.kind == nodeGroup && n.group.cursorLands()) || n.kind == nodeRow
}

// staysOnScreen says the node can hold its place above a table that scrolls.
func (n node) staysOnScreen() bool {
	return n.kind == nodeBlank || (n.kind == nodeGroup && n.group.Still && n.depth == 0)
}

// foldMark is what a node's chevron cell holds: blank where the toggle would change nothing.
func (n node) foldMark() string {
	switch {
	case !n.toggles || n.unmarked:
		return " "
	case n.open:
		return "▾"
	}
	return "▸"
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
	// lock is what the tabs are locked to, which Tabs and each of its tabs share, and nil
	// outside Tabs.
	lock *tabLock
	// wholePage says this is the entity page, which shows one thing whole, so a lock marks the
	// lines naming what is locked and hides none.
	wholePage bool
	// matched keeps what the filter reads of each row, because one draw walks the rows several
	// times, the clock draws a frame a second, and a row's text can run to pages.
	matched *rowMatches
	// panelShown says the side panel is drawn beside the table.
	panelShown bool
	// toggles counts every write to folded and opened, so a kept layout knows it is stale without
	// comparing the maps, which grow to a key a row once every row has been opened.
	toggles uint64
}

// fold and open are the only writers of folded and opened.
func (d *display) fold(key string, folded bool) {
	d.folded[key] = folded
	d.toggles++
}

func (d *display) open(key string, opened bool) {
	d.opened[key] = opened
	d.toggles++
}

// rowMatches is each row's answer to the filter and its text lowercased. Answers and the rows
// they belong to are this frame's. The lowercased text is keyed by the text, so a new frame
// whose rows say the same thing lowercases nothing, and it is kept for two frames.
type rowMatches struct {
	filter     string
	answers    map[*Row]bool
	lowered    map[*Row]string
	kept, last map[string]string
}

func newRowMatches() *rowMatches {
	return &rowMatches{answers: map[*Row]bool{}, lowered: map[*Row]string{}, kept: map[string]string{}}
}

func (rm *rowMatches) newFrame() {
	if rm == nil {
		return
	}
	clear(rm.answers)
	clear(rm.lowered)
	rm.last, rm.kept = rm.kept, map[string]string{}
}

func (rm *rowMatches) lowercase(r *Row) string {
	if low, known := rm.lowered[r]; known {
		return low
	}
	text := rowText(r)
	low, known := rm.kept[text]
	if !known {
		if low, known = rm.last[text]; !known {
			low = strings.ToLower(text)
		}
		rm.kept[text] = low
	}
	rm.lowered[r] = low
	return low
}

func (d display) contains(text string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(d.filter))
}

func (d display) matches(r *Row) bool {
	if d.filter == "" {
		return true
	}
	if d.matched == nil {
		return d.contains(rowText(r))
	}
	if d.matched.filter != d.filter {
		d.matched.filter = d.filter
		clear(d.matched.answers)
	}
	got, known := d.matched.answers[r]
	if !known {
		got = strings.Contains(d.matched.lowercase(r), strings.ToLower(d.filter))
		d.matched.answers[r] = got
	}
	return got
}

// rowText is everything of a row the filter reads.
func rowText(r *Row) string {
	var b strings.Builder
	b.WriteString(r.Span.Text() + " " + r.Under.Text + " " + r.Text)
	if len(r.Second) > 0 {
		b.WriteString(" " + r.Second.Text())
	}
	for _, c := range r.Cells {
		b.WriteString(" " + c.Text)
	}
	for _, c := range r.Detail {
		b.WriteString(" " + c.Text)
	}
	return b.String()
}

func (d display) groupMatches(g *Group) bool {
	own := g.Text + " " + g.Under.Text + " " + g.Span.Text() + " " + g.Beside.Text()
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

// heldOpen says nothing can fold the group: it is Still, or a filter or a lock holds every
// group open.
func (d display) heldOpen(g *Group) bool { return d.filter != "" || d.narrowsTo() != nil || g.Still }

// namesLocked says the group itself names what the tabs are locked to, which keeps every line
// under it.
func (d display) namesLocked(g *Group) bool {
	locked := d.narrowsTo()
	return locked != nil && slices.Contains(g.Entities, *locked)
}

// keepsRow says the lock leaves the row: nothing is locked, a group above names it, or the row
// names it itself.
func (d display) keepsRow(r *Row, held bool) bool {
	locked := d.narrowsTo()
	return locked == nil || held || slices.Contains(r.Entities, *locked)
}

// lockedTo is what the tabs are locked to, and nil with no lock.
func (d display) lockedTo() *EntityRef {
	if d.lock == nil {
		return nil
	}
	return d.lock.ref
}

// narrowsTo is the lock this table keeps only the lines of, which the entity page never has.
func (d display) narrowsTo() *EntityRef {
	if d.wholePage {
		return nil
	}
	return d.lockedTo()
}

func (d display) isOpen(g *Group) bool {
	if d.heldOpen(g) {
		return true
	}
	if folded, set := d.folded[g.Key]; set {
		return !folded
	}
	return !g.Folded
}

// shows says the table draws the group at all.
func (d display) shows(g *Group) bool { return d.showsUnder(g, false) }

// showsUnder is shows for a group whose parent the lock may already keep whole. A lock keeps a
// group that holds a line naming what is locked, as a filter keeps one holding a match.
func (d display) showsUnder(g *Group, held bool) bool {
	locked := d.narrowsTo()
	if locked != nil && !held {
		if _, _, holds := entityLine([]Group{*g}, *locked, nil); !holds {
			return false
		}
	}
	if d.filter == "" {
		return !g.FilterOnly || locked != nil
	}
	return d.groupMatches(g)
}

// folds says folding the group changes what shows: it holds a row or a group the table
// draws, and nothing holds it open.
func (d display) folds(g *Group) bool {
	if d.heldOpen(g) {
		return false
	}
	return len(g.Rows) > 0 || slices.ContainsFunc(g.Groups, func(child Group) bool { return d.shows(&child) })
}

// drawsRows says a row sits somewhere under the group, counting only the groups the table
// draws. An empty group under it folds and still draws no row, so this is not folds.
func (d display) drawsRows(g *Group) bool {
	return len(g.Rows) > 0 || slices.ContainsFunc(g.Groups, func(child Group) bool { return d.shows(&child) && d.drawsRows(&child) })
}

func (d display) isRowOpen(r *Row) bool {
	if !r.opens() {
		return false
	}
	if opened, set := d.opened[r.Key]; set {
		return opened
	}
	return r.StartsOpen && (!d.panelShown || !r.panelDrawsDocument())
}

// anyUnder says whether any group or row, folded or not, has a summary to draw at these column
// widths. A folded row has no node, so this reads the groups rather than the nodes.
func anyUnder(groups []Group, widths []int, depth int) bool {
	for i := range groups {
		hasUnder := func(r Row) bool { return r.showsUnder(depth+1, widths) }
		if groups[i].Under.Text != "" || slices.ContainsFunc(groups[i].Rows, hasUnder) || anyUnder(groups[i].Groups, widths, depth+1) {
			return true
		}
	}
	return false
}

// nodes is what shows of groups on a table this wide, the columns with the gaps the layout
// leaves ahead of them, and the width each column takes for it. Each node is marked with
// whether it stays on screen and whether it names what sits inside it.
func (d display) nodes(groups []Group, columns []Column, width int, look Look, hint int) ([]node, []Column, []int) {
	nodes := d.walk(groups, 0, -1, false, make([]node, 0, hint))
	columns, widths := layout(columns, nodes, width, look)
	nodes = withoutHiddenUnders(nodes, widths)
	pinned := true
	for i := range nodes {
		n := &nodes[i]
		pinned = pinned && n.staysOnScreen()
		n.pinned = pinned
		n.names = !pinned && n.holdsNamedLines()
		n.unmarked = n.kind == nodeRow && look.NoRowChevrons
	}
	return nodes, columns, widths
}

// withoutHiddenUnders drops each row's Under line that has no summary to draw at these widths.
// No node sits inside an Under line, so a parent never drops, and the nodes after one move up.
func withoutHiddenUnders(nodes []node, widths []int) []node {
	hidden := func(n node) bool { return n.kind == nodeUnder && n.row != nil && !n.row.showsUnder(n.depth, widths) }
	if !slices.ContainsFunc(nodes, hidden) {
		return nodes
	}
	kept := make([]node, 0, len(nodes))
	at := make([]int, len(nodes))
	for i, n := range nodes {
		at[i] = len(kept)
		if n.kind == nodeUnder && n.row != nil && !n.row.showsUnder(n.depth, widths) {
			continue
		}
		if n.parent >= 0 {
			n.parent = at[n.parent]
		}
		kept = append(kept, n)
	}
	return kept
}

// holdsNamedLines says lines can sit inside the node and it has words to name them by. A group
// with none would spend a line of the band on a chevron.
func (n node) holdsNamedLines() bool {
	switch n.kind {
	case nodeGroup:
		return len(groupWords(n.group)) > 0
	case nodeRow:
		return n.open
	}
	return false
}

// walk is the nodes of groups. held says a group above names what is locked, so every line
// under it stays.
func (d display) walk(groups []Group, depth, parent int, held bool, out []node) []node {
	for i := range groups {
		g := &groups[i]
		if !d.showsUnder(g, held) {
			continue
		}
		heldHere := held || d.namesLocked(g)
		open := d.isOpen(g)
		self := len(out)
		out = append(out, node{kind: nodeGroup, key: g.Key, depth: depth, group: g, open: open, toggles: d.folds(g), drawsRows: d.drawsRows(g), parent: parent})
		if d.under && g.Under.Text != "" {
			out = append(out, node{kind: nodeUnder, depth: depth + 1, cell: g.Under, wraps: true, column: spanColumn(depth, g), parent: parent})
		}
		if g.Document != nil {
			column := spanColumn(depth, g)
			if column == 0 {
				column = indentOf(depth + 2)
			}
			out = append(out, node{kind: nodeDocument, depth: depth + 2, document: g.Document, column: column, parent: self})
		}
		if open {
			drawnRows := 0
			for j := range g.Rows {
				r := &g.Rows[j]
				if !d.matches(r) || !d.keepsRow(r, heldHere) {
					continue
				}
				if g.BetweenRows != RuleNone && drawnRows > 0 {
					out = append(out, node{kind: nodeBlank, parent: self}, node{kind: nodeRule, weight: g.BetweenRows, parent: self})
				}
				drawnRows++
				rowOpen := d.isRowOpen(r)
				row := len(out)
				out = append(out, node{
					kind: nodeRow, key: r.Key, depth: depth + 1, row: r, open: rowOpen, toggles: r.opens(), parent: self,
					locked: d.lockedTo() != nil && slices.Contains(r.Entities, *d.lockedTo()),
				})
				if d.under && r.Under.Text != "" {
					out = append(out, node{kind: nodeUnder, depth: depth + 1, row: r, cell: r.Under, parent: self})
				}
				if rowOpen {
					if r.Document != nil {
						out = append(out, node{kind: nodeDocument, depth: depth + 2, document: r.Document, column: indentOf(depth + 2), parent: row})
						continue
					}
					for _, c := range r.Detail {
						out = append(out, node{kind: nodeDetail, depth: depth + 2, cell: c, parent: row})
					}
				}
			}
			out = d.walk(g.Groups, depth+1, self, heldHere, out)
		}
		switch {
		case g.Rule:
			if g.BlankAboveRule {
				out = append(out, node{kind: nodeBlank, parent: parent})
			}
			out = append(out, node{kind: nodeRule, depth: depth, weight: RuleThin, parent: parent})
		case g.Gap:
			out = append(out, node{kind: nodeBlank, parent: parent})
		}
	}
	return out
}

// indentOf is where a depth's text starts, past the pointer's one column.
func indentOf(depth int) int { return 1 + 2*depth }

// spanColumn is where a Span group's Span starts, and zero for a group with none.
func spanColumn(depth int, g *Group) int {
	if len(g.Span) == 0 {
		return 0
	}
	return indentOf(depth) + 2 + groupLabel(g).width() + len(columnGap)
}

// layout is the width each column takes for what shows, within width, and the columns with
// the gaps ahead of them. The last column takes what is left and is never padded, unless
// look spreads the columns and no row or group wraps its last column under itself.
func layout(columns []Column, nodes []node, width int, look Look) ([]Column, []int) {
	widths := make([]int, len(columns))
	for i, c := range columns {
		widths[i] = textWidth(c.Title)
	}
	for _, n := range nodes {
		switch {
		case n.kind == nodeGroup && len(n.group.Cells) > 0:
			widths[0] = max(widths[0], indentOf(n.depth)+2+groupLabel(n.group).width())
			for i, c := range n.group.Cells {
				if i+1 < len(widths) {
					widths[i+1] = max(widths[i+1], textWidth(c.Text))
				}
			}
		// A top group's label fills the first column, so the titles or a folded group's
		// summary after it start on the second.
		case n.kind == nodeGroup && n.depth == 0:
			widths[0] = max(widths[0], indentOf(0)+2+groupLabel(n.group).width())
		case n.kind == nodeRow && len(n.row.Cells) > 0:
			for i, c := range n.row.Cells {
				w := textWidth(c.Text)
				if i == 0 {
					w += n.row.lead(n.depth)
				}
				widths[i] = max(widths[i], w)
			}
			if len(n.row.Second) > 0 {
				widths[0] = max(widths[0], n.row.lead(n.depth)+textWidth(n.row.Second[0].Text))
			}
		}
	}
	for i, c := range columns {
		widths[i] = max(widths[i], c.Min)
		if c.Max > 0 {
			widths[i] = min(widths[i], c.Max)
		}
	}
	wrapsLast := slices.ContainsFunc(nodes, func(n node) bool {
		return n.kind == nodeRow && len(n.row.WrapLast) > 0 || n.kind == nodeGroup && len(n.group.WrapLast) > 0
	})
	if look.Spread && !wrapsLast {
		return spread(columns, widths, width)
	}
	return columns, squeeze(widths, width)
}

// spread fills width with the columns at their own widths, dealing what they leave out
// between them a column at a time from the left, so the table ends on its edge rather than
// its last column taking the slack. A column with no title continues the one before it, so
// it keeps columnGap and takes none of the slack. Columns wider than width give way through
// narrowWidest, and the last is cut to what is left.
func spread(columns []Column, widths []int, width int) ([]Column, []int) {
	last := len(widths) - 1
	if last < 1 {
		return columns, squeeze(widths, width)
	}
	used := func() int {
		sum := len(columnGap) * last
		for _, w := range widths {
			sum += w
		}
		return sum
	}
	narrowWidest(widths, len(widths), func() bool { return used() > width })
	slack := width - used()
	if slack <= 0 {
		widths[last] = max(widths[last]+slack, 0)
		return columns, widths
	}
	var titled []int
	for i := 1; i <= last; i++ {
		if columns[i].Title != "" {
			titled = append(titled, i)
		}
	}
	if len(titled) == 0 {
		return columns, widths
	}
	columns = slices.Clone(columns)
	each, extra := slack/len(titled), slack%len(titled)
	for k, i := range titled {
		columns[i].gap = len(columnGap) + each
		if k < extra {
			columns[i].gap++
		}
	}
	return columns, widths
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
	narrowWidest(widths, last, func() bool { return fixed()+lastColumnFloor > width })
	widths[last] = max(width-fixed(), 0)
	return widths
}

// narrowestColumn is as far as narrowWidest takes a column.
const narrowestColumn = 6

// narrowWidest takes a column at a time from the widest of widths[:among] while over holds,
// stopping once the widest is down to narrowestColumn.
func narrowWidest(widths []int, among int, over func() bool) {
	for over() {
		widest := 0
		for i, w := range widths[:among] {
			if w > widths[widest] {
				widest = i
			}
		}
		if widths[widest] <= narrowestColumn {
			return
		}
		widths[widest]--
	}
}

// groupLabel is the group's title, then its count in the color of the title's first cell.
func groupLabel(g *Group) Line {
	label := slices.Clone(g.Title)
	if g.Counted {
		count := Cell{Text: " · " + strconv.Itoa(g.Count)}
		if len(g.Title) > 0 {
			count.Code, count.Role = g.Title[0].Code, g.Title[0].Role
		}
		label = append(label, count)
	}
	return label
}

// groupWords is the group's label and its Span, for a path to name it by.
func groupWords(g *Group) Line {
	words := groupLabel(g)
	if len(g.Span) > 0 {
		words = append(append(words, C(columnGap, "")), g.Span...)
	}
	return words
}

// fit is text cut to width with an ellipsis, then padded to it.
func fit(text string, width int, right bool) string {
	at := textWidth(text)
	if at > width {
		text = ansi.Truncate(text, width, "…")
		at = textWidth(text)
	}
	pad := strings.Repeat(" ", max(width-at, 0))
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
	// whole is a Span group's first line with nothing wrapped away, for the band.
	whole string
	// owners are the first lines of what this line sits inside, outermost first. A node's own
	// lines sit in its parent, so a node holding nothing under it never joins a band.
	owners []string
	// continues says the line is a row's Second, which the cursor keeps in view with the row.
	continues bool
}

// The band holds at most this many lines, and at most this share of what scrolls.
const (
	bandMost     = 5
	bandPerLines = 3
)

// firstLine is the line a node shows in the band, past a group's overline.
func firstLine(lines []drawn, n node) string {
	line := lines[0]
	if n.kind == nodeGroup && n.group.Overline != RuleNone {
		line = lines[1]
	}
	if line.whole != "" {
		return line.whole
	}
	return line.text
}

// drawNode is the lines one node takes.
func drawNode(p painter, n node, columns []Column, widths []int, width int, cursor string, look cursorLook, wantsTitles bool) []drawn {
	lines := drawNodeBody(p, n, columns, widths, width, cursor, look, wantsTitles)
	if n.kind == nodeGroup && n.group.Overline != RuleNone {
		lines = append([]drawn{{text: n.group.Overline.line(p, width, n.group.Tone)}}, lines...)
	}
	return lines
}

// RuleWeight is how heavy a line across the table is drawn.
type RuleWeight int

// The weights a rule takes. RuleDotted is the lightest, for between rows.
const (
	RuleNone RuleWeight = iota
	RuleThin
	RuleDouble
	RuleDotted
)

// line is the rule a group draws, one column short of width.
func (w RuleWeight) line(p painter, width int, tone string) string {
	return w.across(p, width-1, tone)
}

func (w RuleWeight) across(p painter, n int, tone string) string {
	if tone == "" {
		tone = RuleCode
	}
	glyph := "─"
	switch w {
	case RuleDouble:
		glyph = "═"
	case RuleDotted:
		glyph = "┈"
	}
	return p.cell(C(strings.Repeat(glyph, max(n, 0)), tone))
}

func drawNodeBody(p painter, n node, columns []Column, widths []int, width int, cursor string, look cursorLook, wantsTitles bool) []drawn {
	pointer := " "
	selected := n.selectable() && n.key == cursor
	wear := func(lines []drawn) []drawn {
		if selected {
			return look.wear(lines, width)
		}
		return lines
	}
	switch {
	case selected:
		pointer = look.pointer()
	case n.wearsCue():
		pointer = p.cell(C(CueGlyph, CueCode))
	}
	var text string
	var wrapped Line
	switch n.kind {
	case nodeBlank:
		text = ""
	case nodeRule:
		return []drawn{{text: n.weight.line(p, width, "")}}
	case nodeGroup:
		if len(n.group.Span) > 0 {
			return append(wear(spanLines(p, n, pointer, columns, widths, width)), belowLines(p, n, width)...)
		}
		text = pointer + drawGroup(p, n, columns, widths, wantsTitles)
		wrapped = n.group.WrapLast
	case nodeRow:
		text = pointer + drawRow(p, n, columns, widths)
		wrapped = n.row.WrapLast
	case nodeUnder:
		return underLines(p, n, width)
	case nodeDetail:
		var lines []drawn
		for _, piece := range wrap(n.cell.Text, detailRoom(n, width)) {
			lines = append(lines, drawn{text: strings.Repeat(" ", indentOf(n.depth)) + p.cell(n.cell.withText(piece))})
		}
		return lines
	case nodeDocument:
		var lines []drawn
		for _, line := range n.document(documentRoom(n, width)) {
			lines = append(lines, drawn{text: documentLine(p, n, line, width)})
		}
		return lines
	}
	lines := []drawn{{text: cut(text, width), key: selectableKey(n)}}
	lines = append(lines, continuedLines(p, wrapped, columns, widths)...)
	if n.kind == nodeRow && len(n.row.Second) > 0 {
		for i := 1; i < len(lines); i++ {
			lines[i].continues = true
		}
		second := " "
		if selected {
			second = look.pointer()
		}
		lines = append(lines, drawn{text: cut(second+secondLine(p, n, columns, widths), width), continues: true})
	}
	lines = wear(lines)
	if n.kind == nodeGroup {
		lines = append(lines, belowLines(p, n, width)...)
	}
	return lines
}

// secondLine is a row's Second past the pointer's column: its first cell where the row's first
// sits, then the rest from the second column's start.
func secondLine(p painter, n node, columns []Column, widths []int) string {
	second := n.row.Second
	lead := strings.Repeat(" ", n.row.lead(n.depth)-1)
	text := lead + p.cell(second[0].withText(fit(second[0].Text, n.row.cellRoom(n.depth, 0, widths), false)))
	if len(second) == 1 {
		return text
	}
	spacer := columnGap
	if len(columns) > 1 {
		spacer = columns[1].spacer()
	}
	return text + spacer + p.line(second[1:])
}

// underLines is a summary on its └─ row, running on under it where it wraps.
func underLines(p painter, n node, width int) []drawn {
	lead := strings.Repeat(" ", indentOf(n.depth)) + "└─ "
	if n.column >= ansi.StringWidth(lead)-indentOf(n.depth) {
		lead = strings.Repeat(" ", n.column-ansi.StringWidth("└─ ")) + "└─ "
	}
	if !n.wraps {
		return []drawn{{text: cut(lead+p.cell(n.cell), width)}}
	}
	room := underRoom(n, width)
	var lines []drawn
	for i, piece := range wrap(n.cell.Text, room) {
		if i == 1 {
			lead = strings.Repeat(" ", ansi.StringWidth(lead))
		}
		lines = append(lines, drawn{text: lead + p.cell(n.cell.withText(piece))})
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
func columnTitles(p painter, columns []Column, widths []int) string {
	var b strings.Builder
	for i := 1; i < len(columns); i++ {
		title := columns[i].Title
		if i == len(columns)-1 {
			b.WriteString(columns[i].spacer() + p.bold(title))
			break
		}
		b.WriteString(columns[i].spacer() + p.bold(fit(title, widths[i], columns[i].Right)))
	}
	return b.String()
}

// columnCells draws cells on the columns from one on. wrapped takes the last column
// where it is set, and shows there as much of itself as that column holds.
func columnCells(p painter, cells []Cell, from int, wrapped Line, columns []Column, widths []int) string {
	last := len(widths) - 1
	var b strings.Builder
	at := from
	for _, c := range cells {
		if at > last || (at == last && len(wrapped) > 0) {
			break
		}
		if at == last {
			b.WriteString(columns[at].spacer() + p.cell(c.withText(cut(c.Text, widths[at]))))
		} else {
			b.WriteString(columns[at].spacer() + p.cell(c.withText(fit(c.Text, widths[at], columns[at].Right))))
		}
		at++
	}
	if len(wrapped) == 0 {
		return b.String()
	}
	for ; at < last; at++ {
		b.WriteString(columns[at].spacer() + strings.Repeat(" ", widths[at]))
	}
	if pieces := wrapLine(wrapped, widths[last]); len(pieces) > 0 {
		b.WriteString(columns[last].spacer() + p.line(pieces[0]))
	}
	return b.String()
}

// continuedLines is what a wrapping last column holds past its first line, each
// starting at that column.
func continuedLines(p painter, wrapped Line, columns []Column, widths []int) []drawn {
	if len(wrapped) == 0 {
		return nil
	}
	last := len(widths) - 1
	indent := strings.Repeat(" ", start(columns, widths, last))
	pieces := wrapLine(wrapped, widths[last])
	lines := make([]drawn, 0, len(pieces))
	for _, piece := range pieces[min(1, len(pieces)):] {
		lines = append(lines, drawn{text: indent + p.line(piece)})
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
				current = append(current, c.withText(text))
				room -= w
				break
			}
			current = append(current, c.withText(ansi.Cut(text, 0, room)))
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
func belowLines(p painter, n node, width int) []drawn {
	lines := make([]drawn, 0, len(n.group.Below)+1)
	if n.group.Underline != RuleNone {
		lines = append(lines, drawn{text: n.group.Underline.line(p, width, n.group.Tone)})
	}
	indent := strings.Repeat(" ", indentOf(n.depth)+2)
	for _, line := range n.group.Below {
		lines = append(lines, drawn{text: ansi.Truncate(indent+p.line(line.elided(width-len(indent))), width, "…")})
	}
	return lines
}

// groupHead is a group's indent and fold mark, then its label as this frame wears it.
func groupHead(n node) (string, Line) {
	label := groupLabel(n.group)
	if n.group.changed {
		label = label.emboldened()
	}
	return strings.Repeat(" ", indentOf(n.depth)-1) + n.foldMark() + " ", label
}

// spanLines is a Span group: its label and Span, wrapping under the Span's start, then
// Beside and the cells on their own line.
func spanLines(p painter, n node, pointer string, columns []Column, widths []int, width int) []drawn {
	head, label := groupHead(n)
	column := spanColumn(n.depth, n.group)
	indent := strings.Repeat(" ", column)
	var lines []drawn
	opening := pointer + head + p.line(label) + columnGap
	for i, piece := range wrapWords(n.group.Span, spanRoom(n, width)) {
		if i == 0 {
			lines = append(lines, drawn{
				text: ansi.Truncate(opening+p.line(piece), width, "…"), key: selectableKey(n),
				whole: ansi.Truncate(opening+p.line(n.group.Span), width, "…"),
			})
			continue
		}
		lines = append(lines, drawn{text: ansi.Truncate(indent+p.line(piece), width, "…")})
	}
	if len(n.group.Cells) == 0 && len(n.group.WrapLast) == 0 && len(n.group.Beside) == 0 {
		return lines
	}
	room := max(widths[0]-column, 0)
	beside := n.group.Beside.elided(room)
	first := strings.Repeat(" ", min(column, widths[0])) + ansi.Truncate(p.line(beside), room, "…")
	first += strings.Repeat(" ", max(widths[0]-ansi.StringWidth(first), 0))
	text := first + columnCells(p, n.group.Cells, 1, n.group.WrapLast, columns, widths)
	lines = append(lines, drawn{text: ansi.Truncate(text, width, "…")})
	return append(lines, continuedLines(p, n.group.WrapLast, columns, widths)...)
}

func drawGroup(p painter, n node, columns []Column, widths []int, titles bool) string {
	head, label := groupHead(n)
	if len(n.group.Cells) > 0 || len(n.group.WrapLast) > 0 {
		room := max(widths[0]-1-ansi.StringWidth(head), 1)
		painted := ansi.Truncate(p.line(label), room, "…")
		out := head + painted + strings.Repeat(" ", max(room-ansi.StringWidth(painted), 0))
		return out + columnCells(p, n.group.Cells, 1, n.group.WrapLast, columns, widths)
	}
	if n.depth > 0 {
		out := head + p.line(label)
		if (!n.open || n.group.Still) && len(n.group.Summary) > 0 {
			out += "  " + p.line(n.group.Summary)
		}
		return out
	}
	pad := strings.Repeat(" ", max(widths[0]-1-ansi.StringWidth(head)-label.width(), 0))
	out := head + p.line(label) + pad
	switch {
	case titles:
		out += columnTitles(p, columns, widths)
	case !n.open || !n.drawsRows:
		if len(n.group.Summary) > 0 {
			out += columnGap + p.line(n.group.Summary)
		}
	}
	return out
}

// rowIndent is the cells ahead of a row's symbol. The last two hold MarkGlyph and the
// chevron, each in a cell of its own, so neither moves a column or takes the symbol's. The
// chevron in the last sits one cell in from its parent's, so the nesting shows.
func rowIndent(p painter, n node) string {
	mark := " "
	switch {
	case n.locked:
		mark = p.cell(C(LockGlyph, LockCode))
	case n.row.Marked:
		mark = p.cell(C(MarkGlyph, MarkCode))
	}
	return strings.Repeat(" ", indentOf(n.depth)-3) + mark + n.foldMark()
}

func drawRow(p painter, n node, columns []Column, widths []int) string {
	r := n.row
	indent := rowIndent(p, n)
	if len(r.Cells) == 0 {
		span := r.Span
		if r.changed {
			span = span.emboldened()
		}
		return indent + p.line(span)
	}
	symbol := ""
	if cell := r.Symbol; cell.Text != "" {
		symbol = p.cell(cell) + " "
	}
	first := r.Cells[0].withText(fit(r.Cells[0].Text, r.cellRoom(n.depth, 0, widths), false))
	if r.Marked {
		first.Code, first.Role = MarkCode, ""
	}
	if r.changed {
		first.Code = emboldened(first.Code)
	}
	return indent + symbol + p.cell(first) + columnCells(p, r.Cells[1:], 1, r.WrapLast, columns, widths)
}

// wrapWords breaks a line at its line breaks, then at spaces to width, each cell keeping its
// color. A word wider than width is cut.
func wrapWords(line Line, width int) []Line {
	if parts := splitAtLineBreaks(line); len(parts) > 1 {
		var out []Line
		for _, part := range parts {
			out = append(out, wrapWords(part, width)...)
		}
		return out
	}
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
				out = append(out, Line{c.withText(ansi.Cut(text, 0, width))})
				text = ansi.Cut(text, width, w)
			}
			current = append(current, c.withText(text+spaces))
			room = max(room-ansi.StringWidth(text+spaces), 0)
		}
	}
	if len(current) > 0 {
		finish()
	}
	return out
}

// splitAtLineBreaks cuts a line at each "\n" in its cells. A break reaching the terminal moves
// the cursor to column 0, outside the panel.
func splitAtLineBreaks(line Line) []Line {
	out := []Line{nil}
	for _, c := range line {
		for i, text := range strings.Split(c.Text, "\n") {
			if i > 0 {
				out = append(out, nil)
			}
			if text != "" {
				out[len(out)-1] = append(out[len(out)-1], c.withText(text))
			}
		}
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

// TallyBar is one cell per unit counted: done ▰ in green, under way ▱ in yellow, blocked ▱ in
// red and the rest ▱ plain. Its length shows how many there are, and one blocked unit among
// sixty still shows, where a fixed width would round it away. Without color only done is
// filled, so no unfinished unit reads as done. In a WrapLast it keeps every cell however
// narrow the row.
func TallyBar(done, underWay, blocked, rest int) Line {
	cells := func(glyph string, n int) string { return strings.Repeat(glyph, max(n, 0)) }
	return Line{
		C(cells("▰", done), Green),
		C(cells("▱", underWay), Yellow),
		C(cells("▱", blocked), Red),
		C(cells("▱", rest), ""),
	}
}

// RuleCode is the gray a rule draws in, which the terminal palette's theme sets.
const RuleCode = "\033[0;90m"
