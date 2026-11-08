package chart

import (
	"github.com/NimbleMarkets/ntcharts/v2/sparkline"
	"github.com/datapointchris/gotermboard"
)

// Sparkline draws values, oldest first, as columns of eighth blocks with the newest at the right
// edge. A value of zero or less draws nothing, and any value above zero draws at least ▁, so a quiet
// stretch reads apart from an empty one.
type Sparkline struct {
	Values []float64
	Code   string
	// Max is the value drawn full height. Zero takes the largest value.
	Max float64
	// Fill widens each value into an even run of columns where there are fewer values than
	// columns, so a short series spans the chart rather than its right edge.
	Fill bool
}

// Lines is the sparkline in width by height cells, one value to a column. More values than columns
// are averaged in even runs, so the whole span shows.
func (s Sparkline) Lines(width, height int) []gotermboard.Line {
	values := buckets(s.Values, width)
	if s.Fill {
		values = widened(values, width)
	}
	top := s.Max
	for _, v := range values {
		if s.Max <= 0 {
			top = max(top, v)
		}
	}
	if width < 1 || height < 1 || top <= 0 {
		return blank(width, height)
	}
	var cs codes
	line := sparkline.New(width, height, sparkline.WithStyle(cs.style(s.Code)),
		sparkline.WithNoAutoMaxValue(), sparkline.WithMaxValue(top))
	least := top / float64(8*height)
	for _, v := range values {
		if v > 0 {
			v = max(v, least)
		}
		line.Push(min(v, top))
	}
	line.DrawColumnsOnly()
	return cs.lines(&line.Canvas)
}

// widened is values spread over n columns, each value an even run of them.
func widened(values []float64, n int) []float64 {
	if len(values) == 0 || len(values) >= n {
		return values
	}
	out := make([]float64, n)
	for i := range out {
		out[i] = values[i*len(values)/n]
	}
	return out
}

// buckets is values averaged into at most n runs of as near an equal length as they go.
func buckets(values []float64, n int) []float64 {
	if n < 1 || len(values) <= n {
		return values
	}
	out := make([]float64, n)
	for i := range out {
		run := values[i*len(values)/n : (i+1)*len(values)/n]
		for _, v := range run {
			out[i] += v / float64(len(run))
		}
	}
	return out
}
