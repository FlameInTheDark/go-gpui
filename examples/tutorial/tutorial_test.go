package main

import (
	"bytes"
	"image/png"
	"testing"

	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/platform/headless"
)

// TestTutorialRenders pins the tutorial app's first frame: the window
// opens, the state renders and the frame carries the header, input, rows
// and footer. The presented PNG is decoded for the assertions (the raw
// window image can be mid-swap right after Draw).
func TestTutorialRenders(t *testing.T) {
	hp := headless.New(geometry.SizePixels(420, 520))
	appl := gpui.NewApplication(hp)
	_, err := gpui.OpenView(appl,
		platform.WindowOptions{Title: "tutorial"},
		newTaskList(hp), (*taskList).render)
	if err != nil {
		t.Fatal(err)
	}
	pngBytes, err := hp.LastWindow().PNG()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatal(err)
	}
	bounds := decoded.Bounds()
	bright := func(x, y int) bool {
		r, g, b, a := decoded.At(x, y).RGBA()
		return a > 200*257 && r+g+b > 240*257
	}
	ink := func(y0, y1 int) int {
		n := 0
		for y := y0; y < y1 && y < bounds.Dy(); y++ {
			for x := 0; x < bounds.Dx(); x++ {
				if bright(x, y) {
					n++
				}
			}
		}
		return n
	}
	// Ink bands: header (top), input (below), two task rows, footer help.
	if n := ink(0, 40); n < 30 {
		t.Fatalf("header band too dim (%d px) — title/count did not render", n)
	}
	if n := ink(40, 90); n < 20 {
		t.Fatalf("input band too dim (%d px) — placeholder did not render", n)
	}
	if n := ink(90, 300); n < 100 {
		t.Fatalf("rows band too dim (%d px) — tasks did not render", n)
	}
	if n := ink(bounds.Dy()-40, bounds.Dy()); n < 20 {
		t.Fatalf("footer band too dim (%d px) — help text did not render", n)
	}
}

func TestDeleteDoesNotToggleTheNextTask(t *testing.T) {
	hp := headless.New(geometry.SizePixels(420, 520))
	a := gpui.NewApplication(hp)
	h, err := gpui.OpenView(a, platform.WindowOptions{}, newTaskList(hp), (*taskList).render)
	if err != nil {
		t.Fatal(err)
	}
	for _, hitbox := range h.Window.RenderedHitboxes() {
		if hitbox.Key != "n:btn-task-del-0" {
			continue
		}
		b := hitbox.Bounds
		pos := geometry.PointPixels((b.Origin.X + b.Size.Width/2).AsF32(), (b.Origin.Y + b.Size.Height/2).AsF32())
		w := hp.LastWindow()
		w.DispatchInput(platform.MouseDownEvent{Button: platform.ButtonLeft, Position: pos, ClickCount: 1})
		w.DispatchInput(platform.MouseUpEvent{Button: platform.ButtonLeft, Position: pos, ClickCount: 1})
		tasks := h.View.Read(a.App).tasks
		if len(tasks) != 1 || tasks[0] != (task{title: "Build an app"}) {
			t.Fatalf("unexpected tasks after delete: %+v", tasks)
		}
		return
	}
	t.Fatal("first delete button has no hitbox")
}
