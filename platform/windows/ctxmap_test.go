//go:build windows

package windows

import (
	"strings"
	"syscall"
	"testing"
	"unsafe"
)

// Synthetic ID3D11DeviceContext for Map regression tests: a bare COM object
// whose vtable slots all point at Go callbacks (syscall.NewCallback).

// mapSentinel is a non-nil pointer the success stub writes into pData.
var mapSentinel uintptr

// ctxMapFailStub returns S_OK without populating the mapped resource.
var ctxMapFailStub = syscall.NewCallback(func(this, resource, sub, kind, flags, out uintptr) uintptr {
	return 0
})

// ctxMapOKStub emulates a successful Map: fills pData with a non-null
// sentinel and returns 0.
var ctxMapOKStub = syscall.NewCallback(func(this, resource, sub, kind, flags, out uintptr) uintptr {
	if out != 0 {
		m := (*d3d11MappedSubresource)(unsafe.Pointer(out))
		m.pData = unsafe.Pointer(&mapSentinel) // any non-nil pointer
		m.RowPitch, m.DepthPitch = 64, 64
	}
	return 0
})

// fakeDeviceContext builds a COM interface pointer whose slot 14 (Map)
// invokes fn. All other slots get the fail stub (ctxMap tests never reach
// them).
func fakeDeviceContext(fn uintptr) uintptr {
	vtbl := make([]uintptr, slotCtxMap+1)
	for i := range vtbl {
		vtbl[i] = ctxMapFailStub
	}
	vtbl[slotCtxMap] = fn
	obj := &struct{ vtbl *uintptr }{vtbl: &vtbl[0]}
	return uintptr(unsafe.Pointer(obj))
}

// TestCtxMapNilPointerIsError guards downstream unsafe.Slice calls even if
// a driver returns success without returning a usable mapping.
func TestCtxMapNilPointerIsError(t *testing.T) {
	ctx := fakeDeviceContext(ctxMapFailStub)
	out := d3d11MappedSubresource{pData: unsafe.Pointer(&mapSentinel)}
	err := ctxMap(ctx, 0x1234, 0, d3d11MapWriteDiscard, 0, &out)
	if err == nil {
		t.Fatal("ctxMap must fail when Map leaves pData nil (device removed)")
	}
}

func TestCtxMapReportsDeviceRemovedHRESULT(t *testing.T) {
	fn := syscall.NewCallback(func(this, resource, sub, kind, flags, out uintptr) uintptr { return 0x887a0005 })
	ctx := fakeDeviceContext(fn)
	var out d3d11MappedSubresource
	if err := ctxMap(ctx, 0x1234, 0, d3d11MapWriteDiscard, 0, &out); err == nil || !strings.Contains(err.Error(), "887a0005") {
		t.Fatalf("Map did not report DXGI_ERROR_DEVICE_REMOVED: %v", err)
	}
}

func TestDXGIPresentReportsFailure(t *testing.T) {
	vtable := make([]uintptr, slotScPresent+1)
	vtable[slotScPresent] = syscall.NewCallback(func(this, interval, flags uintptr) uintptr { return 0x887a0007 })
	obj := &struct{ vtbl *uintptr }{&vtable[0]}
	if err := scPresent(uintptr(unsafe.Pointer(obj)), 1, 0); err == nil || !strings.Contains(err.Error(), "887a0007") {
		t.Fatalf("Present did not report DXGI_ERROR_DEVICE_RESET: %v", err)
	}
}

// TestCtxMapSuccessPasses checks the guard does not reject successful maps.
func TestCtxMapSuccessPasses(t *testing.T) {
	ctx := fakeDeviceContext(ctxMapOKStub)
	out := d3d11MappedSubresource{}
	if err := ctxMap(ctx, 0x1234, 0, d3d11MapWriteDiscard, 0, &out); err != nil {
		t.Fatalf("ctxMap rejected a successful map: %v", err)
	}
	if out.pData == nil {
		t.Fatal("success stub did not fill pData")
	}
}
