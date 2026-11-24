package geometry

import (
	"math"
	"testing"
)

func approx(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-5 }

func TestPixelsArithmetic(t *testing.T) {
	a, b := Px(10), Px(4)
	if a.Add(b).AsF32() != 14 {
		t.Fatal("add")
	}
	if a.Sub(b).AsF32() != 6 {
		t.Fatal("sub")
	}
	if a.MulScalar(2).AsF32() != 20 {
		t.Fatal("mul")
	}
	if a.DivScalar(4).AsF32() != 2.5 {
		t.Fatal("div")
	}
	if a.Quo(b) != 2.5 {
		t.Fatal("quo")
	}
	if a.Rem(b).AsF32() != 2 {
		t.Fatal("rem")
	}
	if a.Neg().AsF32() != -10 {
		t.Fatal("neg")
	}
	if Px(7.5).Floor().AsF32() != 7 || Px(7.4).Ceil().AsF32() != 8 || Px(7.5).Round().AsF32() != 8 {
		t.Fatal("floor/ceil/round")
	}
}

func TestPixelsToI32RustAsSemantics(t *testing.T) {
	if Px(float32(math.NaN())).ToI32() != 0 {
		t.Fatal("nan should map to 0")
	}
	if Px(3e38).ToI32() != math.MaxInt32 {
		t.Fatal("should saturate to MaxInt32")
	}
	if Px(-3e38).ToI32() != math.MinInt32 {
		t.Fatal("should saturate to MinInt32")
	}
	if Px(2.7).ToI32() != 2 || Px(-2.7).ToI32() != -2 {
		t.Fatal("should truncate toward zero")
	}
}

func TestScaledToDevicePixelsCeils(t *testing.T) {
	if ScaledPx(3.1).ToDevicePixels().AsI32() != 4 {
		t.Fatal("scaled -> device must ceil")
	}
	if ScaledPx(-3.1).ToDevicePixels().AsI32() != -3 {
		t.Fatal("ceil(-3.1) = -3")
	}
}

func TestSizeToDevicePixelsRounds(t *testing.T) {
	s := SizeToDevicePixels(SizePixels(10.4, 10.6), 2)
	if s.Width.AsI32() != 21 || s.Height.AsI32() != 21 {
		t.Fatalf("round: %v", s)
	}
	back := DeviceSizeToPixels(s, 2)
	if back.Width.AsF32() != 10.5 {
		t.Fatalf("back: %v", back)
	}
}

func TestBoundsContainsHalfOpen(t *testing.T) {
	b := BoundsPixels(0, 0, 10, 10)
	if !BoundsContains(b, PointPixels(0, 0)) {
		t.Fatal("origin contained")
	}
	if !BoundsContains(b, PointPixels(9.9, 9.9)) {
		t.Fatal("inside contained")
	}
	if BoundsContains(b, PointPixels(10, 5)) {
		t.Fatal("right edge excluded (half-open)")
	}
	if BoundsContains(b, PointPixels(5, 10)) {
		t.Fatal("bottom edge excluded (half-open)")
	}
}

func TestBoundsIntersectUnion(t *testing.T) {
	a := BoundsPixels(0, 0, 10, 10)
	b := BoundsPixels(5, 5, 10, 10)
	i := IntersectBounds(a, b)
	if i.Origin.X.AsF32() != 5 || i.Origin.Y.AsF32() != 5 ||
		i.Size.Width.AsF32() != 5 || i.Size.Height.AsF32() != 5 {
		t.Fatalf("intersect: %v", i)
	}
	c := BoundsPixels(20, 20, 5, 5)
	d := IntersectBounds(a, c)
	if d.Size.Width.AsF32() != 0 || d.Size.Height.AsF32() != 0 {
		t.Fatalf("degenerate: %v", d)
	}
	u := UnionBounds(a, b)
	if u.Origin.X.AsF32() != 0 || u.Size.Width.AsF32() != 15 || u.Size.Height.AsF32() != 15 {
		t.Fatalf("union: %v", u)
	}
	if !BoundsIntersects(a, b) {
		t.Fatal("should intersect")
	}
	e := BoundsPixels(10, 0, 5, 5)
	if BoundsIntersects(a, e) {
		t.Fatal("touching edges are not intersection")
	}
}

func TestBoundsIsContainedWithin(t *testing.T) {
	outer := BoundsPixels(0, 0, 10, 10)
	if BoundsIsContainedWithin(outer, outer) {
		t.Fatal("equal bounds are not strictly contained (half-open contains)")
	}
	inner := BoundsPixels(2, 2, 6, 6)
	if !BoundsIsContainedWithin(inner, outer) {
		t.Fatal("inner should be contained")
	}
}

func TestBoundsJoinEmptyIdentity(t *testing.T) {
	empty := BoundsPixels(0, 0, 0, 0)
	b := BoundsPixels(1, 1, 5, 5)
	if JoinBounds(empty, b) != b {
		t.Fatal("empty join b == b")
	}
	if JoinBounds(b, empty) != b {
		t.Fatal("b join empty == b")
	}
}

func TestBoundsDilateInsetExtend(t *testing.T) {
	b := BoundsPixels(10, 10, 10, 10)
	d := b.Dilate(Px(2))
	if d.Origin.X.AsF32() != 8 || d.Size.Width.AsF32() != 14 {
		t.Fatalf("dilate: %v", d)
	}
	i := d.Inset(Px(2))
	if i.Origin.X.AsF32() != 10 || i.Size.Width.AsF32() != 10 {
		t.Fatalf("inset: %v", i)
	}
	e := b.Extend(Edges[Pixels]{Top: Px(1), Right: Px(2), Bottom: Px(3), Left: Px(4)})
	if e.Origin.X.AsF32() != 6 || e.Origin.Y.AsF32() != 9 ||
		e.Size.Width.AsF32() != 16 || e.Size.Height.AsF32() != 14 {
		t.Fatalf("extend: %v", e)
	}
}

func TestLengthResolution(t *testing.T) {
	remSize := Px(16)
	if !approx(PixelsLength(Px(12)).ToPixels(remSize).AsF32(), 12) {
		t.Fatal("px length")
	}
	if !approx(RemsLength(Rems(1.5)).ToPixels(remSize).AsF32(), 24) {
		t.Fatal("rem length")
	}
	d := RelativeDefinite(0.5)
	if got := d.ToPixels(PixelsLength(Px(20)), remSize); !approx(got.AsF32(), 10) {
		t.Fatalf("fraction of px: %v", got)
	}
	if got := d.ToPixels(RemsLength(Rems(2)), remSize); !approx(got.AsF32(), 16) {
		t.Fatalf("fraction of rem: %v", got)
	}
	l := AutoLength()
	if _, ok := l.ToPixels(PixelsLength(Px(10)), remSize); ok {
		t.Fatal("auto has no pixels")
	}
	if !l.IsAuto() || l.IsZero() {
		t.Fatal("auto flags")
	}
	if !Phi().IsFraction() {
		t.Fatal("phi is fraction")
	}
}

func TestLengthParsing(t *testing.T) {
	if p, err := ParsePixels("12px"); err != nil || p.AsF32() != 12 {
		t.Fatal("parse px")
	}
	if r, err := ParseRems("1.5rem"); err != nil || r.AsF32() != 1.5 {
		t.Fatal("parse rem")
	}
	if d, err := ParseDefiniteLength("50%"); err != nil || !d.IsFraction() {
		t.Fatal("parse percent")
	}
	if l, err := ParseLength("auto"); err != nil || !l.IsAuto() {
		t.Fatal("parse auto")
	}
	if _, err := ParseLength("bogus"); err == nil {
		t.Fatal("should fail")
	}
}

func TestEdgesResolveFractionAxes(t *testing.T) {
	// top/bottom resolve against parent height; left/right against width.
	e := EdgesDefinite{
		Top: RelativeDefinite(0.5), Right: RelativeDefinite(0.25),
		Bottom: RelativeDefinite(0.5), Left: RelativeDefinite(0.25),
	}
	p := e.Resolve(PixelsLength(Px(100)), PixelsLength(Px(40)), Px(16))
	if p.Top.AsF32() != 20 || p.Bottom.AsF32() != 20 {
		t.Fatalf("vertical fraction: %v", p)
	}
	if p.Left.AsF32() != 25 || p.Right.AsF32() != 25 {
		t.Fatalf("horizontal fraction: %v", p)
	}
	ae := EdgesAbsolute{Top: PixelsLength(Px(1)), Right: RemsLength(Rems(1)),
		Bottom: PixelsLength(Px(3)), Left: PixelsLength(Px(4))}
	ap := ae.ToPixels(Px(16))
	if ap.Top.AsF32() != 1 || ap.Right.AsF32() != 16 || ap.Bottom.AsF32() != 3 {
		t.Fatalf("absolute edges: %v", ap)
	}
	if MaxEdge(ap).AsF32() != 16 {
		t.Fatal("max edge")
	}
}

func TestRoundingHelpers(t *testing.T) {
	cases := []struct{ in, out float32 }{
		{0.5, 0}, {1.5, 1}, {-0.5, 0}, {-1.5, -1}, {0.4, 0}, {0.6, 1}, {2.5, 2},
	}
	for _, c := range cases {
		if RoundHalfTowardZero(c.in) != c.out {
			t.Fatalf("RoundHalfTowardZero(%v) = %v, want %v", c.in, RoundHalfTowardZero(c.in), c.out)
		}
	}
	if RoundStrokeToDevicePixel(0, 2) != 0 {
		t.Fatal("zero stroke stays zero")
	}
	if RoundStrokeToDevicePixel(0.2, 2) != 0.5 {
		t.Fatal("non-zero stroke clamps to 1 device px (0.5 logical at 2x)")
	}
	if RoundToDevicePixel(100, 2) != 100 {
		t.Fatal("layout values must stay logical: 100@2x = 100")
	}
	if RoundToDevicePixel(10.3, 2) != 10.5 {
		t.Fatal("10.3@2x snaps to 10.5 logical (21 device px)")
	}
}

func TestTransformationMatrix(t *testing.T) {
	m := Rotate(RadiansFrom(math.Pi / 2))
	p := m.Apply(PointPixels(1, 0))
	if math.Abs(float64(p.X.AsF32())) > 1e-6 || math.Abs(float64(p.Y.AsF32())-1) > 1e-6 {
		t.Fatalf("rotate: %v", p)
	}
	tm := Translate(Point[ScaledPixels]{X: ScaledPx(10), Y: ScaledPx(0)})
	rm := Rotate(RadiansFrom(math.Pi / 2))
	res := rm.Compose(tm) // translate then rotate
	out := res.Apply(PointPixels(0, 0))
	if math.Abs(float64(out.X.AsF32())) > 1e-6 || math.Abs(float64(out.Y.AsF32())-10) > 1e-6 {
		t.Fatalf("compose order: %v", out)
	}
	b := Bounds[ScaledPixels]{
		Origin: Point[ScaledPixels]{X: ScaledPx(0), Y: ScaledPx(0)},
		Size:   Size[ScaledPixels]{Width: ScaledPx(2), Height: ScaledPx(1)},
	}
	tb := Rotate(RadiansFrom(math.Pi / 2)).TransformBounds(b)
	if tb.Size.Width.AsF32() < 0.99 || tb.Size.Height.AsF32() < 1.99 {
		t.Fatalf("envelope: %v", tb)
	}
}

func TestPointMagnitudeRelativeTo(t *testing.T) {
	p := PointPixels(3, 4)
	if p.Magnitude() != 5 {
		t.Fatalf("magnitude: %v", p.Magnitude())
	}
	o := PointPixels(1, 1)
	r := p.RelativeTo(o)
	if r.X.AsF32() != 2 || r.Y.AsF32() != 3 {
		t.Fatalf("relative: %v", r)
	}
}

func TestCornerClamp(t *testing.T) {
	c := ClampCornerRadii(AllCorners(Px(20)), SizePixels(10, 20))
	if c.TopLeft.AsF32() != 5 || c.BottomRight.AsF32() != 5 {
		t.Fatalf("clamp: %v", c)
	}
	if MaxCorner(c).AsF32() != 5 {
		t.Fatal("max corner")
	}
}

func TestAnchorHelpers(t *testing.T) {
	if AnchorTopLeft.Opposite() != AnchorBottomRight {
		t.Fatal("opposite")
	}
	if AnchorTopLeft.OtherSideAlong(AxisHorizontal) != AnchorBottomLeft {
		t.Fatal("other side horizontal")
	}
	if !AnchorTopCenter.IsCenter() || AnchorTopLeft.IsCenter() {
		t.Fatal("is center")
	}
	if !AnchorBottomLeft.IsBottom() || AnchorTopLeft.IsBottom() {
		t.Fatal("is bottom")
	}
	if AxisVertical.Invert() != AxisHorizontal {
		t.Fatal("invert axis")
	}
}

func TestFromAnchorAndSize(t *testing.T) {
	origin := PointPixels(100, 100)
	size := SizePixels(20, 10)
	b := FromAnchorAndSize(AnchorTopRight, origin, size)
	if b.Origin.X.AsF32() != 80 || b.Origin.Y.AsF32() != 100 {
		t.Fatalf("top right: %v", b)
	}
	b = FromAnchorAndSize(AnchorTopCenter, origin, size)
	if b.Origin.X.AsF32() != 90 || b.Origin.Y.AsF32() != 100 {
		t.Fatalf("top center: %v", b)
	}
	b = FromAnchorAndSize(AnchorLeftCenter, origin, size)
	if b.Origin.X.AsF32() != 100 || b.Origin.Y.AsF32() != 95 {
		t.Fatalf("left center: %v", b)
	}
}

func TestCmpTotalOrder(t *testing.T) {
	negZero := Px(math.Float32frombits(0x80000000))
	if negZero.Cmp(Px(0.0)) != -1 {
		t.Fatal("total cmp -0 < 0")
	}
	if Px(1).Cmp(Px(2)) != -1 || Px(2).Cmp(Px(1)) != 1 {
		t.Fatal("basic cmp")
	}
	if ScaledPx(1).Cmp(ScaledPx(2)) != -1 {
		t.Fatal("scaled cmp")
	}
}

func TestFromCornersCenteredAt(t *testing.T) {
	b := FromCorners(PointPixels(10, 10), PointPixels(20, 30))
	if b.Origin.X.AsF32() != 10 || b.Size.Width.AsF32() != 10 || b.Size.Height.AsF32() != 20 {
		t.Fatalf("from corners: %v", b)
	}
	c := CenteredAt(PointPixels(50, 50), SizePixels(10, 20))
	if c.Origin.X.AsF32() != 45 || c.Origin.Y.AsF32() != 40 {
		t.Fatalf("centered at: %v", c)
	}
}

func TestSpaceWithin(t *testing.T) {
	outer := BoundsPixels(0, 0, 100, 100)
	inner := BoundsPixels(10, 10, 80, 80)
	gap := inner.SpaceWithin(outer)
	if gap.Top.AsF32() != 10 || gap.Right.AsF32() != 10 || gap.Bottom.AsF32() != 10 || gap.Left.AsF32() != 10 {
		t.Fatalf("space within: %+v", gap)
	}
}

func TestLocalize(t *testing.T) {
	b := BoundsPixels(10, 10, 10, 10)
	if p, ok := LocalizePoint(b, PointPixels(15, 12)); !ok || p.X.AsF32() != 5 || p.Y.AsF32() != 2 {
		t.Fatalf("localize: %v %v", p, ok)
	}
	if _, ok := LocalizePoint(b, PointPixels(50, 50)); ok {
		t.Fatal("outside not localized")
	}
}

func TestBoundsConversion(t *testing.T) {
	b := BoundsPixels(10.25, 10.5, 100, 50)
	db := BoundsToDevicePixels(b, 2)
	if db.Origin.X.AsI32() != 21 || db.Origin.Y.AsI32() != 21 || db.Size.Width.AsI32() != 200 {
		t.Fatalf("to device: %v", db)
	}
	lb := DeviceBoundsToPixels(db, 2)
	if lb.Origin.X.AsF32() != 10.5 || lb.Size.Width.AsF32() != 100 {
		t.Fatalf("to logical: %v", lb)
	}
	if !DeviceBoundsContains(db, PointDevice(30, 30)) {
		t.Fatal("device contains")
	}
	sb := ScaleBounds(b, 2)
	if !ScaledBoundsContains(sb, Point[ScaledPixels]{X: ScaledPx(30), Y: ScaledPx(30)}) {
		t.Fatal("scaled contains")
	}
}

func TestSizeMaxMin(t *testing.T) {
	a := SizePixels(10, 20)
	b := SizePixels(20, 10)
	if SizePixelsMax(a, b).Width.AsF32() != 20 || SizePixelsMax(a, b).Height.AsF32() != 20 {
		t.Fatal("size max")
	}
	if SizePixelsMin(a, b).Width.AsF32() != 10 || SizePixelsMin(a, b).Height.AsF32() != 10 {
		t.Fatal("size min")
	}
	if a.Center().X.AsF32() != 5 || a.Center().Y.AsF32() != 10 {
		t.Fatal("size center")
	}
}

func TestHalfAndPerimeter(t *testing.T) {
	b := BoundsPixels(0, 0, 10, 20)
	if b.HalfPerimeter().AsF32() != 30 {
		t.Fatal("half perimeter")
	}
	if DevicePx(5).Half().AsI32() != 2 {
		t.Fatal("device half floors")
	}
	if (Rems(3)).Half().AsF32() != 1.5 {
		t.Fatal("rems half")
	}
}
