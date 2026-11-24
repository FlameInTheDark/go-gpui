package gpui

import (
	"fmt"
	"image"

	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/render"
	"github.com/FlameInTheDark/go-gpui/style"
	"github.com/FlameInTheDark/go-gpui/text"
)

// snapBoundsPx rounds each edge to device pixels (upstream snap_bounds).
func snapBoundsPx(b geometry.Bounds[geometry.Pixels], scale float32) geometry.Bounds[geometry.Pixels] {
	x0 := geometry.RoundToDevicePixel(b.Origin.X.AsF32(), scale)
	y0 := geometry.RoundToDevicePixel(b.Origin.Y.AsF32(), scale)
	x1 := geometry.RoundToDevicePixel(b.Right().AsF32(), scale)
	y1 := geometry.RoundToDevicePixel(b.Bottom().AsF32(), scale)
	return geometry.Bounds[geometry.Pixels]{
		Origin: geometry.Point[geometry.Pixels]{X: geometry.Px(x0), Y: geometry.Px(y0)},
		Size:   geometry.Size[geometry.Pixels]{Width: geometry.Px(x1 - x0), Height: geometry.Px(y1 - y0)},
	}
}

// PaintQuad paints a quad primitive with upstream's snapping and opacity
// handling (window.paint_quad).
func (w *Window) PaintQuad(quadStyle style.Style, bounds geometry.Bounds[geometry.Pixels]) {
	scale := w.ScaleFactor()
	b := snapBoundsPx(bounds, scale)
	if b.Size.Width.AsF32() <= 0 || b.Size.Height.AsF32() <= 0 {
		return
	}
	rem := w.RemSize()

	hasBackground := quadStyle.Background != nil && !quadStyle.Background.IsTransparent()
	hasBorder := quadStyle.IsBorderVisible(rem)
	if !hasBackground && !hasBorder {
		return
	}

	background := quadStyle.Background
	if background == nil {
		background = &gc.Background{}
		background.Tag = gc.BackgroundTagSolid
		background.Solid = gc.Hsla{A: 0}
	}

	borderWidths := geometry.Edges[geometry.ScaledPixels]{
		Top:    geometry.ScaledPx(geometry.RoundStrokeToDevicePixel(quadStyle.BorderWidths.Top.ToPixels(rem).AsF32(), scale) * scale),
		Right:  geometry.ScaledPx(geometry.RoundStrokeToDevicePixel(quadStyle.BorderWidths.Right.ToPixels(rem).AsF32(), scale) * scale),
		Bottom: geometry.ScaledPx(geometry.RoundStrokeToDevicePixel(quadStyle.BorderWidths.Bottom.ToPixels(rem).AsF32(), scale) * scale),
		Left:   geometry.ScaledPx(geometry.RoundStrokeToDevicePixel(quadStyle.BorderWidths.Left.ToPixels(rem).AsF32(), scale) * scale),
	}
	// Clamp the corner radii to half the shorter side (upstream
	// clamp_radii_for_quad_size; rounded-full relies on this).
	cornerRadii := geometry.ScaleCorners(
		geometry.ClampCornerRadii(quadStyle.CornerRadii.ToPixels(rem), b.Size), scale)

	borderColor := gc.Hsla{A: 0}
	if quadStyle.BorderColor != nil {
		borderColor = *quadStyle.BorderColor
		borderColor.A *= w.elementOpacity
	}
	bg := background.Opacity(w.elementOpacity)

	w.nextFrame.scene.InsertQuad(render.Quad{
		Bounds:       geometry.ScaleBounds(b, scale),
		ContentMask:  w.ScaledContentMask(),
		Background:   bg,
		BorderColor:  borderColor,
		CornerRadii:  cornerRadii,
		BorderWidths: borderWidths,
		BorderStyle:  uint8(quadStyle.BorderStyle),
	})
}

// PaintDropShadow paints a drop shadow (upstream paint_shadow).
func (w *Window) PaintDropShadow(sh style.BoxShadow, bounds geometry.Bounds[geometry.Pixels], radii geometry.Corners[geometry.Pixels], scale float32) {
	offset := geometry.Point[geometry.Pixels]{X: geometry.Px(sh.OffsetX), Y: geometry.Px(sh.OffsetY)}
	shapeBounds := bounds
	shapeBounds.Origin.X += offset.X
	shapeBounds.Origin.Y += offset.Y
	shapeBounds = shapeBounds.Dilate(geometry.Px(sh.SpreadRadius))
	cover := shapeBounds.Dilate(geometry.Px(sh.BlurRadius * 3))
	color := sh.Color.Opacity(w.elementOpacity)
	w.nextFrame.scene.InsertShadow(render.Shadow{
		BlurRadius:  geometry.ScaledPx(sh.BlurRadius * scale),
		Bounds:      geometry.ScaleBounds(cover, scale),
		CornerRadii: geometry.ScaleCorners(radii, scale),
		ContentMask: w.ScaledContentMask(),
		Color:       color,
		ShapeBounds: geometry.ScaleBounds(shapeBounds, scale),
		ShapeRadii:  geometry.ScaleCorners(radii, scale),
	})
}

// PaintInsetShadow paints an inner shadow.
func (w *Window) PaintInsetShadow(sh style.BoxShadow, bounds geometry.Bounds[geometry.Pixels], radii geometry.Corners[geometry.Pixels], scale float32) {
	offset := geometry.Point[geometry.Pixels]{X: geometry.Px(sh.OffsetX), Y: geometry.Px(sh.OffsetY)}
	hole := bounds
	hole.Origin.X += offset.X
	hole.Origin.Y += offset.Y
	hole = hole.Dilate(geometry.Px(-sh.SpreadRadius))
	clampedRadii := radii
	if sh.SpreadRadius > 0 {
		clampedRadii = geometry.ClampCornerRadii(radii, bounds.Size)
	}
	w.nextFrame.scene.InsertShadow(render.Shadow{
		BlurRadius:  geometry.ScaledPx(sh.BlurRadius * scale),
		Bounds:      geometry.ScaleBounds(bounds, scale),
		CornerRadii: geometry.ScaleCorners(clampedRadii, scale),
		ContentMask: w.ScaledContentMask(),
		Color:       sh.Color.Opacity(w.elementOpacity),
		ShapeBounds: geometry.ScaleBounds(hole, scale),
		ShapeRadii:  geometry.ScaleCorners(clampedRadii, scale),
		Inset:       true,
	})
}

// PaintStyle paints the box (background, borders, shadows) around a
// continuation (upstream Style::paint ordering).
func (w *Window) PaintStyle(st style.Style, bounds geometry.Bounds[geometry.Pixels], continuation func()) {
	rem := w.RemSize()
	// Upstream clamps the radii once against the quad size and reuses them
	// for shadows and the quad.
	radii := geometry.ClampCornerRadii(st.CornerRadii.ToPixels(rem), bounds.Size)
	// Drop shadows, background, continuation, inset shadows (border painted
	// by PaintQuad).
	for _, sh := range st.BoxShadow {
		if !sh.Inset {
			w.PaintDropShadow(sh, bounds, radii, w.ScaleFactor())
		}
	}
	w.PaintQuad(st, bounds)
	if continuation != nil {
		continuation()
	}
	for _, sh := range st.BoxShadow {
		if sh.Inset {
			w.PaintInsetShadow(sh, bounds, radii, w.ScaleFactor())
		}
	}
}

// PaintUnderline paints an underline primitive.
func (w *Window) PaintUnderline(origin geometry.Point[geometry.Pixels], width, thickness geometry.Pixels, underStyle style.UnderlineStyle) {
	scale := w.ScaleFactor()
	h := thickness
	if underStyle.Wavy {
		h = thickness.MulScalar(3)
	}
	b := geometry.Bounds[geometry.Pixels]{
		Origin: origin,
		Size:   geometry.Size[geometry.Pixels]{Width: width, Height: h},
	}
	w.nextFrame.scene.InsertUnderline(render.Underline{
		Bounds:      geometry.ScaleBounds(snapBoundsPx(b, scale), scale),
		ContentMask: w.ScaledContentMask(),
		Color:       underStyle.Color.Opacity(w.elementOpacity),
		Thickness:   geometry.ScaledPx(thickness.AsF32() * scale),
		Wavy:        underStyle.Wavy,
	})
}

// PaintGlyph paints a single glyph through the atlas (upstream paint_glyph
// with 1/4px x-subpixel quantization).
func (w *Window) PaintGlyph(origin geometry.Point[geometry.Pixels], fontID text.FontID, glyphID uint32, fontSize geometry.Pixels, color gc.Hsla) {
	if w.text == nil {
		return
	}
	face := w.text.Face(fontID)
	if face == nil {
		return
	}
	scale := w.ScaleFactor()
	glyphOriginScaled := geometry.ScalePoint(origin, scale)
	// Quantize x to 1/4 px.
	qx := geometry.RoundHalfTowardZero(glyphOriginScaled.X.AsF32()*4) / 4
	integerOriginX := float32(int(qx))
	subpixelX := qx - integerOriginX
	integerOriginY := float32(int(glyphOriginScaled.Y.AsF32()))

	tileKey := render.AtlasKey{
		Kind:        render.AtlasMonochrome,
		FontID:      int(fontID),
		GlyphID:     glyphID,
		FontSize:    fontSize.AsF32(),
		SubpixelX:   uint8(subpixelX * 4),
		ScaleFactor: scale,
	}
	var bearingX, bearingY float32
	tile, err := w.atlas.GetOrInsert(tileKey, func() (render.TileContent, error) {
		px := fontSize.AsF32() * scale
		gw, gh, bx, by, mask := text.RasterizeGlyph(face, uint16(glyphID), px, subpixelX)
		if gw == 0 || gh == 0 {
			return render.TileContent{Size: geometry.SizeDevice(0, 0)}, nil
		}
		// Bearing is stored on the tile so cache hits keep it
		// (upstream AtlasTile::offset).
		return render.TileContent{
			Size:   geometry.SizeDevice(int32(gw), int32(gh)),
			Pixels: mask,
			Offset: geometry.Point[geometry.DevicePixels]{
				X: geometry.DevicePx(int32(bx)),
				Y: geometry.DevicePx(int32(by)),
			},
		}, nil
	})
	if err != nil || tile.Bounds.Size.Width.AsI32() == 0 {
		return
	}
	bearingX = float32(tile.Offset.X.AsI32())
	bearingY = float32(tile.Offset.Y.AsI32())
	_ = integerOriginY
	bounds := geometry.Bounds[geometry.ScaledPixels]{
		Origin: geometry.Point[geometry.ScaledPixels]{
			X: geometry.ScaledPx(integerOriginX + bearingX),
			Y: geometry.ScaledPx(float32(int(glyphOriginScaled.Y.AsF32())) + bearingY),
		},
		Size: geometry.Size[geometry.ScaledPixels]{
			Width:  geometry.ScaledPx(float32(tile.Bounds.Size.Width.AsI32())),
			Height: geometry.ScaledPx(float32(tile.Bounds.Size.Height.AsI32())),
		},
	}
	w.nextFrame.scene.InsertMonoSprite(render.MonochromeSprite{
		Tile:        tile,
		Bounds:      bounds,
		ContentMask: w.ScaledContentMask(),
		Color:       color.Opacity(w.elementOpacity),
	})
}

// PaintImage paints an image with an object fit (upstream paint_image).
// The image is rasterized into the atlas once; the cache key is derived
// from the backing pixel buffer's identity, so repeated frames hit the
// atlas instead of re-copying the pixels every frame.
func (w *Window) PaintImage(bounds geometry.Bounds[geometry.Pixels], cornerRadii geometry.Corners[geometry.Pixels], img image.Image, grayscale bool) {
	scale := w.ScaleFactor()
	b := img.Bounds()
	key := render.AtlasKey{Kind: render.AtlasPolychrome, ImageKey: imageKeyOf(img)}
	tile, err := w.atlas.GetOrInsert(key, func() (render.TileContent, error) {
		px := make([]uint8, b.Dx()*b.Dy()*4)
		i := 0
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r, g, bl, a := img.At(x, y).RGBA()
				px[i] = uint8(r >> 8)
				px[i+1] = uint8(g >> 8)
				px[i+2] = uint8(bl >> 8)
				px[i+3] = uint8(a >> 8)
				i += 4
			}
		}
		return render.TileContent{Size: geometry.SizeDevice(int32(b.Dx()), int32(b.Dy())), Pixels: px}, nil
	})
	if err != nil {
		return
	}
	w.nextFrame.scene.InsertPolySprite(render.PolychromeSprite{
		Tile:        tile,
		Grayscale:   grayscale,
		Opacity:     w.elementOpacity,
		Bounds:      geometry.ScaleBounds(bounds, scale),
		ContentMask: w.ScaledContentMask(),
		CornerRadii: geometry.ScaleCorners(cornerRadii, scale),
	})
}

// imageKeyCounter disambiguates images whose backing buffer identity cannot
// be derived (rare: non-standard image.Image implementations).
var imageKeyCounter uint64

// imageKeyOf derives a stable atlas key from an image's backing buffer.
// Standard library images (RGBA/NRGBA/Gray/Uniform) expose their pixel
// storage; the pointer identity is stable across frames because the caller
// holds the image object. Unknown types fall back to a per-object key
// registered on first sight (still stable across frames, unlike a counter).
func imageKeyOf(img image.Image) string {
	switch v := img.(type) {
	case *image.RGBA:
		return fmtImageKey(v.Pix, v.Bounds())
	case *image.NRGBA:
		return fmtImageKey(v.Pix, v.Bounds())
	case *image.Gray:
		return fmtImageKey(v.Pix, v.Bounds())
	case *image.YCbCr:
		return fmtImageKey(v.Y[:1], v.Bounds())
	case *image.Uniform:
		r, _, _, _ := v.At(0, 0).RGBA()
		return fmt.Sprintf("img-uniform-%08x-%dx%d", uint32(r>>16)&0xffffff, v.Bounds().Dx(), v.Bounds().Dy())
	}
	// Unknown implementation: one key per object (address-stable while
	// the element holds a reference).
	imageKeyCounter++
	return fmt.Sprintf("img-object-%d", imageKeyCounter)
}

func fmtImageKey(pix []uint8, b image.Rectangle) string {
	if len(pix) == 0 {
		return fmt.Sprintf("img-empty-%dx%d", b.Dx(), b.Dy())
	}
	return fmt.Sprintf("img-%p-%d-%dx%d", &pix[0], len(pix), b.Dx(), b.Dy())
}

func fmtUint(v uint64) string {
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// PaintPath paints a filled path given in logical pixels. The curves are
// scaled to device pixels here so they land in the same space as the
// scaled Bounds and content mask (the rasterizer treats all three
// uniformly - see render.ScalePath).
func (w *Window) PaintPath(builder *render.PathBuilder, bounds geometry.Bounds[geometry.Pixels], color gc.Hsla) {
	path, ok := builder.Build()
	if !ok {
		return
	}
	w.nextFrame.scene.InsertPath(render.PaintedPath{
		Path:        render.ScalePath(path, w.ScaleFactor()),
		Bounds:      geometry.ScaleBounds(bounds, w.ScaleFactor()),
		ContentMask: w.ScaledContentMask(),
		Color:       gc.SolidBackground(color.Opacity(w.elementOpacity)),
	})
}
