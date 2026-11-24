package elements

import (
	"image"
	"image/color"
	"testing"

	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/style"
)

// TestContainerQueryReceivesAssignedSize verifies the render callback gets
// the size assigned during layout — the full parent size by default, or
// the style's explicit size when one is set (upstream container_query
// semantics: the element's own size comes solely from its style).
func TestContainerQueryReceivesAssignedSize(t *testing.T) {
	var defaultSize, sizedSize geometry.Size[geometry.Pixels]

	// Default: fills the parent (relative 1.).
	openAnchoredWindow(t, 400, 300, func(w *gpui.Window) gpui.Element {
		return NewContainerQuery(func(size geometry.Size[geometry.Pixels], w *gpui.Window) gpui.Element {
			defaultSize = size
			return NewDiv()
		})
	})
	if defaultSize.Width.AsF32() < 399 || defaultSize.Height.AsF32() < 299 {
		t.Fatalf("default container query should fill the parent, got %v", defaultSize)
	}

	// Explicit size on the element itself.
	openAnchoredWindow(t, 400, 300, func(w *gpui.Window) gpui.Element {
		cq := NewContainerQuery(func(size geometry.Size[geometry.Pixels], w *gpui.Window) gpui.Element {
			sizedSize = size
			return NewDiv()
		})
		cq.StyleRef().W(geometry.LengthFromPixels(geometry.Px(120)))
		cq.StyleRef().H(geometry.LengthFromPixels(geometry.Px(80)))
		return cq
	})
	if d := sizedSize.Width.AsF32() - 120; d < -0.51 || d > 0.51 {
		t.Fatalf("sized query width = %v, want 120", sizedSize.Width.AsF32())
	}
	if d := sizedSize.Height.AsF32() - 80; d < -0.51 || d > 0.51 {
		t.Fatalf("sized query height = %v, want 80", sizedSize.Height.AsF32())
	}
}

// TestContainerQuerySwitchesLayoutBySize verifies the CSS container-query
// use case: the callback renders different content depending on the
// assigned size.
func TestContainerQuerySwitchesLayoutBySize(t *testing.T) {
	var sawNarrow, sawWide bool

	openAnchoredWindow(t, 500, 100, func(w *gpui.Window) gpui.Element {
		cq := NewContainerQuery(func(size geometry.Size[geometry.Pixels], w *gpui.Window) gpui.Element {
			if size.Width < geometry.Px(240) {
				sawNarrow = true
				return NewDiv().W(geometry.LengthFromPixels(geometry.Px(100))).H(geometry.LengthFromPixels(geometry.Px(10)))
			}
			sawWide = true
			return NewDiv().W(geometry.LengthFromPixels(geometry.Px(400))).H(geometry.LengthFromPixels(geometry.Px(10)))
		})
		cq.StyleRef().W(geometry.LengthFromPixels(geometry.Px(200)))
		return cq
	})
	if !sawNarrow {
		t.Fatal("expected narrow branch at width 200")
	}

	openAnchoredWindow(t, 500, 100, func(w *gpui.Window) gpui.Element {
		cq := NewContainerQuery(func(size geometry.Size[geometry.Pixels], w *gpui.Window) gpui.Element {
			if size.Width < geometry.Px(240) {
				return NewDiv().W(geometry.LengthFromPixels(geometry.Px(100)))
			}
			sawWide = true
			return NewDiv().W(geometry.LengthFromPixels(geometry.Px(400)))
		})
		cq.StyleRef().W(geometry.LengthFromPixels(geometry.Px(480)))
		return cq
	})
	if !sawWide {
		t.Fatal("expected wide branch at width 480")
	}
}

// TestSurfacePaintsContent verifies the surface element paints its source
// image within its bounds (upstream surface; the port routes through the
// image path with ObjectFit semantics).
func TestSurfacePaintsContent(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			src.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	win := openAnchoredWindow(t, 200, 100, func(w *gpui.Window) gpui.Element {
		s := NewSurface(src)
		s.StyleRef().W(geometry.LengthFromPixels(geometry.Px(80)))
		s.StyleRef().H(geometry.LengthFromPixels(geometry.Px(40)))
		return s
	})
	if len(win.RenderedScene().PolySprites) == 0 && len(win.RenderedScene().Quads) == 0 {
		t.Fatalf("surface painted nothing: %d polys, %d quads", len(win.RenderedScene().PolySprites), len(win.RenderedScene().Quads))
	}
}

var _ = style.ObjectFitContain
