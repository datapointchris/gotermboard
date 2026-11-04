# gotermboard — Claude Code instructions

A Go library, not a CLI. No `main` package, nothing to install. It ships as git tags,
and Go command-line tools import it.

## Constraints that must not regress

- **A major here is an outage rather than a version**, because the module path carries
  no `/vN` suffix. A breaking change ships as a minor while the module is on 0.x.
- **It knows nothing of what its callers show.** A row, a group and a column are
  generic. A caller's domain words never reach this package.
- **Colors are ANSI 0-15, through gotermstyle**, so a board follows the terminal's theme.
- **A hidden tab starts no read and builds no frame.** A tab's return draws its last good
  read first, then reads what is due.
- **No cue or mark moves a column.** A change cue sits in the pointer's column and a mark
  in a row's indent, so a table never shifts under the eye.
