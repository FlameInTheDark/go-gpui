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

// stubMonitorAndFrame substitutes a synthetic monitor with the given work
// area and a 8 px resize frame (the real SM_CXSIZEFRAME 4 + SM_CXPADDEDBORDER
// 4 at 96 DPI) for the win32 lookups the frameless client-rect computation
// makes; the production values are restored on cleanup.
func stubMonitorAndFrame(t *testing.T, work RECT) {
	t.Helper()
	oldWork, oldFrame := nearestMonitorWorkArea, resizeFrameMetrics
	nearestMonitorWorkArea = func(syscall.Handle) (RECT, bool) { return work, true }
	resizeFrameMetrics = func() (int32, int32) { return 8, 8 }
	t.Cleanup(func() { nearestMonitorWorkArea, resizeFrameMetrics = oldWork, oldFrame })
}

// TestNCCalcSizeDragRestoreWhileZoomed pins mechanism A of the native
// caption-strip bug: during the SC_MOVE drag-restore of a maximized window
// a WM_NCCALCSIZE can arrive while IsZoomed still reports TRUE and the
// proposed rect is already the restored on-screen rect. The handler must
// keep client == proposed rect: overwriting it with the monitor work area
// detaches the client from the window, DWM paints the native caption into
// the mismatch band, and nothing re-sends WM_NCCALCSIZE after the drag to
// repair it (the strip persisted until the next zoom toggle).
func TestNCCalcSizeDragRestoreWhileZoomed(t *testing.T) {
	w := registerTestWindow(t, true)
	stubMonitorAndFrame(t, RECT{Left: 0, Top: 0, Right: 1920, Bottom: 1080})
	proposed := RECT{Left: 200, Top: 150, Right: 900, Bottom: 800}
	rc := proposed
	if ret := calcFramelessClientRect(w, w.hwnd, &rc, true); ret != 0 {
		t.Fatalf("calcFramelessClientRect = %d, want 0 (frameless)", ret)
	}
	if rc != proposed {
		t.Fatalf("drag-restore client rect = %v, want the proposed rect %v (a restored rect must not be replaced by the work area even while IsZoomed reports TRUE)", rc, proposed)
	}
}

// TestNCCalcSizeDraggedWindowAboveWorkArea pins mechanism B: the restored
// window tracks the cursor, and the grab point near the top of the old
// maximized window can leave the restored top edge a few pixels above the
// work area. The snap-bleed clamp must not fire — a dragged window does
// not span the work area in the edge's perpendicular axis — so client.Top
// stays at window.Top and no non-client band exists for DWM to paint the
// native caption into.
func TestNCCalcSizeDraggedWindowAboveWorkArea(t *testing.T) {
	w := registerTestWindow(t, true)
	work := RECT{Left: 0, Top: 0, Right: 1920, Bottom: 1080}
	stubMonitorAndFrame(t, work)
	proposed := RECT{Left: 300, Top: work.Top - 5, Right: 1100, Bottom: 760}
	rc := proposed
	if ret := calcFramelessClientRect(w, w.hwnd, &rc, false); ret != 0 {
		t.Fatalf("calcFramelessClientRect = %d, want 0 (frameless)", ret)
	}
	if rc != proposed {
		t.Fatalf("dragged-window client rect = %v, want %v (client.Top must stay at window.Top)", rc, proposed)
	}
}

// TestNCCalcSizeSnappedWindowStillClamped preserves the original purpose
// of the bleed clamp: an Aero-snapped window is placed at the work area
// inflated by the invisible resize frame (8 px here) on every side, and
// every edge is pulled back so the client fits the snap tile exactly —
// the behavior the dragged-window fix must not lose.
func TestNCCalcSizeSnappedWindowStillClamped(t *testing.T) {
	w := registerTestWindow(t, true)
	work := RECT{Left: 0, Top: 0, Right: 1920, Bottom: 1080}
	stubMonitorAndFrame(t, work)
	rc := RECT{
		Left:   work.Left - 8,
		Top:    work.Top - 8,
		Right:  work.Right + 8,
		Bottom: work.Bottom + 8,
	}
	if ret := calcFramelessClientRect(w, w.hwnd, &rc, false); ret != 0 {
		t.Fatalf("calcFramelessClientRect = %d, want 0 (frameless)", ret)
	}
	if rc != work {
		t.Fatalf("snapped-window client rect = %v, want the work area %v (every snap-bleed edge must be pulled back)", rc, work)
	}
}

// TestNCCalcSizeMaximizedUsesWorkArea pins the genuine maximize fit: the
// maximized window rect extends past the work area on all four sides
// (Windows maximizes past the monitor by the resize frame), and the client
// must cover exactly the work area so no content hides under the taskbar.
func TestNCCalcSizeMaximizedUsesWorkArea(t *testing.T) {
	w := registerTestWindow(t, true)
	work := RECT{Left: 0, Top: 0, Right: 1920, Bottom: 1040} // taskbar below
	stubMonitorAndFrame(t, work)
	rc := RECT{Left: -8, Top: -8, Right: 1928, Bottom: 1088} // maximized placement
	if ret := calcFramelessClientRect(w, w.hwnd, &rc, true); ret != 0 {
		t.Fatalf("calcFramelessClientRect = %d, want 0 (frameless)", ret)
	}
	if rc != work {
		t.Fatalf("maximized client rect = %v, want the work area %v", rc, work)
	}
}

// TestNCCalcSizeWParamZero pins the wParam FALSE variant: custom
// decorations answer it directly with 0 — the proposed client rect stands
// untouched and DefWindowProc (which reserves the standard frame) never
// runs for any NCCALCSIZE variant.
func TestNCCalcSizeWParamZero(t *testing.T) {
	w := registerTestWindow(t, true)
	proposed := RECT{Left: 100, Top: 100, Right: 900, Bottom: 700}
	rc := proposed
	if ret := wndProc(w.hwnd, WM_NCCALCSIZE, 0, uintptr(unsafe.Pointer(&rc))); ret != 0 {
		t.Fatalf("WM_NCCALCSIZE(custom, wParam=0) = %d, want 0", ret)
	}
	if rc != proposed {
		t.Fatalf("wParam=0 modified the rect: got %v, want %v (the proposed client rect must stand)", rc, proposed)
	}
}

// TestNeedsFrameResyncOnExitSizeMove pins the post-drag frame resync
// trigger: the move/size modal loop ends with WM_EXITSIZEMOVE and nothing
// else re-runs WM_NCCALCSIZE against the final placement, so exactly that
// message must fire the frame change. Its sibling WM_ENTERSIZEMOVE opens
// the loop (the placement is not final) and ordinary position messages
// arrive during it; neither may re-sync.
func TestNeedsFrameResyncOnExitSizeMove(t *testing.T) {
	if !needsFrameResync(WM_EXITSIZEMOVE) {
		t.Fatal("WM_EXITSIZEMOVE must trigger a frame resync: it is the only signal that a drag/snap/resize gesture ended")
	}
	for _, msg := range []uint32{WM_ENTERSIZEMOVE, WM_MOVE, WM_SIZE, WM_NCCALCSIZE, WM_MOUSEMOVE} {
		if needsFrameResync(msg) {
			t.Fatalf("message %d must not trigger a frame resync", msg)
		}
	}
}
