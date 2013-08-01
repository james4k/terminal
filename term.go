package terminal

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"unicode"
)

const (
	tabspaces = 8

	defaultfg = 7
	defaultbg = 0
	defaultcs = 256
)

const (
	attrReverse = 1 << iota
	attrUnderline
	attrBold
	attrGfx
	attrItalic
	attrBlink
	attrWrap
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
	modeMouseSgr
	mode8bit
	modeBlink
	modeFBlink
	modeFocus
	modeMouseX10
	modeMouseMany
	modeMouseMask = modeMouseButton | modeMouseMotion | modeMouseX10 | modeMouseMany
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

type stateFn func(c rune)

type Term struct {
	cols, rows    int
	lines         []line
	altLines      []line
	dirty         []bool // line dirtiness
	cur, curSaved cursor
	top, bottom   int // scroll limits
	mode          modeFlags
	state         stateFn
	str           strEscape
	csi           csiEscape
	numlock       bool
	tabs          []bool
	pty           *os.File

	Stderr io.Writer // defaults to os.Stderr
}

func New(columns, rows int, pty *os.File) *Term {
	t := &Term{
		numlock: true,
		pty:     pty,
	}
	t.state = t.parse
	t.Stderr = os.Stderr
	t.resize(columns, rows)
	t.reset()
	return t
}

func (t *Term) logf(format string, args ...interface{}) {
	fmt.Fprintf(t.Stderr, format, args...)
}

func (t *Term) log(s string) {
	fmt.Fprintln(t.Stderr, s)
}

func (t *Term) Cell(x, y int) (rune, uint16, uint16) {
	return t.lines[y][x].c, t.lines[y][x].fg, t.lines[y][x].bg
}

func (t *Term) Cursor() (int, int) {
	return t.cur.x, t.cur.y
}

// Write takes pty input that is assumed to be utf8 encoded. Use io.Copy or
// ReadFrom() for better efficiency and simpler usage.
func (t *Term) Write(p []byte) (int, error) {
	// FIXME: The bufio.NewReader() call in ReadFrom has a sizeable alloc
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
			t.log("encountered invalid utf8 sequence")
			continue
		}
		t.put(c)
	}
	return written, nil
}

func (t *Term) put(c rune) {
	t.state(c)
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
		t.scrollUp(t.top, 1)
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
	if attr.mode&attrGfx != 0 {
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
	for i := tabspaces; i < len(t.tabs); i += tabspaces {
		t.tabs[i] = true
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
	if slide > 0 {
		copy(t.lines, t.lines[slide:slide+rows])
		copy(t.altLines, t.altLines[slide:slide+rows])
	}

	t.lines = make([]line, rows)
	t.altLines = make([]line, rows)
	t.dirty = make([]bool, rows)
	t.tabs = make([]bool, cols)

	for i := 0; i < rows; i++ {
		t.dirty[i] = true
		t.lines[i] = make(line, cols)
		t.altLines[i] = make(line, cols)
	}
	for i := range t.tabs {
		t.tabs[i] = false
	}
	for i := tabspaces; i < len(t.tabs); i += tabspaces {
		t.tabs[i] = true
	}

	t.cols = cols
	t.rows = rows
	t.setScroll(0, rows-1)
	t.moveTo(t.cur.x, t.cur.y)
	// TODO: clear alt screen
	// TODO: clear screen (only appropriate rows/cols)
	// TODO: tty resize via ioctl
	t.ttyResize()
	t.log("resize")
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

func (t *Term) moveAbsTo(x, y int) {
	if t.cur.state&cursorOrigin != 0 {
		y += t.top
	}
	t.moveTo(x, y)
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

func between(val, min, max int) bool {
	if val < min || val > max {
		return false
	}
	return true
}

func (t *Term) scrollDown(orig, n int) {
	n = clamp(n, 0, t.bottom-orig+1)
	t.clear(0, t.bottom-n+1, t.cols-1, t.bottom)
	for i := t.bottom; i >= orig+n; i-- {
		t.lines[i], t.lines[i-n] = t.lines[i-n], t.lines[i]
		t.dirty[i] = true
		t.dirty[i-n] = true
	}

	// TODO: selection scroll
}

func (t *Term) scrollUp(orig, n int) {
	n = clamp(n, 0, t.bottom-orig+1)
	t.clear(0, orig, t.cols-1, orig+n-1)
	for i := orig; i <= t.bottom-n; i++ {
		t.lines[i], t.lines[i+n] = t.lines[i+n], t.lines[i]
		t.dirty[i] = true
		t.dirty[i+n] = true
	}

	// TODO: selection scroll
}

func (t *Term) modMode(set bool, bit modeFlags) {
	if set {
		t.mode |= bit
	} else {
		t.mode &^= bit
	}
}

func (t *Term) setMode(priv bool, set bool, args []int) {
	if priv {
		for _, a := range args {
			switch a {
			case 1: // DECCKM - cursor key
				t.modMode(set, modeAppCursor)
			case 5: // DECSCNM - reverse video
				mode := t.mode
				t.modMode(set, modeReverse)
				if mode != t.mode {
					// TODO: redraw
				}
			case 6: // DECOM - origin
				if set {
					t.cur.state |= cursorOrigin
				} else {
					t.cur.state &^= cursorOrigin
				}
				t.moveAbsTo(0, 0)
			case 7: // DECAWM - auto wrap
				t.modMode(set, modeWrap)
			// IGNORED:
			case 0, // error
				2,  // DECANM - ANSI/VT52
				3,  // DECCOLM - column
				4,  // DECSCLM - scroll
				8,  // DECARM - auto repeat
				18, // DECPFF - printer feed
				19, // DECPEX - printer extent
				42, // DECNRCM - national characters
				12: // att610 - start blinking cursor
				break
			case 25: // DECTCEM - text cursor enable mode
				t.modMode(!set, modeHide)
			case 9: // X10 mouse compatibility mode
				t.modMode(false, modeMouseMask)
				t.modMode(set, modeMouseX10)
			case 1000: // report button press
				t.modMode(false, modeMouseMask)
				t.modMode(set, modeMouseButton)
			case 1002: // report motion on button press
				t.modMode(false, modeMouseMask)
				t.modMode(set, modeMouseMotion)
			case 1003: // enable all mouse motions
				t.modMode(false, modeMouseMask)
				t.modMode(set, modeMouseMany)
			case 1004: // send focus events to tty
				t.modMode(set, modeFocus)
			case 1006: // extended reporting mode
				t.modMode(set, modeMouseSgr)
			case 1034:
				t.modMode(set, mode8bit)
			case 1049, // = 1047 and 1048
				47, 1047:
				alt := t.mode&modeAltScreen != 0
				if alt {
					t.clear(0, 0, t.cols-1, t.rows-1)
				}
				t.log("alt screen...")
				if !set || !alt {
					t.swapScreen()
					t.log("swapped screen")
				}
				if a != 1049 {
					break
				}
				fallthrough
			case 1048:
				if set {
					t.saveCursor()
				} else {
					t.restoreCursor()
				}
			case 1001:
				// mouse highlight mode; can hang the terminal by design when
				// implemented
			case 1005:
				// utf8 mouse mode; will confuse applications not supporting
				// utf8 and luit
			case 1015:
				// urxvt mangled mouse mode; incompatiblt and can be mistaken
				// for other control codes
			default:
				t.logf("unknown private set/reset mode %d\n", a)
			}
		}
	} else {
		for _, a := range args {
			switch a {
			case 0: // Error (ignored)
			case 2: // KAM - keyboard action
				t.modMode(set, modeKeyboardLock)
			case 4: // IRM - insertion-replacement
				t.modMode(set, modeInsert)
			case 12: // SRM - send/receive
				t.modMode(set, modeEcho)
			case 20: // LNM - linefeed/newline
				t.modMode(set, modeCRLF)
			default:
				t.logf("unknown set/reset mode %d\n", a)
			}
		}
	}
}

func (t *Term) setAttr(attr []int) {
	for i := 0; i < len(attr); i++ {
		a := attr[i]
		switch a {
		case 0:
			t.cur.attr.mode &^= attrReverse | attrUnderline | attrBold | attrItalic | attrBlink
			t.cur.attr.fg = defaultfg
			t.cur.attr.bg = defaultbg
		case 1:
			t.cur.attr.mode |= attrBold
		case 3:
			t.cur.attr.mode |= attrItalic
		case 4:
			t.cur.attr.mode |= attrUnderline
		case 5, 6: // slow, rapid blink
			t.cur.attr.mode |= attrBlink
		case 7:
			t.cur.attr.mode |= attrReverse
		case 21, 22:
			t.cur.attr.mode &^= attrBold
		case 23:
			t.cur.attr.mode &^= attrItalic
		case 24:
			t.cur.attr.mode &^= attrUnderline
		case 25, 26:
			t.cur.attr.mode &^= attrBlink
		case 27:
			t.cur.attr.mode &^= attrReverse
		case 38:
			if i+2 < len(attr) && attr[i+1] == 5 {
				i += 2
				if between(attr[i], 0, 255) {
					t.cur.attr.fg = uint16(attr[i])
				} else {
					t.logf("bad fgcolor %d\n", attr[i])
				}
			} else {
				t.logf("gfx attr %d unknown\n", a)
			}
		case 39:
			t.cur.attr.fg = defaultfg
		case 48:
			if i+2 < len(attr) && attr[i+1] == 5 {
				i += 2
				if between(attr[i], 0, 255) {
					t.cur.attr.bg = uint16(attr[i])
				} else {
					t.logf("bad bgcolor %d\n", attr[i])
				}
			} else {
				t.logf("gfx attr %d unknown\n", a)
			}
		case 49:
			t.cur.attr.bg = defaultbg
		default:
			if between(a, 30, 37) {
				t.cur.attr.fg = uint16(a - 30)
			} else if between(a, 40, 47) {
				t.cur.attr.bg = uint16(a - 40)
			} else if between(a, 90, 97) {
				t.cur.attr.fg = uint16(a - 90 + 8)
			} else if between(a, 100, 107) {
				t.cur.attr.bg = uint16(a - 100 + 8)
			} else {
				t.logf("gfx attr %d unknown\n", a)
			}
		}
	}
}

func (t *Term) insertBlanks(n int) {
	src := t.cur.x
	dst := src + n
	size := t.cols - dst
	t.dirty[t.cur.y] = true

	if dst >= t.cols {
		t.clear(t.cur.x, t.cur.y, t.cols-1, t.cur.y)
	} else {
		copy(t.lines[t.cur.y][dst:dst+size], t.lines[t.cur.y][src:src+size])
		t.clear(src, t.cur.y, dst-1, t.cur.y)
	}
}

func (t *Term) insertBlankLines(n int) {
	if t.cur.y < t.top || t.cur.y > t.bottom {
		return
	}
	t.scrollDown(t.cur.y, n)
}

func (t *Term) deleteLines(n int) {
	if t.cur.y < t.top || t.cur.y > t.bottom {
		return
	}
	t.scrollUp(t.cur.y, n)
}

func (t *Term) deleteChars(n int) {
	src := t.cur.x + n
	dst := t.cur.x
	size := t.cols - src
	t.dirty[t.cur.y] = true

	if src >= t.cols {
		t.clear(t.cur.x, t.cur.y, t.cols-1, t.cur.y)
	} else {
		copy(t.lines[t.cur.y][dst:dst+size], t.lines[t.cur.y][src:src+size])
		t.clear(t.cols-n, t.cur.y, t.cols-1, t.cur.y)
	}
}
