package gotermboard

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/datapointchris/gotermstyle"
)

// logBoard is a board shaped as a log is: a group a day, four columns, a summary under every row,
// a document on every row, read from its newest row, with a strip that moves.
type logBoard struct {
	frame    Frame
	revision uint64
	frames   int
}

func (b *logBoard) Sources() []Refresher { return nil }
func (b *logBoard) Groupings() []string  { return nil }
func (b *logBoard) Tails() bool          { return true }
func (b *logBoard) TwoLines() bool       { return true }
func (b *logBoard) Look(int) Look        { return Look{Rules: true, NoRowChevrons: true} }

func (b *logBoard) Frame(time.Time, string) Frame {
	b.frames++
	frame := b.frame
	frame.Revision = b.revision
	return frame
}

func (b *logBoard) MoveEvery() time.Duration { return time.Second }

func (b *logBoard) StripAt(now time.Time) []Line {
	return []Line{{C("read "+strconv.Itoa(now.Second())+"s ago", "")}}
}

const logRowsPerGroup = 500

// logDocumentLines is how many lines each row's document holds, built again on every call.
var logDocumentLines = 3

func newLogBoard(rows int) *logBoard {
	kinds := []struct{ text, code string }{{"changed", Green}, {"queued", Yellow}, {"failed", Red}, {"closed", ""}}
	var groups []Group
	for at := 0; at < rows; at += logRowsPerGroup {
		g := Group{Key: "day-" + strconv.Itoa(at), Title: Line{C("2026-11-"+strconv.Itoa(1+at/logRowsPerGroup%28), Bold)}, Counted: true}
		for i := at; i < min(at+logRowsPerGroup, rows); i++ {
			kind := kinds[i%len(kinds)]
			key := "row-" + strconv.Itoa(i)
			g.Rows = append(g.Rows, Row{
				Key: key,
				Cells: []Cell{
					C(fmt.Sprintf("11-%02d %02d:%02d:%02d", 1+i/logRowsPerGroup%28, i/3600%24, i/60%60, i%60), ""),
					C("repo-"+strconv.Itoa(i%60), Cyan), C(kind.text, kind.code),
					C("a passage to rewrite, a passage to rewrite, a passage to rewrite "+strconv.Itoa(i), ""),
				},
				Under: C(key+"  queue · inbox-problems-fix · 6m9s · uncached input 36, cache write 139k, cache read 1.3M, output 13.2k", ""),
				Text:  "50a62674-84a7-4fc9-bc2f-cc0cf0b4cde4 " + key,
				Document: func(int) []Line {
					lines := []Line{{C("# "+key, Bold)}, nil, {C(`{"a": 1}`, Cyan)}}
					for i := len(lines); i < logDocumentLines; i++ {
						lines = append(lines, Line{C("line "+strconv.Itoa(i)+" of a long document, built again on every call", "")})
					}
					return lines
				},
				ReaderTitle: key,
			})
		}
		g.Count = len(g.Rows)
		groups = append(groups, g)
	}
	return &logBoard{frame: Frame{
		Columns: []Column{{Title: "WHEN"}, {Title: "REPO"}, {Title: "KIND"}, {Title: "WHAT"}},
		Groups:  groups,
	}}
}

func newLogModel(rows int) (*Model, *logBoard) {
	b := newLogBoard(rows)
	m := NewModel(b, gotermstyle.NewPalette(true))
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	m.Update(tickMsg(time.Now()))
	m.View()
	return m, b
}

var redrawSizes = []int{500, 5000, 50000}

// BenchmarkRedraw times each thing a board does and the draw after it, at three sizes. A step that
// reads no new frame should cost the same at every size.
func BenchmarkRedraw(b *testing.B) {
	keys := func(a, z tea.KeyPressMsg) func(*Model, *logBoard, int) {
		return func(m *Model, _ *logBoard, i int) {
			if i%2 == 0 {
				m.Update(a)
			} else {
				m.Update(z)
			}
		}
	}
	steps := []struct {
		name string
		// long gives every row a document of 2,000 lines, and only runs the step at one size.
		long  bool
		only  int
		setup func(*Model, *logBoard)
		step  func(m *Model, board *logBoard, i int)
	}{
		{name: "view", step: func(*Model, *logBoard, int) {}},
		{name: "key", step: keys(upKey, downKey)},
		{name: "page", step: keys(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})},
		{name: "ends", step: keys(typedKey("g"), typedKey("G"))},
		{
			name:  "tickHeld",
			setup: func(m *Model, board *logBoard) { board.revision = 1; m.Update(tickMsg(time.Now())) },
			step:  func(m *Model, _ *logBoard, _ int) { m.Update(tickMsg(time.Now())) },
		},
		{name: "tickNew", step: func(m *Model, _ *logBoard, _ int) { m.Update(tickMsg(time.Now())) }},
		{name: "beat", step: func(m *Model, _ *logBoard, _ int) {
			m.Update(moveMsg{model: m, clock: m.moveClock, at: time.Now()})
		}},
		{name: "unhandled", step: func(m *Model, _ *logBoard, _ int) { m.Update(struct{}{}) }},
		{name: "resize", step: func(m *Model, _ *logBoard, i int) {
			m.Update(tea.WindowSizeMsg{Width: 200 - 40*(i%2), Height: 50})
		}},
		{name: "filter", step: func(m *Model, _ *logBoard, i int) {
			if i%2 == 0 {
				m.setFilter("row-7")
			} else {
				m.setFilter("")
			}
		}},
		{name: "open", step: func(m *Model, _ *logBoard, _ int) { m.Update(spaceKey) }},
		{
			name:  "everyOpen",
			setup: func(m *Model, _ *logBoard) { m.Update(typedKey("p")); m.ToggleEveryRow() },
			step:  keys(upKey, downKey),
		},
		{
			name:  "everyOpenView",
			setup: func(m *Model, _ *logBoard) { m.Update(typedKey("p")); m.ToggleEveryRow() },
			step:  func(*Model, *logBoard, int) {},
		},
		{
			name:  "everyOpenTickNew",
			setup: func(m *Model, _ *logBoard) { m.Update(typedKey("p")); m.ToggleEveryRow() },
			step:  func(m *Model, _ *logBoard, _ int) { m.Update(tickMsg(time.Now())) },
		},
		{name: "noPanel", setup: func(m *Model, _ *logBoard) { m.Update(typedKey("p")) }, step: keys(upKey, downKey)},
		{name: "panelLong", long: true, step: keys(upKey, downKey)},
		{name: "panelLongView", long: true, step: func(*Model, *logBoard, int) {}},
		{
			name: "everyOpenLongView", long: true, only: 500,
			setup: func(m *Model, _ *logBoard) { m.Update(typedKey("p")); m.ToggleEveryRow() },
			step:  func(*Model, *logBoard, int) {},
		},
	}
	for _, step := range steps {
		for _, rows := range redrawSizes {
			if step.only != 0 && rows != step.only {
				continue
			}
			b.Run(step.name+"/"+strconv.Itoa(rows), func(b *testing.B) {
				logDocumentLines = 3
				if step.long {
					logDocumentLines = 2000
				}
				m, board := newLogModel(rows)
				if step.setup != nil {
					step.setup(m, board)
				}
				step.step(m, board, 1)
				m.View()
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					step.step(m, board, i)
					m.View()
				}
			})
		}
	}
}

// newLogTabs is tabs tabs of rows rows each, every one drawn once so it holds a frame.
func newLogTabs(tabs, rows int) *Tabs {
	list := make([]Tab, 0, tabs)
	for i := range tabs {
		list = append(list, Tab{Name: "tab" + strconv.Itoa(i), Board: newLogBoard(rows)})
	}
	t, err := NewTabs(list, gotermstyle.NewPalette(true), "")
	if err != nil {
		panic(err)
	}
	t.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	for i := range tabs {
		t.show(i)
	}
	t.show(0)
	t.Update(tickMsg(time.Now()))
	t.View()
	return t
}

// BenchmarkTabs times what twelve tabs of 5,200 rows pay for the eleven that are hidden.
func BenchmarkTabs(b *testing.B) {
	steps := []struct {
		name  string
		setup func(*Tabs)
		step  func(t *Tabs, i int)
	}{
		{name: "view", step: func(*Tabs, int) {}},
		{name: "resize", step: func(t *Tabs, i int) { t.Update(tea.WindowSizeMsg{Width: 200 - 40*(i%2), Height: 50}) }},
		{name: "forwarded", step: func(t *Tabs, _ int) { t.Update(struct{}{}) }},
		{name: "lockedView", setup: func(t *Tabs) { t.lockTo(&EntityRef{Kind: "repo", ID: "held by no row"}) }, step: func(*Tabs, int) {}},
	}
	for _, step := range steps {
		b.Run(step.name, func(b *testing.B) {
			logDocumentLines = 3
			t := newLogTabs(12, 5200)
			if step.setup != nil {
				step.setup(t)
			}
			step.step(t, 1)
			t.View()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				step.step(t, i)
				t.View()
			}
		})
	}
}
