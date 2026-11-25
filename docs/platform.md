# Platforms and headless testing

The [`platform`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/platform)
package defines windows, input events and operating-system services. Most
applications do not implement it themselves: the root `gpui.Run` selects a
native platform, while applications using typed views can explicitly create
one with `auto.New` and pass it to `gpui.NewApplication`.

## Backend selection

| Backend | Selection | Rendering and notes |
| --- | --- | --- |
| Windows | `platform/auto.New` on Windows | Native Win32 windows; Direct3D 11 with software fallback. |
| X11 | `auto.New` on supported Unix builds when `DISPLAY` is set | Native X11 window and software rendering; also works with XWayland. |
| Wayland | `auto.New` on supported Unix builds when `DISPLAY` is unset and `WAYLAND_DISPLAY` is set | Native Wayland window and software rendering. |
| Headless | Explicit `platform/headless.New` or `NewDefault` | In-memory windows and software rendering for tests and snapshots. |

On Unix, `auto.New` returns an error if neither display environment variable is
set; it chooses based on the variables and does not retry Wayland if an X11
initialization fails. The auto backend returns an error on unsupported OSes,
including macOS. Native dialogs, clipboard, close vetoes, input methods and
accessibility integration vary by backend; consult the implementation before
depending on a particular service.

The [tutorial's typed-view example](tutorial.md#3-move-growing-state-into-a-typed-view)
shows backend initialization, window creation and the event loop. Create the
backend, open windows and call `Application.Run` on the same goroutine. See
[State, events, and goroutines](reactivity.md#worker-results-and-the-ui-thread)
before accessing app or window state from background work.

## Headless tests

`headless.NewDefault()` provides an 800×600 logical-pixel viewport;
`headless.New(size)` specifies another size. Pass it to
`gpui.NewApplication` and open a view as you would with a native platform.
`OpenView` draws the first frame, so a basic snapshot does not need an event
loop. Use `p.LastWindow().PNG()` to encode the last image, or
`p.LastWindow().DispatchInput(...)` to simulate input. The [tutorial](tutorial.md#5-test-without-a-display)
contains a test and links to runnable examples.

Headless `Run` returns immediately, and `DispatchOnMainThread` calls the
function **inline**, on the caller's goroutine. It is not a mechanism for
transferring worker results to the goroutine that owns the app. Keep test UI
operations on the owning goroutine; transfer results over a channel first.

## Implementing a backend

A backend implements [`platform.Platform`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/platform#Platform)
and [`platform.PlatformWindow`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/platform#PlatformWindow).
`OpenWindow` receives window options and callbacks for input, resize, frame
requests and lifecycle events. The backend presents a `render.Scene` through
the window's `Draw` method and supplies a text system and sprite atlas. Use
the [headless implementation](../platform/headless/headless.go) as a compact
starting point and the [architecture guide](architecture.md#platform-backends)
for the native backends. Document any unavailable services and the scheduling
semantics of your dispatcher; do not assume all implementations queue it.
