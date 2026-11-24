package elements

import (
	"sync"

	"github.com/FlameInTheDark/go-gpui/geometry"
)

// ScrollHandle observes and drives a scrollable div's offset from outside
// the element tree (up gpui::ScrollHandle). A div opts in with
// TrackScroll(handle); every prepaint the handle mirrors the live offset,
// content size and viewport, and programmatic requests (SetTop,
// ScrollToBottom, Autoscroll) apply on the next prepaint — the same
// next-frame timing as upstream request_scroll_to.
//
// The zero value is not usable; construct with NewScrollHandle.
type ScrollHandle struct {
	mu sync.Mutex

	// offset mirrors the tracked div's scroll offset (<= 0, window
	// convention: scrolling down moves content up).
	offset geometry.Point[geometry.Pixels]
	// contentSize is the tracked div's scrollable extent.
	contentSize geometry.Size[geometry.Pixels]
	// viewportSize is the tracked div's visible size.
	viewportSize geometry.Size[geometry.Pixels]

	// requests
	pendingTop  *geometry.Pixels // logical top to scroll to (>= 0)
	autoFollow  bool             // keep following the bottom
	pendingLeft *geometry.Pixels // logical left to scroll to (>= 0)
}

// NewScrollHandle creates a handle.
func NewScrollHandle() *ScrollHandle {
	return &ScrollHandle{}
}

// Offset returns the live scroll offset (<= 0; scrolling down decreases Y).
func (h *ScrollHandle) Offset() geometry.Point[geometry.Pixels] {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.offset
}

// Top returns the logical top offset (>= 0) of the viewport in the content.
func (h *ScrollHandle) Top() geometry.Pixels {
	h.mu.Lock()
	defer h.mu.Unlock()
	return -h.offset.Y
}

// ContentSize returns the tracked content's full scrollable extent.
func (h *ScrollHandle) ContentSize() geometry.Size[geometry.Pixels] {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.contentSize
}

// ViewportSize returns the tracked div's visible size.
func (h *ScrollHandle) ViewportSize() geometry.Size[geometry.Pixels] {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.viewportSize
}

// MaxTop returns the largest logical top offset (content - viewport).
func (h *ScrollHandle) MaxTop() geometry.Pixels {
	h.mu.Lock()
	defer h.mu.Unlock()
	max := h.contentSize.Height.AsF32() - h.viewportSize.Height.AsF32()
	if max < 0 {
		max = 0
	}
	return geometry.Px(max)
}

// Left returns the logical left offset (>= 0) of the viewport in the
// content (horizontal scroll containers).
func (h *ScrollHandle) Left() geometry.Pixels {
	h.mu.Lock()
	defer h.mu.Unlock()
	return -h.offset.X
}

// MaxLeft returns the largest logical left offset (content - viewport).
func (h *ScrollHandle) MaxLeft() geometry.Pixels {
	h.mu.Lock()
	defer h.mu.Unlock()
	max := h.contentSize.Width.AsF32() - h.viewportSize.Width.AsF32()
	if max < 0 {
		max = 0
	}
	return geometry.Px(max)
}

// SetLeft requests a logical left offset (>= 0, clamped); it applies on the
// next prepaint of the tracked div (SetTop's horizontal twin).
func (h *ScrollHandle) SetLeft(left geometry.Pixels) {
	h.mu.Lock()
	defer h.mu.Unlock()
	l := left
	if l.AsF32() < 0 {
		l = 0
	}
	h.pendingLeft = &l
}

// ScrollTopBy adds delta to the pending logical top offset, accumulating
// same-frame requests (upstream's scroll_by semantics): several wheel
// events between frames must sum, while Top() mirrors only the last
// prepaint. It applies on the next prepaint of the tracked div.
func (h *ScrollHandle) ScrollTopBy(delta float32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	base := -h.offset.Y.AsF32()
	if h.pendingTop != nil {
		base = h.pendingTop.AsF32()
	}
	tv := base + delta
	if tv < 0 {
		tv = 0
	}
	tp := geometry.Px(tv)
	h.pendingTop = &tp
	h.autoFollow = false
}

// ScrollLeftBy adds delta to the pending logical left offset (the
// accumulating horizontal twin of ScrollTopBy).
func (h *ScrollHandle) ScrollLeftBy(delta float32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	base := -h.offset.X.AsF32()
	if h.pendingLeft != nil {
		base = h.pendingLeft.AsF32()
	}
	lv := base + delta
	if lv < 0 {
		lv = 0
	}
	lp := geometry.Px(lv)
	h.pendingLeft = &lp
}

// SetTop requests a logical top offset (>= 0, clamped); it applies on the
// next prepaint of the tracked div (upstream request_scroll_to timing).
func (h *ScrollHandle) SetTop(top geometry.Pixels) {
	h.mu.Lock()
	defer h.mu.Unlock()
	t := top
	if t.AsF32() < 0 {
		t = 0
	}
	h.pendingTop = &t
	h.autoFollow = false
}

// ScrollToTop scrolls the tracked div to the top.
func (h *ScrollHandle) ScrollToTop() { h.SetTop(geometry.Px(0)) }

// ScrollToBottom scrolls the tracked div to the bottom once.
func (h *ScrollHandle) ScrollToBottom() { h.SetTop(h.MaxTop()) }

// Autoscroll marks the handle to follow the bottom: every prepaint of the
// tracked div clamps its offset to the newest content bottom (upstream
// autoscroll / scroll-to-bottom behavior, e.g. terminal tail-following).
func (h *ScrollHandle) Autoscroll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.autoFollow = true
}

// IsAutoscrolling reports whether the handle currently follows the bottom.
func (h *ScrollHandle) IsAutoscrolling() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.autoFollow
}

// StopAutoscroll clears bottom-following (a user scroll-up cancels it).
func (h *ScrollHandle) StopAutoscroll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.autoFollow = false
}

// peek returns and consumes pending requests without mirroring (the
// caller applies them and then calls sync with the final offset).
func (h *ScrollHandle) peek() (pendingTop *geometry.Pixels, autoFollow bool, pendingLeft *geometry.Pixels) {
	h.mu.Lock()
	defer h.mu.Unlock()
	pendingTop = h.pendingTop
	h.pendingTop = nil
	pendingLeft = h.pendingLeft
	h.pendingLeft = nil
	return pendingTop, h.autoFollow, pendingLeft
}

// sync mirrors the tracked div's state into the handle (called from the
// div's prepaint after requests and clamping applied).
func (h *ScrollHandle) sync(offset geometry.Point[geometry.Pixels], content, viewport geometry.Size[geometry.Pixels]) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.offset = offset
	h.contentSize = content
	h.viewportSize = viewport
}

// syncNow mirrors a fresh offset into the handle without consuming
// requests (called from the div's scroll listener after user input).
func (h *ScrollHandle) syncNow(offset geometry.Point[geometry.Pixels]) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.offset = offset
}

// clearAutoFollow cancels bottom-following (user wheel/pan input).
func (h *ScrollHandle) clearAutoFollow() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.autoFollow = false
}
