package render

import (
	"math"
	"testing"

	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
)

func TestPathSubrowCoverageMatchesRayCasting(t *testing.T) {
	b := NewPathBuilder()
	// Overlapping, subpixel-aligned curves exercise both fill rules and
	// more intersections on a subrow than fit in the inline scratch array.
	for i := 0; i < 10; i++ {
		b.AddCircle(geometry.Point[float32]{X: 12.25 + 2*float32(i), Y: 14.75}, 9)
	}
	p, ok := b.Build()
	if !ok {
		t.Fatal("could not build path")
	}
	for _, evenOdd := range []bool{false, true} {
		p.EvenOdd = evenOdd
		r := NewRasterizer(48, 30, NewAtlas())
		r.paintPath(PaintedPath{Path: p, ContentMask: FullContentMask(), Color: solid(gc.White())})
		img := r.Image()
		for y := 0; y < img.Height; y++ {
			for x := 0; x < img.Width; x++ {
				covered := 0
				for _, dy := range []float32{0.25, 0.75} {
					for _, dx := range []float32{0.25, 0.75} {
						w := referencePathWinding(float32(x)+dx, float32(y)+dy, p.Curves)
						if (evenOdd && w%2 != 0) || (!evenOdd && w != 0) {
							covered++
						}
					}
				}
				if got, want := img.Pixels[(y*img.Width+x)*4+3], toByte(float32(covered)/4); got != want {
					t.Fatalf("evenOdd=%v at (%d,%d): alpha=%d, want %d", evenOdd, x, y, got, want)
				}
			}
		}
	}
}

// referencePathWinding solves the curve equation independently for each
// sample, as the software renderer did before sharing work across a row.
func referencePathWinding(px, py float32, curves []PathCurve) int {
	winding := 0
	for _, c := range curves {
		if py < c.P0.Y || py >= c.P1.Y {
			continue
		}
		var roots []float32
		a, b := c.A.Y, c.B.Y
		if a != 0 {
			if d := b*b - 4*a*(c.P0.Y-py); d >= 0 {
				sq := float32(math.Sqrt(float64(d)))
				roots = append(roots, (-b+sq)/(2*a), (-b-sq)/(2*a))
			}
		} else if b != 0 {
			roots = append(roots, (py-c.P0.Y)/b)
		}
		for _, t := range roots {
			if t > 0 && t <= 1 && c.A.X*t*t+c.B.X*t+c.P0.X >= px {
				winding += int(c.Sign)
			}
		}
	}
	return winding
}
