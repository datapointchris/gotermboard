package markdown

import (
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

func TestCodeIsBoxedCutAndHighlightedOnlyInALanguageItNames(t *testing.T) {
	lines := New(nil).Lines("```go\nfunc read() string { return \"a very long string that runs past the box\" }\n```\n\n```\nfunc plain\n```\n", 30)
	if got := lines[0].Text(); got != "╭─ go" {
		t.Errorf("the box should name its language: %q", got)
	}
	if got := lines[1].Text(); !strings.HasSuffix(got, "…") || ansi.StringWidth(got) != 30 {
		t.Errorf("a long code line should be cut at the box: %q", got)
	}
	if !strings.Contains(lines[1][1].Code+lines[1][2].Code, gotermboard.Magenta) {
		t.Errorf("a keyword should wear a color: %+v", lines[1])
	}
	plain := lines[len(lines)-2]
	if plain.Text() != "│ func plain" || plain[1].Code != "" {
		t.Errorf("a fence naming no language stays plain: %+v", plain)
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
