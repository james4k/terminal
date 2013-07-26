package terminal

import (
	"bufio"
	"bytes"
	"io"
	"unicode"
)

const (
	tabspaces = 8 // probably a better way to do this
)

const (
	glyphAttrReverse = 1 << iota
	glyphAttrUnderline
	glyphAttrBold
	glyphAttrGfx
	glyphAttrItalic
	glyphAttrBlink
	glyphAttrWrap
)

const (
	cursorDefault = 1 << iota
	cursorWrapNext
	cursorOrigin
)

type modeFlags uint32

const (
	modeWrap modeFlags = 1 << iota
	modeInsert
	modeAppKeypad
	modeAltScreen
	modeCRLF
	modeMouseButton
	modeMouseMotion
	modeReverse
	modeKeyboardLock
	modeHide
	modeEcho
	modeAppCursor
	modeMouseGr
	mode8bit
	modeBlink
	modeFBlink
	modeFocus
	modeMouseX10
	modeMouseMany
	modeMouseMask = modeMouseButton | modeMouseMotion | modeMouseX10 | modeMouseMany
)

const (
	escStart = 1 << iota
	escCSI
	escStr
	escAltCharset
	escStrEnd
	escTest
)

type glyph struct {
	c      rune
	mode   int16
	fg, bg uint16
}

type line []glyph

type cursor struct {
	attr  glyph
	x, y  int
	state uint8
}

type Term struct {
	cols, rows    int
	lines         []line
	altLines      []line
	dirty         []bool // line dirtiness
	cur, curSaved cursor
	top, bottom   int // scroll limits
	mode          modeFlags
	esc           int32 // escape state flags
	str           strEscape
	csi           csiEscape
	numlock       bool
	tabs          []bool
}

func New(columns, rows int) *Term {
	t := &Term{
		numlock: true,
	}
	t.resize(columns, rows)
	t.reset()
	return t
}

// Write takes pty input that is assumed to be utf8 encoded. Use io.Copy or
// ReadFrom() for better efficiency.
func (t *Term) Write(p []byte) (int, error) {
	n, err := t.ReadFrom(bytes.NewReader(p))
	return int(n), err
}

// ReadFrom reads from r until EOF or error. r is a pty file in the common
// case.
func (t *Term) ReadFrom(r io.Reader) (int64, error) {
	// FIXME: does it make sense to record bytes written? ATM just doing this
	// to conform to io.ReaderFrom
	var written int64
	buf := bufio.NewReader(r)
	for {
		c, sz, err := buf.ReadRune()
		if err != nil {
			return written, err
		}
		written += int64(sz)
		if c == unicode.ReplacementChar && sz == 1 {
			// TODO: should we just ignore?
			// TODO: create a debug log
			continue
		}
		t.put(c)
	}
	return written, nil
}

// TODO: eventually we should refactor this ESC state machine into
// something more idiomatic. method values instead of the bitmask.
func (t *Term) put(c rune) {
	control := c < 0x20 || c == 0177
	// STR sequences can contain control codes, so must check early
	if t.esc&escStr != 0 {
		switch c {
		case '\033':
			t.esc = escStart | escStrEnd
		case '\a': // backwards compatiblity to xterm
			t.esc = 0
			t.handleSTR()
		default:
			t.str.put(c)
		}
		return
	}

	// Directly from st:
	// Actions of control codes must be performed as soon as they arrive
	// because they can be embedded inside a control sequence, and they must
	// not cause conflicts with sequences.
	if control {
		switch c {
		// HT
		case '\t':
			t.putTab(true)
			return
		// BS
		case '\b':
			t.moveTo(t.cur.x-1, t.cur.y)
			return
		// CR
		case '\r':
			t.moveTo(0, t.cur.y)
			return
		// LF, VT, LF
		case '\f', '\v', '\n':
			// go to first col if mode is set
			t.newline(t.mode&modeCRLF != 0)
			return
		// BEL
		case '\a':
			// TODO: emit sound
			// TODO: window alert if not focused
			return
		// ESC
		case 033:
			t.csi.reset()
			t.esc = escStart
			return
		// SO, SI
		case 016, 017:
			// different charsets not supported. apps should use the correct
			// alt charset escapes, probably for line drawing
			return
		// SUB, CAN
		case 032, 030:
			t.csi.reset()
			return
		// ignore ENQ, NUL, XON, XOFF, DEL
		case 005, 000, 021, 023, 0177:
			return
		}
	} else if t.esc&escStart != 0 {
		if t.esc&escCSI != 0 {
			if t.csi.put(byte(c)) {
				t.handleCSI()
			}
		} else if t.esc&escStrEnd != 0 {
			t.esc = 0
			if c == '\\' {
				t.handleSTR()
			}
		} else if t.esc&escAltCharset != 0 {
			switch c {
			case '0': // line drawing set
				t.cur.attr.mode |= glyphAttrGfx
			case 'B': // USASCII
				t.cur.attr.mode &^= glyphAttrGfx
			case 'A', // UK (ignored)
				'<', // multinational (ignored)
				'5', // Finnish (ignored)
				'C', // Finnish (ignored)
				'K': // German (ignored)
			default:
				// TODO: stderr log, unhandled charset
			}
			t.esc = 0
		} else if t.esc&escTest != 0 {
			// DEC screen alignment test
			if c == '8' {
				for y := 0; y < t.rows; y++ {
					for x := 0; x < t.cols; x++ {
						t.setChar('E', &t.cur.attr, x, y)
					}
				}
			}
			t.esc = 0
		} else {
			switch c {
			case '[':
				t.esc |= escCSI
			case '#':
				t.esc |= escTest
			case 'P', // DCS - Device Control String
				'_', // APC - Application Program Command
				'^', // PM - Privacy Message
				']', // OSC - Operating System Command
				'k': // old title set compatibility
				t.str.reset()
				t.str.typ = c
				t.esc |= escStr
			case '(': // set primary charset G0
				t.esc |= escAltCharset
			case ')', // set secondary charset G1 (ignored)
				'*', // set tertiary charset G2 (ignored)
				'+': // set quaternary charset G3 (ignored)
				t.esc = 0
			case 'D': // IND - linefeed
				if t.cur.y == t.bottom {
					// TODO: t.scrollUp(t.top, 1)
				} else {
					t.moveTo(t.cur.x, t.cur.y+1)
				}
				t.esc = 0
			case 'E': // NEL - next line
				t.newline(true)
				t.esc = 0
			case 'H': // HTS - horizontal tab stop
				t.tabs[t.cur.x] = true
				t.esc = 0
			case 'M': // RI - reverse index
				if t.cur.y == t.top {
					// TODO: t.scrollDown(t.top, 1)
				} else {
					t.moveTo(t.cur.x, t.cur.y-1)
				}
			case 'Z': // DECID - identify terminal
				// TODO: write to our writer our id
				t.esc = 0
			case 'c': // RIS - reset to initial state
				t.reset()
				t.esc = 0
			case '=': // DECPAM - application keypad
				t.mode |= modeAppKeypad
				t.esc = 0
			case '>': // DECPNM - normal keypad
				t.mode &^= modeAppKeypad
				t.esc = 0
			case '7': // DECSC - save cursor
				t.saveCursor()
				t.esc = 0
			case '8': // DECRC - restore cursor
				t.restoreCursor()
				t.esc = 0
			case '\\': // ST - stop
				t.esc = 0
			default:
				// TODO: log to stderr unknown ESC sequence
				t.esc = 0
			}
		}
		// don't print characters that are part of a sequence
		return
	}

	if control && t.cur.attr.mode&glyphAttrGfx == 0 {
		return
	}

	// TODO: update selection

	if t.mode&modeWrap != 0 && t.cur.state&cursorWrapNext != 0 {
		t.lines[t.cur.y][t.cur.x].mode |= glyphAttrWrap
		t.newline(true)
	}

	if t.mode&modeInsert != 0 && t.cur.x+1 < t.cols {
		// TODO: move shiz, look at st.c:2458
	}

	t.setChar(c, &t.cur.attr, t.cur.x, t.cur.y)
	if t.cur.x+1 < t.cols {
		t.moveTo(t.cur.x+1, t.cur.y)
	} else {
		t.cur.state |= cursorWrapNext
	}
}

func (t *Term) putTab(forward bool) {
	x := t.cur.x
	if forward {
		if x == t.cols {
			return
		}
		for x++; x < t.cols && !t.tabs[x]; x++ {
		}
	} else {
		if x == 0 {
			return
		}
		for x--; x > 0 && !t.tabs[x]; x-- {
		}
	}
	t.moveTo(x, t.cur.y)
}

func (t *Term) newline(firstCol bool) {
	y := t.cur.y
	if y == t.bottom {
		// TODO: t.scrollUp(t.top, 1)
	} else {
		y++
	}
	if firstCol {
		t.moveTo(0, y)
	} else {
		t.moveTo(t.cur.x, y)
	}
}

// table from st, which in turn is from rxvt :)
var gfxCharTable = [62]rune{
	'↑', '↓', '→', '←', '█', '▚', '☃', // A - G
	0, 0, 0, 0, 0, 0, 0, 0, // H - O
	0, 0, 0, 0, 0, 0, 0, 0, // P - W
	0, 0, 0, 0, 0, 0, 0, ' ', // X - _
	'◆', '▒', '␉', '␌', '␍', '␊', '°', '±', // ` - g
	'␤', '␋', '┘', '┐', '┌', '└', '┼', '⎺', // h - o
	'⎻', '─', '⎼', '⎽', '├', '┤', '┴', '┬', // p - w
	'│', '≤', '≥', 'π', '≠', '£', '·', // x - ~
}

func (t *Term) setChar(c rune, attr *glyph, x, y int) {
	if attr.mode&glyphAttrGfx != 0 {
		if c >= 0x41 && c <= 0x7e && gfxCharTable[c-0x41] != 0 {
			c = gfxCharTable[c-0x41]
		}
	}
	t.dirty[y] = true
	t.lines[y][x] = *attr
	t.lines[y][x].c = c
}

func (t *Term) reset() {
	t.cur = cursor{}
	for i := range t.tabs {
		t.tabs[i] = false
	}
	t.top = 0
	t.bottom = t.rows - 1
	t.mode = modeWrap
	t.clear(0, 0, t.rows-1, t.cols-1)
	t.moveTo(0, 0)
	t.saveCursor()
}

// TODO: definitely can improve allocs
func (t *Term) resize(cols, rows int) bool {
	if cols < 1 || rows < 1 {
		return false
	}
	slide := t.cur.y - rows + 1
	/*
		if slide > 0 {
			copy(t.lines, t.lines[slide:slide+rows])
			copy(t.altLines, t.altLines[slide:slide+rows])
		}
	*/

	t.lines = make([]line, rows)
	t.altLines = make([]line, rows)
	t.dirty = make([]bool, rows)
	t.tabs = make([]bool, rows)

	for i := 0; i < rows; i++ {
		t.dirty[i] = true
		t.lines[i] = make(line, cols)
		t.altLines[i] = make(line, cols)
	}
	// TODO: update tabs? wtf is the tabs thign for anyways, a lookup table for
	// something?

	t.cols = cols
	t.rows = rows
	t.setScroll(0, rows-1)
	t.moveTo(t.cur.x, t.cur.y)
	t.clearAll()
	// TODO: reset t.tabs
	// TODO: tty resize via ioctl
	return slide > 0
}

func (t *Term) saveCursor() {
	t.curSaved = t.cur
}

func (t *Term) restoreCursor() {
	t.cur = t.curSaved
	t.moveTo(t.cur.x, t.cur.y)
}

func (t *Term) clear(x0, y0, x1, y1 int) {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	x0 = clamp(x0, 0, t.cols-1)
	x1 = clamp(x1, 0, t.cols-1)
	y0 = clamp(y0, 0, t.rows-1)
	y1 = clamp(y1, 0, t.rows-1)
	for y := y0; y <= y1; y++ {
		t.dirty[y] = true
		for x := x0; x <= x1; x++ {
			t.lines[y][x] = t.cur.attr
			t.lines[y][x].c = ' '
		}
	}
}

func (t *Term) clearAll() {
	t.clear(0, 0, t.cols-1, t.rows-1)
}

func (t *Term) moveTo(x, y int) {
	var miny, maxy int
	if t.cur.state&cursorOrigin != 0 {
		miny = t.top
		maxy = t.bottom
	} else {
		miny = 0
		maxy = t.rows - 1
	}
	x = clamp(x, 0, t.cols-1)
	y = clamp(y, miny, maxy)
	t.cur.state &^= cursorWrapNext
	t.cur.x = x
	t.cur.y = y
}

func (t *Term) swapScreen() {
	t.lines, t.altLines = t.altLines, t.lines
	t.mode ^= modeAltScreen
	t.dirtyAll()
}

func (t *Term) dirtyAll() {
	for y := 0; y < t.rows; y++ {
		t.dirty[y] = true
	}
}

func (t *Term) setScroll(top, bottom int) {
	top = clamp(top, 0, t.rows-1)
	bottom = clamp(bottom, 0, t.rows-1)
	if top > bottom {
		top, bottom = bottom, top
	}
	t.top = top
	t.bottom = bottom
}

func clamp(val, min, max int) int {
	if val < min {
		return min
	} else if val > max {
		return max
	}
	return val
}
