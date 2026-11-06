// Package source reads terminal output from piped stdin or a pseudo-terminal.
package source

import "github.com/charmbracelet/x/term"

const (
	// DefaultCols and DefaultRows are the fallback PTY dimensions when no
	// terminal size can be detected.
	DefaultCols = 80
	DefaultRows = 24
)

type fdProvider interface {
	Fd() uintptr
}

// TermSize returns the terminal dimensions (cols, rows) of the first stream in
// streams that refers to a terminal, or (80, 24) when none is a terminal.
func TermSize(streams ...any) (cols, rows int) {
	for _, s := range streams {
		f, ok := s.(fdProvider)
		if !ok {
			continue
		}
		if w, h, err := term.GetSize(f.Fd()); err == nil && w > 0 && h > 0 {
			return w, h
		}
	}
	return DefaultCols, DefaultRows
}
