package gotermboard

import (
	"slices"
	"strconv"
)

// RawRowsKey switches the whole board between drawing each fold as one row and drawing the raw
// rows.
const RawRowsKey = "Z"

// rowFold is consecutive rows of one group sharing a Fold, and the row the frame's FoldRow made of
// them.
type rowFold struct {
	// from and to bound rows in the group's Rows.
	from, to int
	rows     []Row
	row      Row
}

// foldSource is what a group's folds are worked out from: its rows and its key. A copy of a group
// shares both, and two groups slicing one array differ in one.
type foldSource struct {
	first *Row
	rows  int
	group string
}

func sourceOf(g *Group) foldSource {
	return foldSource{first: &g.Rows[0], rows: len(g.Rows), group: g.Key}
}

// frameFolds is every group's folds in one frame, worked out for every group the first time
// anything asks, so FoldRow runs once per fold per frame. It is dropped with its frame, and the
// keys its folds held pass to the next frame's.
type frameFolds struct {
	foldRow func(rows []Row) Row
	groups  []Group
	of      map[foldSource][]rowFold
	worked  bool
	// any says some group holds a fold, and panelText that some fold's row has text of its own for
	// the side panel.
	any, panelText bool
	// holding is the key of the fold holding each row, by the row's key. before is the last worked
	// frame's, whose keys a fold holding one of the same rows takes up.
	holding, before map[string]string
}

func newFrameFolds() *frameFolds { return &frameFolds{of: map[foldSource][]rowFold{}} }

func (f *frameFolds) newFrame(groups []Group, foldRow func(rows []Row) Row) {
	if f == nil {
		return
	}
	if f.worked {
		f.before = f.holding
	}
	f.groups, f.foldRow, f.worked, f.any, f.panelText, f.holding = groups, foldRow, false, false, false, nil
	clear(f.of)
}

// in is g's folds, in the order of its rows.
func (f *frameFolds) in(g *Group) []rowFold {
	if !f.work() || len(g.Rows) < 2 {
		return nil
	}
	return f.of[sourceOf(g)]
}

// work works out every group's folds once, and says whether the frame folds rows at all.
func (f *frameFolds) work() bool {
	if f == nil || f.foldRow == nil {
		return false
	}
	if !f.worked {
		f.worked, f.holding = true, map[string]string{}
		f.workGroups(f.groups, map[string]bool{})
	}
	return true
}

func (f *frameFolds) workGroups(groups []Group, taken map[string]bool) {
	for i := range groups {
		g := &groups[i]
		var folds []rowFold
		for from := 0; from < len(g.Rows); {
			to := foldEnd(g.Rows, from)
			if to-from > 1 {
				folds = append(folds, f.made(g.Key, g.Rows[from:to:to], from, taken))
			}
			from = to
		}
		if len(folds) > 0 {
			f.of[sourceOf(g)], f.any = folds, true
		}
		f.workGroups(g.Groups, taken)
	}
}

// made is the fold of rows, which start at from in group's rows.
func (f *frameFolds) made(group string, rows []Row, from int, taken map[string]bool) rowFold {
	row := strippedRow(f.foldRow(rows))
	// A row made from a copy of one of its rows would otherwise wear that row's cue as it was.
	row.changed = false
	row.Key = f.key(group, row.Key, rows, taken)
	for _, r := range rows {
		if r.Key != "" {
			f.holding[r.Key] = row.Key
		}
	}
	f.panelText = f.panelText || row.hasPanelText()
	return rowFold{from: from, to: from + len(rows), rows: rows, row: row}
}

// key is a fold's key: the one FoldRow set where none of its rows holds it, or else the key a fold
// in the last frame held where it held one of the same rows, so the key holds as rows join and
// leave either end. A fold with neither is keyed by its group, its Fold and its first row. The NUL
// bytes keep a made key from any key a board writes.
func (f *frameFolds) key(group, set string, rows []Row, taken map[string]bool) string {
	if set != "" && !slices.ContainsFunc(rows, func(r Row) bool { return r.Key == set }) {
		taken[set] = true
		return set
	}
	for _, r := range rows {
		if before, held := f.before[r.Key]; held && r.Key != "" && !taken[before] {
			taken[before] = true
			return before
		}
	}
	made := "\x00fold\x00" + group + "\x00" + rows[0].Fold + "\x00" + rows[0].Key
	key := made
	for n := 1; taken[key]; n++ {
		key = made + "\x00" + strconv.Itoa(n)
	}
	taken[key] = true
	return key
}

// foldEnd is where the rows sharing the Fold of the row at from end.
func foldEnd(rows []Row, from int) int {
	to := from + 1
	for rows[from].Fold != "" && to < len(rows) && rows[to].Fold == rows[from].Fold {
		to++
	}
	return to
}

// anyFold says some group in the frame holds a fold.
func (f *frameFolds) anyFold() bool { return f.work() && f.any }

// anyPanelText says some fold's row has text of its own for the side panel.
func (f *frameFolds) anyPanelText() bool { return f.work() && f.panelText }

// foldHolding is the key of the fold holding the row keyed key, and false where no fold does.
func (f *frameFolds) foldHolding(key string) (string, bool) {
	if !f.work() || key == "" {
		return "", false
	}
	folded, held := f.holding[key]
	return folded, held
}
