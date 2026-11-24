package windows

// Shell notification support (upstream platform.rs `notify`): Windows
// shows system notifications as tray balloons through Shell_NotifyIconW.
//
// Like shell_slots.go this file has NO build constraint on the data (the
// layout table and its size check compile - and are asserted by unit
// tests - on every platform); the syscall helper lives in the
// //go:build windows files.

import "unsafe"

// notifyIconDataW mirrors NOTIFYICONDATAW (shellapi.h) with explicit
// padding for 8-byte-aligned pointer targets (amd64/arm64: cbSize=976,
// the audited x64 sizeof). On 32-bit targets the member alignment
// differs and balloons are disabled (notificationBalloonAvailable).
type notifyIconDataW struct {
	CbSize           uint32
	_                [4]byte // align HWnd to 8
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	_                [4]byte // align HIcon to 8
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UTimeout         uint32 // union { uTimeout; uVersion }
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     uintptr
}

// notifyIconSize64 is sizeof(NOTIFYICONDATAW) on 64-bit Windows (976).
const notifyIconSize64 = 976

// notificationBalloonAvailable reports whether the balloon struct layout
// matches the compiled ABI (64-bit pointer targets only; the layout
// above records 8-byte alignment).
func notificationBalloonAvailable() bool {
	return int(unsafe.Sizeof(notifyIconDataW{})) == notifyIconSize64
}

// writeUTF16 copies a Go string into a fixed UTF-16 buffer (truncating,
// always NUL-terminated).
func writeUTF16(dst []uint16, s string) {
	runes := []rune(s)
	n := len(dst) - 1
	if len(runes) < n {
		n = len(runes)
	}
	for i := 0; i < n; i++ {
		dst[i] = uint16(runes[i])
	}
	for i := n; i < len(dst); i++ {
		dst[i] = 0
	}
}
