# Examples and troubleshooting

Run these from the repository root. They open a native window unless otherwise noted.

| Example | Run | What to look at |
| --- | --- | --- |
| [Counter](../examples/counter) | `go run ./examples/counter` | The smallest interactive app: one window, a flex column, and a click handler. |
| [Task list](../examples/tutorial) | `go run ./examples/tutorial` | Typed view state, keyboard focus, a simple input, and modal prompts. See the [tutorial](tutorial.md). |
| [Hello](../examples/hello) | `go run ./examples/hello` | Hover styling, cards, gradients, SVG, prompts, and zoom. |
| [Gallery](../examples/gallery) | `go run ./examples/gallery` | Menus, painting primitives, bidirectional text, notifications, file drop, and an autoscrolling log. |
| [Virtual list](../examples/listdemo) | `go run ./examples/listdemo` | A 1,000-row list with wheel scrolling, selection, and a scroll handle. |

## Flint editor

The former editor example is now [Flint](https://github.com/FlameInTheDark/flint), a separate application built with go-gpui. Its repository has the build instructions and editor documentation.

## Debugging rendering

The hello, gallery, and list demo share `--headless`, `--out DIR`, `--timeout DURATION`, `--scene FILE`, and `--debug`. The counter and tutorial use simpler entry points and do not parse these flags.

```sh
go run ./examples/hello --headless --out frames
go run ./examples/gallery --debug --timeout 10s
```

Headless runs write PNGs without a display server. For native Linux testing under Xvfb, launch Xvfb separately and set `DISPLAY` before running an example. `cmd/x11shot` captures the X11 root screen, and `cmd/x11live` can capture or drive a window through X11.

On Windows, choose the renderer with `GPUI_RENDERER=auto|hardware|warp|gdi` (`gdi` forces software). `GPUI_VSYNC=0` disables vsync. With `GPUI_DEBUG=1`, the backend logs which renderer it selected.
