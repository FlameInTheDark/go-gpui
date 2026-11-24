package render

import (
	"math"

	"github.com/FlameInTheDark/go-gpui/geometry"
)

// PathBuilder builds Paths with SVG semantics, ported from gpui's
// path_builder.rs (which wraps lyon). Curves are split into xy-monotone
// quadratic segments oriented downward, exactly like the upstream Outline.
type PathBuilder struct {
	curves    []PathCurve
	start     *geometry.Point[float32]
	last      geometry.Point[float32]
	nonFinite bool
	closed    bool

	strokeWidth float32
	strokeDash  []float32
}

// NewPathBuilder creates a fill builder.
func NewPathBuilder() *PathBuilder { return &PathBuilder{} }

func finite(p geometry.Point[float32]) bool {
	return !math.IsNaN(float64(p.X)) && !math.IsInf(float64(p.X), 0) &&
		!math.IsNaN(float64(p.Y)) && !math.IsInf(float64(p.Y), 0)
}

// MoveTo closes the current contour and starts a new one.
func (b *PathBuilder) MoveTo(p geometry.Point[float32]) {
	if !finite(p) {
		b.nonFinite = true
		return
	}
	b.closeContour()
	pt := p
	b.start = &pt
	b.last = p
}

func (b *PathBuilder) closeContour() {
	if b.start != nil && (b.last.X != b.start.X || b.last.Y != b.start.Y) {
		b.lineTo(*b.start)
	}
	b.start = nil
}

// LineTo appends a line segment.
func (b *PathBuilder) LineTo(p geometry.Point[float32]) {
	if !finite(p) {
		b.nonFinite = true
		return
	}
	if b.start == nil {
		b.MoveTo(p)
		return
	}
	if p.X == b.last.X && p.Y == b.last.Y {
		return // skip duplicate points like upstream
	}
	b.pushQuad(b.last, p, p)
	b.last = p
}

func (b *PathBuilder) lineTo(p geometry.Point[float32]) {
	if p.X == b.last.X && p.Y == b.last.Y {
		return
	}
	b.pushQuad(b.last, p, p)
	b.last = p
}

// QuadTo appends a quadratic bezier from the current point through ctrl to to.
func (b *PathBuilder) QuadTo(ctrl, to geometry.Point[float32]) {
	if !finite(ctrl) || !finite(to) {
		b.nonFinite = true
		return
	}
	if b.start == nil {
		b.MoveTo(to)
		return
	}
	// Clamp control point into the endpoint box (upstream behavior).
	c := ctrl
	if c.X < minf(b.last.X, to.X) {
		c.X = minf(b.last.X, to.X)
	}
	if c.X > maxf(b.last.X, to.X) {
		c.X = maxf(b.last.X, to.X)
	}
	if c.Y < minf(b.last.Y, to.Y) {
		c.Y = minf(b.last.Y, to.Y)
	}
	if c.Y > maxf(b.last.Y, to.Y) {
		c.Y = maxf(b.last.Y, to.Y)
	}
	// Split at y extrema into monotone pieces.
	p0 := b.last
	b.quadMonotoneY(p0, c, to)
	b.last = to
}

// quadMonotoneY splits a quadratic at vertical extrema and emits pieces.
func (b *PathBuilder) quadMonotoneY(p0, c, p1 geometry.Point[float32]) {
	// y(t) = (p0 - 2c + p1) t^2 + 2(c - p0) t + p0
	a := 2*c.Y - p0.Y - p1.Y
	q := 2 * (c.Y - p0.Y)
	var ts []float32
	if a != 0 {
		t := -q / (2 * a)
		if t > 0 && t < 1 {
			ts = append(ts, t)
		}
	}
	b.emitQuadPieces(p0, c, p1, ts)
}

// emitQuadPieces emits quadratic pieces split at the given t values,
// splitting again for x monotonicity within each piece.
func (b *PathBuilder) emitQuadPieces(p0, c, p1 geometry.Point[float32], splits []float32) {
	if len(splits) == 0 {
		b.emitXMono(p0, c, p1)
		return
	}
	t := splits[0]
	// de Casteljau split at t
	c01 := lerpP(p0, c, t)
	c12 := lerpP(c, p1, t)
	mid := lerpP(c01, c12, t)
	b.emitXMono(p0, c01, mid)
	b.emitQuadPieces(mid, c12, p1, splits[1:])
}

func (b *PathBuilder) emitXMono(p0, c, p1 geometry.Point[float32]) {
	a := 2*c.X - p0.X - p1.X
	q := 2 * (c.X - p0.X)
	var ts []float32
	if a != 0 {
		t := -q / (2 * a)
		if t > 0 && t < 1 {
			ts = append(ts, t)
		}
	}
	if len(ts) == 0 {
		b.pushQuad(p0, c, p1)
		return
	}
	t := ts[0]
	c01 := lerpP(p0, c, t)
	c12 := lerpP(c, p1, t)
	mid := lerpP(c01, c12, t)
	b.pushQuad(p0, c01, mid)
	b.pushQuad(mid, c12, p1)
}

// pushQuad stores a downward-oriented monotone quadratic.
func (b *PathBuilder) pushQuad(p0, c, p1 geometry.Point[float32]) {
	// Coefficients: point(t) = a t^2 + b t + p0.
	a := geometry.Point[float32]{
		X: p0.X - 2*c.X + p1.X,
		Y: p0.Y - 2*c.Y + p1.Y,
	}
	bb := geometry.Point[float32]{
		X: 2 * (c.X - p0.X),
		Y: 2 * (c.Y - p0.Y),
	}
	sign := float32(1)
	start, end := p0, p1
	if p1.Y < p0.Y {
		// Flip the parametrization so P0.y <= P1.y:
		// r(t) = q(1-t) = a t^2 + (-2a - b) t + q(1).
		sign = -1
		start, end = p1, p0
		bb = geometry.Point[float32]{X: -2*a.X - bb.X, Y: -2*a.Y - bb.Y}
	}
	b.curves = append(b.curves, PathCurve{P0: start, P1: end, A: a, B: bb, Sign: sign})
}

// CubicTo appends a cubic bezier, flattened to quadratics.
func (b *PathBuilder) CubicTo(ctrl1, ctrl2, to geometry.Point[float32]) {
	if !finite(ctrl1) || !finite(ctrl2) || !finite(to) {
		b.nonFinite = true
		return
	}
	if b.start == nil {
		b.MoveTo(to)
		return
	}
	// Flatten with adaptive subdivision at 0.25px tolerance (upstream).
	b.flattenCubic(b.last, ctrl1, ctrl2, to, 0)
	b.last = to
}

func (b *PathBuilder) flattenCubic(p0, c1, c2, p1 geometry.Point[float32], depth int) {
	// Control polygon deviation test.
	d1 := dist(c1, midp(p0, c1))
	d2 := dist(c2, midp(c1, c2))
	d3 := dist(p1, midp(c2, p1))
	da := dist(midp(p0, c1), midp(c1, c2))
	db := dist(midp(c1, c2), midp(c2, p1))
	err := 3*(da+db) - 2*(d1+d2+d3)
	if depth > 16 || err*err < 0.0625 { // 0.25px squared
		// Approximate with a single quadratic through the midpoints.
		qc := geometry.Point[float32]{X: (c1.X + c2.X) / 2, Y: (c1.Y + c2.Y) / 2}
		b.pushQuadSplitSafe(p0, qc, p1)
		return
	}
	m01 := midp(p0, c1)
	m12 := midp(c1, c2)
	m23 := midp(c2, p1)
	m012 := midp(m01, m12)
	m123 := midp(m12, m23)
	mid := midp(m012, m123)
	b.flattenCubic(p0, m01, m012, mid, depth+1)
	b.flattenCubic(mid, m123, m23, p1, depth+1)
}

func (b *PathBuilder) pushQuadSplitSafe(p0, c, p1 geometry.Point[float32]) {
	b.quadMonotoneY(p0, c, p1)
}

// ArcTo appends an elliptical arc (SVG path semantics).
func (b *PathBuilder) ArcTo(radii geometry.Size[float32], xRotationDeg float32, largeArc, sweep bool, to geometry.Point[float32]) {
	if b.last == to {
		return
	}
	if radii.Width == 0 || radii.Height == 0 {
		b.LineTo(to)
		return
	}
	SVGArcCubics(b.last, radii, xRotationDeg, largeArc, sweep, to, b.CubicTo)
}

// SVGArcCubics converts a non-degenerate SVG endpoint arc to cubic segments
// of at most 90 degrees. Degenerate arcs emit no cubics.
func SVGArcCubics(from geometry.Point[float32], radii geometry.Size[float32], xRotationDeg float32, largeArc, sweep bool, to geometry.Point[float32], emit func(c1, c2, end geometry.Point[float32])) {
	rx, ry := math.Abs(float64(radii.Width)), math.Abs(float64(radii.Height))
	if rx == 0 || ry == 0 || from == to {
		return
	}
	x0, y0, x1, y1 := float64(from.X), float64(from.Y), float64(to.X), float64(to.Y)
	sinPhi, cosPhi := math.Sincos(float64(xRotationDeg) * math.Pi / 180)
	dx2, dy2 := (x0-x1)/2, (y0-y1)/2
	x1p := cosPhi*dx2 + sinPhi*dy2
	y1p := -sinPhi*dx2 + cosPhi*dy2
	ux, uy := x1p/rx, y1p/ry
	lambda := ux*ux + uy*uy
	if lambda > 1 {
		s := math.Sqrt(lambda)
		rx *= s
		ry *= s
		ux, uy = ux/s, uy/s
		lambda = 1
	}
	co := math.Sqrt(max(0, (1-lambda)/lambda))
	if largeArc == sweep {
		co = -co
	}
	cxp, cyp := co*uy, -co*ux
	cx := cosPhi*(rx*cxp) - sinPhi*(ry*cyp) + (x0+x1)/2
	cy := sinPhi*(rx*cxp) + cosPhi*(ry*cyp) + (y0+y1)/2
	ang := math.Atan2(uy-cyp, ux-cxp)
	span := math.Atan2(-uy-cyp, -ux-cxp) - ang
	if !sweep && span > 0 {
		span -= 2 * math.Pi
	}
	if sweep && span < 0 {
		span += 2 * math.Pi
	}
	segs := max(1, int(math.Ceil(math.Abs(span)/(math.Pi/2))))
	delta := span / float64(segs)
	k := 4.0 / 3 * math.Tan(delta/4)
	pointAndTangent := func(a float64) (x, y, dx, dy float64) {
		s, c := math.Sincos(a)
		return cx + cosPhi*rx*c - sinPhi*ry*s, cy + sinPhi*rx*c + cosPhi*ry*s,
			-cosPhi*rx*s - sinPhi*ry*c, -sinPhi*rx*s + cosPhi*ry*c
	}
	x, y := x0, y0
	_, _, dx, dy := pointAndTangent(ang)
	for i := 0; i < segs; i++ {
		nx, ny, ndx, ndy := pointAndTangent(ang + delta*float64(i+1))
		if i == segs-1 {
			nx, ny = x1, y1
		}
		emit(geometry.Point[float32]{X: float32(x + k*dx), Y: float32(y + k*dy)},
			geometry.Point[float32]{X: float32(nx - k*ndx), Y: float32(ny - k*ndy)},
			geometry.Point[float32]{X: float32(nx), Y: float32(ny)})
		x, y, dx, dy = nx, ny, ndx, ndy
	}
}

// Close closes the current contour.
func (b *PathBuilder) Close() {
	b.closeContour()
	b.closed = true
}

// AddPolygon appends a polygon (optionally closed).
func (b *PathBuilder) AddPolygon(points []geometry.Point[float32], closed bool) {
	if len(points) == 0 {
		return
	}
	b.MoveTo(points[0])
	for _, p := range points[1:] {
		b.LineTo(p)
	}
	if closed {
		b.Close()
	}
}

// AddCircle appends a circle as four quadratics.
func (b *PathBuilder) AddCircle(center geometry.Point[float32], radius float32) {
	k := float32(0.5522847498307936) // 4/3 * (sqrt(2)-1)
	r := radius
	b.MoveTo(geometry.Point[float32]{X: center.X - r, Y: center.Y})
	b.QuadTo(geometry.Point[float32]{X: center.X - r, Y: center.Y - r*k}, geometry.Point[float32]{X: center.X - r*k, Y: center.Y - r})
	b.QuadTo(geometry.Point[float32]{X: center.X, Y: center.Y - 2*r*k*0}, geometry.Point[float32]{X: center.X + r*k, Y: center.Y - r})
	b.QuadTo(geometry.Point[float32]{X: center.X + r, Y: center.Y - r*k*0}, geometry.Point[float32]{X: center.X + r, Y: center.Y})
	b.QuadTo(geometry.Point[float32]{X: center.X + r, Y: center.Y + r*k*0}, geometry.Point[float32]{X: center.X + r*k, Y: center.Y + r})
	b.QuadTo(geometry.Point[float32]{X: center.X, Y: center.Y + 2*r*k*0}, geometry.Point[float32]{X: center.X - r*k, Y: center.Y + r})
	b.QuadTo(geometry.Point[float32]{X: center.X - r, Y: center.Y + r*k*0}, geometry.Point[float32]{X: center.X - r, Y: center.Y})
	b.Close()
}

// AddEllipse appends an axis-aligned ellipse.
func (b *PathBuilder) AddEllipse(center geometry.Point[float32], rx, ry float32) {
	k := float32(0.5522847498307936)
	b.MoveTo(geometry.Point[float32]{X: center.X - rx, Y: center.Y})
	b.QuadTo(geometry.Point[float32]{X: center.X - rx, Y: center.Y - ry*k}, geometry.Point[float32]{X: center.X - rx*k, Y: center.Y - ry})
	b.QuadTo(geometry.Point[float32]{X: center.X, Y: center.Y - 2*ry*k*0}, geometry.Point[float32]{X: center.X + rx*k, Y: center.Y - ry})
	b.QuadTo(geometry.Point[float32]{X: center.X + rx, Y: center.Y - ry*k*0}, geometry.Point[float32]{X: center.X + rx, Y: center.Y})
	b.QuadTo(geometry.Point[float32]{X: center.X + rx, Y: center.Y + ry*k*0}, geometry.Point[float32]{X: center.X + rx*k, Y: center.Y + ry})
	b.QuadTo(geometry.Point[float32]{X: center.X, Y: center.Y + 2*ry*k*0}, geometry.Point[float32]{X: center.X - rx*k, Y: center.Y + ry})
	b.QuadTo(geometry.Point[float32]{X: center.X - rx, Y: center.Y + ry*k*0}, geometry.Point[float32]{X: center.X - rx, Y: center.Y})
	b.Close()
}

// Build finalizes the path. ok is false when any point was non-finite (the
// whole path is rejected, like upstream).
func (b *PathBuilder) Build() (*Path, bool) {
	if b.nonFinite {
		return nil, false
	}
	if b.strokeWidth > 0 {
		// Stroked paths keep their natural (possibly open) contours:
		// auto-closing would add a spurious segment back to the start
		// (an open checkmark would become a triangle).
		return b.buildStrokeOutline()
	}
	b.closeContour()
	curves := append([]PathCurve(nil), b.curves...)
	return &Path{Curves: curves}, true
}

// StrokeStyle configures stroke-to-outline conversion (upstream
// PathBuilder::stroke / tiny-skia stroking; this port approximates with
// round joins and round caps, which union cleanly under the nonzero rule).
type StrokeStyle struct {
	Width float32
	// Dash, when non-empty, alternates on/off lengths along the path.
	Dash []float32
}

// Stroke converts the builder to a stroked path: Build() returns the fill
// outline of the stroke (upstream PathBuilder::stroke).
func (b *PathBuilder) Stroke(width float32) *PathBuilder {
	b.strokeWidth = width
	return b
}

// WithDash sets a dash pattern for stroked paths (upstream with_dash_array).
func (b *PathBuilder) WithDash(dash []float32) *PathBuilder {
	b.strokeDash = append([]float32(nil), dash...)
	return b
}

// buildStrokeOutline converts the builder's curves into the closed outline
// of a stroked polyline with round joins and caps.
func (b *PathBuilder) buildStrokeOutline() (*Path, bool) {
	if b.nonFinite {
		return nil, false
	}
	out := NewPathBuilder()
	contours := b.contours()
	for _, pts := range contours {
		strokeContour(out, pts, b.strokeWidth/2, b.strokeDash)
	}
	return out.buildFill()
}

func (b *PathBuilder) buildFill() (*Path, bool) {
	curves := append([]PathCurve(nil), b.curves...)
	return &Path{Curves: curves}, true
}

// contours recovers point lists from the flattened curves (each MoveTo
// starts a contour). Curves are stored direction-normalized; Sign recovers
// the original direction.
func (b *PathBuilder) contours() [][]geometry.Point[float32] {
	var out [][]geometry.Point[float32]
	var cur []geometry.Point[float32]
	for _, c := range b.curves {
		start, end := c.P0, c.P1
		if c.Sign < 0 {
			start, end = c.P1, c.P0
		}
		if len(cur) == 0 || cur[len(cur)-1] != start {
			if len(cur) > 1 {
				out = append(out, cur)
			}
			cur = []geometry.Point[float32]{start}
		}
		if cur[len(cur)-1] != end {
			cur = append(cur, end)
		}
	}
	if len(cur) > 1 {
		out = append(out, cur)
	}
	return out
}

// strokeContour appends the stroke outline of one polyline to out.
// Joins: the original vertex is inserted between offset sides so convex
// notches are covered (overlaps union under nonzero winding).
func strokeContour(out *PathBuilder, pts []geometry.Point[float32], halfW float32, dash []float32) {
	if len(pts) < 2 || halfW <= 0 {
		return
	}
	// Optional dashing: split the polyline into dashes first.
	polys := [][]geometry.Point[float32]{pts}
	if len(dash) > 0 {
		polys = dashPolyline(pts, dash)
	}
	for _, poly := range polys {
		strokeSolidContour(out, poly, halfW)
	}
}

func strokeSolidContour(out *PathBuilder, pts []geometry.Point[float32], halfW float32) {
	if len(pts) < 2 || halfW <= 0 {
		return
	}
	// The stroke is built as a UNION of per-segment rectangles and
	// per-vertex discs, all sharing one winding so the nonzero fill rule
	// merges them into a seamless outline. (The previous single-offset-
	// polyline walk skipped alternating offset points at every join, which
	// left diagonal spikes and missing corners on multi-segment strokes -
	// at icon sizes the shape collapsed into corner-bracket fragments.)
	for i := 0; i+1 < len(pts); i++ {
		a, c := pts[i], pts[i+1]
		dx, dy := c.X-a.X, c.Y-a.Y
		l := float32(math.Sqrt(float64(dx*dx + dy*dy)))
		if l == 0 {
			continue
		}
		nx, ny := -dy/l*halfW, dx/l*halfW
		emitStrokePolygon(out, []geometry.Point[float32]{
			{X: a.X + nx, Y: a.Y + ny},
			{X: c.X + nx, Y: c.Y + ny},
			{X: c.X - nx, Y: c.Y - ny},
			{X: a.X - nx, Y: a.Y - ny},
		})
	}
	// Round caps at the ends; round joins only where the turn is sharp
	// enough that the quad union alone would leave a visible notch (flat
	// continuations keep the stroke cheap for long flattened paths).
	for i := range pts {
		if i > 0 && i < len(pts)-1 && shallowJoin(pts[i-1], pts[i], pts[i+1]) {
			continue
		}
		emitStrokeDisc(out, pts[i], halfW)
	}
}

// shallowJoin reports whether the turn at b between a-b and b-c is so small
// that the two segment rectangles already overlap across the joint.
func shallowJoin(a, b, c geometry.Point[float32]) bool {
	ax, ay := b.X-a.X, b.Y-a.Y
	bx, by := c.X-b.X, c.Y-b.Y
	la := float32(math.Sqrt(float64(ax*ax + ay*ay)))
	lb := float32(math.Sqrt(float64(bx*bx + by*by)))
	if la == 0 || lb == 0 {
		return true
	}
	// cos of the turn angle; > 0.995 (~5.7 degrees) counts as straight.
	cos := (ax*bx + ay*by) / (la * lb)
	return cos > 0.995
}

// strokeWindingTarget is the signed area (shoelace) sign every stroke
// contour is normalized to. The per-segment rectangles come out negative
// (clockwise); discs are flipped to match so overlapping contours add up
// under the nonzero rule instead of cancelling.
const strokeWindingTarget = -1.0

// emitStrokePolygon appends one closed polygon contour to out, reversed if
// needed so all stroke contours share the target winding.
func emitStrokePolygon(out *PathBuilder, poly []geometry.Point[float32]) {
	if len(poly) < 3 {
		return
	}
	var area float32
	for i := 0; i < len(poly); i++ {
		a := poly[i]
		c := poly[(i+1)%len(poly)]
		area += a.X*c.Y - c.X*a.Y
	}
	sign := float32(-1)
	if area > 0 {
		sign = 1
	}
	if sign != strokeWindingTarget { // wrong direction: walk it backwards
		for i := len(poly) - 1; i >= 0; i-- {
			if i == len(poly)-1 {
				out.MoveTo(poly[i])
			} else {
				out.LineTo(poly[i])
			}
		}
	} else {
		out.MoveTo(poly[0])
		for i := 1; i < len(poly); i++ {
			out.LineTo(poly[i])
		}
	}
	out.Close()
}

// emitStrokeDisc appends a filled disc (polygon approximation) with the
// stroke winding (round joins and caps).
func emitStrokeDisc(out *PathBuilder, p geometry.Point[float32], r float32) {
	const segments = 12
	poly := make([]geometry.Point[float32], segments)
	for i := 0; i < segments; i++ {
		a := (float32(2) * math.Pi) * float32(i) / segments
		poly[i] = geometry.Point[float32]{X: p.X + r*float32(math.Cos(float64(a))), Y: p.Y + r*float32(math.Sin(float64(a)))}
	}
	emitStrokePolygon(out, poly)
}

// dashPolyline splits a polyline into dashes (on/off lengths alternating).
func dashPolyline(pts []geometry.Point[float32], dash []float32) [][]geometry.Point[float32] {
	var out [][]geometry.Point[float32]
	di := 0
	on := true
	remaining := dash[0]
	if remaining <= 0 {
		return [][]geometry.Point[float32]{pts}
	}
	var cur []geometry.Point[float32]
	cur = append(cur, pts[0])
	for i := 0; i+1 < len(pts); i++ {
		a, c := pts[i], pts[i+1]
		seg := float32(math.Sqrt(float64((c.X-a.X)*(c.X-a.X) + (c.Y-a.Y)*(c.Y-a.Y))))
		for seg > 0 {
			if seg < remaining {
				if on {
					cur = append(cur, c)
				} else {
					cur = []geometry.Point[float32]{c}
				}
				remaining -= seg
				seg = 0
			} else {
				t := remaining / seg
				mid := geometry.Point[float32]{X: a.X + (c.X-a.X)*t, Y: a.Y + (c.Y-a.Y)*t}
				if on {
					cur = append(cur, mid)
					out = append(out, cur)
					cur = nil
				} else {
					cur = []geometry.Point[float32]{mid}
				}
				a = mid
				seg -= remaining
				di = (di + 1) % len(dash)
				remaining = dash[di]
				if remaining <= 0 {
					remaining = 0.01
				}
				on = !on
			}
		}
	}
	if on && len(cur) > 1 {
		out = append(out, cur)
	}
	return out
}

// BuildEvenOdd finalizes with the even-odd fill rule.
func (b *PathBuilder) BuildEvenOdd() (*Path, bool) {
	p, ok := b.Build()
	if p != nil {
		p.EvenOdd = true
	}
	return p, ok
}

// ScalePath returns a copy of the path with every curve coordinate
// multiplied by s. PaintedPath.Bounds and the content mask are stored in
// device pixels (the window scales them on insert), so a path built in
// logical pixels must be scaled the same way before it can be rasterized
// against them - otherwise at HiDPI scales > 1 the path paints at
// 1/scale of its intended position and most of it falls outside its own
// content mask (SVG icons vanished or stacked at the window origin on
// Windows 125%/150% displays).
func ScalePath(p *Path, s float32) *Path {
	if p == nil || s == 1 {
		return p
	}
	out := &Path{EvenOdd: p.EvenOdd, Curves: make([]PathCurve, len(p.Curves))}
	for i, c := range p.Curves {
		out.Curves[i] = PathCurve{
			P0:   geometry.Point[float32]{X: c.P0.X * s, Y: c.P0.Y * s},
			P1:   geometry.Point[float32]{X: c.P1.X * s, Y: c.P1.Y * s},
			A:    geometry.Point[float32]{X: c.A.X * s, Y: c.A.Y * s},
			B:    geometry.Point[float32]{X: c.B.X * s, Y: c.B.Y * s},
			Sign: c.Sign,
		}
	}
	return out
}

func lerpP(a, c geometry.Point[float32], t float32) geometry.Point[float32] {
	return geometry.Point[float32]{X: a.X + (c.X-a.X)*t, Y: a.Y + (c.Y-a.Y)*t}
}

func midp(a, b geometry.Point[float32]) geometry.Point[float32] {
	return geometry.Point[float32]{X: (a.X + b.X) / 2, Y: (a.Y + b.Y) / 2}
}

func dist(a, b geometry.Point[float32]) float32 {
	dx, dy := a.X-b.X, a.Y-b.Y
	return float32(math.Sqrt(float64(dx*dx + dy*dy)))
}

func minf(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func maxf(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
