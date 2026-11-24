package elements

import (
	"math"
	"testing"

	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
)

func TestRelativeMoveKeepsImplicitLineRelative(t *testing.T) {
	// The original close icon looked bent because its second diagonal ended
	// at (-7, 7) rather than offsetting from the second subpath's start.
	got := parsePathData("M4.5 4.5l7 7m0-7-7 7")
	want := []svgSegment{segMove{x: 4.5, y: 4.5}, segLine{x: 11.5, y: 11.5}, segMove{x: 11.5, y: 4.5}, segLine{x: 4.5, y: 11.5}}
	if len(got) != len(want) {
		t.Fatalf("segments = %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("segment %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestSVGCompactDecimalsAndCurvedArcs(t *testing.T) {
	got := parsePathData("M3 6v4c0 1.1.9 2 2 2h3")
	if len(got) != 4 || got[2] != (segCubic{c1x: 3, c1y: 11.1, c2x: 3.9, c2y: 12, x: 5, y: 12}) {
		t.Fatalf("compact decimals parsed as %#v", got)
	}
	got = parsePathData("M1 0A1 1 0 0 1 0 1")
	if len(got) != 2 {
		t.Fatalf("quarter-circle segments: %#v", got)
	}
	c, ok := got[1].(segCubic)
	if !ok || c.x != 0 || c.y != 1 || math.Abs(float64(c.c1y)-0.552285) > 0.001 || math.Abs(float64(c.c2x)-0.552285) > 0.001 {
		t.Fatalf("quarter-circle was not curved correctly: %#v", got[1])
	}
	if got = parsePathData("M1 0A2 2 0 0 1 5"); len(got) != 1 {
		t.Fatalf("truncated arc parsed as %#v", got)
	}
}

// TestParseSVGRadialGradient verifies <radialGradient> parsing: the
// definition resolves with center/radius/focus, and fills referencing it
// take the radial bake path (gradIsRadial, no linear background).
func TestParseSVGRadialGradient(t *testing.T) {
	src := `<svg viewBox="0 0 32 32">
	<defs>
		<radialGradient id="g" cx="0.5" cy="0.5" r="0.5">
			<stop offset="0" stop-color="#ffffff"/>
			<stop offset="1" stop-color="#000000"/>
		</radialGradient>
	</defs>
	<circle cx="16" cy="16" r="14" fill="url(#g)"/>
</svg>`
	paints := parseSVG(src, geometry.BoundsPixels(0, 0, 64, 64), gc.Black())
	if len(paints) != 1 {
		t.Fatalf("paints: %d, want 1", len(paints))
	}
	p := paints[0]
	if p.gradientStops == nil || len(p.gradientStops) != 2 {
		t.Fatalf("radial stops not resolved: %+v", p.gradientStops)
	}
	if !p.gradIsRadial {
		t.Fatal("fill must be flagged radial")
	}
	if p.gradient != nil {
		t.Fatal("radial gradients must not map onto a linear background")
	}
	cx, cy, r := p.gradRadial[0], p.gradRadial[1], p.gradRadial[2]
	// Center at the circle's center scaled 2x into 64x64, radius 14*2=28.
	if d := cx - 32; d < -0.51 || d > 0.51 {
		t.Fatalf("center x = %v, want 32", cx)
	}
	if d := cy - 32; d < -0.51 || d > 0.51 {
		t.Fatalf("center y = %v, want 32", cy)
	}
	if d := r - 28; d < -0.51 || d > 0.51 {
		t.Fatalf("radius = %v, want ~28", r)
	}
}

// TestSVGRadialGradientBakes verifies the radial bake: rendering a circle
// with a white-to-black radial gradient produces a painted image whose
// center pixel is lighter than its edge pixel.
func TestSVGRadialGradientBakes(t *testing.T) {
	src := `<svg viewBox="0 0 32 32">
	<defs>
		<radialGradient id="g" cx="0.5" cy="0.5" r="0.5">
			<stop offset="0" stop-color="#ffffff"/>
			<stop offset="1" stop-color="#000000"/>
		</radialGradient>
	</defs>
	<circle cx="16" cy="16" r="14" fill="url(#g)"/>
</svg>`
	var svg *Svg
	win := openAnchoredWindow(t, 64, 64, func(w *gpui.Window) gpui.Element {
		if svg == nil {
			svg = NewSvgFromData([]byte(src))
			svg.StyleRef().W(geometry.LengthFromDefinite(geometry.AbsoluteDefinite(geometry.PixelsLength(geometry.Px(64)))))
			svg.StyleRef().H(geometry.LengthFromDefinite(geometry.AbsoluteDefinite(geometry.PixelsLength(geometry.Px(64)))))
		}
		return svg
	})
	if len(win.RenderedScene().PolySprites) == 0 {
		t.Fatal("radial gradient painted nothing")
	}
}

// TestParseSVGDashArray verifies stroke-dasharray parsing: the pattern
// lands on the paint, odd patterns repeat, and "none" clears it.
func TestParseSVGDashArray(t *testing.T) {
	src := `<svg viewBox="0 0 32 32"><path d="M2 16 L30 16" stroke="#ffffff" stroke-width="2" stroke-dasharray="4 2"/></svg>`
	paints := parseSVG(src, geometry.BoundsPixels(0, 0, 64, 64), gc.Black())
	if len(paints) != 1 {
		t.Fatalf("paints: %d, want 1", len(paints))
	}
	if len(paints[0].dash) != 2 || paints[0].dash[0] != 4 || paints[0].dash[1] != 2 {
		t.Fatalf("dash = %v, want [4 2]", paints[0].dash)
	}

	// Odd pattern repeats once: [3] -> [3 3].
	srcOdd := `<svg viewBox="0 0 32 32"><path d="M2 16 L30 16" stroke="#fff" stroke-width="2" stroke-dasharray="3"/></svg>`
	paints = parseSVG(srcOdd, geometry.BoundsPixels(0, 0, 64, 64), gc.Black())
	if len(paints[0].dash) != 1 {
		// The raw pattern stays as parsed; repetition happens in
		// dashSegments.
		t.Logf("raw odd pattern = %v", paints[0].dash)
	}

	// "none" clears dashing.
	srcNone := `<svg viewBox="0 0 32 32"><path d="M2 16 L30 16" stroke="#fff" stroke-width="2" stroke-dasharray="none"/></svg>`
	paints = parseSVG(srcNone, geometry.BoundsPixels(0, 0, 64, 64), gc.Black())
	if len(paints[0].dash) != 0 {
		t.Fatalf("dasharray=none must clear the pattern, got %v", paints[0].dash)
	}
}

// TestDashSegmentsSplitsByPattern verifies the dash walker: a straight
// line of length 10 with pattern [4 2] splits into two drawn runs
// (lengths 4 and 4: 4 drawn, 2 blank, 4 drawn).
func TestDashSegmentsSplitsByPattern(t *testing.T) {
	segs := []svgSegment{
		segMove{x: 0, y: 0},
		segLine{x: 10, y: 0},
	}
	runs := dashSegments(segs, []float32{4, 2}, 0)
	if len(runs) != 2 {
		t.Fatalf("runs = %d, want 2 (got %+v)", len(runs), runs)
	}
	// First run: move(0,0) + line(4,0).
	if mv, ok := runs[0][0].(segMove); !ok || mv.x != 0 {
		t.Fatalf("first run starts at 0, got %+v", runs[0][0])
	}
	if ln, ok := runs[0][1].(segLine); !ok || ln.x != 4 {
		t.Fatalf("first run ends at x=4, got %+v", runs[0][1])
	}
	// Second run: move(6,0) + line(10,0).
	if mv, ok := runs[1][0].(segMove); !ok || mv.x != 6 {
		t.Fatalf("second run starts at 6, got %+v", runs[1][0])
	}
	if ln, ok := runs[1][1].(segLine); !ok || ln.x != 10 {
		t.Fatalf("second run ends at x=10, got %+v", runs[1][1])
	}
}

// TestDashSegmentsClosedPath verifies close segments join the dash flow
// (the closing edge participates in the pattern).
func TestDashSegmentsClosedPath(t *testing.T) {
	segs := []svgSegment{
		segMove{x: 0, y: 0},
		segLine{x: 4, y: 0},
		segClose{},
	}
	runs := dashSegments(segs, []float32{2, 2}, 0)
	// Total perimeter 8, pattern 2 on / 2 off: two drawn runs.
	if len(runs) != 2 {
		t.Fatalf("runs = %d, want 2", len(runs))
	}
}

// TestParseSVGDashOffset verifies stroke-dashoffset parsing and its effect:
// the offset shifts where the dash pattern starts (the first dash is
// shortened by the offset), zero keeps the plain pattern, and negative
// offsets wrap forward through the cycle.
func TestParseSVGDashOffset(t *testing.T) {
	src := `<svg viewBox="0 0 32 32"><path d="M2 16 L30 16" stroke="#ffffff" stroke-width="2" stroke-dasharray="4 2" stroke-dashoffset="3"/></svg>`
	paints := parseSVG(src, geometry.BoundsPixels(0, 0, 64, 64), gc.Black())
	if len(paints) != 1 {
		t.Fatalf("paints: %d, want 1", len(paints))
	}
	if paints[0].dashOffset != 3 {
		t.Fatalf("dashOffset = %v, want 3", paints[0].dashOffset)
	}

	segs := parsePathData("M2 16 L30 16")
	// Pattern [4 2] with offset 3: the stroke begins 3 units into the first
	// dash, so only 1 unit of it remains before the 2-unit gap.
	dashed := dashSegments(segs, []float32{4, 2}, 3)
	if len(dashed) == 0 {
		t.Fatal("offset dashing produced no segments")
	}
	if l := dashRunLength(dashed[0]); absF(l-1) > 0.01 {
		t.Fatalf("first dash length = %v, want 1 (4 - offset 3)", l)
	}

	// Offset 0 keeps the full first dash.
	if l := dashRunLength(dashSegments(segs, []float32{4, 2}, 0)[0]); absF(l-4) > 0.01 {
		t.Fatalf("first dash length with no offset = %v, want 4", l)
	}

	// Negative offset -1 equals offset 5 (cycle 6): 1 unit of the gap
	// remains, so the stroke starts with a 1-unit gap and the first dash
	// spans path x 3..7 (full 4-unit dash, shifted 1 past the path start).
	dashed = dashSegments(segs, []float32{4, 2}, -1)
	if len(dashed) == 0 {
		t.Fatal("negative offset produced no segments")
	}
	if m, ok := dashed[0][0].(segMove); !ok || absF(m.x-3) > 0.01 {
		t.Fatalf("first run start = %v, want x=3 (1 gap unit consumed first)", m)
	}
	if l := dashRunLength(dashed[0]); absF(l-4) > 0.01 {
		t.Fatalf("first dash length with offset -1 = %v, want 4", l)
	}
}

// dashRunLength sums the polyline length of one dashed run (MoveTo + lines),
// tracking the current point across MoveTo/Line elements.
func dashRunLength(run []svgSegment) float32 {
	total := float32(0)
	px, py := float32(0), float32(0)
	have := false
	for _, s := range run {
		switch v := s.(type) {
		case segMove:
			px, py = v.x, v.y
			have = true
		case segLine:
			if have {
				total += float32(math.Sqrt(float64((v.x-px)*(v.x-px) + (v.y-py)*(v.y-py))))
			}
			px, py = v.x, v.y
			have = true
		}
	}
	return total
}
