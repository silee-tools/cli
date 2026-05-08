package object

import (
	"strings"

	"github.com/silee-tools/unid/internal/width"
)

type Text struct {
	Col, Row     int
	Content      string
	ID           string
	ContentAlign ContentAlign
}

func NewText(col, row int, content string) *Text {
	return &Text{Col: col, Row: row, Content: content}
}

func (t *Text) BBoxWidth() int {
	maxW := 0
	for _, line := range strings.Split(t.Content, "\n") {
		if w := width.StrWidth(line); w > maxW {
			maxW = w
		}
	}
	return maxW
}

func (t *Text) bboxHeight() int {
	n := strings.Count(t.Content, "\n") + 1
	if n < 1 {
		return 1
	}
	return n
}

func (t *Text) SrcAnchor(side Side) (int, int) {
	w := t.BBoxWidth()
	h := t.bboxHeight()
	switch side {
	case SideTop:
		return t.Col + w/2, max(t.Row, 1) - 1
	case SideBottom:
		return t.Col + w/2, t.Row + h
	case SideLeft:
		return max(t.Col, 1) - 1, t.Row + h/2
	case SideRight:
		return t.Col + w, t.Row + h/2
	}
	return 0, 0
}

func (t *Text) DstAnchor(side Side) (int, int) {
	return t.SrcAnchor(side)
}
