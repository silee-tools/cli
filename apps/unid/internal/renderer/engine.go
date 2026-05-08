package renderer

import (
	"fmt"
	"strings"

	"github.com/silee-tools/unid/internal/canvas"
	uerr "github.com/silee-tools/unid/internal/errors"
	"github.com/silee-tools/unid/internal/object"
	"github.com/silee-tools/unid/internal/width"
)

type Renderer struct {
	Canvas         *canvas.Canvas
	Collision      bool
	objects        []object.DrawObject
	GlobalOverflow object.ContentOverflow
	GlobalAlign    object.ContentAlign
}

func New(cv *canvas.Canvas, collision bool) *Renderer {
	return &Renderer{Canvas: cv, Collision: collision}
}

func (r *Renderer) Draw(obj object.DrawObject) error {
	idx := len(r.objects)
	r.objects = append(r.objects, obj)
	var err error
	switch o := obj.(type) {
	case *object.Rect:
		err = r.drawRect(o, idx)
	case *object.Text:
		err = r.drawText(o, idx)
	case *object.HLine:
		err = r.drawHLine(o, idx)
	case *object.VLine:
		err = r.drawVLine(o, idx)
	case *object.ResolvedArrow:
		err = r.drawArrow(o, idx)
	}
	if err != nil {
		return r.enrichError(err, obj, idx)
	}
	return nil
}

func (r *Renderer) DrawAll(objects []object.DrawObject) error {
	for _, obj := range objects {
		r.objects = append(r.objects, obj)
	}

	// Pass 1: Structure
	for idx, obj := range objects {
		var err error
		switch o := obj.(type) {
		case *object.Rect:
			err = r.drawRectStructure(o, idx)
		case *object.HLine:
			err = r.drawHLineStructure(o, idx)
		case *object.VLine:
			err = r.drawVLineStructure(o, idx)
		case *object.ResolvedArrow:
			err = r.drawArrowStructure(o, idx)
		case *object.Text:
			// Text is content-only
		}
		if err != nil {
			return r.enrichError(err, obj, idx)
		}
	}

	// Pass 2: Text content
	for idx, obj := range objects {
		var err error
		switch o := obj.(type) {
		case *object.Rect:
			err = r.drawRectContent(o, idx)
		case *object.Text:
			err = r.drawText(o, idx)
		case *object.HLine:
			err = r.drawHLineContent(o, idx)
		case *object.VLine:
			err = r.drawVLineContent(o, idx)
		case *object.ResolvedArrow:
			err = r.drawArrowContent(o, idx)
		}
		if err != nil {
			return r.enrichError(err, obj, idx)
		}
	}
	return nil
}

func (r *Renderer) Render() string {
	return r.Canvas.Render()
}

func (r *Renderer) enrichError(err error, _ object.DrawObject, idx int) error {
	ce, ok := err.(*uerr.CollisionError)
	if !ok {
		return err
	}

	incomingDesc := "unknown"
	if idx < len(r.objects) {
		incomingDesc = r.objects[idx].CollisionDesc()
	}
	existingDesc := "unknown"
	if ce.ExistingIdx < len(r.objects) {
		existingDesc = r.objects[ce.ExistingIdx].CollisionDesc()
	}

	oc, or2, oec, oer, ow, oh := ce.OverlapCol, ce.OverlapRow, ce.OverlapEndCol, ce.OverlapEndRow, ce.OverlapW, ce.OverlapH
	if idx < len(r.objects) && ce.ExistingIdx < len(r.objects) {
		oc, or2, oec, oer, ow, oh = r.computeOverlap(idx, ce.ExistingIdx)
	}

	return &uerr.CollisionError{
		IncomingIdx:   idx + 1,
		IncomingDesc:  incomingDesc,
		ExistingIdx:   ce.ExistingIdx + 1,
		ExistingDesc:  existingDesc,
		OverlapCol:    oc,
		OverlapRow:    or2,
		OverlapEndCol: oec,
		OverlapEndRow: oer,
		OverlapW:      ow,
		OverlapH:      oh,
	}
}

func (r *Renderer) computeOverlap(idxA, idxB int) (int, int, int, int, int, int) {
	aPosC, aPosR := r.objects[idxA].Position()
	aBndC, aBndR := r.objects[idxA].Bounds()
	bPosC, bPosR := r.objects[idxB].Position()
	bBndC, bBndR := r.objects[idxB].Bounds()

	startCol := max(aPosC, bPosC)
	startRow := max(aPosR, bPosR)
	endCol := min(aBndC, bBndC)
	endRow := min(aBndR, bBndR)

	if endCol > startCol && endRow > startRow {
		return startCol, startRow, endCol - 1, endRow - 1, endCol - startCol, endRow - startRow
	}
	return startCol, startRow, startCol, startRow, 1, 1
}

// --- Rect ---

func (r *Renderer) drawRect(rect *object.Rect, idx int) error {
	if err := r.drawRectStructure(rect, idx); err != nil {
		return err
	}
	return r.drawRectContent(rect, idx)
}

func (r *Renderer) drawRectStructure(rect *object.Rect, idx int) error {
	tl, tr, bl, br, h, v := borderChars(rect.Style)
	col, row := rect.Col, rect.Row
	iw, ih := rect.Width, rect.Height

	if err := r.Canvas.PutChar(col, row, tl, r.Collision, idx); err != nil {
		return err
	}
	for c := 1; c <= iw; c++ {
		if err := r.Canvas.PutChar(col+c, row, h, r.Collision, idx); err != nil {
			return err
		}
	}
	if err := r.Canvas.PutChar(col+iw+1, row, tr, r.Collision, idx); err != nil {
		return err
	}

	for rr := 1; rr <= ih; rr++ {
		if err := r.Canvas.PutChar(col, row+rr, v, r.Collision, idx); err != nil {
			return err
		}
		if err := r.Canvas.PutChar(col+iw+1, row+rr, v, r.Collision, idx); err != nil {
			return err
		}
	}

	if err := r.Canvas.PutChar(col, row+ih+1, bl, r.Collision, idx); err != nil {
		return err
	}
	for c := 1; c <= iw; c++ {
		if err := r.Canvas.PutChar(col+c, row+ih+1, h, r.Collision, idx); err != nil {
			return err
		}
	}
	return r.Canvas.PutChar(col+iw+1, row+ih+1, br, r.Collision, idx)
}

func (r *Renderer) drawRectContent(rect *object.Rect, idx int) error {
	col, row := rect.Col, rect.Row
	iw, ih := rect.Width, rect.Height

	if rect.HasContent {
		lines := strings.Split(rect.Content, "\n")
		lineCount := len(lines)
		overflow := rect.ContentOverflow
		align := rect.ContentAlign

		startRow := row + 1
		if lineCount <= ih {
			startRow = row + 1 + (ih-lineCount)/2
		}

		for i, line := range lines {
			rr := startRow + i
			if rr > row+ih {
				break
			}
			if err := r.renderContentLine(col, rr, line, iw, overflow, align, idx); err != nil {
				return err
			}
		}
	}

	if rect.Legend != nil {
		effectivePos := rect.Legend.Pos
		if effectivePos == object.LegendAuto {
			effectivePos = object.LegendTop
		}
		lgCol := col
		var lgRow int
		switch effectivePos {
		case object.LegendTop:
			lgRow = max(row, 1) - 1
		case object.LegendBottom:
			lgRow = row + ih + 2
		default:
			lgRow = max(row, 1) - 1
		}
		if err := r.drawLegendText(lgCol, lgRow, rect.Legend, idx); err != nil {
			return err
		}
	}
	return nil
}

func (r *Renderer) renderContentLine(col, row int, line string, innerW int, overflow object.ContentOverflow, align object.ContentAlign, idx int) error {
	contentW := width.StrWidth(line)

	var display string
	if contentW <= innerW {
		display = line
	} else {
		switch overflow {
		case object.OverflowEllipsis:
			display = ellipsisTruncate(line, contentW, innerW, align)
		case object.OverflowOverflow:
			display = line
		case object.OverflowHidden:
			display = hiddenTruncate(line, innerW, align)
		case object.OverflowError:
			return &uerr.LabelOverflowError{Label: line, LabelWidth: contentW, InnerWidth: innerW}
		}
	}

	displayW := width.StrWidth(display)
	var padLeft int
	switch align {
	case object.AlignCenter:
		padLeft = max(innerW-displayW, 0) / 2
	case object.AlignRight:
		padLeft = max(innerW-displayW, 0)
	}

	return r.Canvas.PutStr(col+1+padLeft, row, display, false, idx)
}

func (r *Renderer) drawLegendText(col, row int, legend *object.Legend, idx int) error {
	canvasW := r.Canvas.W
	canvasH := r.Canvas.H

	for i, line := range strings.Split(legend.Text, "\n") {
		targetRow := row + i
		if targetRow >= canvasH || col >= canvasW {
			break
		}

		availableW := canvasW - col
		textW := width.StrWidth(line)

		if textW <= availableW {
			if err := r.Canvas.PutStr(col, targetRow, line, false, idx); err != nil {
				return err
			}
		} else {
			var display string
			switch legend.Overflow {
			case object.OverflowEllipsis:
				display = ellipsisTruncate(line, textW, availableW, object.AlignLeft)
			case object.OverflowOverflow:
				display = line
			case object.OverflowHidden:
				display = hiddenTruncate(line, availableW, object.AlignLeft)
			case object.OverflowError:
				return fmt.Errorf("content overflow: '%s' (%d cols) exceeds available width (%d cols)", line, textW, availableW)
			}
			if err := r.Canvas.PutStr(col, targetRow, display, false, idx); err != nil {
				return err
			}
		}
	}
	return nil
}

// --- Text ---

func (r *Renderer) drawText(text *object.Text, idx int) error {
	maxW := text.BBoxWidth()
	for i, line := range strings.Split(text.Content, "\n") {
		padLeft := 0
		lineW := width.StrWidth(line)
		switch text.ContentAlign {
		case object.AlignCenter:
			padLeft = max(maxW-lineW, 0) / 2
		case object.AlignRight:
			padLeft = max(maxW-lineW, 0)
		}
		if err := r.Canvas.PutStr(text.Col+padLeft, text.Row+i, line, r.Collision, idx); err != nil {
			return err
		}
	}
	return nil
}

// --- HLine ---

func (r *Renderer) drawHLine(hl *object.HLine, idx int) error {
	if err := r.drawHLineStructure(hl, idx); err != nil {
		return err
	}
	return r.drawHLineContent(hl, idx)
}

func (r *Renderer) drawHLineStructure(hl *object.HLine, idx int) error {
	ch := hlineChar(hl.Style)
	for c := 0; c < hl.Length; c++ {
		if err := r.Canvas.PutChar(hl.Col+c, hl.Row, ch, r.Collision, idx); err != nil {
			return err
		}
	}
	return nil
}

func (r *Renderer) drawHLineContent(hl *object.HLine, idx int) error {
	if hl.Legend == nil {
		return nil
	}
	lg := hl.Legend
	effectivePos := lg.Pos
	if effectivePos == object.LegendAuto {
		effectivePos = object.LegendTop
	}

	var lgCol, lgRow int
	switch effectivePos {
	case object.LegendTop:
		lgCol, lgRow = hl.Col, max(hl.Row, 1)-1
	case object.LegendBottom:
		lgCol, lgRow = hl.Col, hl.Row+1
	case object.LegendLeft:
		tw := width.StrWidth(lg.Text)
		lgCol, lgRow = max(hl.Col-tw-1, 0), hl.Row
	case object.LegendRight:
		lgCol, lgRow = hl.Col+hl.Length+1, hl.Row
	}
	return r.drawLegendText(lgCol, lgRow, lg, idx)
}

// --- VLine ---

func (r *Renderer) drawVLine(vl *object.VLine, idx int) error {
	if err := r.drawVLineStructure(vl, idx); err != nil {
		return err
	}
	return r.drawVLineContent(vl, idx)
}

func (r *Renderer) drawVLineStructure(vl *object.VLine, idx int) error {
	ch := vlineChar(vl.Style)
	for rr := 0; rr < vl.Length; rr++ {
		if err := r.Canvas.PutChar(vl.Col, vl.Row+rr, ch, r.Collision, idx); err != nil {
			return err
		}
	}
	return nil
}

func (r *Renderer) drawVLineContent(vl *object.VLine, idx int) error {
	if vl.Legend == nil {
		return nil
	}
	lg := vl.Legend
	effectivePos := lg.Pos
	if effectivePos == object.LegendAuto {
		effectivePos = object.LegendRight
	}
	midRow := vl.Row + vl.Length/2

	var lgCol, lgRow int
	switch effectivePos {
	case object.LegendTop:
		lgCol, lgRow = vl.Col, max(vl.Row, 1)-1
	case object.LegendBottom:
		lgCol, lgRow = vl.Col, vl.Row+vl.Length
	case object.LegendLeft:
		tw := width.StrWidth(lg.Text)
		lgCol, lgRow = max(vl.Col-tw-1, 0), midRow
	case object.LegendRight:
		lgCol, lgRow = vl.Col+2, midRow
	}
	return r.drawLegendText(lgCol, lgRow, lg, idx)
}

// --- Arrow ---

func (r *Renderer) drawArrow(a *object.ResolvedArrow, idx int) error {
	if err := r.drawArrowStructure(a, idx); err != nil {
		return err
	}
	return r.drawArrowContent(a, idx)
}

func (r *Renderer) drawArrowStructure(a *object.ResolvedArrow, idx int) error {
	wp := a.Waypoints
	if len(wp) < 2 {
		return nil
	}

	for i := 0; i < len(wp)-1; i++ {
		isLast := i == len(wp)-2
		var tipChar rune
		if a.HasHead {
			tipChar = a.Head
		}
		if err := r.drawStraightSegment(wp[i][0], wp[i][1], wp[i+1][0], wp[i+1][1], isLast, tipChar, a.HasHead, idx); err != nil {
			return err
		}
	}

	for i := 1; i < len(wp)-1; i++ {
		incoming := object.SegmentDir(wp[i-1][0], wp[i-1][1], wp[i][0], wp[i][1])
		outgoing := object.SegmentDir(wp[i][0], wp[i][1], wp[i+1][0], wp[i+1][1])
		corner := object.CornerChar(incoming, outgoing)
		if err := r.Canvas.PutChar(wp[i][0], wp[i][1], corner, false, idx); err != nil {
			return err
		}
	}

	if a.Both && len(wp) >= 2 {
		dir := object.SegmentDir(wp[1][0], wp[1][1], wp[0][0], wp[0][1])
		var tip rune
		if a.HasHead {
			tip = object.ResolveArrowhead(a.Head, dir)
		} else {
			tip = object.DefaultArrowhead(dir)
		}
		if err := r.Canvas.PutChar(wp[0][0], wp[0][1], tip, false, idx); err != nil {
			return err
		}
	}
	return nil
}

func (r *Renderer) drawArrowContent(a *object.ResolvedArrow, idx int) error {
	if a.Legend != nil {
		return r.drawArrowLegend(a.Waypoints, a.Legend, idx)
	}
	return nil
}

func (r *Renderer) drawArrowLegend(wp [][2]int, legend *object.Legend, idx int) error {
	if len(wp) < 2 {
		return nil
	}

	lgCol, lgRow, _ := object.LegendPosition(wp, legend)
	return r.drawLegendText(lgCol, lgRow, legend, idx)
}

func (r *Renderer) drawStraightSegment(fc, fr, tc, tr int, withTip bool, tipChar rune, hasTip bool, idx int) error {
	if fr == tr {
		// Horizontal
		minC, maxC := fc, tc
		if fc > tc {
			minC, maxC = tc, fc
		}
		for c := minC; c <= maxC; c++ {
			if err := r.Canvas.PutChar(c, fr, '─', r.Collision, idx); err != nil {
				return err
			}
		}
		if withTip {
			dir := object.DirRight
			if tc < fc {
				dir = object.DirLeft
			}
			var tip rune
			if hasTip {
				tip = object.ResolveArrowhead(tipChar, dir)
			} else {
				tip = object.DefaultArrowhead(dir)
			}
			if err := r.Canvas.PutChar(tc, fr, tip, r.Collision, idx); err != nil {
				return err
			}
		}
	} else if fc == tc {
		// Vertical
		minR, maxR := fr, tr
		if fr > tr {
			minR, maxR = tr, fr
		}
		for row := minR; row <= maxR; row++ {
			if err := r.Canvas.PutChar(fc, row, '│', r.Collision, idx); err != nil {
				return err
			}
		}
		if withTip {
			dir := object.DirDown
			if tr < fr {
				dir = object.DirUp
			}
			var tip rune
			if hasTip {
				tip = object.ResolveArrowhead(tipChar, dir)
			} else {
				tip = object.DefaultArrowhead(dir)
			}
			if err := r.Canvas.PutChar(fc, tr, tip, r.Collision, idx); err != nil {
				return err
			}
		}
	}
	return nil
}

// --- Truncation ---

func ellipsisTruncate(content string, contentW, innerW int, align object.ContentAlign) string {
	if innerW < 2 {
		return ".."[:min(innerW, 2)]
	}

	switch align {
	case object.AlignLeft, object.AlignCenter:
		for digits := 1; digits <= 5; digits++ {
			suffixLen := 2 + digits
			if suffixLen > innerW {
				continue
			}
			prefixSpace := innerW - suffixLen
			prefix := truncateToWidth(content, prefixSpace)
			prefixW := width.StrWidth(prefix)
			truncatedW := contentW - prefixW
			nStr := fmt.Sprintf("%d", truncatedW)
			if len(nStr) == digits {
				result := prefix + ".." + nStr
				if width.StrWidth(result) <= innerW {
					return result
				}
			}
		}
	case object.AlignRight:
		for digits := 1; digits <= 5; digits++ {
			prefixLen := digits + 2
			if prefixLen > innerW {
				continue
			}
			suffixSpace := innerW - prefixLen
			suffix := truncateFromEnd(content, suffixSpace)
			suffixW := width.StrWidth(suffix)
			truncatedW := contentW - suffixW
			nStr := fmt.Sprintf("%d", truncatedW)
			if len(nStr) == digits {
				result := nStr + ".." + suffix
				if width.StrWidth(result) <= innerW {
					return result
				}
			}
		}
	}
	return ".."
}

func hiddenTruncate(content string, innerW int, align object.ContentAlign) string {
	switch align {
	case object.AlignLeft, object.AlignCenter:
		return truncateToWidth(content, innerW)
	case object.AlignRight:
		return truncateFromEnd(content, innerW)
	}
	return truncateToWidth(content, innerW)
}

func truncateToWidth(s string, maxWidth int) string {
	var b strings.Builder
	currentW := 0
	for _, ch := range s {
		w := width.CharWidth(ch)
		if currentW+w > maxWidth {
			break
		}
		b.WriteRune(ch)
		currentW += w
	}
	return b.String()
}

func truncateFromEnd(s string, maxWidth int) string {
	runes := []rune(s)
	var result []rune
	currentW := 0
	for i := len(runes) - 1; i >= 0; i-- {
		w := width.CharWidth(runes[i])
		if currentW+w > maxWidth {
			break
		}
		result = append([]rune{runes[i]}, result...)
		currentW += w
	}
	return string(result)
}

// --- Character maps ---

func borderChars(style object.BorderStyle) (tl, tr, bl, br, h, v rune) {
	switch style {
	case object.BorderHeavy:
		return '┏', '┓', '┗', '┛', '━', '┃'
	case object.BorderDouble:
		return '╔', '╗', '╚', '╝', '═', '║'
	case object.BorderRounded:
		return '╭', '╮', '╰', '╯', '─', '│'
	default:
		return '┌', '┐', '└', '┘', '─', '│'
	}
}

func hlineChar(style object.LineStyle) rune {
	switch style {
	case object.LineHeavy:
		return '━'
	case object.LineDouble:
		return '═'
	case object.LineDash:
		return '╌'
	default:
		return '─'
	}
}

func vlineChar(style object.LineStyle) rune {
	switch style {
	case object.LineHeavy:
		return '┃'
	case object.LineDouble:
		return '║'
	case object.LineDash:
		return '╎'
	default:
		return '│'
	}
}
