package gotermboard

import tea "charm.land/bubbletea/v2"

// foldPrefixKey is vim's z, which leaves the table waiting for the fold key after it.
const foldPrefixKey = "z"

// prefixAction is key as a prefix key: pressing it leaves the table waiting, and the next key
// runs the one of second bound to it or, bound to none, ends the wait and does nothing.
func prefixAction(key string, second []Action) Action {
	prefix := Action{Keys: []string{key}, secondKeys: second}
	prefix.Run = func(m *Model) tea.Cmd {
		m.pendingPrefix = &prefix
		return nil
	}
	return prefix
}

// foldActions is the keys after z, as vim's are. A group's fold, a row's text open in place and
// a fold's rows are all folds here, as vim has one kind, so reading a tree start to finish needs
// one key.
func foldActions() []Action {
	onGroupOrOpening := func(m *Model) bool {
		n, ok := m.selected()
		return ok && (n.kind == nodeGroup || n.toggles)
	}
	return []Action{
		{
			Keys: []string{"R"}, Label: "open every group and every row's text in place, through the whole table",
			Footer: "R open all", Run: (*Model).OpenAll,
		},
		{
			Keys: []string{"M"}, Label: "fold every group and close every row's text, through the whole table",
			Footer: "M fold all", Run: (*Model).FoldAll,
		},
		{
			Keys: []string{"O"}, Label: "open the cursor's group or row and every group and row's text under it",
			Footer: "O open here", Run: (*Model).OpenUnderCursor, Applies: onGroupOrOpening,
		},
		{
			Keys: []string{"C"}, Label: "fold the cursor's group or row and every group and row's text under it",
			Footer: "C fold here", Run: (*Model).FoldUnderCursor, Applies: onGroupOrOpening,
		},
	}
}

// pressedAfterPrefix ends the wait a prefix key began, running the second key bound to key.
func (m *Model) pressedAfterPrefix(key string) tea.Cmd {
	prefix := m.pendingPrefix
	m.pendingPrefix = nil
	cmd, _ := RunKey(m, applyingTo(m, prefix.secondKeys), key)
	return cmd
}

// OpenAll is zR: it opens every group in the table, every fold, and every row's text in place,
// however deep a folded group held it, so the whole tree reads start to finish. Each is kept as
// space keeps it.
func (m *Model) OpenAll() tea.Cmd {
	m.openEach(m.everyNode(), true)
	m.settleAfterFolds(nil)
	m.revealDetail()
	return nil
}

// FoldAll is zM: it folds every group in the table, every fold, and closes every row's text. A
// cursor whose line folds away goes to the group that took it in.
func (m *Model) FoldAll() tea.Cmd {
	owners := m.cursorOwners()
	m.openEach(m.everyNode(), false)
	m.settleAfterFolds(owners)
	m.displayChanged()
	return nil
}

// OpenUnderCursor is zO: it opens the cursor's group or row and every group, fold and row's text
// under it, and nothing beside it.
func (m *Model) OpenUnderCursor() tea.Cmd {
	m.openEach(m.underCursor(), true)
	m.settleAfterFolds(nil)
	m.revealDetail()
	return nil
}

// FoldUnderCursor is zC: it folds the cursor's group or row and every group, fold and row's text
// under it, and nothing beside it.
func (m *Model) FoldUnderCursor() tea.Cmd {
	owners := m.cursorOwners()
	m.openEach(m.underCursor(), false)
	m.settleAfterFolds(owners)
	m.displayChanged()
	return nil
}

// everyNode is every group, fold and row the table would draw were every group and fold open,
// in the order it would draw them.
func (m *Model) everyNode() []node {
	d := m.display
	d.panelShown = m.panelWidth() > 0
	d.everyOpen = true
	return d.walk(m.frame.Groups, 0, -1, false, nil)
}

// underCursor is the cursor's node and every node under it, from everyNode.
func (m *Model) underCursor() []node {
	at, ok := m.selected()
	if !ok {
		return nil
	}
	every := m.everyNode()
	var under []node
	inside := map[int]bool{}
	for i, n := range every {
		switch {
		case n.kind == at.kind && n.key == at.key && n.selectable():
		case n.parent >= 0 && inside[n.parent]:
		default:
			continue
		}
		inside[i] = true
		under = append(under, n)
	}
	return under
}

// openEach opens or folds each of nodes that toggles and is not that way already. A group's fold
// and a row's text are kept, and a fold's is not, as space keeps them.
func (m *Model) openEach(nodes []node, open bool) {
	now := m.display
	now.panelShown = m.panelWidth() > 0
	for _, n := range nodes {
		switch {
		case !n.toggles:
		case n.kind == nodeGroup:
			if now.isOpen(n.group) != open {
				m.display.fold(n.key, !open)
				m.rememberFolded(n.key, !open)
			}
		case n.kind == nodeRow && n.fold != nil:
			if now.isFoldOpen(n.fold) != open {
				m.openInPlace(n, open)
			}
		case n.kind == nodeRow:
			if now.isRowOpen(n.row) != open {
				m.openInPlace(n, open)
			}
		}
	}
}

// cursorOwners is the keys of the groups and folds holding the cursor's line, innermost first.
func (m *Model) cursorOwners() []string {
	k := m.kept()
	if m.cursorAt < 0 || m.cursorAt >= len(k.ix.stops) {
		return nil
	}
	var owners []string
	for at := k.nodes[k.ix.stops[m.cursorAt]].parent; at >= 0; at = k.nodes[at].parent {
		owners = append(owners, k.nodes[at].key)
	}
	return owners
}

// settleAfterFolds puts the cursor back on its line, or on the innermost of owners that shows
// where a fold took its line in.
func (m *Model) settleAfterFolds(owners []string) {
	stops := m.kept().ix.stopAt
	if _, shows := stops[m.cursor]; !shows {
		for _, owner := range owners {
			if _, shows := stops[owner]; shows {
				m.cursor = owner
				break
			}
		}
	}
	m.place()
}
