package elements

import (
	"testing"

	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/platform/headless"
	"github.com/FlameInTheDark/go-gpui/render"
)

// openUniformWindow mirrors openListWindow for the uniform list tests.
func openUniformWindow(t *testing.T, width, height float32, build func(w *gpui.Window) gpui.Element) (*gpui.Window, *headless.Window) {
	t.Helper()
	return openListWindow(t, width, height, build)
}

// TestUniformListScrollToCenter verifies ScrollToStrategy(Center) places the
// item in the middle of the viewport (upstream scroll_to_item with
// ScrollStrategy::Center): a 50-item list of 20px rows in a 100px viewport,
// centered on item 25 (top 500px), scrolls so the viewport spans
// [450, 550).
func TestUniformListScrollToCenter(t *testing.T) {
	var list *UniformList
	win, _ := openUniformWindow(t, 200, 100, func(w *gpui.Window) gpui.Element {
		if list == nil {
			list = NewUniformList(50, func(ix int, w *gpui.Window) gpui.Element {
				return listRow(ix, 20)
			})
		}
		return NewDiv().Flex().FlexCol().Child(list)
	})
	handleDrawUniform(win)
	list.ScrollToStrategy(25, ScrollCenter)
	handleDrawUniform(win)
	from, to := list.VisibleRange()
	if from < 19 || from > 24 {
		t.Fatalf("center scroll: first visible = %d, want ~22 (450/20)", from)
	}
	if to <= from {
		t.Fatalf("empty visible range")
	}
}

// TestUniformListScrollToNearestScrollsOnlyWhenHidden verifies Nearest does
// nothing when the item is visible (upstream ScrollStrategy::Nearest).
func TestUniformListScrollToNearestScrollsOnlyWhenHidden(t *testing.T) {
	var list *UniformList
	win, _ := openUniformWindow(t, 200, 100, func(w *gpui.Window) gpui.Element {
		if list == nil {
			list = NewUniformList(50, func(ix int, w *gpui.Window) gpui.Element {
				return listRow(ix, 20)
			})
		}
		return NewDiv().Flex().FlexCol().Child(list)
	})
	handleDrawUniform(win)
	// Item 1 is visible at the top; Nearest must not move the viewport.
	list.ScrollToStrategy(1, ScrollNearest)
	handleDrawUniform(win)
	from, _ := list.VisibleRange()
	if from != 0 {
		t.Fatalf("nearest on visible item scrolled: first = %d, want 0", from)
	}

	// Item 40 is far below; Nearest places it at the bottom.
	list.ScrollToStrategy(40, ScrollNearest)
	handleDrawUniform(win)
	from, to := list.VisibleRange()
	// Bottom placement: the viewport ends at the item's bottom edge
	// (40*20+20=820 -> viewport [720, 820] -> first item 36).
	if from < 33 || from > 38 {
		t.Fatalf("nearest below: first = %d, want ~36", from)
	}
	if to <= from {
		t.Fatalf("empty visible range")
	}
}

// stickyHeader is a UniformListDecoration painting a 20px header bar pinned
// to the top of the list bounds (the canonical decoration: sticky headers).
type stickyHeader struct{}

func (stickyHeader) Compute(visibleRange [2]int, bounds geometry.Bounds[geometry.Pixels], scrollOffsetY geometry.Pixels, itemHeight geometry.Pixels, itemCount int, w *gpui.Window) gpui.Element {
	header := NewDiv().BgColor(gc.Red())
	header.StyleRef().W(geometry.LengthFromDefinite(geometry.AbsoluteDefinite(geometry.PixelsLength(bounds.Size.Width))))
	header.StyleRef().H(geometry.LengthFromPixels(itemHeight))
	return header
}

// TestUniformListDecorationPaintsOverItems verifies decorations paint above
// the items and clip to the list bounds (upstream
// frame_state.decorations).
func TestUniformListDecorationPaintsOverItems(t *testing.T) {
	var list *UniformList
	win, _ := openUniformWindow(t, 200, 100, func(w *gpui.Window) gpui.Element {
		if list == nil {
			list = NewUniformList(50, func(ix int, w *gpui.Window) gpui.Element {
				return listRow(ix, 20)
			}).AddDecoration(stickyHeader{})
		}
		return NewDiv().Flex().FlexCol().Child(list)
	})
	handleDrawUniform(win)
	// The header quad (fully saturated red) must be in the scene above the
	// item quads.
	var found *render.Quad
	quads := win.PaintedQuads()
	red := gc.Red()
	for i := range quads {
		q := &quads[i]
		if solid, ok := q.Background.AsSolid(); ok && solid == red {
			found = q
			break
		}
	}
	if found == nil {
		t.Fatal("sticky header decoration painted no red quad")
	}
	if found.Bounds.Size.Height.AsF32() < 19 {
		t.Fatalf("header height = %v, want ~20", found.Bounds.Size.Height.AsF32())
	}
}

// TestUniformListYFlipped verifies the y-flipped rendering: the first
// visible row shows the LAST logical item, and the flipped scroll places
// logical item 0 at the top of the viewport (upstream y_flipped).
func TestUniformListYFlipped(t *testing.T) {
	rendered := map[int]int{} // logical item -> paint count
	var list *UniformList
	win, _ := openUniformWindow(t, 200, 100, func(w *gpui.Window) gpui.Element {
		if list == nil {
			list = NewUniformList(50, func(ix int, w *gpui.Window) gpui.Element {
				rendered[ix]++
				return listRow(ix, 20)
			}).YFlipped(true)
		}
		return NewDiv().Flex().FlexCol().Child(list)
	})
	handleDrawUniform(win)

	// Unscrolled flipped list: visual row 0 is logical item 49.
	from, to := list.VisibleRange()
	if from != 0 || to < 5 {
		t.Fatalf("visible range = [%d,%d), want [0,>=5)", from, to)
	}
	// The renderer must have been called for the flipped (last) items.
	sawLast := false
	for ix := 44; ix < 50; ix++ {
		if rendered[ix] > 0 {
			sawLast = true
		}
	}
	if !sawLast {
		t.Fatal("y-flipped list should render the last logical items first")
	}
	// Items after the measured sample must not render at the top of the
	// flipped list (item 0 is measured by RequestLayout; items 1.. are
	// only painted if the flip is wrong).
	for i := 1; i < 10; i++ {
		if rendered[i] > 0 {
			t.Fatal("y-flipped unscrolled list must not render the first logical items")
		}
	}

	// ScrollTo a low logical item: with the flip, logical 2 shows at visual
	// index 47; top placement scrolls it to the viewport top.
	for ix := range rendered {
		delete(rendered, ix)
	}
	list.ScrollToStrategy(2, ScrollTop)
	handleDrawUniform(win)
	sawLow := false
	for ix := 0; ix < 8; ix++ {
		if rendered[ix] > 0 {
			sawLow = true
		}
	}
	if !sawLow {
		t.Fatal("scrolling to a low logical item in a flipped list must render it")
	}
}

func handleDrawUniform(win *gpui.Window) { win.Draw() }
