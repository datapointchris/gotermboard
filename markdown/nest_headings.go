package markdown

import (
	"slices"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// NestHeadings moves every heading in source deeper by one amount, so its shallowest sits at
// depth. Text pasted under a section then keeps its own headings inside that section rather than
// closing it, which is what the Reader's path and its section jumps read. A heading goes no deeper
// than 6, and source whose shallowest heading is already at depth or deeper stays as it is. A
// setext heading becomes one ATX line, and a # inside a code block is code.
func NestHeadings(source string, depth int) string {
	src := []byte(source)
	var headings []*ast.Heading
	_ = ast.Walk(parser.Parse(text.NewReader(src)), func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if h, ok := n.(*ast.Heading); ok && entering && h.Pos() >= 0 {
			headings = append(headings, h)
		}
		return ast.WalkContinue, nil
	})
	shallowest := 6
	for _, h := range headings {
		shallowest = min(shallowest, h.Level)
	}
	shift := depth - shallowest
	if len(headings) == 0 || shift <= 0 {
		return source
	}
	// Each edit is made from the end back, so the offsets of those before it still hold.
	for _, h := range slices.Backward(headings) {
		src = deepened(src, h, min(h.Level+shift, 6))
	}
	return string(src)
}

// deepened writes heading h at level. goldmark puts an ATX heading's position on its run of #,
// which grows, empty heading or not, and a setext heading's on its first word. A setext heading's
// lines join by a space, as they render, and its underline goes.
func deepened(src []byte, h *ast.Heading, level int) []byte {
	at := h.Pos()
	hashes := at
	for hashes < len(src) && src[hashes] == '#' {
		hashes++
	}
	if hashes > at && (hashes == len(src) || strings.ContainsRune(" \t\r\n", rune(src[hashes]))) {
		return slices.Concat(src[:at], []byte(strings.Repeat("#", level)), src[hashes:])
	}
	lines := h.Lines()
	if lines.Len() == 0 {
		return src
	}
	words := make([]string, 0, lines.Len())
	for i := range lines.Len() {
		line := lines.At(i)
		words = append(words, strings.TrimSpace(string(line.Value(src))))
	}
	lastLine := lines.At(lines.Len() - 1).Start
	lineEnd := len(src)
	if i := slices.Index(src[lastLine:], '\n'); i >= 0 {
		lineEnd = lastLine + i
	}
	underlineEnd := len(src)
	if lineEnd < len(src) {
		if i := slices.Index(src[lineEnd+1:], '\n'); i >= 0 {
			underlineEnd = lineEnd + 1 + i
		}
	}
	return slices.Concat(src[:at], []byte(strings.Repeat("#", level)+" "+strings.Join(words, " ")), src[underlineEnd:])
}
