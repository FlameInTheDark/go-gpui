// Package software contains common framebuffer storage for CPU backends.
package software

import "github.com/FlameInTheDark/go-gpui/render"

// Framebuffer reuses a rasterizer until the device-pixel size or atlas changes.
// It belongs to a single window and is not safe for concurrent use.
type Framebuffer struct {
	raster *render.Rasterizer
	atlas  *render.Atlas
	image  render.RenderImage
}

func (f *Framebuffer) paint(scene *render.Scene, atlas *render.Atlas, width, height int) *render.Rasterizer {
	reuse := false
	if f.raster != nil && f.atlas == atlas {
		w, h := f.raster.Size()
		reuse = w == width && h == height
	}
	if reuse {
		f.raster.Clear()
	} else {
		f.raster = render.NewRasterizer(width, height, atlas)
		f.atlas = atlas
	}
	f.raster.PaintScene(scene)
	return f.raster
}

// Draw returns a borrowed image for native presentation. The next Draw
// overwrites it.
func (f *Framebuffer) Draw(scene *render.Scene, atlas *render.Atlas, width, height int) *render.RenderImage {
	f.paint(scene, atlas, width, height).ImageInto(&f.image)
	return &f.image
}

// Snapshot returns an independent image that callers can keep across frames.
func (f *Framebuffer) Snapshot(scene *render.Scene, atlas *render.Atlas, width, height int) *render.RenderImage {
	return f.paint(scene, atlas, width, height).Image()
}
