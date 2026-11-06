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
	"ssterm/internal/enhance"
	"ssterm/internal/theme"
)

const (
	fontSize = 14
	cellW    = enhance.CellW
	cellH    = enhance.CellH
)

// Options controls optional PNG rendering behavior.
type Options struct {
	SkipScrollback bool
	// Cols and Rows override the rendered grid width and height when > 0.
	Cols, Rows int
	// Padding adds pixels of Theme background around all four sides of the Screen grid.
	Padding int
	Chrome  bool
	Shadow  bool
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
	lay := enhance.ComputeLayout(e, opts.SkipScrollback, opts.Cols, opts.Rows, opts.Padding, opts.Chrome, opts.Shadow)

	img := image.NewRGBA(image.Rect(0, 0, lay.TotalW, lay.TotalH))
	defBg := parseHex(th.Background)
	defFg := parseHex(th.Foreground)
	if opts.Shadow {
		drawShadow(img, lay.WinX, lay.WinY, lay.WinW, lay.WinH)
	}
	if opts.Chrome {
		drawChrome(img, th, lay.WinX, lay.WinY, lay.WinW)
	}
	draw.Draw(img, image.Rect(lay.BodyX, lay.BodyY, lay.BodyX+lay.BodyW, lay.BodyY+lay.BodyH), &image.Uniform{C: defBg}, image.Point{}, draw.Src)

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

	for row := 0; row < lay.TotalRows; row++ {
		y0 := lay.ContentY + row*cellH
		baseline := y0 + cellH - 5
		// Pass 1: draw all non-default cell backgrounds in the row.
		for col := 0; col < lay.Cols; {
			c := cellAt(e, row, col, lay.SB)
			cw := max(c.Width, 1)
			_, bg := resolveColors(th, c, defFg, defBg)
			if bg != defBg {
				x0 := lay.ContentX + col*cellW
				draw.Draw(img, image.Rect(x0, y0, x0+cw*cellW, y0+cellH), &image.Uniform{C: bg}, image.Point{}, draw.Src)
			}
			col += cw
		}
		// Pass 2: draw glyphs and text decorations on top of row backgrounds.
		for col := 0; col < lay.Cols; {
			c := cellAt(e, row, col, lay.SB)
			cw := max(c.Width, 1)
			x0 := lay.ContentX + col*cellW
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

func drawShadow(img *image.RGBA, winX, winY, winW, winH int) {
	const radius = 12
	const r2 = radius * radius
	sx0, sy0 := winX, winY+4
	sx1, sy1 := winX+winW, winY+winH+4
	b := img.Bounds()
	yMin := max(b.Min.Y, sy0-radius)
	yMax := min(b.Max.Y, sy1+radius)
	xMin := max(b.Min.X, sx0-radius)
	xMax := min(b.Max.X, sx1+radius)
	for y := yMin; y < yMax; y++ {
		dy := max(0, max(sy0-y, y-(sy1-1)))
		for x := xMin; x < xMax; x++ {
			dx := max(0, max(sx0-x, x-(sx1-1)))
			if d2 := dx*dx + dy*dy; d2 < r2 {
				a := uint8(110 * (r2 - d2) / r2)
				img.SetRGBA(x, y, color.RGBA{A: a})
			}
		}
	}
}

func drawChrome(img *image.RGBA, th theme.Theme, winX, winY, winW int) {
	spec := enhance.ComputeChrome(th, winX, winY, winW)
	draw.Draw(img, image.Rect(spec.X, spec.Y, spec.X+spec.W, spec.Y+spec.H), &image.Uniform{C: parseHex(spec.Background)}, image.Point{}, draw.Src)
	for _, btn := range spec.Buttons {
		col := parseHex(btn.Color)
		if btn.Circle {
			fillCircle(img, btn.CX, btn.CY, btn.R, col)
		} else {
			draw.Draw(img, image.Rect(btn.X, btn.Y, btn.X+btn.W, btn.Y+btn.H), &image.Uniform{C: col}, image.Point{}, draw.Src)
		}
	}
}

func fillCircle(img *image.RGBA, cx, cy, r int, col color.RGBA) {
	r2 := r * r
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy*dy <= r2 {
				img.SetRGBA(cx+dx, cy+dy, col)
			}
		}
	}
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
