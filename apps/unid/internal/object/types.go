package object

import (
	"fmt"
	"math"
	"strings"

	"github.com/silee-tools/unid/internal/width"
)

type LegendPos int

const (
	LegendTop LegendPos = iota
	LegendBottom
	LegendLeft
	LegendRight
	LegendAuto
)

type Legend struct {
	Text     string
	Pos      LegendPos
	Overflow ContentOverflow
	Align    ContentAlign
}

// Anchorable is implemented by objects that can be arrow endpoints.
type Anchorable interface {
	SrcAnchor(side Side) (int, int)
	DstAnchor(side Side) (int, int)
}

// DrawObject is the interface for all drawable objects.
type DrawObject interface {
	Bounds() (int, int)   // (maxColExclusive, maxRowExclusive)
	Position() (int, int) // (col, row) top-left
	TypeName() string
	CollisionDesc() string
	Summary() string
}

// Bounds/Position/TypeName/CollisionDesc/Summary for each object type

func (r *Rect) Bounds() (int, int) {
	return r.Col + r.OuterWidth(), r.Row + r.OuterHeight()
}

func (r *Rect) Position() (int, int) { return r.Col, r.Row }
func (r *Rect) TypeName() string     { return "box" }

func (r *Rect) CollisionDesc() string {
	return fmt.Sprintf("box at (%d,%d) %dx%d", r.Col, r.Row, r.OuterWidth(), r.OuterHeight())
}

func (r *Rect) Summary() string {
	s := fmt.Sprintf("box (%d,%d) %dx%d %s", r.Col, r.Row, r.Width, r.Height, borderStyleName(r.Style))
	if r.HasContent && r.Content != "" {
		s += fmt.Sprintf(" \"%s\"", r.Content)
	}
	return s
}

func (t *Text) Bounds() (int, int) {
	maxW := 0
	lineCount := 1
	for _, line := range strings.Split(t.Content, "\n") {
		if w := width.StrWidth(line); w > maxW {
			maxW = w
		}
		lineCount++
	}
	lineCount = strings.Count(t.Content, "\n") + 1
	if lineCount < 1 {
		lineCount = 1
	}
	return t.Col + maxW, t.Row + lineCount
}

func (t *Text) Position() (int, int) { return t.Col, t.Row }
func (t *Text) TypeName() string     { return "text" }

func (t *Text) CollisionDesc() string {
	return fmt.Sprintf("text at (%d,%d) w=%d", t.Col, t.Row, width.StrWidth(t.Content))
}

func (t *Text) Summary() string {
	return fmt.Sprintf("text (%d,%d) \"%s\"", t.Col, t.Row, t.Content)
}

func (h *HLine) Bounds() (int, int) { return h.Col + h.Length, h.Row + 1 }
func (h *HLine) Position() (int, int) { return h.Col, h.Row }
func (h *HLine) TypeName() string     { return "hline" }

func (h *HLine) CollisionDesc() string {
	return fmt.Sprintf("hline at (%d,%d) len=%d", h.Col, h.Row, h.Length)
}

func (h *HLine) Summary() string {
	return fmt.Sprintf("hline (%d,%d) len=%d %s", h.Col, h.Row, h.Length, lineStyleName(h.Style))
}

func (v *VLine) Bounds() (int, int) { return v.Col + 1, v.Row + v.Length }
func (v *VLine) Position() (int, int) { return v.Col, v.Row }
func (v *VLine) TypeName() string     { return "vline" }

func (v *VLine) CollisionDesc() string {
	return fmt.Sprintf("vline at (%d,%d) len=%d", v.Col, v.Row, v.Length)
}

func (v *VLine) Summary() string {
	return fmt.Sprintf("vline (%d,%d) len=%d %s", v.Col, v.Row, v.Length, lineStyleName(v.Style))
}

func (a *ResolvedArrow) Bounds() (int, int) {
	maxCol, maxRow := 0, 0
	for _, wp := range a.Waypoints {
		if wp[0] > maxCol {
			maxCol = wp[0]
		}
		if wp[1] > maxRow {
			maxRow = wp[1]
		}
	}
	if a.Legend != nil {
		lc, lr, lw := LegendPosition(a.Waypoints, a.Legend)
		if lc+lw > maxCol {
			maxCol = lc + lw
		}
		if lr+1 > maxRow {
			maxRow = lr + 1
		}
	}
	return maxCol + 1, maxRow + 1
}

func (a *ResolvedArrow) Position() (int, int) {
	minCol, minRow := math.MaxInt, math.MaxInt
	for _, wp := range a.Waypoints {
		if wp[0] < minCol {
			minCol = wp[0]
		}
		if wp[1] < minRow {
			minRow = wp[1]
		}
	}
	return minCol, minRow
}

func (a *ResolvedArrow) TypeName() string { return "arrow" }

func (a *ResolvedArrow) CollisionDesc() string {
	pts := make([]string, len(a.Waypoints))
	for i, wp := range a.Waypoints {
		pts[i] = fmt.Sprintf("(%d,%d)", wp[0], wp[1])
	}
	return fmt.Sprintf("arrow %s", strings.Join(pts, "->"))
}

func (a *ResolvedArrow) Summary() string {
	pts := make([]string, len(a.Waypoints))
	for i, wp := range a.Waypoints {
		pts[i] = fmt.Sprintf("(%d,%d)", wp[0], wp[1])
	}
	return fmt.Sprintf("arrow %s", strings.Join(pts, " -> "))
}

func borderStyleName(s BorderStyle) string {
	switch s {
	case BorderLight:
		return "Light"
	case BorderHeavy:
		return "Heavy"
	case BorderDouble:
		return "Double"
	case BorderRounded:
		return "Rounded"
	}
	return "Light"
}

func lineStyleName(s LineStyle) string {
	switch s {
	case LineLight:
		return "Light"
	case LineHeavy:
		return "Heavy"
	case LineDouble:
		return "Double"
	case LineDash:
		return "Dash"
	}
	return "Light"
}
