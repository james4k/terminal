package terminal

// TODO: once lazy arg parsing is done for CSI, we can probably just use
// csiEscape for these sequences as well which would simplify things and cut a
// bit of memory usage on the buffers.

// STR sequences are similar to CSI sequences, but have string arguments (and
// as far as I can tell, don't really have a name; STR is the name I took from
// suckless which I imagine comes from rxvt or xterm).
type strEscape struct {
	typ  rune
	buf  []rune
	args [][]rune
}

func (s *strEscape) reset() {
	s.typ = 0
	s.buf = nil
	s.args = nil
}

func (s *strEscape) put(c rune) {
	// TODO: improve allocs with an array backed slice; bench first
	if len(s.buf) < 256 {
		s.buf = append(s.buf, c)
	}
	// Going by st, it is better to remain silent when the STR sequence is not
	// ended so that it is apparent to users something is wrong. The length sanity
	// check ensures we don't absorb the entire stream into memory.
	// TODO: see what rxvt or xterm does
}

func (s *strEscape) parse() {
}

func (t *Term) handleSTR() {
	s := &t.str
	s.parse()

	switch s.typ {
	case ']': // OSC - operating system command
		switch s.arg(0, 0) {
		case 0, 1, 2:
			title := s.argString(1, "")
			if title != "" {
				// TODO: setTitle(title)
			}
		case 4: // color set
			if len(s.args) < 3 {
				break
			}
			// setcolorname(s.arg(1, 0), s.argString(2, ""))
		case 104: // color reset
			// TODO: complain about invalid color, redraw, etc.
			// setcolorname(s.arg(1, 0), nil)
		default:
			// TODO: stderr log
			// TODO: s.dump()
		}
	case 'k': // old title set compatibility
		// TODO: setTitle(s.argString(0, ""))
	default:
		// TODO: Ignore these codes instead of complain?
		// 'P': // DSC - device control string
		// '_': // APC - application program command
		// '^': // PM - privacy message

		// TODO stderr log
		// t.str.dump()
	}
}
