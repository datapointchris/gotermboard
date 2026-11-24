// Package printedtable prints rows as a table for a command that prints and exits, as a rich
// table prints for a Python one. It lays the table out through lipgloss's table, which measures
// a cell by the width it prints at, so a colored cell lines up where text/tabwriter would count
// its escapes as width. Each cell prints in the gotermstyle code it carries, through a palette
// the caller resolves once, so a palette that is off prints no escape at all.
package printedtable

import (
	"io"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/datapointchris/gotermstyle"
)

// defaultGap is the blank columns between two neighbors where a table sets none, as rich's
// borderless table leaves them.
const defaultGap = 2

// Column is one column's title and how its cells sit.
type Column struct {
	// Title heads the column in bold. A table whose columns all have none prints no title line.
	Title string
	// Right sets the column's cells and title against its right edge, as a count reads.
	Right bool
	// Whole keeps the column's cells whole where the table is cut to its Width, because a cut
	// handle is a command that fails when typed. The other columns give up the room, and where
	// they cannot, the table prints wider than Width.
	Whole bool
}

// Cell is a cell's text and the gotermstyle code it prints in, such as gotermstyle.CodeRed. An
// empty Code prints the text in the terminal's own foreground.
type Cell struct {
	Text string
	Code string
}

// C is a cell of text in code.
func C(text, code string) Cell { return Cell{Text: text, Code: code} }

// Table is one printed table's columns and rows, and the room it prints in. A row holding more
// cells than Columns names adds columns with no title.
type Table struct {
	Columns []Column
	Rows    [][]Cell
	// Width is how many columns wide the table may print. A wider table cuts its widest cells to
	// fit, each ending in "…". Zero prints every cell whole, as piped output is read whole.
	Width int
	// Gap is the blank columns between two neighbors. Zero takes two.
	Gap int
}

// Print writes the table's lines to w, each ending in a newline.
func (t Table) Print(w io.Writer, p gotermstyle.Palette) error {
	var b strings.Builder
	for _, line := range t.Lines(p) {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// Lines is the table's lines, one per row under the title line, each with no trailing space.
// p paints each cell in its code and each title in bold.
func (t Table) Lines(p gotermstyle.Palette) []string {
	columns := t.columns()
	if len(columns) == 0 {
		return nil
	}
	gap := t.Gap
	if gap <= 0 {
		gap = defaultGap
	}
	// lead is the blank columns before column j's text.
	lead := func(j int) int {
		if j == 0 {
			return 0
		}
		return gap
	}

	widest := make([]int, len(columns))
	titles := make([]string, len(columns))
	titled := false
	for j, column := range columns {
		widest[j] = lipgloss.Width(column.Title)
		titles[j] = painted(p, gotermstyle.CodeBold, column.Title)
		titled = titled || column.Title != ""
	}
	rows := make([][]string, len(t.Rows))
	for i, row := range t.Rows {
		rows[i] = make([]string, len(columns))
		for j, cell := range row {
			widest[j] = max(widest[j], lipgloss.Width(cell.Text))
			rows[i][j] = painted(p, cell.Code, cell.Text)
		}
	}

	drawn := table.New().
		BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).
		BorderHeader(false).BorderColumn(false).
		Wrap(false).
		Rows(rows...).
		StyleFunc(func(_, j int) lipgloss.Style {
			style := lipgloss.NewStyle().PaddingLeft(lead(j))
			if columns[j].Right {
				style = style.Align(lipgloss.Right)
			}
			if columns[j].Whole {
				style = style.Width(lead(j) + widest[j])
			}
			return style
		})
	if titled {
		drawn.Headers(titles...)
	}

	// lipgloss widens a table narrower than its Width to fill it, so only a table that would
	// overflow is given one. The floor gives each Whole column its widest cell and every other
	// column one cell, which is as far as the cut can go.
	natural, floor := 0, 0
	for j, column := range columns {
		natural += lead(j) + widest[j]
		if column.Whole {
			floor += lead(j) + widest[j]
		} else {
			floor += lead(j) + 1
		}
	}
	if t.Width > 0 && natural > t.Width {
		drawn.Width(max(t.Width, floor))
	}

	lines := strings.Split(drawn.Render(), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return lines
}

// columns is every column the table prints: the ones it names, then one with no title for each
// cell past them in the longest row.
func (t Table) columns() []Column {
	count := len(t.Columns)
	for _, row := range t.Rows {
		count = max(count, len(row))
	}
	columns := make([]Column, count)
	copy(columns, t.Columns)
	return columns
}

// painted is text in code, and an empty text stays empty rather than a bare pair of escapes.
func painted(p gotermstyle.Palette, code, text string) string {
	if text == "" {
		return ""
	}
	return p.Paint(code, text)
}
