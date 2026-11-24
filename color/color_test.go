package color

import (
	"testing"
)

func TestRGBConstructors(t *testing.T) {
	c := RGB(0xff0000)
	if c.R != 1 || c.G != 0 || c.B != 0 || c.A != 1 {
		t.Fatalf("rgb red: %+v", c)
	}
	c = RGBA(0xff0000ff)
	if c.R != 1 || c.A != 1 {
		t.Fatalf("rgba red: %+v", c)
	}
	c = RGBA(0xff000080)
	if c.A < 0.5 || c.A > 0.503 {
		t.Fatalf("rgba alpha: %+v", c)
	}
}

func TestHslaToRgb(t *testing.T) {
	// red
	c := Red().ToRgb()
	if abs32(c.R-1) > 1e-6 || c.G != 0 || c.B != 0 {
		t.Fatalf("red: %+v", c)
	}
	// white / black / gray keep s=0
	if Black().ToRgb().L() > 0 {
		t.Fatal("black")
	}
	if White().ToRgb().L() < 1 {
		t.Fatal("white")
	}
	// 120deg green-ish hue -> g max
	c = HSLA(1.0/3.0, 1, 0.5, 1).ToRgb()
	if !(c.G >= c.R && c.G >= c.B) {
		t.Fatalf("green max: %+v", c)
	}
	// round trip stability
	for _, h := range []float32{0, 0.1, 0.25, 0.5, 0.75, 0.9} {
		for _, s := range []float32{0, 0.3, 1} {
			for _, l := range []float32{0.1, 0.5, 0.9} {
				orig := HSLA(h, s, l, 1)
				rt := orig.ToRgb().Hsla()
				if abs32(rt.H-orig.H) > 0.02 && !(s == 0 || abs32(s) < 0.02) {
					t.Fatalf("round trip hue drift: %v -> %v", orig, rt)
				}
				if abs32(rt.L-orig.L) > 0.005 {
					t.Fatalf("round trip lightness drift: %v -> %v", orig, rt)
				}
			}
		}
	}
}

// L is a helper for tests: luminance-ish max channel.
func (c Rgba) L() float32 { return maxf(c.R, maxf(c.G, c.B)) }

func maxf(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func TestRgbaToHsla(t *testing.T) {
	// gray has hue 0
	h := RGB(0x808080).Hsla()
	if h.H != 0 || h.S != 0 {
		t.Fatalf("gray hsla: %+v", h)
	}
	// red: h=0, s=1, l=0.5
	h = RGB(0xff0000).Hsla()
	if abs32(h.H) > 1e-6 || abs32(h.S-1) > 1e-6 || abs32(h.L-0.5) > 1e-6 {
		t.Fatalf("red hsla: %+v", h)
	}
	// max==g branch produces h around 1/3
	h = RGB(0x00ff00).Hsla()
	if abs32(h.H-1.0/3.0) > 1e-5 {
		t.Fatalf("green hsla: %+v", h)
	}
	// max==b branch produces h around 2/3
	h = RGB(0x0000ff).Hsla()
	if abs32(h.H-2.0/3.0) > 1e-5 {
		t.Fatalf("blue hsla: %+v", h)
	}
	// negative hue branch wraps via rem_euclid: (g-b)/delta = -1 -> 5/6
	h = RGB(0xff00ff).Hsla()
	if abs32(h.H-5.0/6.0) > 1e-5 {
		t.Fatalf("magenta hsla: %+v (want h=5/6)", h)
	}
}

func TestBlend(t *testing.T) {
	// opaque over anything = opaque
	if (Black().ToRgb().Blend(White().ToRgb())) != White().ToRgb() {
		t.Fatal("opaque over")
	}
	// transparent over = background
	if Black().ToRgb().Blend(TransparentWhite().ToRgb()) != Black().ToRgb() {
		t.Fatal("transparent over")
	}
	// half-over
	got := Black().ToRgb().Blend(RGBA(0xffffff80))
	if got.R < 0.49 || got.R > 0.51 || got.G != got.R || got.B != got.R {
		t.Fatalf("half blend: %+v", got)
	}
	// result keeps background alpha
	if got.A != 1 {
		t.Fatalf("blend keeps background alpha: %+v", got.A)
	}
	// hsla blend
	hb := Black().Blend(HSLA(0, 0, 1, 0.5))
	if hb.L < 0.49 || hb.L > 0.51 {
		t.Fatalf("hsla blend: %+v", hb)
	}
}

func TestParseRgba(t *testing.T) {
	c, err := ParseRgba("#f00")
	if err != nil || c.R != 1 {
		t.Fatalf("3-digit: %+v %v", c, err)
	}
	c, err = ParseRgba("#ff0000")
	if err != nil || c.R != 1 || c.A != 1 {
		t.Fatalf("6-digit: %+v %v", c, err)
	}
	c, err = ParseRgba("#ff000080")
	if err != nil || abs32(c.A-0.503) > 0.01 {
		t.Fatalf("8-digit: %+v %v", c, err)
	}
	c, err = ParseRgba("#ff00")
	if err != nil || c.R != 1 || c.B != 0 || c.A != 0 {
		t.Fatalf("4-digit: %+v %v", c, err)
	}
	if _, err := ParseRgba("ff0000"); err == nil {
		t.Fatal("missing # should fail")
	}
	if _, err := ParseRgba("#ff00000"); err == nil {
		t.Fatal("bad length should fail")
	}
}

func TestRgbaU32(t *testing.T) {
	c := RGBA(0xff0000ff)
	if c.ToU32() != 0xff0000ff {
		t.Fatalf("u32: %08x", c.ToU32())
	}
}

func TestOpacityAlpha(t *testing.T) {
	c := White().Opacity(0.5)
	if abs32(c.A-0.5) > 1e-6 {
		t.Fatal("opacity")
	}
	c = White().Alpha(0.25)
	if abs32(c.A-0.25) > 1e-6 {
		t.Fatal("alpha")
	}
	// clamping
	if White().Opacity(2).A != 1 || White().Alpha(-1).A != 0 {
		t.Fatal("clamp")
	}
	h := White().Opacity(0.5)
	if abs32(h.A-0.5) > 1e-6 {
		t.Fatal("hsla opacity")
	}
	var hp Hsla = White()
	hp.FadeOut(0.25)
	if abs32(hp.A-0.75) > 1e-6 {
		t.Fatal("fade out")
	}
}

func TestBackgrounds(t *testing.T) {
	b := SolidBackground(Red())
	c, ok := b.AsSolid()
	if !ok || !c.Equal(Red()) {
		t.Fatal("solid")
	}
	if b.IsTransparent() {
		t.Fatal("solid red not transparent")
	}
	g := LinearGradient(45, NewLinearColorStop(Black(), 0), NewLinearColorStop(White(), 1))
	if g.Tag != BackgroundTagLinearGradient {
		t.Fatal("gradient tag")
	}
	g = g.WithColorSpace(ColorSpaceOklab)
	if g.ColorSpace != ColorSpaceOklab {
		t.Fatal("color space")
	}
	// opacity multiplies stops
	g2 := g.Opacity(0.5)
	if abs32(g2.Colors[0].Color.A-0.5) > 1e-6 {
		t.Fatal("gradient opacity")
	}
	p := PatternSlash(Red(), 2, 8)
	w, i := p.PatternSlashParams()
	if abs32(w-2) > 0.01 || abs32(i-8) > 0.01 {
		t.Fatalf("pattern params: %v %v", w, i)
	}
	if Checkerboard(Red(), 4).Tag != BackgroundTagCheckerboard {
		t.Fatal("checkerboard")
	}
	if !(SolidBackground(TransparentBlack()).IsTransparent()) {
		t.Fatal("transparent solid")
	}
}

func TestPalettes(t *testing.T) {
	d, l := DarkColors(), LightColors()
	if d.Background != RGB(0x222222) || l.Background != RGB(0xffffff) {
		t.Fatal("palette values")
	}
	if ColorsForAppearance(WindowAppearanceDark) != (Colors{}) {
		_ = d // presence check only
	}
	if ColorsForAppearance(WindowAppearanceLight).Background != RGB(0xffffff) {
		t.Fatal("appearance mapping")
	}
	if ColorsForAppearance(WindowAppearanceVibrantDark).Background != RGB(0x222222) {
		t.Fatal("vibrant dark mapping")
	}
}

func TestContrast(t *testing.T) {
	ratio := ContrastRatio(RGB(0xffffff), RGB(0x000000))
	if ratio < 20.9 || ratio > 21.1 {
		t.Fatalf("black/white contrast: %v", ratio)
	}
}
