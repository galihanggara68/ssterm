//go:build !windows

package source

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/creack/pty"
)

// RunPTY runs cmdArgs attached to a pseudo-terminal of size cols x rows and
// returns the raw output bytes and the command's exit code. When useShell is
// true, cmdArgs is executed via the user's interactive shell ($SHELL) so
// aliases and functions defined in shell profiles (.bashrc, .zshrc) are loaded.
func RunPTY(cmdArgs []string, cols, rows int, useShell bool) ([]byte, int, error) {
	var cmd *exec.Cmd
	if useShell {
		cmd = shellCommand(cmdArgs)
	} else {
		cmd = exec.Command(cmdArgs[0], cmdArgs[1:]...)
	}
	env := os.Environ()
	if term := os.Getenv("TERM"); term == "" || term == "dumb" {
		env = append(env, "TERM=xterm-256color")
	}
	if os.Getenv("COLORTERM") == "" {
		env = append(env, "COLORTERM=truecolor")
	}
	if os.Getenv("CLICOLOR_FORCE") == "" {
		env = append(env, "CLICOLOR_FORCE=1")
	}
	if os.Getenv("FORCE_COLOR") == "" {
		env = append(env, "FORCE_COLOR=1")
	}
	if useShell && os.Getenv("PROMPT_EOL_MARK") == "" {
		env = append(env, "PROMPT_EOL_MARK=")
	}
	cmd.Env = env
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

func shellCommand(args []string) *exec.Cmd {
	sh := userShell()
	joined := joinShellCommand(args)
	switch filepath.Base(sh) {
	case "bash":
		script := "if [ -f \"$HOME/.bashrc\" ]; then . \"$HOME/.bashrc\" >/dev/null 2>&1; fi\n" + joined
		return exec.Command(sh, "--norc", "-i", "-c", script)
	case "zsh":
		script := "if [ -f \"$HOME/.zshrc\" ]; then . \"$HOME/.zshrc\" >/dev/null 2>&1; fi\n" + joined
		return exec.Command(sh, "--no-rcs", "-i", "-c", script)
	default:
		return exec.Command(sh, "-i", "-c", joined)
	}
}

func userShell() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	if sh, err := exec.LookPath("bash"); err == nil {
		return sh
	}
	return "sh"
}

func joinShellCommand(args []string) string {
	if len(args) == 1 {
		return args[0]
	}
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = shellQuote(a)
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '_' || c == '-' || c == '.' || c == '/' || c == ':' || c == '=' || c == '+' || c == '@') {
			return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
		}
	}
	return s
}
