// Package printedframe picks the size one printed frame draws at, as a TUI's
// --once prints it. It imports nothing that draws, so a screen that is no board
// sizes its frame the way a board does.
package printedframe

import (
	"os"
	"strconv"

	"golang.org/x/term"
)

// Size is COLUMNS by LINES, then the size of the terminal on stdout, then the
// fallback, each read where the one before it says nothing.
func Size(fallbackWidth, fallbackHeight int) (width, height int) {
	width, height = fallbackWidth, fallbackHeight
	if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		width, height = w, h
	}
	if w, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && w > 0 {
		width = w
	}
	if h, err := strconv.Atoi(os.Getenv("LINES")); err == nil && h > 0 {
		height = h
	}
	return width, height
}
