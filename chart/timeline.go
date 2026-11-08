package chart

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermboard"
)

// Timeline draws spans of time on one time axis, a row each, as a Gantt chart or a trace
// waterfall lays them out. Each bar starts on the cell its span starts in and runs to where it
// ends, so a long span stands out by its length and a wait between two shows as the gap between
// their bars.
type Timeline struct {
	Spans []Span
	// From and To bound the time axis. A zero one takes the spans' earliest start or latest end. A
	// span wholly outside them draws its label alone.
	From, To time.Time
	// TimeLabel names the time under a column of the axis. Nil names how far along the axis it
	// sits: 45s, 12m, 3h or 2d.
	TimeLabel func(time.Time) string
}

// Span is one row of a timeline: its label at the left, then a bar from Start to End in its own
// color. A span whose End is not after its Start is a moment, drawn as a milestone's ◆. A span
// with a Start and no End is still under way and runs to the axis's end, so a chart of work under
// way sets the timeline's To to now. A span with no Start draws its label alone, as what has not
// happened.
type Span struct {
	Label      string
	Start, End time.Time
	Code       string
	// Text sits after the bar where it fits and else before it, such as how long the span took.
	Text string
}

// milestone marks a moment, as a Gantt chart marks one.
const milestone = "◆"

// Lines is the timeline in width by height cells: a row per span, then a line naming the time
// along the axis. Given fewer lines than that, it drops the axis first, then ends on a +N line
// counting the spans that did not fit. A bar starts on the cell its start falls in and ends to an
// eighth of a cell, and a span lasting any time at all draws at least ▏.
func (t Timeline) Lines(width, height int) []gotermboard.Line {
	from, to := t.span()
	labelWidth := labelColumn(t.Spans, func(s Span) string { return s.Label })
	plot := width - labelWidth
	if plot < 2 || height < 1 || len(t.Spans) == 0 || from.IsZero() {
		return blank(width, height)
	}
	shown, axisFits := fitRows(len(t.Spans), height, true)
	out := make([]gotermboard.Line, 0, height)
	for _, s := range t.Spans[:shown] {
		out = append(out, padded(t.row(s, from, to, labelWidth, plot), width))
	}
	if cut := len(t.Spans) - shown; cut > 0 {
		out = append(out, padded(overflow(cut), width))
	}
	if axisFits {
		out = append(out, padded(t.axis(from, to, labelWidth, plot), width))
	}
	return append(out, blank(width, height-len(out))...)
}

// span is the time the axis covers, taking what From and To leave unset from the spans, and a
// second past a lone instant so a moment still has an axis to sit on.
func (t Timeline) span() (from, to time.Time) {
	from, to = t.From, t.To
	for _, s := range t.Spans {
		if s.Start.IsZero() {
			continue
		}
		end := s.End
		if end.Before(s.Start) {
			end = s.Start
		}
		if t.From.IsZero() && (from.IsZero() || s.Start.Before(from)) {
			from = s.Start
		}
		if t.To.IsZero() && end.After(to) {
			to = end
		}
	}
	if !from.IsZero() && !to.After(from) {
		to = from.Add(time.Second)
	}
	return from, to
}

// row is one span's label, then its bar placed along the plot, with its text beside it.
func (t Timeline) row(s Span, from, to time.Time, labelWidth, plot int) gotermboard.Line {
	line := gotermboard.Line{gotermboard.C(s.Label+strings.Repeat(" ", labelWidth-ansi.StringWidth(s.Label)), "")}
	end, underWay := s.End, s.End.IsZero()
	if underWay {
		end = to
	}
	if s.Start.IsZero() || s.Start.After(to) || end.Before(from) {
		return line
	}
	perSecond := float64(8*plot) / to.Sub(from).Seconds()
	eighth := func(at time.Time) int {
		return min(max(int(math.Round(at.Sub(from).Seconds()*perSecond)), 0), 8*plot)
	}
	startCell := min(eighth(s.Start)/8, plot-1)
	bar := milestone
	if underWay || end.After(s.Start) {
		bar = run(max(eighth(end)-8*startCell, 1), plot-startCell)
	}
	barWidth := ansi.StringWidth(bar)
	text := ansi.StringWidth(s.Text)
	switch {
	case text > 0 && startCell+barWidth+1+text <= plot:
		line = appendText(line, strings.Repeat(" ", startCell), "")
		line = appendText(line, bar, s.Code)
		line = appendText(line, " "+s.Text, "")
	case text > 0 && text+1 <= startCell:
		line = appendText(line, strings.Repeat(" ", startCell-text-1)+s.Text+" ", "")
		line = appendText(line, bar, s.Code)
	default:
		line = appendText(line, strings.Repeat(" ", startCell), "")
		line = appendText(line, bar, s.Code)
	}
	return line
}

// axis names the start, the middle and the end of the time the plot covers, under its first
// column, its middle and its last.
func (t Timeline) axis(from, to time.Time, labelWidth, plot int) gotermboard.Line {
	label := t.TimeLabel
	if label == nil {
		label = func(at time.Time) string { return elapsed(at.Sub(from)) }
	}
	cells := []rune(strings.Repeat(" ", plot))
	// write puts a name where it and a cell either side of it are blank, so two never touch.
	write := func(at int, text string) {
		runes := []rune(text)
		if at < 0 || at+len(runes) > len(cells) {
			return
		}
		for i := max(at-1, 0); i < min(at+len(runes)+1, len(cells)); i++ {
			if cells[i] != ' ' {
				return
			}
		}
		copy(cells[at:], runes)
	}
	last, middle := label(to), label(from.Add(to.Sub(from)/2))
	write(0, label(from))
	write(plot-len([]rune(last)), last)
	write(plot/2-len([]rune(middle))/2, middle)
	return gotermboard.Line{gotermboard.C(strings.Repeat(" ", labelWidth)+strings.TrimRight(string(cells), " "), "")}
}

// elapsed is a duration short enough for an axis, in its largest whole unit.
func elapsed(d time.Duration) string {
	switch {
	case d <= 0:
		return "0"
	case d < time.Minute:
		return strconv.Itoa(int(d.Seconds())) + "s"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	case d < 48*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h"
	}
	return strconv.Itoa(int(d.Hours()/24)) + "d"
}
