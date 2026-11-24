package terminalcontrol

import "testing"

func TestStripTakesEachSequenceOutWholeAndKeepsTheLinesAroundIt(t *testing.T) {
	for _, test := range []struct{ name, text, want string }{
		{"a newline and a tab stay", "one\ttwo\nthree", "one\ttwo\nthree"},
		{"a newline ends an OSC with no terminator, and the next line shows", "a\x1b]0;title\nb", "a\nb"},
		{"a newline ends a CSI", "a\x1b[1\nb", "a\nb"},
		{"an OSC with no terminator takes the rest", "a\x1b]52;c;ZWNobyBoaQ==", "a"},
		{"an OSC ends at ESC \\", "a\x1b]8;;https://example.com\x1b\\b", "ab"},
		{"an OSC ends at ST as a C1 control", "a\x1b]0;title\u009cb", "ab"},
		{"an ESC inside an OSC opens the next sequence", "a\x1b]0;t\x1b[31mb", "ab"},
		{"an APC runs to its terminator", "a\x1b_Gf=24;AAAA\x1b\\b", "ab"},
		{"ESC and intermediate bytes take one final byte", "a\x1b(Bb\x1b#8c", "abc"},
		{"a C1 control leaves alone, as a code point or a lone byte, so a quote decoded in the wrong charset keeps the words after it", "â€\u009dyes\x9b2J", "â€yes2J"},
		{"DEL leaves", "a\x7fb", "ab"},
		{"an ESC before another control, and one ending the text, leave alone", "a\x1b\x01b\x1b", "ab"},
		{"characters whose bytes fall in C1's range stay", "…—█é", "…—█é"},
	} {
		if got := Strip(test.text); got != test.want {
			t.Errorf("%s: Strip(%q) = %q, want %q", test.name, test.text, got, test.want)
		}
	}
}
