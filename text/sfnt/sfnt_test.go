package sfnt

import (
	"os"
	"testing"
)

func mustParse(t *testing.T, path string) *Font {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("font %s unavailable: %v", path, err)
	}
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return f
}

func TestParseDejaVu(t *testing.T) {
	f := mustParse(t, "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf")
	if f.UnitsPerEm != 2048 {
		t.Fatalf("upem: %d", f.UnitsPerEm)
	}
	if f.FamilyName != "DejaVu Sans" {
		t.Fatalf("family: %q", f.FamilyName)
	}
	// cmap: A -> nonzero glyph.
	if g := f.GlyphID('A'); g == 0 {
		t.Fatal("glyph for A")
	}
	// Cyrillic coverage.
	if g := f.GlyphID('я'); g == 0 {
		t.Fatal("glyph for Cyrillic ya")
	}
	// Advance for A is positive.
	if adv := f.Advance(f.GlyphID('A')); adv <= 0 {
		t.Fatalf("advance for A: %d", adv)
	}
	// Metrics.
	m := f.Metrics()
	if m.Ascent <= 0 || m.Descent >= 0 {
		t.Fatalf("metrics: %+v", m)
	}
	// GSUB/GPOS presence (DejaVu has both).
	if f.GSUB == nil {
		t.Fatal("GSUB")
	}
	if f.GPOS == nil {
		t.Fatal("GPOS")
	}
	if f.Kern(f.GlyphID('A'), f.GlyphID('V')) == 0 {
		t.Log("note: kern(A,V) == 0; DejaVu may keep kerning in GPOS only")
	}
}

func TestOutlineA(t *testing.T) {
	f := mustParse(t, "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf")
	o := f.GlyphOutline(f.GlyphID('A'))
	if len(o.Points) < 3 || len(o.Ends) < 1 {
		t.Fatalf("outline for A too small: %d points %d contours", len(o.Points), len(o.Ends))
	}
	if o.YMax <= 0 {
		t.Fatalf("A should reach above the baseline: %v", o.YMax)
	}
}

func TestCmapFormats(t *testing.T) {
	f := mustParse(t, "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf")
	// Unicode ranges.
	for _, r := range []rune{'a', 'Z', '0', ' ', '~', 0x00e9, 0x0444, 0x4e2d, 0x1F600} {
		gid := f.GlyphID(r)
		if r <= 0x2FFF && gid == 0 {
			t.Fatalf("missing glyph for U+%04X", r)
		}
		t.Logf("U+%04X -> gid %d", r, gid)
	}
}

func TestNames(t *testing.T) {
	f := mustParse(t, "/usr/share/fonts/truetype/liberation/LiberationSerif-Regular.ttf")
	if f.FamilyName != "Liberation Serif" {
		t.Fatalf("family: %q", f.FamilyName)
	}
	if f.SubfamilyName != "Regular" {
		t.Fatalf("subfamily: %q", f.SubfamilyName)
	}
	bold := mustParse(t, "/usr/share/fonts/truetype/liberation/LiberationSerif-Bold.ttf")
	if !bold.IsBold {
		t.Fatal("bold flag")
	}
	if bold.WeightClass < 600 {
		t.Fatalf("weight: %d", bold.WeightClass)
	}
}

func TestGSUBLigatureData(t *testing.T) {
	f := mustParse(t, "/usr/share/fonts/truetype/liberation/LiberationSerif-Regular.ttf")
	if f.GSUB == nil {
		t.Skip("no GSUB")
	}
	liga := f.GSUB.FeatureLookups("liga")
	t.Logf("liga lookups: %v (features: %d)", liga, len(f.GSUB.Features))
}

func TestLiberationKern(t *testing.T) {
	f := mustParse(t, "/usr/share/fonts/truetype/liberation/LiberationSerif-Regular.ttf")
	a := f.GlyphID('A')
	v := f.GlyphID('V')
	if a == 0 || v == 0 {
		t.Fatal("missing glyphs")
	}
	if k := f.Kern(a, v); k == 0 {
		t.Log("note: kern table empty for AV (GPOS may carry it)")
	}
}
