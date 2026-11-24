package elements

import (
	"testing"

	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/gpui"
)

// TestUniformListOverdraw verifies Overdraw(n) renders n extra items beyond
// the visible range on each side (upstream uniform_list with_overdraw) while
// VisibleRange keeps reporting the strictly visible range.
func TestUniformListOverdraw(t *testing.T) {
	var list *UniformList
	rendered := map[int]bool{}
	win, hw := openListWindow(t, 200, 100, func(w *gpui.Window) gpui.Element {
		if list == nil {
			list = NewUniformList(200, func(ix int, w *gpui.Window) gpui.Element {
				rendered[ix] = true
				return NewDiv().W(pxLen(200)).H(pxLen(20)).BgColor(gc.RGB(0x4488ff).Hsla())
			}).Overdraw(3)
		}
		return NewDiv().Flex().FlexCol().Child(list)
	})

	// Visible window: first 0, last 0 + 100/20 + 2 = 7 (the port's built-in
	// two-item bottom margin). VisibleRange must not include the overdraw.
	first, last := list.VisibleRange()
	if first != 0 || last != 7 {
		t.Fatalf("visible range = [%d,%d), want [0,7)", first, last)
	}
	// Overdraw extends the prepainted range to [0-3, 7+3) clamped to the
	// item count: items 0..9.
	for i := 0; i < 10; i++ {
		if !rendered[i] {
			t.Fatalf("item %d not rendered with Overdraw(3)", i)
		}
	}
	if rendered[10] {
		t.Fatal("item 10 rendered beyond the overdraw range")
	}

	// Scrolled into the middle: overdraw extends both sides of the visible
	// window.
	dispatchScroll(t, hw, 100, 50, -6) // 120px -> first = 6, last = 13
	handleDraw(win)
	first, last = list.VisibleRange()
	if first != 6 || last != 13 {
		t.Fatalf("scrolled visible range = [%d,%d), want [6,13)", first, last)
	}
	for i := 3; i < 16; i++ {
		if !rendered[i] {
			t.Fatalf("item %d not rendered in the overdraw window", i)
		}
	}
	if rendered[16] {
		t.Fatal("item 16 rendered beyond the overdraw range")
	}
}

// TestUniformListDefaultRangeUnchanged locks in that the default (no
// Overdraw) rendering stays exactly the visible window plus the built-in
// two-item bottom margin — the existing API is untouched.
func TestUniformListDefaultRangeUnchanged(t *testing.T) {
	var list *UniformList
	rendered := map[int]bool{}
	openListWindow(t, 200, 100, func(w *gpui.Window) gpui.Element {
		if list == nil {
			list = NewUniformList(200, func(ix int, w *gpui.Window) gpui.Element {
				rendered[ix] = true
				return NewDiv().W(pxLen(200)).H(pxLen(20))
			})
		}
		return NewDiv().Flex().FlexCol().Child(list)
	})

	first, last := list.VisibleRange()
	if first != 0 || last != 7 {
		t.Fatalf("visible range = [%d,%d), want [0,7)", first, last)
	}
	for i := 0; i < 7; i++ {
		if !rendered[i] {
			t.Fatalf("item %d not rendered", i)
		}
	}
	if rendered[7] {
		t.Fatal("item 7 rendered without Overdraw")
	}
}
