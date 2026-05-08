package object

type LineStyle int

const (
	LineLight LineStyle = iota
	LineHeavy
	LineDouble
	LineDash
)

type HLine struct {
	Col, Row int
	Length   int
	Style    LineStyle
	ID       string
	Legend   *Legend
}

func NewHLine(col, row, length int) *HLine {
	return &HLine{Col: col, Row: row, Length: length}
}

func (h *HLine) SrcAnchor(side Side) (int, int) {
	mid := h.Col + h.Length/2
	switch side {
	case SideTop:
		return mid, max(h.Row, 1) - 1
	case SideBottom:
		return mid, h.Row + 1
	case SideLeft:
		return max(h.Col, 1) - 1, h.Row
	case SideRight:
		return h.Col + h.Length, h.Row
	}
	return 0, 0
}

func (h *HLine) DstAnchor(side Side) (int, int) {
	return h.SrcAnchor(side)
}

type VLine struct {
	Col, Row int
	Length   int
	Style    LineStyle
	ID       string
	Legend   *Legend
}

func NewVLine(col, row, length int) *VLine {
	return &VLine{Col: col, Row: row, Length: length}
}

func (v *VLine) SrcAnchor(side Side) (int, int) {
	mid := v.Row + v.Length/2
	switch side {
	case SideTop:
		return v.Col, max(v.Row, 1) - 1
	case SideBottom:
		return v.Col, v.Row + v.Length
	case SideLeft:
		return max(v.Col, 1) - 1, mid
	case SideRight:
		return v.Col + 1, mid
	}
	return 0, 0
}

func (v *VLine) DstAnchor(side Side) (int, int) {
	return v.SrcAnchor(side)
}
