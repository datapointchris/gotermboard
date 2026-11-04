# gotermboard

A self-refreshing terminal board for Go command-line tools, on Bubble Tea v2.

A program built on it has a tab per view. Each view is one table of folding groups
sharing one set of columns, with a side panel that follows the cursor. Each source
of data refreshes at its own pace, and the board draws its last good read with that
read's age, so a failing backend leaves the table drawn and says the read is old.
