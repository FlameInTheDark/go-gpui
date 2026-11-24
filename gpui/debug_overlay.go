package gpui

import (
	"fmt"
	"time"

	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/style"
	"github.com/FlameInTheDark/go-gpui/text"
)

// Debug overlay (upstream: the upstream editor's dev overlay / debug HUD — not ported
// upstream as a library feature, but the frame statistics this records
// mirror the upstream profiler's counters). Enable with
// SetDebugOverlay(true) or the GPUI_DEBUG=1 environment variable.

// debugFrameStats carries one frame's measurements.
type debugFrameStats struct {
	frame        int
	startedAt    time.Time
	duration     time.Duration
	quads        int
	shadows      int
	paths        int
	underlines   int
	monoSprites  int
	polySprites  int
	subSprites   int
	hitboxes     int
	elements     int
	deferredDraw int
	drawCount    int
}

// frameTimeWindow is the rolling FPS window.
const frameTimeWindow = 500 * time.Millisecond

// SetDebugOverlay enables or disables the debug overlay (frame
// statistics painted above everything).
func (w *Window) SetDebugOverlay(enabled bool) {
	w.debugOverlay = enabled
	w.dirty = true
	if w.drawPhase == PhaseNone {
		w.ScheduleFrame()
	}
}

// DebugOverlayEnabled reports overlay state.
func (w *Window) DebugOverlayEnabled() bool { return w.debugOverlay }

// beginDebugFrame records the frame start.
func (w *Window) beginDebugFrame() {
	w.debugFrameStart = time.Now()
}

// endDebugFrame finishes the measurements and updates the rolling FPS.
func (w *Window) endDebugFrame() {
	if !w.debugOverlay && w.debugFrames == nil {
		return
	}
	stats := debugFrameStats{
		frame:     w.drawCount,
		startedAt: w.debugFrameStart,
		duration:  time.Since(w.debugFrameStart),
	}
	s := w.renderedFrame.scene
	if s != nil {
		stats.quads = len(s.Quads)
		stats.shadows = len(s.Shadows)
		stats.paths = len(s.Paths)
		stats.underlines = len(s.Underlines)
		stats.monoSprites = len(s.MonoSprites)
		stats.polySprites = len(s.PolySprites)
		stats.subSprites = len(s.SubpixelSprites)
	}
	stats.hitboxes = len(w.renderedFrame.hitboxes)
	stats.elements = len(w.elementNode)
	stats.deferredDraw = len(w.deferredDraws)
	stats.drawCount = w.drawCount

	w.debugFrames = append(w.debugFrames, stats)
	// Keep the rolling window only.
	cutoff := time.Now().Add(-frameTimeWindow)
	drop := 0
	for _, f := range w.debugFrames {
		if f.startedAt.Before(cutoff) {
			drop++
		} else {
			break
		}
	}
	if drop > 0 {
		w.debugFrames = w.debugFrames[drop:]
	}
}

// debugSummaryLines computes the overlay's text lines.
func (w *Window) debugSummaryLines() []string {
	n := len(w.debugFrames)
	if n == 0 {
		return nil
	}
	var total time.Duration
	var maxFrame time.Duration
	for _, f := range w.debugFrames {
		total += f.duration
		if f.duration > maxFrame {
			maxFrame = f.duration
		}
	}
	avg := total / time.Duration(n)
	fps := float64(time.Second) / float64(avg+1)
	last := w.debugFrames[n-1]
	return []string{
		fmt.Sprintf("frame %d  %.1f fps (avg %.1fms, max %.1fms)", last.frame, fps, float64(avg)/1e6, float64(maxFrame)/1e6),
		fmt.Sprintf("frame time %.2fms", float64(last.duration)/1e6),
		fmt.Sprintf("quads %d  paths %d  shadows %d  underlines %d", last.quads, last.paths, last.shadows, last.underlines),
		fmt.Sprintf("sprites mono %d  poly %d  subpixel %d", last.monoSprites, last.polySprites, last.subSprites),
		fmt.Sprintf("hitboxes %d  elements %d  deferred %d", last.hitboxes, last.elements, last.deferredDraw),
	}
}

// debugOverlayElement paints the statistics HUD in a corner.
type debugOverlayElement struct {
	window *Window
}

// ElementID implements Element.
func (d *debugOverlayElement) ElementID() ElementId { return NameElementId("debug-overlay") }

// RequestLayout implements Element.
func (d *debugOverlayElement) RequestLayout(w *Window) {
	st := style.DefaultStyle()
	st.Size.Width = geometry.LengthFromDefinite(geometry.RelativeDefinite(1.0))
	st.Size.Height = geometry.LengthFromDefinite(geometry.RelativeDefinite(1.0))
	w.RequestLayout(st, nil)
}

// Prepaint implements Element.
func (d *debugOverlayElement) Prepaint(w *Window, bounds geometry.Bounds[geometry.Pixels]) {
	// The HUD only reports; it never blocks input.
}

// Paint implements Element.
func (d *debugOverlayElement) Paint(w *Window) {
	lines := w.debugSummaryLines()
	if len(lines) == 0 {
		return
	}
	rem := w.RemSize().AsF32()
	if rem <= 0 {
		rem = 16
	}
	fontSize := style.DefaultStyle().Text.FontSize.ToPixels(w.RemSize())
	lineH := fontSize.AsF32() * 1.5
	panelW := rem * 30
	panelH := lineH*float32(len(lines)) + rem*0.8
	content := w.LogicalContentSize()
	// Top-right corner.
	x := content.Width.AsF32() - panelW - rem*0.4
	if x < 0 {
		x = rem * 0.4
	}
	y := rem * 0.4
	panel := geometry.Bounds[geometry.Pixels]{
		Origin: geometry.PointPixels(x, y),
		Size:   geometry.SizePixels(panelW, panelH),
	}
	st := style.DefaultStyle()
	bg := gc.Hsla{H: 0, S: 0, L: 0, A: 0.7}
	b := gc.SolidBackground(bg)
	st.Background = &b
	border := gc.Hsla{H: 0, S: 0, L: 1, A: 0.2}
	st.BorderColor = &border
	st.BorderWidths = geometry.EdgesAbsolute{
		Top: geometry.PixelsLength(geometry.Px(1)), Right: geometry.PixelsLength(geometry.Px(1)),
		Bottom: geometry.PixelsLength(geometry.Px(1)), Left: geometry.PixelsLength(geometry.Px(1)),
	}
	w.PaintQuad(st, panel)

	fg := gc.Hsla{H: 120, S: 0.8, L: 0.7, A: 1}
	pad := rem * 0.4
	for i, line := range lines {
		l := debugShapeLine(w, line, fontSize)
		paintDebugLine(w, l, geometry.PointPixels(x+pad, y+pad+fontSize.AsF32()*0.85+lineH*float32(i)), fg)
	}
}

// debugShapeLine shapes a HUD line with the window base font.
func debugShapeLine(w *Window, str string, size geometry.Pixels) *text.LineLayout {
	id, _ := w.TextSystem().FontID(w.TextStyle().ToFont())
	return w.TextSystem().LayoutLine(str, size, []text.FontRun{{Len: len(str), FontID: id}})
}

// paintDebugLine paints one HUD line.
func paintDebugLine(w *Window, l *text.LineLayout, origin geometry.Point[geometry.Pixels], color gc.Hsla) {
	for _, run := range l.Runs {
		for _, g := range run.Glyphs {
			w.PaintGlyph(
				geometry.PointPixels(origin.X.AsF32()+g.Position.X.AsF32(), origin.Y.AsF32()+g.Position.Y.AsF32()),
				run.FontID, g.ID, l.FontSize, color)
		}
	}
}

// paintDebugOverlay paints the HUD as the topmost layer (above prompts).
func (w *Window) paintDebugOverlay() {
	if !w.debugOverlay {
		return
	}
	el := &debugOverlayElement{window: w}
	close := w.PaintLayer()
	w.WithContentMask(geometry.Bounds[geometry.Pixels]{
		Origin: geometry.Point[geometry.Pixels]{},
		Size:   w.LogicalContentSize(),
	}, func() {
		w.PaintElement(el)
	})
	close()
}

// DebugSummaryLines exposes the HUD's summary lines (test/read-only
// access to the frame statistics).
func (w *Window) DebugSummaryLines() []string { return w.debugSummaryLines() }
