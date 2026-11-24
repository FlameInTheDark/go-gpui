package render

import (
	"math"

	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
)

// This file ports the wgsl color pipeline exactly:
//
//   - hslaToRgba treats the HSL math as producing "linear" values (per the
//     upstream comment), which numerically equals the standard HSL formula.
//   - Solid colors use that value directly.
//   - Gradients: endpoints are converted once (linear->sRGB for the sRGB
//     space, linear->Oklab for Oklab); the mixed pixel color is converted
//     back to linear (srgba_to_linear / oklab_to_linear_srgb) and dithered;
//     compositing (over) happens in that space and is written raw to the
//     non-sRGB framebuffer, exactly like the upstream pipeline.

type rgbaF struct {
	R, G, B, A float32
}

// hslaToRgbaF is the shader's hsla_to_rgba.
func hslaToRgbaF(h gc.Hsla) rgbaF {
	hh := h.H * 6.0
	s := h.S
	l := h.L
	c := (1.0 - abs32(2.0*l-1.0)) * s
	x := c * (1.0 - abs32(mod2(hh)-1.0))
	m := l - c/2.0
	var r, g, b float32 = m, m, m
	switch {
	case hh >= 0 && hh < 1:
		r += c
		g += x
	case hh >= 1 && hh < 2:
		r += x
		g += c
	case hh >= 2 && hh < 3:
		g += c
		b += x
	case hh >= 3 && hh < 4:
		g += x
		b += c
	case hh >= 4 && hh < 5:
		r += x
		b += c
	default:
		r += c
		b += x
	}
	return rgbaF{R: r, G: g, B: b, A: h.A}
}

func mod2(v float32) float32 {
	v = float32(math.Mod(float64(v), 2.0))
	if v < 0 {
		v += 2
	}
	return v
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func saturate(v float32) float32 { return clamp01(v) }

// srgbToLinearC converts one sRGB channel to linear.
func srgbToLinearC(a float32) float32 {
	if a < 0.04045 {
		return a / 12.92
	}
	return float32(math.Pow((float64(a)+0.055)/1.055, 2.4))
}

func srgbToLinear(r, g, b float32) (float32, float32, float32) {
	return srgbToLinearC(r), srgbToLinearC(g), srgbToLinearC(b)
}

// linearToSrgbC converts one linear channel to sRGB.
func linearToSrgbC(a float32) float32 {
	if a < 0.0031308 {
		return a * 12.92
	}
	return 1.055*float32(math.Pow(float64(a), 1.0/2.4)) - 0.055
}

func linearToSrgb(r, g, b float32) (float32, float32, float32) {
	return linearToSrgbC(r), linearToSrgbC(g), linearToSrgbC(b)
}

type oklab struct{ L, A, B, alpha float32 }

// linearSrgbToOklab is the shader's linear_srgb_to_oklab.
func linearSrgbToOklab(c rgbaF) oklab {
	l := 0.4122214708*c.R + 0.5363325363*c.G + 0.0514459929*c.B
	m := 0.2119034982*c.R + 0.6806995451*c.G + 0.1073969566*c.B
	s := 0.0883024619*c.R + 0.2817188376*c.G + 0.6299787005*c.B
	l_ := cbrt(l)
	m_ := cbrt(m)
	s_ := cbrt(s)
	return oklab{
		L:     0.2104542553*l_ + 0.7936177850*m_ - 0.0040720468*s_,
		A:     1.9779984951*l_ - 2.4285922050*m_ + 0.4505937099*s_,
		B:     0.0259040371*l_ + 0.7827717662*m_ - 0.8086757660*s_,
		alpha: c.A,
	}
}

// oklabToLinearSrgb is the shader's oklab_to_linear_srgb.
func oklabToLinearSrgb(c oklab) rgbaF {
	l_ := c.L + 0.3963377774*c.A + 0.2158037573*c.B
	m_ := c.L - 0.1055613458*c.A - 0.0638541728*c.B
	s_ := c.L - 0.0894841775*c.A - 1.2914855480*c.B
	l := l_ * l_ * l_
	m := m_ * m_ * m_
	s := s_ * s_ * s_
	return rgbaF{
		R: 4.0767416621*l - 3.3077115913*m + 0.2309699292*s,
		G: -1.2684380046*l + 2.6097574011*m - 0.3413193965*s,
		B: -0.0041960863*l - 0.7034186147*m + 1.7076147010*s,
		A: c.alpha,
	}
}

func cbrt(v float32) float32 {
	return float32(math.Cbrt(float64(v)))
}

// over is the shader's over() (source-over in the working space).
func over(below, above rgbaF) rgbaF {
	alpha := above.A + below.A*(1.0-above.A)
	if alpha == 0 {
		return rgbaF{}
	}
	pre := rgbaF{
		R: above.R*above.A + below.R*below.A*(1.0-above.A),
		G: above.G*above.A + below.G*below.A*(1.0-above.A),
		B: above.B*above.A + below.B*below.A*(1.0-above.A),
		A: alpha,
	}
	return rgbaF{R: pre.R / alpha, G: pre.G / alpha, B: pre.B / alpha, A: alpha}
}

// ditherOffset is the shader's Bayer 4x4 dither remapped to [-1, +1].
func ditherOffset(x, y int) float32 {
	cx, cy := uint32(x)&3, uint32(y)&3
	z := cx ^ cy
	threshold := ((z & 1) << 3) | ((cy & 1) << 2) | (z & 2) | ((cy & 2) >> 1)
	return (float32(threshold)+0.5)*(1.0/8.0) - 1.0
}

// preparedBackground is the shader's PreparedBackground.
type preparedBackground struct {
	color0, color1 rgbaF
	tag            uint8
}

func prepareBackground(bg gc.Background) preparedBackground {
	var p preparedBackground
	p.tag = uint8(bg.Tag)
	if bg.Tag == gc.BackgroundTagLinearGradient {
		p.color0 = hslaToRgbaF(bg.Colors[0].Color)
		p.color1 = hslaToRgbaF(bg.Colors[1].Color)
		if bg.ColorSpace == gc.ColorSpaceSrgb {
			p.color0 = linearToSrgbaF(p.color0)
			p.color1 = linearToSrgbaF(p.color1)
		} else {
			// Keep oklab components (L, a, b, alpha) in the rgba slots so
			// the per-pixel mix happens in oklab like the shader.
			p.color0 = oklabToRgba(linearSrgbToOklab(p.color0))
			p.color1 = oklabToRgba(linearSrgbToOklab(p.color1))
		}
	} else {
		p.color0 = hslaToRgbaF(bg.Solid)
	}
	return p
}

func linearToSrgbaF(c rgbaF) rgbaF {
	r, g, b := linearToSrgb(c.R, c.G, c.B)
	return rgbaF{R: r, G: g, B: b, A: c.A}
}

func srgbaToLinearF(c rgbaF) rgbaF {
	r, g, b := srgbToLinear(c.R, c.G, c.B)
	return rgbaF{R: r, G: g, B: b, A: c.A}
}

func oklabToRgba(o oklab) rgbaF {
	// Store oklab channels in rgba slots (no conversion).
	return rgbaF{R: o.L, G: o.A, B: o.B, A: o.alpha}
}

// backgroundColor evaluates a Background at a position. Port of the wgsl
// background_color (gradients with aspect-corrected CSS angle, slash
// pattern, checkerboard).
func backgroundColor(p preparedBackground, bg gc.Background, bounds geometry.Bounds[geometry.ScaledPixels], x, y float32, pixX, pixY int) rgbaF {
	color := p.color0
	if p.tag == 0 {
		return color
	}
	switch p.tag {
	case 1: // linear gradient
		radians := (float32(math.Mod(float64(bg.GradientAngleOrPatternHeight), 360.0)) - 90.0) * float32(math.Pi) / 180.0
		dirX := float32(math.Cos(float64(radians)))
		dirY := float32(math.Sin(float64(radians)))
		w := bounds.Size.Width.AsF32()
		h := bounds.Size.Height.AsF32()
		if w > h {
			dirY *= h / w
		} else {
			dirX *= w / h
		}
		var extent float32
		if abs32(dirX) > abs32(dirY) {
			extent = w
		} else {
			extent = h
		}
		directionLength := float32(math.Sqrt(float64(dirX*dirX + dirY*dirY)))
		span := bg.Colors[1].Percentage - bg.Colors[0].Percentage
		offset := 0.5 - bg.Colors[0].Percentage
		var gx, gy, gz float32
		if directionLength != 0 && extent != 0 {
			if span == 0 {
				const step = 1e30
				gx = dirX / (directionLength * extent) * step
				gy = dirY / (directionLength * extent) * step
				gz = offset * step
			} else {
				gx = dirX / (directionLength * extent * span)
				gy = dirY / (directionLength * extent * span)
				gz = offset / span
			}
		}
		cx := bounds.Origin.X.AsF32() + w*0.5
		cy := bounds.Origin.Y.AsF32() + h*0.5
		t := saturate((x-cx)*gx + (y-cy)*gy + gz)
		color = mixRgba(p.color0, p.color1, t)
		if bg.ColorSpace == gc.ColorSpaceOklab {
			color = oklabToLinearSrgb(oklab{L: color.R, A: color.G, B: color.B, alpha: color.A})
		} else {
			color = srgbaToLinearF(color)
		}
		d := ditherOffset(pixX, pixY)
		color = rgbaF{R: color.R + d*1.0/255.0, G: color.G + d*1.0/255.0, B: color.B + d*1.0/255.0, A: color.A + d*1.5/255.0}
	case 2: // pattern slash
		packed := bg.GradientAngleOrPatternHeight
		patternWidth := (packed / 65535.0) / 255.0
		patternInterval := (float32(math.Mod(float64(packed), 65535.0))) / 255.0
		patternHeight := patternWidth + patternInterval
		stripeAngle := float32(math.Pi) / 4.0
		sd := geometry.Point[float32]{X: float32(math.Cos(float64(stripeAngle))), Y: float32(math.Sin(float64(stripeAngle)))}
		patternPeriod := patternHeight * float32(math.Sin(float64(stripeAngle)))
		threshold := patternPeriod * (patternWidth / patternHeight) / 2.0
		rel := (x-bounds.Origin.X.AsF32())*sd.X + (y-bounds.Origin.Y.AsF32())*sd.Y
		pattern := float32(math.Mod(float64(rel), float64(patternPeriod)))
		if pattern < 0 {
			pattern += patternPeriod
		}
		distance := minf(pattern, patternPeriod-pattern) - threshold
		color.A *= saturate(0.5 - distance)
	case 3: // checkerboard
		cellSize := bg.GradientAngleOrPatternHeight
		relX := x - bounds.Origin.X.AsF32()
		relY := y - bounds.Origin.Y.AsF32()
		xi := math.Floor(float64(relX / cellSize))
		yi := math.Floor(float64(relY / cellSize))
		parity := math.Mod(xi+yi, 2.0)
		if parity < 0 {
			parity += 2
		}
		color.A *= saturate(float32(parity))
	}
	return color
}

func mixRgba(a, b rgbaF, t float32) rgbaF {
	return rgbaF{
		R: a.R + (b.R-a.R)*t,
		G: a.G + (b.G-a.G)*t,
		B: a.B + (b.B-a.B)*t,
		A: a.A + (b.A-a.A)*t,
	}
}
