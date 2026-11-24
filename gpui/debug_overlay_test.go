package gpui_test

import (
	"strings"
	"testing"

	"github.com/FlameInTheDark/go-gpui/app"
	"github.com/FlameInTheDark/go-gpui/elements"
	"github.com/FlameInTheDark/go-gpui/gpui"
)

// TestDebugOverlayPaintsStats verifies the debug HUD: enabling it paints
// text lines above the root (mono sprites appear), and the rolling
// window records frame statistics.
func TestDebugOverlayPaintsStats(t *testing.T) {
	w := openInteractivityWindow(t, 400, 300, func(w *gpui.Window, cx *app.App) gpui.Element {
		return elements.NewDiv()
	})
	spritesBefore := len(w.RenderedScene().MonoSprites)

	w.SetDebugOverlay(true)
	w.Draw()
	spritesAfter := len(w.RenderedScene().MonoSprites)
	if spritesAfter <= spritesBefore {
		t.Fatalf("debug overlay painted nothing (%d -> %d sprites)", spritesBefore, spritesAfter)
	}

	// Frame statistics accumulate.
	w.Draw()
	w.Draw()
	lines := w.DebugSummaryLines()
	if len(lines) == 0 {
		t.Fatal("no debug summary lines")
	}
	if !strings.Contains(lines[0], "fps") {
		t.Fatalf("summary line 0 = %q, want fps", lines[0])
	}
	if !strings.Contains(lines[2], "quads") {
		t.Fatalf("summary line 2 = %q, want quads", lines[2])
	}

	// Disabling removes the HUD text.
	w.SetDebugOverlay(false)
	w.Draw()
	if len(w.RenderedScene().MonoSprites) != spritesBefore {
		t.Fatalf("hud not removed after disable (%d vs %d)", len(w.RenderedScene().MonoSprites), spritesBefore)
	}
}
