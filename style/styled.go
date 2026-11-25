package style

import (
	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/font"
	"github.com/FlameInTheDark/go-gpui/geometry"
)

// This file implements the Styled builder API: the Go equivalent of gpui's
// Styled trait. All methods mutate the refinement in place and return it for
// chaining. The numeric Tailwind-style scale methods (W_8, P_2, Gap_4, ...)
// are generated into styled_gen.go by cmd/genstyles.

// ---------------------------------------------------------------------------
// Display / visibility
// ---------------------------------------------------------------------------

// Block sets display to block: children stack vertically in normal flow (CSS
// display: block, Tailwind block).
func (r *StyleRefinement) Block() *StyleRefinement { d := DisplayBlock; r.Display = &d; return r }

// Flex sets display to flex (CSS display: flex, Tailwind flex). Flex is also
// the default display of [DefaultStyle]; the direction defaults to row.
func (r *StyleRefinement) Flex() *StyleRefinement { d := DisplayFlex; r.Display = &d; return r }

// Grid sets display to grid (CSS display: grid, Tailwind grid). Combine with
// [StyleRefinement.SetGridCols] and friends.
func (r *StyleRefinement) Grid() *StyleRefinement { d := DisplayGrid; r.Display = &d; return r }

// Hidden sets display to none: the element is skipped by layout and paint
// (Tailwind hidden). Use [StyleRefinement.Invisible] to keep the layout space.
func (r *StyleRefinement) Hidden() *StyleRefinement { d := DisplayNone; r.Display = &d; return r }

// Visible sets visibility to visible (Tailwind visible).
func (r *StyleRefinement) Visible() *StyleRefinement {
	v := VisibilityVisible
	r.Visibility = &v
	return r
}

// Invisible sets visibility to hidden: the element still takes up layout space
// but is not painted (Tailwind invisible).
func (r *StyleRefinement) Invisible() *StyleRefinement {
	v := VisibilityHidden
	r.Visibility = &v
	return r
}

// ---------------------------------------------------------------------------
// Sizes (param forms)
// ---------------------------------------------------------------------------

func (r *StyleRefinement) setW(l geometry.Length) *StyleRefinement {
	if r.Size == nil {
		r.Size = &SizeLengthRefinement{}
	}
	r.Size.Width = &l
	return r
}

func (r *StyleRefinement) setH(l geometry.Length) *StyleRefinement {
	if r.Size == nil {
		r.Size = &SizeLengthRefinement{}
	}
	r.Size.Height = &l
	return r
}

// W sets the width (CSS width). Length may be pixels, rems, a fraction of the
// parent or auto. See also the generated W_* scale methods.
func (r *StyleRefinement) W(l geometry.Length) *StyleRefinement { return r.setW(l) }

// H sets the height (CSS height). Length may be pixels, rems, a fraction of
// the parent or auto. See also the generated H_* scale methods.
func (r *StyleRefinement) H(l geometry.Length) *StyleRefinement { return r.setH(l) }

// SetSize sets the width to w and the height to h. Elements expose it as Size.
func (r *StyleRefinement) SetSize(w, h geometry.Length) *StyleRefinement {
	r.setW(w)
	return r.setH(h)
}

// SetSizeUniform sets both width and height to l (Tailwind size-*). The
// generated Size_* scale methods call it.
func (r *StyleRefinement) SetSizeUniform(l geometry.Length) *StyleRefinement {
	r.setW(l)
	return r.setH(l)
}

// MinW sets the minimum width (CSS min-width). An auto minimum lets the
// flex/grid algorithms derive one from the content.
func (r *StyleRefinement) MinW(l geometry.Length) *StyleRefinement {
	if r.MinSize == nil {
		r.MinSize = &SizeLengthRefinement{}
	}
	r.MinSize.Width = &l
	return r
}

// MinH sets the minimum height (CSS min-height). An auto minimum lets the
// flex/grid algorithms derive one from the content.
func (r *StyleRefinement) MinH(l geometry.Length) *StyleRefinement {
	if r.MinSize == nil {
		r.MinSize = &SizeLengthRefinement{}
	}
	r.MinSize.Height = &l
	return r
}

// MaxW sets the maximum width (CSS max-width). Auto means no maximum.
func (r *StyleRefinement) MaxW(l geometry.Length) *StyleRefinement {
	if r.MaxSize == nil {
		r.MaxSize = &SizeLengthRefinement{}
	}
	r.MaxSize.Width = &l
	return r
}

// MaxH sets the maximum height (CSS max-height). Auto means no maximum.
func (r *StyleRefinement) MaxH(l geometry.Length) *StyleRefinement {
	if r.MaxSize == nil {
		r.MaxSize = &SizeLengthRefinement{}
	}
	r.MaxSize.Height = &l
	return r
}

// SetAspectRatio sets the preferred width/height ratio (CSS aspect-ratio),
// e.g. 16.0/9.0. Elements expose it as AspectRatio.
func (r *StyleRefinement) SetAspectRatio(v float32) *StyleRefinement {
	r.AspectRatio = &v
	return r
}

// AspectSquare sets the aspect ratio to 1 (Tailwind aspect-square).
func (r *StyleRefinement) AspectSquare() *StyleRefinement {
	return r.SetAspectRatio(1.0)
}

// ---------------------------------------------------------------------------
// Padding / margin (param forms)
// ---------------------------------------------------------------------------

func (r *StyleRefinement) setPadTop(l geometry.DefiniteLength) *StyleRefinement {
	if r.Padding == nil {
		r.Padding = &EdgesDefiniteRefinement{}
	}
	r.Padding.Top = &l
	return r
}

func (r *StyleRefinement) setPadBottom(l geometry.DefiniteLength) *StyleRefinement {
	if r.Padding == nil {
		r.Padding = &EdgesDefiniteRefinement{}
	}
	r.Padding.Bottom = &l
	return r
}

func (r *StyleRefinement) setPadLeft(l geometry.DefiniteLength) *StyleRefinement {
	if r.Padding == nil {
		r.Padding = &EdgesDefiniteRefinement{}
	}
	r.Padding.Left = &l
	return r
}

func (r *StyleRefinement) setPadRight(l geometry.DefiniteLength) *StyleRefinement {
	if r.Padding == nil {
		r.Padding = &EdgesDefiniteRefinement{}
	}
	r.Padding.Right = &l
	return r
}

// P sets the padding on all four edges (CSS padding). Padding is a
// [geometry.DefiniteLength]: pixels, rems or a fraction of the parent size.
// See also P_* scale methods.
func (r *StyleRefinement) P(l geometry.DefiniteLength) *StyleRefinement {
	r.setPadTop(l)
	r.setPadRight(l)
	r.setPadBottom(l)
	return r.setPadLeft(l)
}

// Pt sets the top padding (CSS padding-top).
func (r *StyleRefinement) Pt(l geometry.DefiniteLength) *StyleRefinement { return r.setPadTop(l) }

// Pb sets the bottom padding (CSS padding-bottom).
func (r *StyleRefinement) Pb(l geometry.DefiniteLength) *StyleRefinement { return r.setPadBottom(l) }

// Pl sets the left padding (CSS padding-left).
func (r *StyleRefinement) Pl(l geometry.DefiniteLength) *StyleRefinement { return r.setPadLeft(l) }

// Pr sets the right padding (CSS padding-right).
func (r *StyleRefinement) Pr(l geometry.DefiniteLength) *StyleRefinement { return r.setPadRight(l) }

// Px sets the left and right padding (Tailwind px-*).
func (r *StyleRefinement) Px(l geometry.DefiniteLength) *StyleRefinement {
	r.setPadLeft(l)
	return r.setPadRight(l)
}

// Py sets the top and bottom padding (Tailwind py-*).
func (r *StyleRefinement) Py(l geometry.DefiniteLength) *StyleRefinement {
	r.setPadTop(l)
	return r.setPadBottom(l)
}

func (r *StyleRefinement) setMarTop(l geometry.Length) *StyleRefinement {
	if r.Margin == nil {
		r.Margin = &EdgesLengthRefinement{}
	}
	r.Margin.Top = &l
	return r
}

func (r *StyleRefinement) setMarBottom(l geometry.Length) *StyleRefinement {
	if r.Margin == nil {
		r.Margin = &EdgesLengthRefinement{}
	}
	r.Margin.Bottom = &l
	return r
}

func (r *StyleRefinement) setMarLeft(l geometry.Length) *StyleRefinement {
	if r.Margin == nil {
		r.Margin = &EdgesLengthRefinement{}
	}
	r.Margin.Left = &l
	return r
}

func (r *StyleRefinement) setMarRight(l geometry.Length) *StyleRefinement {
	if r.Margin == nil {
		r.Margin = &EdgesLengthRefinement{}
	}
	r.Margin.Right = &l
	return r
}

// M sets the margin on all four edges (CSS margin). Margins are a
// [geometry.Length], so auto is allowed. See also M_* scale methods.
func (r *StyleRefinement) M(l geometry.Length) *StyleRefinement {
	r.setMarTop(l)
	r.setMarRight(l)
	r.setMarBottom(l)
	return r.setMarLeft(l)
}

// Mt sets the top margin (CSS margin-top).
func (r *StyleRefinement) Mt(l geometry.Length) *StyleRefinement { return r.setMarTop(l) }

// Mb sets the bottom margin (CSS margin-bottom).
func (r *StyleRefinement) Mb(l geometry.Length) *StyleRefinement { return r.setMarBottom(l) }

// Ml sets the left margin (CSS margin-left).
func (r *StyleRefinement) Ml(l geometry.Length) *StyleRefinement { return r.setMarLeft(l) }

// Mr sets the right margin (CSS margin-right).
func (r *StyleRefinement) Mr(l geometry.Length) *StyleRefinement { return r.setMarRight(l) }

// Mx sets the left and right margin (Tailwind mx-*).
func (r *StyleRefinement) Mx(l geometry.Length) *StyleRefinement {
	r.setMarLeft(l)
	return r.setMarRight(l)
}

// My sets the top and bottom margin (Tailwind my-*).
func (r *StyleRefinement) My(l geometry.Length) *StyleRefinement {
	r.setMarTop(l)
	return r.setMarBottom(l)
}

// MxAuto sets the left and right margins to auto, which centers a fixed-width
// element in its container (Tailwind mx-auto).
func (r *StyleRefinement) MxAuto() *StyleRefinement { return r.Mx(geometry.AutoLength()) }

// MlAuto sets the left margin to auto, pushing the element to the right
// (Tailwind ml-auto).
func (r *StyleRefinement) MlAuto() *StyleRefinement { return r.Ml(geometry.AutoLength()) }

// MrAuto sets the right margin to auto, pushing the element to the left
// (Tailwind mr-auto).
func (r *StyleRefinement) MrAuto() *StyleRefinement { return r.Mr(geometry.AutoLength()) }

// ---------------------------------------------------------------------------
// Gap
// ---------------------------------------------------------------------------

func (r *StyleRefinement) setGapW(l geometry.DefiniteLength) *StyleRefinement {
	if r.Gap == nil {
		r.Gap = &SizeDefiniteRefinement{}
	}
	r.Gap.Width = &l
	return r
}

func (r *StyleRefinement) setGapH(l geometry.DefiniteLength) *StyleRefinement {
	if r.Gap == nil {
		r.Gap = &SizeDefiniteRefinement{}
	}
	r.Gap.Height = &l
	return r
}

// SetGap sets both the column gap and the row gap (CSS gap). Elements expose
// it as Gap. The gap applies between flex/grid children, never at the outer
// edges.
func (r *StyleRefinement) SetGap(l geometry.DefiniteLength) *StyleRefinement {
	r.setGapW(l)
	return r.setGapH(l)
}

// SetGapX sets the column gap, the space between horizontally adjacent
// children (CSS column-gap). Elements expose it as GapX.
func (r *StyleRefinement) SetGapX(l geometry.DefiniteLength) *StyleRefinement { return r.setGapW(l) }

// SetGapY sets the row gap, the space between vertically adjacent children
// (CSS row-gap). Elements expose it as GapY.
func (r *StyleRefinement) SetGapY(l geometry.DefiniteLength) *StyleRefinement { return r.setGapH(l) }

// ---------------------------------------------------------------------------
// Borders / corners (param forms)
// ---------------------------------------------------------------------------

func (r *StyleRefinement) setBorderTop(l geometry.AbsoluteLength) *StyleRefinement {
	if r.BorderWidths == nil {
		r.BorderWidths = &EdgesAbsoluteRefinement{}
	}
	r.BorderWidths.Top = &l
	return r
}

func (r *StyleRefinement) setBorderBottom(l geometry.AbsoluteLength) *StyleRefinement {
	if r.BorderWidths == nil {
		r.BorderWidths = &EdgesAbsoluteRefinement{}
	}
	r.BorderWidths.Bottom = &l
	return r
}

func (r *StyleRefinement) setBorderLeft(l geometry.AbsoluteLength) *StyleRefinement {
	if r.BorderWidths == nil {
		r.BorderWidths = &EdgesAbsoluteRefinement{}
	}
	r.BorderWidths.Left = &l
	return r
}

func (r *StyleRefinement) setBorderRight(l geometry.AbsoluteLength) *StyleRefinement {
	if r.BorderWidths == nil {
		r.BorderWidths = &EdgesAbsoluteRefinement{}
	}
	r.BorderWidths.Right = &l
	return r
}

// Border sets the border width on all four edges (CSS border-width). The
// border is only painted when a border color is set, see
// [StyleRefinement.SetBorderColor]. Border_* scale methods set common pixel
// widths.
func (r *StyleRefinement) Border(l geometry.AbsoluteLength) *StyleRefinement {
	r.setBorderTop(l)
	r.setBorderRight(l)
	r.setBorderBottom(l)
	return r.setBorderLeft(l)
}

// BorderT sets the top border width.
func (r *StyleRefinement) BorderT(l geometry.AbsoluteLength) *StyleRefinement {
	return r.setBorderTop(l)
}

// BorderB sets the bottom border width.
func (r *StyleRefinement) BorderB(l geometry.AbsoluteLength) *StyleRefinement {
	return r.setBorderBottom(l)
}

// BorderL sets the left border width.
func (r *StyleRefinement) BorderL(l geometry.AbsoluteLength) *StyleRefinement {
	return r.setBorderLeft(l)
}

// BorderR sets the right border width.
func (r *StyleRefinement) BorderR(l geometry.AbsoluteLength) *StyleRefinement {
	return r.setBorderRight(l)
}

// BorderX sets the left and right border widths.
func (r *StyleRefinement) BorderX(l geometry.AbsoluteLength) *StyleRefinement {
	r.setBorderLeft(l)
	return r.setBorderRight(l)
}

// BorderY sets the top and bottom border widths.
func (r *StyleRefinement) BorderY(l geometry.AbsoluteLength) *StyleRefinement {
	r.setBorderTop(l)
	return r.setBorderBottom(l)
}

// SetBorderColor sets the border color (CSS border-color). Elements expose it
// as BorderColor. A transparent or unset color paints no border, see
// [Style.IsBorderVisible].
func (r *StyleRefinement) SetBorderColor(c gc.Hsla) *StyleRefinement { r.BorderColor = &c; return r }

// BorderDashed sets the border style to dashed (Tailwind border-dashed). The
// default style is solid.
func (r *StyleRefinement) BorderDashed() *StyleRefinement {
	b := BorderStyleDashed
	r.BorderStyle = &b
	return r
}

func (r *StyleRefinement) setCornerRadius(field func(*CornersAbsoluteRefinement) **geometry.AbsoluteLength, l geometry.AbsoluteLength) *StyleRefinement {
	if r.CornerRadii == nil {
		r.CornerRadii = &CornersAbsoluteRefinement{}
	}
	p := field(r.CornerRadii)
	*p = &l
	return r
}

// Rounded sets the corner radius on all four corners (CSS border-radius).
// Radii larger than half the shorter side are clamped at paint time. Rounded_*
// scale methods set the Tailwind radii.
func (r *StyleRefinement) Rounded(l geometry.AbsoluteLength) *StyleRefinement {
	r.setCornerRadius(func(c *CornersAbsoluteRefinement) **geometry.AbsoluteLength { return &c.TopLeft }, l)
	r.setCornerRadius(func(c *CornersAbsoluteRefinement) **geometry.AbsoluteLength { return &c.TopRight }, l)
	r.setCornerRadius(func(c *CornersAbsoluteRefinement) **geometry.AbsoluteLength { return &c.BottomRight }, l)
	return r.setCornerRadius(func(c *CornersAbsoluteRefinement) **geometry.AbsoluteLength { return &c.BottomLeft }, l)
}

// RoundedT sets the top-left and top-right corner radii.
func (r *StyleRefinement) RoundedT(l geometry.AbsoluteLength) *StyleRefinement {
	r.setCornerRadius(func(c *CornersAbsoluteRefinement) **geometry.AbsoluteLength { return &c.TopLeft }, l)
	return r.setCornerRadius(func(c *CornersAbsoluteRefinement) **geometry.AbsoluteLength { return &c.TopRight }, l)
}

// RoundedB sets the bottom-left and bottom-right corner radii.
func (r *StyleRefinement) RoundedB(l geometry.AbsoluteLength) *StyleRefinement {
	r.setCornerRadius(func(c *CornersAbsoluteRefinement) **geometry.AbsoluteLength { return &c.BottomLeft }, l)
	return r.setCornerRadius(func(c *CornersAbsoluteRefinement) **geometry.AbsoluteLength { return &c.BottomRight }, l)
}

// RoundedL sets the top-left and bottom-left corner radii.
func (r *StyleRefinement) RoundedL(l geometry.AbsoluteLength) *StyleRefinement {
	r.setCornerRadius(func(c *CornersAbsoluteRefinement) **geometry.AbsoluteLength { return &c.TopLeft }, l)
	return r.setCornerRadius(func(c *CornersAbsoluteRefinement) **geometry.AbsoluteLength { return &c.BottomLeft }, l)
}

// RoundedR sets the top-right and bottom-right corner radii.
func (r *StyleRefinement) RoundedR(l geometry.AbsoluteLength) *StyleRefinement {
	r.setCornerRadius(func(c *CornersAbsoluteRefinement) **geometry.AbsoluteLength { return &c.TopRight }, l)
	return r.setCornerRadius(func(c *CornersAbsoluteRefinement) **geometry.AbsoluteLength { return &c.BottomRight }, l)
}

// RoundedTL sets the top-left corner radius.
func (r *StyleRefinement) RoundedTL(l geometry.AbsoluteLength) *StyleRefinement {
	return r.setCornerRadius(func(c *CornersAbsoluteRefinement) **geometry.AbsoluteLength { return &c.TopLeft }, l)
}

// RoundedTR sets the top-right corner radius.
func (r *StyleRefinement) RoundedTR(l geometry.AbsoluteLength) *StyleRefinement {
	return r.setCornerRadius(func(c *CornersAbsoluteRefinement) **geometry.AbsoluteLength { return &c.TopRight }, l)
}

// RoundedBL sets the bottom-left corner radius.
func (r *StyleRefinement) RoundedBL(l geometry.AbsoluteLength) *StyleRefinement {
	return r.setCornerRadius(func(c *CornersAbsoluteRefinement) **geometry.AbsoluteLength { return &c.BottomLeft }, l)
}

// RoundedBR sets the bottom-right corner radius.
func (r *StyleRefinement) RoundedBR(l geometry.AbsoluteLength) *StyleRefinement {
	return r.setCornerRadius(func(c *CornersAbsoluteRefinement) **geometry.AbsoluteLength { return &c.BottomRight }, l)
}

// ---------------------------------------------------------------------------
// Background / opacity
// ---------------------------------------------------------------------------

// Bg sets the background fill, a solid color, gradient or pattern (CSS
// background). See [gc.Background].
func (r *StyleRefinement) Bg(b gc.Background) *StyleRefinement { r.Background = &b; return r }

// BgColor sets a solid background color (Tailwind bg-*).
func (r *StyleRefinement) BgColor(c gc.Hsla) *StyleRefinement {
	return r.Bg(gc.SolidBackground(c))
}

// SetOpacity sets the opacity of the element and its descendants, from 0
// (transparent) to 1 (opaque) (CSS opacity). Elements expose it as Opacity.
func (r *StyleRefinement) SetOpacity(v float32) *StyleRefinement { r.Opacity = &v; return r }

// ---------------------------------------------------------------------------
// Position / inset
// ---------------------------------------------------------------------------

// Absolute sets position to absolute: the element leaves the normal flow and
// is placed using its insets inside its parent (Tailwind absolute).
func (r *StyleRefinement) Absolute() *StyleRefinement {
	p := PositionAbsolute
	r.Position = &p
	return r
}

// Relative sets position to relative, the default: the element takes part in
// layout (Tailwind relative).
func (r *StyleRefinement) Relative() *StyleRefinement {
	p := PositionRelative
	r.Position = &p
	return r
}

func (r *StyleRefinement) setInsetTop(l geometry.Length) *StyleRefinement {
	if r.Inset == nil {
		r.Inset = &EdgesLengthRefinement{}
	}
	r.Inset.Top = &l
	return r
}

func (r *StyleRefinement) setInsetBottom(l geometry.Length) *StyleRefinement {
	if r.Inset == nil {
		r.Inset = &EdgesLengthRefinement{}
	}
	r.Inset.Bottom = &l
	return r
}

func (r *StyleRefinement) setInsetLeft(l geometry.Length) *StyleRefinement {
	if r.Inset == nil {
		r.Inset = &EdgesLengthRefinement{}
	}
	r.Inset.Left = &l
	return r
}

func (r *StyleRefinement) setInsetRight(l geometry.Length) *StyleRefinement {
	if r.Inset == nil {
		r.Inset = &EdgesLengthRefinement{}
	}
	r.Inset.Right = &l
	return r
}

// Top sets the top inset (CSS top). It positions absolute elements and offsets
// relative ones. Top_* scale methods cover the Tailwind values.
func (r *StyleRefinement) Top(l geometry.Length) *StyleRefinement { return r.setInsetTop(l) }

// Bottom sets the bottom inset (CSS bottom).
func (r *StyleRefinement) Bottom(l geometry.Length) *StyleRefinement { return r.setInsetBottom(l) }

// Left sets the left inset (CSS left).
func (r *StyleRefinement) Left(l geometry.Length) *StyleRefinement { return r.setInsetLeft(l) }

// Right sets the right inset (CSS right).
func (r *StyleRefinement) Right(l geometry.Length) *StyleRefinement { return r.setInsetRight(l) }

// SetInset sets the top, right, bottom and left insets to l (CSS inset).
// Elements expose it as Inset.
func (r *StyleRefinement) SetInset(l geometry.Length) *StyleRefinement {
	r.setInsetTop(l)
	r.setInsetRight(l)
	r.setInsetBottom(l)
	return r.setInsetLeft(l)
}

// ---------------------------------------------------------------------------
// Overflow / scrollbar
// ---------------------------------------------------------------------------

func (r *StyleRefinement) setOverflowX(o Overflow) *StyleRefinement {
	if r.Overflow == nil {
		r.Overflow = &PointOverflowRefinement{}
	}
	r.Overflow.X = &o
	return r
}

func (r *StyleRefinement) setOverflowY(o Overflow) *StyleRefinement {
	if r.Overflow == nil {
		r.Overflow = &PointOverflowRefinement{}
	}
	r.Overflow.Y = &o
	return r
}

// OverflowX sets how content overflowing horizontally is handled (CSS
// overflow-x).
func (r *StyleRefinement) OverflowX(o Overflow) *StyleRefinement { return r.setOverflowX(o) }

// OverflowY sets how content overflowing vertically is handled (CSS
// overflow-y).
func (r *StyleRefinement) OverflowY(o Overflow) *StyleRefinement { return r.setOverflowY(o) }

// OverflowHidden clips overflow on both axes (Tailwind overflow-hidden).
func (r *StyleRefinement) OverflowHidden() *StyleRefinement {
	r.setOverflowX(OverflowHidden)
	return r.setOverflowY(OverflowHidden)
}

// OverflowScroll makes both axes scroll containers (Tailwind overflow-scroll).
func (r *StyleRefinement) OverflowScroll() *StyleRefinement {
	r.setOverflowX(OverflowScroll)
	return r.setOverflowY(OverflowScroll)
}

// OverflowXHidden clips horizontal overflow (Tailwind overflow-x-hidden).
func (r *StyleRefinement) OverflowXHidden() *StyleRefinement { return r.setOverflowX(OverflowHidden) }

// OverflowYHidden clips vertical overflow (Tailwind overflow-y-hidden).
func (r *StyleRefinement) OverflowYHidden() *StyleRefinement { return r.setOverflowY(OverflowHidden) }

// OverflowXScroll makes the horizontal axis scrollable (Tailwind
// overflow-x-scroll).
func (r *StyleRefinement) OverflowXScroll() *StyleRefinement { return r.setOverflowX(OverflowScroll) }

// OverflowYScroll makes the vertical axis scrollable (Tailwind
// overflow-y-scroll).
func (r *StyleRefinement) OverflowYScroll() *StyleRefinement { return r.setOverflowY(OverflowScroll) }

// SetScrollbarWidth sets the space reserved for a scrollbar on scrolling axes
// (CSS scrollbar-width). Elements expose it as ScrollbarWidth.
func (r *StyleRefinement) SetScrollbarWidth(l geometry.AbsoluteLength) *StyleRefinement {
	r.ScrollbarWidth = &l
	return r
}

// SetRestrictScrollToAxis locks pixel-precise scroll gestures (trackpads) to
// the dominant axis. Elements expose it as RestrictScrollToAxis.
func (r *StyleRefinement) SetRestrictScrollToAxis() *StyleRefinement {
	b := true
	r.RestrictScrollToAxis = &b
	return r
}

// SetAllowConcurrentScroll lets a scroll gesture move both axes at once.
// Elements expose it as AllowConcurrentScroll.
func (r *StyleRefinement) SetAllowConcurrentScroll() *StyleRefinement {
	b := true
	r.AllowConcurrentScroll = &b
	return r
}

// ---------------------------------------------------------------------------
// Flex
// ---------------------------------------------------------------------------

// FlexCol sets flex-direction to column: children stack top to bottom
// (Tailwind flex-col).
func (r *StyleRefinement) FlexCol() *StyleRefinement {
	d := FlexDirectionColumn
	r.FlexDirection = &d
	return r
}

// FlexColReverse sets flex-direction to column-reverse: children stack bottom
// to top (Tailwind flex-col-reverse).
func (r *StyleRefinement) FlexColReverse() *StyleRefinement {
	d := FlexDirectionColumnReverse
	r.FlexDirection = &d
	return r
}

// FlexRow sets flex-direction to row: children run left to right (Tailwind
// flex-row). This is the default.
func (r *StyleRefinement) FlexRow() *StyleRefinement {
	d := FlexDirectionRow
	r.FlexDirection = &d
	return r
}

// FlexRowReverse sets flex-direction to row-reverse: children run right to
// left (Tailwind flex-row-reverse).
func (r *StyleRefinement) FlexRowReverse() *StyleRefinement {
	d := FlexDirectionRowReverse
	r.FlexDirection = &d
	return r
}

// SetFlexWrap sets flex-wrap to wrap: children that do not fit move to a new
// line (Tailwind flex-wrap). Elements expose it as FlexWrap.
func (r *StyleRefinement) SetFlexWrap() *StyleRefinement {
	w := FlexWrapWrap
	r.FlexWrap = &w
	return r
}

// SetFlexWrapReverse sets flex-wrap to wrap-reverse: new lines are added in
// the reverse cross direction (Tailwind flex-wrap-reverse). Elements expose it
// as FlexWrapReverse.
func (r *StyleRefinement) SetFlexWrapReverse() *StyleRefinement {
	w := FlexWrapWrapReverse
	r.FlexWrap = &w
	return r
}

// SetFlexNowrap sets flex-wrap to nowrap, the default: all children share a
// single line (Tailwind flex-nowrap). Elements expose it as FlexNowrap.
func (r *StyleRefinement) SetFlexNowrap() *StyleRefinement {
	w := FlexWrapNone
	r.FlexWrap = &w
	return r
}

// SetFlexBasis sets the initial main size of a flex item before growing and
// shrinking (CSS flex-basis). Elements expose it as FlexBasis.
func (r *StyleRefinement) SetFlexBasis(l geometry.Length) *StyleRefinement {
	r.FlexBasis = &l
	return r
}

// SetFlexGrow sets how the item shares positive free space (CSS flex-grow).
// The default is 0. Elements expose it as FlexGrow.
func (r *StyleRefinement) SetFlexGrow(v float32) *StyleRefinement { r.FlexGrow = &v; return r }

// SetFlexGrow0 sets flex-grow to 0 (Tailwind grow-0).
func (r *StyleRefinement) SetFlexGrow0() *StyleRefinement { return r.SetFlexGrow(0) }

// SetFlexGrow1 sets flex-grow to 1 (Tailwind grow).
func (r *StyleRefinement) SetFlexGrow1() *StyleRefinement { return r.SetFlexGrow(1) }

// SetFlexShrink sets how the item gives up space when the line overflows (CSS
// flex-shrink). The default is 1. Elements expose it as FlexShrink.
func (r *StyleRefinement) SetFlexShrink(v float32) *StyleRefinement { r.FlexShrink = &v; return r }

// SetFlexShrink0 sets flex-shrink to 0, so the item never shrinks (Tailwind
// shrink-0).
func (r *StyleRefinement) SetFlexShrink0() *StyleRefinement { return r.SetFlexShrink(0) }

// SetFlexShrink1 sets flex-shrink to 1 (Tailwind shrink).
func (r *StyleRefinement) SetFlexShrink1() *StyleRefinement { return r.SetFlexShrink(1) }

// Flex1 sets flex-grow 1, flex-shrink 1 and flex-basis 0% (CSS flex: 1 1 0%,
// Tailwind flex-1): the item fills the remaining space and ignores its content
// size.
func (r *StyleRefinement) Flex1() *StyleRefinement {
	r.SetFlexGrow(1)
	r.SetFlexShrink(1)
	return r.SetFlexBasis(geometry.LengthFromDefinite(geometry.RelativeDefinite(0)))
}

// FlexAuto sets flex-grow 1, flex-shrink 1 and flex-basis auto (CSS flex: 1 1
// auto, Tailwind flex-auto): the item starts from its content size and grows.
func (r *StyleRefinement) FlexAuto() *StyleRefinement {
	r.SetFlexGrow(1)
	r.SetFlexShrink(1)
	return r.SetFlexBasis(geometry.AutoLength())
}

// FlexInitial sets flex-grow 0, flex-shrink 1 and flex-basis auto (CSS flex: 0
// 1 auto, Tailwind flex-initial): the item can shrink but never grows.
func (r *StyleRefinement) FlexInitial() *StyleRefinement {
	r.SetFlexGrow(0)
	r.SetFlexShrink(1)
	return r.SetFlexBasis(geometry.AutoLength())
}

// FlexNone sets flex-grow 0, flex-shrink 0 and flex-basis auto (CSS flex:
// none, Tailwind flex-none): the item is fully inflexible.
func (r *StyleRefinement) FlexNone() *StyleRefinement {
	r.SetFlexGrow(0)
	r.SetFlexShrink(0)
	return r.SetFlexBasis(geometry.AutoLength())
}

// ---------------------------------------------------------------------------
// Alignment
// ---------------------------------------------------------------------------

// ItemsStart aligns children to the start of the cross axis, flex-start
// semantics (Tailwind items-start; CSS align-items: flex-start).
func (r *StyleRefinement) ItemsStart() *StyleRefinement {
	v := AlignItemsFlexStart
	r.AlignItems = &v
	return r
}

// ItemsEnd aligns children to the end of the cross axis, flex-end semantics
// (Tailwind items-end).
func (r *StyleRefinement) ItemsEnd() *StyleRefinement {
	v := AlignItemsFlexEnd
	r.AlignItems = &v
	return r
}

// ItemsCenter centers children on the cross axis (Tailwind items-center).
func (r *StyleRefinement) ItemsCenter() *StyleRefinement {
	v := AlignItemsCenter
	r.AlignItems = &v
	return r
}

// ItemsBaseline aligns children by their text baseline (Tailwind
// items-baseline). The layout engine currently falls back to start alignment.
func (r *StyleRefinement) ItemsBaseline() *StyleRefinement {
	v := AlignItemsBaseline
	r.AlignItems = &v
	return r
}

// ItemsStretch stretches children with an auto cross size to fill the cross
// axis (Tailwind items-stretch). This is the behavior when no alignment is
// set.
func (r *StyleRefinement) ItemsStretch() *StyleRefinement {
	v := AlignItemsStretch
	r.AlignItems = &v
	return r
}

// SelfStart overrides the parent align-items for this element and aligns it to
// the cross-axis start (Tailwind self-start).
func (r *StyleRefinement) SelfStart() *StyleRefinement {
	v := AlignItemsStart
	r.AlignSelf = &v
	return r
}

// SelfEnd overrides the parent align-items for this element and aligns it to
// the cross-axis end (Tailwind self-end).
func (r *StyleRefinement) SelfEnd() *StyleRefinement {
	v := AlignItemsEnd
	r.AlignSelf = &v
	return r
}

// SelfCenter overrides the parent align-items for this element and centers it
// on the cross axis (Tailwind self-center).
func (r *StyleRefinement) SelfCenter() *StyleRefinement {
	v := AlignItemsCenter
	r.AlignSelf = &v
	return r
}

// SelfStretch overrides the parent align-items for this element so it
// stretches on the cross axis (Tailwind self-stretch).
func (r *StyleRefinement) SelfStretch() *StyleRefinement {
	v := AlignItemsStretch
	r.AlignSelf = &v
	return r
}

// JustifyStart packs children at the start of the main axis (Tailwind
// justify-start).
func (r *StyleRefinement) JustifyStart() *StyleRefinement {
	v := AlignContentStart
	r.JustifyContent = &v
	return r
}

// JustifyEnd packs children at the end of the main axis (Tailwind
// justify-end).
func (r *StyleRefinement) JustifyEnd() *StyleRefinement {
	v := AlignContentEnd
	r.JustifyContent = &v
	return r
}

// JustifyCenter centers children on the main axis (Tailwind justify-center).
func (r *StyleRefinement) JustifyCenter() *StyleRefinement {
	v := AlignContentCenter
	r.JustifyContent = &v
	return r
}

// JustifyBetween distributes free space between children, with the first and
// last flush to the edges (Tailwind justify-between).
func (r *StyleRefinement) JustifyBetween() *StyleRefinement {
	v := AlignContentSpaceBetween
	r.JustifyContent = &v
	return r
}

// JustifyAround distributes free space around children, giving the edges half
// the space between children (Tailwind justify-around).
func (r *StyleRefinement) JustifyAround() *StyleRefinement {
	v := AlignContentSpaceAround
	r.JustifyContent = &v
	return r
}

// JustifyEvenly distributes free space so that the gaps between children and
// at the edges are equal (Tailwind justify-evenly).
func (r *StyleRefinement) JustifyEvenly() *StyleRefinement {
	v := AlignContentSpaceEvenly
	r.JustifyContent = &v
	return r
}

// ContentNormal clears align-content on this refinement (Tailwind
// content-normal). Note that this unsets only this layer's value; it does not
// remove an inherited one after merging.
func (r *StyleRefinement) ContentNormal() *StyleRefinement { r.AlignContent = nil; return r }

// ContentCenter centers wrapped lines on the cross axis (Tailwind
// content-center).
func (r *StyleRefinement) ContentCenter() *StyleRefinement {
	v := AlignContentCenter
	r.AlignContent = &v
	return r
}

// ContentStart packs wrapped lines at the cross start (Tailwind
// content-start).
func (r *StyleRefinement) ContentStart() *StyleRefinement {
	v := AlignContentFlexStart
	r.AlignContent = &v
	return r
}

// ContentEnd packs wrapped lines at the cross end (Tailwind content-end).
func (r *StyleRefinement) ContentEnd() *StyleRefinement {
	v := AlignContentFlexEnd
	r.AlignContent = &v
	return r
}

// ContentBetween spreads wrapped lines with equal space between them (Tailwind
// content-between).
func (r *StyleRefinement) ContentBetween() *StyleRefinement {
	v := AlignContentSpaceBetween
	r.AlignContent = &v
	return r
}

// ContentAround spreads wrapped lines with half-size gaps at the edges
// (Tailwind content-around).
func (r *StyleRefinement) ContentAround() *StyleRefinement {
	v := AlignContentSpaceAround
	r.AlignContent = &v
	return r
}

// ContentEvenly spreads wrapped lines with equal gaps including the edges
// (Tailwind content-evenly).
func (r *StyleRefinement) ContentEvenly() *StyleRefinement {
	v := AlignContentSpaceEvenly
	r.AlignContent = &v
	return r
}

// ContentStretch stretches wrapped lines to fill the cross axis (Tailwind
// content-stretch).
func (r *StyleRefinement) ContentStretch() *StyleRefinement {
	v := AlignContentStretch
	r.AlignContent = &v
	return r
}

// ---------------------------------------------------------------------------
// Typography
// ---------------------------------------------------------------------------

func (r *StyleRefinement) text() *TextStyleRefinement {
	if r.Text == nil {
		r.Text = &TextStyleRefinement{}
	}
	return r.Text
}

// TextColor sets the text color (CSS color, Tailwind text-*). Text color is
// inherited by descendants.
func (r *StyleRefinement) TextColor(c gc.Hsla) *StyleRefinement {
	r.text().Color = &c
	return r
}

// FontWeight sets the font weight (CSS font-weight, Tailwind font-*).
func (r *StyleRefinement) FontWeight(w font.FontWeight) *StyleRefinement {
	r.text().FontWeight = &w
	return r
}

// TextBg sets the background color painted behind text runs (not the element
// box; use [StyleRefinement.BgColor] for that).
func (r *StyleRefinement) TextBg(c gc.Hsla) *StyleRefinement {
	r.text().BackgroundColor = &c
	return r
}

// TextSize sets the font size (CSS font-size). Rem sizes resolve against the
// window rem size. Text_* scale methods set the Tailwind sizes.
func (r *StyleRefinement) TextSize(l geometry.AbsoluteLength) *StyleRefinement {
	r.text().FontSize = &l
	return r
}

// FontFamily sets the font family name (CSS font-family, Tailwind font-sans
// etc.).
func (r *StyleRefinement) FontFamily(family string) *StyleRefinement {
	r.text().FontFamily = &family
	return r
}

// FontFeatures sets OpenType font features (CSS font-feature-settings).
func (r *StyleRefinement) FontFeatures(features font.FontFeatures) *StyleRefinement {
	r.text().FontFeatures = &features
	return r
}

// Font sets family, features, weight, style and fallbacks from the font
// descriptor f in one call.
func (r *StyleRefinement) Font(f font.Font) *StyleRefinement {
	t := r.text()
	t.FontFamily = &f.Family
	t.FontFeatures = &f.Features
	t.FontWeight = &f.Weight
	fs := f.Style
	t.FontStyle = &fs
	if f.Fallbacks != nil {
		t.FontFallbacks = f.Fallbacks
	}
	return r
}

// LineHeight sets the line height (CSS line-height). A fractional value is a
// multiple of the font size; the default is the golden ratio, see
// [geometry.Phi].
func (r *StyleRefinement) LineHeight(l geometry.DefiniteLength) *StyleRefinement {
	r.text().LineHeight = &l
	return r
}

// Italic sets the font style to italic (Tailwind italic).
func (r *StyleRefinement) Italic() *StyleRefinement {
	s := font.StyleItalic
	r.text().FontStyle = &s
	return r
}

// NotItalic sets the font style back to normal (Tailwind not-italic).
func (r *StyleRefinement) NotItalic() *StyleRefinement {
	s := font.StyleNormal
	r.text().FontStyle = &s
	return r
}

// Underline adds a 1px straight underline in the text color (Tailwind
// underline). Call the TextDecoration* methods after it to change color,
// thickness or waviness.
func (r *StyleRefinement) Underline() *StyleRefinement {
	r.text().Underline = &UnderlineStyle{Thickness: 1}
	r.text().ClearUnderline = false
	return r
}

// LineThrough adds a 1px strikethrough in the text color (Tailwind
// line-through). Call the TextDecoration* methods after it to change color or
// thickness.
func (r *StyleRefinement) LineThrough() *StyleRefinement {
	r.text().Strikethrough = &StrikethroughStyle{Thickness: 1}
	r.text().ClearStrikethrough = false
	return r
}

// TextDecorationNone removes underline and strikethrough, including ones
// inherited from the cascade (Tailwind no-underline).
func (r *StyleRefinement) TextDecorationNone() *StyleRefinement {
	t := r.text()
	t.Underline = nil
	t.ClearUnderline = true
	t.Strikethrough = nil
	t.ClearStrikethrough = true
	return r
}

// TextDecorationColor sets the color of the underline and strikethrough
// (Tailwind decoration-*). It only affects decorations already enabled on this
// refinement, so call it after [StyleRefinement.Underline] or
// [StyleRefinement.LineThrough].
func (r *StyleRefinement) TextDecorationColor(c gc.Hsla) *StyleRefinement {
	t := r.text()
	if t.Underline != nil {
		t.Underline.Color = &c
	}
	if t.Strikethrough != nil {
		t.Strikethrough.Color = &c
	}
	return r
}

// TextDecorationWavy makes an existing underline wavy (Tailwind
// decoration-wavy). It has no effect unless [StyleRefinement.Underline] was
// called first.
func (r *StyleRefinement) TextDecorationWavy() *StyleRefinement {
	t := r.text()
	if t.Underline != nil {
		t.Underline.Wavy = true
	}
	return r
}

// TextDecorationSolid makes an existing underline straight (Tailwind
// decoration-solid). It has no effect unless [StyleRefinement.Underline] was
// called first.
func (r *StyleRefinement) TextDecorationSolid() *StyleRefinement {
	t := r.text()
	if t.Underline != nil {
		t.Underline.Wavy = false
	}
	return r
}

// TextDecoration0 sets the thickness of existing decorations to 0 (Tailwind
// decoration-0). Call it after Underline or LineThrough.
func (r *StyleRefinement) TextDecoration0() *StyleRefinement { return r.textDecorationWidth(0) }

// TextDecoration1 sets the thickness of existing decorations to 1px (Tailwind
// decoration-1). Call it after Underline or LineThrough.
func (r *StyleRefinement) TextDecoration1() *StyleRefinement { return r.textDecorationWidth(1) }

// TextDecoration2 sets the thickness of existing decorations to 2px (Tailwind
// decoration-2). Call it after Underline or LineThrough.
func (r *StyleRefinement) TextDecoration2() *StyleRefinement { return r.textDecorationWidth(2) }

// TextDecoration4 sets the thickness of existing decorations to 4px (Tailwind
// decoration-4). Call it after Underline or LineThrough.
func (r *StyleRefinement) TextDecoration4() *StyleRefinement { return r.textDecorationWidth(4) }

// TextDecoration8 sets the thickness of existing decorations to 8px (Tailwind
// decoration-8). Call it after Underline or LineThrough.
func (r *StyleRefinement) TextDecoration8() *StyleRefinement { return r.textDecorationWidth(8) }

func (r *StyleRefinement) textDecorationWidth(w float32) *StyleRefinement {
	t := r.text()
	if t.Underline != nil {
		t.Underline.Thickness = w
	}
	if t.Strikethrough != nil {
		t.Strikethrough.Thickness = w
	}
	return r
}

// WhitespaceNormal lets text wrap at the available width (CSS white-space:
// normal, Tailwind whitespace-normal).
func (r *StyleRefinement) WhitespaceNormal() *StyleRefinement {
	ws := WhiteSpaceNormal
	r.text().WhiteSpace = &ws
	return r
}

// WhitespaceNowrap keeps text on one line (CSS white-space: nowrap, Tailwind
// whitespace-nowrap).
func (r *StyleRefinement) WhitespaceNowrap() *StyleRefinement {
	ws := WhiteSpaceNowrap
	r.text().WhiteSpace = &ws
	return r
}

// TextEllipsis truncates overflowing text at the end and appends an ellipsis
// (Tailwind text-ellipsis). It is normally combined with overflow hidden and
// nowrap; see [StyleRefinement.Truncate].
func (r *StyleRefinement) TextEllipsis() *StyleRefinement {
	to := TextOverflow{Kind: TextOverflowTruncate, Affix: DefaultEllipsis}
	r.text().TextOverflow = &to
	return r
}

// TextEllipsisStart truncates overflowing text at the start and prepends an
// ellipsis. This is a gpui extension without a CSS equivalent.
func (r *StyleRefinement) TextEllipsisStart() *StyleRefinement {
	to := TextOverflow{Kind: TextOverflowTruncateStart, Affix: DefaultEllipsis}
	r.text().TextOverflow = &to
	return r
}

// TextEllipsisMiddle truncates overflowing text in the middle and inserts an
// ellipsis. This is a gpui extension without a CSS equivalent.
func (r *StyleRefinement) TextEllipsisMiddle() *StyleRefinement {
	to := TextOverflow{Kind: TextOverflowTruncateMiddle, Affix: DefaultEllipsis}
	r.text().TextOverflow = &to
	return r
}

// TextOverflow sets a custom truncation mode and affix, see [TextOverflow].
func (r *StyleRefinement) TextOverflow(to TextOverflow) *StyleRefinement {
	r.text().TextOverflow = &to
	return r
}

// TextAlignLeft aligns text to the left (Tailwind text-left).
func (r *StyleRefinement) TextAlignLeft() *StyleRefinement {
	a := TextAlignLeft
	r.text().TextAlign = &a
	return r
}

// TextAlignCenter centers text (Tailwind text-center).
func (r *StyleRefinement) TextAlignCenter() *StyleRefinement {
	a := TextAlignCenter
	r.text().TextAlign = &a
	return r
}

// TextAlignRight aligns text to the right (Tailwind text-right).
func (r *StyleRefinement) TextAlignRight() *StyleRefinement {
	a := TextAlignRight
	r.text().TextAlign = &a
	return r
}

// Truncate hides overflow, disables wrapping and adds an end ellipsis, a
// single-line truncation (Tailwind truncate).
func (r *StyleRefinement) Truncate() *StyleRefinement {
	r.OverflowHidden()
	r.WhitespaceNowrap()
	return r.TextEllipsis()
}

// LineClamp limits text to n lines and hides the overflow (Tailwind
// line-clamp-n).
func (r *StyleRefinement) LineClamp(n int) *StyleRefinement {
	r.text().LineClamp = &n
	return r.OverflowHidden()
}

// ---------------------------------------------------------------------------
// Cursor
// ---------------------------------------------------------------------------

// CursorDefault requests the default arrow cursor while hovering (Tailwind
// cursor-default).
func (r *StyleRefinement) CursorDefault() *StyleRefinement { return r.cursor(CursorArrow) }

// CursorPointer requests the pointing-hand cursor while hovering (Tailwind
// cursor-pointer).
func (r *StyleRefinement) CursorPointer() *StyleRefinement { return r.cursor(CursorPointingHand) }

// CursorText requests the I-beam text cursor while hovering (Tailwind
// cursor-text).
func (r *StyleRefinement) CursorText() *StyleRefinement { return r.cursor(CursorIBeam) }

// CursorNotAllowed requests the not-allowed cursor while hovering (Tailwind
// cursor-not-allowed).
func (r *StyleRefinement) CursorNotAllowed() *StyleRefinement {
	return r.cursor(CursorOperationNotAllowed)
}

// CursorMove requests the closed-hand cursor while hovering (Tailwind
// cursor-move).
func (r *StyleRefinement) CursorMove() *StyleRefinement { return r.cursor(CursorClosedHand) }

// CursorColResize requests the column-resize cursor while hovering (Tailwind
// cursor-col-resize).
func (r *StyleRefinement) CursorColResize() *StyleRefinement { return r.cursor(CursorResizeColumn) }

// CursorRowResize requests the row-resize cursor while hovering (Tailwind
// cursor-row-resize).
func (r *StyleRefinement) CursorRowResize() *StyleRefinement { return r.cursor(CursorResizeRow) }

func (r *StyleRefinement) cursor(c CursorStyle) *StyleRefinement {
	r.MouseCursor = &c
	return r
}

// ---------------------------------------------------------------------------
// Grid
// ---------------------------------------------------------------------------

// SetGridCols sets grid-template-columns to n equal tracks, repeat(n,
// minmax(0, 1fr)) (Tailwind grid-cols-n). Elements expose it as GridCols.
func (r *StyleRefinement) SetGridCols(n uint16) *StyleRefinement {
	t := GridTemplate{Repeat: n, MinSize: GridTemplateMinSizeZero}
	r.GridCols = &t
	return r
}

// GridColsMinContent sets grid-template-columns to n tracks of
// minmax(min-content, 1fr).
func (r *StyleRefinement) GridColsMinContent(n uint16) *StyleRefinement {
	t := GridTemplate{Repeat: n, MinSize: GridTemplateMinSizeMinContent}
	r.GridCols = &t
	return r
}

// GridColsMaxContent sets grid-template-columns to n tracks of minmax(0,
// max-content).
func (r *StyleRefinement) GridColsMaxContent(n uint16) *StyleRefinement {
	t := GridTemplate{Repeat: n, MinSize: GridTemplateMinSizeMaxContent}
	r.GridCols = &t
	return r
}

// SetGridRows sets grid-template-rows to n equal tracks, repeat(n, minmax(0,
// 1fr)) (Tailwind grid-rows-n). Elements expose it as GridRows.
func (r *StyleRefinement) SetGridRows(n uint16) *StyleRefinement {
	t := GridTemplate{Repeat: n, MinSize: GridTemplateMinSizeZero}
	r.GridRows = &t
	return r
}

// ColStart sets the grid line where the item starts (Tailwind col-start-n).
// Lines are 1-based; negative values count from the end.
func (r *StyleRefinement) ColStart(line int16) *StyleRefinement {
	r.gridLocation().Start = geometry.GridPlacementLine(line)
	return r
}

// ColEnd sets the grid line where the item ends (Tailwind col-end-n).
func (r *StyleRefinement) ColEnd(line int16) *StyleRefinement {
	r.gridLocation().End = geometry.GridPlacementLine(line)
	return r
}

// ColSpan makes the item span the given number of columns (Tailwind
// col-span-n).
func (r *StyleRefinement) ColSpan(span uint16) *StyleRefinement {
	r.gridLocation().End = geometry.GridPlacementSpan(span)
	return r
}

// RowStart sets the grid line where the item starts (Tailwind row-start-n).
// Lines are 1-based; negative values count from the end.
func (r *StyleRefinement) RowStart(line int16) *StyleRefinement {
	r.gridLocation().Start = geometry.GridPlacementLine(line)
	return r
}

// RowEnd sets the grid line where the item ends (Tailwind row-end-n).
func (r *StyleRefinement) RowEnd(line int16) *StyleRefinement {
	r.gridLocation().End = geometry.GridPlacementLine(line)
	return r
}

// RowSpan makes the item span the given number of rows (Tailwind row-span-n).
func (r *StyleRefinement) RowSpan(span uint16) *StyleRefinement {
	r.gridLocation().End = geometry.GridPlacementSpan(span)
	return r
}

func (r *StyleRefinement) gridLocation() *geometry.GridLocation {
	if r.GridLocation == nil {
		r.GridLocation = &geometry.GridLocation{}
	}
	return r.GridLocation
}
