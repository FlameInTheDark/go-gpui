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

// Block sets display: block.
func (r *StyleRefinement) Block() *StyleRefinement { d := DisplayBlock; r.Display = &d; return r }

// Flex sets display: flex.
func (r *StyleRefinement) Flex() *StyleRefinement { d := DisplayFlex; r.Display = &d; return r }

// Grid sets display: grid.
func (r *StyleRefinement) Grid() *StyleRefinement { d := DisplayGrid; r.Display = &d; return r }

// Hidden sets display: none.
func (r *StyleRefinement) Hidden() *StyleRefinement { d := DisplayNone; r.Display = &d; return r }

// Visible sets visibility: visible.
func (r *StyleRefinement) Visible() *StyleRefinement {
	v := VisibilityVisible
	r.Visibility = &v
	return r
}

// Invisible sets visibility: hidden.
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

// W sets the width.
func (r *StyleRefinement) W(l geometry.Length) *StyleRefinement { return r.setW(l) }

// H sets the height.
func (r *StyleRefinement) H(l geometry.Length) *StyleRefinement { return r.setH(l) }

// SetSize sets both width and height (see Size on elements).
func (r *StyleRefinement) SetSize(w, h geometry.Length) *StyleRefinement {
	r.setW(w)
	return r.setH(h)
}

// SetSizeUniform sets both width and height to the same length (used by the
// size_N scale methods).
func (r *StyleRefinement) SetSizeUniform(l geometry.Length) *StyleRefinement {
	r.setW(l)
	return r.setH(l)
}

// MinW sets the minimum width.
func (r *StyleRefinement) MinW(l geometry.Length) *StyleRefinement {
	if r.MinSize == nil {
		r.MinSize = &SizeLengthRefinement{}
	}
	r.MinSize.Width = &l
	return r
}

// MinH sets the minimum height.
func (r *StyleRefinement) MinH(l geometry.Length) *StyleRefinement {
	if r.MinSize == nil {
		r.MinSize = &SizeLengthRefinement{}
	}
	r.MinSize.Height = &l
	return r
}

// MaxW sets the maximum width.
func (r *StyleRefinement) MaxW(l geometry.Length) *StyleRefinement {
	if r.MaxSize == nil {
		r.MaxSize = &SizeLengthRefinement{}
	}
	r.MaxSize.Width = &l
	return r
}

// MaxH sets the maximum height.
func (r *StyleRefinement) MaxH(l geometry.Length) *StyleRefinement {
	if r.MaxSize == nil {
		r.MaxSize = &SizeLengthRefinement{}
	}
	r.MaxSize.Height = &l
	return r
}

// SetAspectRatio sets width / height (see AspectRatio on elements).
func (r *StyleRefinement) SetAspectRatio(v float32) *StyleRefinement {
	r.AspectRatio = &v
	return r
}

// AspectSquare sets aspect ratio 1.
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

// P sets padding on all edges.
func (r *StyleRefinement) P(l geometry.DefiniteLength) *StyleRefinement {
	r.setPadTop(l)
	r.setPadRight(l)
	r.setPadBottom(l)
	return r.setPadLeft(l)
}

// Pt sets the top padding.
func (r *StyleRefinement) Pt(l geometry.DefiniteLength) *StyleRefinement { return r.setPadTop(l) }

// Pb sets the bottom padding.
func (r *StyleRefinement) Pb(l geometry.DefiniteLength) *StyleRefinement { return r.setPadBottom(l) }

// Pl sets the left padding.
func (r *StyleRefinement) Pl(l geometry.DefiniteLength) *StyleRefinement { return r.setPadLeft(l) }

// Pr sets the right padding.
func (r *StyleRefinement) Pr(l geometry.DefiniteLength) *StyleRefinement { return r.setPadRight(l) }

// Px sets the horizontal padding.
func (r *StyleRefinement) Px(l geometry.DefiniteLength) *StyleRefinement {
	r.setPadLeft(l)
	return r.setPadRight(l)
}

// Py sets the vertical padding.
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

// M sets the margin on all edges.
func (r *StyleRefinement) M(l geometry.Length) *StyleRefinement {
	r.setMarTop(l)
	r.setMarRight(l)
	r.setMarBottom(l)
	return r.setMarLeft(l)
}

// Mt sets the top margin.
func (r *StyleRefinement) Mt(l geometry.Length) *StyleRefinement { return r.setMarTop(l) }

// Mb sets the bottom margin.
func (r *StyleRefinement) Mb(l geometry.Length) *StyleRefinement { return r.setMarBottom(l) }

// Ml sets the left margin.
func (r *StyleRefinement) Ml(l geometry.Length) *StyleRefinement { return r.setMarLeft(l) }

// Mr sets the right margin.
func (r *StyleRefinement) Mr(l geometry.Length) *StyleRefinement { return r.setMarRight(l) }

// Mx sets the horizontal margin.
func (r *StyleRefinement) Mx(l geometry.Length) *StyleRefinement {
	r.setMarLeft(l)
	return r.setMarRight(l)
}

// My sets the vertical margin.
func (r *StyleRefinement) My(l geometry.Length) *StyleRefinement {
	r.setMarTop(l)
	return r.setMarBottom(l)
}

// MxAuto sets horizontal margins to auto (centering in flex).
func (r *StyleRefinement) MxAuto() *StyleRefinement { return r.Mx(geometry.AutoLength()) }

// MlAuto sets the left margin to auto.
func (r *StyleRefinement) MlAuto() *StyleRefinement { return r.Ml(geometry.AutoLength()) }

// MrAuto sets the right margin to auto.
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

// SetGap sets both row and column gaps (see Gap on elements).
func (r *StyleRefinement) SetGap(l geometry.DefiniteLength) *StyleRefinement {
	r.setGapW(l)
	return r.setGapH(l)
}

// SetGapX sets the column gap (see GapX on elements).
func (r *StyleRefinement) SetGapX(l geometry.DefiniteLength) *StyleRefinement { return r.setGapW(l) }

// SetGapY sets the row gap (see GapY on elements).
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

// Border sets the border width on all edges.
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

// BorderX sets the horizontal border widths.
func (r *StyleRefinement) BorderX(l geometry.AbsoluteLength) *StyleRefinement {
	r.setBorderLeft(l)
	return r.setBorderRight(l)
}

// BorderY sets the vertical border widths.
func (r *StyleRefinement) BorderY(l geometry.AbsoluteLength) *StyleRefinement {
	r.setBorderTop(l)
	return r.setBorderBottom(l)
}

// SetBorderColor sets the border color (see BorderColor on elements).
func (r *StyleRefinement) SetBorderColor(c gc.Hsla) *StyleRefinement { r.BorderColor = &c; return r }

// BorderDashed sets the border style to dashed.
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

// Rounded sets the corner radius on all corners.
func (r *StyleRefinement) Rounded(l geometry.AbsoluteLength) *StyleRefinement {
	r.setCornerRadius(func(c *CornersAbsoluteRefinement) **geometry.AbsoluteLength { return &c.TopLeft }, l)
	r.setCornerRadius(func(c *CornersAbsoluteRefinement) **geometry.AbsoluteLength { return &c.TopRight }, l)
	r.setCornerRadius(func(c *CornersAbsoluteRefinement) **geometry.AbsoluteLength { return &c.BottomRight }, l)
	return r.setCornerRadius(func(c *CornersAbsoluteRefinement) **geometry.AbsoluteLength { return &c.BottomLeft }, l)
}

// RoundedT sets the top corner radii.
func (r *StyleRefinement) RoundedT(l geometry.AbsoluteLength) *StyleRefinement {
	r.setCornerRadius(func(c *CornersAbsoluteRefinement) **geometry.AbsoluteLength { return &c.TopLeft }, l)
	return r.setCornerRadius(func(c *CornersAbsoluteRefinement) **geometry.AbsoluteLength { return &c.TopRight }, l)
}

// RoundedB sets the bottom corner radii.
func (r *StyleRefinement) RoundedB(l geometry.AbsoluteLength) *StyleRefinement {
	r.setCornerRadius(func(c *CornersAbsoluteRefinement) **geometry.AbsoluteLength { return &c.BottomLeft }, l)
	return r.setCornerRadius(func(c *CornersAbsoluteRefinement) **geometry.AbsoluteLength { return &c.BottomRight }, l)
}

// RoundedL sets the left corner radii.
func (r *StyleRefinement) RoundedL(l geometry.AbsoluteLength) *StyleRefinement {
	r.setCornerRadius(func(c *CornersAbsoluteRefinement) **geometry.AbsoluteLength { return &c.TopLeft }, l)
	return r.setCornerRadius(func(c *CornersAbsoluteRefinement) **geometry.AbsoluteLength { return &c.BottomLeft }, l)
}

// RoundedR sets the right corner radii.
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

// Bg sets the background (solid or gradient).
func (r *StyleRefinement) Bg(b gc.Background) *StyleRefinement { r.Background = &b; return r }

// BgColor sets a solid background color.
func (r *StyleRefinement) BgColor(c gc.Hsla) *StyleRefinement {
	return r.Bg(gc.SolidBackground(c))
}

// SetOpacity sets the element opacity (see Opacity on elements).
func (r *StyleRefinement) SetOpacity(v float32) *StyleRefinement { r.Opacity = &v; return r }

// ---------------------------------------------------------------------------
// Position / inset
// ---------------------------------------------------------------------------

// Absolute sets position: absolute.
func (r *StyleRefinement) Absolute() *StyleRefinement {
	p := PositionAbsolute
	r.Position = &p
	return r
}

// Relative sets position: relative.
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

// Top sets the top inset.
func (r *StyleRefinement) Top(l geometry.Length) *StyleRefinement { return r.setInsetTop(l) }

// Bottom sets the bottom inset.
func (r *StyleRefinement) Bottom(l geometry.Length) *StyleRefinement { return r.setInsetBottom(l) }

// Left sets the left inset.
func (r *StyleRefinement) Left(l geometry.Length) *StyleRefinement { return r.setInsetLeft(l) }

// Right sets the right inset.
func (r *StyleRefinement) Right(l geometry.Length) *StyleRefinement { return r.setInsetRight(l) }

// SetInset sets all insets (see Inset on elements).
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

// OverflowX sets the horizontal overflow.
func (r *StyleRefinement) OverflowX(o Overflow) *StyleRefinement { return r.setOverflowX(o) }

// OverflowY sets the vertical overflow.
func (r *StyleRefinement) OverflowY(o Overflow) *StyleRefinement { return r.setOverflowY(o) }

// OverflowHidden hides overflow on both axes.
func (r *StyleRefinement) OverflowHidden() *StyleRefinement {
	r.setOverflowX(OverflowHidden)
	return r.setOverflowY(OverflowHidden)
}

// OverflowScroll makes both axes scrollable.
func (r *StyleRefinement) OverflowScroll() *StyleRefinement {
	r.setOverflowX(OverflowScroll)
	return r.setOverflowY(OverflowScroll)
}

// OverflowXHidden hides horizontal overflow.
func (r *StyleRefinement) OverflowXHidden() *StyleRefinement { return r.setOverflowX(OverflowHidden) }

// OverflowYHidden hides vertical overflow.
func (r *StyleRefinement) OverflowYHidden() *StyleRefinement { return r.setOverflowY(OverflowHidden) }

// OverflowXScroll makes the horizontal axis scrollable.
func (r *StyleRefinement) OverflowXScroll() *StyleRefinement { return r.setOverflowX(OverflowScroll) }

// OverflowYScroll makes the vertical axis scrollable.
func (r *StyleRefinement) OverflowYScroll() *StyleRefinement { return r.setOverflowY(OverflowScroll) }

// SetScrollbarWidth reserves scrollbar space (see ScrollbarWidth on elements).
func (r *StyleRefinement) SetScrollbarWidth(l geometry.AbsoluteLength) *StyleRefinement {
	r.ScrollbarWidth = &l
	return r
}

// SetRestrictScrollToAxis locks pixel-scroll gestures to the dominant axis
// (see RestrictScrollToAxis on elements).
func (r *StyleRefinement) SetRestrictScrollToAxis() *StyleRefinement {
	b := true
	r.RestrictScrollToAxis = &b
	return r
}

// SetAllowConcurrentScroll allows scrolling both axes at once (see
// AllowConcurrentScroll on elements).
func (r *StyleRefinement) SetAllowConcurrentScroll() *StyleRefinement {
	b := true
	r.AllowConcurrentScroll = &b
	return r
}

// ---------------------------------------------------------------------------
// Flex
// ---------------------------------------------------------------------------

// FlexCol sets flex-direction: column.
func (r *StyleRefinement) FlexCol() *StyleRefinement {
	d := FlexDirectionColumn
	r.FlexDirection = &d
	return r
}

// FlexColReverse sets flex-direction: column-reverse.
func (r *StyleRefinement) FlexColReverse() *StyleRefinement {
	d := FlexDirectionColumnReverse
	r.FlexDirection = &d
	return r
}

// FlexRow sets flex-direction: row.
func (r *StyleRefinement) FlexRow() *StyleRefinement {
	d := FlexDirectionRow
	r.FlexDirection = &d
	return r
}

// FlexRowReverse sets flex-direction: row-reverse.
func (r *StyleRefinement) FlexRowReverse() *StyleRefinement {
	d := FlexDirectionRowReverse
	r.FlexDirection = &d
	return r
}

// SetFlexWrap sets flex-wrap: wrap (see FlexWrap on elements).
func (r *StyleRefinement) SetFlexWrap() *StyleRefinement {
	w := FlexWrapWrap
	r.FlexWrap = &w
	return r
}

// SetFlexWrapReverse sets flex-wrap: wrap-reverse (see FlexWrapReverse).
func (r *StyleRefinement) SetFlexWrapReverse() *StyleRefinement {
	w := FlexWrapWrapReverse
	r.FlexWrap = &w
	return r
}

// SetFlexNowrap sets flex-wrap: nowrap (see FlexNowrap on elements).
func (r *StyleRefinement) SetFlexNowrap() *StyleRefinement {
	w := FlexWrapNone
	r.FlexWrap = &w
	return r
}

// SetFlexBasis sets flex-basis (see FlexBasis on elements).
func (r *StyleRefinement) SetFlexBasis(l geometry.Length) *StyleRefinement {
	r.FlexBasis = &l
	return r
}

// SetFlexGrow sets flex-grow (see FlexGrow on elements).
func (r *StyleRefinement) SetFlexGrow(v float32) *StyleRefinement { r.FlexGrow = &v; return r }

// SetFlexGrow0 sets flex-grow: 0.
func (r *StyleRefinement) SetFlexGrow0() *StyleRefinement { return r.SetFlexGrow(0) }

// SetFlexGrow1 sets flex-grow: 1.
func (r *StyleRefinement) SetFlexGrow1() *StyleRefinement { return r.SetFlexGrow(1) }

// SetFlexShrink sets flex-shrink (see FlexShrink on elements).
func (r *StyleRefinement) SetFlexShrink(v float32) *StyleRefinement { r.FlexShrink = &v; return r }

// SetFlexShrink0 sets flex-shrink: 0.
func (r *StyleRefinement) SetFlexShrink0() *StyleRefinement { return r.SetFlexShrink(0) }

// SetFlexShrink1 sets flex-shrink: 1.
func (r *StyleRefinement) SetFlexShrink1() *StyleRefinement { return r.SetFlexShrink(1) }

// Flex1 is grow 1, shrink 1, basis 0%.
func (r *StyleRefinement) Flex1() *StyleRefinement {
	r.SetFlexGrow(1)
	r.SetFlexShrink(1)
	return r.SetFlexBasis(geometry.LengthFromDefinite(geometry.RelativeDefinite(0)))
}

// FlexAuto is grow 1, shrink 1, basis auto.
func (r *StyleRefinement) FlexAuto() *StyleRefinement {
	r.SetFlexGrow(1)
	r.SetFlexShrink(1)
	return r.SetFlexBasis(geometry.AutoLength())
}

// FlexInitial is grow 0, shrink 1, basis auto.
func (r *StyleRefinement) FlexInitial() *StyleRefinement {
	r.SetFlexGrow(0)
	r.SetFlexShrink(1)
	return r.SetFlexBasis(geometry.AutoLength())
}

// FlexNone is grow 0, shrink 0, basis auto.
func (r *StyleRefinement) FlexNone() *StyleRefinement {
	r.SetFlexGrow(0)
	r.SetFlexShrink(0)
	return r.SetFlexBasis(geometry.AutoLength())
}

// ---------------------------------------------------------------------------
// Alignment
// ---------------------------------------------------------------------------

// ItemsStart aligns children to the cross start.
func (r *StyleRefinement) ItemsStart() *StyleRefinement {
	v := AlignItemsFlexStart
	r.AlignItems = &v
	return r
}

// ItemsEnd aligns children to the cross end.
func (r *StyleRefinement) ItemsEnd() *StyleRefinement {
	v := AlignItemsFlexEnd
	r.AlignItems = &v
	return r
}

// ItemsCenter centers children on the cross axis.
func (r *StyleRefinement) ItemsCenter() *StyleRefinement {
	v := AlignItemsCenter
	r.AlignItems = &v
	return r
}

// ItemsBaseline aligns children by baseline.
func (r *StyleRefinement) ItemsBaseline() *StyleRefinement {
	v := AlignItemsBaseline
	r.AlignItems = &v
	return r
}

// ItemsStretch stretches children on the cross axis.
func (r *StyleRefinement) ItemsStretch() *StyleRefinement {
	v := AlignItemsStretch
	r.AlignItems = &v
	return r
}

// SelfStart aligns this element to the cross start.
func (r *StyleRefinement) SelfStart() *StyleRefinement {
	v := AlignItemsStart
	r.AlignSelf = &v
	return r
}

// SelfEnd aligns this element to the cross end.
func (r *StyleRefinement) SelfEnd() *StyleRefinement {
	v := AlignItemsEnd
	r.AlignSelf = &v
	return r
}

// SelfCenter centers this element on the cross axis.
func (r *StyleRefinement) SelfCenter() *StyleRefinement {
	v := AlignItemsCenter
	r.AlignSelf = &v
	return r
}

// SelfStretch stretches this element on the cross axis.
func (r *StyleRefinement) SelfStretch() *StyleRefinement {
	v := AlignItemsStretch
	r.AlignSelf = &v
	return r
}

// JustifyStart justifies to the main start.
func (r *StyleRefinement) JustifyStart() *StyleRefinement {
	v := AlignContentStart
	r.JustifyContent = &v
	return r
}

// JustifyEnd justifies to the main end.
func (r *StyleRefinement) JustifyEnd() *StyleRefinement {
	v := AlignContentEnd
	r.JustifyContent = &v
	return r
}

// JustifyCenter centers on the main axis.
func (r *StyleRefinement) JustifyCenter() *StyleRefinement {
	v := AlignContentCenter
	r.JustifyContent = &v
	return r
}

// JustifyBetween spreads with space between, edges flush.
func (r *StyleRefinement) JustifyBetween() *StyleRefinement {
	v := AlignContentSpaceBetween
	r.JustifyContent = &v
	return r
}

// JustifyAround spreads with half-gaps at the edges.
func (r *StyleRefinement) JustifyAround() *StyleRefinement {
	v := AlignContentSpaceAround
	r.JustifyContent = &v
	return r
}

// JustifyEvenly distributes equal space including edges.
func (r *StyleRefinement) JustifyEvenly() *StyleRefinement {
	v := AlignContentSpaceEvenly
	r.JustifyContent = &v
	return r
}

// ContentNormal unsets align-content.
func (r *StyleRefinement) ContentNormal() *StyleRefinement { r.AlignContent = nil; return r }

// ContentCenter centers lines on the cross axis.
func (r *StyleRefinement) ContentCenter() *StyleRefinement {
	v := AlignContentCenter
	r.AlignContent = &v
	return r
}

// ContentStart packs lines at the cross start.
func (r *StyleRefinement) ContentStart() *StyleRefinement {
	v := AlignContentFlexStart
	r.AlignContent = &v
	return r
}

// ContentEnd packs lines at the cross end.
func (r *StyleRefinement) ContentEnd() *StyleRefinement {
	v := AlignContentFlexEnd
	r.AlignContent = &v
	return r
}

// ContentBetween spreads lines with space between.
func (r *StyleRefinement) ContentBetween() *StyleRefinement {
	v := AlignContentSpaceBetween
	r.AlignContent = &v
	return r
}

// ContentAround spreads lines with half-gaps at the edges.
func (r *StyleRefinement) ContentAround() *StyleRefinement {
	v := AlignContentSpaceAround
	r.AlignContent = &v
	return r
}

// ContentEvenly distributes lines with equal space.
func (r *StyleRefinement) ContentEvenly() *StyleRefinement {
	v := AlignContentSpaceEvenly
	r.AlignContent = &v
	return r
}

// ContentStretch stretches lines.
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

// TextColor sets the text color.
func (r *StyleRefinement) TextColor(c gc.Hsla) *StyleRefinement {
	r.text().Color = &c
	return r
}

// FontWeight sets the text weight.
func (r *StyleRefinement) FontWeight(w font.FontWeight) *StyleRefinement {
	r.text().FontWeight = &w
	return r
}

// TextBg sets the text background color.
func (r *StyleRefinement) TextBg(c gc.Hsla) *StyleRefinement {
	r.text().BackgroundColor = &c
	return r
}

// TextSize sets the font size.
func (r *StyleRefinement) TextSize(l geometry.AbsoluteLength) *StyleRefinement {
	r.text().FontSize = &l
	return r
}

// FontFamily sets the font family.
func (r *StyleRefinement) FontFamily(family string) *StyleRefinement {
	r.text().FontFamily = &family
	return r
}

// FontFeatures sets the font features.
func (r *StyleRefinement) FontFeatures(features font.FontFeatures) *StyleRefinement {
	r.text().FontFeatures = &features
	return r
}

// Font sets family, features, weight and style from a font descriptor.
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

// LineHeight sets the line height.
func (r *StyleRefinement) LineHeight(l geometry.DefiniteLength) *StyleRefinement {
	r.text().LineHeight = &l
	return r
}

// Italic sets italic text.
func (r *StyleRefinement) Italic() *StyleRefinement {
	s := font.StyleItalic
	r.text().FontStyle = &s
	return r
}

// NotItalic resets to upright text.
func (r *StyleRefinement) NotItalic() *StyleRefinement {
	s := font.StyleNormal
	r.text().FontStyle = &s
	return r
}

// Underline adds an underline.
func (r *StyleRefinement) Underline() *StyleRefinement {
	r.text().Underline = &UnderlineStyle{Thickness: 1}
	r.text().ClearUnderline = false
	return r
}

// LineThrough adds strikethrough.
func (r *StyleRefinement) LineThrough() *StyleRefinement {
	r.text().Strikethrough = &StrikethroughStyle{Thickness: 1}
	r.text().ClearStrikethrough = false
	return r
}

// TextDecorationNone clears underline and strikethrough.
func (r *StyleRefinement) TextDecorationNone() *StyleRefinement {
	t := r.text()
	t.Underline = nil
	t.ClearUnderline = true
	t.Strikethrough = nil
	t.ClearStrikethrough = true
	return r
}

// TextDecorationColor sets the decoration color (for both kinds).
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

// TextDecorationWavy makes the underline wavy.
func (r *StyleRefinement) TextDecorationWavy() *StyleRefinement {
	t := r.text()
	if t.Underline != nil {
		t.Underline.Wavy = true
	}
	return r
}

// TextDecorationSolid makes the underline straight.
func (r *StyleRefinement) TextDecorationSolid() *StyleRefinement {
	t := r.text()
	if t.Underline != nil {
		t.Underline.Wavy = false
	}
	return r
}

// TextDecoration0 sets decoration thickness to 0.
func (r *StyleRefinement) TextDecoration0() *StyleRefinement { return r.textDecorationWidth(0) }

// TextDecoration1 sets decoration thickness to 1px.
func (r *StyleRefinement) TextDecoration1() *StyleRefinement { return r.textDecorationWidth(1) }

// TextDecoration2 sets decoration thickness to 2px.
func (r *StyleRefinement) TextDecoration2() *StyleRefinement { return r.textDecorationWidth(2) }

// TextDecoration4 sets decoration thickness to 4px.
func (r *StyleRefinement) TextDecoration4() *StyleRefinement { return r.textDecorationWidth(4) }

// TextDecoration8 sets decoration thickness to 8px.
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

// WhitespaceNormal wraps text.
func (r *StyleRefinement) WhitespaceNormal() *StyleRefinement {
	ws := WhiteSpaceNormal
	r.text().WhiteSpace = &ws
	return r
}

// WhitespaceNowrap disables wrapping.
func (r *StyleRefinement) WhitespaceNowrap() *StyleRefinement {
	ws := WhiteSpaceNowrap
	r.text().WhiteSpace = &ws
	return r
}

// TextEllipsis truncates overflowing text with "…".
func (r *StyleRefinement) TextEllipsis() *StyleRefinement {
	to := TextOverflow{Kind: TextOverflowTruncate, Affix: DefaultEllipsis}
	r.text().TextOverflow = &to
	return r
}

// TextEllipsisStart truncates from the start.
func (r *StyleRefinement) TextEllipsisStart() *StyleRefinement {
	to := TextOverflow{Kind: TextOverflowTruncateStart, Affix: DefaultEllipsis}
	r.text().TextOverflow = &to
	return r
}

// TextEllipsisMiddle truncates the middle.
func (r *StyleRefinement) TextEllipsisMiddle() *StyleRefinement {
	to := TextOverflow{Kind: TextOverflowTruncateMiddle, Affix: DefaultEllipsis}
	r.text().TextOverflow = &to
	return r
}

// TextOverflow sets a custom truncation style.
func (r *StyleRefinement) TextOverflow(to TextOverflow) *StyleRefinement {
	r.text().TextOverflow = &to
	return r
}

// TextAlignLeft aligns text left.
func (r *StyleRefinement) TextAlignLeft() *StyleRefinement {
	a := TextAlignLeft
	r.text().TextAlign = &a
	return r
}

// TextAlignCenter centers text.
func (r *StyleRefinement) TextAlignCenter() *StyleRefinement {
	a := TextAlignCenter
	r.text().TextAlign = &a
	return r
}

// TextAlignRight aligns text right.
func (r *StyleRefinement) TextAlignRight() *StyleRefinement {
	a := TextAlignRight
	r.text().TextAlign = &a
	return r
}

// Truncate is overflow hidden + nowrap + ellipsis.
func (r *StyleRefinement) Truncate() *StyleRefinement {
	r.OverflowHidden()
	r.WhitespaceNowrap()
	return r.TextEllipsis()
}

// LineClamp limits text to n lines with overflow hidden.
func (r *StyleRefinement) LineClamp(n int) *StyleRefinement {
	r.text().LineClamp = &n
	return r.OverflowHidden()
}

// ---------------------------------------------------------------------------
// Cursor
// ---------------------------------------------------------------------------

// CursorDefault sets the arrow cursor.
func (r *StyleRefinement) CursorDefault() *StyleRefinement { return r.cursor(CursorArrow) }

// CursorPointer sets the pointing-hand cursor.
func (r *StyleRefinement) CursorPointer() *StyleRefinement { return r.cursor(CursorPointingHand) }

// CursorText sets the I-beam cursor.
func (r *StyleRefinement) CursorText() *StyleRefinement { return r.cursor(CursorIBeam) }

// CursorNotAllowed sets the not-allowed cursor.
func (r *StyleRefinement) CursorNotAllowed() *StyleRefinement {
	return r.cursor(CursorOperationNotAllowed)
}

// CursorMove sets the move cursor.
func (r *StyleRefinement) CursorMove() *StyleRefinement { return r.cursor(CursorClosedHand) }

// CursorColResize sets the column resize cursor.
func (r *StyleRefinement) CursorColResize() *StyleRefinement { return r.cursor(CursorResizeColumn) }

// CursorRowResize sets the row resize cursor.
func (r *StyleRefinement) CursorRowResize() *StyleRefinement { return r.cursor(CursorResizeRow) }

func (r *StyleRefinement) cursor(c CursorStyle) *StyleRefinement {
	r.MouseCursor = &c
	return r
}

// ---------------------------------------------------------------------------
// Grid
// ---------------------------------------------------------------------------

// SetGridCols sets a repeated column template (see GridCols on elements).
func (r *StyleRefinement) SetGridCols(n uint16) *StyleRefinement {
	t := GridTemplate{Repeat: n, MinSize: GridTemplateMinSizeZero}
	r.GridCols = &t
	return r
}

// GridColsMinContent uses min-content track minimums.
func (r *StyleRefinement) GridColsMinContent(n uint16) *StyleRefinement {
	t := GridTemplate{Repeat: n, MinSize: GridTemplateMinSizeMinContent}
	r.GridCols = &t
	return r
}

// GridColsMaxContent uses max-content track minimums.
func (r *StyleRefinement) GridColsMaxContent(n uint16) *StyleRefinement {
	t := GridTemplate{Repeat: n, MinSize: GridTemplateMinSizeMaxContent}
	r.GridCols = &t
	return r
}

// SetGridRows sets a repeated row template (see GridRows on elements).
func (r *StyleRefinement) SetGridRows(n uint16) *StyleRefinement {
	t := GridTemplate{Repeat: n, MinSize: GridTemplateMinSizeZero}
	r.GridRows = &t
	return r
}

// ColStart sets the grid column start line.
func (r *StyleRefinement) ColStart(line int16) *StyleRefinement {
	r.gridLocation().Start = geometry.GridPlacementLine(line)
	return r
}

// ColEnd sets the grid column end line.
func (r *StyleRefinement) ColEnd(line int16) *StyleRefinement {
	r.gridLocation().End = geometry.GridPlacementLine(line)
	return r
}

// ColSpan sets the column span.
func (r *StyleRefinement) ColSpan(span uint16) *StyleRefinement {
	r.gridLocation().End = geometry.GridPlacementSpan(span)
	return r
}

// RowStart sets the grid row start line.
func (r *StyleRefinement) RowStart(line int16) *StyleRefinement {
	r.gridLocation().Start = geometry.GridPlacementLine(line)
	return r
}

// RowEnd sets the grid row end line.
func (r *StyleRefinement) RowEnd(line int16) *StyleRefinement {
	r.gridLocation().End = geometry.GridPlacementLine(line)
	return r
}

// RowSpan sets the row span.
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
