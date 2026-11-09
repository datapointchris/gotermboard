# gotermboard — Claude Code instructions

A Go library that Go command-line tools import, shipped as git tags. Its one `main` package,
`cmd/read-markdown`, puts a file on the reader and holds no logic the library lacks.

## Never write the breaking-change trailer in a commit message

The words `BREAKING CHANGE` — either number, colon or not, subject or body — ask the release for a
major. `commit-analyzer-cz` matches them unanchored against the raw message and ORs the result with
the configured major rules, so no config stops it, and it majors even a `fix:` commit.

While the release workflow passes `allow-initial-development-versions: true`, that major is cut as
a minor. The day the module leaves 0.x, the same words cut a major. The module path carries no
`/vN` suffix, so `go get …@latest` cannot see the major, and every caller stays on the old line
while the release looks shipped.

**The ban covers a commit that merely discusses the trailer.** Name it some other way — "that
marker" — and never quote it.

## Constraints that must not regress

- **A major here is an outage rather than a version**, because the module path carries
  no `/vN` suffix. A breaking change ships as a minor while the module is on 0.x.
- **It knows nothing of what its callers show.** A row, a group and a column are
  generic. A caller's domain words never reach this package.
- **Colors come from the TUI theme file through gotermstyle, or ANSI 0–15 without one**, so
  a board follows the terminal's theme. A color named as a theme role reads the file. A color
  given as an ANSI code stays that code.
- **Every cell inside a board paints at draw time through its painter's `line` or `cell`**, so
  a Role resolves in the live theme and a reload recolors it. The palette the painter holds is
  reachable anywhere in the package, so the reload test over every surface holds this, and a
  new surface adds its cell there.
- **A hidden tab starts no read, builds no frame and moves no strip.** A tab's return draws its last good
  read first, then reads what is due.
- **No cue or mark moves a column.** A change cue sits in the pointer's column, and a mark
  and a chevron each in a cell of a row's indent, so a table never shifts under the eye.
- **The sticky band covers lines and never moves them,** so the text under it holds still as
  it grows and shrinks.
- **Every piece of a board's look is off in its zero value, and a board that sets none draws
  the default look byte for byte.** Each other board's frames rest on that, so a new piece
  of a look adds nothing until a board sets it.
- **A moving strip redraws only itself.** A beat builds no frame and draws no table, and only
  the tab showing runs one.
