// Package cli implements the ssterm command line.
package cli

import (
	"bytes"
	"encoding/json"
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

// Version is the ssterm version string, injected at build time via -ldflags.
var Version = "dev"

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
	themeName := fs.String("theme", theme.Default().Name, "Theme name ("+strings.Join(theme.Names(), ", ")+")")
	cols := fs.Int("cols", 0, "screen width in columns (default: fit content)")
	rows := fs.Int("rows", 0, "screen height in rows (default: fit content)")
	font := fs.String("font", "", "SVG font-family stack")
	trim := fs.Bool("trim", false, "trim leading and trailing blank rows and columns")
	cropSpec := fs.String("crop", "", "crop Screen grid to COL,ROW,WxH")
	padding := fs.Int("padding", 0, "padding in pixels around the Screen grid")
	radius := fs.Int("radius", 0, "corner radius in pixels for rounded window corners")
	scale := fs.Int("scale", 1, "scale factor for PNG output (1 to 5)")
	chrome := fs.Bool("chrome", false, "draw the Theme's Chrome")
	title := fs.String("title", "", "window title drawn in Chrome (implies --chrome)")
	shadow := fs.Bool("shadow", false, "draw a drop shadow around the Screenshot")
	shell := fs.Bool("shell", false, "run PTY command through interactive $SHELL -i -c (loads .bashrc/.zshrc)")
	fs.BoolVar(shell, "s", false, "shorthand for --shell")
	showVersion := fs.Bool("version", false, "print the ssterm version and exit")
	fs.BoolVar(showVersion, "v", false, "shorthand for --version")
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
	if *showVersion {
		fmt.Fprintf(stdout, "ssterm %s\n", Version)
		return 0
	}

	cfg, err := loadUserConfig()
	if err != nil {
		fmt.Fprintf(stderr, "ssterm: %v\n", err)
		return 2
	}
	setFlags := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) {
		setFlags[f.Name] = true
	})
	if cfg.Theme != nil && !setFlags["theme"] {
		*themeName = *cfg.Theme
	}
	if cfg.Font != nil && !setFlags["font"] {
		*font = *cfg.Font
	}
	if cfg.Padding != nil && !setFlags["padding"] {
		*padding = *cfg.Padding
	}
	if cfg.Radius != nil && !setFlags["radius"] {
		*radius = *cfg.Radius
	}
	if cfg.Scale != nil && !setFlags["scale"] {
		*scale = *cfg.Scale
	}
	if cfg.Trim != nil && !setFlags["trim"] {
		*trim = *cfg.Trim
	}
	if cfg.Chrome != nil && !setFlags["chrome"] {
		*chrome = *cfg.Chrome
	}
	if cfg.Shadow != nil && !setFlags["shadow"] {
		*shadow = *cfg.Shadow
	}
	if cfg.Shell != nil && !setFlags["shell"] && !setFlags["s"] {
		*shell = *cfg.Shell
	}

	colsSet, rowsSet := setFlags["cols"], setFlags["rows"]
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
	if *radius < 0 {
		fmt.Fprintln(stderr, "ssterm: --radius must be >= 0")
		return 2
	}
	if *scale < 1 || *scale > 5 {
		fmt.Fprintln(stderr, "ssterm: --scale must be between 1 and 5")
		return 2
	}
	isPNG := strings.HasSuffix(strings.ToLower(*output), ".png")
	if !isPNG && setFlags["scale"] {
		fmt.Fprintln(stderr, "ssterm: --scale is only supported for PNG output (-o *.png)")
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

	skipScrollback := rowsSet
	renderCols, renderRows := usedBounds(em, skipScrollback)
	if colsSet {
		renderCols = *cols
	}
	if rowsSet {
		renderRows = *rows
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
			Radius:         *radius,
			Scale:          *scale,
			Chrome:         *chrome,
			Title:          *title,
			Shadow:         *shadow,
		})
	} else {
		img = svg.RenderWithOptions(em, th, svg.Options{
			Font:           *font,
			SkipScrollback: skipScrollback,
			Cols:           renderCols,
			Rows:           renderRows,
			Padding:        *padding,
			Radius:         *radius,
			Chrome:         *chrome,
			Title:          *title,
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

type userConfig struct {
	Theme   *string
	Font    *string
	Padding *int
	Radius  *int
	Scale   *int
	Trim    *bool
	Chrome  *bool
	Shadow  *bool
	Shell   *bool
}

func loadUserConfig() (userConfig, error) {
	var path string
	explicit := false
	if envPath := os.Getenv("SSTERM_CONFIG"); envPath != "" {
		path = envPath
		explicit = true
	} else if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		path = filepath.Join(xdg, "ssterm", "config.json")
	} else if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		path = filepath.Join(dir, "ssterm", "config.json")
	} else {
		return userConfig{}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if !explicit && errors.Is(err, os.ErrNotExist) {
			return userConfig{}, nil
		}
		return userConfig{}, fmt.Errorf("config %s: %w", path, err)
	}

	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return userConfig{}, fmt.Errorf("config %s: invalid JSON: expected object", path)
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(data, &entries); err != nil {
		return userConfig{}, fmt.Errorf("config %s: invalid JSON: %w", path, err)
	}

	var cfg userConfig
	strFields := map[string]**string{
		"theme": &cfg.Theme,
		"font":  &cfg.Font,
	}
	boolFields := map[string]**bool{
		"trim":   &cfg.Trim,
		"chrome": &cfg.Chrome,
		"shadow": &cfg.Shadow,
		"shell":  &cfg.Shell,
	}
	for k, v := range entries {
		isNull := bytes.Equal(bytes.TrimSpace(v), []byte("null"))
		switch {
		case strFields[k] != nil:
			var s string
			if isNull || json.Unmarshal(v, &s) != nil {
				return userConfig{}, fmt.Errorf("config %s: invalid field %q: expected string", path, k)
			}
			*strFields[k] = &s
		case boolFields[k] != nil:
			var b bool
			if isNull || json.Unmarshal(v, &b) != nil {
				return userConfig{}, fmt.Errorf("config %s: invalid field %q: expected boolean", path, k)
			}
			*boolFields[k] = &b
		case k == "padding":
			n, err := parseIntConfig(path, k, v, isNull, 0, 0, false)
			if err != nil {
				return userConfig{}, err
			}
			cfg.Padding = n
		case k == "radius":
			n, err := parseIntConfig(path, k, v, isNull, 0, 0, false)
			if err != nil {
				return userConfig{}, err
			}
			cfg.Radius = n
		case k == "scale":
			n, err := parseIntConfig(path, k, v, isNull, 1, 5, true)
			if err != nil {
				return userConfig{}, err
			}
			cfg.Scale = n
		default:
			return userConfig{}, fmt.Errorf("config %s: unknown field %q", path, k)
		}
	}
	return cfg, nil
}

func parseIntConfig(path, k string, v json.RawMessage, isNull bool, min, max int, hasMax bool) (*int, error) {
	var n int
	if isNull || bytes.ContainsAny(v, `".eE`) || json.Unmarshal(v, &n) != nil {
		return nil, fmt.Errorf("config %s: invalid field %q: expected integer", path, k)
	}
	if n < min || (hasMax && n > max) {
		if hasMax {
			return nil, fmt.Errorf("config %s: invalid field %q: must be between %d and %d", path, k, min, max)
		}
		return nil, fmt.Errorf("config %s: invalid field %q: must be >= %d", path, k, min)
	}
	return &n, nil
}
