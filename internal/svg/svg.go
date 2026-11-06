// Package svg renders a Screen grid as an SVG Screenshot.
package svg

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"

	"ssterm/internal/emulator"
	"ssterm/internal/enhance"
	"ssterm/internal/theme"
)

const (
	fontSize = 14
	cellW    = enhance.CellW
	cellH    = enhance.CellH
)

// DefaultFont is the font-family stack used when none is given.
const DefaultFont = "ui-monospace, SFMono-Regular, Menlo, Consolas, 'DejaVu Sans Mono', monospace"

// Options controls optional SVG rendering behavior.
type Options struct {
	Font           string
	SkipScrollback bool
	// Cols and Rows override the rendered grid width and height when > 0.
	Cols, Rows int
	// Padding adds pixels of Theme background around all four sides of the Screen grid.
	Padding int
	Chrome  bool
	Shadow  bool
}

// RenderWithOptions draws the Screen grid using the Theme and Options.
func RenderWithOptions(e emulator.Emulator, th theme.Theme, opts Options) []byte {
	font := opts.Font
	if font == "" {
		font = DefaultFont
	}
	lay := enhance.ComputeLayout(e, opts.SkipScrollback, opts.Cols, opts.Rows, opts.Padding, opts.Chrome, opts.Shadow)
	var b bytes.Buffer
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`+"\n", lay.TotalW, lay.TotalH, lay.TotalW, lay.TotalH)
	if !opts.Chrome && !opts.Shadow {
		fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="%s"/>`+"\n", lay.BodyW, lay.BodyH, th.Background)
	} else {
		if opts.Shadow {
			b.WriteString(`<defs><filter id="shadow" x="-20%" y="-20%" width="140%" height="140%"><feDropShadow dx="0" dy="4" stdDeviation="6" flood-color="#000000" flood-opacity="0.45"/></filter></defs>` + "\n")
			fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" fill="#000000" opacity="0.35" filter="url(#shadow)"/>`+"\n", lay.WinX, lay.WinY+4, lay.WinW, lay.WinH)
		}
		if opts.Chrome {
			writeChrome(&b, th, lay.WinX, lay.WinY, lay.WinW)
		}
		fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s"/>`+"\n", lay.BodyX, lay.BodyY, lay.BodyW, lay.BodyH, th.Background)
	}
	fmt.Fprintf(&b, `<g font-family="%s" font-size="%d" fill="%s" xml:space="preserve">`+"\n", escape(font), fontSize, th.Foreground)
	for row := 0; row < lay.TotalRows; row++ {
		runs, segments := rowRuns(e, th, row, lay.SB, lay.Cols)
		x := 0
		for _, r := range runs {
			if r.bg != "" {
				fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s"/>`+"\n", lay.ContentX+x*cellW, lay.ContentY+row*cellH, r.cols*cellW, cellH, r.bg)
			}
			x += r.cols
		}
		if len(segments) == 0 {
			continue
		}
		y := lay.ContentY + row*cellH + cellH - 5
		for _, seg := range segments {
			fmt.Fprintf(&b, `<text x="%d" y="%d" textLength="%d" lengthAdjust="spacing">`, lay.ContentX+seg.col*cellW, y, seg.cols*cellW)
			curURL := ""
			for _, r := range seg.runs {
				if r.url != curURL {
					if curURL != "" {
						b.WriteString("</a>")
					}
					if r.url != "" {
						fmt.Fprintf(&b, `<a href="%s">`, escape(r.url))
					}
					curURL = r.url
				}
				if r.attrs == "" {
					b.WriteString(escape(r.text))
				} else {
					fmt.Fprintf(&b, `<tspan%s>%s</tspan>`, r.attrs, escape(r.text))
				}
			}
			if curURL != "" {
				b.WriteString("</a>")
			}
			b.WriteString("</text>\n")
		}
	}
	b.WriteString("</g>\n</svg>\n")
	return b.Bytes()
}

func writeChrome(b *bytes.Buffer, th theme.Theme, winX, winY, winW int) {
	spec := enhance.ComputeChrome(th, winX, winY, winW)
	fmt.Fprintf(b, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s"/>`+"\n", spec.X, spec.Y, spec.W, spec.H, spec.Background)
	for _, btn := range spec.Buttons {
		if btn.Circle {
			fmt.Fprintf(b, `<circle cx="%d" cy="%d" r="%d" fill="%s"/>`+"\n", btn.CX, btn.CY, btn.R, btn.Color)
		} else {
			fmt.Fprintf(b, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s"/>`+"\n", btn.X, btn.Y, btn.W, btn.H, btn.Color)
		}
	}
}

// run is a span of cells sharing one style, hyperlink and cell width.
type run struct {
	text  string
	cols  int
	wide  bool
	bg    string // CSS color, empty for the Theme background
	attrs string // SVG attribute string, empty for default style
	url   string // OSC 8 hyperlink URL, empty when unlinked
	deco  bool   // whether underline or strikethrough is set
}

// textSegment is a contiguous group of runs with the same cell-width class
// (either all 1-column cells or all 2-column cells), anchored at its starting
// column so wide glyphs never skew narrow columns in the same row.
type textSegment struct {
	col  int
	cols int
	runs []run
}

// rowRuns splits a row (Scrollback rows first, then Screen rows) into background
// runs and column-anchored text segments with trailing unstyled blanks removed.
func rowRuns(e emulator.Emulator, th theme.Theme, row, scrollback, maxCols int) ([]run, []textSegment) {
	var bgRuns []run
	var cellRuns []run
	for col := 0; col < maxCols; {
		var c emulator.Cell
		if row < scrollback {
			c = e.ScrollbackCell(row, col)
		} else {
			c = e.Cell(row-scrollback, col)
		}
		text := c.Text
		if text == "" {
			text = " "
		}
		bg, attrs := style(th, c)
		deco := c.Attrs&(emulator.AttrUnderline|emulator.AttrStrikethrough) != 0
		wide := c.Width > 1
		if n := len(bgRuns); n > 0 && bgRuns[n-1].bg == bg {
			bgRuns[n-1].cols += c.Width
		} else {
			bgRuns = append(bgRuns, run{cols: c.Width, bg: bg})
		}
		cellRuns = append(cellRuns, run{text: text, cols: c.Width, wide: wide, attrs: attrs, url: c.URL, deco: deco})
		col += c.Width
	}
	// Trim trailing unstyled blank cells (preserving spaces with underline/strikethrough or a link).
	for len(cellRuns) > 0 {
		last := cellRuns[len(cellRuns)-1]
		if last.text == " " && !last.deco && last.url == "" {
			cellRuns = cellRuns[:len(cellRuns)-1]
			continue
		}
		break
	}
	if len(cellRuns) == 0 {
		return bgRuns, nil
	}
	var segments []textSegment
	col := 0
	for _, cr := range cellRuns {
		if n := len(segments); n > 0 && segments[n-1].runs[0].wide == cr.wide {
			seg := &segments[n-1]
			seg.cols += cr.cols
			if m := len(seg.runs); m > 0 && seg.runs[m-1].attrs == cr.attrs && seg.runs[m-1].url == cr.url {
				seg.runs[m-1].text += cr.text
				seg.runs[m-1].cols += cr.cols
			} else {
				seg.runs = append(seg.runs, cr)
			}
		} else {
			segments = append(segments, textSegment{
				col:  col,
				cols: cr.cols,
				runs: []run{cr},
			})
		}
		col += cr.cols
	}
	return bgRuns, segments
}

// style resolves a cell's background color and SVG text attributes.
func style(th theme.Theme, c emulator.Cell) (bg, attrs string) {
	bold := c.Attrs&emulator.AttrBold != 0
	fg := resolve(th, c.Fg, bold && th.BoldAsBright)
	bgc := resolve(th, c.Bg, false)
	if c.Attrs&emulator.AttrReverse != 0 {
		if fg == "" {
			fg = th.Foreground
		}
		if bgc == "" {
			bgc = th.Background
		}
		fg, bgc = bgc, fg
	}
	var sb strings.Builder
	if fg != "" {
		fmt.Fprintf(&sb, ` fill="%s"`, fg)
	}
	if bold {
		sb.WriteString(` font-weight="bold"`)
	}
	if c.Attrs&emulator.AttrItalic != 0 {
		sb.WriteString(` font-style="italic"`)
	}
	var deco []string
	if c.Attrs&emulator.AttrUnderline != 0 {
		deco = append(deco, "underline")
	}
	if c.Attrs&emulator.AttrStrikethrough != 0 {
		deco = append(deco, "line-through")
	}
	if len(deco) > 0 {
		fmt.Fprintf(&sb, ` text-decoration="%s"`, strings.Join(deco, " "))
	}
	if c.Attrs&emulator.AttrDim != 0 {
		sb.WriteString(` opacity="0.5"`)
	}
	// Blink is rendered static: no attribute.
	return bgc, sb.String()
}

// resolve returns a CSS color, or "" for the Theme default.
func resolve(th theme.Theme, c emulator.Color, bright bool) string {
	switch c.Kind {
	case emulator.ColorPalette:
		i := int(c.Index)
		if bright && i < 8 {
			i += 8
		}
		return th.ANSI[i]
	case emulator.ColorRGB:
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return ""
}

func escape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
