package chart_test

import (
	"slices"
	"testing"
	"time"

	"github.com/datapointchris/gotermboard"
	"github.com/datapointchris/gotermboard/chart"
)

// Two layers meeting inside a cell split it at the eighth between them, the lower layer's block
// over the upper layer's color as background, so a band's edge reads finer than a row.
func TestAStackedAreaSplitsACellAtTheEighthWhereTwoLayersMeet(t *testing.T) {
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	flat := []chart.Point{{At: start, Value: 1}, {At: start.Add(time.Hour), Value: 1}}
	area := chart.StackedArea{Layers: []chart.Layer{{Points: flat, Code: gotermboard.Green}, {Points: flat, Code: gotermboard.Magenta}}, Max: 3.2}

	got := area.Lines(4, 2)

	want := []gotermboard.Line{
		{gotermboard.C("▂▂▂▂", gotermboard.Magenta)},
		{gotermboard.C("▅▅▅▅", gotermboard.Green+"\033[105m")},
	}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("the stack drew\n%+v\nwant\n%+v", got, want)
	}
}
