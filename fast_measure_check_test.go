package gotermboard

import (
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestTheFastMeasuresEqualAnsi holds textWidth and cut to the ansi package: on every glyph narrow
// names, alone and beside others, and on random strings of colors, ASCII, the glyphs a board draws,
// wide text, emoji and joiners. FAST_MEASURE_CHECK sets how many random strings it tries.
func TestTheFastMeasuresEqualAnsi(t *testing.T) {
	tries := 20000
	if n, err := strconv.Atoi(os.Getenv("FAST_MEASURE_CHECK")); err == nil && n > 1 {
		tries = n
	}
	parts := []string{
		"a", "Z", " ", "~", "0", "\033[1;95m", "\033[0m", "\033[38;2;10;20;30m", "▾", "▸", "•", "▌", "│", "└─", "─", "┄", "═", "◆", "…", "·",
		"›", "↪", "▪", "▰", "▱", "●", "◦", "☐", "☑", "⠋", "█", "░", "▁",
		"日", "本", "語", "🚀", "é", "e\u0301", "λ", "\u200d", "👨\u200d👩\u200d👧", "\t", "ｱ", "✓", "\ufe0f", "⚠\ufe0f", "\033[", "\033", "\x7f",
		"\U0001f3fb", "\u20e3", "\u0300", "🇯", "🇵",
	}
	for r := rune(0x80); r <= 0x2fff; r++ {
		if !narrow(r) {
			continue
		}
		for _, s := range []string{string(r), "a" + string(r) + "b", string(r) + string(r), "\033[1m" + string(r) + "\033[0m"} {
			if got, want := textWidth(s), ansi.StringWidth(s); got != want {
				t.Fatalf("textWidth(%q) = %d, and ansi.StringWidth says %d", s, got, want)
			}
		}
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for range tries {
		var b strings.Builder
		for range rng.IntN(40) {
			b.WriteString(parts[rng.IntN(len(parts))])
		}
		s := b.String()
		if got, want := textWidth(s), ansi.StringWidth(s); got != want {
			t.Fatalf("textWidth(%q) = %d, and ansi.StringWidth says %d", s, got, want)
		}
		width := rng.IntN(45)
		if got, want := cut(s, width), ansi.Truncate(s, width, "…"); got != want {
			t.Fatalf("cut(%q, %d) = %q, and ansi.Truncate says %q", s, width, got, want)
		}
	}
	t.Logf("%d random strings measured and cut the same both ways", tries)
}
