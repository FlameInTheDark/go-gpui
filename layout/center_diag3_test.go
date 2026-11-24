package layout

import (
	"fmt"
	"testing"
)

func TestDiagCenterFullApp(t *testing.T) {
	tree := NewTree()
	// Root column: [menubar 28, tabbar 34].
	rootS := DefaultStyle()
	rootS.FlexDirection = FlexColumn
	rs := tree.NewNode(rootS)

	barS := DefaultStyle()
	barS.Size = SizeDimension{Height: LengthDimension(28)}
	bn := tree.NewNode(barS)

	tabbarS := DefaultStyle()
	ta := AlignStretch
	tabbarS.AlignItems = &ta
	tabbarS.Size = SizeDimension{Height: LengthDimension(34)}
	tb := tree.NewNode(tabbarS)

	tabS := DefaultStyle()
	ca := AlignCenter
	tabS.AlignItems = &ca
	tabS.Padding = EdgeDimension{Top: LengthDimension(0), Right: LengthDimension(10), Bottom: LengthDimension(0), Left: LengthDimension(10)}
	tn := tree.NewNode(tabS)

	iconS := DefaultStyle()
	iconS.Size = SizeDimension{Width: LengthDimension(14), Height: LengthDimension(14)}
	in := tree.NewNode(iconS)

	tree.SetChildren(tn, []Node{in})
	tree.SetChildren(tb, []Node{tn})
	tree.SetChildren(rs, []Node{bn, tb})
	tree.ComputeLayout(rs, DefiniteAvailable(400, 600))
	la := tree.AbsoluteBounds(in)
	fmt.Printf("icon abs x=%.2f y=%.2f (want x=10 y=38)\n", la.X, la.Y)
}
