package gpui_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/FlameInTheDark/go-gpui/action"
	"github.com/FlameInTheDark/go-gpui/app"
	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/elements"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/keymap"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/platform/headless"
	"github.com/FlameInTheDark/go-gpui/render"
)

type galleryView struct{}

func (g *galleryView) Render(w *gpui.Window, cx *app.App) gpui.Element {
	return elements.NewDiv().
		Flex().
		FlexCol().
		Gap_4().
		P_8().
		BgColor(gc.RGB(0x1e1e2e).Hsla()).
		W(geometry.LengthFromPixels(geometry.Px(400))).
		H(geometry.LengthFromPixels(geometry.Px(300))).
		Child(
			elements.NewDiv().
				P_4().
				Rounded_Md().
				BgColor(gc.RGB(0x89b4fa).Hsla()).
				Child(elements.NewText("Hello, go-gpui!")),
		).
		Child(
			elements.NewDiv().
				P_4().
				Rounded_Lg().
				BgColor(gc.HSLA(0.3, 0.7, 0.5, 1)).
				Child(elements.NewText("Rounded & colored")),
		).
		Child(
			elements.NewDiv().
				P_4().
				Border_1().
				BorderColor(gc.White()).
				Rounded_Sm().
				Child(elements.NewText("Bordered")),
		)
}

func TestEndToEndRender(t *testing.T) {
	hp := headless.New(geometry.SizePixels(400, 300))
	appl := gpui.NewApplication(hp)
	handle, err := gpui.OpenWindow[galleryView](appl, platform.WindowOptions{Title: "gallery"}, func(w *gpui.Window, cx *app.Context[galleryView]) galleryView {
		return galleryView{}
	})
	if err != nil {
		t.Fatalf("open window: %v", err)
	}
	handle.Window.SetRootView(handle.View.ID(), func(w *gpui.Window, cx *app.App) gpui.Element {
		return (&galleryView{}).Render(w, cx)
	})
	handle.Window.Draw()

	hw := hp.LastWindow()
	if hw == nil {
		t.Fatal("no headless window")
	}
	img := hw.Image()
	if img == nil || img.Width != 400 || img.Height != 300 {
		t.Fatalf("image: %+v", img)
	}

	seen := map[uint32]bool{}
	opaque := 0
	total := 0
	for i := 0; i+4 <= len(img.Pixels); i += 4 {
		total++
		if img.Pixels[i+3] == 255 {
			opaque++
		}
		key := uint32(img.Pixels[i])<<16 | uint32(img.Pixels[i+1])<<8 | uint32(img.Pixels[i+2])
		seen[key] = true
	}
	opaqueRatio := float64(opaque) / float64(total)
	t.Logf("opaque ratio: %.2f, unique colors: %d", opaqueRatio, len(seen))
	if opaqueRatio < 0.9 {
		t.Fatalf("window should be opaque, got %.2f", opaqueRatio)
	}
	if len(seen) < 3 {
		t.Fatalf("expected multiple colors (bg + cards + text), got %d", len(seen))
	}

	// Write the PNG for visual verification when requested.
	if os.Getenv("GPUI_WRITE_PNG") != "" {
		pngBytes, err := hw.PNG()
		if err != nil {
			t.Fatal(err)
		}
		if len(pngBytes) < 100 {
			t.Fatal("empty png")
		}
		if err := os.WriteFile("/tmp/gpui_e2e.png", pngBytes, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestClickInteraction drives the mouse pipeline: click counter.
type clickView struct {
	clicks int
}

func TestClickInteraction(t *testing.T) {
	hp := headless.New(geometry.SizePixels(200, 100))
	appl := gpui.NewApplication(hp)
	handle, err := gpui.OpenWindow[clickView](appl, platform.WindowOptions{Title: "clicks"}, func(w *gpui.Window, cx *app.Context[clickView]) clickView {
		return clickView{}
	})
	if err != nil {
		t.Fatal(err)
	}

	// Root: a button that counts clicks, and a label showing the count.
	handle.Window.SetRootView(handle.View.ID(), func(w *gpui.Window, cx *app.App) gpui.Element {
		return elements.NewDiv().
			P_4().
			Child(
				elements.NewDiv().
					ID(elements.ButtonID("counter")).
					P_4().
					BgColor(gc.RGB(0x4488ff).Hsla()).
					OnClick(func(ev gpui.ClickEvent, w *gpui.Window) {
						handle.View.Update(appl.App, func(view *clickView, cx *app.Context[clickView]) any {
							view.clicks++
							return nil
						})
					}).
					Child(elements.NewText("Click me")),
			).
			Child(elements.NewDiv().Child(
				elements.NewText("Count will update on click"),
			))
	})
	handle.Window.Draw()

	// Simulate a click through the platform input pipeline.
	hw := hp.LastWindow()
	pos := geometry.PointPixels(30, 30)
	result := hw.DispatchInput(platform.MouseDownEvent{Button: platform.ButtonLeft, Position: pos})
	if !result.Propagate {
		t.Log("mouse down consumed")
	}
	result = hw.DispatchInput(platform.MouseUpEvent{Button: platform.ButtonLeft, Position: pos})
	t.Logf("mouse up propagate: %v", result.Propagate)

	handle.View.Read(appl.App)
	if handle.View.Read(appl.App).clicks != 1 {
		t.Fatalf("click count: %d", handle.View.Read(appl.App).clicks)
	}
}

// TestTextRendering checks glyph ink lands on the canvas.
func TestTextRendering(t *testing.T) {
	hp := headless.New(geometry.SizePixels(200, 60))
	appl := gpui.NewApplication(hp)
	handle, err := gpui.OpenWindow[galleryView](appl, platform.WindowOptions{Title: "text"}, func(w *gpui.Window, cx *app.Context[galleryView]) galleryView {
		return galleryView{}
	})
	if err != nil {
		t.Fatal(err)
	}
	handle.Window.SetRootView(handle.View.ID(), func(w *gpui.Window, cx *app.App) gpui.Element {
		return elements.NewDiv().
			BgColor(gc.White()).
			P_8().
			Child(elements.NewText("Ag").WithTextColor(gc.Black()))
	})
	handle.Window.Draw()

	img := hp.LastWindow().Image()
	ink := 0
	for i := 0; i < img.Width*img.Height; i++ {
		if img.Pixels[i*4] < 128 && img.Pixels[i*4+3] > 200 {
			ink++
		}
	}
	if ink < 10 {
		t.Fatalf("expected glyph ink, got %d dark pixels", ink)
	}
}

// TestNestedFlexLayout verifies children stack vertically with padding.
func TestNestedFlexLayout(t *testing.T) {
	hp := headless.New(geometry.SizePixels(100, 200))
	appl := gpui.NewApplication(hp)
	handle, err := gpui.OpenWindow[galleryView](appl, platform.WindowOptions{}, func(w *gpui.Window, cx *app.Context[galleryView]) galleryView {
		return galleryView{}
	})
	if err != nil {
		t.Fatal(err)
	}
	handle.Window.SetRootView(handle.View.ID(), func(w *gpui.Window, cx *app.App) gpui.Element {
		return elements.NewDiv().
			FlexCol().
			// P_10 is the Tailwind spacing scale: 10 * 0.25rem = 40px.
			P_10().
			BgColor(gc.Black()).
			Child(elements.NewDiv().H(geometry.LengthFromPixels(geometry.Px(50))).BgColor(gc.Red()))
	})
	handle.Window.Draw()

	img := hp.LastWindow().Image()
	if img == nil {
		t.Fatal("no image")
	}
	// Padding box is 40px: the child fills (40,40)..(60,90) (width stretched
	// to the content box, height 50).
	at := func(x, y int) (int, int, int) {
		idx := (y*img.Width + x) * 4
		return int(img.Pixels[idx]), int(img.Pixels[idx+1]), int(img.Pixels[idx+2])
	}
	if r, g, b := at(50, 50); r < 200 || g > 100 || b > 100 {
		t.Fatalf("expected red at (50,50): %d %d %d", r, g, b)
	}
	if r, g, b := at(50, 100); r > 50 || g > 50 || b > 50 {
		t.Fatalf("expected black below the child at (50,100): %d %d %d", r, g, b)
	}
	if r, _, _ := at(10, 10); r > 50 {
		t.Fatalf("expected black in the padding at (10,10): %d", r)
	}
}

// TestVirtualizedList checks the uniform list renders a scrolled window.
func TestVirtualizedList(t *testing.T) {
	hp := headless.New(geometry.SizePixels(200, 150))
	appl := gpui.NewApplication(hp)
	handle, err := gpui.OpenWindow[galleryView](appl, platform.WindowOptions{}, func(w *gpui.Window, cx *app.Context[galleryView]) galleryView {
		return galleryView{}
	})
	if err != nil {
		t.Fatal(err)
	}
	handle.Window.SetRootView(handle.View.ID(), func(w *gpui.Window, cx *app.App) gpui.Element {
		list := elements.NewUniformList(1000, func(ix int, w *gpui.Window) gpui.Element {
			return elements.NewDiv().P_2().Child(
				elements.NewText("Item " + itoa(ix)))
		})
		// A flex column with the default shrink lets the list take the
		// viewport height (the standard upstream-style list container).
		return elements.NewDiv().Flex().FlexCol().P_4().BgColor(gc.White()).Child(list)
	})
	handle.Window.Draw()

	img := hp.LastWindow().Image()
	if img == nil {
		t.Fatal("no image")
	}
	// Text ink on the white background.
	colors := map[uint32]bool{}
	ink := 0
	for i := 0; i < img.Width*img.Height; i++ {
		colors[uint32(img.Pixels[i*4])<<16|uint32(img.Pixels[i*4+1])<<8|uint32(img.Pixels[i*4+2])] = true
		if img.Pixels[i*4] < 128 && img.Pixels[i*4+3] > 200 {
			ink++
		}
	}
	if len(colors) < 2 || ink < 10 {
		t.Fatalf("list rendered nothing: %d colors, %d ink", len(colors), ink)
	}
	// Virtualization: only a window of items is in the scene, not all 1000.
	sprites := len(handle.Window.Scene().MonoSprites)
	if sprites > 500 {
		t.Fatalf("virtualization broken: %d sprites for 1000 items", sprites)
	}
	_ = render.FullContentMask()
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}

// TestActionPipeline drives the keymap binding pipeline: keystroke ->
// BindingsForInput -> action dispatch -> OnAction listener.
type testQuitAction struct{ action.Unit[testQuitAction] }

func (testQuitAction) Name() string { return "test::Quit" }

func TestActionPipeline(t *testing.T) {
	hp := headless.New(geometry.SizePixels(200, 100))
	appl := gpui.NewApplication(hp)
	handle, err := gpui.OpenWindow[galleryView](appl, platform.WindowOptions{}, func(w *gpui.Window, cx *app.Context[galleryView]) galleryView {
		return galleryView{}
	})
	if err != nil {
		t.Fatal(err)
	}

	gpui.RegisterActionName(testQuitAction{}, "test::Quit")
	quitCount := 0
	handle.Window.SetRootView(handle.View.ID(), func(w *gpui.Window, cx *app.App) gpui.Element {
		gpui.OnAction(w, func(a testQuitAction, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase == gpui.BubblePhase {
				quitCount++
			}
		})
		return elements.NewDiv().KeyContext("test").Child(
			elements.NewText("press ctrl-q to quit"))
	})

	// Bind ctrl-q to test::Quit.
	km := keymap.New(keymap.OSLinux)
	binding, err := keymap.NewBinding("ctrl-q", testQuitAction{}, "test", keymap.OSLinux)
	if err != nil {
		t.Fatal(err)
	}
	km.AddBindings(binding)
	handle.Window.SetKeymap(km)

	handle.Window.Draw()

	// Dispatch a ctrl-q key-down through the window pipeline.
	ks, err := keymap.ParseKeystroke("ctrl-q", keymap.OSLinux)
	if err != nil {
		t.Fatal(err)
	}
	hw := hp.LastWindow()
	hw.DispatchInput(platform.KeyDownEvent{Keystroke: ks})

	if quitCount != 1 {
		t.Fatalf("expected action to fire once, got %d", quitCount)
	}
	if !handle.Window.IsActionAvailable("test::Quit") {
		t.Fatal("action should be available")
	}
	if b, ok := handle.Window.HighestPrecedenceBindingForAction("test::Quit"); !ok || len(b.Keystroke) != 1 {
		t.Fatalf("highest precedence binding missing: %+v ok=%v", b, ok)
	}
}

var _ = bytes.MinRead

// TestHiDPIHitTestingAndRender locks in the logical/physical pixel
// separation that DPI-aware platform backends must follow: layout and mouse
// input in logical pixels, render target in physical pixels (scale x).
func TestHiDPIHitTestingAndRender(t *testing.T) {
	hp := headless.New(geometry.SizePixels(300, 200))
	appl := gpui.NewApplication(hp)
	clicked := 0
	handle, err := gpui.OpenWindow[struct{}](appl, platform.WindowOptions{
		ScaleFactor: 2,
	}, func(w *gpui.Window, cx *app.Context[struct{}]) struct{} { return struct{}{} })
	if err != nil {
		t.Fatal(err)
	}

	button := elements.NewDiv().
		ID(elements.ButtonID("hidpi-target")).
		Absolute().
		Left(geometry.LengthFromPixels(geometry.Px(10))).
		Top(geometry.LengthFromPixels(geometry.Px(10))).
		W(geometry.LengthFromPixels(geometry.Px(100))).
		H(geometry.LengthFromPixels(geometry.Px(30))).
		BgColor(gc.RGB(0x89b4fa).Hsla()).
		OnClick(func(ev gpui.ClickEvent, w *gpui.Window) {
			clicked++
		})
	handle.Window.SetRootView(handle.View.ID(), func(w *gpui.Window, cx *app.App) gpui.Element {
		return elements.NewDiv().Flex().FlexCol().BgColor(gc.RGB(0x1e1e2e).Hsla()).Child(button)
	})
	handle.Window.Draw()

	hw := hp.LastWindow()

	// The render target must be the physical size: 300x200 logical at 2x.
	img := hw.Image()
	if img == nil {
		t.Fatal("no rendered frame")
	}
	if img.Width != 600 || img.Height != 400 {
		t.Fatalf("render target = %dx%d, want 600x400 (logical 300x200 at 2x)", img.Width, img.Height)
	}

	// Mouse input is logical: the button spans logical (10,10)-(110,40).
	click := func(x, y float32) {
		pos := geometry.PointPixels(x, y)
		hw.DispatchInput(platform.MouseDownEvent{Button: platform.ButtonLeft, Position: pos})
		hw.DispatchInput(platform.MouseUpEvent{Button: platform.ButtonLeft, Position: pos})
	}
	click(50, 25) // center of the button, logical
	if clicked != 1 {
		t.Fatalf("logical click inside the button should hit, clicked=%d", clicked)
	}
	click(150, 25) // logical point outside the button
	if clicked != 1 {
		t.Fatalf("click outside must not hit, clicked=%d", clicked)
	}

	// Hover detection must also be logical: (50,25) hovers the button.
	hw.DispatchInput(platform.MouseMoveEvent{Position: geometry.PointPixels(50, 25)})
	for _, hb := range handle.Window.RenderedHitboxes() {
		if hb.ID == 0 {
			continue
		}
		b := hb.Bounds
		if !geometry.BoundsContains(b, geometry.PointPixels(50, 25)) {
			t.Fatalf("hitbox %v must contain logical (50,25)", b)
		}
	}

	// The rendered image must show the button's color at its physical
	// position: logical (50,25) is physical (100,50).
	px := img.Pixels[(50*img.Width+100)*4:]
	r, g, b := int(px[0]), int(px[1]), int(px[2])
	if r < 0x60 || g < 0x80 || b < 0xc0 {
		t.Fatalf("physical (100,50) = rgb(%d,%d,%d), want the button's blue", r, g, b)
	}
}
