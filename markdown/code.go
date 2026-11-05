package markdown

import (
	"slices"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/datapointchris/gotermboard"
)

// code draws a fence in a box labeled with its language. A line wider than the box is
// cut, so a diagram or a table of output stays lined up.
func (l *layout) code(language, source string, first, rest gotermboard.Line, width int) {
	label := "╭─"
	if language != "" && language != "text" {
		label += " " + language
	}
	bar := gotermboard.Line{gotermboard.C("│ ", grayCode)}
	rows := []gotermboard.Line{{gotermboard.C(label, grayCode)}}
	for _, line := range highlighted(language, strings.TrimRight(source, "\n")) {
		rows = append(rows, slices.Concat(bar, clipped(line, width-2)))
	}
	l.emit(first, rest, append(rows, gotermboard.Line{gotermboard.C("╰─", grayCode)}))
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
	case token == chroma.NameFunction, token == chroma.NameTag:
		return gotermboard.Blue
	case token == chroma.GenericInserted:
		return gotermboard.Green
	case token == chroma.GenericDeleted:
		return gotermboard.Red
	}
	return ""
}
