package x11

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"

	"github.com/FlameInTheDark/go-gpui/geometry"
)

// CaptureScreen grabs a rectangle of the root window via GetImage and
// returns it as an RGBA image. It is a debugging aid for headless test
// setups (Xvfb) to inspect what an interactive window actually painted.
func CaptureScreen(bounds geometry.Bounds[int32]) (*image.RGBA, error) {
	c, err := Connect()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	root := c.screen.RootWindow
	x, y := int(bounds.Origin.X), int(bounds.Origin.Y)
	w, h := int(bounds.Size.Width), int(bounds.Size.Height)
	if w <= 0 || h <= 0 {
		return nil, nil
	}

	// GetImage: opcode 73, format 2 (ZPixmap), drawable, x, y, w, h,
	// plane-mask 0xFFFFFFFF.
	req := newRequest(73, 2).
		u32(root).
		u16(uint16(x)).u16(uint16(y)).
		u16(uint16(w)).u16(uint16(h)).
		u32(0xFFFFFFFF)
	seq := c.nextSeq()
	if err := c.writeRequest(req.finalize()); err != nil {
		return nil, err
	}
	reply, err := c.readReply(seq)
	if err != nil {
		return nil, err
	}
	// X11 reply: 32-byte header, then one depth byte + 24-bit pad, pixel
	// data starts at offset 32.
	data := reply[32:]

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for py := 0; py < h; py++ {
		for px := 0; px < w; px++ {
			o := (py*w + px) * 4
			if o+3 >= len(data) {
				continue
			}
			// ZPixmap little-endian: BGRX byte order.
			b, g, r := data[o], data[o+1], data[o+2]
			img.SetRGBA(px, py, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	_ = binary.LittleEndian
	return img, nil
}

// CaptureScreenPNG grabs a rectangle of the root window as PNG bytes.
func CaptureScreenPNG(bounds geometry.Bounds[int32]) ([]byte, error) {
	img, err := CaptureScreen(bounds)
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, nil
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
