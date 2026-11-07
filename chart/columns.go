package chart

import (
	"strings"

	"github.com/NimbleMarkets/ntcharts/v2/barchart"
	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermboard"
)

// Columns draws one column per value, each in its own color, over a value axis labeled at the top,
// the middle and zero, with each column's label under the axis.
type Columns struct {
	Columns []Column
	// Max is the value drawn full height, and the axis reaches further where a value goes past.
	// Zero takes the largest value.
	Max float64
	// ValueLabel writes a value on the value axis. Nil writes Compact.
	ValueLabel func(float64) string
}

// Column is one value of a column chart.
type Column struct {
	Label string
	Value float64
	Code  string
}

// Lines is the chart in width by height cells: the columns, then the axis, then their labels.
func (c Columns) Lines(width, height int) []gotermboard.Line {
	top := c.Max
	for _, col := range c.Columns {
		top = max(top, col.Value)
	}
	if width < 1 || height < 3 || len(c.Columns) == 0 || top <= 0 {
		return blank(width, height)
	}
	label := c.ValueLabel
	if label == nil {
		label = Compact
	}
	rows := height - 2
	marks := map[int]string{0: label(top), rows / 2: label(top * float64(rows-rows/2) / float64(rows)), rows: label(0)}
	margin := 0
	for _, mark := range marks {
		margin = max(margin, ansi.StringWidth(mark))
	}
	if width <= margin+1+len(c.Columns) {
		return blank(width, height)
	}
	var cs codes
	data := make([]barchart.BarData, 0, len(c.Columns))
	for _, col := range c.Columns {
		value := barchart.BarValue{Name: col.Label, Value: col.Value, Style: cs.style(col.Code)}
		data = append(data, barchart.BarData{Label: col.Label, Values: []barchart.BarValue{value}})
	}
	bars := barchart.New(width-margin-1, height,
		barchart.WithStyles(cs.style(gotermboard.RuleCode), cs.style("")),
		barchart.WithNoAutoMaxValue(), barchart.WithMaxValue(top), barchart.WithDataSet(data))
	bars.Draw()
	out := make([]gotermboard.Line, 0, height)
	for y, line := range cs.lines(&bars.Canvas) {
		mark, edge := marks[y], "│"
		switch {
		case y == rows:
			edge = "└"
		case y > rows:
			edge = " "
		case mark != "":
			edge = "┤"
		}
		head := gotermboard.Line{
			gotermboard.C(strings.Repeat(" ", margin-ansi.StringWidth(mark))+mark, ""),
			gotermboard.C(edge, gotermboard.RuleCode),
		}
		out = append(out, append(head, line...))
	}
	return out
}
