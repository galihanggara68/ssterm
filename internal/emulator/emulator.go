// Package emulator turns a stream of terminal output into a Screen grid.
//
// The rest of ssterm depends only on the Emulator interface, so the VT
// library behind it can be replaced.
package emulator

// Cell is one position of the Screen grid.
type Cell struct {
	// Text is the grapheme shown in the cell; empty for a blank cell.
	Text string
	// Width is the number of columns the cell spans.
	Width int
	// Fg and Bg are the cell colors; the zero value means the Theme default.
	Fg, Bg Color
	// Attrs is a set of Attr flags.
	Attrs Attr
}

// ColorKind says how a Color is specified.
type ColorKind uint8

const (
	// ColorDefault is the Theme's default foreground or background.
	ColorDefault ColorKind = iota
	// ColorPalette is one of the 16 ANSI colors, resolved by the Theme.
	ColorPalette
	// ColorRGB is an exact color, as emitted (256-color and truecolor).
	ColorRGB
)

// Color is a cell color.
type Color struct {
	Kind    ColorKind
	Index   uint8 // 0-15 when Kind is ColorPalette
	R, G, B uint8 // when Kind is ColorRGB
}

// Attr is a set of text attributes.
type Attr uint16

const (
	AttrBold Attr = 1 << iota
	AttrDim
	AttrItalic
	AttrUnderline
	AttrStrikethrough
	AttrReverse
	AttrBlink
)

// Emulator consumes terminal output and exposes the resulting Screen grid.
type Emulator interface {
	// Write feeds terminal output (text and escape sequences).
	Write(p []byte) (int, error)
	// Cols and Rows give the size of the Screen grid.
	Cols() int
	Rows() int
	// Cell returns the cell at the given zero-based row and column.
	Cell(row, col int) Cell
	// ScrollbackLen gives the number of Scrollback lines (lines that
	// scrolled off the top of the Screen grid), oldest first.
	ScrollbackLen() int
	// ScrollbackCell returns the cell at the given zero-based Scrollback
	// row (0 is the oldest) and column.
	ScrollbackCell(row, col int) Cell
}
