package text

import (
	"github.com/FlameInTheDark/go-gpui/text/sfnt"
)

// RasterizeGlyph renders a glyph to an 8-bit alpha mask at a pixel size
// with a subpixel x offset (0..1). TrueType quadratic outlines (and CFF
// outlines via the sfnt package) are flattened to line segments and filled
// with an analytic scanline (nonzero) rule.
func RasterizeGlyph(face *sfnt.Font, gid uint16, sizePx float32, subpixelX float32) (w, h int, bearingX, bearingY float32, mask []uint8) {
	outline := face.GlyphOutline(gid)
	if len(outline.Points) == 0 {
		return 0, 0, 0, 0, nil
	}
	upem := float32(face.UnitsPerEm)
	if upem == 0 {
		upem = 1000
	}
	scale := sizePx / upem

	pts := make([]ptT, len(outline.Points))
	for i, p := range outline.Points {
		pts[i] = ptT{x: p.X*scale + subpixelX, y: -p.Y * scale}
	}

	minX, minY := pts[0].x, pts[0].y
	maxX, maxY := minX, minY
	for _, p := range pts {
		minX = minF(minX, p.x)
		minY = minF(minY, p.y)
		maxX = maxF(maxX, p.x)
		maxY = maxF(maxY, p.y)
	}
	bx := int(floorF(minX)) - 1
	by := int(floorF(minY)) - 1
	tx := int(ceilF(maxX)) + 1
	ty := int(ceilF(maxY)) + 1
	w = tx - bx
	h = ty - by
	if w <= 0 || h <= 0 || w > 4096 || h > 4096 {
		return 0, 0, 0, 0, nil
	}
	bearingX = float32(bx)
	bearingY = float32(by)

	// Flatten contours into edges, translated into raster-local
	// coordinates (the y flip makes above-baseline coords negative).
	var edges []edge
	contourStart := 0
	for _, end := range outline.Ends {
		local := make([]ptT, end+1-contourStart)
		for i, p := range pts[contourStart : end+1] {
			local[i] = ptT{x: p.x - float32(bx), y: p.y - float32(by)}
		}
		edges = flattenContour(edges, local, outline.OnCurve[contourStart:end+1])
		contourStart = end + 1
	}

	coverage := make([]float32, w*h)
	fillScanlines(coverage, w, h, edges)

	mask = make([]uint8, w*h)
	for i, c := range coverage {
		a := c
		if a < 0 {
			a = -a
		}
		if a > 1 {
			a = 1
		}
		mask[i] = uint8(a*255 + 0.5)
	}
	return w, h, bearingX, bearingY, mask
}

type ptT struct{ x, y float32 }

type edge struct {
	x0, y0, x1, y1 float32
}

// flattenContour converts one TrueType contour (alternating on/off-curve
// points, implicitly closed) into line edges, flattening quadratic spans.
func flattenContour(edges []edge, pts []ptT, onCurve []bool) []edge {
	n := len(pts)
	if n == 0 {
		return edges
	}
	if n == 1 {
		return edges
	}
	addLine := func(a, b ptT) {
		if a != b {
			edges = append(edges, edge{a.x, a.y, b.x, b.y})
		}
	}
	addQuad := func(p0, c, p1 ptT) {
		edges = flattenQuad(edges, p0, c, p1, 0)
	}

	// Find the first on-curve point, or synthesize one.
	firstOn := -1
	for i := 0; i < n; i++ {
		if onCurve[i] {
			firstOn = i
			break
		}
	}
	var start ptT
	if firstOn == -1 {
		// All off-curve: the contour starts at the midpoint of the last and
		// first control points.
		start = ptT{(pts[n-1].x + pts[0].x) / 2, (pts[n-1].y + pts[0].y) / 2}
	} else {
		start = pts[firstOn]
	}

	cur := start
	var pending []ptT
	flush := func(endPt ptT) {
		prev := cur
		for i, cp := range pending {
			var next ptT
			if i+1 < len(pending) {
				next = ptT{(cp.x + pending[i+1].x) / 2, (cp.y + pending[i+1].y) / 2}
			} else {
				next = endPt
			}
			addQuad(prev, cp, next)
			prev = next
		}
		pending = pending[:0]
		cur = endPt
	}

	for k := 0; k < n; k++ {
		i := (firstOn + 1 + k) % n
		if firstOn == -1 {
			i = k
		}
		if onCurve[i] {
			if len(pending) == 0 {
				addLine(cur, pts[i])
			}
			flush(pts[i])
		} else {
			pending = append(pending, pts[i])
		}
	}
	// Close back to start.
	if len(pending) > 0 {
		flush(start)
	} else if cur != start {
		addLine(cur, start)
	}
	return edges
}

// flattenQuad subdivides a quadratic bezier until flat, appending edges.
func flattenQuad(edges []edge, p0, c, p1 ptT, depth int) []edge {
	// Flatness: distance of the control point from the chord.
	mid := ptT{(p0.x + p1.x) / 2, (p0.y + p1.y) / 2}
	dx, dy := c.x-mid.x, c.y-mid.y
	if depth >= 10 || (dx*dx+dy*dy) < 0.01 {
		edges = append(edges, edge{p0.x, p0.y, p1.x, p1.y})
		return edges
	}
	c01 := ptT{(p0.x + c.x) / 2, (p0.y + c.y) / 2}
	c12 := ptT{(c.x + p1.x) / 2, (c.y + p1.y) / 2}
	m := ptT{(c01.x + c12.x) / 2, (c01.y + c12.y) / 2}
	edges = flattenQuad(edges, p0, c01, m, depth+1)
	edges = flattenQuad(edges, m, c12, p1, depth+1)
	return edges
}

func floorF(v float32) float32 {
	if v < 0 {
		i := int(v)
		if float32(i) != v {
			return float32(i - 1)
		}
		return v
	}
	return float32(int(v))
}

func ceilF(v float32) float32 {
	return -floorF(-v)
}

// fillScanlines rasterizes edges into per-pixel signed coverage using the
// nonzero winding rule.
//
// Each pixel row is sampled on its midline (row + 0.5). An edge crosses the
// sample line iff its y-span contains it (half-open [yMin, yMax), so
// vertices are counted exactly once). Crossings are sorted by x and the
// winding is integrated across the row, adding winding * horizontal overlap
// per pixel. This keeps the up/down crossing balance intact for any closed
// contour, including C-shaped stems whose transitions land mid-row.
func fillScanlines(coverage []float32, w, h int, edges []edge) {
	type crossing struct {
		x   float32
		dir float32
	}
	crossings := make([]crossing, 0, 16)
	for row := 0; row < h; row++ {
		s := float32(row) + 0.5
		crossings = crossings[:0]
		for _, e := range edges {
			if e.y0 == e.y1 {
				continue
			}
			yMin, yMax := e.y0, e.y1
			dir := float32(1)
			if yMax < yMin {
				yMin, yMax = yMax, yMin
				dir = -1
			}
			if s < yMin || s >= yMax {
				continue
			}
			t := (s - e.y0) / (e.y1 - e.y0)
			x := e.x0 + (e.x1-e.x0)*t
			crossings = append(crossings, crossing{x: x, dir: dir})
		}
		if len(crossings) == 0 {
			continue
		}
		// Sort crossings by x (insertion sort: crossing counts are tiny).
		for i := 1; i < len(crossings); i++ {
			c := crossings[i]
			j := i - 1
			for j >= 0 && crossings[j].x > c.x {
				crossings[j+1] = crossings[j]
				j--
			}
			crossings[j+1] = c
		}
		base := row * w
		wind := float32(0)
		segStart := float32(0)
		for _, c := range crossings {
			addWindSegment(coverage, base, w, segStart, c.x, wind)
			wind += c.dir
			segStart = c.x
		}
		addWindSegment(coverage, base, w, segStart, float32(w), wind)
	}
}

// addWindSegment adds wind * [xa, xb) horizontal overlap to the row's
// coverage, clamped to the raster width.
func addWindSegment(coverage []float32, base, w int, xa, xb, wind float32) {
	if wind == 0 || xb <= xa {
		return
	}
	if xa < 0 {
		xa = 0
	}
	if xb > float32(w) {
		xb = float32(w)
	}
	if xb <= xa {
		return
	}
	x0 := int(floorF(xa))
	x1 := int(ceilF(xb))
	for x := x0; x <= x1 && x < w; x++ {
		lo := maxF(xa, float32(x))
		hi := minF(xb, float32(x)+1)
		if hi > lo && x >= 0 {
			coverage[base+x] += wind * (hi - lo)
		}
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxF(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func minF(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}
