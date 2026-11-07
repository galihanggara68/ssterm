// Package cli implements the ssterm command line.
package cli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"ssterm/internal/emulator"
	"ssterm/internal/enhance"
	"ssterm/internal/png"
	"ssterm/internal/source"
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
	trim := fs.Bool("trim", false, "trim leading and trailing blank rows and columns")
	cropSpec := fs.String("crop", "", "crop Screen grid to COL,ROW,WxH")
	padding := fs.Int("padding", 0, "padding in pixels around the Screen grid")
	chrome := fs.Bool("chrome", false, "draw the Theme's Chrome")
	shadow := fs.Bool("shadow", false, "draw a drop shadow around the Screenshot")
	shell := fs.Bool("shell", false, "run PTY command through interactive $SHELL -i -c (loads .bashrc/.zshrc)")
	fs.BoolVar(shell, "s", false, "shorthand for --shell")
	var cmdArgs []string
	ptyMode := false
	if idx := slices.Index(args, "--"); idx >= 0 {
		ptyMode = true
		cmdArgs = args[idx+1:]
		args = args[:idx]
	}
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
	if *padding < 0 {
		fmt.Fprintln(stderr, "ssterm: --padding must be >= 0")
		return 2
	}
	var cropRect enhance.CropRect
	if *cropSpec != "" {
		var err error
		cropRect, err = enhance.ParseCrop(*cropSpec)
		if err != nil {
			fmt.Fprintf(stderr, "ssterm: %v\n", err)
			return 2
		}
	}
	ext := strings.ToLower(filepath.Ext(*output))
	if *output != "" && ext != ".svg" && ext != ".png" {
		fmt.Fprintf(stderr, "ssterm: unsupported output format %q (expected .svg or .png)\n", filepath.Ext(*output))
		return 2
	}

	th, err := theme.Resolve(*themeName)
	if err != nil {
		fmt.Fprintf(stderr, "ssterm: %v\n", err)
		return 2
	}

	var (
		data     []byte
		w, h     int
		exitCode int
	)
	if ptyMode {
		if len(cmdArgs) == 0 {
			fmt.Fprintln(stderr, "ssterm: missing command after --")
			return 2
		}
		w, h = source.TermSize(stdin, stdout, stderr)
		if colsSet {
			w = *cols
		}
		if rowsSet {
			h = *rows
		}
		data, exitCode, err = source.RunPTY(cmdArgs, w, h, *shell)
		if err != nil {
			fmt.Fprintf(stderr, "ssterm: %v\n", err)
			return exitCode
		}
	} else {
		data, err = source.ReadStdin(stdin)
		if err != nil {
			if errors.Is(err, source.ErrEmptyStdin) {
				fmt.Fprintln(stderr, "ssterm: empty stdin")
			} else {
				fmt.Fprintf(stderr, "ssterm: reading stdin: %v\n", err)
			}
			return 1
		}
		w, h = fitEstimate(data)
		if colsSet {
			w = *cols
		}
		if rowsSet {
			h = *rows
		}
	}

	em := emulator.New(w, h)
	if _, err := em.Write(data); err != nil {
		fmt.Fprintf(stderr, "ssterm: %v\n", err)
		return 1
	}

	skipScrollback := ptyMode || rowsSet
	renderCols, renderRows := w, h
	if !ptyMode {
		renderCols, renderRows = usedBounds(em, skipScrollback)
		if colsSet {
			renderCols = *cols
		}
		if rowsSet {
			renderRows = *rows
		}
	}
	if *trim {
		em = enhance.Trim(em, skipScrollback, renderCols, renderRows)
		renderCols, renderRows = em.Cols(), em.Rows()
	}
	if *cropSpec != "" {
		em = enhance.Crop(em, skipScrollback, renderCols, renderRows, cropRect)
		renderCols, renderRows = em.Cols(), em.Rows()
	}

	var img []byte
	if ext == ".png" {
		img = png.RenderWithOptions(em, th, png.Options{
			SkipScrollback: skipScrollback,
			Cols:           renderCols,
			Rows:           renderRows,
			Padding:        *padding,
			Chrome:         *chrome,
			Shadow:         *shadow,
		})
	} else {
		img = svg.RenderWithOptions(em, th, svg.Options{
			Font:           *font,
			SkipScrollback: skipScrollback,
			Cols:           renderCols,
			Rows:           renderRows,
			Padding:        *padding,
			Chrome:         *chrome,
			Shadow:         *shadow,
		})
	}
	if *output != "" {
		if err := os.WriteFile(*output, img, 0o644); err != nil {
			fmt.Fprintf(stderr, "ssterm: %v\n", err)
			return 1
		}
		return exitCode
	}
	if _, err := stdout.Write(img); err != nil {
		fmt.Fprintf(stderr, "ssterm: %v\n", err)
		return 1
	}
	return exitCode
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
			cell := enhance.CellAt(em, sb, r, c)
			if !enhance.IsBlankCell(cell) {
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
