// Package enhance applies opt-in cosmetic Enhancements (trim, crop, padding,
// Chrome, shadow) to an emulated Screen grid and computes Screenshot layout.
package enhance

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"ssterm/internal/emulator"
	"ssterm/internal/theme"
)

// Cell and Chrome dimensions in pixels.
const (
	CellW        = 9
	CellH        = 18
	ChromeH      = 32
	ShadowPad    = 20
	chromeMargin = 72
)

// Layout holds the pixel geometry of a rendered Screenshot.
type Layout struct {
	SB, Cols, TotalRows int
	BodyW, BodyH        int
	WinX, WinY          int
	WinW, WinH          int
	Radius              int
	BodyX, BodyY        int
	ContentX, ContentY  int
	TotalW, TotalH      int
}

// ComputeLayout calculates the pixel geometry for rendering em with the given
// view constraints and cosmetic Enhancements.
func ComputeLayout(em emulator.Emulator, skipScrollback bool, cols, rows, padding, radius int, chrome, shadow bool) Layout {
	sb, c, r := ViewDims(em, skipScrollback, cols, rows)
	p := max(padding, 0)
	w, h := c*CellW+2*p, r*CellH+2*p
	winX, winY := 0, 0
	if shadow {
		winX, winY = ShadowPad, ShadowPad
	}
	bodyX, bodyY := winX, winY
	winW, winH := w, h
	if chrome {
		bodyY += ChromeH
		winH += ChromeH
	}
	rad := max(radius, 0)
	maxR := min(winW/2, winH/2)
	if rad > maxR {
		rad = maxR
	}
	return Layout{
		SB:        sb,
		Cols:      c,
		TotalRows: r,
		BodyW:     w,
		BodyH:     h,
		WinX:      winX,
		WinY:      winY,
		WinW:      winW,
		WinH:      winH,
		Radius:    rad,
		BodyX:     bodyX,
		BodyY:     bodyY,
		ContentX:  bodyX + p,
		ContentY:  bodyY + p,
		TotalW:    winW + 2*winX,
		TotalH:    winH + 2*winY,
	}
}

// ChromeButton describes a single window control button in Chrome.
type ChromeButton struct {
	Circle     bool
	CX, CY, R  int
	X, Y, W, H int
	Color      string
}

// ChromeSpec describes the resolved title bar and buttons for a Theme's Chrome.
type ChromeSpec struct {
	Background string
	Foreground string
	Title      string
	TitleCols  int
	TitleX     int
	TitleY     int
	X, Y, W, H int
	Buttons    [3]ChromeButton
}

// ComputeChrome returns the resolved Chrome geometry for th at window origin
// (winX, winY) and width winW.
func ComputeChrome(th theme.Theme, winX, winY, winW int, title string) ChromeSpec {
	c := th.Chrome
	bg := c.Background
	if bg == "" {
		bg = th.Background
	}
	fg := c.Foreground
	if fg == "" {
		fg = th.Foreground
	}
	cy := winY + ChromeH/2
	spec := ChromeSpec{
		Background: bg,
		Foreground: fg,
		X:          winX,
		Y:          winY,
		W:          winW,
		H:          ChromeH,
	}
	if maxCells := (winW - 2*chromeMargin) / CellW; title != "" && maxCells >= 1 {
		fitted, cols := truncateTitle(title, maxCells)
		if fitted != "" {
			spec.Title = fitted
			spec.TitleCols = cols
			spec.TitleX = winX + (winW-cols*CellW)/2
			spec.TitleY = cy + 5
		}
	}
	switch strings.ToLower(c.Layout) {
	case "gnome":
		startX := max(winX+16, winX+winW-56)
		for i, col := range c.Buttons {
			spec.Buttons[i] = ChromeButton{Circle: true, CX: startX + i*20, CY: cy, R: 6, Color: col}
		}
	case "windows":
		startX := max(winX+8, winX+winW-64)
		spec.Buttons[0] = ChromeButton{X: startX, Y: cy - 1, W: 10, H: 2, Color: c.Buttons[0]}
		spec.Buttons[1] = ChromeButton{X: startX + 20, Y: cy - 5, W: 10, H: 10, Color: c.Buttons[1]}
		spec.Buttons[2] = ChromeButton{X: startX + 40, Y: cy - 8, W: 16, H: 16, Color: c.Buttons[2]}
	default:
		for i, col := range c.Buttons {
			spec.Buttons[i] = ChromeButton{Circle: true, CX: winX + 16 + i*20, CY: cy, R: 6, Color: col}
		}
	}
	return spec
}

func truncateTitle(title string, maxCells int) (string, int) {
	w := ansi.StringWidth(title)
	if w <= maxCells {
		return title, w
	}
	if maxCells == 1 {
		return "…", 1
	}
	var b strings.Builder
	cols := 0
	for _, r := range title {
		rw := ansi.StringWidth(string(r))
		if cols+rw > maxCells-1 {
			break
		}
		b.WriteRune(r)
		cols += rw
	}
	b.WriteString("…")
	return b.String(), cols + 1
}

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
func Crop(em emulator.Emulator, skipScrollback bool, cols, rows int, rect CropRect) emulator.Emulator {
	sb, totalCols, totalRows := ViewDims(em, skipScrollback, cols, rows)
	return extract(em, sb, totalCols, totalRows, rect.X, rect.Y, rect.W, rect.H)
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

func (g *subGrid) Write(p []byte) (int, error)               { return len(p), nil }
func (g *subGrid) Cols() int                                 { return g.cols }
func (g *subGrid) Rows() int                                 { return g.rows }
func (g *subGrid) ScrollbackLen() int                        { return 0 }
func (g *subGrid) ScrollbackCell(row, col int) emulator.Cell { return emulator.Cell{Width: 1} }

func (g *subGrid) Cell(row, col int) emulator.Cell {
	if row < 0 || row >= g.rows || col < 0 || col >= g.cols {
		return emulator.Cell{Width: 1}
	}
	return g.cells[row][col]
}

// Trim strips leading and trailing blank rows and columns around the active
// Screen grid view, leaving at least a 1x1 grid when all cells are blank.
func Trim(em emulator.Emulator, skipScrollback bool, cols, rows int) emulator.Emulator {
	sb, totalCols, totalRows := ViewDims(em, skipScrollback, cols, rows)
	var (
		found          bool
		minRow, maxRow int
		minCol, maxCol int
	)
	for r := 0; r < totalRows; r++ {
		for c := 0; c < totalCols; {
			cell := CellAt(em, sb, r, c)
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
	return extract(em, sb, totalCols, totalRows, minCol, minRow, w, h)
}

// ViewDims resolves the active Scrollback row count and clamped grid dimensions.
func ViewDims(em emulator.Emulator, skipScrollback bool, cols, rows int) (sb, totalCols, totalRows int) {
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

// CellAt returns the cell at (row, col) from Scrollback (when row < sb) or the
// active Screen grid.
func CellAt(em emulator.Emulator, sb, row, col int) emulator.Cell {
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
				c := CellAt(em, sb, srcR, srcC)
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
