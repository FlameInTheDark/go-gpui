package render

import (
	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
)

// This file converts a Scene into the GPU frame structure the backends draw.
// The architecture mirrors upstream gpui_wgpu's wgpu_renderer.rs:
//
//   - every primitive kind has its own instance list, drawn by its own
//     pipeline (one uber-shader, per-kind entry points);
//   - fully opaque, unshaped quads are additionally drawn by an OPAQUE PRE-PASS
//     (no blending, depth write) before the blended batches, so transparent
//     fragments behind them are depth-rejected exactly like upstream;
//   - the primitive lists are sorted and then interleaved into BATCHES in the
//     (order, kind) sequence the software rasterizer paints; each non-quad
//     batch draws at the fixed depth of the quad cursor at that point;
//   - paths are NOT baked into atlas tiles: they are binned into PathTiles
//     (path_winding.rs port) and rasterized per-pixel on the GPU (Slug).
//
// The per-pixel math itself (SDF quads, borders, shadows, gradients) lives in
// the shader that consumes these instances; the layout below is the shared
// contract between BuildGpuFrame and the backend shaders
// (platform/windows/d3d11.go).

// GpuPrim discriminates the shader path (Misc2[0]).
const (
	GpuPrimQuad = 0
	// GpuPrimShadow paints an (optionally blurred) shadow SDF.
	GpuPrimShadow = 1
	// GpuPrimUnderlineSolid fills its bounds with color (alpha pre-squared
	// on the CPU side, matching the software renderer).
	GpuPrimUnderlineSolid = 2
	// GpuPrimUnderlineWavy paints the sine-wave underline SDF.
	GpuPrimUnderlineWavy = 3
	// GpuPrimSprite samples an atlas tile.
	GpuPrimSprite = 4
)

// GpuTexKind selects the sprite texture (Misc2[3]).
const (
	GpuTexNone = 0
	GpuTexMono = 1
	// GpuTexPoly samples the polychrome atlas (images, baked paths).
	GpuTexPoly = 2
	// GpuTexSubpixel samples the subpixel atlas (ClearType RGB coverage).
	GpuTexSubpixel = 3
)

// GpuInstance is one instanced quad: 15 float4 rows (240 bytes, matching
// the HLSL Instance struct). Field meanings by row:
//
//	 0 Bounds          x, y, w, h                (device px)
//	 1 ContentMask     x0, y0, x1, y1            (device px, inclusive)
//	 2 Radii           top-left, top-right, bottom-right, bottom-left
//	 3 BorderWidths    top, right, bottom, left
//	 4 Color0          background color or gradient stop 0 (straight RGBA)
//	 5 Color1          gradient stop 1 (straight RGBA)
//	 6 BorderColor     straight RGBA
//	 7 ShapeBounds     shadow shape x, y, w, h
//	 8 ShapeRadii      shadow shape corner radii
//	 9 UVMat row 0     u = a*px + b*py + c
//	10 UVMat row 1     v = d*px + e*py + f
//	11 Misc            backgroundTag, angleOrPatternParam, blurRadius, inset
//	12 Misc2           primType, borderStyle, underlineThickness, texKind
//	13 Misc3           texIndex, opacity, clipRounded, grayscale
//	14 Misc4           colorSpace, stop0Pct, stop1Pct, unused
type GpuInstance [15][4]float32

const gpuInstanceStride = len(GpuInstance{}) * 4 * 4 // 240

// GpuBatchKind identifies one draw pipeline.
type GpuBatchKind uint8

const (
	GpuBatchQuads GpuBatchKind = iota
	GpuBatchShadows
	GpuBatchPaths
	GpuBatchUnderlines
	GpuBatchMono
	GpuBatchSub
	GpuBatchPoly
)

// GpuBatch is one contiguous instance range of one kind in the draw sequence.
// Start/Count index into the kind's instance list. Depth is the fixed
// fragment depth for non-quad batches (upstream: the quad cursor depth at
// that point in the batch sequence, applied via the viewport depth range);
// quad batches use the full depth range and per-quad depths.
type GpuBatch struct {
	Kind  GpuBatchKind
	Start uint32
	Count uint32
	Depth float32
}

// GpuFrame is the GPU-ready representation of one scene (upstream's
// write_instances + batches() result).
type GpuFrame struct {
	Width, Height int
	// QuadCount matches len(Quads): the opaque pre-pass reads quads reversed
	// (quadCount-1-instance) and culls non-opaque ones in the vertex shader.
	QuadCount  int
	DepthScale float32

	Quads     []GpuInstance
	Shadows   []GpuInstance
	Underines []GpuInstance
	Mono      []GpuInstance
	Sub       []GpuInstance
	Poly      []GpuInstance

	// Paths: tiles binned per path (upstream PathTiles), with Paint indices
	// into PathPaints.
	PathTiles  []PathTileGpu
	PathCurves []PathCurve
	PathPaints []PathPaintGpu

	Batches []GpuBatch
}

// BuildGpuFrame converts a scene into the GPU frame structure.
func BuildGpuFrame(scene *Scene, atlas *Atlas) GpuFrame {
	f := GpuFrame{}
	if scene == nil || scene.Empty() {
		return f
	}
	indices := scene.SortedIndices()
	f.Quads = make([]GpuInstance, len(scene.Quads))
	for i := range scene.Quads {
		if inst, ok := gpuQuad(scene.Quads[i]); ok {
			f.Quads[i] = inst
		}
	}
	f.QuadCount = len(f.Quads)
	f.DepthScale = quadDepthScale(len(f.Quads))

	// Batches: walk the interleaved (order, kind) sequence, coalescing
	// contiguous same-kind ranges, tracking the quad cursor for depths.
	var cur *GpuBatch
	quadCursor := 0
	appendBatch := func(kind GpuBatchKind, seqIdx int) {
		if cur != nil && cur.Kind == kind {
			cur.Count++
			return
		}
		f.Batches = append(f.Batches, GpuBatch{Kind: kind, Start: 0, Count: 1})
		cur = &f.Batches[len(f.Batches)-1]
		_ = seqIdx
	}
	for _, idx := range indices {
		var kind GpuBatchKind
		switch idx.kind {
		case kindShadow:
			kind = GpuBatchShadows
			inst, ok := gpuShadow(scene.Shadows[idx.index])
			if ok {
				f.Shadows = append(f.Shadows, inst)
				appendBatch(kind, idx.index)
			}
		case kindQuad:
			kind = GpuBatchQuads
			quadCursor++
			appendBatch(kind, idx.index)
		case kindPath:
			kind = GpuBatchPaths
			n := f.appendPathTiles(scene.Paths[idx.index], atlas)
			if n > 0 {
				if cur != nil && cur.Kind == kind {
					cur.Count += uint32(n)
				} else {
					f.Batches = append(f.Batches, GpuBatch{Kind: kind, Start: uint32(len(f.PathTiles) - n), Count: uint32(n)})
					cur = &f.Batches[len(f.Batches)-1]
				}
			}
		case kindUnder:
			kind = GpuBatchUnderlines
			inst, ok := gpuUnderline(scene.Underlines[idx.index])
			if ok {
				f.Underines = append(f.Underines, inst)
				appendBatch(kind, idx.index)
			}
		case kindMono:
			kind = GpuBatchMono
			inst, ok := gpuMonoSprite(scene.MonoSprites[idx.index])
			if ok {
				f.Mono = append(f.Mono, inst)
				appendBatch(kind, idx.index)
			}
		case kindSub:
			kind = GpuBatchSub
			inst, ok := gpuSubpixelSprite(scene.SubpixelSprites[idx.index])
			if ok {
				f.Sub = append(f.Sub, inst)
				appendBatch(kind, idx.index)
			}
		case kindPoly:
			kind = GpuBatchPoly
			inst, ok := gpuPolySprite(scene.PolySprites[idx.index])
			if ok {
				f.Poly = append(f.Poly, inst)
				appendBatch(kind, idx.index)
			}
		}
	}
	// Fixed depths for non-quad batches from the quad cursor at batch start
	// (upstream record_frame: quad_depth(quad_cursor, depth_scale)).
	quadCursor = 0
	for i := range f.Batches {
		if f.Batches[i].Kind == GpuBatchQuads {
			quadCursor += int(f.Batches[i].Count)
			continue
		}
		f.Batches[i].Depth = quadDepth(uint32(quadCursor), f.DepthScale)
	}
	return f
}

// appendPathTiles bins one painted path into GPU tiles and appends its paint
// entry. Returns the number of tiles added (upstream PaintedPath::new +
// record_frame's tile accounting).
func (f *GpuFrame) appendPathTiles(p PaintedPath, atlas *Atlas) int {
	if p.Path == nil || len(p.Path.Curves) == 0 {
		return 0
	}
	// Path curves are stored in scaled (device) pixels already - the same
	// space the CPU rasterizer and PaintedPath.Bounds use - so binning runs
	// at scale 1 (upstream scales inside bin() because it bins logical
	// PathBuilder output).
	binned := BinPathTiles(p.Path.Curves, p.Path.EvenOdd, 1.0)
	if len(binned.Tiles) == 0 {
		return 0
	}
	// Drop tiles entirely outside the content mask (upstream retain_visible).
	mask := p.ContentMask.Bounds
	mLeft, mTop := mask.Origin.X.AsF32(), mask.Origin.Y.AsF32()
	mRight, mBottom := mask.Right().AsF32(), mask.Bottom().AsF32()
	visible := binned.Tiles[:0]
	for _, t := range binned.Tiles {
		if t.TopLeft[0] < mRight && t.TopLeft[1] < mBottom &&
			t.BottomRight[0] > mLeft && t.BottomRight[1] > mTop {
			visible = append(visible, t)
		}
	}
	if len(visible) == 0 {
		return 0
	}
	paintIndex := uint32(len(f.PathPaints))
	f.PathPaints = append(f.PathPaints, buildPathPaint(p))
	curveBase := uint32(len(f.PathCurves))
	f.PathCurves = append(f.PathCurves, binned.Curves...)
	for i := range visible {
		visible[i].Paint = paintIndex
		visible[i].CurveStart += curveBase
	}
	f.PathTiles = append(f.PathTiles, visible...)
	return len(visible)
}

// buildPathPaint packs a PaintedPath's fill into a PathPaintGpu (colors
// pre-converted exactly like the quad background path).
func buildPathPaint(p PaintedPath) PathPaintGpu {
	var out PathPaintGpu
	b := p.Bounds
	out.Bounds = [4]float32{b.Origin.X.AsF32(), b.Origin.Y.AsF32(), b.Size.Width.AsF32(), b.Size.Height.AsF32()}
	m := p.ContentMask
	if m.Bounds.Size.Width.AsF32() < 1e8 {
		out.ContentMask = [4]float32{m.Bounds.Origin.X.AsF32(), m.Bounds.Origin.Y.AsF32(),
			m.Bounds.Right().AsF32(), m.Bounds.Bottom().AsF32()}
	} else {
		out.ContentMask = [4]float32{-1e9, -1e9, 1e9, 1e9}
	}
	prepared := prepareBackground(p.Color)
	out.Color0 = rgbaRow(prepared.color0)
	out.Color1 = rgbaRow(prepared.color1)
	out.Tag = uint32(p.Color.Tag)
	out.ColorSpace = uint32(p.Color.ColorSpace)
	out.Pct0 = p.Color.Colors[0].Percentage
	out.Pct1 = p.Color.Colors[1].Percentage
	out.Angle = p.Color.GradientAngleOrPatternHeight
	if p.Path.EvenOdd {
		out.EvenOdd = 1
	}
	return out
}

// quadDepthScale ports upstream quad_depth_scale.
func quadDepthScale(quadCount int) float32 {
	denominator := quadCount
	if denominator > (1<<24)-1 {
		denominator = (1 << 24) - 1
	}
	denominator++ // +1
	// next power of two
	pow := 1
	for pow < denominator {
		pow <<= 1
	}
	return 1.0 / float32(pow)
}

// quadDepth ports upstream quad_depth: greater = closer.
func quadDepth(quadID uint32, scale float32) float32 {
	d := (float32(quadID) + 1.0) * scale
	if d > 1.0 {
		d = 1.0
	}
	return d
}

// IsOpaqueRectangle reports whether a quad is drawn by the opaque pre-pass
// (upstream is_opaque_rectangle).
func isOpaqueRectangle(q Quad) bool {
	return q.Background.Tag == gc.BackgroundTagSolid &&
		q.Background.Solid.A >= 1.0 &&
		q.CornerRadii.TopLeft.AsF32() == 0 && q.CornerRadii.TopRight.AsF32() == 0 &&
		q.CornerRadii.BottomRight.AsF32() == 0 && q.CornerRadii.BottomLeft.AsF32() == 0 &&
		q.BorderWidths.Top.AsF32() == 0 && q.BorderWidths.Right.AsF32() == 0 &&
		q.BorderWidths.Bottom.AsF32() == 0 && q.BorderWidths.Left.AsF32() == 0
}

func setBounds(inst *GpuInstance, b geometry.Bounds[geometry.ScaledPixels]) {
	inst[0][0] = b.Origin.X.AsF32()
	inst[0][1] = b.Origin.Y.AsF32()
	inst[0][2] = b.Size.Width.AsF32()
	inst[0][3] = b.Size.Height.AsF32()
}

func setMask(inst *GpuInstance, mask ContentMask) {
	if mask.Bounds.Size.Width.AsF32() < 1e8 {
		inst[1][0] = mask.Bounds.Origin.X.AsF32()
		inst[1][1] = mask.Bounds.Origin.Y.AsF32()
		inst[1][2] = mask.Bounds.Right().AsF32()
		inst[1][3] = mask.Bounds.Bottom().AsF32()
	} else {
		inst[1][0] = -1e9
		inst[1][1] = -1e9
		inst[1][2] = 1e9
		inst[1][3] = 1e9
	}
}

// gpuQuad packs a quad primitive. Colors are pre-converted exactly like
// prepareBackground so the shader only interpolates.
func gpuQuad(q Quad) (GpuInstance, bool) {
	if q.Bounds.Size.Width.AsF32() <= 0 || q.Bounds.Size.Height.AsF32() <= 0 {
		return GpuInstance{}, false
	}
	var inst GpuInstance
	setBounds(&inst, q.Bounds)
	setMask(&inst, q.ContentMask)
	inst[2] = [4]float32{q.CornerRadii.TopLeft.AsF32(), q.CornerRadii.TopRight.AsF32(),
		q.CornerRadii.BottomRight.AsF32(), q.CornerRadii.BottomLeft.AsF32()}
	inst[3] = [4]float32{q.BorderWidths.Top.AsF32(), q.BorderWidths.Right.AsF32(),
		q.BorderWidths.Bottom.AsF32(), q.BorderWidths.Left.AsF32()}
	prepared := prepareBackground(q.Background)
	inst[4] = rgbaRow(prepared.color0)
	inst[5] = rgbaRow(prepared.color1)
	inst[6] = rgbaRow(hslaToRgbaF(q.BorderColor))
	inst[11] = [4]float32{float32(q.Background.Tag), q.Background.GradientAngleOrPatternHeight, 0, 0}
	inst[12] = [4]float32{GpuPrimQuad, float32(q.BorderStyle), 0, 0}
	inst[14] = [4]float32{float32(q.Background.ColorSpace),
		q.Background.Colors[0].Percentage, q.Background.Colors[1].Percentage, 0}
	return inst, true
}

func gpuShadow(sh Shadow) (GpuInstance, bool) {
	if sh.Bounds.Size.Width.AsF32() <= 0 || sh.Bounds.Size.Height.AsF32() <= 0 {
		return GpuInstance{}, false
	}
	var inst GpuInstance
	setBounds(&inst, sh.Bounds)
	setMask(&inst, sh.ContentMask)
	inst[7] = [4]float32{sh.ShapeBounds.Origin.X.AsF32(), sh.ShapeBounds.Origin.Y.AsF32(),
		sh.ShapeBounds.Size.Width.AsF32(), sh.ShapeBounds.Size.Height.AsF32()}
	inst[8] = [4]float32{sh.ShapeRadii.TopLeft.AsF32(), sh.ShapeRadii.TopRight.AsF32(),
		sh.ShapeRadii.BottomRight.AsF32(), sh.ShapeRadii.BottomLeft.AsF32()}
	inst[4] = rgbaRow(hslaToRgbaF(sh.Color))
	inset := float32(0)
	if sh.Inset {
		inset = 1
	}
	inst[2] = [4]float32{sh.CornerRadii.TopLeft.AsF32(), sh.CornerRadii.TopRight.AsF32(),
		sh.CornerRadii.BottomRight.AsF32(), sh.CornerRadii.BottomLeft.AsF32()}
	inst[11] = [4]float32{0, 0, sh.BlurRadius.AsF32(), inset}
	inst[12] = [4]float32{GpuPrimShadow, 0, 0, 0}
	return inst, true
}

func gpuUnderline(u Underline) (GpuInstance, bool) {
	if u.Bounds.Size.Width.AsF32() <= 0 || u.Bounds.Size.Height.AsF32() <= 0 {
		return GpuInstance{}, false
	}
	var inst GpuInstance
	setBounds(&inst, u.Bounds)
	setMask(&inst, u.ContentMask)
	// The software renderer fills solid underlines with squared alpha.
	c := hslaToRgbaF(u.Color)
	c.A *= c.A
	inst[4] = rgbaRow(c)
	prim := GpuPrimUnderlineSolid
	if u.Wavy {
		prim = GpuPrimUnderlineWavy
	}
	inst[12] = [4]float32{float32(prim), 0, u.Thickness.AsF32(), 0}
	return inst, true
}

func rgbaRow(c rgbaF) [4]float32 { return [4]float32{c.R, c.G, c.B, c.A} }

// uvMatrix folds the chain device px -> (optional inverse transform) ->
// sprite-local uv -> atlas tile uv into one 2x3 matrix.
func uvMatrix(bounds geometry.Bounds[geometry.ScaledPixels], tile AtlasTile,
	transform geometry.TransformationMatrix) (row0, row1 [4]float32, ok bool) {
	bw := bounds.Size.Width.AsF32()
	bh := bounds.Size.Height.AsF32()
	if bw == 0 || bh == 0 {
		return row0, row1, false
	}
	tw := float32(tile.Bounds.Size.Width.AsI32())
	th := float32(tile.Bounds.Size.Height.AsI32())
	tx := float32(tile.Bounds.Origin.X.AsI32())
	ty := float32(tile.Bounds.Origin.Y.AsI32())
	tex := float32(AtlasTextureSize)
	inv := invertMatrix(transform)
	m := inv.RotationScale
	t := inv.Translation
	bx := bounds.Origin.X.AsF32()
	by := bounds.Origin.Y.AsF32()
	// u = (m00*px + m01*py + t0 - bx)/bw * tw + tx, all over tex.
	row0 = [4]float32{
		m[0][0] * tw / (bw * tex),
		m[0][1] * tw / (bw * tex),
		((t[0]-bx)/bw*tw + tx) / tex,
		0,
	}
	row1 = [4]float32{
		m[1][0] * th / (bh * tex),
		m[1][1] * th / (bh * tex),
		((t[1]-by)/bh*th + ty) / tex,
		0,
	}
	return row0, row1, true
}

func gpuMonoSprite(sp MonochromeSprite) (GpuInstance, bool) {
	return gpuSpriteCommon(sp.Bounds, sp.ContentMask, sp.Tile, sp.Color, sp.Transform, GpuTexMono, 1)
}

func gpuSubpixelSprite(sp SubpixelSprite) (GpuInstance, bool) {
	return gpuSpriteCommon(sp.Bounds, sp.ContentMask, sp.Tile, sp.Color, geometry.IdentityMatrix(), GpuTexSubpixel, 1)
}

func gpuPolySprite(sp PolychromeSprite) (GpuInstance, bool) {
	clip := float32(0)
	if sp.CornerRadii.TopLeft.AsF32() > 0 || sp.CornerRadii.TopRight.AsF32() > 0 ||
		sp.CornerRadii.BottomRight.AsF32() > 0 || sp.CornerRadii.BottomLeft.AsF32() > 0 {
		clip = 1
	}
	gray := float32(0)
	if sp.Grayscale {
		gray = 1
	}
	inst, ok := gpuSpriteCommon(sp.Bounds, sp.ContentMask, sp.Tile, gc.Hsla{A: 1}, geometry.IdentityMatrix(), GpuTexPoly, sp.Opacity)
	if !ok {
		return GpuInstance{}, false
	}
	inst[2] = [4]float32{sp.CornerRadii.TopLeft.AsF32(), sp.CornerRadii.TopRight.AsF32(),
		sp.CornerRadii.BottomRight.AsF32(), sp.CornerRadii.BottomLeft.AsF32()}
	inst[13][2] = clip
	inst[13][3] = gray
	return inst, true
}

func gpuSpriteCommon(bounds geometry.Bounds[geometry.ScaledPixels], mask ContentMask, tile AtlasTile,
	color gc.Hsla, transform geometry.TransformationMatrix, texKind float32, opacity float32) (GpuInstance, bool) {
	if bounds.Size.Width.AsF32() <= 0 || bounds.Size.Height.AsF32() <= 0 {
		return GpuInstance{}, false
	}
	if tile.Bounds.Size.Width.AsI32() <= 0 || tile.Bounds.Size.Height.AsI32() <= 0 {
		return GpuInstance{}, false
	}
	var inst GpuInstance
	setBounds(&inst, bounds)
	setMask(&inst, mask)
	row0, row1, ok := uvMatrix(bounds, tile, transform)
	if !ok {
		return GpuInstance{}, false
	}
	inst[9] = row0
	inst[10] = row1
	inst[4] = rgbaRow(hslaToRgbaF(color))
	inst[12] = [4]float32{GpuPrimSprite, 0, 0, texKind}
	inst[13] = [4]float32{float32(tile.TextureIndex), opacity, 0, 0}
	return inst, true
}

// pathScratch is the winding scratch rasterizer (windingAt uses no state).
var pathScratch = &Rasterizer{}

func init() { _ = pathScratch }
