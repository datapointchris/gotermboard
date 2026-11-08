package chart

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/NimbleMarkets/ntcharts/v2/canvas"
	"github.com/NimbleMarkets/ntcharts/v2/linechart"
	"github.com/datapointchris/gotermboard"
)

// StackedArea draws layers over time, each on top of the ones before it from zero, as a stacked
// area chart or a cumulative flow diagram reads. A layer's edge falls to an eighth of a row: where
// two layers meet inside a cell, the lower one fills it from the bottom and the upper one paints the
// rest as the cell's background.
type StackedArea struct {
	// Layers stack in order, the first on the axis.
	Layers []Layer
	// From and To bound the time axis. A zero one takes the readings' first or last time.
	From, To time.Time
	// Max is the value drawn full height, and the axis reaches further where the stack goes past.
	// Zero takes the tallest stack.
	Max float64
	// TimeLabel and ValueLabel name the axes' marks, as TimeSeries' do.
	TimeLabel  func(time.Time) string
	ValueLabel func(float64) string
}

// Layer is one band of a stacked area: its own thickness at each reading, oldest first, joined
// straight between readings and zero outside them. Its Code is a foreground code, which the chart
// moves to the background where the layer sits above another inside a cell.
type Layer struct {
	Name   string
	Points []Point
	Code   string
}

// upTo are the blocks filling a cell from its bottom edge, by how many eighths they fill.
var upTo = []rune(" ▁▂▃▄▅▆▇█")

// Lines is the chart in width by height cells, over the axes TimeSeries draws.
func (c StackedArea) Lines(width, height int) []gotermboard.Line {
	sets := make([][]Point, len(c.Layers))
	for i, layer := range c.Layers {
		sets[i] = layer.Points
	}
	from, to := spanOver(c.From, c.To, sets)
	top := c.Max
	for _, p := range slices.Concat(sets...) {
		top = max(top, c.heightAt(seconds(p.At)))
	}
	if top <= 0 {
		top = 1
	}
	var cs codes
	lc, ok := timeAxes(width, height, from, to, 0, top, c.TimeLabel, c.ValueLabel, &cs)
	if !ok {
		return blank(width, height)
	}
	c.fill(&lc, &cs)
	return cs.lines(&lc.Canvas)
}

// heightAt is the stack's height at a time, in seconds.
func (c StackedArea) heightAt(at float64) float64 {
	var sum float64
	for _, layer := range c.Layers {
		v, _ := valueAt(layer.Points, at)
		sum += max(v, 0)
	}
	return sum
}

// fill draws the stack column by column over the plotting area, each layer's top rounded to an
// eighth of a row.
func (c StackedArea) fill(lc *linechart.Model, cs *codes) {
	width, height := lc.GraphWidth(), lc.GraphHeight()
	scale := float64(8*height) / (lc.MaxY() - lc.MinY())
	ends := make([]int, len(c.Layers))
	for col := range width {
		at := lc.MinX() + (float64(col)+0.5)/float64(width)*(lc.MaxX()-lc.MinX())
		var sum float64
		for i, layer := range c.Layers {
			v, _ := valueAt(layer.Points, at)
			sum += max(v, 0)
			ends[i] = min(int(sum*scale+0.5), 8*height)
		}
		for row := range height {
			glyph, code := c.cell(ends, 8*row)
			if glyph == ' ' {
				break
			}
			lc.Canvas.SetCell(canvas.Point{X: plotLeft(lc) + col, Y: height - 1 - row}, canvas.NewCellWithStyle(glyph, cs.style(code)))
		}
	}
}

// cell is the glyph and code of the cell whose eighths start at lo, given where each layer ends.
// The layer at the cell's bottom fills to where it ends. Above that, the layer at the cell's top
// paints the background, or where the stack ends inside the cell, the bottom layer fills to there.
func (c StackedArea) cell(ends []int, lo int) (rune, string) {
	bottom := slices.IndexFunc(ends, func(end int) bool { return end > lo })
	if bottom < 0 {
		return ' ', ""
	}
	if ends[bottom] >= lo+8 {
		return upTo[8], c.Layers[bottom].Code
	}
	top := slices.IndexFunc(ends, func(end int) bool { return end >= lo+8 })
	if top < 0 {
		return upTo[min(ends[len(ends)-1]-lo, 8)], c.Layers[bottom].Code
	}
	return upTo[ends[bottom]-lo], c.Layers[bottom].Code + background(c.Layers[top].Code)
}

// Legend names each layer that has a name beside its mark, the top layer first, as the chart reads
// down.
func (c StackedArea) Legend() gotermboard.Line {
	var line gotermboard.Line
	for _, layer := range slices.Backward(c.Layers) {
		if layer.Name == "" {
			continue
		}
		if len(line) > 0 {
			line = append(line, gotermboard.C("   ", ""))
		}
		line = append(line, gotermboard.C("██", layer.Code), gotermboard.C(" "+layer.Name, ""))
	}
	return line
}

// background is a foreground code's colors as a background's: each SGR color 30 to 37 or 90 to 97
// moved up ten, and every other parameter dropped.
func background(code string) string {
	var params []string
	for _, sequence := range strings.Split(code, "\033[") {
		body, ok := strings.CutSuffix(sequence, "m")
		if !ok {
			continue
		}
		for _, param := range strings.Split(body, ";") {
			if n, err := strconv.Atoi(param); err == nil && (n >= 30 && n <= 37 || n >= 90 && n <= 97) {
				params = append(params, strconv.Itoa(n+10))
			}
		}
	}
	if len(params) == 0 {
		return ""
	}
	return "\033[" + strings.Join(params, ";") + "m"
}
