package text

import (
	"fmt"
	"testing"
	"time"

	"github.com/FlameInTheDark/go-gpui/font"
	"github.com/FlameInTheDark/go-gpui/geometry"
)

// BenchmarkLayoutLineEditor measures shaping a typical editor line, the
// per-frame cost reported by the Windows editor (roughly one second of
// delay per keystroke).
func BenchmarkLayoutLineEditor(b *testing.B) {
	sys := NewSystem()
	if err := sys.LoadDir("/usr/share/fonts/truetype"); err != nil && len(sys.faces) == 0 {
		b.Skipf("no system fonts: %v", err)
	}
	fid, ok := sys.DefaultFontID()
	if !ok {
		b.Skip("no default font")
	}
	_ = fid
	line := "func (ws *Workspace) Render(handle *gpui.WindowHandle[Workspace], cx *app.App, w *gpui.Window) gpui.Element {"
	runs := []FontRun{{Len: len(line), FontID: 0}}
	size := geometry.Px(14)

	// Warm-up.
	_ = sys.LayoutLine(line, size, runs)

	start := time.Now()
	n := 0
	for i := 0; i < b.N; i++ {
		_ = sys.LayoutLine(line, size, runs)
		n++
	}
	el := time.Since(start)
	fmt.Printf("LayoutLine: %v/shape (%d shapes)\n", el/time.Duration(max(n, 1)), n)
}

// BenchmarkWorkspaceFrame estimates a full editor frame: 45 visible code
// lines + 45 gutter numbers shaped twice per frame (measure + prepaint),
// which is what the editor's RenderPane does today.
func BenchmarkWorkspaceFrame(b *testing.B) {
	sys := NewSystem()
	if err := sys.LoadDir("/usr/share/fonts/truetype"); err != nil && len(sys.faces) == 0 {
		b.Skipf("no system fonts: %v", err)
	}
	if _, ok := sys.DefaultFontID(); !ok {
		b.Skip("no default font")
	}
	lines := make([]string, 90)
	for i := range lines {
		lines[i] = fmt.Sprintf("\tws.scrolls = append(ws.scrolls, %d) // adjust the visible row", i)
	}
	runs := make([]FontRun, 0, 8)
	size := geometry.Px(14)

	start := time.Now()
	for i := 0; i < b.N; i++ {
		for _, l := range lines {
			runs = runs[:0]
			runs = append(runs, FontRun{Len: len(l), FontID: 0})
			_ = sys.LayoutLine(l, size, runs)
			_ = sys.LayoutLine(l, size, runs) // prepaint shapes again
		}
	}
	el := time.Since(start)
	fmt.Printf("frame (90 texts x 2 shapes): %v\n", el/time.Duration(max(b.N, 1)))
}

var _ = font.Font{}
