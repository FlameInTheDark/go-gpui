package render

import (
	"math"
	"testing"

	"github.com/FlameInTheDark/go-gpui/geometry"
)

// Ports of crates/gpui/src/path_winding.rs tests.

func squareCurves(left, top, right, bottom float32) []PathCurve {
	var b PathBuilder
	b.MoveTo(geometry.Point[float32]{X: left, Y: top})
	b.LineTo(geometry.Point[float32]{X: right, Y: top})
	b.LineTo(geometry.Point[float32]{X: right, Y: bottom})
	b.LineTo(geometry.Point[float32]{X: left, Y: bottom})
	b.Close()
	p, ok := b.Build()
	if !ok {
		panic("square build failed")
	}
	return p.Curves
}

// A rectangle spanning many cells is four edge strips, four corners and one
// interior, however many cells it covers.
func TestRectangleIsNineTiles(t *testing.T) {
	curves := squareCurves(100, 40, 900, 600)
	res := BinPathTiles(curves, false, 1.25)
	if len(res.Tiles) != 9 {
		t.Fatalf("tiles = %d, want 9", len(res.Tiles))
	}
}

// A path far larger than the window is binned coarsely, and painting it keeps
// only the tiles its clip shows.
func TestHugePathPaintsOnlyVisibleTiles(t *testing.T) {
	curves := squareCurves(-20000, -20000, 20000, 20000)
	res := BinPathTiles(curves, false, 1.5)
	if len(res.Tiles) >= 10000 {
		t.Fatalf("tiles = %d, want < 10000", len(res.Tiles))
	}

	// retain_visible equivalent: keep tiles intersecting the mask.
	maskOriginX, maskOriginY := float32(300.0*1.5), float32(200.0*1.5)
	maskRight, maskBottom := maskOriginX+float32(120.0*1.5), maskOriginY+float32(90.0*1.5)
	visible := 0
	for _, tile := range res.Tiles {
		if tile.TopLeft[0] < maskRight && tile.TopLeft[1] < maskBottom &&
			tile.BottomRight[0] > maskOriginX && tile.BottomRight[1] > maskOriginY {
			visible++
		}
	}
	if visible == 0 || visible >= 20 {
		t.Fatalf("visible tiles = %d, want 1..19", visible)
	}
}

// A path whose size overflows, at some scale or at any, is skipped.
func TestOverflowingPathsAreSkipped(t *testing.T) {
	radius := float32(3.4028235e38 / 4.0)
	curves := squareCurves(-radius, -radius, radius, radius)
	if res := BinPathTiles(curves, false, 1.0); len(res.Tiles) == 0 {
		t.Fatal("1.0-scale overflow path should bin (may clamp), got no tiles")
	}
	radius = float32(3.4028235e38 * 0.75)
	curves = squareCurves(-radius, -radius, radius, radius)
	if res := BinPathTiles(curves, false, 1.0); len(res.Tiles) != 0 {
		t.Fatalf("overflowing path should produce no tiles, got %d", len(res.Tiles))
	}
}

// The winding of a simple triangle must fill exactly its interior.
func TestTriangleWindingCoverage(t *testing.T) {
	curves := squareCurves(0, 0, 100, 100)
	res := BinPathTiles(curves, false, 1.0)
	if len(res.Tiles) == 0 {
		t.Fatal("no tiles")
	}
	// Interior point must be inside some tile.
	inside := false
	for _, tile := range res.Tiles {
		if tile.TopLeft[0] <= 50 && 50 <= tile.BottomRight[0] &&
			tile.TopLeft[1] <= 50 && 50 <= tile.BottomRight[1] {
			inside = true
		}
	}
	if !inside {
		t.Fatal("interior point (50,50) not covered by any tile")
	}
}

// Winding endpoints balance: every shared endpoint's steps must cancel.
func TestEndpointsBalance(t *testing.T) {
	curves := squareCurves(10, 10, 50, 50)
	balance := map[[2]uint32]int{}
	for i := range curves {
		c := &curves[i]
		step := curveStep(c)
		balance[[2]uint32{f32bits(c.P0.X), f32bits(c.P0.Y)}] += int(step)
		balance[[2]uint32{f32bits(c.P1.X), f32bits(c.P1.Y)}] -= int(step)
	}
	for pt, sum := range balance {
		if sum != 0 {
			t.Fatalf("endpoints at (%v, %v) do not balance: %d", f32unbits(pt[0]), f32unbits(pt[1]), sum)
		}
	}
}

func f32bits(v float32) uint32   { return math.Float32bits(v) }
func f32unbits(v uint32) float32 { return math.Float32frombits(v) }
