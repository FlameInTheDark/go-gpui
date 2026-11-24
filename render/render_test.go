package render

import (
	"testing"

	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
)

func solid(color gc.Hsla) gc.Background { return gc.SolidBackground(color) }

func ScaledPx(v float32) geometry.ScaledPixels { return geometry.ScaledPx(v) }

func quadAt(x, y, w, h float32, color gc.Hsla) Quad {
	return Quad{
		Bounds: geometry.Bounds[geometry.ScaledPixels]{
			Origin: geometry.Point[geometry.ScaledPixels]{X: ScaledPx(x), Y: ScaledPx(y)},
			Size:   geometry.Size[geometry.ScaledPixels]{Width: ScaledPx(w), Height: ScaledPx(h)},
		},
		ContentMask: FullContentMask(),
		Background:  solid(color),
	}
}

func pixel(img *RenderImage, x, y int) (float32, float32, float32, float32) {
	i := (y*img.Width + x) * 4
	return float32(img.Pixels[i]) / 255, float32(img.Pixels[i+1]) / 255, float32(img.Pixels[i+2]) / 255, float32(img.Pixels[i+3]) / 255
}

func approx(t *testing.T, name string, got, want float32) {
	t.Helper()
	d := got - want
	if d < -0.02 || d > 0.02 {
		t.Fatalf("%s: got %v want %v", name, got, want)
	}
}

func TestSolidQuad(t *testing.T) {
	r := NewRasterizer(20, 20, NewAtlas())
	scene := &Scene{}
	red := gc.RGB(0xff0000).Hsla()
	scene.InsertQuad(quadAt(5, 5, 10, 10, red))
	scene.Finish()
	r.Clear()
	r.PaintScene(scene)
	img := r.Image()

	pr, pg, pb, pa := pixel(img, 10, 10)
	approx(t, "red r", pr, 1)
	approx(t, "red g", pg, 0)
	approx(t, "red b", pb, 0)
	approx(t, "alpha", pa, 1)

	// Outside is transparent.
	_, _, _, pa = pixel(img, 1, 1)
	approx(t, "outside alpha", pa, 0)

	// Edge pixel (x=5 boundary) is half covered by the 0.5-centered AA.
	_, _, _, pa = pixel(img, 4, 10)
	if pa > 0.1 {
		t.Fatalf("pixel left of quad should be empty: %v", pa)
	}
}

func TestZOrder(t *testing.T) {
	r := NewRasterizer(20, 20, NewAtlas())
	scene := &Scene{}
	blue := gc.RGB(0x0000ff).Hsla()
	yellow := gc.RGB(0xffff00).Hsla()
	// blue first, yellow second: yellow wins on top.
	scene.InsertQuad(quadAt(0, 0, 20, 20, blue))
	scene.InsertQuad(quadAt(5, 5, 10, 10, yellow))
	scene.Finish()
	r.Clear()
	r.PaintScene(scene)
	img := r.Image()
	pr, _, pb, _ := pixel(img, 10, 10)
	approx(t, "yellow on top r", pr, 1)
	approx(t, "yellow on top b", pb, 0)
	_, _, pb, _ = pixel(img, 1, 1)
	approx(t, "blue beneath b", pb, 1)
}

func TestLayersHoistAboveSiblings(t *testing.T) {
	r := NewRasterizer(20, 20, NewAtlas())
	scene := &Scene{}
	bg := gc.RGB(0x00ff00).Hsla()
	fg := gc.RGB(0xff0000).Hsla()
	// A full-size quad painted after the layer pop must still be BELOW the
	// layer's contents (the layer got its order when it opened... actually
	// upstream semantics: layer contents paint above everything painted
	// before pop_layer at the same level, in layer-open order).
	scene.InsertQuad(quadAt(0, 0, 20, 20, bg))
	scene.PushLayer()
	scene.InsertQuad(quadAt(0, 0, 20, 20, fg))
	scene.PopLayer()
	scene.InsertQuad(quadAt(0, 0, 20, 20, bg))
	scene.Finish()
	r.Clear()
	r.PaintScene(scene)
	img := r.Image()
	pr, _, _, _ := pixel(img, 10, 10)
	// The final bg quad has a later order than the layer, so it wins.
	approx(t, "post-layer quad on top", pr, 0)
	_, pg, _, _ := pixel(img, 10, 10)
	approx(t, "green wins", pg, 1)

	// But a quad painted BEFORE the layer is below it.
	scene2 := &Scene{}
	scene2.InsertQuad(quadAt(0, 0, 20, 20, bg))
	scene2.PushLayer()
	scene2.InsertQuad(quadAt(0, 0, 20, 20, fg))
	scene2.PopLayer()
	scene2.Finish()
	r.Clear()
	r.PaintScene(scene2)
	img = r.Image()
	pr, _, _, _ = pixel(img, 10, 10)
	approx(t, "layer above earlier quad", pr, 1)
}

func TestRoundedCorners(t *testing.T) {
	r := NewRasterizer(30, 30, NewAtlas())
	scene := &Scene{}
	q := quadAt(5, 5, 20, 20, gc.White())
	q.CornerRadii = geometry.Corners[geometry.ScaledPixels]{
		TopLeft: ScaledPx(10), TopRight: ScaledPx(10),
		BottomRight: ScaledPx(10), BottomLeft: ScaledPx(10),
	}
	scene.InsertQuad(q)
	scene.Finish()
	r.Clear()
	r.PaintScene(scene)
	img := r.Image()
	// Center is filled.
	_, _, _, pa := pixel(img, 15, 15)
	approx(t, "center", pa, 1)
	// Top-left corner of the bounds is empty (inside the corner radius).
	_, _, _, pa = pixel(img, 6, 6)
	if pa > 0.35 {
		t.Fatalf("rounded corner should be mostly empty: %v", pa)
	}
	// Along the top edge near the middle it is filled.
	_, _, _, pa = pixel(img, 15, 6)
	approx(t, "top edge center", pa, 1)
}

func TestGradient(t *testing.T) {
	r := NewRasterizer(40, 10, NewAtlas())
	scene := &Scene{}
	grad := gc.LinearGradient(0, gc.NewLinearColorStop(gc.Black(), 0), gc.NewLinearColorStop(gc.White(), 1))
	q := quadAt(0, 0, 40, 10, gc.Hsla{})
	q.Background = grad
	scene.InsertQuad(q)
	scene.Finish()
	r.Clear()
	r.PaintScene(scene)
	img := r.Image()
	// Angle 0 = towards top: vertical gradient; near x-independent.
	l0, _, _, a0 := pixel(img, 20, 1) // near top
	l1, _, _, a1 := pixel(img, 20, 8) // near bottom
	approx(t, "alpha top", a0, 1)
	approx(t, "alpha bottom", a1, 1)
	if l0 <= l1 {
		t.Fatalf("gradient should be lighter at top (CSS angle 0 points up): %v vs %v", l0, l1)
	}
	// 90deg = towards right: horizontal.
	r2 := NewRasterizer(40, 10, NewAtlas())
	s2 := &Scene{}
	grad2 := gc.LinearGradient(90, gc.NewLinearColorStop(gc.Black(), 0), gc.NewLinearColorStop(gc.White(), 1))
	q2 := quadAt(0, 0, 40, 10, gc.Hsla{})
	q2.Background = grad2
	s2.InsertQuad(q2)
	s2.Finish()
	r2.Clear()
	r2.PaintScene(s2)
	img2 := r2.Image()
	c0, _, _, _ := pixel(img2, 1, 5)
	c1, _, _, _ := pixel(img2, 38, 5)
	if c1 <= c0 {
		t.Fatalf("horizontal gradient wrong: %v -> %v", c0, c1)
	}
}

func TestShadowBlurDecays(t *testing.T) {
	r := NewRasterizer(60, 60, NewAtlas())
	scene := &Scene{}
	shape := geometry.Bounds[geometry.ScaledPixels]{
		Origin: geometry.Point[geometry.ScaledPixels]{X: ScaledPx(15), Y: ScaledPx(15)},
		Size:   geometry.Size[geometry.ScaledPixels]{Width: ScaledPx(30), Height: ScaledPx(30)},
	}
	cover := shape.Dilate(ScaledPx(24))
	scene.InsertShadow(Shadow{
		BlurRadius:  ScaledPx(4),
		Bounds:      cover,
		ShapeBounds: shape,
		ShapeRadii:  geometry.AllCorners(ScaledPx(4)),
		ContentMask: FullContentMask(),
		Color:       gc.Black(),
	})
	scene.Finish()
	r.Clear()
	r.PaintScene(scene)
	img := r.Image()
	// Alpha should be highest near the shape edge and decay outward.
	_, _, _, center := pixel(img, 30, 30)
	_, _, _, near := pixel(img, 47, 30)
	_, _, _, far := pixel(img, 57, 30)
	if center < 0.9 {
		t.Fatalf("center of shadow shape should be ~1: %v", center)
	}
	if near < far {
		t.Fatalf("alpha should decay with distance: near=%v far=%v", near, far)
	}
	if far > 0.1 {
		t.Fatalf("far tail should be tiny: %v", far)
	}
}

func TestUnderlineWavy(t *testing.T) {
	r := NewRasterizer(60, 12, NewAtlas())
	scene := &Scene{}
	scene.InsertUnderline(Underline{
		Bounds: geometry.Bounds[geometry.ScaledPixels]{
			Origin: geometry.Point[geometry.ScaledPixels]{X: ScaledPx(0), Y: ScaledPx(2)},
			Size:   geometry.Size[geometry.ScaledPixels]{Width: ScaledPx(60), Height: ScaledPx(6)},
		},
		ContentMask: FullContentMask(),
		Color:       gc.Blue(),
		Thickness:   ScaledPx(1),
		Wavy:        true,
	})
	scene.Finish()
	r.Clear()
	r.PaintScene(scene)
	img := r.Image()
	// Some pixels must be painted across the width.
	count := 0
	for x := 0; x < 60; x++ {
		for y := 0; y < 12; y++ {
			if _, _, _, a := pixel(img, x, y); a > 0.1 {
				count++
				break
			}
		}
	}
	if count < 50 {
		t.Fatalf("wavy underline should paint across the row: %d", count)
	}
}

func TestPathFillRectangle(t *testing.T) {
	r := NewRasterizer(30, 30, NewAtlas())
	scene := &Scene{}
	pb := NewPathBuilder()
	pb.MoveTo(geometry.Point[float32]{X: 5, Y: 5})
	pb.LineTo(geometry.Point[float32]{X: 25, Y: 5})
	pb.LineTo(geometry.Point[float32]{X: 25, Y: 20})
	pb.LineTo(geometry.Point[float32]{X: 5, Y: 20})
	pb.Close()
	path, ok := pb.Build()
	if !ok {
		t.Fatal("path build")
	}
	scene.InsertPath(PaintedPath{
		ContentMask: FullContentMask(),
		Path:        path,
		Color:       solid(gc.Green()),
	})
	scene.Finish()
	r.Clear()
	r.PaintScene(scene)
	img := r.Image()
	_, _, _, pa := pixel(img, 15, 12)
	approx(t, "inside path", pa, 1)
	_, _, _, pa = pixel(img, 2, 2)
	approx(t, "outside path", pa, 0)
}

func TestPathFillCircleAndEvenOdd(t *testing.T) {
	r := NewRasterizer(40, 40, NewAtlas())
	scene := &Scene{}
	pb := NewPathBuilder()
	pb.AddCircle(geometry.Point[float32]{X: 20, Y: 20}, 12)
	path, _ := pb.Build()
	scene.InsertPath(PaintedPath{
		ContentMask: FullContentMask(),
		Path:        path,
		Color:       solid(gc.White()),
	})
	scene.Finish()
	r.Clear()
	r.PaintScene(scene)
	img := r.Image()
	_, _, _, pa := pixel(img, 20, 20)
	if pa < 0.9 {
		t.Fatalf("circle center: %v", pa)
	}
	_, _, _, pa = pixel(img, 20, 6)
	if pa > 0.1 {
		t.Fatalf("above circle: %v", pa)
	}
	// Edge should be antialiased (partial coverage).
	_, _, _, pa = pixel(img, 32, 20)
	if pa >= 1.0 {
		t.Fatalf("circle edge should be partially covered, got %v", pa)
	}
}

func TestAtlasRoundTrip(t *testing.T) {
	a := NewAtlas()
	// Insert a 2x2 mono tile.
	tile, err := a.GetOrInsert(AtlasKey{Kind: AtlasMonochrome, ImageKey: "test"}, func() (TileContent, error) {
		return TileContent{Size: geometry.SizeDevice(2, 2), Pixels: []byte{0, 128, 200, 255}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if tile.Bounds.Size.Width.AsI32() != 2 {
		t.Fatal("tile size")
	}
	// Same key returns same tile.
	tile2, _ := a.GetOrInsert(AtlasKey{Kind: AtlasMonochrome, ImageKey: "test"}, func() (TileContent, error) {
		t.Fatal("should not rebuild")
		return TileContent{}, nil
	})
	if tile2 != tile {
		t.Fatal("cache hit")
	}
	// Sample through the rasterizer.
	r := NewRasterizer(2, 2, a)
	scene := &Scene{}
	scene.InsertMonoSprite(MonochromeSprite{
		Tile: tile,
		Bounds: geometry.Bounds[geometry.ScaledPixels]{
			Origin: geometry.Point[geometry.ScaledPixels]{X: 0, Y: 0},
			Size:   geometry.Size[geometry.ScaledPixels]{Width: ScaledPx(2), Height: ScaledPx(2)},
		},
		ContentMask: FullContentMask(),
		Color:       gc.White(),
	})
	scene.Finish()
	r.Clear()
	r.PaintScene(scene)
	img := r.Image()
	_, _, _, pa := pixel(img, 1, 1)
	approx(t, "mono sprite alpha", pa, 1)
	_, _, _, pa = pixel(img, 0, 0)
	approx(t, "mono sprite zero", pa, 0)
}

func TestPolySprite(t *testing.T) {
	a := NewAtlas()
	tile, _ := a.GetOrInsert(AtlasKey{Kind: AtlasPolychrome, ImageKey: "img"}, func() (TileContent, error) {
		return TileContent{Size: geometry.SizeDevice(2, 2), Pixels: []uint8{
			255, 0, 0, 255, 255, 0, 0, 255,
			255, 0, 0, 255, 255, 0, 0, 255,
		}}, nil
	})
	r := NewRasterizer(2, 2, a)
	scene := &Scene{}
	scene.InsertPolySprite(PolychromeSprite{
		Tile: tile,
		Bounds: geometry.Bounds[geometry.ScaledPixels]{
			Origin: geometry.Point[geometry.ScaledPixels]{X: 0, Y: 0},
			Size:   geometry.Size[geometry.ScaledPixels]{Width: ScaledPx(2), Height: ScaledPx(2)},
		},
		ContentMask: FullContentMask(),
		Opacity:     1,
	})
	scene.Finish()
	r.Clear()
	r.PaintScene(scene)
	img := r.Image()
	pr, _, _, pa := pixel(img, 1, 1)
	approx(t, "poly r", pr, 1)
	approx(t, "poly alpha", pa, 1)
}

func TestContentMaskClips(t *testing.T) {
	r := NewRasterizer(20, 20, NewAtlas())
	scene := &Scene{}
	q := quadAt(0, 0, 20, 20, gc.White())
	q.ContentMask = ContentMask{Bounds: geometry.Bounds[geometry.ScaledPixels]{
		Origin: geometry.Point[geometry.ScaledPixels]{X: ScaledPx(0), Y: ScaledPx(0)},
		Size:   geometry.Size[geometry.ScaledPixels]{Width: ScaledPx(10), Height: ScaledPx(20)},
	}}
	scene.InsertQuad(q)
	scene.Finish()
	r.Clear()
	r.PaintScene(scene)
	img := r.Image()
	_, _, _, pa := pixel(img, 5, 10)
	approx(t, "inside mask", pa, 1)
	_, _, _, pa = pixel(img, 15, 10)
	approx(t, "outside mask", pa, 0)
}

func TestBorderQuad(t *testing.T) {
	r := NewRasterizer(30, 30, NewAtlas())
	scene := &Scene{}
	q := Quad{
		Bounds: geometry.Bounds[geometry.ScaledPixels]{
			Origin: geometry.Point[geometry.ScaledPixels]{X: ScaledPx(5), Y: ScaledPx(5)},
			Size:   geometry.Size[geometry.ScaledPixels]{Width: ScaledPx(20), Height: ScaledPx(20)},
		},
		ContentMask:  FullContentMask(),
		Background:   solid(gc.RGB(0x0000ff).Hsla()),
		BorderColor:  gc.RGB(0xff0000).Hsla(),
		BorderWidths: geometry.AllEdges(ScaledPx(3)),
	}
	scene.InsertQuad(q)
	scene.Finish()
	r.Clear()
	r.PaintScene(scene)
	img := r.Image()
	// Border region is red.
	pr, _, pb, _ := pixel(img, 6, 15)
	approx(t, "border r", pr, 1)
	approx(t, "border b", pb, 0)
	// Interior is blue.
	pr, _, pb, _ = pixel(img, 15, 15)
	approx(t, "interior r", pr, 0)
	approx(t, "interior b", pb, 1)
}
