package text

import (
	"testing"

	"github.com/FlameInTheDark/go-gpui/font"
	"github.com/FlameInTheDark/go-gpui/geometry"
)

// TestTabAdvanceShapesTabStops verifies tabs advance to multiples of
// TabWidthEm spaces and produce no glyph.
func TestTabAdvanceShapesTabStops(t *testing.T) {
	sys := testSystem(t)
	fontID, ok := sys.FontID(font.New("Monospace"))
	if !ok {
		fontID, ok = sys.FontID(font.New("DejaVu Sans"))
		if !ok {
			t.Skip("no usable font available")
		}
	}
	size := geometry.Px(16)
	run := []FontRun{{Len: 5, FontID: fontID}}

	l := sys.LayoutLine("a\tb", size, run)

	// No glyph may exist for the tab itself.
	for _, r := range l.Runs {
		for _, g := range r.Glyphs {
			if g.Index == 1 {
				t.Fatalf("tab produced a glyph at index 1 (glyph id %d)", g.ID)
			}
		}
	}

	// Width: 'a' + tab-to-next-stop + 'b' — and decisively smaller than the
	// old broken notdef advance (~5em per tab).
	space := sys.LayoutLine(" 0", size, run).Width.AsF32() - sys.LayoutLine("0", size, run).Width.AsF32()
	oneChar := sys.LayoutLine("a", size, run).Width.AsF32()
	tabStop := TabWidthEm * space
	want := tabStop + oneChar // a(≈oneChar) -> stop, then b
	if l.Width.AsF32() > want+space || l.Width.AsF32() < tabStop {
		t.Fatalf("tabbed width %v, want ~%v (tab stop %v, char %v)", l.Width.AsF32(), want, tabStop, oneChar)
	}
	if l.Width.AsF32() > 4*size.AsF32() {
		t.Fatalf("tab advance still huge: width %v for \"a\\tb\" at %vpx (limit %v)", l.Width.AsF32(), size.AsF32(), 4*size.AsF32())
	}

	// A tab at pen 0 advances exactly one tab stop.
	l2 := sys.LayoutLine("\tx", size, []FontRun{{Len: 2, FontID: fontID}})
	if x := l2.XForIndex(1).AsF32(); x < tabStop-0.5 || x > tabStop+space {
		t.Fatalf("XForIndex after leading tab = %v, want ~%v", x, tabStop)
	}

	// Two consecutive tabs land on consecutive stops.
	l3 := sys.LayoutLine("\t\tx", size, []FontRun{{Len: 3, FontID: fontID}})
	if x := l3.XForIndex(2).AsF32(); x < 2*tabStop-0.5 || x > 2*tabStop+space {
		t.Fatalf("XForIndex after two tabs = %v, want ~%v", x, 2*tabStop)
	}
}
