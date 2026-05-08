package errors

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoCollisionError(t *testing.T) {
	assert.Equal(t, "collision not defined", (&NoCollisionError{}).Error())
}

func TestOutOfBoundsError(t *testing.T) {
	e := &OutOfBoundsError{Col: 5, Row: 10, CanvasWidth: 20, CanvasHeight: 15}
	assert.Equal(t, "position (5, 10) is out of canvas bounds (20x15)", e.Error())
}

func TestCollisionError(t *testing.T) {
	e := &CollisionError{
		IncomingIdx: 2, IncomingDesc: "box at (0,0) 5x3",
		ExistingIdx: 1, ExistingDesc: "box at (2,1) 4x2",
		OverlapCol: 2, OverlapRow: 1, OverlapEndCol: 4, OverlapEndRow: 2, OverlapW: 3, OverlapH: 2,
	}
	assert.Contains(t, e.Error(), "collision: object #2")
	assert.Contains(t, e.Error(), "overlaps object #1")
}

func TestLabelOverflowError(t *testing.T) {
	e := &LabelOverflowError{Label: "Hello World", LabelWidth: 11, InnerWidth: 5}
	assert.Contains(t, e.Error(), "content overflow")
	assert.Contains(t, e.Error(), "'Hello World'")
}

func TestParseError(t *testing.T) {
	e := &ParseError{Line: 3, Message: "unknown command 'foo'"}
	assert.Equal(t, "parse error at line 3: unknown command 'foo'", e.Error())
}
