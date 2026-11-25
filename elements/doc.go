// Package elements provides the building blocks used to construct go-gpui
// element trees. Div is a styled, interactive container; Text displays text;
// Img and Svg display images; Canvas supports custom painting; and List and
// UniformList virtualize long lists. Anchored and Deferred help position and
// paint overlays.
//
// Elements are normally rebuilt in a view's render function. Keep durable
// application state in a view or another owner instead of assuming an element
// instance will survive the next frame. Event handlers and rendering access
// the window on the UI goroutine. For examples and threading guidance, see the
// repository's docs/elements.md and docs/reactivity.md.
package elements
