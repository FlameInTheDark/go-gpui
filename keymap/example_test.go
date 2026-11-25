package keymap_test

import (
	"fmt"

	"github.com/FlameInTheDark/go-gpui/action"
	"github.com/FlameInTheDark/go-gpui/keymap"
)

type toggle struct{ action.Unit[toggle] }

func (toggle) Name() string { return "example::Toggle" }

func ExampleKeymap_BindingsForInput() {
	const os = keymap.OSLinux
	binding, err := keymap.NewBinding("secondary-k secondary-t", toggle{}, "Editor", os)
	if err != nil {
		panic(err)
	}
	keys := keymap.New(os)
	keys.AddBindings(binding)
	context, err := keymap.NewContext("Editor")
	if err != nil {
		panic(err)
	}
	first, _ := keymap.ParseKeystroke("secondary-k", os)
	second, _ := keymap.ParseKeystroke("secondary-t", os)
	_, pending := keys.BindingsForInput([]keymap.Keystroke{first}, []keymap.KeyContext{context})
	matches, _ := keys.BindingsForInput([]keymap.Keystroke{first, second}, []keymap.KeyContext{context})
	fmt.Println(pending, matches[0].Action.Name())
	// Output: true example::Toggle
}
