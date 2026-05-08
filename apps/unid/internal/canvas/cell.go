package canvas

import "github.com/silee-tools/unid/internal/width"

type Cell struct {
	Ch             rune
	DisplayWidth   int
	IsContinuation bool
}

func NewCell(ch rune) Cell {
	return Cell{Ch: ch, DisplayWidth: width.CharWidth(ch)}
}

func SpaceCell() Cell {
	return Cell{Ch: ' ', DisplayWidth: 1}
}

func ContinuationCell() Cell {
	return Cell{Ch: 0, DisplayWidth: 0, IsContinuation: true}
}
