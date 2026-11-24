package style

import (
	"testing"

	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/font"
	"github.com/FlameInTheDark/go-gpui/geometry"
)

func toPx(t *testing.T, l geometry.Length, base geometry.AbsoluteLength, rem geometry.Pixels) float32 {
	t.Helper()
	p, ok := l.ToPixels(base, rem)
	if !ok {
		t.Fatalf("length %v is auto", l)
	}
	return p.AsF32()
}

func toPxD(t *testing.T, d geometry.DefiniteLength, base geometry.AbsoluteLength, rem geometry.Pixels) float32 {
	t.Helper()
	return d.ToPixels(base, rem).AsF32()
}

func toPxA(t *testing.T, a geometry.AbsoluteLength, rem geometry.Pixels) float32 {
	t.Helper()
	return a.ToPixels(rem).AsF32()
}

func approxPx(t *testing.T, got, want float32, msg string) {
	t.Helper()
	d := got - want
	if d < -0.001 || d > 0.001 {
		t.Fatalf("%s: got %v want %v", msg, got, want)
	}
}

func TestDefaultStyle(t *testing.T) {
	s := DefaultStyle()
	if s.Display != DisplayFlex {
		t.Fatal("default display is flex")
	}
	if s.Position != PositionRelative {
		t.Fatal("default position is relative (unlike CSS)")
	}
	if s.FlexDirection != FlexDirectionRow {
		t.Fatal("default direction is row")
	}
	if s.FlexShrink != 1 {
		t.Fatal("default flex shrink is 1 (unlike CSS)")
	}
	if s.FlexBasis.IsAuto() != true {
		t.Fatal("default flex basis is auto")
	}
	if s.Size.Width.IsAuto() != true || s.MinSize.Height.IsAuto() != true {
		t.Fatal("default sizes are auto")
	}
	if s.Overflow[0] != OverflowVisible || s.Overflow[1] != OverflowVisible {
		t.Fatal("default overflow visible")
	}
	if s.Text.FontFamily != ".SystemUIFont" {
		t.Fatal("default font family")
	}
	if s.Text.LineHeight.IsFraction() != true {
		t.Fatal("default line height is phi fraction")
	}
	if s.BorderColor != nil || s.Background != nil {
		t.Fatal("no default background/border")
	}
}

func TestRefinementCascade(t *testing.T) {
	// base sets width + bg
	base := NewStyleRefinement().W_8().BgColor(gc.Red())
	// hover sets bg + height
	hover := NewStyleRefinement().H_4().BgColor(gc.Blue())

	s := DefaultStyle()
	s.Refine(base)
	approxPx(t, toPx(t, s.Size.Width, geometry.PixelsLength(geometry.Px(100)), geometry.Px(16)), 32, "w_8 = 2rem = 32px")
	if s.Background.Solid != (gc.Hsla{H: 0, S: 1, L: 0.5, A: 1}) {
		t.Fatalf("bg red: %+v", s.Background.Solid)
	}
	// apply hover on top
	s.Refine(hover)
	approxPx(t, toPx(t, s.Size.Height, geometry.PixelsLength(geometry.Px(100)), geometry.Px(16)), 16, "h_4 = 1rem")
	// width survives, bg overridden
	approxPx(t, toPx(t, s.Size.Width, geometry.PixelsLength(geometry.Px(100)), geometry.Px(16)), 32, "width survives")
	if !s.Background.Solid.Equal(gc.Blue()) {
		t.Fatalf("bg overridden to blue: %+v", s.Background.Solid)
	}
}

func TestRefineOrderMatters(t *testing.T) {
	a := NewStyleRefinement().P_2()
	b := NewStyleRefinement().P_4()
	s := DefaultStyle()
	s.Refine(a)
	s.Refine(b)
	approxPx(t, toPxD(t, s.Padding.Top, geometry.PixelsLength(geometry.Px(10)), geometry.Px(16)), 16, "later refinement wins")
}

func TestMergeRefinements(t *testing.T) {
	a := NewStyleRefinement().W_8()
	b := NewStyleRefinement().H_4().W_4()
	m := a.Merge(b)
	s := DefaultStyle()
	s.Refine(m)
	rem := geometry.Px(16)
	approxPx(t, toPx(t, s.Size.Width, geometry.PixelsLength(geometry.Px(1)), rem), 16, "merge: width from b")
	approxPx(t, toPx(t, s.Size.Height, geometry.PixelsLength(geometry.Px(1)), rem), 16, "merge: height from b")
}

func TestScaleValues(t *testing.T) {
	rem := geometry.Px(16)
	cases := []struct {
		name string
		fn   func(*StyleRefinement) *StyleRefinement
		want float32
	}{
		{"w_4", func(r *StyleRefinement) *StyleRefinement { return r.W_4() }, 16},
		{"w_0p5", func(r *StyleRefinement) *StyleRefinement { return r.W_0p5() }, 2},
		{"w_1p5", func(r *StyleRefinement) *StyleRefinement { return r.W_1p5() }, 6},
		{"w_px", func(r *StyleRefinement) *StyleRefinement { return r.W_Px() }, 1},
		{"p_2", func(r *StyleRefinement) *StyleRefinement { return r.P_2() }, 8},
		{"px_4", func(r *StyleRefinement) *StyleRefinement { return r.Px_4() }, 16},
		{"py_1", func(r *StyleRefinement) *StyleRefinement { return r.Py_1() }, 4},
		{"m_neg_1", func(r *StyleRefinement) *StyleRefinement { return r.M_Neg_1() }, -4},
		{"gap_2", func(r *StyleRefinement) *StyleRefinement { return r.Gap_2() }, 8},
		{"mt_12", func(r *StyleRefinement) *StyleRefinement { return r.Mt_12() }, 48},
		{"min_w_96", func(r *StyleRefinement) *StyleRefinement { return r.MinW_96() }, 384},
	}
	for _, c := range cases {
		r := NewStyleRefinement()
		c.fn(r)
		s := DefaultStyle()
		s.Refine(r)
		var got float32
		switch c.name {
		case "p_2":
			got = toPxD(t, s.Padding.Top, geometry.PixelsLength(geometry.Px(1)), rem)
		case "px_4":
			got = toPxD(t, s.Padding.Left, geometry.PixelsLength(geometry.Px(1)), rem)
		case "py_1":
			got = toPxD(t, s.Padding.Top, geometry.PixelsLength(geometry.Px(1)), rem)
		case "m_neg_1":
			got = toPx(t, s.Margin.Top, geometry.PixelsLength(geometry.Px(1)), rem)
		case "gap_2":
			got = toPxD(t, s.Gap.Width, geometry.PixelsLength(geometry.Px(1)), rem)
		case "mt_12":
			got = toPx(t, s.Margin.Top, geometry.PixelsLength(geometry.Px(1)), rem)
		case "min_w_96":
			got = toPx(t, s.MinSize.Width, geometry.PixelsLength(geometry.Px(1)), rem)
		default:
			got = toPx(t, s.Size.Width, geometry.PixelsLength(geometry.Px(1)), rem)
		}
		approxPx(t, got, c.want, c.name)
	}
}

func TestFractionScale(t *testing.T) {
	r := NewStyleRefinement().W_1_2()
	s := DefaultStyle()
	s.Refine(r)
	p, ok := s.Size.Width.ToPixels(geometry.PixelsLength(geometry.Px(100)), geometry.Px(16))
	if !ok {
		t.Fatal("w_1_2 is definite")
	}
	approxPx(t, p.AsF32(), 50, "w_1_2 = 50% of 100px")

	r = NewStyleRefinement().H_Full()
	s = DefaultStyle()
	s.Refine(r)
	p, _ = s.Size.Height.ToPixels(geometry.PixelsLength(geometry.Px(200)), geometry.Px(16))
	approxPx(t, p.AsF32(), 200, "h_full = 100%")
}

func TestFlexSugar(t *testing.T) {
	r := NewStyleRefinement().Flex1()
	s := DefaultStyle()
	s.Refine(r)
	if s.FlexGrow != 1 || s.FlexShrink != 1 {
		t.Fatal("flex_1")
	}
	if d, _ := s.FlexBasis.Definite(); !d.IsFraction() {
		t.Fatal("flex_1 basis is 0%")
	}

	r = NewStyleRefinement().FlexNone()
	s = DefaultStyle()
	s.Refine(r)
	if s.FlexGrow != 0 || s.FlexShrink != 0 {
		t.Fatal("flex_none")
	}
}

func TestTruncate(t *testing.T) {
	r := NewStyleRefinement().Truncate()
	s := DefaultStyle()
	s.Refine(r)
	if s.Overflow[0] != OverflowHidden || s.Overflow[1] != OverflowHidden {
		t.Fatal("truncate sets overflow hidden")
	}
	if s.Text.WhiteSpace != WhiteSpaceNowrap {
		t.Fatal("truncate sets nowrap")
	}
	if s.Text.TextOverflow == nil || s.Text.TextOverflow.Kind != TextOverflowTruncate {
		t.Fatal("truncate sets ellipsis")
	}
}

func TestTypographyRefinement(t *testing.T) {
	r := NewStyleRefinement().TextXs().FontWeight(font.WeightSemibold).Italic().TextColor(gc.White())
	s := DefaultStyle()
	s.Refine(r)
	approxPx(t, toPxA(t, s.Text.FontSize, geometry.Px(16)), 12, "text_xs = 0.75rem")
	if s.Text.FontWeight != font.WeightSemibold {
		t.Fatal("font weight")
	}
	if s.Text.FontStyle != font.StyleItalic {
		t.Fatal("italic")
	}
	if !s.Text.Color.Equal(gc.White()) {
		t.Fatal("text color")
	}
}

func TestBorderAndRadius(t *testing.T) {
	r := NewStyleRefinement().Border_1().SetBorderColor(gc.Black()).Rounded_Md()
	s := DefaultStyle()
	s.Refine(r)
	approxPx(t, toPxA(t, s.BorderWidths.Top, geometry.Px(16)), 1, "border_1 is 1px not 1rem")
	approxPx(t, toPxA(t, s.CornerRadii.TopLeft, geometry.Px(16)), 6, "rounded_md = 0.375rem")
	if !s.IsBorderVisible(geometry.Px(16)) {
		t.Fatal("border should be visible")
	}
}

func TestShadows(t *testing.T) {
	r := NewStyleRefinement().ShadowMd()
	s := DefaultStyle()
	s.Refine(r)
	if len(s.BoxShadow) != 2 {
		t.Fatalf("shadow_md has 2 layers: %d", len(s.BoxShadow))
	}
	approxPx(t, s.BoxShadow[0].BlurRadius, 6, "shadow_md first blur")
	approxPx(t, s.BoxShadow[1].SpreadRadius, -2, "shadow_md second spread")
}

func TestOverflowMaskLogic(t *testing.T) {
	// OverflowMask is exercised through the window; here we verify the
	// overflow refinement axis handling.
	r := NewStyleRefinement().OverflowYScroll()
	s := DefaultStyle()
	s.Refine(r)
	if s.Overflow[1] != OverflowScroll {
		t.Fatal("overflow_y_scroll")
	}
	if s.Overflow[0] != OverflowVisible {
		t.Fatal("x untouched")
	}
}

func TestNestedRefinementFields(t *testing.T) {
	// Padding refinement only touches the set edges.
	r := NewStyleRefinement()
	r.Pt(geometry.PixelsDefinite(geometry.Px(10)))
	s := DefaultStyle()
	s.Refine(r)
	approxPx(t, toPxD(t, s.Padding.Top, geometry.PixelsLength(geometry.Px(1)), geometry.Px(16)), 10, "pt(10px)")
	approxPx(t, toPxD(t, s.Padding.Bottom, geometry.PixelsLength(geometry.Px(1)), geometry.Px(16)), 0, "bottom default 0")
}

func TestHasOpaqueBackground(t *testing.T) {
	s := DefaultStyle()
	if s.HasOpaqueBackground() {
		t.Fatal("no background by default")
	}
	s.Refine(NewStyleRefinement().BgColor(gc.Red()))
	if !s.HasOpaqueBackground() {
		t.Fatal("opaque red background")
	}
	s2 := DefaultStyle()
	s2.Refine(NewStyleRefinement().BgColor(gc.HSLA(0, 1, 0.5, 0.5)))
	if s2.HasOpaqueBackground() {
		t.Fatal("translucent is not opaque")
	}
}

func TestGridRefinement(t *testing.T) {
	r := NewStyleRefinement().SetGridCols(3)
	s := DefaultStyle()
	s.Refine(r)
	if s.GridCols == nil || s.GridCols.Repeat != 3 {
		t.Fatal("grid cols")
	}
}

func TestTextStyleHighlight(t *testing.T) {
	ts := DefaultTextStyle()
	hl := HighlightStyle{Color: &gc.Hsla{H: 0, S: 1, L: 0.5, A: 1}}
	out := ts.Highlight(hl)
	if !out.Color.Equal(gc.Hsla{H: 0, S: 1, L: 0.5, A: 1}) {
		t.Fatalf("highlight color: %+v", out.Color)
	}
	// fade out multiplies alpha
	fade := float32(0.5)
	out = ts.Highlight(HighlightStyle{FadeOut: &fade})
	if out.Color.A > 0.51 {
		t.Fatal("fade out")
	}
}
