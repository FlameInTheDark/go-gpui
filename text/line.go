package text

import (
	"sort"

	"github.com/FlameInTheDark/go-gpui/geometry"
)

// visualGlyph is one glyph in visual (left-to-right) order.
type visualGlyph struct {
	g    ShapedGlyph
	font FontID
}

// visualOrder collects every glyph of the line sorted by x (the visual
// order; glyph positions are baked by the shaper's bidi pass).
func (l *LineLayout) visualOrder() []visualGlyph {
	var out []visualGlyph
	for ri := range l.Runs {
		for _, g := range l.Runs[ri].Glyphs {
			out = append(out, visualGlyph{g: g, font: l.Runs[ri].FontID})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].g.Position.X.AsF32() < out[j].g.Position.X.AsF32()
	})
	return out
}

// glyphRTL reports the direction of the glyph at byte index ix (via the
// recorded bidi segments; LTR when unrecorded).
func (l *LineLayout) glyphRTL(ix int) bool {
	for _, sg := range l.BidiSegments {
		if ix >= sg.Start && ix < sg.End {
			return sg.RTL
		}
	}
	return false
}

// caretBoundary is one caret slot: a visual x and the logical byte index
// the caret takes there (upstream line_layout's index↔pixel maps are
// bidi-aware; this port derives them from the visual glyph walk).
type caretBoundary struct {
	x     float32
	index int
}

// caretBoundaries builds the caret slot list in visual order. For pure
// LTR lines the slots are exactly the glyph left edges (index = glyph
// index) plus the line end. For reordered lines an RTL glyph's left edge
// carries the index AFTER its cluster (the caret before the cluster sits
// at its right edge), and the line end carries 0 for RTL base direction.
func (l *LineLayout) caretBoundaries() []caretBoundary {
	vis := l.visualOrder()
	if len(vis) == 0 {
		return []caretBoundary{{x: 0, index: 0}}
	}
	// nextLogical[ix]: the next glyph start strictly after byte ix.
	logical := make([]int, 0, len(vis))
	for _, v := range vis {
		logical = append(logical, v.g.Index)
	}
	sort.Ints(logical)
	nextLogical := func(ix int) int {
		for _, li := range logical {
			if li > ix {
				return li
			}
		}
		return l.Len
	}

	bs := make([]caretBoundary, 0, len(vis)+1)
	for _, v := range vis {
		if l.BidiSegments != nil && l.glyphRTL(v.g.Index) {
			bs = append(bs, caretBoundary{x: v.g.Position.X.AsF32(), index: nextLogical(v.g.Index)})
		} else {
			bs = append(bs, caretBoundary{x: v.g.Position.X.AsF32(), index: v.g.Index})
		}
	}
	// The line-end slot: after the last logical character for LTR base,
	// before the first for RTL base.
	if l.BidiBaseRTL {
		bs = append(bs, caretBoundary{x: l.Width.AsF32(), index: 0})
	} else {
		bs = append(bs, caretBoundary{x: l.Width.AsF32(), index: l.Len})
	}
	return bs
}

// IndexForX returns the byte offset of the glyph at or before x; ok=false
// when x is past the end (upstream index_for_x). Bidi-aware: the visual
// boundary walk replaces the logical-order scan on reordered lines.
func (l *LineLayout) IndexForX(x geometry.Pixels) (int, bool) {
	if x.AsF32() >= l.Width.AsF32() {
		return 0, false
	}
	if l.BidiSegments == nil {
		for ri := len(l.Runs) - 1; ri >= 0; ri-- {
			run := l.Runs[ri]
			for gi := len(run.Glyphs) - 1; gi >= 0; gi-- {
				g := run.Glyphs[gi]
				if g.Position.X.Cmp(x) <= 0 {
					return g.Index, true
				}
			}
		}
		return 0, true
	}
	result := 0
	for _, b := range l.caretBoundaries() {
		if b.x <= x.AsF32() {
			result = b.index
		} else {
			break
		}
	}
	return result, true
}

// ClosestIndexForX returns the nearest byte offset to x (upstream
// closest_index_for_x). Bidi-aware: nearest caret slot in visual space.
func (l *LineLayout) ClosestIndexForX(x geometry.Pixels) int {
	if l.BidiSegments == nil {
		prevIndex, prevX := 0, geometry.Px(0)
		for _, run := range l.Runs {
			for _, g := range run.Glyphs {
				if g.Position.X.Cmp(x) >= 0 {
					// Nearer of (prev, g).
					if x.AsF32()-prevX.AsF32() <= g.Position.X.AsF32()-x.AsF32() {
						return prevIndex
					}
					return g.Index
				}
				prevIndex, prevX = g.Index, g.Position.X
			}
		}
		if l.Len == 1 {
			if x.AsF32() > l.Width.AsF32()/2 {
				return 1
			}
			return 0
		}
		return l.Len
	}
	bs := l.caretBoundaries()
	best := bs[len(bs)-1].index
	bestDist := absF32(x.AsF32() - bs[len(bs)-1].x)
	for i := len(bs) - 2; i >= 0; i-- {
		d := absF32(x.AsF32() - bs[i].x)
		if d <= bestDist { // ties prefer the left slot (upstream tie behavior)
			best = bs[i].index
			bestDist = d
		}
	}
	return best
}

func absF32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// XForIndex returns the x of the first glyph at or after the byte index
// (upstream x_for_index). Bidi-aware: the visually-leftmost slot carrying
// the index, else the leftmost glyph at or after it.
func (l *LineLayout) XForIndex(index int) geometry.Pixels {
	if l.BidiSegments == nil {
		for _, run := range l.Runs {
			for _, g := range run.Glyphs {
				if g.Index >= index {
					return g.Position.X
				}
			}
		}
		return l.Width
	}
	for _, b := range l.caretBoundaries() {
		if b.index == index {
			return geometry.Px(b.x)
		}
	}
	// No exact slot (mid-cluster): the leftmost glyph at or after index.
	best := l.Width.AsF32()
	for _, v := range l.visualOrder() {
		if v.g.Index >= index && v.g.Position.X.AsF32() < best {
			best = v.g.Position.X.AsF32()
		}
	}
	return geometry.Px(best)
}

// FontIDForIndex returns the font of the first glyph at or after index.
func (l *LineLayout) FontIDForIndex(index int) (FontID, bool) {
	for _, run := range l.Runs {
		for _, g := range run.Glyphs {
			if g.Index >= index {
				return run.FontID, true
			}
		}
	}
	return 0, false
}

// SplitAt splits the layout at a byte index, shifting positions and
// rebasing indices in the right half (upstream LineLayout::split_at).
func (l *LineLayout) SplitAt(byteIndex int) (LineLayout, LineLayout) {
	xOffset := l.XForIndex(byteIndex)
	var left, right LineLayout
	left.FontSize, right.FontSize = l.FontSize, l.FontSize
	left.Ascent, left.Descent = l.Ascent, l.Descent
	right.Ascent, right.Descent = l.Ascent, l.Descent
	left.Width = xOffset
	right.Width = geometry.Px(l.Width.AsF32() - xOffset.AsF32())
	left.Len = byteIndex
	right.Len = l.Len - byteIndex
	for _, run := range l.Runs {
		var lrun, rrun ShapedRun
		lrun.FontID, rrun.FontID = run.FontID, run.FontID
		for _, g := range run.Glyphs {
			if g.Index < byteIndex {
				lrun.Glyphs = append(lrun.Glyphs, g)
			} else {
				ng := g
				ng.Position.X = geometry.Px(g.Position.X.AsF32() - xOffset.AsF32())
				ng.Index -= byteIndex
				rrun.Glyphs = append(rrun.Glyphs, ng)
			}
		}
		if len(lrun.Glyphs) > 0 {
			left.Runs = append(left.Runs, lrun)
		}
		if len(rrun.Glyphs) > 0 {
			right.Runs = append(right.Runs, rrun)
		}
	}
	return left, right
}

// WrapBoundary marks where a wrapped line begins (upstream WrapBoundary).
type WrapBoundary struct {
	RunIx   int
	GlyphIx int
}

// WrappedLineLayout is a line plus its wrap boundaries (upstream
// WrappedLineLayout).
type WrappedLineLayout struct {
	Unwrapped      *LineLayout
	WrapBoundaries []WrapBoundary
	WrapWidth      *geometry.Pixels
	// Indent is the pixel width reserved at the start of every continuation
	// row (upstream IndentAdjustment::SameIndent): continuation rows align
	// under the first row's leading whitespace.
	Indent float32
}

// RowRanges returns the [start, end) byte range of each wrapped row (the
// first row starts at 0; each boundary starts the next row). The final
// range ends at the text length.
func (w *WrappedLineLayout) RowRanges(text string) [][2]int {
	if w == nil || w.Unwrapped == nil {
		return [][2]int{{0, len(text)}}
	}
	var ranges [][2]int
	start := 0
	for _, b := range w.WrapBoundaries {
		// The boundary's byte index comes from the glyph at (RunIx, GlyphIx).
		if b.RunIx < len(w.Unwrapped.Runs) {
			run := w.Unwrapped.Runs[b.RunIx]
			if b.GlyphIx < len(run.Glyphs) {
				ix := run.Glyphs[b.GlyphIx].Index
				if ix > start {
					ranges = append(ranges, [2]int{start, ix})
					start = ix
				}
			}
		}
	}
	ranges = append(ranges, [2]int{start, len(text)})
	return ranges
}

// Len returns the unwrapped length.
func (w *WrappedLineLayout) Len() int { return w.Unwrapped.Len }

// Width is min(wrapWidth, unwrapped width).
func (w *WrappedLineLayout) Width() geometry.Pixels {
	if w.WrapWidth == nil || w.Unwrapped.Width.Cmp(*w.WrapWidth) < 0 {
		return w.Unwrapped.Width
	}
	return *w.WrapWidth
}

// Size returns the wrapped size at a line height.
func (w *WrappedLineLayout) Size(lineHeight geometry.Pixels) geometry.Size[geometry.Pixels] {
	return geometry.Size[geometry.Pixels]{
		Width:  w.Width(),
		Height: geometry.Px(lineHeight.AsF32() * float32(len(w.WrapBoundaries)+1)),
	}
}

// IndexForPosition maps a point to a byte index; err returns the clamped
// boundary index (upstream index_for_position).
func (w *WrappedLineLayout) IndexForPosition(pos geometry.Point[geometry.Pixels], lineHeight geometry.Pixels) (int, int, bool) {
	wrappedIx := int(pos.Y.AsF32() / lineHeight.AsF32())
	if wrappedIx < 0 {
		wrappedIx = 0
	}
	if wrappedIx > len(w.WrapBoundaries) {
		wrappedIx = len(w.WrapBoundaries)
	}
	rowStartX, rowStartIndex := geometry.Px(0), 0
	if wrappedIx > 0 {
		b := w.WrapBoundaries[wrappedIx-1]
		g := glyphAt(w.Unwrapped, b.RunIx, b.GlyphIx)
		rowStartX = g.Position.X
		rowStartIndex = g.Index
	}
	var rowEndX = w.Unwrapped.Width
	var rowEndIndex = w.Unwrapped.Len
	if wrappedIx < len(w.WrapBoundaries) {
		b := w.WrapBoundaries[wrappedIx]
		g := glyphAt(w.Unwrapped, b.RunIx, b.GlyphIx)
		rowEndX = g.Position.X
		rowEndIndex = g.Index
	}
	x := geometry.Px(pos.X.AsF32() + rowStartX.AsF32())
	if x.Cmp(rowStartX) < 0 {
		return rowStartIndex, rowStartIndex, false
	}
	if x.Cmp(rowEndX) >= 0 {
		return rowEndIndex, rowEndIndex, false
	}
	if idx, ok := w.Unwrapped.IndexForX(x); ok {
		return idx, idx, true
	}
	return rowEndIndex, rowEndIndex, false
}

// PositionForIndex maps a byte index to a point.
func (w *WrappedLineLayout) PositionForIndex(index int, lineHeight geometry.Pixels) (geometry.Point[geometry.Pixels], bool) {
	row := 0
	lineStartIndex := 0
	lineStartX := geometry.Px(0)
	// Walk rows.
	for bi, b := range w.WrapBoundaries {
		g := glyphAt(w.Unwrapped, b.RunIx, b.GlyphIx)
		if g.Index > index {
			row = bi
			break
		}
		row = bi + 1
	}
	if row > 0 {
		b := w.WrapBoundaries[row-1]
		g := glyphAt(w.Unwrapped, b.RunIx, b.GlyphIx)
		lineStartIndex = g.Index
		lineStartX = g.Position.X
	}
	var rowEndIndex = w.Unwrapped.Len
	if row < len(w.WrapBoundaries) {
		b := w.WrapBoundaries[row]
		g := glyphAt(w.Unwrapped, b.RunIx, b.GlyphIx)
		rowEndIndex = g.Index
	}
	if index < lineStartIndex || index > rowEndIndex {
		return geometry.Point[geometry.Pixels]{}, false
	}
	x := w.Unwrapped.XForIndex(index).AsF32() - lineStartX.AsF32()
	return geometry.Point[geometry.Pixels]{
		X: geometry.Px(x),
		Y: geometry.Px(float32(row) * lineHeight.AsF32()),
	}, true
}

func glyphAt(l *LineLayout, runIx, glyphIx int) ShapedGlyph {
	if runIx < len(l.Runs) && glyphIx < len(l.Runs[runIx].Glyphs) {
		return l.Runs[runIx].Glyphs[glyphIx]
	}
	return ShapedGlyph{}
}

// ComputeWrapBoundaries is the exact port of upstream
// LineLayoutCache::compute_wrap_boundaries: walk glyph indices (deduped by
// cluster), track break-opportunity candidates, and wrap when a row
// exceeds the width.
func (w *WrappedLineLayout) ComputeWrapBoundaries(text string, wrapWidth geometry.Pixels, maxLines int) []WrapBoundary {
	l := w.Unwrapped
	breaks := BreakOpportunities(text)
	breakSet := map[int]bool{}
	for _, b := range breaks {
		breakSet[b] = true
	}

	// Glyph start indices.
	glyphIndices := map[int]bool{}
	for _, run := range l.Runs {
		for _, g := range run.Glyphs {
			glyphIndices[g.Index] = true
		}
	}

	type gpos struct {
		b  WrapBoundary
		ix int
		x  float32
	}
	var glyphs []gpos
	for ri, run := range l.Runs {
		for gi, g := range run.Glyphs {
			if len(glyphs) > 0 && glyphs[len(glyphs)-1].ix == g.Index {
				continue // multi-glyph cluster
			}
			glyphs = append(glyphs, gpos{
				b:  WrapBoundary{RunIx: ri, GlyphIx: gi},
				ix: g.Index,
				x:  g.Position.X.AsF32(),
			})
		}
	}

	var boundaries []WrapBoundary
	lastBoundary := WrapBoundary{RunIx: 0, GlyphIx: 0}
	lastBoundaryX := float32(0)

	// Indent adjustment (upstream IndentAdjustment::SameIndent): the width
	// of the leading whitespace carries over to continuation rows, which
	// align under it. Capped at half the wrap width so pathological
	// indents cannot squeeze the continuation rows to nothing.
	indent := float32(0)
	for _, g := range glyphs {
		if g.ix >= len(text) || (text[g.ix] != ' ' && text[g.ix] != '\t') {
			break
		}
		var nextX float32
		for _, ng := range glyphs {
			if ng.ix > g.ix {
				nextX = ng.x
				break
			}
		}
		if nextX > g.x {
			indent = nextX
		} else {
			indent = g.x
		}
		if indent > wrapWidth.AsF32()/2 {
			indent = wrapWidth.AsF32() / 2
			break
		}
	}
	w.Indent = indent
	firstRow := true

	currentSegment := -1
	var lastCandidate *gpos
	skipNewline := func(ix int) bool {
		return ix < len(text) && (text[ix] == '\n' || text[ix] == '\r')
	}

	for i, g := range glyphs {
		if skipNewline(g.ix) {
			continue
		}
		isSpace := g.ix < len(text) && text[g.ix] == ' '
		if !isSpace {
			// Segment containing this glyph.
			seg := -1
			for si, b := range breaks {
				if g.ix < b {
					seg = si
					break
				}
			}
			if seg == -1 {
				seg = len(breaks)
			}
			if currentSegment != seg {
				if currentSegment != -1 {
					// Every segment start is a break
					// opportunity; keep the most recent
					// one (upstream wraps at the last
					// candidate before the overflow).
					lastCandidate = &glyphs[i]
				}
				currentSegment = seg
			}
		}
		var nextX float32
		if i+1 < len(glyphs) {
			nextX = glyphs[i+1].x
		} else {
			nextX = l.Width.AsF32()
		}
		rowStart := float32(0)
		if !firstRow {
			rowStart = indent
		}
		rowWidth := rowStart + (nextX - lastBoundaryX)
		// Epsilon guard: a text sized by a fit-content ancestor gets an
		// available width equal to its own shaped width; scale-dependent
		// rounding on the way through the layout can push rowWidth a
		// fraction of a pixel past wrapWidth, wrapping the last glyph
		// onto a phantom second row (3-digit line numbers broke after
		// 99 on Windows 125% displays). Only a real excess wraps.
		if rowWidth > wrapWidth.AsF32()+0.01 {
			if maxLines > 0 && len(boundaries) >= maxLines-1 {
				break
			}
			if lastCandidate != nil && lastCandidate.b != lastBoundary {
				boundaries = append(boundaries, lastCandidate.b)
				lastBoundary = lastCandidate.b
				lastBoundaryX = lastCandidate.x
			} else {
				boundaries = append(boundaries, g.b)
				lastBoundary = g.b
				lastBoundaryX = g.x
			}
			lastCandidate = nil
			firstRow = false
		}
	}
	return boundaries
}
