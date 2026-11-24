package text

import (
	"testing"

	"github.com/FlameInTheDark/go-gpui/geometry"
)

// bidiLayout shapes a mixed-direction line with a real system (skipped
// when no font is available).
func bidiLayout(t *testing.T, s string) *LineLayout {
	t.Helper()
	sys := testSystem(t)
	l := sys.LayoutLine(s, geometry.Px(16), nil)
	return l
}

// TestBidiVisualOrderArabicNumberLatin verifies the classic mixed line
// "abc-אבג" renders with the Hebrew segment reversed in visual order:
// glyph x positions of the Hebrew characters decrease with logical order.
func TestBidiVisualOrderArabicNumberLatin(t *testing.T) {
	l := bidiLayout(t, "abc אבג def")
	if len(l.BidiSegments) == 0 {
		t.Skip("no bidi segments recorded (font fallback missing?)")
	}
	// Collect the Hebrew glyphs (logical byte offsets 4..10) in visual
	// (left-to-right) order with their logical indices.
	type gpos struct {
		x float32
		i int
	}
	var gs []gpos
	for _, run := range l.Runs {
		for _, g := range run.Glyphs {
			if g.Index >= 4 && g.Index < 10 {
				gs = append(gs, gpos{x: g.Position.X.AsF32(), i: g.Index})
			}
		}
	}
	if len(gs) < 3 {
		t.Fatalf("hebrew glyphs = %d, want >= 3", len(gs))
	}
	// Visual order: the visually-leftmost Hebrew glyph carries the
	// HIGHEST logical index (the run is reversed: א ב ג logical → ג ב א
	// visual), and x increases left to right.
	for i := 1; i < len(gs); i++ {
		if gs[i].x <= gs[i-1].x {
			t.Fatalf("glyph xs not left-to-right: %+v", gs)
		}
		if gs[i].i >= gs[i-1].i {
			t.Fatalf("hebrew logical indices not reversed in visual order: %+v", gs)
		}
	}
	// The visually-first Hebrew glyph sits right of "abc " (~24px).
	if gs[0].x <= 24 {
		t.Fatalf("hebrew segment misplaced: %+v", gs)
	}
}

// TestBidiCaretBoundariesMonotonicX verifies the caret boundary list is
// sorted by x and its indices are valid logical offsets.
func TestBidiCaretBoundariesMonotonicX(t *testing.T) {
	l := bidiLayout(t, "abc אבג def")
	if len(l.BidiSegments) == 0 {
		t.Skip("no bidi segments recorded")
	}
	bs := l.caretBoundaries()
	for i := 1; i < len(bs); i++ {
		if bs[i].x < bs[i-1].x {
			t.Fatalf("boundaries not sorted by x: %v", bs)
		}
		if bs[i].index < 0 || bs[i].index > l.Len {
			t.Fatalf("boundary index out of range: %d (len %d)", bs[i].index, l.Len)
		}
	}
}

// TestBidiClosestIndexForXRountrips verifies caret queries land on valid
// boundaries and clicking at a glyph's visual x returns a coherent index.
func TestBidiClosestIndexForXRountrips(t *testing.T) {
	l := bidiLayout(t, "abc אבג def")
	if len(l.BidiSegments) == 0 {
		t.Skip("no bidi segments recorded")
	}
	// Clicking near x=0 (before "a") gives an early logical index;
	// clicking near the line end gives the end index.
	if ix := l.ClosestIndexForX(geometry.Px(0)); ix != 0 {
		t.Fatalf("closest at x=0 = %d, want 0", ix)
	}
	if ix := l.ClosestIndexForX(l.Width); ix != l.Len {
		t.Fatalf("closest at line end = %d, want %d", ix, l.Len)
	}
	// Clicking just right of every boundary edge returns exactly that
	// boundary's index (the caret lands on the slot).
	for _, b := range l.caretBoundaries() {
		got := l.ClosestIndexForX(geometry.Px(b.x + 0.1))
		if got != b.index {
			t.Fatalf("boundary x=%.1f idx=%d: closest just right = %d", b.x, b.index, got)
		}
	}
	// A logical index shared by two visual slots (BiDi boundary
	// ambiguity, e.g. the caret before/after the Hebrew run) maps to the
	// visually-first slot.
	x := l.XForIndex(10).AsF32()
	if x != 32 {
		t.Fatalf("XForIndex(10) = %.1f, want 32 (first slot)", x)
	}
}

// TestBidiIndexForXVisual verifies IndexForX walks visual order: querying
// inside the Hebrew run returns a Hebrew byte offset (4..10), not an
// English one.
func TestBidiIndexForXVisual(t *testing.T) {
	l := bidiLayout(t, "abc אבג def")
	if len(l.BidiSegments) == 0 {
		t.Skip("no bidi segments recorded")
	}
	// The Hebrew run occupies the visual middle: between the x of the
	// first English glyph and the last.
	var minX, maxX float32 = 1e9, -1e9
	for _, run := range l.Runs {
		for _, g := range run.Glyphs {
			if g.Index >= 4 && g.Index < 10 {
				if g.Position.X.AsF32() < minX {
					minX = g.Position.X.AsF32()
				}
				if g.Position.X.AsF32() > maxX {
					maxX = g.Position.X.AsF32()
				}
			}
		}
	}
	if minX >= maxX {
		t.Fatalf("no hebrew glyph range: %v..%v", minX, maxX)
	}
	mid := (minX + maxX) / 2
	ix, ok := l.IndexForX(geometry.Px(mid))
	if !ok {
		t.Fatal("IndexForX inside hebrew failed")
	}
	if ix < 4 || ix > 10 {
		t.Fatalf("IndexForX(mid hebrew) = %d, want 4..10", ix)
	}
}
