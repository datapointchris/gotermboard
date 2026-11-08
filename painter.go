package gotermboard

import "github.com/datapointchris/gotermstyle"

// painter is what a board, its tabs and its readers draw every cell with: the palette, and the
// live TUI theme a cell's Role resolves in. A reload swaps the theme it holds, so the next draw
// recolors every role. Line.Paint is the way to draw outside a board, in the built-in theme.
type painter struct {
	pal   gotermstyle.Palette
	theme *liveTheme
}

// line is l in its cells' colors.
func (p painter) line(l Line) string { return l.paint(p.pal, p.theme.current) }

// cell is c's text in its color: its Role in the live theme, then its Code.
func (p painter) cell(c Cell) string { return p.pal.Paint(c.code(p.theme.current), c.Text) }

func (p painter) bold(text string) string { return p.pal.Bold(text) }

// on says the palette draws color.
func (p painter) on() bool { return p.pal.On() }

// cursorLook is how the cursor's row is drawn in the live theme.
func (p painter) cursorLook() cursorLook {
	selection := p.theme.current.Style("ui.selection")
	if !p.on() || selection == builtInSelection {
		return cursorLook{painter: p}
	}
	return cursorLook{painter: p, tint: selection.Bg.BackgroundCode()}
}

// matchCode is what a search match draws in: the theme's match color in bold, over the
// selection background where the theme sets one, else reversed. Either way it stands apart from
// text that already wears the match color, as a code span can.
func (p painter) matchCode() string {
	match := emboldened(p.theme.current.Code("ui.match"))
	if tint := p.cursorLook().tint; tint != "" {
		return match + tint
	}
	return match + Reverse
}
