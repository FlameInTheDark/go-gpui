package gpui

import (
	"reflect"
)

var actionTypeNames = map[reflect.Type]string{}

// RegisterActionName associates the Go type of exemplar with an action name
// (replaces Rust's inventory auto-registration). The name is the key under
// which [OnAction] listeners are recorded and matched by
// [Window.DispatchAction], and the name reported by [Window.IsActionAvailable]
// and [Window.AvailableActions]. It should equal the Name method of the
// corresponding [action.Action] implementation, e.g. "editor::MoveUp", so
// that lookups by binding action name agree with listener names. Registering
// the same type again overwrites its name. Types that are never registered
// fall back to their Go type string (such as "main.Quit"), which still lets
// listeners and dispatch of that type match each other.
//
// The registry is a plain map without locking: call RegisterActionName during
// program start-up (init or main, before windows dispatch input), never
// concurrently with dispatch.
func RegisterActionName(exemplar any, name string) {
	actionTypeNames[reflect.TypeOf(exemplar)] = name
}

func actionTypeName[A any]() string {
	var zero A
	if name, ok := actionTypeNames[reflect.TypeOf(zero)]; ok {
		return name
	}
	return reflect.TypeOf(zero).String()
}

func actionNameOf(a any) string {
	if name, ok := actionTypeNames[reflect.TypeOf(a)]; ok {
		return name
	}
	return reflect.TypeOf(a).String()
}
