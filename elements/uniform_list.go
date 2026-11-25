package elements

import (
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/layout"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/style"
)

// ScrollStrategy places the item scrolled to (upstream list.rs
// ScrollStrategy).
type ScrollStrategy uint8

const (
	// ScrollTop places the element at the top of the list's viewport.
	ScrollTop ScrollStrategy = 0
	// ScrollCenter attempts to place the element in the middle of the
	// list's viewport; falls back to the closest possible position.
	ScrollCenter ScrollStrategy = 1
	// ScrollBottom attempts to place the element at the bottom of the
	// list's viewport; falls back to the closest possible position.
	ScrollBottom ScrollStrategy = 2
	// ScrollNearest scrolls the minimum distance that reveals the element:
	// top of the viewport when it is above the visible elements, bottom
	// when below, nothing when already visible.
	ScrollNearest ScrollStrategy = 3
)

// deferredScroll is a pending scroll-to-item request, applied at the next
// prepaint when the item height and viewport are known (upstream
// DeferredScrollToItem).
type deferredScroll struct {
	itemIndex int
	strategy  ScrollStrategy
	offset    int  // offset in number of items
	strict    bool // scroll even when the item is already visible
}

// uniformState is the persistent uniform-list state.
type uniformState struct {
	itemHeight float32
	scroll     float32
	scrollMax  float32
	// renderedFirst/renderedLast track the last prepainted visible item
	// range (upstream ListState::rendered_item_range). The elastic overdraw
	// extends the actually prepainted range beyond this window.
	renderedFirst int
	renderedLast  int
	// pendingScroll is the deferred scroll request (upstream
	// UniformListScrollState.deferred_scroll_to_item).
	pendingScroll *deferredScroll
}

// UniformListDecoration computes a decoration element painted over the
// list's items — sticky headers/footers and the like (upstream
// UniformListDecoration).
type UniformListDecoration interface {
	// Compute receives the visible item range, the scrolled bounds, the
	// scroll offset, the uniform item height and the total item count, and
	// returns the element to paint above the items.
	Compute(visibleRange [2]int, bounds geometry.Bounds[geometry.Pixels], scrollOffsetY geometry.Pixels, itemHeight geometry.Pixels, itemCount int, w *gpui.Window) gpui.Element
}

// UniformList is a fixed-height virtualized list (upstream uniform_list:
// one measured item height applies to all rows).
type UniformList struct {
	st            *style.StyleRefinement
	itemCount     int
	renderItem    func(ix int, w *gpui.Window) gpui.Element
	itemToMeasure int
	state         uniformState
	items         []gpui.Element
	decorations   []UniformListDecoration
	hitbox        gpui.Hitbox
	// decorationEls are this frame's prepainted decoration elements.
	decorationEls []gpui.Element
	// overdraw is the number of extra items rendered beyond the visible
	// range on each side (upstream uniform_list with_overdraw).
	overdraw int
	// yFlipped renders the list vertically flipped: visual position ix
	// shows logical item itemCount-1-ix (upstream y_flipped; used for
	// bottom-anchored lists where new items appear at the bottom).
	yFlipped bool
}

// NewUniformList creates a virtualized list of itemCount fixed-height rows.
// It measures one item and applies that height to every row. renderItem
// builds the element for an index as needed.
func NewUniformList(itemCount int, renderItem func(ix int, w *gpui.Window) gpui.Element) *UniformList {
	return &UniformList{
		st:            style.NewStyleRefinement(),
		itemCount:     itemCount,
		renderItem:    renderItem,
		itemToMeasure: 0,
	}
}

// StyleRef returns the refinement.
func (u *UniformList) StyleRef() *style.StyleRefinement { return u.st }

// ElementID implements Element.
func (u *UniformList) ElementID() gpui.ElementId { return gpui.NameElementId("uniform-list") }

// RequestLayout implements Element.
func (u *UniformList) RequestLayout(w *gpui.Window) {
	// Preserve a scroll request made through this instance (see Prepaint):
	// the load below would otherwise clobber it.
	pending := u.state.pendingScroll
	gpui.UseElementState(w, func() uniformState {
		return uniformState{}
	}, func(state *uniformState) {
		u.state = *state
	})
	if u.state.pendingScroll == nil {
		u.state.pendingScroll = pending
	}

	st := style.DefaultStyle()
	st.Refine(u.st)
	st.Overflow[1] = style.OverflowScroll

	itemCount := u.itemCount
	var itemSize geometry.Size[geometry.Pixels]
	// Measure the sample item once (upstream measure_item).
	if u.itemCount > 0 && u.renderItem != nil {
		el := u.renderItem(u.itemToMeasure, w)
		itemSize = w.MeasureElement(el)
		u.state.itemHeight = itemSize.Height.AsF32()
	}

	w.RequestMeasuredLayout(st, func(known layout.SizeDimension, available layout.AvailableSpace) layout.SizeResult {
		// Upstream sizing: width from known/definite available (or the item
		// width); height = desired, capped by definite available space.
		desiredHeight := itemSize.Height.AsF32() * float32(itemCount)
		width, wDef := known.Width.Resolve(0)
		if !wDef {
			if v, ok := available.IsDefiniteAxis(true); ok {
				width, wDef = v, true
			}
		}
		if !wDef {
			width = itemSize.Width.AsF32()
		}
		height := desiredHeight
		if v, ok := available.IsDefiniteAxis(false); ok && v < height {
			height = v
		}
		return layout.SizeResult{Width: width, Height: height}
	})
	u.flushState(w)
}

// ScrollTo scrolls an item to the top of the viewport (upstream
// scroll_to_item with ScrollStrategy::Top).
func (u *UniformList) ScrollTo(ix int) {
	u.ScrollToStrategy(ix, ScrollTop)
}

// ScrollToStrategy scrolls an item into view with the given placement
// strategy (upstream scroll_to_item(ix, strategy)). The request is
// deferred to the next prepaint, where the item height and viewport size
// are known.
func (u *UniformList) ScrollToStrategy(ix int, strategy ScrollStrategy) {
	u.state.pendingScroll = &deferredScroll{itemIndex: ix, strategy: strategy}
}

// ScrollToStrict scrolls an item into view even when it is already
// visible (upstream scroll_to_item_strict).
func (u *UniformList) ScrollToStrict(ix int, strategy ScrollStrategy) {
	u.state.pendingScroll = &deferredScroll{itemIndex: ix, strategy: strategy, strict: true}
}

// ScrollToWithOffset scrolls an item into view treating the first `offset`
// items as reserved space (upstream scroll_to_item_with_offset: shrinking
// the effective viewport from the top for Top/Center, from the bottom for
// Bottom).
func (u *UniformList) ScrollToWithOffset(ix int, strategy ScrollStrategy, offset int) {
	u.state.pendingScroll = &deferredScroll{itemIndex: ix, strategy: strategy, offset: offset}
}

// Overdraw sets the number of extra items rendered beyond the visible range
// on each side during scrolling, so fast scrolls still have content ready to
// paint (upstream uniform_list with_overdraw). VisibleRange keeps reporting
// the strictly visible range; the extra items are prepainted (and painted,
// masked by the viewport) around it.
func (u *UniformList) Overdraw(n int) *UniformList {
	if n < 0 {
		n = 0
	}
	u.overdraw = n
	return u
}

// YFlipped renders the list vertically flipped: the item at visual position
// ix is logical item itemCount-1-ix, and scroll-to indices are mapped
// through the same flip (upstream uniform_list.y_flipped; bottom-anchored
// lists where new items appear at the bottom edge).
func (u *UniformList) YFlipped(flipped bool) *UniformList {
	u.yFlipped = flipped
	return u
}

// AddDecoration appends a decoration painted above the items (upstream
// UniformList::decorations). Decorations receive the visible range and the
// scrolled bounds; sticky headers use them to float over the items.
func (u *UniformList) AddDecoration(d UniformListDecoration) *UniformList {
	u.decorations = append(u.decorations, d)
	return u
}

// VisibleRange returns the rendered item range [first, last) of the most
// recent frame (upstream rendered_item_range).
func (u *UniformList) VisibleRange() (int, int) {
	if u.state.itemHeight == 0 {
		return 0, 0
	}
	return u.state.renderedFirst, u.state.renderedLast
}

// applyDeferredScroll applies the pending scroll request now that the
// viewport and item height are known (upstream uniform_list prepaint's
// deferred_scroll_to_item handling).
func (u *UniformList) applyDeferredScroll(bounds geometry.Bounds[geometry.Pixels]) {
	req := u.state.pendingScroll
	if req == nil || u.state.itemHeight <= 0 {
		return
	}
	u.state.pendingScroll = nil
	ix := req.itemIndex
	if ix < 0 {
		ix = 0
	}
	if ix >= u.itemCount {
		ix = u.itemCount - 1
	}
	if u.yFlipped {
		// The visual index of logical item ix under the flip.
		ix = u.itemCount - 1 - ix
	}
	listHeight := bounds.Size.Height.AsF32()
	itemHeight := u.state.itemHeight
	itemTop := itemHeight * float32(ix)
	itemBottom := itemTop + itemHeight
	scrollTop := -u.state.scroll
	offsetPixels := itemHeight * float32(req.offset)

	// Is the item above/below the currently visible items?
	isAbove := itemTop < scrollTop+offsetPixels
	isBelow := itemBottom > scrollTop+listHeight

	strategy := req.strategy
	if req.strict || isAbove || isBelow {
		if strategy == ScrollNearest {
			if isAbove {
				strategy = ScrollTop
			} else if isBelow {
				strategy = ScrollBottom
			}
		}
		contentHeight := itemHeight * float32(u.itemCount)
		maxTop := contentHeight - listHeight
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
		switch strategy {
		case ScrollTop:
			u.state.scroll = -clamp(itemTop - offsetPixels)
		case ScrollCenter:
			itemCenter := itemTop + itemHeight/2
			viewportHeight := listHeight - offsetPixels
			viewportCenter := offsetPixels + viewportHeight/2
			u.state.scroll = -clamp(itemCenter - viewportCenter)
		case ScrollBottom:
			u.state.scroll = -clamp(itemBottom - listHeight)
		case ScrollNearest:
			// Nearest, but the item is visible: no scroll required.
		}
	}
}

// Prepaint implements Element.
func (u *UniformList) Prepaint(w *gpui.Window, bounds geometry.Bounds[geometry.Pixels]) {
	// Preserve a scroll request made through this element instance since
	// the last frame (upstream defers through the scroll handle, which
	// lives outside the element state): the load below would otherwise
	// clobber it with the persisted state.
	pending := u.state.pendingScroll
	gpui.UseElementState(w, func() uniformState {
		return uniformState{}
	}, func(state *uniformState) {
		u.state = *state
	})
	if u.state.pendingScroll == nil {
		u.state.pendingScroll = pending
	}

	u.hitbox = w.InsertHitbox(bounds, gpui.HitboxNormal)

	h := u.state.itemHeight
	// The item height comes from the measured sample; the node's height is
	// the (possibly capped) desired height, so derive the item height from
	// the count only when it was never measured.
	if h <= 0 || u.itemCount == 0 {
		u.flushState(w)
		return
	}

	u.applyDeferredScroll(bounds)

	// Clamp to the scroll extent after the deferred scroll.
	height := bounds.Size.Height.AsF32()
	u.state.scrollMax = float32(u.itemCount)*h - height
	if u.state.scrollMax < 0 {
		u.state.scrollMax = 0
	}
	if u.state.scroll > 0 {
		u.state.scroll = 0
	}
	if u.state.scroll < -u.state.scrollMax {
		u.state.scroll = -u.state.scrollMax
	}

	first := int(-u.state.scroll / h)
	if first < 0 {
		first = 0
	}
	last := first + int(height/h) + 2
	if last > u.itemCount {
		last = u.itemCount
	}
	u.state.renderedFirst = first
	u.state.renderedLast = last

	// Elastic overdraw (upstream uniform_list with_overdraw): render n
	// extra items beyond the visible window on each side. The extra items
	// are masked by the content mask and keep fast scroll frames smooth.
	rf, rl := first, last
	if u.overdraw > 0 {
		rf -= u.overdraw
		if rf < 0 {
			rf = 0
		}
		rl += u.overdraw
		if rl > u.itemCount {
			rl = u.itemCount
		}
	}

	u.items = nil
	for i := rf; i < rl; i++ {
		if u.renderItem == nil {
			continue
		}
		// Under y_flipped, visual position i shows logical item
		// itemCount-1-i (upstream renders the flipped range reversed).
		logical := i
		if u.yFlipped {
			logical = u.itemCount - 1 - i
		}
		el := u.renderItem(logical, w)
		w.LayoutElement(el, layout.DefiniteAvailable(bounds.Size.Width.AsF32(), h))
		if node, ok := w.NodeOf(el); ok {
			origin := geometry.Point[geometry.Pixels]{
				X: bounds.Origin.X,
				Y: bounds.Origin.Y + geometry.Px(float32(i)*h+u.state.scroll),
			}
			w.PrepaintElementAt(el, node, origin)
		}
		u.items = append(u.items, el)
	}

	// Decorations prepaint over the scrolled bounds (upstream: decorations
	// are laid out as roots at the padded bounds + scroll offset).
	u.decorationEls = nil
	if len(u.decorations) > 0 {
		scrolled := bounds
		scrolled.Origin.Y += geometry.Px(u.state.scroll)
		for _, d := range u.decorations {
			el := d.Compute([2]int{first, last}, scrolled, geometry.Px(u.state.scroll), geometry.Px(h), u.itemCount, w)
			if el == nil {
				continue
			}
			node := w.ComputeElementLayout(el, scrolled.Size)
			w.PrepaintElementAt(el, node, scrolled.Origin)
			u.decorationEls = append(u.decorationEls, el)
		}
	}
	u.flushState(w)
}

// flushState stores the (possibly updated) state for next frame.
func (u *UniformList) flushState(w *gpui.Window) {
	gpui.UseElementState(w, func() uniformState {
		return uniformState{}
	}, func(state *uniformState) {
		*state = u.state
	})
}

// Paint implements Element.
func (u *UniformList) Paint(w *gpui.Window) {
	gpui.UseElementState(w, func() uniformState {
		return uniformState{}
	}, func(state *uniformState) {
		u.state = *state
	})
	bounds := w.ElementBounds(u)
	mask := w.ContentMask()
	mask = geometry.IntersectBounds(mask, bounds)
	w.WithContentMask(mask, func() {
		for _, el := range u.items {
			w.PaintElement(el)
		}
		// Decorations paint above the items, still clipped to the list
		// bounds (upstream paints frame_state.decorations after items).
		for _, el := range u.decorationEls {
			w.PaintElement(el)
		}
	})
	u.items = nil
	u.decorationEls = nil
	if u.hitbox.ID != 0 {
		// The listener runs between frames; capture the element path so
		// the scroll offset persists in element state (upstream holds
		// an Rc into the state instead).
		gid := w.CurrentElementID()
		hitbox := u.hitbox
		scrollMax := u.state.scrollMax
		w.OnMouseEvent("scroll", 0, hitbox, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase != gpui.BubblePhase {
				return
			}
			if !hitbox.ShouldHandleScroll(w) {
				return
			}
			e := ev.(platform.ScrollEvent)
			delta := e.Delta.PixelDelta(geometry.Px(20))
			gpui.UpdateElementStateAt(w, gid, func() uniformState { return uniformState{} }, func(s *uniformState) {
				s.scroll += delta.Y.AsF32()
				if s.scroll > 0 {
					s.scroll = 0
				}
				if scrollMax > 0 && s.scroll < -scrollMax {
					s.scroll = -scrollMax
				}
			})
		})
	}
}
