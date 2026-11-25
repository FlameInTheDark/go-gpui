# Documentation

New to go-gpui? Start with the [quick start](../README.md#quick-start), then
follow the [tutorial](tutorial.md) to build and test an interactive UI. For a
working program, browse the [examples](examples.md). The [Go API reference](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui)
lists exported types and methods; use the package links below to jump directly
to the relevant API.

## Find a feature

| I want to… | Start here |
| --- | --- |
| Open one window and build a UI | [Quick start](../README.md#quick-start), [root API](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui) |
| Manage typed view state or multiple windows | [Tutorial: typed views](tutorial.md#3-move-growing-state-into-a-typed-view), [`gpui` API](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/gpui) |
| Send signals or observe state changes | [State, events, and goroutines](reactivity.md), [`app` API](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/app) |
| Update a UI after background work | [Worker results and the UI thread](reactivity.md#worker-results-and-the-ui-thread) |
| Add boxes, labels, images, SVG or custom drawing | [Elements and styling](elements.md), [`elements` API](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/elements), [examples](examples.md) |
| Style, size and lay out elements | [Tutorial: render a tree](tutorial.md#1-render-a-tree-of-elements), [`style` API](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/style), [`layout` API](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/layout) |
| Handle clicks, keyboard focus and input | [Tutorial: handle clicks](tutorial.md#2-handle-clicks), [task-list example](../examples/tutorial/main.go) |
| Display a long scrolling list | [Virtual-list example](../examples/listdemo/main.go), [`List`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/elements#List), [`UniformList`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/elements#UniformList) |
| Display, shape or measure text; load fonts | [Text and fonts](text.md), [`text` API](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/text) |
| Configure shortcuts and actions | [Actions and key bindings](keymaps.md), [`keymap` API](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/keymap), [`action` API](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/action) |
| Use prompts, menus, zoom or gestures | [`gpui` API](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/gpui), [`gestures` API](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/gestures), [examples](examples.md) |
| Test without a display or capture a PNG | [Platforms and headless testing](platform.md#headless-tests), [tutorial](tutorial.md#5-test-without-a-display), [debugging rendering](examples.md#debugging-rendering) |
| Understand rendering and the platform backends | [Architecture](architecture.md), [`render` API](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/render), [`platform` API](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/platform) |
| Investigate performance | [Performance and profiling](performance.md) |

## Package directory

These packages are part of the module `github.com/FlameInTheDark/go-gpui`.
Most applications start with the root package and only import lower-level
packages as needed. Package links below lead to Go documentation.

| Package | Responsibility |
| --- | --- |
| [root](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui) | Convenience API for a single native window and common elements, colors and lengths. |
| [`gpui`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/gpui) | Applications, windows, typed views, focus, input dispatch, prompts and painting. |
| [`app`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/app) | Entities, contexts, effects, observers, events and executors. |
| [`elements`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/elements) | UI building blocks, including containers, text, images, canvas and lists. |
| [`style`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/style) | Style refinements, text styles and builder methods. |
| [`layout`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/layout) | Layout tree and flex, block and grid algorithms. |
| [`geometry`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/geometry) | Pixels, lengths, points, bounds and other geometry. |
| [`color`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/color) | Colors, conversions, backgrounds and gradients. |
| [`font`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/font), [`text`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/text), [`text/sfnt`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/text/sfnt) | Font descriptions, text shaping and rasterization, and font-file parsing. |
| [`render`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/render) | Scenes, primitives, sprite atlases and rasterization. |
| [`platform`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/platform) | Backend interfaces, window options and platform input events. |
| [`platform/auto`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/platform/auto) | Select a native backend for the current platform. |
| [`platform/headless`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/platform/headless) | In-memory windows for testing, synthetic input and snapshots. |
| [`platform/windows`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/platform/windows), [`platform/x11`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/platform/x11), [`platform/wayland`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/platform/wayland) | OS-specific window and rendering backends. Availability and features vary by backend. |
| [`keymap`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/keymap), [`action`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/action) | Keystrokes, bindings and named actions. |
| [`gestures`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/gestures) | Gesture recognition. |
| [`spring`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/spring) | Spring animation calculations. |

For package relationships and the frame lifecycle, read [Architecture](architecture.md).
For version notes, see [Releases](releases.md).
