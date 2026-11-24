//go:build windows

package windows

import (
	"syscall"
	"testing"
	"unsafe"
)

// Regression tests for client-side (custom) window decorations: the native
// frame is removed by the WM_NCCALCSIZE handler. The production defect was
// that WM_NCCALCSIZE fired during CreateWindowExW — before the Window was
// registered, windowForHWND returned nil and DefWindowProc reserved the
// standard frame, leaving the OS title bar visible above the app's own
// title bar (fixed by re-sending WM_NCCALCSIZE via SWP_FRAMECHANGED after
// registration, in OpenWindow).

// registerTestWindow registers a synthetic Window so wndProc's
// windowForHWND lookup resolves it (no real win32 window is created: the
// handler paths under test early-out on the invalid hwnd).
func registerTestWindow(t *testing.T, custom bool) *Window {
	t.Helper()
	p := &Platform{windows: make(map[syscall.Handle]*Window)}
	w := &Window{p: p, hwnd: 0x4711, customDecorations: custom}
	p.windows[w.hwnd] = w
	old := activePlatform
	activePlatform = p
	t.Cleanup(func() { activePlatform = old })
	return w
}

// TestFramelessNCCalcSizeRemovesFrame pins the frame removal: with custom
// decorations and wParam TRUE, WM_NCCALCSIZE must return 0 — the client
// rect covers the whole window and no standard-frame area is reserved.
// The invalid test hwnd makes MonitorFromWindow fail; the handler's
// early-out must still report the frameless client (return 0), never fall
// through to DefWindowProc (which would keep the OS title bar).
func TestFramelessNCCalcSizeRemovesFrame(t *testing.T) {
	w := registerTestWindow(t, true)
	ret := wndProc(w.hwnd, WM_NCCALCSIZE, 1, uintptr(unsafe.Pointer(&RECT{Left: 100, Top: 100, Right: 900, Bottom: 700})))
	if ret != 0 {
		t.Fatalf("WM_NCCALCSIZE(custom, wParam=1) = %d, want 0 (client must cover the whole window)", ret)
	}
}

// TestFramelessNCActivateSkipsFramePaint pins the WM_NCACTIVATE handling:
// DefWindowProc would repaint the removed standard frame on every
// activation change (always on DWM-off / RDP sessions); the frameless
// window must answer TRUE itself and skip the non-client painting.
func TestFramelessNCActivateSkipsFramePaint(t *testing.T) {
	w := registerTestWindow(t, true)
	if ret := wndProc(w.hwnd, WM_NCACTIVATE, 1, 0); ret != 1 {
		t.Fatalf("WM_NCACTIVATE(custom) = %d, want TRUE (1)", ret)
	}
	if ret := wndProc(w.hwnd, WM_NCACTIVATE, 0, 0); ret != 1 {
		t.Fatalf("WM_NCACTIVATE(custom, deactivate) = %d, want TRUE (1)", ret)
	}
}
