package style

import (
	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
)

// SizeLength is a width/height pair of Length values.
type SizeLength struct {
	Width, Height geometry.Length
}

// SizeDefinite is a width/height pair of DefiniteLength values.
type SizeDefinite struct {
	Width, Height geometry.DefiniteLength
}

// AutoSizeLength is Size::auto() — both axes auto.
func AutoSizeLength() SizeLength {
	return SizeLength{Width: geometry.AutoLength(), Height: geometry.AutoLength()}
}

// FullSizeDefinite is Size::full() — both axes 100%.
func FullSizeDefinite() SizeDefinite {
	return SizeDefinite{Width: geometry.RelativeDefinite(1), Height: geometry.RelativeDefinite(1)}
}

// Style is the fully-resolved style of an element. Zero value is NOT the
// default; use DefaultStyle().
type Style struct {
	Display               Display
	Visibility            Visibility
	Overflow              [2]Overflow // x, y
	ScrollbarWidth        geometry.AbsoluteLength
	AllowConcurrentScroll bool
	RestrictScrollToAxis  bool

	Position Position
	Inset    geometry.EdgesOfLength

	Size        SizeLength
	MinSize     SizeLength
	MaxSize     SizeLength
	AspectRatio *float32

	Margin       geometry.EdgesOfLength
	Padding      geometry.EdgesDefinite
	BorderWidths geometry.EdgesAbsolute

	AlignItems     *AlignItems
	AlignSelf      *AlignItems
	AlignContent   *AlignContent
	JustifyContent *AlignContent
	Gap            SizeDefinite

	FlexDirection FlexDirection
	FlexWrap      FlexWrap
	FlexBasis     geometry.Length
	FlexGrow      float32
	FlexShrink    float32

	Background  *gc.Background
	BorderColor *gc.Hsla
	BorderStyle BorderStyle
	CornerRadii geometry.CornersAbsolute
	BoxShadow   []BoxShadow

	Text        TextStyle
	MouseCursor *CursorStyle
	Opacity     *float32

	GridCols     *GridTemplate
	GridRows     *GridTemplate
	GridLocation *geometry.GridLocation
}

// DefaultStyle returns the upstream defaults.
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

// IsBlock reports Display == Block.
func (s Style) IsBlock() bool { return s.Display == DisplayBlock }

// IsFlex reports Display == Flex.
func (s Style) IsFlex() bool { return s.Display == DisplayFlex }

// IsGrid reports Display == Grid.
func (s Style) IsGrid() bool { return s.Display == DisplayGrid }

// IsNone reports Display == None.
func (s Style) IsNone() bool { return s.Display == DisplayNone }

// HasOpaqueBackground reports a non-transparent solid background.
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
// set, it is not transparent, and at least one width is non-zero.
func (s Style) IsBorderVisible(remSize geometry.Pixels) bool {
	if s.BorderColor == nil || s.BorderColor.IsTransparent() {
		return false
	}
	w := s.BorderWidths.ToPixels(remSize)
	return w.Top.AsF32() > 0 || w.Right.AsF32() > 0 || w.Bottom.AsF32() > 0 || w.Left.AsF32() > 0
}

// Refine applies a partial style on top of this one; the refinement's set
// fields override, and nested refinements recurse.
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

// PointOverflowRefinement partially overrides [2]Overflow.
type PointOverflowRefinement struct {
	X, Y *Overflow
}

// SizeLengthRefinement partially overrides SizeLength.
type SizeLengthRefinement struct {
	Width, Height *geometry.Length
}

// SizeDefiniteRefinement partially overrides SizeDefinite.
type SizeDefiniteRefinement struct {
	Width, Height *geometry.DefiniteLength
}

// EdgesLengthRefinement partially overrides Length edges.
type EdgesLengthRefinement struct {
	Top, Right, Bottom, Left *geometry.Length
}

// EdgesDefiniteRefinement partially overrides DefiniteLength edges.
type EdgesDefiniteRefinement struct {
	Top, Right, Bottom, Left *geometry.DefiniteLength
}

// EdgesAbsoluteRefinement partially overrides AbsoluteLength edges.
type EdgesAbsoluteRefinement struct {
	Top, Right, Bottom, Left *geometry.AbsoluteLength
}

// CornersAbsoluteRefinement partially overrides AbsoluteLength corners.
type CornersAbsoluteRefinement struct {
	TopLeft, TopRight, BottomRight, BottomLeft *geometry.AbsoluteLength
}

// StyleRefinement is the partial (cascade) form of Style.
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

// NewStyleRefinement returns an empty refinement.
func NewStyleRefinement() *StyleRefinement { return &StyleRefinement{} }

// IsEmpty reports whether nothing is set.
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

// Merge returns a refinement equivalent to applying r then other (other
// wins), recursing into nested refinements.
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

// Clone deep-copies the refinement.
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
