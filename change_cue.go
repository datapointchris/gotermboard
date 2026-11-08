package gotermboard

import (
	"hash/maphash"
	"strconv"
	"strings"
	"time"
)

// The cue a row or group wears after it changed. It sits in the pointer's column, so no
// column moves for it.
const (
	CueGlyph = "•"
	CueCode  = BoldCyan
	CueFor   = 30 * time.Second
)

// settleFor is how long a quiet lasts at least. A first read often feeds another, which
// fills in on its next pass, and that is no more news than the first read was.
const settleFor = 5 * time.Second

// changeCue remembers each row's and group's fingerprint between frames, and when each
// last changed. A quiet frame takes the fingerprints and marks nothing, because a first
// read, a tab's return or a new grouping would otherwise light the whole board.
type changeCue struct {
	seed  maphash.Seed
	seen  map[string]uint64
	since map[string]time.Time
	quiet bool
	// quietFrom is when the quiet began. It holds past settleFor until the reads that
	// started by then return.
	quietFrom time.Time
}

func newChangeCue() changeCue {
	return changeCue{seed: maphash.MakeSeed(), seen: map[string]uint64{}, since: map[string]time.Time{}, quiet: true}
}

func (c *changeCue) hush(now time.Time) { c.quiet, c.quietFrom = true, now }

// mark compares the frame's groups with the last frame's and sets each one's cue.
func (c *changeCue) mark(groups []Group, now time.Time) {
	seen, since := make(map[string]uint64, len(c.seen)), make(map[string]time.Time, len(c.since))
	c.markGroups(groups, "", now, seen, since)
	c.seen, c.since = seen, since
}

func (c *changeCue) markGroups(groups []Group, parent string, now time.Time, seen map[string]uint64, since map[string]time.Time) {
	for i := range groups {
		g := &groups[i]
		path := parent + "/" + g.Key
		g.changed = c.note(path, c.groupPrint(g), now, seen, since)
		for j := range g.Rows {
			r := &g.Rows[j]
			r.changed = c.note(path+"//"+r.Key, c.rowPrint(r), now, seen, since)
		}
		c.markGroups(g.Groups, path, now, seen, since)
	}
}

// note records one fingerprint and says whether its cue is on.
func (c *changeCue) note(path string, fingerprint uint64, now time.Time, seen map[string]uint64, since map[string]time.Time) bool {
	seen[path] = fingerprint
	if c.quiet {
		return false
	}
	at, marked := c.since[path]
	if before, known := c.seen[path]; !known || before != fingerprint {
		at, marked = now, true
	}
	if !marked || now.Sub(at) >= CueFor {
		return false
	}
	since[path] = at
	return true
}

func (c *changeCue) rowPrint(r *Row) uint64 {
	var b strings.Builder
	printCells(&b, []Cell{r.Symbol})
	printCells(&b, r.Cells)
	printCells(&b, r.Span)
	printCells(&b, r.WrapLast)
	return maphash.String(c.seed, b.String())
}

func (c *changeCue) groupPrint(g *Group) uint64 {
	var b strings.Builder
	printCells(&b, g.Title)
	if g.Counted {
		b.WriteString(strconv.Itoa(g.Count))
	}
	printCells(&b, g.Span)
	printCells(&b, g.Beside)
	printCells(&b, g.Cells)
	printCells(&b, g.WrapLast)
	printCells(&b, g.Summary)
	for _, line := range g.Below {
		printCells(&b, line)
	}
	return maphash.String(c.seed, b.String())
}

// printCells writes what a change is made of: each cell's color, and its text unless
// that follows the clock. The separators keep two cells from reading as one.
func printCells(b *strings.Builder, cells []Cell) {
	for _, cell := range cells {
		b.WriteString(cell.Code)
		b.WriteString(cell.Role)
		b.WriteByte(0)
		if !cell.Ticks {
			b.WriteString(cell.Text)
		}
		b.WriteByte(0)
	}
}
