//go:build windows

package windows

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/keymap"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/render"
)

// Software-fallback frame pacing: rasterizing the whole scene on the CPU
// costs tens of milliseconds per frame, while the app keeps requesting
// frames at its usual rate (fps counter, cursor blink). Capping the
// invalidate rate keeps the UI thread responsive on the fallback path
// instead of running paints back to back.
const (
	softwareFrameInterval = 33 * time.Millisecond // ~30 fps cap
	softwareRepaintTimer  = 5                     // one-shot WM_TIMER id
	gpuRetryTimer         = 6
	gpuRetryDelayMS       = 1000
)

// Window is a win32 platform window. Layout, hit-testing and mouse input
// operate in logical pixels (platform convention); the client area, the
// renderer and presented frames are physical pixels; scale converts between
// them (per-monitor DPI aware).
//
// Frame driving follows upstream gpui's model (winit: draws happen in the
// window's paint handler): Window.Draw only records the latest scene and
// invalidates the window; every Direct3D 11 call - device creation, atlas
// uploads, presenting - runs inside the message pump's WM_PAINT handler.
// Presenting from any other context (OpenWindow before the loop runs, or a
// window-proc input handler) can deadlock with DWM, because DXGI's
// flip-model Present must synchronize with the compositor while the
// compositor sends messages to the window's thread.
type Window struct {
	p         *Platform
	hwnd      syscall.Handle
	size      geometry.Size[geometry.Pixels] // logical client size
	scale     float32
	callbacks platform.WindowCallbacks
	atlas     *render.Atlas
	title     string
	visible   bool
	active    bool
	closed    bool
	// bgAppearance is the window's compositing mode (opaque windows use
	// the hwnd swap chain; transparent/blurred ones use composition).
	bgAppearance platform.WindowBackgroundAppearance

	// gpu presents via Direct3D 11 (nil until the first WM_PAINT, or on
	// init failure -> GDI fallback).
	gpu *d3d11Renderer
	// gpuTried: device creation attempted (do not retry).
	// gpuLost: a live device was lost once; at most one recreation
	// attempt follows (driver resets are often transient, but a device
	// that dies again stays dead - permanent GDI from then on).
	gpuTried        bool
	gpuLost         bool
	gpuRetryPending bool

	// Software-fallback frame pacing: rasterizing the whole scene on the
	// CPU costs tens of milliseconds per frame, while the app keeps
	// requesting frames at its usual rate (fps counter, cursor blink).
	// Capping the invalidate rate keeps the UI thread responsive on the
	// fallback path instead of running paints back to back.
	lastPresent        time.Time
	softwareTimerArmed bool

	// lastScene is the most recent scene recorded by Draw; the WM_PAINT
	// handler renders it with the active renderer.
	lastScene                     *render.Scene
	sceneVersion, softwareVersion uint64
	// painting guards against re-entrant WM_PAINT: DXGI's flip-model
	// Present can dispatch pending window messages, including paints.
	painting bool
	// needsShow defers ShowWindow until the event loop runs (the first
	// frame presents right after the window appears: no white flash).
	needsShow bool

	// lastImage is the most recent software-rasterized frame (GDI fallback).
	lastImage *render.RenderImage
	// raster is the persistent software rasterizer (reused across frames;
	// reallocating a full framebuffer per frame thrashes the GC on 4K
	// displays).
	raster *render.Rasterizer

	// bgra caches the color-swapped presentation buffer (reallocated on
	// size changes only).
	bgra []byte

	// fullscreen state: the borderless toggle saves the window placement
	// on entry and restores it on exit (upstream Windows fullscreen).
	fullscreen     bool
	savedPlacement WINDOWPLACEMENT

	// customDecorations draws the app's own title bar (the OS frame is
	// removed via WM_NCCALCSIZE; the app's title bar drives dragging
	// through StartWindowDrag).
	customDecorations bool

	// click tracking for double-click detection
	lastClickTime int32
	lastClickX    int32
	lastClickY    int32
	clickCount    int

	input platform.InputHandler

	// IME state (IMM32): defaultIMC is the context captured at window
	// creation so focus changes can re-associate it; imeResultDelivered
	// dedupes WM_IME_ENDCOMPOSITION after a GCS_RESULTSTR commit.
	defaultIMC         uintptr
	imeResultDelivered bool
}

// windowDPI returns the window's DPI (96 fallback), using GetDpiForWindow
// when available and GetDeviceCaps otherwise.
func (w *Window) windowDPI() float32 {
	if err := procGetDpiForWindow.Find(); err == nil {
		dpi, _, _ := procGetDpiForWindow.Call(uintptr(w.hwnd))
		if dpi != 0 {
			return float32(dpi)
		}
	}
	if hdc, _, _ := procGetDC.Call(uintptr(w.hwnd)); hdc != 0 {
		defer procReleaseDC.Call(uintptr(w.hwnd), hdc)
		// LOGPIXELSX = 88
		if dpi, _, _ := procGetDeviceCaps.Call(hdc, 88); dpi != 0 {
			return float32(int32(dpi))
		}
	}
	return USER_DEFAULT_SCREEN_DPI
}

// queryDPI returns the DPI scale for the window.
func (w *Window) queryDPI() float32 {
	return w.windowDPI() / USER_DEFAULT_SCREEN_DPI
}

// clientSizePhysical returns the client area in physical pixels.
func (w *Window) clientSizePhysical() (int, int) {
	var rc RECT
	procGetClientRect.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&rc)))
	return int(rc.Right - rc.Left), int(rc.Bottom - rc.Top)
}

func (w *Window) SetTitle(title string) {
	w.title = title
	procSetWindowTextW.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(utf16Ptr(title))))
}

func (w *Window) Title() string {
	n, _, _ := procGetWindowTextW.Call(uintptr(w.hwnd), 0, 0)
	if n == 0 {
		return w.title
	}
	buf := make([]uint16, n+1)
	procGetWindowTextW.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
}

// Resize sets the client size in logical pixels.
func (w *Window) Resize(size geometry.Size[geometry.Pixels]) {
	w.size = size
	cw := int(math.Round(float64(size.Width.AsF32() * w.scale)))
	ch := int(math.Round(float64(size.Height.AsF32() * w.scale)))
	w.applyClientSize(cw, ch)
}

// applyClientSize resizes the window so the client area is cw x ch physical
// pixels, using DPI-aware frame metrics when available. Frameless windows
// (custom decorations) have no non-client area: the window rect IS the
// client rect, so no AdjustWindowRect padding is applied — padding would
// grow the window past the requested size on every programmatic resize.
func (w *Window) applyClientSize(cw, ch int) {
	rc := RECT{0, 0, int32(cw), int32(ch)}
	if !w.customDecorations {
		dpi := uint32(0)
		if err := procGetDpiForWindow.Find(); err == nil {
			if d, _, _ := procGetDpiForWindow.Call(uintptr(w.hwnd)); d != 0 {
				dpi = uint32(d)
			}
		}
		if dpi != 0 && procAdjustWindowRectForDpi.Find() == nil {
			procAdjustWindowRectForDpi.Call(uintptr(unsafe.Pointer(&rc)), WS_OVERLAPPEDWINDOW, 0, uintptr(dpi))
		} else {
			procAdjustWindowRect.Call(uintptr(unsafe.Pointer(&rc)), WS_OVERLAPPEDWINDOW, 0)
		}
	}
	procSetWindowPos.Call(uintptr(w.hwnd), 0, 0, 0,
		uintptr(rc.Right-rc.Left), uintptr(rc.Bottom-rc.Top),
		SWP_NOMOVE|SWP_NOZORDER|SWP_NOACTIVATE)
}

func (w *Window) Bounds() geometry.Bounds[geometry.Pixels] {
	cw, ch := w.clientSizePhysical()
	return geometry.BoundsPixels(0, 0, float32(cw)/w.scale, float32(ch)/w.scale)
}

// ContentSize returns the client size in logical pixels (platform
// convention): physical pixels divided by the scale factor.
func (w *Window) ContentSize() geometry.Size[geometry.Pixels] {
	cw, ch := w.clientSizePhysical()
	if cw > 0 || ch > 0 {
		return geometry.SizePixels(float32(cw)/w.scale, float32(ch)/w.scale)
	}
	return w.size
}

func (w *Window) ScaleFactor() float32 { return w.scale }

func (w *Window) Appearance() color.WindowAppearance { return w.p.Appearance() }

func (w *Window) BackgroundAppearance() platform.WindowBackgroundAppearance {
	return w.bgAppearance
}

func (w *Window) IsMaximized() bool {
	zoomed, _, _ := procIsZoomed.Call(uintptr(w.hwnd))
	return zoomed != 0
}

// ToggleFullscreen switches between bordered and borderless-fullscreen on
// the window's current monitor (upstream Windows fullscreen: the placement
// is saved on entry and restored on exit; the style flips between
// WS_OVERLAPPEDWINDOW and WS_POPUP).
func (w *Window) ToggleFullscreen() {
	// GWL_STYLE is negative: through a variable the uintptr conversion
	// is a runtime operation (a direct constant conversion overflows).
	gwlStyle := int64(GWL_STYLE)
	if w.fullscreen {
		// Restore the saved placement and style.
		wp := w.savedPlacement
		procSetWindowPlacement.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&wp)))
		style, _, _ := procGetWindowLongPtrW.Call(uintptr(w.hwnd), uintptr(gwlStyle))
		procSetWindowLongPtrW.Call(uintptr(w.hwnd), uintptr(gwlStyle),
			(style&^WS_POPUP)|WS_OVERLAPPEDWINDOW)
		// Re-apply the placement once more: SetWindowLong can reset the
		// frame, and placement carries the pre-fullscreen show state.
		procSetWindowPlacement.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&wp)))
		w.fullscreen = false
	} else {
		// Save the current placement, then cover the nearest monitor.
		wp := WINDOWPLACEMENT{Length: WINDOWPLACEMENT_LENGTH}
		procGetWindowPlacement.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&wp)))
		w.savedPlacement = wp
		mon, _, _ := procMonitorFromWindow.Call(uintptr(w.hwnd), MONITOR_DEFAULTTONEAREST)
		var mi MONITORINFO
		mi.CbSize = uint32(unsafe.Sizeof(mi))
		if mon != 0 {
			procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi)))
		} else {
			cx, _, _ := procGetSystemMetrics.Call(0) // SM_CXSCREEN
			cy, _, _ := procGetSystemMetrics.Call(1) // SM_CYSCREEN
			mi.RcMonitor = RECT{0, 0, int32(cx), int32(cy)}
		}
		style, _, _ := procGetWindowLongPtrW.Call(uintptr(w.hwnd), uintptr(gwlStyle))
		procSetWindowLongPtrW.Call(uintptr(w.hwnd), uintptr(gwlStyle),
			(style&^WS_OVERLAPPEDWINDOW)|WS_POPUP|WS_VISIBLE)
		procSetWindowPos.Call(uintptr(w.hwnd), 0,
			uintptr(mi.RcMonitor.Left), uintptr(mi.RcMonitor.Top),
			uintptr(mi.RcMonitor.Right-mi.RcMonitor.Left),
			uintptr(mi.RcMonitor.Bottom-mi.RcMonitor.Top),
			SWP_NOZORDER|SWP_FRAMECHANGED)
		w.fullscreen = true
	}
	procInvalidateRect.Call(uintptr(w.hwnd), 0, 0)
}

func (w *Window) IsFullscreen() bool { return w.fullscreen }

func (w *Window) Minimize() { procShowWindow.Call(uintptr(w.hwnd), SW_MINIMIZE) }

// SupportsCustomDecorations reports the client-side-decorations mode this
// window was created with.
func (w *Window) SupportsCustomDecorations() bool { return w.customDecorations }

// StartWindowDrag begins a native move gesture from the app's custom
// title bar: releasing the capture and sending WM_NCLBUTTONDOWN with
// HTCAPTION makes DefWindowProc run the modal drag loop (the window
// follows the cursor, snapping to screen edges like a native window).
func (w *Window) StartWindowDrag() {
	procReleaseCapture.Call()
	procSendMessageW.Call(uintptr(w.hwnd), WM_NCLBUTTONDOWN, HTCAPTION, 0)
}

// ToggleMaximize switches the maximized/restored state (the custom title
// bar's maximize button and double-click on the bar).
func (w *Window) ToggleMaximize() {
	if w.IsMaximized() {
		procShowWindow.Call(uintptr(w.hwnd), SW_RESTORE)
	} else {
		procShowWindow.Call(uintptr(w.hwnd), SW_MAXIMIZE)
	}
}

func (w *Window) Activate() { procSetForegroundWin.Call(uintptr(w.hwnd)) }

func (w *Window) IsActive() bool { return w.active }

func (w *Window) IsVisible() bool {
	if w.closed {
		return false
	}
	ret, _, _ := procIsWindowVisible.Call(uintptr(w.hwnd))
	return ret != 0
}

func (w *Window) Show() {
	w.needsShow = false
	procShowWindow.Call(uintptr(w.hwnd), SW_SHOWNORMAL)
	procInvalidateRect.Call(uintptr(w.hwnd), 0, 0)
}

func (w *Window) Hide() { procShowWindow.Call(uintptr(w.hwnd), SW_HIDE) }

func (w *Window) Close() {
	if !w.closed {
		procDestroyWindow.Call(uintptr(w.hwnd))
	}
}

// Draw records the scene for the next WM_PAINT and invalidates the client
// area. It performs no presentation: with the GPU renderer this is just a
// pointer store, so input floods coalesce into one present per paint.
// On the software fallback (no D3D11 or the device was lost) the invalidate
// rate is capped at softwareFrameInterval: a Draw arriving sooner arms a
// one-shot WM_TIMER that invalidates when the interval has elapsed, so the
// CPU rasterizer cannot consume the whole UI thread.
func (w *Window) Draw(scene *render.Scene) {
	if w.closed {
		return
	}
	w.lastScene = scene
	w.sceneVersion++
	if scene == nil {
		return
	}
	if w.gpu == nil && w.gpuTried {
		if since := time.Since(w.lastPresent); since < softwareFrameInterval {
			w.armSoftwareRepaint(softwareFrameInterval - since)
			return
		}
	}
	procInvalidateRect.Call(uintptr(w.hwnd), 0, 0)
}

// armSoftwareRepaint schedules a deferred invalidation after d (one-shot
// WM_TIMER on this window). The earliest pending deadline wins.
func (w *Window) armSoftwareRepaint(d time.Duration) {
	if w.softwareTimerArmed {
		return
	}
	ms := int(d / time.Millisecond)
	if ms < 1 {
		ms = 1
	}
	if ms > 100 {
		ms = 100
	}
	id, _, _ := procSetTimer.Call(uintptr(w.hwnd), softwareRepaintTimer, uintptr(ms), 0)
	w.softwareTimerArmed = id != 0
	if id == 0 {
		procInvalidateRect.Call(uintptr(w.hwnd), 0, 0)
	}
}

// ensureGPU lazily creates the D3D11 renderer. Called from the WM_PAINT
// handler only, so the device and its swap chain are created while the
// message loop is pumping (see the Window doc).
func (w *Window) ensureGPU(cw, ch int) {
	if w.gpu != nil || w.gpuTried || w.closed {
		return
	}
	w.gpuTried = true
	gpu, err := newD3D11Renderer(w.hwnd, cw, ch, w.bgAppearance != platform.BackgroundOpaque)
	if err != nil {
		fmt.Fprintln(os.Stderr, "go-gpui: Direct3D 11 unavailable ("+err.Error()+"); using software rendering")
		logRenderer("software (GDI)")
		return
	}
	if w.closed {
		gpu.Close()
		return
	}
	w.gpu = gpu
	logRenderer("Direct3D 11")
}

// paintFrame renders the recorded scene. Runs ONLY inside the message
// pump's WM_PAINT handler (see the Window doc).
func (w *Window) paintFrame(hdc syscall.Handle) {
	if w.closed || w.painting {
		return // re-entrant paint during Present: already being handled
	}
	cw, ch := w.clientSizePhysical()
	if cw <= 0 || ch <= 0 {
		return
	}
	w.painting = true
	start := time.Now()
	defer func() {
		w.painting = false
		if w.closed && w.gpu != nil {
			w.gpu.Close()
			w.gpu = nil
		}
		// A single frame taking seconds means the active renderer is
		// too slow for this display (or the driver hung): leave dated
		// evidence in the log next to the device-lost messages.
		if d := time.Since(start); d > time.Second {
			fmt.Fprintf(os.Stderr, "go-gpui: frame presented in %dms; the active renderer is too slow (or hung) for this display\n", d.Milliseconds())
		}
	}()
	w.ensureGPU(cw, ch)
	if w.closed {
		return
	}
	if w.gpu != nil {
		transparent := w.bgAppearance != platform.BackgroundOpaque
		rerr := w.gpu.Render(w.lastScene, w.sceneVersion, w.atlas, cw, ch, transparent)
		if rerr == nil || w.closed {
			return
		}
		// Show a software frame immediately. Wait before the single GPU
		// retry so we don't re-enter the driver while it is still resetting.
		fmt.Fprintln(os.Stderr, "go-gpui: Direct3D 11 render failed ("+rerr.Error()+")")
		w.gpu.Close()
		w.gpu = nil
		if !w.gpuLost {
			w.gpuLost = true
			if id, _, _ := procSetTimer.Call(uintptr(w.hwnd), gpuRetryTimer, gpuRetryDelayMS, 0); id != 0 {
				w.gpuRetryPending = true
				fmt.Fprintln(os.Stderr, "go-gpui: using software rendering during Direct3D 11 recovery; retrying in one second")
			} else {
				fmt.Fprintln(os.Stderr, "go-gpui: unable to schedule Direct3D 11 recovery; using software rendering")
			}
		} else {
			fmt.Fprintln(os.Stderr, "go-gpui: Direct3D 11 recovery failed; using software rendering for this window")
		}
	}
	w.presentSoftware(hdc, cw, ch)
}

// presentSoftware rasterizes the recorded scene on the CPU and blits it
// (GDI fallback path). The output image and the BGRA staging buffer are
// reused across frames: allocating a fresh frame-sized buffer per present
// churned megabytes through the GC and showed up as RAM growth.
func (w *Window) presentSoftware(hdc syscall.Handle, cw, ch int) {
	if w.lastScene == nil {
		return
	}
	// Exposure paints and timer ticks need not repeat CPU rasterization if
	// there is no new scene and the frame already matches the client size.
	if w.softwareVersion == w.sceneVersion && w.lastImage != nil && w.lastImage.Width == cw && w.lastImage.Height == ch {
		w.presentHDC(hdc)
		return
	}
	if w.raster == nil {
		w.raster = render.NewRasterizer(cw, ch, w.atlas)
	} else if rw, rh := w.raster.Size(); rw != cw || rh != ch {
		w.raster = render.NewRasterizer(cw, ch, w.atlas)
	}
	w.raster.Clear()
	w.raster.PaintScene(w.lastScene)
	if w.lastImage == nil || w.lastImage.Width != cw || w.lastImage.Height != ch {
		w.lastImage = &render.RenderImage{Width: cw, Height: ch, Pixels: make([]uint8, cw*ch*4)}
	}
	w.raster.ImageInto(w.lastImage)
	w.softwareVersion = w.sceneVersion
	// Atlas dirty regions only feed the GPU mirror; drain them so the list
	// cannot grow unboundedly on the software path.
	w.atlas.TakeDirtyRegions()
	w.prepareDIB()
	w.presentHDC(hdc)
	w.lastPresent = time.Now()
}

func (w *Window) prepareDIB() {
	img := w.lastImage
	// render.RenderImage is RGBA; a DIB wants BGRA. The swapped buffer is
	// cached across frames and only reallocated on size changes. Words
	// are swapped 4 bytes at a time (a byte loop over a 1100x700 frame
	// cost several milliseconds per present).
	if len(w.bgra) < len(img.Pixels) {
		w.bgra = make([]byte, len(img.Pixels))
	}
	src := img.Pixels[:len(img.Pixels)&^3]
	dst := w.bgra[:len(src)]
	for i := 0; i+4 <= len(src); i += 4 {
		v := binary.LittleEndian.Uint32(src[i:])
		// RGBA -> BGRA: swap the R and B bytes, keep G and A.
		binary.LittleEndian.PutUint32(dst[i:], v<<16|v&0xFF00FF00|(v>>16)&0x000000FF)
	}
}

// presentHDC blits the cached frame as a top-down 32bpp DIB.
func (w *Window) presentHDC(hdc syscall.Handle) {
	img := w.lastImage
	if img == nil || w.closed || len(w.bgra) == 0 {
		return
	}
	bi := BITMAPINFO{}
	bi.Header.BiSize = uint32(unsafe.Sizeof(bi.Header))
	bi.Header.BiWidth = int32(img.Width)
	bi.Header.BiHeight = -int32(img.Height) // top-down
	bi.Header.BiPlanes = 1
	bi.Header.BiBitCount = 32
	bi.Header.BiCompression = BI_RGB

	cw, ch := w.clientSizePhysical()
	if cw <= 0 || ch <= 0 {
		return
	}

	if img.Width == cw && img.Height == ch {
		// Fast path: 1:1 copy, no scaling, no blur.
		//
		// SetDIBitsToDevice(hdc, xDest, yDest, w, h, xSrc, ySrc, StartScan,
		// cLines, bits, info, usage) - 12 arguments; the top-down DIB starts
		// at scan line 0.
		procSetDIBitsToDevice.Call(
			uintptr(hdc),
			0, 0, uintptr(cw), uintptr(ch), // destination
			0, 0, // source origin
			0,                   // starting scan line
			uintptr(img.Height), // scan line count
			uintptr(unsafe.Pointer(&w.bgra[0])),
			uintptr(unsafe.Pointer(&bi)),
			DIB_RGB_COLORS,
		)
		return
	}
	procStretchDIBits.Call(
		uintptr(hdc),
		0, 0, uintptr(cw), uintptr(ch), // destination rect (client area)
		0, 0, uintptr(img.Width), uintptr(img.Height), // source rect (device px)
		uintptr(unsafe.Pointer(&w.bgra[0])),
		uintptr(unsafe.Pointer(&bi)),
		DIB_RGB_COLORS, SRCCOPY,
	)
}

func (w *Window) SpriteAtlas() *render.Atlas { return w.atlas }

// logRenderer prints the active presentation path when GPUI_DEBUG is set
// (any value except "0" and "false").
func logRenderer(what string) {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("GPUI_DEBUG")))
	if v == "" || v == "0" || v == "false" {
		return
	}
	fmt.Fprintln(os.Stderr, "go-gpui: renderer: "+what)
}

func (w *Window) RequestFrame(opts platform.RequestFrameOptions) {
	w.callbacks.OnRequestFrame(opts)
}

// SetInputHandler stores the text input handler and follows the IME focus
// flow: associating the IME context when an editor takes input on the
// active window, dissociating it when the handler is cleared.
func (w *Window) SetInputHandler(handler platform.InputHandler) {
	w.input = handler
	w.setIMEEnabled(handler != nil && w.active)
}

func (w *Window) TakeInputHandler() platform.InputHandler { return w.input }

// InvalidateCharacterCoordinates repositions the IME candidate window at the
// caret reported by OnImePositionRequest (upstream:
// invalidate_character_coordinates).
func (w *Window) InvalidateCharacterCoordinates(bounds geometry.Bounds[geometry.Pixels]) {
	w.positionIME()
}

func (w *Window) WriteClipboard(item platform.ClipboardItem) {
	w.p.WriteClipboard(item)
}

func (w *Window) ReadClipboard() (platform.ClipboardItem, bool) {
	return w.p.ReadClipboard()
}

func (w *Window) ShowWindowMenu(position geometry.Point[geometry.Pixels]) {
	// The native system menu (title-bar menu): restored as a popup at the
	// given client position (upstream ShowWindowMenu).
	menu, _, _ := procGetSystemMenu.Call(uintptr(w.hwnd), 0)
	if menu == 0 {
		return
	}
	var pt POINT
	pt.X = int32(position.X.AsF32() * w.scale)
	pt.Y = int32(position.Y.AsF32() * w.scale)
	// TrackPopupMenu needs screen coordinates.
	procClientToScreen.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWin.Call(uintptr(w.hwnd))
	procTrackPopupMenu.Call(menu, TPM_RIGHTBUTTON,
		uintptr(pt.X), uintptr(pt.Y), 0, uintptr(w.hwnd), 0)
}

// sizeChanged reports a client-area resize (physical pixels) to the app in
// logical pixels.
func (w *Window) sizeChanged(width, height int) {
	logical := geometry.SizePixels(float32(width)/w.scale, float32(height)/w.scale)
	if logical != w.size {
		w.size = logical
		w.callbacks.OnResize(logical, w.scale)
	}
	procInvalidateRect.Call(uintptr(w.hwnd), 0, 0)
}

// scaleChanged reports a DPI change to the app.
func (w *Window) scaleChanged(scale float32) {
	if scale != w.scale && scale > 0 {
		w.scale = scale
		w.callbacks.OnResize(w.size, scale)
	}
}

// ---------------------------------------------------------------------------
// Input translation (physical -> logical)
// ---------------------------------------------------------------------------

// toLogical converts client-area physical pixels to logical pixels.
func (w *Window) toLogical(x, y int32) geometry.Point[geometry.Pixels] {
	return geometry.PointPixels(float32(x)/w.scale, float32(y)/w.scale)
}

// mouseButton dispatches a button event with double-click counting.
func (w *Window) mouseButton(button platform.MouseButton, lParam uintptr, press bool) {
	x := int16(lParam & 0xFFFF)
	y := int16((lParam >> 16) & 0xFFFF)
	mods := currentModifiers()
	pos := w.toLogical(int32(x), int32(y))
	if press {
		t, _, _ := procGetMessageTime.Call()
		ms := int32(t)
		// A second click at nearly the same position within the
		// double-click window increments the count (gpui's click
		// cadence: 1, 2, 3, then back to 1).
		if ms-w.lastClickTime < doubleClickMs &&
			abs32(int32(x)-w.lastClickX) < 4 && abs32(int32(y)-w.lastClickY) < 4 {
			w.clickCount++
		} else {
			w.clickCount = 1
		}
		w.lastClickTime = ms
		w.lastClickX = int32(x)
		w.lastClickY = int32(y)
		if w.clickCount > 3 {
			w.clickCount = 1
		}
		w.callbacks.OnInput(platform.MouseDownEvent{
			Button: button, Position: pos, Modifiers: mods, ClickCount: w.clickCount,
		})
	} else {
		w.callbacks.OnInput(platform.MouseUpEvent{
			Button: button, Position: pos, Modifiers: mods, ClickCount: w.clickCount,
		})
	}
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

// vkToKey maps virtual keys to gpui key names.
var vkToKey = map[int]string{
	0x08: "backspace", 0x09: "tab", 0x0D: "enter", 0x10: "shift",
	0x11: "control", 0x12: "alt", 0x1B: "escape", 0x20: "space",
	0x21: "pageup", 0x22: "pagedown", 0x23: "end", 0x24: "home",
	0x25: "left", 0x26: "up", 0x27: "right", 0x28: "down",
	0x2D: "insert", 0x2E: "delete",
	0x30: "0", 0x31: "1", 0x32: "2", 0x33: "3", 0x34: "4",
	0x35: "5", 0x36: "6", 0x37: "7", 0x38: "8", 0x39: "9",
	0x41: "a", 0x42: "b", 0x43: "c", 0x44: "d", 0x45: "e", 0x46: "f",
	0x47: "g", 0x48: "h", 0x49: "i", 0x4A: "j", 0x4B: "k", 0x4C: "l",
	0x4D: "m", 0x4E: "n", 0x4F: "o", 0x50: "p", 0x51: "q", 0x52: "r",
	0x53: "s", 0x54: "t", 0x55: "u", 0x56: "v", 0x57: "w", 0x58: "x",
	0x59: "y", 0x5A: "z",
	0x5B: "platform", 0x5C: "platform",
	0x60: "numpad0", 0x61: "numpad1", 0x62: "numpad2", 0x63: "numpad3",
	0x64: "numpad4", 0x65: "numpad5", 0x66: "numpad6", 0x67: "numpad7",
	0x68: "numpad8", 0x69: "numpad9",
	0x6A: "numpad*", 0x6B: "numpad+", 0x6D: "numpad-", 0x6E: "numpad.",
	0x6F: "numpad/",
	0x70: "f1", 0x71: "f2", 0x72: "f3", 0x73: "f4", 0x74: "f5",
	0x75: "f6", 0x76: "f7", 0x78: "f8", 0x79: "f9", 0x7A: "f10",
	0x7B: "f11", 0x7C: "f12",
	0xA0: "shift", 0xA1: "shift", 0xA2: "control", 0xA3: "control",
	0xA4: "alt", 0xA5: "alt",
	0xBA: ";", 0xBB: "=", 0xBC: ",", 0xBD: "-", 0xBE: ".",
	0xBF: "/", 0xC0: "`", 0xDB: "[", 0xDC: "\\", 0xDD: "]", 0xDE: "'",
}

// vkToShifted maps printable keys to their shifted character.
var vkToShifted = map[string]string{
	"a": "A", "b": "B", "c": "C", "d": "D", "e": "E", "f": "F", "g": "G",
	"h": "H", "i": "I", "j": "J", "k": "K", "l": "L", "m": "M", "n": "N",
	"o": "O", "p": "P", "q": "Q", "r": "R", "s": "S", "t": "T", "u": "U",
	"v": "V", "w": "W", "x": "X", "y": "Y", "z": "Z",
	"1": "!", "2": "@", "3": "#", "4": "$", "5": "%", "6": "^", "7": "&",
	"8": "*", "9": "(", "0": ")",
	";": ":", "=": "+", ",": "<", "-": "_", ".": ">", "/": "?",
	"`": "~", "[": "{", "\\": "|", "]": "}", "'": "\"",
}

// keyEvent dispatches key press/release with a ToUnicode-resolved KeyChar.
// pointerTouchEvent converts one WM_POINTER* message into a touch event
// (upstream: WM_POINTERDOWN/UPDATE/UP with GetPointerTouchInfo). Only
// touch pointers are converted; pen and mouse keep their classic paths.
func (w *Window) pointerTouchEvent(pointerID uint32) {
	var ptype uint32 = 0
	if r, _, _ := procGetPointerType.Call(uintptr(pointerID), uintptr(unsafe.Pointer(&ptype))); r == 0 || ptype != PT_TOUCH {
		return
	}
	var info POINTER_TOUCH_INFO
	if r, _, _ := procGetPointerTouchInfo.Call(uintptr(pointerID), uintptr(unsafe.Pointer(&info))); r == 0 {
		return
	}
	phase := platform.TouchPhaseMoved
	switch {
	case info.PointerInfo.PointerFlags&POINTER_FLAG_DOWN != 0:
		phase = platform.TouchPhaseStarted
	case info.PointerInfo.PointerFlags&POINTER_FLAG_UP != 0:
		phase = platform.TouchPhaseEnded
	}
	// Screen -> client coordinates, then to logical pixels.
	pt := info.PointerInfo.PtPixelLocation
	procScreenToClient.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&pt)))
	pressure := float32(info.Pressure) / 1024.0 // 0..1024 normalized
	w.callbacks.OnInput(platform.TouchEvent{
		ID:       platform.TouchID(info.PointerInfo.PointerId),
		Phase:    phase,
		Position: geometry.PointPixels(float32(pt.X)/w.scale, float32(pt.Y)/w.scale),
		Force:    &pressure,
	})
}

func (w *Window) keyEvent(hwnd syscall.Handle, wParam, lParam uintptr, press bool) {
	vk := int(wParam)
	key, ok := vkToKey[vk]
	if !ok {
		return
	}
	mods := currentModifiers()
	stroke := keymap.Keystroke{Key: key, Modifiers: mods}

	// Resolve the printable character (US-layout fallback covers layouts
	// where ToUnicode cannot be used, e.g. IME-intercepted keys). The
	// space bar's key name is "space", not a single character, so it must
	// be included explicitly or key-downs arrived with an empty KeyChar
	// and text surfaces dropped the space.
	if press && (len(key) == 1 || key == "space") && !mods.Control && !mods.Alt && !mods.Platform {
		if ch, resolved := toUnicodeChar(vk, lParam); resolved {
			stroke.KeyChar = ch
		} else if key == "space" {
			stroke.KeyChar = " "
		} else if mods.Shift {
			stroke.KeyChar = vkToShifted[key]
		} else {
			stroke.KeyChar = key
		}
	}

	if press {
		w.callbacks.OnInput(platform.KeyDownEvent{Keystroke: stroke})
	} else {
		w.callbacks.OnInput(platform.KeyUpEvent{Keystroke: stroke})
	}
}

// toUnicodeChar converts the current key press to text using the active
// keyboard layout. It returns ok=false when the key is a dead key or has no
// text representation.
func toUnicodeChar(vk int, lParam uintptr) (string, bool) {
	var kb [256]byte
	ret, _, _ := procGetKeyboardState.Call(uintptr(unsafe.Pointer(&kb[0])))
	if ret == 0 {
		return "", false
	}
	scanCode := (lParam >> 16) & 0xFF
	var buf [8]uint16 // ToUnicode can return up to a few UTF-16 units
	n, _, _ := procToUnicode.Call(
		uintptr(vk), scanCode,
		uintptr(unsafe.Pointer(&kb[0])),
		uintptr(unsafe.Pointer(&buf[0])), 4, 0)
	switch int32(n) {
	case -1: // dead key
		return "", false
	case 0:
		return "", false
	default:
		s := syscall.UTF16ToString(buf[:n])
		return s, s != ""
	}
}
