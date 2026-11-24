package elements

import (
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"math"
	"sort"
	"strconv"
	"strings"

	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/render"
	"github.com/FlameInTheDark/go-gpui/style"
)

// Svg renders a vector graphic (a pure-Go mini SVG parser replaces
// usvg/resvg: paths, basic shapes, fills, strokes, groups with
// transforms, <use>/<defs> references, linear gradients and viewBox).
type Svg struct {
	st      *style.StyleRefinement
	path    string
	data    []byte
	recolor *gc.Hsla
}

// NewSvg creates an SVG element from an asset path.
func NewSvg(path string) *Svg {
	return &Svg{st: style.NewStyleRefinement(), path: path}
}

// NewSvgFromData creates an SVG from raw bytes.
func NewSvgFromData(data []byte) *Svg {
	return &Svg{st: style.NewStyleRefinement(), data: data}
}

// StyleRef returns the refinement.
func (s *Svg) StyleRef() *style.StyleRefinement { return s.st }

// Recolor forces every solid fill and stroke to the given color instead of
// the file's own palette (upstream svg recolor: the SVG is rendered as a
// monochrome mask tinted with the color). Shapes with fill="none" stay
// invisible; shapes that stroked keep their stroke geometry, now in the
// recolor color.
func (s *Svg) Recolor(color gc.Hsla) *Svg {
	s.recolor = &color
	return s
}

// ElementID implements Element.
func (s *Svg) ElementID() gpui.ElementId { return gpui.ElementId{} }

// RequestLayout implements Element.
func (s *Svg) RequestLayout(w *gpui.Window) {
	st := style.DefaultStyle()
	st.Refine(s.st)
	w.RequestLayout(st, nil)
}

// Prepaint implements Element.
func (s *Svg) Prepaint(w *gpui.Window, bounds geometry.Bounds[geometry.Pixels]) {}

// Paint implements Element.
func (s *Svg) Paint(w *gpui.Window) {
	bounds := w.ElementBounds(s)
	color := w.TextStyle().Color
	if s.st != nil && s.st.Text != nil && s.st.Text.Color != nil {
		color = *s.st.Text.Color
	}

	var src []byte
	if s.data != nil {
		src = s.data
	} else if loader := w.AssetSource(); loader != nil {
		src, _ = loader(s.path)
	}
	if len(src) == 0 {
		return
	}

	for _, pb := range parseSVGOptions(string(src), bounds, color, svgParseOptions{recolor: s.recolor}) {
		if pb.stroke != nil && pb.width > 0 {
			// Dashed strokes split the flattened path by the dash
			// pattern before outlining (upstream resvg dashing).
			if len(pb.dash) > 0 {
				for _, segs := range dashSegments(flattenSegments(pb.segments), pb.dash, pb.dashOffset) {
					b := render.NewPathBuilder()
					for _, seg := range segs {
						seg.apply(b)
					}
					w.PaintPath(b.Stroke(pb.width), bounds, *pb.stroke)
				}
			} else {
				b := render.NewPathBuilder()
				for _, seg := range pb.segments {
					seg.apply(b)
				}
				w.PaintPath(b.Stroke(pb.width), bounds, *pb.stroke)
			}
		}
		if pb.gradient != nil || pb.gradIsRadial || len(pb.gradientStops) > 0 {
			// Gradient fill (upstream resvg gradient paints): two-stop
			// linear rect fills map onto a gradient quad; radial and
			// multi-stop gradients are baked per pixel from the path
			// coverage (cached like a decoded image).
			s.paintGradient(w, pb, bounds)
			continue
		}
		if pb.fill != nil {
			b := render.NewPathBuilder()
			for _, seg := range pb.segments {
				seg.apply(b)
			}
			w.PaintPath(b, bounds, *pb.fill)
		}
	}
}

// paintGradient renders one gradient fill: a plain (axis-aligned) rectangle
// maps onto a gradient quad (gc.LinearGradient over the quad bounds);
// other shapes are baked per pixel via the software rasterizer's path
// coverage multiplied by the exact multi-stop gradient.
func (s *Svg) paintGradient(w *gpui.Window, pb svgPaint, bounds geometry.Bounds[geometry.Pixels]) {
	grad := pb.gradient
	if grad == nil && !pb.gradIsRadial {
		// Only linear gradients (and solid stop-less paints) resolve
		// pb.gradient; radial gradients bake per pixel below.
		return
	}
	if grad != nil && pb.rectFill != nil {
		// Fast path: the fill is an axis-aligned rectangle — one quad with a
		// gc.LinearGradient background (angle approximated from the gradient
		// vector mapped onto the rect bounds).
		st := style.DefaultStyle()
		bg := *grad
		st.Background = &bg
		w.PaintQuad(st, *pb.rectFill)
		return
	}
	img := s.bakeGradientFill(w, pb)
	if img == nil {
		return
	}
	// The baked image covers the paint's bounding box in window space.
	rect := svgPaintBounds(pb)
	if rect == nil {
		return
	}
	w.PaintImage(*rect, geometry.ZeroCornersAbsolute().ToPixels(w.RemSize()), img, false)
}

// svgPaintBounds computes the axis-aligned bounds of a paint's primitives in
// logical pixels (the same box bakeGradientFill rasterizes).
func svgPaintBounds(pb svgPaint) *geometry.Bounds[geometry.Pixels] {
	prims := flattenSegments(pb.segments)
	if len(prims) == 0 {
		return nil
	}
	minX, minY := float32(math.MaxFloat32), float32(math.MaxFloat32)
	maxX, maxY := float32(-math.MaxFloat32), float32(-math.MaxFloat32)
	for _, p := range prims {
		for _, pt := range segmentPoints(p) {
			minX = minf(minX, pt.X)
			minY = minf(minY, pt.Y)
			maxX = maxf(maxX, pt.X)
			maxY = maxf(maxY, pt.Y)
		}
	}
	b := geometry.Bounds[geometry.Pixels]{
		Origin: geometry.Point[geometry.Pixels]{X: geometry.Px(minX), Y: geometry.Px(minY)},
		Size:   geometry.Size[geometry.Pixels]{Width: geometry.Px(maxX - minX), Height: geometry.Px(maxY - minY)},
	}
	return &b
}

// bakeGradientFill rasterizes the path's coverage (white fill, no mask) and
// multiplies each pixel by the exact gradient color. The result is cached
// in the image cache keyed by the SVG source, gradient and raster size.
func (s *Svg) bakeGradientFill(w *gpui.Window, pb svgPaint) image.Image {
	prims := flattenSegments(pb.segments)
	if len(prims) == 0 {
		return nil
	}
	minX, minY := float32(math.MaxFloat32), float32(math.MaxFloat32)
	maxX, maxY := float32(-math.MaxFloat32), float32(-math.MaxFloat32)
	for _, p := range prims {
		for _, pt := range segmentPoints(p) {
			minX = minf(minX, pt.X)
			minY = minf(minY, pt.Y)
			maxX = maxf(maxX, pt.X)
			maxY = maxf(maxY, pt.Y)
		}
	}
	scale := w.ScaleFactor()
	pw := (maxX - minX) * scale
	ph := (maxY - minY) * scale
	width := int(math.Ceil(float64(pw)))
	height := int(math.Ceil(float64(ph)))
	if width <= 0 || height <= 0 || width > 8192 || height > 8192 {
		return nil
	}

	gen := imageCacheGeneration(w)
	key := fmt.Sprintf("svggrad:%x|%x|%dx%d|%v", fnvString(s.sourceKey()), pb.gradientKey, width, height, scale)
	if entry, ok := imageCacheGet(key, gen); ok {
		select {
		case <-entry.done:
			return entry.img
		default:
			return nil
		}
	}

	// Build the path in raster-local coordinates (upstream rasterizes into
	// a framebuffer covering the path bounds).
	b := render.NewPathBuilder()
	dx, dy := minX, minY
	for _, seg := range prims {
		applyTranslated(seg, b, dx, dy)
	}
	path, ok := b.Build()
	if !ok {
		return nil
	}
	pb0 := geometry.Bounds[geometry.Pixels]{
		Origin: geometry.Point[geometry.Pixels]{X: geometry.Px(minX), Y: geometry.Px(minY)},
		Size:   geometry.Size[geometry.Pixels]{Width: geometry.Px(maxX - minX), Height: geometry.Px(maxY - minY)},
	}
	sb := geometry.ScaleBounds(pb0, scale)
	scene := &render.Scene{}
	scene.InsertPath(render.PaintedPath{
		Path:        path,
		Bounds:      sb,
		ContentMask: render.FullContentMask(),
		Color:       gc.SolidBackground(gc.White()),
	})
	r := render.NewRasterizer(width, height, w.Atlas())
	r.Clear()
	r.PaintScene(scene)
	cover := r.Image()

	// Per-pixel gradient in raster space (exact multi-stop evaluation).
	baked := image.NewRGBA(image.Rect(0, 0, width, height))
	if pb.gradIsRadial {
		// Radial gradient: t is the normalized distance from the focal
		// point to the gradient circle along the ray through the pixel
		// (SVG 1.1 radial gradients with a focus inside the circle).
		cx, cy, r, fx, fy := pb.gradRadial[0], pb.gradRadial[1], pb.gradRadial[2], pb.gradRadial[3], pb.gradRadial[4]
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				i := (y*width + x) * 4
				a := float32(cover.Pixels[i+3]) / 255
				if a <= 0 {
					continue
				}
				px := minX + (float32(x)+0.5)/scale
				py := minY + (float32(y)+0.5)/scale
				dx, dy := px-fx, py-fy
				dist := sqrtf(dx*dx + dy*dy)
				t := float32(1)
				if dist > 0 && r > 0 {
					// Distance from the focus to the circle along
					// u = (dx,dy)/dist: solve |F + s*u - C| = r for s.
					ux, uy := dx/dist, dy/dist
					gx, gy := fx-cx, fy-cy
					bq := gx*ux + gy*uy
					cq := gx*gx + gy*gy - r*r
					disc := bq*bq - cq
					if disc > 0 {
						edge := -bq + sqrtf(disc)
						if edge > 0 {
							t = dist / edge
						}
					}
				}
				if t < 0 {
					t = 0
				}
				if t > 1 {
					t = 1
				}
				cr, cg, cb, ca := svgGradientColor(pb.gradientStops, t)
				baked.SetRGBA(x, y, color.RGBA{
					uint8(cr * a * 255),
					uint8(cg * a * 255),
					uint8(cb * a * 255),
					uint8(ca * a * 255),
				})
			}
		}
		done := make(chan struct{})
		close(done)
		imageCacheStore(key, gen, &decodedImage{img: baked, done: done})
		return baked
	}
	gradX1, gradY1, gradX2, gradY2 := pb.gradPoints[0], pb.gradPoints[1], pb.gradPoints[2], pb.gradPoints[3]
	p1x, p1y := (gradX1-minX)*scale, (gradY1-minY)*scale
	p2x, p2y := (gradX2-minX)*scale, (gradY2-minY)*scale
	dvX, dvY := p2x-p1x, p2y-p1y
	denom := dvX*dvX + dvY*dvY
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			i := (y*width + x) * 4
			a := float32(cover.Pixels[i+3]) / 255
			if a <= 0 {
				continue
			}
			t := float32(0)
			if denom > 0 {
				t = ((float32(x)+0.5-p1x)*dvX + (float32(y)+0.5-p1y)*dvY) / denom
				t = minf(1, maxf(0, t))
			}
			cr, cg, cb, ca := svgGradientColor(pb.gradientStops, t)
			// Premultiplied output for the polychrome sprite path.
			baked.SetRGBA(x, y, color.RGBA{
				uint8(cr * a * 255),
				uint8(cg * a * 255),
				uint8(cb * a * 255),
				uint8(ca * a * 255),
			})
		}
	}
	done := make(chan struct{})
	close(done)
	imageCacheStore(key, gen, &decodedImage{img: baked, done: done})
	return baked
}

// sourceKey identifies the SVG source for cache keys.
func (s *Svg) sourceKey() string {
	if s.data != nil {
		return fmt.Sprintf("data:%p", s.data)
	}
	return "path:" + s.path
}

// svgGradientColor evaluates the multi-stop gradient at t (pad semantics).
func svgGradientColor(stops []svgGradientStop, t float32) (float32, float32, float32, float32) {
	if len(stops) == 0 {
		return 0, 0, 0, 0
	}
	if t <= stops[0].offset {
		c := stops[0].color
		return c.ToRgb().R, c.ToRgb().G, c.ToRgb().B, c.A
	}
	last := stops[len(stops)-1]
	if t >= last.offset {
		c := last.color
		return c.ToRgb().R, c.ToRgb().G, c.ToRgb().B, c.A
	}
	for i := 0; i+1 < len(stops); i++ {
		a, b := stops[i], stops[i+1]
		if t >= a.offset && t <= b.offset {
			span := b.offset - a.offset
			mix := float32(0)
			if span > 0 {
				mix = (t - a.offset) / span
			}
			ca, cb := a.color.ToRgb(), b.color.ToRgb()
			return ca.R + (cb.R-ca.R)*mix, ca.G + (cb.G-ca.G)*mix, ca.B + (cb.B-ca.B)*mix, a.color.A + (b.color.A-a.color.A)*mix
		}
	}
	c := last.color
	return c.ToRgb().R, c.ToRgb().G, c.ToRgb().B, c.A
}

// svgPaint is one parsed paint: an outline stroked with `stroke` and/or
// filled with `fill` (nil color = attribute "none"), optionally filled
// with a linear gradient (gradient + gradientStops + gradPoints; rectFill
// marks the axis-aligned rectangle fast path).
type svgPaint struct {
	segments []svgSegment
	fill     *gc.Hsla
	stroke   *gc.Hsla
	width    float32

	// Linear gradient fill (fill="url(#id)").
	gradientDef   *svgGradientDef                   // resolved definition
	gradient      *gc.Background                    // 2-stop background (rect quad fast path)
	gradientStops []svgGradientStop                 // full stop list (per-pixel bake)
	gradPoints    [4]float32                        // gradient vector in window space
	gradRadial    [5]float32                        // radial: cx, cy, r, fx, fy in window space
	gradIsRadial  bool                              // radial gradient bake
	gradientKey   uint64                            // definition hash (bake cache keys)
	rectFill      *geometry.Bounds[geometry.Pixels] // set for a single axis-aligned rect
	// dash is the stroke-dasharray pattern (alternating on/off lengths;
	// odd counts repeat once per the SVG spec).
	dash       []float32
	dashOffset float32
}

// svgGradientStop is one <stop> of a linear gradient.
type svgGradientStop struct {
	color  gc.Hsla
	offset float32
}

// svgGradientDef is a parsed <linearGradient> or <radialGradient>
// definition.
type svgGradientDef struct {
	x1, y1, x2, y2 float32
	userSpace      bool
	stops          []svgGradientStop
	// radial marks a <radialGradient>: cx/cy/r in bounding-box (or
	// user-space) coordinates, fx/fy the focal point (defaults to the
	// center).
	radial            bool
	cx, cy, r, fx, fy float32
	// fr is the focal radius (SVG 2); 0 in SVG 1.1 documents.
	fr float32
}

// svgParseOptions carries parse-time overrides.
type svgParseOptions struct {
	// recolor forces every solid fill and stroke to this color (upstream
	// svg recolor: monochrome mask tinted with the color).
	recolor *gc.Hsla
}

// svgNode is one element of the parsed SVG tree.
type svgNode struct {
	name     string
	attrs    map[string]string
	children []*svgNode
}

func (n *svgNode) attr(name string) (string, bool) {
	v, ok := n.attrs[name]
	return v, ok
}

// svgInherited carries fill/stroke inheritance down the tree.
type svgInherited struct {
	fill        *string
	stroke      *string
	strokeWidth *float32
}

func inherit(n *svgNode, inh svgInherited) svgInherited {
	out := inh
	if v, ok := n.attr("fill"); ok {
		out.fill = &v
	}
	if v, ok := n.attr("stroke"); ok {
		out.stroke = &v
	}
	if v, ok := n.attr("stroke-width"); ok {
		if f := parseFloats(v); len(f) > 0 {
			out.strokeWidth = &f[0]
		}
	}
	return out
}

type svgSegment interface{ apply(b *render.PathBuilder) }

type segMove struct{ x, y float32 }
type segLine struct{ x, y float32 }
type segQuad struct{ cx, cy, x, y float32 }
type segCubic struct{ c1x, c1y, c2x, c2y, x, y float32 }
type segClose struct{}
type segRect struct{ x, y, w, h, rx float32 }
type segCircle struct{ cx, cy, r float32 }
type segEllipse struct{ cx, cy, rx, ry float32 }
type segPoly struct {
	points []float32
	closed bool
}

func (s segMove) apply(b *render.PathBuilder) { b.MoveTo(geometry.Point[float32]{X: s.x, Y: s.y}) }
func (s segLine) apply(b *render.PathBuilder) { b.LineTo(geometry.Point[float32]{X: s.x, Y: s.y}) }
func (s segQuad) apply(b *render.PathBuilder) {
	b.QuadTo(geometry.Point[float32]{X: s.cx, Y: s.cy}, geometry.Point[float32]{X: s.x, Y: s.y})
}
func (s segCubic) apply(b *render.PathBuilder) {
	b.CubicTo(geometry.Point[float32]{X: s.c1x, Y: s.c1y}, geometry.Point[float32]{X: s.c2x, Y: s.c2y}, geometry.Point[float32]{X: s.x, Y: s.y})
}
func (s segClose) apply(b *render.PathBuilder) { b.Close() }
func (s segRect) apply(b *render.PathBuilder) {
	r := s.rx
	if r > s.w/2 {
		r = s.w / 2
	}
	if r > s.h/2 {
		r = s.h / 2
	}
	x, y, w, h := s.x, s.y, s.w, s.h
	b.MoveTo(geometry.Point[float32]{X: x + r, Y: y})
	b.LineTo(geometry.Point[float32]{X: x + w - r, Y: y})
	b.QuadTo(geometry.Point[float32]{X: x + w, Y: y}, geometry.Point[float32]{X: x + w, Y: y + r})
	b.LineTo(geometry.Point[float32]{X: x + w, Y: y + h - r})
	b.QuadTo(geometry.Point[float32]{X: x + w, Y: y + h}, geometry.Point[float32]{X: x + w - r, Y: y + h})
	b.LineTo(geometry.Point[float32]{X: x + r, Y: y + h})
	b.QuadTo(geometry.Point[float32]{X: x, Y: y + h}, geometry.Point[float32]{X: x, Y: y + h - r})
	b.LineTo(geometry.Point[float32]{X: x, Y: y + r})
	b.QuadTo(geometry.Point[float32]{X: x, Y: y}, geometry.Point[float32]{X: x + r, Y: y})
	b.Close()
}
func (s segCircle) apply(b *render.PathBuilder) {
	b.AddCircle(geometry.Point[float32]{X: s.cx, Y: s.cy}, s.r)
}
func (s segEllipse) apply(b *render.PathBuilder) {
	b.AddEllipse(geometry.Point[float32]{X: s.cx, Y: s.cy}, s.rx, s.ry)
}
func (s segPoly) apply(b *render.PathBuilder) {
	var pts []geometry.Point[float32]
	for i := 0; i+1 < len(s.points); i += 2 {
		pts = append(pts, geometry.Point[float32]{X: s.points[i], Y: s.points[i+1]})
	}
	if len(pts) > 0 {
		b.AddPolygon(pts, s.closed)
	}
}

// scanElementTree parses the SVG source into a tree (open tags nest, close
// tags pop, self-closing tags stay leaves). Comments, processing
// instructions and the XML declaration are skipped.
func scanElementTree(src string) []*svgNode {
	var roots []*svgNode
	var stack []*svgNode
	i := 0
	for {
		lt := strings.IndexByte(src[i:], '<')
		if lt < 0 {
			break
		}
		i += lt + 1
		if i >= len(src) {
			break
		}
		if src[i] == '!' || src[i] == '?' {
			// Comment, CDATA or processing instruction: skip to '>'.
			end := strings.IndexByte(src[i:], '>')
			if end < 0 {
				break
			}
			i += end + 1
			continue
		}
		if src[i] == '/' {
			// Closing tag: pop to the matching open element.
			end := strings.IndexByte(src[i:], '>')
			if end < 0 {
				break
			}
			name := strings.TrimSpace(src[i+1 : i+end])
			for len(stack) > 0 && stack[len(stack)-1].name != name {
				stack = stack[:len(stack)-1]
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			i += end + 1
			continue
		}
		gt := strings.IndexByte(src[i:], '>')
		if gt < 0 {
			break
		}
		tag := src[i : i+gt]
		selfClosing := strings.HasSuffix(strings.TrimRight(tag, " \t\n\r"), "/")
		tag = strings.TrimRight(strings.TrimRight(tag, " \t\n\r"), "/")
		nameEnd := strings.IndexAny(tag, " \t\n\r")
		name := tag
		attrSrc := tag
		if nameEnd > 0 {
			name = tag[:nameEnd]
			attrSrc = tag[nameEnd:]
		}
		node := &svgNode{name: name, attrs: map[string]string{}}
		for _, kv := range scanAttrs(attrSrc) {
			node.attrs[kv[0]] = kv[1]
		}
		if len(stack) > 0 {
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, node)
		} else {
			roots = append(roots, node)
		}
		if !selfClosing {
			stack = append(stack, node)
		}
		i += gt + 1
	}
	return roots
}

// svgParser holds one parse run.
type svgParser struct {
	paints       []svgPaint
	defaultColor gc.Hsla
	opts         svgParseOptions
	gradients    map[string]*svgGradientDef
	defs         map[string]*svgNode
}

// parseSVG parses the supported subset, mapping the viewBox into bounds.
func parseSVG(src string, bounds geometry.Bounds[geometry.Pixels], defaultColor gc.Hsla) []svgPaint {
	return parseSVGOptions(src, bounds, defaultColor, svgParseOptions{})
}

// parseSVGOptions parses the supported subset with recolor override:
// paths, basic shapes, groups with transforms, <use>/<defs> references and
// linear gradients, mapping the viewBox into bounds.
func parseSVGOptions(src string, bounds geometry.Bounds[geometry.Pixels], defaultColor gc.Hsla, opts svgParseOptions) []svgPaint {
	nodes := scanElementTree(src)
	p := &svgParser{
		defaultColor: defaultColor,
		opts:         opts,
		gradients:    map[string]*svgGradientDef{},
		defs:         map[string]*svgNode{},
	}
	collectSVGDefs(nodes, p.defs, p.gradients)
	p.walk(nodes, identityMatrix(), svgInherited{}, 0)

	// Map the viewBox into the element's bounds: scale by the size ratio and
	// translate to the element's origin (paths render in window space).
	if len(nodes) > 0 {
		if v, ok := nodes[0].attr("viewBox"); ok {
			if f := parseFloats(v); len(f) >= 4 && f[2] > 0 && f[3] > 0 {
				bw, bh := bounds.Size.Width.AsF32(), bounds.Size.Height.AsF32()
				sx, sy := bw/f[2], bh/f[3]
				vb := svgMatrix{a: sx, d: sy, e: bounds.Origin.X.AsF32() - f[0]*sx, f: bounds.Origin.Y.AsF32() - f[1]*sy}
				for i := range p.paints {
					p.paints[i].finalizeTransform(vb)
				}
			}
		} else if ox, oy := bounds.Origin.X.AsF32(), bounds.Origin.Y.AsF32(); ox != 0 || oy != 0 {
			// No viewBox: still translate user units to the element origin.
			vb := svgMatrix{a: 1, d: 1, e: ox, f: oy}
			for i := range p.paints {
				p.paints[i].finalizeTransform(vb)
			}
		}
	}
	return p.paints
}

// finalizeTransform applies an axis-aligned outer transform (viewBox
// mapping) to a paint's geometry and detects the rect fast path.
func (p *svgPaint) finalizeTransform(m svgMatrix) {
	p.segments = transformSegmentsMatrix(p.segments, m)
	p.width *= (absf32(m.a) + absf32(m.d)) / 2
	if p.gradientStops != nil {
		p.gradPoints[0], p.gradPoints[1] = m.apply(p.gradPoints[0], p.gradPoints[1])
		p.gradPoints[2], p.gradPoints[3] = m.apply(p.gradPoints[2], p.gradPoints[3])
	}
	if p.gradIsRadial {
		// Map the center/focus through the outer transform and scale the
		// radius by the dominant axis scale (the bake treats the gradient
		// circle as circular; viewBox scales are uniform in practice).
		cx, cy := m.apply(p.gradRadial[0], p.gradRadial[1])
		fx, fy := m.apply(p.gradRadial[3], p.gradRadial[4])
		sc := absf32(m.a)
		if s := absf32(m.d); s > sc {
			sc = s
		}
		p.gradRadial = [5]float32{cx, cy, p.gradRadial[2] * sc, fx, fy}
	}
	p.detectRectFill()
}

// detectRectFill marks the paint as an axis-aligned rectangle fill (single
// rect segment) so gradient fills can map onto one quad.
func (p *svgPaint) detectRectFill() {
	p.rectFill = nil
	if len(p.segments) != 1 {
		return
	}
	if r, ok := p.segments[0].(segRect); ok {
		p.rectFill = &geometry.Bounds[geometry.Pixels]{
			Origin: geometry.Point[geometry.Pixels]{X: geometry.Px(r.x), Y: geometry.Px(r.y)},
			Size:   geometry.Size[geometry.Pixels]{Width: geometry.Px(r.w), Height: geometry.Px(r.h)},
		}
	}
}

// collectSVGDefs walks the tree collecting id-referenced definitions:
// gradients by id and every other id'd element for <use>.
func collectSVGDefs(nodes []*svgNode, defs map[string]*svgNode, gradients map[string]*svgGradientDef) {
	var walk func(n *svgNode)
	walk = func(n *svgNode) {
		if id, ok := n.attr("id"); ok && id != "" {
			if n.name == "linearGradient" || n.name == "radialGradient" {
				gradients[id] = parseGradientDef(n)
			} else {
				defs[id] = n
			}
		}
		for _, c := range n.children {
			walk(c)
		}
	}
	for _, n := range nodes {
		walk(n)
	}
}

// parseGradientDef parses a <linearGradient> or <radialGradient>
// definition (stops, offsets and the gradient vector;
// gradientUnits="userSpaceOnUse" switches from the default
// objectBoundingBox coordinates).
func parseGradientDef(n *svgNode) *svgGradientDef {
	g := &svgGradientDef{x1: 0, y1: 0, x2: 1, y2: 0}
	if n.name == "radialGradient" {
		g.radial = true
		g.cx, g.cy, g.r = 0.5, 0.5, 0.5
		g.fx, g.fy = -1, -1 // sentinel: defaults to the center
		if f := attrFloatsNode(n, "cx", "cy", "r"); len(f) == 3 {
			g.cx, g.cy, g.r = f[0], f[1], f[2]
		}
		if f := attrFloatsNode(n, "fx", "fy"); len(f) == 2 {
			g.fx, g.fy = f[0], f[1]
		}
		if g.r <= 0 {
			g.r = 0.5
		}
	} else if f := attrFloatsNode(n, "x1", "y1", "x2", "y2"); len(f) == 4 {
		g.x1, g.y1, g.x2, g.y2 = f[0], f[1], f[2], f[3]
	}
	if units, ok := n.attr("gradientUnits"); ok && units == "userSpaceOnUse" {
		g.userSpace = true
	}
	autoOffsets := parseStopOffsets(n)
	for i, c := range n.children {
		if c.name != "stop" {
			continue
		}
		var st svgGradientStop
		st.color = gc.Black()
		if v, ok := c.attr("stop-color"); ok {
			if col, ok2 := parseSVGColor(v); ok2 {
				st.color = col
			}
		}
		if v, ok := c.attr("stop-opacity"); ok {
			if f := parseFloats(v); len(f) > 0 {
				st.color.A *= f[0]
			}
		}
		if v, ok := c.attr("offset"); ok {
			st.offset = parseSVGOffset(v)
		} else if i < len(autoOffsets) {
			st.offset = autoOffsets[i]
		}
		g.stops = append(g.stops, st)
	}
	sort.SliceStable(g.stops, func(i, j int) bool { return g.stops[i].offset < g.stops[j].offset })
	return g
}

// parseStopOffsets computes SVG's implicit stop offsets for stops without
// an offset attribute: 0 for the first, 1 for the last, midway between
// neighbours otherwise.
func parseStopOffsets(n *svgNode) []float32 {
	stops := 0
	for _, c := range n.children {
		if c.name == "stop" {
			stops++
		}
	}
	out := make([]float32, 0, stops)
	for i := 0; i < stops; i++ {
		switch {
		case i == 0 && stops > 1:
			out = append(out, 0)
		case i == stops-1:
			out = append(out, 1)
		default:
			out = append(out, float32(i)/float32(stops-1))
		}
	}
	return out
}

// parseSVGOffset parses a stop offset ("0.5", "50%").
func parseSVGOffset(s string) float32 {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "%") {
		if f, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 32); err == nil {
			return float32(f) / 100
		}
		return 0
	}
	if f, err := strconv.ParseFloat(s, 32); err == nil {
		return float32(f)
	}
	return 0
}

// walk renders the tree: groups apply transforms, defs/gradient elements
// are skipped, use instantiates its reference and shapes emit paints.
func (p *svgParser) walk(nodes []*svgNode, m svgMatrix, inh svgInherited, depth int) {
	for _, n := range nodes {
		switch n.name {
		case "defs", "linearGradient", "radialGradient", "pattern", "style", "title", "desc", "metadata", "clipPath", "mask", "filter":
			continue
		case "svg", "g", "a":
			mm := m
			if t, ok := n.attr("transform"); ok {
				mm = mulMatrix(m, parseTransformAttr(t))
			}
			p.walk(n.children, mm, inherit(n, inh), depth)
			continue
		case "use":
			p.emitUse(n, m, inh, depth)
			continue
		case "path", "rect", "circle", "ellipse", "polygon", "polyline", "line":
			p.emitShape(n, m, inh)
			continue
		}
		// Unknown containers: recurse so inner shapes keep their context.
		if len(n.children) > 0 {
			mm := m
			if t, ok := n.attr("transform"); ok {
				mm = mulMatrix(m, parseTransformAttr(t))
			}
			p.walk(n.children, mm, inherit(n, inh), depth)
		}
	}
}

// emitUse instantiates the element referenced by href="#id" (or
// xlink:href), translating by the use's x/y (upstream usvg <use>).
func (p *svgParser) emitUse(n *svgNode, m svgMatrix, inh svgInherited, depth int) {
	if depth > 8 {
		return // reference cycle guard
	}
	href, ok := n.attr("href")
	if !ok {
		href, _ = n.attr("xlink:href")
	}
	id := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(href), "#"))
	if id == "" {
		return
	}
	target, ok := p.defs[id]
	if !ok {
		return
	}
	mm := m
	if t, ok := n.attr("transform"); ok {
		mm = mulMatrix(m, parseTransformAttr(t))
	}
	f := attrFloatsNode(n, "x", "y")
	mm = mulMatrix(mm, svgMatrix{a: 1, d: 1, e: f[0], f: f[1]})
	inh2 := inherit(n, inh)
	if target.name == "g" || target.name == "svg" || target.name == "a" {
		p.walk(target.children, mm, inherit(target, inh2), depth+1)
		return
	}
	p.emitShape(target, mm, inh2)
}

// emitShape builds one shape paint: fill/stroke resolution (with recolor
// override and gradient fills), geometry, then the composed transform.
func (p *svgParser) emitShape(n *svgNode, m svgMatrix, inh svgInherited) {
	paint := svgPaint{}
	// Fill resolution: attribute > inherited > default text color.
	var fillStr string
	hasFill := false
	if v, ok := n.attr("fill"); ok {
		fillStr, hasFill = v, true
	} else if inh.fill != nil {
		fillStr, hasFill = *inh.fill, true
	}
	if hasFill && fillStr != "none" {
		if strings.HasPrefix(strings.TrimSpace(fillStr), "url(") {
			p.applyGradient(&paint, fillStr)
		} else if c, ok := parseSVGColor(fillStr); ok {
			paint.fill = &c
		} else {
			paint.fill = &p.defaultColor
		}
	} else if !hasFill {
		paint.fill = &p.defaultColor
	}
	if p.opts.recolor != nil && paint.fill != nil {
		c := *p.opts.recolor
		paint.fill = &c
	}

	// Stroke resolution.
	var strokeStr string
	hasStroke := false
	if v, ok := n.attr("stroke"); ok {
		strokeStr, hasStroke = v, true
	} else if inh.stroke != nil {
		strokeStr, hasStroke = *inh.stroke, true
	}
	if hasStroke && strokeStr != "" && strokeStr != "none" {
		if c, ok := parseSVGColor(strokeStr); ok {
			paint.stroke = &c
			paint.width = 1
			if sw := n.attrStrokeWidth(inh); sw != nil {
				paint.width = *sw
			}
			// stroke-dasharray (upstream resvg dashes): a list of
			// on/off lengths; odd counts repeat once (SVG spec).
			if v, ok := n.attr("stroke-dasharray"); ok && v != "" && v != "none" {
				if ds := parseFloats(v); len(ds) > 0 {
					paint.dash = ds
				}
			}
			// stroke-dashoffset shifts the dash pattern start (SVG spec;
			// negative values shift the pattern forward).
			if v, ok := n.attr("stroke-dashoffset"); ok && v != "" {
				if f, ok := parseFloatStrict(v); ok {
					paint.dashOffset = f
				}
			}
		}
	}
	if p.opts.recolor != nil && paint.stroke != nil {
		c := *p.opts.recolor
		paint.stroke = &c
	}

	// Geometry in user space.
	switch n.name {
	case "path":
		if d, ok := n.attr("d"); ok {
			paint.segments = parsePathData(d)
		}
	case "rect":
		f := attrFloatsNode(n, "x", "y", "width", "height", "rx")
		paint.segments = []svgSegment{segRect{x: f[0], y: f[1], w: f[2], h: f[3], rx: f[4]}}
	case "circle":
		f := attrFloatsNode(n, "cx", "cy", "r")
		paint.segments = []svgSegment{segCircle{cx: f[0], cy: f[1], r: f[2]}}
	case "ellipse":
		f := attrFloatsNode(n, "cx", "cy", "rx", "ry")
		paint.segments = []svgSegment{segEllipse{cx: f[0], cy: f[1], rx: f[2], ry: f[3]}}
	case "polygon", "polyline":
		if pts, ok := n.attr("points"); ok {
			paint.segments = []svgSegment{segPoly{points: parseFloats(pts), closed: n.name == "polygon"}}
		}
	case "line":
		f := attrFloatsNode(n, "x1", "y1", "x2", "y2")
		paint.segments = []svgSegment{
			segMove{x: f[0], y: f[1]},
			segLine{x: f[2], y: f[3]},
		}
	}
	if len(paint.segments) == 0 {
		return
	}

	// Composed transform: parent matrix x the shape's own transform.
	mm := m
	if t, ok := n.attr("transform"); ok {
		mm = mulMatrix(m, parseTransformAttr(t))
	}
	// Gradient vector mapping uses the pre-transform geometry (object
	// bounding box) or raw user-space coordinates, both mapped through the
	// composed matrix into window space.
	if paint.gradientStops != nil {
		p.mapGradientPoints(&paint, mm)
	}
	paint.segments = transformSegmentsMatrix(paint.segments, mm)
	paint.width *= sqrtf(absf32(mm.a*mm.d - mm.b*mm.c))
	p.paints = append(p.paints, paint)
}

// attrStrokeWidth resolves the effective stroke width (attribute or
// inherited).
func (n *svgNode) attrStrokeWidth(inh svgInherited) *float32 {
	if v, ok := n.attr("stroke-width"); ok {
		if f := parseFloats(v); len(f) > 0 {
			return &f[0]
		}
	}
	return inh.strokeWidth
}

// applyGradient resolves fill="url(#id)" into a gradient definition.
func (p *svgParser) applyGradient(paint *svgPaint, ref string) {
	id := strings.TrimSpace(ref)
	id = strings.TrimPrefix(id, "url(")
	id = strings.TrimSuffix(id, ")")
	id = strings.TrimSpace(strings.TrimPrefix(id, "#"))
	def, ok := p.gradients[id]
	if !ok || len(def.stops) == 0 {
		return // unresolved reference: no fill
	}
	paint.gradientDef = def
	paint.gradientStops = def.stops
	paint.gradientKey = gradientDefKey(def)
}

// gradientDefKey hashes a gradient definition for bake cache keys.
func gradientDefKey(def *svgGradientDef) uint64 {
	h := fnv.New64a()
	fmt.Fprintf(h, "%v|%v|%v|%v|%v|%v|%v,%v,%v,%v,%v|", def.x1, def.y1, def.x2, def.y2, def.userSpace,
		def.radial, def.cx, def.cy, def.r, def.fx, def.fy)
	for _, st := range def.stops {
		rgba := st.color.ToRgb()
		fmt.Fprintf(h, "%v/%v/%v/%v@%v;", rgba.R, rgba.G, rgba.B, st.color.A, st.offset)
	}
	return h.Sum64()
}

// mapGradientPoints computes the window-space gradient vector for a paint.
// Object bounding box gradients anchor to the shape's user-space bbox;
// userSpace gradients use raw coordinates. Both map through the shape
// transform into window space. Two-stop gradients additionally map onto
// gc.LinearGradient (the angle approximates the vector across the shape
// bounds); multi-stop fills take the exact per-pixel bake.
func (p *svgParser) mapGradientPoints(paint *svgPaint, m svgMatrix) {
	def := paint.gradientDef
	if def == nil {
		return
	}
	if def.radial {
		// Center/radius/focus in window space: bounding-box units anchor to
		// the shape's bbox, user-space units use raw coordinates.
		var cx, cy, r, fx, fy float32
		if def.userSpace {
			cx, cy, r = def.cx, def.cy, def.r
		} else {
			bb := segmentsBBox(paint.segments)
			w := bb.maxX - bb.minX
			h := bb.maxY - bb.minY
			cx = bb.minX + def.cx*w
			cy = bb.minY + def.cy*h
			r = def.r * sqrtf(w*w+h*h) / sqrtf(2) // SVG: r is against the diagonal-normalized bbox
		}
		if def.fx < 0 || def.fy < 0 {
			fx, fy = cx, cy
		} else if def.userSpace {
			fx, fy = def.fx, def.fy
		} else {
			bb := segmentsBBox(paint.segments)
			fx = bb.minX + def.fx*(bb.maxX-bb.minX)
			fy = bb.minY + def.fy*(bb.maxY-bb.minY)
		}
		cxW, cyW := m.apply(cx, cy)
		fxW, fyW := m.apply(fx, fy)
		// Radius scales by the matrix's largest singular value approximation
		// (uniform strokes use the determinant; non-uniform scales shear the
		// circle — the bake treats it as circular, matching UI usage).
		rW := r * sqrtf(absf32(m.det()))
		paint.gradRadial = [5]float32{cxW, cyW, rW, fxW, fyW}
		paint.gradIsRadial = true
		// Radial gradients always take the exact per-pixel bake (the quad
		// background only models linear gradients).
		paint.gradient = nil
		return
	}
	var p1x, p1y, p2x, p2y float32
	if def.userSpace {
		p1x, p1y, p2x, p2y = def.x1, def.y1, def.x2, def.y2
	} else {
		bb := segmentsBBox(paint.segments)
		w := bb.maxX - bb.minX
		h := bb.maxY - bb.minY
		p1x = bb.minX + def.x1*w
		p1y = bb.minY + def.y1*h
		p2x = bb.minX + def.x2*w
		p2y = bb.minY + def.y2*h
	}
	x1, y1 := m.apply(p1x, p1y)
	x2, y2 := m.apply(p2x, p2y)
	paint.gradPoints = [4]float32{x1, y1, x2, y2}
	if len(def.stops) == 2 {
		bg := gc.LinearGradient(svgGradientAngle(x1, y1, x2, y2),
			gc.NewLinearColorStop(def.stops[0].color, def.stops[0].offset),
			gc.NewLinearColorStop(def.stops[1].color, def.stops[1].offset))
		paint.gradient = &bg
	}
}

// det returns the matrix determinant.
func (m svgMatrix) det() float32 { return m.a*m.d - m.b*m.c }

// attrFloatsNode reads named float attributes from a tree node.
func attrFloatsNode(n *svgNode, names ...string) []float32 {
	out := make([]float32, len(names))
	for i, name := range names {
		if v, ok := n.attr(name); ok {
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 32); err == nil {
				out[i] = float32(f)
			}
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Affine transform helpers
// ---------------------------------------------------------------------------

// svgMatrix is a 2D affine transform: x' = a*x + c*y + e,
// y' = b*x + d*y + f (SVG transform semantics, y-down space).
type svgMatrix struct {
	a, b, c, d, e, f float32
}

// identityMatrix returns the identity transform.
func identityMatrix() svgMatrix { return svgMatrix{a: 1, d: 1} }

// apply maps one point through the matrix.
func (m svgMatrix) apply(x, y float32) (float32, float32) {
	return m.a*x + m.c*y + m.e, m.b*x + m.d*y + m.f
}

// mulMatrix composes m after n (apply n first, then m): SVG transform
// lists read left-to-right with the leftmost transform outermost.
func mulMatrix(m, n svgMatrix) svgMatrix {
	return svgMatrix{
		a: m.a*n.a + m.c*n.b,
		b: m.b*n.a + m.d*n.b,
		c: m.a*n.c + m.c*n.d,
		d: m.b*n.c + m.d*n.d,
		e: m.a*n.e + m.c*n.f + m.e,
		f: m.b*n.e + m.d*n.f + m.f,
	}
}

// parseTransformAttr parses a transform attribute into one matrix:
// translate, scale, rotate (with optional center) and matrix are
// supported; skewX/skewY are ignored (rare in UI icons).
func parseTransformAttr(s string) svgMatrix {
	m := identityMatrix()
	i := 0
	for i < len(s) {
		start := i
		for i < len(s) && s[i] != '(' {
			i++
		}
		if i >= len(s) {
			break
		}
		name := strings.ToLower(strings.TrimSpace(s[start:i]))
		end := strings.IndexByte(s[i:], ')')
		if end < 0 {
			break
		}
		args := parseFloats(s[i+1 : i+end])
		i += end + 1
		switch name {
		case "translate":
			tx, ty := float32(0), float32(0)
			if len(args) > 0 {
				tx = args[0]
			}
			if len(args) > 1 {
				ty = args[1]
			}
			m = mulMatrix(m, svgMatrix{a: 1, d: 1, e: tx, f: ty})
		case "scale":
			sx, sy := float32(0), float32(0)
			if len(args) > 0 {
				sx = args[0]
				sy = args[0]
			}
			if len(args) > 1 {
				sy = args[1]
			}
			m = mulMatrix(m, svgMatrix{a: sx, d: sy})
		case "rotate":
			if len(args) > 0 {
				rot := rotationMatrix(args[0])
				if len(args) >= 3 {
					m = mulMatrix(m, svgMatrix{a: 1, d: 1, e: args[1], f: args[2]})
					m = mulMatrix(m, rot)
					m = mulMatrix(m, svgMatrix{a: 1, d: 1, e: -args[1], f: -args[2]})
				} else {
					m = mulMatrix(m, rot)
				}
			}
		case "matrix":
			if len(args) >= 6 {
				m = mulMatrix(m, svgMatrix{a: args[0], b: args[1], c: args[2], d: args[3], e: args[4], f: args[5]})
			}
		}
	}
	return m
}

// rotationMatrix builds the clockwise (y-down) rotation matrix.
func rotationMatrix(deg float32) svgMatrix {
	r := deg * float32(math.Pi) / 180
	c, s := cosf(r), sinf(r)
	return svgMatrix{a: c, b: s, c: -s, d: c}
}

// transformSegmentsMatrix applies an affine transform to a segment list.
// Axis-aligned matrices keep the shape types (rect/circle/ellipse remain
// cheap); rotated or skewed matrices flatten shapes to point primitives.
func transformSegmentsMatrix(segs []svgSegment, m svgMatrix) []svgSegment {
	out := make([]svgSegment, 0, len(segs))
	axisAligned := m.b == 0 && m.c == 0
	for _, s := range segs {
		if axisAligned {
			switch v := s.(type) {
			case segRect:
				x, y := m.apply(v.x, v.y)
				out = append(out, segRect{x: x, y: y, w: v.w * absf32(m.a), h: v.h * absf32(m.d), rx: v.rx * absf32(m.a)})
				continue
			case segCircle:
				x, y := m.apply(v.cx, v.cy)
				out = append(out, segCircle{cx: x, cy: y, r: v.r * (absf32(m.a) + absf32(m.d)) / 2})
				continue
			case segEllipse:
				x, y := m.apply(v.cx, v.cy)
				out = append(out, segEllipse{cx: x, cy: y, rx: v.rx * absf32(m.a), ry: v.ry * absf32(m.d)})
				continue
			case segPoly:
				out = append(out, transformPointSegment(s, m))
				continue
			}
		}
		flat := flattenSegments([]svgSegment{s})
		for _, f := range flat {
			out = append(out, transformPointSegment(f, m))
		}
	}
	return out
}

// transformPointSegment maps every point of a point-based segment through
// the matrix.
func transformPointSegment(s svgSegment, m svgMatrix) svgSegment {
	switch v := s.(type) {
	case segMove:
		x, y := m.apply(v.x, v.y)
		return segMove{x: x, y: y}
	case segLine:
		x, y := m.apply(v.x, v.y)
		return segLine{x: x, y: y}
	case segQuad:
		cx, cy := m.apply(v.cx, v.cy)
		x, y := m.apply(v.x, v.y)
		return segQuad{cx: cx, cy: cy, x: x, y: y}
	case segCubic:
		c1x, c1y := m.apply(v.c1x, v.c1y)
		c2x, c2y := m.apply(v.c2x, v.c2y)
		x, y := m.apply(v.x, v.y)
		return segCubic{c1x: c1x, c1y: c1y, c2x: c2x, c2y: c2y, x: x, y: y}
	case segPoly:
		out := make([]float32, len(v.points))
		for i := 0; i+1 < len(v.points); i += 2 {
			out[i], out[i+1] = m.apply(v.points[i], v.points[i+1])
		}
		return segPoly{points: out, closed: v.closed}
	}
	return s
}

// flattenSegments converts shape segments (rect, circle, ellipse, polygon,
// polyline) into point-based primitives (move/line/quad/cubic/close) so a
// general affine transform can be applied to every point.
func flattenSegments(segs []svgSegment) []svgSegment {
	var out []svgSegment
	for _, s := range segs {
		switch v := s.(type) {
		case segRect:
			out = append(out, rectPrimitives(v)...)
		case segCircle:
			out = append(out, ellipsePrimitives(v.cx, v.cy, v.r, v.r)...)
		case segEllipse:
			out = append(out, ellipsePrimitives(v.cx, v.cy, v.rx, v.ry)...)
		case segPoly:
			out = append(out, polyPrimitives(v)...)
		default:
			out = append(out, s)
		}
	}
	return out
}

// rectPrimitives flattens a rounded rect into path primitives (mirrors
// segRect.apply).
func rectPrimitives(v segRect) []svgSegment {
	r := v.rx
	if r > v.w/2 {
		r = v.w / 2
	}
	if r > v.h/2 {
		r = v.h / 2
	}
	x, y, w, h := v.x, v.y, v.w, v.h
	return []svgSegment{
		segMove{x: x + r, y: y},
		segLine{x: x + w - r, y: y},
		segQuad{cx: x + w, cy: y, x: x + w, y: y + r},
		segLine{x: x + w, y: y + h - r},
		segQuad{cx: x + w, cy: y + h, x: x + w - r, y: y + h},
		segLine{x: x + r, y: y + h},
		segQuad{cx: x, cy: y + h, x: x, y: y + h - r},
		segLine{x: x, y: y + r},
		segQuad{cx: x, cy: y, x: x + r, y: y},
		segClose{},
	}
}

// svgKappa is the cubic Bézier constant for a quarter circle.
const svgKappa = 0.5522847498309936

// ellipsePrimitives flattens an ellipse into four cubic curves.
func ellipsePrimitives(cx, cy, rx, ry float32) []svgSegment {
	kx, ky := svgKappa*rx, svgKappa*ry
	return []svgSegment{
		segMove{x: cx + rx, y: cy},
		segCubic{c1x: cx + rx, c1y: cy + ky, c2x: cx + kx, c2y: cy + ry, x: cx, y: cy + ry},
		segCubic{c1x: cx - kx, c1y: cy + ry, c2x: cx - rx, c2y: cy + ky, x: cx - rx, y: cy},
		segCubic{c1x: cx - rx, c1y: cy - ky, c2x: cx - kx, c2y: cy - ry, x: cx, y: cy - ry},
		segCubic{c1x: cx + kx, c1y: cy - ry, c2x: cx + rx, c2y: cy - ky, x: cx + rx, y: cy},
		segClose{},
	}
}

// polyPrimitives flattens a polygon/polyline into move+line primitives.
func polyPrimitives(v segPoly) []svgSegment {
	var out []svgSegment
	first := true
	for i := 0; i+1 < len(v.points); i += 2 {
		if first {
			out = append(out, segMove{x: v.points[i], y: v.points[i+1]})
			first = false
		} else {
			out = append(out, segLine{x: v.points[i], y: v.points[i+1]})
		}
	}
	if v.closed {
		out = append(out, segClose{})
	}
	return out
}

// segmentPoints returns every control/end point of a segment (used for
// bounding boxes).
func segmentPoints(s svgSegment) []geometry.Point[float32] {
	switch v := s.(type) {
	case segMove:
		return []geometry.Point[float32]{{X: v.x, Y: v.y}}
	case segLine:
		return []geometry.Point[float32]{{X: v.x, Y: v.y}}
	case segQuad:
		return []geometry.Point[float32]{{X: v.cx, Y: v.cy}, {X: v.x, Y: v.y}}
	case segCubic:
		return []geometry.Point[float32]{{X: v.c1x, Y: v.c1y}, {X: v.c2x, Y: v.c2y}, {X: v.x, Y: v.y}}
	case segRect:
		return []geometry.Point[float32]{
			{X: v.x, Y: v.y}, {X: v.x + v.w, Y: v.y}, {X: v.x, Y: v.y + v.h}, {X: v.x + v.w, Y: v.y + v.h},
		}
	case segCircle:
		return []geometry.Point[float32]{
			{X: v.cx - v.r, Y: v.cy}, {X: v.cx + v.r, Y: v.cy}, {X: v.cx, Y: v.cy - v.r}, {X: v.cx, Y: v.cy + v.r},
		}
	case segEllipse:
		return []geometry.Point[float32]{
			{X: v.cx - v.rx, Y: v.cy}, {X: v.cx + v.rx, Y: v.cy}, {X: v.cx, Y: v.cy - v.ry}, {X: v.cx, Y: v.cy + v.ry},
		}
	case segPoly:
		var pts []geometry.Point[float32]
		for i := 0; i+1 < len(v.points); i += 2 {
			pts = append(pts, geometry.Point[float32]{X: v.points[i], Y: v.points[i+1]})
		}
		return pts
	}
	return nil
}

// svgBBox is a float bounding box.
type svgBBox struct{ minX, minY, maxX, maxY float32 }

// segmentsBBox computes the bounding box of a segment list.
func segmentsBBox(segs []svgSegment) svgBBox {
	bb := svgBBox{minX: math.MaxFloat32, minY: math.MaxFloat32, maxX: -math.MaxFloat32, maxY: -math.MaxFloat32}
	for _, s := range segs {
		for _, pt := range segmentPoints(s) {
			bb.minX = minf(bb.minX, pt.X)
			bb.minY = minf(bb.minY, pt.Y)
			bb.maxX = maxf(bb.maxX, pt.X)
			bb.maxY = maxf(bb.maxY, pt.Y)
		}
	}
	return bb
}

// applyTranslated applies a segment to the builder with points shifted by
// (-dx, -dy) (bake rasterization is framebuffer-local).
func applyTranslated(s svgSegment, b *render.PathBuilder, dx, dy float32) {
	switch v := s.(type) {
	case segMove:
		b.MoveTo(geometry.Point[float32]{X: v.x - dx, Y: v.y - dy})
	case segLine:
		b.LineTo(geometry.Point[float32]{X: v.x - dx, Y: v.y - dy})
	case segQuad:
		b.QuadTo(geometry.Point[float32]{X: v.cx - dx, Y: v.cy - dy}, geometry.Point[float32]{X: v.x - dx, Y: v.y - dy})
	case segCubic:
		b.CubicTo(geometry.Point[float32]{X: v.c1x - dx, Y: v.c1y - dy}, geometry.Point[float32]{X: v.c2x - dx, Y: v.c2y - dy}, geometry.Point[float32]{X: v.x - dx, Y: v.y - dy})
	case segClose:
		b.Close()
	case segRect:
		for _, p := range rectPrimitives(v) {
			applyTranslated(p, b, dx, dy)
		}
	case segCircle:
		for _, p := range ellipsePrimitives(v.cx, v.cy, v.r, v.r) {
			applyTranslated(p, b, dx, dy)
		}
	case segEllipse:
		for _, p := range ellipsePrimitives(v.cx, v.cy, v.rx, v.ry) {
			applyTranslated(p, b, dx, dy)
		}
	case segPoly:
		for _, p := range polyPrimitives(v) {
			applyTranslated(p, b, dx, dy)
		}
	}
}

// svgGradientAngle converts a gradient vector (window space, y-down) into
// the gc.LinearGradient angle convention (degrees, 0 = up, clockwise).
func svgGradientAngle(x1, y1, x2, y2 float32) float32 {
	angle := 90 + float32(math.Atan2(float64(y2-y1), float64(x2-x1)))*180/float32(math.Pi)
	angle = float32(math.Mod(float64(angle), 360))
	if angle < 0 {
		angle += 360
	}
	return angle
}

// fnvString hashes a string for cache keys.
func fnvString(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

// absf32 returns the absolute value.
func absf32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

type svgElement struct {
	name  string
	attrs map[string]string
}

func scanElements(src string) []svgElement {
	var out []svgElement
	i := 0
	for {
		lt := strings.IndexByte(src[i:], '<')
		if lt < 0 {
			break
		}
		i += lt + 1
		if i >= len(src) || src[i] == '/' || src[i] == '?' || src[i] == '!' {
			continue
		}
		gt := strings.IndexByte(src[i:], '>')
		if gt < 0 {
			break
		}
		tag := src[i : i+gt]
		nameEnd := strings.IndexAny(tag, " \t\n\r/")
		name := tag
		if nameEnd > 0 {
			name = tag[:nameEnd]
		}
		el := svgElement{name: name, attrs: map[string]string{}}
		// Attribute scanning starts after the element name: the tag string
		// still contains it, which would otherwise leak into the first
		// attribute's key (e.g. `path d=` parsed as key "path d").
		attrSrc := tag
		if nameEnd > 0 {
			attrSrc = tag[nameEnd:]
		}
		for _, kv := range scanAttrs(attrSrc) {
			el.attrs[kv[0]] = kv[1]
		}
		out = append(out, el)
		i += gt + 1
	}
	return out
}

func scanAttrs(tag string) [][2]string {
	var out [][2]string
	i := 0
	for i < len(tag) {
		for i < len(tag) && (tag[i] == ' ' || tag[i] == '\t' || tag[i] == '\n' || tag[i] == '/') {
			i++
		}
		if i >= len(tag) {
			break
		}
		eq := strings.IndexByte(tag[i:], '=')
		if eq < 0 {
			break
		}
		key := strings.TrimSpace(tag[i : i+eq])
		i += eq + 1
		if i >= len(tag) {
			break
		}
		if tag[i] != '"' {
			// skip to next space
			sp := strings.IndexAny(tag[i:], " \t\n")
			if sp < 0 {
				sp = len(tag) - i
			}
			out = append(out, [2]string{key, tag[i : i+sp]})
			i += sp
			continue
		}
		end := strings.IndexByte(tag[i+1:], '"')
		if end < 0 {
			break
		}
		out = append(out, [2]string{key, tag[i+1 : i+1+end]})
		i += end + 2
	}
	return out
}

func (e svgElement) attr(name string) (string, bool) {
	v, ok := e.attrs[name]
	return v, ok
}

func attrFloats(el svgElement, names ...string) []float32 {
	out := make([]float32, len(names))
	for i, n := range names {
		if v, ok := el.attr(n); ok {
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 32); err == nil {
				out[i] = float32(f)
			}
		}
	}
	return out
}

func parseFloats(s string) []float32 {
	var out []float32
	for _, tok := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == ',' || r == '\n' || r == '\t' || r == '\r'
	}) {
		if f, err := strconv.ParseFloat(tok, 32); err == nil {
			out = append(out, float32(f))
		}
	}
	return out
}

// parseFloatStrict parses one float (attribute values like dashoffset).
func parseFloatStrict(s string) (float32, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 32)
	if err != nil {
		return 0, false
	}
	return float32(f), true
}

func parseSVGColor(s string) (gc.Hsla, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "none", "transparent":
		return gc.Hsla{A: 0}, true
	case "black":
		return gc.Black(), true
	case "white":
		return gc.White(), true
	case "red":
		return gc.Red(), true
	case "green":
		return gc.Green(), true
	case "blue":
		return gc.Blue(), true
	case "yellow":
		return gc.Yellow(), true
	case "currentcolor", "":
		return gc.Hsla{}, false
	}
	if strings.HasPrefix(s, "#") && len(s) == 7 {
		if v, err := strconv.ParseUint(s[1:], 16, 32); err == nil {
			return gc.RGB(uint32(v)).Hsla(), true
		}
	}
	// rgb(r,g,b) / rgba(r,g,b,a) functional notation.
	if strings.HasPrefix(s, "rgb(") && strings.HasSuffix(s, ")") {
		f := parseFloats(s[4 : len(s)-1])
		if len(f) >= 3 {
			v := uint32(clampChannel(f[0]))<<16 | uint32(clampChannel(f[1]))<<8 | uint32(clampChannel(f[2]))
			h := gc.RGB(v).Hsla()
			if len(f) >= 4 {
				h.A = f[3]
			}
			return h, true
		}
	}
	return gc.Hsla{}, false
}

func clampChannel(v float32) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return int(v)
}

// parsePathData parses SVG path "d" attributes into segments.
func parsePathData(d string) []svgSegment {
	var out []svgSegment
	var cmd rune
	var nums []float32
	i := 0
	push := func(segs ...svgSegment) { out = append(out, segs...) }
	// Track current point for relative commands.
	cx, cy := float32(0), float32(0)
	startX, startY := float32(0), float32(0)
	prevCtrlX, prevCtrlY := float32(0), float32(0)
	havePrevCtrl := false

	nextNums := func(n int) []float32 {
		for len(nums) < n && i < len(d) {
			// parse another number
			for i < len(d) && (d[i] == ' ' || d[i] == ',' || d[i] == '\n' || d[i] == '\t' || d[i] == '\r') {
				i++
			}
			start := i
			if i < len(d) && (d[i] == '-' || d[i] == '+') {
				i++
			}
			digits := i
			for i < len(d) && d[i] >= '0' && d[i] <= '9' {
				i++
			}
			count := i - digits
			if i < len(d) && d[i] == '.' {
				i++
				digits = i
				for i < len(d) && d[i] >= '0' && d[i] <= '9' {
					i++
				}
				count += i - digits
			}
			if count == 0 {
				return nums
			}
			if i < len(d) && (d[i] == 'e' || d[i] == 'E') {
				end := i + 1
				if end < len(d) && (d[end] == '+' || d[end] == '-') {
					end++
				}
				digits = end
				for end < len(d) && d[end] >= '0' && d[end] <= '9' {
					end++
				}
				if end > digits {
					i = end
				}
			}
			if f, err := strconv.ParseFloat(d[start:i], 32); err == nil {
				nums = append(nums, float32(f))
			}
		}
		if len(nums) < n {
			return nums
		}
		vals := nums[:n]
		nums = nums[n:]
		return vals
	}

	for i < len(d) {
		c := d[i]
		if c == ' ' || c == ',' || c == '\n' || c == '\t' || c == '\r' {
			i++
			continue
		}
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			// Z/z closes the current subpath the moment the command letter
			// is seen (SVG semantics): it takes no arguments, so the numeric
			// switch below never runs for it. Emitting the close here keeps
			// the explicit closing edge in the segment list - without it a
			// closed path strokes as an OPEN polyline (the file-sheet icon
			// lost its left edge), and dashes never cover the final side.
			// Fills hid the bug because PathBuilder.Build auto-closes fills.
			if c == 'Z' || c == 'z' {
				push(segClose{})
				cx, cy = startX, startY
			}
			cmd = rune(c)
			i++
			nums = nil
			continue
		}
		switch cmd {
		case 'M', 'm':
			v := nextNums(2)
			if len(v) < 2 {
				i++
				continue
			}
			if cmd == 'm' {
				v[0] += cx
				v[1] += cy
			}
			push(segMove{x: v[0], y: v[1]})
			cx, cy = v[0], v[1]
			startX, startY = cx, cy
			if cmd == 'm' {
				cmd = 'l'
			} else {
				cmd = 'L'
			}
		case 'L', 'l':
			v := nextNums(2)
			if len(v) < 2 {
				i++
				continue
			}
			if cmd == 'l' {
				v[0] += cx
				v[1] += cy
			}
			push(segLine{x: v[0], y: v[1]})
			cx, cy = v[0], v[1]
		case 'H', 'h':
			v := nextNums(1)
			if len(v) < 1 {
				i++
				continue
			}
			x := v[0]
			if cmd == 'h' {
				x += cx
			}
			push(segLine{x: x, y: cy})
			cx = x
		case 'V', 'v':
			v := nextNums(1)
			if len(v) < 1 {
				i++
				continue
			}
			y := v[0]
			if cmd == 'v' {
				y += cy
			}
			push(segLine{x: cx, y: y})
			cy = y
		case 'Q', 'q':
			v := nextNums(4)
			if len(v) < 4 {
				i++
				continue
			}
			if cmd == 'q' {
				v[0] += cx
				v[1] += cy
				v[2] += cx
				v[3] += cy
			}
			push(segQuad{cx: v[0], cy: v[1], x: v[2], y: v[3]})
			prevCtrlX, prevCtrlY = v[0], v[1]
			havePrevCtrl = true
			cx, cy = v[2], v[3]
		case 'T', 't':
			v := nextNums(2)
			if len(v) < 2 {
				i++
				continue
			}
			if cmd == 't' {
				v[0] += cx
				v[1] += cy
			}
			var qx, qy float32
			if havePrevCtrl {
				qx, qy = 2*cx-prevCtrlX, 2*cy-prevCtrlY
			} else {
				qx, qy = cx, cy
			}
			push(segQuad{cx: qx, cy: qy, x: v[0], y: v[1]})
			prevCtrlX, prevCtrlY = qx, qy
			havePrevCtrl = true
			cx, cy = v[0], v[1]
		case 'C', 'c':
			v := nextNums(6)
			if len(v) < 6 {
				i++
				continue
			}
			if cmd == 'c' {
				for j := 0; j < 6; j += 2 {
					v[j] += cx
					v[j+1] += cy
				}
			}
			push(segCubic{c1x: v[0], c1y: v[1], c2x: v[2], c2y: v[3], x: v[4], y: v[5]})
			prevCtrlX, prevCtrlY = v[2], v[3]
			havePrevCtrl = true
			cx, cy = v[4], v[5]
		case 'S', 's':
			v := nextNums(4)
			if len(v) < 4 {
				i++
				continue
			}
			if cmd == 's' {
				for j := 0; j < 4; j += 2 {
					v[j] += cx
					v[j+1] += cy
				}
			}
			c1x, c1y := cx, cy
			if havePrevCtrl {
				c1x, c1y = 2*cx-prevCtrlX, 2*cy-prevCtrlY
			}
			push(segCubic{c1x: c1x, c1y: c1y, c2x: v[0], c2y: v[1], x: v[2], y: v[3]})
			prevCtrlX, prevCtrlY = v[0], v[1]
			havePrevCtrl = true
			cx, cy = v[2], v[3]
		case 'Z', 'z':
			push(segClose{})
			cx, cy = startX, startY
		case 'A', 'a':
			v := nextNums(7)
			if len(v) < 7 {
				i++
				continue
			}
			x, y := v[5], v[6]
			if cmd == 'a' {
				x += cx
				y += cy
			}
			if x != cx || y != cy {
				if v[0] == 0 || v[1] == 0 {
					push(segLine{x: x, y: y})
				} else {
					render.SVGArcCubics(geometry.Point[float32]{X: cx, Y: cy}, geometry.Size[float32]{Width: v[0], Height: v[1]}, v[2], v[3] != 0, v[4] != 0,
						geometry.Point[float32]{X: x, Y: y}, func(c1, c2, to geometry.Point[float32]) {
							push(segCubic{c1x: c1.X, c1y: c1.Y, c2x: c2.X, c2y: c2.Y, x: to.X, y: to.Y})
						})
				}
			}
			havePrevCtrl = false
			cx, cy = x, y
		default:
			i++
		}
	}
	return out
}

var _ = fmt.Sprintf
var _ = math.Pi

// ---------------------------------------------------------------------------
// Stroke dashing (upstream resvg stroke-dasharray)
// ---------------------------------------------------------------------------

// dashSegments splits a flattened path into dash segments: walking the
// primitives in order, the dash pattern alternates drawn/blank lengths
// (an odd count repeats the list once, per the SVG spec). Curves are
// subdivided to lines before dashing. Each returned group is one or more
// contiguous drawn runs, starting with a MoveTo.
func dashSegments(segs []svgSegment, pattern []float32, offset float32) [][]svgSegment {
	// Normalize the pattern: positive lengths only; odd lists repeat.
	var pat []float32
	for _, d := range pattern {
		if d > 0 {
			pat = append(pat, d)
		}
	}
	if len(pat) == 0 {
		return nil
	}
	if len(pat)%2 == 1 {
		pat = append(append([]float32{}, pat...), pat...)
	}
	sum := float32(0)
	for _, d := range pat {
		sum += d
	}
	if sum <= 0 {
		return nil
	}

	// Flatten curves into line polylines (dash math runs on straight runs).
	type pt struct{ x, y float32 }
	var lines []struct {
		a, b pt
		move bool // the line starts a new subpath
	}
	cur := pt{0, 0}
	start := pt{0, 0}
	haveCur := false
	appendCurve := func(p0, c pt, cubic bool, p1, p2 pt) {
		const steps = 24
		prev := p0
		for i := 1; i <= steps; i++ {
			t := float32(i) / steps
			var q pt
			if cubic {
				u := 1 - t
				q.x = u*u*u*p0.x + 3*u*u*t*p1.x + 3*u*t*t*p2.x + t*t*t*c.x
				q.y = u*u*u*p0.y + 3*u*u*t*p1.y + 3*u*t*t*p2.y + t*t*t*c.y
			} else {
				u := 1 - t
				q.x = u*u*p0.x + 2*u*t*p1.x + t*t*c.x
				q.y = u*u*p0.y + 2*u*t*p1.y + t*t*c.y
			}
			lines = append(lines, struct {
				a, b pt
				move bool
			}{prev, q, false})
			prev = q
		}
	}
	for _, seg := range segs {
		switch v := seg.(type) {
		case segMove:
			cur = pt{v.x, v.y}
			start = cur
			haveCur = true
		case segLine:
			if haveCur {
				lines = append(lines, struct {
					a, b pt
					move bool
				}{cur, pt{v.x, v.y}, false})
			}
			cur = pt{v.x, v.y}
			haveCur = true
		case segQuad:
			if haveCur {
				appendCurve(cur, pt{v.x, v.y}, false, pt{v.cx, v.cy}, pt{})
			}
			cur = pt{v.x, v.y}
		case segCubic:
			if haveCur {
				appendCurve(cur, pt{v.x, v.y}, true, pt{v.c1x, v.c1y}, pt{v.c2x, v.c2y})
			}
			cur = pt{v.x, v.y}
		case segClose:
			if haveCur && (cur.x != start.x || cur.y != start.y) {
				lines = append(lines, struct {
					a, b pt
					move bool
				}{cur, start, false})
			}
			cur = start
		}
	}

	// Walk the lines applying the pattern.
	var out [][]svgSegment
	var run []svgSegment
	flushRun := func() {
		if len(run) > 1 { // a MoveTo alone is nothing
			out = append(out, run)
		}
		run = nil
	}
	dashIdx := 0
	dashRemaining := pat[0]
	drawing := true
	// stroke-dashoffset seeds the pattern state: the pattern starts that far
	// into its cycle (negative offsets wrap forward through the cycle, SVG
	// spec 11.4).
	if offset != 0 {
		for offset < 0 {
			offset += sum
		}
		for offset > 0 {
			if offset >= dashRemaining {
				offset -= dashRemaining
				dashIdx = (dashIdx + 1) % len(pat)
				dashRemaining = pat[dashIdx]
				drawing = !drawing
			} else {
				dashRemaining -= offset
				offset = 0
			}
		}
	}
	for li, ln := range lines {
		isFirstOfSubpath := false
		if li == 0 || lines[li-1].b != ln.a {
			isFirstOfSubpath = true
		}
		_ = isFirstOfSubpath
		ax, ay := ln.a.x, ln.a.y
		bx, by := ln.b.x, ln.b.y
		segLen := sqrtf((bx-ax)*(bx-ax) + (by-ay)*(by-ay))
		if segLen <= 1e-6 {
			continue
		}
		// Unit direction of the ORIGINAL line: steps advance along it, not
		// along the shrinking remainder (that under-stepped each dash).
		ux, uy := (bx-ax)/segLen, (by-ay)/segLen
		for remaining := segLen; remaining > 1e-6; {
			if dashRemaining <= 1e-6 {
				// Advance the pattern.
				dashIdx = (dashIdx + 1) % len(pat)
				dashRemaining = pat[dashIdx]
				drawing = !drawing
				if !drawing {
					flushRun()
				}
				continue
			}
			take := dashRemaining
			if take > remaining {
				take = remaining
			}
			nx := ax + ux*take
			ny := ay + uy*take
			if drawing {
				if len(run) == 0 {
					run = append(run, segMove{x: ax, y: ay})
				}
				run = append(run, segLine{x: nx, y: ny})
			}
			ax, ay = nx, ny
			remaining -= take
			dashRemaining -= take
		}
		// A zero-length jump between subpaths starts fresh.
	}
	flushRun()
	return out
}
