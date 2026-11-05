package gotermboard

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermstyle"
)

// readingWidth is the widest a reader lays text out, because a longer line is hard to
// follow back to its start. A wider screen keeps the rest as margin.
const readingWidth = 100

// hitContext is how many rows a search leaves above the row it lands on.
const hitContext = 2

// Reader is a full screen over one Document. It lays out every row at the width and draws
// the window that fits, because Bubble Tea keeps only the last lines of a taller view.
// It runs as a program of its own, or a Model opens one with Read.
type Reader struct {
	title    string
	document Document
	pal      gotermstyle.Palette
	// back is what esc does once nothing is left to clear. Nil means esc stays put.
	back func()
	quit func() tea.Cmd

	width, height int
	rows          []Line
	sections      []int
	top           int
	overlay       bool

	searching    bool
	searchedFrom int
	query        string
	hits         []int
	hit          int
}

// NewReader is a Reader over document, sized until the terminal says otherwise.
func NewReader(title string, document Document, pal gotermstyle.Palette) *Reader {
	r := &Reader{title: title, document: document, pal: pal, quit: func() tea.Cmd { return tea.Quit }}
	r.resize(80, 24)
	return r
}

// Once is the whole document at width, for output nobody scrolls.
func (r *Reader) Once(width int) string {
	var b strings.Builder
	for _, line := range r.document(width) {
		b.WriteString(line.paint(r.pal))
		b.WriteByte('\n')
	}
	return b.String()
}

// Init is Bubble Tea's.
func (r *Reader) Init() tea.Cmd { return nil }

// Update is Bubble Tea's.
func (r *Reader) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		r.resize(msg.Width, msg.Height)
	case tea.PasteMsg:
		r.paste(msg.Content)
	case tea.KeyPressMsg:
		cmd := r.pressed(msg)
		return r, cmd
	}
	return r, nil
}

// View is Bubble Tea's.
func (r *Reader) View() tea.View {
	v := tea.NewView(r.screen())
	v.AltScreen = true
	return v
}

// resize lays the text out again and keeps the same words at the top of the screen.
func (r *Reader) resize(width, height int) {
	above := lettersIn(r.rows[:min(r.top, len(r.rows))])
	r.width, r.height = width, height
	r.rows = r.document(min(max(width-2, 1), readingWidth))
	r.sections = r.sections[:0]
	for i, row := range r.rows {
		if slices.ContainsFunc(row, func(c Cell) bool { return c.Section }) {
			r.sections = append(r.sections, i)
		}
	}
	r.scrollTo(rowAfterLetters(r.rows, above))
	r.findHits()
	r.hit = max(slices.IndexFunc(r.hits, func(row int) bool { return row >= r.top }), 0)
}

func lettersIn(rows []Line) int {
	count := 0
	for _, row := range rows {
		for _, c := range row.Text() {
			if unicode.IsLetter(c) || unicode.IsDigit(c) {
				count++
			}
		}
	}
	return count
}

func rowAfterLetters(rows []Line, letters int) int {
	for i := range rows {
		if letters <= 0 {
			return i
		}
		letters -= lettersIn(rows[i : i+1])
	}
	return len(rows)
}

func (r *Reader) bodyHeight() int { return max(r.height-1, 1) }

func (r *Reader) scrollTo(top int) {
	r.top = max(min(top, len(r.rows)-r.bodyHeight()), 0)
}

// readerAction is one row of the key table the handler, the footer and the ? overlay read.
type readerAction struct {
	keys    []string
	label   string
	footer  string
	run     func(*Reader) tea.Cmd
	applies func(*Reader) bool
}

func readerActions() []readerAction {
	by := func(lines func(*Reader) int) func(*Reader) tea.Cmd {
		return func(r *Reader) tea.Cmd {
			r.scrollTo(r.top + lines(r))
			return nil
		}
	}
	one := func(lines int) func(*Reader) int { return func(*Reader) int { return lines } }
	half := func(direction int) func(*Reader) int {
		return func(r *Reader) int { return direction * max(r.bodyHeight()/2, 1) }
	}
	all := func(direction int) func(*Reader) int {
		return func(r *Reader) int { return direction * len(r.rows) }
	}
	hasSections := func(r *Reader) bool { return len(r.sections) > 0 }
	hasHits := func(r *Reader) bool { return len(r.hits) > 0 }
	return []readerAction{
		{keys: []string{"j", "down"}, label: "scroll down a line", footer: "j/k scroll", run: by(one(1))},
		{keys: []string{"k", "up"}, label: "scroll up a line", run: by(one(-1))},
		{keys: []string{"ctrl+d", "pgdown"}, label: "half a screen down", run: by(half(1))},
		{keys: []string{"ctrl+u", "pgup"}, label: "half a screen up", run: by(half(-1))},
		{keys: []string{"g", "home"}, label: "go to the top", run: by(all(-1))},
		{keys: []string{"G", "end"}, label: "go to the bottom", run: by(all(1))},
		{keys: []string{"]"}, label: "jump to the next section", footer: "[/] section", run: func(r *Reader) tea.Cmd {
			if at := slices.IndexFunc(r.sections, func(row int) bool { return row > r.top }); at >= 0 {
				r.scrollTo(r.sections[at])
			}
			return nil
		}, applies: hasSections},
		{keys: []string{"["}, label: "jump to the previous section", run: func(r *Reader) tea.Cmd {
			for _, row := range slices.Backward(r.sections) {
				if row < r.top {
					r.scrollTo(row)
					break
				}
			}
			return nil
		}, applies: hasSections},
		{keys: []string{"/"}, label: "search the text", footer: "/ search", run: func(r *Reader) tea.Cmd {
			r.searching, r.searchedFrom = true, r.top
			r.setQuery("")
			return nil
		}},
		{keys: []string{"n"}, label: "go to the next match", run: func(r *Reader) tea.Cmd {
			r.landOnHit(r.hit + 1)
			return nil
		}, applies: hasHits},
		{keys: []string{"N"}, label: "go to the previous match", run: func(r *Reader) tea.Cmd {
			r.landOnHit(r.hit - 1)
			return nil
		}, applies: hasHits},
		{keys: []string{"?"}, label: "show these keys", footer: "? keys", run: func(r *Reader) tea.Cmd {
			r.overlay = true
			return nil
		}},
		{keys: []string{"esc"}, label: "clear the search, or go back", footer: "esc back", run: func(r *Reader) tea.Cmd {
			if r.query != "" {
				r.setQuery("")
			} else if r.back != nil {
				r.back()
			}
			return nil
		}, applies: func(r *Reader) bool { return r.back != nil || r.query != "" }},
		{keys: []string{"q", "ctrl+c"}, label: "quit", footer: "q quit", run: func(r *Reader) tea.Cmd { return r.quit() }},
	}
}

func (r *Reader) actions() []readerAction {
	return slices.DeleteFunc(readerActions(), func(a readerAction) bool {
		return a.applies != nil && !a.applies(r)
	})
}

func (r *Reader) pressed(key tea.KeyPressMsg) tea.Cmd {
	if r.overlay {
		r.overlay = false
		return nil
	}
	if r.searching {
		r.typed(key)
		return nil
	}
	name := key.String()
	for _, action := range r.actions() {
		if slices.Contains(action.keys, name) {
			return action.run(r)
		}
	}
	return nil
}

func (r *Reader) typed(key tea.KeyPressMsg) {
	switch key.String() {
	case "enter":
		r.searching = false
	case "esc", "ctrl+c":
		r.searching = false
		r.setQuery("")
		r.scrollTo(r.searchedFrom)
	case "backspace":
		if query := []rune(r.query); len(query) > 0 {
			r.setQuery(string(query[:len(query)-1]))
		}
	case "ctrl+u":
		r.setQuery("")
	default:
		if key.Mod&^tea.ModShift == 0 && key.Text != "" {
			r.setQuery(r.query + key.Text)
		}
	}
}

func (r *Reader) paste(content string) {
	if r.searching {
		r.setQuery(r.query + strings.Join(strings.Fields(content), " "))
	}
}

// setQuery lands on the first match below where the search began, as a pager's
// incremental search does.
func (r *Reader) setQuery(query string) {
	r.query = query
	r.findHits()
	if len(r.hits) == 0 {
		if r.searching {
			r.scrollTo(r.searchedFrom)
		}
		return
	}
	r.landOnHit(max(slices.IndexFunc(r.hits, func(row int) bool { return row >= r.searchedFrom }), 0))
}

func (r *Reader) landOnHit(hit int) {
	if len(r.hits) == 0 {
		return
	}
	r.hit = (hit + len(r.hits)) % len(r.hits)
	r.scrollTo(r.hits[r.hit] - hitContext)
}

func (r *Reader) findHits() {
	r.hits = r.hits[:0]
	if r.query == "" {
		return
	}
	for i, row := range r.rows {
		if slices.Contains(r.matchedIn(row.Text()), true) {
			r.hits = append(r.hits, i)
		}
	}
}

// matchedIn says which runes of text are part of a match. A query with no capital in it
// ignores case, as ripgrep's --smart-case does.
func (r *Reader) matchedIn(text string) []bool {
	fold := func(s string) []rune { return []rune(s) }
	if !strings.ContainsFunc(r.query, unicode.IsUpper) {
		fold = func(s string) []rune {
			runes := []rune(s)
			for i, c := range runes {
				runes[i] = unicode.ToLower(c)
			}
			return runes
		}
	}
	runes, query := fold(text), fold(r.query)
	matched := make([]bool, len(runes))
	if len(query) == 0 {
		return matched
	}
	for i := 0; i+len(query) <= len(runes); i++ {
		if slices.Equal(runes[i:i+len(query)], query) {
			for j := range query {
				matched[i+j] = true
			}
			i += len(query) - 1
		}
	}
	return matched
}

// marked is row with each match reversed, its other cells as they were.
func (r *Reader) marked(row Line) Line {
	if r.query == "" {
		return row
	}
	matched := r.matchedIn(row.Text())
	if !slices.Contains(matched, true) {
		return row
	}
	var out Line
	at := 0
	for _, cell := range row {
		runes := []rune(cell.Text)
		for start := 0; start < len(runes); {
			end := start + 1
			for end < len(runes) && matched[at+end] == matched[at+start] {
				end++
			}
			code := cell.Code
			if matched[at+start] {
				code = Reverse
			}
			out = append(out, C(string(runes[start:end]), code))
			start = end
		}
		at += len(runes)
	}
	return out
}

func (r *Reader) screen() string {
	body := r.bodyHeight()
	lines := make([]string, 0, body+1)
	if r.overlay {
		lines = append(lines, r.pal.Bold("Keys, any key closes this"))
		for _, action := range r.actions() {
			lines = append(lines, "  "+fit(strings.Join(action.keys, " "), 16, false)+"  "+action.label)
		}
		lines = lines[:min(len(lines), body)]
	} else {
		margin := strings.Repeat(" ", max((r.width-readingWidth)/2, 1))
		for _, row := range r.rows[r.top:min(r.top+body, len(r.rows))] {
			lines = append(lines, ansi.Truncate(margin+r.marked(row).paint(r.pal), r.width, "…"))
		}
	}
	for len(lines) < body {
		lines = append(lines, "")
	}
	return strings.Join(append(lines, ansi.Truncate(r.footer(), r.width, "…")), "\n")
}

// footer ends in the title, because it is the part a narrow screen can lose.
func (r *Reader) footer() string {
	parts := []string{fmt.Sprintf("%d/%d", min(r.top+r.bodyHeight(), len(r.rows)), len(r.rows))}
	switch {
	case r.searching:
		parts = append(parts, "/"+r.query+"█", r.hitsSaid())
	case r.query != "":
		parts = append(parts, "/"+r.query, r.hitsSaid(), "n/N match", "esc clears")
	default:
		for _, action := range r.actions() {
			if action.footer != "" {
				parts = append(parts, action.footer)
			}
		}
	}
	return strings.Join(append(parts, r.title), "  ")
}

func (r *Reader) hitsSaid() string {
	switch {
	case r.query == "":
		return ""
	case len(r.hits) == 0:
		return "no match"
	}
	return fmt.Sprintf("%d of %d", r.hit+1, len(r.hits))
}
