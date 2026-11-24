package render

import (
	"slices"
	"testing"

	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
)

func TestAxisAlignedGlyphMatchesGeneralSampler(t *testing.T) {
	a := NewAtlas()
	for j := 0; j < 2; j++ {
		tile, err := a.GetOrInsert(AtlasKey{Kind: AtlasMonochrome, GlyphID: uint32(j + 1)}, func() (TileContent, error) {
			pixels := make([]uint8, 30)
			for i := range pixels {
				pixels[i] = uint8(i * 79)
			}
			return TileContent{Size: geometry.SizeDevice(5, 6), Pixels: pixels}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		got, want := NewRasterizer(32, 24, a), NewRasterizer(32, 24, a)
		id := geometry.IdentityMatrix()
		c := gc.RGB(0x8877ba).Hsla().Alpha(0.7)
		for _, bounds := range []geometry.Bounds[geometry.ScaledPixels]{
			geometry.ScaleBounds(geometry.BoundsPixels(-1.25, 1.25, 9.125, 14.75), 1),
			geometry.ScaleBounds(geometry.BoundsPixels(5.5, 4.5, 5, 6), 1),
		} {
			mask := ContentMask{Bounds: geometry.ScaleBounds(geometry.BoundsPixels(2, 2, 24, 10), 1)}
			got.paintMonoCommon(bounds, mask, tile, c, id)
			x0, y0, x1, y1, _ := want.pixelBounds(bounds, mask)
			data, w, h, _ := want.tileMono(tile.TextureIndex)
			want.paintMonoTransformed(bounds, tile, hslaToRgbaF(c), id, data, w, h, x0, y0, x1, y1)
			if !slices.Equal(got.pix, want.pix) {
				t.Fatalf("different output for tile %d, bounds %v", j, bounds)
			}
		}
	}
}

func TestRoundedQuadFastPathMatchesGeneralShader(t *testing.T) {
	for _, width := range []float32{0.25, 10, 38.75} {
		for _, alpha := range []float32{0.5, 1} {
			q := quadAt(4.25, 2.5, width, 25.5, gc.RGB(0x8877ba).Hsla().Alpha(alpha))
			q.CornerRadii = geometry.Corners[geometry.ScaledPixels]{TopLeft: 3, TopRight: 4, BottomLeft: 6, BottomRight: 2}
			q.ContentMask.Bounds = geometry.ScaleBounds(geometry.BoundsPixels(2, 4.25, 32, 21.5), 1)
			got, want := NewRasterizer(48, 32, nil), NewRasterizer(48, 32, nil)
			got.paintQuad(q)
			x0, y0, x1, y1, ok := want.pixelBounds(q.Bounds, q.ContentMask)
			prepared := prepareBackground(q.Background)
			for y := y0; ok && y <= y1; y++ {
				for x := x0; x <= x1; x++ {
					want.paintQuadEdge(q, prepared, x, y, 0.5, false)
				}
			}
			if !slices.Equal(got.pix, want.pix) {
				t.Fatalf("different output for width %.2f, alpha %.2f", width, alpha)
			}
		}
	}
}
