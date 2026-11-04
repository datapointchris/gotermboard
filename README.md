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

## A board can add keys

A board implementing `Keyed` adds `Action`s, which take precedence over the table's and
appear in the footer and the `?` overlay. A board implementing `TailBoard` reads from its
newest row, as a log does.
