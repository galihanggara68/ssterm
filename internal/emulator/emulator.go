// Package emulator turns a stream of terminal output into a Screen grid.
//
// The rest of ssterm depends only on the Emulator interface, so the VT
// library behind it can be replaced.
package emulator

// Cell is one position of the Screen grid.
type Cell struct {
	// Text is the grapheme shown in the cell; empty for a blank cell.
	Text string
	// Width is the number of columns the cell spans.
	Width int
}

// Emulator consumes terminal output and exposes the resulting Screen grid.
type Emulator interface {
	// Write feeds terminal output (text and escape sequences).
	Write(p []byte) (int, error)
	// Cols and Rows give the size of the Screen grid.
	Cols() int
	Rows() int
	// Cell returns the cell at the given zero-based row and column.
	Cell(row, col int) Cell
	// ScrollbackLen gives the number of Scrollback lines (lines that
	// scrolled off the top of the Screen grid), oldest first.
	ScrollbackLen() int
	// ScrollbackCell returns the cell at the given zero-based Scrollback
	// row (0 is the oldest) and column.
	ScrollbackCell(row, col int) Cell
}
