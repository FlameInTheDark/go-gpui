package layout

import (
	"testing"
)

// TestWorkspaceChromeLayout mirrors the editor workspace structure:
// fixed-height chrome (shrink 0) + a content row whose column holds a
// scrollable list with far more content than fits. The row must shrink to
// the remaining space and the list must scroll inside it.
func TestWorkspaceChromeLayout(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(func() Style {
		s := DefaultStyle()
		s.Display = DisplayFlex
		s.FlexDirection = FlexColumn
		return s
	}())

	menu := tree.NewNode(fixedSize(1000, 28))
	tree.Style(menu).FlexShrink = 0

	tab := tree.NewNode(fixedSize(1000, 34))
	tree.Style(tab).FlexShrink = 0

	row := tree.NewNode(func() Style {
		s := DefaultStyle()
		s.Display = DisplayFlex
		s.FlexDirection = FlexRow
		return s
	}())
	tree.Style(row).FlexGrow = 1

	col := tree.NewNode(func() Style {
		s := DefaultStyle()
		s.Display = DisplayFlex
		s.FlexDirection = FlexColumn
		s.Size.Width = LengthDimension(240)
		return s
	}())

	header := tree.NewNode(fixedSize(240, 30))
	tree.Style(header).FlexShrink = 0
	list := tree.NewNode(func() Style {
		s := DefaultStyle()
		s.Display = DisplayFlex
		s.FlexDirection = FlexColumn
		s.Overflow[1] = OverflowScroll
		return s
	}())
	tree.Style(list).FlexGrow = 1
	// 43 rows of 24px.
	for i := 0; i < 43; i++ {
		r := tree.NewNode(fixedSize(239, 24))
		tree.SetChildren(list, append(tree.Children(list), r))
	}
	tree.SetChildren(col, []Node{header, list})
	tree.SetChildren(row, []Node{col})

	status := tree.NewNode(fixedSize(1000, 26))
	tree.Style(status).FlexShrink = 0

	tree.SetChildren(root, []Node{menu, tab, row, status})
	// The window walk stretches auto-sized roots to the viewport (the app's
	// root is a definite 1000x650 box).
	tree.StretchAutoSizeToFill(root, 1000, 650)
	tree.ComputeLayout(root, DefiniteAvailable(1000, 650))

	lr := tree.Layout(row)
	ll := tree.Layout(list)
	lm := tree.Layout(menu)
	lt := tree.Layout(tab)
	ls := tree.Layout(status)
	t.Logf("menu %.1f tab %.1f row %.1f (y %.1f) status %.1f (y %.1f) list h %.1f (y %.1f, scrollH %.1f)",
		lm.Height, lt.Height, lr.Height, lr.Y, ls.Height, ls.Y, ll.Height, ll.Y, ll.ScrollHeight)

	if got, want := lm.Height, float32(28); got != want {
		t.Fatalf("menu height %.1f, want %.1f", got, want)
	}
	if got, want := lt.Height, float32(34); got != want {
		t.Fatalf("tab height %.1f, want %.1f", got, want)
	}
	if got, want := ls.Height, float32(26); got != want {
		t.Fatalf("status height %.1f, want %.1f", got, want)
	}
	if got, want := ls.Y, float32(624); got != want {
		t.Fatalf("status y %.1f, want %.1f", got, want)
	}
	if got, want := lr.Height, float32(562); got != want {
		t.Fatalf("row height %.1f, want %.1f", got, want)
	}
	if got, want := ll.Height, float32(532); got != want {
		t.Fatalf("list height %.1f, want %.1f (scrollable content)", got, want)
	}
	if ll.Height > 0 && ll.ScrollHeight <= ll.Height {
		t.Fatalf("list scroll height %.1f should exceed viewport %.1f", ll.ScrollHeight, ll.Height)
	}
}
