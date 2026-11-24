// Unit tests for the pure-Go clipboard DIB codec. These run on every
// platform (no win32 calls involved); real clipboard I/O can only be
// exercised interactively on Windows.
package windows

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"testing"
)

// rainbow builds a w x h RGBA image with deterministic mixed-alpha pixels.
func rainbow(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(x * 37 % 256),
				G: uint8(y * 53 % 256),
				B: uint8((x + y) * 29 % 256),
				A: uint8((x*y + 17) % 256),
			})
		}
	}
	return img
}

func assertImagesEqual(t *testing.T, want, got image.Image) {
	t.Helper()
	wb, gb := want.Bounds(), got.Bounds()
	if wb != gb {
		t.Fatalf("bounds differ: want %v got %v", wb, gb)
	}
	for y := wb.Min.Y; y < wb.Max.Y; y++ {
		for x := wb.Min.X; x < wb.Max.X; x++ {
			wr, wg, wbl, wa := want.At(x, y).RGBA()
			gr, gg, gbl, ga := got.At(x, y).RGBA()
			if wr != gr || wg != gg || wbl != gbl || wa != ga {
				t.Fatalf("pixel (%d,%d) differs: want %v got %v", x, y,
					color.RGBA{uint8(wr >> 8), uint8(wg >> 8), uint8(wbl >> 8), uint8(wa >> 8)},
					color.RGBA{uint8(gr >> 8), uint8(gg >> 8), uint8(gbl >> 8), uint8(ga >> 8)})
			}
		}
	}
}

// TestDIBRoundTrip encodes an image and decodes it back: every channel must
// survive (mixed alpha avoids the all-zero-opaque heuristic).
func TestDIBRoundTrip(t *testing.T) {
	src := rainbow(7, 5)
	img, ok := parseDIB(encodeDIB(src))
	if !ok {
		t.Fatal("parseDIB rejected our own encoding")
	}
	assertImagesEqual(t, src, img)
}

// TestEncodeDIBLayout verifies the exact wire bytes: 40-byte header,
// negative (top-down) height, BGRA pixel order.
func TestEncodeDIBLayout(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 1, 2))
	src.SetRGBA(0, 0, color.RGBA{0x11, 0x22, 0x33, 0xFF}) // top row
	src.SetRGBA(0, 1, color.RGBA{0x44, 0x55, 0x66, 0x80}) // bottom row
	b := encodeDIB(src)
	if len(b) != 40+2*4 {
		t.Fatalf("unexpected length %d", len(b))
	}
	if got := binary.LittleEndian.Uint32(b[0:4]); got != 40 {
		t.Fatalf("biSize = %d, want 40", got)
	}
	if got := int32(binary.LittleEndian.Uint32(b[8:12])); got != -2 {
		t.Fatalf("biHeight = %d, want -2 (top-down)", got)
	}
	if got := binary.LittleEndian.Uint16(b[14:16]); got != 32 {
		t.Fatalf("biBitCount = %d, want 32", got)
	}
	if got := binary.LittleEndian.Uint32(b[16:20]); got != 0 {
		t.Fatalf("biCompression = %d, want BI_RGB", got)
	}
	// Top-down: scan line 0 is the first image row; bytes are BGRA.
	if !bytes.Equal(b[40:44], []byte{0x33, 0x22, 0x11, 0xFF}) {
		t.Fatalf("row 0 = %v, want BGRX of #112233", b[40:44])
	}
	if !bytes.Equal(b[44:48], []byte{0x66, 0x55, 0x44, 0x80}) {
		t.Fatalf("row 1 = %v, want BGRX of #445566", b[44:48])
	}
}

// TestParseDIB24bppBottomUp hand-builds a classic 24bpp bottom-up DIB
// (height positive, rows DWORD-padded) and checks row order + channels.
func TestParseDIB24bppBottomUp(t *testing.T) {
	// 2x2 image; rows are 6 bytes each padded to 8.
	dib := []byte{
		40, 0, 0, 0, // biSize
		2, 0, 0, 0, // biWidth
		2, 0, 0, 0, // biHeight (positive = bottom-up)
		1, 0, // biPlanes
		24, 0, // biBitCount
		0, 0, 0, 0, // biCompression = BI_RGB
		0, 0, 0, 0, // biSizeImage
		0, 0, 0, 0, 0, 0, 0, 0, // resolution
		0, 0, 0, 0, 0, 0, 0, 0, // clr used/important
		// bottom scan line (image row 1): B G R
		0x10, 0x20, 0x30, 0x40, 0x50, 0x60, 0, 0,
		// top scan line (image row 0):
		0x70, 0x80, 0x90, 0xA0, 0xB0, 0xC0, 0, 0,
	}
	img, ok := parseDIB(dib)
	if !ok {
		t.Fatal("parseDIB rejected a valid 24bpp DIB")
	}
	if got := img.At(0, 0).(color.RGBA); got != (color.RGBA{R: 0x90, G: 0x80, B: 0x70, A: 0xFF}) {
		t.Fatalf("pixel (0,0) = %v", got)
	}
	if got := img.At(1, 0).(color.RGBA); got != (color.RGBA{R: 0xC0, G: 0xB0, B: 0xA0, A: 0xFF}) {
		t.Fatalf("pixel (1,0) = %v", got)
	}
	if got := img.At(0, 1).(color.RGBA); got != (color.RGBA{R: 0x30, G: 0x20, B: 0x10, A: 0xFF}) {
		t.Fatalf("pixel (0,1) = %v", got)
	}
}

// TestParseDIBBitFields checks the 40-byte-header BI_BITFIELDS layout
// (three mask DWORDs follow the header) and mask scaling.
func TestParseDIBBitFields(t *testing.T) {
	dib := make([]byte, 52+2*8) // 40-byte header + 3 mask DWORDs + 2 rows of 8 bytes
	binary.LittleEndian.PutUint32(dib[0:4], 40)
	binary.LittleEndian.PutUint32(dib[4:8], 2)  // width
	binary.LittleEndian.PutUint32(dib[8:12], 2) // height (bottom-up)
	binary.LittleEndian.PutUint16(dib[14:16], 32)
	binary.LittleEndian.PutUint32(dib[16:20], 3) // BI_BITFIELDS
	// 565-style masks to exercise scaling (zero alpha mask).
	binary.LittleEndian.PutUint32(dib[40:44], 0xF800) // red
	binary.LittleEndian.PutUint32(dib[44:48], 0x07E0) // green
	binary.LittleEndian.PutUint32(dib[48:52], 0x001F) // blue
	// One pixel with R=31(max) G=0 B=31(max): pixel value 0xF81F.
	binary.LittleEndian.PutUint32(dib[52:56], 0x00F81F)
	binary.LittleEndian.PutUint32(dib[56:60], 0)

	img, ok := parseDIB(dib)
	if !ok {
		t.Fatal("parseDIB rejected a valid BI_BITFIELDS DIB")
	}
	if got := img.At(0, 1).(color.RGBA); got.R != 0xFF || got.G != 0 || got.B != 0xFF || got.A != 0xFF {
		t.Fatalf("pixel (0,1) = %v, want R=255 B=255 (scaled from 5-bit max)", got)
	}
}

// TestParseDIBZeroAlphaPlane documents the real-world heuristic: a 32bpp
// BI_RGB DIB whose alpha plane is uniformly zero reads as opaque.
func TestParseDIBZeroAlphaPlane(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			src.SetRGBA(x, y, color.RGBA{uint8(x * 80), uint8(y * 100), 9, 0})
		}
	}
	img, ok := parseDIB(encodeDIB(src))
	if !ok {
		t.Fatal("parseDIB failed")
	}
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			if got := img.At(x, y).(color.RGBA); got.A != 0xFF || got.R != src.At(x, y).(color.RGBA).R {
				t.Fatalf("pixel (%d,%d) = %v, want opaque with source RGB", x, y, got)
			}
		}
	}
}

// TestParseDIBRejectsGarbage guards against out-of-bounds panics on
// truncated or unsupported payloads.
func TestParseDIBRejectsGarbage(t *testing.T) {
	cases := [][]byte{
		nil,
		[]byte{1, 2, 3},  // truncated header
		make([]byte, 40), // zero width/height
		{1, 0, 0, 0},     // biSize too small
		func() []byte { // valid header, missing pixels
			dib := make([]byte, 40)
			binary.LittleEndian.PutUint32(dib[0:4], 40)
			binary.LittleEndian.PutUint32(dib[4:8], 4)
			binary.LittleEndian.PutUint32(dib[8:12], 4)
			binary.LittleEndian.PutUint16(dib[14:16], 32)
			return dib
		}(),
		func() []byte { // RLE compression
			dib := make([]byte, 40+4)
			binary.LittleEndian.PutUint32(dib[0:4], 40)
			binary.LittleEndian.PutUint32(dib[4:8], 1)
			binary.LittleEndian.PutUint32(dib[8:12], 1)
			binary.LittleEndian.PutUint16(dib[14:16], 8)
			binary.LittleEndian.PutUint32(dib[16:20], 1) // BI_RLE8
			return dib
		}(),
	}
	for i, dib := range cases {
		if img, ok := parseDIB(dib); ok || img != nil {
			t.Fatalf("case %d: unexpected success", i)
		}
	}
}

// TestEncodeDIBEmpty encodes nothing and must return nil.
func TestEncodeDIBEmpty(t *testing.T) {
	if b := encodeDIB(image.NewRGBA(image.Rect(0, 0, 0, 5))); b != nil {
		t.Fatal("expected nil for zero-width image")
	}
}
