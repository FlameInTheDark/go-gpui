package render

import (
	"math"

	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
)

// Rasterizer paints a Scene into an RGBA8 framebuffer in pure Go. The
// per-pixel math mirrors the upstream wgsl shaders.

type Rasterizer struct {
	width, height int
	pix           []float32 // linear RGBA, w*h*4
	atlas         *Atlas
}

// NewRasterizer creates a framebuffer rasterizer.
func NewRasterizer(width, height int, atlas *Atlas) *Rasterizer {
	return &Rasterizer{
		width:  width,
		height: height,
		pix:    make([]float32, width*height*4),
		atlas:  atlas,
	}
}

// Size returns the framebuffer size.
func (r *Rasterizer) Size() (int, int) { return r.width, r.height }

// Clear resets the framebuffer to transparent black.
func (r *Rasterizer) Clear() {
	for i := range r.pix {
		r.pix[i] = 0
	}
}

// Image converts the framebuffer to an 8-bit RGBA image (straight alpha).
// Values are written raw like the upstream non-sRGB targets.
func (r *Rasterizer) Image() *RenderImage {
	out := &RenderImage{Width: r.width, Height: r.height, Pixels: make([]uint8, r.width*r.height*4)}
	r.ImageInto(out)
	return out
}

// ImageInto converts the framebuffer into an existing image (the GDI
// present path reuses one buffer per window instead of allocating a
// fresh frame-sized image every frame).
func (r *Rasterizer) ImageInto(out *RenderImage) {
	n := r.width * r.height * 4
	if cap(out.Pixels) < n {
		out.Pixels = make([]uint8, n)
	} else {
		out.Pixels = out.Pixels[:n]
	}
	out.Width = r.width
	out.Height = r.height
	src := r.pix[:n:n]
	dst := out.Pixels[:n:n]
	// Flat indexing over full-capacity slices (no per-pixel slice
	// headers): this loop runs millions of times per software frame.
	for i := 0; i < n; i += 4 {
		dst[i] = toByte(src[i])
		dst[i+1] = toByte(src[i+1])
		dst[i+2] = toByte(src[i+2])
		dst[i+3] = toByte(src[i+3])
	}
}

// toByteLUT maps [0,1] quantized to 1/4096 steps onto 8-bit outputs
// (Image() converts millions of pixels per frame on the software path).
var toByteLUT = func() [4097]uint8 {
	var lut [4097]uint8
	for i := range lut {
		lut[i] = uint8(float32(i)/4096.0*255.0 + 0.5)
	}
	return lut
}()

func toByte(v float32) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 255
	}
	return toByteLUT[int(v*4096)]
}

// PaintScene paints a whole scene back to front.
func (r *Rasterizer) PaintScene(scene *Scene) {
	for _, idx := range scene.SortedIndices() {
		switch idx.kind {
		case kindShadow:
			r.paintShadow(scene.Shadows[idx.index])
		case kindQuad:
			r.paintQuad(scene.Quads[idx.index])
		case kindPath:
			r.paintPath(scene.Paths[idx.index])
		case kindUnder:
			r.paintUnderline(scene.Underlines[idx.index])
		case kindMono:
			r.paintMonoSprite(scene.MonoSprites[idx.index])
		case kindSub:
			r.paintSubpixelSprite(scene.SubpixelSprites[idx.index])
		case kindPoly:
			r.paintPolySprite(scene.PolySprites[idx.index])
		}
	}
}

func (r *Rasterizer) blendPixel(x, y int, above rgbaF) {
	if above.A <= 0 {
		return
	}
	if x < 0 || y < 0 || x >= r.width || y >= r.height {
		return
	}
	i := (y*r.width + x) * 4
	dst := rgbaF{R: r.pix[i], G: r.pix[i+1], B: r.pix[i+2], A: r.pix[i+3]}
	out := over(dst, above)
	r.pix[i], r.pix[i+1], r.pix[i+2], r.pix[i+3] = out.R, out.G, out.B, out.A
}

// pixelBounds intersects primitive bounds with the content mask and the
// framebuffer, returning the integer pixel range to iterate.
func (r *Rasterizer) pixelBounds(bounds geometry.Bounds[geometry.ScaledPixels], mask ContentMask) (x0, y0, x1, y1 int, ok bool) {
	b := bounds
	if mask.Bounds.Size.Width.AsF32() < 1e8 {
		b = IntersectScaled(b, mask.Bounds)
	}
	x0f := math.Ceil(float64(b.Origin.X.AsF32() - 0.5))
	y0f := math.Ceil(float64(b.Origin.Y.AsF32() - 0.5))
	x1f := math.Floor(float64(b.Right().AsF32() + 0.5))
	y1f := math.Floor(float64(b.Bottom().AsF32() + 0.5))
	x0 = int(x0f)
	y0 = int(y0f)
	x1 = int(x1f)
	y1 = int(y1f)
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > r.width-1 {
		x1 = r.width - 1
	}
	if y1 > r.height-1 {
		y1 = r.height - 1
	}
	return x0, y0, x1, y1, x0 <= x1 && y0 <= y1
}

// IntersectScaled intersects two scaled bounds, collapsing to zero.
func IntersectScaled(a, b geometry.Bounds[geometry.ScaledPixels]) geometry.Bounds[geometry.ScaledPixels] {
	x0 := maxScaled(a.Origin.X, b.Origin.X)
	y0 := maxScaled(a.Origin.Y, b.Origin.Y)
	x1 := minScaled(a.BottomRight().X, b.BottomRight().X)
	y1 := minScaled(a.BottomRight().Y, b.BottomRight().Y)
	if x1 < x0 {
		x1 = x0
	}
	if y1 < y0 {
		y1 = y0
	}
	return geometry.Bounds[geometry.ScaledPixels]{
		Origin: geometry.Point[geometry.ScaledPixels]{X: x0, Y: y0},
		Size:   geometry.Size[geometry.ScaledPixels]{Width: x1 - x0, Height: y1 - y0},
	}
}

func maxScaled(a, b geometry.ScaledPixels) geometry.ScaledPixels {
	if a.Cmp(b) >= 0 {
		return a
	}
	return b
}

func minScaled(a, b geometry.ScaledPixels) geometry.ScaledPixels {
	if a.Cmp(b) < 0 {
		return a
	}
	return b
}

// ---------------------------------------------------------------------------
// Quads (fs_quad port)
// ---------------------------------------------------------------------------

func hasRoundedCorners(c geometry.Corners[geometry.ScaledPixels]) bool {
	return c.TopLeft.AsF32() > 0 || c.TopRight.AsF32() > 0 ||
		c.BottomRight.AsF32() > 0 || c.BottomLeft.AsF32() > 0
}

func pickCornerRadius(cx, cy float32, c geometry.Corners[geometry.ScaledPixels]) float32 {
	if cx < 0 {
		if cy < 0 {
			return c.TopLeft.AsF32()
		}
		return c.BottomLeft.AsF32()
	}
	if cy < 0 {
		return c.TopRight.AsF32()
	}
	return c.BottomRight.AsF32()
}

func pickCornerEdge(cx, cy float32, e geometry.Edges[geometry.ScaledPixels]) float32 {
	if cx < 0 {
		if cy < 0 {
			return e.Top.AsF32()
		}
		return e.Bottom.AsF32()
	}
	if cy < 0 {
		return e.Right.AsF32()
	}
	return e.Left.AsF32()
}

func quadSDFImpl(ccx, ccy, radius float32) float32 {
	if radius == 0 {
		return maxf(ccx, ccy)
	}
	sdInset := float32(math.Sqrt(float64(maxf(0, ccx)*maxf(0, ccx)+maxf(0, ccy)*maxf(0, ccy)))) +
		minf(0, maxf(ccx, ccy))
	return sdInset - radius
}

func quadSDF(x, y float32, bounds geometry.Bounds[geometry.ScaledPixels], radii geometry.Corners[geometry.ScaledPixels]) float32 {
	hw := bounds.Size.Width.AsF32() / 2
	hh := bounds.Size.Height.AsF32() / 2
	px := x - bounds.Origin.X.AsF32() - hw
	py := y - bounds.Origin.Y.AsF32() - hh
	ccx := abs32(px) - hw
	ccy := abs32(py) - hh
	r := pickCornerRadius(px, py, radii)
	return quadSDFImpl(ccx+r, ccy+r, r)
}

func quarterEllipseSDF(px, py, rx, ry float32) float32 {
	cx, cy := px/rx, py/ry
	unit := float32(math.Sqrt(float64(cx*cx+cy*cy))) - 1
	return unit * (rx + ry) * -0.5
}

func (r *Rasterizer) paintQuad(q Quad) {
	x0, y0, x1, y1, ok := r.pixelBounds(q.Bounds, q.ContentMask)
	if !ok {
		return
	}
	prepared := prepareBackground(q.Background)
	unrounded := !hasRoundedCorners(q.CornerRadii)
	aaThreshold := float32(0.5)

	// Solid, borderless quads have an interior strip that needs no SDF.
	// For rounded quads stay outside every corner's radius and leave the
	// AA edges to the general shader. An opaque interior is a plain store.
	if prepared.tag == 0 && q.BorderStyle == 0 &&
		q.BorderWidths.Top.AsF32() <= 0 && q.BorderWidths.Right.AsF32() <= 0 &&
		q.BorderWidths.Bottom.AsF32() <= 0 && q.BorderWidths.Left.AsF32() <= 0 &&
		(unrounded || (q.BorderWidths == (geometry.Edges[geometry.ScaledPixels]{}) && q.BorderColor.A == 0)) {
		bg := prepared.color0
		solid := bg.A >= 1
		ox := q.Bounds.Origin.X.AsF32()
		oy := q.Bounds.Origin.Y.AsF32()
		bw := q.Bounds.Size.Width.AsF32()
		bh := q.Bounds.Size.Height.AsF32()
		// The interior band (pixel centers strictly inside the quad)
		// is the same for every row/column: derive it once. Pixel x
		// is inside when x+0.5 > ox (x >= floor(ox-0.5)+1) and
		// x+0.5 < ox+bw (x <= ceil(ox+bw-0.5)-1).
		inset := float32(0)
		if !unrounded {
			c := q.CornerRadii
			inset = max(c.TopLeft.AsF32(), c.TopRight.AsF32(), c.BottomLeft.AsF32(), c.BottomRight.AsF32())
		}
		xi0 := max(x0, int(math.Floor(float64(ox+inset-0.5)))+1)
		xi1 := min(x1, int(math.Ceil(float64(ox+bw-inset-0.5)))-1)
		yi0 := int(math.Floor(float64(oy-0.5))) + 1
		yi1 := int(math.Ceil(float64(oy+bh-0.5))) - 1
		if xi0 > xi1 {
			xi0, xi1 = x0, x0-1
		}
		pix := r.pix
		w4 := r.width * 4
		for y := y0; y <= y1; y++ {
			row := y * w4
			if y < yi0 || y > yi1 {
				for x := x0; x <= x1; x++ {
					r.paintQuadEdge(q, prepared, x, y, aaThreshold, unrounded)
				}
				continue
			}
			for x := x0; x < xi0; x++ {
				r.paintQuadEdge(q, prepared, x, y, aaThreshold, unrounded)
			}
			if solid {
				// Interior of an opaque quad: over(dst, src)
				// == src, a plain 4-float store (the editor
				// paints several full-window background layers
				// per frame, so this loop dominates the frame).
				i := row + xi0*4
				end := row + (xi1+1)*4
				for ; i < end; i += 4 {
					pix[i] = bg.R
					pix[i+1] = bg.G
					pix[i+2] = bg.B
					pix[i+3] = bg.A
				}
			} else {
				for x := xi0; x <= xi1; x++ {
					r.blendPixel(x, y, bg)
				}
			}
			for x := xi1 + 1; x <= x1; x++ {
				r.paintQuadEdge(q, prepared, x, y, aaThreshold, unrounded)
			}
		}
		return
	}

	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			r.paintQuadEdge(q, prepared, x, y, aaThreshold, unrounded)
		}
	}
}

// paintQuadEdge runs the full per-pixel quad math for one pixel: the AA
// edge ring of the fast path and every pixel of the general path. The
// math is ported verbatim from the original paintQuad loop body.
func (r *Rasterizer) paintQuadEdge(q Quad, prepared preparedBackground, x, y int, aaThreshold float32, unrounded bool) {
	px := float32(x) + 0.5
	py := float32(y) + 0.5
	background := backgroundColor(prepared, q.Background, q.Bounds, px, py, x, y)

	w := q.Bounds.Size.Width.AsF32()
	h := q.Bounds.Size.Height.AsF32()
	hw, hh := w/2, h/2
	cx := px - q.Bounds.Origin.X.AsF32() - hw
	cy := py - q.Bounds.Origin.Y.AsF32() - hh

	cornerRadius := pickCornerRadius(cx, cy, q.CornerRadii)

	borderX := q.BorderWidths.Right.AsF32()
	if cx < 0 {
		borderX = q.BorderWidths.Left.AsF32()
	}
	borderY := q.BorderWidths.Bottom.AsF32()
	if cy < 0 {
		borderY = q.BorderWidths.Top.AsF32()
	}

	reducedX := borderX
	if borderX == 0 {
		reducedX = -aaThreshold
	}
	reducedY := borderY
	if borderY == 0 {
		reducedY = -aaThreshold
	}

	cornerToX := abs32(cx) - hw
	cornerToY := abs32(cy) - hh
	ccx := cornerToX + cornerRadius
	ccy := cornerToY + cornerRadius
	isNearRounded := ccx >= 0 && ccy >= 0

	straightX := cornerToX + reducedX
	straightY := cornerToY + reducedY
	isBeyondInnerStraight := straightX > 0 || straightY > 0
	isWithinInnerStraight := straightX < -aaThreshold && straightY < -aaThreshold

	var color rgbaF
	if isWithinInnerStraight && !isNearRounded {
		color = background
	} else {
		outerSDF := quadSDFImpl(ccx, ccy, cornerRadius)

		var innerSDF float32
		if ccx <= 0 || ccy <= 0 {
			innerSDF = -maxf(straightX, straightY)
		} else if isBeyondInnerStraight {
			innerSDF = -1
		} else if reducedX == reducedY {
			innerSDF = -(outerSDF + reducedX)
		} else {
			erx := maxf(0, cornerRadius-reducedX)
			ery := maxf(0, cornerRadius-reducedY)
			innerSDF = quarterEllipseSDF(ccx, ccy, erx, ery)
		}

		borderSDF := maxf(innerSDF, outerSDF)
		color = background
		if borderSDF < aaThreshold {
			borderColor := hslaToRgbaF(q.BorderColor)
			if q.BorderStyle == 1 {
				borderColor.A *= r.dashAlpha(q, cx, cy, cornerRadius, unrounded,
					ccx, ccy, isNearRounded, cornerToX, cornerToY, w, h, aaThreshold)
			}
			borderColor.A *= saturate(aaThreshold - innerSDF)
			color = over(background, borderColor)
		}
		color.A *= saturate(aaThreshold - outerSDF)
		if color.A <= 0 {
			return
		}
	}
	r.blendPixel(x, y, color)
}

// dashAlpha ports the wgsl dashed-border parameterization.
func (r *Rasterizer) dashAlpha(q Quad, cx, cy, cornerRadius float32, unrounded bool,
	ccx, ccy float32, isNearRounded bool, cornerToX, cornerToY, w, h, aaThreshold float32) float32 {
	const dashLengthPerWidth = float32(2.0)
	const dashGapPerWidth = float32(1.0)
	const dashPeriodPerWidth = dashLengthPerWidth + dashGapPerWidth
	dvNumerator := 1.0 / dashPeriodPerWidth

	var t, maxT, dashVelocity float32

	if unrounded {
		isHorizontal := ccx < ccy
		dashedBorderX := maxf(q.BorderWidths.Bottom.AsF32(), q.BorderWidths.Top.AsF32())
		dashedBorderY := maxf(q.BorderWidths.Right.AsF32(), q.BorderWidths.Left.AsF32())
		borderWidth := dashedBorderY
		if isHorizontal {
			borderWidth = dashedBorderX
		}
		if borderWidth <= 0 {
			return 1
		}
		dashVelocity = dvNumerator / borderWidth
		point := cx + w/2
		size := w
		if isHorizontal {
			point = cy + h/2
			size = h
		}
		t = point * dashVelocity
		maxT = size * dashVelocity
	} else {
		rTR := q.CornerRadii.TopRight.AsF32()
		rBR := q.CornerRadii.BottomRight.AsF32()
		rBL := q.CornerRadii.BottomLeft.AsF32()
		rTL := q.CornerRadii.TopLeft.AsF32()
		wT := q.BorderWidths.Top.AsF32()
		wR := q.BorderWidths.Right.AsF32()
		wB := q.BorderWidths.Bottom.AsF32()
		wL := q.BorderWidths.Left.AsF32()
		dvT := pickDashV(dvNumerator, wT)
		dvR := pickDashV(dvNumerator, wR)
		dvB := pickDashV(dvNumerator, wB)
		dvL := pickDashV(dvNumerator, wL)
		sT := (w - rTL - rTR) * dvT
		sR := (h - rTR - rBR) * dvR
		sB := (w - rBR - rBL) * dvB
		sL := (h - rBL - rTL) * dvL
		cvTR := cornerDashVelocity(dvT, dvR)
		cvBR := cornerDashVelocity(dvB, dvR)
		cvBL := cornerDashVelocity(dvB, dvL)
		cvTL := cornerDashVelocity(dvT, dvL)
		cTR := rTR * (float32(math.Pi) / 2) * cvTR
		cBR := rBR * (float32(math.Pi) / 2) * cvBR
		cBL := rBL * (float32(math.Pi) / 2) * cvBL
		cTL := rTL * (float32(math.Pi) / 2) * cvTL
		uptoTR := sT
		uptoR := uptoTR + cTR
		uptoBR := uptoR + sR
		uptoB := uptoBR + cBR
		uptoBL := uptoB + sB
		uptoL := uptoBL + cBL
		uptoTL := uptoL + sL
		maxT = uptoTL + cTL

		pointX := cx + w/2
		pointY := cy + h/2
		if isNearRounded {
			radians := float32(math.Atan2(float64(ccy), float64(ccx)))
			cornerT := radians * cornerRadius
			if cx >= 0 {
				if cy < 0 {
					dashVelocity = cvTR
					t = uptoR - cornerT*dashVelocity
				} else {
					dashVelocity = cvBR
					t = uptoBR + cornerT*dashVelocity
				}
			} else {
				if cy >= 0 {
					dashVelocity = cvBL
					t = uptoL - cornerT*dashVelocity
				} else {
					dashVelocity = cvTL
					t = uptoTL + cornerT*dashVelocity
				}
			}
		} else {
			isHorizontal := ccx < ccy
			if isHorizontal {
				if cy < 0 {
					dashVelocity = dvT
					t = (pointX - rTL) * dashVelocity
				} else {
					dashVelocity = dvB
					t = uptoBL - (pointX-rBL)*dashVelocity
				}
			} else {
				if cx < 0 {
					dashVelocity = dvL
					t = uptoTL - (pointY-rTL)*dashVelocity
				} else {
					dashVelocity = dvR
					t = uptoR + (pointY-rTR)*dashVelocity
				}
			}
		}
	}

	dashLength := dashLengthPerWidth / dashPeriodPerWidth
	if unrounded {
		maxT -= dashLength
	}
	if maxT >= 1.0 {
		dashCount := float32(math.Floor(float64(maxT)))
		dashPeriod := maxT / dashCount
		return dashAlphaFn(t, dashPeriod, dashLength, dashVelocity, aaThreshold)
	} else if unrounded {
		dashGap := maxT - dashLength
		if dashGap > 0 {
			dashPeriod := dashLength + dashGap
			return dashAlphaFn(t, dashPeriod, dashLength, dashVelocity, aaThreshold)
		}
	}
	return 1
}

func pickDashV(num, w float32) float32 {
	if w <= 0 {
		return 0
	}
	return num / w
}

func cornerDashVelocity(dv1, dv2 float32) float32 {
	if dv1 == 0 {
		return dv2
	}
	if dv2 == 0 {
		return dv1
	}
	return minf(dv1, dv2)
}

func dashAlphaFn(t, period, length, dashVelocity, aaThreshold float32) float32 {
	if dashVelocity == 0 {
		return 1
	}
	halfPeriod := period / 2
	halfLength := length / 2
	centered := float32(math.Mod(float64(t+halfPeriod-halfLength), float64(period))) - halfPeriod
	signedDistance := abs32(centered) - halfLength
	return saturate(aaThreshold - signedDistance/dashVelocity)
}

// ---------------------------------------------------------------------------
// Shadows (fs_shadow port)
// ---------------------------------------------------------------------------

func (r *Rasterizer) paintShadow(sh Shadow) {
	x0, y0, x1, y1, ok := r.pixelBounds(sh.Bounds, sh.ContentMask)
	if !ok {
		return
	}
	color := hslaToRgbaF(sh.Color)

	var blur *preparedBlur
	if sh.BlurRadius.AsF32() > 0 {
		b := prepareBlurredRoundedRect(sh.ShapeBounds.Size, sh.ShapeRadii, sh.BlurRadius.AsF32())
		blur = &b
	}
	centerX := sh.ShapeBounds.Origin.X.AsF32() + sh.ShapeBounds.Size.Width.AsF32()/2
	centerY := sh.ShapeBounds.Origin.Y.AsF32() + sh.ShapeBounds.Size.Height.AsF32()/2

	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			px := float32(x) + 0.5
			py := float32(y) + 0.5
			var alpha float32
			if sh.BlurRadius.AsF32() == 0 {
				d := quadSDF(px, py, sh.ShapeBounds, sh.ShapeRadii)
				alpha = saturate(0.5 - d)
			} else {
				alpha = blurredRoundedRect(px-centerX, py-centerY, *blur)
			}
			if sh.Inset {
				alpha = 1.0 - alpha
				elementDistance := quadSDF(px, py, sh.Bounds, sh.CornerRadii)
				alpha *= saturate(0.5 - elementDistance)
			}
			if alpha <= 0 {
				continue
			}
			c := color
			c.A *= alpha
			r.blendPixel(x, y, c)
		}
	}
}

// ---------------------------------------------------------------------------
// Underlines (fs_underline port)
// ---------------------------------------------------------------------------

func (r *Rasterizer) paintUnderline(u Underline) {
	x0, y0, x1, y1, ok := r.pixelBounds(u.Bounds, u.ContentMask)
	if !ok {
		return
	}
	color := hslaToRgbaF(u.Color)
	if !u.Wavy {
		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				c := color
				c.A *= color.A
				r.blendPixel(x, y, c)
			}
		}
		return
	}
	const waveFrequency = 2.0
	const waveHeightRatio = 0.8
	halfThickness := u.Thickness.AsF32() * 0.5
	bh := u.Bounds.Size.Height.AsF32()
	frequency := float32(math.Pi) * waveFrequency * u.Thickness.AsF32() / bh
	amplitude := (u.Thickness.AsF32() * waveHeightRatio) / bh

	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			px := float32(x) + 0.5
			py := float32(y) + 0.5
			stX := (px - u.Bounds.Origin.X.AsF32()) / bh
			stY := (py-u.Bounds.Origin.Y.AsF32())/bh - 0.5
			sine := float32(math.Sin(float64(stX*frequency))) * amplitude
			dSine := float32(math.Cos(float64(stX*frequency))) * amplitude * frequency
			distance := (stY - sine) / float32(math.Sqrt(1+float64(dSine*dSine)))
			dip := distance * bh
			alpha := saturate(0.5 - maxf(-dip-halfThickness, dip-halfThickness))
			if alpha <= 0 {
				continue
			}
			c := color
			c.A *= alpha * color.A
			r.blendPixel(x, y, c)
		}
	}
}

// ---------------------------------------------------------------------------
// Sprites
// ---------------------------------------------------------------------------

// tileMono fetches a monochrome tile's pixels (once per sprite, not once
// per pixel: the per-pixel loop used to re-lock the atlas for every pixel
// and dominated software-frame profiles).
func (r *Rasterizer) tileMono(index uint32) (data []uint8, w, h int, ok bool) {
	if r.atlas == nil {
		return nil, 0, 0, false
	}
	data, w, h, mono, ok := r.atlas.Texture(AtlasMonochrome, int(index))
	if !ok || !mono {
		return nil, 0, 0, false
	}
	return data, w, h, true
}

// tilePoly fetches a polychrome tile's pixels (once per sprite).
func (r *Rasterizer) tilePoly(index uint32) (data []uint8, w, h int, ok bool) {
	if r.atlas == nil {
		return nil, 0, 0, false
	}
	data, w, h, mono, ok := r.atlas.Texture(AtlasPolychrome, int(index))
	if !ok || mono {
		return nil, 0, 0, false
	}
	return data, w, h, true
}

// tileUV maps sprite-space (u, v) into tile-page texel coordinates.
func tileUV(tile AtlasTile, u, v float32, w, h int) (float32, float32) {
	tu := (float32(tile.Bounds.Origin.X.AsI32()) + u*float32(tile.Bounds.Size.Width.AsI32())) / float32(w)
	tv := (float32(tile.Bounds.Origin.Y.AsI32()) + v*float32(tile.Bounds.Size.Height.AsI32())) / float32(h)
	return tu, tv
}

func (r *Rasterizer) paintMonoSprite(sp MonochromeSprite) {
	r.paintMonoCommon(sp.Bounds, sp.ContentMask, sp.Tile, sp.Color, sp.Transform)
}

func (r *Rasterizer) paintSubpixelSprite(sp SubpixelSprite) {
	// Grayscale fallback: average of RGB coverage.
	r.paintMonoCommon(sp.Bounds, sp.ContentMask, sp.Tile, sp.Color, geometry.IdentityMatrix())
}

const monoCachedColumns = 64

func (r *Rasterizer) paintMonoCommon(bounds geometry.Bounds[geometry.ScaledPixels], mask ContentMask, tile AtlasTile, color gc.Hsla, transform geometry.TransformationMatrix) {
	if bounds.Size.Width <= 0 || bounds.Size.Height <= 0 {
		return
	}
	x0, y0, x1, y1, ok := r.pixelBounds(bounds, mask)
	if !ok {
		return
	}
	data, w, h, ok := r.tileMono(tile.TextureIndex)
	if !ok {
		return
	}
	c := hslaToRgbaF(color)
	inverse := invertMatrix(transform)
	if inverse == geometry.IdentityMatrix() && x1-x0+1 <= monoCachedColumns {
		r.paintMonoAxisAligned(bounds, tile, c, data, w, h, x0, y0, x1, y1)
		return
	}
	r.paintMonoTransformed(bounds, tile, c, inverse, data, w, h, x0, y0, x1, y1)
}

func (r *Rasterizer) paintMonoTransformed(bounds geometry.Bounds[geometry.ScaledPixels], tile AtlasTile, c rgbaF, inverse geometry.TransformationMatrix, data []uint8, w, h, x0, y0, x1, y1 int) {
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			px := float32(x) + 0.5
			py := float32(y) + 0.5
			// Transform into sprite space.
			tp := inverse.Apply(geometry.Point[geometry.Pixels]{X: geometry.Px(px), Y: geometry.Px(py)})
			u := (tp.X.AsF32() - bounds.Origin.X.AsF32()) / bounds.Size.Width.AsF32()
			v := (tp.Y.AsF32() - bounds.Origin.Y.AsF32()) / bounds.Size.Height.AsF32()
			if u < 0 || u > 1 || v < 0 || v > 1 {
				continue
			}
			tu, tv := tileUV(tile, u, v, w, h)
			coverage := SampleMono(data, w, h, tu, tv)
			if coverage <= 0 {
				continue
			}
			sc := c
			sc.A *= coverage
			r.blendPixel(x, y, sc)
		}
	}
}

type monoAxisSample struct {
	lo, hi int
	frac   float32
}

func monoSampleAxis(pixel int, origin, length, tileOrigin, tileLength float32, textureSize int) monoAxisSample {
	u := (float32(pixel) + 0.5 - origin) / length
	if u < 0 || u > 1 {
		return monoAxisSample{lo: -1}
	}
	// Keep the same arithmetic and edge clamping as tileUV + SampleMono.
	tex := (tileOrigin + u*tileLength) / float32(textureSize)
	x := tex*float32(textureSize) - 0.5
	i := int(floorf(x))
	return monoAxisSample{lo: min(max(i, 0), textureSize-1), hi: min(max(i+1, 0), textureSize-1), frac: x - float32(i)}
}

// Most glyphs are untransformed and narrow. Compute their horizontal
// sample coordinates once, and pick the two texture rows once per scanline.
func (r *Rasterizer) paintMonoAxisAligned(bounds geometry.Bounds[geometry.ScaledPixels], tile AtlasTile, c rgbaF, data []uint8, w, h, x0, y0, x1, y1 int) {
	var cols [monoCachedColumns]monoAxisSample
	columns := cols[:x1-x0+1]
	for i := range columns {
		columns[i] = monoSampleAxis(x0+i, bounds.Origin.X.AsF32(), bounds.Size.Width.AsF32(),
			float32(tile.Bounds.Origin.X), float32(tile.Bounds.Size.Width), w)
	}
	for y := y0; y <= y1; y++ {
		v := monoSampleAxis(y, bounds.Origin.Y.AsF32(), bounds.Size.Height.AsF32(),
			float32(tile.Bounds.Origin.Y), float32(tile.Bounds.Size.Height), h)
		if v.lo < 0 {
			continue
		}
		row0, row1 := data[v.lo*w:][:w], data[v.hi*w:][:w]
		for i, u := range columns {
			if u.lo < 0 {
				continue
			}
			top := (float32(row0[u.lo])/255)*(1-u.frac) + (float32(row0[u.hi])/255)*u.frac
			bottom := (float32(row1[u.lo])/255)*(1-u.frac) + (float32(row1[u.hi])/255)*u.frac
			coverage := top*(1-v.frac) + bottom*v.frac
			if coverage > 0 {
				sc := c
				sc.A *= coverage
				r.blendPixel(x0+i, y, sc)
			}
		}
	}
}

func invertMatrix(m geometry.TransformationMatrix) geometry.TransformationMatrix {
	if m == geometry.IdentityMatrix() {
		return m
	}
	det := m.RotationScale[0][0]*m.RotationScale[1][1] - m.RotationScale[0][1]*m.RotationScale[1][0]
	if det == 0 {
		return geometry.IdentityMatrix()
	}
	invDet := 1 / det
	var inv geometry.TransformationMatrix
	inv.RotationScale[0][0] = m.RotationScale[1][1] * invDet
	inv.RotationScale[0][1] = -m.RotationScale[0][1] * invDet
	inv.RotationScale[1][0] = -m.RotationScale[1][0] * invDet
	inv.RotationScale[1][1] = m.RotationScale[0][0] * invDet
	inv.Translation[0] = -(inv.RotationScale[0][0]*m.Translation[0] + inv.RotationScale[0][1]*m.Translation[1])
	inv.Translation[1] = -(inv.RotationScale[1][0]*m.Translation[0] + inv.RotationScale[1][1]*m.Translation[1])
	return inv
}

func (r *Rasterizer) paintPolySprite(sp PolychromeSprite) {
	x0, y0, x1, y1, ok := r.pixelBounds(sp.Bounds, sp.ContentMask)
	if !ok {
		return
	}
	data, w, h, ok := r.tilePoly(sp.Tile.TextureIndex)
	if !ok {
		return
	}
	radii := sp.CornerRadii
	simple := radii.TopLeft.AsF32() == 0 && radii.TopRight.AsF32() == 0 &&
		radii.BottomRight.AsF32() == 0 && radii.BottomLeft.AsF32() == 0
	hw := sp.Bounds.Size.Width.AsF32() / 2
	hh := sp.Bounds.Size.Height.AsF32() / 2
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			px := float32(x) + 0.5
			py := float32(y) + 0.5
			u := (px - sp.Bounds.Origin.X.AsF32()) / sp.Bounds.Size.Width.AsF32()
			v := (py - sp.Bounds.Origin.Y.AsF32()) / sp.Bounds.Size.Height.AsF32()
			if u < 0 || u > 1 || v < 0 || v > 1 {
				continue
			}
			tu, tv := tileUV(sp.Tile, u, v, w, h)
			sr, sg, sb, sa := SamplePoly(data, w, h, tu, tv)
			var c rgbaF
			if sp.Grayscale {
				lum := 0.2126*sr + 0.7152*sg + 0.0722*sb
				c = rgbaF{R: lum, G: lum, B: lum, A: sa}
			} else {
				c = rgbaF{R: sr, G: sg, B: sb, A: sa}
			}
			c.A *= sp.Opacity
			if !simple {
				alpha := saturate(0.5 - quadSDF(px, py, sp.Bounds, radii))
				c.A *= alpha
			}
			if c.A <= 0 {
				continue
			}
			r.blendPixel(x, y, c)
		}
	}
	_ = hw
	_ = hh
}

// ---------------------------------------------------------------------------
// Paths: scanline winding rasterizer over monotone quadratics
// ---------------------------------------------------------------------------

func (r *Rasterizer) paintPath(p PaintedPath) {
	if p.Path == nil || len(p.Path.Curves) == 0 {
		return
	}
	bounds := p.Bounds
	if bounds.Size.Width.AsF32() == 0 && bounds.Size.Height.AsF32() == 0 {
		bounds = PathBounds(p.Path)
	}
	x0, y0, x1, y1, ok := r.pixelBounds(bounds, p.ContentMask)
	if !ok {
		return
	}
	prepared := prepareBackground(p.Color)
	evenOdd := p.Path.EvenOdd

	// Every horizontal sample on a subrow uses the same curve roots. Reuse
	// the two crossing lists across the row and keep small icons on the stack.
	var upperBuf, lowerBuf [16]pathCrossing
	upper, lower := upperBuf[:0], lowerBuf[:0]
	for y := y0; y <= y1; y++ {
		upper = pathCrossings(upper[:0], float32(y)+0.25, p.Path.Curves)
		lower = pathCrossings(lower[:0], float32(y)+0.75, p.Path.Curves)
		if len(upper) == 0 && len(lower) == 0 {
			continue
		}
		for x := x0; x <= x1; x++ {
			left, right := float32(x)+0.25, float32(x)+0.75
			covSum := pathCoverage(left, upper, evenOdd) + pathCoverage(right, upper, evenOdd) +
				pathCoverage(left, lower, evenOdd) + pathCoverage(right, lower, evenOdd)
			cov := covSum / 4
			if cov <= 0.001 {
				continue
			}
			px := float32(x) + 0.5
			py := float32(y) + 0.5
			bg := backgroundColor(prepared, p.Color, p.Bounds, px, py, x, y)
			bg.A *= cov
			r.blendPixel(x, y, bg)
		}
	}
}

type pathCrossing struct{ x, sign float32 }

// pathCrossings records the signed intersections with one subrow, in curve
// order. Keep the same endpoint and root checks as the per-sample ray test.
func pathCrossings(dst []pathCrossing, py float32, curves []PathCurve) []pathCrossing {
	for i := range curves {
		c := &curves[i]
		if py < c.P0.Y || py >= c.P1.Y {
			continue
		}
		// Solve y(t) = py for t in (0, 1]: y(t) = a t^2 + b t + p0.y.
		a, b := c.A.Y, c.B.Y
		y0 := c.P0.Y
		if a != 0 {
			disc := b*b - 4*a*(y0-py)
			if disc >= 0 {
				sq := float32(math.Sqrt(float64(disc)))
				dst = appendPathCrossing(dst, c, (-b+sq)/(2*a))
				dst = appendPathCrossing(dst, c, (-b-sq)/(2*a))
			}
		} else if b != 0 {
			dst = appendPathCrossing(dst, c, (py-y0)/b)
		}
	}
	return dst
}

func appendPathCrossing(dst []pathCrossing, c *PathCurve, t float32) []pathCrossing {
	if t > 0 && t <= 1 {
		dst = append(dst, pathCrossing{x: c.A.X*t*t + c.B.X*t + c.P0.X, sign: c.Sign})
	}
	return dst
}

func pathCoverage(px float32, crossings []pathCrossing, evenOdd bool) float32 {
	var winding float32
	for _, crossing := range crossings {
		if crossing.x >= px {
			winding += crossing.sign
		}
	}
	if evenOdd {
		w := float32(math.Mod(math.Abs(float64(winding)), 2))
		return min(w, 2-w)
	}
	return min(abs32(winding), 1)
}

// PathBounds computes the bounding box of a path from its curve endpoints
// (the curves are xy-monotone after the builder's splitting, so endpoints
// bound each segment; control points are clamped into the endpoint box).
func PathBounds(p *Path) geometry.Bounds[geometry.ScaledPixels] {
	if p == nil || len(p.Curves) == 0 {
		return geometry.Bounds[geometry.ScaledPixels]{}
	}
	minX, minY := p.Curves[0].P0.X, p.Curves[0].P0.Y
	maxX, maxY := minX, minY
	for i := range p.Curves {
		c := &p.Curves[i]
		for _, pt := range []geometry.Point[float32]{c.P0, c.P1} {
			if pt.X < minX {
				minX = pt.X
			}
			if pt.X > maxX {
				maxX = pt.X
			}
			if pt.Y < minY {
				minY = pt.Y
			}
			if pt.Y > maxY {
				maxY = pt.Y
			}
		}
	}
	return geometry.Bounds[geometry.ScaledPixels]{
		Origin: geometry.Point[geometry.ScaledPixels]{X: geometry.ScaledPx(minX), Y: geometry.ScaledPx(minY)},
		Size:   geometry.Size[geometry.ScaledPixels]{Width: geometry.ScaledPx(maxX - minX), Height: geometry.ScaledPx(maxY - minY)},
	}
}
