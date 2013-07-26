package terminal

func isControlCode(c rune) bool {
	return c < 0x20 || c == 0177
}

func (t *Term) parse(c rune) {
	if isControlCode(c) {
		if t.handleControlCodes(c) || t.cur.attr.mode&glyphAttrGfx == 0 {
			return
		}
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

func (t *Term) parseEscCSI(c rune) {
	if t.handleControlCodes(c) {
		return
	}
	if t.csi.put(byte(c)) {
		t.handleCSI()
	}
}

func (t *Term) parseEscStrEnd(c rune) {
	if t.handleControlCodes(c) {
		return
	}
	t.state = t.parse
	if c == '\\' {
		t.handleSTR()
	}
}

func (t *Term) parseEscAltCharset(c rune) {
	if t.handleControlCodes(c) {
		return
	}
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
	t.state = t.parse
}

func (t *Term) parseEscTest(c rune) {
	if t.handleControlCodes(c) {
		return
	}
	// DEC screen alignment test
	if c == '8' {
		for y := 0; y < t.rows; y++ {
			for x := 0; x < t.cols; x++ {
				t.setChar('E', &t.cur.attr, x, y)
			}
		}
	}
	t.state = t.parse
}

func (t *Term) parseEsc(c rune) {
	if t.handleControlCodes(c) {
		return
	}
	switch c {
	case '[':
		t.state = t.parseEscCSI
	case '#':
		t.state = t.parseEscTest
	case 'P', // DCS - Device Control String
		'_', // APC - Application Program Command
		'^', // PM - Privacy Message
		']', // OSC - Operating System Command
		'k': // old title set compatibility
		t.str.reset()
		t.str.typ = c
		t.state = t.parseEscStr
	case '(': // set primary charset G0
		t.state = t.parseEscAltCharset
	case ')', // set secondary charset G1 (ignored)
		'*', // set tertiary charset G2 (ignored)
		'+': // set quaternary charset G3 (ignored)
		t.state = t.parse
	case 'D': // IND - linefeed
		if t.cur.y == t.bottom {
			// TODO: t.scrollUp(t.top, 1)
		} else {
			t.moveTo(t.cur.x, t.cur.y+1)
		}
		t.state = t.parse
	case 'E': // NEL - next line
		t.newline(true)
		t.state = t.parse
	case 'H': // HTS - horizontal tab stop
		t.tabs[t.cur.x] = true
		t.state = t.parse
	case 'M': // RI - reverse index
		if t.cur.y == t.top {
			// TODO: t.scrollDown(t.top, 1)
		} else {
			t.moveTo(t.cur.x, t.cur.y-1)
		}
		t.state = t.parse
	case 'Z': // DECID - identify terminal
		// TODO: write to our writer our id
		t.state = t.parse
	case 'c': // RIS - reset to initial state
		t.reset()
		t.state = t.parse
	case '=': // DECPAM - application keypad
		t.mode |= modeAppKeypad
		t.state = t.parse
	case '>': // DECPNM - normal keypad
		t.mode &^= modeAppKeypad
		t.state = t.parse
	case '7': // DECSC - save cursor
		t.saveCursor()
		t.state = t.parse
	case '8': // DECRC - restore cursor
		t.restoreCursor()
		t.state = t.parse
	case '\\': // ST - stop
		t.state = t.parse
	default:
		// TODO: log to stderr unknown ESC sequence
		t.state = t.parse
	}
}

func (t *Term) parseEscStr(c rune) {
	switch c {
	case '\033':
		t.state = t.parseEscStrEnd
	case '\a': // backwards compatiblity to xterm
		t.state = t.parse
		t.handleSTR()
	default:
		t.str.put(c)
	}
}

func (t *Term) handleControlCodes(c rune) bool {
	if !isControlCode(c) {
		return false
	}
	switch c {
	// HT
	case '\t':
		t.putTab(true)
	// BS
	case '\b':
		t.moveTo(t.cur.x-1, t.cur.y)
	// CR
	case '\r':
		t.moveTo(0, t.cur.y)
	// LF, VT, LF
	case '\f', '\v', '\n':
		// go to first col if mode is set
		t.newline(t.mode&modeCRLF != 0)
	// BEL
	case '\a':
		// TODO: emit sound
		// TODO: window alert if not focused
	// ESC
	case 033:
		t.csi.reset()
		t.state = t.parseEsc
	// SO, SI
	case 016, 017:
		// different charsets not supported. apps should use the correct
		// alt charset escapes, probably for line drawing
	// SUB, CAN
	case 032, 030:
		t.csi.reset()
	// ignore ENQ, NUL, XON, XOFF, DEL
	case 005, 000, 021, 023, 0177:
	default:
		return false
	}
	return true
}
