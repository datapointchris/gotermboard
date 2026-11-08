package markdown

import (
	"slices"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermboard"
)

// continuedMark opens a row carrying on the code line above it.
const continuedMark = "↪ "

// code draws a fence in a box labeled with its language. A fence naming a language wraps a
// line wider than the box, so all of its code can be read. Any other fence cuts that line,
// so a diagram or a table of output stays lined up, unless the renderer wraps every fence.
func (l *layout) code(language, source string, first, rest gotermboard.Line, width int) {
	label := "╭─"
	isCode := language != "" && language != "text"
	if isCode {
		label += " " + language
	}
	bar := gotermboard.Line{inRole("│ ", borderRole)}
	rows := []gotermboard.Line{{inRole(label, borderRole)}}
	for _, line := range highlighted(language, strings.TrimRight(source, "\n")) {
		pieces := []gotermboard.Line{clipped(line, width-2)}
		if isCode || l.renderer.WrapEveryFence {
			pieces = codeRows(line, width-2)
		}
		for _, piece := range pieces {
			rows = append(rows, slices.Concat(bar, piece))
		}
	}
	l.emit(first, rest, append(rows, gotermboard.Line{inRole("╰─", borderRole)}))
}

// codeRows breaks a line wider than width. Each row after the first keeps the line's indent
// behind continuedMark, so the code's shape still reads down the box.
func codeRows(line gotermboard.Line, width int) []gotermboard.Line {
	text := line.Text()
	indent := min(len(text)-len(strings.TrimLeft(text, " ")), width/2)
	continued := gotermboard.Line{gotermboard.C(strings.Repeat(" ", indent), ""), inRole(continuedMark, mutedRole)}
	var rows []gotermboard.Line
	var lead gotermboard.Line
	room := width
	for ansi.StringWidth(line.Text()) > room {
		head, tail := splitCells(line, rowEnd(line.Text(), room))
		rows = append(rows, slices.Concat(lead, head))
		line, lead = tail, continued
		room = max(width-ansi.StringWidth(continued.Text()), 1)
	}
	return append(rows, slices.Concat(lead, line))
}

// rowEnd is the column a row of code ends at: after its last space in the row's second half,
// because a break there keeps a word whole, else at the row's edge.
func rowEnd(text string, room int) int {
	row := ansi.Cut(text, 0, room)
	if space := strings.LastIndexByte(row, ' '); space >= 0 {
		if column := ansi.StringWidth(row[:space+1]); column > room/2 {
			return column
		}
	}
	return max(ansi.StringWidth(row), 1)
}

// highlighted is source by line, colored where its language is one chroma knows.
func highlighted(language, source string) []gotermboard.Line {
	plain := func() []gotermboard.Line {
		var lines []gotermboard.Line
		for line := range strings.SplitSeq(source, "\n") {
			lines = append(lines, gotermboard.Line{inRole(line, blockRole)})
		}
		return lines
	}
	lexer := lexers.Get(language)
	if language == "" || language == "text" || lexer == nil {
		return plain()
	}
	tokens, err := chroma.Coalesce(lexer).Tokenise(nil, source)
	if err != nil {
		return plain()
	}
	var lines []gotermboard.Line
	var line gotermboard.Line
	for _, token := range tokens.Tokens() {
		for at, piece := range strings.Split(token.Value, "\n") {
			if at > 0 {
				lines = append(lines, line)
				line = nil
			}
			if piece != "" {
				line = append(line, inRole(piece, tokenRole(token.Type)))
			}
		}
	}
	if len(line) > 0 {
		lines = append(lines, line)
	}
	return lines
}

// blockRole is the role of code no syntax role covers.
const blockRole = "markdown.raw.block"

// tokenRole is the theme role a token draws in, named for the nvim-treesitter capture closest to
// the chroma token, so a theme file made from an editor's colors reaches it. A narrower name
// falls back to the wider one in a theme that lacks it, as syntax.type.builtin does to
// syntax.type. The specific checks come first, since each falls inside a wider category.
func tokenRole(token chroma.TokenType) string {
	switch {
	case token.InCategory(chroma.Comment):
		return "syntax.comment"
	case token == chroma.KeywordType:
		return "syntax.type.builtin"
	case token == chroma.KeywordNamespace:
		return "syntax.keyword.import"
	case token.InCategory(chroma.Keyword):
		return "syntax.keyword"
	case token == chroma.LiteralStringDoc:
		return "syntax.string.documentation"
	case token == chroma.LiteralStringEscape:
		return "syntax.string.escape"
	case token == chroma.LiteralStringRegex:
		return "syntax.string.regexp"
	case token.InSubCategory(chroma.LiteralString):
		return "syntax.string"
	case token.InSubCategory(chroma.LiteralNumber):
		return "syntax.number"
	case token == chroma.NameBuiltin:
		return "syntax.function.builtin"
	case token == chroma.NameBuiltinPseudo:
		return "syntax.variable.builtin"
	case token.InSubCategory(chroma.NameFunction):
		return "syntax.function"
	case token == chroma.NameClass:
		return "syntax.type"
	case token == chroma.NameAttribute, token == chroma.NameDecorator:
		return "syntax.attribute"
	case token == chroma.NameTag:
		return "syntax.tag"
	case token == chroma.NameNamespace:
		return "syntax.module"
	case token == chroma.NameProperty:
		return "syntax.property"
	case token == chroma.NameLabel:
		return "syntax.label"
	case token == chroma.Name, token.InSubCategory(chroma.NameVariable):
		return "syntax.variable"
	case token.InCategory(chroma.Operator):
		return "syntax.operator"
	case token.InCategory(chroma.Punctuation):
		return "syntax.punctuation"
	case token == chroma.GenericInserted:
		return "syntax.diff.plus"
	case token == chroma.GenericDeleted:
		return "syntax.diff.minus"
	case token == chroma.GenericHeading:
		return "syntax.function"
	case token == chroma.GenericSubheading:
		return "syntax.attribute"
	}
	return blockRole
}
