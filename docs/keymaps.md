# Actions and key bindings

The [`action`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/action)
package represents named commands. The
[`keymap`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/keymap)
package matches keystrokes to those commands, optionally depending on the
current context. Use them when shortcuts should invoke named operations rather
than being hard-coded directly into individual key handlers.

## Define an action

For an action without data, embed `action.Unit` and implement `Name`:

```go
type toggle struct{ action.Unit[toggle] }

func (toggle) Name() string { return "example::Toggle" }
```

`Unit` supplies cloning and equality for a zero-valued named action. Actions
with data can implement `action.Action` themselves; a `Buildable` action can
also initialize itself from JSON. An `action.Registry` maps names to
constructors for building actions by name. `Registry.Register` panics if a
name is registered twice.

## Match a shortcut

`NewBinding` parses a space-separated sequence of keystrokes and an optional
context predicate. For example, `secondary-k secondary-t` is a two-stroke
sequence; `secondary` maps to the platform's secondary modifier. A nonempty
context such as `Editor` limits where the shortcut matches.

```go
binding, err := keymap.NewBinding("secondary-k secondary-t", toggle{}, "Editor", keymap.OSLinux)
if err != nil { /* handle invalid binding */ }
keys := keymap.New(keymap.OSLinux)
keys.AddBindings(binding)
```

`BindingsForInput` takes the keystrokes entered so far and a stack of
`KeyContext` values. It returns full matches and a `pending` flag indicating
that a longer matching sequence may still be completed. See the runnable
[`ExampleKeymap_BindingsForInput`](../keymap/example_test.go) for parsing,
context creation and resolution.

Later bindings take precedence, and matching also accounts for context depth.
`action.NoAction` and `action.Unbind` can suppress bindings; see the detailed
rules in [`Keymap.BindingsForInput`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/keymap#Keymap.BindingsForInput).
Use `Keymap.AddBindings` rather than editing the `Bindings` slice directly so
that suppression bookkeeping stays in sync.
