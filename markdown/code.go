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
// so a diagram or a table of output stays lined up.
func (l *layout) code(language, source string, first, rest gotermboard.Line, width int) {
	label := "╭─"
	isCode := language != "" && language != "text"
	if isCode {
		label += " " + language
	}
	bar := gotermboard.Line{gotermboard.C("│ ", grayCode)}
	rows := []gotermboard.Line{{gotermboard.C(label, grayCode)}}
	for _, line := range highlighted(language, strings.TrimRight(source, "\n")) {
		pieces := []gotermboard.Line{clipped(line, width-2)}
		if isCode {
			pieces = codeRows(line, width-2)
		}
		for _, piece := range pieces {
			rows = append(rows, slices.Concat(bar, piece))
		}
	}
	l.emit(first, rest, append(rows, gotermboard.Line{gotermboard.C("╰─", grayCode)}))
}

// codeRows breaks a line wider than width. Each row after the first keeps the line's indent
// behind continuedMark, so the code's shape still reads down the box.
func codeRows(line gotermboard.Line, width int) []gotermboard.Line {
	text := line.Text()
	indent := min(len(text)-len(strings.TrimLeft(text, " ")), width/2)
	continued := gotermboard.Line{gotermboard.C(strings.Repeat(" ", indent), ""), gotermboard.C(continuedMark, grayCode)}
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
			lines = append(lines, gotermboard.Line{gotermboard.C(line, "")})
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
				line = append(line, gotermboard.C(piece, tokenCode(token.Type)))
			}
		}
	}
	if len(line) > 0 {
		lines = append(lines, line)
	}
	return lines
}

// tokenCode colors a token by the kind of thing it is, in ANSI 0-15 so code follows the
// theme. The specific checks come first, since each falls inside a wider category.
func tokenCode(token chroma.TokenType) string {
	switch {
	case token.InCategory(chroma.Comment):
		return grayCode + "\033[3m"
	case token == chroma.KeywordType, token == chroma.NameBuiltin, token == chroma.NameClass, token == chroma.NameAttribute:
		return gotermboard.Cyan
	case token.InCategory(chroma.Keyword):
		return gotermboard.Magenta
	case token.InSubCategory(chroma.LiteralString):
		return gotermboard.Green
	case token.InSubCategory(chroma.LiteralNumber):
		return gotermboard.Yellow
	case token == chroma.NameFunction, token == chroma.NameTag, token == chroma.NameDecorator:
		return gotermboard.Blue
	case token == chroma.GenericInserted:
		return gotermboard.Green
	case token == chroma.GenericDeleted:
		return gotermboard.Red
	case token == chroma.GenericHeading:
		return gotermboard.Bold
	case token == chroma.GenericSubheading:
		return gotermboard.Cyan
	}
	return ""
}
