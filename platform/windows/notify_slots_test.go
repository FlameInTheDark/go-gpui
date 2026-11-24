package windows

import (
	"testing"
	"unsafe"
)

// TestNotifyIconDataLayout verifies the NOTIFYICONDATAW byte layout used
// for Shell_NotifyIconW balloons: the explicit padding must reproduce
// the audited x64 offsets (cbSize=976, hWnd at 8, szInfo at 304,
// szInfoTitle at 820).
func TestNotifyIconDataLayout(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("layout asserted for 64-bit pointer targets")
	}
	var nid notifyIconDataW
	if got := unsafe.Sizeof(nid); got != notifyIconSize64 {
		t.Fatalf("sizeof(NOTIFYICONDATAW) = %d, want %d", got, notifyIconSize64)
	}
	if off := unsafe.Offsetof(nid.HWnd); off != 8 {
		t.Fatalf("hWnd offset = %d, want 8", off)
	}
	if off := unsafe.Offsetof(nid.UFlags); off != 20 {
		t.Fatalf("uFlags offset = %d, want 20", off)
	}
	if off := unsafe.Offsetof(nid.SzTip); off != 40 {
		t.Fatalf("szTip offset = %d, want 40", off)
	}
	if off := unsafe.Offsetof(nid.SzInfo); off != 304 {
		t.Fatalf("szInfo offset = %d, want 304", off)
	}
	if off := unsafe.Offsetof(nid.UTimeout); off != 816 {
		t.Fatalf("uTimeout offset = %d, want 816", off)
	}
	if off := unsafe.Offsetof(nid.SzInfoTitle); off != 820 {
		t.Fatalf("szInfoTitle offset = %d, want 820", off)
	}
	if off := unsafe.Offsetof(nid.HBalloonIcon); off != 968 {
		t.Fatalf("hBalloonIcon offset = %d, want 968", off)
	}
	if !notificationBalloonAvailable() {
		t.Fatal("balloons must be available on 64-bit targets")
	}
}

// TestWriteUTF16 verifies the fixed-buffer string copy: truncates,
// NUL-terminates.
func TestWriteUTF16(t *testing.T) {
	buf := make([]uint16, 8)
	writeUTF16(buf, "hello")
	if buf[0] != 'h' || buf[4] != 'o' || buf[5] != 0 {
		t.Fatalf("writeUTF16 = %v", buf)
	}
	writeUTF16(buf, "abcdefghijklmnopqrstuvwxyz")
	for i := range buf {
		if i < 7 && buf[i] == 0 {
			t.Fatalf("truncation left a hole at %d: %v", i, buf)
		}
	}
	if buf[7] != 0 {
		t.Fatalf("buffer must stay NUL-terminated: %v", buf)
	}
}
