package gpui

// Window zoom (upstream Window::zoom / set_zoom + the upstream editor's zoom actions):
// the zoom multiplies the platform scale factor, so every rem, pixel
// scale and rasterization follows it (upstream keeps zoom in
// scale_factor()).

// ZoomStep is the multiplicative step of ZoomIn/ZoomOut (the upstream editor's 1.1 zoom
// factor).
const ZoomStep float32 = 1.1

// MinZoom / MaxZoom bound the zoom factor (the upstream editor's limits).
const (
	MinZoom float32 = 0.5
	MaxZoom float32 = 3.0
)

// Zoom returns the current zoom factor (1.0 = default).
func (w *Window) Zoom() float32 { return w.zoom }

// SetZoom sets the zoom factor (clamped to [MinZoom, MaxZoom]) and
// redraws. The whole UI scales: layout rem sizes, text, quads and the
// rasterization scale.
func (w *Window) SetZoom(z float32) {
	if z < MinZoom {
		z = MinZoom
	}
	if z > MaxZoom {
		z = MaxZoom
	}
	if z == w.zoom {
		return
	}
	w.zoom = z
	w.dirty = true
	if w.drawPhase == PhaseNone {
		w.ScheduleFrame()
	}
}

// ZoomIn zooms one step in (ctrl+= in the upstream editor).
func (w *Window) ZoomIn() { w.SetZoom(w.zoom * ZoomStep) }

// ZoomOut zooms one step out (ctrl+- in the upstream editor).
func (w *Window) ZoomOut() { w.SetZoom(w.zoom / ZoomStep) }

// ZoomReset restores the default zoom (ctrl+0 in the upstream editor).
func (w *Window) ZoomReset() { w.SetZoom(1) }
