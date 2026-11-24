// Package demo is the shared runner for go-gpui examples. Every example is
// a fully interactive application by default: it opens a real native window
// (win32 on Windows, X11/Wayland on Linux) and runs until the user closes
// it. The --headless debug flag swaps in the headless platform so the
// example renders frames to PNGs and self-verifies instead, which is what
// CI runs do.
//
// Flags (shared by all examples):
//
//	--headless        render to PNGs and exit instead of opening a window
//	--out DIR         output directory for --headless PNGs (default ".")
//	--timeout DURATION  quit the interactive window after this duration
//	                   (0 = run until the window is closed)
package demo

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/FlameInTheDark/go-gpui/app"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/platform/auto"
	"github.com/FlameInTheDark/go-gpui/platform/headless"
	"github.com/FlameInTheDark/go-gpui/render"
)

// Config describes one example program.
type Config struct {
	// Name prefixes errors and PNG filenames (e.g. "editor").
	Name string
	// Title is the native window title.
	Title string
	// Size is the initial window size and the headless viewport.
	Size geometry.Size[geometry.Pixels]
	// Timeout auto-quits the interactive window (for CI); 0 runs until
	// the user closes the window. Overridable with --timeout.
	Timeout time.Duration
	// OnShouldClose (optional) vetoes window close: return false to
	// keep the window open (e.g. an unsaved-changes prompt). Called
	// on the main thread.
	OnShouldClose func() bool
	// CustomDecorations asks the platform for client-side window
	// decorations: the app draws its own title bar (close/minimize/
	// maximize, draggable top strip). Platforms without support keep
	// the OS frame (the flag reports nothing; the app checks the
	// window's SupportsCustomDecorations).
	CustomDecorations bool
}

// App wires one example window on the selected platform.
type App[T any] struct {
	// Platform is the selected backend (native or headless).
	Platform platform.Platform
	// App is the gpui application.
	App *gpui.Application
	// Handle is the opened window's root-view handle.
	Handle gpui.WindowHandle[T]
	// Headless is non-nil when running with --headless.
	Headless *headless.Platform
	// OutDir receives the headless PNGs.
	OutDir string

	cfg       Config
	scenePath string
}

// Start parses the example flags, selects the platform and opens the
// window. Call SetRoot next; then either run the example's headless
// verification (when a.Headless != nil) or Loop.
func Start[T any](cfg Config, buildView func(w *gpui.Window, cx *app.Context[T]) T) *App[T] {
	headlessMode := flag.Bool("headless", false, "render frames to PNGs and exit instead of opening a window")
	outDir := flag.String("out", ".", "output directory for --headless PNGs")
	timeout := flag.Duration("timeout", cfg.Timeout, "quit the interactive window after this duration (0 = until closed)")
	sceneDump := flag.String("scene", "", "append a per-frame scene/hitbox dump to this file (debug)")
	debugOverlay := flag.Bool("debug", os.Getenv("GPUI_DEBUG") != "", "paint the debug overlay (FPS / frame stats)")
	flag.Parse()
	cfg.Timeout = *timeout

	a := &App[T]{OutDir: *outDir, cfg: cfg, scenePath: *sceneDump}
	if err := os.MkdirAll(a.OutDir, 0o755); err != nil {
		Fatal(cfg.Name, err)
	}

	var plat platform.Platform
	if *headlessMode {
		hp := headless.New(cfg.Size)
		a.Headless = hp
		plat = hp
	} else {
		p, err := auto.New()
		if err != nil {
			Fatal(cfg.Name, err)
		}
		plat = p
	}
	a.Platform = plat
	a.App = gpui.NewApplication(plat)
	// Background loads (img assets) publish their results on the main
	// thread through the platform dispatcher.
	app.SetMainThreadDispatcher(plat.DispatchOnMainThread)

	bounds := platform.WindowedBounds(geometry.BoundsPixels(0, 0, cfg.Size.Width.AsF32(), cfg.Size.Height.AsF32()))
	winOpts := platform.WindowOptions{
		Title:        cfg.Title,
		WindowBounds: &bounds,
	}
	if cfg.CustomDecorations {
		client := platform.DecorationsClient
		winOpts.WindowDecorations = &client
	}
	handle, err := gpui.OpenWindowWith[T](a.App, winOpts, buildView, func(callbacks *platform.WindowCallbacks) {
		if cfg.OnShouldClose != nil {
			callbacks.OnShouldClose = cfg.OnShouldClose
		}
	})
	if err != nil {
		Fatal(cfg.Name, err)
	}
	a.Handle = handle
	// Hover cursor plumbing: resolved hover cursor requests go to the
	// platform's system cursor (upstream Window::reset_cursor_style →
	// platform.set_cursor_style).
	handle.Window.SetCursorApplier(plat.SetCursor)
	if *debugOverlay {
		handle.Window.SetDebugOverlay(true)
	}
	if a.scenePath != "" {
		frame := 0
		handle.Window.SetAfterDraw(func(scene *render.Scene) {
			frame++
			dumpScene(a.scenePath, cfg.Name, frame, scene, handle.Window)
		})
	}
	return a
}

// dumpScene appends one frame's primitives and hitboxes to path (debug tool;
// coordinates are scaled/device pixels).
func dumpScene(path, name string, frame int, scene *render.Scene, w *gpui.Window) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "=== %s frame %d ===\n", name, frame)
	for i := range scene.Quads {
		q := &scene.Quads[i]
		fmt.Fprintf(f, "quad order=%d bounds=[%.1f %.1f %.1f %.1f] bg=%v border=%v bw=[%v %v %v %v] radii=[%v %v %v %v]\n",
			q.Order, q.Bounds.Origin.X.AsF32(), q.Bounds.Origin.Y.AsF32(),
			q.Bounds.Size.Width.AsF32(), q.Bounds.Size.Height.AsF32(),
			q.Background, q.BorderColor,
			q.BorderWidths.Top.AsF32(), q.BorderWidths.Right.AsF32(), q.BorderWidths.Bottom.AsF32(), q.BorderWidths.Left.AsF32(),
			q.CornerRadii.TopLeft.AsF32(), q.CornerRadii.TopRight.AsF32(), q.CornerRadii.BottomRight.AsF32(), q.CornerRadii.BottomLeft.AsF32())
	}
	for i := range scene.MonoSprites {
		s := &scene.MonoSprites[i]
		fmt.Fprintf(f, "mono order=%d bounds=[%.1f %.1f %.1f %.1f] color=%v tile=[%d %d %d %d]\n",
			s.Order, s.Bounds.Origin.X.AsF32(), s.Bounds.Origin.Y.AsF32(),
			s.Bounds.Size.Width.AsF32(), s.Bounds.Size.Height.AsF32(), s.Color,
			s.Tile.Bounds.Origin.X.AsI32(), s.Tile.Bounds.Origin.Y.AsI32(),
			s.Tile.Bounds.Size.Width.AsI32(), s.Tile.Bounds.Size.Height.AsI32())
	}
	for _, hb := range w.RenderedHitboxes() {
		fmt.Fprintf(f, "hitbox id=%d bounds=[%.1f %.1f %.1f %.1f] behavior=%d\n",
			hb.ID, hb.Bounds.Origin.X.AsF32(), hb.Bounds.Origin.Y.AsF32(),
			hb.Bounds.Size.Width.AsF32(), hb.Bounds.Size.Height.AsF32(), hb.Behavior)
	}
}

// SetRoot installs the root render callback and draws the first frame.
func (a *App[T]) SetRoot(render func(w *gpui.Window, cx *app.App) gpui.Element) {
	a.Handle.Window.SetRootView(a.Handle.View.ID(), render)
	a.Handle.Window.Draw()
}

// Loop runs the interactive event loop until the window is closed (or the
// timeout fires). It returns immediately in headless mode; run the
// example's verification code instead.
func (a *App[T]) Loop() {
	if a.Headless != nil {
		return
	}
	if a.cfg.Timeout > 0 {
		go func() {
			time.Sleep(a.cfg.Timeout)
			a.Platform.Quit()
		}()
	}
	a.Platform.Run(func() {})
}

// Fatal prints a name-prefixed error and exits with status 1.
func Fatal(name string, err error) {
	fmt.Fprintln(os.Stderr, name+":", err)
	os.Exit(1)
}

// Printf prints a name-prefixed informational line.
func Printf(name, format string, args ...any) {
	fmt.Println(name+":", fmt.Sprintf(format, args...))
}
