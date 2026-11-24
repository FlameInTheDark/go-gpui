package elements

import (
	"testing"

	"github.com/FlameInTheDark/go-gpui/app"
	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/platform/headless"
	"github.com/FlameInTheDark/go-gpui/render"
)

// openListWindow opens a headless window and renders the element built by
// build; the element builder runs on every draw (view render semantics), so
// the list element is captured once by the caller and reused.
func openListWindow(t *testing.T, width, height float32, build func(w *gpui.Window) gpui.Element) (*gpui.Window, *headless.Window) {
	t.Helper()
	hp := headless.New(geometry.SizePixels(width, height))
	appl := gpui.NewApplication(hp)
	handle, err := gpui.OpenWindow[struct{}](appl, platform.WindowOptions{}, func(w *gpui.Window, cx *app.Context[struct{}]) struct{} {
		return struct{}{}
	})
	if err != nil {
		t.Fatalf("open window: %v", err)
	}
	handle.Window.SetRootView(handle.View.ID(), func(w *gpui.Window, cx *app.App) gpui.Element {
		return build(w)
	})
	// Two frames: the List sizes its leaf node from the cached item
	// measurements (measure-as-you-scroll), so the first frame warms the
	// cache and the second frame has the real geometry.
	handle.Window.Draw()
	handle.Window.Draw()
	return handle.Window, hp.LastWindow()
}

func listRow(ix int, h float32) gpui.Element {
	return NewDiv().H(geometry.LengthFromPixels(geometry.Px(h))).
		BgColor(gc.RGB(0x4488ff).Hsla())
}

// TestListFollowBottomPinsToNewItems verifies the follow-mode semantics
// (upstream list.rs FollowMode::Tail): with FollowBottom enabled the view
// stays pinned to the bottom when items extend the list, stops following
// when the user scrolls away, re-engages at the bottom, and pins again on
// further growth.
func TestListFollowBottomPinsToNewItems(t *testing.T) {
	var list *List
	win, hw := openListWindow(t, 200, 100, func(w *gpui.Window) gpui.Element {
		if list == nil {
			list = NewList(5, func(ix int, w *gpui.Window) gpui.Element {
				return listRow(ix, 20)
			}).FollowBottom()
		}
		return NewDiv().Flex().FlexCol().Child(list)
	})

	// Content fits the viewport: pinned, nothing to scroll.
	if top := list.LogicalScrollTop(); top != 0 {
		t.Fatalf("initial logical scroll top = %d, want 0", top)
	}
	if !list.IsFollowingTail() {
		t.Fatal("FollowBottom should start following immediately")
	}

	// Items extend the list: the view stays pinned to the bottom
	// (upstream layout() anchors the scroll at the end while following).
	list.SetItemCount(8)
	handleDraw(win)
	if top := list.LogicalScrollTop(); top != 3 {
		t.Fatalf("logical scroll top after growth = %d, want 3 (pinned to the bottom)", top)
	}
	if _, to := list.VisibleRange(); to != 8 {
		t.Fatalf("visible range end = %d, want 8 (last items rendered)", to)
	}

	// Scrolling away from the bottom stops tail-following (upstream
	// paint_scroll: delta.y > 0 stops following) ...
	dispatchScroll(t, hw, 100, 50, 3)
	handleDraw(win)
	if list.IsFollowingTail() {
		t.Fatal("scrolling up must stop tail-following")
	}
	if top := list.LogicalScrollTop(); top != 0 {
		t.Fatalf("logical scroll top after scrolling up = %d, want 0", top)
	}

	// ... and scrolling back to the bottom re-engages it (upstream layout():
	// scroll_offset + available_height >= total_height - 1).
	dispatchScroll(t, hw, 100, 50, -3)
	handleDraw(win)
	if !list.IsFollowingTail() {
		t.Fatal("returning to the bottom must re-engage tail-following")
	}

	// Further growth keeps the view pinned.
	list.SetItemCount(10)
	handleDraw(win)
	if top := list.LogicalScrollTop(); top != 5 {
		t.Fatalf("logical scroll top after second growth = %d, want 5", top)
	}
	if !list.IsFollowingTail() {
		t.Fatal("still following after growth")
	}
}

// TestListUnfollowStopsPinning verifies Unfollow disables the tail mode: a
// growing list no longer moves the view.
func TestListUnfollowStopsPinning(t *testing.T) {
	var list *List
	win, _ := openListWindow(t, 200, 100, func(w *gpui.Window) gpui.Element {
		if list == nil {
			list = NewList(8, func(ix int, w *gpui.Window) gpui.Element {
				return listRow(ix, 20)
			}).FollowBottom()
		}
		return NewDiv().Flex().FlexCol().Child(list)
	})
	if top := list.LogicalScrollTop(); top != 3 {
		t.Fatalf("pinned scroll top = %d, want 3", top)
	}

	list.Unfollow()
	list.SetItemCount(12)
	handleDraw(win)
	if list.IsFollowingTail() {
		t.Fatal("Unfollow must disable tail-following")
	}
	if top := list.LogicalScrollTop(); top != 3 {
		t.Fatalf("scroll top after Unfollow + growth = %d, want 3 (position kept)", top)
	}
}

// TestListOnScrollHandler verifies the scroll handler fires after each paint
// with the logical scroll top, the visible item count and the total item
// count (upstream ListState::set_scroll_handler / ListScrollEvent).
func TestListOnScrollHandler(t *testing.T) {
	var list *List
	calls := 0
	var lastTop, lastViewport, lastTotal int
	win, hw := openListWindow(t, 200, 100, func(w *gpui.Window) gpui.Element {
		if list == nil {
			list = NewList(30, func(ix int, w *gpui.Window) gpui.Element {
				return listRow(ix, 20)
			}).OnScrollHandler(func(top, viewport, total int) {
				calls++
				lastTop, lastViewport, lastTotal = top, viewport, total
			})
		}
		return NewDiv().Flex().FlexCol().Child(list)
	})

	if calls == 0 {
		t.Fatal("scroll handler must be called after each paint")
	}
	if lastTotal != 30 {
		t.Fatalf("handler total = %d, want 30", lastTotal)
	}
	if lastTop != 0 {
		t.Fatalf("handler top = %d, want 0", lastTop)
	}
	if lastViewport < 1 {
		t.Fatalf("handler viewport = %d, want at least 1 visible item", lastViewport)
	}

	// Scroll down 3 lines (60px): the handler reports the new logical top.
	dispatchScroll(t, hw, 100, 50, -3)
	handleDraw(win)
	if lastTop < 1 || lastTop > 5 {
		t.Fatalf("handler top after scroll = %d, want ~3", lastTop)
	}
	if calls < 2 {
		t.Fatalf("handler calls = %d, want one per paint", calls)
	}
}

// TestListScrollbarPaintsTrackAndThumb verifies WithScrollbar paints a thin
// track+thumb quad pair after the items, driven by the scroll offsets, and
// registers a grab-zone hitbox for interaction (upstream scrollbar drag).
func TestListScrollbarPaintsTrackAndThumb(t *testing.T) {
	var list *List
	win, hw := openListWindow(t, 200, 100, func(w *gpui.Window) gpui.Element {
		if list == nil {
			list = NewList(50, func(ix int, w *gpui.Window) gpui.Element {
				return listRow(ix, 20)
			}).WithScrollbar()
		}
		return NewDiv().Flex().FlexCol().Child(list)
	})

	b := win.ElementBounds(list)
	trackX := b.Right().AsF32() - 6
	track, thumb := findScrollbarQuads(t, win.PaintedQuads(), trackX)
	if track == nil {
		t.Fatal("scrollbar track quad not painted")
	}
	if got := track.Bounds.Size.Height.AsF32(); got < b.Size.Height.AsF32()-1 {
		t.Fatalf("track height = %v, want the viewport height %v", got, b.Size.Height.AsF32())
	}
	if thumb == nil {
		t.Fatal("scrollbar thumb quad not painted")
	}
	if thumb.Bounds.Size.Height.AsF32() >= b.Size.Height.AsF32() {
		t.Fatalf("thumb height %v must be smaller than the viewport", thumb.Bounds.Size.Height.AsF32())
	}
	thumbY := thumb.Bounds.Origin.Y.AsF32()

	// The scrollbar registers a grab-zone hitbox alongside the list's own.
	if n := len(win.RenderedHitboxes()); n != 2 {
		t.Fatalf("hitboxes = %d, want 2 (the list + the scrollbar grab zone)", n)
	}

	// Scrolling moves the thumb along the track.
	dispatchScroll(t, hw, 100, 50, -3)
	handleDraw(win)
	_, thumb2 := findScrollbarQuads(t, win.PaintedQuads(), trackX)
	if thumb2 == nil {
		t.Fatal("thumb quad missing after scroll")
	}
	if thumb2.Bounds.Origin.Y.AsF32() <= thumbY {
		t.Fatalf("thumb y = %v, want > %v after scrolling down", thumb2.Bounds.Origin.Y.AsF32(), thumbY)
	}
}

// TestListScrollbarDragScrolls verifies dragging the scrollbar thumb
// scrolls the list proportionally to the pointer travel (upstream
// set_offset_from_scrollbar): pressing the thumb, dragging to the bottom of
// the track and releasing lands the list at the bottom (logical top = the
// last page of items), and IsScrollbarDragging tracks the gesture.
func TestListScrollbarDragScrolls(t *testing.T) {
	var list *List
	win, hw := openListWindow(t, 200, 100, func(w *gpui.Window) gpui.Element {
		if list == nil {
			list = NewList(50, func(ix int, w *gpui.Window) gpui.Element {
				return listRow(ix, 20)
			}).WithScrollbar()
		}
		return NewDiv().Flex().FlexCol().Child(list)
	})

	b := win.ElementBounds(list)
	trackX := b.Right().AsF32() - 3 // center of the grab zone
	_, thumb := findScrollbarQuads(t, win.PaintedQuads(), b.Right().AsF32()-6)
	if thumb == nil {
		t.Fatal("thumb quad missing")
	}
	before := list.LogicalScrollTop()

	// Press inside the thumb.
	thumbCenterY := thumb.Bounds.Origin.Y.AsF32() + thumb.Bounds.Size.Height.AsF32()/2
	hw.DispatchInput(platform.MouseDownEvent{
		Button:   platform.ButtonLeft,
		Position: geometry.PointPixels(trackX, thumbCenterY),
	})
	handleDraw(win)
	if !list.IsScrollbarDragging() {
		t.Fatal("expected IsScrollbarDragging during the drag")
	}

	// Drag to the bottom of the track and release.
	hw.DispatchInput(platform.MouseMoveEvent{
		Position: geometry.PointPixels(trackX, b.Origin.Y.AsF32()+b.Size.Height.AsF32()-1),
	})
	handleDraw(win)
	hw.DispatchInput(platform.MouseUpEvent{
		Button:   platform.ButtonLeft,
		Position: geometry.PointPixels(trackX, b.Origin.Y.AsF32()+b.Size.Height.AsF32()-1),
	})
	handleDraw(win)

	if list.IsScrollbarDragging() {
		t.Fatal("drag must end on release")
	}
	after := list.LogicalScrollTop()
	if after <= before {
		t.Fatalf("dragging to the bottom must scroll down: before=%d after=%d", before, after)
	}
	// At the bottom of a 50x20px list in a 100px viewport, the logical top
	// is around item 45.
	if after < 40 || after > 50 {
		t.Fatalf("bottom drag should land near the last page, got %d", after)
	}
}

// handleDraw redraws the window built by openListWindow.
func handleDraw(win *gpui.Window) { win.Draw() }

// dispatchScroll sends a wheel event with the given number of line ticks.
func dispatchScroll(t *testing.T, hw *headless.Window, x, y, lines float32) {
	t.Helper()
	hw.DispatchInput(platform.ScrollEvent{
		Position: geometry.PointPixels(x, y),
		Delta:    platform.ScrollDeltaLines(0, lines),
	})
}

// findScrollbarQuads locates the 6px-wide track and thumb quads painted at
// the given x position. The track spans the full viewport height; the thumb
// is the shorter quad.
func findScrollbarQuads(t *testing.T, quads []render.Quad, x float32) (*render.Quad, *render.Quad) {
	t.Helper()
	var track, thumb *render.Quad
	for i := range quads {
		q := &quads[i]
		if q.Bounds.Size.Width.AsF32() != 6 {
			continue
		}
		if d := q.Bounds.Origin.X.AsF32() - x; d < -0.51 || d > 0.51 {
			continue
		}
		if q.Bounds.Size.Height.AsF32() >= 99 {
			track = q
		} else {
			thumb = q
		}
	}
	return track, thumb
}

// TestListLogicalScrollOffset verifies the full logical scroll anchor
// (upstream ListOffset{item_ix, offset_in_item}): scrolling by non-item
// amounts reports the item at the viewport top plus the pixels into it.
func TestListLogicalScrollOffset(t *testing.T) {
	var list *List
	win, hw := openListWindow(t, 200, 100, func(w *gpui.Window) gpui.Element {
		if list == nil {
			list = NewList(10, func(ix int, w *gpui.Window) gpui.Element {
				return listRow(ix, 20)
			})
		}
		return NewDiv().Flex().FlexCol().Child(list)
	})

	if off := list.LogicalScrollOffset(); off.ItemIx != 0 || off.OffsetInItem != 0 {
		t.Fatalf("initial offset = %+v, want {0 0}", off)
	}

	// Scroll half an item: item 0 at top with 10px scrolled into it.
	dispatchScroll(t, hw, 100, 50, -0.5)
	handleDraw(win)
	if off := list.LogicalScrollOffset(); off.ItemIx != 0 || absF(off.OffsetInItem-10) > 0.5 {
		t.Fatalf("offset after half-item scroll = %+v, want {0 10}", off)
	}

	// Scroll 2.5 items total: item 2 at top, 10px into it.
	dispatchScroll(t, hw, 100, 50, -2)
	handleDraw(win)
	if off := list.LogicalScrollOffset(); off.ItemIx != 2 || absF(off.OffsetInItem-10) > 0.5 {
		t.Fatalf("offset after 2.5 items = %+v, want {2 10}", off)
	}
	if top := list.LogicalScrollTop(); top != 2 {
		t.Fatalf("logical scroll top = %d, want 2", top)
	}

	// Scroll past the end: the position clamps to scrollMax, so the last
	// page starts at item 5 with nothing scrolled into it.
	dispatchScroll(t, hw, 100, 50, -50)
	handleDraw(win)
	if off := list.LogicalScrollOffset(); off.ItemIx != 5 || off.OffsetInItem != 0 {
		t.Fatalf("clamped offset = %+v, want {5 0}", off)
	}
	if _, to := list.VisibleRange(); to != 10 {
		t.Fatalf("visible range end = %d, want 10 (last item rendered)", to)
	}
}

func absF(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
