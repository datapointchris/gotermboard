package markdown

import (
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermboard"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
)

const columnSeparatorWidth = 3

func (l *layout) table(t *east.Table, prefix gotermboard.Line, width int, base style) {
	var grid [][]gotermboard.Line
	var alignments [][]east.Alignment
	for row := t.FirstChild(); row != nil; row = row.NextSibling() {
		s := base
		if _, isHeader := row.(*east.TableHeader); isHeader {
			s.bold = true
		}
		var cells []gotermboard.Line
		var aligned []east.Alignment
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			cells = append(cells, l.inline(cell, s))
			aligned = append(aligned, alignmentOf(cell))
		}
		grid, alignments = append(grid, cells), append(alignments, aligned)
	}
	if len(grid) == 0 {
		return
	}
	columns := len(grid[0])
	widths := columnWidths(grid, columns, width-columnSeparatorWidth*(columns-1))
	rule := make([]string, columns)
	for i, w := range widths {
		rule[i] = strings.Repeat("─", w)
	}
	for at, cells := range grid {
		l.tableRow(cells, alignments[at], widths, prefix)
		if at == 0 {
			l.out = append(l.out, slices.Concat(prefix, gotermboard.Line{gotermboard.C(strings.Join(rule, "─┼─"), grayCode)}))
		}
	}
}

func alignmentOf(cell ast.Node) east.Alignment {
	if c, isCell := cell.(*east.TableCell); isCell {
		return c.Alignment
	}
	return east.AlignNone
}

// tableRow draws one row across as many lines as its tallest wrapped cell.
func (l *layout) tableRow(cells []gotermboard.Line, aligned []east.Alignment, widths []int, prefix gotermboard.Line) {
	wrappedCells := make([][]gotermboard.Line, len(widths))
	tall := 1
	for i := range widths {
		if i < len(cells) {
			wrappedCells[i] = wrapped(cells[i], widths[i])
			tall = max(tall, len(wrappedCells[i]))
		}
	}
	for line := range tall {
		row := slices.Clone(prefix)
		for i, w := range widths {
			if i > 0 {
				row = append(row, gotermboard.C(" │ ", grayCode))
			}
			var piece gotermboard.Line
			if line < len(wrappedCells[i]) {
				piece = wrappedCells[i][line]
			}
			pad := spaces(w - ansi.StringWidth(piece.Text()))
			switch {
			case i < len(aligned) && aligned[i] == east.AlignRight:
				row = slices.Concat(row, pad, piece)
			case i == len(widths)-1:
				row = slices.Concat(row, piece)
			default:
				row = slices.Concat(row, piece, pad)
			}
		}
		l.out = append(l.out, row)
	}
}

// columnWidths shares room among columns the way a browser's automatic table layout does.
// Each column keeps its longest word, then the rest of the room goes to each in proportion
// to how much wider its longest cell wants to be, so a column of short values stays whole
// beside one of long sentences.
func columnWidths(grid [][]gotermboard.Line, columns, room int) []int {
	least, most := make([]int, columns), make([]int, columns)
	for _, cells := range grid {
		for i, cell := range cells {
			if i >= columns {
				break
			}
			text := cell.Text()
			most[i] = max(most[i], ansi.StringWidth(text))
			for word := range strings.FieldsSeq(text) {
				least[i] = max(least[i], ansi.StringWidth(word))
			}
		}
	}
	sum := func(widths []int) int {
		total := 0
		for _, w := range widths {
			total += w
		}
		return total
	}
	if sum(most) <= room {
		return most
	}
	spare := room - sum(least)
	if spare <= 0 {
		return sharedFairly(least, room)
	}
	widths := make([]int, columns)
	wanting := sum(most) - sum(least)
	for i := range widths {
		widths[i] = least[i] + (most[i]-least[i])*spare/max(wanting, 1)
	}
	return widths
}

// sharedFairly is room where the columns' longest words do not all fit. Narrowest first, each
// column keeps its longest word while that fits an even share of what is left, and the rest
// split what remains, so one unbreakable word such as a path never squeezes a short column.
func sharedFairly(least []int, room int) []int {
	order := make([]int, len(least))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return least[a] - least[b] })
	widths := make([]int, len(least))
	left := room
	for n, i := range order {
		widths[i] = max(min(least[i], left/(len(order)-n)), 1)
		left -= widths[i]
	}
	return widths
}
