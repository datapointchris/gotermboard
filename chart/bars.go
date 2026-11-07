package chart

import (
	"strings"

	"github.com/NimbleMarkets/ntcharts/v2/barchart"
	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermboard"
)

// Bars draws one upright bar per value, each in its own color, over a value axis labeled at the top,
// the middle and zero, with each bar's label under the axis.
type Bars struct {
	Bars []Bar
	// Max is the value drawn full height, and the axis reaches further where a value goes past.
	// Zero takes the largest value.
	Max float64
	// ValueLabel writes a value on the value axis. Nil writes Compact.
	ValueLabel func(float64) string
}

// Bar is one value of a bar chart.
type Bar struct {
	Label string
	Value float64
	Code  string
}

// Lines is the chart in width by height cells: the bars, then the axis, then their labels.
func (c Bars) Lines(width, height int) []gotermboard.Line {
	top := c.Max
	for _, bar := range c.Bars {
		top = max(top, bar.Value)
	}
	if width < 1 || height < 3 || len(c.Bars) == 0 || top <= 0 {
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
	if width <= margin+1+len(c.Bars) {
		return blank(width, height)
	}
	var cs codes
	data := make([]barchart.BarData, 0, len(c.Bars))
	for _, bar := range c.Bars {
		value := barchart.BarValue{Name: bar.Label, Value: bar.Value, Style: cs.style(bar.Code)}
		data = append(data, barchart.BarData{Label: bar.Label, Values: []barchart.BarValue{value}})
	}
	chart := barchart.New(width-margin-1, height,
		barchart.WithStyles(cs.style(gotermboard.RuleCode), cs.style("")),
		barchart.WithNoAutoMaxValue(), barchart.WithMaxValue(top), barchart.WithDataSet(data))
	chart.Draw()
	out := make([]gotermboard.Line, 0, height)
	for y, line := range cs.lines(&chart.Canvas) {
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
