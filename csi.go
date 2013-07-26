package terminal

import (
	"strconv"
	"strings"
)

// CSI (Control Sequence Introducer)
// ESC+[
type csiEscape struct {
	buf  []byte
	args []int
	mode byte
	priv bool
}

func (c *csiEscape) reset() {
	c.buf = c.buf[:0]
	c.args = c.args[:0]
}

func (c *csiEscape) put(b byte) bool {
	c.buf = append(c.buf, b)
	if b >= 0x40 && b <= 0x7E || len(c.buf) >= 256 {
		c.parse()
		return true
	}
	return false
}

// TODO: make parsing lazy via arg() method? many codes don't need the string
// conversion and integer parsing
func (c *csiEscape) parse() {
	s := string(c.buf)
	c.args = c.args[:0]
	if s[0] == '?' {
		c.priv = true
		s = s[1:]
	}
	c.mode = s[len(s)-1]
	s = s[:len(s)-1]
	ss := strings.Split(s, ";")
	for _, p := range ss {
		i, err := strconv.Atoi(p)
		if err != nil {
			// TODO: log to stderr
			break
		}
		c.args = append(c.args, i)
	}
}

func (c *csiEscape) arg(i, def int) int {
	if i >= len(c.args) || i < 0 {
		return def
	}
	return c.args[i]
}

func (t *Term) handleCSI() {
	c := &t.csi
	switch c.mode {
	default:
		goto unknown
	case '@': // ICH - insert <n> blank char
		// TODO: t.insertBlank(c.arg(0, 1))
	case 'A': // CUU - cursor <n> up
		// TODO: t.moveTo(t.cur.x, t.cur.y - c.arg(0, 1))
	case 'B', 'e': // CUD, VPR - cursor <n> down
		// TODO: t.moveTo(t.cur.x, t.cur.y + c.arg(0, 1))
	case 'c': // DA - device attributes
		if c.arg(0, 0) == 0 {
			// TODO: write vt102 id
		}
	case 'C', 'a': // CUF, HPR - cursor <n> forward
		// TODO: t.moveTo(t.cur.x + c.arg(0, 1), t.cur.y)
	case 'D': // CUB - cursor <n> backward
		// TODO: t.moveTo(t.cur.x - c.arg(0, 1), t.cur.y)
	case 'E': // CNL - cursor <n> down and first col
		// TODO: t.moveTo(0, t.cur.y + c.arg(0, 1))
	case 'F': // CPL - cursor <n> up and first col
		// TODO: t.moveTo(0, t.cur.y - c.arg(0, 1))
	case 'g': // TBC - tabulation clear
		switch c.arg(0, 0) {
		// clear current tab stop
		case 0:
			// TODO: t.tabs[t.cur.x] = false
		// clear all tabs
		case 3:
			/*
				// TODO:
				for i := range t.tabs {
					t.tabs = false
				}
			*/
		default:
			goto unknown
		}
	case 'G', '`': // CHA, HPA - Move to <col>
		// TODO: t.moveTo(c.arg(0, 1) - 1, t.cur.y)
	case 'H', 'f': // CUP, HVP - move to <row> <col>
		// TODO: t.moveAbsTo(c.arg(1, 1) - 1, c.arg(0, 1) - 1)
	case 'I': // CHT - cursor forward tabulation <n> tab stops
		n := c.arg(0, 1)
		for i := 0; i < n; i++ {
			// TODO: t.putTab(1)
		}
	case 'J': // ED - clear screen
		// TODO: sel.ob.x = -1
		switch c.arg(0, 0) {
		case 0: // below
			// TODO:
			/*
				t.clear(t.cur.x, t.cur.y, t.cols-1, t.cur.y)
				if t.cur.y < t.rows - 1 {
					t.clear(0, t.cur.y+1, t.cols-1, t.rows-1)
				}
			*/
		case 1: // above
			// TODO:
			/*
				if t.cur.y > 1 {
					t.clear(0, 0, t.cols-1, t.cur.y-1)
				}
				t.clear(0, t.cur.y, t.cur.x, t.cur.y)
			*/
		case 2: // all
			// TODO: t.clear(0, 0, t.cols-1, t.rows-1)
		default:
			goto unknown
		}
	case 'K': // EL - clear line
		switch c.arg(0, 0) {
		case 0: // right
			// TODO: t.clear(t.cur.x, t.cur.y, t.cols-1, t.cur.y)
		case 1: // left
			// TODO: t.clear(0, t.cur.y, t.cur.x, t.cur.y)
		case 2: // all
			// TODO: t.clear(0, t.cur.y, t.cols-1, t.cur.y)
		}
	case 'S': // SU - scroll <n> lines up
		// TODO: t.scrollUp(t.top, c.arg(0, 1))
	case 'T': // SD - scroll <n> lines down
		// TODO: t.scrollDown(t.top, c.arg(0, 1))
	case 'L': // IL - insert <n> blank lines
		// TODO: t.insertBlankLines(c.arg(0, 1))
	case 'l': // RM - reset mode
		// TODO: tsetmode(c.priv, 0, c.args)
	case 'M': // DL - delete <n> lines
		// TODO: t.deleteLines(c.arg(0, 1))
	case 'X': // ECH - erase <n> chars
		// TODO: t.clear(t.cur.x, t.cur.y, t.cur.x+c.arg(0, 1)-1, t.cur.y)
	case 'P': // DCH - delete <n> chars
		// TODO: t.deleteChars(c.arg(0, 1))
	case 'Z': // CBT - cursor backward tabulation <n> tab stops
		// TODO:
		/*
			n := c.arg(0, 1)
			for i := 0; i < n; n++ {
				t.putTab(false)
			}
		*/
	case 'd': // VPA - move to <row>
		// TODO: t.moveAbsTo(t.cur.x, c.arg(0, 1)-1)
	case 'h': // SM - set terminal mode
		// TODO: tsetmode(c.priv, 1, c.args)
	case 'm': // SGR - terminal attribute (color)
		// TODO: tsetattr(c.args)
	case 'r': // DECSTBM - set scrolling region
		// TODO:
		/*
			if c.priv {
				goto unknown
			} else {
				t.setScroll(c.arg(0, 1)-1, c.arg(1, t.rows)-1)
				t.moveAbsTo(0, 0)
			}
		*/
	case 's': // DECSC - save cursor position (ANSI.SYS)
		// TODO: t.saveCursor()
	case 'u': // DECRC - restore cursor position (ANSI.SYS)
		// TODO: t.loadCursor()
	}
	return
unknown: // TODO: get rid of this goto
	// TODO: log to stderr
	// TODO: c.dump()
}
