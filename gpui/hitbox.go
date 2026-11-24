package gpui

import (
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/render"
)

// nextHitboxID mints monotonic hitbox ids.
var nextHitboxID HitboxID = 1

// InsertHitbox registers a hitbox during prepaint.
func (w *Window) InsertHitbox(bounds geometry.Bounds[geometry.Pixels], behavior HitboxBehavior) Hitbox {
	return w.insertHitbox(bounds, behavior, "")
}

// InsertKeyedHitbox registers a hitbox carrying its element's stable
// identity (keyed captures survive structural re-renders; see Hitbox.Key).
func (w *Window) InsertKeyedHitbox(bounds geometry.Bounds[geometry.Pixels], behavior HitboxBehavior, key string) Hitbox {
	return w.insertHitbox(bounds, behavior, key)
}

func (w *Window) insertHitbox(bounds geometry.Bounds[geometry.Pixels], behavior HitboxBehavior, key string) Hitbox {
	hb := Hitbox{
		ID:          nextHitboxID,
		Bounds:      bounds,
		ContentMask: render.ContentMask{Bounds: geometry.ScaleBounds(w.contentMask, w.ScaleFactor())},
		Behavior:    behavior,
		Key:         key,
	}
	nextHitboxID++
	w.nextFrame.hitboxes = append(w.nextFrame.hitboxes, hb)
	return hb
}

// hitTestOf walks hitboxes front-to-back (reverse insertion), collecting
// ids under the position until a blocking hitbox (upstream Frame::hit_test).
// Bounds are intersected with the hitbox's content mask first (upstream
// hit_test intersects with content_mask.bounds): children clipped by a
// scroll container's mask must not catch input outside the visible area.
func (w *Window) hitTestOf(hitboxes []Hitbox, position geometry.Point[geometry.Pixels]) hitTest {
	var ht hitTest
	for i := len(hitboxes) - 1; i >= 0; i-- {
		hb := hitboxes[i]
		if !geometry.BoundsContains(hb.Bounds, position) {
			continue
		}
		if mask := w.maskBoundsOf(hb); mask.Size.Width > 0 && mask.Size.Height > 0 &&
			!geometry.BoundsContains(mask, position) {
			continue
		}
		ht.ids = append(ht.ids, hb.ID)
		if hb.Behavior == HitboxBlockMouseExceptScroll {
			ht.hoverHitboxCount = len(ht.ids)
		}
		if hb.Behavior == HitboxBlockMouse {
			break
		}
	}
	if ht.hoverHitboxCount == 0 {
		ht.hoverHitboxCount = len(ht.ids)
	}
	return ht
}

// maskBoundsOf converts a hitbox's device-space content mask back to
// logical pixels for hit testing.
func (w *Window) maskBoundsOf(hb Hitbox) geometry.Bounds[geometry.Pixels] {
	if w.scaleFactor <= 0 || w.scaleFactor == 1 {
		mb := hb.ContentMask.Bounds
		return geometry.Bounds[geometry.Pixels]{
			Origin: geometry.Point[geometry.Pixels]{X: geometry.Px(mb.Origin.X.AsF32()), Y: geometry.Px(mb.Origin.Y.AsF32())},
			Size:   geometry.Size[geometry.Pixels]{Width: geometry.Px(mb.Size.Width.AsF32()), Height: geometry.Px(mb.Size.Height.AsF32())},
		}
	}
	inv := 1 / w.ScaleFactor()
	mb := hb.ContentMask.Bounds
	return geometry.Bounds[geometry.Pixels]{
		Origin: geometry.Point[geometry.Pixels]{X: geometry.Px(mb.Origin.X.AsF32() * inv), Y: geometry.Px(mb.Origin.Y.AsF32() * inv)},
		Size:   geometry.Size[geometry.Pixels]{Width: geometry.Px(mb.Size.Width.AsF32() * inv), Height: geometry.Px(mb.Size.Height.AsF32() * inv)},
	}
}

// HitTest runs hit testing against the rendered frame (event dispatch).
func (w *Window) HitTest(position geometry.Point[geometry.Pixels]) hitTest {
	return w.hitTestOf(w.renderedFrame.hitboxes, position)
}

// hitboxByID finds a hitbox of the current frame.
func (w *Window) hitboxByID(id HitboxID) (Hitbox, bool) {
	for _, hb := range w.nextFrame.hitboxes {
		if hb.ID == id {
			return hb, true
		}
	}
	for _, hb := range w.renderedFrame.hitboxes {
		if hb.ID == id {
			return hb, true
		}
	}
	return Hitbox{}, false
}

// captureMatches reports whether hb is the pointer-captured element: by
// stable element key when the capturer had one (drags survive structural
// re-renders), by numeric id otherwise.
func (w *Window) captureMatches(hb Hitbox) bool {
	if w.capturedHitbox == nil {
		return false
	}
	if w.capturedKey != "" {
		return hb.Key == w.capturedKey
	}
	return *w.capturedHitbox == hb.ID
}

// IsHovered reports whether the hitbox is under the mouse (upstream
// Hitbox::is_hovered; keyboard modality suppresses hover).
func (h Hitbox) IsHovered(w *Window) bool {
	if w.captureMatches(h) {
		return true
	}
	if w.lastInputKeyboard {
		return false
	}
	for i, id := range w.mouseHitTest.ids {
		if id == h.ID {
			return i < w.mouseHitTest.hoverHitboxCount
		}
	}
	return false
}

// ShouldHandleScroll reports scroll eligibility (scroll passes through
// BlockMouseExceptScroll overlays).
func (h Hitbox) ShouldHandleScroll(w *Window) bool {
	if w.captureMatches(h) {
		return true
	}
	for _, id := range w.mouseHitTest.ids {
		if id == h.ID {
			return true
		}
	}
	return false
}

// OnMouseEvent registers a mouse listener during paint (one-frame lifetime).
func (w *Window) OnMouseEvent(kind string, button platform.MouseButton, hitbox Hitbox, capture bool, handler func(ev any, phase DispatchPhase, w *Window)) {
	w.nextFrame.mouseListeners = append(w.nextFrame.mouseListeners, mouseListener{
		kind: kind, button: button, hitbox: hitbox, handler: handler, capture: capture,
		depth: w.listenerDepth, seq: len(w.nextFrame.mouseListeners),
	})
}

// ListenerHitbox returns the hitbox of the mouse listener currently being
// dispatched. Inside a mouse handler it reports the bounds of the element
// that registered the listener (upstream exposes this via the element path
// the handler runs on), letting handlers translate event positions into
// element-local coordinates without reaching back into layout.
func (w *Window) ListenerHitbox() (Hitbox, bool) {
	if w.activeListenerHitbox == nil {
		return Hitbox{}, false
	}
	return *w.activeListenerHitbox, true
}

// StopPropagation stops event bubbling.
func (w *Window) StopPropagation() { w.propagate = false }

// FocusMovedThisDispatch reports whether focus changed since the current
// mouse event's dispatch began. Focusable containers check it in their
// focus-on-click handlers so the innermost focused element wins (a text
// input inside a TrackFocus modal keeps the focus it claimed on click
// instead of the modal stealing it back in the same event's bubble phase).
func (w *Window) FocusMovedThisDispatch() bool {
	return w.dispatchFocusStart != w.Focused()
}

// Propagate reports whether propagation continues.
func (w *Window) Propagate() bool { return w.propagate }

// PreventDefault marks the default action handled.
func (w *Window) PreventDefault() { w.defaultPrevented = true }

// DefaultPrevented reports the default-prevented state.
func (w *Window) DefaultPrevented() bool { return w.defaultPrevented }

// DispatchMouseEvent routes a mouse event: capture phase in registration
// order (parents first), bubble phase in reverse (children first).
func (w *Window) DispatchMouseEvent(ev any) DispatchEventResult {
	w.propagate = true
	w.defaultPrevented = false
	w.listenersFired = 0
	w.mouseHitTest = w.HitTest(w.mousePosition)
	// Snapshot the focused handle so nested elements can detect that a
	// deeper listener already moved focus during THIS event (the innermost
	// click-focus must win; see Div's focus-on-click).
	w.dispatchFocusStart = w.Focused()

	listeners := w.renderedFrame.mouseListeners
	// The slice is pre-sorted in bubble order (innermost first). Capture
	// runs it backwards: outermost first (upstream capture semantics).
	for i := len(listeners) - 1; i >= 0; i-- {
		l := listeners[i]
		if !l.capture || !w.propagate {
			continue
		}
		if w.listenerMatches(l, ev) {
			w.listenersFired++
			hb := l.hitbox
			w.activeListenerHitbox = &hb
			l.handler(ev, CapturePhase, w)
		}
	}
	// Bubble: innermost first.
	for i := 0; i < len(listeners); i++ {
		l := listeners[i]
		if l.capture || !w.propagate {
			continue
		}
		if w.listenerMatches(l, ev) {
			w.listenersFired++
			hb := l.hitbox
			w.activeListenerHitbox = &hb
			l.handler(ev, BubblePhase, w)
		}
	}
	w.activeListenerHitbox = nil
	return DispatchEventResult{Propagate: w.propagate, DefaultPrevented: w.defaultPrevented}
}

func (w *Window) listenerMatches(l mouseListener, ev any) bool {
	// Pointer capture is exclusive while a drag is in flight (DOM
	// pointer-capture semantics): move, up and down events belong to the
	// capturing element alone. The match runs through captureMatches so a
	// keyed capture stays valid across re-renders that renumber hitboxes
	// (scrollbars appearing mid-drag no longer drop a resize gesture — and
	// the moves no longer leak to whatever element the renumbered id
	// happens to point at). Exit listeners stay live so hover state still
	// cleans up when the cursor leaves a box.
	if w.capturedHitbox != nil && !w.captureMatches(l.hitbox) {
		switch ev.(type) {
		case platform.MouseUpEvent, platform.MouseDownEvent:
			return false
		case platform.MouseMoveEvent:
			if l.kind != "exit" {
				return false
			}
		}
	}
	switch ev := ev.(type) {
	case platform.MouseDownEvent:
		if l.kind != "down" && l.kind != "any-down" && l.kind != "down-out" {
			return false
		}
		if l.kind == "down-out" {
			// Capture-phase only, fires when the press is OUTSIDE the box.
			if geometry.BoundsContains(l.hitbox.Bounds, w.mousePosition) {
				return false
			}
			return true
		}
		if l.kind == "down" && ev.Button != l.button {
			return false
		}
		return l.hitbox.IsHovered(w)
	case platform.MouseUpEvent:
		if l.kind != "up" && l.kind != "any-up" {
			return false
		}
		if l.kind == "up" && ev.Button != l.button {
			return false
		}
		return l.hitbox.IsHovered(w)
	case platform.MouseMoveEvent:
		if l.kind != "move" && l.kind != "exit" {
			return false
		}
		if l.kind == "exit" {
			return !l.hitbox.IsHovered(w)
		}
		return l.hitbox.IsHovered(w)
	case platform.MouseExitEvent:
		return l.kind == "exit"
	case platform.ScrollEvent:
		return l.kind == "scroll" && l.hitbox.ShouldHandleScroll(w)
	case platform.PinchEvent:
		return l.kind == "pinch" && l.hitbox.IsHovered(w)
	case platform.TouchDragEvent:
		return l.kind == "touch-drag" && l.hitbox.IsHovered(w)
	case platform.LongPressEvent:
		return l.kind == "long-press" && l.hitbox.IsHovered(w)
	case platform.BackGestureEvent:
		// Every phase is delivered at the start position, so the
		// innermost element under it claims the whole gesture (upstream).
		return l.kind == "back-gesture" && geometry.BoundsContains(l.hitbox.Bounds, w.mousePosition)
	case platform.FileDropEvent:
		return l.kind == "file-drop" && l.hitbox.IsHovered(w)
	}
	return false
}

// MousePosition returns the tracked pointer position.
func (w *Window) MousePosition() geometry.Point[geometry.Pixels] { return w.mousePosition }

// SetMousePosition updates the tracked position (platform input handler).
func (w *Window) SetMousePosition(p geometry.Point[geometry.Pixels]) { w.mousePosition = p }

// Modifiers returns the tracked modifier state.
func (w *Window) Modifiers() map[string]bool {
	return map[string]bool{
		"control":  w.modifiers.Control,
		"alt":      w.modifiers.Alt,
		"shift":    w.modifiers.Shift,
		"platform": w.modifiers.Platform,
		"function": w.modifiers.Function,
	}
}

// SetModifiers updates the modifier state.
func (w *Window) SetModifiers(m map[string]bool) {
	w.modifiers.Control = m["control"]
	w.modifiers.Alt = m["alt"]
	w.modifiers.Shift = m["shift"]
	w.modifiers.Platform = m["platform"]
	w.modifiers.Function = m["function"]
}

// CapturePointer captures the mouse for a hitbox. The hitbox's element
// key (when it has one) is remembered too: later frames match the capture
// by key, surviving structural re-renders that renumber hitbox ids.
func (w *Window) CapturePointer(id HitboxID) {
	w.capturedHitbox = &id
	w.capturedKey = w.hitboxKeyOf(id)
}

// hitboxKeyOf resolves the element key of a hitbox id, preferring the
// frame the dispatching listeners came from (renderedFrame); nextFrame
// (the older, post-swap frame) is the fallback.
func (w *Window) hitboxKeyOf(id HitboxID) string {
	for _, hb := range w.renderedFrame.hitboxes {
		if hb.ID == id {
			return hb.Key
		}
	}
	for _, hb := range w.nextFrame.hitboxes {
		if hb.ID == id {
			return hb.Key
		}
	}
	return ""
}

// ReleasePointer ends capture.
func (w *Window) ReleasePointer() {
	w.capturedHitbox = nil
	w.capturedKey = ""
}

// ValidateCapture drops pointer captures that no longer resolve to any
// hitbox of the freshly rendered frame (the element disappeared, or the
// id renumbered onto a different element with no key). Without this a
// stale capture silenced every subsequent press — "lost the ability to
// click on anything" — until some handler happened to release it.
func (w *Window) ValidateCapture() {
	if w.capturedHitbox == nil {
		return
	}
	for _, hb := range w.renderedFrame.hitboxes {
		if w.capturedKey != "" {
			if hb.Key == w.capturedKey {
				// Re-anchor the numeric id to the current frame so
				// captureMatches' id fallback stays coherent.
				id := hb.ID
				w.capturedHitbox = &id
				return
			}
		} else if hb.ID == *w.capturedHitbox {
			return
		}
	}
	w.ReleasePointer()
}

var _ = render.ContentMask{}
