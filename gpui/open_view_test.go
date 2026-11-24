package gpui_test

import (
	"testing"

	"github.com/FlameInTheDark/go-gpui/app"
	"github.com/FlameInTheDark/go-gpui/elements"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/platform/headless"
)

func TestOpenViewRendersAndUpdaterNotifies(t *testing.T) {
	p := headless.New(geometry.SizePixels(40, 40))
	a := gpui.NewApplication(p)
	builds, frames, rendered := 0, 0, 0
	var update func(func(*int))
	h, err := gpui.OpenView(a, platform.WindowOptions{}, func(w *gpui.Window, cx *app.Context[int]) int {
		builds++
		return 3
	}, func(state *int, w *gpui.Window, cx *app.Context[int]) gpui.Element {
		frames++
		rendered = *state
		update = cx.Updater()
		return elements.NewDiv().Size_Full()
	})
	if err != nil {
		t.Fatal(err)
	}
	if builds != 1 || frames != 1 || rendered != 3 || p.LastScene() == nil {
		t.Fatalf("initial window: builds=%d frames=%d rendered=%d", builds, frames, rendered)
	}

	// No input event or manual Draw to mask a missing notification.
	update(func(state *int) { *state += 2 })
	if builds != 1 || frames != 2 || rendered != 5 || *h.View.Read(a.App) != 5 {
		t.Fatalf("after update: builds=%d frames=%d rendered=%d", builds, frames, rendered)
	}
}
