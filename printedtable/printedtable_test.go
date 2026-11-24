package printedtable

import (
	"bytes"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermstyle"
)

// dueRows are shaped like a to-do screen's: a label, what is owed, how late in its urgency's color,
// and the command that completes it, which no cut may reach.
func dueRows() Table {
	return Table{
		Columns: []Column{{}, {}, {}, {Whole: true}},
		Rows: [][]Cell{
			{C("daily", gotermstyle.CodeCyan), C("Stretch for ten minutes before the first call of the day", ""), C("2d late", gotermstyle.CodeRed), C("↳ todo done stretch", gotermstyle.CodeCommand)},
			{C("", ""), C("Water the plants", ""), C("today", gotermstyle.CodeYellow), C("↳ todo done 41", gotermstyle.CodeCommand)},
			{C("weekly", gotermstyle.CodeCyan), C("Read the pull requests waiting on a second look from the queue", ""), C("overdue 9d", gotermstyle.CodeRed), C("↳ todo done pr-queue", gotermstyle.CodeCommand)},
			{C("read", gotermstyle.CodeCyan), C("Finish the chapter on reconciliation loops", ""), C("", ""), C("↳ todo log 12", gotermstyle.CodeCommand)},
		},
	}
}

func TestColoredCellsLineUpAtEachWidth(t *testing.T) {
	for _, width := range []int{80, 160} {
		rows := dueRows()
		rows.Width = width
		colored := rows.Lines(gotermstyle.NewPalette(true))
		plain := rows.Lines(gotermstyle.NewPalette(false))
		if !strings.Contains(strings.Join(colored, "\n"), "\x1b[") {
			t.Fatalf("at %d the colored table carries no escape", width)
		}
		if len(colored) != len(rows.Rows) {
			t.Fatalf("at %d the table printed %d lines for %d rows", width, len(colored), len(rows.Rows))
		}
		handleAt := -1
		for i, line := range colored {
			stripped := ansi.Strip(line)
			if stripped != plain[i] {
				t.Errorf("at %d line %d stripped of color reads\n%q\nand printed plain reads\n%q", width, i, stripped, plain[i])
			}
			if got := lipgloss.Width(stripped); got > width {
				t.Errorf("at %d line %d is %d columns wide", width, i, got)
			}
			handle := rows.Rows[i][3].Text
			at := strings.Index(stripped, handle)
			if at < 0 {
				t.Fatalf("at %d line %d lost its whole handle %q: %q", width, i, handle, stripped)
			}
			column := lipgloss.Width(stripped[:at])
			if handleAt >= 0 && column != handleAt {
				t.Errorf("at %d line %d starts its handle at column %d, and the line above at %d", width, i, column, handleAt)
			}
			handleAt = column
		}
	}
}

func TestNoColorOutranksForceColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "1")
	rows := dueRows()
	rows.Columns[1].Title = "owed"
	rows.Width = 80
	var out bytes.Buffer
	if err := rows.Print(&out, gotermstyle.PaletteFor(&out)); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "\x1b") {
		t.Errorf("under NO_COLOR the table printed an escape:\n%q", out.String())
	}
}
