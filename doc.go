// Package gotermboard draws a self-refreshing terminal board on Bubble Tea v2:
// a tab per view, folding groups on shared columns, a side panel that follows the
// cursor, and one action table the keys, the footer and the ? overlay all read.
//
// A view is a read model, as in CQRS. Its reader joins sources into a snapshot,
// and its renderer turns the snapshot into a Frame this package draws. A failed
// read never gives up a good value: a Source keeps its last good read and says how
// old it is.
package gotermboard
