package emulator

import "github.com/charmbracelet/x/vt"

type vtEmulator struct {
	term       *vt.SafeEmulator
	cols, rows int
}

// New returns an Emulator with a Screen grid of the given size, backed by
// charmbracelet/x/vt.
func New(cols, rows int) Emulator {
	return &vtEmulator{term: vt.NewSafeEmulator(cols, rows), cols: cols, rows: rows}
}

func (e *vtEmulator) Write(p []byte) (int, error) { return e.term.Write(p) }
func (e *vtEmulator) Cols() int                   { return e.cols }
func (e *vtEmulator) Rows() int                   { return e.rows }

func (e *vtEmulator) Cell(row, col int) Cell {
	c := e.term.CellAt(col, row)
	if c == nil {
		return Cell{Width: 1}
	}
	w := c.Width
	if w == 0 {
		w = 1
	}
	return Cell{Text: c.Content, Width: w}
}
