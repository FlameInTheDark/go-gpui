package layout

import (
	"fmt"
	"testing"
)

var debugRowSizes string

func TestGridDebugRowMajor(t *testing.T) {
	gridDebug = true
	defer func() { gridDebug = false }()
	tree := NewTree()
	root := tree.NewNode(gridStyle(
		[]GridSection{CountGridSection(2, PxGridTrack(80))},
		nil,
	))
	tree.Style(root).Size = SizeDimension{Width: LengthDimension(160)}
	var children []Node
	for i := 0; i < 4; i++ {
		children = append(children, tree.NewNode(sizedLeaf(30, 20)))
	}
	tree.SetChildren(root, children)
	tree.ComputeLayout(root, DefiniteAvailable(160, 100))
	for i, n := range children {
		l := tree.Layout(n)
		fmt.Printf("child %d: x=%v y=%v w=%v h=%v\n", i, l.X, l.Y, l.Width, l.Height)
	}
	// print raw slice values via fmt
	fmt.Println("rowSizes?", debugRowSizes)
}
