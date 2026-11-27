// Command customframe demonstrates a frameless window with client-side
// decorations: the app draws its own title bar (drag, double-click
// maximize toggle, minimize/maximize-restore/close caption buttons) and
// its own border decoration around the whole client area. It asks the
// platform for DecorationsClient; on Windows the win32 backend removes
// the native frame (the WM_NCCALCSIZE frameless client-rect computation)
// while keeping the DWM shadow, snap and the resize borders re-exposed
// through WM_NCHITTEST, so the drawn border always sits flush with the
// window edge — including right after dragging a maximized window, where
// the drag-restore frame resync keeps the client glued to the window rect.
//
//      go run ./examples/customframe
//      go run ./examples/customframe --headless --out /tmp/frames
//
// The headless mode renders frames to PNGs and runs the visual checks of
// verifyHeadless: the border decoration ring, the title bar band, the
// caption-button layout, the drag/caption interactions and the border's
// state feedback (the border repaints in the accent color while the
// maximized visual is active). The checks also pin the title bar to its
// designed height in a much taller window (the bar is a fixed band and
// must never grow with the window) and verify the layout again after
// resizing back.
package main

import (
        "bytes"
        "fmt"
        "image"
        "image/png"
        "os"
        "path/filepath"
        "strings"

        "github.com/FlameInTheDark/go-gpui/app"
        gc "github.com/FlameInTheDark/go-gpui/color"
        "github.com/FlameInTheDark/go-gpui/elements"
        "github.com/FlameInTheDark/go-gpui/examples/internal/demo"
        "github.com/FlameInTheDark/go-gpui/geometry"
        "github.com/FlameInTheDark/go-gpui/gpui"
        "github.com/FlameInTheDark/go-gpui/platform"
        "github.com/FlameInTheDark/go-gpui/platform/headless"
        "github.com/FlameInTheDark/go-gpui/style"
)

// Catppuccin-ish palette; accent is the custom border decoration color.
var (
        pageBg   = gc.RGB(0x1e1e2e).Hsla()
        surface  = gc.RGB(0x313244).Hsla()
        overlay  = gc.RGB(0x45475a).Hsla()
        accent   = gc.RGB(0xfab387).Hsla() // border decoration, restored state
        accentOn = gc.RGB(0xa6e3a1).Hsla() // border decoration, maximized visual
        textC    = gc.RGB(0xcdd6f4).Hsla()
        dim      = gc.RGB(0x6c7086).Hsla()
        red      = gc.RGB(0xf38ba8).Hsla()
        redHover = gc.RGB(0xba6aad).Hsla()
)

// Title-bar metrics (logical px). The visual checks sample the frame at
// coordinates derived from these.
const (
        borderW   = 2  // Border_2: the custom border decoration thickness
        titleBarH = 40 // the title bar band inside the border
        viewportW = 640
        viewportH = 420
        tallH     = 900 // the tall-window regression size (see verifyHeadless)
)

// frameState is the example's view state.
type frameState struct {
        // maximized mirrors the visual maximize toggle (the headless platform
        // cannot really zoom a window; the native ToggleMaximize result shows
        // up through IsMaximized on the next real frame).
        maximized bool
        // events records caption interactions for the headless verification.
        events []string
}

func (s *frameState) note(ev string) { s.events = append(s.events, ev) }

// captionGlyph draws simple geometric glyphs from divs so the example
// needs no icon font: a bar (minimize) or a square (maximize/restore).
func captionGlyph(kind string, filled bool, c gc.Hsla) gpui.Element {
        px := func(v float32) geometry.Length { return geometry.LengthFromPixels(geometry.Px(v)) }
        switch kind {
        case "min":
                return elements.NewDiv().W(px(10)).H(px(2)).BgColor(c)
        case "max":
                if filled {
                        return elements.NewDiv().W(px(10)).H(px(10)).BgColor(c)
                }
                return elements.NewDiv().W(px(10)).H(px(10)).Border_1().BorderColor(c)
        case "close":
                return elements.NewText("\u00d7").WithTextColor(c)
        default:
                return elements.NewDiv().W(px(10)).H(px(10))
        }
}

// captionButton is one title-bar button: a fixed target with hover
// feedback that stops propagation (a press on a button must not start a
// window drag from the bar underneath). record observes the activation
// (the headless verification asserts on it).
func captionButton(name string, hoverBg gc.Hsla, glyph string, filled bool, note string,
        action func(w *gpui.Window), record func()) *elements.Div {
        return elements.NewDiv().
                ID(elements.ButtonID(name)).
                A11y(gpui.AccessibilityButton, note).
                W(geometry.LengthFromPixels(geometry.Px(44))).
                H(geometry.LengthFromPixels(geometry.Px(30))).
                Rounded_Md().
                Flex().ItemsCenter().JustifyCenter().
                BgColor(surface).
                Hover(func(s *style.StyleRefinement) { s.BgColor(hoverBg) }).
                Child(captionGlyph(glyph, filled, textC)).
                OnMouseDown(func(ev platform.MouseDownEvent, w *gpui.Window) {
                        w.StopPropagation() // keep the title bar's drag handler out of it
                }).
                OnClick(func(ev gpui.ClickEvent, w *gpui.Window) {
                        w.StopPropagation()
                        if record != nil {
                                record()
                        }
                        action(w)
                        w.Draw()
                })
}

// titleBar builds the custom title bar: a drag surface (press anywhere and
// move = native window drag via StartWindowDrag), double-click toggles the
// maximized state, and the caption buttons live at its right end.
func titleBar(handle *gpui.WindowHandle[frameState], cx *app.App, state *frameState) *elements.Div {
        px := func(v float32) geometry.Length { return geometry.LengthFromPixels(geometry.Px(v)) }

        // Fixed height in every window size: the bar is the drag surface of
        // exactly titleBarH logical pixels. It must not flex-grow inside the
        // window column — a growing bar shares the free vertical space of a
        // tall window and balloons to hundreds of pixels, pushing the title
        // row into the middle of the screen while the content below keeps
        // its own size (exactly what the tall-window check below pins).
        bar := elements.NewDiv().
                ID(elements.ButtonID("cf-titlebar")).
                Flex().ItemsCenter().
                H(px(titleBarH)).
                Px_3().
                Gap_2().
                BgColor(surface).
                OnMouseDown(func(ev platform.MouseDownEvent, w *gpui.Window) {
                        if ev.Button != platform.ButtonLeft {
                                return
                        }
                        handle.View.Update(cx, func(s *frameState, c *app.Context[frameState]) any {
                                s.note("drag")
                                return nil
                        })
                        // Native: runs the modal SC_MOVE drag loop (restores a
                        // maximized window and drags it under the cursor); the
                        // library's drag-restore frame resync keeps the drawn border
                        // flush with the window edges afterwards. Headless: a no-op.
                        w.PlatformWindow().StartWindowDrag()
                        w.Draw()
                }).
                OnClick(func(ev gpui.ClickEvent, w *gpui.Window) {
                        if ev.Up.ClickCount < 2 {
                                return // single click: the drag surface does nothing
                        }
                        handle.View.Update(cx, func(s *frameState, c *app.Context[frameState]) any {
                                s.note("double-click")
                                s.maximized = !s.maximized
                                return nil
                        })
                        w.PlatformWindow().ToggleMaximize()
                        w.Draw()
                })

        record := func(note string) func() {
                return func() {
                        handle.View.Update(cx, func(s *frameState, c *app.Context[frameState]) any {
                                s.note(note)
                                return nil
                        })
                }
        }
        buttons := elements.NewDiv().Flex().Gap_1().ItemsCenter().
                Child(captionButton("cf-min", overlay, "min", false, "minimize",
                        func(w *gpui.Window) { w.PlatformWindow().Minimize() }, record("minimize"))).
                Child(captionButton("cf-max", overlay, "max", state.maximized, "maximize",
                        func(w *gpui.Window) {
                                handle.View.Update(cx, func(s *frameState, c *app.Context[frameState]) any {
                                        s.maximized = !s.maximized
                                        return nil
                                })
                                w.PlatformWindow().ToggleMaximize()
                        }, record("maximize"))).
                Child(captionButton("cf-close", redHover, "close", false, "close",
                        func(w *gpui.Window) { w.PlatformWindow().Close() }, record("close")))

        dot := elements.NewDiv().W(px(8)).H(px(8)).Rounded_Full().BgColor(accentOn)
        label := elements.NewText("go-gpui - custom frame").WithTextColor(textC)
        spacer := elements.NewDiv().FlexGrow(1)
        return bar.Children(dot, label, spacer, buttons)
}

// borderBox is the custom border decoration: an accent-colored ring drawn
// by the app (not the OS), wrapped around everything. While the maximized
// visual is on it repaints in accentOn so the state feedback is visible —
// on Windows this stays flush with the window edge in every placement
// because the frameless client rect follows the window rect exactly.
func borderBox(handle *gpui.WindowHandle[frameState], cx *app.App, state *frameState, root gpui.Element) *elements.Div {
        ring := accent
        if state.maximized {
                ring = accentOn
        }
        return elements.NewDiv().
                Flex().FlexCol().FlexGrow(1).
                Border_2().BorderColor(ring).
                Rounded_Lg().OverflowHidden().
                BgColor(pageBg).
                Children(titleBar(handle, cx, state), root)
}

// content builds the body: the state card and usage hints.
func content(handle *gpui.WindowHandle[frameState], state *frameState) gpui.Element {
        px := func(v float32) geometry.Length { return geometry.LengthFromPixels(geometry.Px(v)) }

        deco := "platform default"
        if dw, ok := handle.Window.PlatformWindow().(interface{ SupportsCustomDecorations() bool }); ok && dw.SupportsCustomDecorations() {
                deco = "client (custom frame)"
        }
        maxLabel := "no"
        if state.maximized {
                maxLabel = "yes"
        }

        card := elements.NewDiv().
                Flex().FlexCol().Gap_2().
                W(px(250)).P_3().Rounded_Md().BgColor(surface).
                Child(elements.NewText("window state").WithTextColor(dim)).
                Child(elements.NewText("decorations: " + deco).WithTextColor(textC)).
                Child(elements.NewText("maximized: " + maxLabel).WithTextColor(textC)).
                Child(elements.NewText(fmt.Sprintf("scale: %.2f", handle.Window.ScaleFactor())).WithTextColor(textC))

        hints := elements.NewDiv().
                Flex().FlexCol().Gap_1().P_3().
                Child(elements.NewText("- drag the title bar to move the window").WithTextColor(dim)).
                Child(elements.NewText("- double-click the title bar to maximize").WithTextColor(dim)).
                Child(elements.NewText("- resize from any window edge").WithTextColor(dim)).
                Child(elements.NewText("- maximize, then drag: the border stays flush").WithTextColor(dim))

        return elements.NewDiv().
                Flex().FlexCol().Gap_3().FlexGrow(1).
                P_4().BgColor(pageBg).
                Children(card, hints)
}

func main() {
        a := demo.Start[frameState](demo.Config{
                Name:              "customframe",
                Title:             "custom frame",
                Size:              geometry.SizePixels(viewportW, viewportH),
                CustomDecorations: true,
        }, func(w *gpui.Window, cx *app.Context[frameState]) frameState {
                return frameState{}
        })
        cx := a.App.App

        a.SetRoot(func(w *gpui.Window, c *app.App) gpui.Element {
                state := a.Handle.View.Read(cx)
                return borderBox(&a.Handle, cx, state, content(&a.Handle, state))
        })

        if a.Headless == nil {
                a.Loop() // blocks until the window is closed
                return
        }
        verifyHeadless(a, cx)
}

// verifyHeadless renders frames and runs the visual checks: pixel samples
// of the border decoration ring and the title bar band, the caption-button
// layout, the drag/caption interactions and the border's state feedback.
func verifyHeadless(a *demo.App[frameState], cx *app.App) {
        hw := a.Headless.LastWindow()
        w := a.Handle.Window

        checkFrame := func(name string, vw, vh int) {
                img, err := png.Decode(bytes.NewReader(mustPNG(hw)))
                if err != nil {
                        demo.Fatal("customframe", fmt.Errorf("%s: decode png: %w", name, err))
                }
                b := img.Bounds()
                if b.Dx() != vw || b.Dy() != vh {
                        demo.Fatal("customframe", fmt.Errorf("%s: frame %dx%d, want %dx%d", name, b.Dx(), b.Dy(), vw, vh))
                }
                // The border decoration ring: two samples per edge (the ring is
                // Border_2 thick; corners are rounded away and never sampled).
                wantRing := rgbaOf(accent)
                if a.Handle.View.Read(cx).maximized {
                        wantRing = rgbaOf(accentOn)
                }
                for _, p := range [][2]int{
                        {vw / 2, 0}, {vw / 2, 1}, // top
                        {vw / 2, vh - 1}, {vw / 2, vh - 2}, // bottom
                        {0, vh / 2}, {1, vh / 2}, // left
                        {vw - 1, vh / 2}, {vw - 2, vh / 2}, // right
                } {
                        if got := rgbaAt(img, p[0], p[1]); !near(got, wantRing, 2) {
                                demo.Fatal("customframe", fmt.Errorf("%s: border ring at (%d,%d) = %v, want %v (the custom border decoration must sit flush with the window edge)", name, p[0], p[1], got, wantRing))
                        }
                }
                // The title bar band under the top border (left padding area,
                // clear of the status dot and the caption buttons).
                wantBar := rgbaOf(surface)
                if got := rgbaAt(img, 8, borderW+titleBarH/2); !near(got, wantBar, 2) {
                        demo.Fatal("customframe", fmt.Errorf("%s: title bar at (%d,%d) = %v, want %v", name, 8, borderW+titleBarH/2, got, wantBar))
                }
                // The band keeps exactly its designed height in every window
                // size: bar surface right above its bottom edge, content
                // background right below it. A title bar that grows with the
                // window paints bar surface at the below sample and fails.
                if got := rgbaAt(img, 8, borderW+titleBarH-3); !near(got, wantBar, 2) {
                        demo.Fatal("customframe", fmt.Errorf("%s: title bar bottom at (%d,%d) = %v, want %v", name, 8, borderW+titleBarH-3, got, wantBar))
                }
                wantBg := rgbaOf(pageBg)
                below := borderW + titleBarH + 3
                if got := rgbaAt(img, 8, below); !near(got, wantBg, 2) {
                        demo.Fatal("customframe", fmt.Errorf("%s: content below the title bar at (%d,%d) = %v, want %v (the title bar must keep its fixed %dpx height in every window size)", name, 8, below, got, wantBg, titleBarH))
                }
                // The content background (right of the state card).
                if got := rgbaAt(img, vw-24, vh/2+40); !near(got, wantBg, 2) {
                        demo.Fatal("customframe", fmt.Errorf("%s: content at (%d,%d) = %v, want %v", name, vw-24, vh/2+40, got, wantBg))
                }
        }

        // Caption-button layout: three buttons inside the title bar band,
        // ordered min | max | close left to right — re-checked per window
        // size, the band never moves or grows with the window.
        captionButtons := func(where string) map[string]geometry.Bounds[geometry.Pixels] {
                buttons := map[string]geometry.Bounds[geometry.Pixels]{}
                for _, hb := range w.RenderedHitboxes() {
                        name, ok := strings.CutPrefix(hb.Key, "n:btn-cf-")
                        if ok && name != "titlebar" {
                                buttons[name] = hb.Bounds
                        }
                }
                if len(buttons) != 3 {
                        demo.Fatal("customframe", fmt.Errorf("%s: caption buttons hit = %d (%v), want 3", where, len(buttons), keys(buttons)))
                }
                minB, maxB, closeB := buttons["min"], buttons["max"], buttons["close"]
                for id, b := range map[string]geometry.Bounds[geometry.Pixels]{"min": minB, "max": maxB, "close": closeB} {
                        cy := b.Origin.Y + b.Size.Height/2
                        if cy < borderW || cy >= borderW+titleBarH {
                                demo.Fatal("customframe", fmt.Errorf("%s: %s center y = %.0f outside the title bar band [%d,%d)", where, id, cy, borderW, borderW+titleBarH))
                        }
                        if b.Size.Width < 20 || b.Size.Height < 20 {
                                demo.Fatal("customframe", fmt.Errorf("%s: %s is %.0fx%.0f, want a caption-sized target", where, id, b.Size.Width, b.Size.Height))
                        }
                }
                if !(minB.Origin.X < maxB.Origin.X && maxB.Origin.X < closeB.Origin.X) {
                        demo.Fatal("customframe", fmt.Errorf("%s: caption order wrong: min %.0f, max %.0f, close %.0f", where, minB.Origin.X, maxB.Origin.X, closeB.Origin.X))
                }
                return buttons
        }

        checkFrame("initial", viewportW, viewportH)
        if err := savePNG(hw, filepath.Join(a.OutDir, "customframe_normal.png")); err != nil {
                demo.Fatal("customframe", err)
        }
        captionButtons("initial size")

        // Tall-window regression: the content area grows with the window,
        // the title bar band does not. (Without the fixed band height the
        // bar shares the free vertical space of the window column and
        // balloons to hundreds of pixels on a tall or maximized window,
        // dragging the title row away from the top edge.)
        hw.Resize(geometry.SizePixels(viewportW, tallH))
        w.Draw()
        checkFrame("tall window", viewportW, tallH)
        captionButtons("tall window")
        if err := savePNG(hw, filepath.Join(a.OutDir, "customframe_tall.png")); err != nil {
                demo.Fatal("customframe", err)
        }
        hw.Resize(geometry.SizePixels(viewportW, viewportH))
        w.Draw()
        checkFrame("resized back", viewportW, viewportH)

        buttons := captionButtons("after resize back")
        minB, maxB := buttons["min"], buttons["max"]

        click := func(b geometry.Bounds[geometry.Pixels]) {
                pos := geometry.PointPixels(b.Origin.X.AsF32()+b.Size.Width.AsF32()/2, b.Origin.Y.AsF32()+b.Size.Height.AsF32()/2)
                hw.DispatchInput(platform.MouseDownEvent{Button: platform.ButtonLeft, Position: pos})
                hw.DispatchInput(platform.MouseUpEvent{Button: platform.ButtonLeft, Position: pos, ClickCount: 1})
        }

        // Maximize button: toggles the visual state; the border decoration
        // repaints in the maximized accent.
        click(maxB)
        if state := a.Handle.View.Read(cx); !state.maximized {
                demo.Fatal("customframe", fmt.Errorf("maximize click did not toggle the state: events %v", state.events))
        }
        w.Draw()
        checkFrame("maximized", viewportW, viewportH)
        if err := savePNG(hw, filepath.Join(a.OutDir, "customframe_maximized.png")); err != nil {
                demo.Fatal("customframe", err)
        }

        // Title bar drag surface: a press on the bar records the drag start
        // (StartWindowDrag; a no-op under the headless platform).
        pos := geometry.PointPixels(200, borderW+titleBarH/2)
        hw.DispatchInput(platform.MouseDownEvent{Button: platform.ButtonLeft, Position: pos})
        hw.DispatchInput(platform.MouseUpEvent{Button: platform.ButtonLeft, Position: pos, ClickCount: 1})
        if state := a.Handle.View.Read(cx); !contains(state.events, "drag") {
                demo.Fatal("customframe", fmt.Errorf("title bar press did not start a drag: %v", state.events))
        }

        // Double-click the title bar: toggles the maximized visual back off
        // (the border ring returns to the restored accent).
        hw.DispatchInput(platform.MouseDownEvent{Button: platform.ButtonLeft, Position: pos, ClickCount: 1})
        hw.DispatchInput(platform.MouseUpEvent{Button: platform.ButtonLeft, Position: pos, ClickCount: 1})
        hw.DispatchInput(platform.MouseDownEvent{Button: platform.ButtonLeft, Position: pos, ClickCount: 2})
        hw.DispatchInput(platform.MouseUpEvent{Button: platform.ButtonLeft, Position: pos, ClickCount: 2})
        if state := a.Handle.View.Read(cx); state.maximized {
                demo.Fatal("customframe", fmt.Errorf("double-click did not restore the maximized visual"))
        }
        w.Draw()
        checkFrame("restored", viewportW, viewportH)

        // Minimize button (headless: a no-op, but the handler must run).
        click(minB)
        if state := a.Handle.View.Read(cx); !contains(state.events, "minimize") {
                demo.Fatal("customframe", fmt.Errorf("minimize click was not delivered: %v", state.events))
        }

        demo.Printf("customframe", "visual checks pass (frames: %s, %s)",
                filepath.Join(a.OutDir, "customframe_normal.png"),
                filepath.Join(a.OutDir, "customframe_maximized.png"))
}

// --- small helpers for the verification -------------------------------------

func mustPNG(hw *headless.Window) []byte {
        b, err := hw.PNG()
        if err != nil {
                demo.Fatal("customframe", err)
        }
        return b
}

func savePNG(hw *headless.Window, path string) error {
        return os.WriteFile(path, mustPNG(hw), 0o644)
}

func keys(m map[string]geometry.Bounds[geometry.Pixels]) []string {
        out := make([]string, 0, len(m))
        for k := range m {
                out = append(out, k)
        }
        return out
}

func contains(list []string, v string) bool {
        for _, s := range list {
                if s == v {
                        return true
                }
        }
        return false
}

// rgbaOf converts a palette color into the RGBA the renderer writes for an
// opaque fill.
func rgbaOf(c gc.Hsla) [4]uint8 {
        rgb := c.ToRgb()
        return [4]uint8{
                uint8(rgb.R*255 + 0.5),
                uint8(rgb.G*255 + 0.5),
                uint8(rgb.B*255 + 0.5),
                uint8(rgb.A*255 + 0.5),
        }
}

// rgbaAt samples one pixel.
func rgbaAt(img image.Image, x, y int) [4]uint8 {
        r, g, b, a := img.At(x, y).RGBA()
        return [4]uint8{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}

// near compares two RGBA values with a per-channel tolerance (the renderer
// blends in sRGB space; exact equality is expected but rounding-safe).
func near(a, b [4]uint8, tol int) bool {
        for i := range a {
                d := int(a[i]) - int(b[i])
                if d < 0 {
                        d = -d
                }
                if d > tol {
                        return false
                }
        }
        return true
}
