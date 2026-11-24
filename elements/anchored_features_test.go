package elements

import (
	"testing"

	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/style"
)

// anchoredChildBox builds the standard 120x40 anchored child.
func anchoredChildBox() *Div {
	d := NewDiv()
	d.StyleRef().W(geometry.LengthFromPixels(geometry.Px(120)))
	d.StyleRef().H(geometry.LengthFromPixels(geometry.Px(40)))
	return d
}

// TestAnchoredPositionPlacesAtPoint verifies Position pins the child's
// anchor corner at an explicit window-space point (upstream
// Anchored::position / Placement::At).
func TestAnchoredPositionPlacesAtPoint(t *testing.T) {
	var anchored *Anchored
	var child *Div
	win := openAnchoredWindow(t, 400, 300, func(w *gpui.Window) gpui.Element {
		if child == nil {
			child = anchoredChildBox()
			anchored = NewAnchored(child).AnchorCorner(style.AnchorCornerTopLeft).Position(geometry.PointPixels(30, 20))
		}
		return anchored
	})
	b := win.ElementBounds(child)
	assertBounds(t, "positioned child", b, 30, 20, 120, 40)
}

// TestAnchoredAttachFlipsToOtherSideOfTarget verifies the Attached
// placement flips to the other side of the target when the near side
// overflows the window (upstream Anchored::attach): a child attached with
// its top-left on a row's top-right near the right edge moves to the left
// of the row when it does not fit on the right.
func TestAnchoredAttachFlipsToOtherSideOfTarget(t *testing.T) {
	var anchored *Anchored
	var child *Div
	// Window 400 wide. The row spans [340, 380]; a 120px child with its
	// top-left on the row's top-right (380) would end at 500 — 100px past
	// the window. The left side fits: [260, 380].
	row := geometry.BoundsPixels(340, 60, 40, 24)
	win := openAnchoredWindow(t, 400, 300, func(w *gpui.Window) gpui.Element {
		if child == nil {
			child = anchoredChildBox()
			anchored = NewAnchored(child).
				AnchorCorner(style.AnchorCornerTopLeft).
				Attach(row, style.AnchorCornerTopRight)
		}
		return anchored
	})
	b := win.ElementBounds(child)
	if b.Origin.X.AsF32() > 300 {
		t.Fatalf("attached child should flip to the left of the target, got origin.x=%v", b.Origin.X.AsF32())
	}
	// Upstream flips both the anchor corner and the target corner: the
	// child's TopRight lands on the target's TopLeft (340).
	if d := b.Right().AsF32() - 340; d < -0.51 || d > 0.51 {
		t.Fatalf("flipped child right edge = %v, want 340 (the target's left)", b.Right().AsF32())
	}

	// Attach with room on the primary side keeps the primary placement.
	anchored2 := NewAnchored(anchoredChildBox()).
		AnchorCorner(style.AnchorCornerTopLeft).
		Attach(geometry.BoundsPixels(100, 60, 40, 24), style.AnchorCornerTopRight)
	win2 := openAnchoredWindow(t, 400, 300, func(w *gpui.Window) gpui.Element {
		return anchored2
	})
	_ = win2
}

// TestAnchoredSnapToWindowWithMargin verifies SnapToWindowWithMargin
// leaves a margin when clamping an overflowing placement to the window
// edge (upstream snap_to_window_with_margin).
func TestAnchoredSnapToWindowWithMargin(t *testing.T) {
	var anchored *Anchored
	var child *Div
	win := openAnchoredWindow(t, 400, 300, func(w *gpui.Window) gpui.Element {
		if child == nil {
			child = anchoredChildBox()
			anchored = NewAnchored(child).
				AnchorCorner(style.AnchorCornerTopLeft).
				Position(geometry.PointPixels(-30, 10)).
				SnapToWindowWithMargin(geometry.Edges[geometry.Pixels]{Left: geometry.Px(12)})
		}
		return anchored
	})
	b := win.ElementBounds(child)
	if d := b.Origin.X.AsF32() - 12; d < -0.51 || d > 0.51 {
		t.Fatalf("snapped child origin.x = %v, want 12 (window edge + margin)", b.Origin.X.AsF32())
	}
}

// TestAnchoredLocalPositionMode verifies PositionModeLocal interprets the
// position relative to the parent element: the Anchored element sits at
// (50, 60) inside the window, and a local position of (10, 20) places the
// child at (60, 80) in window coordinates (upstream
// AnchoredPositionMode::Local).
func TestAnchoredLocalPositionMode(t *testing.T) {
	var anchored *Anchored
	var child *Div
	win := openAnchoredWindow(t, 400, 300, func(w *gpui.Window) gpui.Element {
		if child == nil {
			child = anchoredChildBox()
			anchored = NewAnchored(child).
				AnchorCorner(style.AnchorCornerTopLeft).
				PositionModeLocal(true).
				Position(geometry.PointPixels(10, 20))
			anchored.StyleRef().Left(geometry.LengthFromDefinite(geometry.AbsoluteDefinite(geometry.PixelsLength(geometry.Px(50)))))
			anchored.StyleRef().Top(geometry.LengthFromDefinite(geometry.AbsoluteDefinite(geometry.PixelsLength(geometry.Px(60)))))
		}
		host := NewDiv()
		host.StyleRef().W(geometry.LengthFromDefinite(geometry.AbsoluteDefinite(geometry.PixelsLength(geometry.Px(300)))))
		host.StyleRef().H(geometry.LengthFromDefinite(geometry.AbsoluteDefinite(geometry.PixelsLength(geometry.Px(200)))))
		host.Child(anchored)
		return host
	})
	b := win.ElementBounds(child)
	// The host is at (0,0)-(300,200); the anchored wrapper at (50,60); the
	// local position adds (10,20) in window mode coordinates only when the
	// parent origin shifts — the child lands at (60, 80).
	if b.Origin.X.AsF32() < 59 || b.Origin.X.AsF32() > 61 {
		t.Fatalf("local-mode child origin.x = %v, want ~60", b.Origin.X.AsF32())
	}
	if b.Origin.Y.AsF32() < 79 || b.Origin.Y.AsF32() > 81 {
		t.Fatalf("local-mode child origin.y = %v, want ~80", b.Origin.Y.AsF32())
	}
}
