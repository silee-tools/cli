package canvas

import (
	"strings"

	uerr "github.com/silee-tools/unid/internal/errors"
	"github.com/silee-tools/unid/internal/width"
)

type Canvas struct {
	W     int
	H     int
	grid  [][]Cell
	owner [][]int // -1 = no owner
}

func New(w, h int) *Canvas {
	grid := make([][]Cell, h)
	owner := make([][]int, h)
	for r := range h {
		grid[r] = make([]Cell, w)
		owner[r] = make([]int, w)
		for c := range w {
			grid[r][c] = SpaceCell()
			owner[r][c] = -1
		}
	}
	return &Canvas{W: w, H: h, grid: grid, owner: owner}
}

func (cv *Canvas) OwnerAt(col, row int) int {
	if row < 0 || row >= cv.H || col < 0 || col >= cv.W {
		return -1
	}
	return cv.owner[row][col]
}

func (cv *Canvas) PutChar(col, row int, ch rune, collision bool, objectIdx int) error {
	w := width.CharWidth(ch)

	if col+w > cv.W || row >= cv.H || col < 0 || row < 0 {
		return &uerr.OutOfBoundsError{Col: col, Row: row, CanvasWidth: cv.W, CanvasHeight: cv.H}
	}

	if collision {
		existing := &cv.grid[row][col]
		if existing.Ch != ' ' && !existing.IsContinuation {
			return cv.collisionError(col, row, objectIdx)
		}
		if w == 2 {
			next := &cv.grid[row][col+1]
			if next.Ch != ' ' && !next.IsContinuation {
				return cv.collisionError(col+1, row, objectIdx)
			}
		}
	}

	cv.grid[row][col] = NewCell(ch)
	cv.owner[row][col] = objectIdx
	if w == 2 {
		cv.grid[row][col+1] = ContinuationCell()
		cv.owner[row][col+1] = objectIdx
	}
	return nil
}

func (cv *Canvas) PutStr(col, row int, s string, collision bool, objectIdx int) error {
	currentCol := col
	for _, ch := range s {
		if err := cv.PutChar(currentCol, row, ch, collision, objectIdx); err != nil {
			return err
		}
		currentCol += width.CharWidth(ch)
	}
	return nil
}

func (cv *Canvas) Render() string {
	lines := make([]string, 0, cv.H)
	for _, row := range cv.grid {
		var b strings.Builder
		for _, cell := range row {
			if cell.IsContinuation {
				continue
			}
			b.WriteRune(cell.Ch)
		}
		lines = append(lines, strings.TrimRight(b.String(), " "))
	}
	// Remove trailing empty lines
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func (cv *Canvas) collisionError(col, row, incomingIdx int) error {
	existingIdx := cv.owner[row][col]
	if existingIdx < 0 {
		existingIdx = 0
	}
	return &uerr.CollisionError{
		IncomingIdx:   incomingIdx,
		ExistingIdx:   existingIdx,
		OverlapCol:    col,
		OverlapRow:    row,
		OverlapEndCol: col,
		OverlapEndRow: row,
		OverlapW:      1,
		OverlapH:      1,
	}
}
