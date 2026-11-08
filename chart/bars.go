package chart

import (
	"cmp"
	"math"
	"slices"
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

// Bar is one value of a bar chart. A bar with a Stack draws each part over the one before it,
// from zero, so its height is their sum and its Value and Code go unread. A stack draws in whole
// rows, so a part under half a row can draw none.
type Bar struct {
	Label string
	Value float64
	Code  string
	Stack []Part
}

// Part is one share of a stacked bar or a meter, in its own color.
type Part struct {
	Value float64
	Code  string
}

// parts is what the bar draws, bottom first.
func (b Bar) parts() []Part {
	if len(b.Stack) > 0 {
		return b.Stack
	}
	return []Part{{Value: b.Value, Code: b.Code}}
}

func (b Bar) total() float64 {
	var sum float64
	for _, part := range b.parts() {
		sum += part.Value
	}
	return sum
}

// inRows is a bar's parts measured in rows. A drawn cell holds one color, so an edge between two
// parts inside a row would leave the rest of that row blank. A stack therefore draws in whole
// rows, its height rounded to the nearest, shared out by largest remainder: each part gets the
// floor of its share, and the rows left go to the parts with the largest fractions.
func inRows(parts []Part, rowsPer float64) []Part {
	out := make([]Part, len(parts))
	if len(parts) == 1 {
		out[0] = Part{Value: parts[0].Value * rowsPer, Code: parts[0].Code}
		return out
	}
	var total, floors float64
	fractions := make([]float64, len(parts))
	for i, part := range parts {
		share := part.Value * rowsPer
		out[i] = Part{Value: math.Floor(share), Code: part.Code}
		fractions[i] = share - out[i].Value
		total += share
		floors += out[i].Value
	}
	order := make([]int, len(parts))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(fractions[b], fractions[a]) })
	for _, i := range order[:int(math.Round(total)-floors)] {
		out[i].Value++
	}
	return out
}

// Lines is the chart in width by height cells: the bars, then the axis, then their labels.
func (c Bars) Lines(width, height int) []gotermboard.Line {
	top := c.Max
	for _, bar := range c.Bars {
		top = max(top, bar.total())
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
		var values []barchart.BarValue
		for _, part := range inRows(bar.parts(), float64(rows)/top) {
			values = append(values, barchart.BarValue{Name: bar.Label, Value: part.Value, Style: cs.style(part.Code)})
		}
		data = append(data, barchart.BarData{Label: bar.Label, Values: values})
	}
	chart := barchart.New(width-margin-1, height,
		barchart.WithStyles(cs.style(gotermboard.RuleCode), cs.style("")),
		barchart.WithNoAutoMaxValue(), barchart.WithMaxValue(float64(rows)), barchart.WithDataSet(data))
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
