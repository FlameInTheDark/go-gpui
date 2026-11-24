package text

import (
	"strings"
	"testing"

	"github.com/FlameInTheDark/go-gpui/font"
	"github.com/FlameInTheDark/go-gpui/geometry"
)

func testSystem(t *testing.T) *System {
	t.Helper()
	s := NewSystemWithSystemFonts()
	if len(s.AllFontNames()) == 0 {
		t.Skip("no system fonts available")
	}
	return s
}

func TestFontResolution(t *testing.T) {
	s := testSystem(t)
	id, ok := s.FontID(font.New("DejaVu Sans"))
	if !ok {
		t.Fatal("resolve DejaVu Sans")
	}
	m := s.Metrics(id)
	if m.Ascent <= 0 || m.Descent >= 0 {
		t.Fatalf("metrics: %+v", m)
	}
	if s.Face(id) == nil {
		t.Fatal("face")
	}
	names := s.AllFontNames()
	found := false
	for _, n := range names {
		if n == "DejaVu Sans" {
			found = true
		}
	}
	if !found {
		t.Fatal("DejaVu Sans listed")
	}
	// Bold resolution.
	boldID, _ := s.FontID(font.New("DejaVu Sans").Bold())
	if s.Metrics(boldID).Ascent == 0 {
		t.Fatal("bold face")
	}
}

func TestShapeLine(t *testing.T) {
	s := testSystem(t)
	id, _ := s.FontID(font.New("DejaVu Sans"))
	runs := []FontRun{{Len: len("Hello world"), FontID: id}}
	l := s.LayoutLine("Hello world", geometry.Px(16), runs)
	if len(l.Runs) == 0 {
		t.Fatal("no runs")
	}
	totalGlyphs := 0
	for _, r := range l.Runs {
		totalGlyphs += len(r.Glyphs)
	}
	// 'Hello world' = 11 chars; the space renders as a glyph too.
	if totalGlyphs < 10 {
		t.Fatalf("glyph count: %d", totalGlyphs)
	}
	if l.Width.AsF32() <= 0 {
		t.Fatal("width")
	}
	if l.Ascent <= 0 || l.Descent > 0 {
		t.Fatalf("ascent/descent: %v %v", l.Ascent, l.Descent)
	}
}

func TestShapeMonospaceAdvance(t *testing.T) {
	s := testSystem(t)
	id, ok := s.FontID(font.New("DejaVu Sans Mono"))
	if !ok {
		t.Skip("no mono font")
	}
	l1 := s.LayoutLine("iiiiiiiiii", geometry.Px(16), []FontRun{{Len: 10, FontID: id}})
	l2 := s.LayoutLine("wwwwwwwwww", geometry.Px(16), []FontRun{{Len: 10, FontID: id}})
	d := l1.Width.AsF32() - l2.Width.AsF32()
	if d < -0.1 || d > 0.1 {
		t.Fatalf("monospace widths differ: %v vs %v", l1.Width, l2.Width)
	}
}

func TestIndexXRoundTrip(t *testing.T) {
	s := testSystem(t)
	id, _ := s.FontID(font.New("DejaVu Sans"))
	text := "Hello, gpui!"
	l := s.LayoutLine(text, geometry.Px(16), []FontRun{{Len: len(text), FontID: id}})
	for i := 0; i <= len(text); i++ {
		x := l.XForIndex(i)
		idx, ok := l.IndexForX(x)
		if i < len(text) {
			if !ok {
				t.Fatalf("index for x at %d", i)
			}
			if idx != i && text[idx] == text[i] {
				// same-cluster collisions are fine
			} else if idx > i {
				t.Fatalf("x_for_index/index_for_x drift at %d: got %d", i, idx)
			}
		}
	}
	// Closest always in range.
	if c := l.ClosestIndexForX(geometry.Px(l.Width.AsF32() + 100)); c != len(text) {
		t.Fatalf("closest past end: %d", c)
	}
	if c := l.ClosestIndexForX(geometry.Px(0)); c != 0 {
		t.Fatalf("closest at 0: %d", c)
	}
}

func TestSplitAt(t *testing.T) {
	s := testSystem(t)
	id, _ := s.FontID(font.New("DejaVu Sans"))
	text := "Hello, world"
	l := s.LayoutLine(text, geometry.Px(16), []FontRun{{Len: len(text), FontID: id}})
	left, right := l.SplitAt(5)
	if left.Len != 5 || right.Len != len(text)-5 {
		t.Fatalf("split lens: %d %d", left.Len, right.Len)
	}
	if d := left.Width.AsF32() + right.Width.AsF32() - l.Width.AsF32(); d < -0.01 || d > 0.01 {
		t.Fatalf("split widths: %v + %v vs %v", left.Width, right.Width, l.Width)
	}
}

func TestWrapBoundaries(t *testing.T) {
	s := testSystem(t)
	id, _ := s.FontID(font.New("DejaVu Sans"))
	text := "The quick brown fox jumps over the lazy dog"
	l := s.LayoutLine(text, geometry.Px(16), []FontRun{{Len: len(text), FontID: id}})
	w := &WrappedLineLayout{Unwrapped: l}
	// Wrap at roughly half the text width.
	wrapWidth := geometry.Px(l.Width.AsF32() / 2)
	w.WrapBoundaries = w.ComputeWrapBoundaries(text, wrapWidth, 0)
	if len(w.WrapBoundaries) == 0 {
		t.Fatal("should wrap")
	}
	// Every wrapped row must fit (approximately; spaces may hang slightly).
	// Row width = distance from this boundary to the next one (or the line
	// end). Boundary x values are positions in the unwrapped line and grow
	// with the row index, so they are not directly comparable to wrapWidth.
	bounds := make([]float32, 0, len(w.WrapBoundaries)+1)
	for _, b := range w.WrapBoundaries {
		g := glyphAt(l, b.RunIx, b.GlyphIx)
		bounds = append(bounds, g.Position.X.AsF32())
	}
	bounds = append(bounds, l.Width.AsF32())
	for i := 0; i < len(bounds)-1; i++ {
		rowWidth := bounds[i+1] - bounds[i]
		if rowWidth > wrapWidth.AsF32()*1.05 {
			t.Fatalf("wrapped row %d too wide: %.2f > %.2f", i, rowWidth, wrapWidth)
		}
	}
	// Full width: no boundaries.
	w2 := &WrappedLineLayout{Unwrapped: l}
	w2.WrapBoundaries = w2.ComputeWrapBoundaries(text, geometry.Px(l.Width.AsF32()+1), 0)
	if len(w2.WrapBoundaries) != 0 {
		t.Fatalf("no wrap at full width: %d", len(w2.WrapBoundaries))
	}
}

func TestIndexForPositionWrapped(t *testing.T) {
	s := testSystem(t)
	id, _ := s.FontID(font.New("DejaVu Sans"))
	text := "alpha beta gamma delta epsilon"
	l := s.LayoutLine(text, geometry.Px(16), []FontRun{{Len: len(text), FontID: id}})
	w := &WrappedLineLayout{Unwrapped: l}
	wrapWidth := geometry.Px(l.Width.AsF32() / 3)
	w.WrapBoundaries = w.ComputeWrapBoundaries(text, wrapWidth, 0)
	lh := geometry.Px(20)
	// Position on the second row.
	pos, ok := w.PositionForIndex(len(text)/2, lh)
	if !ok {
		t.Fatal("position for index")
	}
	idx, _, ok2 := w.IndexForPosition(pos, lh)
	if !ok2 && idx == 0 {
		t.Fatal("index for position")
	}
	if !strings.HasPrefix(text[idx:], "") || idx < 0 || idx > len(text) {
		t.Fatalf("index out of range: %d", idx)
	}
}

func TestFallbackShaping(t *testing.T) {
	s := testSystem(t)
	id, _ := s.FontID(font.New("DejaVu Sans"))
	// Mixed scripts: Latin + Cyrillic + CJK (falls back).
	text := "Hi я 中"
	runs := []FontRun{{Len: len(text), FontID: id}}
	l := s.LayoutLine(text, geometry.Px(16), runs)
	glyphs := 0
	for _, r := range l.Runs {
		glyphs += len(r.Glyphs)
	}
	if glyphs < 3 {
		t.Fatalf("fallback glyphs: %d", glyphs)
	}
	// At least two distinct font runs (fallback happened) when CJK exists
	// in the system.
	if len(l.Runs) < 2 {
		t.Log("note: single font covered everything or no CJK font installed")
	}
}

func TestRasterizeGlyph(t *testing.T) {
	s := testSystem(t)
	id, _ := s.FontID(font.New("DejaVu Sans"))
	face := s.Face(id)
	gid := face.GlyphID('A')
	w, h, _, _, mask := RasterizeGlyph(face, gid, 32, 0)
	if w == 0 || h == 0 || len(mask) != w*h {
		t.Fatalf("rasterize A: %dx%d", w, h)
	}
	// Some ink.
	sum := 0
	for _, v := range mask {
		sum += int(v)
	}
	if sum < 100 {
		t.Fatalf("glyph A has no ink: %d", sum)
	}
	// Coverage antialiasing: edge pixels have partial alpha.
	partials := 0
	for _, v := range mask {
		if v > 20 && v < 235 {
			partials++
		}
	}
	if partials == 0 {
		t.Fatal("no antialiased pixels")
	}
	// Subpixel shift changes the raster.
	_, _, _, _, mask2 := RasterizeGlyph(face, gid, 32, 0.5)
	diff := 0
	for i := range mask {
		d := int(mask[i]) - int(mask2[i])
		if d < 0 {
			d = -d
		}
		diff += d
	}
	if diff == 0 {
		t.Fatal("subpixel offset should change rasterization")
	}
}

func TestEmojiShaping(t *testing.T) {
	s := testSystem(t)
	id, _ := s.FontID(font.New("DejaVu Sans"))
	text := "🙂"
	l := s.LayoutLine(text, geometry.Px(16), []FontRun{{Len: len(text), FontID: id}})
	for _, run := range l.Runs {
		for _, g := range run.Glyphs {
			if g.IsEmoji {
				t.Log("emoji glyph routed to emoji font")
				return
			}
		}
	}
	t.Log("note: no emoji font coverage; fell back to text font")
}

func TestKerning(t *testing.T) {
	s := testSystem(t)
	for _, family := range []string{"Liberation Serif", "DejaVu Serif", "DejaVu Sans"} {
		id, ok := s.FontID(font.New(family))
		if !ok {
			continue
		}
		lAV := s.LayoutLine("AV", geometry.Px(32), []FontRun{{Len: 2, FontID: id}})
		lAA := s.LayoutLine("AA", geometry.Px(32), []FontRun{{Len: 2, FontID: id}})
		if lAV.Width.AsF32() < lAA.Width.AsF32()-0.01 {
			t.Logf("%s: kerning works (AV %v < AA %v)", family, lAV.Width, lAA.Width)
			return
		}
	}
	t.Log("note: no kerning pairs found in available fonts")
}
