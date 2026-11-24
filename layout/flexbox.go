package layout

import "math"

// This file ports taffy's compute/flexbox.rs (v0.15) — the CSS flexbox
// algorithm. The implementation parametrizes axes instead of branching on
// row/column everywhere; function names track upstream
// (compute_preliminary, determine_flex_base_size, collect_flex_lines,
// resolve_flexible_lengths, ...).

// axis abstracts main vs cross access into sizes and edge arrays.
type axis struct {
	mainIsWidth bool
}

func (a axis) main(size SizeResult) float32 {
	if a.mainIsWidth {
		return size.Width
	}
	return size.Height
}

func (a axis) cross(size SizeResult) float32 {
	if a.mainIsWidth {
		return size.Height
	}
	return size.Width
}

// mainAxisIndex is the style overflow slot of the main axis (0 = x, 1 = y).
func (a axis) mainAxisIndex() int {
	if a.mainIsWidth {
		return 0
	}
	return 1
}

func (a axis) setMain(size *SizeResult, v float32) {
	if a.mainIsWidth {
		size.Width = v
	} else {
		size.Height = v
	}
}

func (a axis) setCross(size *SizeResult, v float32) {
	if a.mainIsWidth {
		size.Height = v
	} else {
		size.Width = v
	}
}

func (a axis) mainDim(sd SizeDimension) Dimension {
	if a.mainIsWidth {
		return sd.Width
	}
	return sd.Height
}

func (a axis) crossDim(sd SizeDimension) Dimension {
	if a.mainIsWidth {
		return sd.Height
	}
	return sd.Width
}

func (a axis) setMainDim(sd *SizeDimension, d Dimension) {
	if a.mainIsWidth {
		sd.Width = d
	} else {
		sd.Height = d
	}
}

// horizontal edge sum (1,3 indices)
func hSum(e [4]float32) float32 { return e[1] + e[3] }

// vertical edge sum (0,2 indices)
func vSum(e [4]float32) float32 { return e[0] + e[2] }

func (a axis) mainEdgeSum(e [4]float32) float32 {
	if a.mainIsWidth {
		return hSum(e)
	}
	return vSum(e)
}

func (a axis) crossEdgeSum(e [4]float32) float32 {
	if a.mainIsWidth {
		return vSum(e)
	}
	return hSum(e)
}

// item is an in-flow child being laid out.
type item struct {
	node    Node
	style   *Style
	margin  [4]float32 // resolved
	border  [4]float32
	padding [4]float32

	flexBasis         float32
	hypotheticalMain  float32 // outer (includes margins)
	targetMain        float32 // inner content size on the main axis
	hypotheticalCross float32 // inner cross size
	finalMain         float32
	finalCross        float32
	posMain           float32
	posCross          float32
	frozen            bool
	// autoMinMain is the automatic minimum main size (CSS flexbox
	// min-size:auto, taffy resolved_minimum_main_size): zero for scroll
	// containers, the definite main size otherwise, or the subtree's
	// min-content contribution for auto-sized containers.
	autoMinMain float32
}

func (it *item) outerMain(a axis) float32 { return it.targetMain + a.mainEdgeSum(it.margin) }

type flexLine struct {
	items       []*item
	crossSize   float32
	offsetCross float32
}

func (t *Tree) computeFlexbox(node Node, style *Style, input layoutInput) layoutOutput {
	a := axis{mainIsWidth: style.FlexDirection.IsRow()}
	reverseMain := style.FlexDirection.IsReverse()
	_ = node

	// Container known dimensions resolved from style + input. Like taffy's
	// initial containing block, a root's auto width defaults to the
	// available width; height stays content-based.
	input = input.withRootDefaults(style)
	containerW, wDef := resolveContainerAxis(input, style.Size.Width, style.MinSize.Width, style.MaxSize.Width, true)
	containerH, hDef := resolveContainerAxis(input, style.Size.Height, style.MinSize.Height, style.MaxSize.Height, false)

	margin := style.Margin.Resolve(containerW)
	border := style.Border.Resolve(containerW)
	padding := style.Padding.Resolve(containerW)

	// Inner sizes.
	innerW := containerW - edgeSum3(margin, border, padding, true)
	innerH := containerH - edgeSum3(margin, border, padding, false)
	if !wDef {
		innerW = 0
	}
	if !hDef {
		innerH = 0
	}
	if innerW < 0 {
		innerW = 0
	}
	if innerH < 0 {
		innerH = 0
	}

	mainInner, mainDef := innerW, wDef
	if !a.mainIsWidth {
		mainInner, mainDef = innerH, hDef
	}
	crossInner, crossDef := innerH, hDef
	if !a.mainIsWidth {
		crossInner, crossDef = innerW, wDef
	}
	// A stretch-measured container lays its children out in the definite
	// available cross space instead of max-content (upstream: the
	// stretched hypothetical cross size constrains nested layout).
	if !crossDef && input.stretchCross {
		if v, ok := input.available.IsDefiniteAxis(!a.mainIsWidth); ok {
			crossInner, crossDef = v, true
		}
	}
	// Fallbacks for percentage resolution: use available space when the
	// container size is not definite.
	mainParent := mainInner
	if v, ok := input.available.IsDefiniteAxis(a.mainIsWidth); ok {
		mainParent = v
	}
	crossParent := crossInner
	if v, ok := input.available.IsDefiniteAxis(!a.mainIsWidth); ok {
		crossParent = v
	}

	gapMain := a.mainDim(style.Gap).MaybeResolve(mainParent, 0)
	gapCross := a.crossDim(style.Gap).MaybeResolve(crossParent, 0)

	// ---- generate items -------------------------------------------------
	var items []*item
	for _, child := range t.children[node] {
		cs := &t.styles[child]
		if cs.Display == DisplayNone {
			t.layouts[child] = Layout{Order: nextOrder()}
			continue
		}
		if cs.Position == PositionAbsolute {
			continue // handled after in-flow layout
		}
		it := &item{node: child, style: cs}
		it.margin = cs.Margin.Resolve(containerW)
		it.border = cs.Border.Resolve(containerW)
		it.padding = cs.Padding.Resolve(containerW)
		items = append(items, it)
	}

	// ---- determine_flex_base_size + hypothetical main size --------------
	for _, it := range items {
		cs := it.style
		basis, ok := cs.FlexBasis.Resolve(mainParent)
		if !ok {
			if d, isDef := a.mainDim(cs.Size).Resolve(mainParent); isDef {
				basis, ok = d, true
			}
		}
		if !ok {
			// Content sizing.
			basis = t.contentSize(it.node, cs, a, mainParent, crossInner, crossDef, input)
		}
		// taffy resolved_minimum_main_size (condensed): an explicit min
		// wins; otherwise scroll containers along the main axis have an
		// automatic minimum of zero, and other items never shrink below
		// their min-content contribution clamped by a definite main
		// size (CSS flexbox min-size:auto). Without it the tree list's
		// content height inflates the workspace row's flex basis and
		// the shrink phase collapses every fixed-height chrome bar
		// (menu/tab/status) instead of only the scrollable area.
		it.autoMinMain = t.automaticMinMainSize(it.node, cs, a, mainParent, crossInner, crossDef, input)
		basis = clampInto(basis,
			a.mainDim(cs.MinSize).MaybeResolve(mainParent, it.autoMinMain),
			a.mainDim(cs.MaxSize).ResolveLoose(mainParent))
		it.flexBasis = basis
		it.targetMain = basis
		it.hypotheticalMain = basis + a.mainEdgeSum(it.margin)
	}

	// ---- collect_flex_lines (greedy) ------------------------------------
	lines := collectFlexLines(items, style.FlexWrap, gapMain, mainInner, mainDef, a)

	// ---- resolve_flexible_lengths ---------------------------------------
	// Children of a SCROLL container do not flex along the scrollable axis
	// (taffy lays them out at their content size; they overflow and scroll).
	// OverflowHidden does not count: a hidden-overflow container flexes its
	// children normally and clips whatever spills (the editor wrapper fills
	// and centers its child inside a fixed pane).
	mainScrollable := style.Overflow[a.mainAxisIndex()] == OverflowScroll
	for _, line := range lines {
		resolveFlexibleLengths(line, a, gapMain, mainInner, mainDef, mainScrollable)
	}

	// ---- determine_container_main_size ----------------------------------
	if !mainDef {
		mainInner = 0
		for _, line := range lines {
			l := -gapMain
			for _, it := range line.items {
				l += gapMain + it.outerMain(a)
			}
			if l > mainInner {
				mainInner = l
			}
		}
	}

	// ---- cross sizing ----------------------------------------------------
	// Resolved alignment (used by both cross sizing and placement).
	alignItems := AlignStretch
	if style.AlignItems != nil {
		alignItems = *style.AlignItems
	}
	for _, line := range lines {
		line.crossSize = 0
		for _, it := range line.items {
			align := alignItems
			if it.style.AlignSelf != nil {
				align = *it.style.AlignSelf
			}
			cross := t.hypotheticalCrossSize(it, a, crossInner, crossDef, crossParent, input, align)
			it.hypotheticalCross = cross
			outer := cross + a.crossEdgeSum(it.margin)
			// A stretch-aligned item with an auto cross size was
			// measured as the stretch FILL of the definite cross
			// (the stretchCross branch in hypotheticalCrossSize).
			// Adding its margins on top of the fill double-counts
			// them: the line grows by the margin sum, and the
			// alignment step below re-subtracts the margins once,
			// leaving the item wider than the container by exactly
			// the margins (menu rows with Mx(4) overflowed their
			// panel and clipped the right-aligned shortcut text).
			// Taffy sizes lines with content-based hypotheticals
			// and applies the stretch in alignment; here the fill
			// is the item's whole outer contribution. The condition
			// mirrors the stretchCross branch exactly.
			if crossDef && align.Resolve(false) == AlignStretch &&
				a.crossDim(it.style.Size).IsAuto() &&
				flexBasisAutoLike(it.style.FlexBasis) {
				outer = cross
			}
			if outer > line.crossSize {
				line.crossSize = outer
			}
		}
	}

	if !crossDef {
		crossInner = 0
		for _, line := range lines {
			crossInner += line.crossSize
		}
		if len(lines) > 1 {
			crossInner += gapCross * float32(len(lines)-1)
		}
		crossInner = clampInto(crossInner,
			a.crossDim(style.MinSize).MaybeResolve(crossParent, 0),
			a.crossDim(style.MaxSize).ResolveLoose(crossParent))
	}

	// ---- align_content ---------------------------------------------------
	alignContent := AlignContentStretch
	if style.AlignContent != nil {
		alignContent = *style.AlignContent
	}
	if style.FlexWrap != FlexNoWrap {
		distributeCross(lines, alignContent.Resolve(false), crossInner, gapCross)
	} else if len(lines) == 1 {
		lines[0].crossSize = maxf32(lines[0].crossSize, crossInner)
	}

	// Reverse cross for wrap-reverse.
	crossCursor := float32(0)
	if style.FlexWrap == FlexWrapReverse {
		total := float32(0)
		for _, l := range lines {
			total += l.crossSize
		}
		total += gapCross * float32(len(lines)-1)
		crossCursor = crossInner - total
	}

	// ---- main/cross placement -------------------------------------------
	justify := AlignContentFlexStart
	if style.JustifyContent != nil {
		justify = *style.JustifyContent
	}

	for _, line := range lines {
		lineMain := -gapMain
		for _, it := range line.items {
			lineMain += gapMain + it.outerMain(a)
		}
		freeMain := mainInner - lineMain
		offset, spacing := justifyDistribution(justify, freeMain, len(line.items))

		mainPos := offset
		if reverseMain {
			mainPos = mainInner - offset
		}
		for i, it := range line.items {
			outer := it.outerMain(a)
			if reverseMain {
				it.posMain = mainPos - outer
				mainPos = it.posMain - gapMain
			} else {
				it.posMain = mainPos
				mainPos = it.posMain + outer + gapMain
			}
			if i < len(line.items)-1 {
				// Inter-item spacing from space-between/evenly/around.
				it.posMain += 0 // spacing applied below via cursor
				mainPos += spacing
			}

			// Cross alignment inside the line.
			align := alignItems
			if it.style.AlignSelf != nil {
				align = *it.style.AlignSelf
			}
			availableCross := line.crossSize - a.crossEdgeSum(it.margin)
			crossPos, crossSize := alignCross(align, it, availableCross, a)
			it.posCross = line.offsetCross + crossPos
			it.finalCross = crossSize
		}
		// Advance the line cursor.
		line.offsetCross = crossCursor
		crossCursor += line.crossSize + gapCross
	}

	// Recompute positions with the (possibly updated) line offsets.
	for _, line := range lines {
		for _, it := range line.items {
			align := alignItems
			if it.style.AlignSelf != nil {
				align = *it.style.AlignSelf
			}
			availableCross := line.crossSize - a.crossEdgeSum(it.margin)
			crossPos, _ := alignCross(align, it, availableCross, a)
			it.posCross = line.offsetCross + crossPos
		}
	}

	// ---- subtree layout for items ---------------------------------------
	for _, line := range lines {
		for _, it := range line.items {
			t.layoutItemSubtree(it, a, mainInner, crossInner, input)
		}
	}

	// ---- container size --------------------------------------------------
	var size SizeResult
	a.setMain(&size, mainInner)
	a.setCross(&size, crossInner)
	w := size.Width + edgeSum3(margin, border, padding, true)
	h := size.Height + edgeSum3(margin, border, padding, false)

	// Content extents for scrolling.
	scrollW, scrollH := float32(0), float32(0)
	for _, line := range lines {
		for _, it := range items2all(line) {
			endMain := it.posMain + it.outerMain(a)
			endCross := it.posCross + it.finalCross + a.crossEdgeSum(it.margin)
			var m, c float32
			if a.mainIsWidth {
				m, c = endMain, endCross
			} else {
				m, c = endCross, endMain
			}
			if m > scrollW {
				scrollW = m
			}
			if c > scrollH {
				scrollH = c
			}
		}
	}

	if vw, ok := style.Size.Width.Resolve(containerW); ok && wDef {
		w = containerW
	} else if ok {
		w = vw + margin[1] + margin[3]
	}
	if vh, ok := style.Size.Height.Resolve(containerH); ok && hDef {
		h = containerH
	} else if ok {
		h = vh + margin[0] + margin[2]
	}
	if wDef {
		w = containerW
	}
	if hDef {
		h = containerH
	}
	w = clampInto(w, style.MinSize.Width.MaybeResolve(containerW, 0), style.MaxSize.Width.ResolveLoose(containerW))
	h = clampInto(h, style.MinSize.Height.MaybeResolve(containerH, 0), style.MaxSize.Height.ResolveLoose(containerH))

	layout := Layout{Order: nextOrder(), Width: w, Height: h, ScrollWidth: scrollW, ScrollHeight: scrollH}

	// Store child layouts in container border-box coordinates.
	innerOriginX := border[3] + padding[3]
	innerOriginY := border[0] + padding[0]
	for _, line := range lines {
		for _, it := range line.items {
			var x, y float32
			if a.mainIsWidth {
				x = innerOriginX + it.posMain + it.margin[3]
				y = innerOriginY + it.posCross + it.margin[0]
			} else {
				x = innerOriginX + it.posCross + it.margin[3]
				y = innerOriginY + it.posMain + it.margin[0]
			}
			_ = x
			_ = y
			cl := t.layouts[it.node]
			cl.Order = nextOrder()
			if a.mainIsWidth {
				cl.X = innerOriginX + it.posMain + it.margin[3]
				cl.Y = innerOriginY + it.posCross + it.margin[0]
			} else {
				cl.X = innerOriginX + it.posCross + it.margin[3]
				cl.Y = innerOriginY + it.posMain + it.margin[0]
			}
			t.layouts[it.node] = cl
		}
	}
	t.layouts[node] = layout

	// Absolute children.
	for _, child := range t.children[node] {
		cs := &t.styles[child]
		if cs.Position == PositionAbsolute && cs.Display != DisplayNone {
			t.layoutAbsolute(child, cs, input.available, innerW, innerH, border, padding, input.commit)
		}
	}

	return layoutOutput{size: SizeResult{Width: w, Height: h}}
}

func items2all(line *flexLine) []*item { return line.items }

// resolveContainerAxis resolves one container axis from the input and the
// style's size/min/max.
func resolveContainerAxis(input layoutInput, sizeD, minD, maxD Dimension, isWidth bool) (float32, bool) {
	parent := input.available.Axis(isWidth)
	var v float32
	def := false
	if sd, ok := sizeD.Resolve(parent); ok {
		v, def = sd, true
	}
	if kd, ok := func() (float32, bool) {
		if isWidth {
			return input.known.Width.Resolve(parent)
		}
		return input.known.Height.Resolve(parent)
	}(); ok {
		v, def = kd, true
	}
	if def {
		v = clampInto(v, minD.MaybeResolve(parent, 0), maxD.ResolveLoose(parent))
	}
	return v, def
}

// contentSize computes the content-based main size of an auto item.
func (t *Tree) contentSize(node Node, style *Style, a axis, mainParent, crossInner float32, crossDef bool, input layoutInput) float32 {
	// Leaves with measure functions.
	if t.measure[node] != nil && len(t.children[node]) == 0 {
		avail := input.available
		known := SizeDimension{Width: AutoDimension(), Height: AutoDimension()}
		if crossDef {
			if a.mainIsWidth {
				known = SizeDimension{Width: AutoDimension(), Height: LengthDimension(crossInner)}
			} else {
				known = SizeDimension{Width: LengthDimension(crossInner), Height: AutoDimension()}
			}
		}
		res := t.runMeasure(node, known, avail)
		return a.main(res)
	}
	if len(t.children[node]) == 0 {
		return 0
	}
	// Container: measure at max content along the main axis, with the
	// container's definite cross space handed in as a known cross
	// dimension — the same constraint leaves get above. Without it a
	// wrapping subtree (a settings row's description) shapes at
	// max-content width where it fits on one line, the item's flex basis
	// misses the wrapped lines, and the row never grows to fit them.
	sub := layoutInput{
		known:       SizeDimension{Width: AutoDimension(), Height: AutoDimension()},
		available:   MaxContentAvailable(),
		parentStyle: style,
	}
	if crossDef && a.crossDim(style.Size).IsAuto() {
		if a.mainIsWidth {
			sub.known.Height = LengthDimension(crossInner)
		} else {
			sub.known.Width = LengthDimension(crossInner)
		}
	}
	sub.forceRelative = true
	out := compute(t, node, sub, false)
	return a.main(out.size)
}

// automaticMinMainSize is the automatic minimum size of a flex item along
// the main axis (CSS flexbox min-size:auto, taffy
// resolved_minimum_main_size): scroll containers (hidden or scroll overflow
// on the main axis) contribute zero, and other items contribute their
// min-content size capped by a definite main size and floored by the
// main-axis padding+border.
func (t *Tree) automaticMinMainSize(node Node, cs *Style, a axis, mainParent, crossInner float32, crossDef bool, input layoutInput) float32 {
	if o := cs.Overflow[a.mainAxisIndex()]; o == OverflowHidden || o == OverflowScroll {
		return 0
	}
	// Measured leaves contribute zero: their measured size depends on the
	// available space (wrapping text grows taller as it shrinks), so a
	// max-content based floor would forbid the shrink that re-wrapping
	// requires — and re-measuring here would poison the leaf's own memo
	// (the last shape call wins), breaking wrapped layouts.
	if len(t.children[node]) == 0 {
		return 0
	}
	minContent := t.minContentMainSize(node, cs, a, mainParent, crossInner, crossDef, input)
	if d, ok := a.mainDim(cs.Size).Resolve(mainParent); ok && d < minContent {
		// The specified-size suggestion caps the content suggestion, so a
		// fixed-size item never auto-grows its minimum beyond its own size.
		minContent = d
	}
	if pb := a.mainEdgeSum(cs.Padding.Resolve(0)) + a.mainEdgeSum(cs.Border.Resolve(0)); minContent < pb {
		minContent = pb
	}
	return minContent
}

// minContentMainSize estimates the node's min-content contribution along the
// main axis, excluding the node's own padding/border (taffy measures with
// Size::MIN_CONTENT): measured leaves report their measurement, and nowrap
// containers sum their in-flow children's automatic minimums. Wrapping lists
// never reach this walk — their scroll container ancestors short-circuit to
// zero above.
func (t *Tree) minContentMainSize(node Node, cs *Style, a axis, mainParent, crossInner float32, crossDef bool, input layoutInput) float32 {
	if len(t.children[node]) == 0 {
		if t.measure[node] == nil {
			return 0
		}
		known := SizeDimension{Width: AutoDimension(), Height: AutoDimension()}
		if crossDef {
			if a.mainIsWidth {
				known = SizeDimension{Width: AutoDimension(), Height: LengthDimension(crossInner)}
			} else {
				known = SizeDimension{Width: LengthDimension(crossInner), Height: AutoDimension()}
			}
		}
		res := t.runMeasure(node, known, input.available)
		return a.main(res)
	}
	// Container: gaps + in-flow children's automatic minimums (border-box
	// definite child sizes already include their own edges). Children
	// resolve percentages against this container's available main size
	// (mainParent); auto-sized containers pass it through unchanged.
	gap := a.mainDim(cs.Gap).MaybeResolve(mainParent, 0)
	total := float32(0)
	n := 0
	for _, ch := range t.children[node] {
		chs := &t.styles[ch]
		if chs.Display == DisplayNone || chs.Position == PositionAbsolute {
			continue
		}
		contrib := t.automaticMinMainSize(ch, chs, a, mainParent, crossInner, crossDef, input)
		if _, ok := a.mainDim(chs.Size).Resolve(mainParent); !ok {
			contrib += a.mainEdgeSum(chs.Padding.Resolve(0)) + a.mainEdgeSum(chs.Border.Resolve(0))
		}
		total += contrib + a.mainEdgeSum(chs.Margin.Resolve(0))
		n++
	}
	if n > 1 {
		total += gap * float32(n-1)
	}
	return total
}

// runMeasure invokes a node's measure function, memoized per ComputeLayout
// pass: the flexbox algorithm measures the same leaf with the same inputs
// several times (flex base size, hypothetical cross size, line
// distribution), and container content-size probes repeat subtree walks.
// Measure closures are pure functions of (node, known, available) within a
// pass, so the memo is safe; ComputeLayout clears it.
func (t *Tree) runMeasure(node Node, known SizeDimension, available AvailableSpace) SizeResult {
	if t.measure[node] == nil {
		return SizeResult{}
	}
	key := measureKey{node: node, known: known, avail: available}
	if r, ok := t.measureCache[key]; ok {
		return r
	}
	r := t.measure[node](known, available)
	res := SizeResult{Width: r.Width, Height: r.Height}
	t.measureCache[key] = res
	return res
}

// hypotheticalCrossSize computes the item's inner cross size.
func (t *Tree) hypotheticalCrossSize(it *item, a axis, crossInner float32, crossDef bool, crossParent float32, input layoutInput, align AlignItems) float32 {
	cs := it.style
	if v, ok := a.crossDim(cs.Size).Resolve(crossParent); ok {
		return clampInto(v, a.crossDim(cs.MinSize).MaybeResolve(crossParent, 0), a.crossDim(cs.MaxSize).ResolveLoose(crossParent))
	}
	// Auto: content size (stretch is applied during alignment).
	if t.measure[it.node] != nil && len(t.children[it.node]) == 0 {
		// Upstream determine_hypothetical_cross_size: the item is measured with
		// its (resolved) main-axis target size as a known dimension and the
		// container's available space, whose definite cross axis caps items
		// like virtualized lists (desired height > viewport).
		known := SizeDimension{Width: AutoDimension(), Height: AutoDimension()}
		if a.mainIsWidth {
			known.Width = LengthDimension(it.targetMain)
		} else {
			known.Height = LengthDimension(it.targetMain)
		}
		avail := input.available
		if crossDef {
			if a.mainIsWidth {
				avail = DefiniteAvailable(avail.Width, crossInner)
			} else {
				avail = DefiniteAvailable(crossInner, avail.Height)
			}
		}
		res := t.runMeasure(it.node, known, avail)
		v := a.cross(res)
		return clampInto(v, a.crossDim(cs.MinSize).MaybeResolve(crossParent, 0), a.crossDim(cs.MaxSize).ResolveLoose(crossParent))
	}
	if len(t.children[it.node]) > 0 {
		sub := layoutInput{
			known:       SizeDimension{Width: AutoDimension(), Height: AutoDimension()},
			available:   MaxContentAvailable(),
			parentStyle: cs,
		}
		// The item is measured with its RESOLVED main size as a known
		// dimension — the same constraint leaves get above. Without it
		// a wrapping subtree measures one line tall at max-content,
		// the line cross freezes at that pre-shrink height, and the
		// item's final (narrower) layout overflows the row.
		if a.mainIsWidth {
			sub.known.Width = LengthDimension(it.targetMain)
		} else {
			sub.known.Height = LengthDimension(it.targetMain)
		}
		// Containers are measured with the container's cross availability
		// (upstream child_available_cross), so nested scroll containers cap
		// the same way leaves do.
		if crossDef {
			// Keep the main axis max-content while the cross is
			// definite: a definite main of MaxFloat32 would make
			// mainDef true and flex containers would grow their
			// children into infinite free space (the tree list laid
			// out thousands of pixels tall, pushing the whole app
			// content out of the window).
			if a.mainIsWidth {
				sub.available = MaxMainDefiniteCrossAvailable(true, crossInner)
			} else {
				sub.available = MaxMainDefiniteCrossAvailable(false, crossInner)
			}
		}
		// A stretch-aligned item with an auto cross size fills the parent's
		// definite cross space; its own children must lay out in that space
		// rather than at max-content (upstream stretched measurements
		// constrain the subtree — without it a wide leaf inflates every
		// auto-sized ancestor and pushes chrome off-screen).
		if crossDef && align.Resolve(false) == AlignStretch &&
			a.crossDim(cs.Size).IsAuto() &&
			flexBasisAutoLike(cs.FlexBasis) {
			sub.stretchCross = true
			// CSS resolves the stretched cross size before laying the
			// item out, so hand the measurement a definite cross: nested
			// flex rows then shrink their children into it.
			if a.mainIsWidth {
				sub.known.Height = LengthDimension(crossInner)
			} else {
				sub.known.Width = LengthDimension(crossInner)
			}
		}
		sub.forceRelative = true
		out := compute(t, it.node, sub, false)
		v := a.cross(out.size)
		return clampInto(v, a.crossDim(cs.MinSize).MaybeResolve(crossParent, 0), a.crossDim(cs.MaxSize).ResolveLoose(crossParent))
	}
	return 0
}

func collectFlexLines(items []*item, wrap FlexWrap, gap, mainInner float32, mainDef bool, a axis) []*flexLine {
	if wrap == FlexNoWrap || !mainDef {
		return []*flexLine{{items: append([]*item(nil), items...)}}
	}
	var lines []*flexLine
	var current []*item
	currentLen := float32(0)
	for _, it := range items {
		outer := it.outerMain(a)
		if len(current) > 0 && currentLen+gap+outer > mainInner {
			lines = append(lines, &flexLine{items: current})
			current = nil
			currentLen = 0
		}
		if len(current) > 0 {
			currentLen += gap
		}
		current = append(current, it)
		currentLen += outer
	}
	if len(current) > 0 {
		lines = append(lines, &flexLine{items: current})
	}
	if len(lines) == 0 {
		lines = []*flexLine{{}}
	}
	return lines
}

// resolveFlexibleLengths ports taffy's frozen-item loop (CSS flexbox
// section 9.7). scrollableMain disables grow/shrink (scroll-container
// children overflow instead).
func resolveFlexibleLengths(line *flexLine, a axis, gap, mainInner float32, mainDef, scrollableMain bool) {
	items := line.items
	for _, it := range items {
		it.targetMain = it.flexBasis
		it.frozen = false
	}
	if !mainDef || len(items) == 0 || scrollableMain {
		return
	}

	// usedSpace sums frozen items at their (clamped) target size and
	// unfrozen items at their flex base (CSS step 3/b: "for frozen items,
	// use their outer target main size; for other items, use their outer
	// flex base size"). Recomputing against the mutated targets instead
	// made the shrink accumulate twice: the workspace row shrank in
	// iteration one, then iteration two saw the reduced free space again
	// and shrank the row by that delta a second time, overflowing the
	// window (the tree list extended past the status bar).
	usedSpace := func() float32 {
		total := -gap
		for _, it := range items {
			if it.frozen {
				total += gap + it.targetMain + a.mainEdgeSum(it.margin)
			} else {
				total += gap + it.flexBasis + a.mainEdgeSum(it.margin)
			}
		}
		return total
	}

	growing := usedSpace() < mainInner
	shrinking := usedSpace() > mainInner
	if growing == shrinking {
		// Exactly sized: freeze everything at the hypothetical size.
		for _, it := range items {
			it.frozen = true
		}
		return
	}

	// Size inflexible items (CSS step 2): zero flex factor, or a flex
	// factor pointing away from the used direction.
	for _, it := range items {
		if (growing && it.style.FlexGrow == 0 && it.style.FlexShrink == 0) ||
			(growing && it.flexBasis > it.targetMain) ||
			(shrinking && it.style.FlexGrow == 0 && it.style.FlexShrink == 0) ||
			(shrinking && it.flexBasis < it.targetMain) {
			it.frozen = true
		}
	}
	if allFrozen(items) {
		return
	}

	initialFree := mainInner - usedSpace()
	for {
		if allFrozen(items) {
			break
		}
		grow := float32(0)
		shrinkSum := float32(0)
		scaledShrink := float32(0)
		for _, it := range items {
			if it.frozen {
				continue
			}
			grow += it.style.FlexGrow
			shrinkSum += it.style.FlexShrink
			scaledShrink += it.style.FlexShrink * it.flexBasis
		}

		free := mainInner - usedSpace()
		// CSS step b: with a flex factor sum below one, only that fraction
		// of the free space is distributed.
		if growing && grow < 1 && grow > 0 {
			f := initialFree*grow - gap
			if v := mainInner - usedSpace(); f > v {
				f = v
			}
			free = f
		} else if shrinking && shrinkSum < 1 && shrinkSum > 0 {
			f := initialFree*shrinkSum - gap
			if v := mainInner - usedSpace(); f < v {
				f = v
			}
			free = f
		}

		if free > 0 && growing && grow > 0 {
			for _, it := range items {
				if it.frozen {
					continue
				}
				it.targetMain = it.flexBasis + free*(it.style.FlexGrow/grow)
			}
		} else if free < 0 && shrinking && scaledShrink > 0 {
			for _, it := range items {
				if it.frozen {
					continue
				}
				it.targetMain = it.flexBasis + free*((it.style.FlexShrink*it.flexBasis)/scaledShrink)
			}
		}

		// Fix min/max violations (CSS step d) and freeze by the total
		// violation sign (step e): min violators freeze on a positive total,
		// max violators on a negative one, everything freezes on zero.
		totalViolation := float32(0)
		violations := make([]float32, len(items))
		for i, it := range items {
			if it.frozen {
				continue
			}
			min := a.mainDim(it.style.MinSize).MaybeResolve(mainInner, it.autoMinMain)
			max := a.mainDim(it.style.MaxSize).ResolveLoose(mainInner)
			clamped := clampInto(it.targetMain, min, max)
			if clamped < 0 {
				clamped = 0
			}
			violations[i] = clamped - it.targetMain
			totalViolation += violations[i]
			it.targetMain = clamped
		}
		for i, it := range items {
			if it.frozen {
				continue
			}
			switch {
			case totalViolation > 0:
				it.frozen = violations[i] > 0
			case totalViolation < 0:
				it.frozen = violations[i] < 0
			default:
				it.frozen = true
			}
		}
	}
}

// layoutItemSubtree lays out an item's subtree given its final main size.
func (t *Tree) layoutItemSubtree(it *item, a axis, mainInner, crossInner float32, input layoutInput) {
	known := SizeDimension{Width: AutoDimension(), Height: AutoDimension()}
	if a.mainIsWidth {
		known.Width = LengthDimension(it.targetMain)
		known.Height = LengthDimension(it.finalCross)
	} else {
		known.Height = LengthDimension(it.targetMain)
		known.Width = LengthDimension(it.finalCross)
	}
	avail := input.available
	// Cross axis: the item's line gives it space.
	if a.mainIsWidth {
		avail = DefiniteAvailable(mainInner, crossInner)
	} else {
		avail = DefiniteAvailable(crossInner, mainInner)
	}
	compute(t, it.node, layoutInput{
		known:         known,
		parentStyle:   it.style,
		available:     avail,
		forceRelative: true,
		// Commit propagates down the item tree: every descendant's
		// stored layout must reflect these final sizes.
		commit: input.commit,
	}, false)
}

func alignCross(align AlignItems, it *item, available float32, a axis) (pos float32, size float32) {
	inner := it.hypotheticalCross
	switch align.Resolve(false) {
	case AlignEnd:
		return available - inner, inner
	case AlignCenter:
		return (available - inner) / 2, inner
	case AlignStretch:
		if !a.crossDim(it.style.Size).IsAuto() {
			return 0, inner
		}
		if flexBasisAutoLike(it.style.FlexBasis) {
			// Auto (or zero-basis) cross size stretches.
			stretched := available
			min := a.crossDim(it.style.MinSize).MaybeResolve(available, 0)
			max := a.crossDim(it.style.MaxSize).ResolveLoose(available)
			return 0, clampInto(stretched, min, max)
		}
		return 0, inner
	default: // start, baseline, flex-start
		return 0, inner
	}
}

func distributeCross(lines []*flexLine, alignContent AlignContent, containerCross, gap float32) {
	total := float32(0)
	for _, l := range lines {
		total += l.crossSize
	}
	total += gap * float32(len(lines)-1)
	free := containerCross - total
	if free <= 0 || len(lines) == 0 {
		return
	}
	switch alignContent {
	case AlignContentStretch:
		if total > 0 {
			for _, l := range lines {
				l.crossSize += free * (l.crossSize / total)
			}
		} else {
			for _, l := range lines {
				l.crossSize += free / float32(len(lines))
			}
		}
	case AlignContentCenter:
		if lines[0].offsetCross == 0 {
			lines[0].offsetCross = free / 2
		}
	case AlignContentEnd:
		lines[0].offsetCross = free
	case AlignContentSpaceBetween:
		for i := 1; i < len(lines); i++ {
			lines[i].offsetCross = free / float32(len(lines)-1)
		}
	case AlignContentSpaceEvenly:
		for i := range lines {
			lines[i].offsetCross = free / float32(len(lines)+1)
		}
	case AlignContentSpaceAround:
		for i := range lines {
			lines[i].offsetCross = free / float32(len(lines)) / 2
		}
	}
	// Accumulate.
	acc := float32(0)
	for _, l := range lines {
		base := l.offsetCross
		l.offsetCross = acc + base
		acc += l.crossSize + gap
	}
}

func maxf32(vals ...float32) float32 {
	m := float32(math.Inf(-1))
	for _, v := range vals {
		if v > m {
			m = v
		}
	}
	return m
}

func edgeSum3(margin, border, padding [4]float32, horizontal bool) float32 {
	if horizontal {
		return margin[1] + margin[3] + border[1] + border[3] + padding[1] + padding[3]
	}
	return margin[0] + margin[2] + border[0] + border[2] + padding[0] + padding[2]
}

func allFrozen(items []*item) bool {
	for _, it := range items {
		if !it.frozen {
			return false
		}
	}
	return true
}

// justifyDistribution returns the leading offset and inter-item spacing for
// a justify-content value with the given free space.
func justifyDistribution(justify AlignContent, free float32, count int) (offset float32, spacing float32) {
	if free <= 0 || count == 0 {
		return 0, 0
	}
	switch justify {
	case AlignContentCenter:
		return free / 2, 0
	case AlignContentEnd:
		return free, 0
	case AlignContentSpaceBetween:
		return 0, free / float32(count-1)
	case AlignContentSpaceEvenly:
		return free / float32(count+1), free / float32(count+1)
	case AlignContentSpaceAround:
		return free / float32(count) / 2, free / float32(count)
	default:
		return 0, 0
	}
}
