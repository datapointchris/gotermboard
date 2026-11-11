package gotermboard

import (
	"encoding/binary"
	"hash/maphash"
	"strconv"
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
	// print hashes one fingerprint or one path at a time, so marking a frame builds no string.
	print *maphash.Hash
	// seen and since are keyed by a hash of each line's path of keys. The spares are the maps the
	// frame before last filled, which the next mark fills again.
	seen, spareSeen   map[uint64]uint64
	since, spareSince map[uint64]time.Time
	quiet             bool
	// quietFrom is when the quiet began. It holds past settleFor until the reads that
	// started by then return.
	quietFrom time.Time
}

func newChangeCue() changeCue {
	print := &maphash.Hash{}
	print.SetSeed(maphash.MakeSeed())
	return changeCue{
		print: print, quiet: true,
		seen: map[uint64]uint64{}, spareSeen: map[uint64]uint64{},
		since: map[uint64]time.Time{}, spareSince: map[uint64]time.Time{},
	}
}

func (c *changeCue) hush(now time.Time) { c.quiet, c.quietFrom = true, now }

// mark compares the frame's groups with the last frame's and sets each one's cue. A node points at
// its group or row, so a layout made before the mark shows what the mark set.
func (c *changeCue) mark(groups []Group, now time.Time) {
	seen, since := c.spareSeen, c.spareSince
	clear(seen)
	clear(since)
	c.markGroups(groups, 0, now, seen, since)
	c.spareSeen, c.spareSince, c.seen, c.since = c.seen, c.since, seen, since
}

func (c *changeCue) markGroups(groups []Group, parent uint64, now time.Time, seen map[uint64]uint64, since map[uint64]time.Time) {
	for i := range groups {
		g := &groups[i]
		path := c.path(parent, g.Key, false)
		g.changed = c.note(path, c.groupPrint(g), now, seen, since)
		for j := range g.Rows {
			r := &g.Rows[j]
			r.changed = c.note(c.path(path, r.Key, true), c.rowPrint(r), now, seen, since)
		}
		c.markGroups(g.Groups, path, now, seen, since)
	}
}

// path is a group's or a row's place in the frame: the path of what holds it, then its own key. A
// row and a group under one parent can share a key, so row tells them apart.
func (c *changeCue) path(parent uint64, key string, row bool) uint64 {
	var lead [9]byte
	binary.LittleEndian.PutUint64(lead[:], parent)
	if row {
		lead[8] = 1
	}
	c.print.Reset()
	_, _ = c.print.Write(lead[:])
	_, _ = c.print.WriteString(key)
	return c.print.Sum64()
}

// note records one fingerprint and says whether its cue is on.
func (c *changeCue) note(path, fingerprint uint64, now time.Time, seen map[uint64]uint64, since map[uint64]time.Time) bool {
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
	c.print.Reset()
	c.printCell(&r.Symbol)
	c.printCells(r.Cells)
	c.printCells(r.Span)
	c.printCells(r.WrapLast)
	return c.print.Sum64()
}

func (c *changeCue) groupPrint(g *Group) uint64 {
	c.print.Reset()
	c.printCells(g.Title)
	if g.Counted {
		_, _ = c.print.WriteString(strconv.Itoa(g.Count))
	}
	c.printCells(g.Span)
	c.printCells(g.Beside)
	c.printCells(g.Cells)
	c.printCells(g.WrapLast)
	c.printCells(g.Summary)
	for _, line := range g.Below {
		c.printCells(line)
	}
	return c.print.Sum64()
}

func (c *changeCue) printCells(cells []Cell) {
	for i := range cells {
		c.printCell(&cells[i])
	}
}

// printCell writes what a change is made of: the cell's color, and its text unless that follows
// the clock. The separators keep two cells from reading as one.
func (c *changeCue) printCell(cell *Cell) {
	_, _ = c.print.WriteString(cell.Code)
	_, _ = c.print.WriteString(cell.Role)
	_ = c.print.WriteByte(0)
	if !cell.Ticks {
		_, _ = c.print.WriteString(cell.Text)
	}
	_ = c.print.WriteByte(0)
}
