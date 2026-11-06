// Package png renders a Screen grid as a PNG Screenshot.
package png

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	stdpng "image/png"
	"strconv"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"ssterm/internal/emulator"
	"ssterm/internal/theme"
)

const (
	fontSize = 14
	cellW    = 9
	cellH    = 18
)

// Options controls optional PNG rendering behavior.
type Options struct {
	SkipScrollback bool
	// Cols and Rows override the rendered grid width and height when > 0.
	Cols, Rows int
	// Padding adds pixels of Theme background around all four sides of the Screen grid.
	Padding int
}

var (
	fontsOnce   sync.Once
	parsedReg   *opentype.Font
	parsedBold  *opentype.Font
	parsedItal  *opentype.Font
	parsedBoldI *opentype.Font
)

func initFonts() {
	fontsOnce.Do(func() {
		var err error
		if parsedReg, err = opentype.Parse(gomono.TTF); err != nil {
			panic(err)
		}
		if parsedBold, err = opentype.Parse(gomonobold.TTF); err != nil {
			panic(err)
		}
		if parsedItal, err = opentype.Parse(gomonoitalic.TTF); err != nil {
			panic(err)
		}
		if parsedBoldI, err = opentype.Parse(gomonobolditalic.TTF); err != nil {
			panic(err)
		}
	})
}

func newFace(bold, italic bool) font.Face {
	initFonts()
	f := parsedReg
	switch {
	case bold && italic:
		f = parsedBoldI
	case bold:
		f = parsedBold
	case italic:
		f = parsedItal
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    fontSize,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		panic(err)
	}
	return face
}

// RenderWithOptions draws the Screen grid using the Theme and Options.
func RenderWithOptions(e emulator.Emulator, th theme.Theme, opts Options) []byte {
	sb := e.ScrollbackLen()
	if opts.SkipScrollback {
		sb = 0
	}
	totalRows := sb + e.Rows()
	if opts.Rows > 0 && opts.Rows < totalRows {
		totalRows = opts.Rows
	}
	cols := e.Cols()
	if opts.Cols > 0 && opts.Cols < cols {
		cols = opts.Cols
	}
	p := max(opts.Padding, 0)
	w, h := cols*cellW+2*p, totalRows*cellH+2*p

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	defBg := parseHex(th.Background)
	defFg := parseHex(th.Foreground)
	draw.Draw(img, img.Bounds(), &image.Uniform{C: defBg}, image.Point{}, draw.Src)

	faces := [4]font.Face{
		newFace(false, false),
		newFace(true, false),
		newFace(false, true),
		newFace(true, true),
	}
	defer func() {
		for _, f := range faces {
			_ = f.Close()
		}
	}()

	for row := 0; row < totalRows; row++ {
		y0 := p + row*cellH
		baseline := y0 + cellH - 5
		// Pass 1: draw all non-default cell backgrounds in the row.
		for col := 0; col < cols; {
			c := cellAt(e, row, col, sb)
			cw := max(c.Width, 1)
			_, bg := resolveColors(th, c, defFg, defBg)
			if bg != defBg {
				x0 := p + col*cellW
				draw.Draw(img, image.Rect(x0, y0, x0+cw*cellW, y0+cellH), &image.Uniform{C: bg}, image.Point{}, draw.Src)
			}
			col += cw
		}
		// Pass 2: draw glyphs and text decorations on top of row backgrounds.
		for col := 0; col < cols; {
			c := cellAt(e, row, col, sb)
			cw := max(c.Width, 1)
			x0 := p + col*cellW
			fg, _ := resolveColors(th, c, defFg, defBg)

			if c.Text != "" && c.Text != " " {
				idx := 0
				if c.Attrs&emulator.AttrBold != 0 {
					idx |= 1
				}
				if c.Attrs&emulator.AttrItalic != 0 {
					idx |= 2
				}
				d := &font.Drawer{
					Dst:  img,
					Src:  &image.Uniform{C: fg},
					Face: faces[idx],
					Dot:  fixed.P(x0, baseline),
				}
				d.DrawString(c.Text)
			}

			if c.Attrs&emulator.AttrUnderline != 0 {
				uy := baseline + 2
				draw.Draw(img, image.Rect(x0, uy, x0+cw*cellW, uy+1), &image.Uniform{C: fg}, image.Point{}, draw.Src)
			}
			if c.Attrs&emulator.AttrStrikethrough != 0 {
				sy := y0 + cellH/2
				draw.Draw(img, image.Rect(x0, sy, x0+cw*cellW, sy+1), &image.Uniform{C: fg}, image.Point{}, draw.Src)
			}

			col += cw
		}
	}

	var buf bytes.Buffer
	_ = stdpng.Encode(&buf, img)
	return buf.Bytes()
}

func cellAt(e emulator.Emulator, row, col, sb int) emulator.Cell {
	if row < sb {
		return e.ScrollbackCell(row, col)
	}
	return e.Cell(row-sb, col)
}

func resolveColors(th theme.Theme, c emulator.Cell, defFg, defBg color.RGBA) (fg, bg color.RGBA) {
	bold := c.Attrs&emulator.AttrBold != 0
	var ok bool
	fg, ok = resolve(th, c.Fg, bold && th.BoldAsBright)
	if !ok {
		fg = defFg
	}
	bg, ok = resolve(th, c.Bg, false)
	if !ok {
		bg = defBg
	}
	if c.Attrs&emulator.AttrReverse != 0 {
		fg, bg = bg, fg
	}
	if c.Attrs&emulator.AttrDim != 0 {
		fg = color.RGBA{
			R: uint8((uint16(fg.R) + uint16(bg.R)) / 2),
			G: uint8((uint16(fg.G) + uint16(bg.G)) / 2),
			B: uint8((uint16(fg.B) + uint16(bg.B)) / 2),
			A: 0xff,
		}
	}
	return fg, bg
}

func resolve(th theme.Theme, c emulator.Color, bright bool) (color.RGBA, bool) {
	switch c.Kind {
	case emulator.ColorPalette:
		i := int(c.Index)
		if bright && i < 8 {
			i += 8
		}
		return parseHex(th.ANSI[i]), true
	case emulator.ColorRGB:
		return color.RGBA{R: c.R, G: c.G, B: c.B, A: 0xff}, true
	}
	return color.RGBA{}, false
}

func parseHex(s string) color.RGBA {
	if len(s) == 7 && s[0] == '#' {
		v, err := strconv.ParseUint(s[1:], 16, 32)
		if err == nil {
			return color.RGBA{
				R: uint8(v >> 16),
				G: uint8(v >> 8),
				B: uint8(v),
				A: 0xff,
			}
		}
	}
	return color.RGBA{A: 0xff}
}
