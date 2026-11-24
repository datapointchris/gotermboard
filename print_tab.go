package gotermboard

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/datapointchris/gotermstyle"
)

// SnapshotBoard is a Board that also answers a snapshot of one shape. The snapshot holds the
// data that draws its frame, so a program's --json prints what its frame shows.
type SnapshotBoard[S any] interface {
	Board
	Snapshot(now time.Time) S
}

// PrintTab prints a tab once, from what its board's sources already hold. With asJSON set it
// writes the snapshot indented two spaces, with HTML escaping on. Otherwise it writes one frame
// in pal at OnceSize, given the fallback size.
func PrintTab[S any](out io.Writer, asJSON bool, board SnapshotBoard[S], pal gotermstyle.Palette, fallbackWidth, fallbackHeight int) error {
	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(board.Snapshot(time.Now()))
	}
	width, height := OnceSize(fallbackWidth, fallbackHeight)
	_, err := fmt.Fprintln(out, Once(board, pal, width, height))
	return err
}

// RefuseUnknownView is the error for a --view naming none of tabs, and nil where view is empty
// or names one of them. A program wraps it as it wraps its own usage errors.
func RefuseUnknownView(view string, tabs []string) error {
	if view == "" || slices.Contains(tabs, view) {
		return nil
	}
	return fmt.Errorf("--view takes one of %s, not %q", strings.Join(tabs, ", "), view)
}
