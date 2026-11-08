package gotermboard

import (
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/datapointchris/gotermstyle"
	"github.com/datapointchris/gotermstyle/tuitheme"
)

// liveTheme is the TUI theme file a program draws in, which every tab and reader of the
// program shares, so one reload recolors them all. A Theme never changes once loaded, so a
// reload swaps current.
type liveTheme struct {
	path    string
	current *tuitheme.Theme
	signals chan os.Signal
}

// loadTheme reads the theme file where tuitheme looks for it. A missing file is the built-in
// theme in ANSI 0-15 with no error. A file that will not load is the built-in theme too, and
// the error says why.
func loadTheme() (*liveTheme, error) {
	path, err := tuitheme.DefaultPath()
	if err != nil {
		return &liveTheme{current: tuitheme.Default()}, err
	}
	theme, err := tuitheme.Load(path)
	return &liveTheme{path: path, current: theme}, err
}

// themeLoadedMsg is the theme file read again on a reload request, or why it would not load.
type themeLoadedMsg struct {
	theme *tuitheme.Theme
	err   error
}

// watch waits for tuitheme's next request to read the theme file again, then reads it. A
// program's Init starts it, and reloaded starts each wait after.
func (lt *liveTheme) watch() tea.Cmd {
	if lt.path == "" {
		return nil
	}
	if lt.signals == nil {
		lt.signals = make(chan os.Signal, 1)
		tuitheme.Notify(lt.signals)
	}
	signals, path := lt.signals, lt.path
	return func() tea.Msg {
		<-signals
		theme, err := tuitheme.Load(path)
		return themeLoadedMsg{theme: theme, err: err}
	}
}

// reloaded is what every program root does with a theme read again: draw in it, or keep the
// theme on screen and say why where it would not load, then wait for the next request. A
// program root takes the message before handing any on, so a program holds one wait.
func (lt *liveTheme) reloaded(msg themeLoadedMsg, say func(error)) tea.Cmd {
	if msg.err != nil {
		say(msg.err)
	} else {
		lt.current = msg.theme
	}
	return lt.watch()
}

// builtInTheme is the theme a line draws in outside a board.
var builtInTheme = tuitheme.Default()

// builtInSelection is the selection the built-in theme gives, a light slab in ANSI 0-15
// that the cursor's row never wears.
var builtInSelection = builtInTheme.Style("ui.selection")

// cursorLook is how the cursor's row is drawn: across the table's width in the theme's
// selection background where it sets one other than the built-in, else behind ▌ in the
// pointer's column, bold where there is color. Each cell keeps its own color either way, so a
// state's color still reads on the cursor's row.
type cursorLook struct {
	painter painter
	// tint is the selection background's SGR sequence, empty where the theme's selection is
	// the built-in one or there is no color.
	tint string
	// bold draws the row bold over the tint too, as a board's Look.BoldCursor asks.
	bold bool
}

// pointer is what the pointer's column holds on the cursor's row.
func (c cursorLook) pointer() string {
	if c.tint != "" {
		return " "
	}
	return c.painter.bold("▌")
}

// wear draws a row's lines as the cursor's, a tinted line padded to width so the tint runs
// under the gaps between columns and out to the table's edge.
func (c cursorLook) wear(lines []drawn, width int) []drawn {
	if !c.painter.on() {
		return lines
	}
	code, room := Bold, 0
	switch {
	case c.tint != "" && c.bold:
		code, room = c.tint+Bold, width
	case c.tint != "":
		code, room = c.tint, width
	}
	for i := range lines {
		lines[i].text = laidOver(lines[i].text, code, room)
		if lines[i].whole != "" {
			lines[i].whole = laidOver(lines[i].whole, code, room)
		}
	}
	return lines
}

// laidOver is a painted line with code set again after each color in it, so code holds
// through the cells' own colors and the resets between them. It ends padded to width.
func laidOver(line, code string, width int) string {
	var b strings.Builder
	b.WriteString(code)
	for i := 0; i < len(line); {
		if end := sgrEnd(line, i); end > 0 {
			b.WriteString(line[i:end])
			b.WriteString(code)
			i = end
			continue
		}
		b.WriteByte(line[i])
		i++
	}
	b.WriteString(strings.Repeat(" ", max(width-ansi.StringWidth(line), 0)))
	b.WriteString(gotermstyle.CodeReset)
	return b.String()
}

// sgrEnd is where the SGR sequence starting at i ends, or 0 where none starts there.
func sgrEnd(line string, i int) int {
	if !strings.HasPrefix(line[i:], "\033[") {
		return 0
	}
	for j := i + 2; j < len(line); j++ {
		if c := line[j]; c >= 0x40 && c <= 0x7e {
			if c == 'm' {
				return j + 1
			}
			return 0
		}
	}
	return 0
}
