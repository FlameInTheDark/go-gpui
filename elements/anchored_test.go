package elements

import (
	"testing"

	"github.com/FlameInTheDark/go-gpui/app"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/platform/headless"
	"github.com/FlameInTheDark/go-gpui/style"
)

// openAnchoredWindow opens a headless window of the given logical size and
// renders the element built by build (re-invoked on every draw, like a view
// render). It returns the window for bounds/scene assertions.
func openAnchoredWindow(t *testing.T, width, height float32, build func(w *gpui.Window) gpui.Element) *gpui.Window {
	t.Helper()
	hp := headless.New(geometry.SizePixels(width, height))
	appl := gpui.NewApplication(hp)
	handle, err := gpui.OpenWindow[struct{}](appl, platform.WindowOptions{}, func(w *gpui.Window, cx *app.Context[struct{}]) struct{} {
		return struct{}{}
	})
	if err != nil {
		t.Fatalf("open window: %v", err)
	}
	handle.Window.SetRootView(handle.View.ID(), func(w *gpui.Window, cx *app.App) gpui.Element {
		return build(w)
	})
	handle.Window.Draw()
	return handle.Window
}

func pxLen(v float32) geometry.Length { return geometry.LengthFromPixels(geometry.Px(v)) }

func assertBounds(t *testing.T, name string, b geometry.Bounds[geometry.Pixels], x, y, w, h float32) {
	t.Helper()
	if d := b.Origin.X.AsF32() - x; d < -0.51 || d > 0.51 {
		t.Fatalf("%s origin.x = %v, want %v", name, b.Origin.X.AsF32(), x)
	}
	if d := b.Origin.Y.AsF32() - y; d < -0.51 || d > 0.51 {
		t.Fatalf("%s origin.y = %v, want %v", name, b.Origin.Y.AsF32(), y)
	}
	if d := b.Size.Width.AsF32() - w; d < -0.51 || d > 0.51 {
		t.Fatalf("%s width = %v, want %v", name, b.Size.Width.AsF32(), w)
	}
	if d := b.Size.Height.AsF32() - h; d < -0.51 || d > 0.51 {
		t.Fatalf("%s height = %v, want %v", name, b.Size.Height.AsF32(), h)
	}
}

// TestAnchoredFlipsWhenOverflowingBottom verifies the child flips vertically
// when it would overflow the window bottom (upstream AnchoredFitMode::
// SwitchAnchor: the corner switches along the axis when the flipped side
// fits). The trigger sits near the bottom of a 400x300 window; a 120px-tall
// menu pinned with its top-left corner to the trigger's top-left corner
// would end 70px past the bottom, so it flips above the anchor instead.
func TestAnchoredFlipsWhenOverflowingBottom(t *testing.T) {
	var menu, trigger *Div
	win := openAnchoredWindow(t, 400, 300, func(w *gpui.Window) gpui.Element {
		trigger = NewDiv().
			Absolute().
			Left(pxLen(100)).
			Top(pxLen(250)).
			W(pxLen(80)).
			H(pxLen(24))
		RegisterAnchor(w, "trigger", trigger)
		menu = NewDiv().W(pxLen(100)).H(pxLen(120))
		anchored := NewAnchored(menu).
			AnchorCorner(style.AnchorCornerTopLeft).
			Anchor("trigger")
		return NewDiv().Flex().FlexCol().Children(trigger, anchored)
	})

	assertBounds(t, "trigger", win.ElementBounds(trigger), 100, 250, 80, 24)
	b := win.ElementBounds(menu)
	// Desired (100, 250)-(200, 370) overflows the bottom: flipped to
	// (100, 130)-(200, 250), entirely inside the window.
	assertBounds(t, "menu", b, 100, 130, 100, 120)
	if b.Bottom().AsF32() > 300 {
		t.Fatalf("menu bottom = %v overflows the 300px window", b.Bottom().AsF32())
	}
}

// TestAnchoredSwitchesAnchorHorizontally verifies the child flips
// horizontally (submenu behavior: a menu pinned to the right edge of its
// anchor flips to the left side when the right side overflows).
func TestAnchoredSwitchesAnchorHorizontally(t *testing.T) {
	var menu, trigger *Div
	win := openAnchoredWindow(t, 400, 300, func(w *gpui.Window) gpui.Element {
		trigger = NewDiv().
			Absolute().
			Left(pxLen(350)).
			Top(pxLen(100)).
			W(pxLen(40)).
			H(pxLen(20))
		RegisterAnchor(w, "trigger", trigger)
		menu = NewDiv().W(pxLen(100)).H(pxLen(50))
		anchored := NewAnchored(menu).
			AnchorCorner(style.AnchorCornerTopLeft).
			Anchor("trigger")
		return NewDiv().Flex().FlexCol().Children(trigger, anchored)
	})

	// Desired (350, 100)-(450, 150) overflows the right edge: flipped to
	// (250, 100)-(350, 150), entirely inside the window.
	assertBounds(t, "menu", win.ElementBounds(menu), 250, 100, 100, 50)
}

// TestAnchoredClampsHorizontally verifies SnapToWindow clamps the child to
// the window edges instead of flipping the corner (upstream
// AnchoredFitMode::SnapToWindow: snap desired.right() to limits.right()).
func TestAnchoredClampsHorizontally(t *testing.T) {
	var menu, trigger *Div
	win := openAnchoredWindow(t, 400, 300, func(w *gpui.Window) gpui.Element {
		trigger = NewDiv().
			Absolute().
			Left(pxLen(350)).
			Top(pxLen(100)).
			W(pxLen(40)).
			H(pxLen(20))
		RegisterAnchor(w, "trigger", trigger)
		menu = NewDiv().W(pxLen(100)).H(pxLen(50))
		anchored := NewAnchored(menu).
			AnchorCorner(style.AnchorCornerTopLeft).
			Anchor("trigger").
			SnapToWindow(true)
		return NewDiv().Flex().FlexCol().Children(trigger, anchored)
	})

	// No flip: the desired (350..450) is clamped so the right edge lands on
	// the window edge (origin.x = 400 - 100 = 300).
	assertBounds(t, "menu", win.ElementBounds(menu), 300, 100, 100, 50)
}

// TestAnchoredInPlaceAndOffset verifies the default in-place placement (the
// child sits at the Anchored element's own layout origin, upstream
// Placement::InPlace) and that Offset shifts it (upstream Anchored::offset).
func TestAnchoredInPlaceAndOffset(t *testing.T) {
	var menu *Div
	win := openAnchoredWindow(t, 200, 200, func(w *gpui.Window) gpui.Element {
		menu = NewDiv().W(pxLen(50)).H(pxLen(40))
		anchored := NewAnchored(menu).Offset(geometry.Point[geometry.Pixels]{
			X: geometry.Px(10), Y: geometry.Px(5),
		})
		return NewDiv().Flex().FlexCol().Child(anchored)
	})

	// In place at the window origin plus the offset; no overflow, so no
	// flip and no clamping.
	assertBounds(t, "menu", win.ElementBounds(menu), 10, 5, 50, 40)
}
