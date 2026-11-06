package emulator

import (
	"image/color"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
)

type vtEmulator struct {
	term       *vt.SafeEmulator
	cols, rows int
}

// New returns an Emulator with a Screen grid of the given size, backed by
// charmbracelet/x/vt.
func New(cols, rows int) Emulator {
	return &vtEmulator{term: vt.NewSafeEmulator(cols, rows), cols: cols, rows: rows}
}

func (e *vtEmulator) Write(p []byte) (int, error) { return e.term.Write(p) }
func (e *vtEmulator) Cols() int                   { return e.cols }
func (e *vtEmulator) Rows() int                   { return e.rows }

func (e *vtEmulator) Cell(row, col int) Cell {
	c := e.term.CellAt(col, row)
	if c == nil {
		return Cell{Width: 1}
	}
	w := c.Width
	if w == 0 {
		w = 1
	}
	return Cell{
		Text:  c.Content,
		Width: w,
		Fg:    convertColor(c.Style.Fg),
		Bg:    convertColor(c.Style.Bg),
		Attrs: convertAttrs(c.Style),
	}
}

func convertAttrs(s uv.Style) Attr {
	var a Attr
	for _, m := range []struct {
		from uint8
		to   Attr
	}{
		{uv.AttrBold, AttrBold},
		{uv.AttrFaint, AttrDim},
		{uv.AttrItalic, AttrItalic},
		{uv.AttrStrikethrough, AttrStrikethrough},
		{uv.AttrReverse, AttrReverse},
		{uv.AttrBlink | uv.AttrRapidBlink, AttrBlink},
	} {
		if s.Attrs&m.from != 0 {
			a |= m.to
		}
	}
	if s.Underline != uv.UnderlineStyleNone {
		a |= AttrUnderline
	}
	return a
}

func convertColor(c color.Color) Color {
	switch c := c.(type) {
	case nil:
		return Color{}
	case ansi.BasicColor:
		return Color{Kind: ColorPalette, Index: uint8(c)}
	case ansi.IndexedColor:
		if c < 16 {
			return Color{Kind: ColorPalette, Index: uint8(c)}
		}
		r, g, b := xterm256(uint8(c))
		return Color{Kind: ColorRGB, R: r, G: g, B: b}
	default:
		r, g, b, _ := c.RGBA()
		return Color{Kind: ColorRGB, R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8)}
	}
}

// xterm256 returns the standard xterm RGB value of palette index 16-255.
func xterm256(i uint8) (r, g, b uint8) {
	if i >= 232 {
		v := 8 + 10*(i-232)
		return v, v, v
	}
	i -= 16
	level := func(n uint8) uint8 {
		if n == 0 {
			return 0
		}
		return 55 + 40*n
	}
	return level(i / 36), level(i / 6 % 6), level(i % 6)
}
