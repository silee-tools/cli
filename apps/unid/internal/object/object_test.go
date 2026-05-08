package object

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRectBoundsAndPosition(t *testing.T) {
	r := NewRect(2, 3, 10, 5) // inner 10x5 → outer 12x7
	bx, by := r.Bounds()
	assert.Equal(t, 14, bx) // 2 + 12
	assert.Equal(t, 10, by) // 3 + 7
	px, py := r.Position()
	assert.Equal(t, 2, px)
	assert.Equal(t, 3, py)
}

func TestRectAnchor(t *testing.T) {
	r := NewRect(2, 2, 4, 2) // outer: (2,2)-(7,5)
	// center: col=2+1+2=5, row=2+1+1=4
	cx, cy := r.SrcAnchor(SideTop)
	assert.Equal(t, 5, cx)
	assert.Equal(t, 1, cy) // row-1
	rx, ry := r.SrcAnchor(SideRight)
	assert.Equal(t, 8, rx) // col+w+2
	assert.Equal(t, 4, ry)
}

func TestTextBoundsAndPosition(t *testing.T) {
	txt := NewText(1, 1, "Hello")
	bx, by := txt.Bounds()
	assert.Equal(t, 6, bx) // 1+5
	assert.Equal(t, 2, by) // 1+1
	px, py := txt.Position()
	assert.Equal(t, 1, px)
	assert.Equal(t, 1, py)
}

func TestTextMultiline(t *testing.T) {
	txt := NewText(0, 0, "AB\nCD\nEF")
	bx, by := txt.Bounds()
	assert.Equal(t, 2, bx)
	assert.Equal(t, 3, by)
}

func TestHLineBounds(t *testing.T) {
	hl := NewHLine(5, 3, 10)
	bx, by := hl.Bounds()
	assert.Equal(t, 15, bx)
	assert.Equal(t, 4, by)
}

func TestVLineBounds(t *testing.T) {
	vl := NewVLine(5, 3, 8)
	bx, by := vl.Bounds()
	assert.Equal(t, 6, bx)
	assert.Equal(t, 11, by)
}

func TestArrowheadValidation(t *testing.T) {
	assert.True(t, IsValidArrowhead('▶'))
	assert.True(t, IsValidArrowhead('→'))
	assert.True(t, IsValidArrowhead('⇒'))
	assert.False(t, IsValidArrowhead('X'))
	assert.False(t, IsValidArrowhead('*'))
}

func TestResolveArrowhead(t *testing.T) {
	assert.Equal(t, '▼', ResolveArrowhead('▶', DirDown))
	assert.Equal(t, '▲', ResolveArrowhead('▶', DirUp))
	assert.Equal(t, '◀', ResolveArrowhead('▶', DirLeft))
	assert.Equal(t, '↓', ResolveArrowhead('→', DirDown))
}

func TestCornerChar(t *testing.T) {
	assert.Equal(t, '┐', CornerChar(DirRight, DirDown))
	assert.Equal(t, '└', CornerChar(DirDown, DirRight))
	assert.Equal(t, '┘', CornerChar(DirRight, DirUp))
	assert.Equal(t, '─', CornerChar(DirRight, DirRight))
	assert.Equal(t, '│', CornerChar(DirDown, DirDown))
}

func TestSegmentDir(t *testing.T) {
	assert.Equal(t, DirRight, SegmentDir(0, 0, 5, 0))
	assert.Equal(t, DirLeft, SegmentDir(5, 0, 0, 0))
	assert.Equal(t, DirDown, SegmentDir(0, 0, 0, 5))
	assert.Equal(t, DirUp, SegmentDir(0, 5, 0, 0))
}

func TestComputeRouteStraight(t *testing.T) {
	wp := ComputeRoute(0, 0, SideRight, 5, 0, SideLeft)
	assert.Equal(t, [][2]int{{0, 0}, {5, 0}}, wp)
}

func TestComputeRouteLShape(t *testing.T) {
	wp := ComputeRoute(5, 0, SideRight, 10, 5, SideTop)
	assert.Len(t, wp, 3)
	assert.Equal(t, [2]int{5, 0}, wp[0])
	assert.Equal(t, [2]int{10, 0}, wp[1])
	assert.Equal(t, [2]int{10, 5}, wp[2])
}

func TestSummaryFormats(t *testing.T) {
	r := NewRect(0, 0, 5, 3)
	assert.Contains(t, r.Summary(), "box")
	assert.Contains(t, r.TypeName(), "box")

	txt := NewText(0, 0, "hi")
	assert.Contains(t, txt.Summary(), "text")

	hl := NewHLine(0, 0, 5)
	assert.Contains(t, hl.Summary(), "hline")

	vl := NewVLine(0, 0, 5)
	assert.Contains(t, vl.Summary(), "vline")
}
