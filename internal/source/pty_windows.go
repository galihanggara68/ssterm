//go:build windows

package source

import "errors"

// RunPTY returns an unsupported error on Windows.
func RunPTY(_ []string, _, _ int) ([]byte, int, error) {
	return nil, 1, errors.New("PTY mode is not supported on Windows")
}
