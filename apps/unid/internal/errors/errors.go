package errors

import "fmt"

type NoCollisionError struct{}

func (e *NoCollisionError) Error() string { return "collision not defined" }

type OutOfBoundsError struct {
	Col, Row, CanvasWidth, CanvasHeight int
}

func (e *OutOfBoundsError) Error() string {
	return fmt.Sprintf("position (%d, %d) is out of canvas bounds (%dx%d)",
		e.Col, e.Row, e.CanvasWidth, e.CanvasHeight)
}

type CollisionError struct {
	IncomingIdx  int
	IncomingDesc string
	ExistingIdx  int
	ExistingDesc string
	OverlapCol   int
	OverlapRow   int
	OverlapEndCol int
	OverlapEndRow int
	OverlapW     int
	OverlapH     int
}

func (e *CollisionError) Error() string {
	return fmt.Sprintf(
		"collision: object #%d (%s) overlaps object #%d (%s) at (%d,%d)-(%d,%d) size %dx%d",
		e.IncomingIdx, e.IncomingDesc,
		e.ExistingIdx, e.ExistingDesc,
		e.OverlapCol, e.OverlapRow,
		e.OverlapEndCol, e.OverlapEndRow,
		e.OverlapW, e.OverlapH,
	)
}

type LabelOverflowError struct {
	Label      string
	LabelWidth int
	InnerWidth int
}

func (e *LabelOverflowError) Error() string {
	return fmt.Sprintf("content overflow: '%s' (%d cols) exceeds box inner width (%d cols)",
		e.Label, e.LabelWidth, e.InnerWidth)
}

type ParseError struct {
	Line    int
	Message string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("parse error at line %d: %s", e.Line, e.Message)
}
