package gotermboard

import (
	"strings"
	"testing"
	"time"

	"github.com/datapointchris/gotermstyle"
)

// titledBoard is a fixedBoard whose snapshot holds one title.
type titledBoard struct{ fixedBoard }

type titleSnapshot struct {
	Title string `json:"title"`
}

func (b titledBoard) Snapshot(time.Time) titleSnapshot { return titleSnapshot{Title: "<b>&</b>"} }

func TestPrintTabPrintsTheSnapshotAsIndentedEscapedJSONOrOneFrameAtOnceSize(t *testing.T) {
	board := titledBoard{fixedBoard{Frame{Columns: []Column{{Title: "NAME"}}, Groups: []Group{{Key: "g", Rows: []Row{
		{Key: "one", Cells: []Cell{C("first", Red)}},
	}}}}}}
	pal := gotermstyle.NewPalette(true)

	var printed strings.Builder
	if err := PrintTab(&printed, true, board, pal, 188, 88); err != nil {
		t.Fatal(err)
	}
	if want := "{\n  \"title\": \"\\u003cb\\u003e\\u0026\\u003c/b\\u003e\"\n}\n"; printed.String() != want {
		t.Errorf("JSON should print the snapshot indented two spaces with HTML escaped, not:\n%s", printed.String())
	}

	t.Setenv("COLUMNS", "50")
	t.Setenv("LINES", "6")
	printed.Reset()
	if err := PrintTab(&printed, false, board, pal, 188, 88); err != nil {
		t.Fatal(err)
	}
	if want := Once(board, pal, 50, 6) + "\n"; printed.String() != want || !strings.Contains(want, "\x1b[") {
		t.Errorf("the frame should print in color at COLUMNS by LINES:\n%q\nnot:\n%q", want, printed.String())
	}
}
