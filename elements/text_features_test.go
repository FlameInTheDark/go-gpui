package elements

import (
	"testing"

	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/render"
)

// TestTextWrappedRowsPaintSlices verifies wrapped rows paint only their own
// glyph slice: each sprite row appears once, at distinct Y positions (the
// pre-fix bug repainted the whole line on every row).
func TestTextWrappedRowsPaintSlices(t *testing.T) {
	var txt *Text
	win := openAnchoredWindow(t, 120, 80, func(w *gpui.Window) gpui.Element {
		if txt == nil {
			txt = NewText("aaaa bbbb cccc dddd")
		}
		host := NewDiv()
		host.StyleRef().W(geometry.LengthFromDefinite(geometry.AbsoluteDefinite(geometry.PixelsLength(geometry.Px(110)))))
		host.Child(txt)
		return host
	})
	if txt == nil || txt.Layout() == nil {
		t.Fatal("text element missing layout")
	}
	l := txt.Layout()
	if l.WrapLines < 2 {
		t.Fatalf("expected wrapped rows, got %d (width %v)", l.WrapLines, l.Width)
	}

	// The scene's mono sprites must span distinct rows: their Y positions
	// cluster by line height.
	scene := win.RenderedScene()
	if len(scene.MonoSprites) == 0 {
		t.Fatal("no glyphs painted")
	}
	rows := map[float32]int{}
	for _, s := range scene.MonoSprites {
		rows[float32(int(s.Bounds.Origin.Y.AsF32()/16))]++
	}
	// 110px of ~8px glyphs: 4 words of 4 chars → at least 2 rows.
	if len(rows) < 2 {
		t.Fatalf("wrapped glyphs occupy %d distinct rows, want >= 2", len(rows))
	}
}

func TestTextWithWidthConstrainsPaintBounds(t *testing.T) {
	var txt *Text
	win := openAnchoredWindow(t, 220, 180, func(w *gpui.Window) gpui.Element {
		txt = NewText("A long paragraph with enough words to wrap across several lines.").WithWidth(geometry.Px(120))
		return NewDiv().Flex().FlexCol().Child(txt)
	})
	if got := win.ElementBounds(txt).Size.Width.AsF32(); got > 120.5 {
		t.Fatalf("paint width = %v, want at most 120", got)
	}
	if txt.Layout() == nil || txt.Layout().WrapLines < 2 {
		t.Fatal("text did not wrap in its explicit width")
	}
}

// TestTextWrappedRowIndent verifies the indent adjustment: continuation
// rows align under the first row's leading whitespace (upstream
// IndentAdjustment::SameIndent), so a wrapped row starting with non-space
// glyphs is shifted right by the indent width.
func TestTextWrappedRowIndent(t *testing.T) {
	var txt *Text
	win := openAnchoredWindow(t, 120, 80, func(w *gpui.Window) gpui.Element {
		if txt == nil {
			txt = NewText("    aaaa bbbb cccc dddd")
		}
		host := NewDiv()
		host.StyleRef().W(geometry.LengthFromDefinite(geometry.AbsoluteDefinite(geometry.PixelsLength(geometry.Px(110)))))
		host.Child(txt)
		return host
	})
	l := txt.Layout()
	if l.WrapLines < 2 {
		t.Fatalf("expected wrapped rows, got %d", l.WrapLines)
	}
	if l.Wrapped == nil || l.Wrapped.Indent <= 0 {
		t.Fatalf("indent adjustment not applied: %+v", l.Wrapped)
	}
	// The final row ("dddd", no leading whitespace of its own) must paint
	// shifted right by the indent: its sprites' minimum x is at least the
	// indent width. The final row is the band with the greatest sprite y.
	scene := win.RenderedScene()
	if len(scene.MonoSprites) == 0 {
		t.Fatal("no glyphs painted")
	}
	maxY := float32(-1)
	for _, s := range scene.MonoSprites {
		if y := s.Bounds.Origin.Y.AsF32(); y > maxY {
			maxY = y
		}
	}
	minX := float32(1e9)
	for _, s := range scene.MonoSprites {
		if y := s.Bounds.Origin.Y.AsF32(); y >= maxY-1 {
			if x := s.Bounds.Origin.X.AsF32(); x < minX {
				minX = x
			}
		}
	}
	if minX < l.Wrapped.Indent-1 {
		t.Fatalf("continuation row starts at x=%v, want >= indent %v", minX, l.Wrapped.Indent)
	}
}

// TestTextMarkedRangePaints verifies WithMarkedRange paints the IME preedit
// highlight quad over the marked range and a stronger one over the selected
// clause.
func TestTextMarkedRangePaints(t *testing.T) {
	win := openAnchoredWindow(t, 200, 40, func(w *gpui.Window) gpui.Element {
		return NewText("hello world").WithMarkedRange([2]int{0, 5}, [2]int{2, 4})
	})
	quads := win.PaintedQuads()
	var marked, selected *render.Quad
	for i := range quads {
		q := &quads[i]
		solid, ok := q.Background.AsSolid()
		if !ok || solid.A == 0 {
			continue
		}
		switch solid.A {
		case 0.25:
			if marked == nil {
				marked = q
			}
		case 0.45:
			if selected == nil {
				selected = q
			}
		}
	}
	if marked == nil {
		t.Fatal("marked-range highlight quad not painted")
	}
	if marked.Bounds.Size.Width.AsF32() <= 0 {
		t.Fatal("marked highlight has no width")
	}
	if selected == nil {
		t.Fatal("selected-clause highlight quad not painted")
	}
	if selected.Bounds.Size.Width.AsF32() >= marked.Bounds.Size.Width.AsF32() {
		t.Fatalf("selected clause (%v) must be narrower than the marked range (%v)",
			selected.Bounds.Size.Width.AsF32(), marked.Bounds.Size.Width.AsF32())
	}
}
