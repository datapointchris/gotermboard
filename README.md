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
on. Each tab keeps its own cursor, folds and filter. A hidden tab starts no read,
builds no frame and moves no strip. On its return it draws its last good read with that read's age, and
every source past its interval reads at once. A source inside its interval keeps its
pace, so cycling the tabs quickly starts nothing extra. A program with one tab draws no
tab line, and Tab does nothing in it.

The tab line marks the current tab by color and weight rather than a box, as a tmux status
line marks its window: bold bright yellow, with the rest in bright black, each padded by a
space and two spaces apart. Both are ANSI slots, so the terminal's theme sets them.

`Once` draws one frame from what a board's sources already hold, and `OnceSize` picks
its size: `COLUMNS` by `LINES`, then the terminal, then a fallback you pass. A screen that
is no board picks its printed frame's size the same way with `printedframe.Size`, which
imports nothing that draws.

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
`TallyBar` is such a bar, one cell per unit, because a fixed width rounds one blocked unit
in sixty away. Each kind has its own glyph as well as its color, so a frame printed without
color still shows which units are done and which are blocked. `Line.Paint` turns a line into
text in its colors, so a command that prints rather than runs a board draws the same bar.

`Items` makes a line of `Below` or `Beside` that the draw cuts to the items that fit,
ending in `+N`, because only the draw knows the width and it moves with the side panel.
`Under` is a summary on a `└─` line while `f` has them on: a row's under the row, and a
group's wrapping under `Below`.
`UnderWhereCut` on a row draws its `Under` only where the last column is too narrow for the
row's cell there, for a summary that repeats that cell. `f` leaves the footer while no group
or row has a summary to draw.

`Span` draws a long value whole past a group's title, wrapping under itself, so it never
sets the first column's width. The group's cells then take the next line, with `Beside`
ahead of them in the first column. `Column.Min` holds that column wide enough to be useful.

## A row can open a document laid out at its width

`Document` on a `Row` or a `Group` is a function of width, because hanging bullets, tables
and boxes of code cannot be wrapped after they are drawn. A row's opens under it on `space`,
however tall it is. `enter` opens it the same way, or on the full-screen reader where it runs
past the screen or the side panel draws it, whether or not it shows in place. `StartsOpen` on a row shows its text
from the first frame, so a view can draw its newest entry whole and still fold it to its first
line. A group's draws under `Below` whether the group is folded or not. The `markdown` package
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

## What holds the text stays at the top while it scrolls

Reading down an open row's text keeps the row and the groups above it on the band at the top of
the table, as VS Code's sticky scroll and nvim-treesitter-context do. The band shows the first line
of each node holding the first line it leaves showing, so the cursor's row carries its pointer there.
A node joins once its own line has scrolled off, and a sibling arriving under the band takes its
predecessor's place.

- **The band covers lines rather than pushing them down,** so nothing under it moves as it grows.
- **It holds a few lines, never a large share of the table,** and drops the outermost first.
- **A band line is no cursor stop.** A cursor moved under the band brings its row out below it.
- **Only a row or group with lines under it and words to name them by joins,** so a folded
  group, a closed row and a group with no title never take a line of the band. A `Still` group
  at the top of the frame stays above the table already, so it never joins either.

Every board gets the band with no field to set, because gotermboard draws the table.

A `Still` group has no fold mark and the cursor passes over it. At the top of a frame it
stays on screen with the column titles while the table scrolls, which suits a row of totals.

## A chevron marks only what opens

The board decides the mark from what a node holds. A row draws `▸` or `▾` where it holds `Detail`
or a `Document`, and a group where it holds a row or a group the table draws. Anywhere else `enter`
and `space` change nothing, so no mark promises otherwise and the footer and `?` leave both keys out
on that line. A filter holds every group open, so a group draws no mark while one is typed.

A row's chevron sits in the last cell of its indent, one cell in from its parent's, with `Symbol`
beside it. `MarkGlyph` sits in the cell before the chevron. No column moves for either.

## A filter can reach past what shows

`FilterOnly` keeps a group out of the table until a filter is typed, so a view that shows
the first few of something still finds the rest. `Text` is more the filter matches beyond
what a group draws, and `Heading` says a title only labels what sits under it, so a word in
it matches nothing.

## The side panel follows the cursor

From `PanelFrom` columns the panel grows with the screen, up to `ReadingWidth`. It draws where any
row in the frame holds `Panel`, `Document` or `Detail`, or any group but a `Still` one holds `Panel`
or `Detail`, counting lines folded away or held back for a filter. A frame with none of them draws
no panel, and the table takes the width. The frame decides rather than the cursor's line, so the
table never jumps as the cursor moves.

`Panel` on a row or a group is what the side panel shows in `Detail`'s place, a function of
the panel's width whose lines wrap at spaces with each cell's own color. A row with no `Panel`
shows its `Document`, under its `ReaderTitle` in bold where it has one. Such a row then reads
its `Document` beside the table rather than under itself: `StartsOpen` leaves it folded while
the panel shows, and `enter` opens it on the full-screen reader. A board implementing `CursorBoard` is
told each key the cursor lands on, so a read that only one row needs can follow it.
`Frame.StartAt` names the key the cursor starts on. Without one it starts on the first line
holding cells, or the last on a `TailBoard`, past any group with no panel text. With no such line
it starts on the first line with text to read.

## A tall document reads on a full screen

`Reader` is a full screen over one `Document`. It lays the text out at up to 100 columns,
centers it on a wider screen, and keeps the same words at the top through a resize. `j` and
`k`, `ctrl+d` and `ctrl+u`, `g` and `G` move through it, `[` and `]` jump between sections,
and `/` searches with `n` and `N`. A query with no capital in it ignores case. `y` copies
`CopyText`, or the text as drawn, over OSC 52. `e` opens `EditFile` in `$VISUAL` or `$EDITOR`
and lays the text out again after. `EditorCommand(path)` is that editor on any file, for a
board's own key to hand the terminal with `tea.ExecProcess`.

Its top line names where the text sits: the groups the table's band would name over the row that
opened it, its title, then each heading still open above the top row. A heading joins once its own
row scrolls off, as the table's band does. Where the line runs long, the longest name is cut first,
and the outermost leaves once every name is down to a few words.

That cut is `breadcrumb.Fit`, over the names' widths alone. A screen that is no board fits its own
path line the same way: the `breadcrumb` package imports nothing that draws, so Bubble Tea stays
out of that screen's build.

`Model.Read` opens one over the board from a board's own key, and `esc` or `space` returns to the
board. `Model.Tall` says whether a document runs past the screen. `NewReader` runs as a program of its
own, which is all `cmd/read-markdown` does with a file or a pipe:

```bash
go install github.com/datapointchris/gotermboard/cmd/read-markdown@latest
read-markdown README.md
```

A `Cell` with `Section` set starts the words of a heading, and its value is the heading's depth, 1
outermost. A heading closes every open one at its depth or deeper. The `markdown` package sets it
on each heading's first word, past the bar that marks it.

## A board's strip can move between its frames

A board implementing `MovingBoard` gets its strip from `StripAt(now)`, redrawn every `MoveEvery()`,
for a diagram of work in motion between its reads. Only the strip redraws at that pace, so the table,
its change cue and the side panel wait for the next frame. Only the tab showing runs the clock, and a
reader over the board holds it until the key that closes the reader. A `MoveEvery` of zero holds the
strip still until the board's next tick asks again, so a board with nothing moving spends nothing.

```go
func (b *board) MoveEvery() time.Duration { return 100 * time.Millisecond }

func (b *board) StripAt(now time.Time) []gotermboard.Line {
 return []gotermboard.Line{b.lane(b.runs.Value(), now)}
}
```

`StripAt` replaces `Frame.Strip` on every draw, so it returns the lines that hold still as well. A
strip changing its height between beats redraws the table, because the table's height moves with it.

A strip that turns a spinner, such as one beside a read under way, draws `SpinnerAt(now)` and
returns `SpinEvery` from `MoveEvery` while it turns. Those are the footer's drawings at the
footer's pace, so the two spinners on one screen match.

A beat builds no frame and draws no table, so a moving strip costs a fraction of rebuilding the whole
frame at the same pace. What a beat does cost is mostly Bubble Tea's renderer, which compares the
whole screen on every frame that changed.

## Charts take the cells the draw gives them

The `chart` package draws plain series as lines of cells:

- `TimeSeries` draws braille lines over a time axis and a value axis. A `Dotted` series reads as a
  forecast, a `Band` shades the cells between two edges, and `Legend` names each beside its mark.
  Readings that all share one time draw an hour either side of it.
- `Heatmap` shades a grid such as hour against weekday, from `░` to `█` in one color. A shade orders
  by density in any theme, where a ramp of the sixteen colors orders differently in each. Given fewer
  lines than it has rows, it drops its column names first, then ends on a `+N` line for the rest.
- `Sparkline` draws eighth-block columns, averaging more values than columns so the whole span shows.
  Any value above zero draws at least `▁`, so a quiet stretch reads apart from an empty one.
- `Bars` draws an upright bar per value in its own color, over an axis labeled at the top, the middle
  and zero.

Each fills exactly the width and height it is given, so `chart.Beside` lays them side by side. A
chart colors its cells with the codes the caller passes, and the board's palette paints them. It
draws through [ntcharts](https://github.com/NimbleMarkets/ntcharts) and reads each cell back, so
ntcharts never writes an escape of its own.

A board knows the screen's size only at the draw, so `Frame.Charts` is a function of it, drawn
between the strip and the table:

```go
frame.Charts = func(width, height int) []gotermboard.Line {
 week := chart.TimeSeries{Series: []chart.Series{{Name: "used", Points: used, Code: gotermboard.Cyan}}, Max: 100}
 return append([]gotermboard.Line{week.Legend()}, week.Lines(width, min(14, height/5))...)
}
```

A row's `Panel` takes a chart the same way, at the panel's width: `chart.Document(week, 12)`. The
charts are laid out once a frame, so a moving strip's beat leaves them as they stand.

## A board can add keys

A board implementing `Keyed` adds `Action`s, which take precedence over the table's and
appear in the footer and the `?` overlay. A board implementing `TailBoard` reads from its
newest row, as a log does.

`enter` and `space` are separate keys in the table, so a board that takes `enter` leaves `space`
folding in place. `Model.Toggle` is what the table's `enter` does, for a board's `enter` that acts
only on some rows. Bind it to `enter` alone. A row open in place that turns tall, as on a pane
shrink, reads on `Toggle` and closes only on the table's `space`.

## A key can ask for a line of text on the footer

`Model.Ask` puts a `Question` on the footer's lines. With `Typed` set it takes one line typed under
the question, and `enter` hands `Answer` what was typed, trimmed. Without it `enter` alone answers,
for an act that takes no text. `esc` or `ctrl+c` takes the question away and runs `Cancel` where it
is set, so a board can say that nothing was done. Until one of them every key goes to the question:
`q` and `?` type themselves, `tab` does nothing, and a paste lands as one line.

```go
{Keys: []string{"m"}, Label: "merge the row into a key you type", Footer: "m merge", Run: func(m *gotermboard.Model) tea.Cmd {
 key := m.Cursor()
 m.Ask(gotermboard.Question{Text: "merge " + key + " into:", Typed: true, Answer: func(into string) tea.Cmd {
  m.Say(merge(key, into))
  return nil
 }})
 return nil
}}
```

The question draws in bold as written, so its own punctuation ends it. It wraps rather than being
cut, and so does what `Say` puts on the footer. Each stops at a few lines, the last ending in `…`
where more was left. The typed line is cut from the left, so its end and the cursor stay on the
screen, as the filter and the reader's search are. A footer that grows keeps the cursor's row in
view. The answer runs after the question closes, so it can `Say` what it did, ask another question,
or return a command, such as one that opens an editor.

## A key's command reports back to its board

A key's `Run` returns a command, and a command that runs in the background or hands the terminal
over with `tea.ExecProcess` ends in a message. A board implementing `ReceivingBoard` is handed every
message the model does not handle itself, with the model, so it can `Say` what came of the command,
`Rebuild`, or return the next command. `Tabs` hands such a message to every tab, because the tab
that started the command may be hidden by the time it ends, so a board switches on its own types.

A board implementing `BusyBoard` names its work under way from `UnderWay()`. The footer draws it
after a spinner, above what the last key did, and a key does not clear it. The spinner turns only
while the tab shows and nothing is open over the board.

```go
func (b *board) merge(m *gotermboard.Model) tea.Cmd {
 b.underWay = "merging #12"
 return func() tea.Msg { return mergedMsg{err: exec.Command("gh", "pr", "merge", "12", "--merge").Run()} }
}

func (b *board) UnderWay() string { return b.underWay }

func (b *board) Receive(m *gotermboard.Model, msg tea.Msg) tea.Cmd {
 if merged, ok := msg.(mergedMsg); ok {
  b.underWay = ""
  m.Say(mergeResult(merged.err))
 }
 return nil
}
```
