package elements_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/FlameInTheDark/go-gpui/app"
	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/elements"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/platform/headless"
)

const folderClosed = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><path d="M1.5 3.5 H6 L7.5 5.5 H14.5 V13 H1.5 Z" fill="rgb(203,166,247)" stroke="rgb(203,166,247)" stroke-width="1.4" stroke-linejoin="round"/></svg>`

const folderOpen = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><path d="M1.5 3.5 H6 L7.5 5.5 H14.5 V13 H1.5 Z" fill="none" stroke="rgb(203,166,247)" stroke-width="1.4" stroke-linejoin="round"/></svg>`

const fileSheet = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><path d="M4 2 L10 2 L13 5 L13 14 L4 14 Z" fill="none" stroke="rgb(137,220,235)" stroke-width="1.5" stroke-linejoin="round"/></svg>`

const checkPath = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><path d="M3 8.5 L6.5 12 L13 4.5" fill="none" stroke="rgb(137,180,250)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>`

const chevronRight = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><path d="M5 3 L11 8 L5 13 Z" fill="rgb(127,132,156)"/></svg>`

// TestDumpSvgIcons renders the tree icons large (64px) and small (14px)
// into /tmp/icondump/svg-icons.png for visual inspection.
func TestDumpSvgIcons(t *testing.T) {
	if os.Getenv("GO_TEST_ICON_DUMP") == "" {
		t.Skip("diagnostic dump; set GO_TEST_ICON_DUMP=1")
	}
	hp := headless.New(geometry.SizePixels(620, 240))
	appl := gpui.NewApplication(hp)
	handle, err := gpui.OpenWindow[struct{}](appl, platform.WindowOptions{}, func(w *gpui.Window, cx *app.Context[struct{}]) struct{} {
		return struct{}{}
	})
	if err != nil {
		t.Fatal(err)
	}
	handle.Window.SetRootView(handle.View.ID(), func(w *gpui.Window, cx *app.App) gpui.Element {
		srcs := []string{folderClosed, folderOpen, fileSheet, checkPath, chevronRight}
		row := elements.NewDiv().Flex().ItemsCenter().H(pxL(80)).BgColor(gc.RGB(0x181825).Hsla())
		for _, src := range srcs {
			row = row.Child(elements.NewSvgFromData([]byte(src)).W(pxL(64)).H(pxL(64)).
				Mx(pxL(8)))
		}
		small := elements.NewDiv().Flex().ItemsCenter().H(pxL(40)).BgColor(gc.RGB(0x181825).Hsla())
		for _, src := range srcs {
			small = small.Child(elements.NewSvgFromData([]byte(src)).W(pxL(14)).H(pxL(14)).
				Mx(pxL(12)))
		}
		return elements.NewDiv().Flex().FlexCol().Children(row, small)
	})
	handle.Window.Draw()
	hw := hp.LastWindow()
	if hw == nil {
		t.Fatal("no headless window")
	}
	png, err := hw.PNG()
	if err != nil {
		t.Fatal(err)
	}
	dir := "/tmp/icondump"
	os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "svg-icons.png")
	if err := os.WriteFile(path, png, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", path)
}

func pxL(v float32) geometry.Length { return geometry.LengthFromPixels(geometry.Px(v)) }
