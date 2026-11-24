package gpui

import (
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/layout"
	"github.com/FlameInTheDark/go-gpui/style"
)

// LayoutStyleOf converts a gpui style.Style into a layout.Style, resolving
// rem lengths and snapping to device pixels like upstream taffy.rs
// (round_to_device_pixel for sizes, round_stroke for borders).
func LayoutStyleOf(st style.Style, remSize geometry.Pixels, scaleFactor float32) layout.Style {
	out := layout.DefaultStyle()
	switch st.Display {
	case style.DisplayBlock:
		out.Display = layout.DisplayBlock
	case style.DisplayFlex:
		out.Display = layout.DisplayFlex
	case style.DisplayNone:
		out.Display = layout.DisplayNone
	case style.DisplayGrid:
		out.Display = layout.DisplayFlex // grid not ported
	}
	if st.Position == style.PositionAbsolute {
		out.Position = layout.PositionAbsolute
	}
	out.Overflow = [2]layout.Overflow{
		overflowOf(st.Overflow[0]), overflowOf(st.Overflow[1]),
	}
	out.InsetSize = layout.SizeDimension{
		Width:  lengthDim(st.Size.Width, remSize, scaleFactor),
		Height: lengthDim(st.Size.Height, remSize, scaleFactor),
	}

	out.Inset = layout.EdgeDimension{
		Top:    lengthDim(st.Inset.Top, remSize, scaleFactor),
		Right:  lengthDim(st.Inset.Right, remSize, scaleFactor),
		Bottom: lengthDim(st.Inset.Bottom, remSize, scaleFactor),
		Left:   lengthDim(st.Inset.Left, remSize, scaleFactor),
	}
	out.Size = layout.SizeDimension{
		Width:  lengthDim(st.Size.Width, remSize, scaleFactor),
		Height: lengthDim(st.Size.Height, remSize, scaleFactor),
	}
	out.MinSize = layout.SizeDimension{
		Width:  lengthDim(st.MinSize.Width, remSize, scaleFactor),
		Height: lengthDim(st.MinSize.Height, remSize, scaleFactor),
	}
	out.MaxSize = layout.SizeDimension{
		Width:  lengthDim(st.MaxSize.Width, remSize, scaleFactor),
		Height: lengthDim(st.MaxSize.Height, remSize, scaleFactor),
	}
	out.Margin = layout.EdgeDimension{
		Top:    lengthDim(st.Margin.Top, remSize, scaleFactor),
		Right:  lengthDim(st.Margin.Right, remSize, scaleFactor),
		Bottom: lengthDim(st.Margin.Bottom, remSize, scaleFactor),
		Left:   lengthDim(st.Margin.Left, remSize, scaleFactor),
	}
	out.Padding = layout.EdgeDimension{
		Top:    definiteDim(st.Padding.Top, remSize, scaleFactor),
		Right:  definiteDim(st.Padding.Right, remSize, scaleFactor),
		Bottom: definiteDim(st.Padding.Bottom, remSize, scaleFactor),
		Left:   definiteDim(st.Padding.Left, remSize, scaleFactor),
	}
	out.Border = layout.EdgeDimension{
		Top:    strokeDim(st.BorderWidths.Top, remSize, scaleFactor),
		Right:  strokeDim(st.BorderWidths.Right, remSize, scaleFactor),
		Bottom: strokeDim(st.BorderWidths.Bottom, remSize, scaleFactor),
		Left:   strokeDim(st.BorderWidths.Left, remSize, scaleFactor),
	}
	out.FlexDirection = layout.FlexDirection(st.FlexDirection)
	out.FlexWrap = layout.FlexWrap(st.FlexWrap)
	out.FlexBasis = lengthDim(st.FlexBasis, remSize, scaleFactor)
	out.FlexGrow = st.FlexGrow
	out.FlexShrink = st.FlexShrink
	if st.AlignItems != nil {
		out.AlignItems = alignItemsPtr(*st.AlignItems)
	}
	if st.AlignSelf != nil {
		out.AlignSelf = alignItemsPtr(*st.AlignSelf)
	}
	if st.AlignContent != nil {
		out.AlignContent = alignContentPtr(*st.AlignContent)
	}
	if st.JustifyContent != nil {
		out.JustifyContent = alignContentPtr(*st.JustifyContent)
	}
	out.Gap = layout.SizeDimension{
		Width:  definiteDim(st.Gap.Width, remSize, scaleFactor),
		Height: definiteDim(st.Gap.Height, remSize, scaleFactor),
	}
	if st.AspectRatio != nil {
		ar := *st.AspectRatio
		out.AspectRatio = &ar
	}
	return out
}

func overflowOf(o style.Overflow) layout.Overflow {
	switch o {
	case style.OverflowClip:
		return layout.OverflowClip
	case style.OverflowHidden:
		return layout.OverflowHidden
	case style.OverflowScroll:
		return layout.OverflowScroll
	default:
		return layout.OverflowVisible
	}
}

// lengthDim converts a Length (px/rem/percent/auto) to a layout Dimension
// with rem resolution and device snapping.
func lengthDim(l geometry.Length, rem geometry.Pixels, scale float32) layout.Dimension {
	if l.IsAuto() {
		return layout.AutoDimension()
	}
	d, _ := l.Definite()
	return definiteDim(d, rem, scale)
}

// definiteDim converts a DefiniteLength with rem resolution and snapping.
func definiteDim(d geometry.DefiniteLength, rem geometry.Pixels, scale float32) layout.Dimension {
	if f, ok := d.Fraction(); ok {
		return layout.PercentDimension(f)
	}
	a, _ := d.Absolute()
	return layout.LengthDimension(snapDim(a.ToPixels(rem), scale))
}

// strokeDim snaps border widths (never below 1 device px when nonzero).
func strokeDim(a geometry.AbsoluteLength, rem geometry.Pixels, scale float32) layout.Dimension {
	px := a.ToPixels(rem)
	if px.AsF32() == 0 {
		return layout.LengthDimension(0)
	}
	return layout.LengthDimension(geometry.RoundStrokeToDevicePixel(px.AsF32(), scale))
}

func snapDim(px geometry.Pixels, scale float32) float32 {
	return geometry.RoundToDevicePixel(px.AsF32(), scale)
}

func alignItemsPtr(a style.AlignItems) *layout.AlignItems {
	v := layout.AlignItems(a)
	return &v
}

func alignContentPtr(a style.AlignContent) *layout.AlignContent {
	v := layout.AlignContent(a)
	return &v
}
