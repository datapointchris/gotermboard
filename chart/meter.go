package chart

import (
	"strings"

	"github.com/datapointchris/gotermboard"
)

// Meter draws one bar across a line, to an eighth of a cell, each part after the one before it
// in its own color. A cell two parts share takes the color of the part holding more of it.
type Meter struct {
	Parts []Part
	// Max is the value the whole width draws. Zero takes the parts' sum.
	Max float64
}

// eighths are the blocks filling a cell from its left edge, by how many eighths they fill.
var eighths = []rune(" ▏▎▍▌▋▊▉█")

// Lines is the meter on the first of height lines, each width cells wide.
func (m Meter) Lines(width, height int) []gotermboard.Line {
	out := blank(width, height)
	if width < 1 || height < 1 {
		return out
	}
	var total float64
	ends := make([]float64, len(m.Parts))
	for i, part := range m.Parts {
		total += max(part.Value, 0)
		ends[i] = total
	}
	top := m.Max
	if top <= 0 {
		top = total
	}
	if top <= 0 {
		return out
	}
	scale := float64(8*width) / top
	filled := min(int(total*scale+0.5), 8*width)
	var line gotermboard.Line
	bar := []rune(run(filled, width))
	for cell, block := range bar {
		lo := 8 * cell
		line = appendText(line, string(block), m.Parts[holder(ends, scale, lo, lo+min(filled-lo, 8))].Code)
	}
	out[0] = appendText(line, strings.Repeat(" ", width-len(bar)), "")
	return out
}

// holder is the part holding most of the eighths from lo to hi, its ends scaled to eighths.
func holder(ends []float64, scale float64, lo, hi int) int {
	best, most, start := 0, -1.0, 0.0
	for i, end := range ends {
		end *= scale
		if held := min(end, float64(hi)) - max(start, float64(lo)); held > most {
			best, most = i, held
		}
		start = end
	}
	return best
}
