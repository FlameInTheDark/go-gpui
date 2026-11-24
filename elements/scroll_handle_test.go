package elements

import (
	"testing"

	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/platform"
)

// scrollTestWindow builds a window whose root is a 200x150 scrollable div
// with 10 stacked 100x100 children (content height 1000) tracked by the
// handle.
func scrollTestWindow(t *testing.T, handle *ScrollHandle) *gpui.Window {
	t.Helper()
	return openAnchoredWindow(t, 200, 150, func(w *gpui.Window) gpui.Element {
		container := NewDiv().
			ID(gpui.NameElementId("scroller")).
			TrackScroll(handle).
			FlexCol().
			OverflowYScroll()
		for i := 0; i < 10; i++ {
			container.Child(NewDiv().
				ID(gpui.NameElementId("row")).
				W(pxLen(100)).H(pxLen(100)).
				BgColor(gc.RGB(0x3366aa).Hsla()))
		}
		return container
	})
}

// TestScrollHandleMirrorsOffset verifies TrackScroll: prepaint mirrors the
// div's offset, content and viewport sizes into the handle.
func TestScrollHandleMirrorsOffset(t *testing.T) {
	handle := NewScrollHandle()
	w := scrollTestWindow(t, handle)

	if got := handle.ContentSize().Height.AsF32(); got < 999 || got > 1001 {
		t.Fatalf("content height = %v, want ~1000", got)
	}
	if got := handle.ViewportSize().Height.AsF32(); got < 149 || got > 151 {
		t.Fatalf("viewport height = %v, want ~150", got)
	}
	if handle.Top().AsF32() != 0 {
		t.Fatalf("initial top = %v, want 0", handle.Top().AsF32())
	}
	// Scroll down 100px via a wheel event in the middle of the viewport
	// (positive wheel delta scrolls up; negative scrolls down).
	center := geometry.PointPixels(100, 75)
	w.HandleInput(platform.ScrollEvent{
		Position: center,
		Delta:    platform.ScrollDeltaPixels(0, geometry.Px(-100)),
	})
	w.Draw()
	if top := handle.Top().AsF32(); top < 99 || top > 101 {
		t.Fatalf("top after wheel = %v, want ~100", top)
	}
}

// TestScrollHandleSetTopAndBottom verifies programmatic scrolling: SetTop
// applies on the next prepaint and clamps to the content extent.
func TestScrollHandleSetTopAndBottom(t *testing.T) {
	handle := NewScrollHandle()
	w := scrollTestWindow(t, handle)

	handle.SetTop(geometry.Px(300))
	w.Draw()
	if top := handle.Top().AsF32(); top < 299 || top > 301 {
		t.Fatalf("top after SetTop(300) = %v, want ~300", top)
	}

	handle.ScrollToBottom()
	w.Draw()
	max := handle.MaxTop().AsF32()
	if max < 849 || max > 851 {
		t.Fatalf("max top = %v, want ~850", max)
	}
	if top := handle.Top().AsF32(); top < max-1 || top > max+1 {
		t.Fatalf("top after ScrollToBottom = %v, want ~%v", top, max)
	}

	handle.ScrollToTop()
	w.Draw()
	if handle.Top().AsF32() != 0 {
		t.Fatalf("top after ScrollToTop = %v, want 0", handle.Top().AsF32())
	}
}

// TestScrollHandleAutoscrollFollowsBottom verifies the autoscroll contract:
// while following, every prepaint keeps the offset pinned to the newest
// content bottom; user scroll-up cancels following.
func TestScrollHandleAutoscrollFollowsBottom(t *testing.T) {
	handle := NewScrollHandle()
	w := scrollTestWindow(t, handle)

	handle.Autoscroll()
	w.Draw()
	max := handle.MaxTop().AsF32()
	if top := handle.Top().AsF32(); top < max-1 || top > max+1 {
		t.Fatalf("autoscroll top = %v, want ~%v", top, max)
	}
	if !handle.IsAutoscrolling() {
		t.Fatal("handle must keep following")
	}

	// User scrolls up (positive wheel delta): following cancels and the
	// viewport moves up 200px from the bottom.
	center := geometry.PointPixels(100, 75)
	w.HandleInput(platform.ScrollEvent{
		Position: center,
		Delta:    platform.ScrollDeltaPixels(0, geometry.Px(200)),
	})
	if handle.IsAutoscrolling() {
		t.Fatal("user scroll must cancel autoscroll")
	}
	w.Draw()
	if top := handle.Top().AsF32(); top > max-200 {
		t.Fatalf("top after user scroll-up = %v, want below %v", top, max-200)
	}
}
