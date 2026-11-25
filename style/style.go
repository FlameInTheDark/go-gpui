package style

import (
	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
)

// SizeLength is a width/height pair of [geometry.Length] values, each of
// which may be auto.
type SizeLength struct {
	Width, Height geometry.Length
}

// SizeDefinite is a width/height pair of [geometry.DefiniteLength] values
// (never auto). It is used for the flex/grid gap, where Width is the column
// gap and Height is the row gap.
type SizeDefinite struct {
	Width, Height geometry.DefiniteLength
}

// AutoSizeLength returns a [SizeLength] with both axes auto (upstream
// Size::auto()).
func AutoSizeLength() SizeLength {
	return SizeLength{Width: geometry.AutoLength(), Height: geometry.AutoLength()}
}

// FullSizeDefinite returns a [SizeDefinite] with both axes 100% of the
// parent (upstream Size::full()).
func FullSizeDefinite() SizeDefinite {
	return SizeDefinite{Width: geometry.RelativeDefinite(1), Height: geometry.RelativeDefinite(1)}
}

// Style is the fully resolved style of an element: every property has a
// concrete value, unlike [StyleRefinement] where each property is optional.
// A Style is produced by starting from [DefaultStyle] and applying
// refinements with [Style.Refine].
//
// The zero value is NOT the default style (it would be display: block with
// zero flex-shrink); always start from [DefaultStyle]. Pointer fields use nil
// to mean "not specified", which the layout and paint code treat as the CSS
// initial value (for example a nil AlignItems stretches).
type Style struct {
	// Display selects the layout algorithm (block, flex, grid) or none.
	Display Display
	// Visibility hides painting while keeping layout space.
	Visibility Visibility
	// Overflow is the per-axis overflow behavior, indexed x (0) then y (1).
	Overflow [2]Overflow
	// ScrollbarWidth is the space reserved for a scrollbar on scrolling axes.
	ScrollbarWidth geometry.AbsoluteLength
	// AllowConcurrentScroll lets one scroll gesture move both axes.
	AllowConcurrentScroll bool
	// RestrictScrollToAxis locks pixel-scroll gestures to the dominant axis.
	RestrictScrollToAxis bool

	// Position is relative (in flow) or absolute (placed by Inset).
	Position Position
	// Inset is the top/right/bottom/left offset. Auto by default.
	Inset geometry.EdgesOfLength

	// Size is the preferred width and height. Auto by default.
	Size SizeLength
	// MinSize is the minimum width and height. Auto lets layout derive a
	// content-based minimum.
	MinSize SizeLength
	// MaxSize is the maximum width and height. Auto means unbounded.
	MaxSize SizeLength
	// AspectRatio is the preferred width/height ratio, nil when unset.
	AspectRatio *float32

	// Margin is the outer spacing; may be auto. Zero by default.
	Margin geometry.EdgesOfLength
	// Padding is the inner spacing between border and content. Zero by default.
	Padding geometry.EdgesDefinite
	// BorderWidths are the per-edge border widths. Zero by default.
	BorderWidths geometry.EdgesAbsolute

	// AlignItems aligns children on the cross axis (CSS align-items).
	AlignItems *AlignItems
	// AlignSelf overrides the parent's AlignItems for this element.
	AlignSelf *AlignItems
	// AlignContent distributes wrapped flex lines on the cross axis.
	AlignContent *AlignContent
	// JustifyContent distributes children on the main axis.
	JustifyContent *AlignContent
	// Gap is the spacing between flex/grid children: Width is the column
	// gap, Height the row gap.
	Gap SizeDefinite

	// FlexDirection is the main axis direction. Row by default.
	FlexDirection FlexDirection
	// FlexWrap controls wrapping onto multiple lines. None by default.
	FlexWrap FlexWrap
	// FlexBasis is the initial main size of a flex item. Auto by default.
	FlexBasis geometry.Length
	// FlexGrow is the share of positive free space (default 0).
	FlexGrow float32
	// FlexShrink is the share of negative free space absorbed (default 1).
	FlexShrink float32

	// Background is the fill behind the element, nil for none.
	Background *gc.Background
	// BorderColor is the border color, nil for none. A border paints only
	// with a non-transparent color and a non-zero width.
	BorderColor *gc.Hsla
	// BorderStyle is solid or dashed.
	BorderStyle BorderStyle
	// CornerRadii are the per-corner radii, clamped to half the shorter
	// side at paint time.
	CornerRadii geometry.CornersAbsolute
	// BoxShadow lists the shadows in paint order. Empty for none.
	BoxShadow []BoxShadow

	// Text is the text style, inherited by descendants through the cascade.
	Text TextStyle
	// MouseCursor is the cursor requested while hovering, nil to inherit.
	MouseCursor *CursorStyle
	// Opacity is the element opacity in [0, 1], nil for fully opaque.
	Opacity *float32

	// GridCols and GridRows are the repeated track templates for grid
	// containers.
	GridCols *GridTemplate
	GridRows *GridTemplate
	// GridLocation places this element within its grid parent.
	GridLocation *geometry.GridLocation
}

// DefaultStyle returns the style every element starts from, matching the
// upstream defaults: display flex, direction row, no wrapping,
// flex-shrink 1, auto sizes and insets, zero margin/padding/border,
// visible overflow and [DefaultTextStyle] text.
func DefaultStyle() Style {
	return Style{
		Display:        DisplayFlex,
		Visibility:     VisibilityVisible,
		Overflow:       [2]Overflow{OverflowVisible, OverflowVisible},
		ScrollbarWidth: geometry.PixelsLength(geometry.PixelsZero),
		Position:       PositionRelative,
		Inset:          geometry.AutoEdgesOfLength(),
		Size:           AutoSizeLength(),
		MinSize:        AutoSizeLength(),
		MaxSize:        AutoSizeLength(),
		Margin:         geometry.ZeroEdgesOfLength(),
		Padding:        geometry.ZeroEdgesDefinite(),
		BorderWidths:   geometry.ZeroEdgesAbsolute(),
		FlexDirection:  FlexDirectionRow,
		FlexWrap:       FlexWrapNone,
		FlexBasis:      geometry.AutoLength(),
		FlexShrink:     1.0,
		CornerRadii:    geometry.ZeroCornersAbsolute(),
		Text:           DefaultTextStyle(),
	}
}

// IsBlock reports whether Display is [DisplayBlock].
func (s Style) IsBlock() bool { return s.Display == DisplayBlock }

// IsFlex reports whether Display is [DisplayFlex].
func (s Style) IsFlex() bool { return s.Display == DisplayFlex }

// IsGrid reports whether Display is [DisplayGrid].
func (s Style) IsGrid() bool { return s.Display == DisplayGrid }

// IsNone reports whether Display is [DisplayNone], i.e. the element is skipped
// by layout and paint.
func (s Style) IsNone() bool { return s.Display == DisplayNone }

// HasOpaqueBackground reports whether the background fully covers what is
// behind it. It returns false when Background is nil, true for any gradient
// or pattern, and for a solid fill only when its alpha is 1. Painters use it
// to skip drawing content underneath.
func (s Style) HasOpaqueBackground() bool {
	if s.Background == nil {
		return false
	}
	if s.Background.Tag != gc.BackgroundTagSolid {
		return true
	}
	return s.Background.Solid.A >= 1.0
}

// IsBorderVisible reports whether a border should paint: a border color is
// set, it is not transparent, and at least one edge width is non-zero after
// resolving rem widths against remSize (the window rem size in pixels).
func (s Style) IsBorderVisible(remSize geometry.Pixels) bool {
	if s.BorderColor == nil || s.BorderColor.IsTransparent() {
		return false
	}
	w := s.BorderWidths.ToPixels(remSize)
	return w.Top.AsF32() > 0 || w.Right.AsF32() > 0 || w.Bottom.AsF32() > 0 || w.Left.AsF32() > 0
}

// Refine applies ref on top of s: every field set in the refinement
// overwrites the corresponding field of s, unset (nil) fields leave s
// untouched. Composite refinements (size, margin, padding, border widths,
// insets, gap, corner radii, overflow, text) override per component, so
// setting only the width keeps the existing height. A nil ref is a no-op.
// Refine is how the cascade is resolved: apply the base style, then each
// state refinement (hover, active, ...) in order.
func (s *Style) Refine(ref *StyleRefinement) {
	if ref == nil {
		return
	}
	if ref.Display != nil {
		s.Display = *ref.Display
	}
	if ref.Visibility != nil {
		s.Visibility = *ref.Visibility
	}
	if ref.Overflow != nil {
		if ref.Overflow.X != nil {
			s.Overflow[0] = *ref.Overflow.X
		}
		if ref.Overflow.Y != nil {
			s.Overflow[1] = *ref.Overflow.Y
		}
	}
	if ref.ScrollbarWidth != nil {
		s.ScrollbarWidth = *ref.ScrollbarWidth
	}
	if ref.AllowConcurrentScroll != nil {
		s.AllowConcurrentScroll = *ref.AllowConcurrentScroll
	}
	if ref.RestrictScrollToAxis != nil {
		s.RestrictScrollToAxis = *ref.RestrictScrollToAxis
	}
	if ref.Position != nil {
		s.Position = *ref.Position
	}
	if ref.Inset != nil {
		refineEdgeLength(&s.Inset.Top, ref.Inset.Top)
		refineEdgeLength(&s.Inset.Right, ref.Inset.Right)
		refineEdgeLength(&s.Inset.Bottom, ref.Inset.Bottom)
		refineEdgeLength(&s.Inset.Left, ref.Inset.Left)
	}
	if ref.Size != nil {
		refineLength(&s.Size.Width, ref.Size.Width)
		refineLength(&s.Size.Height, ref.Size.Height)
	}
	if ref.MinSize != nil {
		refineLength(&s.MinSize.Width, ref.MinSize.Width)
		refineLength(&s.MinSize.Height, ref.MinSize.Height)
	}
	if ref.MaxSize != nil {
		refineLength(&s.MaxSize.Width, ref.MaxSize.Width)
		refineLength(&s.MaxSize.Height, ref.MaxSize.Height)
	}
	if ref.AspectRatio != nil {
		ar := *ref.AspectRatio
		s.AspectRatio = &ar
	}
	if ref.Margin != nil {
		refineEdgeLength(&s.Margin.Top, ref.Margin.Top)
		refineEdgeLength(&s.Margin.Right, ref.Margin.Right)
		refineEdgeLength(&s.Margin.Bottom, ref.Margin.Bottom)
		refineEdgeLength(&s.Margin.Left, ref.Margin.Left)
	}
	if ref.Padding != nil {
		refineEdgeDefinite(&s.Padding.Top, ref.Padding.Top)
		refineEdgeDefinite(&s.Padding.Right, ref.Padding.Right)
		refineEdgeDefinite(&s.Padding.Bottom, ref.Padding.Bottom)
		refineEdgeDefinite(&s.Padding.Left, ref.Padding.Left)
	}
	if ref.BorderWidths != nil {
		refineEdgeAbsolute(&s.BorderWidths.Top, ref.BorderWidths.Top)
		refineEdgeAbsolute(&s.BorderWidths.Right, ref.BorderWidths.Right)
		refineEdgeAbsolute(&s.BorderWidths.Bottom, ref.BorderWidths.Bottom)
		refineEdgeAbsolute(&s.BorderWidths.Left, ref.BorderWidths.Left)
	}
	if ref.AlignItems != nil {
		s.AlignItems = ref.AlignItems
	}
	if ref.AlignSelf != nil {
		s.AlignSelf = ref.AlignSelf
	}
	if ref.AlignContent != nil {
		s.AlignContent = ref.AlignContent
	}
	if ref.JustifyContent != nil {
		s.JustifyContent = ref.JustifyContent
	}
	if ref.Gap != nil {
		refineDefinite(&s.Gap.Width, ref.Gap.Width)
		refineDefinite(&s.Gap.Height, ref.Gap.Height)
	}
	if ref.FlexDirection != nil {
		s.FlexDirection = *ref.FlexDirection
	}
	if ref.FlexWrap != nil {
		s.FlexWrap = *ref.FlexWrap
	}
	if ref.FlexBasis != nil {
		s.FlexBasis = *ref.FlexBasis
	}
	if ref.FlexGrow != nil {
		s.FlexGrow = *ref.FlexGrow
	}
	if ref.FlexShrink != nil {
		s.FlexShrink = *ref.FlexShrink
	}
	if ref.Background != nil {
		s.Background = ref.Background
	}
	if ref.BorderColor != nil {
		s.BorderColor = ref.BorderColor
	}
	if ref.BorderStyle != nil {
		s.BorderStyle = *ref.BorderStyle
	}
	if ref.CornerRadii != nil {
		refineCornerAbsolute(&s.CornerRadii.TopLeft, ref.CornerRadii.TopLeft)
		refineCornerAbsolute(&s.CornerRadii.TopRight, ref.CornerRadii.TopRight)
		refineCornerAbsolute(&s.CornerRadii.BottomRight, ref.CornerRadii.BottomRight)
		refineCornerAbsolute(&s.CornerRadii.BottomLeft, ref.CornerRadii.BottomLeft)
	}
	if ref.BoxShadow != nil {
		s.BoxShadow = ref.BoxShadow
	}
	if ref.Text != nil && !ref.Text.IsEmpty() {
		s.Text.Refine(ref.Text)
	}
	if ref.MouseCursor != nil {
		s.MouseCursor = ref.MouseCursor
	}
	if ref.Opacity != nil {
		o := *ref.Opacity
		s.Opacity = &o
	}
	if ref.GridCols != nil {
		s.GridCols = ref.GridCols
	}
	if ref.GridRows != nil {
		s.GridRows = ref.GridRows
	}
	if ref.GridLocation != nil {
		s.GridLocation = ref.GridLocation
	}
}

func refineLength(dst *geometry.Length, src *geometry.Length) {
	if src != nil {
		*dst = *src
	}
}

func refineEdgeLength(dst *geometry.Length, src *geometry.Length) {
	if src != nil {
		*dst = *src
	}
}

func refineEdgeDefinite(dst *geometry.DefiniteLength, src *geometry.DefiniteLength) {
	if src != nil {
		*dst = *src
	}
}

func refineEdgeAbsolute(dst *geometry.AbsoluteLength, src *geometry.AbsoluteLength) {
	if src != nil {
		*dst = *src
	}
}

func refineDefinite(dst *geometry.DefiniteLength, src *geometry.DefiniteLength) {
	if src != nil {
		*dst = *src
	}
}

func refineCornerAbsolute(dst *geometry.AbsoluteLength, src *geometry.AbsoluteLength) {
	if src != nil {
		*dst = *src
	}
}

// ---------------------------------------------------------------------------
// StyleRefinement: the partial form of Style. Every field is optional; a nil
// field means "not set" and does not override during Refine.
// ---------------------------------------------------------------------------

// PointOverflowRefinement is the optional form of the per-axis overflow pair.
// A nil axis is left unchanged by [Style.Refine].
type PointOverflowRefinement struct {
	X, Y *Overflow
}

// SizeLengthRefinement is the optional form of [SizeLength]: a nil axis is
// left unchanged.
type SizeLengthRefinement struct {
	Width, Height *geometry.Length
}

// SizeDefiniteRefinement is the optional form of [SizeDefinite]: a nil axis is
// left unchanged.
type SizeDefiniteRefinement struct {
	Width, Height *geometry.DefiniteLength
}

// EdgesLengthRefinement is the optional form of [geometry.EdgesOfLength]
// (margin and inset): a nil edge is left unchanged.
type EdgesLengthRefinement struct {
	Top, Right, Bottom, Left *geometry.Length
}

// EdgesDefiniteRefinement is the optional form of [geometry.EdgesDefinite]
// (padding): a nil edge is left unchanged.
type EdgesDefiniteRefinement struct {
	Top, Right, Bottom, Left *geometry.DefiniteLength
}

// EdgesAbsoluteRefinement is the optional form of [geometry.EdgesAbsolute]
// (border widths): a nil edge is left unchanged.
type EdgesAbsoluteRefinement struct {
	Top, Right, Bottom, Left *geometry.AbsoluteLength
}

// CornersAbsoluteRefinement is the optional form of
// [geometry.CornersAbsolute] (corner radii): a nil corner is left unchanged.
type CornersAbsoluteRefinement struct {
	TopLeft, TopRight, BottomRight, BottomLeft *geometry.AbsoluteLength
}

// StyleRefinement is the partial form of [Style] used by the cascade and by
// the Styled builder API. Every field is optional: nil means "not set" and
// does not override anything when applied with [Style.Refine] or combined
// with [StyleRefinement.Merge]. The builder methods (W, P_4, Flex, ...) all
// live on *StyleRefinement, mutate it in place and return it for chaining.
// The zero value is an empty refinement, equivalent to [NewStyleRefinement].
//
// Fields mirror [Style]; see there for the meaning of each property.
// BoxShadow is distinguished by nil versus empty: nil leaves the shadow
// alone, an empty non-nil slice clears it.
type StyleRefinement struct {
	Display               *Display
	Visibility            *Visibility
	Overflow              *PointOverflowRefinement
	ScrollbarWidth        *geometry.AbsoluteLength
	AllowConcurrentScroll *bool
	RestrictScrollToAxis  *bool

	Position *Position
	Inset    *EdgesLengthRefinement

	Size        *SizeLengthRefinement
	MinSize     *SizeLengthRefinement
	MaxSize     *SizeLengthRefinement
	AspectRatio *float32

	Margin       *EdgesLengthRefinement
	Padding      *EdgesDefiniteRefinement
	BorderWidths *EdgesAbsoluteRefinement

	AlignItems     *AlignItems
	AlignSelf      *AlignItems
	AlignContent   *AlignContent
	JustifyContent *AlignContent
	Gap            *SizeDefiniteRefinement

	FlexDirection *FlexDirection
	FlexWrap      *FlexWrap
	FlexBasis     *geometry.Length
	FlexGrow      *float32
	FlexShrink    *float32

	Background  *gc.Background
	BorderColor *gc.Hsla
	BorderStyle *BorderStyle
	CornerRadii *CornersAbsoluteRefinement
	BoxShadow   []BoxShadow

	Text        *TextStyleRefinement
	MouseCursor *CursorStyle
	Opacity     *float32

	GridCols     *GridTemplate
	GridRows     *GridTemplate
	GridLocation *geometry.GridLocation
}

// NewStyleRefinement returns a pointer to an empty refinement, the usual
// starting point of a builder chain.
func NewStyleRefinement() *StyleRefinement { return &StyleRefinement{} }

// IsEmpty reports whether no property is set. A nil receiver is empty.
func (r *StyleRefinement) IsEmpty() bool {
	return r == nil || (r.Display == nil && r.Visibility == nil && r.Overflow == nil &&
		r.ScrollbarWidth == nil && r.AllowConcurrentScroll == nil && r.RestrictScrollToAxis == nil &&
		r.Position == nil && r.Inset == nil && r.Size == nil && r.MinSize == nil &&
		r.MaxSize == nil && r.AspectRatio == nil && r.Margin == nil && r.Padding == nil &&
		r.BorderWidths == nil && r.AlignItems == nil && r.AlignSelf == nil &&
		r.AlignContent == nil && r.JustifyContent == nil && r.Gap == nil &&
		r.FlexDirection == nil && r.FlexWrap == nil && r.FlexBasis == nil &&
		r.FlexGrow == nil && r.FlexShrink == nil && r.Background == nil &&
		r.BorderColor == nil && r.BorderStyle == nil && r.CornerRadii == nil &&
		r.BoxShadow == nil && r.Text.IsEmpty() && r.MouseCursor == nil &&
		r.Opacity == nil && r.GridCols == nil && r.GridRows == nil && r.GridLocation == nil)
}

// Merge returns a refinement equivalent to applying r and then other, so
// values set in other win. Composite fields (size, margin, padding, ...)
// merge per component. When either side is nil or empty the other is
// returned as is (not copied), so treat the result as shared; use
// [StyleRefinement.Clone] before mutating it. r itself is never modified.
func (r *StyleRefinement) Merge(other *StyleRefinement) *StyleRefinement {
	if r == nil || r.IsEmpty() {
		return other
	}
	if other == nil || other.IsEmpty() {
		return r
	}
	out := *r
	if other.Display != nil {
		out.Display = other.Display
	}
	if other.Visibility != nil {
		out.Visibility = other.Visibility
	}
	if other.Overflow != nil {
		if out.Overflow == nil {
			out.Overflow = other.Overflow
		} else {
			m := *out.Overflow
			if other.Overflow.X != nil {
				m.X = other.Overflow.X
			}
			if other.Overflow.Y != nil {
				m.Y = other.Overflow.Y
			}
			out.Overflow = &m
		}
	}
	if other.ScrollbarWidth != nil {
		out.ScrollbarWidth = other.ScrollbarWidth
	}
	if other.AllowConcurrentScroll != nil {
		out.AllowConcurrentScroll = other.AllowConcurrentScroll
	}
	if other.RestrictScrollToAxis != nil {
		out.RestrictScrollToAxis = other.RestrictScrollToAxis
	}
	if other.Position != nil {
		out.Position = other.Position
	}
	if other.Inset != nil {
		if out.Inset == nil {
			out.Inset = other.Inset
		} else {
			m := *out.Inset
			if other.Inset.Top != nil {
				m.Top = other.Inset.Top
			}
			if other.Inset.Right != nil {
				m.Right = other.Inset.Right
			}
			if other.Inset.Bottom != nil {
				m.Bottom = other.Inset.Bottom
			}
			if other.Inset.Left != nil {
				m.Left = other.Inset.Left
			}
			out.Inset = &m
		}
	}
	if other.Size != nil {
		if out.Size == nil {
			out.Size = other.Size
		} else {
			m := *out.Size
			if other.Size.Width != nil {
				m.Width = other.Size.Width
			}
			if other.Size.Height != nil {
				m.Height = other.Size.Height
			}
			out.Size = &m
		}
	}
	if other.MinSize != nil {
		if out.MinSize == nil {
			out.MinSize = other.MinSize
		} else {
			m := *out.MinSize
			if other.MinSize.Width != nil {
				m.Width = other.MinSize.Width
			}
			if other.MinSize.Height != nil {
				m.Height = other.MinSize.Height
			}
			out.MinSize = &m
		}
	}
	if other.MaxSize != nil {
		if out.MaxSize == nil {
			out.MaxSize = other.MaxSize
		} else {
			m := *out.MaxSize
			if other.MaxSize.Width != nil {
				m.Width = other.MaxSize.Width
			}
			if other.MaxSize.Height != nil {
				m.Height = other.MaxSize.Height
			}
			out.MaxSize = &m
		}
	}
	if other.AspectRatio != nil {
		out.AspectRatio = other.AspectRatio
	}
	if other.Margin != nil {
		if out.Margin == nil {
			out.Margin = other.Margin
		} else {
			m := *out.Margin
			if other.Margin.Top != nil {
				m.Top = other.Margin.Top
			}
			if other.Margin.Right != nil {
				m.Right = other.Margin.Right
			}
			if other.Margin.Bottom != nil {
				m.Bottom = other.Margin.Bottom
			}
			if other.Margin.Left != nil {
				m.Left = other.Margin.Left
			}
			out.Margin = &m
		}
	}
	if other.Padding != nil {
		if out.Padding == nil {
			out.Padding = other.Padding
		} else {
			m := *out.Padding
			if other.Padding.Top != nil {
				m.Top = other.Padding.Top
			}
			if other.Padding.Right != nil {
				m.Right = other.Padding.Right
			}
			if other.Padding.Bottom != nil {
				m.Bottom = other.Padding.Bottom
			}
			if other.Padding.Left != nil {
				m.Left = other.Padding.Left
			}
			out.Padding = &m
		}
	}
	if other.BorderWidths != nil {
		if out.BorderWidths == nil {
			out.BorderWidths = other.BorderWidths
		} else {
			m := *out.BorderWidths
			if other.BorderWidths.Top != nil {
				m.Top = other.BorderWidths.Top
			}
			if other.BorderWidths.Right != nil {
				m.Right = other.BorderWidths.Right
			}
			if other.BorderWidths.Bottom != nil {
				m.Bottom = other.BorderWidths.Bottom
			}
			if other.BorderWidths.Left != nil {
				m.Left = other.BorderWidths.Left
			}
			out.BorderWidths = &m
		}
	}
	if other.AlignItems != nil {
		out.AlignItems = other.AlignItems
	}
	if other.AlignSelf != nil {
		out.AlignSelf = other.AlignSelf
	}
	if other.AlignContent != nil {
		out.AlignContent = other.AlignContent
	}
	if other.JustifyContent != nil {
		out.JustifyContent = other.JustifyContent
	}
	if other.Gap != nil {
		if out.Gap == nil {
			out.Gap = other.Gap
		} else {
			m := *out.Gap
			if other.Gap.Width != nil {
				m.Width = other.Gap.Width
			}
			if other.Gap.Height != nil {
				m.Height = other.Gap.Height
			}
			out.Gap = &m
		}
	}
	if other.FlexDirection != nil {
		out.FlexDirection = other.FlexDirection
	}
	if other.FlexWrap != nil {
		out.FlexWrap = other.FlexWrap
	}
	if other.FlexBasis != nil {
		out.FlexBasis = other.FlexBasis
	}
	if other.FlexGrow != nil {
		out.FlexGrow = other.FlexGrow
	}
	if other.FlexShrink != nil {
		out.FlexShrink = other.FlexShrink
	}
	if other.Background != nil {
		out.Background = other.Background
	}
	if other.BorderColor != nil {
		out.BorderColor = other.BorderColor
	}
	if other.BorderStyle != nil {
		out.BorderStyle = other.BorderStyle
	}
	if other.CornerRadii != nil {
		if out.CornerRadii == nil {
			out.CornerRadii = other.CornerRadii
		} else {
			m := *out.CornerRadii
			if other.CornerRadii.TopLeft != nil {
				m.TopLeft = other.CornerRadii.TopLeft
			}
			if other.CornerRadii.TopRight != nil {
				m.TopRight = other.CornerRadii.TopRight
			}
			if other.CornerRadii.BottomRight != nil {
				m.BottomRight = other.CornerRadii.BottomRight
			}
			if other.CornerRadii.BottomLeft != nil {
				m.BottomLeft = other.CornerRadii.BottomLeft
			}
			out.CornerRadii = &m
		}
	}
	if other.BoxShadow != nil {
		out.BoxShadow = other.BoxShadow
	}
	if other.Text != nil {
		out.Text = out.Text.Merge(other.Text)
	}
	if other.MouseCursor != nil {
		out.MouseCursor = other.MouseCursor
	}
	if other.Opacity != nil {
		out.Opacity = other.Opacity
	}
	if other.GridCols != nil {
		out.GridCols = other.GridCols
	}
	if other.GridRows != nil {
		out.GridRows = other.GridRows
	}
	if other.GridLocation != nil {
		out.GridLocation = other.GridLocation
	}
	return &out
}

// Clone returns a copy of the refinement that can be mutated with the
// builder methods without affecting r: the composite sub-refinements and
// the BoxShadow slice are copied. Scalar values are shared, which is safe
// because the builders replace values instead of mutating them. Clone of a
// nil refinement returns nil.
func (r *StyleRefinement) Clone() *StyleRefinement {
	if r == nil {
		return nil
	}
	out := *r
	// Pointers are immutable once set (callers replace whole values), so a
	// shallow copy is safe except for nested refinement structs we own.
	if r.Overflow != nil {
		m := *r.Overflow
		out.Overflow = &m
	}
	if r.Inset != nil {
		m := *r.Inset
		out.Inset = &m
	}
	if r.Size != nil {
		m := *r.Size
		out.Size = &m
	}
	if r.MinSize != nil {
		m := *r.MinSize
		out.MinSize = &m
	}
	if r.MaxSize != nil {
		m := *r.MaxSize
		out.MaxSize = &m
	}
	if r.Margin != nil {
		m := *r.Margin
		out.Margin = &m
	}
	if r.Padding != nil {
		m := *r.Padding
		out.Padding = &m
	}
	if r.BorderWidths != nil {
		m := *r.BorderWidths
		out.BorderWidths = &m
	}
	if r.CornerRadii != nil {
		m := *r.CornerRadii
		out.CornerRadii = &m
	}
	if r.Gap != nil {
		m := *r.Gap
		out.Gap = &m
	}
	if r.Text != nil {
		m := *r.Text
		out.Text = &m
	}
	if r.BoxShadow != nil {
		out.BoxShadow = append([]BoxShadow(nil), r.BoxShadow...)
	}
	return &out
}
