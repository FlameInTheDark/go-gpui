package layout

import "testing"

// Replicates the popup's exact style chain in the raw engine: column panel
// 260x60, child row = display flex + align-items center + height 26 +
// margin lr 4, with a 140px measured-ish leaf grandchild.
func TestStretchItemMarginsNotDoubleCounted(t *testing.T) {
	tree := NewTree()
	panel := DefaultStyle()
	panel.FlexDirection = FlexColumn
	panel.Size = SizeDimension{Width: LengthDimension(260), Height: LengthDimension(60)}
	root := tree.NewNode(panel)

	row := DefaultStyle() // DisplayFlex is the default (1)
	ai := AlignStretch
	row.AlignItems = &ai
	row.Size = SizeDimension{Height: LengthDimension(26)}
	row.Margin = EdgeDimension{Top: LengthDimension(0), Right: LengthDimension(4), Bottom: LengthDimension(0), Left: LengthDimension(4)}
	r := tree.NewNode(row)

	leaf := DefaultStyle()
	leaf.Size = SizeDimension{Width: LengthDimension(140), Height: LengthDimension(16)}
	l := tree.NewNode(leaf)
	tree.SetChildren(r, []Node{l})
	tree.SetChildren(root, []Node{r})
	tree.ComputeLayout(root, DefiniteAvailable(400, 600))
	lr := tree.Layout(r)
	// The stretch-aligned row fills the panel's content box minus its own
	// margins: x = margin-left, w = 260 - 4 - 4. Before the fix the line
	// size was inflated by the margins (fill + margins), so the row was
	// laid out 260 wide at x=4 - overflowing the 260px panel.
	approxF(t, "row.x", lr.X, 4)
	approxF(t, "row.w", lr.Width, 252)
}
