package layout

import (
	"fmt"
	"math"
)

// This file ports taffy 0.15's CSS Grid implementation
// (src/compute/grid/{mod,placement,explicit_grid,track_sizing,alignment}.rs)
// for the common-correctness subset:
//
//   - Track sizing: explicit tracks (px, %, fr), auto tracks, minmax(min, max)
//     with a fit-content(arg) approximation, repeat(n, tracks) and
//     repeat(auto-fill / auto-fit, tracks) with fixed-size track lists,
//     implicit tracks (grid-auto-rows/columns, cycled like taffy), row/column
//     gaps, and spanning items (span n) distributing space across tracks.
//   - Item placement: definite placement via grid_row/grid_column (lines and
//     spans, including negative lines), sparse row-major (and column-major,
//     dense variants) auto-placement with the placement cursor from
//     placement.rs.
//   - Alignment: justify/align-self per item (start/center/end/stretch,
//     default stretch) via justify/align-items + self overrides, and
//     justify/align-content distribution of the tracks
//     (start/center/end/stretch/space-between/space-around/space-evenly).
//   - Container sizing: intrinsic min/max content sizes from the track
//     template, definite-size stretch, and fr distribution per taffy's
//     find_size_of_fr (leftover space after non-flexible tracks; respects
//     track base sizes via the restart loop).
//
// NOT implemented (documented honestly):
//   - subgrid, masonry, named grid lines/areas, baseline alignment (falls
//     back to start), fit-content() percentage arguments, indefinite-space
//     auto-fill (repeats once when the container axis is indefinite, like
//     taffy), the percentage/intrinsic-contribution re-sizing re-passes in
//     mod.rs (single sizing pass), auto margins on grid items, scroll
//     containers' automatic minimum size quirks, and float layout (upstream
//     taffy supports float only minimally; skipped deliberately to keep grid
//     correct — see PORTING_STATUS.md).

const gridMaxTracks = 128

var gridDebug = false

// f32inf is +infinity as float32; isInfF32 tests for positive infinity.
func f32inf() float32         { return float32(math.Inf(1)) }
func isInfF32(v float32) bool { return math.IsInf(float64(v), 1) }

// ---------------------------------------------------------------------------
// Placement types
// ---------------------------------------------------------------------------

// ozPlacement is a GridPlacement resolved into origin-zero coordinates (the
// line at the left/top edge of the explicit grid is 0; negative lines are
// implicit tracks before it).
type ozPlacement struct {
	auto  bool
	span  bool
	line  int // valid when !auto && !span
	spanN int
}

func ozPlacementOf(p GridPlacement, explicitTrackCount int) ozPlacement {
	switch {
	case p.IsLine():
		l := int(p.Line)
		var oz int
		if l > 0 {
			oz = l - 1
		} else {
			// Negative lines count from the end of the explicit grid.
			oz = l + explicitTrackCount + 1
		}
		return ozPlacement{line: oz}
	case p.IsSpan():
		s := int(p.Span)
		if s < 1 {
			s = 1
		}
		if s > gridMaxTracks {
			s = gridMaxTracks
		}
		return ozPlacement{span: true, spanN: s}
	default:
		return ozPlacement{auto: true}
	}
}

// ozPairDefinite reports whether the pair resolves to a definite track range
// (at least one end is a line).
func ozPairDefinite(start, end ozPlacement) bool {
	return (!start.auto && !start.span) || (!end.auto && !end.span)
}

// resolveOZDefinite resolves a definite pair into a [start, end) track range
// (taffy Line<OriginZeroGridPlacement>::resolve_definite_grid_lines).
func resolveOZDefinite(start, end ozPlacement) [2]int {
	lineOf := func(p ozPlacement) int { return p.line }
	switch {
	case !start.auto && !start.span && !end.auto && !end.span:
		a, b := lineOf(start), lineOf(end)
		if a == b {
			return [2]int{a, a + 1}
		}
		if a > b {
			a, b = b, a
		}
		return [2]int{a, b}
	case !start.auto && !start.span && end.span:
		return [2]int{lineOf(start), lineOf(start) + end.spanN}
	case !start.auto && !start.span:
		return [2]int{lineOf(start), lineOf(start) + 1}
	case start.span && !end.auto && !end.span:
		return [2]int{lineOf(end) - start.spanN, lineOf(end)}
	default: // (auto, line)
		return [2]int{lineOf(end) - 1, lineOf(end)}
	}
}

// ozIndefiniteSpan resolves the span of an indefinite placement.
func ozIndefiniteSpan(start, end ozPlacement) int {
	if start.span {
		return start.spanN
	}
	if end.span {
		return end.spanN
	}
	return 1
}

func resolveOZIndefinite(start, span int) [2]int { return [2]int{start, start + span} }

// ---------------------------------------------------------------------------
// Occupancy matrix
// ---------------------------------------------------------------------------

type gridCellState uint8

const (
	cellDefinitelyPlaced gridCellState = 1
	cellAutoPlaced       gridCellState = 2
)

type gridTrackCounts struct {
	neg, expl, pos int
}

func (c gridTrackCounts) len() int { return c.neg + c.expl + c.pos }
func (c gridTrackCounts) startLine() int {
	return -c.neg
}
func (c gridTrackCounts) endLine() int { return c.neg + c.expl + c.pos }

// gridMatrix tracks cell occupancy and the implicit grid extent (compact
// stand-in for taffy's CellOccupancyMatrix).
type gridMatrix struct {
	cells map[[2]int]gridCellState // (row, col) oz lines
	cols  gridTrackCounts
	rows  gridTrackCounts
	// autoCursor per secondary-axis oz start line: next free primary line
	// (advanced by auto-placed items, per taffy's auto_placement_cursor).
	autoCursor map[int]int
}

func newGridMatrix(colExpl, rowExpl int) *gridMatrix {
	return &gridMatrix{
		cells:      map[[2]int]gridCellState{},
		cols:       gridTrackCounts{expl: colExpl},
		rows:       gridTrackCounts{expl: rowExpl},
		autoCursor: map[int]int{},
	}
}

// markArea records an item's [rowStart,rowEnd) x [colStart,colEnd) area and
// grows the implicit grid to fit.
func (m *gridMatrix) markArea(rowSpan, colSpan [2]int, state gridCellState, primaryIsRow bool) {
	for r := rowSpan[0]; r < rowSpan[1]; r++ {
		for c := colSpan[0]; c < colSpan[1]; c++ {
			key := [2]int{r, c}
			if old, ok := m.cells[key]; ok && old == cellDefinitelyPlaced {
				continue // never overwrite definitely-placed cells
			}
			m.cells[key] = state
		}
	}
	if rowSpan[0] < -m.rows.neg {
		m.rows.neg = -rowSpan[0]
	}
	if rowSpan[1] > m.rows.expl+m.rows.pos {
		m.rows.pos = rowSpan[1] - m.rows.expl
	}
	if colSpan[0] < -m.cols.neg {
		m.cols.neg = -colSpan[0]
	}
	if colSpan[1] > m.cols.expl+m.cols.pos {
		m.cols.pos = colSpan[1] - m.cols.expl
	}
	if state == cellAutoPlaced && rowSpan[0] < rowSpan[1] && colSpan[0] < colSpan[1] {
		// Advance the sparse cursor on the secondary-axis track.
		if primaryIsRow {
			key := colSpan[0]
			if cur, ok := m.autoCursor[key]; !ok || rowSpan[1] > cur {
				m.autoCursor[key] = rowSpan[1]
			}
		} else {
			key := rowSpan[0]
			if cur, ok := m.autoCursor[key]; !ok || colSpan[1] > cur {
				m.autoCursor[key] = colSpan[1]
			}
		}
	}
}

// primaryCollisionJump scans the area spanned by the item; if any cell is
// occupied it returns the primary line just past the collision. primary is a
// line range in the primary axis (rows for column flow, columns for row
// flow), secondary in the other axis.
func (m *gridMatrix) primaryCollisionJump(primaryIsRow bool, primary, secondary [2]int) (int, bool) {
	jump := -1 << 30
	found := false
	check := func(row, col int) {
		if _, ok := m.cells[[2]int{row, col}]; ok {
			found = true
			p := col
			if primaryIsRow {
				p = row
			}
			if p+1 > jump {
				jump = p + 1
			}
		}
	}
	if primaryIsRow {
		// Primary axis = rows; secondary = columns.
		for c := secondary[0]; c < secondary[1]; c++ {
			for r := primary[0]; r < primary[1]; r++ {
				check(r, c)
			}
		}
	} else {
		// Primary axis = columns; secondary = rows.
		for r := secondary[0]; r < secondary[1]; r++ {
			for c := primary[0]; c < primary[1]; c++ {
				check(r, c)
			}
		}
	}
	return jump, found
}

// occupiedTrackJump finds the last occupied track in the secondary span (all
// primary tracks) and returns the secondary line past it.
func (m *gridMatrix) occupiedTrackJump(primaryIsRow bool, secondary [2]int) (int, bool) {
	for s := secondary[1] - 1; s >= secondary[0]; s-- {
		if primaryIsRow {
			// Secondary axis = columns.
			for r := m.rows.startLine(); r < m.rows.endLine(); r++ {
				if _, ok := m.cells[[2]int{r, s}]; ok {
					return s + 1, true
				}
			}
		} else {
			// Secondary axis = rows.
			for c := m.cols.startLine(); c < m.cols.endLine(); c++ {
				if _, ok := m.cells[[2]int{s, c}]; ok {
					return s + 1, true
				}
			}
		}
	}
	return 0, false
}

// colIsOccupied reports whether the column (oz line) holds any item.
func (m *gridMatrix) colIsOccupied(colOZ int) bool {
	for r := m.rows.startLine(); r < m.rows.endLine(); r++ {
		if _, ok := m.cells[[2]int{r, colOZ}]; ok {
			return true
		}
	}
	return false
}

// rowIsOccupied reports whether the row (oz line) holds any item.
func (m *gridMatrix) rowIsOccupied(rowOZ int) bool {
	for c := m.cols.startLine(); c < m.cols.endLine(); c++ {
		if _, ok := m.cells[[2]int{rowOZ, c}]; ok {
			return true
		}
	}
	return false
}

// secondaryCursor returns the sparse auto-placement cursor for a definite
// secondary-axis start line (taffy auto_placement_cursor).
func (m *gridMatrix) secondaryCursor(primaryIsRow bool, secStart int) int {
	var start int
	if primaryIsRow {
		start = m.rows.startLine()
	} else {
		start = m.cols.startLine()
	}
	cur, ok := m.autoCursor[secStart]
	if !ok || cur < start {
		return start
	}
	return cur
}

// ---------------------------------------------------------------------------
// Items
// ---------------------------------------------------------------------------

type gridItem struct {
	node   Node
	style  *Style
	margin [4]float32
	col    [2]int // resolved [start, end) oz lines
	row    [2]int
	placed bool

	crossesFlexCol, crossesFlexRow bool
	crossesIntrCol, crossesIntrRow bool
	mcW, maxW, mcH, maxH           *float32 // measured contributions
}

func (it *gridItem) span(isWidth bool) int {
	if isWidth {
		return it.col[1] - it.col[0]
	}
	return it.row[1] - it.row[0]
}

func (it *gridItem) rangeOZ(isWidth bool) [2]int {
	if isWidth {
		return it.col
	}
	return it.row
}

// ---------------------------------------------------------------------------
// Grid state (container-level)
// ---------------------------------------------------------------------------

type gridState struct {
	colTracks, rowTracks []gridTrack
	colGap, rowGap       float32
	colNeg, rowNeg       int // negative implicit track counts
	items                []*gridItem
	// sizingRows: true while the row (block) pass runs; the other-axis track
	// estimate switches from max-fn-definite (column pass) to base size.
	sizingRows bool
	innerW     float32 // definite inner content size or 0
	innerWDef  bool
	innerH     float32
	innerHDef  bool
}

// indexRange converts the item's oz placement into track vector indexes.
func (g *gridState) indexRange(it *gridItem, isWidth bool) [2]int {
	oz := it.rangeOZ(isWidth)
	neg := g.colNeg
	if !isWidth {
		neg = g.rowNeg
	}
	return [2]int{oz[0] + neg, oz[1] + neg}
}

// resolveTrackFnDefinite resolves a track function with a definite value
// (px always; percent when the basis is definite).
func resolveTrackFnDefinite(f GridTrackFn, basis float32, basisDef bool) (float32, bool) {
	switch f.Kind {
	case GridTrackFnPx:
		return f.Value, true
	case GridTrackFnPercent:
		if basisDef {
			return f.Value * basis, true
		}
		return 0, false
	}
	return 0, false
}

func trackFnIsFixed(f GridTrackFn) bool {
	return f.Kind == GridTrackFnPx || f.Kind == GridTrackFnPercent
}

func trackFnIsIntrinsic(f GridTrackFn) bool {
	return f.Kind == GridTrackFnAuto || f.Kind == GridTrackFnMinContent || f.Kind == GridTrackFnMaxContent
}

func trackIsFlexible(tr *gridTrack) bool { return tr.maxFn.Kind == GridTrackFnFr }

// ---------------------------------------------------------------------------
// Explicit grid size (explicit_grid.rs)
// ---------------------------------------------------------------------------

// computeExplicitGridSize returns (auto-repetition count, template track
// count) for one axis, porting compute_explicit_grid_size_in_axis. When
// underflowStrategy is set (only a min-size basis), the repetition count is
// the minimum that overflows the size; otherwise the maximum that fits.
func computeExplicitGridSize(sections []GridSection, size float32, sizeDef bool, underflowStrategy bool, gap float32) (int, int) {
	if len(sections) == 0 {
		return 0, 0
	}
	trackCount := 0
	autoRepCount := 0
	allFixed := true
	for _, s := range sections {
		switch s.Repeat {
		case GridRepeatNone:
			trackCount += len(s.Tracks)
		case GridRepeatCount:
			trackCount += int(s.Count) * len(s.Tracks)
		default:
			autoRepCount++
			if len(s.Tracks) == 0 {
				return 0, 0
			}
		}
		for _, tr := range s.Tracks {
			if !trackFnIsFixed(tr.Min) && !trackFnIsFixed(tr.Max) {
				allFixed = false
			}
		}
	}
	if autoRepCount > 1 || (autoRepCount == 1 && !allFixed) {
		// Invalid template: disregard entirely (taffy returns zero tracks).
		return 0, 0
	}
	if autoRepCount == 0 {
		return 0, trackCount
	}

	// Fixed value of a sizing function for repetition counting: the max fn if
	// definite (floored by the min fn), else the min fn, else 0.
	defValue := func(tr GridTrack) float32 {
		if v, ok := resolveTrackFnDefinite(tr.Max, size, sizeDef); ok {
			if m, ok2 := resolveTrackFnDefinite(tr.Min, size, sizeDef); ok2 && m > v {
				return m
			}
			return v
		}
		if v, ok := resolveTrackFnDefinite(tr.Min, size, sizeDef); ok {
			return v
		}
		return 0
	}

	var autoSection GridSection
	nonAutoTracks := 0
	nonAutoUsed := float32(0)
	for _, s := range sections {
		switch s.Repeat {
		case GridRepeatNone:
			nonAutoTracks += len(s.Tracks)
			for _, tr := range s.Tracks {
				nonAutoUsed += defValue(tr)
			}
		case GridRepeatCount:
			nonAutoTracks += int(s.Count) * len(s.Tracks)
			sum := float32(0)
			for _, tr := range s.Tracks {
				sum += defValue(tr)
			}
			nonAutoUsed += sum * float32(s.Count)
		default:
			autoSection = s
		}
	}
	repTrackCount := len(autoSection.Tracks)
	perRepUsed := float32(0)
	for _, tr := range autoSection.Tracks {
		perRepUsed += defValue(tr)
	}

	numReps := 1
	if sizeDef {
		first := nonAutoUsed + perRepUsed + float32(nonAutoTracks+repTrackCount-1)*gap
		if first > size {
			numReps = 1
		} else {
			perRep := perRepUsed + float32(repTrackCount)*gap
			fit := (size - first) / perRep
			if underflowStrategy {
				numReps = int(math.Ceil(float64(fit))) + 1
			} else {
				numReps = int(math.Floor(float64(fit))) + 1
			}
		}
	}
	if numReps < 1 {
		numReps = 1
	}
	if numReps > gridMaxTracks {
		numReps = gridMaxTracks
	}
	total := nonAutoTracks + repTrackCount*numReps
	if total > gridMaxTracks {
		total = gridMaxTracks
	}
	return numReps, total
}

// expandTemplate expands a track-list template into per-track sizing
// functions, recording which tracks come from auto-fit repeats.
func expandTemplate(sections []GridSection, autoRepCount int) (fns []GridTrack, fromAutoFit []bool) {
	for _, s := range sections {
		switch s.Repeat {
		case GridRepeatNone:
			for _, tr := range s.Tracks {
				fns = append(fns, tr)
				fromAutoFit = append(fromAutoFit, false)
			}
		case GridRepeatCount:
			for i := 0; i < int(s.Count); i++ {
				for _, tr := range s.Tracks {
					fns = append(fns, tr)
					fromAutoFit = append(fromAutoFit, false)
				}
			}
		default:
			for i := 0; i < autoRepCount; i++ {
				for _, tr := range s.Tracks {
					fns = append(fns, tr)
					fromAutoFit = append(fromAutoFit, s.Repeat == GridRepeatAutoFit)
				}
			}
		}
	}
	return fns, fromAutoFit
}

// autoTrackFn picks the implicit track function for a track index (cycled).
func autoTrackFn(auto []GridTrack, index int) GridTrack {
	if len(auto) == 0 {
		return AutoGridTrack()
	}
	return auto[index%len(auto)]
}

// ---------------------------------------------------------------------------
// Track lists
// ---------------------------------------------------------------------------

type gridTrack struct {
	minFn, maxFn                     GridTrackFn
	base, growth                     float32
	fitLimit                         float32 // resolved fit-content() argument (inf when absent)
	collapsed                        bool    // auto-fit track without items
	offset                           float32 // final start offset (content-box relative)
	itemInc                          float32 // in-flight distributed increase
	plannedBaseInc, plannedGrowthInc float32
	infinitelyGrowable               bool
}

// initializeGridTracks builds the track list (negative implicit, explicit,
// positive implicit); sizing functions come from the expanded template and
// the cycled implicit track functions.
func (g *gridState) initializeGridTracks(counts gridTrackCounts, sections []GridSection, autoRepCount int, autoTracks []GridTrack, hasItems func(int) bool) []gridTrack {
	template, fromAutoFit := expandTemplate(sections, autoRepCount)
	tracks := make([]gridTrack, 0, counts.len())

	newTrack := func(fn GridTrack) gridTrack {
		return gridTrack{minFn: fn.Min, maxFn: fn.Max, growth: f32inf(), fitLimit: f32inf()}
	}

	// Negative implicit tracks.
	for i := 0; i < counts.neg; i++ {
		tracks = append(tracks, newTrack(autoTrackFn(autoTracks, i)))
	}
	// Explicit tracks (truncate to counts.expl for overlarge grids).
	for i := 0; i < counts.expl; i++ {
		tr := newTrack(autoTrackFn(autoTracks, 0))
		if i < len(template) {
			tr = newTrack(template[i])
			if fromAutoFit[i] && !hasItems(i) {
				tr.collapsed = true
			}
		}
		tracks = append(tracks, tr)
	}
	// Positive implicit tracks.
	for i := 0; i < counts.pos; i++ {
		tracks = append(tracks, newTrack(autoTrackFn(autoTracks, counts.neg+counts.expl+i)))
	}
	return tracks
}

// gapBetween computes the effective gap between track i and i+1 (auto-fit
// collapsed tracks and their adjacent gaps collapse to zero).
func gapBetween(tracks []gridTrack, i int, gap float32) float32 {
	if tracks[i].collapsed {
		return 0
	}
	for j := i + 1; j < len(tracks); j++ {
		if !tracks[j].collapsed {
			return gap
		}
	}
	// All tracks after i are collapsed: collapse the gap (matches taffy's
	// trailing auto-fit special case).
	return 0
}

func tracksUsedSpace(tracks []gridTrack, gap float32) float32 {
	sum := float32(0)
	for i := range tracks {
		sum += tracks[i].base
		if i+1 < len(tracks) {
			sum += gapBetween(tracks, i, gap)
		}
	}
	return sum
}

func tracksGapsSum(tracks []gridTrack, gap float32) float32 {
	sum := float32(0)
	for i := 0; i+1 < len(tracks); i++ {
		sum += gapBetween(tracks, i, gap)
	}
	return sum
}

// ---------------------------------------------------------------------------
// Item measurement (IntrinsicSizeMeasurer)
// ---------------------------------------------------------------------------

// trackEstimate estimates a track's size in the axis opposite to the one
// being sized: the definite max sizing function during the column pass, the
// base size during the row pass.
func (g *gridState) trackEstimate(tr *gridTrack, isWidth bool) (float32, bool) {
	if g.sizingRows {
		return tr.base, true
	}
	inner := g.innerH
	innerDef := g.innerHDef
	if !isWidth {
		inner = g.innerW
		innerDef = g.innerWDef
	}
	return resolveTrackFnDefinite(tr.maxFn, inner, innerDef)
}

func (g *gridState) estimateSpanSize(tracks []gridTrack, gap float32, from, to int, isWidth bool) (float32, bool) {
	sum := float32(0)
	for i := from; i < to && i < len(tracks); i++ {
		v, ok := g.trackEstimate(&tracks[i], isWidth)
		if !ok {
			return 0, false
		}
		sum += v
		if i+1 < to {
			sum += gapBetween(tracks, i, gap)
		}
	}
	return sum, true
}

// gridMeasureItem measures an item's content size along one axis under a
// min-content or max-content constraint (mirrors taffy's
// min/max_content_contribution): the sizing axis is measured indefinite
// unless every spanned track is fixed (min == max definite), and the other
// axis carries its estimated grid area size minus the item's margins. The
// result includes the margin sums (outer contribution).
func (t *Tree) gridMeasureItem(g *gridState, it *gridItem, isWidth bool, maxContent bool) float32 {
	var cache **float32
	if isWidth {
		if maxContent {
			cache = &it.maxW
		} else {
			cache = &it.mcW
		}
	} else {
		if maxContent {
			cache = &it.maxH
		} else {
			cache = &it.mcH
		}
	}
	if *cache != nil {
		return **cache
	}

	sizingTracks, sizingGap := g.colTracks, g.colGap
	otherTracks, otherGap := g.rowTracks, g.rowGap
	basis, basisDef := g.innerW, g.innerWDef
	if !isWidth {
		sizingTracks, sizingGap = g.rowTracks, g.rowGap
		otherTracks, otherGap = g.colTracks, g.colGap
		basis, basisDef = g.innerH, g.innerHDef
	}

	sizingIdx := g.indexRange(it, isWidth)
	otherIdx := g.indexRange(it, !isWidth)
	sizingKnown, sizingKnownOK := fixedSpanSize(sizingTracks, sizingGap, sizingIdx, basis, basisDef)
	otherSize, otherDef := g.estimateSpanSize(otherTracks, otherGap, otherIdx[0], otherIdx[1], !isWidth)

	var marginSums float32
	if isWidth {
		marginSums = it.margin[1] + it.margin[3]
	} else {
		marginSums = it.margin[0] + it.margin[2]
	}
	otherMarginSums := it.margin[0] + it.margin[2]
	if !isWidth {
		otherMarginSums = it.margin[1] + it.margin[3]
	}

	// Definite preferred size in the measured axis contributes directly
	// (clamped by min/max), like taffy's min/max-content contribution for
	// items with definite preferred sizes.
	var prefOK bool
	var pref float32
	if isWidth {
		if v, ok := it.style.Size.Width.Resolve(g.innerW); ok {
			pref = clampInto(v, it.style.MinSize.Width.MaybeResolve(g.innerW, 0), it.style.MaxSize.Width.ResolveLoose(g.innerW))
			prefOK = true
		}
	} else {
		if v, ok := it.style.Size.Height.Resolve(g.innerW); ok {
			pref = clampInto(v, it.style.MinSize.Height.MaybeResolve(g.innerW, 0), it.style.MaxSize.Height.ResolveLoose(g.innerW))
			prefOK = true
		}
	}
	if prefOK {
		size := pref + marginSums
		*cache = &size
		return size
	}

	known := SizeDimension{Width: AutoDimension(), Height: AutoDimension()}
	var avail AvailableSpace
	if maxContent {
		avail = MaxContentAvailable()
	} else {
		avail = MinContentAvailable()
	}
	if sizingKnownOK {
		if isWidth {
			known.Width = LengthDimension(sizingKnown)
		} else {
			known.Height = LengthDimension(sizingKnown)
		}
	}
	if otherDef {
		space := otherSize - otherMarginSums
		if space < 0 {
			space = 0
		}
		if isWidth {
			known.Height = LengthDimension(space)
			avail = avail.WithDefiniteAxis(false, space)
		} else {
			known.Width = LengthDimension(space)
			avail = avail.WithDefiniteAxis(true, space)
		}
	}

	var size float32
	if len(t.children[it.node]) == 0 {
		res := t.runMeasure(it.node, known, avail)
		size = res.Width
		if !isWidth {
			size = res.Height
		}
	} else {
		out := compute(t, it.node, layoutInput{
			known:         known,
			parentStyle:   it.style,
			available:     avail,
			forceRelative: true,
		}, false)
		size = out.size.Width
		if !isWidth {
			size = out.size.Height
		}
	}
	// Outer contribution: add the margin sums.
	size += marginSums
	*cache = &size
	return size
}

// fixedSpanSize returns the span's size when every spanned track is fixed
// (min and max sizing functions both definite and equal); the sizing axis is
// treated as known in that case (taffy grid_area_size).
func fixedSpanSize(tracks []gridTrack, gap float32, idx [2]int, basis float32, basisDef bool) (float32, bool) {
	sum := float32(0)
	for i := idx[0]; i < idx[1] && i < len(tracks); i++ {
		tr := &tracks[i]
		minV, minOK := resolveTrackFnDefinite(tr.minFn, basis, basisDef)
		maxV, maxOK := resolveTrackFnDefinite(tr.maxFn, basis, basisDef)
		if !(minOK && maxOK && minV == maxV) {
			return 0, false
		}
		sum += tr.base
		if i+1 < idx[1] {
			sum += gapBetween(tracks, i, gap)
		}
	}
	return sum, true
}

// gridMinimumContribution is the item's minimum contribution; approximated
// by the min-content contribution (equal when the preferred size is auto,
// which is the common case; documented deviation).
func (t *Tree) gridMinimumContribution(g *gridState, it *gridItem, isWidth bool) float32 {
	return t.gridMeasureItem(g, it, isWidth, false)
}

// ---------------------------------------------------------------------------
// Space distribution helpers (track_sizing.rs)
// ---------------------------------------------------------------------------

const gridDistributeThreshold = 0.01
const gridBeyondThreshold = 0.000001

// distributeSpaceUpToLimits ports taffy's distribute_space_up_to_limits.
func distributeSpaceUpToLimits(space float32, tracks []gridTrack, affected func(*gridTrack) bool, prop func(*gridTrack) float32, get func(*gridTrack) float32, limit func(*gridTrack) float32) float32 {
	maxIter := len(tracks) + 1
	for iter := 0; iter < maxIter; iter++ {
		if space <= gridDistributeThreshold {
			break
		}
		propSum := float32(0)
		minIncrease := float32(f32inf())
		for i := range tracks {
			tr := &tracks[i]
			if !affected(tr) || get(tr)+tr.itemInc >= limit(tr) {
				continue
			}
			p := prop(tr)
			propSum += p
			lim := (limit(tr) - get(tr) - tr.itemInc) / p
			if lim < minIncrease {
				minIncrease = lim
			}
		}
		if propSum == 0 {
			break
		}
		iterInc := minIncrease
		if v := space / propSum; v < iterInc {
			iterInc = v
		}
		for i := range tracks {
			tr := &tracks[i]
			if !affected(tr) {
				continue
			}
			inc := iterInc * prop(tr)
			if inc > 0 && get(tr)+tr.itemInc+inc <= limit(tr)+gridDistributeThreshold {
				tr.itemInc += inc
				space -= inc
			}
		}
	}
	return space
}

type gridContributionType uint8

const (
	contribMinimum gridContributionType = 0
	contribMaximum gridContributionType = 1
)

// distributeItemSpaceToBaseSize ports distribute_item_space_to_base_size
// (11.5.1): distribute an item's space contribution across its spanned
// tracks' base sizes.
func distributeItemSpaceToBaseSize(isFlex bool, space float32, tracks []gridTrack, affected func(*gridTrack) bool, limit func(*gridTrack) float32, cType gridContributionType) {
	prop := func(*gridTrack) float32 { return 1 }
	if isFlex {
		origAffected := affected
		filter := func(tr *gridTrack) bool { return trackIsFlexible(tr) && origAffected(tr) }
		flexSum := float32(0)
		for i := range tracks {
			if filter(&tracks[i]) {
				flexSum += tracks[i].maxFn.Value
			}
		}
		if flexSum > 0 {
			prop = func(tr *gridTrack) float32 { return tr.maxFn.Value }
		}
		affected = filter
	}
	if space == 0 {
		return
	}
	any := false
	for i := range tracks {
		if affected(&tracks[i]) {
			any = true
			break
		}
	}
	if !any {
		return
	}

	// 1. Find the space to distribute.
	sizes := float32(0)
	for i := range tracks {
		sizes += tracks[i].base
	}
	extra := space - sizes
	if extra < 0 {
		extra = 0
	}

	// 2. Distribute up to limits.
	get := func(tr *gridTrack) float32 { return tr.base }
	extra = distributeSpaceUpToLimits(extra, tracks, affected, prop, get, limit)

	// 3. Distribute remaining space beyond limits.
	if extra > gridBeyondThreshold {
		filter := func(tr *gridTrack) bool {
			if cType == contribMinimum {
				return trackFnIsIntrinsic(tr.maxFn)
			}
			return tr.maxFn.Kind == GridTrackFnMaxContent || tr.maxFn.Kind == GridTrackFnFitContent
		}
		count := 0
		for i := range tracks {
			if affected(&tracks[i]) && filter(&tracks[i]) {
				count++
			}
		}
		if count == 0 {
			filter = func(*gridTrack) bool { return true }
		}
		distributeSpaceUpToLimits(extra, tracks, func(tr *gridTrack) bool { return affected(tr) && filter(tr) }, prop, get,
			func(tr *gridTrack) float32 { return tr.fitLimit })
	}

	// 4. Accumulate planned increases.
	for i := range tracks {
		if tracks[i].itemInc > tracks[i].plannedBaseInc {
			tracks[i].plannedBaseInc = tracks[i].itemInc
		}
		tracks[i].itemInc = 0
	}
}

// distributeItemSpaceToGrowthLimit ports distribute_item_space_to_growth_limit.
func distributeItemSpaceToGrowthLimit(space float32, tracks []gridTrack, affected func(*gridTrack) bool) {
	count := 0
	for i := range tracks {
		if affected(&tracks[i]) {
			count++
		}
	}
	if space == 0 || count == 0 {
		return
	}
	get := func(tr *gridTrack) float32 {
		if isInfF32(tr.growth) {
			return tr.base
		}
		return tr.growth
	}
	sizes := float32(0)
	for i := range tracks {
		sizes += get(&tracks[i])
	}
	extra := space - sizes
	if extra < 0 {
		extra = 0
	}
	extra = distributeSpaceUpToLimits(extra, tracks, func(tr *gridTrack) bool {
		return affected(tr) && (tr.infinitelyGrowable || isInfF32(tr.growth))
	}, func(*gridTrack) float32 { return 1 }, get, func(tr *gridTrack) float32 { return tr.fitLimit })
	if extra > 0 {
		distributeSpaceUpToLimits(extra, tracks, affected, func(*gridTrack) float32 { return 1 }, get,
			func(tr *gridTrack) float32 { return tr.fitLimit })
	}
	for i := range tracks {
		if tracks[i].itemInc > tracks[i].plannedGrowthInc {
			tracks[i].plannedGrowthInc = tracks[i].itemInc
		}
		tracks[i].itemInc = 0
	}
}

func fitContentLimitedGrowth(tr *gridTrack) float32 {
	lim := tr.growth
	if tr.fitLimit < lim {
		lim = tr.fitLimit
	}
	return lim
}

// ---------------------------------------------------------------------------
// Track sizing algorithm (track_sizing.rs)
// ---------------------------------------------------------------------------

// gridTrackSizing runs the full track sizing algorithm for one axis.
// availableKind: 0 = definite (availableValue), 1 = min-content, 2 =
// max-content.
func (t *Tree) gridTrackSizing(g *gridState, isWidth bool, minSize float32, minDef bool, maxSize float32, maxDef bool, alignContent AlignContent, availableKind uint8, availableValue float32) {
	tracks := g.colTracks
	gap := g.colGap
	inner := g.innerW
	innerDef := g.innerWDef
	if !isWidth {
		tracks = g.rowTracks
		gap = g.rowGap
		inner = g.innerH
		innerDef = g.innerHDef
	}

	// 11.4 Initialise track sizes.
	for i := range tracks {
		tr := &tracks[i]
		tr.base = 0
		if v, ok := resolveTrackFnDefinite(tr.minFn, inner, innerDef); ok {
			tr.base = v
		}
		tr.growth = f32inf()
		if v, ok := resolveTrackFnDefinite(tr.maxFn, inner, innerDef); ok {
			tr.growth = v
		}
		if tr.growth < tr.base {
			tr.growth = tr.base
		}
		tr.fitLimit = f32inf()
		if tr.maxFn.Kind == GridTrackFnFitContent {
			tr.fitLimit = tr.maxFn.Value
		}
	}

	// 11.5 Resolve intrinsic track sizes.
	t.gridResolveIntrinsicTrackSizes(g, isWidth, tracks, gap, availableKind)

	// 11.6 Maximise tracks: distribute free space to finite growth limits.
	used := tracksUsedSpace(tracks, gap)
	var free float32
	if availableKind == 0 {
		free = availableValue - used
	} else {
		free = f32inf()
	}
	if isInfF32(free) {
		for i := range tracks {
			tracks[i].base = tracks[i].growth
		}
	} else if free > 0 {
		distributeSpaceUpToLimits(free, tracks, func(*gridTrack) bool { return true },
			func(*gridTrack) float32 { return 1 },
			func(tr *gridTrack) float32 { return tr.base },
			func(tr *gridTrack) float32 { return fitContentLimitedGrowth(tr) })
		for i := range tracks {
			tracks[i].base += tracks[i].itemInc
			tracks[i].itemInc = 0
		}
	}

	// Space for the final two expansion steps: the container's own size when
	// definite, else max-content (min-content on the inline axis only).
	expKind, expValue := availableKind, availableValue
	if innerDef {
		expKind, expValue = 0, inner
	} else if !(availableKind == 1 && isWidth) {
		expKind = 2
	}

	// 11.7 Expand flexible tracks.
	t.gridExpandFlexibleTracks(g, isWidth, tracks, gap, minSize, minDef, maxSize, maxDef, expKind, expValue)

	// 11.8 Stretch auto tracks.
	if alignContent.Resolve(false) == AlignContentStretch {
		gridStretchAutoTracks(tracks, gap, minSize, minDef, expKind, expValue)
	}
}

// gridResolveIntrinsicTrackSizes ports resolve_intrinsic_track_sizes (the
// generic batching path; taffy's span-1 fast path is an optimization with
// the same result).
func (t *Tree) gridResolveIntrinsicTrackSizes(g *gridState, isWidth bool, tracks []gridTrack, gap float32, availableKind uint8) {
	// Batch items: non-flex ascending by span, flex items last.
	type batched struct {
		it   *gridItem
		flex bool
		span int
	}
	var ordered []batched
	var flexItems []batched
	for _, it := range g.items {
		flex := false
		idx := g.indexRange(it, isWidth)
		for i := idx[0]; i < idx[1] && i < len(tracks); i++ {
			if trackIsFlexible(&tracks[i]) {
				flex = true
				break
			}
		}
		b := batched{it: it, flex: flex, span: it.span(isWidth)}
		if flex {
			flexItems = append(flexItems, b)
		} else {
			ordered = append(ordered, b)
		}
	}
	// Sort non-flex items by span (stable, to preserve document order).
	for i := 1; i < len(ordered); i++ {
		for j := i; j > 0 && ordered[j].span < ordered[j-1].span; j-- {
			ordered[j], ordered[j-1] = ordered[j-1], ordered[j]
		}
	}
	all := append(ordered, flexItems...)

	spannedTracks := func(it *gridItem) []gridTrack {
		idx := g.indexRange(it, isWidth)
		lo := idx[0]
		if lo < 0 {
			lo = 0
		}
		if lo > len(tracks) {
			lo = len(tracks)
		}
		hi := idx[1]
		if hi > len(tracks) {
			hi = len(tracks)
		}
		if hi < lo {
			hi = lo
		}
		return tracks[lo:hi]
	}

	flushBase := func() {
		for i := range tracks {
			tracks[i].base += tracks[i].plannedBaseInc
			tracks[i].plannedBaseInc = 0
		}
	}
	flushGrowth := func(setGrowable bool) {
		for i := range tracks {
			tr := &tracks[i]
			if tr.plannedGrowthInc > 0 {
				if isInfF32(tr.growth) {
					tr.growth = tr.base + tr.plannedGrowthInc
				} else {
					tr.growth += tr.plannedGrowthInc
				}
				tr.infinitelyGrowable = setGrowable
			} else {
				tr.infinitelyGrowable = false
			}
			tr.plannedGrowthInc = 0
		}
	}

	// Batch boundaries: consecutive items with the same span; flex items in
	// one trailing batch.
	for bi := 0; bi < len(all); {
		bj := bi + 1
		if !all[bi].flex {
			for bj < len(all) && !all[bj].flex && all[bj].span == all[bi].span {
				bj++
			}
		} else {
			bj = len(all)
		}
		isFlex := all[bi].flex
		batch := all[bi:bj]

		// Step 1: intrinsic minimums — distribute items' minimum
		// contributions to tracks with intrinsic min sizing functions.
		for _, b := range batch {
			span := spannedTracks(b.it)
			hasIntrMin := false
			for i := range span {
				if trackFnIsIntrinsic(span[i].minFn) {
					hasIntrMin = true
					break
				}
			}
			if !hasIntrMin {
				continue
			}
			space := t.gridMinimumContribution(g, b.it, isWidth)
			if space > 0 {
				distributeItemSpaceToBaseSize(isFlex, space, span,
					func(tr *gridTrack) bool { return trackFnIsIntrinsic(tr.minFn) },
					func(tr *gridTrack) float32 { return tr.growth },
					contribMinimum)
			}
		}
		flushBase()

		// Step 2: content-based minimums — min-content contributions to
		// tracks with a min-content/max-content min sizing function.
		for _, b := range batch {
			span := spannedTracks(b.it)
			match := false
			for i := range span {
				if span[i].minFn.Kind == GridTrackFnMinContent || span[i].minFn.Kind == GridTrackFnMaxContent {
					match = true
					break
				}
			}
			if !match {
				continue
			}
			space := t.gridMeasureItem(g, b.it, isWidth, false)
			if space > 0 {
				distributeItemSpaceToBaseSize(isFlex, space, span,
					func(tr *gridTrack) bool {
						return tr.minFn.Kind == GridTrackFnMinContent || tr.minFn.Kind == GridTrackFnMaxContent
					},
					func(tr *gridTrack) float32 { return tr.growth },
					contribMinimum)
			}
		}
		flushBase()

		// Step 3: max-content minimums. The max-content min fn part always
		// applies; the auto-min part only under a max-content constraint.
		for _, b := range batch {
			span := spannedTracks(b.it)
			hasMaxContentMin := false
			hasAutoMin := false
			for i := range span {
				if span[i].minFn.Kind == GridTrackFnMaxContent {
					hasMaxContentMin = true
				}
				if span[i].minFn.Kind == GridTrackFnAuto {
					hasAutoMin = true
				}
			}
			if !hasMaxContentMin && !(hasAutoMin && availableKind == 2) {
				continue
			}
			space := t.gridMeasureItem(g, b.it, isWidth, true)
			if space > 0 {
				if hasMaxContentMin {
					distributeItemSpaceToBaseSize(isFlex, space, span,
						func(tr *gridTrack) bool { return tr.minFn.Kind == GridTrackFnMaxContent },
						func(*gridTrack) float32 { return f32inf() },
						contribMaximum)
				} else {
					distributeItemSpaceToBaseSize(isFlex, space, span,
						func(tr *gridTrack) bool { return tr.minFn.Kind == GridTrackFnAuto },
						func(tr *gridTrack) float32 { return fitContentLimitedGrowth(tr) },
						contribMaximum)
				}
			}
		}
		flushBase()

		// Step 4: growth limit >= base size.
		for i := range tracks {
			if tracks[i].growth < tracks[i].base {
				tracks[i].growth = tracks[i].base
			}
		}

		if !isFlex {
			// Step 5: intrinsic maximums — growth limits of tracks with
			// intrinsic max sizing functions.
			for _, b := range batch {
				span := spannedTracks(b.it)
				match := false
				for i := range span {
					if trackFnIsIntrinsic(span[i].maxFn) {
						match = true
						break
					}
				}
				if !match {
					continue
				}
				space := t.gridMeasureItem(g, b.it, isWidth, false)
				if space > 0 {
					distributeItemSpaceToGrowthLimit(space, span, func(tr *gridTrack) bool {
						return trackFnIsIntrinsic(tr.maxFn)
					})
				}
			}
			flushGrowth(true)

			// Step 6: max-content maximums — growth limits of tracks with a
			// max-content (or fit-content) max sizing function.
			for _, b := range batch {
				span := spannedTracks(b.it)
				match := false
				for i := range span {
					if span[i].maxFn.Kind == GridTrackFnMaxContent || span[i].maxFn.Kind == GridTrackFnFitContent {
						match = true
						break
					}
				}
				if !match {
					continue
				}
				space := t.gridMeasureItem(g, b.it, isWidth, true)
				if space > 0 {
					distributeItemSpaceToGrowthLimit(space, span, func(tr *gridTrack) bool {
						return tr.maxFn.Kind == GridTrackFnMaxContent || tr.maxFn.Kind == GridTrackFnFitContent
					})
				}
			}
			flushGrowth(false)
		}
		bi = bj
	}

	// Final: infinite growth limits fall back to base sizes so the maximise
	// step cannot affect flexible or empty tracks.
	for i := range tracks {
		if isInfF32(tracks[i].growth) {
			tracks[i].growth = tracks[i].base
		}
		if tracks[i].growth < tracks[i].base {
			tracks[i].growth = tracks[i].base
		}
	}
}

// findSizeOfFr ports taffy's find_size_of_fr (11.7.1): the largest fr size
// that does not exceed spaceToFill. gaps is the total gap space between the
// tracks (gutters count as fixed-size tracks in taffy's representation).
func findSizeOfFr(tracks []gridTrack, spaceToFill, gaps float32) float32 {
	if spaceToFill == 0 {
		return 0
	}
	hypothetical := f32inf()
	maxIter := len(tracks) + 1
	for iter := 0; iter < maxIter; iter++ {
		used := gaps
		flexSum := float32(0)
		for i := range tracks {
			tr := &tracks[i]
			if trackIsFlexible(tr) && tr.maxFn.Value*hypothetical >= tr.base {
				flexSum += tr.maxFn.Value
			} else {
				used += tr.base
			}
		}
		leftover := spaceToFill - used
		flex := flexSum
		if flex < 1 {
			flex = 1
		}
		prev := hypothetical
		hypothetical = leftover / flex
		valid := true
		for i := range tracks {
			tr := &tracks[i]
			if !trackIsFlexible(tr) {
				continue
			}
			ff := tr.maxFn.Value
			if !(ff*hypothetical >= tr.base || ff*prev < tr.base) {
				valid = false
				break
			}
		}
		if valid {
			break
		}
	}
	return hypothetical
}

// gridExpandFlexibleTracks ports expand_flexible_tracks (11.7).
func (t *Tree) gridExpandFlexibleTracks(g *gridState, isWidth bool, tracks []gridTrack, gap float32, minSize float32, minDef bool, maxSize float32, maxDef bool, availableKind uint8, availableValue float32) {
	gapSum := tracksGapsSum(tracks, gap)
	flexFraction := float32(0)
	switch availableKind {
	case 0: // definite
		used := tracksUsedSpace(tracks, gap)
		if availableValue-used > 0 {
			flexFraction = findSizeOfFr(tracks, availableValue, gapSum)
		}
	case 1: // min-content constraint
		flexFraction = 0
	default: // max-content
		best := float32(0)
		for i := range tracks {
			tr := &tracks[i]
			if !trackIsFlexible(tr) {
				continue
			}
			v := tr.base
			if tr.maxFn.Value > 1 {
				v = tr.base / tr.maxFn.Value
			}
			if v > best {
				best = v
			}
		}
		for _, it := range g.items {
			idx := g.indexRange(it, isWidth)
			crosses := false
			for i := idx[0]; i < idx[1] && i < len(tracks); i++ {
				if trackIsFlexible(&tracks[i]) {
					crosses = true
					break
				}
			}
			if !crosses {
				continue
			}
			contribution := t.gridMeasureItem(g, it, isWidth, true)
			lo := idx[0]
			if lo < 0 {
				lo = 0
			}
			hi := idx[1]
			if hi > len(tracks) {
				hi = len(tracks)
			}
			if lo >= hi {
				continue
			}
			spanGaps := float32(0)
			for i := lo; i+1 < hi; i++ {
				spanGaps += gapBetween(tracks, i, gap)
			}
			if v := findSizeOfFr(tracks[lo:hi], contribution, spanGaps); v > best {
				best = v
			}
		}
		flexFraction = best
		// Clamp against the container's min/max sizes.
		hypothetical := float32(0)
		for i := range tracks {
			tr := &tracks[i]
			if trackIsFlexible(tr) {
				hypothetical += maxf32(tr.base, tr.maxFn.Value*flexFraction)
			} else {
				hypothetical += tr.base
			}
		}
		hypothetical += gapSum
		if minDef && hypothetical < minSize {
			flexFraction = findSizeOfFr(tracks, minSize, gapSum)
		} else if maxDef && hypothetical > maxSize {
			flexFraction = findSizeOfFr(tracks, maxSize, gapSum)
		}
	}
	for i := range tracks {
		tr := &tracks[i]
		if trackIsFlexible(tr) {
			tr.base = maxf32(tr.base, tr.maxFn.Value*flexFraction)
		}
	}
}

// gridStretchAutoTracks ports stretch_auto_tracks (11.8).
func gridStretchAutoTracks(tracks []gridTrack, gap float32, minSize float32, minDef bool, availableKind uint8, availableValue float32) {
	numAuto := 0
	for i := range tracks {
		if tracks[i].maxFn.Kind == GridTrackFnAuto {
			numAuto++
		}
	}
	if numAuto == 0 {
		return
	}
	used := tracksUsedSpace(tracks, gap)
	var free float32
	if availableKind == 0 {
		free = availableValue - used
	} else if minDef {
		free = minSize - used
	} else {
		free = 0
	}
	if free > 0 {
		extra := free / float32(numAuto)
		for i := range tracks {
			if tracks[i].maxFn.Kind == GridTrackFnAuto {
				tracks[i].base += extra
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Alignment (alignment.rs)
// ---------------------------------------------------------------------------

// gridAlignContentKeyword resolves a content alignment value with the
// distributed-keyword fallback (apply_alignment_fallback).
func gridAlignContentKeyword(align AlignContent, free float32, numTracks int) AlignContent {
	a := align.Resolve(false)
	if numTracks <= 1 || free <= 0 {
		switch a {
		case AlignContentStretch, AlignContentSpaceBetween:
			return AlignContentFlexStart
		case AlignContentSpaceAround, AlignContentSpaceEvenly:
			return AlignContentCenter
		}
	}
	return a
}

// alignGridTracks computes per-track start offsets within the content box
// for a content alignment (align_tracks). collapsed marks auto-fit tracks
// that contribute no size and no gap.
func alignGridTracks(sizes []float32, gaps []float32, collapsed []bool, containerSize float32, align AlignContent) []float32 {
	offsets := make([]float32, len(sizes))
	used := float32(0)
	num := 0
	for i, s := range sizes {
		used += s
		if i < len(gaps) {
			used += gaps[i]
		}
		if i >= len(collapsed) || !collapsed[i] {
			num++
		}
	}
	free := containerSize - used
	keyword := gridAlignContentKeyword(align, free, num)
	leading := float32(0)
	between := float32(0)
	switch keyword {
	case AlignContentEnd:
		leading = free
	case AlignContentCenter:
		leading = free / 2
	case AlignContentSpaceBetween:
		if num > 1 {
			between = free / float32(num-1)
		}
	case AlignContentSpaceAround:
		leading = free / float32(num) / 2
		between = free / float32(num)
	case AlignContentSpaceEvenly:
		leading = free / float32(num+1)
		between = free / float32(num+1)
	}
	pos := leading
	for i := range sizes {
		offsets[i] = pos
		pos += sizes[i]
		if i < len(gaps) {
			pos += gaps[i] + between
		}
	}
	return offsets
}

// alignGridItemInArea returns the offset of the item within its area for one
// axis (align_item_within_area, without insets/auto margins).
func alignGridItemInArea(alignment AlignItems, area, size float32) float32 {
	free := area - size
	if free < 0 {
		free = 0
	}
	switch alignment.Resolve(false) {
	case AlignEnd:
		return free
	case AlignCenter:
		return free / 2
	default: // start, baseline, stretch
		return 0
	}
}

// ---------------------------------------------------------------------------
// Placement (placement.rs)
// ---------------------------------------------------------------------------

// gridPlaceItems ports place_grid_items: definite placements, then
// definite-secondary placements, then sparse auto-placement.
func gridPlaceItems(m *gridMatrix, items []*gridItem, flow GridAutoFlow) {
	// The "primary" axis is the one the auto-placement cursor advances
	// along: columns for row flow (row-major filling), rows for column flow
	// (taffy GridAutoFlow::primary_axis).
	primaryIsRow := flow.IsColumn()

	colStart := func(it *gridItem) ozPlacement { return ozPlacementOf(it.style.GridColumn.Start, m.cols.expl) }
	colEnd := func(it *gridItem) ozPlacement { return ozPlacementOf(it.style.GridColumn.End, m.cols.expl) }
	rowStart := func(it *gridItem) ozPlacement { return ozPlacementOf(it.style.GridRow.Start, m.rows.expl) }
	rowEnd := func(it *gridItem) ozPlacement { return ozPlacementOf(it.style.GridRow.End, m.rows.expl) }
	// Primary/secondary axis views (primary = auto-flow direction).
	primStart := func(it *gridItem) ozPlacement {
		if primaryIsRow {
			return rowStart(it)
		}
		return colStart(it)
	}
	primEnd := func(it *gridItem) ozPlacement {
		if primaryIsRow {
			return rowEnd(it)
		}
		return colEnd(it)
	}
	secStart := func(it *gridItem) ozPlacement {
		if primaryIsRow {
			return colStart(it)
		}
		return rowStart(it)
	}
	secEnd := func(it *gridItem) ozPlacement {
		if primaryIsRow {
			return colEnd(it)
		}
		return rowEnd(it)
	}

	// 1. Items with definite positions in both axes.
	for _, it := range items {
		if primaryIsRow {
			if !(ozPairDefinite(rowStart(it), rowEnd(it)) && ozPairDefinite(colStart(it), colEnd(it))) {
				continue
			}
			it.row = resolveOZDefinite(rowStart(it), rowEnd(it))
			it.col = resolveOZDefinite(colStart(it), colEnd(it))
		} else {
			if !(ozPairDefinite(colStart(it), colEnd(it)) && ozPairDefinite(rowStart(it), rowEnd(it))) {
				continue
			}
			it.col = resolveOZDefinite(colStart(it), colEnd(it))
			it.row = resolveOZDefinite(rowStart(it), rowEnd(it))
		}
		it.placed = true
		m.markArea(it.row, it.col, cellDefinitelyPlaced, primaryIsRow)
	}

	// 2. Items with a definite secondary-axis position only.
	for _, it := range items {
		if it.placed {
			continue
		}
		var secondaryDefinite bool
		if primaryIsRow {
			secondaryDefinite = ozPairDefinite(colStart(it), colEnd(it)) && !ozPairDefinite(rowStart(it), rowEnd(it))
		} else {
			secondaryDefinite = ozPairDefinite(rowStart(it), rowEnd(it)) && !ozPairDefinite(colStart(it), colEnd(it))
		}
		if !secondaryDefinite {
			continue
		}
		var secRange [2]int
		if primaryIsRow {
			secRange = resolveOZDefinite(colStart(it), colEnd(it))
		} else {
			secRange = resolveOZDefinite(rowStart(it), rowEnd(it))
		}
		primarySpan := ozIndefiniteSpan(primStart(it), primEnd(it))
		pos := m.secondaryCursor(primaryIsRow, secRange[0])
		for {
			prim := resolveOZIndefinite(pos, primarySpan)
			if next, hit := m.primaryCollisionJump(primaryIsRow, prim, secRange); hit {
				pos = next
				continue
			}
			m.record(it, primaryIsRow, prim, secRange)
			break
		}
	}

	// 4. Auto-place the remaining items (indefinite secondary axis).
	var cursorPrimary, cursorSecondary int
	if primaryIsRow {
		cursorPrimary, cursorSecondary = m.rows.startLine(), m.cols.startLine()
	} else {
		cursorPrimary, cursorSecondary = m.cols.startLine(), m.rows.startLine()
	}
	for _, it := range items {
		if it.placed {
			continue
		}
		if ozPairDefinite(secStart(it), secEnd(it)) {
			continue // definite secondary axis handled in pass 2
		}
		primarySpan := ozIndefiniteSpan(primStart(it), primEnd(it))
		secondarySpanN := ozIndefiniteSpan(secStart(it), secEnd(it))
		primaryDefinite := ozPairDefinite(primStart(it), primEnd(it))

		if primaryDefinite {
			prim := resolveOZDefinite(primStart(it), primEnd(it))
			sec := cursorSecondary
			if flow.IsDense() {
				if primaryIsRow {
					sec = m.cols.startLine()
				} else {
					sec = m.rows.startLine()
				}
			} else if prim[0] < cursorPrimary {
				sec++
			}
			for {
				secRange := resolveOZIndefinite(sec, secondarySpanN)
				if next, hit := m.primaryCollisionJump(!primaryIsRow, secRange, prim); hit {
					sec = next
					continue
				}
				m.record(it, primaryIsRow, prim, secRange)
				break
			}
			continue
		}

		gridStartLine, gridEndLine := 0, 0
		if primaryIsRow {
			gridStartLine, gridEndLine = m.rows.startLine(), m.rows.endLine()
		} else {
			gridStartLine, gridEndLine = m.cols.startLine(), m.cols.endLine()
		}
		spansAll := primarySpan >= (gridEndLine - gridStartLine)
		prim, sec := cursorPrimary, cursorSecondary
		for {
			primRange := resolveOZIndefinite(prim, primarySpan)
			secRange := resolveOZIndefinite(sec, secondarySpanN)
			if primRange[1] > gridEndLine {
				if prim == gridStartLine {
					m.record(it, primaryIsRow, primRange, secRange)
					break
				}
				sec++
				prim = gridStartLine
				continue
			}
			if spansAll {
				if next, hit := m.occupiedTrackJump(primaryIsRow, secRange); hit {
					sec = next
					prim = gridStartLine
					continue
				}
				m.record(it, primaryIsRow, primRange, secRange)
				break
			}
			if next, hit := m.primaryCollisionJump(primaryIsRow, primRange, secRange); hit {
				prim = next
				continue
			}
			m.record(it, primaryIsRow, primRange, secRange)
			break
		}
		// Sparse packing: move the cursor past the placed item.
		if !flow.IsDense() {
			cursorPrimary = it.rangeOZ(!primaryIsRow)[1]
			cursorSecondary = it.rangeOZ(primaryIsRow)[0]
		}
	}
}

// record stores an auto/secondary placement and marks occupancy.
func (m *gridMatrix) record(it *gridItem, primaryIsRow bool, primary, secondary [2]int) {
	if gridDebug {
		println("record primIsRow:", primaryIsRow, "prim:", primary[0], primary[1], "sec:", secondary[0], secondary[1])
	}
	var row, col [2]int
	if primaryIsRow {
		row, col = primary, secondary
	} else {
		col, row = primary, secondary
	}
	m.markArea(row, col, cellAutoPlaced, primaryIsRow)
	it.row, it.col = row, col
	it.placed = true
}

// ---------------------------------------------------------------------------
// Container layout entry point
// ---------------------------------------------------------------------------

// computeGrid ports compute_grid_layout for the supported subset.
func (t *Tree) computeGrid(node Node, style *Style, input layoutInput) layoutOutput {
	input = input.withRootDefaults(style)
	containerW, wDef := resolveContainerAxis(input, style.Size.Width, style.MinSize.Width, style.MaxSize.Width, true)
	containerH, hDef := resolveContainerAxis(input, style.Size.Height, style.MinSize.Height, style.MaxSize.Height, false)

	margin := style.Margin.Resolve(containerW)
	border := style.Border.Resolve(containerW)
	padding := style.Padding.Resolve(containerW)

	pbW := border[1] + border[3] + padding[1] + padding[3]
	pbH := border[0] + border[2] + padding[0] + padding[2]

	innerW := containerW - pbW - margin[1] - margin[3]
	innerH := containerH - pbH - margin[0] - margin[2]
	if !wDef {
		innerW = 0
	}
	if !hDef {
		innerH = 0
	}
	if innerW < 0 {
		innerW = 0
	}
	if innerH < 0 {
		innerH = 0
	}

	colGap := style.Gap.Width.MaybeResolve(innerW, 0)
	rowGap := style.Gap.Height.MaybeResolve(innerH, 0)

	// Available grid space kinds: 0 definite (with value), 1 min-content,
	// 2 max-content. A definite kind only applies when the container's own
	// size is definite in that axis (the engine's parents pass known
	// dimensions when they stretch a container, matching taffy's
	// constrained_available_space); otherwise the kind comes from the input
	// available space so intrinsic sizing probes stay content-based.
	availKindW, availValueW := uint8(2), float32(0)
	availKindH, availValueH := uint8(2), float32(0)
	if wDef {
		availKindW, availValueW = 0, innerW
	} else {
		availKindW = input.available.kindW
		if availKindW == 0 {
			availKindW = 2
		}
	}
	if hDef {
		availKindH, availValueH = 0, innerH
	} else {
		availKindH = input.available.kindH
		if availKindH == 0 {
			availKindH = 2
		}
	}

	// Container size basis for auto-fill repetition counting (outer size or
	// max/min size, minus padding/border/margins; taffy
	// auto_fit_container_size).
	autoFitW, autoFitWDef, autoFitUnderflowW := float32(0), false, false
	{
		maxWD, maxWOK := style.MaxSize.Width.Resolve(containerW)
		minWD, minWOK := style.MinSize.Width.Resolve(containerW)
		if wDef {
			autoFitW, autoFitWDef = innerW, true
		} else if maxWOK {
			autoFitW, autoFitWDef = maxWD-pbW-margin[1]-margin[3], true
		} else if minWOK {
			autoFitW, autoFitWDef = minWD-pbW-margin[1]-margin[3], true
			autoFitUnderflowW = true
		}
	}
	autoFitH, autoFitHDef, autoFitUnderflowH := float32(0), false, false
	{
		maxHD, maxHOK := style.MaxSize.Height.Resolve(containerW)
		minHD, minHOK := style.MinSize.Height.Resolve(containerW)
		if hDef {
			autoFitH, autoFitHDef = innerH, true
		} else if maxHOK {
			autoFitH, autoFitHDef = maxHD-pbH-margin[0]-margin[2], true
		} else if minHOK {
			autoFitH, autoFitHDef = minHD-pbH-margin[0]-margin[2], true
			autoFitUnderflowH = true
		}
	}

	colReps, colExplCount := computeExplicitGridSize(style.GridTemplateColumns, autoFitW, autoFitWDef, autoFitUnderflowW, colGap)
	rowReps, rowExplCount := computeExplicitGridSize(style.GridTemplateRows, autoFitH, autoFitHDef, autoFitUnderflowH, rowGap)

	// Build items (in-flow children in document order).
	g := &gridState{colGap: colGap, rowGap: rowGap, innerW: innerW, innerWDef: wDef, innerH: innerH, innerHDef: hDef}
	for _, child := range t.children[node] {
		cs := &t.styles[child]
		if cs.Display == DisplayNone {
			t.layouts[child] = Layout{Order: nextOrder()}
			continue
		}
		if cs.Position == PositionAbsolute {
			continue
		}
		it := &gridItem{node: child, style: cs}
		it.margin = cs.Margin.Resolve(containerW)
		g.items = append(g.items, it)
	}

	// Place items (grows the implicit grid counts).
	m := newGridMatrix(colExplCount, rowExplCount)
	gridPlaceItems(m, g.items, style.GridAutoFlow)
	g.colNeg, g.rowNeg = m.cols.neg, m.rows.neg

	// Initialize tracks.
	hasItemsCol := func(oz int) bool { return m.colIsOccupied(oz) }
	hasItemsRow := func(oz int) bool { return m.rowIsOccupied(oz) }
	g.colTracks = g.initializeGridTracks(m.cols, style.GridTemplateColumns, colReps, style.GridAutoColumns, hasItemsCol)
	g.rowTracks = g.initializeGridTracks(m.rows, style.GridTemplateRows, rowReps, style.GridAutoRows, hasItemsRow)

	// Mark flex/intrinsic crossings.
	for _, it := range g.items {
		colIdx := g.indexRange(it, true)
		rowIdx := g.indexRange(it, false)
		for i := colIdx[0]; i < colIdx[1] && i < len(g.colTracks); i++ {
			if trackIsFlexible(&g.colTracks[i]) {
				it.crossesFlexCol = true
			}
			if trackFnIsIntrinsic(g.colTracks[i].minFn) || trackFnIsIntrinsic(g.colTracks[i].maxFn) {
				it.crossesIntrCol = true
			}
		}
		for i := rowIdx[0]; i < rowIdx[1] && i < len(g.rowTracks); i++ {
			if trackIsFlexible(&g.rowTracks[i]) {
				it.crossesFlexRow = true
			}
			if trackFnIsIntrinsic(g.rowTracks[i].minFn) || trackFnIsIntrinsic(g.rowTracks[i].maxFn) {
				it.crossesIntrRow = true
			}
		}
	}

	// Alignment styles ("normal" behaves as stretch for grid containers).
	alignContent := AlignContentStretch
	if style.AlignContent != nil {
		alignContent = *style.AlignContent
	}
	justifyContent := AlignContentStretch
	if style.JustifyContent != nil {
		justifyContent = *style.JustifyContent
	}

	// Track sizing: inline (columns) first, then block (rows).
	innerMinW, innerMinWDef := style.MinSize.Width.Resolve(containerW)
	innerMaxW, innerMaxWDef := style.MaxSize.Width.Resolve(containerW)
	innerMinH, innerMinHDef := style.MinSize.Height.Resolve(containerW)
	innerMaxH, innerMaxHDef := style.MaxSize.Height.Resolve(containerW)
	t.gridTrackSizing(g, true, innerMinW, innerMinWDef, innerMaxW, innerMaxWDef, justifyContent, availKindW, availValueW)
	colSum := tracksUsedSpace(g.colTracks, colGap)
	g.sizingRows = true
	t.gridTrackSizing(g, false, innerMinH, innerMinHDef, innerMaxH, innerMaxHDef, alignContent, availKindH, availValueH)
	g.sizingRows = false
	rowSum := tracksUsedSpace(g.rowTracks, rowGap)

	// Container size.
	w := colSum + pbW + margin[1] + margin[3]
	h := rowSum + pbH + margin[0] + margin[2]
	if wDef {
		w = containerW
	}
	if hDef {
		h = containerH
	}
	if vw, ok := style.Size.Width.Resolve(containerW); ok && !wDef {
		w = vw + margin[1] + margin[3]
	}
	if vh, ok := style.Size.Height.Resolve(containerW); ok && !hDef {
		h = vh + margin[0] + margin[2]
	}
	w = clampInto(w, style.MinSize.Width.MaybeResolve(containerW, 0), style.MaxSize.Width.ResolveLoose(containerW))
	h = clampInto(h, style.MinSize.Height.MaybeResolve(containerH, 0), style.MaxSize.Height.ResolveLoose(containerH))

	contentW := maxf32(colSum, w-pbW-margin[1]-margin[3])
	contentH := maxf32(rowSum, h-pbH-margin[0]-margin[2])

	// Track alignment (justify/align content).
	colSizes := make([]float32, len(g.colTracks))
	colGaps := make([]float32, len(g.colTracks))
	colCollapsed := make([]bool, len(g.colTracks))
	for i := range g.colTracks {
		colSizes[i] = g.colTracks[i].base
		colCollapsed[i] = g.colTracks[i].collapsed
		if i+1 < len(g.colTracks) {
			colGaps[i] = gapBetween(g.colTracks, i, colGap)
		}
	}
	colOffsets := alignGridTracks(colSizes, colGaps, colCollapsed, contentW, justifyContent)

	rowSizes := make([]float32, len(g.rowTracks))
	rowGaps := make([]float32, len(g.rowTracks))
	rowCollapsed := make([]bool, len(g.rowTracks))
	for i := range g.rowTracks {
		rowSizes[i] = g.rowTracks[i].base
		rowCollapsed[i] = g.rowTracks[i].collapsed
		if i+1 < len(g.rowTracks) {
			rowGaps[i] = gapBetween(g.rowTracks, i, rowGap)
		}
	}
	rowOffsets := alignGridTracks(rowSizes, rowGaps, rowCollapsed, contentH, alignContent)
	if gridDebug {
		fmt.Println("rowSizes:", rowSizes)
		fmt.Println("rowOffsets:", rowOffsets)
		fmt.Println("contentH:", contentH, "rowSum:", rowSum)
		for _, it := range g.items {
			fmt.Println("item", it.node, "row", it.row, "col", it.col)
		}
	}

	innerOriginX := border[3] + padding[3]
	innerOriginY := border[0] + padding[0]

	justifyItems := AlignStretch
	if style.JustifyItems != nil {
		justifyItems = *style.JustifyItems
	}
	alignItems := AlignStretch
	if style.AlignItems != nil {
		alignItems = *style.AlignItems
	}

	scrollW, scrollH := float32(0), float32(0)
	for _, it := range g.items {
		colIdx := g.indexRange(it, true)
		rowIdx := g.indexRange(it, false)
		areaLeft := innerOriginX + trackOffsetAt(colOffsets, colIdx[0])
		var areaRight float32
		if colIdx[1] > 0 && colIdx[1]-1 < len(g.colTracks) {
			areaRight = innerOriginX + trackOffsetAt(colOffsets, colIdx[1]-1) + g.colTracks[colIdx[1]-1].base
		} else {
			areaRight = innerOriginX + contentW
		}
		areaTop := innerOriginY + trackOffsetAt(rowOffsets, rowIdx[0])
		var areaBottom float32
		if rowIdx[1] > 0 && rowIdx[1]-1 < len(g.rowTracks) {
			areaBottom = innerOriginY + trackOffsetAt(rowOffsets, rowIdx[1]-1) + g.rowTracks[rowIdx[1]-1].base
		} else {
			areaBottom = innerOriginY + contentH
		}

		areaW := areaRight - areaLeft - it.margin[1] - it.margin[3]
		areaH := areaBottom - areaTop - it.margin[0] - it.margin[2]
		if areaW < 0 {
			areaW = 0
		}
		if areaH < 0 {
			areaH = 0
		}

		justify := justifyItems
		if it.style.JustifySelf != nil {
			justify = *it.style.JustifySelf
		}
		align := alignItems
		if it.style.AlignSelf != nil {
			align = *it.style.AlignSelf
		}

		// Known dimensions only for stretching auto-sized axes (explicit
		// style sizes win, since known overrides style in compute_leaf).
		known := SizeDimension{Width: AutoDimension(), Height: AutoDimension()}
		if justify.Resolve(false) == AlignStretch && it.style.Size.Width.IsAuto() {
			known.Width = LengthDimension(areaW)
		}
		if align.Resolve(false) == AlignStretch && it.style.Size.Height.IsAuto() {
			known.Height = LengthDimension(areaH)
		}

		_ = compute(t, it.node, layoutInput{
			known:         known,
			parentStyle:   it.style,
			available:     DefiniteAvailable(areaW, areaH),
			forceRelative: true,
			commit:        input.commit,
		}, false)
		cl := t.layouts[it.node]
		wi, hi := cl.Width, cl.Height

		cl.Order = nextOrder()
		cl.X = areaLeft + it.margin[3] + alignGridItemInArea(justify, areaW, wi)
		cl.Y = areaTop + it.margin[0] + alignGridItemInArea(align, areaH, hi)
		t.layouts[it.node] = cl

		if r := cl.X + wi + it.margin[1]; r > scrollW {
			scrollW = r
		}
		if b := cl.Y + hi + it.margin[2]; b > scrollH {
			scrollH = b
		}
	}

	t.layouts[node] = Layout{
		Order: nextOrder(), Width: w, Height: h,
		ScrollWidth:  scrollW - innerOriginX,
		ScrollHeight: scrollH - innerOriginY,
	}

	// Absolute children.
	for _, child := range t.children[node] {
		cs := &t.styles[child]
		if cs.Position == PositionAbsolute && cs.Display != DisplayNone {
			t.layoutAbsolute(child, cs, input.available, innerW, innerH, border, padding, input.commit)
		}
	}

	return layoutOutput{size: SizeResult{Width: w, Height: h}}
}

func trackOffsetAt(offsets []float32, i int) float32 {
	if i < 0 {
		return 0
	}
	if i >= len(offsets) {
		if len(offsets) == 0 {
			return 0
		}
		return offsets[len(offsets)-1]
	}
	return offsets[i]
}
