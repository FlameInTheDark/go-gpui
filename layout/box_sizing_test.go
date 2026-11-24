package layout

import (
	"testing"
)

// TestDefiniteWidthBoxSizing pins the box-sizing contract the workspace's
// scroll containers rely on: a definite style Width is the BORDER box (the
// laid-out width, padding inside), and a scroll container's scroll extent
// equals the children's style widths (so definite-width rows wider than the
// viewport produce horizontal scroll range).
func TestDefiniteWidthBoxSizing(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(fixedSize(100, 50))
	tree.Style(root).FlexDirection = FlexColumn

	inner := DefaultStyle()
	inner.Size = SizeDimension{Width: LengthDimension(80), Height: LengthDimension(20)}
	inner.Padding = EdgeDimension{Left: LengthDimension(5), Right: LengthDimension(7)}
	child := tree.NewNode(inner)
	tree.SetChildren(root, []Node{child})

	tree.ComputeLayout(root, DefiniteAvailable(200, 300))
	cl := tree.Layout(child)
	// Border box: the style width is the whole box, padding inside.
	if cl.Width != 80 {
		t.Fatalf("definite width is border box: got w=%.1f want 80", cl.Width)
	}

	// A wider child defines the container's horizontal scroll extent.
	wide := DefaultStyle()
	wide.Size = SizeDimension{Width: LengthDimension(150), Height: LengthDimension(20)}
	wide.Padding = EdgeDimension{Left: LengthDimension(5), Right: LengthDimension(7)}
	child2 := tree.NewNode(wide)
	tree.SetChildren(root, []Node{child, child2})
	tree.ComputeLayout(root, DefiniteAvailable(200, 300))
	if sw := tree.Layout(root).ScrollWidth; sw != 150 {
		t.Fatalf("scroll extent should equal the widest child's border box: got %.1f want 150", sw)
	}
}
