# gotermboard

A self-refreshing terminal board for Go command-line tools, on Bubble Tea v2.

A program built on it has a tab per view. Each view is one table of folding groups
sharing one set of columns, with a side panel that follows the cursor. Each source
of data refreshes at its own pace, and the board draws its last good read with that
read's age, so a failing backend leaves the table drawn and says the read is old.

```go
type board struct{ jobs *gotermboard.Source[[]job] }

func newBoard() *board {
 return &board{jobs: gotermboard.NewSource("jobs", 5*time.Second, readJobs)}
}

func (b *board) Sources() []gotermboard.Refresher { return []gotermboard.Refresher{b.jobs} }
func (b *board) Groupings() []string              { return nil }

func (b *board) Frame(now time.Time, _ string) gotermboard.Frame {
 group := gotermboard.Group{Key: "jobs", Title: gotermboard.Line{gotermboard.C("JOBS", gotermboard.Bold)}}
 for _, j := range b.jobs.Value() {
  group.Rows = append(group.Rows, gotermboard.Row{Key: j.Name, Cells: []gotermboard.Cell{
   gotermboard.C(j.Name, ""),
   gotermboard.C(j.State, gotermboard.Cyan),
   gotermboard.Ticking(gotermboard.Ago(now.Sub(j.Started)), ""),
  }})
 }
 return gotermboard.Frame{
  Strip:   []gotermboard.Line{gotermboard.ReadStatus([]gotermboard.Status{b.jobs.Status()}, now)},
  Columns: []gotermboard.Column{{Title: ""}, {Title: "STATE"}, {Title: "AGE"}},
  Groups:  []gotermboard.Group{group},
 }
}

func main() {
 pal := gotermstyle.PaletteFor(os.Stdout)
 tabs, err := gotermboard.NewTabs([]gotermboard.Tab{{Name: "jobs", Board: newBoard()}}, pal, "")
 if err != nil {
  log.Fatal(err)
 }
 if _, err := tea.NewProgram(tabs).Run(); err != nil {
  log.Fatal(err)
 }
}
```

## Tabs read only while they show

`NewTabs` takes the tabs in the order Tab cycles them, and the name of the one to open
on. Each tab keeps its own cursor, folds and filter. A hidden tab starts no read and
builds no frame. On its return it draws its last good read with that read's age, and
every source past its interval reads at once. A source inside its interval keeps its
pace, so cycling the tabs quickly starts nothing extra. A program with one tab draws no
tab line, and Tab does nothing in it.

`Once` draws one frame from what a board's sources already hold, and `OnceSize` picks
its size: `COLUMNS` by `LINES`, then the terminal, then a fallback you pass.

## One line says how the reads stand

`ReadStatus` is a line for a view's strip. While every read is on schedule it says
`read 3s ago`. A failed read is named in red with its reason, one past twice its
interval in yellow, and a note on a read that covered less follows plain. The `?`
overlay lists every source's age, interval, cost and fault.

## A row that changed wears a cue

The board compares each row's and group's key and cells between frames, and marks a
changed one for `CueFor`: a dot in the pointer's column and its first cell in bold. No
column moves. A cell built with `Ticking` follows the clock, as an age or a gauge does,
so its text changing is no change to the row, though its color changing is. A first
read, a tab's return and a new grouping mark nothing.

## Group rows share the columns

A `Group` with `Cells` draws them on the columns past the first, which holds its title,
and the column titles then take a line of their own at the top. `Below` holds lines
drawn under the header whether it is folded or not, which the cursor moves past as one
node. `WrapLast` on a `Row` or a `Group` is the last column as cells of their own colors,
continuing on further lines at that column, so a bar wider than the row keeps every cell.

`Items` makes a line of `Below` or `Beside` that the draw cuts to the items that fit,
ending in `+N`, because only the draw knows the width and it moves with the side panel.
`Under` is a group's summary, wrapping under `Below` while `f` has them on.

`Span` draws a long value whole past a group's title, wrapping under itself, so it never
sets the first column's width. The group's cells then take the next line, with `Beside`
ahead of them in the first column. `Column.Min` holds that column wide enough to be useful.

## A row can open a document laid out at its width

`Document` on a `Row` or a `Group` is a function of width, because hanging bullets, tables
and boxes of code cannot be wrapped after they are drawn. A row's opens under it on `enter`,
or on the full-screen reader where it runs past the screen. A group's draws under `Below`
whether the group is folded or not. The `markdown` package
makes one from markdown, in ANSI 0-15 so it follows the theme:

```go
renderer := markdown.New(nil)
row.Document = func(width int) []gotermboard.Line { return renderer.Lines(description, min(width, 100)) }
```

A fence naming a language wraps a line wider than its box, marking each continued row with
`↪`, as `bat` and `delta` do. A `text` or bare fence cuts that line instead, so a diagram or a
table of output stays lined up. `WrapEveryFence` on the renderer wraps those too, for a text whose
every word has to reach the reader.

Its argument colors what a caller recognizes in prose, such as a number that names a record.

A `Still` group has no fold mark and the cursor passes over it. At the top of a frame it
stays on screen with the column titles while the table scrolls, which suits a row of totals.

## A chevron marks only what opens

The board decides the mark from what a node holds. A row draws `▸` or `▾` where it holds `Detail`
or a `Document`, and a group where it holds a row or a group the table draws. Anywhere else space
changes nothing, so nothing promises that it would. A filter holds every group open, so a group
draws no mark while one is typed.

A row's chevron sits in the last cell of its indent, one cell in from its parent's, with `Symbol`
beside it rather than replaced by it. `MarkGlyph` sits in the cell before the chevron. No column
moves for either.

## A filter can reach past what shows

`FilterOnly` keeps a group out of the table until a filter is typed, so a view that shows
the first few of something still finds the rest. `Text` is more the filter matches beyond
what a group draws, and `Heading` says a title only labels what sits under it, so a word in
it matches nothing.

## The side panel follows the cursor

From `PanelFrom` columns the panel takes three tenths of the screen, up to `ReadingWidth`. A frame
with no `Panel`, `Document` or `Detail` on any line the cursor can land on draws no panel, and the
table takes the width. The frame decides rather than the cursor's line, so the table never jumps
as the cursor moves.

`Panel` on a row or a group is what the side panel shows in `Detail`'s place, a function of
the panel's width whose lines wrap at spaces with each cell's own color. A row with no `Panel`
shows its `Document`. A board implementing `CursorBoard` is
told each key the cursor lands on, so a read that only one row needs can follow it.
`Frame.StartAt` names the key the cursor starts on. Without one it starts on the first line
holding cells, past any group with no panel text.

## A tall document reads on a full screen

`Reader` is a full screen over one `Document`. It lays the text out at up to 100 columns,
centers it on a wider screen, and keeps the same words at the top through a resize. `j` and
`k`, `ctrl+d` and `ctrl+u`, `g` and `G` move through it, `[` and `]` jump between sections,
and `/` searches with `n` and `N`. A query with no capital in it ignores case. `y` copies
`CopyText`, or the text as drawn, over OSC 52. `e` opens `EditFile` in `$VISUAL` or `$EDITOR`
and lays the text out again after.

`Model.Read` opens one over the board from a board's own key, and `esc` returns to the board.
`Model.Tall` says whether a document runs past the screen. `NewReader` runs as a program of its
own, which is all `cmd/read-markdown` does with a file or a pipe:

```bash
go install github.com/datapointchris/gotermboard/cmd/read-markdown@latest
read-markdown README.md
```

A `Cell` with `Section` set marks its line as a section. The `markdown` package sets it on each
heading.

## A board can add keys

A board implementing `Keyed` adds `Action`s, which take precedence over the table's and
appear in the footer and the `?` overlay. A board implementing `TailBoard` reads from its
newest row, as a log does.
