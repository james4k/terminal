package terminal

const (
	Black Color = iota
	Red
	Green
	Yellow
	Blue
	Magenta
	Cyan
	LightGrey
	DarkGrey
	LightRed
	LightGreen
	LightYellow
	LightBlue
	LightMagenta
	LightCyan
	White

	// Default colors are potentially distinct to allow for special behavior.
	// For example, a transparent background. Otherwise, the simple case is to
	// map default colors to another color.
	DefaultFG = 0xff80 + iota
	DefaultBG
)

type Color uint16

func (c Color) ANSI() bool {
	return (c < 16)
}
