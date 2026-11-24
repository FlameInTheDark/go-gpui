//go:build windows

package windows

import (
	"syscall"
	"unsafe"
)

// showNotificationBalloon displays a tray balloon notification: a hidden
// tray icon is added with NIF_INFO, the balloon shows, and the icon is
// removed again (notifications in this port are fire-and-forget; the OS
// keeps the balloon on screen for its own timeout).
//
// Must run on the UI thread (the dispatch window receives the callback
// message).
func showNotificationBalloon(hwnd syscall.Handle, title, body string) error {
	var nid notifyIconDataW
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = uintptr(hwnd)
	nid.UID = 1
	nid.UFlags = NIF_MESSAGE | NIF_INFO
	nid.UCallbackMessage = 0x7FFF // WM_APP-range callback slot (unused)
	writeUTF16(nid.SzInfoTitle[:], title)
	writeUTF16(nid.SzInfo[:], body)

	if r, _, _ := procShellNotifyIconW.Call(NIM_ADD, uintptr(unsafe.Pointer(&nid))); r == 0 {
		// An icon with this id may survive an earlier notification:
		// replace it and try once more.
		if r2, _, _ := procShellNotifyIconW.Call(NIM_MODIFY, uintptr(unsafe.Pointer(&nid))); r2 == 0 {
			return syscall.Errno(0)
		}
	}
	// Remove the temporary tray icon.
	procShellNotifyIconW.Call(NIM_DELETE, uintptr(unsafe.Pointer(&nid)))
	return nil
}
