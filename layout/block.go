package layout

// computeBlock ports taffy's block layout (compute/block.rs), condensed:
// children stack vertically; horizontal geometry follows margins/size;
// auto width stretches to the container; auto height is the content sum.
func (t *Tree) computeBlock(node Node, style *Style, input layoutInput) layoutOutput {
	input = input.withRootDefaults(style)
	containerW, wDef := resolveContainerAxis(input, style.Size.Width, style.MinSize.Width, style.MaxSize.Width, true)
	containerH, hDef := resolveContainerAxis(input, style.Size.Height, style.MinSize.Height, style.MaxSize.Height, false)

	margin := style.Margin.Resolve(containerW)
	border := style.Border.Resolve(containerW)
	padding := style.Padding.Resolve(containerW)

	innerW := containerW - edgeSum3(margin, border, padding, true)
	if !wDef {
		innerW = 0
	}
	if innerW < 0 {
		innerW = 0
	}

	var inFlow []Node
	for _, child := range t.children[node] {
		cs := &t.styles[child]
		if cs.Display == DisplayNone {
			t.layouts[child] = Layout{Order: nextOrder()}
			continue
		}
		if cs.Position == PositionAbsolute {
			continue
		}
		inFlow = append(inFlow, child)
	}

	y := border[0] + padding[0]
	contentH := float32(0)
	maxChildRight := float32(0)
	for _, child := range inFlow {
		cs := &t.styles[child]
		cm := cs.Margin.Resolve(containerW)
		// Child width: style width, else stretch to the inner width.
		childW, childWDef := cs.Size.Width.Resolve(innerW)
		if !childWDef {
			childW = innerW - cm[1] - cm[3]
			childW = clampInto(childW, cs.MinSize.Width.MaybeResolve(innerW, 0), cs.MaxSize.Width.ResolveLoose(innerW))
		}
		// Vertical available space in block flow is indefinite (taffy
		// block.rs: AvailableSpace::MaxContent); width is the inner width.
		avail := DefiniteAvailable(innerW, 0).WithHeightMaxContent()
		out := compute(t, child, layoutInput{
			known:       SizeDimension{Width: LengthDimension(childW), Height: cs.Size.Height},
			parentStyle: style,
			available:   avail,
			commit:      input.commit,
		}, false)
		cl := t.layouts[child]
		cl.Order = nextOrder()
		cl.X = border[3] + padding[3] + cm[3]
		cl.Y = y + cm[0]
		t.layouts[child] = cl
		childH := out.size.Height
		y += cm[0] + childH + cm[2]
		contentH += cm[0] + childH + cm[2]
		if right := cl.X + out.size.Width + cm[1]; right > maxChildRight {
			maxChildRight = right
		}
	}

	innerH := containerH - edgeSum3(margin, border, padding, false)
	if !hDef {
		innerH = contentH
	}
	if innerH < 0 {
		innerH = 0
	}

	w := innerW + edgeSum3(margin, border, padding, true)
	h := innerH + edgeSum3(margin, border, padding, false)
	if vw, ok := style.Size.Width.Resolve(containerW); ok && !wDef {
		w = vw + margin[1] + margin[3]
	}
	if vh, ok := style.Size.Height.Resolve(containerH); ok && !hDef {
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

	t.layouts[node] = Layout{
		Order: nextOrder(), Width: w, Height: h,
		ScrollWidth:  maxChildRight,
		ScrollHeight: contentH,
	}

	for _, child := range t.children[node] {
		cs := &t.styles[child]
		if cs.Position == PositionAbsolute && cs.Display != DisplayNone {
			t.layoutAbsolute(child, cs, input.available, innerW, innerH, border, padding, input.commit)
		}
	}
	return layoutOutput{size: SizeResult{Width: w, Height: h}}
}

// layoutAbsolute positions an absolutely-positioned child inside a
// container's padding box, following taffy's oof (out-of-flow) module in
// condensed form: both-side insets win, then size, then shrink-to-fit.
//
// CSS containing block semantics: the insets resolve against the PADDING
// box, so converting them to the border-box coordinates children are
// stored in means adding the container's border+padding offset. The
// static-position fallback below already did this; the inset branches
// must too, or an Absolute().Left(16) inside an 8px-padded field lands 8px
// left of the text (the caret ghosting in the settings inputs).
func (t *Tree) layoutAbsolute(child Node, cs *Style, available AvailableSpace, innerW, innerH float32, border, padding [4]float32, commit bool) {
	// Padding-box size (content + the container's own padding).
	pbW := padding[3] + innerW + padding[1]
	pbH := padding[0] + innerH + padding[2]
	parentW := pbW
	if v, ok := available.IsDefiniteAxis(true); ok && parentW == 0 {
		parentW = v
	}
	parentH := pbH
	if v, ok := available.IsDefiniteAxis(false); ok && parentH == 0 {
		parentH = v
	}

	inset := cs.Inset.Resolve(parentW)
	w, wDef := cs.Size.Width.Resolve(parentW)
	h, hDef := cs.Size.Height.Resolve(parentW)

	// Both-side insets make the size definite.
	l, lDef := inset[3], cs.Inset.Left.kind != 0
	r, rDef := inset[1], cs.Inset.Right.kind != 0
	tp, tDef := inset[0], cs.Inset.Top.kind != 0
	b, bDef := inset[2], cs.Inset.Bottom.kind != 0

	if lDef && rDef {
		w, wDef = parentW-l-r, true
	}
	if tDef && bDef {
		h, hDef = parentH-tp-b, true
	}

	// Shrink-to-fit when the width is not definite: min(max-content, max(
	// min-content, available)) approximated by max-content clamped. The
	// measurement passes forceRelative: without it compute() would route
	// the still-absolute node back here, recursing forever.
	if !wDef {
		out := compute(t, child, layoutInput{
			known:         SizeDimension{Width: AutoDimension(), Height: LengthOrAuto(h, hDef)},
			parentStyle:   cs,
			available:     DefiniteAvailable(parentW, parentH),
			forceRelative: true,
			commit:        commit,
		}, false)
		w = out.size.Width
		w = clampInto(w, cs.MinSize.Width.MaybeResolve(parentW, 0), cs.MaxSize.Width.ResolveLoose(parentW))
		if lDef {
			w = clampInto(w, 0, parentW-l)
		} else if rDef {
			w = clampInto(w, 0, parentW-r)
		}
	}
	if !hDef {
		out := compute(t, child, layoutInput{
			known:         SizeDimension{Width: LengthDimension(w), Height: AutoDimension()},
			parentStyle:   cs,
			available:     DefiniteAvailable(w, parentH),
			forceRelative: true,
			commit:        commit,
		}, false)
		h = out.size.Height
		h = clampInto(h, cs.MinSize.Height.MaybeResolve(parentH, 0), cs.MaxSize.Height.ResolveLoose(parentH))
	}

	w = clampInto(w, cs.MinSize.Width.MaybeResolve(parentW, 0), cs.MaxSize.Width.ResolveLoose(parentW))
	h = clampInto(h, cs.MinSize.Height.MaybeResolve(parentH, 0), cs.MaxSize.Height.ResolveLoose(parentH))

	// Final layout with definite size.
	compute(t, child, layoutInput{
		known:         SizeDimension{Width: LengthDimension(w), Height: LengthDimension(h)},
		parentStyle:   cs,
		available:     DefiniteAvailable(w, h),
		forceRelative: true,
		commit:        commit,
	}, false)

	cm := cs.Margin.Resolve(parentW)

	// Position: static position is the container's content origin; insets
	// resolve inside the padding box and convert to border-box
	// coordinates via the border+padding origin.
	x := border[3] + padding[3] + cm[3]
	y := border[0] + padding[0] + cm[0]
	if lDef {
		x = border[3] + padding[3] + l + cm[3]
	} else if rDef {
		x = border[3] + pbW - r - w - cm[1]
	}
	if tDef {
		y = border[0] + padding[0] + tp + cm[0]
	} else if bDef {
		y = border[0] + pbH - b - h - cm[2]
	}

	cl := t.layouts[child]
	cl.Order = nextOrder()
	cl.X = x
	cl.Y = y
	cl.Width = w
	cl.Height = h
	t.layouts[child] = cl
}

func LengthOrAuto(v float32, def bool) Dimension {
	if def {
		return LengthDimension(v)
	}
	return AutoDimension()
}

// ComputeLayoutWithRoot runs layout treating the given node as a root whose
// position is (0,0).
func (t *Tree) ComputeLayoutWithRoot(root Node, available AvailableSpace) {
	t.ComputeLayout(root, available)
	l := t.layouts[root]
	l.X, l.Y = 0, 0
	t.layouts[root] = l
}
