package software

import (
	"testing"

	"github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/render"
)

func TestFramebufferReuseAndSnapshot(t *testing.T) {
	var f Framebuffer
	atlas := render.NewAtlas()
	scene := &render.Scene{}
	scene.InsertQuad(render.Quad{
		Bounds:      geometry.ScaleBounds(geometry.BoundsPixels(0, 0, 8, 8), 1),
		ContentMask: render.FullContentMask(),
		Background:  color.SolidBackground(color.White()),
	})
	first := f.Draw(scene, atlas, 4, 4)
	storage := &first.Pixels[0]
	raster := f.raster
	snapshot := f.Snapshot(scene, atlas, 4, 4)
	if snapshot.Pixels[3] != 255 {
		t.Fatal("first frame should be opaque")
	}
	second := f.Draw(&render.Scene{}, atlas, 4, 4)
	if &second.Pixels[0] != storage || f.raster != raster {
		t.Fatal("same-size draw did not reuse its buffers")
	}
	if second.Pixels[3] != 0 || snapshot.Pixels[3] != 255 {
		t.Fatal("same-size draw must clear the rasterizer without changing old snapshots")
	}
	resized := f.Draw(&render.Scene{}, atlas, 2, 3)
	if resized.Width != 2 || resized.Height != 3 || len(resized.Pixels) != 2*3*4 {
		t.Fatalf("resized frame is %dx%d with %d bytes", resized.Width, resized.Height, len(resized.Pixels))
	}
	if f.raster == raster || &resized.Pixels[0] != storage {
		t.Fatal("resize should replace the rasterizer but keep a large enough output slice")
	}
}
