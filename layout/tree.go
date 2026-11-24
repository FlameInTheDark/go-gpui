package layout

import "math"

// Node is a handle to a node in the layout tree.
type Node uint32

// Layout is the computed layout of a node (position relative to its parent,
// size, plus border-box padding/border amounts).
type Layout struct {
	Order         int
	X, Y          float32
	Width, Height float32
	// ScrollWidth/Height report the content extent for scroll containers.
	ScrollWidth, ScrollHeight float32
}

// Measurer measures a leaf node (e.g. text). knownDimensions are the
// already-definite dimensions; available is the remaining space.
type Measurer func(known SizeDimension, available AvailableSpace) SizeResult

// SizeResult is the measured size plus optional content size.
type SizeResult struct {
	Width, Height float32
}

// Tree is a mutable layout tree (mirrors taffy_tree::TaffyTree). Nodes are
// created with styles; children are attached; ComputeLayout runs the
// algorithms and stores results. Each node's Layout location is relative to
// its parent; AbsoluteBounds accumulates the chain (upstream taffy.rs
// layout_bounds).
type Tree struct {
	styles   []Style
	children [][]Node
	parent   []Node // self-referential for parentless roots
	measure  []Measurer
	layouts  []Layout
	order    []int
	hasOrder []bool
	nextNode Node
	// absCache memoizes absolute bounds per ComputeLayout pass.
	absCache map[Node]Layout
	// measureCache memoizes measure-closure results per ComputeLayout
	// pass. The flexbox algorithm measures a leaf many times per pass
	// (flex base size, cross size, per-line distribution), and container
	// content-size probes re-walk subtrees; without the cache a frame
	// re-ran the same text measures dozens of times. Cleared at the
	// start of every ComputeLayout (results are pure functions of the
	// inputs, and styles/children are stable within a pass).
	measureCache map[measureKey]SizeResult
	// probeCache memoizes container compute results per (node, input)
	// within one ComputeLayout pass: the flexbox algorithm measures the
	// same subtree several times (flex basis, hypothetical cross size,
	// line distribution) and each probe is a full recursive layout.
	// compute() is a pure function of (node, input) inside a pass, so
	// hits are exact; final passes with changed inputs re-run normally.
	probeCache map[probeKey]layoutOutput
}

// measureKey is a memo entry: one node measured with one input.
type measureKey struct {
	node  Node
	known SizeDimension
	avail AvailableSpace
}

// NewTree creates an empty tree.
func NewTree() *Tree {
	return &Tree{absCache: map[Node]Layout{}, measureCache: map[measureKey]SizeResult{}, probeCache: map[probeKey]layoutOutput{}}
}

// Reset discards all nodes and cached results while retaining storage for
// the next tree. Existing node IDs and pointers from Style are invalidated.
func (t *Tree) Reset() {
	// These slices hold references to children, style data or element closures.
	clear(t.styles)
	clear(t.children)
	clear(t.measure)
	t.styles = t.styles[:0]
	t.children = t.children[:0]
	t.parent = t.parent[:0]
	t.measure = t.measure[:0]
	t.layouts = t.layouts[:0]
	t.order = t.order[:0]
	t.hasOrder = t.hasOrder[:0]
	t.nextNode = 0
	t.clearCaches()
}

func (t *Tree) clearCaches() {
	clear(t.absCache)
	clear(t.measureCache)
	clear(t.probeCache)
}

// NewNode appends a node with the given style.
func (t *Tree) NewNode(style Style) Node {
	n := t.nextNode
	t.nextNode++
	t.styles = append(t.styles, style)
	t.children = append(t.children, nil)
	// Self-parent marks "no parent" (roots of independent subtrees).
	t.parent = append(t.parent, n)
	t.measure = append(t.measure, nil)
	t.layouts = append(t.layouts, Layout{})
	t.order = append(t.order, 0)
	t.hasOrder = append(t.hasOrder, false)
	return n
}

// NewLeaf appends a measured leaf node.
func (t *Tree) NewLeaf(style Style, measure Measurer) Node {
	n := t.NewNode(style)
	t.measure[n] = measure
	return n
}

// SetChildren replaces a node's children.
func (t *Tree) SetChildren(node Node, children []Node) {
	for _, c := range t.children[node] {
		t.parent[c] = c
	}
	t.children[node] = children
	for _, c := range children {
		t.parent[c] = node
	}
}

// Children returns a node's children.
func (t *Tree) Children(node Node) []Node { return t.children[node] }

// Style returns a node's style.
func (t *Tree) Style(node Node) *Style { return &t.styles[node] }

// SetStyle replaces a node's style.
func (t *Tree) SetStyle(node Node, s Style) { t.styles[node] = s }

// Layout returns a node's computed layout (position relative to its
// parent).
func (t *Tree) Layout(node Node) Layout { return t.layouts[node] }

// AbsoluteBounds returns the node's bounds relative to its subtree root by
// accumulating parent-chain locations, memoized per layout pass (upstream
// taffy.rs LayoutEngine::layout_bounds).
func (t *Tree) AbsoluteBounds(node Node) Layout {
	if l, ok := t.absCache[node]; ok {
		return l
	}
	l := t.layouts[node]
	if p := t.parent[node]; p != node {
		pl := t.AbsoluteBounds(p)
		l.X += pl.X
		l.Y += pl.Y
	}
	t.absCache[node] = l
	return l
}

// SetMeasure attaches/replaces a measure function.
func (t *Tree) SetMeasure(node Node, m Measurer) { t.measure[node] = m }

// Remove disposes a node's children.
func (t *Tree) Remove(node Node) {
	t.SetChildren(node, nil)
}

// ComputeLayout runs layout for the subtree rooted at root.
func (t *Tree) ComputeLayout(root Node, available AvailableSpace) {
	t.clearCaches()
	// Keep ComputeLayout usable on zero-valued trees, too.
	if t.absCache == nil {
		t.absCache = map[Node]Layout{}
		t.measureCache = map[measureKey]SizeResult{}
		t.probeCache = map[probeKey]layoutOutput{}
	}
	compute(t, root, layoutInput{
		known:       SizeDimension{Width: AutoDimension(), Height: AutoDimension()},
		parentStyle: nil,
		available:   available,
		// The root runs in commit mode: every item subtree below
		// recomputes its layout fresh, so the stored node layouts the
		// paint walk reads always reflect the FINAL bounds. Measurement
		// probes still share the probe cache (see layoutInput.commit).
		commit: true,
	}, true)
}

// StretchAutoSizeToFill treats any auto dimension of the node's style as
// filling the given size (upstream stretch_auto_size_to_fill). This is
// applied to window roots before layout so they behave like the root element
// on the web, stretching to fill the viewport unless given an explicit size.
// Explicitly styled dimensions are preserved.
func (t *Tree) StretchAutoSizeToFill(node Node, width, height float32) {
	st := &t.styles[node]
	stretchWidth := st.Size.Width.IsAuto()
	stretchHeight := st.Size.Height.IsAuto()
	if !stretchWidth && !stretchHeight {
		return
	}
	if stretchWidth {
		st.Size.Width = LengthDimension(width)
	}
	if stretchHeight {
		st.Size.Height = LengthDimension(height)
	}
}

type layoutInput struct {
	known       SizeDimension
	parentStyle *Style
	available   AvailableSpace
	// forceRelative lays an absolute node out as if relative (used by the
	// parent's out-of-flow pass to size the subtree).
	forceRelative bool
	// stretchCross marks a measurement of an auto-cross-sized child that
	// stretches into a definite parent cross axis (upstream measures
	// stretched items with the container's cross size). The child must lay
	// its own children out in that definite space instead of max-content,
	// otherwise one wide leaf inflates every auto ancestor (the flex
	// shrink-to-fit probe recurses at max-content all the way up).
	stretchCross bool
	// commit marks the FINAL layout pass of an item subtree: the compute
	// bypasses the probe cache so every descendant's stored layout is
	// rewritten with the item's resolved sizes. Without it a later
	// max-content probe (a basis or content-size measurement elsewhere in
	// the tree) can overwrite a leaf's stored layout with unwrapped
	// bounds, and the final bounded compute — a probe-cache hit — leaves
	// that stale layout in place for the paint walk to read (a wrapped
	// settings description then painted as one clipped line under its
	// input field).
	commit bool
}

type layoutOutput struct {
	size SizeResult
}

func compute(t *Tree, node Node, input layoutInput, isRoot bool) layoutOutput {
	style := &t.styles[node]
	if style.Display == DisplayNone {
		t.layouts[node] = Layout{Order: nextOrder()}
		return layoutOutput{}
	}

	// Absolutely-positioned nodes are laid out by their parent (the
	// containing block) after in-flow layout; at the root they lay out
	// like relative.
	if style.Position == PositionAbsolute && !isRoot && input.parentStyle != nil && !input.forceRelative {
		// Position via the parent's known geometry, if any.
		w, _ := input.available.IsDefiniteAxis(true)
		h, _ := input.available.IsDefiniteAxis(false)
		t.layoutAbsolute(node, style, input.available, w, h, [4]float32{}, [4]float32{}, input.commit)
		return layoutOutput{size: SizeResult{Width: t.layouts[node].Width, Height: t.layouts[node].Height}}
	}

	// Childless nodes always go through the leaf layout (taffy routes every
	// leaf through compute_leaf_layout), with or without a measure function.
	if len(t.children[node]) == 0 {
		return t.computeLeaf(node, style, input)
	}

	// Containers memoize per (node, input): measurement probes repeat
	// identical subtree layouts, and a hit returns the memoized output
	// (the earlier run already wrote the same layouts — compute is
	// deterministic within a pass). Commit computes skip the cache: they
	// exist to REWRITE the subtree's stored layouts at the final sizes,
	// which a hit would skip (see layoutInput.commit).
	if !isRoot && !input.commit {
		key := probeKey{node: node, known: input.known, avail: input.available,
			parentStyle: input.parentStyle, forceRelative: input.forceRelative,
			stretchCross: input.stretchCross}
		if out, ok := t.probeCache[key]; ok {
			return out
		}
		var out layoutOutput
		switch style.Display {
		case DisplayFlex:
			out = t.computeFlexbox(node, style, input)
		case DisplayGrid:
			out = t.computeGrid(node, style, input)
		default:
			out = t.computeBlock(node, style, input)
		}
		t.probeCache[key] = out
		return out
	}

	switch style.Display {
	case DisplayFlex:
		return t.computeFlexbox(node, style, input)
	case DisplayGrid:
		return t.computeGrid(node, style, input)
	default:
		return t.computeBlock(node, style, input)
	}
}

// probeKey identifies one memoized container compute (see Tree.probeCache).
// parentStyle participates because the root default path reads it.
type probeKey struct {
	node          Node
	known         SizeDimension
	avail         AvailableSpace
	parentStyle   *Style
	forceRelative bool
	stretchCross  bool
}

// orderCounter assigns document order for stable hit testing.
var orderCounter int

func nextOrder() int {
	orderCounter++
	return orderCounter
}

func clampInto(v, min, max float32) float32 {
	if !math.IsNaN(float64(min)) && min > 0 && v < min {
		v = min
	}
	if !math.IsNaN(float64(max)) && max > 0 && v > max {
		v = max
	}
	return v
}

// withRootDefaults applies the taffy root rule: the root's auto width
// defaults to the available width. It is applied via the known dimensions
// so all downstream resolution sees it.
func (in layoutInput) withRootDefaults(style *Style) layoutInput {
	if in.parentStyle != nil {
		return in
	}
	// Root: only when the style width is auto and no known width.
	if in.known.Width.IsAuto() && style.Size.Width.IsAuto() {
		if v, ok := in.available.IsDefiniteAxis(true); ok {
			in.known.Width = LengthDimension(v)
		}
	}
	return in
}
