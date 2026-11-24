package text

import (
	"os"
	"testing"

	"github.com/FlameInTheDark/go-gpui/font"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/text/sfnt"
)

// shapedGlyphs runs LayoutLine and returns the glyphs of all runs in order.
func shapedGlyphs(t *testing.T, s *System, text string, id FontID) []ShapedGlyph {
	t.Helper()
	l := s.LayoutLine(text, geometry.Px(32), []FontRun{{Len: len(text), FontID: id}})
	var out []ShapedGlyph
	for _, run := range l.Runs {
		out = append(out, run.Glyphs...)
	}
	return out
}

// dejavuID resolves DejaVu Sans, skipping the test when unavailable.
func dejavuID(t *testing.T, s *System) FontID {
	t.Helper()
	id, ok := s.FontID(font.New("DejaVu Sans"))
	if !ok {
		t.Skip("DejaVu Sans not installed")
	}
	return id
}

// TestCombiningMarkPositioning: a combining mark attaches over its base
// (DejaVu's GPOS MarkToBase anchors it; fonts without mark data get the
// fallback shaper's centering) instead of parking at the pen after the
// base's advance, and it never advances the line.
func TestCombiningMarkPositioning(t *testing.T) {
	s := testSystem(t)
	id := dejavuID(t, s)
	withMark := shapedGlyphs(t, s, "a\u0301", id)
	if len(withMark) != 2 {
		t.Fatalf("a+combining acute glyphs: %d (want 2)", len(withMark))
	}
	// The mark must not advance the line.
	lPlain := s.LayoutLine("a", geometry.Px(32), []FontRun{{Len: 1, FontID: id}})
	l := s.LayoutLine("a\u0301", geometry.Px(32), []FontRun{{Len: 4, FontID: id}})
	if diff := l.Width.AsF32() - lPlain.Width.AsF32(); diff > 2 {
		t.Fatalf("combining mark advanced the pen by %v", diff)
	}
	// The mark's ORIGIN sits over the base (DejaVu's acute anchor: dx 1098
	// units of the 1255-unit advance; its ink is drawn left of the origin,
	// landing mid-glyph). The key property: it is attached within the base,
	// not parked after the base's full advance.
	markX := withMark[1].Position.X.AsF32()
	baseAdv := l.Width.AsF32()
	if markX < 0 || markX > baseAdv-1 {
		t.Fatalf("mark origin at x=%v, want attached within the base advance %v", markX, baseAdv)
	}
	if absF32(markX-float32(1098)*float32(32)/float32(s.Face(id).UnitsPerEm)) > 0.1 {
		t.Fatalf("mark origin x=%v, want DejaVu anchor dx 1098 units", markX)
	}
}

// TestMarkToBaseAnchors: Arabic fatha attaches to its base through GPOS
// MarkToBase anchors (DejaVu lookup 9: mark anchor (512,1200), beh anchor
// (900,1000) -> dx 388 units) and lifts above the baseline.
func TestMarkToBaseAnchors(t *testing.T) {
	s := testSystem(t)
	id := dejavuID(t, s)
	face := s.Face(id)
	if face == nil || face.GlyphID(0x0628) == 0 || face.GlyphID(0x064E) == 0 {
		t.Skip("no Arabic coverage in DejaVu Sans")
	}
	glyphs := shapedGlyphs(t, s, "\u0628\u064E", id)
	if len(glyphs) != 2 {
		t.Fatalf("beh+fatha glyphs: %d (want 2)", len(glyphs))
	}
	// Locate the mark by glyph id (RTL reordering changes entry order).
	fathaGID := face.GlyphID(0x064E)
	mark := glyphs[0]
	if mark.ID != uint32(fathaGID) {
		mark = glyphs[1]
	}
	if mark.ID != uint32(fathaGID) {
		t.Fatalf("fatha glyph not found: %v", glyphIDs(glyphs))
	}
	// beh at x=0; fatha at x = 388/2048*32 = 6.0625.
	scale := float32(32) / float32(face.UnitsPerEm)
	want := float32(900-512) * scale
	if got := mark.Position.X.AsF32(); absF32(got-want) > 0.05 {
		t.Fatalf("fatha x = %v, want anchor dx %v", got, want)
	}
	// Mark anchor y (1200) sits above the base anchor y (1000): the fatha's
	// origin lands 200 units BELOW the baseline while its ink floats over
	// the base (ground truth: attach dy -200 font units -> +3.125px at 32px).
	scale = float32(32) / float32(face.UnitsPerEm)
	if got := mark.Position.Y.AsF32(); absF32(got-float32(1200-1000)*scale) > 0.05 {
		t.Fatalf("fatha y = %v, want %v", got, float32(1200-1000)*scale)
	}
}

// TestArabicJoiningForms: the joining-gated init/medi/fina features pick
// positional forms: a dual-joining letter at word start takes the initial
// form, mid-word medial, at the end final; standalone it keeps the isolated
// (base) glyph.
func TestArabicJoiningForms(t *testing.T) {
	s := testSystem(t)
	id := dejavuID(t, s)
	face := s.Face(id)
	beh := face.GlyphID(0x0628)
	if face == nil || beh == 0 {
		t.Skip("no Arabic coverage in DejaVu Sans")
	}

	// Standalone beh: isolated form (the cmap glyph).
	solo := shapedGlyphs(t, s, "\u0628", id)
	if len(solo) != 1 || solo[0].ID != uint32(beh) {
		t.Fatalf("standalone beh should keep the isolated glyph, got %v", glyphIDs(solo))
	}

	// beh beh: first takes initial, second final — both substituted and
	// distinct from each other and from the isolated form.
	pair := shapedGlyphs(t, s, "\u0628\u0628", id)
	if len(pair) != 2 {
		t.Fatalf("beh beh glyphs: %d", len(pair))
	}
	first, second := pair[0].ID, pair[1].ID
	if first == uint32(beh) || second == uint32(beh) {
		t.Fatalf("joined behs not substituted: %v (base %d)", glyphIDs(pair), beh)
	}
	if first == second {
		t.Fatalf("word-start and word-end forms identical: %v", glyphIDs(pair))
	}

	// beh seen meem: init, medi, fina — three distinct substituted forms.
	word := shapedGlyphs(t, s, "\u0628\u0633\u0645", id)
	if len(word) != 3 {
		t.Fatalf("beh seen meem glyphs: %d", len(word))
	}
	if word[0].ID == word[1].ID || word[1].ID == word[2].ID || word[0].ID == word[2].ID {
		t.Fatalf("joining forms not distinct: %v", glyphIDs(word))
	}
}

// TestArabicLamAlefForms: DejaVu carries no lam-alef ligature glyph, so the
// pair renders as lam-initial + alef-final joined forms (both substituted
// away from their isolated glyphs).
func TestArabicLamAlefForms(t *testing.T) {
	s := testSystem(t)
	id := dejavuID(t, s)
	face := s.Face(id)
	lam, alef := face.GlyphID(0x0644), face.GlyphID(0x0627)
	if face == nil || lam == 0 {
		t.Skip("no Arabic coverage in DejaVu Sans")
	}
	glyphs := shapedGlyphs(t, s, "\u0644\u0627", id)
	if len(glyphs) != 2 {
		t.Fatalf("lam+alef glyphs: %d (want 2)", len(glyphs))
	}
	for _, g := range glyphs {
		if g.ID == uint32(lam) || g.ID == uint32(alef) {
			t.Fatalf("joined lam/alef kept an isolated glyph: %v", glyphIDs(glyphs))
		}
	}
}

// TestEmojiZWJLigature: Noto Color Emoji ligates ZWJ sequences (family) into
// a single glyph through the ccmp chain-context lookups, and leftover ZWJ
// code points never produce glyphs.
func TestEmojiZWJLigature(t *testing.T) {
	s := testSystem(t)
	id, ok := s.FontID(font.New("Noto Color Emoji"))
	if !ok {
		t.Skip("Noto Color Emoji not installed")
	}
	fam := "\U0001F468\u200D\U0001F469\u200D\U0001F467"
	glyphs := shapedGlyphs(t, s, fam, id)
	if len(glyphs) != 1 {
		t.Fatalf("family emoji should ligate to 1 glyph, got %d: %v", len(glyphs), glyphIDs(glyphs))
	}
	// Unpaired ZWJ contributes no glyph and no advance.
	lPlain := s.LayoutLine("\U0001F468\U0001F469", geometry.Px(32), []FontRun{{Len: 8, FontID: id}})
	lZWJ := s.LayoutLine("\U0001F468\u200D\U0001F469", geometry.Px(32), []FontRun{{Len: 9, FontID: id}})
	if absF32(lZWJ.Width.AsF32()-lPlain.Width.AsF32()) > 0.01 {
		t.Fatalf("ZWJ changed the width: %v vs %v", lZWJ.Width, lPlain.Width)
	}
}

// TestVariationSelectorNotRendered: a variation selector consumed by a
// ligature (heart+VS16) disappears from the output.
func TestVariationSelectorNotRendered(t *testing.T) {
	s := testSystem(t)
	id, ok := s.FontID(font.New("Noto Color Emoji"))
	if !ok {
		t.Skip("Noto Color Emoji not installed")
	}
	glyphs := shapedGlyphs(t, s, "\u2764\uFE0F", id)
	if len(glyphs) != 1 {
		t.Fatalf("heart+VS16 should render one glyph, got %v", glyphIDs(glyphs))
	}
}

// TestPairKerningThroughEngine: GPOS PairPos still applies through the new
// engine (AV narrower than AA).
func TestPairKerningThroughEngine(t *testing.T) {
	s := testSystem(t)
	id := dejavuID(t, s)
	av := s.LayoutLine("AV", geometry.Px(32), []FontRun{{Len: 2, FontID: id}})
	aa := s.LayoutLine("AA", geometry.Px(32), []FontRun{{Len: 2, FontID: id}})
	if av.Width.AsF32() >= aa.Width.AsF32()-0.01 {
		t.Fatalf("AV (%v) should kern tighter than AA (%v)", av.Width, aa.Width)
	}
}

// TestExtensionLookupUnwrapped: Noto Serif SC's GPOS carries type-9
// extension lookups; the parser must unwrap them to real subtable types.
func TestExtensionLookupUnwrapped(t *testing.T) {
	data, err := os.ReadFile("/usr/share/fonts/truetype/noto-serif-sc/NotoSerifSC-Regular.ttf")
	if err != nil {
		t.Skip("Noto Serif SC not installed")
	}
	f, err := sfnt.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if f.GPOS == nil {
		t.Skip("no GPOS")
	}
	sawExtension := false
	for _, lk := range f.GPOS.Lookups {
		if lk.Type == 9 {
			sawExtension = true
			for _, sub := range lk.Subtables {
				if sub.Type == 9 || sub.Type == 0 {
					t.Fatalf("extension subtable not unwrapped: type %d", sub.Type)
				}
			}
		}
	}
	if !sawExtension {
		t.Log("note: no type-9 lookups in this font revision")
	}
}

// TestGDEFParsed: DejaVu classifies combining marks as GDEF class 3.
func TestGDEFParsed(t *testing.T) {
	s := testSystem(t)
	id := dejavuID(t, s)
	face := s.Face(id)
	if face.GDEF == nil {
		t.Fatal("DejaVu GDEF not parsed")
	}
	if !face.IsMarkGlyph(face.GlyphID(0x0301)) {
		t.Fatal("combining acute should be a GDEF mark")
	}
	if cls := face.GlyphClass(face.GlyphID('a')); cls != sfnt.ClassBase {
		t.Fatalf("letter 'a' class %d, want base", cls)
	}
}

func glyphIDs(glyphs []ShapedGlyph) []uint32 {
	out := make([]uint32, len(glyphs))
	for i, g := range glyphs {
		out[i] = g.ID
	}
	return out
}
