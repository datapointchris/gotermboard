package chart_test

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermboard"
	"github.com/datapointchris/gotermboard/chart"
	"github.com/datapointchris/gotermstyle"
)

type chartBoard struct{ charts []chart.Chart }

func (chartBoard) Sources() []gotermboard.Refresher { return nil }
func (chartBoard) Groupings() []string              { return nil }
func (b chartBoard) Frame(time.Time, string) gotermboard.Frame {
	return gotermboard.Frame{
		Strip: []gotermboard.Line{{gotermboard.C("STRIP", "")}},
		Charts: func(width, _ int) []gotermboard.Line {
			var out []gotermboard.Line
			for _, c := range b.charts {
				out = append(out, c.Lines(width, 6)...)
			}
			return out
		},
		Columns: []gotermboard.Column{{Title: ""}},
		Groups:  []gotermboard.Group{{Key: "g", Title: gotermboard.Line{gotermboard.C("TABLE", "")}}},
	}
}

func TestEachChartFillsTheScreensWidthUnderTheStripAndTheTableFollows(t *testing.T) {
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	var used, forecast []chart.Point
	for h := range 96 {
		point := chart.Point{At: start.Add(time.Duration(h) * time.Hour), Value: float64(h)}
		if h < 60 {
			used = append(used, point)
		} else {
			forecast = append(forecast, point)
		}
	}
	charts := []chart.Chart{
		chart.TimeSeries{
			Series: []chart.Series{{Points: used, Code: gotermboard.Cyan}, {Points: forecast, Code: gotermboard.Yellow, Dotted: true}},
			Bands:  []chart.Band{{Low: forecast, High: forecast[len(forecast)/2:], Code: gotermboard.Blue}},
		},
		chart.Heatmap{Rows: []chart.HeatRow{{Label: "Mon", Values: []float64{0, 1, 2, 3}}}, Columns: []string{"00", "06", "12", "18"}},
		chart.Sparkline{Values: []float64{1, 5, 2, 8, 3, 0, 4}},
		chart.Columns{Columns: []chart.Column{{Label: "a", Value: 3}, {Label: "b", Value: 7}}},
	}
	for _, width := range []int{60, 140} {
		lines := strings.Split(gotermboard.Once(chartBoard{charts}, gotermstyle.NewPalette(false), width, 40), "\n")
		drawn := 6 * len(charts)
		if len(lines) <= 1+drawn || lines[0] != "STRIP" || !strings.Contains(lines[1+drawn], "TABLE") {
			t.Fatalf("the charts should sit between the strip and the table at %d:\n%s", width, strings.Join(lines, "\n"))
		}
		for _, line := range lines[1 : 1+drawn] {
			if got := ansi.StringWidth(line); got != width {
				t.Errorf("a chart line should fill the %d columns, filled %d: %q", width, got, line)
			}
		}
	}
}
