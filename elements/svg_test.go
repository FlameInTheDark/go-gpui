package elements

import (
	"fmt"
	"testing"

	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
)

// TestParseSVGAttributes verifies the mini-SVG parser: attribute keys must
// not leak the tag name (regression: `path d=` parsed the first attribute
// as key "path d"), and stroke/stroke-width must be picked up.
func TestParseSVGAttributes(t *testing.T) {
	src := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><path d="M2 8 L6 12 L14 3" fill="none" stroke="#a6e3a1" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>`
	els := scanElements(src)
	if len(els) != 2 {
		t.Fatalf("elements: %d, want 2", len(els))
	}
	path := els[1]
	if path.name != "path" {
		t.Fatalf("name: %q", path.name)
	}
	if d, ok := path.attr("d"); !ok || d != "M2 8 L6 12 L14 3" {
		t.Fatalf("d attr = %q, %v", d, ok)
	}

	paints := parseSVG(src, geometry.BoundsPixels(0, 0, 32, 32), gc.Black())
	if len(paints) != 1 {
		t.Fatalf("paints: %d, want 1", len(paints))
	}
	p := paints[0]
	if p.fill != nil {
		t.Errorf("fill=none must yield nil fill, got %v", p.fill)
	}
	if p.stroke == nil {
		t.Fatal("stroke not parsed")
	}
	// stroke-width 2 scaled by the viewBox ratio (32/16 = 2) -> 4.
	if p.width != 4 {
		t.Errorf("stroke-width = %v, want 4 (2 scaled by viewBox ratio)", p.width)
	}
	if len(p.segments) != 3 {
		t.Errorf("segments = %d, want 3 (move + 2 lines)", len(p.segments))
	}
}

// TestParseSVGViewBoxTransform verifies the viewBox mapping lands in the
// element's bounds (origin offset included, regression: icons rendered at
// the window origin).
func TestParseSVGViewBoxTransform(t *testing.T) {
	src := `<svg viewBox="0 0 16 16"><path d="M8 8" stroke="#ffffff" stroke-width="1"/></svg>`
	bounds := geometry.BoundsPixels(100, 50, 32, 32) // origin (100,50), size 32x32
	paints := parseSVG(src, bounds, gc.Black())
	if len(paints) != 1 {
		t.Fatalf("paints: %d, want 1", len(paints))
	}
	mv, ok := paints[0].segments[0].(segMove)
	if !ok {
		t.Fatalf("first segment is %T, want segMove", paints[0].segments[0])
	}
	// (8,8) scaled by 32/16=2 and translated to (100,50) -> (116,66).
	if mv.x != 116 || mv.y != 66 {
		t.Errorf("move = (%v,%v), want (116,66)", mv.x, mv.y)
	}
	// Stroke width scales by the viewBox ratio (2).
	if paints[0].width != 2 {
		t.Errorf("scaled stroke width = %v, want 2", paints[0].width)
	}
}

// TestParseSVGFillAndStroke parses an element with both fill and stroke.
func TestParseSVGFillAndStroke(t *testing.T) {
	src := `<svg viewBox="0 0 10 10"><rect x="1" y="1" width="8" height="8" fill="#ff0000" stroke="#00ff00" stroke-width="0.5"/></svg>`
	paints := parseSVG(src, geometry.BoundsPixels(0, 0, 20, 20), gc.Black())
	if len(paints) != 1 {
		t.Fatalf("paints: %d, want 1", len(paints))
	}
	if paints[0].fill == nil || paints[0].stroke == nil {
		t.Fatalf("fill/stroke = %v / %v, want both", paints[0].fill, paints[0].stroke)
	}
	if fmt.Sprintf("%.3f", paints[0].width) != "1.000" {
		t.Errorf("stroke width = %v, want 1 (0.5 scaled by 2)", paints[0].width)
	}
}
