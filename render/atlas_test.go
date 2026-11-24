package render

import (
	"testing"

	"github.com/FlameInTheDark/go-gpui/geometry"
)

// TestAtlasLRUEviction verifies the access-clock LRU: RemoveLRU evicts the
// least recently used keys first, and hits refresh recency (upstream
// eviction model: renderers prune entries the recent scenes no longer
// reference).
func TestAtlasLRUEviction(t *testing.T) {
	a := NewAtlas()
	build := func() (TileContent, error) {
		return TileContent{Size: geometry.Size[geometry.DevicePixels]{Width: geometry.DevicePx(4), Height: geometry.DevicePx(4)}, Pixels: make([]byte, 16)}, nil
	}
	k1 := AtlasKey{Kind: AtlasMonochrome, GlyphID: 1}
	k2 := AtlasKey{Kind: AtlasMonochrome, GlyphID: 2}
	k3 := AtlasKey{Kind: AtlasMonochrome, GlyphID: 3}
	a.GetOrInsert(k1, build)
	a.GetOrInsert(k2, build)
	a.GetOrInsert(k3, build)
	// Touch k1 so k2 becomes the least recently used.
	a.GetOrInsert(k1, build)

	if n := a.RemoveLRU(1); n != 1 {
		t.Fatalf("RemoveLRU evicted %d, want 1", n)
	}
	if _, ok := a.keys[k2]; ok {
		t.Fatal("k2 (least recently used) should have been evicted")
	}
	if _, ok := a.keys[k1]; !ok {
		t.Fatal("k1 was touched recently and must survive")
	}
	if _, ok := a.keys[k3]; !ok {
		t.Fatal("k3 must survive")
	}
}

// TestAtlasPruneTextures verifies whole-page pruning: pages whose keys are
// all stale are dropped, survivors are remapped and re-marked dirty (the
// upstream renderers' texture eviction).
func TestAtlasPruneTextures(t *testing.T) {
	a := NewAtlas()
	build := func() (TileContent, error) {
		return TileContent{Size: geometry.Size[geometry.DevicePixels]{Width: geometry.DevicePx(4), Height: geometry.DevicePx(4)}, Pixels: make([]byte, 16)}, nil
	}
	// Two keys on separate pages: exhaust page 0 by forcing a second page.
	kOld := AtlasKey{Kind: AtlasMonochrome, GlyphID: 100}
	a.GetOrInsert(kOld, build)
	// Fill enough of page 0 that the next big tile spills to page 1.
	big := func() (TileContent, error) {
		return TileContent{Size: geometry.Size[geometry.DevicePixels]{Width: geometry.DevicePx(900), Height: geometry.DevicePx(900)}, Pixels: make([]byte, 900*900)}, nil
	}
	kBig := AtlasKey{Kind: AtlasMonochrome, GlyphID: 101}
	a.GetOrInsert(kBig, big) // page 0
	kNew := AtlasKey{Kind: AtlasMonochrome, GlyphID: 102}
	a.GetOrInsert(kNew, big) // page 1 (page 0 has no room for a second 1200px tile)

	before := a.TextureCount(AtlasMonochrome)
	if before < 2 {
		t.Fatalf("expected at least 2 pages, got %d", before)
	}
	// Age everything except kNew by more than the stale threshold.
	for i := 0; i < AtlasStaleAccesses+10; i++ {
		a.GetOrInsert(kNew, build)
	}
	// Force the stale keys' lastUsed below the threshold explicitly.
	a.mu.Lock()
	a.clock += AtlasStaleAccesses + 10
	a.mu.Unlock()
	a.GetOrInsert(kNew, build)

	if dropped := a.PruneTextures(AtlasMonochrome); dropped == 0 {
		t.Fatal("expected at least one stale page to be pruned")
	}
}
