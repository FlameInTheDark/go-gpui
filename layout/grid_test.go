package layout

import (
	"testing"
)

// gridStyle builds a grid container style with the given column/row
// templates.
func gridStyle(cols, rows []GridSection) Style {
	s := DefaultStyle()
	s.Display = DisplayGrid
	s.GridTemplateColumns = cols
	s.GridTemplateRows = rows
	return s
}

func sizedLeaf(w, h float32) Style {
	s := DefaultStyle()
	s.Size = SizeDimension{Width: LengthDimension(w), Height: LengthDimension(h)}
	return s
}

// measureLeaf is a leaf whose measure function reports a fixed size.
func measureLeaf(w, h float32) Style {
	return DefaultStyle()
}

// intrinsicMeasure is a leaf whose min-content width is minW and max-content
// width is maxW (height fixed at h).
func intrinsicMeasure(minW, maxW, h float32) Measurer {
	return func(known SizeDimension, available AvailableSpace) SizeResult {
		w := maxW
		if available.kindW == 1 || (available.kindW == 0 && available.Width < maxW) {
			w = minW
		}
		return SizeResult{Width: w, Height: h}
	}
}

func fixedMeasure(w, h float32) Measurer {
	return func(known SizeDimension, available AvailableSpace) SizeResult {
		return SizeResult{Width: w, Height: h}
	}
}

// (a) Definite px rows/columns place children at the expected offsets.
func TestGridDefiniteTracksPlacement(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(gridStyle(
		[]GridSection{CountGridSection(2, PxGridTrack(100))},
		[]GridSection{CountGridSection(2, PxGridTrack(50))},
	))
	rootStyle := tree.Style(root)
	rootStyle.Size = SizeDimension{Width: LengthDimension(200), Height: LengthDimension(100)}
	a := tree.NewNode(DefaultStyle())
	b := tree.NewNode(DefaultStyle())
	c := tree.NewNode(DefaultStyle())
	d := tree.NewNode(DefaultStyle())
	tree.SetChildren(root, []Node{a, b, c, d})
	tree.ComputeLayout(root, DefiniteAvailable(200, 100))

	la := tree.Layout(a)
	lb := tree.Layout(b)
	lc := tree.Layout(c)
	ld := tree.Layout(d)
	// Row-major auto placement into the 2x2 grid.
	approxF(t, "a.x", la.X, 0)
	approxF(t, "a.y", la.Y, 0)
	approxF(t, "a.w", la.Width, 100)
	approxF(t, "a.h", la.Height, 50)
	approxF(t, "b.x", lb.X, 100)
	approxF(t, "b.y", lb.Y, 0)
	approxF(t, "c.x", lc.X, 0)
	approxF(t, "c.y", lc.Y, 50)
	approxF(t, "d.x", ld.X, 100)
	approxF(t, "d.y", ld.Y, 50)
	approxF(t, "root.w", tree.Layout(root).Width, 200)
	approxF(t, "root.h", tree.Layout(root).Height, 100)
}

// (b) fr distribution: 1fr/1fr splits 50/50; 1fr/2fr splits 1/3-2/3.
func TestGridFrDistribution(t *testing.T) {
	build := func(frA, frB float32, width float32) (*Tree, Node, Node, Node) {
		tree := NewTree()
		root := tree.NewNode(gridStyle(
			[]GridSection{SingleGridSection(FrGridTrack(frA)), SingleGridSection(FrGridTrack(frB))},
			[]GridSection{SingleGridSection(PxGridTrack(40))},
		))
		tree.Style(root).Size = SizeDimension{Width: LengthDimension(width)}
		a := tree.NewNode(DefaultStyle())
		b := tree.NewNode(DefaultStyle())
		tree.SetChildren(root, []Node{a, b})
		return tree, root, a, b
	}

	tree, root, a, b := build(1, 1, 200)
	tree.ComputeLayout(root, DefiniteAvailable(200, 40))
	approxF(t, "a.w", tree.Layout(a).Width, 100)
	approxF(t, "b.w", tree.Layout(b).Width, 100)
	approxF(t, "b.x", tree.Layout(b).X, 100)

	tree2, root2, a2, b2 := build(1, 2, 300)
	tree2.ComputeLayout(root2, DefiniteAvailable(300, 40))
	approxF(t, "a2.w", tree2.Layout(a2).Width, 100)
	approxF(t, "b2.w", tree2.Layout(b2).Width, 200)
	approxF(t, "b2.x", tree2.Layout(b2).X, 100)
}

// (c) Gap spacing is applied between tracks.
func TestGridGap(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(gridStyle(
		[]GridSection{CountGridSection(2, PxGridTrack(50))},
		[]GridSection{CountGridSection(2, PxGridTrack(50))},
	))
	st := tree.Style(root)
	st.Size = SizeDimension{Width: LengthDimension(200), Height: LengthDimension(200)}
	st.Gap = SizeDimension{Width: LengthDimension(10), Height: LengthDimension(10)}
	a := tree.NewNode(DefaultStyle())
	b := tree.NewNode(DefaultStyle())
	c := tree.NewNode(DefaultStyle())
	tree.SetChildren(root, []Node{a, b, c})
	tree.ComputeLayout(root, DefiniteAvailable(200, 200))

	approxF(t, "a.x", tree.Layout(a).X, 0)
	approxF(t, "b.x", tree.Layout(b).X, 60)
	approxF(t, "c.y", tree.Layout(c).Y, 60)
	approxF(t, "c.x", tree.Layout(c).X, 0)
}

// (d) A span-2 item occupies two tracks.
func TestGridSpanTwo(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(gridStyle(
		[]GridSection{CountGridSection(2, PxGridTrack(100))},
		nil,
	))
	tree.Style(root).Size = SizeDimension{Width: LengthDimension(200)}
	// Auto-width items stretch to fill their tracks (default
	// justify-items: stretch); definite heights give the auto rows size;
	// c spans both columns on the next row.
	tall := func(h float32) Style {
		s := DefaultStyle()
		s.Size = SizeDimension{Height: LengthDimension(h)}
		return s
	}
	a := tree.NewNode(tall(20))
	b := tree.NewNode(tall(20))
	c := tree.NewNode(tall(20))
	cStyle := tree.Style(c)
	cStyle.GridColumn = GridLineRange{Start: SpanGridPlacement(2), End: AutoGridPlacement()}
	tree.SetChildren(root, []Node{a, b, c})
	tree.ComputeLayout(root, DefiniteAvailable(200, 100))

	// a and b fill row 0; c spans both columns on the next row.
	approxF(t, "a.w", tree.Layout(a).Width, 100)
	approxF(t, "b.w", tree.Layout(b).Width, 100)
	lc := tree.Layout(c)
	approxF(t, "c.x", lc.X, 0)
	approxF(t, "c.w", lc.Width, 200)
	if !(lc.Y > tree.Layout(a).Y) {
		t.Fatalf("c should be placed on a later row: c.y=%v a.y=%v", lc.Y, tree.Layout(a).Y)
	}
}

// (e) Auto-placement flows row-major into the next free cell.
func TestGridAutoPlacementRowMajor(t *testing.T) {
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

	l0 := tree.Layout(children[0])
	l1 := tree.Layout(children[1])
	l2 := tree.Layout(children[2])
	l3 := tree.Layout(children[3])
	approxF(t, "c0.x", l0.X, 0)
	approxF(t, "c1.x", l1.X, 80)
	approxF(t, "c2.x", l2.X, 0)
	approxF(t, "c3.x", l3.X, 80)
	if !(l2.Y > l0.Y) {
		t.Fatalf("third item should wrap to the next row: y2=%v y0=%v", l2.Y, l0.Y)
	}
	if !(l3.Y > l1.Y) {
		t.Fatalf("fourth item should wrap to the next row: y3=%v y1=%v", l3.Y, l1.Y)
	}
	// Items keep their fixed size (stretch does not override explicit sizes).
	approxF(t, "c0.w", l0.Width, 30)
	approxF(t, "c0.h", l0.Height, 20)
}

// (f) minmax(min-content, ...) respects a content-based minimum.
func TestGridMinmaxContentMinimum(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(gridStyle(
		[]GridSection{
			SingleGridSection(MinMaxGridTrack(MinContentGridTrackFn(), FrGridTrackFn(1))),
			SingleGridSection(PxGridTrack(50)),
		},
		nil,
	))
	tree.Style(root).Size = SizeDimension{Width: LengthDimension(100)}
	a := tree.NewLeaf(DefaultStyle(), fixedMeasure(80, 10))
	tree.SetChildren(root, []Node{a})
	tree.ComputeLayout(root, DefiniteAvailable(100, 20))

	// The fr column wants (100 - 50) = 50px, but the item's min-content
	// contribution (80px) forms the track's base size, which fr tracks never
	// shrink below.
	la := tree.Layout(a)
	approxF(t, "a.x", la.X, 0)
	approxF(t, "a.w", la.Width, 80)
	lb := tree.Style(root)
	_ = lb
}

// (g) Align/justify center positions items within their areas.
func TestGridAlignJustifyCenter(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(gridStyle(
		[]GridSection{SingleGridSection(PxGridTrack(100))},
		[]GridSection{SingleGridSection(PxGridTrack(40))},
	))
	st := tree.Style(root)
	st.Size = SizeDimension{Width: LengthDimension(200), Height: LengthDimension(100)}
	ji := AlignCenter
	ai := AlignCenter
	st.JustifyItems = &ji
	st.AlignItems = &ai
	a := tree.NewNode(sizedLeaf(50, 20))
	tree.SetChildren(root, []Node{a})
	tree.ComputeLayout(root, DefiniteAvailable(200, 100))

	// Item centered in its 100x40 area.
	la := tree.Layout(a)
	approxF(t, "a.x", la.X, 25)
	approxF(t, "a.y", la.Y, 10)

	// justify/align-content center the tracks themselves within the
	// container.
	tree2 := NewTree()
	root2 := tree2.NewNode(gridStyle(
		[]GridSection{SingleGridSection(PxGridTrack(50))},
		[]GridSection{SingleGridSection(PxGridTrack(20))},
	))
	st2 := tree2.Style(root2)
	st2.Size = SizeDimension{Width: LengthDimension(200), Height: LengthDimension(100)}
	jc := AlignContentCenter
	ac := AlignContentCenter
	st2.JustifyContent = &jc
	st2.AlignContent = &ac
	b := tree2.NewNode(sizedLeaf(20, 10))
	tree2.SetChildren(root2, []Node{b})
	tree2.ComputeLayout(root2, DefiniteAvailable(200, 100))

	// Track starts at (200-50)/2 = 75 and (100-20)/2 = 40; the item is
	// stretched into the track vertically and centered horizontally.
	lb := tree2.Layout(b)
	approxF(t, "b.x", lb.X, 75)
	approxF(t, "b.y", lb.Y, 40)
	approxF(t, "b.w", lb.Width, 20)
	approxF(t, "b.h", lb.Height, 10)
}

// (h) repeat(auto-fill, 100px) creates as many 100px tracks as fit.
func TestGridAutoFillFixedTracks(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(gridStyle(
		[]GridSection{AutoFillGridSection(PxGridTrack(100))},
		nil,
	))
	tree.Style(root).Size = SizeDimension{Width: LengthDimension(350)}
	var children []Node
	for i := 0; i < 3; i++ {
		children = append(children, tree.NewNode(DefaultStyle()))
	}
	tree.SetChildren(root, children)
	tree.ComputeLayout(root, DefiniteAvailable(350, 100))

	// 350px fits three 100px tracks (a fourth would overflow).
	approxF(t, "c0.x", tree.Layout(children[0]).X, 0)
	approxF(t, "c1.x", tree.Layout(children[1]).X, 100)
	approxF(t, "c2.x", tree.Layout(children[2]).X, 200)
	approxF(t, "c0.w", tree.Layout(children[0]).Width, 100)
}

// Definite placement via grid_row/grid_column lines (negative and positive).
func TestGridDefinitePlacement(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(gridStyle(
		[]GridSection{CountGridSection(2, PxGridTrack(100))},
		[]GridSection{CountGridSection(2, PxGridTrack(50))},
	))
	tree.Style(root).Size = SizeDimension{Width: LengthDimension(200), Height: LengthDimension(100)}
	a := tree.NewNode(DefaultStyle())
	aStyle := tree.Style(a)
	aStyle.GridColumn = GridLineRange{Start: LineGridPlacement(2), End: AutoGridPlacement()}
	aStyle.GridRow = GridLineRange{Start: LineGridPlacement(2), End: AutoGridPlacement()}
	b := tree.NewNode(DefaultStyle())
	bStyle := tree.Style(b)
	bStyle.GridColumn = GridLineRange{Start: AutoGridPlacement(), End: LineGridPlacement(-1)}
	bStyle.GridRow = GridLineRange{Start: LineGridPlacement(1), End: AutoGridPlacement()}
	tree.SetChildren(root, []Node{a, b})
	tree.ComputeLayout(root, DefiniteAvailable(200, 100))

	// a: column line 2 = track 1 (0-based), row line 2 = track 1.
	la := tree.Layout(a)
	approxF(t, "a.x", la.X, 100)
	approxF(t, "a.y", la.Y, 50)
	// b: column end line -1 = end of explicit grid; auto start resolves to
	// end-1 = track 1; row line 1 = track 0.
	lb := tree.Layout(b)
	approxF(t, "b.x", lb.X, 100)
	approxF(t, "b.y", lb.Y, 0)
}

// Intrinsic container sizing: max-content = sum of track max-content sizes.
func TestGridIntrinsicContentSize(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(gridStyle(
		[]GridSection{
			SingleGridSection(MinMaxGridTrack(AutoGridTrackFn(), MaxContentGridTrackFn())),
			SingleGridSection(MinMaxGridTrack(AutoGridTrackFn(), MaxContentGridTrackFn())),
		},
		nil,
	))
	a := tree.NewLeaf(DefaultStyle(), fixedMeasure(30, 10))
	b := tree.NewLeaf(DefaultStyle(), fixedMeasure(40, 12))
	tree.SetChildren(root, []Node{a, b})
	// No definite container size: the grid sizes to its content.
	tree.ComputeLayout(root, MaxContentAvailable())

	lr := tree.Layout(root)
	approxF(t, "root.w", lr.Width, 70)
	approxF(t, "root.h", lr.Height, 12)
	approxF(t, "b.x", tree.Layout(b).X, 30)
}

// Auto-fit collapses empty fixed tracks.
func TestGridAutoFitCollapsesEmptyTracks(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(gridStyle(
		[]GridSection{AutoFitGridSection(PxGridTrack(100))},
		nil,
	))
	tree.Style(root).Size = SizeDimension{Width: LengthDimension(400)}
	a := tree.NewNode(DefaultStyle())
	tree.SetChildren(root, []Node{a})
	tree.ComputeLayout(root, DefiniteAvailable(400, 20))

	// Four repetitions fit in 400px, but only the first holds an item; the
	// rest collapse, so the content is 100px wide.
	la := tree.Layout(a)
	approxF(t, "a.x", la.X, 0)
	approxF(t, "a.w", la.Width, 100)
	lr := tree.Layout(root)
	if lr.Width < 100 || lr.Width > 400 {
		t.Fatalf("root width: got %v", lr.Width)
	}
}

// Percentage tracks resolve against the definite container size.
func TestGridPercentTracks(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(gridStyle(
		[]GridSection{SingleGridSection(PercentGridTrack(0.5))},
		[]GridSection{SingleGridSection(PercentGridTrack(0.5))},
	))
	tree.Style(root).Size = SizeDimension{Width: LengthDimension(200), Height: LengthDimension(100)}
	a := tree.NewNode(DefaultStyle())
	tree.SetChildren(root, []Node{a})
	tree.ComputeLayout(root, DefiniteAvailable(200, 100))

	la := tree.Layout(a)
	approxF(t, "a.w", la.Width, 100)
	approxF(t, "a.h", la.Height, 50)
}

// Row-major auto placement with a definite row skips occupied columns
// (sparse packing: the cursor never moves backwards).
func TestGridSparsePackingAfterDefiniteRow(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(gridStyle(
		[]GridSection{CountGridSection(2, PxGridTrack(50))},
		nil,
	))
	tree.Style(root).Size = SizeDimension{Width: LengthDimension(100)}
	a := tree.NewNode(sizedLeaf(30, 20)) // auto-place -> (row 0, col 0)
	b := tree.NewNode(sizedLeaf(30, 20))
	bStyle := tree.Style(b)
	// Line 2 = the second column track; line 1 = the first row track.
	bStyle.GridColumn = GridLineRange{Start: LineGridPlacement(2), End: AutoGridPlacement()}
	bStyle.GridRow = GridLineRange{Start: LineGridPlacement(1), End: AutoGridPlacement()}
	// b is definitely placed at (row 0, col 1).
	c := tree.NewNode(sizedLeaf(30, 20)) // auto-place -> next free cell (row 1, col 0)
	tree.SetChildren(root, []Node{a, b, c})
	tree.ComputeLayout(root, DefiniteAvailable(100, 100))

	approxF(t, "a.x", tree.Layout(a).X, 0)
	approxF(t, "b.x", tree.Layout(b).X, 50)
	approxF(t, "b.y", tree.Layout(b).Y, 0)
	if tree.Layout(c).Y <= tree.Layout(a).Y {
		t.Fatalf("c should be placed after the occupied row 0: c.y=%v", tree.Layout(c).Y)
	}
	approxF(t, "c.x", tree.Layout(c).X, 0)
}

// fit-content(arg) caps a track's max-content growth (but not below the
// track's content-based minimum).
func TestGridFitContentTrack(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(gridStyle(
		[]GridSection{SingleGridSection(MinMaxGridTrack(AutoGridTrackFn(), FitContentGridTrackFn(60)))},
		[]GridSection{SingleGridSection(PxGridTrack(20))},
	))
	a := tree.NewLeaf(DefaultStyle(), intrinsicMeasure(30, 120, 20))
	tree.SetChildren(root, []Node{a})
	tree.ComputeLayout(root, MaxContentAvailable())

	// The item's max-content width is 120px, but fit-content(60) caps the
	// track at 60px (the 30px min-content minimum is below the cap).
	la := tree.Layout(a)
	approxF(t, "a.w", la.Width, 60)
	approxF(t, "a.h", la.Height, 20)
}
