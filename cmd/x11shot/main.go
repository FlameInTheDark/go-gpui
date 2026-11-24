// Command x11shot captures the X11 root screen (or a region of it) to a
// PNG file. It is a debugging aid for verifying interactive windows under
// Xvfb:
//
//	DISPLAY=:99 x11shot -out shot.png -x 0 -y 0 -w 1200 -h 900
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/FlameInTheDark/go-gpui/geometry"
	x11 "github.com/FlameInTheDark/go-gpui/platform/x11"
)

func main() {
	out := flag.String("out", "shot.png", "output PNG path")
	x := flag.Int("x", 0, "region origin x")
	y := flag.Int("y", 0, "region origin y")
	w := flag.Int("w", 0, "region width (0 = full screen)")
	h := flag.Int("h", 0, "region height (0 = full screen)")
	flag.Parse()

	c, err := x11.Connect()
	if err != nil {
		fmt.Fprintln(os.Stderr, "x11shot:", err)
		os.Exit(1)
	}
	defer c.Close()
	if *w <= 0 {
		*w = int(c.Screen().WidthPx)
	}
	if *h <= 0 {
		*h = int(c.Screen().HeightPx)
	}

	bounds := geometry.Bounds[int32]{
		Origin: geometry.Point[int32]{X: int32(*x), Y: int32(*y)},
		Size:   geometry.Size[int32]{Width: int32(*w), Height: int32(*h)},
	}
	pngBytes, err := x11.CaptureScreenPNG(bounds)
	if err != nil {
		fmt.Fprintln(os.Stderr, "x11shot:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, pngBytes, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "x11shot:", err)
		os.Exit(1)
	}
	fmt.Printf("x11shot: wrote %s (%dx%d)\n", *out, *w, *h)
}
