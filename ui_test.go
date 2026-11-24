package gpui

import (
	"testing"

	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/platform/headless"
)

func TestRunRendersAndHandlesInput(t *testing.T) {
	p := headless.New(geometry.SizePixels(40, 40))
	count, rendered := 0, -1
	err := run(p, WindowOptions{Title: "Counter"}, func(*Window) Element {
		rendered = count
		return Column(Div().Size(Px(20), Px(20)).OnClick(func(_ ClickEvent, w *Window) {
			count++
			w.Refresh()
		}))
	})
	if err != nil {
		t.Fatal(err)
	}
	w := p.LastWindow()
	if w == nil || w.Title() != "Counter" || rendered != 0 {
		t.Fatalf("expected the initial counter window and frame, got window %v, count %d", w, rendered)
	}
	pos := geometry.PointPixels(10, 10)
	w.DispatchInput(platform.MouseDownEvent{Button: platform.ButtonLeft, Position: pos, ClickCount: 1})
	w.DispatchInput(platform.MouseUpEvent{Button: platform.ButtonLeft, Position: pos, ClickCount: 1})
	if rendered != 1 {
		t.Fatalf("rendered count after click = %d, want 1", rendered)
	}
	w.Close()
	if !p.Quitting() {
		t.Fatal("closing the last window should quit the application")
	}
}

func TestRunRejectsMissingRenderer(t *testing.T) {
	if err := Run(WindowOptions{}, nil); err == nil {
		t.Fatal("Run accepted a nil render function")
	}
}
