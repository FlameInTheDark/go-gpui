package layout

import (
	"fmt"
	"testing"
)

func TestDiagCenterExplicitHeight(t *testing.T) {
	tree := NewTree()
	// Tab row: 34 tall, align-items center, horizontal main axis.
	row := DefaultStyle()
	ai := AlignCenter
	row.AlignItems = &ai
	row.Size = SizeDimension{Height: LengthDimension(34)}
	root := tree.NewNode(row)
	icon := DefaultStyle()
	icon.Size = SizeDimension{Width: LengthDimension(14), Height: LengthDimension(14)}
	c := tree.NewNode(icon)
	tree.SetChildren(root, []Node{c})
	tree.ComputeLayout(root, DefiniteAvailable(400, 600))
	lc := tree.Layout(c)
	fmt.Printf("icon x=%.2f y=%.2f (want y=10)\n", lc.X, lc.Y)
}
