# Building an app with go-gpui

Start with the [README quick start](../README.md#quick-start) (Go 1.23+). The snippets below introduce one concept at a time; the [counter](../examples/counter/main.go) and [task list](../examples/tutorial/main.go) are complete programs.

## 1. Render a tree of elements

`gpui.Run` selects the desktop backend, opens a window, and runs its event loop. Your render function returns a fresh tree whenever the window draws a frame. Build elements there, but keep application state outside the tree. The framework lays out and paints the result; it does not diff old and new trees.

The examples in this section import `github.com/FlameInTheDark/go-gpui` as `gpui`.

```go
return gpui.Column(
	gpui.Text("Tasks"),
	gpui.Row(gpui.Text("Today"), gpui.Text("2 left")).JustifyBetween(),
).Size_Full().P_4().Gap_2().
	BgColor(gpui.RGB(0x18212f)).TextColor(gpui.RGB(0xffffff))
```

| To do this | Use |
| --- | --- |
| Stack children horizontally or vertically | `gpui.Row(...)`, `gpui.Column(...)` |
| Add children conditionally | `box.Child(element)` or `box.Children(items...)` |
| Fill the window, or set a fixed width | `.Size_Full()`, `.W(gpui.Px(240))` |
| Distribute available space | `.FlexGrow(1)`, `.JustifyBetween()` |
| Align children on the cross axis | `.ItemsCenter()`, `.ItemsStart()` |
| Add spacing | `.P_4()`, `.Px_4()`, `.Gap_2()` (named spacing presets) |
| Color a text subtree or one label | `box.TextColor(c)`, `gpui.Text(value).WithTextColor(c)` |

Sizes passed to `gpui.Px` are **logical pixels**, not physical screen pixels. For per-span styling (such as syntax highlighting), use `Text.WithRuns`; run lengths are UTF-8 byte counts. Plain labels inherit color and font from the parent unless you override them.

## 2. Handle clicks

For a small single-window app, keep state in a local variable captured by the render function. Change it in an event handler and call `w.Refresh()` to mark the window dirty. Don't call `Draw` from an input handler.

```go
count := 0 // outside the render function
err := gpui.Run(gpui.WindowOptions{Title: "Counter"}, func(_ *gpui.Window) gpui.Element {
	return gpui.Column(
		gpui.Text(fmt.Sprintf("Count: %d", count)),
		gpui.Div(gpui.Text("Add one")).P_2().CursorPointer().
			OnClick(func(_ gpui.ClickEvent, w *gpui.Window) {
				count++
				w.Refresh()
			}),
	)
})
if err != nil {
	log.Fatal(err)
}
```

For keyboard interaction, create a focus handle with `NewFocusHandle()` from the core `github.com/FlameInTheDark/go-gpui/gpui` package. Create it once, keep it in view state, and attach it with `.TrackFocus(handle)` on the `Div`. `OnKeyDown` receives key events while that region is focused; `w.Focus(handle)` sets focus programmatically. A focusable `Div` with `OnClick` also responds to Enter and Space. For nested click targets, handlers can bubble from the child to the parent. Use `w.StopPropagation()` when the child should handle the click alone, such as a delete button inside a selectable row.

## 3. Move growing state into a typed view

Choose a typed view when you want the framework to track state changes, inject a backend for tests, or manage multiple windows. Use the same elements; switch from `gpui.Run` to `core.NewApplication` and `core.OpenView`. In this complete program, `gpui` is the top-level package and `core` is the `gpui` subpackage.

```go
package main

import (
	"fmt"
	"log"

	"github.com/FlameInTheDark/go-gpui"
	"github.com/FlameInTheDark/go-gpui/app"
	core "github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/platform/auto"
)

type counter struct{ count int }

func newCounter(*gpui.Window, *app.Context[counter]) counter { return counter{} }

func (s *counter) render(_ *gpui.Window, cx *app.Context[counter]) gpui.Element {
	update := cx.Updater()
	return gpui.Column(
		gpui.Text(fmt.Sprintf("Count: %d", s.count)),
		gpui.Div(gpui.Text("Add one")).P_2().CursorPointer().
			OnClick(func(_ gpui.ClickEvent, _ *gpui.Window) {
				update(func(s *counter) { s.count++ })
			}),
	).P_4().Gap_2()
}

func main() {
	p, err := auto.New()
	if err != nil {
		log.Fatal(err)
	}
	a := core.NewApplication(p)
	if _, err := core.OpenView(a, gpui.WindowOptions{Title: "Typed counter"}, newCounter, (*counter).render); err != nil {
		log.Fatal(err)
	}
	a.Run()
}
```

- The constructor runs once. `OpenView` installs the renderer and draws the initial frame before returning. There is no need to call `SetRootView` or `Draw`.
- Treat render as read-only. Capture `cx.Updater()` during render, then invoke it **later** from a UI event handler. It runs the mutation and calls `Notify`, so no separate `Refresh` is needed. Don't call it while the same entity is already being updated.
- `OpenView` returns a `WindowHandle[T]`: `View` is the state entity and `Window` is the core window. For richer updates, use `handle.Update(a, ...)` and call `cx.Notify()` after changing the state. `OpenViewWith` adds a platform callback hook.
- Existing `OpenWindow` apps can keep using the `core.Render` interface. Use `OpenView` when you'd rather pass an explicit renderer with a typed context.

## 4. Grow the task list

Run `go run ./examples/tutorial` from the repository root. Its source shows how the following pieces fit together:

- **State and layout:** `taskList` stores the tasks, draft text, selection, and focus handles. `render` composes the header, input, rows, and footer.
- **Input and focus:** `renderInput` builds a small single-line field from text, a caret, and key handlers; it is not a general-purpose text field with selection, clipboard, or IME support. `renderList` assigns keyboard navigation to the focused list.
- **Styling:** `Hover(func(r *style.StyleRefinement) { ... })` changes styling without changing application state. Explicit text runs let completed tasks use strikethrough.
- **Prompts:** `Window.Prompt` displays action choices. Wait for `resp.Changed()` in a worker, then check `resp.Index()`. For a text prompt use `PromptForInput` and `resp.Value()`. Never block an input handler waiting for the answer. The example also shows a close veto on backends that honor `OnShouldClose` (Windows).
- **Background work:** on native backends, use `p.DispatchOnMainThread(func() { ... })` before applying a worker result to a window or entity. The headless backend executes dispatched callbacks inline, so in tests drive UI work from your test goroutine. See `renderFooter` for the prompt-to-state pattern.

For longer lists, use [`elements.NewUniformList`](../elements/uniform_list.go) for fixed-height rows or [`elements.NewList`](../elements/list.go) for variable-height rows. See the [examples guide](examples.md) for scrolling, images, SVG, canvas, and editor demos.

## 5. Test without a display

Use `headless.New` (from `github.com/FlameInTheDark/go-gpui/platform/headless`) instead of `auto.New`. This test can live next to the typed counter above:

```go
func TestCounterDraws(t *testing.T) {
	p := headless.NewDefault()
	a := core.NewApplication(p)
	if _, err := core.OpenView(a, gpui.WindowOptions{Title: "test"}, newCounter, (*counter).render); err != nil {
		t.Fatal(err)
	}
	if _, err := p.LastWindow().PNG(); err != nil {
		t.Fatal(err)
	}
}
```

No event loop is needed. Use `p.LastWindow().DispatchInput(...)` to send mouse or keyboard events, then check state, the rendered scene, or decoded pixels. The [task-list test](../examples/tutorial/tutorial_test.go) checks the first frame, and the [quick-start API test](../ui_test.go) simulates a click.

For package responsibilities and the frame lifecycle, see [Architecture](architecture.md).
