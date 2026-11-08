// Package chart draws plain series as lines of cells for a board: a time series with a forecast and
// a band, a heatmap, a sparkline and columns. Each fills exactly the width and height it is given,
// and colors its cells with the codes the caller passes, so the board's palette paints them in the
// terminal's own colors.
package chart

import (
	"image/color"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/NimbleMarkets/ntcharts/v2/canvas"
	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermboard"
)

// Chart is any of them, laid out in a width and height of cells.
type Chart interface {
	Lines(width, height int) []gotermboard.Line
}

// Document lays a chart out at whatever width the board gives it, at a fixed height, as a row's
// Panel takes it.
func Document(c Chart, height int) gotermboard.Document {
	return func(width int) []gotermboard.Line { return c.Lines(width, height) }
}

// codes gives each code a chart draws in an index of its own, which ntcharts carries on a cell's
// style, and reads the code back from a drawn cell. ntcharts then never writes an escape, and the
// board's palette paints every cell.
type codes []string

func (c *codes) style(code string) lipgloss.Style {
	if code == "" {
		return lipgloss.NewStyle()
	}
	i := slices.Index(*c, code)
	if i < 0 {
		i = len(*c)
		*c = append(*c, code)
	}
	return lipgloss.NewStyle().Foreground(ansi.IndexedColor(i))
}

func (c codes) of(fg color.Color) string {
	if i, ok := fg.(ansi.IndexedColor); ok && int(i) < len(c) {
		return c[i]
	}
	return ""
}

// lines reads a drawn canvas back as cells, one run of a color to a cell.
func (c codes) lines(cv *canvas.Model) []gotermboard.Line {
	out := make([]gotermboard.Line, cv.Height())
	for y := range cv.Height() {
		var line gotermboard.Line
		for x := range cv.Width() {
			cell := cv.Cell(canvas.Point{X: x, Y: y})
			text, code := string(cell.Rune), c.of(cell.Style.GetForeground())
			if cell.Rune == 0 || cell.Rune == ' ' {
				text, code = " ", ""
			}
			line = appendText(line, text, code)
		}
		out[y] = line
	}
	return out
}

// appendText adds text to a line, joining the last cell where it holds the same color.
func appendText(line gotermboard.Line, text, code string) gotermboard.Line {
	if n := len(line); n > 0 && line[n-1].Code == code {
		line[n-1].Text += text
		return line
	}
	return append(line, gotermboard.C(text, code))
}

// blank is height lines of width spaces, for a chart given nothing to draw.
func blank(width, height int) []gotermboard.Line {
	out := make([]gotermboard.Line, max(height, 0))
	for i := range out {
		out[i] = gotermboard.Line{gotermboard.C(strings.Repeat(" ", max(width, 0)), "")}
	}
	return out
}

// Beside lays blocks of lines side by side, gap spaces apart, each as wide as its widest line.
func Beside(gap int, blocks ...[]gotermboard.Line) []gotermboard.Line {
	widths := make([]int, len(blocks))
	height := 0
	for i, block := range blocks {
		height = max(height, len(block))
		for _, line := range block {
			widths[i] = max(widths[i], ansi.StringWidth(line.Text()))
		}
	}
	out := make([]gotermboard.Line, height)
	for y := range out {
		var line gotermboard.Line
		for i, block := range blocks {
			if i > 0 {
				line = appendText(line, strings.Repeat(" ", gap), "")
			}
			var part gotermboard.Line
			if y < len(block) {
				part = block[y]
			}
			for _, cell := range padded(part, widths[i]) {
				line = appendText(line, cell.Text, cell.Code)
			}
		}
		out[y] = line
	}
	return out
}

// padded is a line filled out with spaces to width, or cut to it.
func padded(line gotermboard.Line, width int) gotermboard.Line {
	line = slices.Clone(line)
	w := ansi.StringWidth(line.Text())
	if w < width {
		return appendText(line, strings.Repeat(" ", width-w), "")
	}
	for w > width && len(line) > 0 {
		last := &line[len(line)-1]
		lastWidth := ansi.StringWidth(last.Text)
		if w-lastWidth >= width {
			line, w = line[:len(line)-1], w-lastWidth
			continue
		}
		last.Text = ansi.Cut(last.Text, 0, lastWidth-(w-width))
		w = width
	}
	return line
}

// Compact writes a value short enough for an axis: 1.2M, 12k, 12 or 1.5.
func Compact(v float64) string {
	switch a := max(v, -v); {
	case a >= 1e6:
		return strconv.FormatFloat(v/1e6, 'f', 1, 64) + "M"
	case a >= 1e3:
		return strconv.FormatFloat(v/1e3, 'f', 0, 64) + "k"
	case v == float64(int64(v)):
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}
