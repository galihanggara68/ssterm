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

func (e *vtEmulator) Cell(row, col int) Cell { return toCell(e.term.CellAt(col, row)) }

func (e *vtEmulator) ScrollbackLen() int {
	if e.term.IsAltScreen() {
		return 0
	}
	return e.term.ScrollbackLen()
}

func (e *vtEmulator) ScrollbackCell(row, col int) Cell {
	if e.term.IsAltScreen() {
		return Cell{Width: 1}
	}
	return toCell(e.term.ScrollbackCellAt(col, row))
}

func toCell(c *uv.Cell) Cell {
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
	default:
		r, g, b, _ := c.RGBA()
		return Color{Kind: ColorRGB, R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8)}
	}
}
