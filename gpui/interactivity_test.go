package gpui_test

import (
	"testing"
	"time"

	"github.com/FlameInTheDark/go-gpui/app"
	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/elements"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/keymap"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/platform/headless"
	"github.com/FlameInTheDark/go-gpui/style"
)

// probeView is an inert view whose rendering is overridden by SetRootView in
// the harness below (same pattern as the e2e tests).
type probeView struct{}

func (probeView) Render(w *gpui.Window, cx *app.App) gpui.Element {
	return elements.NewDiv()
}

// openInteractivityWindow is the shared harness for the div interactivity
// tests: a headless window whose root is built by build (the headless
// OpenWindow + SetRootView + Draw pattern from e2e_test.go / drag_test.go).
func openInteractivityWindow(t *testing.T, width, height float32, build func(w *gpui.Window, cx *app.App) gpui.Element) *gpui.Window {
	t.Helper()
	hp := headless.New(geometry.SizePixels(width, height))
	appl := gpui.NewApplication(hp)
	handle, err := gpui.OpenWindow[probeView](appl, platform.WindowOptions{Title: "interactivity"}, func(w *gpui.Window, cx *app.Context[probeView]) probeView {
		return probeView{}
	})
	if err != nil {
		t.Fatalf("open window: %v", err)
	}
	w := handle.Window
	w.SetRootView(handle.View.ID(), build)
	w.Draw()
	return w
}

// box is a fixed-size colored div used as a hit target in the tests.
func box(name string, f func(d *elements.Div) *elements.Div) *elements.Div {
	return f(elements.NewDiv().
		ID(elements.ButtonID(name)).
		W(geometry.LengthFromPixels(geometry.Px(100))).
		H(geometry.LengthFromPixels(geometry.Px(50))).
		BgColor(gc.RGB(0x4488ff).Hsla()))
}

// TestDivOnHoverFiresOnEnterAndLeave drives the hover pipeline (upstream
// on_hover): moving onto the element fires hovered=true exactly once, moving
// off fires hovered=false exactly once, and stationary moves and idle moves
// fire nothing.
func TestDivOnHoverFiresOnEnterAndLeave(t *testing.T) {
	var events []bool
	w := openInteractivityWindow(t, 300, 200, func(w *gpui.Window, cx *app.App) gpui.Element {
		return elements.NewDiv().P_4().Child(
			box("hover-target", func(d *elements.Div) *elements.Div {
				return d.OnHover(func(hovered bool, win *gpui.Window) {
					events = append(events, hovered)
				})
			}),
		)
	})

	// Idle move outside the element: no transition.
	w.HandleInput(platform.MouseMoveEvent{Position: geometry.PointPixels(200, 150)})
	if len(events) != 0 {
		t.Fatalf("hover fired before entering: %v", events)
	}
	// Enter (the box sits at (16,16) inside the padded root).
	w.HandleInput(platform.MouseMoveEvent{Position: geometry.PointPixels(50, 40)})
	if len(events) != 1 || !events[0] {
		t.Fatalf("enter: want [true], got %v", events)
	}
	// Stationary move inside: no new transition.
	w.HandleInput(platform.MouseMoveEvent{Position: geometry.PointPixels(60, 45)})
	if len(events) != 1 {
		t.Fatalf("stationary move re-fired hover: %v", events)
	}
	// Leave.
	w.HandleInput(platform.MouseMoveEvent{Position: geometry.PointPixels(250, 180)})
	if len(events) != 2 || events[1] {
		t.Fatalf("leave: want [true false], got %v", events)
	}
}

// TestDivTooltipShowsAfterDelay exercises the tooltip state machine (upstream
// active_tooltip: None -> WaitingForShow -> Visible): the tooltip stays
// hidden until the show delay elapses under a stationary hovering pointer,
// becomes visible on a later frame, and hides when the pointer leaves.
func TestDivTooltipShowsAfterDelay(t *testing.T) {
	painted := false
	w := openInteractivityWindow(t, 300, 200, func(w *gpui.Window, cx *app.App) gpui.Element {
		return elements.NewDiv().P_4().Child(
			box("tooltip-target", func(d *elements.Div) *elements.Div {
				return d.TooltipBuilder(func(*gpui.Window) gpui.Element {
					return elements.NewCanvas(nil, func(bounds geometry.Bounds[geometry.Pixels], _ *gpui.Window) {
						painted = bounds.Size.Width.AsF32() > 0 && bounds.Size.Height.AsF32() > 0
					}).W_8().H_4()
				}).TooltipShowDelay(50)
			}),
		)
	})

	if w.TooltipVisible() {
		t.Fatal("tooltip visible before any hover")
	}
	// Hover the element: arms the waiting state, tooltip not shown yet.
	w.HandleInput(platform.MouseMoveEvent{Position: geometry.PointPixels(50, 40)})
	if w.TooltipVisible() {
		t.Fatal("tooltip visible before the show delay elapsed")
	}
	// Advance past the delay and redraw: the tooltip surfaces.
	time.Sleep(130 * time.Millisecond)
	w.Draw()
	if !w.TooltipVisible() || !painted {
		t.Fatal("tooltip was not laid out and painted after the show delay")
	}
	// Leaving the element hides it again (exit listener + paint-time check).
	w.HandleInput(platform.MouseMoveEvent{Position: geometry.PointPixels(250, 180)})
	if w.TooltipVisible() {
		t.Fatal("tooltip stayed visible after the pointer left")
	}
}

func TestDivTooltipPaintsPillAroundLabel(t *testing.T) {
	w := openInteractivityWindow(t, 300, 200, func(w *gpui.Window, cx *app.App) gpui.Element {
		return elements.NewDiv().P_4().Child(box("tooltip-pill-target", func(d *elements.Div) *elements.Div {
			return d.Tooltip("Run Go package").TooltipShowDelay(20)
		}))
	})
	w.HandleInput(platform.MouseMoveEvent{Position: geometry.PointPixels(50, 40)})
	time.Sleep(60 * time.Millisecond)
	w.Draw()
	for _, q := range w.RenderedScene().Quads {
		if q.Background.Solid != gc.RGB(0x262b35).Hsla() {
			continue
		}
		if q.Bounds.Size.Width.AsF32() < 80 || q.Bounds.Size.Height.AsF32() < 24 || q.CornerRadii.TopLeft.AsF32() < 10 || q.BorderWidths.Top.AsF32() == 0 {
			t.Fatalf("tooltip background is not a padded, rounded pill: %+v", q)
		}
		for _, sprite := range w.RenderedScene().MonoSprites {
			x, y := sprite.Bounds.Origin.X-q.Bounds.Origin.X, sprite.Bounds.Origin.Y-q.Bounds.Origin.Y
			if sprite.Order >= q.Order && x >= 0 && x < q.Bounds.Size.Width && y >= 0 && y < q.Bounds.Size.Height {
				return
			}
		}
		t.Fatal("tooltip label did not paint inside its background")
	}
	t.Fatal("tooltip did not paint its background")
}

// TestDivDragAndDropDeliveredToTarget drives an in-app drag gesture (upstream
// on_drag/on_drop): a press on the source plus a move beyond the drag
// threshold starts the drag carrying the payload (with a feedback view), a
// sub-threshold move does not, and releasing over a compatible target
// delivers the payload exactly once — never firing the source's click and
// never reaching a target typed for a different payload.
func TestDivDragAndDropDeliveredToTarget(t *testing.T) {
	const payload = "tile-1"
	dragOverColor := gc.RGB(0xff5577).Hsla()
	var (
		started  int
		dropped  []any
		rejected []any
		clicks   int
	)
	w := openInteractivityWindow(t, 400, 200, func(w *gpui.Window, cx *app.App) gpui.Element {
		source := box("drag-source", func(d *elements.Div) *elements.Div {
			return d.
				OnDragStart(func(win *gpui.Window) any { return payload }).
				OnDragStarted(func(d *gpui.DragState, win *gpui.Window) { started++ }).
				OnClick(func(ev gpui.ClickEvent, win *gpui.Window) { clicks++ })
		})
		target := box("drop-target", func(d *elements.Div) *elements.Div {
			return d.
				OnDropType("string", func(p any, win *gpui.Window) { dropped = append(dropped, p) }).
				DragOverStyle(func(s *style.StyleRefinement) {
					s.BgColor(dragOverColor)
				})
		})
		other := box("other-target", func(d *elements.Div) *elements.Div {
			return d.OnDropType("int", func(p any, win *gpui.Window) { rejected = append(rejected, p) })
		})
		// Flex row: source (0..100), target (100..200), other (200..300).
		return elements.NewDiv().Flex().Child(source).Child(target).Child(other)
	})

	// Press on the source (strictly inside its (0,0)-(100,50) bounds).
	w.HandleInput(platform.MouseDownEvent{Button: platform.ButtonLeft, Position: geometry.PointPixels(50, 40), ClickCount: 1})
	// Sub-threshold move: no drag yet.
	w.HandleInput(platform.MouseMoveEvent{Position: geometry.PointPixels(51, 40)})
	if w.HasActiveDrag() {
		t.Fatal("drag started below the threshold")
	}
	if started != 0 {
		t.Fatalf("drag-started callback fired below the threshold: %d", started)
	}
	// Move past the threshold (the box is 100x50, so stay inside it).
	w.HandleInput(platform.MouseMoveEvent{Position: geometry.PointPixels(56, 44)})
	if !w.HasActiveDrag() {
		t.Fatal("drag did not start after moving past the threshold")
	}
	if started != 1 {
		t.Fatalf("drag start fired %d times, want 1", started)
	}
	drag := w.ActiveDrag()
	if drag == nil || drag.Payload != payload {
		t.Fatalf("drag payload: got %+v, want %q", drag, payload)
	}
	if drag.View == nil {
		t.Fatal("drag feedback view missing")
	}
	// Hover the compatible target: drag-over is live, nothing delivered yet.
	w.HandleInput(platform.MouseMoveEvent{Position: geometry.PointPixels(150, 40)})
	if len(dropped) != 0 {
		t.Fatalf("drop delivered before release: %v", dropped)
	}
	// The drag-over style paints the target with the highlight color while
	// the compatible drag hovers it (upstream drag_over styles).
	if !paintedSolidQuad(w, dragOverColor) {
		t.Fatal("drag-over style was not painted on the hovered target")
	}
	// Release over the target: payload delivered once, drag consumed.
	w.HandleInput(platform.MouseUpEvent{Button: platform.ButtonLeft, Position: geometry.PointPixels(150, 40), ClickCount: 1})
	if len(dropped) != 1 || dropped[0] != payload {
		t.Fatalf("drop payload: got %v, want [%q]", dropped, payload)
	}
	if w.HasActiveDrag() {
		t.Fatal("drag survived the drop")
	}
	// The gesture never completed a click on the source.
	if clicks != 0 {
		t.Fatalf("click fired after a drag: %d", clicks)
	}
	// The int-typed target never saw the string payload.
	if len(rejected) != 0 {
		t.Fatalf("incompatible drop target received the payload: %v", rejected)
	}
	// And the drag-over highlight is gone once the gesture ended.
	if paintedSolidQuad(w, dragOverColor) {
		t.Fatal("drag-over style persisted after the drop")
	}
}

// paintedSolidQuad reports whether the last frame painted a solid quad of
// the given color (used to observe style refinements from tests).
func paintedSolidQuad(w *gpui.Window, color gc.Hsla) bool {
	for _, q := range w.PaintedQuads() {
		if q.Background.Tag == gc.BackgroundTagSolid && q.Background.Solid == color {
			return true
		}
	}
	return false
}

// TestDivOnMouseDownOutFiresForOutsidePresses checks the dismiss pipeline
// (upstream on_mouse_down_out): a press landing outside the element fires the
// handler with the press position — for any button, and over empty space or
// other elements alike — while a press inside does not.
func TestDivOnMouseDownOutFiresForOutsidePresses(t *testing.T) {
	var outEvents []geometry.Point[geometry.Pixels]
	w := openInteractivityWindow(t, 400, 200, func(w *gpui.Window, cx *app.App) gpui.Element {
		watcher := box("watcher", func(d *elements.Div) *elements.Div {
			return d.OnMouseDownOut(func(ev platform.MouseDownEvent, win *gpui.Window) {
				outEvents = append(outEvents, ev.Position)
			})
		})
		// Watcher occupies (0,0)-(100,50).
		return elements.NewDiv().Child(watcher)
	})

	// Press inside the watcher: no down-out.
	w.HandleInput(platform.MouseDownEvent{Button: platform.ButtonLeft, Position: geometry.PointPixels(50, 25), ClickCount: 1})
	w.HandleInput(platform.MouseUpEvent{Button: platform.ButtonLeft, Position: geometry.PointPixels(50, 25), ClickCount: 1})
	if len(outEvents) != 0 {
		t.Fatalf("mouse-down-out fired for an inside press: %v", outEvents)
	}
	// Press on empty space outside the watcher: fires with the press position.
	w.HandleInput(platform.MouseDownEvent{Button: platform.ButtonLeft, Position: geometry.PointPixels(300, 150), ClickCount: 1})
	w.HandleInput(platform.MouseUpEvent{Button: platform.ButtonLeft, Position: geometry.PointPixels(300, 150), ClickCount: 1})
	if len(outEvents) != 1 || outEvents[0].X.AsF32() != 300 || outEvents[0].Y.AsF32() != 150 {
		t.Fatalf("mouse-down-out events: got %v, want one press at (300,150)", outEvents)
	}
	// A press of another button elsewhere also counts as outside.
	w.HandleInput(platform.MouseDownEvent{Button: platform.ButtonRight, Position: geometry.PointPixels(250, 100), ClickCount: 1})
	w.HandleInput(platform.MouseUpEvent{Button: platform.ButtonRight, Position: geometry.PointPixels(250, 100), ClickCount: 1})
	if len(outEvents) != 2 {
		t.Fatalf("mouse-down-out did not fire for the second outside press: %v", outEvents)
	}
}

// TestDivKeyboardClickOnFocusedElement verifies the keyboard click path
// (upstream keyboard-invoked clicks on stateful divs): Enter and Space click
// a focused clickable div, other keys and modified keys do not, and a
// blurred element is not clicked.
func TestDivKeyboardClickOnFocusedElement(t *testing.T) {
	var clicks int
	fh := gpui.NewFocusHandle()
	w := openInteractivityWindow(t, 300, 200, func(w *gpui.Window, cx *app.App) gpui.Element {
		return elements.NewDiv().P_4().Child(
			box("kb-button", func(d *elements.Div) *elements.Div {
				return d.
					TrackFocus(fh).
					OnClick(func(ev gpui.ClickEvent, win *gpui.Window) { clicks++ })
			}),
		)
	})
	// The explicit handle is shared with the element, so focusing it makes
	// the element's click handler keyboard-reachable.
	w.Focus(fh)

	// Enter clicks.
	w.HandleInput(platform.KeyDownEvent{Keystroke: keymap.Keystroke{Key: "enter"}})
	if clicks != 1 {
		t.Fatalf("enter did not click the focused element (clicks=%d)", clicks)
	}
	// Space clicks too.
	w.HandleInput(platform.KeyDownEvent{Keystroke: keymap.Keystroke{Key: "space"}})
	if clicks != 2 {
		t.Fatalf("space did not click the focused element (clicks=%d)", clicks)
	}
	// Other keys do not.
	w.HandleInput(platform.KeyDownEvent{Keystroke: keymap.Keystroke{Key: "a"}})
	if clicks != 2 {
		t.Fatalf("unrelated key clicked the element (clicks=%d)", clicks)
	}
	// Modified enter does not (upstream requires bare enter/space).
	w.HandleInput(platform.KeyDownEvent{Keystroke: keymap.Keystroke{Modifiers: keymap.Modifiers{Control: true}, Key: "enter"}})
	if clicks != 2 {
		t.Fatalf("modified enter clicked the element (clicks=%d)", clicks)
	}
	// Blur: keys no longer click.
	w.Blur()
	w.HandleInput(platform.KeyDownEvent{Keystroke: keymap.Keystroke{Key: "enter"}})
	if clicks != 2 {
		t.Fatalf("blurred element was clicked (clicks=%d)", clicks)
	}
}

// TestDivCursorStyleReachesWindow checks the cursor pipeline (upstream
// mouse_cursor + window cursor resolution): a hovered element's cursor
// request wins for the frame and the window reports it through LastCursor;
// leaving falls back to the arrow.
func TestDivCursorStyleReachesWindow(t *testing.T) {
	w := openInteractivityWindow(t, 300, 200, func(w *gpui.Window, cx *app.App) gpui.Element {
		return elements.NewDiv().P_4().Child(
			box("cursor-target", func(d *elements.Div) *elements.Div {
				return d.Cursor(style.MouseCursorPointingHand)
			}),
		)
	})

	if w.LastCursor() != style.CursorArrow {
		t.Fatalf("initial cursor: got %v, want arrow", w.LastCursor())
	}
	// Hover the element: the pointing-hand request wins.
	w.HandleInput(platform.MouseMoveEvent{Position: geometry.PointPixels(50, 40)})
	if w.LastCursor() != style.CursorPointingHand {
		t.Fatalf("hovered cursor: got %v, want pointing hand", w.LastCursor())
	}
	// Leave: resolution falls back to the arrow.
	w.HandleInput(platform.MouseMoveEvent{Position: geometry.PointPixels(250, 180)})
	if w.LastCursor() != style.CursorArrow {
		t.Fatalf("unhovered cursor: got %v, want arrow", w.LastCursor())
	}
}
