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
	themeName := fs.String("theme", "dark", "Theme name")
	cols := fs.Int("cols", 0, "screen width in columns (default: fit content)")
	rows := fs.Int("rows", 0, "screen height in rows (default: fit content)")
	font := fs.String("font", "", "SVG font-family stack")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
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
	if len(bytes.TrimSpace(data)) == 0 {
		fmt.Fprintln(stderr, "ssterm: no input on stdin")
		return 1
	}
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	data = bytes.TrimSuffix(data, []byte("\n"))
	data = bytes.ReplaceAll(data, []byte("\n"), []byte("\r\n"))

	w, h := fitContent(data)
	if *cols > 0 {
		w = *cols
	}
	if *rows > 0 {
		h = *rows
	}
	em := emulator.New(w, h)
	if _, err := em.Write(data); err != nil {
		fmt.Fprintf(stderr, "ssterm: %v\n", err)
		return 1
	}

	img := svg.RenderWithOptions(em, th, svg.Options{
		Font:           *font,
		SkipScrollback: *rows > 0,
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

// fitContent sizes the Screen grid to the content (without a trailing
// newline): the widest line in display columns and the number of lines.
func fitContent(data []byte) (cols, rows int) {
	lines := strings.Split(string(data), "\r\n")
	for _, l := range lines {
		// Escape sequences take no columns, and \r overwrites: the widest
		// segment between carriage returns in display columns is used.
		for seg := range strings.SplitSeq(l, "\r") {
			if n := ansi.StringWidth(seg); n > cols {
				cols = n
			}
		}
	}
	return max(cols, 1), max(len(lines), 1)
}
