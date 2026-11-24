package render

import (
	"testing"

	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
)

// TestStrokeOpenPathRaster verifies that stroking an open polyline (the
// shape of a UI icon checkmark) rasterizes ink along the whole path with
// no spurious closing segment, and that both stroke sides survive the
// sign-normalized curve reconstruction (regression: contours() used to
// drop the endpoint of direction-flipped curves).
func TestStrokeOpenPathRaster(t *testing.T) {
	r := NewRasterizer(40, 40, NewAtlas())
	scene := &Scene{}
	pb := NewPathBuilder()
	pb.MoveTo(geometry.Point[float32]{X: 2, Y: 8})
	pb.LineTo(geometry.Point[float32]{X: 6, Y: 12})
	pb.LineTo(geometry.Point[float32]{X: 14, Y: 3})
	stroked, ok := pb.Stroke(4).Build()
	if !ok {
		t.Fatal("stroke build failed")
	}
	if want := 9; len(stroked.Curves) < want-1 {
		t.Fatalf("stroke outline has %d curves, want >= %d (contour endpoint dropped?)", len(stroked.Curves), want-1)
	}
	scene.InsertPath(PaintedPath{
		ContentMask: FullContentMask(),
		Path:        stroked,
		Color:       solid(gc.Green()),
	})
	scene.Finish()
	r.Clear()
	r.PaintScene(scene)
	img := r.Image()

	ink := 0
	for i := 0; i < img.Width*img.Height; i++ {
		if img.Pixels[i*4+3] > 128 {
			ink++
		}
	}
	if ink < 30 {
		t.Fatalf("stroked checkmark rasterized only %d pixels; stroke pipeline broken", ink)
	}
	// Ink must appear near all three anchor points, not only near the start.
	near := func(px, py int) bool {
		for dy := -2; dy <= 2; dy++ {
			for dx := -2; dx <= 2; dx++ {
				x, y := px+dx, py+dy
				if x < 0 || y < 0 || x >= img.Width || y >= img.Height {
					continue
				}
				if img.Pixels[(y*img.Width+x)*4+3] > 100 {
					return true
				}
			}
		}
		return false
	}
	for _, p := range []struct{ x, y int }{{4, 10}, {6, 12}, {11, 7}} {
		if !near(p.x, p.y) {
			t.Errorf("no stroke ink near path point (%d,%d)", p.x, p.y)
		}
	}
	// An explicitly closed variant adds a segment back to the start, so
	// it must leave strictly more ink than the open stroke; if Build()
	// auto-closed open paths the two counts would match.
	closedPB := NewPathBuilder()
	closedPB.MoveTo(geometry.Point[float32]{X: 2, Y: 8})
	closedPB.LineTo(geometry.Point[float32]{X: 6, Y: 12})
	closedPB.LineTo(geometry.Point[float32]{X: 14, Y: 3})
	closedPB.Close()
	closedPath, ok := closedPB.Stroke(4).Build()
	if !ok {
		t.Fatal("closed stroke build failed")
	}
	scene2 := &Scene{}
	scene2.InsertPath(PaintedPath{ContentMask: FullContentMask(), Path: closedPath, Color: solid(gc.Green())})
	scene2.Finish()
	r2 := NewRasterizer(40, 40, NewAtlas())
	r2.Clear()
	r2.PaintScene(scene2)
	img2 := r2.Image()
	ink2 := 0
	for i := 0; i < img2.Width*img2.Height; i++ {
		if img2.Pixels[i*4+3] > 128 {
			ink2++
		}
	}
	if ink2 <= ink {
		t.Errorf("closed stroke ink (%d) should exceed open stroke ink (%d)", ink2, ink)
	}
}

// TestStrokeClosedPathRaster verifies closed contours still stroke.
func TestStrokeClosedPathRaster(t *testing.T) {
	r := NewRasterizer(40, 40, NewAtlas())
	scene := &Scene{}
	pb := NewPathBuilder()
	pb.MoveTo(geometry.Point[float32]{X: 10, Y: 10})
	pb.LineTo(geometry.Point[float32]{X: 30, Y: 10})
	pb.LineTo(geometry.Point[float32]{X: 30, Y: 30})
	pb.LineTo(geometry.Point[float32]{X: 10, Y: 30})
	pb.Close()
	stroked, ok := pb.Stroke(2).Build()
	if !ok {
		t.Fatal("stroke build failed")
	}
	scene.InsertPath(PaintedPath{
		ContentMask: FullContentMask(),
		Path:        stroked,
		Color:       solid(gc.White()),
	})
	scene.Finish()
	r.Clear()
	r.PaintScene(scene)
	img := r.Image()
	// On the top border's center pixel (x=20, y=10).
	onBorder := img.Pixels[(10*img.Width+20)*4+3]
	if onBorder < 230 {
		t.Fatalf("no ink on stroked border: %v", onBorder)
	}
	// The stroke ring's outside (center of the rect) stays clean.
	if inside := img.Pixels[(20*img.Width+20)*4+3]; inside > 20 {
		t.Fatalf("stroke leaked inside the ring: %v", inside)
	}
}

// TestStrokeJoinsCoverCorners pins the union-based stroke outline: a
// multi-segment closed path must leave ink ON every vertex (joins) and must
// not spill far outside the outline. The previous offset-polyline walk
// skipped alternating offset points at every join, which clipped corners and
// threw diagonal spikes - at 14px icon sizes the shapes collapsed into
// corner-bracket fragments (the broken tree/tab icons).
func TestStrokeJoinsCoverCorners(t *testing.T) {
	r := NewRasterizer(48, 48, NewAtlas())
	scene := &Scene{}
	pb := NewPathBuilder()
	// File-sheet style closed path: a rectangle with a folded corner.
	pb.MoveTo(geometry.Point[float32]{X: 6, Y: 4})
	pb.LineTo(geometry.Point[float32]{X: 24, Y: 4})
	pb.LineTo(geometry.Point[float32]{X: 32, Y: 12})
	pb.LineTo(geometry.Point[float32]{X: 32, Y: 34})
	pb.LineTo(geometry.Point[float32]{X: 6, Y: 34})
	pb.Close()
	stroked, ok := pb.Stroke(3).Build()
	if !ok {
		t.Fatal("stroke build failed")
	}
	scene.InsertPath(PaintedPath{
		ContentMask: FullContentMask(),
		Path:        stroked,
		Color:       solid(gc.Green()),
	})
	scene.Finish()
	r.Clear()
	r.PaintScene(scene)
	img := r.Image()

	near := func(px, py int) bool {
		for dy := -2; dy <= 2; dy++ {
			for dx := -2; dx <= 2; dx++ {
				x, y := px+dx, py+dy
				if x < 0 || y < 0 || x >= img.Width || y >= img.Height {
					continue
				}
				if img.Pixels[(y*img.Width+x)*4+3] > 100 {
					return true
				}
			}
		}
		return false
	}
	for _, p := range [][2]int{{6, 4}, {24, 4}, {32, 12}, {32, 34}, {6, 34}} {
		if !near(p[0], p[1]) {
			t.Errorf("no stroke ink near vertex (%d,%d); join not covered", p[0], p[1])
		}
	}
	// The old spike shot diagonally past the folded corner; a point clearly
	// outside the sheet must stay clean.
	if a := img.Pixels[(40*img.Width+36)*4+3]; a > 40 {
		t.Errorf("stroke spilled outside the outline (alpha %d at 36,40)", a)
	}
	if a := img.Pixels[(2*img.Width+2)*4+3]; a > 40 {
		t.Errorf("stroke spilled outside the outline (alpha %d at 2,2)", a)
	}
}
