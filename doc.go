// Package gpui is the starting point for building desktop UIs in Go with
// go-gpui, a pure-Go toolkit inspired by Rust's GPUI.
//
// This root package is a small convenience layer for the common case of one
// native window: [Run] picks a platform backend, opens a window and runs the
// event loop, while [Div], [Row], [Column], [Text], [Px] and [RGB] build the
// element tree. The types [Element], [Window], [ClickEvent] and
// [WindowOptions] are aliases, so values from this package interoperate with
// the subpackages without conversion.
//
// For typed view state, several windows, explicit platform selection, focus
// management, actions, prompts, menus or custom elements, import the
// github.com/FlameInTheDark/go-gpui/gpui subpackage (conventionally renamed to
// core when both are used), the elements package for the full set of
// elements, and platform/auto or platform/headless for backends.
//
// # Model
//
// Run calls the render function every time the window draws a frame and the
// returned tree is laid out and painted from scratch; trees are not diffed.
// Keep application state outside the tree, change it in event handlers and
// call [Window.Refresh] so the window redraws. All of this happens on the UI
// goroutine. With a running native backend, use the platform's
// DispatchOnMainThread to hand worker results back to the UI loop. Headless
// dispatch runs inline; see docs/reactivity.md for threading details.
//
// # Example
//
//	package main
//
//	import (
//		"fmt"
//		"log"
//
//		"github.com/FlameInTheDark/go-gpui"
//	)
//
//	func main() {
//		count := 0
//		err := gpui.Run(gpui.WindowOptions{Title: "Counter"}, func(*gpui.Window) gpui.Element {
//			return gpui.Column(
//				gpui.Text(fmt.Sprintf("Count: %d", count)),
//				gpui.Div(gpui.Text("Add one")).P_2().CursorPointer().
//					OnClick(func(_ gpui.ClickEvent, w *gpui.Window) {
//						count++
//						w.Refresh()
//					}),
//			).P_4().Gap_2().BgColor(gpui.RGB(0x18212f)).TextColor(gpui.RGB(0xffffff))
//		})
//		if err != nil {
//			log.Fatal(err)
//		}
//	}
//
// See docs/index.md in the repository for guides and package references.
package gpui
