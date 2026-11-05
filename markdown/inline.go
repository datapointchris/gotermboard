package markdown

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermboard"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/util"
)

// grayCode marks what frames the text, such as a quote's bar or a link's address.
const grayCode = "\033[0;90m"

// style is what a run of text wears. A color's code opens with a reset, so it leads.
type style struct {
	color     string
	bold      bool
	italic    bool
	underline bool
	strike    bool
	reverse   bool
}

func (s style) code() string {
	code := s.color
	for _, on := range []struct {
		set  bool
		code string
	}{
		{s.bold, gotermboard.Bold},
		{s.italic, "\033[3m"},
		{s.underline, "\033[4m"},
		{s.strike, "\033[9m"},
		{s.reverse, gotermboard.Reverse},
	} {
		if on.set {
			code += on.code
		}
	}
	return code
}

func (l *layout) run(text string, s style) gotermboard.Line {
	if l.renderer.textRun != nil {
		return l.renderer.textRun(text, s.code())
	}
	return gotermboard.Line{gotermboard.C(text, s.code())}
}

// prose is a text node's bytes as a reader sees them, its escapes and entities resolved.
func prose(raw []byte) string {
	return string(util.UnescapePunctuations(util.ResolveNumericReferences(util.ResolveEntityNames(raw))))
}

func (l *layout) inline(parent ast.Node, s style) gotermboard.Line {
	var cells gotermboard.Line
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		switch v := child.(type) {
		case *ast.Text:
			if v.IsRaw() {
				cells = append(cells, gotermboard.C(string(v.Segment.Value(l.source)), s.code()))
			} else {
				cells = append(cells, l.run(prose(v.Segment.Value(l.source)), s)...)
			}
			switch {
			case v.HardLineBreak():
				cells = append(cells, gotermboard.C("\n", ""))
			case v.SoftLineBreak():
				cells = append(cells, gotermboard.C(" ", ""))
			}
		case *ast.String:
			cells = append(cells, gotermboard.C(string(v.Value), s.code()))
		case *ast.Emphasis:
			inner := s
			if v.Level >= 2 {
				inner.bold = true
			} else {
				inner.italic = true
			}
			cells = append(cells, l.inline(v, inner)...)
		case *ast.CodeSpan:
			inner := s
			inner.color = gotermboard.Yellow
			cells = append(cells, gotermboard.C(l.codeSpanText(v), inner.code()))
		case *ast.Link:
			cells = append(cells, l.linked(v, string(v.Destination), s)...)
		case *ast.Image:
			cells = append(cells, l.linked(v, string(v.Destination), s)...)
		case *ast.AutoLink:
			inner := s
			inner.color, inner.underline = gotermboard.Blue, true
			cells = append(cells, gotermboard.C(string(v.Label(l.source)), inner.code()))
		case *east.Strikethrough:
			inner := s
			inner.strike = true
			cells = append(cells, l.inline(v, inner)...)
		case *east.TaskCheckBox:
			box := "☐ "
			if v.IsChecked {
				box = "☑ "
			}
			cells = append(cells, gotermboard.C(box, gotermboard.Green))
		case *ast.RawHTML:
			for i := range v.Segments.Len() {
				segment := v.Segments.At(i)
				cells = append(cells, gotermboard.C(string(segment.Value(l.source)), s.code()))
			}
		default:
			cells = append(cells, l.inline(child, s)...)
		}
	}
	return cells
}

func (l *layout) codeSpanText(v *ast.CodeSpan) string {
	var b strings.Builder
	for child := v.FirstChild(); child != nil; child = child.NextSibling() {
		if t, isText := child.(*ast.Text); isText {
			b.Write(t.Segment.Value(l.source))
		}
	}
	return strings.ReplaceAll(b.String(), "\n", " ")
}

// linked is a link's words, then its address unless the words already are it.
func (l *layout) linked(node ast.Node, destination string, s style) gotermboard.Line {
	inner := s
	inner.color, inner.underline = gotermboard.Blue, true
	cells := l.inline(node, inner)
	if destination == "" || cells.Text() == destination {
		return cells
	}
	return append(cells, gotermboard.C(" ("+destination+")", grayCode))
}

// wrapped breaks cells at spaces to width, each piece keeping its code, so a row scrolled
// to the top of the screen still wears its style. A word wider than width takes rows of its own.
func wrapped(cells gotermboard.Line, width int) []gotermboard.Line {
	width = max(width, 1)
	var rows []gotermboard.Line
	var row, word gotermboard.Line
	used, wordWidth := 0, 0
	endRow := func() {
		rows = append(rows, row)
		row, used = nil, 0
	}
	place := func() {
		if wordWidth == 0 {
			return
		}
		if used > 0 && used+1+wordWidth > width {
			endRow()
		}
		if used > 0 {
			row = appendCell(row, gotermboard.C(" ", spaceCode(row, word)))
			used++
		}
		for wordWidth > width {
			head, tail := splitCells(word, width)
			rows = append(rows, head)
			word = tail
			wordWidth -= width
		}
		for _, piece := range word {
			row = appendCell(row, piece)
		}
		used += wordWidth
		word, wordWidth = nil, 0
	}
	for _, cell := range cells {
		for text := cell.Text; text != ""; {
			at := strings.IndexAny(text, " \n")
			if at < 0 {
				word = appendCell(word, gotermboard.C(text, cell.Code))
				wordWidth += ansi.StringWidth(text)
				break
			}
			if at > 0 {
				word = appendCell(word, gotermboard.C(text[:at], cell.Code))
				wordWidth += ansi.StringWidth(text[:at])
			}
			place()
			if text[at] == '\n' {
				endRow()
			}
			text = text[at+1:]
		}
	}
	place()
	if len(row) > 0 {
		rows = append(rows, row)
	}
	return rows
}

// spaceCode carries a style across a space inside one styled run, so a link's underline stays whole.
func spaceCode(row, word gotermboard.Line) string {
	if len(row) > 0 && len(word) > 0 && row[len(row)-1].Code == word[0].Code {
		return word[0].Code
	}
	return ""
}

func appendCell(line gotermboard.Line, cell gotermboard.Cell) gotermboard.Line {
	if at := len(line) - 1; at >= 0 && line[at].Code == cell.Code {
		line[at].Text += cell.Text
		return line
	}
	return append(line, cell)
}

// splitCells cuts a line after width columns.
func splitCells(line gotermboard.Line, width int) (head, tail gotermboard.Line) {
	room := width
	for at, cell := range line {
		w := ansi.StringWidth(cell.Text)
		if w <= room {
			head = append(head, cell)
			room -= w
			continue
		}
		if room > 0 {
			head = append(head, gotermboard.C(ansi.Cut(cell.Text, 0, room), cell.Code))
		}
		tail = append(tail, gotermboard.C(ansi.Cut(cell.Text, room, w), cell.Code))
		return head, append(tail, line[at+1:]...)
	}
	return head, nil
}

// clipped is a line no wider than width, ending in an ellipsis where it was cut.
func clipped(line gotermboard.Line, width int) gotermboard.Line {
	used := 0
	for _, cell := range line {
		used += ansi.StringWidth(cell.Text)
	}
	if used <= width {
		return line
	}
	head, _ := splitCells(line, max(width-1, 0))
	return append(head, gotermboard.C("…", grayCode))
}
