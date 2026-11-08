package chart

import (
	"time"

	"charm.land/lipgloss/v2"
	"github.com/NimbleMarkets/ntcharts/v2/canvas"
	"github.com/NimbleMarkets/ntcharts/v2/canvas/graph"
	"github.com/NimbleMarkets/ntcharts/v2/linechart"
	"github.com/datapointchris/gotermboard"
)

// Point is one reading of a series.
type Point struct {
	At    time.Time
	Value float64
}

// Series is a line of readings, oldest first, in one color.
type Series struct {
	// Name is the legend's word for it.
	Name   string
	Points []Point
	Code   string
	// Dotted draws every other dot along the line, as a forecast reads.
	Dotted bool
}

// Band is the range between two edges, oldest first, shaded where no line runs, as the range a
// forecast may land in.
type Band struct {
	Name      string
	Low, High []Point
	Code      string
}

// TimeSeries draws series over time as braille lines, over a time axis and a value axis.
type TimeSeries struct {
	// Series draw in order, so a later one covers an earlier where they cross.
	Series []Series
	Bands  []Band
	// From and To bound the time axis. A zero one takes the readings' first or last time, moved an
	// hour out where every reading shares one time.
	From, To time.Time
	// Min and Max bound the value axis, which reaches further where a reading goes past. Where
	// Max is not above Min, the axis runs from zero, or the lowest reading below it, to the largest
	// reading.
	Min, Max float64
	// TimeLabel names the time under a column, drawn where the name changes. Nil names the
	// minute, the hour, the weekday or the date by how long the axis runs.
	TimeLabel func(time.Time) string
	// ValueLabel writes a value on the value axis. Nil writes Compact.
	ValueLabel func(float64) string
}

// dotEvery is how far apart a dotted line's dots sit, in braille dots.
const dotEvery = 2

// Lines is the chart in width by height cells. The time axis takes the last two lines where
// there are four or more, and the value axis takes the left columns where there are twelve or more.
func (c TimeSeries) Lines(width, height int) []gotermboard.Line {
	from, to := c.span()
	if width < 1 || height < 1 || !to.After(from) {
		return blank(width, height)
	}
	low, high := c.valueRange()
	xStep, yStep := 0, 0
	plotHeight := height
	if height >= 4 {
		xStep, plotHeight = 1, height-2
	}
	if width >= 12 {
		yStep = max(2, plotHeight/4)
	}
	timeLabel, valueLabel := c.TimeLabel, c.ValueLabel
	if timeLabel == nil {
		timeLabel = labelFor(to.Sub(from))
	}
	if valueLabel == nil {
		valueLabel = Compact
	}
	var cs codes
	lc := linechart.New(width, height, seconds(from), seconds(to), low, high,
		linechart.WithXYSteps(xStep, yStep),
		linechart.WithStyles(cs.style(gotermboard.RuleCode), cs.style(""), cs.style("")),
		linechart.WithXLabelFormatter(func(_ int, v float64) string {
			return timeLabel(time.UnixMilli(int64(v * 1e3)).In(from.Location()))
		}),
		linechart.WithYLabelFormatter(func(_ int, v float64) string { return valueLabel(v) }),
	)
	if lc.GraphWidth() < 1 || lc.GraphHeight() < 1 {
		return blank(width, height)
	}
	lc.DrawXYAxisAndLabel()
	for _, s := range c.Series {
		drawLine(&lc, s, cs.style(s.Code))
	}
	for _, b := range c.Bands {
		fillBand(&lc, b, &cs)
	}
	return cs.lines(&lc.Canvas)
}

// Legend names each series and band that has a name, beside the mark it draws in.
func (c TimeSeries) Legend() gotermboard.Line {
	var line gotermboard.Line
	add := func(mark, code, name string) {
		if name == "" {
			return
		}
		if len(line) > 0 {
			line = append(line, gotermboard.C("   ", ""))
		}
		line = append(line, gotermboard.C(mark, code), gotermboard.C(" "+name, ""))
	}
	for _, s := range c.Series {
		mark := "──"
		if s.Dotted {
			mark = "··"
		}
		add(mark, s.Code, s.Name)
	}
	for _, b := range c.Bands {
		add("░░", b.Code, b.Name)
	}
	return line
}

func (c TimeSeries) span() (from, to time.Time) {
	from, to = c.From, c.To
	each := func(points []Point) {
		for _, p := range points {
			if c.From.IsZero() && (from.IsZero() || p.At.Before(from)) {
				from = p.At
			}
			if c.To.IsZero() && p.At.After(to) {
				to = p.At
			}
		}
	}
	for _, s := range c.Series {
		each(s.Points)
	}
	for _, b := range c.Bands {
		each(b.Low)
		each(b.High)
	}
	if !from.IsZero() && !to.After(from) {
		if c.From.IsZero() {
			from = from.Add(-time.Hour)
		}
		if c.To.IsZero() {
			to = to.Add(time.Hour)
		}
	}
	return from, to
}

func (c TimeSeries) valueRange() (low, high float64) {
	low, high = c.Min, c.Max
	if high <= low {
		low, high = 0, 0
	}
	each := func(points []Point) {
		for _, p := range points {
			low, high = min(low, p.Value), max(high, p.Value)
		}
	}
	for _, s := range c.Series {
		each(s.Points)
	}
	for _, b := range c.Bands {
		each(b.Low)
		each(b.High)
	}
	if high <= low {
		high = low + 1
	}
	return low, high
}

// labelFor names a column's time by how long the axis runs: its minute within six hours, its
// hour within two days, its weekday within two weeks, and its date beyond. An axis of minutes
// named by the hour would carry one name.
func labelFor(span time.Duration) func(time.Time) string {
	switch {
	case span <= 6*time.Hour:
		return func(t time.Time) string { return t.Format("15:04") }
	case span <= 48*time.Hour:
		return func(t time.Time) string { return t.Format("15:00") }
	case span <= 14*24*time.Hour:
		return func(t time.Time) string { return t.Format("Mon") }
	}
	return func(t time.Time) string { return t.Format("Jan 2") }
}

func seconds(t time.Time) float64 { return float64(t.UnixMilli()) / 1e3 }

// plotLeft is the canvas column the plotting area starts at, past the value axis.
func plotLeft(lc *linechart.Model) int {
	if lc.YStep() > 0 {
		return lc.Origin().X + 1
	}
	return 0
}

// drawLine joins a series' readings on a braille grid over the plotting area.
func drawLine(lc *linechart.Model, s Series, style lipgloss.Style) {
	grid := graph.NewBrailleGrid(lc.GraphWidth(), lc.GraphHeight(), lc.MinX(), lc.MaxX(), lc.MinY(), lc.MaxY())
	var last canvas.Point
	for i, p := range s.Points {
		at := grid.GridPoint(canvas.Float64Point{X: seconds(p.At), Y: p.Value})
		if i == 0 {
			last = at
			if len(s.Points) == 1 {
				grid.Set(at)
			}
			continue
		}
		steep := abs(at.Y-last.Y) > abs(at.X-last.X)
		for _, dot := range graph.GetLinePoints(last, at) {
			along := dot.X
			if steep {
				along = dot.Y
			}
			if !s.Dotted || along%dotEvery == 0 {
				grid.Set(dot)
			}
		}
		last = at
	}
	graph.DrawBraillePatterns(&lc.Canvas, canvas.Point{X: plotLeft(lc)}, grid.BraillePatterns(), style)
}

// fillBand fills the cells between a band's edges that no line or label holds.
func fillBand(lc *linechart.Model, b Band, cs *codes) {
	style := cs.style(b.Code)
	width, height := lc.GraphWidth(), lc.GraphHeight()
	row := func(v float64) int {
		up := int((v - lc.MinY()) / (lc.MaxY() - lc.MinY()) * float64(height))
		return height - 1 - min(max(up, 0), height-1)
	}
	for col := range width {
		at := lc.MinX() + (float64(col)+0.5)/float64(width)*(lc.MaxX()-lc.MinX())
		lo, okLow := valueAt(b.Low, at)
		hi, okHigh := valueAt(b.High, at)
		if !okLow || !okHigh {
			continue
		}
		for y := row(max(lo, hi)); y <= row(min(lo, hi)); y++ {
			p := canvas.Point{X: plotLeft(lc) + col, Y: y}
			if lc.Canvas.Cell(p).Rune == 0 {
				lc.Canvas.SetCell(p, canvas.NewCellWithStyle('░', style))
			}
		}
	}
}

// valueAt is the edge's value at a time, joined straight between readings, and false outside them.
func valueAt(points []Point, at float64) (float64, bool) {
	for i := 1; i < len(points); i++ {
		a, b := seconds(points[i-1].At), seconds(points[i].At)
		if at < a || at > b {
			continue
		}
		if b == a {
			return points[i].Value, true
		}
		return points[i-1].Value + (at-a)/(b-a)*(points[i].Value-points[i-1].Value), true
	}
	return 0, false
}

func abs(n int) int { return max(n, -n) }
