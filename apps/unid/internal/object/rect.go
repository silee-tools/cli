package object

type BorderStyle int

const (
	BorderLight BorderStyle = iota
	BorderHeavy
	BorderDouble
	BorderRounded
)

type ContentOverflow int

const (
	OverflowEllipsis ContentOverflow = iota
	OverflowOverflow
	OverflowHidden
	OverflowError
)

type ContentAlign int

const (
	AlignLeft ContentAlign = iota
	AlignCenter
	AlignRight
)

type Side int

const (
	SideTop Side = iota
	SideRight
	SideBottom
	SideLeft
)

type Rect struct {
	Col, Row       int
	Width, Height  int // inner dimensions
	ID             string
	Content        string
	HasContent     bool
	Style          BorderStyle
	ContentOverflow ContentOverflow
	ContentAlign   ContentAlign
	Legend         *Legend
}

func NewRect(col, row, w, h int) *Rect {
	return &Rect{Col: col, Row: row, Width: w, Height: h}
}

func (r *Rect) OuterWidth() int  { return r.Width + 2 }
func (r *Rect) OuterHeight() int { return r.Height + 2 }
func (r *Rect) CenterCol() int   { return r.Col + 1 + r.Width/2 }
func (r *Rect) CenterRow() int   { return r.Col + 1 + r.Height/2 }

func (r *Rect) SrcAnchor(side Side) (int, int) {
	centerCol := r.Col + 1 + r.Width/2
	centerRow := r.Row + 1 + r.Height/2
	switch side {
	case SideTop:
		return centerCol, max(r.Row, 1) - 1
	case SideRight:
		return r.Col + r.Width + 2, centerRow
	case SideBottom:
		return centerCol, r.Row + r.Height + 2
	case SideLeft:
		return max(r.Col, 1) - 1, centerRow
	}
	return 0, 0
}

func (r *Rect) DstAnchor(side Side) (int, int) {
	return r.SrcAnchor(side) // same for rect
}
