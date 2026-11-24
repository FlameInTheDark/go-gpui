package gpui_test

import (
	"testing"

	"github.com/FlameInTheDark/go-gpui/app"
	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/elements"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/platform/headless"
)

// frameView renders a small interactive tree: a pane with a text child and
// listeners that only mark the window dirty (the post-fix handler pattern).
type frameView struct{}

func (f *frameView) Render(w *gpui.Window, cx *app.App) gpui.Element {
	return elements.NewDiv().
		ID(elements.ButtonID("frame-pane")).
		Flex().FlexCol().P_8().
		W(geometry.LengthFromPixels(geometry.Px(200))).
		H(geometry.LengthFromPixels(geometry.Px(200))).
		BgColor(gc.RGB(0x222233).Hsla()).
		OnMouseDown(func(ev platform.MouseDownEvent, win *gpui.Window) {
			win.Refresh()
		}).
		OnMouseMove(func(ev platform.MouseMoveEvent, win *gpui.Window) {
			win.Refresh()
		}).
		Child(elements.NewText("hello frames"))
}

func newFrameTestWindow(t *testing.T) (*gpui.Window, *headless.Platform) {
	t.Helper()
	hp := headless.New(geometry.SizePixels(200, 150))
	appl := gpui.NewApplication(hp)
	handle, err := gpui.OpenWindow[frameView](appl, platform.WindowOptions{Title: "frames"}, func(w *gpui.Window, cx *app.Context[frameView]) frameView {
		return frameView{}
	})
	if err != nil {
		t.Fatalf("open window: %v", err)
	}
	w := handle.Window
	w.SetRootView(handle.View.ID(), func(win *gpui.Window, cx *app.App) gpui.Element {
		return (&frameView{}).Render(win, cx)
	})
	return w, hp
}

// TestElementTableDoesNotLeakAcrossFrames locks in the per-frame reset of
// the element-to-node table. Every frame builds a fresh element tree, so a
// table that is never reset grows without bound; the editor degraded the
// longer it ran until the reset was restored.
func TestElementTableDoesNotLeakAcrossFrames(t *testing.T) {
	w, _ := newFrameTestWindow(t)
	w.Draw()
	first := w.ElementTableSize()
	if first == 0 {
		t.Fatal("element table empty after the first frame")
	}
	for i := 0; i < 50; i++ {
		w.Draw()
	}
	if got := w.ElementTableSize(); got != first {
		t.Fatalf("element table grew across frames: %d -> %d (per-frame element pointers leak)", first, got)
	}
}

// TestInputEventDrawsExactlyOneFrame locks in the one-frame-per-event
// invariant: a handler that only Refreshes must cost exactly one draw.
// Handlers drawing re-entrantly inside input dispatch wasted multiple full
// frames per event (perceived as laggy menus) and swapped hitboxes under
// the iterating dispatcher.
func TestInputEventDrawsExactlyOneFrame(t *testing.T) {
	w, _ := newFrameTestWindow(t)
	w.Draw()

	pos := geometry.PointPixels(100, 100)
	before := w.DrawCount()
	w.HandleInput(platform.MouseDownEvent{Position: pos, Button: platform.ButtonLeft, ClickCount: 1})
	if got := w.DrawCount() - before; got != 1 {
		t.Fatalf("mouse down cost %d frames, want exactly 1", got)
	}

	before = w.DrawCount()
	w.HandleInput(platform.MouseMoveEvent{Position: geometry.PointPixels(110, 110)})
	if got := w.DrawCount() - before; got != 1 {
		t.Fatalf("mouse move (listener fired) cost %d frames, want exactly 1", got)
	}

	// A move that fires no listener and changes no hover is free.
	before = w.DrawCount()
	w.HandleInput(platform.MouseMoveEvent{Position: geometry.PointPixels(110, 110)})
	if got := w.DrawCount() - before; got > 1 {
		t.Fatalf("idle mouse move cost %d frames, want at most 1", got)
	}
}
