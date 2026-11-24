// Command hello is a small go-gpui application: a click counter with hover
// styling, colored cards, a gradient, text, an SVG icon, a modal prompt
// and window zoom (ctrl+= / ctrl+- / ctrl+0). It opens a native window
// on Windows (win32) and Linux (X11/Wayland) - pure Go, no cgo.
//
// By default it runs interactively until the window is closed; with
// --headless it renders one PNG and verifies a click through the input
// pipeline:
//
//	go run ./examples/hello
//	go run ./examples/hello --headless --out /tmp/frames
package main

import (
	"fmt"
	"os"
	"path/filepath"

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

var (
	bg      = gc.RGB(0x1e1e2e).Hsla()
	surface = gc.RGB(0x313244).Hsla()
	blue    = gc.RGB(0x89b4fa).Hsla()
	green   = gc.RGB(0xa6e3a1).Hsla()
	red     = gc.RGB(0xf38ba8).Hsla()
	peach   = gc.RGB(0xfab387).Hsla()
	textC   = gc.RGB(0xcdd6f4).Hsla()
	dim     = gc.RGB(0x6c7086).Hsla()
)

var lightBlue = gc.RGB(0xb4c9f8).Hsla()

// helloState is the view state: the click count.
type helloState struct {
	clicks int
}

func (s *helloState) render(handle *gpui.WindowHandle[helloState], cx *app.App) gpui.Element {
	px := func(v float32) geometry.Length { return geometry.LengthFromPixels(geometry.Px(v)) }

	button := elements.NewDiv().
		ID(elements.ButtonID("counter")).
		Px_6().Py_3().Rounded_Lg().
		BgColor(blue).
		Hover(func(s *style.StyleRefinement) {
			s.BgColor(lightBlue)
		}).
		Child(elements.NewText("Click me").WithTextColor(bg)).
		OnClick(func(ev gpui.ClickEvent, w *gpui.Window) {
			handle.View.Update(cx, func(state *helloState, c *app.Context[helloState]) any {
				state.clicks++
				return nil
			})
			w.Draw()
		})

	countText := "Clicks: 0"
	if s.clicks > 0 {
		countText = fmt.Sprintf("Clicks: %d", s.clicks)
	}

	return elements.NewDiv().
		Flex().FlexCol().P_8().Gap_6().FlexGrow(1).
		BgColor(bg).
		Child(
			elements.NewDiv().TextSize(geometry.PixelsLength(geometry.Px(22))).
				Child(elements.NewText("hello, go-gpui").WithTextColor(textC)),
		).
		Child(
			elements.NewDiv().Flex().Gap_4().
				Child(elements.NewDiv().W(px(64)).H(px(48)).Rounded_Md().BgColor(green)).
				Child(elements.NewDiv().W(px(64)).H(px(48)).Rounded_Full().BgColor(peach)).
				Child(elements.NewDiv().W(px(64)).H(px(48)).Rounded_Xl().BgColor(red).
					Border_1().BorderColor(textC)).
				Child(elements.NewDiv().W(px(64)).H(px(48)).Rounded_Md().
					Bg(gc.LinearGradient(90,
						gc.NewLinearColorStop(blue, 0),
						gc.NewLinearColorStop(green, 1)))),
		).
		Child(
			elements.NewDiv().Flex().ItemsCenter().Gap_4().
				Child(button).
				Child(elements.NewText(countText).WithTextColor(green)).
				Child(elements.NewText("clicks so far").WithTextColor(dim)).
				Child(promptButton()).
				Child(zoomButtons()),
		).
		Child(
			elements.NewDiv().Flex().ItemsCenter().Gap_2().
				Child(elements.NewSvgFromData([]byte(
					`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><path d="M2 8 L6 12 L14 3" fill="none" stroke="#a6e3a1" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>`)).
					W(px(20)).H(px(20))).
				Child(elements.NewText("pure Go - no cgo").WithTextColor(dim)),
		)
}

// promptButton opens an info prompt (upstream Window::prompt).
func promptButton() *elements.Div {
	return elements.NewDiv().
		ID(elements.ButtonID("prompt")).
		A11y(gpui.AccessibilityButton, "About prompt").
		Px_6().Py_3().Rounded_Lg().BgColor(peach).
		Child(elements.NewText("Prompt").WithTextColor(bg)).
		OnClick(func(ev gpui.ClickEvent, w *gpui.Window) {
			w.Prompt(gpui.PromptLevelInfo, "hello, go-gpui",
				"This dialog is the ported prompt system.", "",
				[]gpui.PromptAction{{Label: "OK"}})
		})
}

// zoomButtons wire the window zoom API (upstream Window::zoom).
func zoomButtons() *elements.Div {
	zoomRow := elements.NewDiv().Flex().ItemsCenter().Gap_2().
		A11y(gpui.AccessibilityContainer, "zoom controls")
	for _, spec := range []struct {
		label string
		fn    func(*gpui.Window)
	}{{"+", (*gpui.Window).ZoomIn}, {"-", (*gpui.Window).ZoomOut}, {"0", (*gpui.Window).ZoomReset}} {
		spec := spec
		zoomRow.Child(elements.NewDiv().
			ID(elements.ButtonID("zoom-"+spec.label)).
			A11y(gpui.AccessibilityButton, "zoom "+spec.label).
			Px_3().Py_2().Rounded_Md().BgColor(surface).
			Child(elements.NewText(spec.label).WithTextColor(textC)).
			OnClick(func(ev gpui.ClickEvent, w *gpui.Window) { spec.fn(w) }))
	}
	return zoomRow
}

func main() {
	a := demo.Start[helloState](demo.Config{
		Name:  "hello",
		Title: "hello go-gpui",
		Size:  geometry.SizePixels(520, 400),
	}, func(w *gpui.Window, cx *app.Context[helloState]) helloState {
		return helloState{}
	})
	cx := a.App.App

	a.SetRoot(func(w *gpui.Window, c *app.App) gpui.Element {
		return a.Handle.View.Read(cx).render(&a.Handle, cx)
	})

	if a.Headless == nil {
		a.Loop() // blocks until the window is closed
		return
	}
	verifyHeadless(a, cx)
}

// verifyHeadless renders the frame, clicks the button through the input
// pipeline and verifies the counter.
func verifyHeadless(a *demo.App[helloState], cx *app.App) {
	hw := a.Headless.LastWindow()

	if err := savePNG(hw, filepath.Join(a.OutDir, "hello.png")); err != nil {
		demo.Fatal("hello", err)
	}

	// Click the counter button at its hitbox center (the counter is the
	// first interactive element painted; the prompt and zoom buttons
	// follow).
	hitboxes := a.Handle.Window.RenderedHitboxes()
	if len(hitboxes) < 1 {
		demo.Fatal("hello", fmt.Errorf("no button hitboxes"))
	}
	b := hitboxes[0].Bounds
	pos := geometry.PointPixels(
		float32(b.Origin.X+b.Size.Width/2),
		float32(b.Origin.Y+b.Size.Height/2))
	hw.DispatchInput(platform.MouseDownEvent{Button: platform.ButtonLeft, Position: pos})
	hw.DispatchInput(platform.MouseUpEvent{Button: platform.ButtonLeft, Position: pos})

	if state := a.Handle.View.Read(cx); state.clicks != 1 {
		demo.Fatal("hello", fmt.Errorf("click count = %d, want 1", state.clicks))
	}
	if err := savePNG(hw, filepath.Join(a.OutDir, "hello_clicked.png")); err != nil {
		demo.Fatal("hello", err)
	}
	demo.Printf("hello", "frame rendered and click verified")
}

func savePNG(hw *headless.Window, path string) error {
	pngBytes, err := hw.PNG()
	if err != nil {
		return err
	}
	return os.WriteFile(path, pngBytes, 0o644)
}
