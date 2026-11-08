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

## A row jumps to the tab that shows what it names, and esc comes back

A jump is a back stack, as a browser's history or Vim's jumplist is. A row or a group lists
what it is about in `Entities`, each an `EntityRef` of a kind and an id every view agrees on.
The package gives no kind a meaning. A board implementing `EntityBoard` names the kinds it
shows, so `t` on a row naming one goes to that tab, pushing the tab it left. Where what a row
names goes to more than one place, `t` lists them by number and goes to the one typed.

- **`ShowEntity` hears the entity first, and says whether the board now shows it whole.** A
  board that follows one thing changes what it reads and answers true. Otherwise the cursor goes
  to the first group or row naming the entity, opening the groups that hold it. Where none names
  it once the reads land, the footer says so, and the jump still lands. A read that failed says
  the tab may hold it instead. Either way the tab starts at its top and reads every source at
  once, as `Restart` does.
- **`esc` at the tab a jump opened goes back, on the row it left.** It sits ahead of the board's
  own `esc`, so that tab is the top of where the jump went. Clearing a filter, closing a question
  and closing a reader come first. With nothing to go back along, `esc` is the board's.
- **Tab and shift+tab empty the stack,** because the reader left the trail the jumps made.
- **`OpensOutside` adds a program that shows a kind whole,** such as a pull request tool opened
  on one. A jump there hands it the terminal and pushes nothing, because quitting it comes back.
  It ends as `HandOver` does: the board receives the `HandedBack`, and a failure says the last
  line of its stderr. `Command` returns a `tea.ExecCommand`, so the caller's own runner starts it.
- **`Tabs.Keys` is the keys a board's own would hide**: tab and shift+tab where there are tabs to
  cycle, and `t` where a row could go somewhere. `TableKeys` is the table's alone.
  `Model.CursorEntities` is what the cursor's line names.

`Once` draws one frame from what a board's sources already hold, and `OnceSize` picks
its size: `COLUMNS` by `LINES`, then the terminal, then a fallback you pass. A screen that
is no board picks its printed frame's size the same way with `printedframe.Size`, which
imports nothing that draws.

## A theme file colors a board by role, and SIGUSR2 recolors a running board

`NewTabs`, `NewModel`, `NewReader` and `Once` read the TUI theme file that gotermstyle's
`tuitheme.DefaultPath` names. Without one, a board draws in the terminal's own 16 colors. A
file that will not load at start leaves those 16 too, and the footer says why.

- **The cursor's row takes the file's `ui.selection` background across the table's width**:
  its indent, its cells and the gaps between columns. Each cell keeps its own foreground, so a
  state's color still reads on it. Without a file the row is bold, with `▌` in the pointer's
  column. No column moves either way.
- **The tab line marks the current tab by color and weight rather than a box**, as a tmux
  status line marks its window. The current tab is bold in `ui.warning` and the rest are in
  `ui.muted`, so the tab line reads like a tmux bar drawn from the same theme. Each name is
  padded by a space, two spaces apart.
- **A reader's search matches draw in `ui.match`, bold, over the `ui.selection` background**,
  or reversed without a file, so a match inside a code span still stands out.
- **A cell draws in the role its `Role` names, with its `Code` laid over that as attributes**,
  wherever a board, its tabs or a reader draws it. A marked row's first cell draws in
  `MarkCode` instead. The `markdown` package names one for every cell, and its code takes the
  nvim-treesitter capture closest to each token, under `syntax`, so a theme file made from an
  editor's colors reaches it. A name the file lacks falls back to the wider one, so
  `syntax.type.builtin` draws as `syntax.type`. A heading stays bold whatever its role says.
  The board looks a role up as it draws, so a reload recolors what is already laid out.
  `Line.Paint` draws a role in the built-in theme, for a line printed outside a board.
- **SIGUSR2 reads the file again**, so a theme change recolors a running board with no
  restart. A file that will not load keeps the colors on screen. The program's `Init` starts
  the wait, so a model wrapping `Tabs` calls its `Init` and hands it the messages it does not
  handle itself.
- **A test that draws a board sets `TUI_THEME_FILE` to a path that does not exist**, so it
  draws the built-in theme whatever file the machine running it has.

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
in sixty away. Done is filled and every other kind is hollow, so a frame printed without
color still shows which units are done. Under way, blocked and the rest differ by color. `Line.Paint` turns a line into
text in its colors, so a command that prints rather than runs a board draws the same bar.

`Items` makes a line of `Below` or `Beside` that the draw cuts to the items that fit,
ending in `+N`, because only the draw knows the width and it moves with the side panel.
`Under` is a summary on a `└─` line while `f` has them on: a row's under the row, and a
group's wrapping under `Below`. They start off, and a board implementing `TwoLineBoard`
starts them on, for rows whose identity reads dim in that summary.
`UnderWhereCut` on a row draws its `Under` only where the last column is too narrow for the
row's cell there, for a summary that repeats that cell. `f` leaves the footer while no group
or row has a summary to draw.

`Span` draws a long value whole past a group's title, wrapping under itself, so it never
sets the first column's width. The group's cells then take the next line, with `Beside`
ahead of them in the first column. `Column.Min` holds that column wide enough to be useful.

## A board can draw its rows as blocks ruled apart, spread across a margined screen

Every piece of this is off in its zero value, so a board that sets none draws the default
look.

- **`Second` on a `Row` is a line it always draws under its first.** Its first cell sits in
  the first column under the row's first cell, and the rest draw one after another from the
  second column, cut at the table's edge. It suits a state over a second state, with the
  row's identity beside them. The filter reads it, and the cursor keeps both lines in view.
  It is not `Under`: a row with both draws `Second`, then its `Under` summary.
- **`BetweenRows` on a `Group` draws a blank line and a rule between each two of its rows**,
  so rows that set `Second` read as blocks. `RuleDotted` is the lightest weight, `┈`.
- **A board implementing `LookBoard` returns a `Look` for the screen's width.** `Sides` and
  `Ends` are a margin of blank columns and lines around the whole screen. `Spread` lays the
  columns out at their own widths and deals what they leave out between them from the left,
  so the table ends on its edge. A column with no title continues the one before it and
  takes none of the spread, so a group of untitled counts stays together. A table whose rows
  wrap their last column with `WrapLast` keeps the slack in it instead. `Rules` puts the
  column titles on a line of their own with a rule under it, and a rule over the footer.
  Both rules span the table's width. `BoldCursor` draws the cursor's row bold over the
  theme's selection, across every line the row draws. `NoRowChevrons` leaves each row's
  chevron cell blank.

## A row can open a document laid out at its width

`Document` on a `Row` or a `Group` is a function of width, because hanging bullets, tables
and boxes of code cannot be wrapped after they are drawn. A row's opens under it on `space`,
however tall it is. `enter` opens it the same way, or on the full-screen reader where it runs
past the screen or the side panel draws it, whether or not it shows in place. `StartsOpen` on a row shows its text
from the first frame, so a view can draw its newest entry whole and still fold it to its first
line. A group's draws under `Below` whether the group is folded or not. The `markdown` package
makes one from markdown, in the theme file's roles:

```go
renderer := markdown.New(nil)
row.Document = func(width int) []gotermboard.Line { return renderer.Lines(description, min(width, 100)) }
```

A fence naming a language wraps a line wider than its box, marking each continued row with
`↪`, as `bat` and `delta` do. A `text` or bare fence cuts that line instead, so a diagram or a
table of output stays lined up. `WrapEveryFence` on the renderer wraps those too, for a text whose
every word has to reach the reader.

Its argument colors what a caller recognizes in prose, such as a number that names a record. It
is handed each run's text and attributes. A cell it returns in those attributes takes the run's
role, and a cell it colors keeps the color it was given. A caller tinting a document's uncolored
words afterward tests `Cell.Plain`, because a cell colored by its role carries no `Code`.

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
beside it. `MarkGlyph` sits in the cell before the chevron. No column moves for either. A board
whose every row opens can blank the row chevrons with `Look.NoRowChevrons`, because there a
chevron tells no row apart. The cell stays, so no column moves for that either.

## A filter can reach past what shows

`FilterOnly` keeps a group out of the table until a filter is typed, so a view that shows
the first few of something still finds the rest. `Text` is more the filter matches beyond
what a group draws, and `Heading` says a title only labels what sits under it, so a word in
it matches nothing.

## The side panel follows the cursor

From `PanelFrom` columns inside a `Look`'s margin, or of the screen where a board has none,
the panel grows with the screen, up to `ReadingWidth`. It draws where any
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
and lays the text out again after, or shows the last line the editor wrote to stderr where it
failed. `EditorCommand(path)` is that editor on any file, for a board's own key to hand over.

Its top line names where the text sits: the groups the table's band would name over the row that
opened it, its title, then each heading still open above the top row. A heading joins once its own
row scrolls off, as the table's band does. Where the line runs long, the longest name is cut first,
and the outermost leaves once every name is down to a few words.

That cut is `breadcrumb.Fit`, over the names' widths alone. A screen that is no board fits its own
path line the same way: the `breadcrumb` package imports nothing that draws, so Bubble Tea stays
out of that screen's build.

`Model.Read` opens one over the board from a board's own key, and `esc` or `space` returns to the
board. `Model.Follow` opens one at the document's last line and keeps it there as the document
grows, as `tail -f` and `less +F` do. Scrolling up holds it where it is, and `G` follows again. The
footer says `following` or `G follows`, and the function it takes runs as the reader closes, so the
board can stop what feeds it. An open reader lays its document out again after each
message a `ReceivingBoard` receives, so lines a board's command streams in show as each lands. `Model.Tall` says whether a document runs past the screen. A program that draws its own screen
rather than a board hands each key, paste and size to a `NewReader` while it shows one, and
`ClosesWith` names what `esc` and `space` return to. `NewReader` runs as a program of its
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
  forecast, a `Columns` series draws each reading as an eighth-block column until the next, as a
  count per day reads, a `Band` shades the cells between two edges, and `Legend` names each beside
  its mark. A column three or more cells wide ends a cell short, so neighbors read apart, and
  readings sharing a cell draw their mean.
  Readings that all share one time draw an hour either side of it. An axis under six hours names
  round minutes, every five on an axis of half an hour.
- `Heatmap` shades a grid such as hour against weekday, from `░` to `█` in one color. A shade orders
  by density in any theme, where a ramp of the sixteen colors orders differently in each. Given fewer
  lines than it has rows, it drops its column names first, then ends on a `+N` line for the rest.
- `Sparkline` draws eighth-block columns, averaging more values than columns so the whole span
  shows, and with `Fill` widening fewer so they span it. Any value above zero draws at least `▁`, so
  a quiet stretch reads apart from an empty one.
- `Bars` draws an upright bar per value in its own color, over an axis labeled at the top, the middle
  and zero. A bar with a `Stack` draws each part over the one before it.
- `Meter` draws one bar across a line to an eighth of a cell, each part after the one before it.
- `Timeline` draws spans on one time axis, a row each, as a Gantt chart or a trace waterfall lays them
  out. A bar starts on the cell its span starts in and ends to an eighth of a cell, so a span lasting
  any time draws at least `▏`, and a moment draws a milestone's `◆`. A span with no `End` is under way
  and runs to the axis's end, so a chart of work under way sets `To` to now. A wait between two spans
  shows as the gap between their bars. Given fewer lines than a row per span and the axis, it drops
  its axis first, then ends on a `+N` line for the rest.
- `StackedArea` stacks layers over time from zero, as a cumulative flow diagram reads, over the axes
  `TimeSeries` draws. Each layer's edge falls to an eighth of a row. Where two layers meet inside a
  cell, the lower one's block sits on the upper one's color, painted as the cell's background.

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
shrink, reads on `Toggle` and closes only on the table's `space`. A board that binds `space` to a
row verb, such as a mark, calls `Model.CloseInPlace` first, which closes the cursor's row where it
is open and says whether it was, so `space` still closes it.

`Model.MoveCursorTo` puts the cursor on a key, for a key that acts on the cursor's row and moves on,
as a merge moves to the next row in its group. It says whether the key shows, so a board can try
the next candidate where the filter or a fold hides one. A key that changed what the board holds
calls `Rebuild` first, so the key is looked for in the new frame.

A program that draws its own screen rather than a board builds its key table from the same rows.
`KeyAction[H]` is a row for a screen of type `H`, and `Action` is a `Model`'s. `UniversalActions`
is the keys every board binds, acting on a `Host`: `j` `k` `g` `G`, `ctrl+d` `ctrl+u`, `J` `K`,
`[` `]`, `enter`, `space`, `?` and `q`. The host answers how to move, scroll, jump, open a row,
show the keys and quit. `ActionsNow(host, own, UniversalActions[H]())` is every row that applies
now, with each of the host's keys hiding the universal one while it applies. Its key handler,
footer and `?` overlay all read that. A row's `Section` heads its group in an overlay that groups
them, and a `Model`'s overlay lists its rows in one run.

## A key can ask on the footer for a line of text or one key from a menu

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

A question about several things names each one in its `Listing`, a `Line` each, drawn above the
question and cut at the screen's edge. The listing takes the screen above the footer and leaves the
table one line, which the cursor's row holds. A listing longer than that ends on how many more it
held.

A question with `Choices` is a menu. Each `Choice` draws under the question as its key and a label,
and pressing that key answers at once, handing `Answer` the key. Every other key does nothing,
`enter` included, so a reflex press picks nothing, and `esc` cancels as it does any question. The
choices are never cut, so the listing gives way to them.

```go
m.Ask(gotermboard.Question{
 Listing: []gotermboard.Line{{gotermboard.C("#13 conflicts with main in 2 files", "")}},
 Text:    "fix #13:",
 Choices: []gotermboard.Choice{{Key: "n", Label: "resolve in nvim"}, {Key: "c", Label: "hand to claude"}},
 Answer:  func(key string) tea.Cmd { return fixes[key](m) },
})
```

## A key's command reports back to its board

A key's `Run` returns a command, and a command that runs in the background ends in a message. A
board implementing `ReceivingBoard` is handed every message the model does not handle itself, with
the model, so it can `Say` what came of the command, `Rebuild`, or return the next command. `Tabs`
hands such a message to every tab, because the tab that started the command may be hidden by the
time it ends, so a board switches on its own types.

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

## A key leaves the board for another program and comes back

`Model.HandOver(cmd, then)` gives the terminal to `cmd` until it exits, and `then` gets how it ended
and the end of what it wrote to stderr. `LastLine(stderr, err)` is the sentence a failure says: the
last line it wrote, or its error where it wrote nothing.

To keep stderr, the program's fd 2 is a pipe, drained to the terminal as bytes arrive. So a program
that asks whether stderr is a terminal hears no, as `git` does before drawing progress. A caller
handing over such a program sets `cmd.Stderr`, such as to `os.Stderr`, and keeps nothing. The pipe
drains for as long as anything holds it open, so a child the program leaves running keeps writing
to the terminal, and the board does not wait for it to exit.

Before `then` runs, the board receives a `HandedBack` through `ReceivingBoard`, to read again what
the program may have changed. Only the tab whose key handed the terminal over receives it, hidden or
not. `EditorCommand(path)` is the user's editor on a file, ready to hand over.

```go
func (b *board) edit(m *gotermboard.Model) tea.Cmd {
 return m.HandOver(gotermboard.EditorCommand(b.path), func(m *gotermboard.Model, err error, stderr []byte) tea.Cmd {
  if err != nil {
   m.Say("the editor failed: " + gotermboard.LastLine(stderr, err))
  }
  return nil
 })
}

func (b *board) Receive(m *gotermboard.Model, msg tea.Msg) tea.Cmd {
 if _, ok := msg.(gotermboard.HandedBack); ok {
  b.source.Force()
 }
 return nil
}
```

`OpenWindow(name, dir, argv, needs...)` opens `argv` in a new tmux window, which tmux makes current,
while the board keeps running in its own. Several arguments run with no shell, so none needs
quoting. tmux opens a window whether or not its program starts, and in another directory where
`dir` is gone. So `OpenWindow` first looks for `dir`, and for tmux, `argv[0]` and each of `needs`
on `PATH`. A lone argument holding a space is a shell command, which tmux runs through its default
shell, so `needs` names the programs it runs. Outside tmux it opens nothing and says why.
`OpenWindowBehind` takes the same arguments and leaves the board's window current, for a command
that runs on its own while the board stays in front.
