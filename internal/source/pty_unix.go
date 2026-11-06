//go:build !windows

package source

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

// RunPTY runs cmdArgs attached to a pseudo-terminal of size cols x rows and
// returns the raw output bytes and the command's exit code.
func RunPTY(cmdArgs []string, cols, rows int) ([]byte, int, error) {
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	if os.Getenv("TERM") == "" {
		cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	}
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
	if err != nil {
		return nil, 1, err
	}
	defer func() { _ = ptmx.Close() }()

	var buf bytes.Buffer
	_, readErr := io.Copy(&buf, ptmx)
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, syscall.EIO) {
		_ = cmd.Wait()
		return nil, 1, readErr
	}

	if waitErr := cmd.Wait(); waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			return buf.Bytes(), exitErr.ExitCode(), nil
		}
		return buf.Bytes(), 1, waitErr
	}
	return buf.Bytes(), 0, nil
}
