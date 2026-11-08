package chart_test

import (
	"slices"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermboard"
	"github.com/datapointchris/gotermboard/chart"
)

// A stacked bar's parts meet on whole rows, since a cell drawn part-empty reads as a gap, and each
// part keeps the row its own share earns.
func TestAStackedBarDrawsWholeRowsByEachPartsShare(t *testing.T) {
	bars := chart.Bars{Bars: []chart.Bar{{Label: "s", Stack: []chart.Part{
		{Value: 11.53, Code: gotermboard.Green},
		{Value: 0.95, Code: gotermboard.Red},
		{Value: 0.43, Code: gotermboard.Yellow},
		{Value: 0.08, Code: gotermboard.Magenta},
	}}}}
	want := append([]string{gotermboard.Red}, slices.Repeat([]string{gotermboard.Green}, 12)...)
	var drawn []string
	for _, line := range bars.Lines(20, 15)[:13] {
		at := slices.IndexFunc([]rune(line.Text()), func(r rune) bool { return r == '│' || r == '┤' }) + 1
		for _, cell := range line {
			if width := ansi.StringWidth(cell.Text); at < width {
				if []rune(cell.Text)[at] != '█' {
					t.Errorf("row %d draws %q, so a part leaves a gap", len(drawn), []rune(cell.Text)[at])
				}
				drawn = append(drawn, cell.Code)
				break
			} else {
				at -= width
			}
		}
	}
	if !slices.Equal(drawn, want) {
		t.Errorf("the bar drew %q from the top, want %q", drawn, want)
	}
}
