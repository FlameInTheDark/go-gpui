# Architecture

How go-gpui works under the API you use to build apps. Read
[the tutorial](tutorial.md) first for the application-facing view;
this document goes one level down.

## The big picture

```
your app            elements/style/layout        gpui                  render                     platform
-----------         -----------------------      ----                  ------                     --------
taskList state  ->  Div/Text/List element tree  Window draw cycle  -> Scene (quads, sprites,   auto -> win32 + D3D11
(update+notify)     (flexbox layout via taffy)   (prepaint/paint)      paths, shadows)            port     x11 / wayland blit
                                                                                        |
                                                                                   Rasterizer (CPU)  <- headless + fallback
```

One frame, top to bottom:

1. **Invalidation** — an effect (`Notify`, `Refresh`, a subscription
   firing) marks the window dirty.
2. **Draw cycle** — `Window.Draw` runs in phases: layout request, layout
   compute, prepaint (hover/hit-test resolution, deferred draws), paint
   (elements emit primitives into a fresh `render.Scene`).
3. **Present** — the platform backend consumes the scene: Direct3D 11
   records one instanced draw per frame on Windows; the software
   rasterizer paints the same scene to a pixel buffer everywhere else
   (and as the automatic fallback).

Elements are rebuilt every frame; cheap construction plus memoized
layout/text/raster caches make that affordable. Nothing diffs trees.

## Packages and their roles

| Package | Role |
| --- | --- |
| Module root (`go-gpui`) | Single-window `Run` and common element, length, and color constructors. |
| `geometry` | Typed units: `Pixels` (logical), `DevicePixels` (physical), `Rems`; points, sizes, bounds, edges, matrices. All layout math is unit-safe arithmetic. |
| `color` | `Rgba`/`Hsla`, Oklab gradients, palette, backgrounds (solid, gradient, patterns). |
| `font`, `text`, `text/sfnt` | The text stack (below). |
| `style` | `StyleRefinement` — the cascade of properties elements style themselves with; ~190 generated builder methods (Tailwind-style names) forward onto it. |
| `layout` | Flexbox + block + grid, ported from the taffy algorithms: parametrized axes, intrinsic sizing, min/max, percentages, absolute children. |
| `app` | The reactive core: entities (slotmap with generation tags), contexts, the six-effect queue (notify/emit/refresh/defer/…), observers, subscriptions, priority background tasks. |
| `gpui` | Application and typed root views (`OpenView`); draw-cycle phases, focus, hitboxes and listener dispatch, painting helpers, prompts, zoom, gestures, accessibility tree, debug overlay. |
| `elements` | The widget kit: `Div`, `Text`, `Canvas`, `Img`, `Svg`, `List`, `UniformList`, `Anchored`, `Deferred`, tooltips, menu bar. |
| `render` | The scene graph and everything that turns it into pixels: sprite atlas, path builder, GPU instance builder, the software rasterizer, shadows and gradient math. |
| `platform` | Backend contracts: events, window options, cursors, notifications, file dialogs; `auto` picks the OS backend. |
| `platform/windows` | win32 + Direct3D 11, pure `syscall` COM — no cgo. |
| `platform/x11`, `platform/wayland` | Raw wire-protocol clients (no libxcb/libwayland). |
| `platform/headless` | Offscreen backend for tests and PNG snapshots. |
| `keymap`, `action` | Keystrokes, modifiers, multi-stroke bindings, action registry. |
| `spring` | Analytic spring integrator for animations. |

## The draw cycle in detail

`Window.Draw()` (in `gpui/window.go`) is the heart:

1. **Next-frame callbacks** run (`runNextFrameCallbacks` — the
   `on_next_frame` hook for animations).
2. **Layout request + compute** — every element builds a layout node
   (`layout.NewTree`); taffy-style algorithms resolve sizes. Container
   subtrees are memoized per pass.
3. **Prepaint** — elements resolve per-frame state that depends on final
   bounds: hover hit-testing, drag overlays, tooltips, deferred draw
   priorities, scroll offsets. The hitbox list (pointer listener
   registration) is collected here.
4. **Paint** — elements walk their bounds and emit primitives:
   `PaintQuad` (SDF-rounded rects, borders), text sprites, paths,
   shadows. Ordering is tree order; `Deferred` + priority escapes it for
   overlays.
5. **Frame finish** — the scene is closed, focus events processed, hover
   cursor applied, accessed entities recorded for invalidation routing.

Input is dispatched *between* frames against the previous frame's
hitboxes — the same model as upstream gpui — so handlers always see a
consistent frame.

## The reactive core (`app`)

A **view state** is an *entity*: a slot in a slotmap with a generation
tag. `WindowHandle[T].View` is a generation-checked handle: if the
entity is released, the handle resolves to nothing instead of
dangling.

Effects flow through one FIFO queue with deduplication, in upstream's
exact order: `Notify` (view dirty → redraw), `Emit` (event to
subscribers), `RefreshWindows`, `NotifyGlobalObservers`, `Defer`,
`EntityCreated`. An update's effects flush after the closure returns;
a `Notify` during a draw flushes as a catch-up frame immediately after
(the nested `Draw` at the end of the cycle).

Observers/subscriptions track entity access automatically: whatever
your render reads is what triggers re-render when it changes.

## The text stack

Pure Go, end to end:

- `text/sfnt` parses SFNT/TrueType/OpenType tables (glyf + composite
  glyphs, CFF Type2 charstrings, cmap, hmtx, kern, name, OS/2).
- The shaper runs GSUB (ligatures, Arabic positional forms) and
  GPOS/kern pair positioning, with per-rune font fallback and sticky
  fallback spans.
- `LayoutLine` memoizes shaped lines (the editor's hot path); BiDi
  visual reordering and UAX#14-style line breaking are applied where
  relevant.
- The rasterizer is analytic scanline coverage with anti-aliasing;
  glyph bitmaps land in the sprite atlas, keyed by (font, size, rune).
- The atlas prunes stale pages and caps its texture count; dirty
  regions upload to the GPU incrementally.

## Rendering: one shader math, two backends

`render.Scene` is backend-independent: an ordered list of quads
(SDF-rounded, borders, gradients, shadows), monochrome/polychrome
sprite references, and path/curve/paint buffers.

- **Direct3D 11** (`platform/windows/d3d11.go`) — COM interfaces are
  called through hand-audited vtable slots over `syscall`; the HLSL
  shaders port the reference WGSL: instanced quad expansion, per-pixel
  SDF coverage, dashed borders, analytic (Levien-style) shadows, wavy
  underlines, Oklab/sRGB gradients with Bayer dithering. Presentation
  is a flip-model swap chain with vsync, driven exclusively from the
  message pump's `WM_PAINT` (presenting from any other thread deadlocks
  with DWM). A device loss (driver reset) is detected through the
  device-removed reason; the renderer is recreated once, then the
  window falls back to software permanently.
- **Software rasterizer** (`render/`) — the same shader math as a
  per-pixel CPU implementation. It backs the Linux backends, the
  headless platform and the Windows GDI fallback, with flat-fill fast
  paths and reused frame buffers.

DPI: layout, hit-testing and mouse input live in logical pixels;
present lives in physical pixels; `Window.ScaleFactor()` converts.
Windows listens to `WM_DPICHANGED` to keep the logical size constant
across monitors.

## Platform backends

- **windows** — win32 window + message pump; D3D11 present; CSD
  (custom title bar) via `WM_NCCALCSIZE`; native file dialogs through
  `IFileOpenDialog` COM.
- **x11** — a hand-rolled X11 wire client: handshake, window, SHM
  PutImage blit; `cmd/x11shot`/`cmd/x11live` use `GetImage` and XTEST
  for CI screenshots and synthetic input.
- **wayland** — a raw Wayland client speaking the wire protocol (no
  libwayland): xdg-shell + wl_shm.
- **headless** — renders to memory, dispatches synthetic input; the
  window tests and `--headless` example smoke modes use it.

Platform features are not at parity with each other or with Rust GPUI.
The Go runtime has no native macOS backend. Linux presentation is software;
the Windows backend can use Direct3D 11. Features such as native file
dialogs, close vetoes, and IME vary by backend. The framework provides an
accessibility tree, which is separate from exposing it to OS assistive tools.

## Performance model

Where the frame budget goes and what the framework does about it:

- **Text** — shaped lines, kern pairs and per-rune fallback are
  memoized; unchanged text is a cache hit (an editor's dominant case).
- **Layout** — container subtrees are memoized per pass.
- **Rasterization** — flat-fill fast path for solid unbordered rects;
  atlas uploads are incremental (dirty regions).
- **Presentation** — input floods coalesce into one GPU frame per
  paint; the post-dispatch redraw is skipped when the effect flush
  already drew.
- The debug overlay (`--debug` / `GPUI_DEBUG=1`) shows FPS, frame times
  and per-frame primitive counts; `--scene FILE` dumps the raw scene.

See [Performance and profiling](performance.md) for repeatable benchmarks,
measured results, and the next hotspots to investigate.

## Testing strategy

Run `go test ./...` for the full suite, or `go test . ./app ./gpui ./elements ./examples/tutorial` for the main API and UI tests. Window tests use the headless backend for pixel assertions and synthetic input. Some text tests assume specific system fonts (e.g. DejaVu Sans). See the [headless testing guide](tutorial.md#5-test-without-a-display) to write your own.
