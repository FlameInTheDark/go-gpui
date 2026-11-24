package elements

import (
	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
)

// Tooltip is the small hover surface shown by Div.Tooltip (upstream tooltip
// views: a compact labeled panel painted above the whole tree near the
// cursor). It is a styled div, so the fluent style API applies; build custom
// tooltips either by styling this element or with Div.TooltipBuilder.
type Tooltip struct {
	*Div
}

// NewTooltip builds a compact pill with enough contrast to float over any
// editor surface. The radius rounds the ends of a one-line label but stays
// fixed for longer descriptions that wrap.
func NewTooltip(text string) *Tooltip {
	px := geometry.Px
	d := NewDiv().
		ID(gpui.NameElementId("tooltip")).
		MaxW(geometry.LengthFromPixels(px(360))).
		BgColor(gc.RGB(0x262b35).Hsla()).
		Border_1().
		BorderColor(gc.RGB(0x535a67).Hsla()).
		Rounded(geometry.PixelsLength(px(14))).
		ShadowMd().
		Px(geometry.PixelsDefinite(px(11))).
		Py(geometry.PixelsDefinite(px(5))).
		TextSize(geometry.PixelsLength(px(12))).
		LineHeight(geometry.PixelsDefinite(px(16))).
		Child(NewText(text).WithTextColor(gc.RGB(0xf3f5f8).Hsla()))
	return &Tooltip{Div: d}
}

// Prepaint registers the embedded div at the tooltip's layout node. Without
// this, only the wrapper has bounds and the div's background paints at zero
// size even though its text child has valid bounds.
func (t *Tooltip) Prepaint(w *gpui.Window, _ geometry.Bounds[geometry.Pixels]) {
	if node, ok := w.NodeOf(t); ok {
		w.SetNodeFor(t.Div, node)
		w.PrepaintElement(t.Div, node)
	}
}

func (t *Tooltip) Paint(w *gpui.Window) {
	w.PaintElement(t.Div)
}
