package chart

import (
	"math"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermboard"
)

// Heatmap shades a grid of values, as hour of day against weekday, from a light shade to a full
// block in one color. A shade orders by density in any theme, where a ramp of the sixteen colors
// orders differently in each.
type Heatmap struct {
	Rows []HeatRow
	// Columns names each column of values on a line above the rows, where the name fits beside
	// the one before it. An empty name leaves its column unnamed, and no names draw no line.
	Columns []string
	Code    string
	// Max is the value drawn as a full block. Zero takes the largest value.
	Max float64
}

// HeatRow is one row of a heatmap, named at its left.
type HeatRow struct {
	Label  string
	Values []float64
}

// shades run from a little to the most, and a value of zero or less draws nothing.
var shades = []rune("░▒▓█")

// Lines is the heatmap in width by height cells. Each column of values takes an equal share of the
// width past the row labels, with a gap where the share is three cells or more, and each row an
// equal share of the lines under the names.
func (h Heatmap) Lines(width, height int) []gotermboard.Line {
	n, top := len(h.Columns), h.Max
	labelWidth := 0
	for _, row := range h.Rows {
		n = max(n, len(row.Values))
		labelWidth = max(labelWidth, ansi.StringWidth(row.Label))
		for _, v := range row.Values {
			top = max(top, v)
		}
	}
	if h.Max > 0 {
		top = h.Max
	}
	if width < 1 || height < 1 || n == 0 || len(h.Rows) == 0 {
		return blank(width, height)
	}
	if labelWidth > 0 {
		labelWidth++
	}
	share := max(1, (width-labelWidth)/n)
	mark := share
	if share >= 3 {
		mark--
	}
	var out []gotermboard.Line
	if len(h.Columns) > 0 {
		out = append(out, h.names(labelWidth, share))
	}
	rowHeight := max(1, (height-len(out))/len(h.Rows))
	for _, row := range h.Rows {
		for i := range rowHeight {
			label := ""
			if i == 0 {
				label = row.Label
			}
			line := gotermboard.Line{gotermboard.C(label+strings.Repeat(" ", labelWidth-ansi.StringWidth(label)), "")}
			for _, v := range row.Values {
				line = appendText(line, strings.Repeat(string(shade(v, top)), mark), h.Code)
				line = appendText(line, strings.Repeat(" ", share-mark), "")
			}
			out = append(out, line)
		}
	}
	for i := range out {
		out[i] = padded(out[i], width)
	}
	return append(out, blank(width, height-len(out))...)[:height]
}

// names is the line naming the columns, each starting over its column.
func (h Heatmap) names(labelWidth, share int) gotermboard.Line {
	line := gotermboard.Line{gotermboard.C(strings.Repeat(" ", labelWidth), "")}
	written, free := 0, 0
	for i, name := range h.Columns {
		start := i * share
		if name == "" || start < free {
			continue
		}
		line = appendText(line, strings.Repeat(" ", start-written)+name, "")
		written = start + ansi.StringWidth(name)
		free = written + 1
	}
	return line
}

func shade(v, top float64) rune {
	if v <= 0 || top <= 0 {
		return ' '
	}
	level := int(math.Ceil(v/top*float64(len(shades)))) - 1
	return shades[min(max(level, 0), len(shades)-1)]
}
