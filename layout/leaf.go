package layout

import "math"

// computeLeaf ports taffy's compute_leaf_layout: applies padding/border,
// aspect ratio and min/max sizes to a childless node, then defers to its
// measure function (or zero) to determine the content size.
//
// A definite input.available is treated as the space available to the node's
// border box; the caller is expected to have already subtracted the node's
// margins from it.
func (t *Tree) computeLeaf(node Node, style *Style, input layoutInput) layoutOutput {
	// Percentage resolution basis (taffy uses the parent's inner size; we
	// approximate with the definite available width).
	parentW := float32(0)
	if v, ok := input.available.IsDefiniteAxis(true); ok {
		parentW = v
	}

	padding := style.Padding.Resolve(parentW)
	border := style.Border.Resolve(parentW)
	pbW := padding[1] + padding[3] + border[1] + border[3]
	pbH := padding[0] + padding[2] + border[0] + border[2]

	// Known dimensions from the parent override the node's own style size.
	knownW, knownWDef := input.known.Width.Resolve(parentW)
	knownH, knownHDef := input.known.Height.Resolve(parentW)
	styleW, styleWDef := style.Size.Width.Resolve(parentW)
	styleH, styleHDef := style.Size.Height.Resolve(parentW)

	sizeW, sizeWDef := knownW, knownWDef
	if !sizeWDef {
		sizeW, sizeWDef = styleW, styleWDef
	}
	sizeH, sizeHDef := knownH, knownHDef
	if !sizeHDef {
		sizeH, sizeHDef = styleH, styleHDef
	}

	minW := style.MinSize.Width.MaybeResolve(parentW, 0)
	maxW := style.MaxSize.Width.ResolveLoose(parentW)
	minH := style.MinSize.Height.MaybeResolve(parentW, 0)
	maxH := style.MaxSize.Height.ResolveLoose(parentW)

	// Early return when both dimensions are already known.
	if sizeWDef && sizeHDef {
		w := clampInto(clampInto(sizeW, minW, maxW), pbW, math.MaxFloat32)
		h := clampInto(clampInto(sizeH, minH, maxH), pbH, math.MaxFloat32)
		t.layouts[node] = Layout{Order: nextOrder(), Width: w, Height: h,
			ScrollWidth: w, ScrollHeight: h}
		return layoutOutput{size: SizeResult{Width: w, Height: h}}
	}

	// Available space for the measure: known dimensions, then the style
	// size, then the passed space; definite values are clamped to min/max
	// and reduced to the content box.
	avail := input.available
	if knownWDef {
		avail = DefiniteAvailable(knownW, avail.Height)
	} else if styleWDef {
		avail = DefiniteAvailable(styleW, avail.Height)
	}
	if knownHDef {
		avail = DefiniteAvailable(avail.Width, knownH)
	} else if styleHDef {
		avail = DefiniteAvailable(avail.Width, styleH)
	}
	if v, ok := avail.IsDefiniteAxis(true); ok {
		avail = DefiniteAvailable(clampInto(v, minW, maxW)-pbW, avail.Height)
	}
	if v, ok := avail.IsDefiniteAxis(false); ok {
		avail = DefiniteAvailable(avail.Width, clampInto(v, minH, maxH)-pbH)
	}

	// Measure the node with the known dimensions (auto where unknown).
	var measured SizeResult
	if t.measure[node] != nil {
		measured = t.measure[node](input.known, avail)
	}

	// Final size: known/style size, else measured + padding/border.
	w, h := sizeW, sizeH
	if !sizeWDef {
		w = measured.Width + pbW
	}
	if !sizeHDef {
		h = measured.Height + pbH
	}
	w = clampInto(w, minW, maxW)
	h = clampInto(h, minH, maxH)
	if ar := style.AspectRatio; ar != nil && *ar != 0 {
		if arH := w / *ar; h < arH {
			h = arH
		}
	}
	if w < pbW {
		w = pbW
	}
	if h < pbH {
		h = pbH
	}

	// A scroll container's own trailing padding is part of its scrollable
	// overflow region (taffy scrollable_overflow_rect).
	scrollW, scrollH := w, h
	if style.Overflow[0] != OverflowVisible || style.Overflow[1] != OverflowVisible {
		scrollW = padding[3] + measured.Width + padding[1]
		scrollH = padding[0] + measured.Height + padding[2]
	}

	t.layouts[node] = Layout{Order: nextOrder(), Width: w, Height: h,
		ScrollWidth: scrollW, ScrollHeight: scrollH}
	return layoutOutput{size: SizeResult{Width: w, Height: h}}
}
