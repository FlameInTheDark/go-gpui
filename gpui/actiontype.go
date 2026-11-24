package gpui

import (
	"reflect"
)

var actionTypeNames = map[reflect.Type]string{}

// RegisterActionName associates a Go type with an action name so keymap
// bindings can target it (replaces Rust's inventory auto-registration).
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
