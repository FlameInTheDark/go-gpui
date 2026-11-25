// Package gpui is the core of go-gpui: the [Application] and [Window] types,
// the frame lifecycle, the [Element] model, hit testing, focus, keyboard and
// action dispatch, modal prompts, menus, zoom and accessibility recording. It
// is a Go port of the window/element layers of Rust's GPUI. Concrete elements
// such as Div and Text live in the elements package, and most programs that
// open a single window should start from the module root package instead
// (see [github.com/FlameInTheDark/go-gpui]); import this package when you
// need typed views, several windows, explicit platform selection, focus,
// actions, prompts or custom elements.
//
// # Concepts
//
// An [Application] owns a platform backend, an [app.App] (entities and
// effects) and a text system. [OpenView],
// [OpenWindow] and their *With variants open a platform window, wrap it in a
// [Window], create the root state entity and draw the first frame. The
// returned [WindowHandle] gives typed access to the state and to the window.
//
// A [Window] draws immediate-mode style: every frame the render function
// builds a fresh tree of [Element] values, the window lays it out, then walks
// it twice. Nothing diffs trees; state that must survive a frame lives in the
// view entity or in the window's element-state table
// ([WithElementState], [UseElementState], [UpdateElementStateAt]).
//
// # The draw cycle
//
// [Window.Draw] runs one frame, split into phases reported by [DrawPhase]:
//
//   - Layout request: [Element.RequestLayout] is called top-down and each
//     element registers a layout node through [Window.RequestLayout] or
//     [Window.RequestMeasuredLayout]. Layout is then computed for the whole
//     tree at the viewport size.
//   - Prepaint ([PhasePrepaint]): [Element.Prepaint] receives the computed
//     bounds. This is where hitboxes ([Window.InsertHitbox]), tab stops
//     ([Window.InsertTabStop]), focus handles ([Window.SetFocusHandle]) and
//     key contexts ([Window.WithKeyContext]) are registered, and where
//     [Window.DeferDraw] queues popovers. Hover state is resolved right after
//     the main tree has been prepainted.
//   - Paint ([PhasePaint]): [Element.Paint] records primitives
//     ([Window.PaintQuad], [Window.PaintGlyph], ...) into the frame's scene and
//     registers one-frame listeners ([Window.OnMouseEvent],
//     [Window.OnKeyEvent], [OnAction]). Deferred draws, the drag or tooltip
//     overlay, prompts and the debug overlay paint after the main tree, in that
//     order.
//   - Present: the finished scene is handed to the platform window, the two
//     frame buffers are swapped, captured pointers are validated and the
//     resolved cursor is applied.
//
// Listeners, hitboxes and tab stops registered during frame N are used to
// dispatch input that arrives after frame N has been presented, until frame
// N+1 replaces them. Input handlers therefore run between frames, outside any
// phase ([PhaseNone]); never call [Window.Draw] from a handler. Mark the
// window dirty with [Window.Refresh] (or notify the view entity) and the
// framework draws once.
//
// # Threading
//
// A Window, an Application's windows and all element code are confined to the
// UI goroutine, which is the goroutine that runs the platform event loop.
// Input callbacks, render functions and element methods are invoked there.
// With a running native backend, use its DispatchOnMainThread to return worker
// results to the UI loop. The global app dispatcher runs inline when not
// installed, and the headless backend also runs callbacks inline. The
// documented exceptions are [NewFocusHandle], [Application.WindowCount], the
// [PromptResponse] and [PromptInputResponse] accessors and channels, and the
// pure helpers on [ElementId], which are safe for concurrent use.
//
// # Input and actions
//
// Mouse input is delivered to listeners whose hitbox is hovered, in capture
// then bubble order (innermost element first when bubbling). Keyboard input
// goes first to key listeners (optionally bound to a [FocusHandle]) and then
// through the window [Keymap]: a matched binding dispatches its action to the
// [OnAction] listeners registered for that action type. Register Go action
// types with [RegisterActionName] so key bindings can name them.
//
// # Example
//
// A single window with typed state, opened over the platform selected by the
// auto backend:
//
//	type counter struct{ n int }
//
//	func (c *counter) render(w *gpui.Window, cx *app.Context[counter]) gpui.Element {
//		update := cx.Updater()
//		return elements.NewDiv().Child(elements.NewText(fmt.Sprint(c.n))).
//			OnClick(func(_ gpui.ClickEvent, _ *gpui.Window) {
//				update(func(c *counter) { c.n++ })
//			})
//	}
//
//	func main() {
//		p, err := auto.New()
//		if err != nil {
//			log.Fatal(err)
//		}
//		a := gpui.NewApplication(p)
//		_, err = gpui.OpenView(a, platform.WindowOptions{Title: "Counter"},
//			func(*gpui.Window, *app.Context[counter]) counter { return counter{} },
//			(*counter).render)
//		if err != nil {
//			log.Fatal(err)
//		}
//		a.Run()
//	}
//
// See the repository's docs/index.md for task-oriented guides.
package gpui
