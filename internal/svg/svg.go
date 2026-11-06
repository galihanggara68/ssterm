// Package svg renders a Screen grid as an SVG Screenshot.
package svg

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"ssterm/internal/emulator"
	"ssterm/internal/theme"
)

const (
	fontSize = 14
	cellW    = 9
	cellH    = 18
)

// DefaultFont is the font-family stack used when none is given.
const DefaultFont = "ui-monospace, SFMono-Regular, Menlo, Consolas, 'DejaVu Sans Mono', monospace"

// Options controls optional SVG rendering behavior.
type Options struct {
	Font           string
	SkipScrollback bool
}

// Render draws the Screen grid faithfully using the Theme.
func Render(e emulator.Emulator, th theme.Theme, font string) []byte {
	return RenderWithOptions(e, th, Options{Font: font})
}

// RenderWithOptions draws the Screen grid using the Theme and Options.
func RenderWithOptions(e emulator.Emulator, th theme.Theme, opts Options) []byte {
	font := opts.Font
	if font == "" {
		font = DefaultFont
	}
	sb := e.ScrollbackLen()
	if opts.SkipScrollback {
		sb = 0
	}
	totalRows := sb + e.Rows()
	w, h := e.Cols()*cellW, totalRows*cellH
	var b bytes.Buffer
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`+"\n", w, h, w, h)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="%s"/>`+"\n", w, h, th.Background)
	fmt.Fprintf(&b, `<g font-family="%s" font-size="%d" fill="%s" xml:space="preserve">`+"\n", escape(font), fontSize, th.Foreground)
	for row := 0; row < totalRows; row++ {
		runs, cols := rowRuns(e, th, row, sb)
		x := 0
		for _, r := range runs {
			if r.bg != "" {
				fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s"/>`+"\n", x*cellW, row*cellH, r.cols*cellW, cellH, r.bg)
			}
			x += r.cols
		}
		trimmed := trimRuns(runs)
		if len(trimmed) == 0 {
			continue
		}
		y := row*cellH + cellH - 5
		fmt.Fprintf(&b, `<text x="0" y="%d" textLength="%d" lengthAdjust="spacing">`, y, cols*cellW)
		for _, r := range trimmed {
			if r.attrs == "" {
				b.WriteString(escape(r.text))
			} else {
				fmt.Fprintf(&b, `<tspan%s>%s</tspan>`, r.attrs, escape(r.text))
			}
		}
		b.WriteString("</text>\n")
	}
	b.WriteString("</g>\n</svg>\n")
	return b.Bytes()
}

// run is a span of cells sharing one style.
type run struct {
	text string
	cols int
	bg   string // CSS color, empty for the Theme background
	// attrs is the SVG attribute string for the span, empty when the span
	// uses the default style.
	attrs string
}

// rowRuns splits a row (Scrollback rows first, then Screen rows) into style
// runs and returns them with the number of display columns the row's text
// spans once trailing blanks are removed.
func rowRuns(e emulator.Emulator, th theme.Theme, row, scrollback int) ([]run, int) {
	var runs []run
	for col := 0; col < e.Cols(); {
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
		if n := len(runs); n > 0 && runs[n-1].bg == bg && runs[n-1].attrs == attrs {
			runs[n-1].text += text
			runs[n-1].cols += c.Width
		} else {
			runs = append(runs, run{text: text, cols: c.Width, bg: bg, attrs: attrs})
		}
		col += c.Width
	}
	trimmed := trimRuns(runs)
	cols := 0
	for _, r := range trimmed {
		cols += ansi.StringWidth(r.text)
	}
	return runs, cols
}

// trimRuns removes trailing blanks (and runs left empty) from the row text.
func trimRuns(runs []run) []run {
	runs = append([]run(nil), runs...)
	for len(runs) > 0 {
		last := &runs[len(runs)-1]
		last.text = strings.TrimRight(last.text, " ")
		if last.text != "" {
			break
		}
		runs = runs[:len(runs)-1]
	}
	return runs
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
