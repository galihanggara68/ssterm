// Package enhance applies opt-in cosmetic Enhancements (trim, crop, padding)
// to an emulated Screen grid before rendering.
package enhance

import (
	"fmt"
	"strconv"
	"strings"

	"ssterm/internal/emulator"
)

// CropRect specifies a rectangular sub-grid in 0-based cell coordinates.
type CropRect struct {
	X, Y int
	W, H int
}

// ParseCrop parses a --crop specification in X,Y,WxH, WxH+X+Y, or X,Y,W,H form.
func ParseCrop(spec string) (CropRect, error) {
	var xStr, yStr, wStr, hStr string
	switch {
	case strings.Contains(spec, "+"):
		parts := strings.Split(spec, "+")
		if len(parts) != 3 {
			return CropRect{}, fmt.Errorf("invalid --crop %q (expected COL,ROW,WxH or WxH+COL+ROW)", spec)
		}
		wh := strings.Split(strings.ToLower(parts[0]), "x")
		if len(wh) != 2 {
			return CropRect{}, fmt.Errorf("invalid --crop %q (expected COL,ROW,WxH or WxH+COL+ROW)", spec)
		}
		wStr, hStr, xStr, yStr = wh[0], wh[1], parts[1], parts[2]
	default:
		parts := strings.Split(spec, ",")
		switch len(parts) {
		case 3:
			wh := strings.Split(strings.ToLower(parts[2]), "x")
			if len(wh) != 2 {
				return CropRect{}, fmt.Errorf("invalid --crop %q (expected COL,ROW,WxH)", spec)
			}
			xStr, yStr, wStr, hStr = parts[0], parts[1], wh[0], wh[1]
		case 4:
			xStr, yStr, wStr, hStr = parts[0], parts[1], parts[2], parts[3]
		default:
			return CropRect{}, fmt.Errorf("invalid --crop %q (expected COL,ROW,WxH)", spec)
		}
	}
	x, errX := strconv.Atoi(xStr)
	y, errY := strconv.Atoi(yStr)
	w, errW := strconv.Atoi(wStr)
	h, errH := strconv.Atoi(hStr)
	if errX != nil || errY != nil || errW != nil || errH != nil {
		return CropRect{}, fmt.Errorf("invalid --crop %q (expected integer coordinates and dimensions)", spec)
	}
	if x < 0 || y < 0 {
		return CropRect{}, fmt.Errorf("invalid --crop %q: column and row must be >= 0", spec)
	}
	if w <= 0 || h <= 0 {
		return CropRect{}, fmt.Errorf("invalid --crop %q: width and height must be > 0", spec)
	}
	return CropRect{X: x, Y: y, W: w, H: h}, nil
}

// Crop extracts the rectangular sub-grid defined by rect from the active
// Screen grid view.
func Crop(em emulator.Emulator, skipScrollback bool, cols, rows int, rect CropRect) (emulator.Emulator, bool, int, int) {
	sb, totalCols, totalRows := viewDims(em, skipScrollback, cols, rows)
	return extract(em, sb, totalCols, totalRows, rect.X, rect.Y, rect.W, rect.H), true, rect.W, rect.H
}

// IsBlankCell reports whether a cell is visually empty (blank text with no
// background color, reverse video, text decoration or hyperlink).
func IsBlankCell(c emulator.Cell) bool {
	return (c.Text == "" || c.Text == " ") &&
		c.Bg.Kind == emulator.ColorDefault &&
		c.Attrs&(emulator.AttrReverse|emulator.AttrUnderline|emulator.AttrStrikethrough) == 0 &&
		c.URL == ""
}

type subGrid struct {
	cols, rows int
	cells      [][]emulator.Cell
}

func (g *subGrid) Write(p []byte) (int, error)         { return len(p), nil }
func (g *subGrid) Cols() int                           { return g.cols }
func (g *subGrid) Rows() int                           { return g.rows }
func (g *subGrid) ScrollbackLen() int                  { return 0 }
func (g *subGrid) ScrollbackCell(row, col int) emulator.Cell { return emulator.Cell{Width: 1} }

func (g *subGrid) Cell(row, col int) emulator.Cell {
	if row < 0 || row >= g.rows || col < 0 || col >= g.cols {
		return emulator.Cell{Width: 1}
	}
	return g.cells[row][col]
}

// Trim strips leading and trailing blank rows and columns around the active
// Screen grid view, leaving at least a 1x1 grid when all cells are blank.
func Trim(em emulator.Emulator, skipScrollback bool, cols, rows int) (emulator.Emulator, bool, int, int) {
	sb, totalCols, totalRows := viewDims(em, skipScrollback, cols, rows)
	var (
		found          bool
		minRow, maxRow int
		minCol, maxCol int
	)
	for r := 0; r < totalRows; r++ {
		for c := 0; c < totalCols; {
			cell := rawCell(em, sb, r, c)
			cw := max(cell.Width, 1)
			if !IsBlankCell(cell) {
				endC := min(c+cw-1, totalCols-1)
				if !found {
					minRow, maxRow = r, r
					minCol, maxCol = c, endC
					found = true
				} else {
					if r < minRow {
						minRow = r
					}
					if r > maxRow {
						maxRow = r
					}
					if c < minCol {
						minCol = c
					}
					if endC > maxCol {
						maxCol = endC
					}
				}
			}
			c += cw
		}
	}
	if !found {
		minRow, maxRow, minCol, maxCol = 0, 0, 0, 0
	}
	w := maxCol - minCol + 1
	h := maxRow - minRow + 1
	return extract(em, sb, totalCols, totalRows, minCol, minRow, w, h), true, w, h
}

func viewDims(em emulator.Emulator, skipScrollback bool, cols, rows int) (sb, totalCols, totalRows int) {
	sb = em.ScrollbackLen()
	if skipScrollback {
		sb = 0
	}
	totalRows = sb + em.Rows()
	if rows > 0 && rows < totalRows {
		totalRows = rows
	}
	totalCols = em.Cols()
	if cols > 0 && cols < totalCols {
		totalCols = cols
	}
	return sb, max(totalCols, 1), max(totalRows, 1)
}

func rawCell(em emulator.Emulator, sb, row, col int) emulator.Cell {
	if row < sb {
		return em.ScrollbackCell(row, col)
	}
	return em.Cell(row-sb, col)
}

func extract(em emulator.Emulator, sb, totalCols, totalRows, x, y, w, h int) *subGrid {
	cells := make([][]emulator.Cell, h)
	for dy := 0; dy < h; dy++ {
		row := make([]emulator.Cell, w)
		for dx := range row {
			row[dx] = emulator.Cell{Width: 1}
		}
		srcR := y + dy
		if srcR >= 0 && srcR < totalRows {
			for srcC := 0; srcC < totalCols; {
				c := rawCell(em, sb, srcR, srcC)
				cw := max(c.Width, 1)
				switch {
				case cw == 2 && srcC+1 == x && w > 0:
					// Wide glyph straddles the left boundary: replace right half with a 1-wide space.
					row[0] = splitWideCell(c)
				case cw == 2 && srcC == x+w-1 && srcC >= x:
					// Wide glyph straddles the right boundary: replace left half with a 1-wide space.
					row[w-1] = splitWideCell(c)
				case srcC >= x && srcC+cw <= x+w:
					dstC := srcC - x
					c.Width = cw
					row[dstC] = c
					if cw == 2 {
						row[dstC+1] = splitWideCell(c)
					}
				}
				srcC += cw
			}
		}
		cells[dy] = row
	}
	return &subGrid{cols: w, rows: h, cells: cells}
}

func splitWideCell(c emulator.Cell) emulator.Cell {
	return emulator.Cell{
		Text:  " ",
		Width: 1,
		Fg:    c.Fg,
		Bg:    c.Bg,
		Attrs: c.Attrs,
		URL:   c.URL,
	}
}
