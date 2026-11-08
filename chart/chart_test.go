package chart_test

import (
	"maps"
	"slices"
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

func TestEachChartFillsTheScreensWidthInItsColorsBetweenTheStripAndTheTable(t *testing.T) {
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	var used, forecast, low, high []chart.Point
	for h := range 96 {
		at := start.Add(time.Duration(h) * time.Hour)
		if h < 60 {
			used = append(used, chart.Point{At: at, Value: float64(h)})
			continue
		}
		forecast = append(forecast, chart.Point{At: at, Value: float64(h)})
		low = append(low, chart.Point{At: at, Value: float64(h - 30)})
		high = append(high, chart.Point{At: at, Value: float64(h + 30)})
	}
	charts := []chart.Chart{
		chart.TimeSeries{
			Series: []chart.Series{{Points: used, Code: gotermboard.Cyan}, {Points: forecast, Code: gotermboard.Yellow, Dotted: true}},
			Bands:  []chart.Band{{Low: low, High: high, Code: gotermboard.Blue}},
		},
		chart.Heatmap{Rows: []chart.HeatRow{{Label: "Mon", Values: []float64{0, 1, 2, 3}}}, Columns: []string{"00", "06", "12", "18"}, Code: gotermboard.Magenta},
		chart.Sparkline{Values: []float64{1, 5, 2, 8, 3, 0, 4}, Code: gotermboard.Cyan},
		chart.Bars{Bars: []chart.Bar{{Label: "a", Value: 3, Code: gotermboard.Green}, {Label: "b", Value: 7, Code: gotermboard.Red}}},
		chart.Bars{Bars: []chart.Bar{{Label: "s", Stack: []chart.Part{{Value: 3, Code: gotermboard.Green}, {Value: 4, Code: gotermboard.Magenta}}}}},
		chart.Meter{Parts: []chart.Part{{Value: 2, Code: gotermboard.Cyan}, {Value: 5, Code: gotermboard.Yellow}}, Max: 10},
		chart.Timeline{Spans: []chart.Span{
			{Label: "queue", Start: start, End: start.Add(3 * time.Minute), Code: gotermboard.Cyan, Text: "3m"},
			{Label: "admitted", Start: start.Add(3 * time.Minute), Code: gotermboard.Green},
			{Label: "work", Start: start.Add(4 * time.Minute), End: start.Add(40 * time.Minute), Code: gotermboard.Yellow},
			{Label: "merged"},
		}},
	}
	// An ntcharts or lipgloss upgrade that hands a style's color back as other than an
	// ansi.IndexedColor would leave every chart uncolored.
	wants := [][]string{
		{gotermboard.Cyan, gotermboard.Yellow, gotermboard.Blue, gotermboard.RuleCode},
		{gotermboard.Magenta},
		{gotermboard.Cyan},
		{gotermboard.Green, gotermboard.Red, gotermboard.RuleCode},
		{gotermboard.Green, gotermboard.Magenta, gotermboard.RuleCode},
		{gotermboard.Cyan, gotermboard.Yellow},
		{gotermboard.Cyan, gotermboard.Green, gotermboard.Yellow},
	}
	for i, c := range charts {
		drawn := map[string]bool{}
		for _, line := range c.Lines(60, 6) {
			for _, cell := range line {
				drawn[cell.Code] = true
			}
		}
		for _, code := range wants[i] {
			if !drawn[code] {
				t.Errorf("%T should draw in %q, drew in %q", c, code, slices.Sorted(maps.Keys(drawn)))
			}
		}
	}
	for _, width := range []int{60, 140} {
		lines := strings.Split(gotermboard.Once(chartBoard{charts}, gotermstyle.NewPalette(false), width, 60), "\n")
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

// The shapes a reader takes a chart's meaning from: where a span's bar starts and ends, what a
// moment, a span under way and one outside the window draw, and which minutes a short axis names.
func TestChartsDrawTheShapesTheirDocsPromise(t *testing.T) {
	t0 := time.Date(2026, 11, 8, 11, 0, 0, 0, time.UTC)
	timeline := chart.Timeline{From: t0, To: t0.Add(12 * time.Minute), Spans: []chart.Span{
		{Label: "made", Start: t0, End: t0},
		{Label: "queue", Start: t0, End: t0.Add(3 * time.Minute), Text: "3m"},
		{Label: "work", Start: t0.Add(3 * time.Minute)},
		{Label: "next"},
		{Label: "before", Start: t0.Add(-5 * time.Minute), End: t0.Add(-time.Minute)},
	}}
	cases := []struct {
		name          string
		chart         chart.Chart
		width, height int
		want          []string
	}{
		{"a timeline with room for its axis", timeline, 30, 6, []string{
			"made   ◆                      ",
			"queue  █████▊ 3m              ",
			"work        ██████████████████",
			"next                          ",
			"before                        ",
			"       0         6m        12m",
		}},
		{"a timeline short of lines drops its axis, then counts the rest", timeline, 30, 3, []string{
			"made   ◆                      ",
			"queue  █████▊ 3m              ",
			"+3                            ",
		}},
		{"a filled sparkline spreads its values evenly", chart.Sparkline{Values: []float64{1, 2, 4}, Fill: true}, 12, 1, []string{
			"▂▂▂▂▄▄▄▄████",
		}},
	}
	for _, c := range cases {
		var got []string
		for _, line := range c.chart.Lines(c.width, c.height) {
			got = append(got, line.Text())
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s drew\n%s\nwant\n%s", c.name, strings.Join(got, "\n"), strings.Join(c.want, "\n"))
		}
	}
	for span, step := range map[time.Duration]int{25 * time.Minute: 5, 6 * time.Hour: 30} {
		ts := chart.TimeSeries{Series: []chart.Series{{Points: []chart.Point{{At: t0, Value: 1}, {At: t0.Add(span), Value: 2}}}}}
		lines := ts.Lines(60, 6)
		names := strings.Fields(lines[len(lines)-1].Text())
		for _, name := range names {
			if at, err := time.Parse("15:04", name); err != nil || at.Minute()%step != 0 {
				t.Errorf("a %v axis named %q among %q, want a minute on a %d-minute step", span, name, names, step)
			}
		}
		if len(names) < 3 {
			t.Errorf("a %v axis named only %q", span, names)
		}
	}
}
