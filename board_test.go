package gotermboard

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermstyle"
)

func TestAFailedReadKeepsTheLastGoodValueAndSaysHowOldItIs(t *testing.T) {
	fail := false
	source := NewSource("queue", time.Second, func(context.Context) (string, string, error) {
		if fail {
			return "", "", errors.New("the backend is restarting")
		}
		return "good", "", nil
	})
	source.Refresh(context.Background())
	fail = true
	source.Refresh(context.Background())
	if got := source.Value(); got != "good" {
		t.Fatalf("value = %q after a failed read, want the last good one", got)
	}
	line := ReadStatus([]Status{source.Status(), source.Status()}, time.Now().Add(time.Minute))
	if got := strings.Count(line.Text(), "the backend is restarting"); got != 1 {
		t.Errorf("the strip says the reason %d times: %q", got, line.Text())
	}
	if !strings.Contains(line.Text(), "queue, queue 1m old: ") {
		t.Errorf("the strip does not name the failed reads and their age: %q", line.Text())
	}
}

func TestTheStripSaysOneThingWhileEveryReadIsOnSchedule(t *testing.T) {
	now := time.Now()
	on := Status{Name: "one", ReadAt: now.Add(-3 * time.Second), every: time.Minute}
	older := Status{Name: "two", ReadAt: now.Add(-30 * time.Second), every: time.Minute}
	if got := ReadStatus([]Status{on, older}, now).Text(); got != "read 3s ago" {
		t.Errorf("on schedule: %q", got)
	}
	late := Status{Name: "late", ReadAt: now.Add(-5 * time.Minute), every: time.Minute, Reading: true}
	asleep := Status{Name: "hosts", ReadAt: now, every: time.Minute, Note: "one did not answer"}
	line := ReadStatus([]Status{on, late, asleep}, now)
	if got := line.Text(); got != "read 0s ago · late 5m old, still reading · hosts: one did not answer" {
		t.Errorf("late and a note: %q", got)
	}
	if line[2].Code != Yellow || line[4].Code != "" {
		t.Errorf("a late read should be yellow and a note plain: %+v", line)
	}
}

type fixedBoard struct{ frame Frame }

func (b fixedBoard) Sources() []Refresher          { return nil }
func (b fixedBoard) Groupings() []string           { return nil }
func (b fixedBoard) Frame(time.Time, string) Frame { return b.frame }

type groupedBoard struct{ fixedBoard }

func (b groupedBoard) Groupings() []string { return []string{"one", "two"} }

func TestTheFooterListsOnlyTheKeysThisViewCanUse(t *testing.T) {
	plain := fixedBoard{Frame{Columns: []Column{{Title: "NAME"}}, Groups: []Group{{Key: "g", Rows: []Row{{Key: "r", Cells: []Cell{C("x", "")}}}}}}}
	withLines := fixedBoard{Frame{Columns: plain.frame.Columns, Groups: []Group{{Key: "g", Groups: []Group{{Key: "inner", Rows: []Row{{Key: "r", Under: C("the first line", ""), Detail: []Cell{C("more", "")}}}}}}}}}
	openRow := fixedBoard{Frame{Columns: plain.frame.Columns, Groups: []Group{{Key: "g", Rows: []Row{{Key: "r", Cells: []Cell{C("x", "")}, Detail: []Cell{C("more", "")}, StartsOpen: true}}}}}}
	fitsWhole := fixedBoard{Frame{Columns: []Column{{Title: "NAME"}, {Title: "DOING"}}, Groups: []Group{{Key: "g", Rows: []Row{
		{Key: "r", Cells: []Cell{C("x", ""), C("fits", "")}, Under: C("fits", ""), UnderWhereCut: true},
	}}}}}
	for _, test := range []struct {
		name         string
		board        Board
		width        int
		shown, unset []string
	}{
		{"nothing to offer", plain, 120, []string{"j/k move", "? keys"}, []string{"v group", "f summaries", "p panel"}},
		{"the cursor on a row holding neither Detail nor Document", plain, 120, nil, []string{"enter open", "space fold"}},
		{"every key has a use", groupedBoard{withLines}, PanelFrom, []string{"enter open", "space fold", "v group", "f summaries", "p panel"}, nil},
		{"a 60-column terminal with the cursor on an open row", openRow, 60, []string{"? keys", "q quit"}, nil},
		{"a summary only where cut, on a row the last column shows whole", fitsWhole, 120, nil, []string{"f summaries"}},
	} {
		screen := Once(test.board, gotermstyle.NewPalette(true), test.width, 10)
		for _, want := range test.shown {
			if !strings.Contains(screen, want) {
				t.Errorf("%s: no %q in %q", test.name, want, screen)
			}
		}
		for _, unwanted := range test.unset {
			if strings.Contains(screen, unwanted) {
				t.Errorf("%s: %q is listed in %q", test.name, unwanted, screen)
			}
		}
	}
}

// A view repeating its last column's text in Under, as a live table of agents repeats what each
// is doing, asks for the summary only where that column cut the text.
func TestASummaryAskedForOnlyWhereCutDrawsUnderTheCutRowAlone(t *testing.T) {
	cut := strings.Repeat("cut ", 40)
	frame := Frame{Columns: []Column{{Title: "NAME"}, {Title: "DOING"}}, Groups: []Group{{Key: "g", Rows: []Row{
		{Key: "fits", Cells: []Cell{C("fits", ""), C("whole", "")}, Under: C("whole", ""), UnderWhereCut: true},
		{Key: "cut", Cells: []Cell{C("cut", ""), C(cut, "")}, Under: C(cut, ""), UnderWhereCut: true},
	}}}}
	m := NewModel(fixedBoard{frame}, gotermstyle.NewPalette(false))
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 12})
	m.Update(typedKey("f"))
	lines := strings.Split(m.screen(), "\n")
	var under []int
	for i, line := range lines {
		if strings.Contains(line, "└─") {
			under = append(under, i)
		}
	}
	if len(under) != 1 || under[0] == 0 || !strings.Contains(lines[under[0]-1], "cut cut") {
		t.Fatalf("want one └─ line, under the row the column cut:\n%s", strings.Join(lines, "\n"))
	}
}

func TestNoLineOfAFrameIsWiderThanItsScreen(t *testing.T) {
	long := strings.Repeat("a long value that would wrap ", 12)
	frame := Frame{
		Strip:   []Line{{C(long, Bold)}},
		Columns: []Column{{Title: "", Max: 30}, {Title: "KIND", Max: 12}, {Title: "DOING"}},
		Groups: []Group{{
			Key: "working", Title: Line{C("WORKING", BoldYellow)}, Count: 1, Counted: true, Summary: Line{C(long, "")},
			Rows: []Row{
				{Key: "span", Span: Line{C(long, "")}, Detail: []Cell{C(long, "")}},
				{Key: "cells", Symbol: C("●", Yellow), Cells: []Cell{C(long, ""), C(long, Cyan), C(long, "")}, Detail: []Cell{C(long, "")}},
				{Key: "wraps", Cells: []Cell{C("short", ""), C("kind", "")}, WrapLast: Line{C(long, Green)}},
			},
			Groups: []Group{{
				Key: "totals", Title: Line{C(long, Bold)}, Cells: []Cell{C(long, "")}, WrapLast: Line{C(long, Green), C(long, Yellow)},
				Below: []Line{{C(long, ""), C(long, Blue)}},
			}, {
				Key: "spanning", Title: Line{C("1544", Cyan)}, Span: Line{C(long, Bold)}, Beside: Items("", "alpha", "beta", "gamma"),
				Cells: []Cell{C("kind", "")}, WrapLast: Line{C(long, Green)}, Under: C(long, ""),
				Document: func(width int) []Line { return []Line{{C(strings.Repeat("x", width), "")}} },
			}},
		}},
	}
	for _, size := range []struct{ width, height int }{{188, 88}, {120, 40}, {PanelFrom, 20}, {60, 10}} {
		screen := Once(fixedBoard{frame}, gotermstyle.NewPalette(true), size.width, size.height)
		lines := strings.Split(screen, "\n")
		if len(lines) > size.height {
			t.Errorf("%dx%d: %d lines", size.width, size.height, len(lines))
		}
		for _, line := range lines {
			if w := ansi.StringWidth(line); w > size.width {
				t.Errorf("%dx%d: a line %d wide: %q", size.width, size.height, w, ansi.Strip(line))
			}
		}
	}
}

// countedBoard draws what its one source last read, and counts every read it makes.
type countedBoard struct {
	source *Source[string]
	reads  *atomic.Int64
}

func newCountedBoard(answer *atomic.Value) countedBoard {
	reads := &atomic.Int64{}
	return countedBoard{reads: reads, source: NewSource("answer", 0, func(context.Context) (string, string, error) {
		reads.Add(1)
		return answer.Load().(string), "", nil
	})}
}

func (b countedBoard) Sources() []Refresher { return []Refresher{b.source} }
func (b countedBoard) Groupings() []string  { return nil }
func (b countedBoard) Frame(time.Time, string) Frame {
	return Frame{Columns: []Column{{Title: "NAME"}, {Title: "VALUE"}}, Groups: []Group{{
		Key: "g", Title: Line{C("GROUP", Bold)},
		Rows: []Row{{Key: "first", Cells: []Cell{C("first", ""), C(b.source.Value(), "")}}, {Key: "second", Cells: []Cell{C("second", ""), C("still", "")}}},
	}}}
}

// runReads runs what a key or a reveal returned, leaving out the clock, which waits.
func runReads(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, each := range batch {
			runReads(each)
		}
	}
}

func press(t *Tabs, key tea.KeyPressMsg) tea.Cmd {
	_, cmd := t.Update(key)
	return cmd
}

var (
	tabKey   = tea.KeyPressMsg{Code: tea.KeyTab}
	downKey  = tea.KeyPressMsg{Code: 'j', Text: "j"}
	upKey    = tea.KeyPressMsg{Code: 'k', Text: "k"}
	enterKey = tea.KeyPressMsg{Code: tea.KeyEnter}
	spaceKey = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
)

func twoTabs(t *testing.T, answer *atomic.Value) (*Tabs, countedBoard, countedBoard) {
	t.Helper()
	one, two := newCountedBoard(answer), newCountedBoard(answer)
	tabs, err := NewTabs([]Tab{{Name: "one", Board: one}, {Name: "two", Board: two}}, gotermstyle.NewPalette(false), "")
	if err != nil {
		t.Fatal(err)
	}
	return tabs, one, two
}

func TestAHiddenTabStartsNoReadAndAReturningTabDrawsItsLastReadThenReads(t *testing.T) {
	answer := &atomic.Value{}
	answer.Store("before")
	tabs, one, two := twoTabs(t, answer)
	// A tick's reads are in a batch with the clock, which waits, so the showing tab's
	// claim is settled here by reading by hand.
	tickShowing := func(showing countedBoard) {
		tabs.Update(tickMsg(time.Now().Add(time.Hour)))
		showing.source.Refresh(context.Background())
	}
	runReads(tabs.active().reveal(time.Now()))
	tickShowing(one)
	if got := two.reads.Load(); got != 0 {
		t.Fatalf("the hidden tab read %d times", got)
	}

	runReads(press(tabs, tabKey))
	settled := one.reads.Load()
	tickShowing(two)
	if got := one.reads.Load(); got != settled {
		t.Errorf("the tab that went hidden read again: %d then %d", settled, got)
	}

	answer.Store("after")
	returned := press(tabs, tabKey)
	if got := tabs.View().Content; !strings.Contains(got, "before") {
		t.Errorf("a returning tab should draw its last read first:\n%s", got)
	}
	runReads(returned)
	tabs.Update(refreshedMsg{})
	if got := tabs.View().Content; !strings.Contains(got, "after") {
		t.Errorf("a returning tab should read at once:\n%s", got)
	}
}

func TestEachTabKeepsItsCursorAndFolds(t *testing.T) {
	answer := &atomic.Value{}
	answer.Store("value")
	tabs, _, _ := twoTabs(t, answer)
	tabs.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	runReads(tabs.active().reveal(time.Now()))
	press(tabs, downKey)
	press(tabs, tabKey)
	press(tabs, upKey)
	press(tabs, enterKey)
	if got := tabs.View().Content; !strings.Contains(got, "[two]") || !strings.Contains(got, "▸ GROUP") {
		t.Errorf("the second tab should have folded its group:\n%s", got)
	}
	press(tabs, tabKey)
	got := tabs.View().Content
	if !strings.Contains(got, "[one]") || !strings.Contains(got, "▾ GROUP") {
		t.Errorf("the first tab should keep its group open:\n%s", got)
	}
	if !strings.Contains(got, "›  second") {
		t.Errorf("the first tab should keep its cursor on the second row:\n%s", got)
	}
}

func TestOneTabDrawsNoTabLineAndAnUnknownTabIsAnError(t *testing.T) {
	answer := &atomic.Value{}
	answer.Store("value")
	single, err := NewTabs([]Tab{{Name: "only", Board: newCountedBoard(answer)}}, gotermstyle.NewPalette(false), "only")
	if err != nil {
		t.Fatal(err)
	}
	if got := single.View().Content; strings.Contains(got, "only") || strings.Contains(got, "tab next view") {
		t.Errorf("one tab should have no line and no key:\n%s", got)
	}
	_, err = NewTabs([]Tab{{Name: "only", Board: newCountedBoard(answer)}}, gotermstyle.NewPalette(false), "other")
	if err == nil || !strings.Contains(err.Error(), "only") {
		t.Errorf("an unknown tab should name the tabs: %v", err)
	}
}

// changingBoard holds a row whose state and age the test moves.
type changingBoard struct{ state, age string }

func (b *changingBoard) Sources() []Refresher { return nil }
func (b *changingBoard) Groupings() []string  { return nil }
func (b *changingBoard) Frame(time.Time, string) Frame {
	group := Group{Key: "g", Rows: []Row{{Key: "row", Cells: []Cell{C("row", ""), C(b.state, ""), Ticking(b.age, "")}}}}
	return Frame{Columns: []Column{{Title: "NAME"}, {Title: "STATE"}, {Title: "AGE"}}, Groups: []Group{group}}
}

func TestARowThatChangedWearsTheCueForAWhileAndAnAgeNeverLightsIt(t *testing.T) {
	board := &changingBoard{state: "running", age: "1s"}
	now := time.Now()
	m := newModel(board, gotermstyle.NewPalette(false), context.Background(), func() {})
	m.width, m.height = 100, 20
	m.reveal(now)
	m.cursor, m.placed = "g", true
	worn := func() bool { return strings.Contains(m.screen(), CueGlyph) }
	if worn() {
		t.Fatalf("a first frame marks nothing:\n%s", m.screen())
	}
	board.state = "filled in by a second pass"
	m.rebuild(now.Add(settleFor))
	if worn() {
		t.Errorf("a frame while the first reads settle marks nothing:\n%s", m.screen())
	}
	board.age = "2s"
	m.rebuild(now.Add(settleFor + time.Second))
	if worn() {
		t.Errorf("an age moving marks nothing:\n%s", m.screen())
	}
	changed := now.Add(settleFor + 2*time.Second)
	board.state = "failed"
	m.rebuild(changed)
	if !worn() {
		t.Errorf("a state moving should wear the cue:\n%s", m.screen())
	}
	m.rebuild(changed.Add(CueFor - time.Second))
	if !worn() {
		t.Errorf("the cue should last:\n%s", m.screen())
	}
	m.rebuild(changed.Add(CueFor))
	if worn() {
		t.Errorf("the cue should end:\n%s", m.screen())
	}
	board.state = "running"
	m.reveal(now.Add(3 * time.Minute))
	if worn() {
		t.Errorf("a return marks nothing, whatever moved while it was hidden:\n%s", m.screen())
	}
}

func TestGroupCellsShareTheRowsColumnsAndALastCellWraps(t *testing.T) {
	bar := Line{C(strings.Repeat("▰", 30), Green), C(strings.Repeat("▱", 30), "")}
	frame := Frame{
		Columns: []Column{{Title: ""}, {Title: "DONE", Right: true}, {Title: "BAR"}},
		Groups: []Group{{
			Key: "g", Title: Line{C("TOTAL", Bold)}, Cells: []Cell{C("12", "")}, WrapLast: bar,
			Below: []Line{{C("the name", "")}},
			Rows:  []Row{{Key: "r", Cells: []Cell{C("a row", ""), C("3", "")}, WrapLast: Line{C("▰▰", Green)}}},
		}},
	}
	screen := Once(fixedBoard{frame}, gotermstyle.NewPalette(false), 40, 12)
	lines := strings.Split(screen, "\n")
	column := func(line, text string) int {
		before, _, _ := strings.Cut(line, text)
		return ansi.StringWidth(before)
	}
	titles, total, continued := lines[0], lines[1], lines[2]
	if at := column(titles, "BAR"); at != column(total, "▰") || at != column(continued, "▰") {
		t.Errorf("the bar should start at its column on every line:\n%s", screen)
	}
	if done, rest := strings.Count(screen, "▰"), strings.Count(screen, "▱"); done != 32 || rest != 30 {
		t.Errorf("the bars should keep every cell, kept %d and %d:\n%s", done, rest, screen)
	}
	if !strings.Contains(lines[4], "the name") {
		t.Errorf("the line below should follow the header and its bar:\n%s", screen)
	}
}

func TestAnItemsLineEndsInTheCountOfWhatDidNotFit(t *testing.T) {
	frame := Frame{Columns: []Column{{Title: ""}}, Groups: []Group{{
		Key: "g", Title: Line{C("head", "")}, Below: []Line{Items("", "alpha", "beta", "gamma", "delta")},
	}}}
	drawn := func(width int) string { return Once(fixedBoard{frame}, gotermstyle.NewPalette(false), width, 6) }
	if got := drawn(60); !strings.Contains(got, "alpha, beta, gamma, delta") {
		t.Errorf("every item should show where they fit:\n%s", got)
	}
	if got := drawn(22); !strings.Contains(got, "alpha, beta, +2") {
		t.Errorf("the items that do not fit should become a count:\n%s", got)
	}
	if got := drawn(5); !strings.Contains(got, "+4") {
		t.Errorf("no room for an item should leave the count alone:\n%s", got)
	}
}

func TestAFilterReachesAGroupTheViewDoesNotShow(t *testing.T) {
	frame := Frame{Columns: []Column{{Title: ""}, {Title: "N"}}, Groups: []Group{
		{Key: "shown", Title: Line{C("first", "")}, Cells: []Cell{C("1", "")}},
		{Key: "beyond", Title: Line{C("second", "")}, Cells: []Cell{C("2", "")}, FilterOnly: true, Text: "the long text"},
		{Key: "labels", Title: Line{C("long text", "")}, Cells: []Cell{C("3", "")}, Heading: true},
	}}
	m := NewModel(fixedBoard{frame}, gotermstyle.NewPalette(false))
	if got := m.screen(); strings.Contains(got, "second") {
		t.Errorf("a group past the view should stay out of the table:\n%s", got)
	}
	m.setFilter("long text")
	got := m.screen()
	if !strings.Contains(got, "second") || strings.Contains(got, "first") {
		t.Errorf("the filter should find the group past the view and nothing else:\n%s", got)
	}
	if slices.ContainsFunc(m.nodes(), func(n node) bool { return n.key == "labels" }) {
		t.Errorf("a heading's own words should match nothing:\n%s", got)
	}
}

func TestAStillHeaderTakesNoCursorAndStaysAboveWhatScrolls(t *testing.T) {
	var rows []Row
	for i := range 30 {
		rows = append(rows, Row{Key: "r" + strconv.Itoa(i), Cells: []Cell{C("row "+strconv.Itoa(i), ""), C("x", "")}})
	}
	frame := Frame{StartAt: "r3", Columns: []Column{{Title: ""}, {Title: "COUNT"}}, Groups: []Group{
		{Key: "total", Title: Line{C("TOTAL", "")}, Cells: []Cell{C("9", "")}, Still: true, Gap: true},
		{Key: "g", Title: Line{C("group", "")}, Cells: []Cell{C("9", "")}, Rows: rows},
	}}
	m := NewModel(fixedBoard{frame}, gotermstyle.NewPalette(false))
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	if m.Cursor() != "r3" {
		t.Errorf("the cursor should start on the key the frame names, not %q", m.Cursor())
	}
	m.move(len(rows))
	got := m.screen()
	for _, want := range []string{"COUNT", "TOTAL", "row 29"} {
		if !strings.Contains(got, want) {
			t.Errorf("scrolled to the bottom, the screen should hold %q:\n%s", want, got)
		}
	}
	m.move(-len(rows) - 5)
	if m.Cursor() != "g" {
		t.Errorf("the cursor should stop short of the still header, on %q", m.Cursor())
	}
}

type followingBoard struct {
	fixedBoard
	heard *[]string
}

func (b followingBoard) CursorOn(key string) { *b.heard = append(*b.heard, key) }

func TestACursorBoardHearsEachKeyTheCursorLandsOnOnce(t *testing.T) {
	frame := Frame{Columns: []Column{{Title: ""}}, Groups: []Group{{Key: "g", Rows: []Row{
		{Key: "one", Cells: []Cell{C("one", "")}}, {Key: "two", Cells: []Cell{C("two", "")}},
	}}}}
	var heard []string
	m := NewModel(followingBoard{fixedBoard{frame}, &heard}, gotermstyle.NewPalette(false))
	m.Update(refreshedMsg{})
	m.Update(downKey)
	m.Update(refreshedMsg{})
	if want := "one,two"; strings.Join(heard, ",") != want {
		t.Errorf("heard %v, want %s", heard, want)
	}
}

func TestAPanelLineWrapsAtSpacesKeepingEachCellsColor(t *testing.T) {
	line := Line{C("green words here ", Green), C("red words", Red), C(" longerthanthepanel", "")}
	pieces := wrapWords(line, 11)
	var joined []string
	for _, piece := range pieces {
		if piece.width() > 11 {
			t.Errorf("%q is wider than the panel", piece.Text())
		}
		joined = append(joined, piece.Text())
	}
	if got := strings.Join(joined, ""); strings.ReplaceAll(got, " ", "") != strings.ReplaceAll(line.Text(), " ", "") {
		t.Errorf("wrapping should keep every word in order, got %q", joined)
	}
	if pieces[0].Text() != "green words" {
		t.Errorf("a word ending on the panel's last column should stay on its line, with no space past it: %q", joined)
	}
	pieces = wrapWords(line[:2], 12)
	if last := pieces[len(pieces)-1]; last[len(last)-1].Code != Red {
		t.Errorf("the last word should keep its cell's color: %+v", last)
	}
}

func TestASpanGroupDrawsItsSpanWholeWithItsCellsAndBesideOnTheLineBelow(t *testing.T) {
	title := strings.Repeat("title ", 10) + "end"
	frame := Frame{
		Columns: []Column{{Title: "", Min: 28}, {Title: "DONE", Right: true}, {Title: "BAR"}},
		Groups: []Group{{Key: "tier", Title: Line{C("high", Bold)}, Cells: []Cell{C("9/9", "")}, Groups: []Group{{
			Key: "one", Title: Line{C("1544", Cyan)}, Span: Line{C(title, Bold)}, Beside: Items("", "alpha", "beta", "gamma", "delta"),
			Cells: []Cell{C("5/7", "")}, WrapLast: Line{C("▰▰▱", Green)},
			Document: func(width int) []Line { return []Line{{C("read at "+strconv.Itoa(width), "")}} },
		}}}},
	}
	lines := strings.Split(Once(fixedBoard{frame}, gotermstyle.NewPalette(false), 70, 12), "\n")
	want := []string{
		"     1544  " + strings.TrimSpace(strings.Repeat("title ", 10)),
		"           end",
		"           alpha, beta, +2     5/7  ▰▰▱",
		"           read at 59",
	}
	if got := lines[2:6]; strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got %q, want %q", got, want)
	}
	if done, cells := strings.Index(lines[0], "DONE")+len("DONE"), strings.Index(want[2], "5/7")+len("5/7"); done != cells {
		t.Errorf("the cells should stay on the shared columns: %q over %q", lines[0], want[2])
	}
}

func TestBracketsJumpGroupsAtTheCursorsDepthOrShallower(t *testing.T) {
	heading := func(key string, groups ...Group) Group {
		return Group{Key: key, Title: Line{C(key, "")}, Groups: groups}
	}
	inner := heading("a1")
	inner.Rows = []Row{{Key: "row", Cells: []Cell{C("row", "")}}}
	frame := Frame{StartAt: "row", Columns: []Column{{Title: "NAME"}}, Groups: []Group{heading("a", inner, heading("a2")), heading("b")}}
	m := NewModel(fixedBoard{frame}, gotermstyle.NewPalette(false))
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	for _, step := range []struct {
		key  rune
		want string
	}{{'[', "a1"}, {']', "a2"}, {']', "b"}, {'[', "a"}, {']', "b"}, {']', "b"}} {
		m.Update(tea.KeyPressMsg{Code: step.key, Text: string(step.key)})
		if m.Cursor() != step.want {
			t.Fatalf("%c should land on %q, not %q", step.key, step.want, m.Cursor())
		}
	}
}

func TestTheHalfScreenKeysReadOpenTextOtherwiseMoveTheRows(t *testing.T) {
	var rows []Row
	var detail []Cell
	for i := range 40 {
		rows = append(rows, Row{Key: "r" + strconv.Itoa(i), Cells: []Cell{C("row "+strconv.Itoa(i), "")}})
		detail = append(detail, C("line "+strconv.Itoa(i), ""))
	}
	rows[0].Detail = detail
	frame := Frame{StartAt: "r0", Columns: []Column{{Title: "NAME"}}, Groups: []Group{{Key: "g", Title: Line{C("g", "")}, Rows: rows}}}
	m := NewModel(fixedBoard{frame}, gotermstyle.NewPalette(false))
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})

	m.Update(tea.KeyPressMsg{Code: 'J', Text: "J"})
	if m.top != 0 {
		t.Errorf("J should do nothing with no text open, but the screen moved to line %d", m.top)
	}
	m.Update(enterKey)
	top := m.top
	m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if m.Cursor() != "r0" || m.top <= top {
		t.Errorf("ctrl+d should read down the open text, leaving the cursor: cursor %q, top %d", m.Cursor(), m.top)
	}
	top = m.top
	m.Update(tea.KeyPressMsg{Code: 'J', Text: "J"})
	if m.top != top+1 {
		t.Errorf("J should scroll the open text one line, from %d to %d", top, m.top)
	}
	m.Update(enterKey)
	m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if m.Cursor() == "r0" {
		t.Error("ctrl+d should move the rows with no text open")
	}
}

func TestAGroupDrawsItsLinesAboveAndBelowItsHeaderAndARuleAfterIt(t *testing.T) {
	frame := Frame{Columns: []Column{{Title: ""}, {Title: "N"}}, Groups: []Group{
		{Key: "a", Title: Line{C("first", "")}, Cells: []Cell{C("1", "")}, Overline: RuleDouble, Underline: RuleThin, Rule: true},
		{Key: "b", Title: Line{C("second", "")}, Cells: []Cell{C("2", "")}},
	}}
	m := NewModel(fixedBoard{frame}, gotermstyle.NewPalette(false))
	lines := strings.Split(m.screen(), "\n")
	at := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, "first") })
	if at < 1 || at+2 >= len(lines) {
		t.Fatalf("the header sits where no lines can surround it:\n%s", m.screen())
	}
	if !strings.HasPrefix(lines[at-1], "══") || !strings.HasPrefix(lines[at+1], "──") || !strings.HasPrefix(lines[at+2], "──") {
		t.Errorf("want a double line above, then a thin line under and a rule after:\n%s", m.screen())
	}
}

func TestAChevronDrawsOnlyOnWhatOpensAndLeavesABulletItsCell(t *testing.T) {
	more := []Cell{C("more text", "")}
	bullet := C("●", "")
	frame := Frame{StartAt: "empty", Columns: []Column{{Title: ""}, {Title: "STATE"}}, Groups: []Group{
		{Key: "empty", Title: Line{C("EMPTY", "")}, Detail: more},
		{Key: "full", Title: Line{C("FULL", "")}, Rows: []Row{
			{Key: "opens", Symbol: bullet, Cells: []Cell{C("opens", ""), C("x", "")}, Detail: more},
			{Key: "plain", Symbol: bullet, Cells: []Cell{C("plain", ""), C("x", "")}},
			{Key: "marked", Symbol: bullet, Marked: true, Cells: []Cell{C("marked", ""), C("x", "")}, Detail: more},
			{Key: "span", Span: Line{C("span", "")}, Detail: more},
		}},
	}}
	m := NewModel(fixedBoard{frame}, gotermstyle.NewPalette(false))
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 14})
	screen := m.screen()
	for _, want := range []string{"  EMPTY", " ▾ FULL", "  ▸● opens", "   ● plain", " ◆▸● marked", "  ▸span"} {
		if !strings.Contains(screen, want) {
			t.Errorf("no %q in:\n%s", want, screen)
		}
	}
	lines := strings.Split(screen, "\n")
	columnsOf := func(name string) [2]int {
		at := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, name) })
		before, _, _ := strings.Cut(lines[at], "●")
		state, _, _ := strings.Cut(lines[at], " x")
		return [2]int{ansi.StringWidth(before), ansi.StringWidth(state)}
	}
	if opens, plain, marked := columnsOf("opens"), columnsOf("plain"), columnsOf("marked"); opens != plain || marked != plain {
		t.Errorf("a chevron or a mark moved a column: %v %v %v:\n%s", opens, plain, marked, screen)
	}

	m.Update(spaceKey)
	if got := m.screen(); got != screen {
		t.Errorf("space on a group holding nothing should change nothing:\n%s", got)
	}
	m.Update(downKey)
	m.Update(downKey)
	m.Update(spaceKey)
	if got := m.screen(); !strings.Contains(got, "▾● opens") || !strings.Contains(got, "more text") {
		t.Errorf("space should open the row in place:\n%s", got)
	}
	m.setFilter("opens")
	if got := m.screen(); !strings.Contains(got, "  FULL") {
		t.Errorf("a group a filter holds open cannot fold, so it draws no mark:\n%s", got)
	}
}

func TestAnOpenRowAndItsGroupsHoldTheTopWhileItsTextScrollsUnderThem(t *testing.T) {
	text := func(name string) Document {
		return func(int) []Line {
			lines := make([]Line, 0, 30)
			for i := range 30 {
				lines = append(lines, Line{C(name+" line "+strconv.Itoa(i), "")})
			}
			return lines
		}
	}
	row := func(key, words string) Row {
		return Row{Key: key, Cells: []Cell{C(words, "")}, Document: text(words), StartsOpen: true}
	}
	frame := Frame{StartAt: "one", Columns: []Column{{Title: ""}}, Groups: []Group{{
		Key: "outer", Title: Line{C("OUTER", "")},
		Groups: []Group{{Key: "inner", Title: Line{C("INNER", "")}, Rows: []Row{row("one", "first"), row("two", "second")}}},
	}}}
	m := NewModel(fixedBoard{frame}, gotermstyle.NewPalette(false))
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	top := func(lines int) []string { return strings.Split(m.screen(), "\n")[:lines] }

	m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	got := top(4)
	if !strings.Contains(got[0], "OUTER") || !strings.Contains(got[1], "INNER") || !strings.Contains(got[2], "› ") || !strings.Contains(got[2], "first") {
		t.Errorf("the groups and the cursor's row should hold the top:\n%s", strings.Join(got, "\n"))
	}
	if !strings.Contains(got[3], "first line 9") {
		t.Errorf("the band should cover lines rather than push them down, so line 9 shows under it:\n%s", strings.Join(got, "\n"))
	}

	m.ScrollBy(22)
	if got := top(3); !strings.Contains(got[2], "second") || strings.Contains(got[2], "›") {
		t.Errorf("the next row's text under the band should put that row in its place:\n%s", strings.Join(got, "\n"))
	}

	m.Update(tea.WindowSizeMsg{Width: 60, Height: 8})
	m.ScrollBy(9)
	if got := top(2); !strings.Contains(got[0], "INNER") || !strings.Contains(got[1], "second") {
		t.Errorf("a short screen should drop the outermost line first:\n%s", strings.Join(got, "\n"))
	}

	m.Update(downKey)
	if got := top(3); !strings.Contains(got[2], "› ") || !strings.Contains(got[2], "second") {
		t.Errorf("a cursor moved under the band should bring its row out below it:\n%s", m.screen())
	}
}

// movingBoard counts what each beat of its strip draws.
type movingBoard struct{ frames, documents, beats int }

func (b *movingBoard) Sources() []Refresher     { return nil }
func (b *movingBoard) Groupings() []string      { return nil }
func (b *movingBoard) MoveEvery() time.Duration { return 100 * time.Millisecond }
func (b *movingBoard) StripAt(time.Time) []Line {
	b.beats++
	return []Line{{C("beat "+strconv.Itoa(b.beats), "")}}
}

func (b *movingBoard) Frame(time.Time, string) Frame {
	b.frames++
	document := func(int) []Line {
		b.documents++
		return []Line{{C("text", "")}}
	}
	return Frame{Columns: []Column{{Title: "NAME"}}, Groups: []Group{{Key: "g", Rows: []Row{
		{Key: "r", Cells: []Cell{C("row", "")}, Document: document, StartsOpen: true},
	}}}}
}

func TestAMovingStripRedrawsAloneAndOnlyWhileItsTabShows(t *testing.T) {
	moving := &movingBoard{}
	still := fixedBoard{Frame{Columns: []Column{{Title: "NAME"}}}}
	tabs, err := NewTabs([]Tab{{Name: "moving", Board: moving}, {Name: "still", Board: still}}, gotermstyle.NewPalette(false), "")
	if err != nil {
		t.Fatal(err)
	}
	tabs.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	shown := tabs.models[0]
	if shown.reveal(time.Now()) == nil {
		t.Fatal("a moving board's tab should start its strip's clock when it shows")
	}
	tabs.View()
	frames, documents := moving.frames, moving.documents
	beat := moveMsg{model: shown, clock: shown.moveClock, at: time.Now()}
	if _, next := tabs.Update(beat); next == nil {
		t.Error("a beat should wait out the next")
	}
	if got := tabs.View().Content; !strings.Contains(got, "beat "+strconv.Itoa(moving.beats)) {
		t.Errorf("a beat should redraw the strip:\n%s", got)
	}
	if moving.frames != frames || moving.documents != documents {
		t.Errorf("a beat built %d frames and drew the table's text %d times", moving.frames-frames, moving.documents-documents)
	}

	press(tabs, tabKey)
	beats := moving.beats
	tabs.Update(beat)
	tabs.Update(moveMsg{model: shown, clock: shown.moveClock, at: time.Now()})
	tabs.View()
	if moving.beats != beats || shown.moving {
		t.Errorf("a hidden tab's strip moved %d times", moving.beats-beats)
	}
}

func TestThePanelGrowsToAReadingWidthWhereAFrameHasTextForItAndTheCursorStartsPastABareHeader(t *testing.T) {
	frame := func(detail []Cell) Frame {
		return Frame{Columns: []Column{{Title: ""}}, Groups: []Group{{Key: "top", Title: Line{C("TOP", "")}, Groups: []Group{
			{Key: "first", Title: Line{C("first", "")}, Detail: detail, Folded: true, Rows: []Row{{Key: "r", Cells: []Cell{C("r", "")}}}},
		}}}}
	}
	m := NewModel(fixedBoard{frame([]Cell{C("what it says", "")})}, gotermstyle.NewPalette(false))
	if m.Cursor() != "first" {
		t.Errorf("the cursor should start past a header with nothing to say, on %q", m.Cursor())
	}
	for _, size := range []struct{ width, panel int }{{188, 56}, {240, 72}, {334, 100}, {378, 100}} {
		m.Update(tea.WindowSizeMsg{Width: size.width, Height: 20})
		if got := m.panelWidth(); got != size.panel {
			t.Errorf("at %d columns the panel is %d wide, want %d", size.width, got, size.panel)
		}
	}
	m.Update(upKey)
	if got := m.panelWidth(); got != 100 {
		t.Errorf("the panel should stay put on a line with nothing to say, so the table does not jump: %d", got)
	}

	silent := NewModel(fixedBoard{frame(nil)}, gotermstyle.NewPalette(false))
	silent.Update(tea.WindowSizeMsg{Width: 378, Height: 20})
	if got := silent.screen(); strings.Contains(got, "│") || strings.Contains(got, "p panel") {
		t.Errorf("a frame with nothing for the panel should give the table its width:\n%s", got)
	}
}
