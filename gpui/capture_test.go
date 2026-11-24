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

func elements2NewDiv() *elements.Div { return elements.NewDiv() }
func gcColor(v uint32) gc.Hsla       { return gc.RGB(v).Hsla() }

func btnP(b platform.MouseButton) *platform.MouseButton { return &b }

// TestPointerCaptureSuppressesAncestors verifies the scrollbar-drag
// contract: a press that captures the pointer keeps delivering moves to the
// capturing element even when the pointer leaves it, and the capturing
// element's StopPropagation suppresses the ancestor's drag handlers (the
// editor pane must not drag-select text while the user holds a scrollbar).
func TestPointerCaptureSuppressesAncestors(t *testing.T) {
	var paneMoved, railDown, railMoved, railUp bool

	w := openInteractivityWindow(t, 300, 200, func(w *gpui.Window, cx *app.App) gpui.Element {
		pane := elements2NewDiv().
			OnMouseMove(func(ev platform.MouseMoveEvent, w *gpui.Window) {
				if ev.Dragging() {
					paneMoved = true
				}
			})
		rail := elements2NewDiv().
			W(uiLen(20)).H(uiLen(100)).
			BgColor(gcColor(0x4488ff)).
			OnMouseDown(func(ev platform.MouseDownEvent, w *gpui.Window) {
				railDown = true
				if hb, ok := w.ListenerHitbox(); ok {
					w.CapturePointer(hb.ID)
				}
				w.StopPropagation()
			}).
			OnMouseMove(func(ev platform.MouseMoveEvent, w *gpui.Window) {
				railMoved = true
				w.StopPropagation()
			}).
			OnMouseUp(func(ev platform.MouseUpEvent, w *gpui.Window) {
				railUp = true
				w.ReleasePointer()
			})
		return pane.Child(rail)
	})

	// The rail is the first child: 20x100 at the window origin.
	press := geometry.PointPixels(10, 50)
	away := geometry.PointPixels(260, 30) // inside the pane, outside the rail

	w.HandleInput(platform.MouseDownEvent{Button: platform.ButtonLeft, Position: press})
	if !railDown {
		t.Fatal("rail did not receive the press")
	}
	w.HandleInput(platform.MouseMoveEvent{Position: away, PressedButton: btnP(platform.ButtonLeft)})
	if !railMoved {
		t.Fatal("captured rail did not receive the off-element move")
	}
	if paneMoved {
		t.Fatal("pane received the move although the rail captured and stopped it")
	}
	w.HandleInput(platform.MouseUpEvent{Button: platform.ButtonLeft, Position: away})
	if !railUp {
		t.Fatal("captured rail did not receive the release")
	}
}

// TestKeyedCaptureSurvivesStructuralRerender pins the tree-resize bug: the
// drag captures the grip, and each drag move re-renders with a CHANGED
// tree (the panel's scrollbar appears once the panel narrows — extra
// hitboxes before the grip renumber every later id). The capture must
// still resolve to the grip (keyed identity), the grip must keep
// receiving the moves, and rows under the cursor must not steal them.
func TestKeyedCaptureSurvivesStructuralRerender(t *testing.T) {
	var gripMoves int
	var rowMoved bool
	scrollbarVisible := false

	hp := headless.New(geometry.SizePixels(300, 200))
	appl := gpui.NewApplication(hp)
	handle, err := gpui.OpenWindow[probeView](appl, platform.WindowOptions{Title: "keyed-capture"}, func(w *gpui.Window, cx *app.Context[probeView]) probeView {
		return probeView{}
	})
	if err != nil {
		t.Fatalf("open window: %v", err)
	}
	w := handle.Window

	build := func(w *gpui.Window, cx *app.App) gpui.Element {
		panel := elements2NewDiv().W(uiLen(120)).H(uiLen(100))
		// A row underneath where the cursor travels while resizing.
		row := elements.NewDiv().
			ID(elements.ButtonID("tree-row")).
			W(uiLen(110)).H(uiLen(90)).
			BgColor(gcColor(0x4488ff)).
			OnMouseMove(func(ev platform.MouseMoveEvent, w *gpui.Window) {
				if ev.Dragging() {
					rowMoved = true
				}
			})
		panel = panel.Child(row)
		// The conditional scrollbar: appearing mid-drag shifts the hitbox
		// ids of everything registered after it (the grip).
		if scrollbarVisible {
			panel = panel.Child(elements.NewDiv().
				ID(elements.ButtonID("tree-vscroll")).
				Absolute().Right(uiLen(0)).Top(uiLen(0)).Bottom(uiLen(0)).
				W(uiLen(4)).
				BgColor(gcColor(0x888888)))
		}
		// The resize grip rides the panel's right edge; dragging it flips
		// the scrollbar into existence (a structural change every move).
		grip := elements.NewDiv().
			ID(elements.ButtonID("tree-resize-grip")).
			Absolute().Right(uiLen(0)).Top(uiLen(0)).Bottom(uiLen(0)).
			W(uiLen(6)).
			BgColor(gcColor(0xff8800)).
			OnMouseDown(func(ev platform.MouseDownEvent, w *gpui.Window) {
				if ev.Button != platform.ButtonLeft {
					return
				}
				if hb, ok := w.ListenerHitbox(); ok {
					w.CapturePointer(hb.ID)
				}
				w.StopPropagation()
			}).
			OnMouseMove(func(ev platform.MouseMoveEvent, w *gpui.Window) {
				if !ev.Dragging() {
					return
				}
				gripMoves++
				scrollbarVisible = true // the re-render changes the tree
				w.Refresh()
				w.StopPropagation()
			}).
			OnMouseUp(func(ev platform.MouseUpEvent, w *gpui.Window) {
				w.ReleasePointer()
			})
		return panel.Child(grip)
	}

	w.SetRootView(handle.View.ID(), build)
	w.Draw()
	press := geometry.PointPixels(116, 50) // on the grip (panel right edge)
	w.HandleInput(platform.MouseDownEvent{Button: platform.ButtonLeft, Position: press})
	w.Draw()

	// Drag across the row's area (x < 110 puts the cursor over the row):
	// the grip must keep the gesture despite the re-render that added the
	// scrollbar and renumbered the hitboxes.
	for _, x := range []float32{100, 90, 80} {
		w.HandleInput(platform.MouseMoveEvent{
			Position:      geometry.PointPixels(x, 50),
			PressedButton: btnP(platform.ButtonLeft),
		})
		w.Draw()
	}
	if gripMoves != 3 {
		t.Fatalf("grip received %d drag moves, want 3 (capture dropped by structural re-render)", gripMoves)
	}
	if rowMoved {
		t.Fatal("row under the cursor received drag moves during the captured resize")
	}
	w.HandleInput(platform.MouseUpEvent{Button: platform.ButtonLeft, Position: geometry.PointPixels(80, 50)})

	// After the gesture ends, ordinary clicks must still work: the
	// auto-release on mouse-up (and the post-draw validation) keeps a
	// stale capture from silencing the next press.
	var rowClicked bool
	w.SetRootView(handle.View.ID(), func(w *gpui.Window, cx *app.App) gpui.Element {
		return elements2NewDiv().
			ID(elements.ButtonID("tree-row")).
			W(uiLen(110)).H(uiLen(90)).
			BgColor(gcColor(0x4488ff)).
			OnClick(func(ev gpui.ClickEvent, w *gpui.Window) { rowClicked = true })
	})
	w.Draw()
	w.HandleInput(platform.MouseDownEvent{Button: platform.ButtonLeft, Position: geometry.PointPixels(50, 40)})
	w.HandleInput(platform.MouseUpEvent{Button: platform.ButtonLeft, Position: geometry.PointPixels(50, 40)})
	if !rowClicked {
		t.Fatal("click after a dropped capture did not reach the row (stale capture silenced input)")
	}
}
