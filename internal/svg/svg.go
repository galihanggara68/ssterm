// Package svg renders a Screen grid as an SVG Screenshot.
package svg

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
	"unicode/utf8"

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

// Render draws the Screen grid faithfully using the Theme.
func Render(e emulator.Emulator, th theme.Theme, font string) []byte {
	if font == "" {
		font = DefaultFont
	}
	w, h := e.Cols()*cellW, e.Rows()*cellH
	var b bytes.Buffer
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`+"\n", w, h, w, h)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="%s"/>`+"\n", w, h, th.Background)
	fmt.Fprintf(&b, `<g font-family="%s" font-size="%d" fill="%s" xml:space="preserve">`+"\n", escape(font), fontSize, th.Foreground)
	for row := 0; row < e.Rows(); row++ {
		line, cols := rowText(e, row)
		if line == "" {
			continue
		}
		y := row*cellH + cellH - 5
		fmt.Fprintf(&b, `<text x="0" y="%d" textLength="%d" lengthAdjust="spacing">%s</text>`+"\n", y, cols*cellW, escape(line))
	}
	b.WriteString("</g>\n</svg>\n")
	return b.Bytes()
}

// rowText returns the row's text with trailing blanks removed and the number
// of columns it spans.
func rowText(e emulator.Emulator, row int) (string, int) {
	var sb strings.Builder
	for col := 0; col < e.Cols(); {
		c := e.Cell(row, col)
		if c.Text == "" {
			sb.WriteByte(' ')
		} else {
			sb.WriteString(c.Text)
		}
		col += c.Width
	}
	line := strings.TrimRight(sb.String(), " ")
	return line, utf8.RuneCountInString(line)
}

func escape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
