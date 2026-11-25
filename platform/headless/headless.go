// Package headless provides a platform backend that renders scenes to
// in-memory images instead of a window (gpui's headless platform; used by
// tests and the PNG example gallery).
package headless

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"sync"
	"time"

	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/platform/internal/software"
	"github.com/FlameInTheDark/go-gpui/render"
	"github.com/FlameInTheDark/go-gpui/text"
)

// WindowID identifies headless windows.
type WindowID uint64

// Platform is the headless platform.
type Platform struct {
	mu        sync.Mutex
	display   geometry.Size[geometry.Pixels]
	scale     float32
	windows   map[WindowID]*Window
	nextID    WindowID
	lastScene *render.Scene
	textSys   *text.System
	clipboard platform.ClipboardItem
	quit      bool
}

// New creates a headless platform with the given default viewport size.
func New(viewport geometry.Size[geometry.Pixels]) *Platform {
	return &Platform{
		display: viewport,
		scale:   1,
		windows: map[WindowID]*Window{},
	}
}

// NewDefault creates an 800x600 headless platform.
func NewDefault() *Platform { return New(geometry.SizePixels(800, 600)) }

// Run invokes onFinish and returns (no event loop).
func (p *Platform) Run(onFinish func()) {
	onFinish()
}

// Quit stops the loop.
func (p *Platform) Quit() { p.quit = true }

// Quitting reports quit state.
func (p *Platform) Quitting() bool { return p.quit }

// OpenWindow creates a headless window.
func (p *Platform) OpenWindow(options platform.WindowOptions, callbacks platform.WindowCallbacks) (platform.PlatformWindow, error) {
	size := p.display
	if options.WindowBackgroundPx != nil {
		size = *options.WindowBackgroundPx
	}
	p.nextID++
	w := &Window{
		id:        p.nextID,
		size:      size,
		scale:     p.scale,
		callbacks: callbacks,
		atlas:     render.NewAtlas(),
		parent:    p,
		title:     options.Title,
	}
	if options.ScaleFactor > 0 {
		w.scale = options.ScaleFactor
	}
	p.mu.Lock()
	p.windows[w.id] = w
	p.mu.Unlock()
	callbacks.OnResize(size, w.scale)
	callbacks.OnActiveStatus(true)
	callbacks.OnVisibility(true)
	return w, nil
}

// Displays lists one virtual display.
func (p *Platform) Displays() []platform.Display {
	return []platform.Display{{ID: 1, Bounds: geometry.BoundsPixels(0, 0, p.display.Width.AsF32(), p.display.Height.AsF32()), ScaleFactor: p.scale}}
}

// PrimaryDisplay returns the virtual display.
func (p *Platform) PrimaryDisplay() (platform.Display, bool) { return p.Displays()[0], true }

// Appearance reports light.
func (p *Platform) Appearance() gc.WindowAppearance { return gc.WindowAppearanceLight }

// Clipboard is in-memory, shared by every headless window (tests copy on
// one surface and paste on another).
func (p *Platform) ReadClipboard() (platform.ClipboardItem, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.clipboard, p.clipboard.Text != "" || p.clipboard.Image != nil
}
func (p *Platform) WriteClipboard(item platform.ClipboardItem) {
	p.mu.Lock()
	p.clipboard = item
	p.mu.Unlock()
}

// DispatchOnMainThread runs f immediately on the calling goroutine. It does
// not move work to a separate UI goroutine; tests must arrange to access app
// and window state from their owning goroutine.
func (p *Platform) DispatchOnMainThread(f func()) { f() }

// Now returns the wall clock.
func (p *Platform) Now() time.Time { return time.Now() }

// TextSystem returns the shared text system (lazily built from system
// fonts).
func (p *Platform) TextSystem() platform.TextSystem {
	if p.textSys == nil {
		p.textSys = text.NewSystemWithSystemFonts()
	}
	return p.textSys
}

// TextSystemConcrete returns the concrete text system.
func (p *Platform) TextSystemConcrete() *text.System {
	_ = p.TextSystem()
	return p.textSys
}

// LastScene returns the most recently presented scene.
func (p *Platform) LastScene() *render.Scene { return p.lastScene }

// Window is a headless platform window: it rasterizes scenes on Draw.
type Window struct {
	id           WindowID
	size         geometry.Size[geometry.Pixels]
	scale        float32
	callbacks    platform.WindowCallbacks
	atlas        *render.Atlas
	parent       *Platform
	title        string
	visible      bool
	active       bool
	closed       bool
	lastImage    *render.RenderImage
	framebuffer  software.Framebuffer
	inputHandler platform.InputHandler
}

// SetTitle sets the title.
func (w *Window) SetTitle(title string) { w.title = title }

// Title returns the title.
func (w *Window) Title() string { return w.title }

// Resize resizes the window.
func (w *Window) Resize(size geometry.Size[geometry.Pixels]) {
	w.size = size
	w.callbacks.OnResize(size, w.scale)
}

// Bounds returns the window bounds.
func (w *Window) Bounds() geometry.Bounds[geometry.Pixels] {
	return geometry.BoundsPixels(0, 0, w.size.Width.AsF32(), w.size.Height.AsF32())
}

// ContentSize returns the client size.
func (w *Window) ContentSize() geometry.Size[geometry.Pixels] { return w.size }

// ScaleFactor returns 1 (or the configured scale).
func (w *Window) ScaleFactor() float32 { return w.scale }

// Appearance reports light.
func (w *Window) Appearance() gc.WindowAppearance { return gc.WindowAppearanceLight }

// BackgroundAppearance reports opaque.
func (w *Window) BackgroundAppearance() platform.WindowBackgroundAppearance {
	return platform.BackgroundOpaque
}

// IsMaximized reports false.
func (w *Window) IsMaximized() bool { return false }

// ToggleFullscreen is a no-op.
func (w *Window) ToggleFullscreen() {}

// IsFullscreen reports false.
func (w *Window) IsFullscreen() bool { return false }

// Minimize is a no-op.
func (w *Window) Minimize() {}

// Activate marks active.
func (w *Window) Activate() {
	if !w.active {
		w.active = true
		w.callbacks.OnActiveStatus(true)
	}
}

// IsActive reports active.
func (w *Window) IsActive() bool { return w.active }

// Close closes the window.
func (w *Window) Close() {
	if !w.closed {
		w.closed = true
		w.active = false
		w.callbacks.OnClose()
	}
}

// IsVisible reports visibility.
func (w *Window) IsVisible() bool { return w.visible }

// Show marks visible.
func (w *Window) Show() {
	if !w.visible {
		w.visible = true
		w.callbacks.OnVisibility(true)
	}
}

// Hide marks hidden.
func (w *Window) Hide() {
	if w.visible {
		w.visible = false
		w.callbacks.OnVisibility(false)
	}
}

// Draw rasterizes the scene.
func (w *Window) Draw(scene *render.Scene) {
	w.parent.mu.Lock()
	w.parent.lastScene = scene
	w.parent.mu.Unlock()
	dev := geometry.SizeToDevicePixels(w.size, w.scale)
	w.lastImage = w.framebuffer.Snapshot(scene, w.atlas, int(dev.Width.AsI32()), int(dev.Height.AsI32()))
}

// Image returns the last rendered frame. Later draws don't overwrite it.
func (w *Window) Image() *render.RenderImage { return w.lastImage }

// PNG encodes the last frame.
func (w *Window) PNG() ([]byte, error) {
	if w.lastImage == nil {
		return nil, nil
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, ImageToRGBA(w.lastImage)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// SpriteAtlas returns the atlas.
func (w *Window) SpriteAtlas() *render.Atlas { return w.atlas }

// ImageToRGBA converts the frame to an stdlib image.
func ImageToRGBA(img *render.RenderImage) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, img.Width, img.Height))
	for y := 0; y < img.Height; y++ {
		for x := 0; x < img.Width; x++ {
			i := (y*img.Width + x) * 4
			out.SetRGBA(x, y, color.RGBA{img.Pixels[i], img.Pixels[i+1], img.Pixels[i+2], img.Pixels[i+3]})
		}
	}
	return out
}

// RequestFrame invokes the frame callback synchronously.
func (w *Window) RequestFrame(opts platform.RequestFrameOptions) {
	w.callbacks.OnRequestFrame(opts)
}

// SetInputHandler stores the handler.
func (w *Window) SetInputHandler(handler platform.InputHandler) { w.inputHandler = handler }

// TakeInputHandler returns the handler.
func (w *Window) TakeInputHandler() platform.InputHandler { return w.inputHandler }

// DispatchInput routes an input event through the app.
func (w *Window) DispatchInput(ev platform.Input) platform.DispatchEventResult {
	return w.callbacks.OnInput(ev)
}

// InvalidateCharacterCoordinates is a no-op.
func (w *Window) InvalidateCharacterCoordinates(bounds geometry.Bounds[geometry.Pixels]) {}

// WriteClipboard stores the item in the platform's shared in-memory
// clipboard.
func (w *Window) WriteClipboard(item platform.ClipboardItem) {
	w.parent.WriteClipboard(item)
}

// ReadClipboard reads the platform's shared in-memory clipboard.
func (w *Window) ReadClipboard() (platform.ClipboardItem, bool) {
	return w.parent.ReadClipboard()
}

// ShowWindowMenu is a no-op.
func (w *Window) ShowWindowMenu(position geometry.Point[geometry.Pixels]) {}

// LastWindow returns the most recently opened window.
func (p *Platform) LastWindow() *Window {
	p.mu.Lock()
	defer p.mu.Unlock()
	var last *Window
	for _, w := range p.windows {
		if last == nil || w.id > last.id {
			last = w
		}
	}
	return last
}

// Windows lists all open windows.
func (p *Platform) Windows() []*Window {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []*Window
	for _, w := range p.windows {
		out = append(out, w)
	}
	return out
}

// PromptForPaths is a no-op on the headless platform (no native dialogs in
// tests): reports cancelled (upstream prompt_for_paths).
func (p *Platform) PromptForPaths(options platform.FileDialogOptions) ([]string, bool) {
	return nil, false
}

// PromptForPath is a no-op on the headless platform (upstream
// prompt_for_new_path).
func (p *Platform) PromptForPath(options platform.FileDialogOptions) (string, bool) {
	return "", false
}

// OpenURL is a no-op on the headless platform.
func (p *Platform) OpenURL(url string) error { return nil }

// SendNotification is a no-op on the headless platform.
func (p *Platform) SendNotification(notification platform.Notification) error { return nil }

// CaptureDisplay is unavailable on the headless platform.
func (p *Platform) CaptureDisplay(displayID uint32) (image.Image, bool) {
	return nil, false
}

// SetCursor is a no-op on the headless platform (no real cursor).
func (p *Platform) SetCursor(style platform.MouseCursorStyle) {}

// SupportsCustomDecorations reports client-side decorations (headless
// windows have no frame at all).
func (w *Window) SupportsCustomDecorations() bool { return false }

// StartWindowDrag is a no-op (no window manager).
func (w *Window) StartWindowDrag() {}

// ToggleMaximize is a no-op.
func (w *Window) ToggleMaximize() {}
