package gotermboard

import (
	"slices"

	"github.com/datapointchris/gotermboard/terminalcontrol"
)

// strippedFrame is f with the terminal controls out of every text the layout measures, so the
// layout measures what the painter draws. The layout measures with x/ansi, which reads some
// controls another way than terminalcontrol.Strip, so a cell measured raw and drawn stripped
// would put its row's columns out of line. Only a line, row or group holding a control is copied,
// so the board's own frame is never written to, and a clean frame costs one scan. A Document's
// lines strip where they are laid out, because a Document is read at a width.
func strippedFrame(f Frame) Frame {
	var s stripper
	f.Strip = s.lines(f.Strip)
	f.Columns = each(&s, f.Columns, strippedColumn)
	f.Groups = each(&s, f.Groups, strippedGroup)
	return f
}

// strippedRow is r with the terminal controls out of every text the layout measures.
func strippedRow(r Row) Row {
	if stripped, changed := strippedRowAt(&r); changed {
		return stripped
	}
	return r
}

// strippedLines is lines with the terminal controls out of every cell's text.
func strippedLines(lines []Line) []Line {
	var s stripper
	return s.lines(lines)
}

// stripper strips texts and remembers whether any held a control.
type stripper struct{ changed bool }

func (s *stripper) text(text string) string {
	stripped := terminalcontrol.Strip(text)
	if len(stripped) != len(text) {
		s.changed = true
	}
	return stripped
}

func (s *stripper) cell(c Cell) Cell {
	c.Text = s.text(c.Text)
	return c
}

// line is l with its cells' texts stripped, and l itself where none holds a control.
func (s *stripper) line(l Line) Line {
	for i := range l {
		text := terminalcontrol.Strip(l[i].Text)
		if len(text) == len(l[i].Text) {
			continue
		}
		out := slices.Clone(l)
		out[i].Text = text
		for j := i + 1; j < len(out); j++ {
			out[j].Text = terminalcontrol.Strip(out[j].Text)
		}
		s.changed = true
		return out
	}
	return l
}

func (s *stripper) lines(lines []Line) []Line { return each(s, lines, strippedLine) }

// The functions each runs below say whether the item held a control, and copy it only where it
// did, so a clean item costs its scan alone.

func strippedLine(l *Line) (Line, bool) {
	var s stripper
	out := s.line(*l)
	return out, s.changed
}

func strippedColumn(c *Column) (Column, bool) {
	title := terminalcontrol.Strip(c.Title)
	if len(title) == len(c.Title) {
		return Column{}, false
	}
	out := *c
	out.Title = title
	return out, true
}

func strippedRowAt(r *Row) (Row, bool) {
	var s stripper
	symbol, under, readerTitle, bar := s.cell(r.Symbol), s.cell(r.Under), s.text(r.ReaderTitle), s.cell(r.Bar)
	cells, span, wrapLast, cutLast := s.line(r.Cells), s.line(r.Span), s.line(r.WrapLast), s.line(r.CutLast)
	second, detail, paragraph, cut := s.line(r.Second), s.line(r.Detail), s.line(r.Paragraph), s.line(r.Cut)
	if !s.changed {
		return Row{}, false
	}
	out := *r
	out.Symbol, out.Under, out.ReaderTitle, out.Bar = symbol, under, readerTitle, bar
	out.Cells, out.Span, out.WrapLast, out.CutLast = cells, span, wrapLast, cutLast
	out.Second, out.Detail, out.Paragraph, out.Cut = second, detail, paragraph, cut
	return out, true
}

func strippedGroup(g *Group) (Group, bool) {
	var s stripper
	title, cells, wrapLast, below := s.line(g.Title), s.line(g.Cells), s.line(g.WrapLast), s.lines(g.Below)
	span, beside, under, summary := s.line(g.Span), s.line(g.Beside), s.cell(g.Under), s.line(g.Summary)
	detail, rows, groups := s.line(g.Detail), each(&s, g.Rows, strippedRowAt), each(&s, g.Groups, strippedGroup)
	if !s.changed {
		return Group{}, false
	}
	out := *g
	out.Title, out.Cells, out.WrapLast, out.Below = title, cells, wrapLast, below
	out.Span, out.Beside, out.Under, out.Summary = span, beside, under, summary
	out.Detail, out.Rows, out.Groups = detail, rows, groups
	return out, true
}

// each is items with strip run on every one. It is items itself where no item held a control,
// and a copy holding the stripped items where any did.
func each[T any](s *stripper, items []T, strip func(*T) (T, bool)) []T {
	var out []T
	for i := range items {
		stripped, changed := strip(&items[i])
		if !changed {
			continue
		}
		if out == nil {
			out = slices.Clone(items)
		}
		out[i] = stripped
	}
	if out == nil {
		return items
	}
	s.changed = true
	return out
}
