// Package cli implements the ssterm command line.
package cli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"ssterm/internal/emulator"
	"ssterm/internal/svg"
	"ssterm/internal/theme"
)

// Run executes ssterm with the given arguments and returns the exit code.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ssterm", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: cmd | ssterm [flags]")
		fs.VisitAll(func(f *flag.Flag) {
			dash := "--"
			if len(f.Name) == 1 {
				dash = "-"
			}
			fmt.Fprintf(stderr, "  %s%s\n    \t%s\n", dash, f.Name, f.Usage)
		})
	}
	output := fs.String("o", "", "write the Screenshot to this file instead of stdout")
	themeName := fs.String("theme", theme.Default().Name, "Theme name")
	cols := fs.Int("cols", 0, "screen width in columns (default: fit content)")
	rows := fs.Int("rows", 0, "screen height in rows (default: fit content)")
	font := fs.String("font", "", "SVG font-family stack")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	var colsSet, rowsSet bool
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "cols":
			colsSet = true
		case "rows":
			rowsSet = true
		}
	})
	if colsSet && *cols <= 0 {
		fmt.Fprintln(stderr, "ssterm: --cols must be greater than 0")
		return 2
	}
	if rowsSet && *rows <= 0 {
		fmt.Fprintln(stderr, "ssterm: --rows must be greater than 0")
		return 2
	}

	th, ok := theme.Lookup(*themeName)
	if !ok {
		fmt.Fprintf(stderr, "ssterm: unknown theme %q (valid themes: %s)\n", *themeName, strings.Join(theme.Names(), ", "))
		return 2
	}

	data, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "ssterm: reading stdin: %v\n", err)
		return 1
	}
	if len(data) == 0 {
		fmt.Fprintln(stderr, "ssterm: empty stdin")
		return 1
	}
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	data = bytes.TrimSuffix(data, []byte("\n"))
	data = bytes.ReplaceAll(data, []byte("\n"), []byte("\r\n"))

	w, h := fitEstimate(data)
	if colsSet {
		w = *cols
	}
	if rowsSet {
		h = *rows
	}
	em := emulator.New(w, h)
	if _, err := em.Write(data); err != nil {
		fmt.Fprintf(stderr, "ssterm: %v\n", err)
		return 1
	}

	renderCols, renderRows := usedBounds(em, rowsSet)
	if colsSet {
		renderCols = *cols
	}
	if rowsSet {
		renderRows = *rows
	}

	img := svg.RenderWithOptions(em, th, svg.Options{
		Font:           *font,
		SkipScrollback: rowsSet,
		Cols:           renderCols,
		Rows:           renderRows,
	})
	if *output != "" {
		if err := os.WriteFile(*output, img, 0o644); err != nil {
			fmt.Fprintf(stderr, "ssterm: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := stdout.Write(img); err != nil {
		fmt.Fprintf(stderr, "ssterm: %v\n", err)
		return 1
	}
	return 0
}

// fitEstimate gives an initial upper-bound Screen grid size before emulation so
// lines do not wrap unless --cols is explicitly set, and cursor addressing on
// the alternate screen has room to land.
func fitEstimate(data []byte) (cols, rows int) {
	lines := strings.Split(string(data), "\r\n")
	for _, l := range lines {
		for seg := range strings.SplitSeq(l, "\r") {
			if n := ansi.StringWidth(seg); n > cols {
				cols = n
			}
		}
	}
	rows = len(lines)
	if bytes.Contains(data, []byte("\x1b[")) && rows < 24 {
		rows = 24
	}
	return max(cols, 1), max(rows, 1)
}

// usedBounds inspects the emulated Screen grid (and Scrollback when included)
// and returns the tightest (cols, rows) bounding box containing any non-blank
// or styled cell, with a minimum of (1, 1).
func usedBounds(em emulator.Emulator, skipScrollback bool) (cols, rows int) {
	sb := em.ScrollbackLen()
	if skipScrollback {
		sb = 0
	}
	totalRows := sb + em.Rows()
	for r := 0; r < totalRows; r++ {
		rowUsed := false
		for c := 0; c < em.Cols(); {
			var cell emulator.Cell
			if r < sb {
				cell = em.ScrollbackCell(r, c)
			} else {
				cell = em.Cell(r-sb, c)
			}
			if !isBlankCell(cell) {
				rowUsed = true
				if end := c + cell.Width; end > cols {
					cols = end
				}
			}
			c += cell.Width
		}
		if rowUsed {
			rows = r + 1
		}
	}
	return max(cols, 1), max(rows, 1)
}

func isBlankCell(c emulator.Cell) bool {
	return (c.Text == "" || c.Text == " ") &&
		c.Bg.Kind == emulator.ColorDefault &&
		c.Attrs&(emulator.AttrReverse|emulator.AttrUnderline|emulator.AttrStrikethrough) == 0
}
