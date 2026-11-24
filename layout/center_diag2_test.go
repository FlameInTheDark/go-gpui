package layout

import (
	"fmt"
	"testing"
)

func TestDiagCenterThreeLevels(t *testing.T) {
	tree := NewTree()
	// Tab bar: stretch items, 34 tall.
	bar := DefaultStyle()
	sa := AlignStretch
	bar.AlignItems = &sa
	bar.Size = SizeDimension{Height: LengthDimension(34)}
	b := tree.NewNode(bar)
	// Tab: center items, padding 10.
	tab := DefaultStyle()
	ca := AlignCenter
	tab.AlignItems = &ca
	tab.Padding = EdgeDimension{Top: LengthDimension(0), Right: LengthDimension(10), Bottom: LengthDimension(0), Left: LengthDimension(10)}
	tn := tree.NewNode(tab)
	// Icon 14x14.
	icon := DefaultStyle()
	icon.Size = SizeDimension{Width: LengthDimension(14), Height: LengthDimension(14)}
	c := tree.NewNode(icon)
	tree.SetChildren(tn, []Node{c})
	tree.SetChildren(b, []Node{tn})
	tree.ComputeLayout(b, DefiniteAvailable(400, 600))
	lc := tree.Layout(c)
	la := tree.AbsoluteBounds(c)
	fmt.Printf("icon rel x=%.2f y=%.2f | abs x=%.2f y=%.2f (want abs y=38)\n", lc.X, lc.Y, la.X, la.Y)
}
