// Package terminalcontrol takes the terminal's controls out of stored or remote text, so that
// text cannot drive the terminal it is drawn on. OSC 52 in such text sets the clipboard, and
// CSI moves the cursor to hide lines on the screen or fake them. A program strips where it prints
// the text and keeps the stored value as written, as an HTML template escapes at render time.
// The package imports nothing that draws, so a command that only prints pays for no board.
package terminalcontrol

import (
	"strings"
	"unicode/utf8"
)

const esc = 0x1b

// Strip is text without its terminal controls. Each escape sequence leaves whole, whether ESC
// or its one-byte C1 form opens it: CSI, OSC, DCS, SOS, PM, APC, and ESC with the bytes it
// takes. Every other C0 and C1 control leaves too, and so does DEL. A newline and a tab stay,
// and each ends a sequence it falls inside, so the text keeps its lines however it is split.
// Text holding no control comes back as it was, with nothing allocated.
func Strip(text string) string {
	start := firstControl(text)
	if start == len(text) {
		return text
	}
	var b strings.Builder
	b.Grow(len(text))
	b.WriteString(text[:start])
	for i := start; i < len(text); {
		c := text[i]
		switch {
		case c == '\n' || c == '\t':
			b.WriteByte(c)
			i++
		case c == esc:
			i = afterEscape(text, i+1)
		case c < 0x20 || c == 0x7f:
			i++
		case c < utf8.RuneSelf:
			b.WriteByte(c)
			i++
		default:
			r, size := utf8.DecodeRuneInString(text[i:])
			if control, ok := c1(r, c, size); ok {
				i = afterC1(text, i+size, control)
				continue
			}
			b.WriteString(text[i : i+size])
			i += size
		}
	}
	return b.String()
}

// firstControl is where text's first control starts, or its length where it holds none.
func firstControl(text string) int {
	for i := 0; i < len(text); {
		c := text[i]
		if c < utf8.RuneSelf {
			if (c < 0x20 && c != '\n' && c != '\t') || c == 0x7f {
				return i
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		if _, ok := c1(r, c, size); ok {
			return i
		}
		i += size
	}
	return len(text)
}

// c1 is the C1 control a decoded rune stands for: a code point from U+0080 to U+009F, or a
// byte in that range standing alone, outside any character.
func c1(r rune, lead byte, size int) (byte, bool) {
	switch {
	case r >= 0x80 && r <= 0x9f:
		return byte(r), true
	case r == utf8.RuneError && size == 1 && lead >= 0x80 && lead <= 0x9f:
		return lead, true
	}
	return 0, false
}

// afterEscape is where the sequence ends whose ESC sits just before i. ESC and a byte from
// 0x40 to 0x5F is that byte's C1 control. ESC and intermediate bytes take one final byte,
// and ESC and any other byte from 0x30 to 0x7E is the whole sequence. An ESC before anything
// else leaves alone.
func afterEscape(text string, i int) int {
	if i == len(text) {
		return i
	}
	switch c := text[i]; {
	case c >= 0x40 && c <= 0x5f:
		return afterC1(text, i+1, c+0x40)
	case c >= 0x20 && c <= 0x2f:
		for i < len(text) && text[i] >= 0x20 && text[i] <= 0x2f {
			i++
		}
		if i < len(text) && text[i] >= 0x30 && text[i] <= 0x7e {
			i++
		}
		return i
	case c >= 0x30 && c <= 0x7e:
		return i + 1
	}
	return i
}

// afterC1 is where a sequence ends when control opens it, with its body starting at i. CSI
// runs through its parameter and intermediate bytes to its final byte. OSC, DCS, SOS, PM and
// APC each run to their terminator. Any other C1 control stands alone.
func afterC1(text string, i int, control byte) int {
	switch control {
	case 0x9b:
		for i < len(text) && text[i] >= 0x20 && text[i] <= 0x3f {
			i++
		}
		if i < len(text) && text[i] >= 0x40 && text[i] <= 0x7e {
			i++
		}
		return i
	case 0x90, 0x98, 0x9d, 0x9e, 0x9f:
		return afterString(text, i)
	}
	return i
}

// afterString is where a control string's terminator ends: BEL, ESC \, or ST as a C1 control.
// A newline, a tab, an ESC opening anything else, or the text's end stops it short of one.
func afterString(text string, i int) int {
	for i < len(text) {
		r, size := utf8.DecodeRuneInString(text[i:])
		control, isC1 := c1(r, text[i], size)
		switch {
		case r == '\a' || isC1 && control == 0x9c:
			return i + size
		case r == '\n' || r == '\t':
			return i
		case r == esc:
			if i+1 < len(text) && text[i+1] == '\\' {
				return i + 2
			}
			return i
		}
		i += size
	}
	return i
}
