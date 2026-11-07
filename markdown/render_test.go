package markdown

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermboard"
)

func texts(lines []gotermboard.Line) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = line.Text()
	}
	return out
}

func TestAWrappedBulletHangsUnderItsTextAndASublistSitsTight(t *testing.T) {
	got := texts(New(nil).Lines("Intro.\n\n- one two three four five\n  - inner\n- last\n", 14))
	want := []string{"Intro.", "", "• one two", "  three four", "  five", "  ◦ inner", "• last"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEveryRowOfAWrappedStyleWearsItAndAWordWiderThanTheRowIsCut(t *testing.T) {
	lines := New(nil).Lines("**"+strings.Repeat("x", 25)+" bold end** plain", 10)
	for _, line := range lines {
		if ansi.StringWidth(line.Text()) > 10 {
			t.Errorf("%q is wider than the row", line.Text())
		}
	}
	for _, line := range lines[:3] {
		if line[0].Code != gotermboard.Bold {
			t.Errorf("%q should wear bold on its own row", line.Text())
		}
	}
	if last := lines[len(lines)-1]; last[len(last)-1].Code != "" {
		t.Errorf("the plain word should wear no code: %+v", last)
	}
}

func TestATableKeepsItsShortColumnWholeAndFitsTheWidth(t *testing.T) {
	source := "| repo | how it reaches a box |\n| --- | --- |\n| abtop | a tag cuts a release that every box then installs on its next apply |\n"
	lines := texts(New(nil).Lines(source, 40))
	if !strings.HasPrefix(lines[0], "repo  │ how") || !strings.Contains(lines[1], "─┼─") || !strings.HasPrefix(lines[2], "abtop │ a tag") {
		t.Errorf("the short column should keep its width: %q", lines)
	}
	if !strings.HasPrefix(lines[3], "      │ ") {
		t.Errorf("a wrapped cell should stay in its column: %q", lines)
	}
	for _, line := range lines {
		if ansi.StringWidth(line) > 40 {
			t.Errorf("%q is wider than the table's room", line)
		}
	}
}

func TestCodeWrapsAndHighlightsInALanguageItNamesAndAnyOtherFenceStaysPlainAndCut(t *testing.T) {
	code := "  func read() string { return \"a very long string that runs past the box\" }"
	lines := New(nil).Lines("```go\n"+code+"\n```\n\n```\nfunc plain with a long line that runs past the box\n```\n", 30)
	if got := lines[0].Text(); got != "╭─ go" {
		t.Errorf("the box should name its language: %q", got)
	}
	var codeRows []gotermboard.Line
	for _, line := range lines[1:] {
		if line.Text() == "╰─" {
			break
		}
		codeRows = append(codeRows, line)
	}
	var rejoined strings.Builder
	for at, row := range codeRows {
		if width := ansi.StringWidth(row.Text()); width > 30 {
			t.Errorf("row %q is %d wide, past the box", row.Text(), width)
		}
		body := strings.TrimPrefix(row.Text(), "│ ")
		if at > 0 {
			if !strings.HasPrefix(body, "  "+continuedMark) {
				t.Errorf("a continued row should keep the indent behind the mark: %q", row.Text())
			}
			body = strings.TrimPrefix(body, "  "+continuedMark)
		}
		rejoined.WriteString(body)
	}
	if len(codeRows) < 2 || rejoined.String() != code {
		t.Errorf("wrapping should keep all of the code, got %d rows of %q", len(codeRows), rejoined.String())
	}
	if !slices.ContainsFunc(codeRows[0], func(c gotermboard.Cell) bool { return c.Code == gotermboard.Magenta }) {
		t.Errorf("a keyword should wear a color: %+v", codeRows[0])
	}
	plain := lines[len(lines)-2]
	if got := plain.Text(); !strings.HasSuffix(got, "…") || ansi.StringWidth(got) != 30 || plain[1].Code != "" {
		t.Errorf("a fence naming no language stays plain and is cut at the box: %+v", plain)
	}
}

func TestARendererThatWrapsEveryFenceKeepsAllOfADiagram(t *testing.T) {
	renderer := New(nil)
	renderer.WrapEveryFence = true
	diagram := "┌ a diagram with a long line that runs well past the box ┐"
	var kept strings.Builder
	for _, line := range renderer.Lines("```text\n"+diagram+"\n```\n", 30) {
		if width := ansi.StringWidth(line.Text()); width > 30 {
			t.Errorf("row %q is %d wide, past the box", line.Text(), width)
		}
		kept.WriteString(line.Text())
	}
	for word := range strings.FieldsSeq(diagram) {
		if !strings.Contains(kept.String(), word) {
			t.Errorf("%q lost its word %q: %q", diagram, word, kept.String())
		}
	}
}

func TestProseResolvesEscapesAndKeepsAPlaceholderAndALinksAddressAsWritten(t *testing.T) {
	got := texts(New(nil).Lines("a \\* b &amp; `<code>` run new <slug> [docs](https://example.com) https://example.org\n", 200))[0]
	want := "a * b & <code> run new <slug> docs (https://example.com) https://example.org"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestACallerColorsWhatItRecognizesInProseAndNeverInCode(t *testing.T) {
	var runs []string
	renderer := New(func(text, code string) gotermboard.Line {
		runs = append(runs, text)
		return gotermboard.Line{gotermboard.C(text, code)}
	})
	renderer.Lines("see 1544 and `1600`\n\n```\n1700\n```\n", 40)
	if joined := strings.Join(runs, "|"); !strings.Contains(joined, "1544") || strings.Contains(joined, "1600") || strings.Contains(joined, "1700") {
		t.Errorf("prose reaches the caller and code does not: %q", runs)
	}
}
