package elements

import (
	"sync"

	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/style"
)

// ---------------------------------------------------------------------------
// Anchored (upstream elements/anchored.rs, condensed)
// ---------------------------------------------------------------------------

// anchoredPlacement selects where the anchored child pins (upstream
// Placement).
type anchoredPlacement struct {
	// kind: 0 = InPlace, 1 = At(position), 2 = Attached{target, corner}
	kind   uint8
	point  geometry.Point[geometry.Pixels]  // At
	target geometry.Bounds[geometry.Pixels] // Attached
	corner style.AnchorCorner               // Attached
}

// Anchored displays a child element that avoids overflowing the window
// bounds (upstream anchored()). During prepaint the child is measured and
// positioned so its AnchorCorner sits on the anchor point; when the placed
// child would overflow the window it flips vertically/horizontally to the
// side with room (upstream SwitchAnchor fit mode), or is clamped to the
// window edges when SnapToWindow is set (upstream SnapToWindow).
//
// The anchor point is, in order:
//   - Position(p) — an explicit point in window coordinates (upstream
//     Placement::At),
//   - Attach(target, corner) — a corner of an explicit target rect (upstream
//     Placement::Attached, with the flip-to-the-other-side behavior),
//   - the corner of the element registered under Anchor(id) (resolved each
//     frame through the window's element bounds table, gpui.Window.ElementBounds),
//   - otherwise the Anchored element's own layout origin (upstream
//     Placement::InPlace).
//
// The anchored child is measured and participates in layout like upstream;
// the wrapper itself lays out absolutely (upstream Style { position:
// Absolute, display: Flex }). Position the Anchored inside a Deferred (see
// elements.Deferred) when it should draw above the rest of the tree, and
// register the anchor element with RegisterAnchor before the Anchored
// prepaints (a sibling rendered earlier in tree order works).
type Anchored struct {
	st           *style.StyleRefinement
	child        gpui.Element
	corner       style.AnchorCorner
	snap         bool
	snapEdges    geometry.Edges[geometry.Pixels]
	hasSnapEdges bool
	anchor       any
	offset       geometry.Point[geometry.Pixels]
	placement    *anchoredPlacement
	// localPosition interprets Position/Attach coordinates relative to the
	// parent element instead of the window (upstream
	// AnchoredPositionMode::Local).
	localPosition bool

	// placedOrigin records the computed child origin for the current frame
	// (prepaint -> paint handoff, like List.items).
	placedOrigin geometry.Point[geometry.Pixels]
}

// NewAnchored wraps child in an anchored element.
func NewAnchored(child gpui.Element) *Anchored {
	return &Anchored{st: style.NewStyleRefinement(), child: child}
}

// AnchorCorner sets which corner (or edge center) of the child is pinned to
// the anchor point (upstream Anchored::anchor).
func (a *Anchored) AnchorCorner(corner style.AnchorCorner) *Anchored {
	a.corner = corner
	return a
}

// SnapToWindow controls the fit mode: when true the child snaps to the
// window edges instead of switching the anchor corner on overflow (upstream
// Anchored::snap_to_window); when false (the default) the corner flips to
// the other side when the flip fits (upstream AnchoredFitMode::SwitchAnchor).
func (a *Anchored) SnapToWindow(v bool) *Anchored {
	a.snap = v
	return a
}

// Anchor pins the child to a registered anchor element: the child's corner
// is placed on the matching corner of the anchor's bounds (upstream has no
// id lookup — it takes explicit positions via Anchored::position/attach;
// the port resolves ids through RegisterAnchor + the window's element bounds
// table). The id must be hashable (string, int, gpui.ElementId, ...). Pass
// an gpui.Element directly to skip the registry.
func (a *Anchored) Anchor(id any) *Anchored {
	a.anchor = id
	return a
}

// Position sets an explicit anchor point in window coordinates (upstream
// Anchored::position / Placement::At); without it, the location where the
// anchored element is rendered is used (Placement::InPlace).
func (a *Anchored) Position(p geometry.Point[geometry.Pixels]) *Anchored {
	a.placement = &anchoredPlacement{kind: 1, point: p}
	return a
}

// Attach places the element with its AnchorCorner on the given corner of
// target (upstream Anchored::attach): a submenu puts its top-left corner
// on its row's top-right corner. If the element does not fit in the window
// it moves to the other side of the target, as long as that side fits
// better; the offset flips with it. SwitchAnchor has no effect here.
func (a *Anchored) Attach(target geometry.Bounds[geometry.Pixels], corner style.AnchorCorner) *Anchored {
	a.placement = &anchoredPlacement{kind: 2, target: target, corner: corner}
	return a
}

// PositionModeLocal interprets Position/Attach coordinates as relative to
// the parent element (upstream AnchoredPositionMode::Local); the default
// interprets them relative to the window.
func (a *Anchored) PositionModeLocal(local bool) *Anchored {
	a.localPosition = local
	return a
}

// SnapToWindowWithMargin snaps to the window edges and leaves margins
// (upstream Anchored::snap_to_window_with_margin).
func (a *Anchored) SnapToWindowWithMargin(edges geometry.Edges[geometry.Pixels]) *Anchored {
	a.snap = true
	a.hasSnapEdges = true
	a.snapEdges = edges
	return a
}

// Offset shifts the final position by the given amount (upstream
// Anchored::offset); the offset flips with the corner when the placement
// flips.
func (a *Anchored) Offset(p geometry.Point[geometry.Pixels]) *Anchored {
	a.offset = p
	return a
}

// StyleRef returns the refinement.
func (a *Anchored) StyleRef() *style.StyleRefinement { return a.st }

// ElementID implements Element (upstream Anchored has no id).
func (a *Anchored) ElementID() gpui.ElementId { return gpui.ElementId{} }

// RequestLayout implements Element. The wrapper is absolutely positioned
// (it does not reserve flow space) and shrink-wraps the measured child.
func (a *Anchored) RequestLayout(w *gpui.Window) {
	st := style.DefaultStyle()
	st.Refine(a.st)
	st.Position = style.PositionAbsolute
	children := []gpui.Element{a.child}
	w.RequestLayout(st, children)
}

// Prepaint implements Element: measure the child, resolve the anchor point,
// flip/clamp against the window bounds and prepaint the child at the
// resulting offset (upstream Anchored::prepaint).
func (a *Anchored) Prepaint(w *gpui.Window, bounds geometry.Bounds[geometry.Pixels]) {
	if a.child == nil {
		return
	}

	// Child size from the layout tree (upstream unions the children's
	// layout_bounds).
	childSize := bounds.Size
	if node, ok := w.NodeOf(a.child); ok {
		childSize = w.LayoutBounds(node).Size
	}

	// Anchor point (upstream Placement / AnchoredPositionMode).
	var anchorPoint *geometry.Point[geometry.Pixels]
	if a.placement != nil {
		switch a.placement.kind {
		case 1: // At
			p := a.placement.point
			anchorPoint = &p
		case 2: // Attached: the named corner of the target
			p := anchorCornerPoint(a.placement.corner, a.placement.target)
			anchorPoint = &p
		}
	} else if a.anchor != nil {
		if el := lookupAnchor(w, a.anchor); el != nil {
			if b := w.ElementBounds(el); b.Size.Width.AsF32() > 0 || b.Size.Height.AsF32() > 0 {
				p := anchorCornerPoint(a.corner, b)
				anchorPoint = &p
			}
		}
	}

	// get_position_and_bounds (upstream AnchoredPositionMode): Window
	// mode anchors at the point (falling back to this element's layout
	// origin); Local mode adds the parent element's origin.
	origin := bounds.Origin
	var desired geometry.Bounds[geometry.Pixels]
	compute := func(anchorPoint *geometry.Point[geometry.Pixels], corner style.AnchorCorner, offset geometry.Point[geometry.Pixels]) (geometry.Point[geometry.Pixels], geometry.Bounds[geometry.Pixels]) {
		if a.localPosition {
			base := bounds.Origin
			if anchorPoint == nil {
				base = geometry.Point[geometry.Pixels]{}
			}
			p := geometry.Point[geometry.Pixels]{
				X: base.X.Add(anchorPointOrZero(anchorPoint).X).Add(offset.X),
				Y: base.Y.Add(anchorPointOrZero(anchorPoint).Y).Add(offset.Y),
			}
			return p, boundsFromAnchorAndSize(corner, p, childSize)
		}
		p := bounds.Origin
		if anchorPoint != nil {
			p = *anchorPoint
		}
		p = geometry.Point[geometry.Pixels]{X: p.X.Add(offset.X), Y: p.Y.Add(offset.Y)}
		return p, boundsFromAnchorAndSize(corner, p, childSize)
	}
	origin, desired = compute(anchorPoint, a.corner, a.offset)

	// Limits: the window viewport (upstream limits = Bounds { origin:
	// Point::default(), size: window.viewport_size() }).
	limits := geometry.Bounds[geometry.Pixels]{Size: w.LogicalContentSize()}

	// Collision avoidance (upstream prepaint).
	if a.placement != nil && a.placement.kind == 2 {
		// Attached: flip to the other side of the target when this
		// side overflows and the flipped side fits better (upstream
		// Placement::Attached handling; SwitchAnchor has no effect).
		overflowAlong := func(b geometry.Bounds[geometry.Pixels], horizontal bool) geometry.Pixels {
			if horizontal {
				l := maxPx(limits.Left().Sub(b.Left()), 0)
				r := maxPx(b.Right().Sub(limits.Right()), 0)
				return l.Add(r)
			}
			t := maxPx(limits.Top().Sub(b.Top()), 0)
			bo := maxPx(b.Bottom().Sub(limits.Bottom()), 0)
			return t.Add(bo)
		}
		// isOutsideTarget: the anchor corner and the target corner are
		// on opposite sides along the axis (upstream side_along).
		isOutsideTarget := func(corner, targetCorner style.AnchorCorner, horizontal bool) bool {
			return sideOf(corner, horizontal) != 0 && sideOf(corner, horizontal) == -sideOf(targetCorner, horizontal)
		}
		corner := a.corner
		targetCorner := a.placement.corner
		offset := a.offset
		for _, horizontal := range []bool{true, false} {
			if !isOutsideTarget(corner, targetCorner, horizontal) {
				continue
			}
			overflow := overflowAlong(desired, horizontal)
			if overflow == 0 {
				continue
			}
			flippedCorner, flippedTarget := corner, targetCorner
			if horizontal {
				flippedCorner = corner.FlipHorizontal()
				flippedTarget = targetCorner.FlipHorizontal()
			} else {
				flippedCorner = corner.FlipVertical()
				flippedTarget = targetCorner.FlipVertical()
			}
			flippedOffset := offset
			if horizontal {
				flippedOffset.X = -flippedOffset.X
			} else {
				flippedOffset.Y = -flippedOffset.Y
			}
			flippedTargetPoint := anchorCornerPoint(flippedTarget, a.placement.target)
			_, flipped := compute(&flippedTargetPoint, flippedCorner, flippedOffset)
			if overflowAlong(flipped, horizontal) < overflow {
				corner = flippedCorner
				targetCorner = flippedTarget
				offset = flippedOffset
				desired = flipped
			}
		}
	} else if !a.snap {
		// SwitchAnchor flips the corner per axis only when the
		// flipped placement fits (upstream AnchoredFitMode::SwitchAnchor).
		corner := a.corner
		if desired.Left().AsF32() < limits.Left().AsF32() || desired.Right().AsF32() > limits.Right().AsF32() {
			switched := boundsFromAnchorAndSize(corner.FlipHorizontal(), origin, childSize)
			if switched.Left().AsF32() >= limits.Left().AsF32() && switched.Right().AsF32() <= limits.Right().AsF32() {
				corner = corner.FlipHorizontal()
				desired = switched
			}
		}
		if desired.Top().AsF32() < limits.Top().AsF32() || desired.Bottom().AsF32() > limits.Bottom().AsF32() {
			switched := boundsFromAnchorAndSize(corner.FlipVertical(), origin, childSize)
			if switched.Top().AsF32() >= limits.Top().AsF32() && switched.Bottom().AsF32() <= limits.Bottom().AsF32() {
				desired = switched
			}
		}
	}

	// Snap the child to the window edges when it still overflows, aligning
	// to the left/top when it is larger than the window (upstream
	// snapping; margins from SnapToWindowWithMargin).
	edgeL, edgeR := a.snapEdges.Left, a.snapEdges.Right
	edgeT, edgeB := a.snapEdges.Top, a.snapEdges.Bottom
	if desired.Right().AsF32() > limits.Right().AsF32() {
		desired.Origin.X = desired.Origin.X.Sub(desired.Right().Sub(limits.Right())).Sub(edgeR)
	}
	if desired.Left().AsF32() < limits.Left().AsF32() {
		desired.Origin.X = limits.Left().Add(edgeL)
	}
	if desired.Bottom().AsF32() > limits.Bottom().AsF32() {
		desired.Origin.Y = desired.Origin.Y.Sub(desired.Bottom().Sub(limits.Bottom())).Sub(edgeB)
	}
	if desired.Top().AsF32() < limits.Top().AsF32() {
		desired.Origin.Y = limits.Top().Add(edgeT)
	}

	// Move the child by the difference between the desired origin and this
	// element's layout origin (upstream with_element_offset(offset) with
	// offset = desired.origin - bounds.origin).
	delta := geometry.Point[geometry.Pixels]{
		X: desired.Origin.X.Sub(bounds.Origin.X),
		Y: desired.Origin.Y.Sub(bounds.Origin.Y),
	}
	a.placedOrigin = desired.Origin
	if node, ok := w.NodeOf(a.child); ok {
		w.WithElementOffset(delta, func() {
			w.PrepaintElement(a.child, node)
		})
	}
}

func anchorPointOrZero(p *geometry.Point[geometry.Pixels]) geometry.Point[geometry.Pixels] {
	if p == nil {
		return geometry.Point[geometry.Pixels]{}
	}
	return *p
}

func maxPx(a, b geometry.Pixels) geometry.Pixels {
	if a > b {
		return a
	}
	return b
}

// sideOf reports which side of the element the corner names along an axis:
// -1 start (left/top), +1 end (right/bottom), 0 center.
func sideOf(corner style.AnchorCorner, horizontal bool) int {
	if horizontal {
		switch corner {
		case style.AnchorCornerTopLeft, style.AnchorCornerBottomLeft:
			return -1
		case style.AnchorCornerTopRight, style.AnchorCornerBottomRight:
			return 1
		}
		return 0
	}
	switch corner {
	case style.AnchorCornerTopLeft, style.AnchorCornerTopRight, style.AnchorCornerTopCenter:
		return -1
	case style.AnchorCornerBottomLeft, style.AnchorCornerBottomRight, style.AnchorCornerBottomCenter:
		return 1
	}
	return 0
}

// Paint implements Element.
func (a *Anchored) Paint(w *gpui.Window) {
	if a.child == nil {
		return
	}
	w.PaintElement(a.child)
}

// ---------------------------------------------------------------------------
// Anchor registry
// ---------------------------------------------------------------------------

// anchorRegistry maps (window, id) to elements for Anchored.Anchor lookups.
// Registration is done by view code through RegisterAnchor; the bounds are
// resolved per frame via gpui.Window.ElementBounds, so only elements that
// participate in the window's tree resolve to a position.
var (
	anchorRegistryMu sync.RWMutex
	anchorRegistry   = map[*gpui.Window]map[any]gpui.Element{}
)

// RegisterAnchor records an element under id in the window so Anchored
// elements can pin to it (the bounds come from the window's element bounds
// table at prepaint time — gpui.Window.ElementBounds). Call it when building
// the anchor element (views re-render each frame, so re-registering is
// expected and cheap). The id must be hashable.
func RegisterAnchor(w *gpui.Window, id any, e gpui.Element) {
	if w == nil || id == nil || e == nil {
		return
	}
	anchorRegistryMu.Lock()
	defer anchorRegistryMu.Unlock()
	byID, ok := anchorRegistry[w]
	if !ok {
		byID = map[any]gpui.Element{}
		anchorRegistry[w] = byID
	}
	byID[id] = e
}

// lookupAnchor resolves an anchor id to its element. A gpui.Element id is
// returned directly; anything else goes through the registry.
func lookupAnchor(w *gpui.Window, id any) gpui.Element {
	if el, ok := id.(gpui.Element); ok {
		return el
	}
	anchorRegistryMu.RLock()
	defer anchorRegistryMu.RUnlock()
	return anchorRegistry[w][id]
}

// ---------------------------------------------------------------------------
// Geometry helpers (upstream Bounds::from_anchor_and_size / Bounds::corner)
// ---------------------------------------------------------------------------

// anchorCornerPoint returns the point of the bounds named by the corner
// (upstream Bounds::corner).
func anchorCornerPoint(corner style.AnchorCorner, b geometry.Bounds[geometry.Pixels]) geometry.Point[geometry.Pixels] {
	switch corner {
	case style.AnchorCornerTopLeft:
		return b.TopLeft()
	case style.AnchorCornerTopRight:
		return b.TopRight()
	case style.AnchorCornerBottomLeft:
		return b.BottomLeft()
	case style.AnchorCornerBottomRight:
		return b.BottomRight()
	case style.AnchorCornerTopCenter:
		return b.TopCenter()
	case style.AnchorCornerBottomCenter:
		return b.BottomCenter()
	}
	return b.Origin
}

// boundsFromAnchorAndSize builds bounds so the given corner sits on p
// (upstream Bounds::from_anchor_and_size).
func boundsFromAnchorAndSize(corner style.AnchorCorner, p geometry.Point[geometry.Pixels], size geometry.Size[geometry.Pixels]) geometry.Bounds[geometry.Pixels] {
	var x, y float32
	switch corner {
	case style.AnchorCornerTopLeft, style.AnchorCornerBottomLeft:
		x = p.X.AsF32()
	case style.AnchorCornerTopRight, style.AnchorCornerBottomRight:
		x = p.X.AsF32() - size.Width.AsF32()
	default: // top/bottom center
		x = p.X.AsF32() - size.Width.AsF32()/2
	}
	switch corner {
	case style.AnchorCornerTopLeft, style.AnchorCornerTopRight, style.AnchorCornerTopCenter:
		y = p.Y.AsF32()
	case style.AnchorCornerBottomLeft, style.AnchorCornerBottomRight, style.AnchorCornerBottomCenter:
		y = p.Y.AsF32() - size.Height.AsF32()
	}
	return geometry.Bounds[geometry.Pixels]{
		Origin: geometry.Point[geometry.Pixels]{X: geometry.Px(x), Y: geometry.Px(y)},
		Size:   size,
	}
}

// scrollbarStyle builds a solid quad style for painted scrollbars (track and
// thumb quads; upstream paints scrollbars with fixed overlay colors).
func scrollbarStyle(color gc.Hsla) style.Style {
	st := style.DefaultStyle()
	st.Background = &gc.Background{Tag: gc.BackgroundTagSolid, Solid: color}
	return st
}
