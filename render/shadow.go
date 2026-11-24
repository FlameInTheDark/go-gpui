package render

import (
	"math"

	"github.com/FlameInTheDark/go-gpui/geometry"
)

// erf7 is the shader's 7-term-ish erf approximation.
func erf7(input float32) float32 {
	x := clampf(input, -10.0, 10.0) * (2.0 / float32(math.Sqrt(math.Pi)))
	xx := x * x
	y := x + (0.24295+(0.03395+0.0104*xx)*xx)*(x*xx)
	return y / float32(math.Sqrt(1.0+float64(y*y)))
}

func clampf(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// preparedBlur is the shader's PreparedBlurredRoundedRect: Raph Levien's
// analytic blurred rounded rectangle.
type preparedBlur struct {
	shape    [4]float32 // 0.5*adjusted_size.xy, s_inv, min_edge
	r1       [4]float32 // corner radii (tl, tr, br, bl)
	exponent [4]float32
	scale    [4]float32
}

// prepareBlurredRoundedRect ports the wgsl prepare_blurred_rounded_rect.
func prepareBlurredRoundedRect(size geometry.Size[geometry.ScaledPixels], radii geometry.Corners[geometry.ScaledPixels], sigma float32) preparedBlur {
	s := float32(math.Max(float64(sigma)*math.Sqrt(2.0), 1e-6))
	sInv := 1.0 / s
	minEdge := minf(size.Width.AsF32(), size.Height.AsF32())
	rMax := 0.5 * minEdge
	rs := [4]float32{radii.TopLeft.AsF32(), radii.TopRight.AsF32(), radii.BottomRight.AsF32(), radii.BottomLeft.AsF32()}
	var r0, r1 [4]float32
	for i, r := range rs {
		r0[i] = minf(float32(math.Sqrt(float64(r*r+1.15*1.15*s*s))), rMax)
		r1[i] = minf(float32(math.Sqrt(float64(r*r+4.0*s*s))), rMax)
	}
	halfExtentX := 0.5 * sInv * size.Width.AsF32()
	halfExtentY := 0.5 * sInv * size.Height.AsF32()
	delta := 1.25 * s * (float32(math.Exp(-float64(halfExtentX*halfExtentX))) - float32(math.Exp(-float64(halfExtentY*halfExtentY))))
	adjX := size.Width.AsF32() + minf(delta, 0)
	adjY := size.Height.AsF32() - maxf(delta, 0)
	var peak [4]float32
	for i, r := range rs {
		v := sInv * 0.5 * (maxf(adjX, adjY) - 0.5*r)
		peak[i] = v
	}
	var out preparedBlur
	out.shape = [4]float32{0.5 * adjX, 0.5 * adjY, sInv, minEdge}
	out.r1 = r1
	for i := range r1 {
		out.exponent[i] = 2.0 * r1[i] / r0[i]
		out.scale[i] = 0.5 * erf7(peak[i])
	}
	return out
}

// pickCorner picks the value for the quadrant the point is in.
func pickCorner(cx, cy float32, v [4]float32) float32 {
	// order: tl, tr, br, bl
	if cx < 0 {
		if cy < 0 {
			return v[0]
		}
		return v[3]
	}
	if cy < 0 {
		return v[1]
	}
	return v[2]
}

// blurredRoundedRect ports the wgsl blurred_rounded_rect.
func blurredRoundedRect(centerToPointX, centerToPointY float32, p preparedBlur) float32 {
	r1 := pickCorner(centerToPointX, centerToPointY, p.r1)
	exponent := pickCorner(centerToPointX, centerToPointY, p.exponent)
	scale := pickCorner(centerToPointX, centerToPointY, p.scale)
	sInv := p.shape[2]
	minEdge := p.shape[3]

	p0x := abs32(centerToPointX) - (p.shape[0] - r1)
	p0y := abs32(centerToPointY) - (p.shape[1] - r1)
	p1x := maxf(p0x, 0)
	p1y := maxf(p0y, 0)
	var dPos float32
	if p1x > 0 && p1y > 0 {
		dPos = float32(math.Pow(float64(math.Pow(float64(p1x), float64(exponent))+math.Pow(float64(p1y), float64(exponent))), 1.0/float64(exponent)))
	} else {
		dPos = p1x + p1y
	}
	dNeg := minf(maxf(p0x, p0y), 0)
	d := dPos + dNeg - r1
	return scale * (erf7(sInv*(minEdge+d)) - erf7(sInv*d))
}
