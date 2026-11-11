package gotermboard

import (
	"flag"
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
	flag.Parse()
	if !benchmarking() {
		holdToTheWholeTable()
		switch os.Getenv("HOLD_TO") {
		case "heights":
			followHook, revealHook, drawHook = nil, nil, nil
		case "nothing":
			paintHook, followHook, revealHook, drawHook = nil, nil, nil, nil
		}
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

func TestARoleOnEveryCellABoardDrawsTakesTheThemeFileAndRecolorsOnAReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "current.json")
	writeTheme := func(hex string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(`{"ui": {"warning": "`+hex+`"}}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeTheme("#ff0000")
	t.Setenv(tuitheme.PathEnv, path)
	role := func(text string) Cell { return Cell{Text: text, Role: "ui.warning"} }
	frame := Frame{
		StartAt: "plain",
		Strip:   []Line{{role("strip")}},
		Charts:  func(int, int) []Line { return []Line{{role("chart")}} },
		Columns: []Column{{Title: "NAME"}, {Title: "STATE"}, {Title: "NOTE"}},
		Groups: []Group{
			{Key: "g", Title: Line{role("label")}, Counted: true, Count: 2, Below: []Line{{role("below")}}, Rows: []Row{
				{Key: "plain", Cells: []Cell{C("the cursor's row stands here", "")}},
				{Key: "cells", Symbol: role("sym"), Cells: []Cell{role("first"), role("middle"), role("last")}, Second: Line{role("second"), role("rest")}},
				{Key: "wraps", Cells: []Cell{C("w", ""), C("w", "")}, WrapLast: Line{role("wrapped")}},
				{Key: "opens", Cells: []Cell{C("o", "")}, Detail: []Cell{role("detail")}, StartsOpen: true},
			}},
			{Key: "folded", Title: Line{C("folded", "")}, Folded: true, Summary: Line{role("summary")}, Rows: []Row{{Key: "hidden", Cells: []Cell{C("hidden", "")}}}},
			{Key: "spans", Title: Line{C("spans", "")}, Span: Line{role("span")}, Beside: Line{role("beside")}, Cells: []Cell{C("x", "")}},
		},
	}
	m := NewModel(fixedBoard{frame}, gotermstyle.NewPalette(true))
	wears := func(code string) {
		t.Helper()
		screen := m.screen()
		for _, text := range []string{"strip", "chart", "label", " · 2", "below", "sym", "first", "middle", "last", "second", "rest", "wrapped", "detail", "summary", "span", "beside"} {
			if !strings.Contains(screen, code+text) {
				t.Errorf("%q should draw in %q:\n%q", text, code, screen)
			}
		}
	}
	wears("\033[0;38;2;255;0;0m")
	writeTheme("#00ff00")
	theme, err := tuitheme.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	m.Update(themeLoadedMsg{theme: theme})
	wears("\033[0;38;2;0;255;0m")
}
