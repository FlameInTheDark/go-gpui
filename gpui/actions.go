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
// apply to every window; per-window keymaps are also supported).
func (w *Window) SetKeymap(k *keymap.Keymap) { w.keymap = k }

// KeymapOf returns the window keymap.
func (w *Window) KeymapOf() *keymap.Keymap { return w.keymap }

// WithKeyContext pushes a key context for f (upstream
// window.with_key_context, used during prepaint). Contexts pushed during
// prepaint become the dispatch context stack for the next input events.
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

// BindingsForActionName lists enabled bindings whose action name matches.
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
// the action in the current key contexts.
func (w *Window) HighestPrecedenceBindingForAction(actName string) (keymap.Binding, bool) {
	for _, b := range w.BindingsForActionName(actName) {
		if _, ok := w.keymap.BindingEnabled(b, w.keyContextStack); ok {
			return b, true
		}
	}
	return keymap.Binding{}, false
}

// IsActionAvailable reports whether any listener was registered for the
// action this frame (upstream window.is_action_available).
func (w *Window) IsActionAvailable(actName string) bool {
	for _, al := range w.renderedFrame.actionListeners {
		if al.typeName == actName {
			return true
		}
	}
	return false
}

// AvailableActions lists action names with registered listeners.
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

// PendingKeystrokes returns the unmatched multi-key prefix typed so far.
func (w *Window) PendingKeystrokes() []keymap.Keystroke {
	return append([]keymap.Keystroke(nil), w.pendingKeystrokes...)
}

// HasPendingKeystrokes reports a partially matched multi-key binding.
func (w *Window) HasPendingKeystrokes() bool { return len(w.pendingKeystrokes) > 0 }

// PossibleBindingsForInput lists bindings that extend the typed prefix (for
// which-key style UIs).
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
