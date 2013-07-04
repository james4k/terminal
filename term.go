package terminal

const (
	tabspaces = 8 // probably a better way to do this
)

const (
	cursorDefault = 1 << iota
	cursorWrapNext
	cursorOrigin
)

const (
	modeWrap = 1 << iota
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
	top, bottom   int   // scroll limits
	mode          int32 // mode flags
	esc           int32 // escape state flags
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

func (t *Term) resize(cols, rows int) {

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

func clamp(val, min, max int) int {
	if val < min {
		return min
	} else if val > max {
		return max
	}
	return val
}
