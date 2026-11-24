package render

import (
	"image"
	"sort"
	"sync"

	"github.com/FlameInTheDark/go-gpui/geometry"
)

// AtlasKind selects an atlas texture kind.
type AtlasKind uint8

const (
	// AtlasMonochrome stores 8-bit alpha masks (glyphs, SVG masks).
	AtlasMonochrome AtlasKind = 0
	// AtlasPolychrome stores 32-bit RGBA (images, emoji).
	AtlasPolychrome AtlasKind = 1
	// AtlasSubpixel stores 32-bit RGB coverage.
	AtlasSubpixel AtlasKind = 2
)

// AtlasKey identifies an atlas entry.
type AtlasKey struct {
	Kind AtlasKind
	// GlyphID, FontID and FontSize identify a glyph tile; empty otherwise.
	FontID               int
	GlyphID              uint32
	FontSize             float32
	SubpixelX, SubpixelY uint8
	ScaleFactor          float32
	// ImageKey identifies non-glyph tiles (images/SVGs).
	ImageKey string
}

// Atlas is a CPU tile atlas: textures of tiles addressed by AtlasTile.
// The software renderer samples directly from these textures, and GPU
// backends mirror them into device textures via TakeDirtyRegions.
// Tiles are allocated with a 1px transparent padding ring (atlasPadding)
// so bilinear sampling at tile edges never bleeds neighboring glyph ink.
const atlasPadding = 1

// AtlasTextureSize is the side length of every atlas texture page. GPU
// backends create matching device texture arrays.
const AtlasTextureSize = 1024

// DirtyRegion describes a rectangle of atlas pixels changed since the last
// TakeDirtyRegions call (GPU backends upload just these boxes).
type DirtyRegion struct {
	Kind         AtlasKind
	TextureIndex int
	X, Y, W, H   int  // atlas pixel coordinates (already includes padding)
	NewTexture   bool // the texture was just created: upload everything
}

type Atlas struct {
	mu       sync.Mutex
	textures map[AtlasKind][]*atlasTexture
	keys     map[AtlasKey]AtlasTile
	nextTile uint32
	dirty    []DirtyRegion
	// clock is a monotonically increasing access counter; lastUsed records
	// the clock value at each key's most recent use (LRU eviction).
	clock    uint64
	lastUsed map[AtlasKey]uint64
}

// AtlasMaxTexturesPerKind caps the pages per kind; beyond it the oldest
// fully-stale pages are pruned (upstream evicts whole textures in the
// renderers; etagere tiles cannot be freed individually). 8 pages bound
// the worst-case atlas at 8MB mono + 32MB poly + 32MB subpixel CPU-side
// (the GPU mirrors the same), instead of the 144MB the 16-page cap allowed
// — reported RSS doubled once pages and mirrors both grew.
const AtlasMaxTexturesPerKind = 8

type atlasTexture struct {
	width, height int
	// monochrome: 1 byte/px; polychrome/subpixel: 4 bytes/px
	mono bool
	data []uint8
	// shelf allocator state
	shelfY int
	shelfH int
	shelfX int
}

const atlasTextureSize = AtlasTextureSize

// Atlas is the interface used by the window paint layer.
type AtlasI interface {
	GetOrInsert(key AtlasKey, build func() (TileContent, error)) (AtlasTile, error)
	Remove(key AtlasKey)
}

// NewAtlas creates an empty atlas.
func NewAtlas() *Atlas {
	return &Atlas{
		textures: map[AtlasKind][]*atlasTexture{},
		keys:     map[AtlasKey]AtlasTile{},
		lastUsed: map[AtlasKey]uint64{},
	}
}

// TileContent is what a tile build produces: pixel size, pixel data and a
// content offset (glyph bearing) stored on the tile so cache hits keep it.
type TileContent struct {
	Size   geometry.Size[geometry.DevicePixels]
	Pixels []uint8
	Offset geometry.Point[geometry.DevicePixels]
}

// GetOrInsert returns the tile for key, rasterizing via build on a miss.
// build returns the tile content (size, pixels, offset); pixels are mono
// (1B/px) or RGBA (4B/px) depending on key.Kind. The offset survives
// cache hits via the returned AtlasTile.
func (a *Atlas) GetOrInsert(key AtlasKey, build func() (TileContent, error)) (AtlasTile, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.clock++
	if tile, ok := a.keys[key]; ok {
		a.lastUsed[key] = a.clock
		return tile, nil
	}
	content, err := build()
	if err != nil {
		return AtlasTile{}, err
	}
	w, h := int(content.Size.Width.AsI32()), int(content.Size.Height.AsI32())
	off := content.Offset
	if w <= 0 || h <= 0 {
		w, h = 1, 1
		content.Pixels = []byte{0}
		off = geometry.Point[geometry.DevicePixels]{}
	}
	kind := key.Kind
	mono := kind == AtlasMonochrome
	if len(a.textures[kind]) == 0 {
		a.textures[kind] = append(a.textures[kind], newAtlasTexture(atlasTextureSize, mono))
		a.dirty = append(a.dirty, DirtyRegion{Kind: kind, TextureIndex: 0, NewTexture: true,
			W: atlasTextureSize, H: atlasTextureSize})
	}
	// Reserve the padding ring around the tile content.
	allocW, allocH := w+2*atlasPadding, h+2*atlasPadding
	var tile AtlasTile
	placed := false
	for ti, tex := range a.textures[kind] {
		if x, y, ok := tex.alloc(allocW, allocH); ok {
			tex.blit(x+atlasPadding, y+atlasPadding, w, h, content.Pixels, mono)
			a.dirty = append(a.dirty, DirtyRegion{Kind: kind, TextureIndex: ti,
				X: x, Y: y, W: allocW, H: allocH})
			tile = AtlasTile{
				TextureIndex: uint32(ti),
				TileID:       a.nextTile,
				Bounds: geometry.Bounds[geometry.DevicePixels]{
					Origin: geometry.Point[geometry.DevicePixels]{X: geometry.DevicePx(int32(x + atlasPadding)), Y: geometry.DevicePx(int32(y + atlasPadding))},
					Size:   geometry.Size[geometry.DevicePixels]{Width: geometry.DevicePx(int32(w)), Height: geometry.DevicePx(int32(h))},
				},
				Offset: off,
			}
			placed = true
			break
		}
	}

	if !placed {
		if allocW > atlasTextureSize || allocH > atlasTextureSize {
			// Oversized content cannot fit any page: park it as an empty
			// tile instead of overlapping other tiles at (0, 0).
			a.nextTile++
			tile = AtlasTile{TileID: a.nextTile,
				Bounds: geometry.Bounds[geometry.DevicePixels]{
					Size: geometry.Size[geometry.DevicePixels]{Width: geometry.DevicePx(1), Height: geometry.DevicePx(1)}}}
			a.keys[key] = tile
			a.lastUsed[key] = a.clock
			return tile, nil
		}
		tex := newAtlasTexture(atlasTextureSize, mono)
		x, y, _ := tex.alloc(allocW, allocH)
		tex.blit(x+atlasPadding, y+atlasPadding, w, h, content.Pixels, mono)
		a.textures[kind] = append(a.textures[kind], tex)
		ti := len(a.textures[kind]) - 1
		a.dirty = append(a.dirty, DirtyRegion{Kind: kind, TextureIndex: ti, NewTexture: true,
			W: atlasTextureSize, H: atlasTextureSize})
		tile = AtlasTile{
			TextureIndex: uint32(len(a.textures[kind]) - 1),
			TileID:       a.nextTile,
			Bounds: geometry.Bounds[geometry.DevicePixels]{
				Origin: geometry.Point[geometry.DevicePixels]{X: geometry.DevicePx(int32(x + atlasPadding)), Y: geometry.DevicePx(int32(y + atlasPadding))},
				Size:   geometry.Size[geometry.DevicePixels]{Width: geometry.DevicePx(int32(w)), Height: geometry.DevicePx(int32(h))},
			},
			Offset: off,
		}
	}
	a.nextTile++
	a.keys[key] = tile
	a.lastUsed[key] = a.clock
	// Whole-texture eviction (upstream prunes atlas textures in the
	// renderers): once a kind exceeds the page cap, drop pages whose keys
	// are all stale (unused for at least AtlasStaleAccesses accesses).
	if len(a.textures[kind]) > AtlasMaxTexturesPerKind {
		a.pruneTexturesLocked(kind)
	}
	return tile, nil
}

// Remove forgets a key (the tile's pixels leak inside its page until that
// page is pruned; upstream etagere behaves the same and the renderers evict
// whole textures).
func (a *Atlas) Remove(key AtlasKey) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.keys, key)
	delete(a.lastUsed, key)
}

// AtlasStaleAccesses is the number of atlas accesses after which an unused
// key is considered stale and its page becomes prunable.
const AtlasStaleAccesses = 4096

// PruneTextures drops pages of the given kind whose keys are all stale (no
// use within the last AtlasStaleAccesses accesses), remaps the surviving
// pages' indices and marks them fully dirty so GPU mirrors rebuild. It
// returns the number of pages dropped (upstream eviction: the renderers
// prune textures the recent scenes no longer reference).
func (a *Atlas) PruneTextures(kind AtlasKind) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pruneTexturesLocked(kind)
}

func (a *Atlas) pruneTexturesLocked(kind AtlasKind) int {
	texs := a.textures[kind]
	if len(texs) == 0 {
		return 0
	}
	staleThreshold := int64(a.clock) - AtlasStaleAccesses
	// A page is live when any of its keys was used within the threshold.
	live := make([]bool, len(texs))
	for key, tile := range a.keys {
		if key.Kind != kind {
			continue
		}
		if int64(a.lastUsed[key]) >= staleThreshold && int(tile.TextureIndex) < len(live) {
			live[tile.TextureIndex] = true
		}
	}
	kept := 0
	for _, l := range live {
		if l {
			kept++
		}
	}
	if kept == len(texs) {
		return 0
	}
	// Remap surviving pages and drop stale keys.
	remap := make([]int, len(texs))
	newTexs := make([]*atlasTexture, 0, kept)
	for i, l := range live {
		if l {
			remap[i] = len(newTexs)
			newTexs = append(newTexs, texs[i])
		} else {
			remap[i] = -1
		}
	}
	for key, tile := range a.keys {
		if key.Kind != kind {
			continue
		}
		idx := int(tile.TextureIndex)
		if idx >= 0 && idx < len(remap) {
			if remap[idx] < 0 {
				delete(a.keys, key)
				delete(a.lastUsed, key)
			} else {
				tile.TextureIndex = uint32(remap[idx])
				a.keys[key] = tile
			}
		}
	}
	a.textures[kind] = newTexs
	// Rebuilding the GPU mirror: every surviving page is fully dirty.
	for i := range newTexs {
		a.dirty = append(a.dirty, DirtyRegion{Kind: kind, TextureIndex: i, NewTexture: true,
			W: newTexs[i].width, H: newTexs[i].height})
	}
	return len(texs) - kept
}

// Texture returns the raw pixels of a texture for sampling.
func (a *Atlas) Texture(kind AtlasKind, index int) (data []uint8, width, height int, mono bool, ok bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	texs := a.textures[kind]
	if index < 0 || index >= len(texs) {
		return nil, 0, 0, false, false
	}
	t := texs[index]
	return t.data, t.width, t.height, t.mono, true
}

// TextureCount reports the number of pages for a kind (GPU backends size
// their texture arrays from this).
func (a *Atlas) TextureCount(kind AtlasKind) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.textures[kind])
}

// TakeDirtyRegions returns the atlas regions changed since the last call
// and clears the list (GPU backends upload these).
func (a *Atlas) TakeDirtyRegions() []DirtyRegion {
	a.mu.Lock()
	defer a.mu.Unlock()
	d := a.dirty
	a.dirty = nil
	return d
}

// Len reports the number of cached tiles.
func (a *Atlas) Len() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.keys)
}

// LastUsedAt reports the LRU clock value of a key (0 when absent).
func (a *Atlas) LastUsedAt(key AtlasKey) uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastUsed[key]
}

// RemoveLRU evicts the n least-recently-used keys (explicit LRU eviction;
// the tiles' pixels stay in their pages until those pages are pruned).
func (a *Atlas) RemoveLRU(n int) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if n >= len(a.keys) {
		k := len(a.keys)
		a.keys = map[AtlasKey]AtlasTile{}
		a.lastUsed = map[AtlasKey]uint64{}
		return k
	}
	type kv struct {
		key AtlasKey
		at  uint64
	}
	entries := make([]kv, 0, len(a.keys))
	for k := range a.keys {
		entries = append(entries, kv{k, a.lastUsed[k]})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].at < entries[j].at })
	evicted := 0
	for _, e := range entries {
		if evicted >= n {
			break
		}
		delete(a.keys, e.key)
		delete(a.lastUsed, e.key)
		evicted++
	}
	return evicted
}

func newAtlasTexture(size int, mono bool) *atlasTexture {
	n := size * size
	if !mono {
		n *= 4
	}
	return &atlasTexture{width: size, height: size, mono: mono, data: make([]uint8, n)}
}

func (t *atlasTexture) alloc(w, h int) (int, int, bool) {
	if t.shelfX+w > t.width {
		// new shelf
		t.shelfY += t.shelfH
		t.shelfH = 0
		t.shelfX = 0
	}
	if t.shelfY+h > t.height {
		return 0, 0, false
	}
	x, y := t.shelfX, t.shelfY
	t.shelfX += w
	if h > t.shelfH {
		t.shelfH = h
	}
	return x, y, true
}

func (t *atlasTexture) blit(x, y, w, h int, pixels []uint8, mono bool) {
	bpp := 4
	if mono {
		bpp = 1
	}
	for row := 0; row < h; row++ {
		srcOff := row * w * bpp
		dstOff := ((y+row)*t.width + x) * bpp
		end := dstOff + w*bpp
		if end > len(t.data) {
			break
		}
		copy(t.data[dstOff:end], pixels[srcOff:srcOff+w*bpp])
	}
}

// SampleMono bilinearly samples a monochrome texture at (u, v) in [0,1].
func SampleMono(data []uint8, width, height int, u, v float32) float32 {
	x := u*float32(width) - 0.5
	y := v*float32(height) - 0.5
	x0 := int(floorf(x))
	y0 := int(floorf(y))
	fx := x - float32(x0)
	fy := y - float32(y0)
	sample := func(px, py int) float32 {
		if px < 0 {
			px = 0
		}
		if py < 0 {
			py = 0
		}
		if px >= width {
			px = width - 1
		}
		if py >= height {
			py = height - 1
		}
		return float32(data[py*width+px]) / 255.0
	}
	top := sample(x0, y0)*(1-fx) + sample(x0+1, y0)*fx
	bottom := sample(x0, y0+1)*(1-fx) + sample(x0+1, y0+1)*fx
	return top*(1-fy) + bottom*fy
}

// SamplePoly bilinearly samples an RGBA texture.
func SamplePoly(data []uint8, width, height int, u, v float32) (r, g, b, a float32) {
	x := u*float32(width) - 0.5
	y := v*float32(height) - 0.5
	x0 := int(floorf(x))
	y0 := int(floorf(y))
	fx := x - float32(x0)
	fy := y - float32(y0)
	sample := func(px, py int) (float32, float32, float32, float32) {
		if px < 0 {
			px = 0
		}
		if py < 0 {
			py = 0
		}
		if px >= width {
			px = width - 1
		}
		if py >= height {
			py = height - 1
		}
		i := (py*width + px) * 4
		return float32(data[i]) / 255, float32(data[i+1]) / 255, float32(data[i+2]) / 255, float32(data[i+3]) / 255
	}
	r0, g0, b0, a0 := sample(x0, y0)
	r1, g1, b1, a1 := sample(x0+1, y0)
	r2, g2, b2, a2 := sample(x0, y0+1)
	r3, g3, b3, a3 := sample(x0+1, y0+1)
	top := rgbaF{R: r0*(1-fx) + r1*fx, G: g0*(1-fx) + g1*fx, B: b0*(1-fx) + b1*fx, A: a0*(1-fx) + a1*fx}
	bot := rgbaF{R: r2*(1-fx) + r3*fx, G: g2*(1-fx) + g3*fx, B: b2*(1-fx) + b3*fx, A: a2*(1-fx) + a3*fx}
	return top.R*(1-fy) + bot.R*fy, top.G*(1-fy) + bot.G*fy, top.B*(1-fy) + bot.B*fy, top.A*(1-fy) + bot.A*fy
}

func floorf(v float32) float32 {
	if v < 0 {
		return -float32(-int32(-v))
	}
	return float32(int32(v))
}

var _ = image.Point{}
