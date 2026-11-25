// Enums of the gpui style system, ported from style.rs / scene.rs /
// platform.rs. Each mirrors a CSS-like concept used by the layout and paint
// pipelines.

package style

import (
	gc "github.com/FlameInTheDark/go-gpui/color"
)

// Display selects the layout algorithm used for an element's children, or
// removes the element from layout entirely (CSS display).
//
// The numeric values are part of the style API only; the layout package has
// its own Display type with different numbering and the gpui package converts
// between them.
type Display uint8

const (
	// DisplayBlock lays children out in the block flow.
	DisplayBlock Display = 0
	// DisplayFlex lays children out with flexbox (the gpui default).
	DisplayFlex Display = 1
	// DisplayGrid lays children out on a grid (limited support).
	DisplayGrid Display = 2
	// DisplayNone skips layout and paint entirely.
	DisplayNone Display = 3
)

// Visibility controls painting while keeping the element in layout (CSS
// visibility).
type Visibility uint8

const (
	// VisibilityVisible paints the element (default).
	VisibilityVisible Visibility = 0
	// VisibilityHidden skips painting but keeps layout space.
	VisibilityHidden Visibility = 1
)

// Position selects the positioning scheme (CSS position).
type Position uint8

const (
	// PositionRelative is the gpui default: the element participates in
	// layout and its inset offsets apply after layout.
	PositionRelative Position = 0
	// PositionAbsolute positions the element against its nearest positioned
	// ancestor; it is still laid out.
	PositionAbsolute Position = 1
)

// FlexDirection is the main-axis direction of a flex container (CSS
// flex-direction).
type FlexDirection uint8

const (
	// FlexDirectionRow lays out left to right (default).
	FlexDirectionRow FlexDirection = 0
	// FlexDirectionColumn lays out top to bottom.
	FlexDirectionColumn FlexDirection = 1
	// FlexDirectionRowReverse lays out right to left.
	FlexDirectionRowReverse FlexDirection = 2
	// FlexDirectionColumnReverse lays out bottom to top.
	FlexDirectionColumnReverse FlexDirection = 3
)

// IsReverse reports whether the direction is one of the reversed variants.
func (d FlexDirection) IsReverse() bool {
	return d == FlexDirectionRowReverse || d == FlexDirectionColumnReverse
}

// IsRow reports whether the main axis is horizontal (row or row-reverse).
func (d FlexDirection) IsRow() bool { return d == FlexDirectionRow || d == FlexDirectionRowReverse }

// FlexWrap controls whether flex children wrap onto several lines (CSS
// flex-wrap).
type FlexWrap uint8

const (
	// FlexWrapNone packs children on a single line (default).
	FlexWrapNone FlexWrap = 0
	// FlexWrapWrap wraps children onto new lines.
	FlexWrapWrap FlexWrap = 1
	// FlexWrapWrapReverse wraps in the reverse cross direction.
	FlexWrapWrapReverse FlexWrap = 2
)

// Overflow controls clipping and scrolling of content that exceeds the
// element bounds, per axis (CSS overflow-x / overflow-y).
type Overflow uint8

const (
	// OverflowVisible lets content draw outside the bounds and contribute
	// to the parent's scroll region.
	OverflowVisible Overflow = 0
	// OverflowClip keeps a content-based min size but does not contribute
	// to the parent's scroll region.
	OverflowClip Overflow = 1
	// OverflowHidden clips content and collapses the auto min size to zero.
	OverflowHidden Overflow = 2
	// OverflowScroll is like Hidden but scrollable and reserves
	// scrollbar_width space.
	OverflowScroll Overflow = 3
)

// AlignItems aligns children on the cross axis (CSS align-items). It is also
// the type of AlignSelf. Start/End are physical; FlexStart/FlexEnd follow
// the flow direction and swap under reversed flow.
type AlignItems uint8

const (
	// AlignItemsStart packs children at the start.
	AlignItemsStart AlignItems = 0
	// AlignItemsEnd packs children at the end.
	AlignItemsEnd AlignItems = 1
	// AlignItemsFlexStart resolves to Start (or End under reversed flow).
	AlignItemsFlexStart AlignItems = 2
	// AlignItemsFlexEnd resolves to End (or Start under reversed flow).
	AlignItemsFlexEnd AlignItems = 3
	// AlignItemsCenter centers children.
	//
	// The VALUES mirror layout.AlignItems (Start 0, End 1, FlexStart 2,
	// FlexEnd 3, Center 4, Baseline 5, Stretch 6) because
	// gpui.LayoutStyleOf converts with a raw numeric cast
	// (alignItemsPtr). Center and Baseline were swapped once (style put
	// Baseline at 4): every ItemsCenter/SelfCenter in the framework was
	// then laid out as baseline alignment - top-packed, never centered -
	// which misaligned tab icons, menu rows and buttons. A regression
	// test (gpui.TestStyleLayoutEnumsAgree) locks the tables together.
	AlignItemsCenter AlignItems = 4
	// AlignItemsBaseline aligns baselines.
	AlignItemsBaseline AlignItems = 5
	// AlignItemsStretch stretches children to fill the cross axis.
	AlignItemsStretch AlignItems = 6
)

// AlignContent distributes wrapped lines on the cross axis (CSS
// align-content). It is also the type of JustifyContent, which distributes
// children on the main axis.
type AlignContent uint8

const (
	// AlignContentStart packs lines at the start.
	AlignContentStart AlignContent = 0
	// AlignContentEnd packs lines at the end.
	AlignContentEnd AlignContent = 1
	// AlignContentFlexStart resolves to Start under reversed flow.
	AlignContentFlexStart AlignContent = 2
	// AlignContentFlexEnd resolves to End under reversed flow.
	AlignContentFlexEnd AlignContent = 3
	// AlignContentCenter centers lines.
	AlignContentCenter AlignContent = 4
	// AlignContentStretch stretches lines.
	AlignContentStretch AlignContent = 5
	// AlignContentSpaceBetween puts equal space between lines, edges flush.
	AlignContentSpaceBetween AlignContent = 6
	// AlignContentSpaceEvenly distributes equal space including edges.
	AlignContentSpaceEvenly AlignContent = 7
	// AlignContentSpaceAround gives edges half the inner gaps.
	AlignContentSpaceAround AlignContent = 8
)

// BorderStyle selects solid or dashed borders (CSS border-style).
type BorderStyle uint8

const (
	// BorderStyleSolid is a solid border (default).
	BorderStyleSolid BorderStyle = 0
	// BorderStyleDashed is a dashed border.
	BorderStyleDashed BorderStyle = 1
)

// CursorStyle mirrors the platform mouse cursor vocabulary. It is what
// [StyleRefinement.CursorPointer] and friends store in the style; the window
// translates it to the native cursor.
type CursorStyle uint8

const (
	// CursorArrow is the default arrow.
	CursorArrow CursorStyle = iota
	// CursorIBeam is the text-selection I-beam.
	CursorIBeam
	// CursorCrosshair is a precise-picking crosshair.
	CursorCrosshair
	// CursorClosedHand is the closed (grabbing) hand.
	CursorClosedHand
	// CursorOpenHand is the open (grab) hand.
	CursorOpenHand
	// CursorPointingHand is the pointing hand used over links and buttons.
	CursorPointingHand
	// CursorResizeLeft resizes the left edge.
	CursorResizeLeft
	// CursorResizeRight resizes the right edge.
	CursorResizeRight
	// CursorResizeLeftRight is the horizontal two-way resize cursor.
	CursorResizeLeftRight
	// CursorResizeUp resizes the top edge.
	CursorResizeUp
	// CursorResizeDown resizes the bottom edge.
	CursorResizeDown
	// CursorResizeUpDown is the vertical two-way resize cursor.
	CursorResizeUpDown
	// CursorResizeUpLeftDownRight is the diagonal resize cursor running
	// from top-left to bottom-right.
	CursorResizeUpLeftDownRight
	// CursorResizeUpRightDownLeft is the diagonal resize cursor running
	// from top-right to bottom-left.
	CursorResizeUpRightDownLeft
	// CursorResizeColumn resizes a column (horizontal splitter).
	CursorResizeColumn
	// CursorResizeRow resizes a row (vertical splitter).
	CursorResizeRow
	// CursorIBeamCursorForVerticalLayout is the I-beam for vertical text.
	CursorIBeamCursorForVerticalLayout
	// CursorOperationNotAllowed signals a disallowed operation.
	CursorOperationNotAllowed
	// CursorDragLink signals that dropping creates a link.
	CursorDragLink
	// CursorDragCopy signals that dropping copies the dragged item.
	CursorDragCopy
	// CursorContextualMenu signals that a context menu is available.
	CursorContextualMenu
)

// MouseCursor is the semantic mouse cursor an element requests while the
// pointer hovers it (upstream crate::MouseCursor). Elements set it through
// the Styled API; the window resolves the hovered request each frame and
// applies it through the platform (upstream Window::reset_cursor_style).
type MouseCursor uint8

const (
	// MouseCursorDefault is the platform arrow (upstream MouseCursor::Default).
	MouseCursorDefault MouseCursor = 0
	// MouseCursorPointingHand marks clickable content (upstream
	// MouseCursor::CursorPointingHand).
	MouseCursorPointingHand MouseCursor = 1
	// MouseCursorText marks selectable text (upstream MouseCursor::IBeam).
	MouseCursorText MouseCursor = 2
	// MouseCursorResizeLeftRight marks horizontal resizers.
	MouseCursorResizeLeftRight MouseCursor = 3
	// MouseCursorResizeUpDown marks vertical resizers.
	MouseCursorResizeUpDown MouseCursor = 4
	// MouseCursorCrosshair marks precise picking.
	MouseCursorCrosshair MouseCursor = 5
	// MouseCursorNotAllowed marks disallowed actions.
	MouseCursorNotAllowed MouseCursor = 6
	// MouseCursorGrab marks grabbable content (open hand).
	MouseCursorGrab MouseCursor = 7
	// MouseCursorGrabbing marks an active grab (closed hand).
	MouseCursorGrabbing MouseCursor = 8
)

// ToCursorStyle maps the semantic cursor to the platform vocabulary
// (upstream MouseCursor::to_cursor_style).
func (m MouseCursor) ToCursorStyle() CursorStyle {
	switch m {
	case MouseCursorPointingHand:
		return CursorPointingHand
	case MouseCursorText:
		return CursorIBeam
	case MouseCursorResizeLeftRight:
		return CursorResizeLeftRight
	case MouseCursorResizeUpDown:
		return CursorResizeUpDown
	case MouseCursorCrosshair:
		return CursorCrosshair
	case MouseCursorNotAllowed:
		return CursorOperationNotAllowed
	case MouseCursorGrab:
		return CursorOpenHand
	case MouseCursorGrabbing:
		return CursorClosedHand
	default:
		return CursorArrow
	}
}

// MouseCursorForStyle maps a platform cursor style back to the closest
// semantic cursor (best-effort inverse of ToCursorStyle; unmapped styles
// resolve to MouseCursorDefault). The window uses it to hand the resolved
// hover cursor to platform backends.
func MouseCursorForStyle(c CursorStyle) MouseCursor {
	switch c {
	case CursorPointingHand:
		return MouseCursorPointingHand
	case CursorIBeam, CursorIBeamCursorForVerticalLayout:
		return MouseCursorText
	case CursorResizeLeftRight, CursorResizeLeft, CursorResizeRight, CursorResizeColumn:
		return MouseCursorResizeLeftRight
	case CursorResizeUpDown, CursorResizeUp, CursorResizeDown, CursorResizeRow:
		return MouseCursorResizeUpDown
	case CursorCrosshair:
		return MouseCursorCrosshair
	case CursorOperationNotAllowed:
		return MouseCursorNotAllowed
	case CursorOpenHand:
		return MouseCursorGrab
	case CursorClosedHand:
		return MouseCursorGrabbing
	default:
		return MouseCursorDefault
	}
}

// ObjectFit selects how an image is fitted into its bounds (CSS
// object-fit).
type ObjectFit uint8

const (
	// ObjectFitFill stretches to fill (distorting).
	ObjectFitFill ObjectFit = 0
	// ObjectFitContain fits inside, centered.
	ObjectFitContain ObjectFit = 1
	// ObjectFitCover fills and crops, centered.
	ObjectFitCover ObjectFit = 2
	// ObjectFitScaleDown is Contain but never upscales.
	ObjectFitScaleDown ObjectFit = 3
	// ObjectFitNone draws at natural size, centered.
	ObjectFitNone ObjectFit = 4
)

// WhiteSpace controls text wrapping (CSS white-space, restricted to
// normal and nowrap).
type WhiteSpace uint8

const (
	// WhiteSpaceNormal wraps text at the wrap width (default).
	WhiteSpaceNormal WhiteSpace = 0
	// WhiteSpaceNowrap never wraps.
	WhiteSpaceNowrap WhiteSpace = 1
)

// TextOverflow selects how text that does not fit is truncated (CSS
// text-overflow, extended with start and middle truncation).
type TextOverflow struct {
	// Kind selects which part of the text is cut.
	Kind TextOverflowKind
	// Affix is the replacement text inserted where the text was cut,
	// usually [DefaultEllipsis].
	Affix string
}

// TextOverflowKind selects where a [TextOverflow] cuts the text.
type TextOverflowKind uint8

const (
	// TextOverflowTruncate cuts the end and appends the affix.
	TextOverflowTruncate TextOverflowKind = 0
	// TextOverflowTruncateStart cuts the beginning.
	TextOverflowTruncateStart TextOverflowKind = 1
	// TextOverflowTruncateMiddle cuts the middle.
	TextOverflowTruncateMiddle TextOverflowKind = 2
)

// DefaultEllipsis is the default truncation affix, the single character
// U+2026 HORIZONTAL ELLIPSIS.
const DefaultEllipsis = "…"

// TextAlign selects horizontal text alignment (CSS text-align).
type TextAlign uint8

const (
	// TextAlignLeft aligns to the leading edge (default).
	TextAlignLeft TextAlign = 0
	// TextAlignCenter centers each line.
	TextAlignCenter TextAlign = 1
	// TextAlignRight aligns to the trailing edge.
	TextAlignRight TextAlign = 2
)

// BoxShadow is a single box shadow (CSS box-shadow). Paint order: drop
// shadows, background, inset shadows. All lengths are logical pixels.
type BoxShadow struct {
	// Color is the shadow color, including its opacity.
	Color gc.Hsla
	// OffsetX and OffsetY move the shadow right and down; negative values
	// move it left and up.
	OffsetX float32
	OffsetY float32
	// BlurRadius softens the edge; 0 is a hard edge.
	BlurRadius float32
	// SpreadRadius grows (positive) or shrinks (negative) the shadow shape
	// before blurring.
	SpreadRadius float32
	// Inset paints the shadow inside the box instead of behind it.
	Inset bool
}

// NewBoxShadow constructs a drop shadow with the given offset (logical
// pixels) and color, no blur and no spread. Chain [BoxShadow.WithBlurRadius],
// [BoxShadow.WithSpreadRadius] and [BoxShadow.WithInset] to refine it.
func NewBoxShadow(offsetX, offsetY float32, color gc.Hsla) BoxShadow {
	return BoxShadow{Color: color, OffsetX: offsetX, OffsetY: offsetY}
}

// WithBlurRadius returns a copy of the shadow with the blur radius set, in
// logical pixels.
func (b BoxShadow) WithBlurRadius(r float32) BoxShadow { b.BlurRadius = r; return b }

// WithSpreadRadius returns a copy of the shadow with the spread radius set,
// in logical pixels; negative values shrink the shadow.
func (b BoxShadow) WithSpreadRadius(r float32) BoxShadow { b.SpreadRadius = r; return b }

// WithInset returns a copy of the shadow marked as an inset (inner) shadow.
func (b BoxShadow) WithInset() BoxShadow { b.Inset = true; return b }

// GridTemplateMinSize selects how the minimum track size of a repeated grid
// template is computed.
type GridTemplateMinSize uint8

const (
	// GridTemplateMinSizeZero makes tracks minmax(0, 1fr).
	GridTemplateMinSizeZero GridTemplateMinSize = 0
	// GridTemplateMinSizeMinContent makes tracks minmax(min-content, 1fr).
	GridTemplateMinSizeMinContent GridTemplateMinSize = 1
	// GridTemplateMinSizeMaxContent makes tracks minmax(0, max-content).
	GridTemplateMinSizeMaxContent GridTemplateMinSize = 2
)

// GridTemplate describes a repeated track pattern: Repeat equal tracks whose
// sizing is selected by MinSize. It is the value behind
// [StyleRefinement.SetGridCols] and [StyleRefinement.SetGridRows].
type GridTemplate struct {
	// Repeat is the number of tracks (CSS repeat(n, ...)).
	Repeat uint16
	// MinSize selects the track sizing function.
	MinSize GridTemplateMinSize
}

// AnchorCorner names the corner (or edge center) of a box used to pin one
// element against another (upstream crate::Anchor; consumed by the Anchored
// element, see elements/anchored.go).
type AnchorCorner uint8

const (
	// AnchorCornerTopLeft names the top-left corner (upstream Anchor::TopLeft).
	AnchorCornerTopLeft AnchorCorner = iota
	// AnchorCornerTopRight names the top-right corner (upstream Anchor::TopRight).
	AnchorCornerTopRight
	// AnchorCornerBottomLeft names the bottom-left corner (upstream Anchor::BottomLeft).
	AnchorCornerBottomLeft
	// AnchorCornerBottomRight names the bottom-right corner (upstream Anchor::BottomRight).
	AnchorCornerBottomRight
	// AnchorCornerTopCenter names the center of the top edge (upstream Anchor::TopCenter).
	AnchorCornerTopCenter
	// AnchorCornerBottomCenter names the center of the bottom edge (upstream Anchor::BottomCenter).
	AnchorCornerBottomCenter
)

// FlipHorizontal mirrors the corner across the vertical axis, upstream
// Anchor::other_side_along(Axis::Horizontal): TopLeft <-> TopRight, the
// center anchors keep their horizontal side.
func (c AnchorCorner) FlipHorizontal() AnchorCorner {
	switch c {
	case AnchorCornerTopLeft, AnchorCornerBottomLeft:
		return c + 1
	case AnchorCornerTopRight, AnchorCornerBottomRight:
		return c - 1
	}
	return c
}

// FlipVertical mirrors the corner across the horizontal axis, upstream
// Anchor::other_side_along(Axis::Vertical): TopLeft <-> BottomLeft, the
// center anchors keep their vertical side.
func (c AnchorCorner) FlipVertical() AnchorCorner {
	switch c {
	case AnchorCornerTopLeft:
		return AnchorCornerBottomLeft
	case AnchorCornerTopRight:
		return AnchorCornerBottomRight
	case AnchorCornerBottomLeft:
		return AnchorCornerTopLeft
	case AnchorCornerBottomRight:
		return AnchorCornerTopRight
	}
	return c
}
