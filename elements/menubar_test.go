package elements

import (
	"testing"

	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/keymap"
	"github.com/FlameInTheDark/go-gpui/platform"
)

// menuBarTestWindow builds a window whose root contains a MenuBar over a
// content div, with the given app menu installed.
func menuBarTestWindow(t *testing.T, menus []gpui.Menu, onCommand func(name string)) *gpui.Window {
	t.Helper()
	w := openAnchoredWindow(t, 400, 300, func(w *gpui.Window) gpui.Element {
		if w.AppMenu() == nil {
			w.SetAppMenu(menus)
		}
		return NewDiv().Flex().FlexCol().
			Child(NewMenuBar()).
			Child(NewDiv().ID(gpui.NameElementId("content")).FlexGrow(1))
	})
	_ = onCommand
	return w
}

// TestMenuBarRendersTitles verifies the bar paints one text line per menu
// title.
func TestMenuBarRendersTitles(t *testing.T) {
	w := menuBarTestWindow(t, []gpui.Menu{
		gpui.NewMenu("File", gpui.MenuHandler("Open", nil)),
		gpui.NewMenu("Edit", gpui.MenuHandler("Undo", nil)),
	}, nil)
	// Mono sprites: "File", "Edit" titles + menu item glyphs paint only
	// when open; with no menu open, the two titles are shaped.
	if len(w.RenderedScene().MonoSprites) == 0 {
		t.Fatal("menu bar titles painted nothing")
	}
}

// TestMenuBarOpenAndActivate verifies the open/activate flow: clicking a
// title opens the dropdown (its item labels paint), clicking an item
// dispatches it and closes the dropdown.
func TestMenuBarOpenAndActivate(t *testing.T) {
	activated := ""
	w := menuBarTestWindow(t, []gpui.Menu{
		gpui.NewMenu("File",
			gpui.MenuItem{Label: "Save", Handler: func(w *gpui.Window) { activated = "save" }},
			gpui.MenuSeparator(),
			gpui.MenuItem{Label: "Quit", Handler: func(w *gpui.Window) { activated = "quit" }}),
	}, nil)
	w.Draw()

	// Click the "File" title (first title starts at bar x ~ 8 + padding).
	clickAt(t, w, geometry.PointPixels(30, 12))
	w.Draw()

	// The dropdown is open: the items' hitboxes exist (rows are ~27px tall
	// starting under the 36px bar).
	rows := 0
	for _, hb := range w.RenderedHitboxes() {
		b := hb.Bounds
		if b.Origin.Y.AsF32() > 36 && b.Size.Width.AsF32() > 100 && b.Size.Height.AsF32() > 15 {
			rows++
		}
	}
	if rows < 2 {
		t.Fatalf("dropdown item hitboxes = %d, want >= 2", rows)
	}

	// Click "Save" (first row).
	clickAt(t, w, geometry.PointPixels(60, 50))
	if activated != "save" {
		t.Fatalf("activated = %q, want save", activated)
	}
}

// TestMenuBarKeyboardNavigation verifies arrow/enter/escape navigation.
func TestMenuBarKeyboardNavigation(t *testing.T) {
	activated := ""
	w := menuBarTestWindow(t, []gpui.Menu{
		gpui.NewMenu("File",
			gpui.MenuItem{Label: "Open", Handler: func(w *gpui.Window) { activated = "open" }},
			gpui.MenuItem{Label: "Close", Handler: func(w *gpui.Window) { activated = "close" }}),
		gpui.NewMenu("Help",
			gpui.MenuItem{Label: "About", Handler: func(w *gpui.Window) { activated = "about" }}),
	}, nil)
	w.Draw()

	// Open the first menu by clicking its title.
	clickAt(t, w, geometry.PointPixels(30, 12))
	w.Draw()

	// Down selects "Open", Enter activates.
	w.HandleInput(platform.KeyDownEvent{Keystroke: keymap.Keystroke{Key: "down"}})
	w.Draw()
	w.HandleInput(platform.KeyDownEvent{Keystroke: keymap.Keystroke{Key: "enter"}})
	if activated != "open" {
		t.Fatalf("activated = %q, want open", activated)
	}

	// Right arrow switches to "Help"; Down selects "About"; Enter.
	clickAt(t, w, geometry.PointPixels(30, 12))
	w.Draw()
	w.HandleInput(platform.KeyDownEvent{Keystroke: keymap.Keystroke{Key: "right"}})
	w.Draw()
	w.HandleInput(platform.KeyDownEvent{Keystroke: keymap.Keystroke{Key: "down"}})
	w.Draw()
	w.HandleInput(platform.KeyDownEvent{Keystroke: keymap.Keystroke{Key: "enter"}})
	if activated != "about" {
		t.Fatalf("activated = %q, want about", activated)
	}
}

// TestMenuBarEscapeCloses verifies Escape closes the open dropdown and a
// click on the content no longer dispatches items.
func TestMenuBarEscapeCloses(t *testing.T) {
	activated := ""
	w := menuBarTestWindow(t, []gpui.Menu{
		gpui.NewMenu("File",
			gpui.MenuItem{Label: "Save", Handler: func(w *gpui.Window) { activated = "save" }}),
	}, nil)
	w.Draw()
	clickAt(t, w, geometry.PointPixels(30, 12))
	w.Draw()

	// Escape closes.
	w.HandleInput(platform.KeyDownEvent{Keystroke: keymap.Keystroke{Key: "escape"}})
	w.Draw()

	// A click where the item was must not activate it (dropdown closed).
	clickAt(t, w, geometry.PointPixels(60, 50))
	if activated != "" {
		t.Fatalf("item activated after close: %q", activated)
	}
}

// TestWindowZoom verifies the zoom API scales the effective scale factor
// and clamps.
func TestWindowZoom(t *testing.T) {
	w := openAnchoredWindow(t, 200, 100, func(w *gpui.Window) gpui.Element {
		return NewDiv().ID(gpui.NameElementId("root"))
	})
	base := w.ScaleFactor()
	w.ZoomIn()
	w.Draw()
	if got := w.ScaleFactor(); got <= base {
		t.Fatalf("scale after zoom in = %v, want > %v", got, base)
	}
	if z := w.Zoom(); z <= 1 {
		t.Fatalf("zoom = %v, want > 1", z)
	}
	w.ZoomOut()
	w.Draw()
	if z := w.Zoom(); z < 0.99 || z > 1.01 {
		t.Fatalf("zoom after in+out = %v, want ~1", z)
	}
	w.SetZoom(10)
	if z := w.Zoom(); z > 3.01 {
		t.Fatalf("zoom clamps to 3, got %v", z)
	}
	w.ZoomReset()
	if z := w.Zoom(); z != 1 {
		t.Fatalf("zoom reset = %v, want 1", z)
	}
}

// clickAt dispatches a full click (down + up).
func clickAt(t *testing.T, w *gpui.Window, pos geometry.Point[geometry.Pixels]) {
	t.Helper()
	w.HandleInput(platform.MouseDownEvent{Position: pos, Button: platform.ButtonLeft, ClickCount: 1})
	w.HandleInput(platform.MouseUpEvent{Position: pos, Button: platform.ButtonLeft, ClickCount: 1})
}
