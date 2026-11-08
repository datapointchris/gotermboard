package markdown

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermboard"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/util"
)

// The theme roles of what frames the text: a box's edge, a quote's bar and a table's rules, and
// the marks the layout adds, such as a cut line's ellipsis.
const (
	borderRole = "ui.border"
	mutedRole  = "ui.muted"
)

// style is what a run of text wears: a TUI theme role, which the board resolves as it draws, and
// the attributes the layout sets over it.
type style struct {
	role    string
	bold    bool
	italic  bool
	strike  bool
	reverse bool
}

// code is the style's attributes, which draw over its role.
func (s style) code() string {
	var code string
	for _, on := range []struct {
		set  bool
		code string
	}{
		{s.bold, gotermboard.Bold},
		{s.italic, "\033[3m"},
		{s.strike, "\033[9m"},
		{s.reverse, gotermboard.Reverse},
	} {
		if on.set {
			code += on.code
		}
	}
	return code
}

func (s style) cell(text string) gotermboard.Cell {
	return gotermboard.Cell{Text: text, Code: s.code(), Role: s.role}
}

// inRole is text drawn in role alone.
func inRole(text, role string) gotermboard.Cell { return gotermboard.Cell{Text: text, Role: role} }

// piece is part of cell's text, in cell's code and role.
func piece(cell gotermboard.Cell, text string) gotermboard.Cell {
	cell.Text = text
	return cell
}

// run is prose in s, through the caller's textRun where there is one. A cell it hands back in
// the run's own code wears the run's role too, and one it colored keeps the color it was given.
func (l *layout) run(text string, s style) gotermboard.Line {
	if l.renderer.textRun == nil {
		return gotermboard.Line{s.cell(text)}
	}
	cells := l.renderer.textRun(text, s.code())
	for i := range cells {
		if cells[i].Code == s.code() && cells[i].Role == "" {
			cells[i].Role = s.role
		}
	}
	return cells
}

// prose is a text node's bytes as a reader sees them, its escapes and entities resolved in one
// pass, so an escaped `&` stays as typed rather than opening a reference.
func prose(raw []byte) string {
	var b strings.Builder
	resolve := func(run []byte) { b.Write(util.ResolveNumericReferences(util.ResolveEntityNames(run))) }
	start := 0
	for i := 0; i < len(raw)-1; i++ {
		if raw[i] == '\\' && util.IsPunct(raw[i+1]) {
			resolve(raw[start:i])
			b.WriteByte(raw[i+1])
			i++
			start = i + 1
		}
	}
	resolve(raw[start:])
	return b.String()
}

func (l *layout) inline(parent ast.Node, s style) gotermboard.Line {
	var cells gotermboard.Line
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		switch v := child.(type) {
		case *ast.Text:
			if v.IsRaw() {
				cells = append(cells, s.cell(string(v.Segment.Value(l.source))))
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
			cells = append(cells, s.cell(string(v.Value)))
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
			inner.role = "markdown.raw"
			cells = append(cells, inner.cell(l.codeSpanText(v)))
		case *ast.Link:
			cells = append(cells, l.linked(v, string(v.Destination), s)...)
		case *ast.Image:
			cells = append(cells, l.linked(v, string(v.Destination), s)...)
		case *ast.AutoLink:
			inner := s
			inner.role = "markdown.link"
			cells = append(cells, inner.cell(string(v.Label(l.source))))
		case *east.Strikethrough:
			inner := s
			inner.strike = true
			cells = append(cells, l.inline(v, inner)...)
		case *east.TaskCheckBox:
			box := inRole("☐ ", "markdown.list.unchecked")
			if v.IsChecked {
				box = inRole("☑ ", "markdown.list.checked")
			}
			cells = append(cells, box)
		case *ast.RawHTML:
			for i := range v.Segments.Len() {
				segment := v.Segments.At(i)
				cells = append(cells, s.cell(string(segment.Value(l.source))))
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
	inner.role = "markdown.link.label"
	cells := l.inline(node, inner)
	if destination == "" || cells.Text() == destination {
		return cells
	}
	return append(cells, inRole(" ("+destination+")", "markdown.link.url"))
}

// wrapped breaks cells at spaces to width, each piece keeping its code and role, so a row
// scrolled to the top of the screen still wears its style. A word wider than width takes rows
// of its own.
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
			row = appendCell(row, space(row, word))
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
				word = appendCell(word, piece(cell, text))
				wordWidth += ansi.StringWidth(text)
				break
			}
			if at > 0 {
				word = appendCell(word, piece(cell, text[:at]))
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

// space is the space between two words, carrying a style across it inside one styled run, so a
// link's underline stays whole.
func space(row, word gotermboard.Line) gotermboard.Cell {
	if len(row) > 0 && len(word) > 0 && sameLook(row[len(row)-1], word[0]) {
		return piece(word[0], " ")
	}
	return gotermboard.C(" ", "")
}

func sameLook(a, b gotermboard.Cell) bool { return a.Code == b.Code && a.Role == b.Role }

func appendCell(line gotermboard.Line, cell gotermboard.Cell) gotermboard.Line {
	if at := len(line) - 1; at >= 0 && sameLook(line[at], cell) {
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
			head = append(head, piece(cell, ansi.Cut(cell.Text, 0, room)))
		}
		tail = append(tail, piece(cell, ansi.Cut(cell.Text, room, w)))
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
	return append(head, inRole("…", mutedRole))
}
