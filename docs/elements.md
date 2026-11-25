# Elements and styling

An element describes part of a frame. Build a fresh element tree in the
render function, and keep durable state in a view or elsewhere outside the
tree. The [tutorial](tutorial.md) walks through a complete application.
The root package offers shortcuts for common elements; import
[`elements`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/elements)
for the rest.

## Choose an element

| Need | Element | Notes |
| --- | --- | --- |
| Container, layout, interaction | `gpui.Div` or `elements.NewDiv` | Add children, styles and event handlers. Root `Row` and `Column` create flex containers. |
| Text | `gpui.Text` or `elements.NewText` | Supports styled runs, caret and highlights; see [Text and fonts](text.md). |
| Image | `elements.NewImg`, `NewImgFromPNG`, `NewImgFromPath`, `NewImgFromURI` | Constructors accept different sources; `NewImgFromPNG` can return a decoding error. |
| SVG | `elements.NewSvg` or `NewSvgFromData` | Load from a path or data. |
| Custom drawing | `elements.NewCanvas` | Provides separate prepaint and paint callbacks with logical-pixel bounds. |
| Large list with equal-height rows | `elements.NewUniformList` | Measures one row and uses its height for the others. |
| Large list with varying row heights | `elements.NewList` | Measures and caches heights as items are visited. |
| Popover positioning | `elements.NewAnchored` | Position relative to a point, rectangle or registered anchor; can avoid window overflow. |
| Overlay drawn after other elements | `elements.NewDeferred` | Higher priority draws above lower priority. Often combined with an anchored element. |
| Tooltip or menu | `elements.NewTooltip`, `elements.NewMenuBar` | `Div` also exposes tooltip helpers. |
| Render based on container size | `elements.NewContainerQuery` | Builds its content using the measured container size. |

See the [element API](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/elements)
for constructors and methods, and the [examples](examples.md) for complete
programs using them. The [virtual-list example](../examples/listdemo/main.go)
shows selection and scrolling.

## Style and interaction

Fluent methods set layout and appearance on elements: for example, `Flex`,
`FlexCol`, `P_4`, `Gap_2`, `BgColor`, and `TextColor`. Numeric spacing
presets use the style scale; `gpui.Px` creates a length in **logical** pixels.
Use the [`style` API](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/style)
for the full set and [geometry](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/geometry)
for units and conversions.

`Div` has handlers for clicks, pointer events, scrolling, key presses, hover,
drag/drop and gestures. Handlers run as part of UI event processing; update
your state there and request a redraw with `Window.Refresh`, or use a typed
view's captured `Context.Updater`. For keyboard input, create and retain a
focus handle and attach it using `TrackFocus`. The tutorial's sections on
[clicks and focus](tutorial.md#2-handle-clicks) and
[typed state](tutorial.md#3-move-growing-state-into-a-typed-view) show both
approaches. Don't access UI state from a worker goroutine: see
[worker results](reactivity.md#worker-results-and-the-ui-thread).
