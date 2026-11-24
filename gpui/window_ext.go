package gpui

import (
	"sort"

	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/layout"
	"github.com/FlameInTheDark/go-gpui/render"
	"github.com/FlameInTheDark/go-gpui/style"
)

// This file adds Window APIs that upstream exposes but the port initially
// deferred: frame scheduling, titles, paint layers, strikethrough, element
// namespaces and invalidation state.

// OnNextFrame schedules f to run at the start of the next draw (upstream
// window.on_next_frame). Use it to drive per-frame animations.
func (w *Window) OnNextFrame(f func(w *Window)) {
	w.nextFrameCallbacks = append(w.nextFrameCallbacks, f)
	w.dirty = true
}

// RequestAnimationFrame is an alias of OnNextFrame matching the browser API
// naming (upstream request_animation_frame).
func (w *Window) RequestAnimationFrame(f func(w *Window)) { w.OnNextFrame(f) }

// runNextFrameCallbacks drains scheduled callbacks at draw start.
func (w *Window) runNextFrameCallbacks() {
	if len(w.nextFrameCallbacks) == 0 {
		return
	}
	cbs := w.nextFrameCallbacks
	w.nextFrameCallbacks = nil
	for _, f := range cbs {
		f(w)
	}
}

// SetWindowTitle sets the platform window title.
func (w *Window) SetWindowTitle(title string) { w.plat.SetTitle(title) }

// WindowTitle returns the platform window title.
func (w *Window) WindowTitle() string { return w.plat.Title() }

// IsDirty reports a pending redraw (upstream window.is_dirty).
func (w *Window) IsDirty() bool { return w.dirty }

// NotDrawing reports whether no draw is in progress.
func (w *Window) NotDrawing() bool { return w.drawPhase == PhaseNone }

// Refresh marks the window dirty (upstream window.refresh).
func (w *Window) Refresh() { w.dirty = true }

// ElementTableSize reports the number of tracked element-to-layout-node
// entries (a test/telemetry hook). The table is rebuilt every frame; a
// count that grows across frames is a leak.
func (w *Window) ElementTableSize() int { return len(w.elementNode) }

// IsFocused reports whether the window's platform window is active.
func (w *Window) IsFocused() bool { return w.plat.IsActive() }

// ComputeElementLayout lays an element out as an independent root within
// the given definite size (upstream layout_as_root). Returns the layout
// node, which the caller passes to PrepaintElementAt.
func (w *Window) ComputeElementLayout(e Element, size geometry.Size[geometry.Pixels]) LayoutID {
	node := w.RequestLayoutElement(e)
	w.layoutTree.ComputeLayout(layout.Node(node), layout.DefiniteAvailable(
		size.Width.AsF32(), size.Height.AsF32()))
	w.SetNodeFor(e, node)
	return node
}

// IsWindowActive reports whether the window is the active window.
func (w *Window) IsWindowActive() bool { return w.plat.IsActive() }

// LastInputWasKeyboard reports the last input source.
func (w *Window) LastInputWasKeyboard() bool { return w.lastInputKeyboard }

// LineHeight returns the current text style's line height.
func (w *Window) LineHeight() geometry.Pixels {
	return geometry.Px(w.textStyle.LineHeightInPixels(w.RemSize()))
}

// TextSize returns the current font size.
func (w *Window) TextSize() geometry.Pixels {
	return w.textStyle.FontSize.ToPixels(w.RemSize())
}

// PaintLayer opens a paint layer: primitives painted until the returned
// close func draw above everything painted before the layer opened (upstream
// window.paint_layer).
func (w *Window) PaintLayer() func() {
	w.nextFrame.scene.PushLayer()
	return w.nextFrame.scene.PopLayer
}

// PaintStrikethrough paints a strikethrough line (upstream
// window.paint_strikethrough).
func (w *Window) PaintStrikethrough(origin geometry.Point[geometry.Pixels], width, thickness geometry.Pixels, st style.UnderlineStyle) {
	// The strike line runs at half the ascent of the line.
	w.PaintUnderline(origin, width, thickness, st)
}

// elementNamespace namespaces element ids (upstream with_element_namespace):
// ids pushed inside f are prefixed, so the same id in sibling lists does not
// collide.
func (w *Window) WithElementNamespace(ns ElementId, f func()) {
	if ns.IsZero() {
		f()
		return
	}
	w.idPath.push(ns)
	defer w.idPath.pop()
	f()
}

// RenderedScene returns the last presented scene (upstream
// window.rendered_scene).
func (w *Window) RenderedScene() *render.Scene { return w.renderedFrame.scene }

// PaintedQuads returns the current frame's quads (upstream
// window.painted_quads, used by tests).
func (w *Window) PaintedQuads() []render.Quad { return w.renderedFrame.scene.Quads }

// PaintedUnderlines returns the current frame's underlines.
func (w *Window) PaintedUnderlines() []render.Underline { return w.renderedFrame.scene.Underlines }

// RenderedHitboxes returns the last presented frame's hitboxes (used by tests
// and example verification tooling to locate interactive elements).
func (w *Window) RenderedHitboxes() []Hitbox { return w.renderedFrame.hitboxes }

// HitTestIDs returns the hitbox ids under a position, front-to-back (upstream
// Frame::hit_test result ids, exposed for tests).
func (w *Window) HitTestIDs(position geometry.Point[geometry.Pixels]) []HitboxID {
	return w.hitTestOf(w.renderedFrame.hitboxes, position).ids
}

// sortFrames is a helper kept for API symmetry.
func sortFrames[T any](s []T, less func(a, b T) bool) {
	sort.SliceStable(s, func(i, j int) bool { return less(s[i], s[j]) })
}
