package keymap

import (
	"fmt"
	"sort"
	"strings"

	"github.com/FlameInTheDark/go-gpui/action"
)

// Binding is one key binding: keystrokes, action, optional context
// predicate and a source-tier index (smaller = stronger, user bindings
// override defaults).
type Binding struct {
	Action    action.Action
	Keystroke []Keystroke // multi-stroke, e.g. cmd-k + cmd-t
	Predicate Predicate
	Meta      uint32 // source tier; NoAction suppression compares tiers
	// ActionInput is the raw JSON input for display purposes.
	ActionInput string
}

// NewBinding parses keystrokes and a context predicate.
func NewBinding(keystrokes string, act action.Action, context string, os OS) (Binding, error) {
	var ks []Keystroke
	for _, tok := range strings.Fields(keystrokes) {
		if tok == "" {
			continue
		}
		k, err := ParseKeystroke(tok, os)
		if err != nil {
			return Binding{}, err
		}
		ks = append(ks, k)
	}
	if len(ks) == 0 {
		return Binding{}, fmt.Errorf("empty keystrokes")
	}
	var pred Predicate
	if context != "" {
		var err error
		pred, err = ParsePredicate(context)
		if err != nil {
			return Binding{}, err
		}
	}
	return Binding{Action: act, Keystroke: ks, Predicate: pred}, nil
}

// matchKeystrokes returns (matched, pending): matched is true on a full
// match; pending is true when the typed input is a strict prefix of this
// binding's keystrokes.
func (b Binding) matchKeystrokes(typed []Keystroke, os OS) (full, pending bool) {
	if len(b.Keystroke) < len(typed) {
		return false, false
	}
	for i := range typed {
		if !typed[i].shouldMatch(b.Keystroke[i], os) {
			return false, false
		}
	}
	if len(b.Keystroke) > len(typed) {
		return false, true
	}
	return true, false
}

// Keymap is the ordered binding list plus resolution.
type Keymap struct {
	Bindings        []Binding
	disabledIndices []int
	os              OS
}

// New creates a keymap for the given OS.
func New(os OS) *Keymap { return &Keymap{os: os} }

// OS returns the keymap's OS.
func (k *Keymap) OS() OS { return k.os }

// AddBindings appends bindings (later bindings take precedence).
func (k *Keymap) AddBindings(bindings ...Binding) {
	for _, b := range bindings {
		if action.IsNoAction(b.Action) {
			k.disabledIndices = append(k.disabledIndices, len(k.Bindings))
		} else if _, isUnbind := action.IsUnbind(b.Action); isUnbind {
			k.disabledIndices = append(k.disabledIndices, len(k.Bindings))
		}
		k.Bindings = append(k.Bindings, b)
	}
}

// Clear removes all bindings.
func (k *Keymap) Clear() {
	k.Bindings = nil
	k.disabledIndices = nil
}

type resolvedBinding struct {
	binding Binding
	index   int
	depth   int
}

// BindingsForInput resolves the bindings that should fire for the typed
// input, plus whether input is pending (a prefix of some binding). This is a
// faithful port of Keymap::resolve_bindings_for_input:
//
//   - bindings are processed in reverse insertion order (later = stronger);
//   - matches sort by predicate depth desc, then index desc;
//   - a NoAction suppresses bindings from equal-or-weaker source tiers
//     (meta >= the minimum NoAction meta seen so far in sorted order);
//   - an Unbind removes later bindings with the same keystrokes + action;
//   - pendings with index below the first match are superseded; NoAction or
//     Unbind pendings remove their keystrokes from the pending set.
func (k *Keymap) BindingsForInput(input []Keystroke, contextStack []KeyContext) (matched []Binding, pending bool) {
	type matchedEntry struct {
		depth int
		index int
		b     Binding
	}
	var matches []matchedEntry
	var pendings []matchedEntry

	for i := len(k.Bindings) - 1; i >= 0; i-- {
		b := k.Bindings[i]
		full, isPending := b.matchKeystrokes(input, k.os)
		if !full && !isPending {
			continue
		}
		depth, ok := k.bindingEnabled(b, contextStack)
		if !ok {
			continue
		}
		e := matchedEntry{depth: depth, index: i, b: b}
		if full {
			matches = append(matches, e)
		} else {
			pendings = append(pendings, e)
		}
	}

	sort.SliceStable(matches, func(a, b int) bool {
		if matches[a].depth != matches[b].depth {
			return matches[a].depth > matches[b].depth
		}
		return matches[a].index > matches[b].index
	})

	var bindings []Binding
	firstBindingIndex := -1
	var unboundBindings []Binding
	noActionMeta := int64(-1)

	for _, m := range matches {
		meta := int64(m.b.Meta)
		if action.IsNoAction(m.b.Action) {
			if noActionMeta == -1 || meta < noActionMeta {
				noActionMeta = meta
			}
			continue
		}
		if noActionMeta != -1 && meta >= noActionMeta {
			continue
		}
		if _, isUnbind := action.IsUnbind(m.b.Action); isUnbind {
			unboundBindings = append(unboundBindings, m.b)
			continue
		}
		if anyUnbound(unboundBindings, m.b) {
			continue
		}
		bindings = append(bindings, m.b)
		if firstBindingIndex == -1 {
			firstBindingIndex = m.index
		}
	}

	pendingSet := map[string]bool{}
	// Rust iterates pending_bindings in reverse; we collected them in
	// reverse index order already, so iterate forward to mirror .rev().
	for i := len(pendings) - 1; i >= 0; i-- {
		p := pendings[i]
		if firstBindingIndex != -1 && firstBindingIndex > p.index {
			continue
		}
		if action.IsNoAction(p.b.Action) || func() bool { _, ok := action.IsUnbind(p.b.Action); return ok }() {
			delete(pendingSet, keystrokesKey(p.b.Keystroke))
			continue
		}
		pendingSet[keystrokesKey(p.b.Keystroke)] = true
	}

	return bindings, len(pendingSet) > 0
}

// bindingEnabled returns the predicate depth if the binding is enabled.
func (k *Keymap) bindingEnabled(b Binding, contextStack []KeyContext) (int, bool) {
	if b.Predicate == nil {
		return len(contextStack), true
	}
	d := DepthOf(b.Predicate, contextStack)
	if d == 0 {
		// DepthOf returns 0 for "no match" and for a genuinely empty stack.
		if len(contextStack) == 0 && PredicateEnabled(b.Predicate, contextStack) {
			return 0, true
		}
		return 0, false
	}
	return d, true
}

func anyUnbound(unbound []Binding, b Binding) bool {
	for _, u := range unbound {
		if target, ok := action.IsUnbind(u.Action); ok {
			if target == b.Action.Name() && sameKeystrokes(u.Keystroke, b.Keystroke) {
				return true
			}
		}
	}
	return false
}

func keystrokesKey(ks []Keystroke) string {
	var sb strings.Builder
	for _, k := range ks {
		sb.WriteString(k.Modifiers.String())
		sb.WriteByte('-')
		sb.WriteString(k.Key)
		sb.WriteByte(';')
	}
	return sb.String()
}

func sameKeystrokes(a, b []Keystroke) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Key != b[i].Key || !a[i].Modifiers.Equal(b[i].Modifiers) {
			return false
		}
	}
	return true
}

// BindingsForAction returns the bindings bound to an action, filtering
// disabled ones.
func (k *Keymap) BindingsForAction(act action.Action) []Binding {
	var out []Binding
	for i := len(k.Bindings) - 1; i >= 0; i-- {
		b := k.Bindings[i]
		if b.Action.Name() != act.Name() || !b.Action.PartialEq(act) {
			continue
		}
		if k.bindingUnbound(b, i) {
			continue
		}
		out = append(out, b)
	}
	return out
}

func (k *Keymap) bindingUnbound(b Binding, idx int) bool {
	for i := len(k.Bindings) - 1; i > idx; i-- {
		other := k.Bindings[i]
		if target, ok := action.IsUnbind(other.Action); ok {
			if target == b.Action.Name() && sameKeystrokes(other.Keystroke, b.Keystroke) {
				return true
			}
		}
		if action.IsNoAction(other.Action) && other.Meta <= b.Meta &&
			sameKeystrokes(other.Keystroke, b.Keystroke) {
			// NoAction with the same keystrokes in a matching-or-wider
			// context suppresses (approximated by tier comparison).
			return true
		}
	}
	return false
}

// PossibleNextBindings returns bindings that extend the typed input, for
// which-key UIs.
func (k *Keymap) PossibleNextBindings(input []Keystroke, contextStack []KeyContext) []Binding {
	var out []Binding
	for _, b := range k.Bindings {
		if len(b.Keystroke) <= len(input) {
			continue
		}
		match := true
		for i := range input {
			if !input[i].shouldMatch(b.Keystroke[i], k.os) {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		// The full binding must actually win dispatch.
		if _, pending := k.BindingsForInput(b.Keystroke, contextStack); pending {
			continue
		}
		full, _ := b.matchKeystrokes(b.Keystroke, k.os)
		_ = full
		out = append(out, b)
	}
	return out
}

// AllBindingsForInput returns every full match regardless of context.
func (k *Keymap) AllBindingsForInput(input []Keystroke) []Binding {
	var out []Binding
	for i := len(k.Bindings) - 1; i >= 0; i-- {
		if full, _ := k.Bindings[i].matchKeystrokes(input, k.os); full {
			out = append(out, k.Bindings[i])
		}
	}
	return out
}

// BindingEnabled reports whether a binding's predicate is satisfied by the
// context stack, returning the match depth (upstream exposes this via
// resolve_bindings internals).
func (k *Keymap) BindingEnabled(b Binding, contextStack []KeyContext) (int, bool) {
	return k.bindingEnabled(b, contextStack)
}
