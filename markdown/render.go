// Package markdown lays markdown out as a board's lines at a given width, in colors that
// follow the terminal's theme. It wraps before it styles, so each line carries its own
// colors, and a bullet's wrapped lines hang under its text.
package markdown

import (
	"slices"
	"strconv"
	"strings"

	"github.com/datapointchris/gotermboard"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// keptRenderings bounds what a Renderer holds. A board draws the same few descriptions
// many times a minute, so this is far more than a screen shows.
const keptRenderings = 64

// narrowest is the width a layout never goes under, so a tiny panel still reads.
const narrowest = 10

var parser = goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser()

type rendering struct {
	source string
	width  int
}

// Renderer lays out markdown and keeps what it laid out, because a board redraws on every
// key and tick while its text stays the same. It belongs to one goroutine.
type Renderer struct {
	textRun  func(text, code string) gotermboard.Line
	rendered map[rendering][]gotermboard.Line
}

// New is a Renderer. textRun, when not nil, turns a run of prose into cells, so a caller
// can color what it recognizes there. Code never reaches it.
func New(textRun func(text, code string) gotermboard.Line) *Renderer {
	return &Renderer{textRun: textRun, rendered: map[rendering][]gotermboard.Line{}}
}

// Lines is source laid out to width. A caller treats them as read-only, since they are kept.
func (r *Renderer) Lines(source string, width int) []gotermboard.Line {
	key := rendering{source, width}
	if lines, held := r.rendered[key]; held {
		return lines
	}
	if len(r.rendered) >= keptRenderings {
		clear(r.rendered)
	}
	lines := r.layOut(source, width)
	r.rendered[key] = lines
	return lines
}

func (r *Renderer) layOut(source string, width int) []gotermboard.Line {
	// A terminal draws a tab to its next stop, which width counting reads as nothing.
	src := []byte(strings.ReplaceAll(source, "\t", "    "))
	l := &layout{renderer: r, source: src}
	l.blocks(parser.Parse(text.NewReader(src)), nil, nil, max(width, narrowest), style{})
	return l.out
}

// layout is one pass over one document.
type layout struct {
	renderer *Renderer
	source   []byte
	out      []gotermboard.Line
}

func (l *layout) emit(first, rest gotermboard.Line, rows []gotermboard.Line) {
	lead := first
	for _, row := range rows {
		l.out = append(l.out, slices.Concat(lead, row))
		lead = rest
	}
}

// blank is an empty line, keeping a quote's bar where one is open.
func (l *layout) blank(prefix gotermboard.Line) {
	if strings.TrimSpace(prefix.Text()) == "" {
		l.out = append(l.out, nil)
		return
	}
	l.out = append(l.out, slices.Clone(prefix))
}

func (l *layout) blocks(parent ast.Node, first, rest gotermboard.Line, width int, base style) {
	lead := first
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		if child != parent.FirstChild() && spacedFromTheBlockBefore(parent, child) {
			l.blank(rest)
		}
		l.block(child, lead, rest, width, base)
		lead = rest
	}
}

// spacedFromTheBlockBefore is false inside a tight list item, whose text and sublist sit together.
func spacedFromTheBlockBefore(parent, child ast.Node) bool {
	if _, inItem := parent.(*ast.ListItem); !inItem {
		return true
	}
	switch child.(type) {
	case *ast.List, *ast.TextBlock:
		return false
	}
	return true
}

func (l *layout) block(node ast.Node, first, rest gotermboard.Line, width int, base style) {
	switch v := node.(type) {
	case *ast.Heading:
		l.heading(v, first, rest, width)
	case *ast.Paragraph, *ast.TextBlock:
		l.emit(first, rest, wrapped(l.inline(v, base), width))
	case *ast.List:
		l.list(v, first, rest, width, base)
	case *ast.Blockquote:
		bar := gotermboard.C("│ ", grayCode)
		base.italic = true
		l.blocks(v, slices.Concat(first, gotermboard.Line{bar}), slices.Concat(rest, gotermboard.Line{bar}), width-2, base)
	case *ast.FencedCodeBlock:
		l.code(string(v.Language(l.source)), l.linesOf(v), first, rest, width)
	case *ast.CodeBlock:
		l.code("", l.linesOf(v), first, rest, width)
	case *east.Table:
		l.table(v, first, width, base)
	case *ast.ThematicBreak:
		l.emit(first, rest, []gotermboard.Line{{gotermboard.C(strings.Repeat("─", min(width, 40)), grayCode)}})
	case *ast.HTMLBlock:
		// Raw HTML shows as written, so a placeholder like <slug> reads as its author typed it.
		source := l.linesOf(v)
		if v.HasClosure() {
			source += string(v.ClosureLine.Value(l.source))
		}
		var rows []gotermboard.Line
		for line := range strings.SplitSeq(strings.TrimRight(source, "\n"), "\n") {
			rows = append(rows, wrapped(gotermboard.Line{gotermboard.C(line, "")}, width)...)
		}
		l.emit(first, rest, rows)
	}
}

func (l *layout) linesOf(node ast.Node) string {
	var b strings.Builder
	lines := node.Lines()
	for i := range lines.Len() {
		segment := lines.At(i)
		b.Write(segment.Value(l.source))
	}
	return b.String()
}

func (l *layout) heading(v *ast.Heading, first, rest gotermboard.Line, width int) {
	s := style{bold: true}
	var mark gotermboard.Line
	switch v.Level {
	case 1:
		s.color, s.reverse = gotermboard.Blue, true
	case 2:
		s.color = gotermboard.Blue
		mark = gotermboard.Line{gotermboard.C("▌ ", gotermboard.Blue)}
	case 3:
		s.color = gotermboard.Cyan
		mark = gotermboard.Line{gotermboard.C("▎ ", gotermboard.Cyan)}
	}
	markWidth := len([]rune(mark.Text()))
	l.emit(slices.Concat(first, mark), slices.Concat(rest, spaces(markWidth)), wrapped(l.inline(v, s), width-markWidth))
}

// bullets are a list's marks by how deep it sits, the deepest repeating.
var bullets = []string{"•", "◦", "▪"}

func (l *layout) list(v *ast.List, first, rest gotermboard.Line, width int, base style) {
	mark := func(number int) string {
		if v.IsOrdered() {
			return strconv.Itoa(number) + "."
		}
		return bullets[min(listDepth(v), len(bullets)-1)]
	}
	markWidth := 2
	if v.IsOrdered() {
		markWidth = len(mark(v.Start+v.ChildCount()-1)) + 1
	}
	lead := first
	number := v.Start
	for item := v.FirstChild(); item != nil; item = item.NextSibling() {
		text := mark(number)
		marker := gotermboard.C(text+strings.Repeat(" ", markWidth-len([]rune(text))), gotermboard.Blue)
		l.blocks(item, slices.Concat(lead, gotermboard.Line{marker}), slices.Concat(rest, spaces(markWidth)), width-markWidth, base)
		lead = rest
		number++
		if !v.IsTight && item.NextSibling() != nil {
			l.blank(rest)
		}
	}
}

func listDepth(v ast.Node) int {
	depth := 0
	for parent := v.Parent(); parent != nil; parent = parent.Parent() {
		if _, isList := parent.(*ast.List); isList {
			depth++
		}
	}
	return depth
}

func spaces(n int) gotermboard.Line {
	if n <= 0 {
		return nil
	}
	return gotermboard.Line{gotermboard.C(strings.Repeat(" ", n), "")}
}
