package gpui

import (
	"github.com/FlameInTheDark/go-gpui/action"
	"github.com/FlameInTheDark/go-gpui/keymap"
	"github.com/FlameInTheDark/go-gpui/platform"
)

// This file ports upstream's action pipeline: keystrokes flow through the
// window keymap (bindings + key contexts) and dispatch the highest-precedence
// matching action (window.dispatch_keystroke / bindings_for_action).

// SetKeymap installs the window's keymap (upstream App-level key bindings
// apply to every window; per-window keymaps are also supported). Each window
// has its own keymap; a nil keymap disables binding dispatch. The keymap is
// consulted at input time, so it may be replaced or edited at any point
// between frames. Call on the UI goroutine.
func (w *Window) SetKeymap(k *keymap.Keymap) { w.keymap = k }

// KeymapOf returns the window keymap, or nil if none was installed.
func (w *Window) KeymapOf() *keymap.Keymap { return w.keymap }

// WithKeyContext pushes the key context ctx for the duration of f (upstream
// window.with_key_context). Call it during prepaint. Contexts pushed during
// prepaint become the dispatch context stack for the next input events, and
// bindings with a context predicate are matched against that stack. ctx uses
// the keymap context syntax ("Editor", "Editor mode=insert"); an empty or
// unparsable ctx just runs f without pushing anything.
func (w *Window) WithKeyContext(ctx string, f func()) {
	if ctx == "" {
		f()
		return
	}
	kc, err := keymap.NewContext(ctx)
	if err != nil {
		f()
		return
	}
	w.prepaintKeyContexts = append(w.prepaintKeyContexts, kc)
	w.nextFrameKeyContexts = append(w.nextFrameKeyContexts, kc)
	defer func() { w.prepaintKeyContexts = w.prepaintKeyContexts[:len(w.prepaintKeyContexts)-1] }()
	f()
}

// dispatchKeystroke runs the binding pipeline for a keystroke (upstream
// Window::dispatch_keystroke). Returns the matched action, if any.
func (w *Window) dispatchKeystroke(ks keymap.Keystroke) (matched action.Action, pending bool) {
	if w.keymap == nil || ks.IsModifierKey() {
		return nil, false
	}
	w.pendingKeystrokes = append(w.pendingKeystrokes, ks)
	bindings, pending := w.keymap.BindingsForInput(w.pendingKeystrokes, w.keyContextStack)
	if pending {
		return nil, true
	}
	w.pendingKeystrokes = nil
	if len(bindings) == 0 {
		return nil, false
	}
	return bindings[len(bindings)-1].Action, false
}

// BindingsForActionName lists the bindings whose action's Name equals
// actName and that are enabled under the current key context stack, with the
// most recently added binding first (highest precedence first). It returns
// nil when no keymap is installed. Use it to show shortcut hints.
func (w *Window) BindingsForActionName(actName string) []keymap.Binding {
	if w.keymap == nil {
		return nil
	}
	var out []keymap.Binding
	for i := len(w.keymap.Bindings) - 1; i >= 0; i-- {
		b := w.keymap.Bindings[i]
		if b.Action == nil || b.Action.Name() != actName {
			continue
		}
		if _, ok := w.keymap.BindingEnabled(b, w.keyContextStack); ok {
			out = append(out, b)
		}
	}
	return out
}

// HighestPrecedenceBindingForAction returns the binding that would win for
// the action named actName in the current key contexts, and false if there is
// none (including when no keymap is installed).
func (w *Window) HighestPrecedenceBindingForAction(actName string) (keymap.Binding, bool) {
	for _, b := range w.BindingsForActionName(actName) {
		if _, ok := w.keymap.BindingEnabled(b, w.keyContextStack); ok {
			return b, true
		}
	}
	return keymap.Binding{}, false
}

// IsActionAvailable reports whether any [OnAction] listener was registered for
// the action named actName in the last rendered frame (upstream
// window.is_action_available). Names are those given to [RegisterActionName],
// or Go type strings for unregistered types. Useful to enable or disable menu
// items.
func (w *Window) IsActionAvailable(actName string) bool {
	for _, al := range w.renderedFrame.actionListeners {
		if al.typeName == actName {
			return true
		}
	}
	return false
}

// AvailableActions lists the distinct names of actions that have listeners in
// the last rendered frame, in first-registered-first order.
func (w *Window) AvailableActions() []string {
	seen := map[string]bool{}
	var out []string
	for _, al := range w.renderedFrame.actionListeners {
		if !seen[al.typeName] {
			seen[al.typeName] = true
			out = append(out, al.typeName)
		}
	}
	return out
}

// PendingKeystrokes returns a copy of the multi-key prefix typed so far that
// partially matches a binding (for example "ctrl-k" while waiting for the
// second chord). It is empty when no multi-key sequence is in progress.
func (w *Window) PendingKeystrokes() []keymap.Keystroke {
	return append([]keymap.Keystroke(nil), w.pendingKeystrokes...)
}

// HasPendingKeystrokes reports whether a multi-key binding is partially
// matched, that is, whether [Window.PendingKeystrokes] is non-empty.
func (w *Window) HasPendingKeystrokes() bool { return len(w.pendingKeystrokes) > 0 }

// PossibleBindingsForInput lists the bindings that could complete the
// pending multi-key prefix under the current key contexts (for which-key
// style UIs). It returns nil when no keymap is installed.
func (w *Window) PossibleBindingsForInput() []keymap.Binding {
	if w.keymap == nil {
		return nil
	}
	return w.keymap.PossibleNextBindings(w.pendingKeystrokes, w.keyContextStack)
}

// dispatchKeyDownWithBindings routes a key-down through the binding pipeline
// after key listeners (upstream dispatch_keystroke).
func (w *Window) dispatchKeyDownWithBindings(ev platform.KeyDownEvent) bool {
	if act, _ := w.dispatchKeystroke(ev.Keystroke); act != nil {
		w.DispatchAction(act)
		return true
	}
	return false
}
