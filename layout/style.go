// Package layout is a Go port of the taffy layout engine (v0.15) as used by
// gpui: flexbox, grid and block layout with percentage lengths, min/max
// constraints, measured leaves (text) and absolutely-positioned children.
//
// Differences from upstream taffy (documented in PORTING_STATUS.md):
// flex line collection uses the greedy algorithm (taffy's default path);
// baseline alignment falls back to start alignment; grid layout implements
// the common-correctness subset (see grid.go for the exact list of
// unsupported features: subgrid, masonry, named lines/areas, fit-content
// percentage arguments, indefinite auto-fill, and the percentage/intrinsic
// re-sizing passes); float layout is not ported.
package layout

import "math"

// Dimension is a length, a percentage of the parent, or auto.
type Dimension struct {
	kind    uint8 // 0 = auto, 1 = length, 2 = percent
	Length  float32
	Percent float32
}

// AutoDimension is the "auto" dimension.
func AutoDimension() Dimension { return Dimension{kind: 0} }

// LengthDimension is a fixed length.
func LengthDimension(v float32) Dimension { return Dimension{kind: 1, Length: v} }

// PercentDimension is a percentage of the parent.
func PercentDimension(v float32) Dimension { return Dimension{kind: 2, Percent: v} }

// IsAuto reports auto.
func (d Dimension) IsAuto() bool { return d.kind == 0 }

// IsLength reports a fixed length.
func (d Dimension) IsLength() bool { return d.kind == 1 }

// IsPercent reports a percentage.
func (d Dimension) IsPercent() bool { return d.kind == 2 }

// flexBasisAutoLike reports whether the flex basis behaves like `auto`:
// auto itself, or an explicit zero (length 0 / percent 0) whose main size
// comes from flexing rather than from content. Stretch handoff applies to
// these: CSS stretches any item whose cross size is auto, regardless of
// the main-axis basis, and a zero basis contributes no content to probe.
func flexBasisAutoLike(d Dimension) bool {
	if d.kind == 0 {
		return true
	}
	if d.kind == 1 {
		return d.Length == 0
	}
	return d.Percent == 0
}

// Resolve resolves against a parent size; ok=false for auto.
func (d Dimension) Resolve(parent float32) (float32, bool) {
	switch d.kind {
	case 1:
		return d.Length, true
	case 2:
		return d.Percent * parent, true
	}
	return 0, false
}

// MaybeResolve resolves or returns the fallback.
func (d Dimension) MaybeResolve(parent, fallback float32) float32 {
	if v, ok := d.Resolve(parent); ok {
		return v
	}
	return fallback
}

// ResolveLoose resolves or returns +inf (no maximum).
func (d Dimension) ResolveLoose(parent float32) float32 {
	if v, ok := d.Resolve(parent); ok {
		return v
	}
	return float32(math.Inf(1))
}

// EdgeDimension is a per-edge dimension value (top/right/bottom/left).
type EdgeDimension struct {
	Top, Right, Bottom, Left Dimension
}

// ZeroEdges is all-zero.
func ZeroEdges() EdgeDimension {
	return EdgeDimension{
		Top: LengthDimension(0), Right: LengthDimension(0),
		Bottom: LengthDimension(0), Left: LengthDimension(0),
	}
}

// AutoEdges is all-auto.
func AutoEdges() EdgeDimension {
	return EdgeDimension{
		Top: AutoDimension(), Right: AutoDimension(),
		Bottom: AutoDimension(), Left: AutoDimension(),
	}
}

func (e EdgeDimension) SumHorizontal(parent float32) float32 {
	return e.Left.MaybeResolve(parent, 0) + e.Right.MaybeResolve(parent, 0)
}

func (e EdgeDimension) SumVertical(parent float32) float32 {
	return e.Top.MaybeResolve(parent, 0) + e.Bottom.MaybeResolve(parent, 0)
}

func (e EdgeDimension) Resolve(parent float32) [4]float32 {
	return [4]float32{
		e.Top.MaybeResolve(parent, 0),
		e.Right.MaybeResolve(parent, 0),
		e.Bottom.MaybeResolve(parent, 0),
		e.Left.MaybeResolve(parent, 0),
	}
}

// Display selects the layout algorithm.
type Display uint8

const (
	DisplayBlock Display = 0
	DisplayFlex  Display = 1
	DisplayNone  Display = 2
	DisplayGrid  Display = 3
)

// Position selects in-flow vs absolute.
type Position uint8

const (
	PositionRelative Position = 0
	PositionAbsolute Position = 1
)

// FlexDirection is the main axis.
type FlexDirection uint8

const (
	FlexRow           FlexDirection = 0
	FlexColumn        FlexDirection = 1
	FlexRowReverse    FlexDirection = 2
	FlexColumnReverse FlexDirection = 3
)

// IsRow reports a horizontal main axis.
func (d FlexDirection) IsRow() bool { return d == FlexRow || d == FlexRowReverse }

// IsReverse reports a reversed axis.
func (d FlexDirection) IsReverse() bool { return d == FlexRowReverse || d == FlexColumnReverse }

// FlexWrap controls wrapping.
type FlexWrap uint8

const (
	FlexNoWrap      FlexWrap = 0
	FlexWrapOn      FlexWrap = 1
	FlexWrapReverse FlexWrap = 2
)

// AlignItems is cross-axis child alignment (and AlignSelf).
type AlignItems uint8

const (
	AlignStart     AlignItems = 0
	AlignEnd       AlignItems = 1
	AlignFlexStart AlignItems = 2
	AlignFlexEnd   AlignItems = 3
	AlignCenter    AlignItems = 4
	AlignBaseline  AlignItems = 5
	AlignStretch   AlignItems = 6
)

// Resolve maps FlexStart/FlexEnd to Start/End accounting for reversed flow.
func (a AlignItems) Resolve(reverse bool) AlignItems {
	switch a {
	case AlignFlexStart:
		if reverse {
			return AlignEnd
		}
		return AlignStart
	case AlignFlexEnd:
		if reverse {
			return AlignStart
		}
		return AlignEnd
	}
	return a
}

// AlignContent is line distribution (and JustifyContent).
type AlignContent uint8

const (
	AlignContentStart        AlignContent = 0
	AlignContentEnd          AlignContent = 1
	AlignContentFlexStart    AlignContent = 2
	AlignContentFlexEnd      AlignContent = 3
	AlignContentCenter       AlignContent = 4
	AlignContentStretch      AlignContent = 5
	AlignContentSpaceBetween AlignContent = 6
	AlignContentSpaceEvenly  AlignContent = 7
	AlignContentSpaceAround  AlignContent = 8
)

// Resolve maps FlexStart/FlexEnd to Start/End accounting for reversed flow.
func (a AlignContent) Resolve(reverse bool) AlignContent {
	switch a {
	case AlignContentFlexStart:
		if reverse {
			return AlignContentEnd
		}
		return AlignContentStart
	case AlignContentFlexEnd:
		if reverse {
			return AlignContentStart
		}
		return AlignContentEnd
	}
	return a
}

// GridTrackFnKind enumerates the kinds of grid track sizing function (one
// side of a minmax(min, max) pair). Mirrors taffy's
// MinSizingFunction/MaxSizingFunction keywords plus fr.
type GridTrackFnKind uint8

const (
	// GridTrackFnAuto is "auto": an intrinsic minimum (like min-content for
	// sizing purposes) with an automatic (stretchable) maximum.
	GridTrackFnAuto GridTrackFnKind = 0
	// GridTrackFnPx is a fixed length (px after conversion).
	GridTrackFnPx GridTrackFnKind = 1
	// GridTrackFnPercent is a percentage of the container's inner size.
	GridTrackFnPercent GridTrackFnKind = 2
	// GridTrackFnFr is a flexible fraction (valid as a max only).
	GridTrackFnFr GridTrackFnKind = 3
	// GridTrackFnMinContent is the min-content size.
	GridTrackFnMinContent GridTrackFnKind = 4
	// GridTrackFnMaxContent is the max-content size.
	GridTrackFnMaxContent GridTrackFnKind = 5
	// GridTrackFnFitContent is fit-content(arg): min(max-content, arg).
	GridTrackFnFitContent GridTrackFnKind = 6
)

// GridTrackFn is a single track sizing function.
type GridTrackFn struct {
	Kind  GridTrackFnKind
	Value float32 // px length, percent (0..1), fr factor or fit-content arg
}

// AutoGridTrackFn is the auto sizing function.
func AutoGridTrackFn() GridTrackFn { return GridTrackFn{Kind: GridTrackFnAuto} }

// PxGridTrackFn is a fixed length.
func PxGridTrackFn(px float32) GridTrackFn { return GridTrackFn{Kind: GridTrackFnPx, Value: px} }

// PercentGridTrackFn is a percentage of the inner container size.
func PercentGridTrackFn(p float32) GridTrackFn {
	return GridTrackFn{Kind: GridTrackFnPercent, Value: p}
}

// FrGridTrackFn is a flexible fraction.
func FrGridTrackFn(fr float32) GridTrackFn { return GridTrackFn{Kind: GridTrackFnFr, Value: fr} }

// MinContentGridTrackFn is min-content.
func MinContentGridTrackFn() GridTrackFn { return GridTrackFn{Kind: GridTrackFnMinContent} }

// MaxContentGridTrackFn is max-content.
func MaxContentGridTrackFn() GridTrackFn { return GridTrackFn{Kind: GridTrackFnMaxContent} }

// FitContentGridTrackFn is fit-content(arg).
func FitContentGridTrackFn(arg float32) GridTrackFn {
	return GridTrackFn{Kind: GridTrackFnFitContent, Value: arg}
}

// IsDefinite reports a fixed/percentage function that resolves without
// measuring content (percentages only when the basis is definite).
func (f GridTrackFn) IsDefinite(basisDefinite bool) bool {
	switch f.Kind {
	case GridTrackFnPx:
		return true
	case GridTrackFnPercent:
		return basisDefinite
	}
	return false
}

// IsIntrinsic reports a content-based function.
func (f GridTrackFn) IsIntrinsic() bool {
	return f.Kind == GridTrackFnAuto || f.Kind == GridTrackFnMinContent || f.Kind == GridTrackFnMaxContent
}

// GridTrack is a minmax(min, max) track sizing function. A bare CSS track
// ("100px") is minmax(px, px); "1fr" is minmax(auto, fr); "auto" is
// minmax(auto, auto).
type GridTrack struct {
	Min, Max GridTrackFn
}

// AutoGridTrack is minmax(auto, auto).
func AutoGridTrack() GridTrack { return GridTrack{Min: AutoGridTrackFn(), Max: AutoGridTrackFn()} }

// PxGridTrack is a fixed-size track (minmax(px, px)).
func PxGridTrack(px float32) GridTrack {
	return GridTrack{Min: PxGridTrackFn(px), Max: PxGridTrackFn(px)}
}

// PercentGridTrack is a percentage track (minmax(p, p)).
func PercentGridTrack(p float32) GridTrack {
	return GridTrack{Min: PercentGridTrackFn(p), Max: PercentGridTrackFn(p)}
}

// FrGridTrack is a flexible track (CSS "1fr" = minmax(auto, 1fr)).
func FrGridTrack(fr float32) GridTrack {
	return GridTrack{Min: AutoGridTrackFn(), Max: FrGridTrackFn(fr)}
}

// MinMaxGridTrack is an explicit minmax(min, max).
func MinMaxGridTrack(min, max GridTrackFn) GridTrack { return GridTrack{Min: min, Max: max} }

// GridRepeatKind selects the repetition mode of a GridSection.
type GridRepeatKind uint8

const (
	// GridRepeatNone: Tracks is a plain (non-repeated) track list.
	GridRepeatNone GridRepeatKind = 0
	// GridRepeatCount: Tracks repeats Count times.
	GridRepeatCount GridRepeatKind = 1
	// GridRepeatAutoFill: repeat(auto-fill, Tracks) — as many repetitions as
	// fit the definite container size.
	GridRepeatAutoFill GridRepeatKind = 2
	// GridRepeatAutoFit: like auto-fill, but repetitions without items
	// collapse to zero size.
	GridRepeatAutoFit GridRepeatKind = 3
)

// GridSection is one component of a grid-template track list: a plain list
// of tracks or a repeat() of them (taffy GridTemplateComponent).
type GridSection struct {
	Repeat GridRepeatKind
	Count  uint16      // used when Repeat == GridRepeatCount
	Tracks []GridTrack // the track list (repeated unit for repeat modes)
}

// SingleGridSection wraps one track without repetition.
func SingleGridSection(t GridTrack) GridSection { return GridSection{Tracks: []GridTrack{t}} }

// CountGridSection is repeat(n, tracks).
func CountGridSection(n uint16, tracks ...GridTrack) GridSection {
	return GridSection{Repeat: GridRepeatCount, Count: n, Tracks: tracks}
}

// AutoFillGridSection is repeat(auto-fill, tracks).
func AutoFillGridSection(tracks ...GridTrack) GridSection {
	return GridSection{Repeat: GridRepeatAutoFill, Tracks: tracks}
}

// AutoFitGridSection is repeat(auto-fit, tracks).
func AutoFitGridSection(tracks ...GridTrack) GridSection {
	return GridSection{Repeat: GridRepeatAutoFit, Tracks: tracks}
}

// GridPlacementKind mirrors taffy's GridPlacement: Auto, a grid Line, or a
// Span.
type GridPlacementKind uint8

const (
	GridPlacementAutoKind GridPlacementKind = 0
	GridPlacementLineKind GridPlacementKind = 1
	GridPlacementSpanKind GridPlacementKind = 2
)

// GridPlacement is one end of a grid item's placement in one axis.
type GridPlacement struct {
	Kind GridPlacementKind
	Line int16  // 1-based grid line (negative counts from the end); 0 invalid
	Span uint16 // number of tracks spanned (>= 1)
}

// AutoGridPlacement is the auto placement.
func AutoGridPlacement() GridPlacement { return GridPlacement{} }

// LineGridPlacement places at a grid line (negative counts from the end of
// the explicit grid).
func LineGridPlacement(line int16) GridPlacement {
	return GridPlacement{Kind: GridPlacementLineKind, Line: line}
}

// SpanGridPlacement spans the given number of tracks.
func SpanGridPlacement(span uint16) GridPlacement {
	if span < 1 {
		span = 1
	}
	return GridPlacement{Kind: GridPlacementSpanKind, Span: span}
}

// IsAuto reports the auto placement.
func (p GridPlacement) IsAuto() bool { return p.Kind == GridPlacementAutoKind }

// IsLine reports a line placement.
func (p GridPlacement) IsLine() bool { return p.Kind == GridPlacementLineKind && p.Line != 0 }

// IsSpan reports a span placement.
func (p GridPlacement) IsSpan() bool { return p.Kind == GridPlacementSpanKind }

// GridLineRange is an item's grid-row/grid-column (start/end pair).
type GridLineRange struct {
	Start, End GridPlacement
}

// GridAutoFlow controls the direction (and density) of auto-placement.
type GridAutoFlow uint8

const (
	GridAutoFlowRow         GridAutoFlow = 0
	GridAutoFlowColumn      GridAutoFlow = 1
	GridAutoFlowRowDense    GridAutoFlow = 2
	GridAutoFlowColumnDense GridAutoFlow = 3
)

// IsColumn reports column-major auto flow.
func (f GridAutoFlow) IsColumn() bool { return f == GridAutoFlowColumn || f == GridAutoFlowColumnDense }

// IsDense reports the dense packing algorithm.
func (f GridAutoFlow) IsDense() bool {
	return f == GridAutoFlowRowDense || f == GridAutoFlowColumnDense
}

// Overflow controls scroll-container sizing.
type Overflow uint8

const (
	OverflowVisible Overflow = 0
	OverflowClip    Overflow = 1
	OverflowHidden  Overflow = 2
	OverflowScroll  Overflow = 3
)

// SizeDimension is a width/height pair.
type SizeDimension struct {
	Width, Height Dimension
}

// AvailableSpace describes the space a node may lay out into. Each axis is
// independently definite, min-content or max-content (taffy
// AvailableSpace).
type AvailableSpace struct {
	kindW  uint8 // 0 = definite, 1 = min content, 2 = max content
	kindH  uint8
	Width  float32
	Height float32
}

// DefiniteAvailable is a definite size.
func DefiniteAvailable(w, h float32) AvailableSpace {
	return AvailableSpace{Width: w, Height: h}
}

// MinContentAvailable measures at min-content.
func MinContentAvailable() AvailableSpace {
	return AvailableSpace{kindW: 1, kindH: 1, Width: math.MaxFloat32, Height: math.MaxFloat32}
}

// MaxContentAvailable measures at max-content.
func MaxContentAvailable() AvailableSpace {
	return AvailableSpace{kindW: 2, kindH: 2, Width: math.MaxFloat32, Height: math.MaxFloat32}
}

// MaxMainDefiniteCrossAvailable measures the main axis at max-content while
// the cross axis is a definite size (container cross-availability probes).
// Passing a definite main of MaxFloat32 instead would report the main axis
// as definite and flex containers would grow their children into infinite
// free space.
func MaxMainDefiniteCrossAvailable(mainIsWidth bool, cross float32) AvailableSpace {
	if mainIsWidth {
		return AvailableSpace{kindW: 2, Width: math.MaxFloat32, Height: cross}
	}
	return AvailableSpace{kindH: 2, Height: math.MaxFloat32, Width: cross}
}

// IsDefiniteAxis reports a definite axis and its value.
func (a AvailableSpace) IsDefiniteAxis(isWidth bool) (float32, bool) {
	if isWidth {
		if a.kindW == 0 {
			return a.Width, true
		}
		return 0, false
	}
	if a.kindH == 0 {
		return a.Height, true
	}
	return 0, false
}

// Axis returns the available space along an axis.
func (a AvailableSpace) Axis(isWidth bool) float32 {
	if isWidth {
		return a.Width
	}
	return a.Height
}

// WithAxis replaces one axis.
func (a AvailableSpace) WithAxis(isWidth bool, v float32) AvailableSpace {
	if isWidth {
		a.Width = v
	} else {
		a.Height = v
	}
	return a
}

// WithDefiniteAxis replaces one axis with a definite value.
func (a AvailableSpace) WithDefiniteAxis(isWidth bool, v float32) AvailableSpace {
	if isWidth {
		a.kindW = 0
		a.Width = v
	} else {
		a.kindH = 0
		a.Height = v
	}
	return a
}

// WithHeightMaxContent marks the height axis indefinite (max-content).
func (a AvailableSpace) WithHeightMaxContent() AvailableSpace {
	a.kindH = 2
	return a
}

// WithWidthMaxContent marks the width axis indefinite (max-content).
func (a AvailableSpace) WithWidthMaxContent() AvailableSpace {
	a.kindW = 2
	return a
}

// MaybeResolve clamps a dimension into the available space like taffy's
// AvailableSpace::maybe_resolve: auto -> the available space if definite.
func (a AvailableSpace) MaybeResolve(d Dimension, isWidth bool) (float32, bool) {
	if v, ok := d.Resolve(a.Axis(isWidth)); ok {
		return v, true
	}
	if v, definite := a.IsDefiniteAxis(isWidth); definite {
		return v, true
	}
	return 0, false
}

// Style is the resolved layout style for one node.
type Style struct {
	Display  Display
	Position Position
	Overflow [2]Overflow // x, y

	InsetSize SizeDimension
	Inset     EdgeDimension

	Size    SizeDimension
	MinSize SizeDimension
	MaxSize SizeDimension

	Margin  EdgeDimension
	Padding EdgeDimension
	Border  EdgeDimension

	FlexDirection FlexDirection
	FlexWrap      FlexWrap
	FlexBasis     Dimension
	FlexGrow      float32
	FlexShrink    float32

	AlignItems     *AlignItems
	AlignSelf      *AlignItems
	AlignContent   *AlignContent
	JustifyContent *AlignContent

	// JustifyItems is the container default for item justify-self (grid
	// inline axis); JustifySelf overrides it per item.
	JustifyItems *AlignItems
	JustifySelf  *AlignItems

	Gap SizeDimension

	AspectRatio *float32

	// Grid track templates and per-item placement (grid layout). See
	// grid.go for the supported subset.
	GridTemplateColumns []GridSection
	GridTemplateRows    []GridSection
	// GridAutoColumns/GridAutoRows size implicit tracks (cycled, like
	// taffy grid_auto_columns/grid_auto_rows); empty = auto tracks.
	GridAutoColumns []GridTrack
	GridAutoRows    []GridTrack
	GridAutoFlow    GridAutoFlow
	// GridRow/GridColumn place this item (read from the item's own
	// style).
	GridRow    GridLineRange
	GridColumn GridLineRange
}

// DefaultStyle returns taffy defaults (flex, no-wrap, row).
func DefaultStyle() Style {
	return Style{
		Display:       DisplayFlex,
		Position:      PositionRelative,
		Overflow:      [2]Overflow{OverflowVisible, OverflowVisible},
		InsetSize:     SizeDimension{Width: AutoDimension(), Height: AutoDimension()},
		Inset:         AutoEdges(),
		Size:          SizeDimension{Width: AutoDimension(), Height: AutoDimension()},
		MinSize:       SizeDimension{Width: AutoDimension(), Height: AutoDimension()},
		MaxSize:       SizeDimension{Width: AutoDimension(), Height: AutoDimension()},
		Margin:        ZeroEdges(),
		Padding:       ZeroEdges(),
		Border:        ZeroEdges(),
		FlexDirection: FlexRow,
		FlexWrap:      FlexNoWrap,
		FlexBasis:     AutoDimension(),
		FlexShrink:    1,
		GridRow:       GridLineRange{Start: AutoGridPlacement(), End: AutoGridPlacement()},
		GridColumn:    GridLineRange{Start: AutoGridPlacement(), End: AutoGridPlacement()},
		GridAutoFlow:  GridAutoFlowRow,
	}
}
