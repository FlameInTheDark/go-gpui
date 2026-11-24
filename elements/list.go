package elements

import (
	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/layout"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/style"
)

// ListItemRender renders one item.
type ListItemRender func(ix int, w *gpui.Window) gpui.Element

// listState is the persistent virtualization state.
type listState struct {
	itemHeights map[int]float32
	scrollTop   float32
	scroll      float32
	scrollMax   float32
	visibleFrom int
	visibleTo   int
	scrollTo    int
	// scrollStrategy / scrollStrict refine scrollTo (upstream
	// ScrollStrategy; ScrollTo keeps the legacy top placement).
	scrollStrategy ScrollStrategy
	scrollStrict   bool
	// follow records that tail-follow mode (FollowBottom) is enabled;
	// following records that the list is currently pinned to the bottom
	// (upstream FollowState::Tail { is_following }).
	follow    bool
	following bool
	// lastHeight is the viewport height of the last layout (upstream
	// last_layout_bounds.size.height; used for viewport-range computations).
	lastHeight float32

	// Scrollbar drag (upstream scrollbar_drag_start_height + the UI
	// scrollbar's drag tracking): while dragging, scrollMax stays frozen
	// at the value captured when the drag began so overdraw measurement
	// cannot shift the thumb under the pointer.
	scrollbarDragging bool
	dragBaseMax       float32
	dragStartY        float32
	dragStartScroll   float32
}

// scrollEvent mirrors platform.ScrollEvent without the import cycle
// (defined here to keep the list self-contained).
type scrollEvent = platformScrollEvent

// items holds this frame's rendered elements.

// List is a variable-height virtualized list (a condensed port of gpui's
// list.rs: measure-as-you-scroll with cached item heights and overdraw).
type List struct {
	st         *style.StyleRefinement
	itemCount  int
	renderItem ListItemRender
	state      *listState
	overscroll float32
	items      []gpui.Element
	// currentHitbox carries the list's scroll hitbox.
	currentHitbox gpui.Hitbox
	// currentScrollbarHitbox is this frame's scrollbar grab-zone hitbox.
	currentScrollbarHitbox gpui.HitboxID
	// followMode enables tail-following (upstream FollowMode::Tail).
	followMode bool
	// onScroll is invoked after each paint with the scroll state (upstream
	// ListState::set_scroll_handler / ListScrollEvent).
	onScroll func(logicalTop int, viewport int, total int)
	// showScrollbar paints a track+thumb pair after the items.
	showScrollbar bool
}

// NewList creates a list.
func NewList(itemCount int, renderItem ListItemRender) *List {
	return &List{
		st:         style.NewStyleRefinement(),
		itemCount:  itemCount,
		renderItem: renderItem,
		overscroll: 100,
	}
}

// StyleRef returns the refinement.
func (l *List) StyleRef() *style.StyleRefinement { return l.st }

// ElementID implements Element.
func (l *List) ElementID() gpui.ElementId { return gpui.NameElementId("list") }

// RequestLayout implements Element.
func (l *List) RequestLayout(w *gpui.Window) {
	l.ensureState(w)
	st := style.DefaultStyle()
	st.Refine(l.st)
	st.Overflow[1] = style.OverflowScroll
	w.RequestMeasuredLayout(st, func(known layout.SizeDimension, available layout.AvailableSpace) layout.SizeResult {
		// Total height from measured items; width from the definite axis.
		if _, ok := available.IsDefiniteAxis(false); !ok && l.state.visibleTo == 0 {
			// First pass: measure the first item to estimate.
			if l.itemCount > 0 && l.renderItem != nil {
				el := l.renderItem(0, w)
				ms := w.MeasureElement(el)
				l.state.itemHeights[0] = ms.Height.AsF32()
				l.state.visibleFrom, l.state.visibleTo = 0, 1
				return layout.SizeResult{Width: available.Width, Height: ms.Height.AsF32() * float32(l.itemCount)}
			}
			return layout.SizeResult{Width: available.Width, Height: 0}
		}
		total := float32(0)
		for i := 0; i < l.itemCount; i++ {
			if h, ok := l.state.itemHeights[i]; ok {
				total += h
			}
		}
		return layout.SizeResult{Width: available.Width, Height: total}
	})
	l.flushState(w)
}

func (l *List) ensureState(w *gpui.Window) {
	if l.state == nil {
		l.state = &listState{itemHeights: map[int]float32{}}
	}
}

// flushState persists the list state under the current element id.
func (l *List) flushState(w *gpui.Window) {
	gpui.UseElementState(w, func() listState {
		return listState{itemHeights: map[int]float32{}}
	}, func(s *listState) {
		*s = *l.state
		s.itemHeights = l.state.itemHeights
	})
}

// loadState restores persisted state (upstream list keeps ListState in
// element state keyed by the global element id).
func (l *List) loadState(w *gpui.Window) {
	if l.state != nil {
		return
	}
	l.state = &listState{itemHeights: map[int]float32{}}
	gpui.UseElementState(w, func() listState {
		return listState{itemHeights: map[int]float32{}}
	}, func(s *listState) {
		if s.itemHeights == nil {
			s.itemHeights = map[int]float32{}
		}
		l.state = s
	})
}

// ScrollTo scrolls to an item index, placing it at the top of the
// viewport (upstream scroll_to with ScrollStrategy::Top).
func (l *List) ScrollTo(ix int) {
	l.ScrollToStrategy(ix, ScrollTop)
}

// ScrollToStrategy scrolls an item into view with the given placement
// strategy (upstream list scroll_to(ix, strategy)).
func (l *List) ScrollToStrategy(ix int, strategy ScrollStrategy) {
	if l.state == nil {
		l.state = &listState{itemHeights: map[int]float32{}, scrollTo: -1}
	}
	l.state.scrollTo = ix
	l.state.scrollStrategy = strategy
	l.state.scrollStrict = false
	// Scrolling to an item above the end stops tail-following (upstream
	// ListState::scroll_to stops following when the target is below the
	// last item).
	if ix < l.itemCount {
		l.state.following = false
	}
}

// ScrollToStrict scrolls an item into view even when already visible
// (upstream scroll_to_strict).
func (l *List) ScrollToStrict(ix int, strategy ScrollStrategy) {
	l.ScrollToStrategy(ix, strategy)
	l.state.scrollStrict = true
}

// SetItemCount changes the item count (upstream ListState::splice/reset,
// condensed to a plain count update: callers grow the list and the next
// frame re-walks the items).
func (l *List) SetItemCount(n int) {
	if n < 0 {
		n = 0
	}
	l.itemCount = n
}

// FollowBottom enables tail-follow mode (upstream ListState::set_follow_mode
// with FollowMode::Tail): while the view is at the bottom, newly added items
// keep it pinned there, and scrolling back to the bottom re-engages the
// pinning after the user scrolled away.
func (l *List) FollowBottom() *List {
	l.followMode = true
	return l
}

// Unfollow disables tail-follow mode entirely (upstream set_follow_mode with
// FollowMode::Normal): the list stops pinning to the bottom and the current
// scroll position is kept.
func (l *List) Unfollow() *List {
	l.followMode = false
	if l.state != nil {
		l.state.following = false
	}
	return l
}

// OnScrollHandler sets a handler called after each paint with the scroll
// state (upstream ListState::set_scroll_handler receives a ListScrollEvent;
// the condensed port passes the index of the item at the top of the
// viewport, the number of visible items and the total item count).
func (l *List) OnScrollHandler(f func(logicalTop int, viewport int, total int)) *List {
	l.onScroll = f
	return l
}

// WithScrollbar paints a thin track+thumb quad pair from the current scroll
// offsets after the items. The scrollbar is paint-only: it registers no
// hitbox and never intercepts the mouse (upstream list.rs paints a
// scrollbar-driven offset surface; the port leaves drag interaction to the
// host element).
func (l *List) WithScrollbar() *List {
	l.showScrollbar = true
	return l
}

// IsFollowingTail reports whether the list is currently pinned to the bottom
// (upstream ListState::is_following_tail).
func (l *List) IsFollowingTail() bool {
	return l.state != nil && l.state.following
}

// ListOffset is the scroll position anchored to an item (upstream
// ListOffset: the item at the top of the viewport plus the pixels scrolled
// into it).
type ListOffset struct {
	ItemIx       int
	OffsetInItem float32
}

// LogicalScrollOffset returns the full logical scroll position: the index of
// the item at the top of the viewport and how far into it the viewport
// starts (upstream ListState::logical_scroll_top / ListOffset). Unmeasured
// items count with the average measured height.
func (l *List) LogicalScrollOffset() ListOffset {
	if l.state == nil {
		return ListOffset{}
	}
	top := l.state.scrollTop
	if top <= 0 {
		return ListOffset{}
	}
	acc := float32(0)
	for i := 0; i < l.itemCount; i++ {
		h, ok := l.state.itemHeights[i]
		if !ok {
			h = averageItemHeight(l.state.itemHeights, l.itemCount)
		}
		if acc+h > top {
			return ListOffset{ItemIx: i, OffsetInItem: top - acc}
		}
		acc += h
	}
	// The scroll top is past the measured content: the end anchor.
	return ListOffset{ItemIx: l.itemCount}
}

// LogicalScrollTop returns the index of the item at the top of the viewport
// (upstream ListState::logical_scroll_top returns a ListOffset{item_ix,
// offset_in_item}; the condensed port exposes the item index only, and
// returns the item count when the view is pinned past the end). Unmeasured
// items count with the average measured height.
func (l *List) LogicalScrollTop() int {
	if l.state == nil {
		return 0
	}
	from, to := l.viewportRange()
	if to <= from {
		// The scroll top is past the measured content: the view is at the
		// end (upstream returns item_ix = count for the end anchor).
		return l.itemCount
	}
	return from
}

// VisibleRange returns the currently rendered item range.
func (l *List) VisibleRange() (int, int) {
	if l.state == nil {
		return 0, 0
	}
	return l.state.visibleFrom, l.state.visibleTo
}

// Prepaint implements Element.
func (l *List) Prepaint(w *gpui.Window, bounds geometry.Bounds[geometry.Pixels]) {
	l.loadState(w)
	st := style.DefaultStyle()
	st.Refine(l.st)
	l.currentHitbox = w.InsertHitbox(bounds, gpui.HitboxNormal)

	// Sync the follow mode into the persisted state. Enabling it starts
	// following immediately (upstream set_follow_mode(FollowMode::Tail)
	// engages the tail on the spot); disabling it stops the pinning.
	if l.followMode != l.state.follow {
		l.state.follow = l.followMode
		if l.followMode {
			l.state.following = true
		}
	}

	// Determine visible range from scrollTop + measured heights.
	rem := w.RemSize()
	_ = rem
	height := bounds.Size.Height.AsF32()
	top := -l.state.scroll
	from, to := l.state.visibleFrom, l.state.visibleTo

	// Jump-scroll request (upstream scroll_to with ScrollStrategy:
	// Top/Center/Bottom/Nearest over the measured-height walk).
	if l.state.scrollTo >= 0 {
		itemTop := float32(0)
		for i := 0; i < l.state.scrollTo && i < l.itemCount; i++ {
			if h, ok := l.state.itemHeights[i]; ok {
				itemTop += h
			} else {
				itemTop += averageItemHeight(l.state.itemHeights, l.itemCount)
			}
		}
		itemH := averageItemHeight(l.state.itemHeights, l.itemCount)
		if h, ok := l.state.itemHeights[l.state.scrollTo]; ok {
			itemH = h
		}
		total := estimateTotalHeight(l.state.itemHeights, l.itemCount)
		maxTop := total - height
		if maxTop < 0 {
			maxTop = 0
		}
		clamp := func(v float32) float32 {
			if v < 0 {
				return 0
			}
			if v > maxTop {
				return maxTop
			}
			return v
		}
		isAbove := itemTop < top
		isBelow := itemTop+itemH > top+height
		strategy := l.state.scrollStrategy
		if isAbove || isBelow || l.state.scrollStrict {
			if strategy == ScrollNearest {
				if isAbove {
					strategy = ScrollTop
				} else if isBelow {
					strategy = ScrollBottom
				}
			}
			switch strategy {
			case ScrollTop:
				l.state.scroll = -clamp(itemTop)
			case ScrollCenter:
				l.state.scroll = -clamp(itemTop + itemH/2 - height/2)
			case ScrollBottom:
				l.state.scroll = -clamp(itemTop + itemH - height)
			case ScrollNearest:
				// visible and Nearest: nothing to do
			}
		}
		top = -l.state.scroll
		l.state.scrollTo = -1
	}

	// Follow semantics (upstream list.rs FollowMode::Tail): while following,
	// the scroll anchor is the end of the list (ListOffset{item_ix: count,
	// offset_in_item: 0}), so a growing list keeps the view pinned to the
	// bottom. The port estimates the total height from the cached
	// measurements (average height for unmeasured items) to start the walk
	// near the bottom; the walk measures the trailing items and the scroll
	// is re-pinned to the measured end below.
	if l.state.follow && l.state.following && height > 0 {
		if est := estimateTotalHeight(l.state.itemHeights, l.itemCount); est > height {
			l.state.scroll = -(est - height)
			l.state.scrollTop = est - height
		} else {
			l.state.scroll = 0
			l.state.scrollTop = 0
		}
		top = -l.state.scroll
	}

	// Walk items accumulating heights.
	y := top
	from = 0
	to = 0
	measured := 0
	for i := 0; i < l.itemCount; i++ {
		h, ok := l.state.itemHeights[i]
		if !ok {
			if float32(measured) < height+l.overscroll || i < to {
				// Measure by rendering.
				if l.renderItem != nil {
					el := l.renderItem(i, w)
					ms := w.MeasureElement(el)
					l.state.itemHeights[i] = ms.Height.AsF32()
					h = ms.Height.AsF32()
					measured++
				} else {
					h = 20
					l.state.itemHeights[i] = h
				}
			} else {
				// Unmeasured beyond the visible region: estimate the rest.
				avg := float32(0)
				n := 0
				for _, vh := range l.state.itemHeights {
					avg += vh
					n++
				}
				if n > 0 {
					h = avg / float32(n)
				} else {
					h = 20
				}
			}
		}
		bottom := y + h
		if bottom > top-height-l.overscroll && y < top+height+l.overscroll {
			if to == from && y >= top-0.01 || from == to && y+h > top-0.01 {
				from = i
			}
			to = i + 1
			// Prepaint the element at its position.
			if l.renderItem != nil {
				el := l.renderItem(i, w)
				w.LayoutElement(el, layout.DefiniteAvailable(bounds.Size.Width.AsF32(), h))
				if node, ok := w.NodeOf(el); ok {
					origin := geometry.Point[geometry.Pixels]{X: bounds.Origin.X, Y: bounds.Origin.Y + geometry.Px(y-top)}
					w.PrepaintElementAt(el, node, origin)
				}
				l.items = append(l.items, el)
			}
		}
		y += h
		if y > top+height+l.overscroll && to > from {
			break
		}
	}
	if to < from {
		to = from
	}
	l.state.visibleFrom, l.state.visibleTo = from, to
	// The scroll extent is the FULL list: measured heights plus the
	// average-measured estimate for unmeasured items (upstream
	// max_scroll_offset walks the sum tree, which holds base-height
	// estimates for every item — ListState::new(base_item_height)).
	// Mapping only the walked window would make the scrollbar cover a
	// fraction of the list that grows as items are discovered.
	l.state.scrollMax = estimateTotalHeight(l.state.itemHeights, l.itemCount) - height
	if l.state.scrollMax < 0 {
		l.state.scrollMax = 0
	}
	l.state.lastHeight = height

	// Follow re-engage and re-pin (upstream layout(): when the scroll
	// position has returned to the bottom -- scroll_offset +
	// available_height >= total_height - 1 -- following re-engages; while
	// following, the scroll stays pinned to the measured end of the
	// list). The walk's content height is y - top (y accumulates from
	// the scroll top), so the measured end offset is total - height.
	// scrollMax keeps its existing progressive semantics (it grows with
	// the scroll top); the follow pin uses the true measured end.
	if l.state.follow && height > 0 {
		end := y - top - height
		if end < 0 {
			end = 0
		}
		if !l.state.following && l.state.scrollTop >= end-1 {
			l.state.following = true
		}
		if l.state.following {
			l.state.scroll = -end
			l.state.scrollTop = end
		}
	}
	l.flushState(w)
}

// Paint implements Element.
func (l *List) Paint(w *gpui.Window) {
	l.loadState(w)
	bounds := w.ElementBounds(l)
	mask := w.ContentMask()
	mask = geometry.IntersectBounds(mask, bounds)
	w.WithContentMask(mask, func() {
		for _, el := range l.items {
			w.PaintElement(el)
		}
		// Scrollbar track+thumb painted after the items (on top), clipped
		// to the list bounds.
		l.paintScrollbar(w, bounds)
	})
	l.items = nil

	// Report the scroll state after each paint (upstream set_scroll_handler
	// fires with a ListScrollEvent; the port passes the logical scroll top
	// item index, the visible item count and the total item count).
	if l.onScroll != nil {
		from, to := l.viewportRange()
		l.onScroll(l.LogicalScrollTop(), to-from, l.itemCount)
	}

	// Scroll handling.
	if l.currentHitbox.ID != 0 {
		hb := l.currentHitbox
		state := l.state
		w.OnMouseEvent("scroll", 0, hb, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase != gpui.BubblePhase {
				return
			}
			e := ev.(platformScrollEvent)
			delta := e.Delta.PixelDelta(geometry.Px(20))
			// Scrolling away from the bottom stops tail-following (upstream
			// paint_scroll: delta.y > 0 stops following).
			if delta.Y.AsF32() > 0 && state.follow {
				state.following = false
			}
			state.scroll += delta.Y.AsF32()
			if state.scroll > 0 {
				state.scroll = 0
			}
			if state.scroll < -state.scrollMax {
				state.scroll = -state.scrollMax
			}
			state.scrollTop = -state.scroll
			l.flushState(w)
			w.StopPropagation()
		})
	}
}

// paintScrollbar draws a thin track+thumb quad pair at the right edge of the
// list from the current scroll offsets (upstream list.rs scrollbar), with a
// hitbox over the track making the thumb draggable and the track
// click-to-jump (upstream: the scrollbar is a separate UI element driving
// ListState::set_offset_from_scrollbar / scrollbar_drag_started).
func (l *List) paintScrollbar(w *gpui.Window, bounds geometry.Bounds[geometry.Pixels]) {
	if !l.showScrollbar || l.state == nil || l.state.scrollMax <= 0 {
		return
	}
	width := geometry.Px(6)
	// The interaction zone is wider than the painted thumb for easier
	// grabbing (upstream SCROLLBAR_GRAB_ZONE_WIDTH).
	grabWidth := geometry.Px(12)
	track := geometry.Bounds[geometry.Pixels]{
		Origin: geometry.Point[geometry.Pixels]{X: bounds.Right().Sub(width), Y: bounds.Origin.Y},
		Size:   geometry.Size[geometry.Pixels]{Width: width, Height: bounds.Size.Height},
	}
	w.PaintQuad(scrollbarStyle(gc.Black().Opacity(0.08)), track)

	// While dragging, the thumb maps against the scroll extent frozen at
	// drag start (upstream scrollbar_drag_started).
	scrollMax := l.state.scrollMax
	if l.state.scrollbarDragging && l.state.dragBaseMax > 0 {
		scrollMax = l.state.dragBaseMax
	}

	// Thumb proportional to the visible fraction of the content, sliding
	// with the scroll offset (upstream scrollbar offsets).
	viewport := bounds.Size.Height.AsF32()
	content := scrollMax + viewport
	thumbH := viewport * (viewport / content)
	if thumbH > viewport {
		thumbH = viewport
	}
	travel := viewport - thumbH
	progress := l.state.scrollTop / scrollMax
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	thumb := geometry.Bounds[geometry.Pixels]{
		Origin: geometry.Point[geometry.Pixels]{X: track.Origin.X, Y: bounds.Origin.Y.Add(geometry.Px(progress * travel))},
		Size:   geometry.Size[geometry.Pixels]{Width: width, Height: geometry.Px(thumbH)},
	}
	thumbOpacity := float32(0.25)
	if l.state.scrollbarDragging {
		thumbOpacity = 0.45
	}
	w.PaintQuad(scrollbarStyle(gc.Black().Opacity(thumbOpacity)), thumb)

	// Interaction: a hitbox over the grab zone. Pressing inside the thumb
	// starts a drag; pressing the track jumps toward the press (upstream
	// scrollbar drag / track click).
	grab := track
	grab.Origin.X = bounds.Right().Sub(grabWidth)
	grab.Size.Width = grabWidth
	hitbox := w.InsertHitbox(grab, gpui.HitboxBlockMouseExceptScroll)
	l.currentScrollbarHitbox = hitbox.ID

	// scrollFromY maps a pointer y to the scroll offset it represents,
	// against the (possibly frozen) drag extent.
	scrollFromY := func(y float32) float32 {
		if travel <= 0 {
			return 0
		}
		rel := y - bounds.Origin.Y.AsF32() - thumbH/2
		if rel < 0 {
			rel = 0
		}
		if rel > travel {
			rel = travel
		}
		return rel / travel * scrollMax
	}
	applyScroll := func(scrollTop float32) {
		l.state.scrollTop = scrollTop
		l.state.scroll = -scrollTop
		if l.state.scroll > 0 {
			l.state.scroll = 0
		}
		if l.state.scroll < -l.state.scrollMax {
			l.state.scroll = -l.state.scrollMax
			l.state.scrollTop = l.state.scrollMax
		}
		// Manual positioning abandons tail-following (upstream stops
		// following on any upward scroll; scrollbar drags always detach).
		l.state.following = false
		l.flushState(w)
		w.Refresh()
	}

	w.OnMouseEvent("down", platform.ButtonLeft, hitbox, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
		if phase != gpui.BubblePhase {
			return
		}
		e := ev.(platform.MouseDownEvent)
		if !hitbox.IsHovered(w) {
			return
		}
		inThumb := e.Position.Y >= thumb.Origin.Y && e.Position.Y <= thumb.Origin.Y.Add(thumb.Size.Height)
		l.state.scrollbarDragging = true
		l.state.dragBaseMax = scrollMax
		l.state.dragStartY = e.Position.Y.AsF32()
		l.state.dragStartScroll = l.state.scrollTop
		w.CapturePointer(hitbox.ID)
		if !inThumb {
			// Track press: jump the thumb center to the pointer.
			applyScroll(scrollFromY(e.Position.Y.AsF32()))
			l.state.dragStartScroll = l.state.scrollTop
		}
		w.StopPropagation()
	})
	w.OnMouseEvent("move", platform.ButtonLeft, hitbox, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
		if phase != gpui.BubblePhase || !l.state.scrollbarDragging {
			return
		}
		e := ev.(platform.MouseMoveEvent)
		if !e.Dragging() {
			return
		}
		// Drag math against the frozen extent: the thumb stays under the
		// pointer even as overdraw measures new items (upstream
		// scrollbar_drag_started contract).
		delta := e.Position.Y.AsF32() - l.state.dragStartY
		newTop := l.state.dragStartScroll + delta/travel*scrollMax
		if newTop < 0 {
			newTop = 0
		}
		if newTop > l.state.scrollMax {
			newTop = l.state.scrollMax
		}
		applyScroll(newTop)
		w.StopPropagation()
	})
	w.OnMouseEvent("up", platform.ButtonLeft, hitbox, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
		if phase != gpui.BubblePhase || !l.state.scrollbarDragging {
			return
		}
		l.state.scrollbarDragging = false
		l.state.dragBaseMax = 0
		w.ReleasePointer()
		l.flushState(w)
		w.Refresh()
	})
}

// IsScrollbarDragging reports whether the scrollbar thumb is currently
// being dragged (upstream ListState::is_scrollbar_dragging); consumers use
// it to suppress auto-scroll behavior during manual positioning.
func (l *List) IsScrollbarDragging() bool {
	return l.state != nil && l.state.scrollbarDragging
}

// viewportRange computes the strict range of items intersecting the viewport
// [scrollTop, scrollTop+height) from the cached heights (upstream
// ListState::visible_range, used by the scroll handler). Unmeasured items
// count with the average measured height.
func (l *List) viewportRange() (int, int) {
	if l.state == nil {
		return 0, 0
	}
	top := l.state.scrollTop
	height := l.state.lastHeight
	if height <= 0 {
		// No measured viewport yet (first frame): report the item at
		// the scroll top with a one-column window.
		height = 1
	}
	avg := averageItemHeight(l.state.itemHeights, l.itemCount)
	from, to := l.itemCount, 0
	acc := float32(0)
	for i := 0; i < l.itemCount; i++ {
		h, ok := l.state.itemHeights[i]
		if !ok {
			h = avg
		}
		if acc+h > top && acc < top+height {
			if i < from {
				from = i
			}
			to = i + 1
		}
		acc += h
	}
	if from > to {
		return 0, 0
	}
	return from, to
}

// estimateTotalHeight sums the cached heights of the first count items,
// estimating unmeasured ones with the average measured height (the port's
// equivalent of upstream walking from the end anchor with a sum tree).
func estimateTotalHeight(heights map[int]float32, count int) float32 {
	total, n := float32(0), 0
	for i := 0; i < count; i++ {
		if h, ok := heights[i]; ok {
			total += h
			n++
		}
	}
	if n < count {
		total += averageItemHeight(heights, count) * float32(count-n)
	}
	return total
}

// averageItemHeight averages the cached heights of the first count items
// (defaulting to the walk's 20px estimate when nothing is measured yet).
func averageItemHeight(heights map[int]float32, count int) float32 {
	total, n := float32(0), 0
	for i := 0; i < count; i++ {
		if h, ok := heights[i]; ok {
			total += h
			n++
		}
	}
	if n == 0 {
		return 20
	}
	return total / float32(n)
}

// platformScrollEvent avoids importing the platform package in list.go's
// scroll handler signature (alias for platform.ScrollEvent).
type platformScrollEvent = platform.ScrollEvent
