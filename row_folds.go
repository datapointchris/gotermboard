package gotermboard

import "strconv"

// FoldKey switches the whole board between drawing each fold as one row and drawing every row.
const FoldKey = "Z"

// rowFold is consecutive rows of one group sharing a Fold, and the row the frame's FoldRow made of
// them.
type rowFold struct {
	// from and to bound rows in the group's Rows.
	from, to int
	rows     []Row
	row      Row
}

// frameFolds is each group's folds in one frame. A walk works a group's out the first time it
// asks, so FoldRow runs once per fold per frame, and never while the board draws every row. It is
// dropped with its frame.
type frameFolds struct {
	foldRow func(rows []Row) Row
	groups  []Group
	// of is keyed by a group's first row, which every copy of the group shares.
	of map[*Row][]rowFold
	// any says some group holds a fold, where known says it was worked out.
	any, known bool
}

func newFrameFolds() *frameFolds { return &frameFolds{of: map[*Row][]rowFold{}} }

func (f *frameFolds) newFrame(groups []Group, foldRow func(rows []Row) Row) {
	if f == nil {
		return
	}
	f.groups, f.foldRow, f.known = groups, foldRow, false
	clear(f.of)
}

// in is g's folds, in the order of its rows.
func (f *frameFolds) in(g *Group) []rowFold {
	if f == nil || f.foldRow == nil || len(g.Rows) < 2 {
		return nil
	}
	if folds, known := f.of[&g.Rows[0]]; known {
		return folds
	}
	var folds []rowFold
	repeats := map[string]int{}
	for from := 0; from < len(g.Rows); {
		to := foldEnd(g.Rows, from)
		if to-from > 1 {
			rows := g.Rows[from:to:to]
			row := f.foldRow(rows)
			// A row made from a copy of one of its rows would otherwise wear that row's cue as it was.
			row.changed = false
			if row.Key == "" {
				row.Key = foldedKey(g.Key, rows[0].Fold, repeats)
			}
			folds = append(folds, rowFold{from: from, to: to, rows: rows, row: row})
		}
		from = to
	}
	f.of[&g.Rows[0]] = folds
	return folds
}

// foldEnd is where the rows sharing the Fold of the row at from end.
func foldEnd(rows []Row, from int) int {
	to := from + 1
	for rows[from].Fold != "" && to < len(rows) && rows[to].Fold == rows[from].Fold {
		to++
	}
	return to
}

// foldedKey keys a fold FoldRow left unkeyed by its group and its Fold, and by how many folds of
// that Fold came before it in the group, so it holds as rows join either end. The NUL bytes keep it
// from any key a board writes.
func foldedKey(group, fold string, repeats map[string]int) string {
	key := "\x00fold\x00" + group + "\x00" + fold
	if n := repeats[fold]; n > 0 {
		key += "\x00" + strconv.Itoa(n)
	}
	repeats[fold]++
	return key
}

// anyFold says some group in the frame holds a fold.
func (f *frameFolds) anyFold() bool {
	if f == nil || f.foldRow == nil {
		return false
	}
	if !f.known {
		f.any, f.known = groupsFold(f.groups), true
	}
	return f.any
}

func groupsFold(groups []Group) bool {
	for i := range groups {
		g := &groups[i]
		for j := 1; j < len(g.Rows); j++ {
			if g.Rows[j].Fold != "" && g.Rows[j].Fold == g.Rows[j-1].Fold {
				return true
			}
		}
		if groupsFold(g.Groups) {
			return true
		}
	}
	return false
}

// holding is the key of the fold holding the row keyed key, and false where no fold does.
func (f *frameFolds) holding(groups []Group, key string) (string, bool) {
	for i := range groups {
		g := &groups[i]
		for _, fold := range f.in(g) {
			for _, r := range fold.rows {
				if r.Key == key {
					return fold.row.Key, true
				}
			}
		}
		if folded, ok := f.holding(g.Groups, key); ok {
			return folded, true
		}
	}
	return "", false
}
