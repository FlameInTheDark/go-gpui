//go:build windows

// Package windows implements the go-gpui platform on top of the win32 API
// (user32/gdi32/kernel32) with no cgo: windows are created with
// CreateWindowExW, frames are software-rasterized by the render package and
// presented with SetDIBitsToDevice, and input events come through the
// standard message loop.
package windows

import (
	"fmt"
	"image"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/keymap"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/render"
	"github.com/FlameInTheDark/go-gpui/text"
)

// ---------------------------------------------------------------------------
// DLLs and procedures
// ---------------------------------------------------------------------------

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	// IME support (composition, candidate window placement). Missing
	// imm32.dll degrades to no-op IME handling.
	imm32    = syscall.NewLazyDLL("imm32.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")

	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	procGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	procGlobalLock       = kernel32.NewProc("GlobalLock")
	procGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
	procGlobalFree       = kernel32.NewProc("GlobalFree")
	procGlobalSize       = kernel32.NewProc("GlobalSize")

	procRegisterClassExW       = user32.NewProc("RegisterClassExW")
	procCreateWindowExW        = user32.NewProc("CreateWindowExW")
	procDefWindowProcW         = user32.NewProc("DefWindowProcW")
	procDestroyWindow          = user32.NewProc("DestroyWindow")
	procGetMessageW            = user32.NewProc("GetMessageW")
	procPeekMessageW           = user32.NewProc("PeekMessageW")
	procTranslateMessage       = user32.NewProc("TranslateMessage")
	procDispatchMessageW       = user32.NewProc("DispatchMessageW")
	procPostQuitMessage        = user32.NewProc("PostQuitMessage")
	procPostMessageW           = user32.NewProc("PostMessageW")
	procShowWindow             = user32.NewProc("ShowWindow")
	procInvalidateRect         = user32.NewProc("InvalidateRect")
	procBeginPaint             = user32.NewProc("BeginPaint")
	procSetTimer               = user32.NewProc("SetTimer")
	procKillTimer              = user32.NewProc("KillTimer")
	procEndPaint               = user32.NewProc("EndPaint")
	procGetDC                  = user32.NewProc("GetDC")
	procReleaseDC              = user32.NewProc("ReleaseDC")
	procGetClientRect          = user32.NewProc("GetClientRect")
	procScreenToClient         = user32.NewProc("ScreenToClient")
	procClientToScreen         = user32.NewProc("ClientToScreen")
	procSetWindowTextW         = user32.NewProc("SetWindowTextW")
	procGetWindowTextW         = user32.NewProc("GetWindowTextW")
	procSetForegroundWin       = user32.NewProc("SetForegroundWindow")
	procIsWindowVisible        = user32.NewProc("IsWindowVisible")
	procGetKeyState            = user32.NewProc("GetKeyState")
	procGetKeyboardState       = user32.NewProc("GetKeyboardState")
	procToUnicode              = user32.NewProc("ToUnicode")
	procGetMessageTime         = user32.NewProc("GetMessageTime")
	procGetDpiForWindow        = user32.NewProc("GetDpiForWindow")
	procGetSystemMetrics       = user32.NewProc("GetSystemMetrics")
	procOpenClipboard          = user32.NewProc("OpenClipboard")
	procCloseClipboard         = user32.NewProc("CloseClipboard")
	procEmptyClipboard         = user32.NewProc("EmptyClipboard")
	procSetClipboardData       = user32.NewProc("SetClipboardData")
	procGetClipboardData       = user32.NewProc("GetClipboardData")
	procIsClipboardFormatAvail = user32.NewProc("IsClipboardFormatAvailable")
	procAdjustWindowRect       = user32.NewProc("AdjustWindowRect")
	procAdjustWindowRectForDpi = user32.NewProc("AdjustWindowRectForDpi")
	procSetWindowPos           = user32.NewProc("SetWindowPos")
	procLoadCursorW            = user32.NewProc("LoadCursorW")
	procSetCursor              = user32.NewProc("SetCursor")

	procImmGetContext            = imm32.NewProc("ImmGetContext")
	procImmReleaseContext        = imm32.NewProc("ImmReleaseContext")
	procImmGetCompositionStringW = imm32.NewProc("ImmGetCompositionStringW")
	procImmSetCompositionWindow  = imm32.NewProc("ImmSetCompositionWindow")
	procImmAssociateContext      = imm32.NewProc("ImmAssociateContext")

	procRegGetValueW = advapi32.NewProc("RegGetValueW")

	// DWM backdrop for blurred windows (best-effort; dwmapi.dll is
	// present on every supported Windows version).
	dwmapi                    = syscall.NewLazyDLL("dwmapi.dll")
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")

	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procSetDIBitsToDevice  = gdi32.NewProc("SetDIBitsToDevice")
	procStretchDIBits      = gdi32.NewProc("StretchDIBits")
	procGetDeviceCaps      = gdi32.NewProc("GetDeviceCaps")

	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procSetProcessDPIAware            = user32.NewProc("SetProcessDPIAware")
	procSendMessageW                  = user32.NewProc("SendMessageW")
	procReleaseCapture                = user32.NewProc("ReleaseCapture")
	procGetWindowRect                 = user32.NewProc("GetWindowRect")

	// Window placement / monitor (fullscreen support).
	procGetWindowPlacement = user32.NewProc("GetWindowPlacement")
	procSetWindowPlacement = user32.NewProc("SetWindowPlacement")
	procMonitorFromWindow  = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfoW    = user32.NewProc("GetMonitorInfoW")
	procIsZoomed           = user32.NewProc("IsZoomed")
	procGetWindowLongPtrW  = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW  = user32.NewProc("SetWindowLongPtrW")
	procGetSystemMenu      = user32.NewProc("GetSystemMenu")
	procTrackPopupMenu     = user32.NewProc("TrackPopupMenu")
	procGetCursorPos       = user32.NewProc("GetCursorPos")

	// Pointer (touch) input.
	procGetPointerTouchInfo = user32.NewProc("GetPointerTouchInfo")
	procGetPointerType      = user32.NewProc("GetPointerType")

	// Shell integration.
	shell32              = syscall.NewLazyDLL("shell32.dll")
	procDragAcceptFiles  = shell32.NewProc("DragAcceptFiles")
	procDragQueryFileW   = shell32.NewProc("DragQueryFileW")
	procDragQueryPoint   = shell32.NewProc("DragQueryPoint")
	procDragFinish       = shell32.NewProc("DragFinish")
	procShellExecuteW    = shell32.NewProc("ShellExecuteW")
	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")

	// Screen capture (GDI).
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procBitBlt                 = gdi32.NewProc("BitBlt")
	procGetDIBits              = gdi32.NewProc("GetDIBits")
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	// Window messages.
	WM_CREATE               = 1
	WM_DESTROY              = 2
	WM_SIZE                 = 5
	WM_ACTIVATE             = 6
	WM_SETFOCUS             = 7
	WM_KILLFOCUS            = 8
	WM_PAINT                = 15
	WM_CLOSE                = 16
	WM_QUIT                 = 18
	WM_ERASEBKGND           = 20
	WM_SHOWWINDOW           = 24
	WM_SETTINGCHANGE        = 26
	WM_KEYDOWN              = 256
	WM_KEYUP                = 257
	WM_CHAR                 = 258
	WM_SYSKEYDOWN           = 260
	WM_SYSKEYUP             = 261
	WM_IME_STARTCOMPOSITION = 0x010D
	WM_IME_ENDCOMPOSITION   = 0x010E
	WM_IME_COMPOSITION      = 0x010F
	WM_IME_SETCONTEXT       = 0x0281
	WM_MOUSEMOVE            = 512
	WM_LBUTTONDOWN          = 513
	WM_LBUTTONUP            = 514
	WM_RBUTTONDOWN          = 516
	WM_RBUTTONUP            = 517
	WM_MBUTTONDOWN          = 519
	WM_MBUTTONUP            = 520
	WM_MOUSEWHEEL           = 522
	WM_XBUTTONDOWN          = 523
	WM_XBUTTONUP            = 524
	WM_MOUSEHWHEEL          = 526
	WM_DPICHANGED           = 528
	WM_DROPFILES            = 563
	WM_POINTERDOWN          = 0x0246
	WM_POINTERUPDATE        = 0x0247
	WM_POINTERUP            = 0x0248
	WM_NCCALCSIZE           = 0x0083
	WM_NCHITTEST            = 0x0084
	WM_NCACTIVATE           = 0x0086
	WM_NCLBUTTONDOWN        = 0x00A1
	WM_APP_DISPATCH         = 0x8000 // posts queued Go closures on the UI thread
	WM_APP_CLOSEALL         = 0x8001 // asks the dispatch window to end the loop
	WM_APP_PROBE            = 0x8002 // liveness probe answered by the dispatch window

	WM_TIMER = 0x0113 // one-shot repaint timer (software fallback pacing)

	// PeekMessage flags.
	PM_REMOVE   = 0x0001
	PM_NOREMOVE = 0x0000
	PM_QS_INPUT = 0x00600000 // mouse/keyboard raw input only
	PM_QS_PAINT = 0x00020000 // paint/posted messages only

	// SetWindowPos flags.
	SWP_NOSIZE       = 0x0001
	SWP_FRAMECHANGED = 0x0020
	SWP_NOMOVE       = 0x0002
	SWP_NOZORDER     = 0x0004
	SWP_NOACTIVATE   = 0x0010

	// ShowWindow commands.
	SW_HIDE       = 0
	SW_SHOWNORMAL = 1
	SW_MAXIMIZE   = 3
	SW_MINIMIZE   = 6
	SW_RESTORE    = 9

	// Non-client hit-test results (custom decorations).
	HTCLIENT      = 1
	HTCAPTION     = 2
	HTLEFT        = 10
	HTRIGHT       = 11
	HTTOP         = 12
	HTTOPLEFT     = 13
	HTTOPRIGHT    = 14
	HTBOTTOM      = 15
	HTBOTTOMLEFT  = 16
	HTBOTTOMRIGHT = 17

	// System metrics for the resize border.
	SM_CXSIZEFRAME    = 45
	SM_CYSIZEFRAME    = 46
	SM_CXPADDEDBORDER = 92

	// Window styles.
	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_VISIBLE          = 0x10000000
	WS_POPUP            = 0x80000000
	// WS_EX_NOREDIRECTIONBITMAP: the window has no GDI redirection
	// surface, so the composition swap chain owns the pixels
	// (transparent/blurred windows only).
	WS_EX_NOREDIRECTIONBITMAP = 0x00200000

	// Class styles.
	CS_HREDRAW = 0x0002
	CS_VREDRAW = 0x0001
	CS_DBLCLKS = 0x0008

	// Virtual keys.
	VK_SHIFT   = 16
	VK_CONTROL = 17
	VK_MENU    = 18 // Alt
	VK_LWIN    = 0x5B
	VK_RWIN    = 0x5C

	// Mouse wheel.
	WHEEL_DELTA = 120

	// Clipboard.
	CF_UNICODETEXT = 13
	CF_DIB         = 8
	CF_DIBV5       = 17
	GMEM_MOVEABLE  = 0x0002

	// ImmGetCompositionString indexes (imm.h). GCS_CURSORPOS and
	// GCS_DELTASTART are "no buffer" indexes: ImmGetCompositionStringW
	// returns the value directly instead of copying bytes.
	GCS_COMPSTR    = 0x0008
	GCS_COMPATTR   = 0x0010
	GCS_CURSORPOS  = 0x0040
	GCS_DELTASTART = 0x0080
	GCS_RESULTSTR  = 0x0800

	// Composition string attributes (imm.h): the clause the IME is
	// actively converting.
	ATTR_TARGET_CONVERTED    = 0x01
	ATTR_TARGET_NOTCONVERTED = 0x02

	// COMPOSITIONFORM.dwStyle: position by caret point.
	CFS_POINT = 0x0002

	// Registry: HKCU theme preference (AppsUseLightTheme DWORD).
	HKEY_CURRENT_USER = 0x80000001
	RRF_RT_REG_DWORD  = 0x10

	// Stock cursor resource ids (LoadCursorW), MAKEINTRESOURCE values.
	IDC_ARROW       = 32512
	IDC_IBEAM       = 32513
	IDC_CROSS       = 32515
	IDC_SIZEWE      = 32644
	IDC_SIZENS      = 32645
	IDC_NO          = 32648
	IDC_HAND        = 32649
	IDC_APPSTARTING = 32650

	// DPI.
	USER_DEFAULT_SCREEN_DPI = 96

	// DWM attributes (dwmapi.h): DWMWA_SYSTEMBACKDROP_TYPE = 38, value
	// 2 per the task spec (DWMSBT values: 1 none, 2 main window, 3
	// acrylic, 4 mica alt). Best-effort: DwmSetWindowAttribute fails
	// harmlessly on builds older than 22H2.
	DWMWA_SYSTEMBACKDROP_TYPE = 38
	DWMSBT_MAINWINDOW         = 2
	// Windows 11's small native rounding; DWM keeps maximized/fullscreen
	// windows square and ignores this attribute on older builds.
	DWMWA_WINDOW_CORNER_PREFERENCE = 33
	DWMWCP_ROUNDSMALL              = 3

	// DIB.
	BI_RGB         = 0
	DIB_RGB_COLORS = 0
	SRCCOPY        = 0x00CC0020

	// DPI awareness.
	DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = ^uintptr(3) // ((HANDLE)-4)

	// Window placement / monitor.
	GWL_STYLE                = -16
	MONITOR_DEFAULTTONEAREST = 2
	WINDOWPLACEMENT_LENGTH   = 44 // sizeof on both 32/64-bit (all fields 4-byte)
	TPM_RIGHTBUTTON          = 0x0002
	TPM_RETURNCMD            = 0x0100

	// Pointer input (pointerflags in POINTER_INFO).
	POINTER_FLAG_DOWN   = 0x00010000
	POINTER_FLAG_UPDATE = 0x00020000
	POINTER_FLAG_UP     = 0x00040000
	PT_TOUCH            = 2

	// ShellExecute.
	SE_SHOWNORMAL = 1

	// Shell_NotifyIcon.
	NIM_ADD     = 0
	NIM_MODIFY  = 1
	NIM_DELETE  = 2
	NIF_MESSAGE = 0x0001
	NIF_INFO    = 0x0010

	// Screen capture.
	SM_XVIRTUALSCREEN  = 76
	SM_YVIRTUALSCREEN  = 77
	SM_CXVIRTUALSCREEN = 78
	SM_CYVIRTUALSCREEN = 79
	SRCCOPY_COPY       = 0x00CC0020
	DIB_RGB_COLORS_BUF = 0

	// double-click window in milliseconds (matches gpui's click cadence)
	doubleClickMs = 500
)

// primaryDPI returns the primary monitor's DPI (96 fallback), used to size
// windows before they exist (GetDpiForWindow needs an hwnd).
func primaryDPI() float32 {
	if hdc, _, _ := procGetDC.Call(0); hdc != 0 {
		defer procReleaseDC.Call(0, hdc)
		// LOGPIXELSX = 88
		if dpi, _, _ := procGetDeviceCaps.Call(hdc, 88); int32(dpi) != 0 {
			return float32(int32(dpi))
		}
	}
	return USER_DEFAULT_SCREEN_DPI
}

// ---------------------------------------------------------------------------
// Structures
// ---------------------------------------------------------------------------

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     syscall.Handle
	HIcon         syscall.Handle
	HCursor       syscall.Handle
	HbrBackground syscall.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       syscall.Handle
}

type POINT struct{ X, Y int32 }

type MSG struct {
	Hwnd     syscall.Handle
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       POINT
	LPrivate uint32
}

type RECT struct{ Left, Top, Right, Bottom int32 }

// WINDOWPLACEMENT records a window's restore position (winuser.h); all
// fields are 4 bytes on both 32- and 64-bit.
type WINDOWPLACEMENT struct {
	Length           uint32
	Flags            uint32
	ShowCmd          uint32
	PtMinPosition    POINT
	PtMaxPosition    POINT
	RcNormalPosition RECT
}

// MONITORINFO describes a monitor (winuser.h).
type MONITORINFO struct {
	CbSize    uint32
	RcMonitor RECT
	RcWork    RECT
	DwFlags   uint32
}

// POINTER_INFO is the common pointer-event header (winuser.h). Layout is
// pointer-size dependent; the struct is only used on 64-bit where the
// syscall ABI matches.
type POINTER_INFO struct {
	PointerType           uint32
	PointerId             uint32
	FrameId               uint32
	PointerFlags          uint32
	SourceDevice          uintptr
	WindowTarget          uintptr
	PtPixelLocation       POINT
	PtHimetricLocation    POINT
	PtPixelLocationRaw    POINT
	PtHimetricLocationRaw POINT
	Time                  uint32
	HistoryCount          uint32
	InputData             int32
	KeyStates             uint32
	PerformanceCount      uint64
	ButtonChangeType      int32
}

// POINTER_TOUCH_INFO is a touch pointer event (winuser.h).
type POINTER_TOUCH_INFO struct {
	PointerInfo POINTER_INFO
	TouchFlags  uint32
	TouchMask   uint32
	Rect        RECT
	Orientation uint32
	Pressure    uint32
}

// COMPOSITIONFORM positions the IME composition/candidate window
// (imm.h): with CFS_POINT, PtCurrentPos is in client coordinates.
type COMPOSITIONFORM struct {
	DwStyle      uint32
	PtCurrentPos POINT
	RcArea       RECT
}

type PAINTSTRUCT struct {
	Hdc         syscall.Handle
	FErase      int32
	RcPaint     RECT
	FRestore    int32
	FIncUpdate  int32
	RGBReserved [32]byte
}

type BITMAPINFOHEADER struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type BITMAPINFO struct {
	Header BITMAPINFOHEADER
	Colors [1]uint32
}

func utf16Ptr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

// ptrFromUintptr reinterprets a win32-returned address (GlobalLock output,
// valid between GlobalLock/GlobalUnlock) as a pointer without tripping go
// vet's unsafeptr check on direct uintptr-to-Pointer conversions.
func ptrFromUintptr(p uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&p))
}

// ---------------------------------------------------------------------------
// Platform (event loop, window registry, dispatch window)
// ---------------------------------------------------------------------------

// Platform implements platform.Platform on win32.
//
// Win32 message queues are per-thread: a window must be created on the same
// OS thread that pumps its messages. NewPlatform therefore calls
// runtime.LockOSThread, pinning the calling goroutine (the app's main
// goroutine) for the process lifetime; OpenWindow and Run must be called
// from that same goroutine (which is how gpui apps drive the platform).
// Without the lock the goroutine can migrate between OpenWindow and Run, and
// the loop thread would never see the window's messages: the window shows up
// but never paints or responds (the process appears frozen).
type Platform struct {
	mu      sync.Mutex
	windows map[syscall.Handle]*Window
	quit    bool
	// running is set once Run starts pumping; windows opened afterwards
	// are shown immediately (before that, showing waits for the loop).
	running bool
	textSys *text.System

	dispatchHwnd syscall.Handle // hidden message-only window
	mainThreadID uint32

	// appearance caches the detected system theme (registry read; the
	// cache is refreshed on WM_SETTINGCHANGE broadcasts).
	appearance color.WindowAppearance

	// cursor tracking: the last requested style and its loaded HCURSOR
	// (shared stock cursors are loaded once; redundant SetCursor calls
	// are skipped).
	cursorStyle  platform.MouseCursorStyle
	cursorHandle uintptr
}

var (
	activePlatform     *Platform
	activePlatformOnce sync.Once
)

// NewPlatform registers the window classes, creates the dispatch window and
// locks the calling goroutine to its OS thread (see the Platform doc). All
// later calls (OpenWindow, Run) must come from this goroutine.
func NewPlatform() (*Platform, error) {
	// Pin the UI goroutine to one OS thread before anything touches
	// user32: window creation and the message loop below then share a
	// single thread, which win32 requires.
	runtime.LockOSThread()

	// Per-monitor DPI awareness: without it Windows lies about window
	// sizes (virtualizes to 96 DPI) and scales the DWM blur. Failure is
	// fine (pre-1703 Windows falls back to the default).
	if err := procSetProcessDpiAwarenessContext.Find(); err == nil {
		procSetProcessDpiAwarenessContext.Call(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2)
	} else if err := procSetProcessDPIAware.Find(); err == nil {
		procSetProcessDPIAware.Call()
	}
	if _, err := classAtom(); err != nil {
		return nil, err
	}
	p := &Platform{
		windows:    map[syscall.Handle]*Window{},
		appearance: detectAppearance(),
	}
	activePlatform = p

	// The dispatch window must live on the message-loop thread; since the
	// loop runs on this same (locked) goroutine, create it right here so
	// DispatchOnMainThread and Quit work even before Run starts.
	if err := p.createDispatchWindow(); err != nil {
		return nil, err
	}
	// Remember the UI thread: file dialogs must run on it, and callers
	// on it must not dispatch+wait (that would deadlock the loop).
	if tid, _, _ := procGetCurrentThreadIdSafe.Call(); tid != 0 {
		p.mainThreadID = uint32(tid)
	}
	return p, nil
}

// createDispatchWindow registers the dispatch class and creates its hidden
// window on the current (locked) thread.
func (p *Platform) createDispatchWindow() error {
	hinst, _, _ := procGetModuleHandleW.Call(0)
	dispatchClass := utf16Ptr("go-gpui-dispatch")
	dwc := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		LpfnWndProc:   dispatchCallback,
		HInstance:     syscall.Handle(hinst),
		LpszClassName: dispatchClass,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&dwc)))
	hwnd, _, err := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(dispatchClass)), 0,
		0, 0, 0, 0, 0, 0, 0, hinst, 0)
	if hwnd == 0 {
		return fmt.Errorf("windows: dispatch window creation failed: %v", err)
	}
	p.mu.Lock()
	p.dispatchHwnd = syscall.Handle(hwnd)
	p.mu.Unlock()
	return nil
}

// classAtom registers the go-gpui window class once and returns its atom.
var (
	classRegistered  bool
	classAtomValue   uint16
	wndProcCallback  = syscall.NewCallback(wndProc)
	dispatchCallback = syscall.NewCallback(dispatchWndProc)
)

func classAtom() (uint16, error) {
	if classRegistered {
		return classAtomValue, nil
	}
	hinst, _, _ := procGetModuleHandleW.Call(0)
	name := utf16Ptr("go-gpui-window")
	wc := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		Style:         CS_HREDRAW | CS_VREDRAW,
		LpfnWndProc:   wndProcCallback,
		HInstance:     syscall.Handle(hinst),
		LpszClassName: name,
	}
	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		return 0, fmt.Errorf("windows: RegisterClassExW failed: %v", err)
	}
	classRegistered = true
	classAtomValue = uint16(atom)
	return classAtomValue, nil
}

// dispatchWndProc handles the hidden dispatch window: it executes queued
// closures and the quit request, and answers the UI-thread liveness probe.
func dispatchWndProc(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_APP_DISPATCH:
		drainDispatchQueue()
		return 0
	case WM_APP_CLOSEALL:
		procPostQuitMessage.Call(0)
		return 0
	case WM_APP_PROBE:
		uiPulse.Store(time.Now().UnixNano())
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return ret
}

// ---------------------------------------------------------------------------
// UI-thread watchdog
// ---------------------------------------------------------------------------

// uiPulse is the time (unix nanoseconds) the UI thread last answered a
// liveness probe. uiWatchdog posts WM_APP_PROBE to the dispatch window; any
// message pump (the main loop, or a modal move/size loop) dispatches it to
// dispatchWndProc, which stamps uiPulse. A UI thread stuck inside a paint,
// a dispatched closure or a driver call stops answering, and the watchdog
// leaves dated evidence in the log instead of the freeze being silent.
var (
	uiPulse        atomic.Int64
	uiStuckSince   atomic.Int64
	uiLastStuckLog atomic.Int64
)

// uiWatchdog probes the UI thread every few seconds and logs when the
// thread stops answering (rate limited to one line per minute). It is a
// diagnostic only: it never touches UI state.
func uiWatchdog(p *Platform) {
	for {
		if p.quitting() {
			return
		}
		t0 := time.Now().UnixNano()
		if hwnd := p.dispatchHwnd; hwnd != 0 {
			procPostMessageW.Call(uintptr(hwnd), WM_APP_PROBE, 0, 0)
		}
		time.Sleep(8 * time.Second)
		if uiPulse.Load() >= t0 {
			uiStuckSince.Store(0)
			continue
		}
		now := time.Now().UnixNano()
		if uiStuckSince.CompareAndSwap(0, t0) {
			// first detection: log immediately
			fmt.Fprintf(os.Stderr, "go-gpui: UI thread unresponsive for at least 8s (a paint, dispatched closure or driver call may be stuck); will keep checking\n")
			uiLastStuckLog.Store(now)
			continue
		}
		if since := now - uiStuckSince.Load(); since > 0 && now-uiLastStuckLog.Load() >= int64(time.Minute) {
			fmt.Fprintf(os.Stderr, "go-gpui: UI thread still unresponsive (>= %d seconds)\n", int(since/int64(time.Second)))
			uiLastStuckLog.Store(now)
		}
	}
}

// Run pumps the win32 message loop until PostQuitMessage. It must be called
// from the same goroutine as NewPlatform (the locked UI thread).
func (p *Platform) Run(onFinish func()) {
	tid, _, _ := procGetCurrentThreadIdSafe.Call()
	p.mainThreadID = uint32(tid)
	p.mu.Lock()
	p.running = true
	p.mu.Unlock()

	// Liveness watchdog: a background goroutine probing the message pump
	// (see uiPulse). Started here because the dispatch window must exist
	// and Run is the single entry into the message loop.
	uiPulse.Store(time.Now().UnixNano())
	go uiWatchdog(p)

	onFinish()

	// Windows opened before Run stay hidden until here so their first
	// frame presents right after they appear (and so nothing ever
	// presents from a thread that is not pumping messages).
	for _, w := range p.allWindows() {
		if w.needsShow {
			w.Show()
		}
	}

	var msg MSG
	for {
		ret, _, err := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			if int32(ret) == -1 {
				_ = err
			}
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		if p.quitting() && len(p.allWindows()) == 0 {
			break
		}
	}
}

func (p *Platform) quitting() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.quit
}

// Quit posts WM_QUIT from any thread. The dispatch window always exists
// (created in NewPlatform), so this works before and after Run starts.
func (p *Platform) Quit() {
	p.mu.Lock()
	p.quit = true
	hwnd := p.dispatchHwnd
	p.mu.Unlock()
	if hwnd != 0 {
		procPostMessageW.Call(uintptr(hwnd), WM_APP_CLOSEALL, 0, 0)
	}
}

// Displays returns the primary monitor size.
func (p *Platform) Displays() []platform.Display {
	w := int32(callI32(procGetSystemMetrics, 0)) // SM_CXSCREEN
	h := int32(callI32(procGetSystemMetrics, 1)) // SM_CYSCREEN
	return []platform.Display{{
		ID:          1,
		Bounds:      geometry.BoundsPixels(0, 0, float32(w), float32(h)),
		ScaleFactor: primaryDPI() / USER_DEFAULT_SCREEN_DPI,
	}}
}

func callI32(p *syscall.LazyProc, arg uintptr) int32 {
	ret, _, _ := p.Call(arg)
	return int32(ret)
}

// PrimaryDisplay returns the primary monitor.
func (p *Platform) PrimaryDisplay() (platform.Display, bool) { return p.Displays()[0], true }

// detectAppearance reads the system light/dark preference from the
// registry (HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\
// Personalize "AppsUseLightTheme", advapi32 RegGetValueW). Missing key or
// missing advapi32 keeps light (the pre-detection default).
func detectAppearance() color.WindowAppearance {
	light, ok := appsUseLightTheme()
	if ok && !light {
		return color.WindowAppearanceDark
	}
	return color.WindowAppearanceLight
}

// appsUseLightTheme reports the raw registry preference: true = light,
// false = dark; ok=false when the lookup fails (defensive: proc lazily
// found, missing advapi32.dll -> no value).
func appsUseLightTheme() (light, ok bool) {
	if err := procRegGetValueW.Find(); err != nil {
		return true, false
	}
	var value uint32
	var size uint32 = 4
	ret, _, _ := procRegGetValueW.Call(
		HKEY_CURRENT_USER,
		uintptr(unsafe.Pointer(utf16Ptr(`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`))),
		uintptr(unsafe.Pointer(utf16Ptr("AppsUseLightTheme"))),
		RRF_RT_REG_DWORD,
		0,
		uintptr(unsafe.Pointer(&value)),
		uintptr(unsafe.Pointer(&size)),
	)
	if ret != 0 {
		return true, false
	}
	return value != 0, true
}

// Appearance reports the cached system appearance; re-detects on
// WM_SETTINGCHANGE ("ImmersiveColorSet") broadcasts.
func (p *Platform) Appearance() color.WindowAppearance {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.appearance
}

// refreshAppearance re-reads the theme and, on change, pushes the new
// appearance through every window's OnAppearance callback (mirrors the
// OnActiveStatus flow: the platform dispatches the event into
// WindowCallbacks).
func (p *Platform) refreshAppearance() {
	appearance := detectAppearance()
	p.mu.Lock()
	changed := appearance != p.appearance
	p.appearance = appearance
	p.mu.Unlock()
	if !changed {
		return
	}
	for _, w := range p.allWindows() {
		if w.callbacks.OnAppearance != nil {
			w.callbacks.OnAppearance(appearance)
		}
	}
}

// ---------------------------------------------------------------------------
// Cursor shapes (upstream Platform::set_cursor_style)
// ---------------------------------------------------------------------------

// cursorResourceIDs maps cursor styles to stock IDC_* resource ids. There is
// no stock open/closed-hand cursor on Windows, so Grab/Grabbing fall back to
// IDC_APPSTARTING (upstream gpui does the same on its Windows backend).
var cursorResourceIDs = map[platform.MouseCursorStyle]uintptr{
	platform.CursorDefault:         IDC_ARROW,
	platform.CursorPointingHand:    IDC_HAND,
	platform.CursorText:            IDC_IBEAM,
	platform.CursorResizeLeftRight: IDC_SIZEWE,
	platform.CursorResizeUpDown:    IDC_SIZENS,
	platform.CursorCrosshair:       IDC_CROSS,
	platform.CursorNotAllowed:      IDC_NO,
	platform.CursorGrab:            IDC_APPSTARTING,
	platform.CursorGrabbing:        IDC_APPSTARTING,
}

// SetCursor applies a system cursor shape. Repeated calls with the same
// style are skipped (last-applied tracking); the loaded shared HCURSOR is
// cached per shape so the div cursor builder can call this every frame.
func (p *Platform) SetCursor(style platform.MouseCursorStyle) {
	p.mu.Lock()
	if p.cursorStyle == style && p.cursorHandle != 0 {
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()
	id := cursorResourceIDs[style]
	if id == 0 {
		id = IDC_ARROW
	}
	h, _, _ := procLoadCursorW.Call(0, id)
	if h == 0 {
		return
	}
	p.mu.Lock()
	p.cursorStyle = style
	p.cursorHandle = h
	p.mu.Unlock()
	procSetCursor.Call(h)
}

// applyCursor reasserts the last requested shape. Called from WM_MOUSEMOVE:
// the window class registers no class cursor, so the system cannot restore
// the shape itself and it must be set on every move (SetCursor with the
// shared HCURSOR is a handle store, not a load).
func (p *Platform) applyCursor() {
	p.mu.Lock()
	h := p.cursorHandle
	p.mu.Unlock()
	if h != 0 {
		procSetCursor.Call(h)
	}
}

// OpenURL opens a URL with the system handler (upstream open_url:
// ShellExecuteW "open").
func (p *Platform) OpenURL(url string) error {
	verb, _ := syscall.UTF16PtrFromString("open")
	target, err := syscall.UTF16PtrFromString(url)
	if err != nil {
		return err
	}
	r, _, callErr := procShellExecuteW.Call(0, uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(target)), 0, 0, SE_SHOWNORMAL)
	// ShellExecuteW returns > 32 on success (SE_ERR_* codes otherwise).
	if r <= 32 {
		if callErr != nil && callErr != syscall.Errno(0) {
			return callErr
		}
		return fmt.Errorf("windows: ShellExecuteW failed with %d", r)
	}
	return nil
}

// SendNotification shows a system notification as a tray balloon
// (upstream notify: Windows notifications go through Shell_NotifyIconW;
// a temporary tray icon carries the balloon and is removed again).
func (p *Platform) SendNotification(notification platform.Notification) error {
	if !notificationBalloonAvailable() {
		return fmt.Errorf("windows: notifications unavailable (32-bit target)")
	}
	return showNotificationBalloon(p.dispatchHwnd, notification.Title, notification.Body)
}

// CaptureDisplay captures a display with GDI (upstream capture_display:
// BitBlt of the screen DC into a DIB). displayID 0 captures the primary
// display; other ids capture the corresponding monitor of Displays().
func (p *Platform) CaptureDisplay(displayID uint32) (image.Image, bool) {
	displays := p.Displays()
	var bounds geometry.Bounds[geometry.Pixels]
	if displayID != 0 {
		for _, d := range displays {
			if d.ID == displayID {
				bounds = d.Bounds
				break
			}
		}
	}
	if bounds.Size.Width.AsF32() <= 0 {
		// Primary display (SM_CXSCREEN/SM_CYSCREEN).
		cx, _, _ := procGetSystemMetrics.Call(0)
		cy, _, _ := procGetSystemMetrics.Call(1)
		bounds = geometry.BoundsPixels(0, 0, float32(cx), float32(cy))
	}
	w := int(bounds.Size.Width.AsF32())
	h := int(bounds.Size.Height.AsF32())
	if w <= 0 || h <= 0 {
		return nil, false
	}
	screen, _, _ := procGetDC.Call(0)
	if screen == 0 {
		return nil, false
	}
	defer procReleaseDC.Call(0, screen)
	memDC, _, _ := procCreateCompatibleDC.Call(screen)
	if memDC == 0 {
		return nil, false
	}
	defer procDeleteDC.Call(memDC)
	bmp, _, _ := procCreateCompatibleBitmap.Call(screen, uintptr(w), uintptr(h))
	if bmp == 0 {
		return nil, false
	}
	defer procDeleteObject.Call(bmp)
	prev, _, _ := procSelectObject.Call(memDC, bmp)
	defer procSelectObject.Call(memDC, prev)
	if r, _, _ := procBitBlt.Call(memDC, 0, 0, uintptr(w), uintptr(h),
		screen, uintptr(int32(bounds.Origin.X.AsF32())), uintptr(int32(bounds.Origin.Y.AsF32())),
		SRCCOPY_COPY); r == 0 {
		return nil, false
	}
	// GetDIBits into a top-down BGRA buffer.
	var bi BITMAPINFO
	bi.Header.BiSize = uint32(unsafe.Sizeof(bi.Header))
	bi.Header.BiWidth = int32(w)
	bi.Header.BiHeight = -int32(h) // top-down
	bi.Header.BiPlanes = 1
	bi.Header.BiBitCount = 32
	bi.Header.BiCompression = BI_RGB
	buf := make([]byte, w*h*4)
	if r, _, _ := procGetDIBits.Call(memDC, bmp, 0, uintptr(h),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bi)),
		DIB_RGB_COLORS_BUF); r == 0 {
		return nil, false
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		img.Pix[i*4+0] = buf[i*4+2] // BGRA -> RGBA
		img.Pix[i*4+1] = buf[i*4+1]
		img.Pix[i*4+2] = buf[i*4+0]
		img.Pix[i*4+3] = 255
	}
	return img, true
}

// ReadClipboard returns CF_UNICODETEXT text and/or a CF_DIB/CF_DIBV5 image
// from the clipboard (upstream ClipboardItem carries both entries).
func (p *Platform) ReadClipboard() (platform.ClipboardItem, bool) {
	if ret, _, _ := procOpenClipboard.Call(0); ret == 0 {
		return platform.ClipboardItem{}, false
	}
	defer procCloseClipboard.Call()
	var item platform.ClipboardItem
	ok := false
	if h, _, _ := procGetClipboardData.Call(CF_UNICODETEXT); h != 0 {
		if text, got := readClipboardText(h); got {
			item.Text = text
			ok = true
		}
	}
	if img, got := clipboardDIB(); got {
		item.Image = img
		ok = true
	}
	return item, ok
}

// readClipboardText copies a CF_UNICODETEXT global handle to a Go string.
func readClipboardText(h uintptr) (string, bool) {
	ptr, _, _ := procGlobalLock.Call(h)
	if ptr == 0 {
		return "", false
	}
	defer procGlobalUnlock.Call(h)
	// Read the allocation size, then copy the UTF-16 run up to NUL.
	sizeBytes, _, _ := procGlobalSize.Call(h)
	if sizeBytes < 2 {
		return "", false
	}
	chars := unsafe.Slice((*uint16)(ptrFromUintptr(ptr)), sizeBytes/2)
	var u16 []uint16
	for _, c := range chars {
		if c == 0 {
			break
		}
		u16 = append(u16, c)
	}
	return syscall.UTF16ToString(u16), true
}

// clipboardDIB fetches the clipboard bitmap: CF_DIBV5 is preferred, CF_DIB
// as the compatibility fallback. Formats are probed with
// IsClipboardFormatAvailable first (upstream reads the DIB formats the same
// way), then the payload is decoded by parseDIB (see dib.go).
func clipboardDIB() (image.Image, bool) {
	for _, cf := range []uintptr{CF_DIBV5, CF_DIB} {
		if ret, _, _ := procIsClipboardFormatAvail.Call(cf); ret == 0 {
			continue
		}
		h, _, _ := procGetClipboardData.Call(cf)
		if h == 0 {
			continue
		}
		size, _, _ := procGlobalSize.Call(h)
		if size == 0 {
			continue
		}
		ptr, _, _ := procGlobalLock.Call(h)
		if ptr == 0 {
			continue
		}
		data := unsafe.Slice((*byte)(ptrFromUintptr(ptr)), int(size))
		img, ok := parseDIB(data)
		procGlobalUnlock.Call(h)
		if ok {
			return img, true
		}
	}
	return nil, false
}

// WriteClipboard sets CF_UNICODETEXT content and, when the item carries a
// bitmap, a 32bpp top-down CF_DIB payload (encoded by encodeDIB).
func (p *Platform) WriteClipboard(item platform.ClipboardItem) {
	if ret, _, _ := procOpenClipboard.Call(0); ret == 0 {
		return
	}
	defer procCloseClipboard.Call()
	procEmptyClipboard.Call()
	u16, err := syscall.UTF16FromString(item.Text)
	if err == nil {
		size := uintptr(len(u16)) * 2
		h, _, _ := procGlobalAlloc.Call(GMEM_MOVEABLE, size)
		if h != 0 {
			ptr, _, _ := procGlobalLock.Call(h)
			if ptr != 0 {
				dst := unsafe.Slice((*uint16)(ptrFromUintptr(ptr)), len(u16))
				copy(dst, u16)
				procGlobalUnlock.Call(h)
				// On success the system owns the handle; never
				// GlobalFree it.
				procSetClipboardData.Call(CF_UNICODETEXT, h)
			} else {
				procGlobalFree.Call(h)
			}
		}
	}
	if item.Image != nil {
		dib := encodeDIB(item.Image)
		if len(dib) > 0 {
			h, _, _ := procGlobalAlloc.Call(GMEM_MOVEABLE, uintptr(len(dib)))
			if h != 0 {
				ptr, _, _ := procGlobalLock.Call(h)
				if ptr != 0 {
					dst := unsafe.Slice((*byte)(ptrFromUintptr(ptr)), len(dib))
					copy(dst, dib)
					procGlobalUnlock.Call(h)
					procSetClipboardData.Call(CF_DIB, h)
				} else {
					procGlobalFree.Call(h)
				}
			}
		}
	}
}

// DispatchOnMainThread runs f on the message-loop thread.
func (p *Platform) DispatchOnMainThread(f func()) {
	dispatchMu.Lock()
	dispatchQueue = append(dispatchQueue, f)
	dispatchMu.Unlock()
	p.mu.Lock()
	hwnd := p.dispatchHwnd
	p.mu.Unlock()
	if hwnd != 0 {
		procPostMessageW.Call(uintptr(hwnd), WM_APP_DISPATCH, 0, 0)
	} else {
		// No dispatch window (NewPlatform failed halfway): run inline.
		f()
	}
}

// Now returns the wall clock.
func (p *Platform) Now() time.Time { return time.Now() }

// TextSystem builds the system text system (scans C:\Windows\Fonts).
func (p *Platform) TextSystem() platform.TextSystem {
	if p.textSys == nil {
		p.textSys = text.NewSystemWithSystemFonts()
	}
	return p.textSys
}

// OpenWindow creates a win32 window. The window is created hidden at its
// final DPI-scaled size; Platform.Run shows it just before the message loop
// starts pumping, so the first frame (recorded by Draw's InvalidateRect)
// presents immediately after the window appears - no white flash, and no
// presentation before the loop runs (see the Window doc in window.go).
func (p *Platform) OpenWindow(options platform.WindowOptions, callbacks platform.WindowCallbacks) (platform.PlatformWindow, error) {
	atom, err := classAtom()
	if err != nil {
		return nil, err
	}
	size := geometry.SizePixels(800, 600)
	if options.WindowBounds != nil {
		size = options.WindowBounds.Bounds.Size
	}
	title := options.Title

	// Size the client area in physical pixels using the primary monitor's
	// DPI (per-monitor aware): the window is born at its final size, with
	// no visible resize pass.
	scale := primaryDPI() / USER_DEFAULT_SCREEN_DPI
	cw := int(float32(size.Width.AsF32())*scale + 0.5)
	ch := int(float32(size.Height.AsF32())*scale + 0.5)
	outer := RECT{0, 0, int32(cw), int32(ch)}

	hinst, _, _ := procGetModuleHandleW.Call(0)
	// Client-side decorations: the app draws its own title bar, so the
	// OS frame is removed (WM_NCCALCSIZE returns the full client) but
	// WS_OVERLAPPEDWINDOW stays in the style — Windows keeps the DWM
	// shadow, snap and the resize border behavior we re-expose via
	// WM_NCHITTEST. The window is born at client size (no
	// AdjustWindowRect padding: there is no frame to account for).
	customDecorations := options.WindowDecorations != nil &&
		*options.WindowDecorations == platform.DecorationsClient
	if !customDecorations {
		dpi := uint32(primaryDPI())
		if dpi != 0 && procAdjustWindowRectForDpi.Find() == nil {
			procAdjustWindowRectForDpi.Call(uintptr(unsafe.Pointer(&outer)), WS_OVERLAPPEDWINDOW, 0, uintptr(dpi))
		} else {
			procAdjustWindowRect.Call(uintptr(unsafe.Pointer(&outer)), WS_OVERLAPPEDWINDOW, 0)
		}
	}
	// Transparent and blurred windows render through a composition swap
	// chain: the window must drop its GDI redirection bitmap, otherwise
	// the compositor never sees the swap chain's alpha.
	var exStyle uintptr
	bgAppearance := options.WindowBackground
	if bgAppearance != platform.BackgroundOpaque {
		exStyle |= WS_EX_NOREDIRECTIONBITMAP
	}
	hwnd, _, err := procCreateWindowExW.Call(
		exStyle, uintptr(atom), uintptr(unsafe.Pointer(utf16Ptr(title))),
		WS_OVERLAPPEDWINDOW, // hidden: shown by Run (needsShow)
		uintptr(100), uintptr(100),
		uintptr(outer.Right-outer.Left), uintptr(outer.Bottom-outer.Top),
		0, 0, hinst, 0)
	if hwnd == 0 {
		return nil, fmt.Errorf("windows: CreateWindowExW failed: %v", err)
	}
	// Accept external file drag-and-drop (WM_DROPFILES).
	procDragAcceptFiles.Call(hwnd, 1)
	if bgAppearance == platform.BackgroundBlurred {
		// System backdrop for blurred windows (best-effort: failures
		// on older Windows builds are ignored, the window simply
		// renders without blur).
		backdrop := uint32(DWMSBT_MAINWINDOW)
		procDwmSetWindowAttribute.Call(
			hwnd, DWMWA_SYSTEMBACKDROP_TYPE,
			uintptr(unsafe.Pointer(&backdrop)), unsafe.Sizeof(backdrop))
	}

	w := &Window{
		p:                 p,
		hwnd:              syscall.Handle(hwnd),
		size:              size,
		callbacks:         callbacks,
		atlas:             render.NewAtlas(),
		title:             title,
		needsShow:         true,
		bgAppearance:      bgAppearance,
		customDecorations: customDecorations,
	}
	// Capture the default IME context now (see associateDefaultIMC); a
	// missing imm32.dll yields 0 and disables IME handling for the window.
	w.defaultIMC = w.associateDefaultIMC()
	p.mu.Lock()
	p.windows[w.hwnd] = w
	p.mu.Unlock()

	// WM_NCCALCSIZE fired during CreateWindowExW, before this Window was
	// registered — windowForHWND returned nil and DefWindowProc reserved
	// the standard frame, so the OS title bar stayed visible (the bug only
	// escaped on displays whose DPI equals the primary's: otherwise the
	// DPI reconcile below re-sent WM_NCCALCSIZE, hiding the damage).
	// SWP_FRAMECHANGED re-sends WM_NCCALCSIZE now that the handler is
	// reachable; the frameless client takes effect while still hidden.
	if customDecorations {
		procSetWindowPos.Call(uintptr(hwnd), 0, 0, 0, 0, 0,
			SWP_FRAMECHANGED|SWP_NOMOVE|SWP_NOSIZE|SWP_NOZORDER|SWP_NOACTIVATE)
		if procDwmSetWindowAttribute.Find() == nil {
			corner := uint32(DWMWCP_ROUNDSMALL)
			procDwmSetWindowAttribute.Call(hwnd, DWMWA_WINDOW_CORNER_PREFERENCE,
				uintptr(unsafe.Pointer(&corner)), unsafe.Sizeof(corner))
		}
	}

	// The window may have landed on a monitor whose DPI differs from the
	// primary's; reconcile once while still hidden.
	w.scale = w.queryDPI()
	if real := int(float32(size.Width.AsF32())*w.scale + 0.5); real != cw {
		w.applyClientSize(real, int(float32(size.Height.AsF32())*w.scale+0.5))
	}

	// Windows opened while the loop is already pumping can show now;
	// otherwise Run shows them right before the first pump iteration.
	p.mu.Lock()
	running := p.running
	p.mu.Unlock()
	if running {
		w.Show()
	}
	return w, nil
}

func (p *Platform) allWindows() []*Window {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]*Window, 0, len(p.windows))
	for _, w := range p.windows {
		out = append(out, w)
	}
	return out
}

func windowForHWND(hwnd syscall.Handle) *Window {
	p := activePlatform
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.windows[hwnd]
}

func unregisterWindow(hwnd syscall.Handle) {
	p := activePlatform
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.windows, hwnd)
}

var (
	dispatchQueue []func()
	dispatchMu    sync.Mutex
)

func drainDispatchQueue() {
	dispatchMu.Lock()
	q := dispatchQueue
	dispatchQueue = nil
	dispatchMu.Unlock()
	for _, f := range q {
		f()
	}
}

// procGetCurrentThreadIdSafe is bound lazily to avoid an unused-variable
// warning pattern; kernel32.GetCurrentThreadId takes no arguments.
var procGetCurrentThreadIdSafe = kernel32.NewProc("GetCurrentThreadId")

// ---------------------------------------------------------------------------
// wndProc
// ---------------------------------------------------------------------------

func wndProc(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	w := windowForHWND(hwnd)
	switch msg {
	case WM_NCCALCSIZE:
		// Client-side decorations: remove the standard frame — the app
		// paints its own title bar over the whole window. Maximized
		// windows are inset to the monitor's work area (Windows
		// maximizes to the monitor plus a frame bleed, which would put
		// our content under the taskbar); snapped windows carry a small
		// offscreen bleed that is pulled back too. wParam==0 (style
		// queries) falls through to DefWindowProc.
		if w != nil && w.customDecorations && wParam != 0 {
			rc := (*RECT)(ptrFromUintptr(lParam))
			mon, _, _ := procMonitorFromWindow.Call(uintptr(hwnd), MONITOR_DEFAULTTONEAREST)
			var mi MONITORINFO
			mi.CbSize = uint32(unsafe.Sizeof(mi))
			if mon != 0 {
				procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi)))
			} else {
				return 0
			}
			if zoomed, _, _ := procIsZoomed.Call(uintptr(hwnd)); zoomed != 0 {
				// The client covers exactly the work area.
				*rc = mi.RcWork
			} else {
				// Aero snap sizes the window with the frame bleeding
				// a few pixels offscreen; clamp tiny excesses back
				// (a user-dragged window can sit far offscreen and
				// must NOT be clamped).
				const maxBleed = int32(16)
				if d := mi.RcWork.Left - rc.Left; d > 0 && d <= maxBleed {
					rc.Left = mi.RcWork.Left
				}
				if d := mi.RcWork.Top - rc.Top; d > 0 && d <= maxBleed {
					rc.Top = mi.RcWork.Top
				}
				if d := rc.Right - mi.RcWork.Right; d > 0 && d <= maxBleed {
					rc.Right = mi.RcWork.Right
				}
				if d := rc.Bottom - mi.RcWork.Bottom; d > 0 && d <= maxBleed {
					rc.Bottom = mi.RcWork.Bottom
				}
			}
			return 0
		}
	case WM_NCHITTEST:
		// With the frame removed, DefWindowProc reports HTCLIENT everywhere
		// and the resize borders die. Re-expose them on the window's edges
		// (never when maximized); everything else stays client: the app's
		// title bar handles dragging (StartWindowDrag) and its buttons are
		// ordinary client elements.
		if w != nil && w.customDecorations {
			if zoomed, _, _ := procIsZoomed.Call(uintptr(hwnd)); zoomed != 0 {
				return HTCLIENT
			}
			x := int32(int16(lParam & 0xFFFF))
			y := int32(int16(lParam >> 16))
			var wrc RECT
			procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&wrc)))
			bx := int32(callI32(procGetSystemMetrics, SM_CXSIZEFRAME))
			by := int32(callI32(procGetSystemMetrics, SM_CYSIZEFRAME))
			pad := int32(callI32(procGetSystemMetrics, SM_CXPADDEDBORDER))
			bx += pad
			by += pad
			left := x - wrc.Left
			right := wrc.Right - x
			top := y - wrc.Top
			bottom := wrc.Bottom - y
			switch {
			case left <= bx && top <= by:
				return HTTOPLEFT
			case right <= bx && top <= by:
				return HTTOPRIGHT
			case left <= bx && bottom <= by:
				return HTBOTTOMLEFT
			case right <= bx && bottom <= by:
				return HTBOTTOMRIGHT
			case top <= by:
				return HTTOP
			case bottom <= by:
				return HTBOTTOM
			case left <= bx:
				return HTLEFT
			case right <= bx:
				return HTRIGHT
			}
			return HTCLIENT
		}
	case WM_NCACTIVATE:
		// Frameless (custom decorations): DefWindowProc paints the
		// removed standard frame on activation changes — always on
		// DWM-off / RDP sessions, as a flash elsewhere. Allow the
		// activation and skip the non-client painting entirely.
		if w != nil && w.customDecorations {
			return 1 // TRUE: activation proceeds, no NC painting
		}
	case WM_ERASEBKGND:
		if w != nil {
			return 1 // the app paints the whole client area itself
		}
	case WM_CLOSE:
		// OnShouldClose reports whether the window should close: destroy
		// only when allowed (true), veto otherwise.
		if w != nil && w.callbacks.OnShouldClose != nil && !w.callbacks.OnShouldClose() {
			return 0
		}
		procDestroyWindow.Call(uintptr(hwnd))
		return 0
	case WM_DESTROY:
		unregisterWindow(hwnd)
		if w != nil {
			w.closed = true
			if w.gpu != nil && !w.painting {
				w.gpu.Close()
				w.gpu = nil
			}
			w.callbacks.OnClose()
		}
		// The last window closing ends the process (upstream quits when all
		// windows are released), whether or not the app called Quit.
		if activePlatform != nil && len(activePlatform.allWindows()) == 0 {
			procPostQuitMessage.Call(0)
		}
		return 0
	case WM_PAINT:
		var ps PAINTSTRUCT
		hdc, _, _ := procBeginPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
		if w != nil {
			// All rendering (GPU device creation, atlas uploads,
			// presenting) happens here, inside the message pump.
			w.paintFrame(syscall.Handle(hdc))
		}
		procEndPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
		return 0
	case WM_TIMER:
		if w != nil && wParam == gpuRetryTimer {
			procKillTimer.Call(uintptr(hwnd), gpuRetryTimer)
			if w.gpuRetryPending {
				w.gpuRetryPending = false
				w.gpuTried = false
				procInvalidateRect.Call(uintptr(hwnd), 0, 0)
			}
			return 0
		}
		// One-shot repaint timer for the software fallback's frame
		// pacing (see Window.Draw): when it fires, invalidate so the
		// next paint presents the pending scene. Other timer ids fall
		// through to DefWindowProc.
		if w != nil && wParam == softwareRepaintTimer {
			w.softwareTimerArmed = false
			procKillTimer.Call(uintptr(hwnd), softwareRepaintTimer)
			procInvalidateRect.Call(uintptr(hwnd), 0, 0)
			return 0
		}
	case WM_SIZE:
		if w != nil {
			width := int32(lParam & 0xFFFF)
			height := int32((lParam >> 16) & 0xFFFF)
			w.sizeChanged(int(width), int(height))
		}
		return 0
	case WM_DPICHANGED:
		if w != nil {
			dpi := int32(wParam & 0xFFFF)
			w.scaleChanged(float32(dpi) / USER_DEFAULT_SCREEN_DPI)
			// Apply the suggested window rect: keeps the logical size
			// constant across monitor moves.
			if rect := (*RECT)(ptrFromUintptr(lParam)); rect != nil {
				procSetWindowPos.Call(uintptr(hwnd), 0,
					uintptr(rect.Left), uintptr(rect.Top),
					uintptr(rect.Right-rect.Left), uintptr(rect.Bottom-rect.Top),
					SWP_NOZORDER|SWP_NOACTIVATE)
			}
		}
		return 0
	case WM_SETFOCUS:
		if w != nil && !w.active {
			w.active = true
			w.callbacks.OnActiveStatus(true)
		}
		// Text input focus enables the IME context (upstream:
		// ImmAssociateContext on window focus when an input handler
		// is set).
		if w != nil {
			w.setIMEEnabled(w.input != nil)
		}
		return 0
	case WM_KILLFOCUS:
		if w != nil && w.active {
			w.active = false
			w.callbacks.OnActiveStatus(false)
		}
		// Losing input focus disables the IME context.
		if w != nil {
			w.setIMEEnabled(false)
		}
		return 0
	case WM_ACTIVATE:
		if w != nil {
			active := int16(wParam) != 0 // WA_INACTIVE is 0
			if active != w.active {
				w.active = active
				w.callbacks.OnActiveStatus(active)
			}
		}
		return 0
	case WM_SHOWWINDOW:
		if w != nil {
			visible := wParam != 0
			if visible != w.visible {
				w.visible = visible
				w.callbacks.OnVisibility(visible)
			}
		}
		return 0
	case WM_KEYDOWN, WM_SYSKEYDOWN:
		if w != nil {
			w.keyEvent(hwnd, wParam, lParam, true)
		}
		return 0
	case WM_KEYUP, WM_SYSKEYUP:
		if w != nil {
			w.keyEvent(hwnd, wParam, lParam, false)
		}
		return 0
	case WM_LBUTTONDOWN:
		if w != nil {
			w.mouseButton(platform.ButtonLeft, lParam, true)
		}
		return 0
	case WM_LBUTTONUP:
		if w != nil {
			w.mouseButton(platform.ButtonLeft, lParam, false)
		}
		return 0
	case WM_RBUTTONDOWN:
		if w != nil {
			w.mouseButton(platform.ButtonRight, lParam, true)
		}
		return 0
	case WM_RBUTTONUP:
		if w != nil {
			w.mouseButton(platform.ButtonRight, lParam, false)
		}
		return 0
	case WM_MBUTTONDOWN:
		if w != nil {
			w.mouseButton(platform.ButtonMiddle, lParam, true)
		}
		return 0
	case WM_MBUTTONUP:
		if w != nil {
			w.mouseButton(platform.ButtonMiddle, lParam, false)
		}
		return 0
	case WM_MOUSEMOVE:
		if w != nil {
			// Coalesce: drop intermediate queued moves and keep the
			// newest position. Without this a fast drag floods the
			// message queue with WM_MOUSEMOVEs; each one built a full
			// frame, the backlog grew unboundedly (the frame rate is
			// below the pointer's event rate), and the visual update
			// lagged seconds behind the pointer - plus WM_PAINT was
			// starved because the queue never drained. Draining here
			// bounds the pump to one frame per iteration and lets
			// WM_PAINT through between them.
			for {
				var next MSG
				r, _, _ := procPeekMessageW.Call(
					uintptr(unsafe.Pointer(&next)), uintptr(hwnd),
					WM_MOUSEMOVE, WM_MOUSEMOVE, PM_REMOVE)
				if r == 0 {
					break
				}
				lParam = next.LParam
			}
			x := int16(lParam & 0xFFFF)
			y := int16((lParam >> 16) & 0xFFFF)
			pos := w.toLogical(int32(x), int32(y))
			w.callbacks.OnInput(platform.MouseMoveEvent{
				Position:  pos,
				Modifiers: currentModifiers(),
			})
			// Reassert the cursor shape: the window class has no
			// class cursor, so nothing else restores it.
			w.p.applyCursor()
		}
		return 0
	case WM_MOUSEWHEEL, WM_MOUSEHWHEEL:
		if w != nil {
			delta := int16(wParam >> 16)
			// Raw sign, upstream gpui_windows does the same: wheel up (away
			// from the user) is positive. Scroll consumers decide the
			// direction (a scroll container's offset decreases scrolling
			// down; the editor negates into its top-line scroll position).
			lines := float32(delta) / WHEEL_DELTA * 3
			var dx, dy float32
			if msg == WM_MOUSEWHEEL {
				dy = lines
			} else {
				// Horizontal: a right tilt (positive raw delta) pans the
				// view right, so dx keeps the raw sign (the vertical axis
				// negates: wheel up = scroll up = earlier content).
				dx = float32(delta) / WHEEL_DELTA * 3
			}
			pt := POINT{X: int32(int16(lParam & 0xFFFF)), Y: int32(int16((lParam >> 16) & 0xFFFF))}
			procScreenToClient.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&pt)))
			pos := w.toLogical(pt.X, pt.Y)
			w.callbacks.OnInput(platform.ScrollEvent{
				Position:  pos,
				Delta:     platform.ScrollDeltaLines(dx, dy),
				Modifiers: currentModifiers(),
				Source:    platform.ScrollSourcePointer,
			})
		}
		return 0
	case WM_DROPFILES:
		// External drag-and-drop of files onto the window (upstream
		// FileDropEvent). The drop position arrives in client pixels.
		if w != nil {
			var pt POINT
			procDragQueryPoint.Call(wParam, uintptr(unsafe.Pointer(&pt)))
			count, _, _ := procDragQueryFileW.Call(wParam, 0xFFFFFFFF, 0, 0)
			paths := make([]string, 0, count)
			for i := uintptr(0); i < count; i++ {
				n, _, _ := procDragQueryFileW.Call(wParam, i, 0, 0)
				if n == 0 {
					continue
				}
				buf := make([]uint16, n+1)
				procDragQueryFileW.Call(wParam, i, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
				paths = append(paths, syscall.UTF16ToString(buf))
			}
			procDragFinish.Call(wParam)
			if len(paths) > 0 {
				w.callbacks.OnInput(platform.FileDropEvent{
					Kind:     platform.FileDropSubmit,
					Position: geometry.PointPixels(float32(pt.X)/w.scale, float32(pt.Y)/w.scale),
					Paths:    paths,
				})
			}
		}
		return 0
	case WM_POINTERDOWN, WM_POINTERUPDATE, WM_POINTERUP:
		// Touch contacts arrive as pointer events (Windows 8+; touch
		// pointers are delivered by default, no registration needed).
		// GET_POINTERID_WPARAM = wParam & 0xFFFF.
		if w != nil {
			w.pointerTouchEvent(uint32(wParam & 0xFFFF))
		}
		return 0
	case WM_SETTINGCHANGE:
		// The system broadcasts theme changes as ImmersiveColorSet.
		// Re-read the registry and report the change through the
		// windows' OnAppearance callbacks.
		if lParamString(lParam) == "ImmersiveColorSet" && activePlatform != nil {
			activePlatform.refreshAppearance()
		}
	case WM_IME_SETCONTEXT:
		if w != nil {
			// The default composition window stays visible (the
			// port does not draw preedit itself); just keep the
			// candidate window anchored to the caret.
			w.positionIME()
		}
	case WM_IME_STARTCOMPOSITION:
		if w != nil {
			w.imeStartComposition()
			return 0
		}
	case WM_IME_COMPOSITION:
		if w != nil {
			w.imeComposition(lParam)
			return 0
		}
	case WM_IME_ENDCOMPOSITION:
		if w != nil {
			w.imeEndComposition()
			return 0
		}
	}
	ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return ret
}

// currentModifiers samples the synchronous key state.
func currentModifiers() keymap.Modifiers {
	shift := keyStateDown(VK_SHIFT)
	control := keyStateDown(VK_CONTROL)
	alt := keyStateDown(VK_MENU)
	win := keyStateDown(VK_LWIN) || keyStateDown(VK_RWIN)
	return keymap.Modifiers{Shift: shift, Control: control, Alt: alt, Platform: win}
}

func keyStateDown(vk int) bool {
	ret, _, _ := procGetKeyState.Call(uintptr(vk))
	return ret&0x8000 != 0
}

// ---------------------------------------------------------------------------
// IME via IMM32 (upstream PlatformInputHandler composition: marked text,
// marked/selected/replaced ranges, candidate window positioning)
// ---------------------------------------------------------------------------

// imeAvailable reports whether imm32.dll exposes the composition API. The
// check is cheap after the first call (LazyProc caches the lookup); missing
// imm32.dll degrades every IME path to a no-op.
func imeAvailable() bool {
	return procImmGetContext.Find() == nil && procImmAssociateContext.Find() == nil
}

// lParamString reads the NUL-terminated UTF-16 string some messages carry in
// their lParam (WM_SETTINGCHANGE's section name), bounded defensively.
func lParamString(lParam uintptr) string {
	if lParam == 0 {
		return ""
	}
	var u16 []uint16
	for i := uintptr(0); i < 512; i++ {
		c := *(*uint16)(ptrFromUintptr(lParam + i*2))
		if c == 0 {
			break
		}
		u16 = append(u16, c)
	}
	return syscall.UTF16ToString(u16)
}

// associateDefaultIMC captures the window's default IME context so it can be
// re-associated after setIMEEnabled(false) dissociates it. Called once from
// OpenWindow while the window still carries its default context; returns 0
// when imm32 is unavailable (IME then stays permanently disabled - the
// defensive path).
func (w *Window) associateDefaultIMC() uintptr {
	if !imeAvailable() {
		return 0
	}
	himc, _, _ := procImmGetContext.Call(uintptr(w.hwnd))
	if himc == 0 {
		return 0
	}
	// Re-associating the current context is a no-op whose return value is
	// the context we want to remember.
	prev, _, _ := procImmAssociateContext.Call(uintptr(w.hwnd), himc)
	return prev
}

// setIMEEnabled enables/disables IME processing for the window by
// associating/dissociating its IME context (upstream: associate on text
// input focus, dissociate when the editor loses focus). Disabled when
// defaultIMC == 0 (no imm32 / capture failed): composition never starts.
func (w *Window) setIMEEnabled(enabled bool) {
	if !imeAvailable() || w.defaultIMC == 0 {
		return
	}
	assoc := uintptr(0)
	if enabled {
		assoc = w.defaultIMC
	}
	procImmAssociateContext.Call(uintptr(w.hwnd), assoc)
}

// positionIME moves the IME composition/candidate window to the caret. The
// caret position comes from the app via OnImePositionRequest (logical
// pixels); nil falls back to the origin.
func (w *Window) positionIME() {
	if !imeAvailable() || w.defaultIMC == 0 {
		return
	}
	himc, _, _ := procImmGetContext.Call(uintptr(w.hwnd))
	if himc == 0 {
		return
	}
	defer procImmReleaseContext.Call(uintptr(w.hwnd), himc)
	pos := geometry.Point[geometry.Pixels]{}
	if w.callbacks.OnImePositionRequest != nil {
		pos = w.callbacks.OnImePositionRequest()
	}
	form := COMPOSITIONFORM{
		DwStyle: CFS_POINT,
		PtCurrentPos: POINT{
			X: int32(pos.X.AsF32()*w.scale + 0.5),
			Y: int32(pos.Y.AsF32()*w.scale + 0.5),
		},
	}
	procImmSetCompositionWindow.Call(himc, uintptr(unsafe.Pointer(&form)))
}

// imeStartComposition reports the beginning of a composition session and
// anchors the candidate window at the caret.
func (w *Window) imeStartComposition() {
	w.imeResultDelivered = false
	start, end := w.imeBaseRange()
	w.callbacks.OnInput(platform.CompositionEvent{
		Phase:         platform.CompositionStarted,
		ReplacedRange: [2]int{start, end},
	})
	w.positionIME()
}

// imeEndComposition reports the end of a composition session. When a result
// string already committed the text (GCS_RESULTSTR), no extra event is
// sent; an end without a result cancels the preedit (empty marked text).
func (w *Window) imeEndComposition() {
	if w.imeResultDelivered {
		w.imeResultDelivered = false
		return
	}
	start, end := w.imeBaseRange()
	w.callbacks.OnInput(platform.CompositionEvent{
		Phase:         platform.CompositionEnded,
		ReplacedRange: [2]int{start, end},
	})
	w.positionIME()
}

// imeComposition delivers WM_IME_COMPOSITION updates: GCS_RESULTSTR commits
// the converted text (CompositionEnded), GCS_COMPSTR updates the preedit
// (CompositionUpdated with marked/selected ranges like upstream ImeComposition).
func (w *Window) imeComposition(lParam uintptr) {
	if !imeAvailable() {
		return
	}
	himc, _, _ := procImmGetContext.Call(uintptr(w.hwnd))
	if himc == 0 {
		return
	}
	defer procImmReleaseContext.Call(uintptr(w.hwnd), himc)
	if lParam&GCS_RESULTSTR != 0 {
		data, ok := immCompositionString(himc, GCS_RESULTSTR)
		if !ok {
			return
		}
		committed := utf16BytesToString(data)
		start, end := w.imeBaseRange()
		w.imeResultDelivered = true
		w.callbacks.OnInput(platform.CompositionEvent{
			Phase:         platform.CompositionEnded,
			MarkedText:    committed,
			ReplacedRange: [2]int{start, end},
		})
		w.positionIME()
		return
	}
	if lParam&GCS_COMPSTR != 0 {
		data, ok := immCompositionString(himc, GCS_COMPSTR)
		if !ok {
			return
		}
		comp := utf16BytesToString(data)
		start, end := w.imeBaseRange()
		markedLen := utf16Len(comp)
		// The caret inside the preedit: GCS_CURSORPOS is an offset
		// within the composition string.
		cursor, _, _ := procImmGetCompositionStringW.Call(himc, GCS_CURSORPOS, 0, 0)
		cursorOffset := int(int32(cursor))
		if cursorOffset < 0 {
			cursorOffset = 0
		}
		selStart, selEnd := start+cursorOffset, start+cursorOffset
		if attrs, attrOk := immCompositionString(himc, GCS_COMPATTR); attrOk && len(attrs) > 0 {
			s, e := imeTargetRun(attrs, cursorOffset)
			selStart, selEnd = start+s, start+e
		} else if delta, _, _ := procImmGetCompositionStringW.Call(himc, GCS_DELTASTART, 0, 0); int32(delta) > 0 {
			// No COMPATTR (some IMEs skip it while typing): fall
			// back to GCS_DELTASTART, the start of the unchanged
			// text within the composition string. The clause
			// under conversion ends at the caret, so its run
			// begins delta positions earlier; clamped to the
			// preedit because IMEs are loose with the value.
			d := int(int32(delta))
			if d > cursorOffset {
				d = cursorOffset
			}
			selStart = start + cursorOffset - d
		}
		w.callbacks.OnInput(platform.CompositionEvent{
			Phase:         platform.CompositionUpdated,
			MarkedText:    comp,
			MarkedRange:   &[2]int{start, start + markedLen},
			SelectedRange: [2]int{selStart, selEnd},
			ReplacedRange: [2]int{start, end},
			CursorOffset:  &cursorOffset,
		})
		w.positionIME()
	}
}

// imeBaseRange returns the buffer range the composition currently replaces:
// the marked range when present, otherwise the selection caret, otherwise
// the end of the text (all in the units the InputHandler reports, UTF-16).
func (w *Window) imeBaseRange() (int, int) {
	if w.input != nil {
		if s, e, ok := w.input.MarkedTextRange(); ok {
			return s, e
		}
		if s, _, _, ok := w.input.SelectedTextRange(true); ok {
			return s, s
		}
		return w.input.TextLength(), w.input.TextLength()
	}
	return 0, 0
}

// immCompositionString reads an ImmGetCompositionStringW value (UTF-16
// strings, attribute arrays). The two-call protocol: first pass with a nil
// buffer returns the required size.
func immCompositionString(himc uintptr, index uint32) ([]byte, bool) {
	size, _, _ := procImmGetCompositionStringW.Call(himc, uintptr(index), 0, 0)
	if int32(size) <= 0 {
		return nil, false
	}
	buf := make([]byte, size)
	ret, _, _ := procImmGetCompositionStringW.Call(
		himc, uintptr(index), uintptr(unsafe.Pointer(&buf[0])), uintptr(size))
	if int32(ret) < 0 {
		return nil, false
	}
	return buf, true
}

// imeTargetRun finds the contiguous run of ATTR_TARGET_* attributes (the
// clause the IME is converting) nearest the caret; falls back to a collapsed
// caret range when the composition string has no target attributes.
func imeTargetRun(attrs []byte, cursor int) (int, int) {
	isTarget := func(a byte) bool {
		return a == ATTR_TARGET_CONVERTED || a == ATTR_TARGET_NOTCONVERTED
	}
	at := cursor
	if at < 0 || at >= len(attrs) || !isTarget(attrs[at]) {
		at = -1
		for i, a := range attrs {
			if isTarget(a) {
				at = i
				break
			}
		}
		if at < 0 {
			return cursor, cursor
		}
	}
	start := at
	for start > 0 && isTarget(attrs[start-1]) {
		start--
	}
	end := at
	for end < len(attrs)-1 && isTarget(attrs[end+1]) {
		end++
	}
	return start, end + 1
}

// utf16BytesToString decodes the raw UTF-16 bytes ImmGetCompositionStringW
// returns.
func utf16BytesToString(b []byte) string {
	if len(b) < 2 {
		return ""
	}
	u16 := unsafe.Slice((*uint16)(unsafe.Pointer(&b[0])), len(b)/2)
	return syscall.UTF16ToString(u16)
}

// utf16Len counts UTF-16 code units (the units ranges are indexed in).
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		if l := utf16.RuneLen(r); l > 0 {
			n += l
		}
	}
	return n
}
