// Package color is a Go port of gpui's color system (color.rs, colors.rs):
// Hsla/Rgba with the exact upstream conversion algorithms, alpha blending,
// backgrounds (solid, linear gradients incl. Oklab interpolation, patterns)
// and the default component palette.
package color

import (
	"fmt"
	"math"
	"strings"
)

// Rgba is a color with red, green, blue and alpha in [0, 1].
type Rgba struct {
	R, G, B, A float32
}

// RGB constructs an opaque color from 0xRRGGBB.
func RGB(hex uint32) Rgba {
	return Rgba{
		R: float32((hex>>16)&0xff) / 255.0,
		G: float32((hex>>8)&0xff) / 255.0,
		B: float32(hex&0xff) / 255.0,
		A: 1,
	}
}

// RGBA constructs a color from 0xRRGGBBAA.
func RGBA(hex uint32) Rgba {
	return Rgba{
		R: float32((hex>>24)&0xff) / 255.0,
		G: float32((hex>>16)&0xff) / 255.0,
		B: float32((hex>>8)&0xff) / 255.0,
		A: float32(hex&0xff) / 255.0,
	}
}

// Opaque returns the color with alpha 1.
func (c Rgba) Opaque() Rgba { c.A = 1; return c }

// Opacity multiplies the alpha by the given factor.
func (c Rgba) Opacity(factor float32) Rgba {
	c.A = clamp01(c.A * clamp01(factor))
	return c
}

// Alpha replaces the alpha channel.
func (c Rgba) Alpha(a float32) Rgba { c.A = clamp01(a); return c }

// Blend composites other over c (alpha-over). The result keeps the
// background's alpha, matching upstream Rgba::blend.
func (c Rgba) Blend(other Rgba) Rgba {
	if other.A >= 1.0 {
		return other
	}
	if other.A <= 0.0 {
		return c
	}
	return Rgba{
		R: c.R*(1-other.A) + other.R*other.A,
		G: c.G*(1-other.A) + other.G*other.A,
		B: c.B*(1-other.A) + other.B*other.A,
		A: c.A,
	}
}

// ToU32 packs to 0xRRGGBBAA (components truncated like Rust's `as u32`).
func (c Rgba) ToU32() uint32 {
	r := uint32(c.R * 255.0)
	g := uint32(c.G * 255.0)
	b := uint32(c.B * 255.0)
	a := uint32(c.A * 255.0)
	return (r << 24) | (g << 16) | (b << 8) | a
}

// Hsla converts to HSLA using the upstream algorithm.
func (c Rgba) Hsla() Hsla {
	max := c.R
	if c.G > max {
		max = c.G
	}
	if c.B > max {
		max = c.B
	}
	min := c.R
	if c.G < min {
		min = c.G
	}
	if c.B < min {
		min = c.B
	}
	delta := max - min
	l := (max + min) / 2.0
	var s float32
	if l != 0.0 && l != 1.0 {
		if l < 0.5 {
			s = delta / (2.0 * l)
		} else {
			s = delta / (2.0 - 2.0*l)
		}
	}
	var h float32
	if delta != 0.0 {
		if max == c.R {
			h = remEuclid6((c.G-c.B)/delta) / 6.0
		} else if max == c.G {
			h = ((c.B-c.R)/delta + 2.0) / 6.0
		} else {
			h = ((c.R-c.G)/delta + 4.0) / 6.0
		}
	}
	return Hsla{H: h, S: s, L: l, A: c.A}
}

// ParseRgba parses "#rgb", "#rgba", "#rrggbb" and "#rrggbbaa".
func ParseRgba(s string) (Rgba, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "#") {
		return Rgba{}, fmt.Errorf("invalid rgba: %q", s)
	}
	hex := s[1:]
	var r, g, b, a uint32 = 0, 0, 0, 0xff
	switch len(hex) {
	case 3:
		v, err := parseHexDigit(hex[0])
		if err != nil {
			return Rgba{}, err
		}
		r = v*16 + v
		v, err = parseHexDigit(hex[1])
		if err != nil {
			return Rgba{}, err
		}
		g = v*16 + v
		v, err = parseHexDigit(hex[2])
		if err != nil {
			return Rgba{}, err
		}
		b = v*16 + v
	case 4:
		v, err := parseHexDigit(hex[0])
		if err != nil {
			return Rgba{}, err
		}
		r = v*16 + v
		v, err = parseHexDigit(hex[1])
		if err != nil {
			return Rgba{}, err
		}
		g = v*16 + v
		v, err = parseHexDigit(hex[2])
		if err != nil {
			return Rgba{}, err
		}
		b = v*16 + v
		v, err = parseHexDigit(hex[3])
		if err != nil {
			return Rgba{}, err
		}
		a = v*16 + v
	case 6, 8:
		var vals [4]uint32
		n := len(hex) / 2
		for i := 0; i < n; i++ {
			v1, err := parseHexDigit(hex[i*2])
			if err != nil {
				return Rgba{}, err
			}
			v2, err := parseHexDigit(hex[i*2+1])
			if err != nil {
				return Rgba{}, err
			}
			vals[i] = v1*16 + v2
		}
		r, g, b = vals[0], vals[1], vals[2]
		if n == 4 {
			a = vals[3]
		}
	default:
		return Rgba{}, fmt.Errorf("invalid rgba: %q", s)
	}
	return Rgba{
		R: float32(r) / 255.0,
		G: float32(g) / 255.0,
		B: float32(b) / 255.0,
		A: float32(a) / 255.0,
	}, nil
}

// String formats as #rrggbbaa with rounded components.
func (c Rgba) String() string {
	return fmt.Sprintf("#%08x", c.ToU32())
}

func parseHexDigit(b byte) (uint32, error) {
	switch {
	case b >= '0' && b <= '9':
		return uint32(b - '0'), nil
	case b >= 'a' && b <= 'f':
		return uint32(b-'a') + 10, nil
	case b >= 'A' && b <= 'F':
		return uint32(b-'A') + 10, nil
	}
	return 0, fmt.Errorf("invalid hex digit %q", string(b))
}

// Hsla is a color in HSL space. H is in turns [0, 1] (not degrees or
// radians); S, L, A are in [0, 1].
type Hsla struct {
	H, S, L, A float32
}

// Black is opaque black.
func Black() Hsla { return Hsla{H: 0, S: 0, L: 0, A: 1} }

// White is opaque white.
func White() Hsla { return Hsla{H: 0, S: 0, L: 1, A: 1} }

// TransparentBlack is fully transparent black.
func TransparentBlack() Hsla { return Hsla{H: 0, S: 0, L: 0, A: 0} }

// TransparentWhite is fully transparent white.
func TransparentWhite() Hsla { return Hsla{H: 0, S: 0, L: 1, A: 0} }

// Red is opaque red.
func Red() Hsla { return Hsla{H: 0, S: 1, L: 0.5, A: 1} }

// Green is the upstream green (h=1/3, s=1, l=0.25).
func Green() Hsla { return Hsla{H: 1.0 / 3.0, S: 1, L: 0.25, A: 1} }

// Blue is opaque blue.
func Blue() Hsla { return Hsla{H: 0.6666666667, S: 1, L: 0.5, A: 1} }

// Yellow is opaque yellow.
func Yellow() Hsla { return Hsla{H: 1.0 / 6.0, S: 1, L: 0.5, A: 1} }

// Hsla constructs an Hsla with all channels clamped to [0, 1].
func HSLA(h, s, l, a float32) Hsla {
	return Hsla{H: clamp01(h), S: clamp01(s), L: clamp01(l), A: clamp01(a)}
}

// OpaqueGrey constructs a grey with the given lightness and opacity.
func OpaqueGrey(lightness, opacity float32) Hsla {
	return Hsla{H: 0, S: 0, L: clamp01(lightness), A: clamp01(opacity)}
}

// IsTransparent reports a == 0.
func (c Hsla) IsTransparent() bool { return c.A == 0 }

// IsOpaque reports a == 1.
func (c Hsla) IsOpaque() bool { return c.A == 1 }

// ToRgb converts to RGBA using the upstream algorithm (operating directly
// in sRGB space).
func (c Hsla) ToRgb() Rgba {
	// Match the exact upstream formula.
	h, s, l := c.H, c.S, c.L
	cc := (1.0 - abs32(2.0*l-1.0)) * s
	x := cc * (1.0 - abs32(modTwo(h*6.0)-1.0))
	m := l - cc/2.0
	cm := cc + m
	xm := x + m
	var r, g, b float32
	switch int(math.Floor(float64(h * 6.0))) {
	case 0, 6:
		r, g, b = cm, xm, m
	case 1:
		r, g, b = xm, cm, m
	case 2:
		r, g, b = m, cm, xm
	case 3:
		r, g, b = m, xm, cm
	case 4:
		r, g, b = xm, m, cm
	default:
		r, g, b = cm, m, xm
	}
	return Rgba{R: clamp01(r), G: clamp01(g), B: clamp01(b), A: c.A}
}

// Blend converts both colors to RGBA, composites other over this, and
// converts back.
func (c Hsla) Blend(other Hsla) Hsla {
	if other.A >= 1.0 {
		return other
	}
	if other.A <= 0.0 {
		return c
	}
	return c.ToRgb().Blend(other.ToRgb()).Hsla()
}

// Grayscale sets saturation to zero, keeping hue.
func (c Hsla) Grayscale() Hsla { c.S = 0; return c }

// FadeOut multiplies the alpha down by the factor (mutating style helper).
func (c *Hsla) FadeOut(factor float32) { c.A *= 1.0 - clamp01(factor) }

// Opacity multiplies the alpha by the factor.
func (c Hsla) Opacity(factor float32) Hsla {
	c.A = clamp01(c.A * clamp01(factor))
	return c
}

// Alpha replaces the alpha channel.
func (c Hsla) Alpha(a float32) Hsla { c.A = clamp01(a); return c }

// Rgba converts to RGBA.
func (c Hsla) Rgba() Rgba { return c.ToRgb() }

// String formats like the upstream Display impl.
func (c Hsla) String() string {
	return fmt.Sprintf("hsla(%.2f, %.2f%%, %.2f%%, %.2f)", c.H*360, c.S*100, c.L*100, c.A)
}

// Cmp compares HSLA channels with f32::total_cmp semantics.
func (c Hsla) Cmp(o Hsla) int {
	if v := totalCmp32(c.H, o.H); v != 0 {
		return v
	}
	if v := totalCmp32(c.S, o.S); v != 0 {
		return v
	}
	if v := totalCmp32(c.L, o.L); v != 0 {
		return v
	}
	return totalCmp32(c.A, o.A)
}

// Equal compares with total_cmp ordering like the upstream PartialEq.
func (c Hsla) Equal(o Hsla) bool { return c.Cmp(o) == 0 }

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func modOne(v float32) float32 {
	v = float32(math.Mod(float64(v), 1.0))
	if v < 0 {
		v += 1
	}
	return v
}

// remEuclid6 computes ((v % 6) + 6) % 6 for the red-max hue branch.
func remEuclid6(v float32) float32 {
	v = float32(math.Mod(float64(v), 6.0))
	if v < 0 {
		v += 6
	}
	return v
}

func totalCmp32(a, b float32) int {
	ia, ib := int32(math.Float32bits(a)), int32(math.Float32bits(b))
	ia ^= int32(uint32(ia>>31) >> 1)
	ib ^= int32(uint32(ib>>31) >> 1)
	switch {
	case ia < ib:
		return -1
	case ia > ib:
		return 1
	default:
		return 0
	}
}

// ---------------------------------------------------------------------------
// Backgrounds, gradients and patterns
// ---------------------------------------------------------------------------

// ColorSpace selects the gradient interpolation space.
type ColorSpace uint8

const (
	// ColorSpaceSrgb interpolates in (non-linear) sRGB.
	ColorSpaceSrgb ColorSpace = 0
	// ColorSpaceOklab interpolates in Oklab.
	ColorSpaceOklab ColorSpace = 1
)

// LinearColorStop is one stop of a linear gradient.
type LinearColorStop struct {
	Color      Hsla
	Percentage float32 // 0..1
}

// LinearColorStop constructs a stop.
func NewLinearColorStop(color Hsla, percentage float32) LinearColorStop {
	return LinearColorStop{Color: color, Percentage: clamp01(percentage)}
}

// Opacity scales the stop's alpha.
func (s LinearColorStop) Opacity(factor float32) LinearColorStop {
	s.Color = s.Color.Opacity(factor)
	return s
}

// BackgroundTag discriminates Background variants.
type BackgroundTag uint8

const (
	// BackgroundTagSolid is a solid fill.
	BackgroundTagSolid BackgroundTag = 0
	// BackgroundTagLinearGradient is a two-stop linear gradient.
	BackgroundTagLinearGradient BackgroundTag = 1
	// BackgroundTagPatternSlash is a 45-degree slash pattern.
	BackgroundTagPatternSlash BackgroundTag = 2
	// BackgroundTagCheckerboard is a checkerboard pattern.
	BackgroundTagCheckerboard BackgroundTag = 3
)

// Background is a fill: solid color, gradient or pattern. It mirrors the
// upstream repr(C) struct with a tag.
type Background struct {
	Tag                          BackgroundTag
	ColorSpace                   ColorSpace
	Solid                        Hsla
	GradientAngleOrPatternHeight float32
	Colors                       [2]LinearColorStop
}

// SolidBackground constructs a solid background.
func SolidBackground(color Hsla) Background {
	return Background{Tag: BackgroundTagSolid, Solid: color}
}

// LinearGradient constructs a gradient. Angle is in degrees, 0 = towards the
// top, increasing clockwise.
func LinearGradient(angle float32, from, to LinearColorStop) Background {
	return Background{
		Tag:                          BackgroundTagLinearGradient,
		GradientAngleOrPatternHeight: angle,
		Colors:                       [2]LinearColorStop{from, to},
	}
}

// PatternSlash constructs a diagonal stripe pattern.
func PatternSlash(color Hsla, width, interval float32) Background {
	return Background{
		Tag:                          BackgroundTagPatternSlash,
		Solid:                        color,
		GradientAngleOrPatternHeight: width*255.0*0xFFFF + interval*255.0,
	}
}

// PatternSlashParams returns the width and interval of a slash pattern.
func (b Background) PatternSlashParams() (width, interval float32) {
	packed := b.GradientAngleOrPatternHeight
	interval = float32(math.Mod(float64(packed), 255.0*0xFFFF)) / 255.0
	width = packed / (255.0 * 0xFFFF)
	return
}

// Checkerboard constructs a checkerboard pattern.
func Checkerboard(color Hsla, size float32) Background {
	return Background{
		Tag:                          BackgroundTagCheckerboard,
		Solid:                        color,
		GradientAngleOrPatternHeight: size,
	}
}

// AsSolid returns the solid color if the background is solid.
func (b Background) AsSolid() (Hsla, bool) {
	if b.Tag == BackgroundTagSolid {
		return b.Solid, true
	}
	return Hsla{}, false
}

// WithColorSpace sets the interpolation color space.
func (b Background) WithColorSpace(cs ColorSpace) Background {
	b.ColorSpace = cs
	return b
}

// Opacity scales the alpha of every color in the background.
func (b Background) Opacity(factor float32) Background {
	switch b.Tag {
	case BackgroundTagSolid:
		b.Solid = b.Solid.Opacity(factor)
	case BackgroundTagLinearGradient:
		b.Colors[0] = b.Colors[0].Opacity(factor)
		b.Colors[1] = b.Colors[1].Opacity(factor)
	case BackgroundTagPatternSlash, BackgroundTagCheckerboard:
		b.Solid = b.Solid.Opacity(factor)
	}
	return b
}

// IsTransparent reports whether the background paints nothing.
func (b Background) IsTransparent() bool {
	switch b.Tag {
	case BackgroundTagSolid:
		return b.Solid.IsTransparent()
	case BackgroundTagLinearGradient:
		return b.Colors[0].Color.IsTransparent() && b.Colors[1].Color.IsTransparent()
	case BackgroundTagPatternSlash, BackgroundTagCheckerboard:
		return b.Solid.IsTransparent()
	}
	return true
}

// ---------------------------------------------------------------------------
// Default palette (colors.rs)
// ---------------------------------------------------------------------------

// Colors is the default gpui component palette.
type Colors struct {
	Text         Rgba `json:"text"`
	SelectedText Rgba `json:"selected_text"`
	Background   Rgba `json:"background"`
	Disabled     Rgba `json:"disabled"`
	Selected     Rgba `json:"selected"`
	Border       Rgba `json:"border"`
	Separator    Rgba `json:"separator"`
	Container    Rgba `json:"container"`
}

// DarkColors returns the dark palette.
func DarkColors() Colors {
	return Colors{
		Text:         RGB(0xffffff),
		SelectedText: RGB(0xffffff),
		Background:   RGB(0x222222),
		Disabled:     RGB(0x565656),
		Selected:     RGB(0x2457ca),
		Border:       RGB(0x000000),
		Separator:    RGB(0xd9d9d9),
		Container:    RGB(0x262626),
	}
}

// LightColors returns the light palette (also the default).
func LightColors() Colors {
	return Colors{
		Text:         RGB(0x252525),
		SelectedText: RGB(0xffffff),
		Background:   RGB(0xffffff),
		Disabled:     RGB(0xb0b0b0),
		Selected:     RGB(0x2a63d9),
		Border:       RGB(0xd9d9d9),
		Separator:    RGB(0xe6e6e6),
		Container:    RGB(0xf4f5f5),
	}
}

// DefaultAppearance is a light/dark preference.
type DefaultAppearance int

const (
	// DefaultAppearanceLight is the light appearance (default).
	DefaultAppearanceLight DefaultAppearance = iota
	// DefaultAppearanceDark is the dark appearance.
	DefaultAppearanceDark
)

// WindowAppearance is the platform window appearance.
type WindowAppearance int

const (
	// WindowAppearanceLight is a light window.
	WindowAppearanceLight WindowAppearance = iota
	// WindowAppearanceVibrantLight is a light window with vibrancy.
	WindowAppearanceVibrantLight
	// WindowAppearanceDark is a dark window.
	WindowAppearanceDark
	// WindowAppearanceVibrantDark is a dark window with vibrancy.
	WindowAppearanceVibrantDark
)

// ColorsForAppearance maps a window appearance to a palette.
func ColorsForAppearance(appearance WindowAppearance) Colors {
	switch appearance {
	case WindowAppearanceLight, WindowAppearanceVibrantLight:
		return LightColors()
	default:
		return DarkColors()
	}
}

// ---------------------------------------------------------------------------
// WCAG contrast helpers (from crates/ui color_contrast.rs)
// ---------------------------------------------------------------------------

// RelativeLuminance computes the WCAG relative luminance of an sRGB color.
func RelativeLuminance(c Rgba) float64 {
	lin := func(v float32) float64 {
		if v <= 0.03928 {
			return float64(v) / 12.92
		}
		return math.Pow((float64(v)+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

// ContrastRatio computes the WCAG contrast ratio between two colors.
func ContrastRatio(a, b Rgba) float64 {
	la, lb := RelativeLuminance(a), RelativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// modTwo computes v mod 2 with a non-negative result (Rust's
// `.rem_euclid(2.0)` equivalent used by the HSL->RGB sector math).
func modTwo(v float32) float32 {
	v = float32(math.Mod(float64(v), 2.0))
	if v < 0 {
		v += 2
	}
	return v
}
