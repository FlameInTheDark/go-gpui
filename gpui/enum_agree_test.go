package gpui_test

import (
	"testing"

	"github.com/FlameInTheDark/go-gpui/app"
	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/elements"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/layout"
	"github.com/FlameInTheDark/go-gpui/style"
)

// TestStyleLayoutEnumsAgree locks the style <-> layout enum tables
// together. LayoutStyleOf converts several style enums with raw numeric
// casts (layout.Display(st.Display), alignItemsPtr(st.AlignItems), ...), so
// the two packages' constants must carry the SAME values. style's
// AlignItems once put Baseline at 4 and Center at 5 while layout had
// Center=4/Baseline=5: every ItemsCenter element was laid out as baseline
// alignment (top-packed), which broke tab icons, menu rows and buttons
// while all tests kept passing (they never asserted vertical centering).
func TestStyleLayoutEnumsAgree(t *testing.T) {
	cases := []struct {
		name      string
		styleVal  uint8
		layoutVal uint8
	}{
		// Raw-cast conversions only (LayoutStyleOf does layout.X(st.X) with
		// no switch). Display, Position and Overflow are mapped explicitly
		// per-value there, so their numbering may differ between packages.
		//
		// FlexDirection.
		{"DirectionRow", uint8(style.FlexDirectionRow), uint8(layout.FlexRow)},
		{"DirectionColumn", uint8(style.FlexDirectionColumn), uint8(layout.FlexColumn)},
		{"DirectionRowReverse", uint8(style.FlexDirectionRowReverse), uint8(layout.FlexRowReverse)},
		{"DirectionColReverse", uint8(style.FlexDirectionColumnReverse), uint8(layout.FlexColumnReverse)},
		// FlexWrap.
		{"WrapNoWrap", uint8(style.FlexWrapNone), uint8(layout.FlexNoWrap)},
		{"WrapWrap", uint8(style.FlexWrapWrap), uint8(layout.FlexWrapOn)},
		{"WrapReverse", uint8(style.FlexWrapWrapReverse), uint8(layout.FlexWrapReverse)},
		// Overflow.
		{"OverflowVisible", uint8(style.OverflowVisible), uint8(layout.OverflowVisible)},
		{"OverflowClip", uint8(style.OverflowClip), uint8(layout.OverflowClip)},
		{"OverflowHidden", uint8(style.OverflowHidden), uint8(layout.OverflowHidden)},
		{"OverflowScroll", uint8(style.OverflowScroll), uint8(layout.OverflowScroll)},
		// AlignItems / AlignSelf: the swapped pair (Center=4, Baseline=5).
		{"AlignItemsStart", uint8(style.AlignItemsStart), uint8(layout.AlignStart)},
		{"AlignItemsEnd", uint8(style.AlignItemsEnd), uint8(layout.AlignEnd)},
		{"AlignItemsFlexStart", uint8(style.AlignItemsFlexStart), uint8(layout.AlignFlexStart)},
		{"AlignItemsFlexEnd", uint8(style.AlignItemsFlexEnd), uint8(layout.AlignFlexEnd)},
		{"AlignItemsCenter", uint8(style.AlignItemsCenter), uint8(layout.AlignCenter)},
		{"AlignItemsBaseline", uint8(style.AlignItemsBaseline), uint8(layout.AlignBaseline)},
		{"AlignItemsStretch", uint8(style.AlignItemsStretch), uint8(layout.AlignStretch)},
		// AlignContent.
		{"AlignContentStart", uint8(style.AlignContentStart), uint8(layout.AlignContentStart)},
		{"AlignContentCenter", uint8(style.AlignContentCenter), uint8(layout.AlignContentCenter)},
		{"AlignContentStretch", uint8(style.AlignContentStretch), uint8(layout.AlignContentStretch)},
		{"AlignContentSpaceBetween", uint8(style.AlignContentSpaceBetween), uint8(layout.AlignContentSpaceBetween)},
		{"AlignContentSpaceEvenly", uint8(style.AlignContentSpaceEvenly), uint8(layout.AlignContentSpaceEvenly)},
		{"AlignContentSpaceAround", uint8(style.AlignContentSpaceAround), uint8(layout.AlignContentSpaceAround)},
	}
	for _, c := range cases {
		if c.styleVal != c.layoutVal {
			t.Errorf("%s: style value %d != layout value %d (raw-cast conversion)",
				c.name, c.styleVal, c.layoutVal)
		}
	}
}

// TestItemsCenterCentersChildren is the end-to-end guard for the enum bug:
// a vertically centered child of an ItemsCenter flex row must land at the
// middle of the row, not at its top.
func TestItemsCenterCentersChildren(t *testing.T) {
	var child gpui.Element
	w := openInteractivityWindow(t, 200, 100, func(w *gpui.Window, cx *app.App) gpui.Element {
		row := elements.NewDiv().Flex().ItemsCenter().H(uiLen(34))
		child = elements.NewDiv().W(uiLen(14)).H(uiLen(14)).
			BgColor(gc.RGB(0x4488ff).Hsla())
		row = row.Child(child)
		return elements.NewDiv().Flex().FlexCol().Child(elements.NewDiv().H(uiLen(28))).Child(row)
	})

	b := w.ElementBounds(child)
	if got, want := b.Origin.Y.AsF32(), float32(28+(34-14)/2); got != want {
		t.Fatalf("child of ItemsCenter row at y=%.0f, want %.0f (centered)", got, want)
	}
}

func uiLen(v float32) geometry.Length {
	return geometry.LengthFromPixels(geometry.Px(v))
}
