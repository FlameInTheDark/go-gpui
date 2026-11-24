//go:build windows

package windows

import (
	"bytes"
	"testing"

	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/render"
)

func TestGPURecoveryTimerIsOneShot(t *testing.T) {
	w := registerTestWindow(t, false)
	w.gpuTried = true
	w.gpuLost = true
	w.gpuRetryPending = true
	wndProc(w.hwnd, WM_TIMER, gpuRetryTimer, 0)
	if w.gpuRetryPending || w.gpuTried || !w.gpuLost {
		t.Fatal("timer did not enable exactly one deferred creation attempt")
	}
	w.gpuTried = true // the recovery attempt has already run
	wndProc(w.hwnd, WM_TIMER, gpuRetryTimer, 0)
	if !w.gpuTried {
		t.Fatal("stale timer allowed another recovery attempt")
	}
}

func TestSoftwarePaintCachesBySubmissionNotScenePointer(t *testing.T) {
	w := registerTestWindow(t, false)
	w.atlas = render.NewAtlas()
	scene := &render.Scene{}
	scene.InsertQuad(render.Quad{
		Bounds: geometry.ScaleBounds(geometry.BoundsPixels(0, 0, 8, 8), 1), ContentMask: render.FullContentMask(),
		Background: gc.SolidBackground(gc.Red()),
	})
	scene.Finish()
	w.Draw(scene)
	w.presentSoftware(0, 8, 8)
	first := bytes.Clone(w.lastImage.Pixels)
	last := w.lastPresent
	w.presentSoftware(0, 8, 8)
	if w.lastPresent != last {
		t.Fatal("repaint unnecessarily rasterized an unchanged frame")
	}
	scene.Quads[0].Background = gc.SolidBackground(gc.RGB(0x00ff00).Hsla())
	w.Draw(scene)
	w.presentSoftware(0, 8, 8)
	if bytes.Equal(w.lastImage.Pixels, first) {
		t.Fatal("new submission reused stale pixels from the same scene pointer")
	}
}
