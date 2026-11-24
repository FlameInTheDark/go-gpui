package render

import (
	"testing"

	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
)

func BenchmarkSoftwareGlyphs(b *testing.B) {
	atlas := NewAtlas()
	const width, height = 11, 17
	pixels := make([]uint8, width*height)
	for i := range pixels {
		pixels[i] = uint8(i * 37)
	}
	tile, err := atlas.GetOrInsert(AtlasKey{Kind: AtlasMonochrome, ImageKey: "bench"}, func() (TileContent, error) {
		return TileContent{Size: geometry.SizeDevice(width, height), Pixels: pixels}, nil
	})
	if err != nil {
		b.Fatal(err)
	}
	r := NewRasterizer(720, 420, atlas)
	sp := MonochromeSprite{ContentMask: FullContentMask(), Tile: tile, Color: gc.White(), Transform: geometry.IdentityMatrix()}
	sp.Bounds.Size = geometry.Size[geometry.ScaledPixels]{Width: width, Height: height}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Clear()
		for y := 0; y < 20; y++ {
			sp.Bounds.Origin.Y = geometry.ScaledPx(float32(y)*20 + 0.25)
			for x := 0; x < 60; x++ {
				sp.Bounds.Origin.X = geometry.ScaledPx(float32(x)*12 + 0.25)
				r.paintMonoSprite(sp)
			}
		}
	}
}

func BenchmarkSoftwareRoundedQuads(b *testing.B) {
	r := NewRasterizer(800, 600, nil)
	qs := [24]Quad{}
	for i := range qs {
		qs[i] = quadAt(float32(i%4)*200+0.25, float32(i/4)*100+0.25, 195, 95, gc.White())
		qs[i].CornerRadii = geometry.Corners[geometry.ScaledPixels]{TopLeft: 6, TopRight: 8, BottomLeft: 10, BottomRight: 12}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Clear()
		for _, q := range qs {
			r.paintQuad(q)
		}
	}
}
