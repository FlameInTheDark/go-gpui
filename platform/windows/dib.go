// Clipboard DIB codec for the Windows platform backend.
//
// This file is pure Go on purpose: it parses and encodes Windows clipboard
// bitmaps (CF_DIB / CF_DIBV5 payloads) without touching user32/gdi32, so its
// unit tests run on every platform (a real clipboard cannot be exercised in
// CI, but the wire format can). It is kept free of build constraints and of
// any dependency on the win32-only files in this package.
package windows

import (
	"encoding/binary"
	"image"
	"image/color"
)

// BITMAPINFOHEADER constants (win32 wingdi.h).
const (
	dibCompressionBIRGB       = 0 // uncompressed
	dibCompressionBIBitfields = 3 // explicit channel masks

	dibBIHSize      = 40  // sizeof(BITMAPINFOHEADER)
	dibV4HSize      = 108 // sizeof(BITMAPV4HEADER)
	dibV5HSize      = 124 // sizeof(BITMAPV5HEADER)
	dibMaxHeaderLen = dibV5HSize

	dibHeaderSizeOffset  = 0  // biSize
	dibWidthOffset       = 4  // biWidth
	dibHeightOffset      = 8  // biHeight
	dibBitCountOffset    = 14 // biBitCount
	dibCompressionOffset = 16 // biCompression

	dibRedMaskOffset   = 40 // bV4RedMask (V4/V5 headers)
	dibGreenMaskOffset = 44
	dibBlueMaskOffset  = 48
	dibAlphaMaskOffset = 52
)

// dibHeader is the decoded BITMAPINFOHEADER of a clipboard DIB.
type dibHeader struct {
	width       int32
	height      int32 // positive = bottom-up rows, negative = top-down
	bitCount    uint16
	compression uint32
	// masks are the effective channel masks (0 when the channel is absent).
	rMask, gMask, bMask, aMask uint32
	// bitsOffset is where pixel data starts within the DIB blob.
	bitsOffset int
}

// parseDIBHeader decodes the BITMAPINFOHEADER (plus BITFIELDS masks where
// present) from the start of a CF_DIB/CF_DIBV5 payload. Only the formats the
// clipboard actually carries are accepted: 24bpp and 32bpp uncompressed or
// BI_BITFIELDS.
func parseDIBHeader(b []byte) (dibHeader, bool) {
	if len(b) < dibBIHSize {
		return dibHeader{}, false
	}
	h := dibHeader{
		width:       int32(binary.LittleEndian.Uint32(b[dibWidthOffset : dibWidthOffset+4])),
		height:      int32(binary.LittleEndian.Uint32(b[dibHeightOffset : dibHeightOffset+4])),
		bitCount:    binary.LittleEndian.Uint16(b[dibBitCountOffset : dibBitCountOffset+2]),
		compression: binary.LittleEndian.Uint32(b[dibCompressionOffset : dibCompressionOffset+4]),
	}
	biSize := int(binary.LittleEndian.Uint32(b[dibHeaderSizeOffset : dibHeaderSizeOffset+4]))
	if biSize < dibBIHSize || biSize > dibMaxHeaderLen || biSize > len(b) {
		return dibHeader{}, false
	}
	if h.width <= 0 || h.height == 0 || h.bitCount == 0 || h.bitCount > 32 {
		return dibHeader{}, false
	}
	switch h.compression {
	case dibCompressionBIRGB:
		switch h.bitCount {
		case 24:
			h.rMask, h.gMask, h.bMask = 0xFF0000, 0x00FF00, 0x0000FF
		case 32:
			// Implicit BGRA byte order.
			h.rMask, h.gMask, h.bMask, h.aMask = 0x00FF0000, 0x0000FF00, 0x000000FF, 0xFF000000
		default:
			return dibHeader{}, false // paletted formats are not worth it
		}
		h.bitsOffset = biSize
	case dibCompressionBIBitfields:
		if h.bitCount != 32 {
			return dibHeader{}, false
		}
		if biSize >= dibV4HSize {
			// BITMAPV4HEADER/V5HEADER carry the masks at fixed offsets.
			h.rMask = binary.LittleEndian.Uint32(b[dibRedMaskOffset : dibRedMaskOffset+4])
			h.gMask = binary.LittleEndian.Uint32(b[dibGreenMaskOffset : dibGreenMaskOffset+4])
			h.bMask = binary.LittleEndian.Uint32(b[dibBlueMaskOffset : dibBlueMaskOffset+4])
			h.aMask = binary.LittleEndian.Uint32(b[dibAlphaMaskOffset : dibAlphaMaskOffset+4])
			h.bitsOffset = biSize
		} else {
			// A 40-byte BITMAPINFOHEADER stores three mask DWORDs right
			// after itself (no alpha channel).
			if len(b) < biSize+12 {
				return dibHeader{}, false
			}
			h.rMask = binary.LittleEndian.Uint32(b[biSize : biSize+4])
			h.gMask = binary.LittleEndian.Uint32(b[biSize+4 : biSize+8])
			h.bMask = binary.LittleEndian.Uint32(b[biSize+8 : biSize+12])
			h.bitsOffset = biSize + 12
		}
		if h.rMask == 0 || h.gMask == 0 || h.bMask == 0 {
			return dibHeader{}, false
		}
	default:
		return dibHeader{}, false // RLE/paletted compressions: best effort no
	}
	return h, true
}

// parseDIB decodes a CF_DIB / CF_DIBV5 clipboard bitmap into an RGBA image.
// Handles 32bpp BI_RGB/BI_BITFIELDS and 24bpp BI_RGB; returns ok=false for
// formats the port cannot decode.
func parseDIB(b []byte) (image.Image, bool) {
	h, ok := parseDIBHeader(b)
	if !ok {
		return nil, false
	}
	height := int(h.height)
	topDown := height < 0
	if topDown {
		height = -height
	}
	rowStride := (int(h.width)*int(h.bitCount) + 31) / 32 * 4 // DWORD-aligned
	total := h.bitsOffset + rowStride*height
	if height <= 0 || rowStride <= 0 || total <= 0 || total > len(b) {
		return nil, false
	}
	// 32bpp BI_RGB payloads frequently carry a uniformly-zero alpha plane
	// (apps that wrote plain BGRX): an all-opaque reading matches what the
	// user actually copied, so detect that case once up front.
	opaquePlane := h.compression == dibCompressionBIRGB && h.bitCount == 32
	if opaquePlane {
		bits := b[h.bitsOffset : h.bitsOffset+rowStride*height]
		for i := 3; i < len(bits); i += 4 {
			if bits[i] != 0 {
				opaquePlane = false
				break
			}
		}
	}
	img := image.NewRGBA(image.Rect(0, 0, int(h.width), height))
	for y := 0; y < height; y++ {
		srcY := y
		if !topDown { // bottom-up DIBs store scan lines from the bottom
			srcY = height - 1 - y
		}
		row := b[h.bitsOffset+srcY*rowStride : h.bitsOffset+(srcY+1)*rowStride]
		for x := 0; x < int(h.width); x++ {
			var r, g, bl, a uint8
			switch h.bitCount {
			case 32:
				px := binary.LittleEndian.Uint32(row[x*4 : x*4+4])
				r = dibExpand(px, h.rMask)
				g = dibExpand(px, h.gMask)
				bl = dibExpand(px, h.bMask)
				a = dibExpand(px, h.aMask) // 0 alpha mask -> opaque
				if opaquePlane {
					a = 0xFF
				}
			case 24:
				o := x * 3
				bl, g, r = row[o], row[o+1], row[o+2]
				a = 0xFF
			}
			img.SetRGBA(x, y, color.RGBA{r, g, bl, a})
		}
	}
	return img, true
}

// dibExpand scales the component selected by mask to 8 bits. An absent mask
// yields opaque: callers use it for the alpha channel, where a zero mask
// means "no alpha channel stored" (BI_BITFIELDS without AlphaMask).
func dibExpand(px, mask uint32) uint8 {
	if mask == 0 {
		return 0xFF
	}
	bits := uint(0)
	for m := mask; m != 0; m >>= 1 {
		if m&1 != 0 {
			bits++
		}
	}
	shift := uint(0)
	for mask&(1<<shift) == 0 {
		shift++
	}
	if bits == 0 {
		return 0
	}
	max := uint32(1)<<bits - 1
	return uint8(((px >> shift) & max) * 0xFF / max)
}

// encodeDIB encodes an image as a 32bpp top-down CF_DIB payload
// (BITMAPINFOHEADER + BGRA scan lines, the format accepted by
// SetClipboardData(CF_DIB, ...)).
func encodeDIB(img image.Image) []byte {
	b := img.Bounds()
	w, hgt := b.Dx(), b.Dy()
	if w <= 0 || hgt <= 0 {
		return nil
	}
	stride := w * 4
	out := make([]byte, dibBIHSize+stride*hgt)
	binary.LittleEndian.PutUint32(out[0:4], dibBIHSize)            // biSize
	binary.LittleEndian.PutUint32(out[4:8], uint32(w))             // biWidth
	binary.LittleEndian.PutUint32(out[8:12], uint32(-hgt))         // biHeight: negative = top-down
	binary.LittleEndian.PutUint16(out[12:14], 1)                   // biPlanes
	binary.LittleEndian.PutUint16(out[14:16], 32)                  // biBitCount
	binary.LittleEndian.PutUint32(out[16:20], dibCompressionBIRGB) // biCompression
	binary.LittleEndian.PutUint32(out[20:24], uint32(stride*hgt))  // biSizeImage
	for y := 0; y < hgt; y++ {
		dst := out[dibBIHSize+y*stride : dibBIHSize+(y+1)*stride]
		for x := 0; x < w; x++ {
			r, g, bl, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			o := x * 4
			dst[o] = byte(bl >> 8)
			dst[o+1] = byte(g >> 8)
			dst[o+2] = byte(r >> 8)
			dst[o+3] = byte(a >> 8)
		}
	}
	return out
}
