package gotermboard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermstyle"
	"github.com/datapointchris/gotermstyle/tuitheme"
)

// TestMain points tuitheme at a file that does not exist, so every test draws the built-in
// theme whatever theme file the machine running it has installed.
func TestMain(m *testing.M) {
	if err := os.Setenv(tuitheme.PathEnv, filepath.Join(os.TempDir(), "gotermboard-tests-read-no-theme", "current.json")); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func TestTheCursorsRowWearsTheThemesSelectionAcrossItsWidthOrBoldBehindABarWithoutOne(t *testing.T) {
	frame := Frame{StartAt: "one", Columns: []Column{{Title: "NAME"}, {Title: "STATE"}}, Groups: []Group{{Key: "g", Rows: []Row{
		{Key: "one", Cells: []Cell{C("first", ""), C("failed", Red)}},
		{Key: "two", Cells: []Cell{C("second", ""), C("ok", Green)}},
	}}}}
	const width = 60
	rows := func() (selected, other string) {
		for _, line := range strings.Split(Once(fixedBoard{frame}, gotermstyle.NewPalette(true), width, 8), "\n") {
			switch {
			case strings.Contains(line, "first"):
				selected = line
			case strings.Contains(line, "second"):
				other = line
			}
		}
		return selected, other
	}
	columnOf := func(line, word string) int {
		before, _, found := strings.Cut(ansi.Strip(line), word)
		if !found {
			t.Fatalf("no %q in %q", word, line)
		}
		return ansi.StringWidth(before)
	}
	columnHolds := func(selected, other string) {
		t.Helper()
		if at, want := columnOf(selected, "failed"), columnOf(other, "ok"); at != want {
			t.Errorf("the cursor's row moved its STATE column to %d, the other row has it at %d", at, want)
		}
	}

	selected, other := rows()
	if !strings.HasPrefix(ansi.Strip(selected), "▌") || !strings.Contains(selected, Red+Bold) {
		t.Errorf("without a theme file the cursor's row should be bold behind ▌, keeping its red: %q", selected)
	}
	if strings.Contains(selected, Reverse) || strings.Contains(selected, "\033[4") {
		t.Errorf("without a theme file the cursor's row should draw no background: %q", selected)
	}
	columnHolds(selected, other)

	path := filepath.Join(t.TempDir(), "current.json")
	if err := os.WriteFile(path, []byte(`{"ui": {"selection": {"fg": "#c5c8c6", "bg": "#373b41"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(tuitheme.PathEnv, path)
	selected, other = rows()
	tint := "\033[48;2;55;59;65m"
	if !strings.HasPrefix(selected, tint) || !strings.Contains(selected, Red+tint) || strings.Contains(ansi.Strip(selected), "▌") {
		t.Errorf("with a theme file the cursor's row should take its selection background, keeping its red: %q", selected)
	}
	if got := ansi.StringWidth(selected); got != width {
		t.Errorf("the selection runs %d columns, want the table's %d", got, width)
	}
	if strings.Contains(other, tint) {
		t.Errorf("a row off the cursor took the selection: %q", other)
	}
	columnHolds(selected, other)
}
