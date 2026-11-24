package layout

import (
	"math"
	"testing"
)

func approxF(t *testing.T, name string, got, want float32) {
	t.Helper()
	d := math.Abs(float64(got - want))
	if d > 0.01 {
		t.Fatalf("%s: got %v want %v", name, got, want)
	}
}

func fixedSize(w, h float32) Style {
	s := DefaultStyle()
	s.Size = SizeDimension{Width: LengthDimension(w), Height: LengthDimension(h)}
	return s
}

func TestFlexRowLaysOutChildren(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(DefaultStyle())
	a := tree.NewNode(fixedSize(50, 100))
	b := tree.NewNode(fixedSize(30, 100))
	tree.SetChildren(root, []Node{a, b})
	tree.ComputeLayout(root, DefiniteAvailable(200, 300))

	la := tree.Layout(a)
	approxF(t, "a.x", la.X, 0)
	approxF(t, "a.w", la.Width, 50)
	lb := tree.Layout(b)
	approxF(t, "b.x", lb.X, 50)
	approxF(t, "b.w", lb.Width, 30)
	// Container height stretches to child height.
	approxF(t, "root.h", tree.Layout(root).Height, 100)
}

func TestFlexColumn(t *testing.T) {
	tree := NewTree()
	s := DefaultStyle()
	s.FlexDirection = FlexColumn
	root := tree.NewNode(s)
	a := tree.NewNode(fixedSize(100, 40))
	b := tree.NewNode(fixedSize(100, 60))
	tree.SetChildren(root, []Node{a, b})
	tree.ComputeLayout(root, DefiniteAvailable(200, 300))

	la := tree.Layout(a)
	lb := tree.Layout(b)
	approxF(t, "a.y", la.Y, 0)
	approxF(t, "b.y", lb.Y, 40)
	approxF(t, "root.h", tree.Layout(root).Height, 100)
	// Taffy root rule: auto width defaults to the available width.
	approxF(t, "root.w", tree.Layout(root).Width, 200)
}

func TestJustifySpaceBetween(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(DefaultStyle())
	a := tree.NewNode(fixedSize(50, 10))
	b := tree.NewNode(fixedSize(50, 10))
	tree.SetChildren(root, []Node{a, b})
	st := tree.Style(root)
	ac := AlignContentSpaceBetween
	st.JustifyContent = &ac
	tree.ComputeLayout(root, DefiniteAvailable(200, 10))

	approxF(t, "a.x", tree.Layout(a).X, 0)
	approxF(t, "b.x", tree.Layout(b).X, 150)
}

func TestJustifyCenter(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(DefaultStyle())
	a := tree.NewNode(fixedSize(50, 10))
	tree.SetChildren(root, []Node{a})
	st := tree.Style(root)
	ac := AlignContentCenter
	st.JustifyContent = &ac
	tree.ComputeLayout(root, DefiniteAvailable(200, 10))
	approxF(t, "a.x", tree.Layout(a).X, 75)
}

func TestFlexGrow(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(DefaultStyle())
	a := tree.NewNode(fixedSize(50, 10))
	b := tree.NewNode(fixedSize(50, 10))
	tree.SetChildren(root, []Node{a, b})
	sa := tree.Style(a)
	sa.FlexGrow = 1
	sb := tree.Style(b)
	sb.FlexGrow = 3
	tree.ComputeLayout(root, DefiniteAvailable(200, 10))

	// Free space 100 split 1:3.
	approxF(t, "a.w", tree.Layout(a).Width, 75)
	approxF(t, "b.w", tree.Layout(b).Width, 125)
}

func TestFlexShrink(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(DefaultStyle())
	a := tree.NewNode(fixedSize(100, 10))
	b := tree.NewNode(fixedSize(100, 10))
	tree.SetChildren(root, []Node{a, b})
	// Default shrink 1 each; 200 of content into 100.
	tree.ComputeLayout(root, DefiniteAvailable(100, 10))
	approxF(t, "a.w", tree.Layout(a).Width, 50)
	approxF(t, "b.w", tree.Layout(b).Width, 50)

	// Shrink 2:1.
	tree2 := NewTree()
	root2 := tree2.NewNode(DefaultStyle())
	x := tree2.NewNode(fixedSize(100, 10))
	y := tree2.NewNode(fixedSize(100, 10))
	tree2.SetChildren(root2, []Node{x, y})
	tree2.Style(x).FlexShrink = 2
	tree2.Style(y).FlexShrink = 1
	tree2.ComputeLayout(root2, DefiniteAvailable(100, 10))
	approxF(t, "x.w", tree2.Layout(x).Width, 33.333)
	approxF(t, "y.w", tree2.Layout(y).Width, 66.666)
}

func TestFlexNoShrink(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(DefaultStyle())
	a := tree.NewNode(fixedSize(150, 10))
	tree.SetChildren(root, []Node{a})
	tree.Style(a).FlexShrink = 0
	tree.ComputeLayout(root, DefiniteAvailable(100, 10))
	approxF(t, "a.w", tree.Layout(a).Width, 150)
	approxF(t, "root.scroll", tree.Layout(root).ScrollWidth, 150)
}

func TestItemsCenter(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(DefaultStyle())
	a := tree.NewNode(fixedSize(50, 40))
	tree.SetChildren(root, []Node{a})
	st := tree.Style(root)
	st.Size = SizeDimension{Width: LengthDimension(200), Height: LengthDimension(100)}
	al := AlignCenter
	st.AlignItems = &al
	tree.ComputeLayout(root, DefiniteAvailable(200, 100))
	approxF(t, "a.y", tree.Layout(a).Y, 30)
}

func TestItemsStretch(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(DefaultStyle())
	a := tree.NewNode(fixedSize(50, 0))
	// Auto height on the child: stretch to the line height.
	sa := tree.Style(a)
	sa.Size = SizeDimension{Width: LengthDimension(50), Height: AutoDimension()}
	tree.SetChildren(root, []Node{a})
	tree.Style(root).Size = SizeDimension{Width: LengthDimension(200), Height: LengthDimension(100)}
	tree.ComputeLayout(root, DefiniteAvailable(200, 100))
	approxF(t, "a.h", tree.Layout(a).Height, 100)
}

func TestGap(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(DefaultStyle())
	a := tree.NewNode(fixedSize(10, 10))
	b := tree.NewNode(fixedSize(10, 10))
	c := tree.NewNode(fixedSize(10, 10))
	tree.SetChildren(root, []Node{a, b, c})
	tree.Style(root).Gap = SizeDimension{Width: LengthDimension(10), Height: LengthDimension(10)}
	tree.ComputeLayout(root, DefiniteAvailable(200, 100))
	approxF(t, "a.x", tree.Layout(a).X, 0)
	approxF(t, "b.x", tree.Layout(b).X, 20)
	approxF(t, "c.x", tree.Layout(c).X, 40)
	// Root auto width defaults to the available width (taffy ICB rule);
	// height is content-based.
	approxF(t, "root.h", tree.Layout(root).Height, 10)
}

func TestFlexWrap(t *testing.T) {
	tree := NewTree()
	s := DefaultStyle()
	s.FlexWrap = FlexWrapOn
	root := tree.NewNode(s)
	a := tree.NewNode(fixedSize(60, 20))
	b := tree.NewNode(fixedSize(60, 20))
	c := tree.NewNode(fixedSize(60, 20))
	tree.SetChildren(root, []Node{a, b, c})
	tree.ComputeLayout(root, DefiniteAvailable(150, 200))

	// a and b on line 1 (60+60 <= 150), c wraps.
	approxF(t, "a.x", tree.Layout(a).X, 0)
	approxF(t, "b.x", tree.Layout(b).X, 60)
	approxF(t, "c.x", tree.Layout(c).X, 0)
	approxF(t, "c.y", tree.Layout(c).Y, 20)
	approxF(t, "root.h", tree.Layout(root).Height, 40)
}

func TestPaddingAndMargin(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(DefaultStyle())
	st := tree.Style(root)
	st.Padding = EdgeDimension{Top: LengthDimension(10), Right: LengthDimension(5), Bottom: LengthDimension(10), Left: LengthDimension(5)}
	tree.ComputeLayout(root, DefiniteAvailable(200, 100))
	// Root width defaults to available (200); height is content-based.
	approxF(t, "root.h", tree.Layout(root).Height, 20)

	// With a child.
	tree2 := NewTree()
	root2 := tree2.NewNode(DefaultStyle())
	child := tree2.NewNode(fixedSize(80, 40))
	tree2.SetChildren(root2, []Node{child})
	tree2.Style(root2).Padding = EdgeDimension{Top: LengthDimension(10), Right: LengthDimension(0), Bottom: LengthDimension(0), Left: LengthDimension(5)}
	tree2.Style(child).Margin = EdgeDimension{Top: LengthDimension(2), Right: LengthDimension(0), Bottom: LengthDimension(0), Left: LengthDimension(3)}
	tree2.ComputeLayout(root2, DefiniteAvailable(200, 200))
	// Height content-based; width is the root default (200).
	approxF(t, "root.h", tree2.Layout(root2).Height, 52)
	approxF(t, "child.x", tree2.Layout(child).X, 8) // 5 pad + 3 margin
	approxF(t, "child.y", tree2.Layout(child).Y, 12)
}

func TestPercentageSizes(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(DefaultStyle())
	a := tree.NewNode(DefaultStyle())
	tree.Style(a).Size = SizeDimension{Width: PercentDimension(0.5), Height: LengthDimension(20)}
	tree.SetChildren(root, []Node{a})
	tree.ComputeLayout(root, DefiniteAvailable(200, 100))
	approxF(t, "a.w", tree.Layout(a).Width, 100)
	approxF(t, "root.w", tree.Layout(root).Width, 200) // root default width
	approxF(t, "root.h", tree.Layout(root).Height, 20)
}

func TestMinMaxConstraints(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(DefaultStyle())
	a := tree.NewNode(DefaultStyle())
	tree.Style(a).Size = SizeDimension{Width: LengthDimension(50), Height: LengthDimension(10)}
	tree.Style(a).MinSize = SizeDimension{Width: LengthDimension(80), Height: AutoDimension()}
	tree.SetChildren(root, []Node{a})
	tree.ComputeLayout(root, DefiniteAvailable(200, 100))
	approxF(t, "a.w", tree.Layout(a).Width, 80)

	tree2 := NewTree()
	root2 := tree2.NewNode(DefaultStyle())
	b := tree2.NewNode(DefaultStyle())
	tree2.Style(b).Size = SizeDimension{Width: LengthDimension(500), Height: LengthDimension(10)}
	tree2.Style(b).MaxSize = SizeDimension{Width: LengthDimension(120), Height: AutoDimension()}
	tree2.SetChildren(root2, []Node{b})
	tree2.ComputeLayout(root2, DefiniteAvailable(200, 100))
	approxF(t, "b.w", tree2.Layout(b).Width, 120)
}

func TestMeasuredLeaf(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(DefaultStyle())
	leaf := tree.NewLeaf(DefaultStyle(), func(known SizeDimension, available AvailableSpace) SizeResult {
		// Text-like: 10px per line of 100px width.
		return SizeResult{Width: 100, Height: 30}
	})
	tree.SetChildren(root, []Node{leaf})
	tree.ComputeLayout(root, DefiniteAvailable(500, 500))
	approxF(t, "leaf.w", tree.Layout(leaf).Width, 100)
	approxF(t, "leaf.h", tree.Layout(leaf).Height, 30)
	approxF(t, "root.w", tree.Layout(root).Width, 500) // root default
	approxF(t, "root.h", tree.Layout(root).Height, 30)
}

func TestResetDiscardsPreviousLayoutAndMeasurements(t *testing.T) {
	tree := NewTree()
	var root, leaf Node
	build := func(width float32, children bool) {
		root = tree.NewNode(DefaultStyle())
		leaf = tree.NewLeaf(DefaultStyle(), func(SizeDimension, AvailableSpace) SizeResult {
			return SizeResult{Width: width, Height: 30}
		})
		if children {
			tree.SetChildren(root, []Node{leaf})
		}
		tree.ComputeLayout(root, DefiniteAvailable(500, 500))
	}
	build(100, true)
	approxF(t, "first leaf width", tree.AbsoluteBounds(leaf).Width, 100)
	tree.Reset()
	if tree.nextNode != 0 || len(tree.absCache)+len(tree.measureCache)+len(tree.probeCache) != 0 {
		t.Fatal("reset left node IDs or cached results from the previous pass")
	}
	if tree.measure[:cap(tree.measure)][leaf] != nil {
		t.Fatal("reset retained the old measure closure")
	}
	build(40, true)
	approxF(t, "new leaf width", tree.AbsoluteBounds(leaf).Width, 40)
	tree.Reset()
	build(0, false)
	if len(tree.Children(root)) != 0 {
		t.Fatal("reset retained children from the previous pass")
	}
}

func TestAbsolutePosition(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(DefaultStyle())
	inset := EdgeDimension{Top: LengthDimension(10), Right: AutoDimension(), Bottom: AutoDimension(), Left: LengthDimension(20)}
	abs := tree.NewNode(fixedSize(30, 30))
	tree.Style(abs).Position = PositionAbsolute
	tree.Style(abs).Inset = inset
	tree.SetChildren(root, []Node{abs})
	// Give the root a definite size.
	tree.Style(root).Size = SizeDimension{Width: LengthDimension(200), Height: LengthDimension(100)}
	tree.ComputeLayout(root, DefiniteAvailable(400, 400))
	la := tree.Layout(abs)
	approxF(t, "abs.x", la.X, 20)
	approxF(t, "abs.y", la.Y, 10)
	approxF(t, "abs.w", la.Width, 30)
}

func TestAbsoluteStretchBothInsets(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(DefaultStyle())
	tree.Style(root).Size = SizeDimension{Width: LengthDimension(200), Height: LengthDimension(100)}
	abs := tree.NewNode(DefaultStyle())
	tree.Style(abs).Position = PositionAbsolute
	tree.Style(abs).Inset = EdgeDimension{
		Top: LengthDimension(10), Right: LengthDimension(20),
		Bottom: LengthDimension(30), Left: LengthDimension(40),
	}
	tree.SetChildren(root, []Node{abs})
	tree.ComputeLayout(root, DefiniteAvailable(400, 400))
	la := tree.Layout(abs)
	approxF(t, "abs.x", la.X, 40)
	approxF(t, "abs.y", la.Y, 10)
	approxF(t, "abs.w", la.Width, 140)
	approxF(t, "abs.h", la.Height, 60)
}

func TestNestedContainers(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(DefaultStyle())
	tree.Style(root).Size = SizeDimension{Width: LengthDimension(300), Height: LengthDimension(300)}
	inner := tree.NewNode(DefaultStyle())
	tree.SetChildren(root, []Node{inner})
	a := tree.NewNode(fixedSize(100, 100))
	b := tree.NewNode(fixedSize(100, 100))
	tree.SetChildren(inner, []Node{a, b})

	tree.ComputeLayout(root, DefiniteAvailable(300, 300))
	// inner's main size is content-based; its cross size stretches.
	li := tree.Layout(inner)
	approxF(t, "inner.w", li.Width, 200)
	approxF(t, "inner.h", li.Height, 300)
	approxF(t, "a.x", tree.Layout(a).X, 0)
	approxF(t, "b.x", tree.Layout(b).X, 100)
}

func TestRowReverse(t *testing.T) {
	tree := NewTree()
	s := DefaultStyle()
	s.FlexDirection = FlexRowReverse
	root := tree.NewNode(s)
	a := tree.NewNode(fixedSize(50, 10))
	b := tree.NewNode(fixedSize(50, 10))
	tree.SetChildren(root, []Node{a, b})
	tree.ComputeLayout(root, DefiniteAvailable(200, 10))
	// First child at the right edge.
	approxF(t, "a.x", tree.Layout(a).X, 150)
	approxF(t, "b.x", tree.Layout(b).X, 100)
}

func TestBlockLayout(t *testing.T) {
	tree := NewTree()
	s := DefaultStyle()
	s.Display = DisplayBlock
	root := tree.NewNode(s)
	a := tree.NewNode(DefaultStyle())
	b := tree.NewNode(DefaultStyle())
	tree.SetChildren(root, []Node{a, b})
	// Auto-width children stretch to the container; heights from content.
	tree.Style(a).Size = SizeDimension{Width: AutoDimension(), Height: LengthDimension(30)}
	tree.Style(b).Size = SizeDimension{Width: AutoDimension(), Height: LengthDimension(20)}
	tree.Style(root).Size = SizeDimension{Width: LengthDimension(100), Height: AutoDimension()}
	tree.ComputeLayout(root, DefiniteAvailable(100, 500))

	approxF(t, "a.w", tree.Layout(a).Width, 100)
	approxF(t, "a.h", tree.Layout(a).Height, 30)
	approxF(t, "b.y", tree.Layout(b).Y, 30)
	approxF(t, "root.h", tree.Layout(root).Height, 50)
}

func TestDisplayNone(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(DefaultStyle())
	a := tree.NewNode(fixedSize(50, 50))
	tree.Style(a).Display = DisplayNone
	b := tree.NewNode(fixedSize(30, 30))
	tree.SetChildren(root, []Node{a, b})
	tree.ComputeLayout(root, DefiniteAvailable(200, 200))
	approxF(t, "b.x", tree.Layout(b).X, 0)
	approxF(t, "root.w", tree.Layout(root).Width, 200) // root default
	approxF(t, "root.h", tree.Layout(root).Height, 30)
}

func TestFlexBasis(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(DefaultStyle())
	a := tree.NewNode(DefaultStyle())
	tree.Style(a).FlexBasis = LengthDimension(120)
	tree.Style(a).Size = SizeDimension{Width: LengthDimension(10), Height: LengthDimension(10)}
	tree.SetChildren(root, []Node{a})
	tree.ComputeLayout(root, DefiniteAvailable(200, 10))
	approxF(t, "a.w", tree.Layout(a).Width, 120)
}

func TestScrollExtent(t *testing.T) {
	tree := NewTree()
	root := tree.NewNode(DefaultStyle())
	tree.Style(root).Size = SizeDimension{Width: LengthDimension(100), Height: LengthDimension(100)}
	a := tree.NewNode(fixedSize(300, 50))
	tree.Style(a).FlexShrink = 0
	tree.SetChildren(root, []Node{a})
	tree.ComputeLayout(root, DefiniteAvailable(100, 100))
	// Root is 100 wide but content is 300: scroll extent reports it.
	approxF(t, "root.scrollW", tree.Layout(root).ScrollWidth, 300)
	approxF(t, "root.w", tree.Layout(root).Width, 100)
}
