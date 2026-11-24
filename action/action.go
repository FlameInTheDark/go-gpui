// Package action implements gpui's Action system: named, cloneable,
// dispatchable command objects plus a registry that can build actions from
// JSON input.
package action

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"sync"
)

// Action is a dispatchable command. Implementations are typically unit
// structs registered via Registry.Register.
type Action interface {
	// Name returns the namespaced name, e.g. "editor::MoveUp".
	Name() string
	// BoxedClone clones the action.
	BoxedClone() Action
	// PartialEq compares against another action of any type.
	PartialEq(other Action) bool
}

// Buildable can be constructed from JSON input (keymap loading).
type Buildable interface {
	Action
	BuildAction(input json.RawMessage) error
}

// Unit is embedded into zero-sized named actions — the Go equivalent of the
// `actions!(namespace, [Name, ...])` macro. Define one with:
//
//	type MoveUp struct{ action.Unit[MoveUp] }
//	func (MoveUp) Name() string { return "editor::MoveUp" }
type Unit[T any] struct{}

// BoxedClone implements Action by returning a zero T.
func (Unit[T]) BoxedClone() Action {
	var zero T
	return any(zero).(Action)
}

// PartialEq implements Action; unit actions are equal to any action with
// the same name.
func (Unit[T]) PartialEq(other Action) bool {
	var zero T
	if a, ok := any(zero).(Action); ok {
		return other != nil && other.Name() == a.Name()
	}
	return false
}

// NoAction unbinds the keystrokes it is the highest-precedence match for
// (keymap JSON `"x": null`).
type NoAction struct {
	Unit[NoAction]
}

// Name implements Action.
func (NoAction) Name() string { return "gpui::NoAction" }

// Unbind unbinds bindings whose action name matches, regardless of context.
type Unbind struct {
	Unit[Unbind]
	// Target is the action name to unbind.
	Target string
}

// Name implements Action.
func (Unbind) Name() string { return "gpui::Unbind" }

// BoxedClone implements Action.
func (u Unbind) BoxedClone() Action { return Unbind{Target: u.Target} }

// PartialEq implements Action.
func (u Unbind) PartialEq(other Action) bool {
	o, ok := other.(Unbind)
	return ok && o.Target == u.Target
}

// IsNoAction reports whether the action is NoAction.
func IsNoAction(a Action) bool {
	if a == nil {
		return false
	}
	_, ok := a.(NoAction)
	return ok
}

// IsUnbind reports whether the action is an Unbind and returns its target.
func IsUnbind(a Action) (string, bool) {
	if a == nil {
		return "", false
	}
	u, ok := a.(Unbind)
	return u.Target, ok
}

// Registry maps action names to constructors, mirroring gpui's
// ActionRegistry. Rust uses the inventory crate for auto-registration; Go
// registers explicitly (typically in an init function).
type Registry struct {
	mu        sync.RWMutex
	builders  map[string]func() Action
	typeNames map[reflect.Type]string
	allNames  []string
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		builders:  map[string]func() Action{},
		typeNames: map[reflect.Type]string{},
	}
}

// Register adds a constructor for an action type under its Name(). Panics on
// duplicate names, like upstream.
func (r *Registry) Register(build func() Action) {
	a := build()
	name := a.Name()
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.builders[name]; exists {
		panic(fmt.Sprintf("action %q is already registered", name))
	}
	r.builders[name] = build
	r.typeNames[reflect.TypeOf(a)] = name
	r.allNames = append(r.allNames, name)
}

// BuildAction constructs an action by name from optional JSON input.
func (r *Registry) BuildAction(name string, input json.RawMessage) (Action, error) {
	r.mu.RLock()
	build, ok := r.builders[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("action %q not found", name)
	}
	a := build()
	if len(input) > 0 && string(input) != "null" && string(input) != "{}" {
		b, ok := a.(Buildable)
		if !ok {
			return nil, fmt.Errorf("action %q does not accept input", name)
		}
		if err := b.BuildAction(input); err != nil {
			return nil, fmt.Errorf("building action %q: %w", name, err)
		}
	}
	return a, nil
}

// LookupType returns the action name for a Go type.
func (r *Registry) LookupType(a Action) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if name, ok := r.typeNames[reflect.TypeOf(a)]; ok {
		return name
	}
	return a.Name()
}

// AllNames returns every registered action name in registration order.
func (r *Registry) AllNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]string(nil), r.allNames...)
}

// SortedNames returns every registered action name, sorted.
func (r *Registry) SortedNames() []string {
	names := r.AllNames()
	sort.Strings(names)
	return names
}
