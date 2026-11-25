// Package render implements go-gpui's scene primitives, sprite atlas, path
// and gradient support, and a pure-Go software rasterizer. A Scene collects
// primitives for presentation by a platform backend. Most applications use
// the window and elements packages rather than creating scenes directly;
// this package is useful for custom renderers and backend implementations.
package render

import (
	"sort"

	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
)

// ContentMask is the clip rectangle every primitive intersects with.
type ContentMask struct {
	Bounds geometry.Bounds[geometry.ScaledPixels]
}

// FullContentMask covers everything.
func FullContentMask() ContentMask {
	return ContentMask{Bounds: geometry.Bounds[geometry.ScaledPixels]{
		Origin: geometry.Point[geometry.ScaledPixels]{X: 0, Y: 0},
		Size:   geometry.Size[geometry.ScaledPixels]{Width: geometry.ScaledPx(1e9), Height: geometry.ScaledPx(1e9)},
	}}
}

// Quad is a filled, optionally rounded and bordered rectangle.
type Quad struct {
	Order        uint32
	Bounds       geometry.Bounds[geometry.ScaledPixels]
	ContentMask  ContentMask
	Background   gc.Background
	BorderColor  gc.Hsla
	CornerRadii  geometry.Corners[geometry.ScaledPixels]
	BorderWidths geometry.Edges[geometry.ScaledPixels]
	BorderStyle  uint8 // 0 solid, 1 dashed
}

// Shadow is a (possibly blurred, possibly inset) rounded-rect shadow.
type Shadow struct {
	Order       uint32
	BlurRadius  geometry.ScaledPixels
	Bounds      geometry.Bounds[geometry.ScaledPixels] // painted extent
	CornerRadii geometry.Corners[geometry.ScaledPixels]
	ContentMask ContentMask
	Color       gc.Hsla
	ShapeBounds geometry.Bounds[geometry.ScaledPixels]
	ShapeRadii  geometry.Corners[geometry.ScaledPixels]
	Inset       bool
}

// Underline is a straight or wavy text decoration.
type Underline struct {
	Order       uint32
	Bounds      geometry.Bounds[geometry.ScaledPixels]
	ContentMask ContentMask
	Color       gc.Hsla
	Thickness   geometry.ScaledPixels
	Wavy        bool
}

// AtlasTile identifies a tile in the sprite atlas. Offset is the content
// offset (glyph bearing) in device pixels, stored so cache hits keep it
// (upstream AtlasTile::offset).
type AtlasTile struct {
	TextureIndex uint32
	TileID       uint32
	Bounds       geometry.Bounds[geometry.DevicePixels]
	Offset       geometry.Point[geometry.DevicePixels]
}

// MonochromeSprite is an alpha-mask sprite tinted with a color (glyphs,
// SVG masks).
type MonochromeSprite struct {
	Order       uint32
	Tile        AtlasTile
	Bounds      geometry.Bounds[geometry.ScaledPixels]
	ContentMask ContentMask
	Color       gc.Hsla
	Transform   geometry.TransformationMatrix
}

// SubpixelSprite is an RGB-coverage sprite (ClearType). The software
// renderer falls back to using its average as coverage.
type SubpixelSprite struct {
	Order       uint32
	Tile        AtlasTile
	Bounds      geometry.Bounds[geometry.ScaledPixels]
	ContentMask ContentMask
	Color       gc.Hsla
}

// PolychromeSprite is an RGBA sprite (images, emoji).
type PolychromeSprite struct {
	Order       uint32
	Tile        AtlasTile
	Grayscale   bool
	Opacity     float32
	Bounds      geometry.Bounds[geometry.ScaledPixels]
	ContentMask ContentMask
	CornerRadii geometry.Corners[geometry.ScaledPixels]
}

// Path is a filled path built from xy-monotone quadratic curves.
type Path struct {
	Curves  []PathCurve
	EvenOdd bool
}

// PathCurve is one monotone quadratic segment. Points are in scaled
// (device) pixels for rasterization.
type PathCurve struct {
	// P0, P1 are endpoints with P0.y <= P1.y.
	P0, P1 geometry.Point[float32]
	// A, B are quadratic coefficients: point(t) = a*t*t + b*t + p0.
	A, B geometry.Point[float32]
	// Sign is -1 if the curve was flipped to make it downward.
	Sign float32
}

// PaintedPath is a Path prepared for painting with a fill.
type PaintedPath struct {
	Order       uint32
	Bounds      geometry.Bounds[geometry.ScaledPixels]
	ContentMask ContentMask
	Path        *Path
	Color       gc.Background
}

// primitiveKind orders batches: Shadow < Quad < Path < Underline <
// MonochromeSprite < SubpixelSprite < PolychromeSprite.
type primitiveKind uint8

const (
	kindShadow primitiveKind = 0
	kindQuad   primitiveKind = 1
	kindPath   primitiveKind = 2
	kindUnder  primitiveKind = 3
	kindMono   primitiveKind = 4
	kindSub    primitiveKind = 5
	kindPoly   primitiveKind = 6
)

// Scene accumulates primitives for one frame. Insert order is z-order;
// layers hoist their contents above earlier siblings.
type Scene struct {
	// Shadows, Quads, etc. are the primitive lists, sorted by (order, kind)
	// after Finish.
	Shadows         []Shadow
	Quads           []Quad
	Paths           []PaintedPath
	Underlines      []Underline
	MonoSprites     []MonochromeSprite
	SubpixelSprites []SubpixelSprite
	PolySprites     []PolychromeSprite

	nextOrder   uint32
	layerStack  []*layerState
	layerStarts []int // primitive counts at PushLayer

	// sortedScratch backs SortedIndices (reused across frames).
	sortedScratch []orderIndex
}

type layerState struct {
	order uint32
}

type orderIndex struct {
	order uint32
	kind  primitiveKind
	index int
}

// PushLayer opens a paint layer: everything painted until the matching
// PopLayer draws above primitives painted before the layer opened.
func (s *Scene) PushLayer() {
	s.layerStack = append(s.layerStack, &layerState{})
}

// PopLayer closes the innermost layer.
func (s *Scene) PopLayer() {
	if len(s.layerStack) == 0 {
		return
	}
	s.layerStack = s.layerStack[:len(s.layerStack)-1]
}

// InsertShadow adds a shadow primitive.
func (s *Scene) InsertShadow(sh Shadow) { sh.Order = s.takeOrder(); s.Shadows = append(s.Shadows, sh) }

// InsertQuad adds a quad primitive.
func (s *Scene) InsertQuad(q Quad) { q.Order = s.takeOrder(); s.Quads = append(s.Quads, q) }

// InsertPath adds a filled path.
func (s *Scene) InsertPath(p PaintedPath) { p.Order = s.takeOrder(); s.Paths = append(s.Paths, p) }

// InsertUnderline adds an underline.
func (s *Scene) InsertUnderline(u Underline) {
	u.Order = s.takeOrder()
	s.Underlines = append(s.Underlines, u)
}

// InsertMonoSprite adds a monochrome sprite.
func (s *Scene) InsertMonoSprite(sp MonochromeSprite) {
	sp.Order = s.takeOrder()
	s.MonoSprites = append(s.MonoSprites, sp)
}

// InsertSubpixelSprite adds a subpixel sprite.
func (s *Scene) InsertSubpixelSprite(sp SubpixelSprite) {
	sp.Order = s.takeOrder()
	s.SubpixelSprites = append(s.SubpixelSprites, sp)
}

// InsertPolySprite adds a polychrome sprite.
func (s *Scene) InsertPolySprite(sp PolychromeSprite) {
	sp.Order = s.takeOrder()
	s.PolySprites = append(s.PolySprites, sp)
}

// takeOrder assigns a draw order. Inside a layer, all primitives share the
// layer's order (assigned when the layer opened), preserving inner order via
// list position. Layers themselves get monotonically increasing orders, so
// later layers (and post-layer primitives) draw on top.
func (s *Scene) takeOrder() uint32 {
	if len(s.layerStack) > 0 {
		st := s.layerStack[len(s.layerStack)-1]
		if st.order == 0 {
			s.nextOrder++
			st.order = s.nextOrder
		}
		return st.order
	}
	s.nextOrder++
	return s.nextOrder
}

// Empty reports whether the scene has no primitives.
func (s *Scene) Empty() bool {
	return len(s.Shadows)+len(s.Quads)+len(s.Paths)+len(s.Underlines)+
		len(s.MonoSprites)+len(s.SubpixelSprites)+len(s.PolySprites) == 0
}

// Finish sorts all primitive lists by (order, kind) so painting the lists
// back-to-front in kind order produces correct z. The renderer interleaves
// explicitly via sortedIndices.
func (s *Scene) Finish() {
	sort.SliceStable(s.Shadows, func(a, b int) bool { return s.Shadows[a].Order < s.Shadows[b].Order })
	sort.SliceStable(s.Quads, func(a, b int) bool { return s.Quads[a].Order < s.Quads[b].Order })
	sort.SliceStable(s.Paths, func(a, b int) bool { return s.Paths[a].Order < s.Paths[b].Order })
	sort.SliceStable(s.Underlines, func(a, b int) bool { return s.Underlines[a].Order < s.Underlines[b].Order })
	sort.SliceStable(s.MonoSprites, func(a, b int) bool { return s.MonoSprites[a].Order < s.MonoSprites[b].Order })
	sort.SliceStable(s.SubpixelSprites, func(a, b int) bool { return s.SubpixelSprites[a].Order < s.SubpixelSprites[b].Order })
	sort.SliceStable(s.PolySprites, func(a, b int) bool { return s.PolySprites[a].Order < s.PolySprites[b].Order })
}

// SortedIndices returns all primitives as an interleaved, globally sorted
// draw list (order, then kind, then insertion). This is what a software
// renderer paints back to front. The scratch slice is reused across
// calls (a full-window repaint runs this every frame; without reuse
// each frame allocated a fresh list).
func (s *Scene) SortedIndices() []orderIndex {
	all := s.sortedScratch[:0]
	for i := range s.Shadows {
		all = append(all, orderIndex{s.Shadows[i].Order, kindShadow, i})
	}
	for i := range s.Quads {
		all = append(all, orderIndex{s.Quads[i].Order, kindQuad, i})
	}
	for i := range s.Paths {
		all = append(all, orderIndex{s.Paths[i].Order, kindPath, i})
	}
	for i := range s.Underlines {
		all = append(all, orderIndex{s.Underlines[i].Order, kindUnder, i})
	}
	for i := range s.MonoSprites {
		all = append(all, orderIndex{s.MonoSprites[i].Order, kindMono, i})
	}
	for i := range s.SubpixelSprites {
		all = append(all, orderIndex{s.SubpixelSprites[i].Order, kindSub, i})
	}
	for i := range s.PolySprites {
		all = append(all, orderIndex{s.PolySprites[i].Order, kindPoly, i})
	}
	sort.SliceStable(all, func(a, b int) bool {
		if all[a].order != all[b].order {
			return all[a].order < all[b].order
		}
		if all[a].kind != all[b].kind {
			return all[a].kind < all[b].kind
		}
		return all[a].index < all[b].index
	})
	return all
}

// IsEmpty reports an empty scene.
func (s *Scene) IsEmpty() bool { return s.Empty() }

// Reset clears the scene for reuse.
func (s *Scene) Reset() {
	s.Shadows = s.Shadows[:0]
	s.Quads = s.Quads[:0]
	s.Paths = s.Paths[:0]
	s.Underlines = s.Underlines[:0]
	s.MonoSprites = s.MonoSprites[:0]
	s.SubpixelSprites = s.SubpixelSprites[:0]
	s.PolySprites = s.PolySprites[:0]
	s.nextOrder = 0
	s.layerStack = s.layerStack[:0]
	s.layerStarts = s.layerStarts[:0]

	if s.sortedScratch != nil {
		s.sortedScratch = s.sortedScratch[:0]
	}
}

// RenderImage is a CPU frame: 8-bit RGBA (non-premultiplied is fine for
// atlas sources; frame buffers are composited in place).
type RenderImage struct {
	Width, Height int
	Pixels        []uint8 // RGBA, row-major, len = 4*w*h
	ScaleFactor   float32
}
