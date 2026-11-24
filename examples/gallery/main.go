// Command gallery is a showcase of go-gpui's rendering primitives and
// interactive features: backgrounds, gradients, borders, rounded
// corners, shadows, flexbox, typography, SVG paths and gradients — plus
// the platform integration set: the app menu bar, modal prompts, window
// zoom, BiDi text, scroll containers with ScrollHandle autoscroll, file
// drag-and-drop, system notifications and the debug overlay.
//
// By default it opens an interactive native window; with --headless it
// renders one PNG and verifies the frame content:
//
//	go run ./examples/gallery
//	go run ./examples/gallery --headless --out /tmp/frames
//	go run ./examples/gallery --debug   # FPS / frame-stat overlay
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/FlameInTheDark/go-gpui/action"
	"github.com/FlameInTheDark/go-gpui/app"
	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/elements"
	"github.com/FlameInTheDark/go-gpui/examples/internal/demo"
	"github.com/FlameInTheDark/go-gpui/font"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/keymap"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/render"
	"github.com/FlameInTheDark/go-gpui/style"
)

// galleryState is the interactive state behind the view.
type galleryState struct {
	scroll *elements.ScrollHandle
	log    []string
}

func (g *galleryState) addLog(line string) {
	g.log = append(g.log, line)
	if len(g.log) > 40 {
		g.log = g.log[len(g.log)-40:]
	}
	// Tail-follow: the log autoscrolls while the user stays at the bottom.
	g.scroll.Autoscroll()
}

// zoomMenuItems returns the View menu's zoom entries.
func zoomMenuItems() []gpui.MenuItem {
	zoomIn := gpui.MenuItem{Label: "Zoom In", Handler: func(w *gpui.Window) { w.ZoomIn() }}
	zoomIn = zoomIn.MenuKeystroke("ctrl+=")
	zoomOut := gpui.MenuItem{Label: "Zoom Out", Handler: func(w *gpui.Window) { w.ZoomOut() }}
	zoomOut = zoomOut.MenuKeystroke("ctrl+-")
	zoomReset := gpui.MenuItem{Label: "Reset Zoom", Handler: func(w *gpui.Window) { w.ZoomReset() }}
	zoomReset = zoomReset.MenuKeystroke("ctrl+0")
	return []gpui.MenuItem{zoomIn, zoomOut, zoomReset}
}

// buildGalleryMenu installs the app menu (upstream SetAppMenu; the
// in-app MenuBar renders it on Windows/Linux).
func (g *galleryState) buildGalleryMenu(w *gpui.Window) {
	aboutItem := gpui.MenuItem{Label: "Prompt: About", Handler: func(w *gpui.Window) {
		w.Prompt(gpui.PromptLevelInfo,
			"About go-gpui",
			"A pure-Go port of the upstream editor's gpui GPU UI framework.",
			"Menus, prompts, zoom, BiDi, scroll handles and notifications are live in this window.",
			[]gpui.PromptAction{{Label: "OK"}})
	}}
	aboutItem = aboutItem.MenuKeystroke("ctrl+i")

	inputItem := gpui.MenuItem{Label: "Prompt: Input", Handler: func(w *gpui.Window) {
		w.PromptForInput(gpui.PromptLevelInfo, "New project",
			"Enter the project name", "", "my-project", "")
	}}
	inputItem = inputItem.MenuKeystroke("ctrl+n")

	notifyItem := gpui.MenuItem{Label: "Send Notification", Handler: func(w *gpui.Window) {
		if err := sendNotification(platform.Notification{
			Title: "go-gpui gallery",
			Body:  "A system notification from a pure-Go UI framework.",
		}); err != nil {
			g.addLog("notification unavailable: " + err.Error())
		} else {
			g.addLog("notification sent")
		}
	}}

	quitItem := gpui.MenuItem{Label: "Quit", Handler: func(w *gpui.Window) {
		activePlatform.Quit()
	}}
	quitItem = quitItem.MenuKeystroke("ctrl+q")

	fullscreenItem := gpui.MenuItem{Label: "Toggle Fullscreen", Handler: func(w *gpui.Window) {
		w.PlatformWindow().ToggleFullscreen()
	}}
	fullscreenItem = fullscreenItem.MenuKeystroke("f11")

	debugItem := gpui.MenuItem{Label: "Debug Overlay", Handler: func(w *gpui.Window) {
		w.SetDebugOverlay(!w.DebugOverlayEnabled())
	}}
	debugItem = debugItem.MenuKeystroke("ctrl+d")

	w.SetAppMenu([]gpui.Menu{
		gpui.NewMenu("File",
			aboutItem,
			inputItem,
			gpui.MenuSeparator(),
			notifyItem,
			gpui.MenuSeparator(),
			quitItem,
		),
		gpui.NewMenu("View", append(zoomMenuItems(),
			gpui.MenuSeparator(),
			fullscreenItem,
			debugItem,
		)...),
		gpui.NewMenu("Help",
			gpui.MenuItem{Label: "About", Handler: func(w *gpui.Window) {
				w.Prompt(gpui.PromptLevelInfo, "About", "go-gpui gallery", "",
					[]gpui.PromptAction{{Label: "OK"}})
			}},
		),
	})
}

func (g *galleryState) render(w *gpui.Window, cx any) gpui.Element {
	if w.AppMenu() == nil {
		g.buildGalleryMenu(w)
	}
	// Action listeners must register during paint: the dispatch pipeline
	// only consults the rendered frame's listeners, which reset every
	// draw. Registering once at startup (outside paint) would be wiped
	// before the first keystroke could match a binding.
	installKeyBindings(w)
	px := func(v float32) geometry.Length { return geometry.LengthFromPixels(geometry.Px(v)) }

	content := elements.NewDiv().Flex().FlexCol().FlexGrow(1).P_6().Gap_4().BgColor(bg0).
		Child(
			elements.NewDiv().TextSize(geometry.PixelsLength(geometry.Px(24))).FontWeight(font.WeightBold).
				Child(elements.NewText("go-gpui gallery").WithTextColor(fg)),
		).
		Child(
			// Backgrounds, gradients, borders and radii.
			elements.NewDiv().Flex().Gap_4().
				Child(swatch(blue, "solid")).
				Child(swatch(peach, "round").Rounded_Xl()).
				Child(elements.NewDiv().
					P_4().W(px(120)).H(px(90)).
					Flex().JustifyCenter().ItemsCenter().
					Bg(gc.LinearGradient(90,
						gc.NewLinearColorStop(blue, 0),
						gc.NewLinearColorStop(green, 1))).
					Child(elements.NewText("gradient").WithTextColor(fg))).
				Child(elements.NewDiv().
					P_4().W(px(120)).H(px(90)).
					Flex().JustifyCenter().ItemsCenter().
					Border_1().BorderColor(fg).Rounded_Md().
					Child(elements.NewText("border").WithTextColor(fg))),
		).
		Child(
			// Drop shadow card.
			elements.NewDiv().
				P_4().Rounded_Lg().BgColor(surface).
				Shadow([]style.BoxShadow{{
					Color:        gc.Black().Opacity(0.5),
					OffsetX:      6,
					OffsetY:      8,
					BlurRadius:   12,
					SpreadRadius: -4,
				}}).
				Child(elements.NewText("Drop shadow + surface").WithTextColor(fg)),
		).
		Child(
			// BiDi: mixed-direction text renders with correct visual
			// order (Hebrew and Arabic segments reversed) and a working
			// caret map.
			elements.NewDiv().Flex().FlexCol().P_4().Gap_2().Rounded_Md().BgColor(surface).
				Child(elements.NewText("BiDi: LTR \u05e2\u05d1\u05e8\u05d9\u05ea mixed \u0645\u0631\u062d\u0628\u0627 text").WithTextColor(fg)).
				Child(elements.NewText("\u05d4\u05e7\u05d3\u05dd \u05d1\u05e1\u05d5\u05e4\u05d4: RTL base line").WithTextColor(fg)),
		).
		Child(
			// SVG: linear gradient + radial gradient + dashes.
			elements.NewDiv().Flex().ItemsCenter().Gap_4().
				Child(elements.NewSvgFromData([]byte(
					`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><path d="M2 8 L6 12 L14 3" fill="none" stroke="#89b4fa" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>`)).
					W(px(32)).H(px(32))).
				Child(elements.NewSvgFromData([]byte(
					`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32">
  <defs>
    <radialGradient id="g" cx="0.5" cy="0.5" r="0.5">
      <stop offset="0" stop-color="#a6e3a1"/>
      <stop offset="1" stop-color="#1e1e2e"/>
    </radialGradient>
  </defs>
  <circle cx="16" cy="16" r="14" fill="url(#g)"/>
</svg>`)).
					W(px(32)).H(px(32))).
				Child(elements.NewSvgFromData([]byte(
					`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 8"><path d="M2 4 L30 4" fill="none" stroke="#f38ba8" stroke-width="2" stroke-dasharray="4 2"/></svg>`)).
					W(px(96)).H(px(24))).
				Child(elements.NewText("SVG: path, radial gradient, dashes").WithTextColor(fg)),
		).
		Child(
			// Interaction demos: prompts, zoom, notification.
			elements.NewDiv().Flex().FlexCol().P_4().Gap_2().Rounded_Md().BgColor(surface).
				Child(elements.NewText("Interactive features").WithTextColor(fg)).
				Child(g.renderButtons(w)),
		).
		Child(
			// The event log: file-drop target + autoscrolling tail.
			elements.NewDiv().Flex().FlexCol().Rounded_Md().BgColor(surface).
				Child(elements.NewText("  Drop files here / event log (autoscrolls)").WithTextColor(fg)).
				Child(g.renderLog(w)),
		)

	// The window root: the menu bar above the content.
	return elements.NewDiv().Flex().FlexCol().
		Child(elements.NewMenuBar()).
		Child(content)
}

// renderButtons builds the interactive demo row.
func (g *galleryState) renderButtons(w *gpui.Window) gpui.Element {
	button := func(label string, onClick func(w *gpui.Window)) *elements.Div {
		return elements.NewDiv().
			ID(gpui.NameElementId("btn-"+strings.ToLower(strings.ReplaceAll(label, " ", "-")))).
			A11y(gpui.AccessibilityButton, label).
			OnClick(func(ev gpui.ClickEvent, w *gpui.Window) { onClick(w) }).
			P_2().Rounded_Md().BgColor(blue).
			Child(elements.NewText(label).WithTextColor(bg0))
	}
	return elements.NewDiv().Flex().Gap_2().FlexWrap().
		Child(button("Prompt", func(w *gpui.Window) {
			w.Prompt(gpui.PromptLevelWarning, "Reset layout?",
				"This resets every gallery panel.", "",
				[]gpui.PromptAction{{Label: "Reset"}, {Label: "Cancel"}})
		})).
		Child(button("Input", func(w *gpui.Window) {
			w.PromptForInput(gpui.PromptLevelInfo, "Goto line", "Line number", "", "10", "")
		})).
		Child(button("Zoom +", func(w *gpui.Window) { w.ZoomIn() })).
		Child(button("Zoom -", func(w *gpui.Window) { w.ZoomOut() })).
		Child(button("Zoom 0", func(w *gpui.Window) { w.ZoomReset() })).
		Child(button("Notify", func(w *gpui.Window) {
			if err := sendNotification(platform.Notification{
				Title: "go-gpui",
				Body:  "Hello from the notification API!",
			}); err != nil {
				g.addLog("notification unavailable: " + err.Error())
			} else {
				g.addLog("notification sent")
			}
			w.Refresh()
		})).
		Child(button("Debug", func(w *gpui.Window) {
			w.SetDebugOverlay(!w.DebugOverlayEnabled())
		}))
}

// renderLog builds the event log: a scrollable div tracked by the
// ScrollHandle (autoscroll keeps the tail visible) and a file-drop
// target.
func (g *galleryState) renderLog(w *gpui.Window) gpui.Element {
	px := func(v float32) geometry.Length { return geometry.LengthFromPixels(geometry.Px(v)) }
	if g.scroll == nil {
		g.scroll = elements.NewScrollHandle()
	}
	rows := make([]gpui.Element, 0, len(g.log))
	for _, line := range g.log {
		rows = append(rows, elements.NewText("  "+line).WithTextColor(fg))
	}
	if len(rows) == 0 {
		rows = append(rows, elements.NewText("  (drop files on the window, click buttons, answer prompts...)").WithTextColor(fg))
	}
	log := elements.NewDiv().
		ID(gpui.NameElementId("log")).
		A11y(gpui.AccessibilityList, "event log").
		TrackScroll(g.scroll).
		Flex().FlexCol().P_2().
		MaxH(px(120)).
		OverflowYScroll().
		OnFileDrop(func(ev platform.FileDropEvent, w *gpui.Window) {
			g.addLog("file drop: " + strings.Join(ev.Paths, ", "))
			w.Refresh()
		})
	for _, r := range rows {
		log.Child(r)
	}
	return log
}

func swatch(color gc.Hsla, label string) *elements.Div {
	return elements.NewDiv().
		P_4().W(geometry.LengthFromPixels(geometry.Px(120))).
		H(geometry.LengthFromPixels(geometry.Px(90))).
		Flex().JustifyCenter().ItemsCenter().
		A11y(gpui.AccessibilityImage, label).
		BgColor(color).Rounded_Md().
		Child(elements.NewText(label).WithTextColor(bg0))
}

// Catppuccin Mocha, the upstream editor's default dark theme family.
var (
	bg0     = gc.RGB(0x1e1e2e).Hsla()
	surface = gc.RGB(0x313244).Hsla()
	blue    = gc.RGB(0x89b4fa).Hsla()
	green   = gc.RGB(0xa6e3a1).Hsla()
	red     = gc.RGB(0xf38ba8).Hsla()
	peach   = gc.RGB(0xfab387).Hsla()
	fg      = gc.RGB(0xcdd6f4).Hsla()
)

// Typed actions (upstream actions! macro -> action.Unit[T]).
type (
	zoomInAction    struct{ action.Unit[zoomInAction] }
	zoomOutAction   struct{ action.Unit[zoomOutAction] }
	zoomResetAction struct{ action.Unit[zoomResetAction] }
)

// Action names (keymap bindings target these).
func (zoomInAction) Name() string    { return "gallery::ZoomIn" }
func (zoomOutAction) Name() string   { return "gallery::ZoomOut" }
func (zoomResetAction) Name() string { return "gallery::ZoomReset" }

// installKeyBindings wires the keymap actions to the window. Called from
// render (paint time) so the listeners exist in every rendered frame.
func installKeyBindings(w *gpui.Window) {
	gpui.OnAction(w, func(a zoomInAction, phase gpui.DispatchPhase, w *gpui.Window) {
		if phase == gpui.BubblePhase {
			w.ZoomIn()
		}
	})
	gpui.OnAction(w, func(a zoomOutAction, phase gpui.DispatchPhase, w *gpui.Window) {
		if phase == gpui.BubblePhase {
			w.ZoomOut()
		}
	})
	gpui.OnAction(w, func(a zoomResetAction, phase gpui.DispatchPhase, w *gpui.Window) {
		if phase == gpui.BubblePhase {
			w.ZoomReset()
		}
	})
}

// activePlatform holds the demo platform for notification sends (the
// window API surfaces PlatformWindow; platform-wide integration lives on
// the Platform).
var activePlatform platform.Platform

func sendNotification(n platform.Notification) error {
	return activePlatform.SendNotification(n)
}

func main() {
	a := demo.Start[galleryState](demo.Config{
		Name:  "gallery",
		Title: "go-gpui gallery",
		Size:  geometry.SizePixels(980, 900),
	}, func(w *gpui.Window, cx *app.Context[galleryState]) galleryState {
		g := &galleryState{scroll: elements.NewScrollHandle()}
		g.addLog("gallery ready")
		return *g
	})

	activePlatform = a.Platform
	state := &galleryState{scroll: elements.NewScrollHandle()}
	a.SetRoot(func(w *gpui.Window, c *app.App) gpui.Element {
		return state.render(w, c)
	})

	// Keyboard shortcuts drive the same actions as the menu (zoom keys
	// match the upstream editor: ctrl+= / ctrl+- / ctrl+0 via the keymap).
	gpui.RegisterActionName(zoomInAction{}, "gallery::ZoomIn")
	gpui.RegisterActionName(zoomOutAction{}, "gallery::ZoomOut")
	gpui.RegisterActionName(zoomResetAction{}, "gallery::ZoomReset")
	km := keymap.New(keymap.OSLinux)
	for _, b := range []struct {
		keys   string
		action action.Action
	}{
		{"ctrl-=", zoomInAction{}},
		{"ctrl--", zoomOutAction{}},
		{"ctrl-0", zoomResetAction{}},
	} {
		binding, err := keymap.NewBinding(b.keys, b.action, "", keymap.OSLinux)
		if err != nil {
			demo.Fatal("gallery", err)
		}
		km.AddBindings(binding)
	}
	a.Handle.Window.SetKeymap(km)

	if a.Headless == nil {
		a.Loop() // blocks until the window is closed
		return
	}

	hw := a.Headless.LastWindow()
	pngBytes, err := hw.PNG()
	if err != nil {
		demo.Fatal("gallery", err)
	}
	out := filepath.Join(a.OutDir, "gallery.png")
	if err := os.WriteFile(out, pngBytes, 0o644); err != nil {
		demo.Fatal("gallery", err)
	}

	// Verify the frame: opaque and colorful (not a blank render).
	img := hw.Image()
	colors, opaque := countPixels(img)
	if colors < 50 {
		demo.Fatal("gallery", fmt.Errorf("suspiciously few colors (%d): rendering likely broken", colors))
	}
	if opaque < 0.95 {
		demo.Fatal("gallery", fmt.Errorf("frame should be opaque, got %.2f", opaque))
	}
	demo.Printf("gallery", "wrote %s (%dx%d, %d unique colors, %.0f%% opaque)",
		out, img.Width, img.Height, colors, opaque*100)
}

func countPixels(img *render.RenderImage) (int, float64) {
	colors := map[uint32]bool{}
	opaque := 0
	for i := 0; i < img.Width*img.Height; i++ {
		if img.Pixels[i*4+3] == 255 {
			opaque++
		}
		colors[uint32(img.Pixels[i*4])<<16|uint32(img.Pixels[i*4+1])<<8|uint32(img.Pixels[i*4+2])] = true
	}
	return len(colors), float64(opaque) / float64(img.Width*img.Height)
}
