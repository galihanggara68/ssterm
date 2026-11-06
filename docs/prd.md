# PRD: ssterm, terminal Screenshots

## Overview
`ssterm` is a Go CLI that turns terminal output into a faithful image (SVG, PNG) of what a terminal would have shown. It takes output from piped stdin or from a command run in a PTY. It emulates the output on a screen grid, so cursor movement, `\r` progress bars and colors render correctly. Look and feel comes from pluggable Themes. Cosmetic changes are opt-in Enhancements. Terms follow [GLOSSARY.md](../GLOSSARY.md).

## Goals
- Produce Screenshots that match what a real terminal displays, including truecolor, 256-color and 16-color output.
- Be faithful by default; offer Enhancements only as opt-in flags.
- Support multiple terminal looks through Themes.
- Ship as a single static binary, with PNG rendered in pure Go and no external tools.
- Run on Linux and macOS (piped and PTY), and on Windows (piped only).

## Quality Gates

These commands must pass for every user story:
- `go build ./...` - Build
- `go vet ./...` - Static checks
- `go test ./...` - Tests

## User Stories

### US-001: Project scaffold and CLI skeleton
**Description:** As a developer, I want a Go module with a CLI entry point so that features have a home.

**Acceptance Criteria:**
- [ ] `go.mod` exists with module path `ssterm` (placeholder), and `cmd/ssterm/main.go` builds
- [ ] Core logic lives under `internal/`
- [ ] `ssterm --help` lists the flags `-o`, `--theme`, `--cols`, `--rows`, `--font`
- [ ] Unknown flags exit non-zero with a usage message

### US-002: Screen grid model and emulator interface
**Description:** As a developer, I want a small emulator interface over a library so that the library can be swapped.

**Acceptance Criteria:**
- [ ] An `internal/emulator` package defines the interface: feed bytes, resize, and read cells (rune, width, fg, bg, attributes, hyperlink), including Scrollback
- [ ] A decision on `charmbracelet/x/vt` versus `vt10x` is recorded in a short note, covering maintenance, wide characters, Scrollback and color handling
- [ ] One implementation passes the unit tests for the interface

### US-003: Emulate ANSI text and colors
**Description:** As a user, I want colored output parsed correctly so that Screenshots show real colors.

**Acceptance Criteria:**
- [ ] Fixture tests cover 16-color, 256-color and truecolor SGR sequences
- [ ] Fixture tests cover bold, italic, underline, strikethrough, dim, reverse and blink (stored as an attribute)
- [ ] Wide characters occupy 2 cells
- [ ] OSC 8 hyperlinks are stored per cell

### US-004: Cursor-driven output and Scrollback
**Description:** As a user, I want progress bars and clears to render as a terminal would so that output is not garbled.

**Acceptance Criteria:**
- [ ] A fixture using `\r` overwrites yields only the final line state
- [ ] Fixtures using cursor movement and erase sequences yield the expected grid
- [ ] Lines scrolled off the top are kept as Scrollback
- [ ] The alternate screen is handled without a crash

### US-005: Piped Source with auto-size
**Description:** As a user, I want `cmd | ssterm` to work without giving a size.

**Acceptance Criteria:**
- [ ] The grid width equals the longest line and the height equals the line count
- [ ] `--cols` and `--rows` override the size
- [ ] `--rows N` keeps only the last N lines
- [ ] Empty stdin exits non-zero with a clear error

### US-006: PTY Source
**Description:** As a user, I want `ssterm -- cmd args` to run a command in a PTY so that the program emits colors.

**Acceptance Criteria:**
- [ ] The command runs with a PTY and the Screenshot is taken from the final grid at exit
- [ ] The size is the current terminal size, falling back to 80x24, and `--cols`/`--rows` override it
- [ ] The command's exit code is propagated
- [ ] On Windows the PTY mode exits with a clear unsupported message

### US-007: Theme model and built-in Themes
**Description:** As a user, I want to pick a Theme so that the Screenshot looks like my terminal.

**Acceptance Criteria:**
- [ ] A Theme defines the 16-color palette, default fg/bg and Chrome style
- [ ] Built-ins ship: default dark, default light, macOS Terminal, iTerm2, GNOME, Windows Terminal
- [ ] `--theme name` selects one, and an unknown name lists the valid ones
- [ ] `--theme path.json` loads a user file, and invalid files report the field in error

### US-008: SVG renderer (faithful)
**Description:** As a user, I want an SVG Screenshot so that it can be embedded and its text selected.

**Acceptance Criteria:**
- [ ] SVG is the default output, written to stdout, or to a file with `-o x.svg`
- [ ] Each cell's colors and attributes render per the Theme, with truecolor and 256-color used exactly as emitted
- [ ] Bold-as-bright follows the Theme
- [ ] Blink renders as static
- [ ] OSC 8 hyperlinks become SVG links
- [ ] The font-family stack is configurable with `--font`
- [ ] Golden-file tests cover the SVG output

### US-009: PNG renderer
**Description:** As a user, I want a PNG Screenshot so that I can share it where SVG is not supported.

**Acceptance Criteria:**
- [ ] `-o x.png` writes a PNG, with the format chosen by extension
- [ ] PNG is rendered in pure Go with a bundled monospace font and no external tools
- [ ] Output is reproducible across machines
- [ ] Tests check the image dimensions and sampled pixel colors

### US-010: Enhancements
**Description:** As a user, I want opt-in cosmetic options so that Screenshots look polished when I ask for it.

**Acceptance Criteria:**
- [ ] Flags exist for `--trim`, `--crop`, `--padding`, `--chrome` and `--shadow`
- [ ] With no flags, the output is identical to faithful mode, with no Chrome
- [ ] `--chrome` draws the Theme's Chrome
- [ ] Each flag works on both SVG and PNG, covered by tests

## Functional Requirements
- FR-1: The CLI reads stdin when there is no `--`, and runs the command after `--` in a PTY.
- FR-2: The output format is chosen by the `-o` extension (`.svg`, `.png`), and defaults to SVG on stdout.
- FR-3: The emulator is accessed only through the `internal/emulator` interface.
- FR-4: The default mode is faithful, with no Enhancements applied.
- FR-5: Scrollback is included by default.
- FR-6: Themes supply the 16 ANSI colors and the default fg/bg. Truecolor and 256-color values are never altered.
- FR-7: Wide characters take 2 cells.
- FR-8: Errors are written to stderr and exit non-zero.

## Non-Goals
- Capturing a live terminal session or the current screen.
- Mid-run capture of TUIs (`--after`, scripted keys), which is planned for v2.
- Windows ConPTY support.
- Animated output (GIF, video).
- Shipping a Nerd Font–patched font.

## Technical Considerations
- Go single static binary, with the core under `internal/` so it can be exported later.
- Prefer an existing VT library behind our interface. An ADR is needed only if we build our own emulator.
- A PNG pure-Go renderer is a candidate ADR (hard to reverse, trade-off against rasterizing the SVG).
- The PTY needs a library such as `creack/pty`, on Unix only.
- Emoji and glyph coverage depends on the font.

## Success Metrics
- Fixture outputs for 16-color, 256-color, truecolor and progress-bar cases match their golden files.
- `go build ./... && go vet ./... && go test ./...` pass.
- A typical `ls --color | ssterm -o out.png` works with no external dependencies.

## Open Questions
- Which VT library, `charmbracelet/x/vt` or `vt10x`? This is settled in US-002.
- The real module path.
