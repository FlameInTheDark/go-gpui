package render

import (
	"math"
	"testing"

	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
)

func sp(v float32) geometry.ScaledPixels { return geometry.ScaledPx(v) }

func sb(x, y, w, h float32) geometry.Bounds[geometry.ScaledPixels] {
	return geometry.Bounds[geometry.ScaledPixels]{
		Origin: geometry.Point[geometry.ScaledPixels]{X: sp(x), Y: sp(y)},
		Size:   geometry.Size[geometry.ScaledPixels]{Width: sp(w), Height: sp(h)},
	}
}

// TestGpuQuadInstance checks quad -> instance packing.
func TestGpuQuadInstance(t *testing.T) {
	scene := &Scene{}
	mask := FullContentMask()
	red := gc.RGB(0xff0000).Hsla()
	bg := gc.SolidBackground(red)
	scene.InsertQuad(Quad{
		Bounds:      sb(10, 20, 100, 50),
		ContentMask: mask,
		Background:  bg,
	})
	scene.Finish()

	atlas := NewAtlas()
	frame := BuildGpuFrame(scene, atlas)
	if len(frame.Quads) != 1 {
		t.Fatalf("want 1 quad instance, got %d", len(frame.Quads))
	}
	inst := frame.Quads[0]
	if inst[0] != [4]float32{10, 20, 100, 50} {
		t.Errorf("bounds row = %v", inst[0])
	}
	if inst[12][0] != GpuPrimQuad {
		t.Errorf("primType = %v, want quad", inst[12][0])
	}
	if inst[12][3] != GpuTexNone {
		t.Errorf("texKind = %v, want none", inst[12][3])
	}
	if inst[4][0] < 0.99 || inst[4][1] > 0.01 || inst[4][2] > 0.01 || inst[4][3] < 0.99 {
		t.Errorf("solid color row = %v (want red, opaque)", inst[4])
	}
	// The full mask must not clip anything.
	if inst[1][0] != -1e9 || inst[1][2] != 1e9 {
		t.Errorf("mask row = %v", inst[1])
	}
}

// TestGpuOrderPreserved checks that instances follow the (order, kind)
// paint order of the software rasterizer.
func TestGpuOrderPreserved(t *testing.T) {
	scene := &Scene{}
	mask := FullContentMask()
	white := gc.RGB(0xffffff).Hsla()

	// Insert sprites first, then a quad: the quad has a later order and
	// must come after the sprites in the instance list.
	tile, err := atlasTileForTest()
	if err != nil {
		t.Fatal(err)
	}
	scene.InsertMonoSprite(MonochromeSprite{
		Bounds:      sb(0, 0, 8, 8),
		ContentMask: mask,
		Tile:        tile,
		Color:       white,
	})
	scene.InsertQuad(Quad{
		Bounds:      sb(0, 0, 4, 4),
		ContentMask: mask,
		Background:  gc.SolidBackground(white),
	})
	scene.InsertQuad(Quad{
		Bounds:      sb(1, 1, 4, 4),
		ContentMask: mask,
		Background:  gc.SolidBackground(white),
	})
	scene.Finish()

	atlas := NewAtlas()
	// Pre-seed the atlas with the tile referenced by the sprite.
	seeded, err := atlas.GetOrInsert(AtlasKey{Kind: AtlasMonochrome, FontID: 1, GlyphID: 7, FontSize: 16, ScaleFactor: 1}, func() (TileContent, error) {
		px := make([]byte, 8*8)
		for i := range px {
			px[i] = 0xff
		}
		return TileContent{Size: geometry.SizeDevice(8, 8), Pixels: px}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seeded != tile {
		// The scene's sprite must reference a real tile for the instance
		// builder to emit it; patch the scene.
		scene.MonoSprites[0].Tile = seeded
	}
	frame := BuildGpuFrame(scene, atlas)
	if len(frame.Batches) != 2 {
		t.Fatalf("want 2 batches (mono then quads), got %+v", frame.Batches)
	}
	if frame.Batches[0].Kind != GpuBatchMono || frame.Batches[0].Count != 1 {
		t.Errorf("first batch should be mono, got %+v", frame.Batches[0])
	}
	if frame.Batches[1].Kind != GpuBatchQuads || frame.Batches[1].Count != 2 {
		t.Errorf("second batch should be quads, got %+v", frame.Batches[1])
	}
	if len(frame.Mono) != 1 || len(frame.Quads) != 2 {
		t.Fatalf("instance lists: mono=%d quads=%d", len(frame.Mono), len(frame.Quads))
	}
	if frame.Quads[0][0][0] != 0 || frame.Quads[1][0][0] != 1 {
		t.Errorf("quad bounds out of order: %v %v", frame.Quads[0][0], frame.Quads[1][0])
	}
	// The opaque pre-pass data: both quads are opaque rectangles.
	if !isOpaqueRectangle(scene.Quads[0]) || !isOpaqueRectangle(scene.Quads[1]) {
		t.Errorf("solid unshaped quads should qualify for the opaque pre-pass")
	}
	if frame.QuadCount != 2 || frame.DepthScale <= 0 || frame.DepthScale >= 1 {
		t.Errorf("quadCount=%d depthScale=%v", frame.QuadCount, frame.DepthScale)
	}
}

func atlasTileForTest() (AtlasTile, error) {
	return AtlasTile{TextureIndex: 0, Bounds: geometry.Bounds[geometry.DevicePixels]{
		Origin: geometry.Point[geometry.DevicePixels]{X: geometry.DevicePx(1), Y: geometry.DevicePx(1)},
		Size:   geometry.Size[geometry.DevicePixels]{Width: geometry.DevicePx(8), Height: geometry.DevicePx(8)},
	}}, nil
}

// TestGpuSpriteUV checks the device-pixel -> atlas-uv matrix: the sprite
// bounds' corners must map onto the tile's atlas rectangle.
func TestGpuSpriteUV(t *testing.T) {
	atlas := NewAtlas()
	tile, err := atlas.GetOrInsert(AtlasKey{Kind: AtlasMonochrome, FontID: 1, GlyphID: 9, FontSize: 16, ScaleFactor: 1}, func() (TileContent, error) {
		px := make([]byte, 4*4)
		for i := range px {
			px[i] = 0x80
		}
		return TileContent{Size: geometry.SizeDevice(4, 4), Pixels: px}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	scene := &Scene{}
	scene.InsertMonoSprite(MonochromeSprite{
		Bounds:      sb(100, 50, 8, 8),
		ContentMask: FullContentMask(),
		Tile:        tile,
		Color:       gc.RGB(0xffffff).Hsla(),
	})
	scene.Finish()

	frame := BuildGpuFrame(scene, atlas)
	if len(frame.Mono) != 1 {
		t.Fatalf("want 1 mono instance, got %d", len(frame.Mono))
	}
	inst := frame.Mono[0]
	uv := func(x, y float32) (float32, float32) {
		return inst[9][0]*x + inst[9][1]*y + inst[9][2],
			inst[10][0]*x + inst[10][1]*y + inst[10][2]
	}
	tx := float32(tile.Bounds.Origin.X.AsI32())
	ty := float32(tile.Bounds.Origin.Y.AsI32())
	tw := float32(tile.Bounds.Size.Width.AsI32())
	th := float32(tile.Bounds.Size.Height.AsI32())
	tex := float32(AtlasTextureSize)

	// Top-left of the sprite maps to the top-left of the tile.
	u, v := uv(100, 50)
	if math.Abs(float64(u-tx/tex)) > 1e-5 || math.Abs(float64(v-ty/tex)) > 1e-5 {
		t.Errorf("top-left uv = (%v, %v), want (%v, %v)", u, v, tx/tex, ty/tex)
	}
	// Bottom-right maps to the tile's far corner.
	u, v = uv(108, 58)
	if math.Abs(float64(u-(tx+tw)/tex)) > 1e-5 || math.Abs(float64(v-(ty+th)/tex)) > 1e-5 {
		t.Errorf("bottom-right uv = (%v, %v), want (%v, %v)", u, v, (tx+tw)/tex, (ty+th)/tex)
	}
	if inst[12][3] != GpuTexMono {
		t.Errorf("texKind = %v, want mono", inst[12][3])
	}
	if inst[13][0] != float32(tile.TextureIndex) {
		t.Errorf("texIndex = %v", inst[13][0])
	}
}

// TestGpuPathBaking checks that paths become atlas sprite instances and
// the atlas reports dirty regions for the baked tile.
func TestGpuPathBaking(t *testing.T) {
	atlas := NewAtlas()
	builder := NewPathBuilder()
	builder.MoveTo(geometry.Point[float32]{X: 0, Y: 0})
	builder.LineTo(geometry.Point[float32]{X: 10, Y: 0})
	builder.LineTo(geometry.Point[float32]{X: 10, Y: 10})
	builder.LineTo(geometry.Point[float32]{X: 0, Y: 10})
	builder.Close()
	path, ok := builder.Build()
	if !ok {
		t.Fatal("path build failed")
	}
	scene := &Scene{}
	blue := gc.RGB(0x0000ff).Hsla()
	scene.InsertPath(PaintedPath{
		Path:        path,
		Bounds:      sb(0, 0, 10, 10),
		ContentMask: FullContentMask(),
		Color:       gc.SolidBackground(blue),
	})
	scene.Finish()

	frame := BuildGpuFrame(scene, atlas)
	// Paths are GPU-tiled now (Slug), not baked into atlas sprites: one
	// batch of tiles with a paint entry and the path's curves.
	if len(frame.PathTiles) == 0 {
		t.Fatal("want path tiles, got none")
	}
	if len(frame.PathPaints) != 1 {
		t.Fatalf("want 1 path paint, got %d", len(frame.PathPaints))
	}
	if frame.PathPaints[0].Color0[2] < 0.99 {
		t.Errorf("paint color = %v, want blue", frame.PathPaints[0].Color0)
	}
	if len(frame.PathCurves) == 0 {
		t.Error("want binned curves")
	}
	if frame.PathTiles[0].Paint != 0 {
		t.Errorf("tile paint index = %d, want 0", frame.PathTiles[0].Paint)
	}
	if len(frame.Batches) == 0 || frame.Batches[0].Kind != GpuBatchPaths {
		t.Errorf("first batch should be paths, got %+v", frame.Batches)
	}
	// The path is a 10x10 square: its tiles must cover the interior point.
	covered := false
	for _, tile := range frame.PathTiles {
		if tile.TopLeft[0] <= 5 && 5 <= tile.BottomRight[0] && tile.TopLeft[1] <= 5 && 5 <= tile.BottomRight[1] {
			covered = true
		}
	}
	if !covered {
		t.Error("interior point (5,5) not covered by path tiles")
	}
}

// TestGpuUnderlineAlphaSquared checks the underline alpha pre-squaring.
func TestGpuUnderlineAlphaSquared(t *testing.T) {
	scene := &Scene{}
	half := gc.Hsla{H: 0, S: 0, L: 1, A: 0.5}
	scene.InsertUnderline(Underline{
		Bounds:      sb(0, 0, 20, 2),
		ContentMask: FullContentMask(),
		Color:       half,
		Thickness:   sp(1),
	})
	scene.InsertUnderline(Underline{
		Bounds:      sb(0, 4, 20, 2),
		ContentMask: FullContentMask(),
		Color:       half,
		Thickness:   sp(1),
		Wavy:        true,
	})
	scene.Finish()
	frame := BuildGpuFrame(scene, NewAtlas())
	if len(frame.Underines) != 2 {
		t.Fatalf("want 2 underline instances, got %d", len(frame.Underines))
	}
	if frame.Underines[0][12][0] != GpuPrimUnderlineSolid {
		t.Errorf("prim = %v, want solid underline", frame.Underines[0][12][0])
	}
	if math.Abs(float64(frame.Underines[0][4][3]-0.25)) > 1e-6 {
		t.Errorf("alpha = %v, want 0.25 (squared)", frame.Underines[0][4][3])
	}
	if frame.Underines[1][12][0] != GpuPrimUnderlineWavy {
		t.Errorf("prim = %v, want wavy underline", frame.Underines[1][12][0])
	}
	if frame.Underines[1][12][2] != 1 {
		t.Errorf("thickness = %v", frame.Underines[1][12][2])
	}
}

// TestGpuShadowInstance checks shadow parameter packing.
func TestGpuShadowInstance(t *testing.T) {
	scene := &Scene{}
	shape := sb(5, 5, 40, 20)
	scene.InsertShadow(Shadow{
		BlurRadius:  sp(4),
		Bounds:      sb(0, 0, 60, 40),
		ShapeBounds: shape,
		ContentMask: FullContentMask(),
		Color:       gc.RGB(0x000000).Hsla().Alpha(0.5),
	})
	scene.Finish()
	frame := BuildGpuFrame(scene, NewAtlas())
	if len(frame.Shadows) != 1 {
		t.Fatalf("want 1 shadow instance, got %d", len(frame.Shadows))
	}
	inst := frame.Shadows[0]
	if inst[12][0] != GpuPrimShadow {
		t.Errorf("prim = %v, want shadow", inst[12][0])
	}
	if inst[11][2] != 4 {
		t.Errorf("blurRadius = %v", inst[11][2])
	}
	if inst[7] != [4]float32{5, 5, 40, 20} {
		t.Errorf("shapeBounds = %v", inst[7])
	}
}

// TestGpuGradientQuad checks gradient stop packing.
func TestGpuGradientQuad(t *testing.T) {
	scene := &Scene{}
	bg := gc.LinearGradient(90, gc.NewLinearColorStop(gc.RGB(0x000000).Hsla(), 0), gc.NewLinearColorStop(gc.RGB(0xffffff).Hsla(), 1))
	scene.InsertQuad(Quad{
		Bounds:      sb(0, 0, 100, 100),
		ContentMask: FullContentMask(),
		Background:  bg,
	})
	scene.Finish()
	frame := BuildGpuFrame(scene, NewAtlas())
	if len(frame.Quads) != 1 {
		t.Fatalf("want 1 quad, got %d", len(frame.Quads))
	}
	inst := frame.Quads[0]
	if inst[11][0] != 1 {
		t.Errorf("bgTag = %v, want linear gradient", inst[11][0])
	}
	if inst[11][1] != 90 {
		t.Errorf("angle = %v", inst[11][1])
	}
	if inst[14][1] != 0 || inst[14][2] != 1 {
		t.Errorf("stop percentages = %v", inst[14])
	}
}

// TestGpuEmptyScene checks the empty case.
func TestGpuEmptyScene(t *testing.T) {
	frame := BuildGpuFrame(&Scene{}, NewAtlas())
	if frame.QuadCount != 0 || len(frame.Batches) != 0 {
		t.Errorf("empty scene should produce no GPU data, got %+v", frame)
	}
}

// TestGpuOversizedPathClipped checks that a huge path is still binned (the
// grid clamps cell sizes) but only the tiles inside the content mask remain.
func TestGpuOversizedPathClipped(t *testing.T) {
	builder := NewPathBuilder()
	builder.MoveTo(geometry.Point[float32]{X: 0, Y: 0})
	builder.LineTo(geometry.Point[float32]{X: 3000, Y: 0})
	builder.LineTo(geometry.Point[float32]{X: 0, Y: 3000})
	builder.Close()
	path, ok := builder.Build()
	if !ok {
		t.Fatal("path build failed")
	}
	scene := &Scene{}
	scene.InsertPath(PaintedPath{
		Path:        path,
		Bounds:      sb(0, 0, 3000, 3000),
		ContentMask: ContentMask{Bounds: sb(1000, 1000, 100, 100)},
		Color:       gc.SolidBackground(gc.RGB(0xff0000).Hsla()),
	})
	scene.Finish()
	frame := BuildGpuFrame(scene, NewAtlas())
	if len(frame.PathTiles) == 0 {
		t.Fatal("huge path should bin into visible tiles, got none")
	}
	// Every visible tile must intersect the mask.
	maskRight, maskBottom := float32(1100), float32(1100)
	for _, tile := range frame.PathTiles {
		if tile.TopLeft[0] >= maskRight || tile.TopLeft[1] >= maskBottom ||
			tile.BottomRight[0] <= 1000 || tile.BottomRight[1] <= 1000 {
			t.Errorf("tile %v..%v outside mask", tile.TopLeft, tile.BottomRight)
		}
	}
}
