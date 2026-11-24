package layout

import (
	"math"
	"testing"
)

// Regression (settings panel): a flex row whose text column wraps must grow
// to fit the wrapped lines, and the wrapped leaf's stored layout must match
// the FINAL bounded context, not an earlier max-content probe.
//
// Two framework bugs hid behind one symptom ("description text is
// overlapping"):
//
//  1. contentSize measured container items at max-content, so the row's
//     flex basis missed the wrapped lines — the row kept its minimum height
//     and the second text line painted over the next row's border.
//  2. The probe cache returned memoized subtree computes, while a later
//     max-content probe rewrote the leaf's stored layout with unwrapped
//     bounds; the final bounded compute — a probe-cache hit — left that
//     stale layout in place and the text painted as one clipped line under
//     the control. Commit mode (layoutInput.commit) now rewrites every
//     descendant's stored layout on the final pass.
func TestWrappedRowGrowsAndCommitsFinalLayout(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(DefaultStyle())

	// The row: auto height with a minimum, definite width from the root.
	row := tree.NewNode(func() Style {
		s := DefaultStyle()
		s.Display = DisplayFlex
		s.MinSize.Height = LengthDimension(40)
		return s
	}())

	// The label column stacks a label leaf above the wrapping description.
	col := tree.NewNode(func() Style {
		s := DefaultStyle()
		s.Display = DisplayFlex
		s.FlexDirection = FlexColumn
		return s
	}())
	label := tree.NewNode(fixedSize(40, 24))

	// The description: 270px of text at a 14px line height, wrapping to the
	// available width exactly like a shaped text element.
	desc := tree.NewNode(DefaultStyle())
	tree.SetMeasure(desc, func(known SizeDimension, available AvailableSpace) SizeResult {
		wrapW := float32(10000)
		if v, ok := known.Width.Resolve(available.Width); ok {
			wrapW = v
		} else if v, ok := available.IsDefiniteAxis(true); ok {
			wrapW = v
		}
		const total = 270
		lines := int((total + wrapW - 1) / wrapW)
		if lines < 1 {
			lines = 1
		}
		return SizeResult{Width: float32(math.Min(float64(total), float64(wrapW))), Height: float32(lines) * 14}
	})
	tree.SetChildren(col, []Node{label, desc})

	// A 240px fixed control right of the column, with a 14px gap.
	control := tree.NewNode(fixedSize(240, 26))
	tree.Style(row).Gap = SizeDimension{Width: LengthDimension(14)}
	tree.SetChildren(row, []Node{col, control})
	tree.SetChildren(root, []Node{row})

	// Root 600px wide: the column gets 600-240-14 = 346 < 270? No — the
	// text measures 270 wide, so it fits unwrapped... at max-content. The
	// row is THEN shrunk to the 500px root: the column narrows to 246 and
	// the text must rewrap to two lines.
	tree.ComputeLayout(root, DefiniteAvailable(500, 500))

	// The row grew past its 40px minimum to hold the wrapped description.
	rowH := tree.Layout(row).Height
	if rowH < 50 {
		t.Fatalf("row did not grow to fit the wrapped text: height %.2f (min 40, two 24px+wrapped lines need ~52)", rowH)
	}

	// The leaf's STORED layout must be the final bounded shape: the column
	// lands below the text's 270px max-content (the row shrank from 524 to
	// its 500px container), so the description reports two 14px lines.
	dl := tree.Layout(desc)
	if dl.Height != 28 {
		t.Fatalf("description did not wrap in its final layout: height %.2f (two 14px lines expected)", dl.Height)
	}

	// ...and the column stretches its children to its final width — the
	// description's box must match the column, not the 270px max-content
	// probe a measurement elsewhere in the tree left behind.
	colW := tree.Layout(col).Width
	if dl.Width != colW {
		t.Fatalf("description box %.2f does not match its column %.2f (stale probe layout)", dl.Width, colW)
	}
	if colW >= 270 {
		t.Fatalf("column did not shrink below the text's max-content: %.2f", colW)
	}
}
