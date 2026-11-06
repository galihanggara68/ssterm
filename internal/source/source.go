// Package source reads terminal output from piped stdin or a pseudo-terminal.
package source

import (
	"bytes"
	"errors"
	"io"

	"github.com/charmbracelet/x/term"
)

const (
	// DefaultCols and DefaultRows are the fallback PTY dimensions when no
	// terminal size can be detected.
	DefaultCols = 80
	DefaultRows = 24
)

// ErrEmptyStdin is returned when piped stdin has zero bytes.
var ErrEmptyStdin = errors.New("empty stdin")

// ReadStdin reads all bytes from piped stdin and normalizes line endings for
// terminal emulation.
func ReadStdin(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, ErrEmptyStdin
	}
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	data = bytes.TrimSuffix(data, []byte("\n"))
	data = bytes.ReplaceAll(data, []byte("\n"), []byte("\r\n"))
	return data, nil
}

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
