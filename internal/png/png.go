// Package png renders a Screen grid as a PNG Screenshot.
package png

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	stdpng "image/png"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/x/ansi"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
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
	Radius  int
	Chrome  bool
	Title   string
	Shadow  bool
}

var (
	fontsOnce   sync.Once
	parsedReg   *opentype.Font
	parsedBold  *opentype.Font
	parsedItal  *opentype.Font
	parsedBoldI *opentype.Font

	fallbackMu     sync.Mutex
	fallbackInited bool
	fallbackPaths  []string
	fallbackFonts  []*opentype.Font
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

func primaryFont(idx int) *opentype.Font {
	initFonts()
	switch idx {
	case 1:
		return parsedBold
	case 2:
		return parsedItal
	case 3:
		return parsedBoldI
	default:
		return parsedReg
	}
}

func newFaceFromFont(f *opentype.Font) (font.Face, error) {
	return opentype.NewFace(f, &opentype.FaceOptions{
		Size:    fontSize,
		DPI:     72,
		Hinting: font.HintingFull,
	})
}

func newFace(bold, italic bool) font.Face {
	idx := 0
	if bold {
		idx |= 1
	}
	if italic {
		idx |= 2
	}
	face, err := newFaceFromFont(primaryFont(idx))
	if err != nil {
		panic(err)
	}
	return face
}

func hasGlyph(f *opentype.Font, buf *sfnt.Buffer, r rune) bool {
	gi, err := f.GlyphIndex(buf, r)
	return err == nil && gi != 0
}

func findFallbackFont(buf *sfnt.Buffer, r rune) *opentype.Font {
	fallbackMu.Lock()
	defer fallbackMu.Unlock()

	if !fallbackInited {
		fallbackPaths = discoverSystemFontPaths()
		fallbackInited = true
	}
	for _, f := range fallbackFonts {
		if hasGlyph(f, buf, r) {
			return f
		}
	}
	for len(fallbackPaths) > 0 {
		path := fallbackPaths[0]
		fallbackPaths = fallbackPaths[1:]
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		f, err := opentype.Parse(data)
		if err != nil {
			continue
		}
		fallbackFonts = append(fallbackFonts, f)
		if hasGlyph(f, buf, r) {
			return f
		}
	}
	return nil
}

func discoverSystemFontPaths() []string {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs,
			filepath.Join(home, ".local", "share", "fonts"),
			filepath.Join(home, ".fonts"),
			filepath.Join(home, "Library", "Fonts"),
		)
	}
	dirs = append(dirs,
		"/usr/local/share/fonts",
		"/usr/share/fonts",
		"/Library/Fonts",
		"/System/Library/Fonts",
		`C:\Windows\Fonts`,
	)
	var paths []string
	seen := make(map[string]bool)
	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(p))
			if (ext == ".ttf" || ext == ".otf") && !seen[p] && isCandidateFallbackFont(p) {
				seen[p] = true
				paths = append(paths, p)
			}
			return nil
		})
	}
	slices.SortStableFunc(paths, func(a, b string) int {
		return fontPriority(a) - fontPriority(b)
	})
	return paths
}

func isCandidateFallbackFont(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	for _, kw := range []string{
		"nerd", "symbol", "icon", "dejavusansmono", "notosansmono", "notomono",
		"consola", "cascadia", "jetbrains", "fira", "meslo", "hack", "ubuntu",
		"zedmono", "menlo", "sfmono", "freemono", "liberationmono", "unifont",
	} {
		if strings.Contains(base, kw) {
			return true
		}
	}
	return false
}

func fontPriority(path string) int {
	base := strings.ToLower(filepath.Base(path))
	score := 50
	switch {
	case strings.Contains(base, "nerd"):
		score = 10
	case strings.Contains(base, "symbol") || strings.Contains(base, "icon"):
		score = 20
	case strings.Contains(base, "dejavusansmono") || strings.Contains(base, "notosansmono") || strings.Contains(base, "notomono"):
		score = 30
	case strings.Contains(base, "cjk") || strings.Contains(base, "zenhei") || strings.Contains(base, "unifont"):
		score = 40
	}
	if strings.Contains(base, "regular") || strings.Contains(base, "book") {
		score -= 3
	}
	return score
}

// RenderWithOptions draws the Screen grid using the Theme and Options.
func RenderWithOptions(e emulator.Emulator, th theme.Theme, opts Options) []byte {
	chromeEnabled := opts.Chrome || opts.Title != ""
	lay := enhance.ComputeLayout(e, opts.SkipScrollback, opts.Cols, opts.Rows, opts.Padding, opts.Radius, chromeEnabled, opts.Shadow)

	img := image.NewRGBA(image.Rect(0, 0, lay.TotalW, lay.TotalH))
	defBg := parseHex(th.Background)
	defFg := parseHex(th.Foreground)

	var winTarget *image.RGBA
	if opts.Shadow {
		winTarget = image.NewRGBA(image.Rect(lay.WinX, lay.WinY, lay.WinX+lay.WinW, lay.WinY+lay.WinH))
	} else {
		winTarget = img
	}

	var chromeSpec enhance.ChromeSpec
	if chromeEnabled {
		chromeSpec = enhance.ComputeChrome(th, lay.WinX, lay.WinY, lay.WinW, opts.Title)
		drawChrome(winTarget, chromeSpec)
	}
	draw.Draw(winTarget, image.Rect(lay.BodyX, lay.BodyY, lay.BodyX+lay.BodyW, lay.BodyY+lay.BodyH), &image.Uniform{C: defBg}, image.Point{}, draw.Src)

	faces := [4]font.Face{
		newFace(false, false),
		newFace(true, false),
		newFace(false, true),
		newFace(true, true),
	}
	extraFaces := make(map[*opentype.Font]font.Face)
	defer func() {
		for _, f := range faces {
			_ = f.Close()
		}
		for _, f := range extraFaces {
			_ = f.Close()
		}
	}()
	var sfntBuf sfnt.Buffer

	faceForRune := func(idx int, r rune) font.Face {
		pFont := primaryFont(idx)
		if !hasGlyph(pFont, &sfntBuf, r) {
			if fb := findFallbackFont(&sfntBuf, r); fb != nil {
				fbFace, ok := extraFaces[fb]
				if !ok {
					if created, err := newFaceFromFont(fb); err == nil {
						fbFace = created
						extraFaces[fb] = fbFace
					}
				}
				if fbFace != nil {
					return fbFace
				}
			}
		}
		return faces[idx]
	}

	if chromeSpec.Title != "" {
		titleFg := parseHex(chromeSpec.Foreground)
		tx := chromeSpec.TitleX
		for _, r := range chromeSpec.Title {
			s := string(r)
			rw := max(ansi.StringWidth(s), 1)
			if r != ' ' {
				d := &font.Drawer{
					Dst:  winTarget,
					Src:  &image.Uniform{C: titleFg},
					Face: faceForRune(0, r),
					Dot:  fixed.P(tx, chromeSpec.TitleY),
				}
				d.DrawString(s)
			}
			tx += rw * cellW
		}
	}

	for row := 0; row < lay.TotalRows; row++ {
		y0 := lay.ContentY + row*cellH
		baseline := y0 + cellH - 5
		// Pass 1: draw all non-default cell backgrounds in the row.
		for col := 0; col < lay.Cols; {
			c := enhance.CellAt(e, lay.SB, row, col)
			cw := max(c.Width, 1)
			_, bg := resolveColors(th, c, defFg, defBg)
			if bg != defBg {
				x0 := lay.ContentX + col*cellW
				draw.Draw(winTarget, image.Rect(x0, y0, x0+cw*cellW, y0+cellH), &image.Uniform{C: bg}, image.Point{}, draw.Src)
			}
			col += cw
		}
		// Pass 2: draw glyphs and text decorations on top of row backgrounds.
		for col := 0; col < lay.Cols; {
			c := enhance.CellAt(e, lay.SB, row, col)
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
				face := faces[idx]
				for _, r := range c.Text {
					face = faceForRune(idx, r)
					break
				}
				d := &font.Drawer{
					Dst:  winTarget,
					Src:  &image.Uniform{C: fg},
					Face: face,
					Dot:  fixed.P(x0, baseline),
				}
				d.DrawString(c.Text)
			}

			if c.Attrs&emulator.AttrUnderline != 0 {
				uy := baseline + 2
				draw.Draw(winTarget, image.Rect(x0, uy, x0+cw*cellW, uy+1), &image.Uniform{C: fg}, image.Point{}, draw.Src)
			}
			if c.Attrs&emulator.AttrStrikethrough != 0 {
				sy := y0 + cellH/2
				draw.Draw(winTarget, image.Rect(x0, sy, x0+cw*cellW, sy+1), &image.Uniform{C: fg}, image.Point{}, draw.Src)
			}

			col += cw
		}
	}

	if lay.Radius > 0 {
		applyCornerMask(winTarget, lay.WinX, lay.WinY, lay.WinW, lay.WinH, lay.Radius)
	}
	if opts.Shadow {
		drawShadow(img, lay.WinX, lay.WinY, lay.WinW, lay.WinH, lay.Radius)
		draw.Draw(img, winTarget.Bounds(), winTarget, winTarget.Bounds().Min, draw.Over)
	}

	var buf bytes.Buffer
	_ = stdpng.Encode(&buf, img)
	return buf.Bytes()
}

func drawShadow(img *image.RGBA, winX, winY, winW, winH, cornerR int) {
	const blurRadius = 12
	const r2 = blurRadius * blurRadius
	sx0, sy0 := winX, winY+4
	sx1, sy1 := winX+winW, winY+winH+4
	b := img.Bounds()
	yMin := max(b.Min.Y, sy0-blurRadius)
	yMax := min(b.Max.Y, sy1+blurRadius)
	xMin := max(b.Min.X, sx0-blurRadius)
	xMax := min(b.Max.X, sx1+blurRadius)
	ix0, ix1 := sx0+cornerR, sx1-1-cornerR
	iy0, iy1 := sy0+cornerR, sy1-1-cornerR
	for y := yMin; y < yMax; y++ {
		cy := min(max(y, iy0), iy1)
		dy := float64(y - cy)
		for x := xMin; x < xMax; x++ {
			cx := min(max(x, ix0), ix1)
			dx := float64(x - cx)
			dInner := math.Hypot(dx, dy)
			d := max(0, dInner-float64(cornerR))
			if d2 := d * d; d2 < r2 {
				a := uint8(110 * (r2 - d2) / r2)
				img.SetRGBA(x, y, color.RGBA{A: a})
			}
		}
	}
}

func applyCornerMask(img *image.RGBA, winX, winY, winW, winH, r int) {
	if r <= 0 {
		return
	}
	rf := float64(r)
	type corner struct {
		x0, x1 int
		y0, y1 int
		cx, cy float64
	}
	corners := [4]corner{
		// Top-left
		{
			x0: winX, x1: winX + r,
			y0: winY, y1: winY + r,
			cx: float64(winX + r), cy: float64(winY + r),
		},
		// Top-right
		{
			x0: winX + winW - r, x1: winX + winW,
			y0: winY, y1: winY + r,
			cx: float64(winX + winW - r), cy: float64(winY + r),
		},
		// Bottom-left
		{
			x0: winX, x1: winX + r,
			y0: winY + winH - r, y1: winY + winH,
			cx: float64(winX + r), cy: float64(winY + winH - r),
		},
		// Bottom-right
		{
			x0: winX + winW - r, x1: winX + winW,
			y0: winY + winH - r, y1: winY + winH,
			cx: float64(winX + winW - r), cy: float64(winY + winH - r),
		},
	}

	for _, c := range corners {
		for y := c.y0; y < c.y1; y++ {
			py := float64(y) + 0.5
			for x := c.x0; x < c.x1; x++ {
				px := float64(x) + 0.5
				d := math.Hypot(px-c.cx, py-c.cy) - rf
				if d > 0.5 {
					img.SetRGBA(x, y, color.RGBA{})
				} else if d >= -0.5 {
					factor := 0.5 - d
					orig := img.RGBAAt(x, y)
					img.SetRGBA(x, y, color.RGBA{
						R: uint8(float64(orig.R)*factor + 0.5),
						G: uint8(float64(orig.G)*factor + 0.5),
						B: uint8(float64(orig.B)*factor + 0.5),
						A: uint8(float64(orig.A)*factor + 0.5),
					})
				}
			}
		}
	}
}

func drawChrome(img *image.RGBA, spec enhance.ChromeSpec) {
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
