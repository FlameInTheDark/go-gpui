package gpui

import (
	"sync/atomic"

	"github.com/FlameInTheDark/go-gpui/keymap"
	"github.com/FlameInTheDark/go-gpui/platform"
)

// DispatchEventResult is the outcome of routing an input event.
type DispatchEventResult = platform.DispatchEventResult

// FocusID identifies a focusable element.
type FocusID uint64

var nextFocusID uint64

// FocusHandle is a refcounted, comparable focus target (upstream
// FocusHandle; Go has no destructors, so release is explicit via the owning
// app shutdown).
type FocusHandle struct {
	id FocusID
}

// NewFocusHandle mints a handle.
func NewFocusHandle() FocusHandle {
	id := atomic.AddUint64(&nextFocusID, 1)
	return FocusHandle{id: FocusID(id)}
}

// FocusID returns the id.
func (f FocusHandle) FocusID() FocusID { return f.id }

// IsValid reports a real handle.
func (f FocusHandle) IsValid() bool { return f.id != 0 }

// Contains reports whether other is within the focused subtree. This port
// tracks containment via the window's rendered focus path.
func (f FocusHandle) Contains(other FocusHandle, w *Window) bool {
	if w == nil {
		return f == other
	}
	return w.focusContains(f, other)
}

// ---------------------------------------------------------------------------
// Focus management on the Window
// ---------------------------------------------------------------------------

// Focus moves focus to the handle.
func (w *Window) Focus(handle FocusHandle) {
	if !w.focusEnabled || !handle.IsValid() {
		return
	}
	if w.focus != nil && w.focus.id == handle.id {
		return
	}
	w.focus = &handle
	w.dirty = true
}

// Blur clears focus.
func (w *Window) Blur() { w.focus = nil }

// Focused returns the focused handle.
func (w *Window) Focused() FocusHandle {
	if w.focus == nil {
		return FocusHandle{}
	}
	return *w.focus
}

// ContainsFocused reports whether the handle contains the focus. Safe
// when nothing is focused (returns false).
func (w *Window) ContainsFocused(handle FocusHandle) bool {
	if w.focus == nil {
		return false
	}
	return w.focusContains(handle, *w.focus)
}

// WithinFocused reports whether the handle is contained by the focus.
// Safe when nothing is focused (returns false).
func (w *Window) WithinFocused(handle FocusHandle) bool {
	if w.focus == nil {
		return false
	}
	return w.focusContains(*w.focus, handle)
}

// focusContains approximates ancestor checks with the id path recorded at
// prepaint: the window records the element path of the focused handle.
func (w *Window) focusContains(ancestor, descendant FocusHandle) bool {
	if !ancestor.IsValid() || !descendant.IsValid() {
		return false
	}
	if ancestor == descendant {
		return true
	}
	// Walk the rendered focus path.
	if path, ok := w.focusPaths[ancestor]; ok {
		for _, id := range path {
			if id == descendant.id {
				return true
			}
		}
	}
	return false
}

// SetFocusHandle records the handle during prepaint (upstream
// window.set_focus_handle).
func (w *Window) SetFocusHandle(handle FocusHandle) {
	if !handle.IsValid() {
		return
	}
	if w.focus != nil && w.focus.id == handle.id {
		w.nextFrame.focus = &handle
		// Record the current element path as the focus path.
		w.focusPaths[handle] = append([]FocusID(nil), w.focusPathStack...)
	}
}

// WithFocusPath pushes a focus handle onto the current focus path (used by
// containers like text inputs).
func (w *Window) WithFocusPath(handle FocusHandle, f func()) {
	if !handle.IsValid() {
		f()
		return
	}
	w.focusPathStack = append(w.focusPathStack, handle.id)
	defer func() { w.focusPathStack = w.focusPathStack[:len(w.focusPathStack)-1] }()
	f()
}

// FocusNext moves focus to the next tab stop.
func (w *Window) FocusNext() {
	stops := w.renderedFrame.tabStops
	if len(stops) == 0 {
		return
	}
	current := w.Focused()
	for i, s := range stops {
		if s.Focus.id == current.id {
			next := stops[(i+1)%len(stops)]
			w.Focus(*next.Focus)
			return
		}
	}
	w.Focus(*stops[0].Focus)
}

// FocusPrev moves focus to the previous tab stop.
func (w *Window) FocusPrev() {
	stops := w.renderedFrame.tabStops
	if len(stops) == 0 {
		return
	}
	current := w.Focused()
	for i, s := range stops {
		if s.Focus.id == current.id {
			prev := stops[(i-1+len(stops))%len(stops)]
			w.Focus(*prev.Focus)
			return
		}
	}
	w.Focus(*stops[len(stops)-1].Focus)
}

// InsertTabStop registers a tab stop during prepaint.
func (w *Window) InsertTabStop(handle FocusHandle, index int) {
	if !handle.IsValid() {
		return
	}
	w.nextFrame.tabStops = append(w.nextFrame.tabStops, TabStop{Focus: &handle, Index: index})
}

// ---------------------------------------------------------------------------
// Keyboard dispatch
// ---------------------------------------------------------------------------

// OnKeyEvent registers a key listener during paint.
func (w *Window) OnKeyEvent(kind string, context string, handler func(ev any, phase DispatchPhase, w *Window)) {
	w.nextFrame.keyListeners = append(w.nextFrame.keyListeners, keyListener{kind: kind, context: context, handler: handler})
}

// OnKeyEventFocused registers a key listener bound to a focus handle: it
// only fires when that handle held focus at the moment the event dispatch
// began (handlers may move focus mid-dispatch; later listeners of the same
// event must not suddenly qualify).
func (w *Window) OnKeyEventFocused(kind string, focus FocusHandle, context string, handler func(ev any, phase DispatchPhase, w *Window)) {
	if !focus.IsValid() {
		w.OnKeyEvent(kind, context, handler)
		return
	}
	f := focus
	w.nextFrame.keyListeners = append(w.nextFrame.keyListeners, keyListener{kind: kind, context: context, handler: handler, focus: &f})
}

// OnAction registers a typed action listener during paint.
func OnAction[A any](w *Window, handler func(a A, phase DispatchPhase, w *Window)) {
	w.nextFrame.actionListeners = append(w.nextFrame.actionListeners, actionListener{
		typeName: actionTypeName[A](),
		listener: func(action any, phase DispatchPhase, w *Window) {
			if a, ok := action.(A); ok {
				handler(a, phase, w)
			}
		},
	})
}

// DispatchKeyEvent routes a key event through key listeners (bubble) and
// the keymap binding pipeline.
func (w *Window) DispatchKeyEvent(ev any) DispatchEventResult {
	w.propagate = true
	w.defaultPrevented = false
	w.listenersFired = 0
	w.lastInputKeyboard = true

	// Framework guarantee: the space bar always carries its character.
	// Some platform backends resolve KeyChar only for single-character
	// key names, so "space" arrived with an empty KeyChar and every
	// `if KeyChar != ""` insert path dropped it. Fill it once here so
	// all text surfaces (editor, terminal, inputs) see it.
	if e, ok := ev.(platform.KeyDownEvent); ok {
		e.Keystroke = e.Keystroke.FillSpaceChar()
		ev = e
	}

	// Key listeners: bubble (innermost first). Focus is snapshotted so a
	// handler that moves focus mid-dispatch cannot re-target later
	// listeners of the same event.
	focusedAtStart := w.Focused()
	listeners := w.renderedFrame.keyListeners
	for i := len(listeners) - 1; i >= 0; i-- {
		l := listeners[i]
		if !w.propagate {
			break
		}
		if l.focus != nil && *l.focus != focusedAtStart {
			continue
		}
		switch ev.(type) {
		case platform.KeyDownEvent:
			if l.kind == "down" {
				w.listenersFired++
				l.handler(ev, BubblePhase, w)
			}
		case platform.KeyUpEvent:
			if l.kind == "up" {
				w.listenersFired++
				l.handler(ev, BubblePhase, w)
			}
		}
	}

	// Binding pipeline: unmatched key-downs flow through the keymap and
	// dispatch the winning action (upstream dispatch_keystroke).
	if w.propagate {
		if e, ok := ev.(platform.KeyDownEvent); ok {
			if w.dispatchKeyDownWithBindings(e) {
				w.listenersFired++
			}
		}
	}
	return DispatchEventResult{Propagate: w.propagate, DefaultPrevented: w.defaultPrevented}
}

// DispatchAction routes an action instance to registered listeners.
func (w *Window) DispatchAction(act any) {
	name := actionNameOf(act)
	w.propagate = false
	for i := len(w.renderedFrame.actionListeners) - 1; i >= 0; i-- {
		al := w.renderedFrame.actionListeners[i]
		if al.typeName != name {
			continue
		}
		al.listener(act, BubblePhase, w)
	}
}

// Keymap is the window's keymap (bound via the app).
type Keymap = keymap.Keymap
