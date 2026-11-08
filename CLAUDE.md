# gotermboard — Claude Code instructions

A Go library that Go command-line tools import, shipped as git tags. Its one `main` package,
`cmd/read-markdown`, puts a file on the reader and holds no logic the library lacks.

## Constraints that must not regress

- **A major here is an outage rather than a version**, because the module path carries
  no `/vN` suffix. A breaking change ships as a minor while the module is on 0.x.
- **It knows nothing of what its callers show.** A row, a group and a column are
  generic. A caller's domain words never reach this package.
- **Colors come from the TUI theme file through gotermstyle, or ANSI 0–15 without one**, so
  a board follows the terminal's theme.
- **A hidden tab starts no read, builds no frame and moves no strip.** A tab's return draws its last good
  read first, then reads what is due.
- **No cue or mark moves a column.** A change cue sits in the pointer's column, and a mark
  and a chevron each in a cell of a row's indent, so a table never shifts under the eye.
- **The sticky band covers lines and never moves them,** so the text under it holds still as
  it grows and shrinks.
- **A moving strip redraws only itself.** A beat builds no frame and draws no table, and only
  the tab showing runs one.
