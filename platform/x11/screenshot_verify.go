//go:build verifyshot

// Temporary tool: connect to $DISPLAY, capture the root window via GetImage
// (opcode 73) and report color statistics. Run with:
//
//	go run -tags verifyshot ./cmd/x11shot
package x11

import (
	"fmt"
	"os"
)

// RunScreenshotCheck is only built with the "verifyshot" tag.
func RunScreenshotCheck() {
	c, err := Connect()
	if err != nil {
		fmt.Println("connect error:", err)
		os.Exit(1)
	}
	defer c.Close()

	root := c.screen.RootWindow
	w := int(c.screen.WidthPx)
	h := int(c.screen.HeightPx)
	if w > 480 {
		w = 480
	}
	if h > 360 {
		h = 360
	}

	// GetImage: format 2 (ZPixmap), drawable, x, y, width, height,
	// plane-mask 0xFFFFFFFF.
	req := newRequest(73, 2).
		u32(root).
		u16(0).u16(0).
		u16(uint16(w)).u16(uint16(h)).
		u32(0xFFFFFFFF)
	seq := c.nextSeq()
	if err := c.writeRequest(req.finalize()); err != nil {
		fmt.Println("write error:", err)
		os.Exit(1)
	}
	reply, err := c.readReply(seq)
	if err != nil {
		fmt.Println("getimage error:", err)
		os.Exit(1)
	}
	depth := reply[1]
	data := reply[32:] // 8 header bytes already consumed by protocol? see below
	// X11 reply: 32-byte header, then depth byte is at offset 1 of the header;
	// pixel data starts at byte 32 of the reply.
	_ = depth

	colors := map[uint32]int{}
	total := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := (y*w + x) * 4
			if o+3 >= len(data) {
				continue
			}
			// ZPixmap little-endian on x86: BGRX byte order.
			b, g, r, _ := data[o], data[o+1], data[o+2], data[o+3]
			rgb := uint32(r)<<16 | uint32(g)<<8 | uint32(b)
			colors[rgb]++
			total++
		}
	}
	fmt.Printf("captured %dx%d depth=%d total=%d unique=%d\n", w, h, depth, total, len(colors))
	if total == 0 {
		os.Exit(1)
	}
	// Report the top colors to confirm the demo actually painted.
	type cc struct {
		rgb uint32
		n   int
	}
	var top []cc
	for rgb, n := range colors {
		top = append(top, cc{rgb, n})
	}
	// simple insertion sort desc
	for i := 1; i < len(top); i++ {
		for j := i; j > 0 && top[j].n > top[j-1].n; j-- {
			top[j], top[j-1] = top[j-1], top[j]
		}
	}
	limit := 8
	if len(top) < limit {
		limit = len(top)
	}
	for i := 0; i < limit; i++ {
		fmt.Printf("  color #%06x : %d px\n", top[i].rgb, top[i].n)
	}

}
