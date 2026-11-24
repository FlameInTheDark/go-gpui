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

// dragView records whether move events report Dragging().
type dragView struct {
	moveDragging *bool
}

func (d *dragView) Render(w *gpui.Window, cx *app.App) gpui.Element {
	return elements.NewDiv().
		ID(elements.ButtonID("drag-pane")).
		W(geometry.LengthFromPixels(geometry.Px(200))).
		H(geometry.LengthFromPixels(geometry.Px(200))).
		BgColor(gc.RGB(0x222233).Hsla()).
		OnMouseDown(func(ev platform.MouseDownEvent, win *gpui.Window) {}).
		OnMouseMove(func(ev platform.MouseMoveEvent, win *gpui.Window) {
			*d.moveDragging = ev.Dragging()
		})
}

// TestHandleInputStampsPressedButton verifies that the window tracks held
// buttons across down/up and stamps MouseMoveEvent.PressedButton, so
// Dragging() works on platforms whose move events carry no button state
// (win32) — the mechanism behind editor drag-to-select.
func TestHandleInputStampsPressedButton(t *testing.T) {
	hp := headless.New(geometry.SizePixels(400, 300))
	appl := gpui.NewApplication(hp)
	dragging := new(bool)
	handle, err := gpui.OpenWindow[dragView](appl, platform.WindowOptions{Title: "drag"}, func(w *gpui.Window, cx *app.Context[dragView]) dragView {
		return dragView{moveDragging: dragging}
	})
	if err != nil {
		t.Fatalf("open window: %v", err)
	}
	w := handle.Window
	w.SetRootView(handle.View.ID(), func(win *gpui.Window, cx *app.App) gpui.Element {
		return (&dragView{moveDragging: dragging}).Render(win, cx)
	})
	w.Draw()

	pos := geometry.PointPixels(100, 100)
	// Outside any press: not dragging.
	w.HandleInput(platform.MouseMoveEvent{Position: pos})
	if *dragging {
		t.Fatal("move without press reported Dragging")
	}
	// Press and move: dragging.
	w.HandleInput(platform.MouseDownEvent{Position: pos, Button: platform.ButtonLeft, ClickCount: 1})
	w.HandleInput(platform.MouseMoveEvent{Position: geometry.PointPixels(130, 130)})
	if !*dragging {
		t.Fatal("move while left button held did not report Dragging (PressedButton not stamped)")
	}
	// Release and move: not dragging again.
	w.HandleInput(platform.MouseUpEvent{Position: geometry.PointPixels(130, 130), Button: platform.ButtonLeft, ClickCount: 1})
	w.HandleInput(platform.MouseMoveEvent{Position: geometry.PointPixels(140, 140)})
	if *dragging {
		t.Fatal("move after release reported Dragging")
	}
}
