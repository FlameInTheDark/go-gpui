package render

import (
	"math"
	"sort"

	"github.com/FlameInTheDark/go-gpui/geometry"
)

// This file ports crates/gpui/src/path_winding.rs verbatim: paths are binned
// into cells on the CPU, each cell lists the curves near it, neighboring
// cells with identical shading merge into rectangular PathTiles, and the GPU
// shader computes per-pixel winding (Slug-style analytic coverage) from the
// tile's curve list plus a backdrop winding. This replaces baking path pixels
// into atlas tiles: rasterization happens on the GPU exactly like upstream.

// pathBinMargin mirrors upstream MARGIN: a tile lists every curve within this
// distance of its bounds; farther curves cannot contribute partial winding.
const pathBinMargin = 0.5 + 1.0/256.0

// PathTileGpu is one merged tile (upstream PathTile). Paint is assigned when
// the frame is built (index into the frame's path paint list).
//
// The D3D11 backend uploads this struct as a StructuredBuffer element
// verbatim (platform/windows/d3d11_shader.go: struct PathTile). The trailing
// Pad keeps the stride a multiple of 16 bytes; the HLSL struct mirrors it
// exactly (uint4 header + 6 float2s = 64 bytes).
type PathTileGpu struct {
	Paint          uint32
	CurveStart     uint32
	CurveCount     uint32
	Backdrop       int32
	TopLeft        [2]float32
	BottomRight    [2]float32
	Reference      [2]float32
	CurvesTopLeft  [2]float32
	CurvesBotRight [2]float32
	Pad            [2]float32 // stride padding to 64 bytes; must stay zero
}

// PathPaintGpu mirrors upstream PathPaint.
type PathPaintGpu struct {
	Bounds      [4]float32 // origin x,y + size w,h
	ContentMask [4]float32 // x0, y0, x1, y1
	// Color packed with prepareBackground: color0 rgb, color1 rgb and the
	// background tag/color space ride along like a quad instance's rows.
	Color0     [4]float32
	Color1     [4]float32
	Tag        uint32
	ColorSpace uint32
	Pct0       float32
	Pct1       float32
	Angle      float32
	EvenOdd    uint32
}

// PathTilesResult is the binning output for one path (upstream PathTiles).
type PathTilesResult struct {
	// Bounds is the path's device-space bounds.
	Bounds geometry.Bounds[geometry.ScaledPixels]
	Tiles  []PathTileGpu
	Curves []PathCurve
}

// BinPathTiles bins a path's curves (already in scaled/device pixels? curves
// are stored in scaled pixels; `scale` multiplies them like upstream
// Binner::scale) into merged tiles for GPU winding rasterization.
func BinPathTiles(curves []PathCurve, evenOdd bool, scale float32) PathTilesResult {
	b := pathBinnerPool.get()
	defer pathBinnerPool.put(b)
	return b.bin(curves, evenOdd, scale)
}

// ---------------------------------------------------------------------------
// Binner (port of path_winding.rs)
// ---------------------------------------------------------------------------

type binnerSpan struct {
	cell  int
	count int
	key   uint64
}

type binner struct {
	input     []PathCurve
	spans     []binnerSpan
	cellEnds  []int
	steps     []int32
	keys      []uint64
	runs      []binRun
	above     []binRun
	tiles     []PathTileGpu
	outCurves []PathCurve
}

// pool avoids per-frame allocations (upstream keeps a thread-local Binner).
type binnerPool struct {
	free []*binner
}

var pathBinnerPool = &binnerPool{}

func (p *binnerPool) get() *binner {
	if n := len(p.free); n > 0 {
		b := p.free[n-1]
		p.free = p.free[:n-1]
		return b
	}
	return &binner{}
}

func (p *binnerPool) put(b *binner) {
	b.spans = b.spans[:0]
	b.tiles = b.tiles[:0]
	b.outCurves = b.outCurves[:0]
	b.runs = b.runs[:0]
	b.above = b.above[:0]
	if len(p.free) < 4 {
		p.free = append(p.free, b)
	}
}

func (b *binner) bin(curves []PathCurve, evenOdd bool, scale float32) PathTilesResult {
	if len(curves) == 0 || !(scale > 0 && !math.IsInf(float64(scale), 0) && !math.IsNaN(float64(scale))) {
		return PathTilesResult{}
	}
	minX, minY, maxX, maxY, length := b.scaleCurves(curves, scale)
	grid, ok := coveringGrid(minX, minY, maxX, maxY, len(curves), length)
	if !ok {
		return PathTilesResult{}
	}
	b.findSpans(grid)
	b.fillCells()
	b.mergeCells(grid, evenOdd)
	devicePoint := func(x, y float32) geometry.Point[geometry.ScaledPixels] {
		return geometry.Point[geometry.ScaledPixels]{X: geometry.ScaledPx(x), Y: geometry.ScaledPx(y)}
	}
	return PathTilesResult{
		Bounds: geometry.Bounds[geometry.ScaledPixels]{
			Origin: devicePoint(minX, minY),
			Size: geometry.Size[geometry.ScaledPixels]{
				Width:  geometry.ScaledPx(maxX - minX),
				Height: geometry.ScaledPx(maxY - minY),
			},
		},
		Tiles:  b.tiles,
		Curves: b.outCurves,
	}
}

func (b *binner) scaleCurves(curves []PathCurve, scale float32) (minX, minY, maxX, maxY, length float32) {
	minX, minY = float32(math.Inf(1)), float32(math.Inf(1))
	maxX, maxY = float32(math.Inf(-1)), float32(math.Inf(-1))
	b.input = b.input[:0]
	for i := range curves {
		c := &curves[i]
		scaled := PathCurve{
			P0:   geometry.Point[float32]{X: c.P0.X * scale, Y: c.P0.Y * scale},
			P1:   geometry.Point[float32]{X: c.P1.X * scale, Y: c.P1.Y * scale},
			A:    geometry.Point[float32]{X: c.A.X * scale, Y: c.A.Y * scale},
			B:    geometry.Point[float32]{X: c.B.X * scale, Y: c.B.Y * scale},
			Sign: c.Sign,
		}
		loX, hiX := curveMinMaxX(&scaled)
		if loX < minX {
			minX = loX
		}
		if hiX > maxX {
			maxX = hiX
		}
		if scaled.P0.Y < minY {
			minY = scaled.P0.Y
		}
		if scaled.P1.Y > maxY {
			maxY = scaled.P1.Y
		}
		length += hiX - loX + scaled.P1.Y - scaled.P0.Y
		b.input = append(b.input, scaled)
	}
	return
}

func (b *binner) findSpans(grid *binGrid) {
	cells := grid.rows * grid.columns
	b.spans = b.spans[:0]
	if cap(b.cellEnds) < cells {
		b.cellEnds = make([]int, cells)
	} else {
		b.cellEnds = b.cellEnds[:cells]
	}
	for i := range b.cellEnds {
		b.cellEnds[i] = 0
	}
	if cap(b.steps) < cells {
		b.steps = make([]int32, cells)
	} else {
		b.steps = b.steps[:cells]
	}
	for i := range b.steps {
		b.steps[i] = 0
	}
	for index := range b.input {
		curve := &b.input[index]
		key := uint64(orderedBits(minf(curve.P0.X, curve.P1.X)))<<32 | uint64(index)
		firstRow := grid.rowOf(curve.P0.Y - pathBinMargin)
		lastRow := grid.rowOf(curve.P1.Y + pathBinMargin)
		for row := firstRow; row <= lastRow; row++ {
			rowTop := grid.y(row)
			low, high := curveXRange(curve, rowTop-pathBinMargin, grid.y(row+1)+pathBinMargin)
			first := grid.columnOf(low - pathBinMargin)
			last := grid.columnOf(high + pathBinMargin)
			cell := row*grid.columns + first
			count := last + 1 - first
			b.spans = append(b.spans, binnerSpan{cell: cell, count: count, key: key})
			for i := cell; i < cell+count && i < len(b.cellEnds); i++ {
				b.cellEnds[i]++
			}
			// Record the curve's winding step in the first cell to the right
			// of its span (contributes to that cell's backdrop).
			if curveCrosses(curve, rowTop) && last+1 < grid.columns {
				idx := cell + count
				if idx < len(b.steps) {
					b.steps[idx] += curveStep(curve)
				}
			}
		}
	}
}

func (b *binner) fillCells() {
	total := 0
	for i := range b.cellEnds {
		count := b.cellEnds[i]
		b.cellEnds[i] = total
		total += count
	}
	if cap(b.keys) < total {
		b.keys = make([]uint64, total)
	} else {
		b.keys = b.keys[:total]
	}
	for _, span := range b.spans {
		for cursor := span.cell; cursor < span.cell+span.count && cursor < len(b.cellEnds); cursor++ {
			slot := b.cellEnds[cursor]
			if slot < len(b.keys) {
				b.keys[slot] = span.key
			}
			b.cellEnds[cursor]++
		}
	}
	start := 0
	for _, end := range b.cellEnds {
		ks := b.keys[start:end]
		sort.Slice(ks, func(i, j int) bool { return ks[i] < ks[j] })
		start = end
	}
}

func (b *binner) mergeCells(grid *binGrid, evenOdd bool) {
	b.tiles = b.tiles[:0]
	b.outCurves = b.outCurves[:0]
	b.above = b.above[:0]
	cellStart := 0
	for row := 0; row < grid.rows; row++ {
		b.runs = b.runs[:0]
		backdrop := int32(0)
		for column := 0; column < grid.columns; column++ {
			index := row*grid.columns + column
			if index < len(b.steps) {
				backdrop += b.steps[index]
			}
			cellEnd := cellStart
			if index < len(b.cellEnds) {
				cellEnd = b.cellEnds[index]
			}
			listStart, listEnd := cellStart, cellEnd
			cellStart = cellEnd

			cell := grid.cell(column, row)
			var topLeft, bottomRight [2]float32
			var haveQuad bool
			if listStart == listEnd {
				if fillsRule(backdrop, evenOdd) {
					topLeft = [2]float32{cell.left, cell.top}
					bottomRight = [2]float32{cell.right, cell.bottom}
					haveQuad = true
				}
			} else {
				m := newMeasure(cell, backdrop)
				for k := listStart; k < listEnd && k < len(b.keys); k++ {
					ci := int(uint32(b.keys[k]))
					if ci < len(b.input) {
						m.add(&b.input[ci])
					}
				}
				if l, r, ok := m.quad(evenOdd); ok {
					topLeft, bottomRight = l, r
					haveQuad = true
				}
			}
			if !haveQuad {
				continue
			}
			run := binRun{
				start:       column,
				end:         column + 1,
				backdrop:    backdrop,
				list:        [2]int{listStart, listEnd},
				topLeft:     topLeft,
				bottomRight: bottomRight,
				tile:        0,
			}
			if n := len(b.runs); n > 0 && b.runs[n-1].end == run.start && b.runs[n-1].shadesLike(&run, b.keys) {
				b.runs[n-1].end = run.end
				extendQuad(&b.runs[n-1].topLeft, &b.runs[n-1].bottomRight, &run)
			} else {
				b.runs = append(b.runs, run)
			}
		}

		aboveIdx := 0
		for ri := range b.runs {
			run := &b.runs[ri]
			for aboveIdx < len(b.above) && b.above[aboveIdx].start < run.start {
				aboveIdx++
			}
			var matched *binRun
			if aboveIdx < len(b.above) {
				cand := &b.above[aboveIdx]
				if cand.start == run.start && cand.end == run.end && cand.shadesLike(run, b.keys) {
					matched = cand
				}
			}
			if matched != nil {
				if t := &b.tiles[matched.tile]; t != nil {
					// Upstream grows the matching tile's quad to cover the run.
					extendQuad(&t.TopLeft, &t.BottomRight, run)
				}
				run.tile = matched.tile
			} else {
				curveStart := len(b.outCurves)
				const boxMargin = pathBinMargin + 1.0/16.0
				cTopLeft := [2]float32{float32(math.Inf(1)), float32(math.Inf(1))}
				cBotRight := [2]float32{float32(math.Inf(-1)), float32(math.Inf(-1))}
				for k := run.list[0]; k < run.list[1] && k < len(b.keys); k++ {
					ci := int(uint32(b.keys[k]))
					if ci >= len(b.input) {
						continue
					}
					curve := &b.input[ci]
					loX, hiX := curveMinMaxX(curve)
					if loX-boxMargin < cTopLeft[0] {
						cTopLeft[0] = loX - boxMargin
					}
					if curve.P0.Y-boxMargin < cTopLeft[1] {
						cTopLeft[1] = curve.P0.Y - boxMargin
					}
					if hiX+boxMargin > cBotRight[0] {
						cBotRight[0] = hiX + boxMargin
					}
					if curve.P1.Y+boxMargin > cBotRight[1] {
						cBotRight[1] = curve.P1.Y + boxMargin
					}
					b.outCurves = append(b.outCurves, *curve)
				}
				b.tiles = append(b.tiles, PathTileGpu{
					Paint:          0,
					CurveStart:     uint32(curveStart),
					CurveCount:     uint32(len(b.outCurves) - curveStart),
					TopLeft:        run.topLeft,
					BottomRight:    run.bottomRight,
					Reference:      [2]float32{grid.x(run.start), grid.y(row)},
					Backdrop:       run.backdrop,
					CurvesTopLeft:  cTopLeft,
					CurvesBotRight: cBotRight,
				})
				run.tile = len(b.tiles) - 1
			}
		}
		b.above, b.runs = b.runs, b.above
	}
}

type binRun struct {
	start, end  int
	backdrop    int32
	list        [2]int
	topLeft     [2]float32
	bottomRight [2]float32
	tile        int
}

func (r *binRun) shadesLike(other *binRun, keys []uint64) bool {
	if r.backdrop != other.backdrop {
		return false
	}
	a := keys[r.list[0]:min2(r.list[1], len(keys))]
	b := keys[other.list[0]:min2(other.list[1], len(keys))]
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func extendQuad(topLeft, bottomRight *[2]float32, run *binRun) {
	if run.topLeft[0] < topLeft[0] {
		topLeft[0] = run.topLeft[0]
	}
	if run.topLeft[1] < topLeft[1] {
		topLeft[1] = run.topLeft[1]
	}
	if run.bottomRight[0] > bottomRight[0] {
		bottomRight[0] = run.bottomRight[0]
	}
	if run.bottomRight[1] > bottomRight[1] {
		bottomRight[1] = run.bottomRight[1]
	}
}

func fillsRule(winding int32, evenOdd bool) bool {
	if evenOdd {
		return winding%2 != 0
	}
	return winding != 0
}

func orderedBits(x float32) uint32 {
	bits := math.Float32bits(x)
	if bits>>31 == 1 {
		return ^bits
	}
	return bits | 1<<31
}

// ---------------------------------------------------------------------------
// Grid
// ---------------------------------------------------------------------------

type binGrid struct {
	left, top     float32
	cellSize      float32
	columns, rows int
}

func coveringGrid(minX, minY, maxX, maxY float32, curves int, length float32) (*binGrid, bool) {
	left := float32(math.Floor(float64(minX - pathBinMargin)))
	top := float32(math.Floor(float64(minY - pathBinMargin)))
	sx := maxX + pathBinMargin - left
	sy := maxY + pathBinMargin - top
	if math.IsInf(float64(sx), 0) || math.IsNaN(float64(sx)) ||
		math.IsInf(float64(sy), 0) || math.IsNaN(float64(sy)) {
		return nil, false
	}
	sizeX, sizeY := sx, sy
	cellSize := pickCellSize(float32(curves), length, sizeX, sizeY)
	return &binGrid{
		left:     left,
		top:      top,
		cellSize: cellSize,
		columns:  int(sizeX/cellSize) + 1,
		rows:     int(sizeY/cellSize) + 1,
	}, true
}

// pickCellSize ports upstream cell_size: the power-of-two size minimizing the
// estimated binning + shading cost.
func pickCellSize(curves float32, length, sizeX, sizeY float32) float32 {
	const (
		listingCPUCost   = float32(320.0)
		emptyCellCPUCost = float32(95.0)
		minCellSize      = float32(16.0)
		maxGridCells     = float32(4096.0)
	)
	gridCells := func(cellSize float32) float32 {
		return (floor32(sizeX/cellSize) + 1.0) * (floor32(sizeY/cellSize) + 1.0)
	}
	extent := maxf(sizeX, sizeY)
	cellSize := minCellSize
	for gridCells(cellSize) > maxGridCells {
		cellSize *= 2.0
	}
	bestCost := float32(math.Inf(1))
	bestSize := cellSize
	for {
		var listings float32
		if cellSize >= extent {
			listings = curves
		} else {
			listings = curves + length/cellSize
		}
		area := minf(cellSize, sizeX) * minf(cellSize, sizeY) / 2.0
		emptyCells := maxf(gridCells(cellSize)-listings, 0.0)
		cost := listings*(listingCPUCost+area) + emptyCells*emptyCellCPUCost
		if cost < bestCost {
			bestCost = cost
			bestSize = cellSize
		}
		if cellSize >= extent {
			return bestSize
		}
		cellSize *= 2.0
	}
}

// fastClamp mirrors Rust fast_clamp: NaN clamps to lo (both comparisons are
// false for NaN, so the max branch returns lo), matching upstream.
func fastClamp(v, lo, hi float32) float32 {
	if v > lo {
		if v < hi {
			return v
		}
		return hi
	}
	return lo
}

type binCell struct {
	left, top, right, bottom float32
}

func (g *binGrid) columnOf(x float32) int {
	column := (x - g.left) / g.cellSize
	return int(fastClamp(column, 0, float32(g.columns-1)))
}

func (g *binGrid) rowOf(y float32) int {
	row := (y - g.top) / g.cellSize
	return int(fastClamp(row, 0, float32(g.rows-1)))
}

func (g *binGrid) x(column int) float32 { return g.left + float32(column)*g.cellSize }
func (g *binGrid) y(row int) float32    { return g.top + float32(row)*g.cellSize }

func (g *binGrid) cell(column, row int) binCell {
	return binCell{left: g.x(column), top: g.y(row), right: g.x(column + 1), bottom: g.y(row + 1)}
}

// ---------------------------------------------------------------------------
// Measure (trim a cell's quad)
// ---------------------------------------------------------------------------

type binMeasure struct {
	cell                        binCell
	backdrop                    int32
	minX, minY, maxX, maxY      float32
	rightWinding, bottomWinding int32
}

func newMeasure(cell binCell, backdrop int32) *binMeasure {
	return &binMeasure{
		cell:          cell,
		backdrop:      backdrop,
		minX:          float32(math.Inf(1)),
		minY:          float32(math.Inf(1)),
		maxX:          float32(math.Inf(-1)),
		maxY:          float32(math.Inf(-1)),
		rightWinding:  backdrop,
		bottomWinding: backdrop,
	}
}

func (m *binMeasure) add(curve *PathCurve) {
	cell := m.cell
	loX, hiX := curveMinMaxX(curve)
	if loX < m.minX {
		m.minX = loX
	}
	if hiX > m.maxX {
		m.maxX = hiX
	}
	if curve.P0.Y < m.minY {
		m.minY = curve.P0.Y
	}
	if curve.P1.Y > m.maxY {
		m.maxY = curve.P1.Y
	}
	if curveCrosses(curve, cell.top) {
		m.rightWinding += curveStep(curve)
	}
	step := curveStep(curve)
	if curve.P0.X <= cell.left && curve.P0.Y > cell.top {
		m.bottomWinding -= step
	}
	if curve.P1.X <= cell.left && curve.P1.Y > cell.top {
		m.bottomWinding += step
	}
}

func (m *binMeasure) quad(evenOdd bool) ([2]float32, [2]float32, bool) {
	cell := m.cell
	unfilled := !fillsRule(m.backdrop, evenOdd)
	left := cell.left
	top := cell.top
	if unfilled {
		left = fastClamp(m.minX-pathBinMargin, cell.left, cell.right)
		top = fastClamp(m.minY-pathBinMargin, cell.top, cell.bottom)
	}
	right := cell.right
	bottom := cell.bottom
	if fillsRule(m.rightWinding, evenOdd) {
		right = cell.right
	} else {
		right = fastClamp(m.maxX+pathBinMargin, cell.left, cell.right)
	}
	if !fillsRule(m.bottomWinding, evenOdd) {
		bottom = fastClamp(m.maxY+pathBinMargin, cell.top, cell.bottom)
	}
	if left < right && top < bottom {
		return [2]float32{left, top}, [2]float32{right, bottom}, true
	}
	return [2]float32{}, [2]float32{}, false
}

// ---------------------------------------------------------------------------
// Curve helpers (mirror the Rust impl on PathCurve)
// ---------------------------------------------------------------------------

func curveMinMaxX(c *PathCurve) (lo, hi float32) {
	lo = minf(c.P0.X, c.P1.X)
	hi = maxf(c.P0.X, c.P1.X)
	return
}

func curveStep(c *PathCurve) int32 {
	if c.Sign < 0 {
		return -1
	}
	return 1
}

func curveCrosses(c *PathCurve, y float32) bool {
	return c.P0.Y <= y && y < c.P1.Y
}

func curveXRange(c *PathCurve, top, bottom float32) (lo, hi float32) {
	top = maxf(top, c.P0.Y)
	bottom = minf(bottom, c.P1.Y)
	var xTop, xBottom float32
	if top <= c.P0.Y {
		xTop = c.P0.X
	} else {
		xTop = curveXAtY(c, top)
	}
	if bottom >= c.P1.Y {
		xBottom = c.P1.X
	} else {
		xBottom = curveXAtY(c, bottom)
	}
	minX := minf(c.P0.X, c.P1.X)
	maxX := maxf(c.P0.X, c.P1.X)
	lo = minf(xTop, xBottom)
	if lo < minX {
		lo = minX
	}
	hi = maxf(xTop, xBottom)
	if hi > maxX {
		hi = maxX
	}
	return lo, hi
}

func curveXAtY(c *PathCurve, y float32) float32 {
	if c.A.Y == 0 {
		if c.B.Y == 0 {
			return c.P0.X
		}
		t := fastClamp((y-c.P0.Y)/c.B.Y, 0, 1)
		return (c.A.X*t+c.B.X)*t + c.P0.X
	}
	t := monotoneQuadraticRoot(c.A.Y, c.B.Y, c.P0.Y-y, 1.0)
	return (c.A.X*t+c.B.X)*t + c.P0.X
}

// monotoneQuadraticRoot solves a*t^2 + b*t + c = 0 for the root in [0, 1].
func monotoneQuadraticRoot(a, b, c, direction float32) float32 {
	disc := b*b - 4*a*c
	if disc < 0 {
		disc = 0
	}
	denominator := b + direction*sqrt32(disc)
	if denominator == 0 {
		return 0
	}
	return fastClamp(-2*c/denominator, 0, 1)
}

// ---------------------------------------------------------------------------
// float helpers (minf/maxf/clampf/abs32 live in path.go/shadow.go/gradient.go)
// ---------------------------------------------------------------------------

func floor32(v float32) float32 { return float32(math.Floor(float64(v))) }
func sqrt32(v float32) float32  { return float32(math.Sqrt(float64(v))) }
