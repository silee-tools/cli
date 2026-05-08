package object

import "github.com/silee-tools/unid/internal/width"

type Dir int

const (
	DirRight Dir = iota
	DirLeft
	DirUp
	DirDown
)

var arrowheadFamilies = [][4]rune{
	{'▶', '◀', '▲', '▼'}, // default (filled triangle)
	{'→', '←', '↑', '↓'}, // arrow
	{'▷', '◁', '△', '▽'}, // outline triangle
	{'⇒', '⇐', '⇑', '⇓'}, // double arrow
}

func IsValidArrowhead(ch rune) bool {
	for _, f := range arrowheadFamilies {
		for _, c := range f {
			if c == ch {
				return true
			}
		}
	}
	return false
}

func ValidArrowheadChars() []rune {
	var chars []rune
	for _, f := range arrowheadFamilies {
		chars = append(chars, f[:]...)
	}
	return chars
}

func ResolveArrowhead(ch rune, dir Dir) rune {
	idx := dirIndex(dir)
	for _, f := range arrowheadFamilies {
		for _, c := range f {
			if c == ch {
				return f[idx]
			}
		}
	}
	return arrowheadFamilies[0][idx]
}

func DefaultArrowhead(dir Dir) rune {
	return arrowheadFamilies[0][dirIndex(dir)]
}

func dirIndex(dir Dir) int {
	switch dir {
	case DirRight:
		return 0
	case DirLeft:
		return 1
	case DirUp:
		return 2
	case DirDown:
		return 3
	}
	return 0
}

type ResolvedArrow struct {
	Waypoints [][2]int // (col, row) pairs
	Head      rune
	HasHead   bool
	Both      bool
	Legend    *Legend
}

func SideToOutgoingDir(side Side) Dir {
	switch side {
	case SideTop:
		return DirUp
	case SideRight:
		return DirRight
	case SideBottom:
		return DirDown
	case SideLeft:
		return DirLeft
	}
	return DirRight
}

func SideToIncomingDir(side Side) Dir {
	switch side {
	case SideTop:
		return DirDown
	case SideRight:
		return DirLeft
	case SideBottom:
		return DirUp
	case SideLeft:
		return DirRight
	}
	return DirRight
}

const routingGap = 2

func adaptiveGap(sx, sy, ex, ey int) int {
	dist := abs(sx-ex) + abs(sy-ey)
	switch {
	case dist <= 10:
		return 2
	case dist <= 25:
		return 3
	default:
		return 4
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func ComputeRoute(sx, sy int, srcSide Side, ex, ey int, dstSide Side) [][2]int {
	srcDir := SideToOutgoingDir(srcSide)
	dstDir := SideToIncomingDir(dstSide)

	srcHorizontal := srcDir == DirLeft || srcDir == DirRight
	dstHorizontal := dstDir == DirLeft || dstDir == DirRight
	perpendicular := srcHorizontal != dstHorizontal

	if perpendicular {
		return routePerpendicular(sx, sy, srcDir, ex, ey, dstDir)
	}
	return routeParallel(sx, sy, srcDir, ex, ey, dstDir)
}

func routePerpendicular(sx, sy int, srcDir Dir, ex, ey int, dstDir Dir) [][2]int {
	var bend [2]int
	switch {
	case (srcDir == DirRight || srcDir == DirLeft) && (dstDir == DirDown || dstDir == DirUp):
		bend = [2]int{ex, sy}
	case (srcDir == DirDown || srcDir == DirUp) && (dstDir == DirRight || dstDir == DirLeft):
		bend = [2]int{sx, ey}
	}

	if isFavorableBend(sx, sy, srcDir, bend[0], bend[1]) &&
		isFavorableBend(bend[0], bend[1], dstDir, ex, ey) {
		return [][2]int{{sx, sy}, bend, {ex, ey}}
	}

	return route3Bend(sx, sy, srcDir, ex, ey)
}

func routeParallel(sx, sy int, srcDir Dir, ex, ey int, dstDir Dir) [][2]int {
	horizontal := srcDir == DirLeft || srcDir == DirRight
	sameDirection := srcDir == dstDir

	if sameDirection {
		var isStraight bool
		if horizontal {
			isStraight = sy == ey &&
				((srcDir == DirRight && ex >= sx) || (srcDir == DirLeft && ex <= sx))
		} else {
			isStraight = sx == ex &&
				((srcDir == DirDown && ey >= sy) || (srcDir == DirUp && ey <= sy))
		}
		if isStraight {
			return [][2]int{{sx, sy}, {ex, ey}}
		}
	}

	return route2Bend(sx, sy, srcDir, ex, ey, horizontal, sameDirection)
}

func isFavorableBend(fx, fy int, dir Dir, tx, ty int) bool {
	switch dir {
	case DirRight:
		return tx >= fx
	case DirLeft:
		return tx <= fx
	case DirDown:
		return ty >= fy
	case DirUp:
		return ty <= fy
	}
	return false
}

func route2Bend(sx, sy int, srcDir Dir, ex, ey int, horizontal, sameDirection bool) [][2]int {
	if horizontal {
		if sameDirection {
			midX := (sx + ex) / 2
			return [][2]int{{sx, sy}, {midX, sy}, {midX, ey}, {ex, ey}}
		}
		gap := adaptiveGap(sx, sy, ex, ey)
		var midX int
		if srcDir == DirRight {
			midX = max(sx, ex) + gap
		} else {
			midX = max(min(sx, ex)-gap, 0)
		}
		return [][2]int{{sx, sy}, {midX, sy}, {midX, ey}, {ex, ey}}
	}

	if sameDirection {
		midY := (sy + ey) / 2
		return [][2]int{{sx, sy}, {sx, midY}, {ex, midY}, {ex, ey}}
	}
	gap := adaptiveGap(sx, sy, ex, ey)
	var midY int
	if srcDir == DirDown {
		midY = max(sy, ey) + gap
	} else {
		midY = max(min(sy, ey)-gap, 0)
	}
	return [][2]int{{sx, sy}, {sx, midY}, {ex, midY}, {ex, ey}}
}

func route3Bend(sx, sy int, srcDir Dir, ex, ey int) [][2]int {
	horizontalSrc := srcDir == DirLeft || srcDir == DirRight
	if horizontalSrc {
		midX := (sx + ex) / 2
		return [][2]int{{sx, sy}, {midX, sy}, {midX, ey}, {ex, ey}}
	}
	midY := (sy + ey) / 2
	return [][2]int{{sx, sy}, {sx, midY}, {ex, midY}, {ex, ey}}
}

func ComputeSelfLoop(sx, sy int, srcSide Side, ex, ey int, dstSide Side) [][2]int {
	gap := routingGap
	srcDir := SideToOutgoingDir(srcSide)
	dstAway := SideToOutgoingDir(dstSide)

	p1c, p1r := extendPoint(sx, sy, srcDir, gap)
	p2c, p2r := extendPoint(ex, ey, dstAway, gap)

	var raw [][2]int
	if p1c == p2c || p1r == p2r {
		raw = [][2]int{{sx, sy}, {p1c, p1r}, {p2c, p2r}, {ex, ey}}
	} else {
		srcHorizontal := srcDir == DirLeft || srcDir == DirRight
		if srcHorizontal {
			raw = [][2]int{{sx, sy}, {p1c, p1r}, {p1c, p2r}, {p2c, p2r}, {ex, ey}}
		} else {
			raw = [][2]int{{sx, sy}, {p1c, p1r}, {p2c, p1r}, {p2c, p2r}, {ex, ey}}
		}
	}

	// Deduplicate consecutive points
	deduped := make([][2]int, 0, len(raw))
	for _, pt := range raw {
		if len(deduped) == 0 || deduped[len(deduped)-1] != pt {
			deduped = append(deduped, pt)
		}
	}
	return deduped
}

func extendPoint(c, r int, dir Dir, distance int) (int, int) {
	switch dir {
	case DirRight:
		return c + distance, r
	case DirLeft:
		return max(c-distance, 0), r
	case DirDown:
		return c, r + distance
	case DirUp:
		return c, max(r-distance, 0)
	}
	return c, r
}

func CornerChar(incoming, outgoing Dir) rune {
	switch {
	case incoming == DirRight && outgoing == DirDown:
		return '┐'
	case incoming == DirRight && outgoing == DirUp:
		return '┘'
	case incoming == DirLeft && outgoing == DirDown:
		return '┌'
	case incoming == DirLeft && outgoing == DirUp:
		return '└'
	case incoming == DirDown && outgoing == DirRight:
		return '└'
	case incoming == DirDown && outgoing == DirLeft:
		return '┘'
	case incoming == DirUp && outgoing == DirRight:
		return '┌'
	case incoming == DirUp && outgoing == DirLeft:
		return '┐'
	case (incoming == DirLeft && outgoing == DirLeft) || (incoming == DirRight && outgoing == DirRight):
		return '─'
	case (incoming == DirUp && outgoing == DirUp) || (incoming == DirDown && outgoing == DirDown):
		return '│'
	default:
		return '┼'
	}
}

// LegendPosition computes arrow legend position and column based on waypoints, legend config, and align.
// Returns (col, row, textWidth).
func LegendPosition(wp [][2]int, legend *Legend) (int, int, int) {
	if len(wp) < 2 {
		return 0, 0, 0
	}

	// For bent arrows, pick the longest segment from the second one onwards.
	segIdx := 0
	if len(wp) >= 3 {
		bestLen := 0
		for i := 1; i < len(wp)-1; i++ {
			l := abs(wp[i][0]-wp[i+1][0]) + abs(wp[i][1]-wp[i+1][1])
			if l >= bestLen {
				bestLen = l
				segIdx = i
			}
		}
	}

	fc, fr := wp[segIdx][0], wp[segIdx][1]
	tc, tr := wp[segIdx+1][0], wp[segIdx+1][1]
	midC := (fc + tc) / 2
	midR := (fr + tr) / 2
	dir := SegmentDir(fc, fr, tc, tr)
	isHorizontal := dir == DirLeft || dir == DirRight

	effectivePos := legend.Pos
	if effectivePos == LegendAuto {
		if isHorizontal {
			effectivePos = LegendTop
		} else {
			effectivePos = LegendRight
		}
	}

	textW := width.StrWidth(legend.Text)

	var lgCol, lgRow int
	switch effectivePos {
	case LegendTop, LegendBottom:
		switch legend.Align {
		case AlignLeft:
			lgCol = midC
		case AlignCenter:
			lgCol = max(midC-textW/2, 0)
		case AlignRight:
			lgCol = max(midC-textW, 0)
		}
		if effectivePos == LegendTop {
			lgRow = max(midR, 1) - 1
		} else {
			lgRow = midR + 1
		}
	case LegendLeft:
		lgCol = max(midC-textW-1, 0)
		lgRow = midR
	case LegendRight:
		lgCol = midC + 1
		lgRow = midR
	}

	return lgCol, lgRow, textW
}

func SegmentDir(fx, fy, tx, ty int) Dir {
	if fy == ty {
		if tx > fx {
			return DirRight
		}
		return DirLeft
	}
	if ty > fy {
		return DirDown
	}
	return DirUp
}
