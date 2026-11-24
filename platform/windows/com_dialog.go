//go:build windows

// File dialogs via the shell COM IFileDialog API (upstream
// Platform::prompt_for_paths / prompt_for_new_path): raw vtable COM in the
// d3d11_com.go style, no cgo. Everything is best effort - any failure
// (missing ole32.dll, COM not available, dialog cancelled) returns
// (nil, false) rather than an error, matching how the port handles
// unavailable platform facilities.
package windows

import (
	"syscall"
	"unsafe"

	"github.com/FlameInTheDark/go-gpui/platform"
)

// ---------------------------------------------------------------------------
// ole32 procedures (COM activation)
// ---------------------------------------------------------------------------

var (
	ole32 = syscall.NewLazyDLL("ole32.dll")

	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
	procCoTaskMemFree    = ole32.NewProc("CoTaskMemFree")
)

const (
	// CoInitializeEx apartment model: file dialogs want STA.
	COINIT_APARTMENTTHREADED = 0x2
	// CoCreateInstance context.
	CLSCTX_INPROC_SERVER = 0x1

	// IFileOpenDialog::GetOptions/SetOptions flags (shobjidl).
	FOS_OVERWRITEPROMPT  = 0x0002
	FOS_NOCHANGEDIR      = 0x0008
	FOS_FORCEFILESYSTEM  = 0x0040
	FOS_PICKFOLDERS      = 0x0020
	FOS_ALLOWMULTISELECT = 0x0200
	FOS_FILEMUSTEXIST    = 0x1000

	// IShellItem::GetDisplayName format.
	SIGDN_FILESYSPATH = 0x80058000
)

// GUIDs (shobjidl_core.h).
var (
	clsidFileOpenDialog = guid{
		Data1: 0xDC1C5A9C, Data2: 0xE88A, Data3: 0x4DDE,
		Data4: [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7},
	}
	clsidFileSaveDialog = guid{
		Data1: 0xC0B4E2F3, Data2: 0x053E, Data3: 0x4C80,
		Data4: [8]byte{0xAD, 0xD6, 0xAB, 0x89, 0xC7, 0xC7, 0xDA, 0x04},
	}
	iidIFileOpenDialog = guid{
		Data1: 0xD57C7288, Data2: 0xD4AD, Data3: 0x4768,
		Data4: [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60},
	}
	iidIFileSaveDialog = guid{
		Data1: 0x84BCCD23, Data2: 0x5FDE, Data3: 0x4CDB,
		Data4: [8]byte{0xAE, 0xA4, 0xAF, 0x64, 0xB8, 0x3D, 0x78, 0xAB},
	}
	// IID_IShellItemArray (shobjidl_core.idl; the riid argument of
	// IFileOpenDialog::GetResults).
	iidShellItemArray = guid{
		Data1: 0xB63EA76D, Data2: 0x1F85, Data3: 0x456F,
		Data4: [8]byte{0xA1, 0x9C, 0x48, 0x15, 0x9E, 0xFA, 0x85, 0x8B},
	}
)

// ---------------------------------------------------------------------------
// Typed vtable calls. Every slot constant is defined (and verified against
// the audited shobjidl_core.idl declaration-order tables) in shell_slots.go:
// IUnknown 0-2, IModalWindow::Show 3, IFileDialog with SetTitle 17,
// SetOkButtonLabel 18, SetFileNameLabel 19 and GetResult 20; IFileOpenDialog
// adds GetResults 27 and GetSelectedItems 28. IShellItem::GetDisplayName 5.
// IShellItemArray: GetCount 7, GetItemAt 8.
// ---------------------------------------------------------------------------

// dialogShow: Show(hwndOwner) - runs the dialog's modal message loop.
func dialogShow(dlg uintptr, hwndOwner uintptr) int32 {
	return int32(comCall(dlg, slotDlgShow, hwndOwner))
}

// dialogSetFileTypes: SetFileTypes(cFileTypes, rgFilterSpec)
// (COMDLG_FILTERSPEC pairs; reserved for future filter options).
func dialogSetFileTypes(dlg uintptr, count uint32, specs unsafe.Pointer) error {
	return comCallHRESULT(dlg, slotDlgSetFileTypes, uintptr(count), uintptr(specs))
}

// dialogSetTitle: SetTitle(pszTitle).
func dialogSetTitle(dlg uintptr, title *uint16) error {
	return comCallHRESULT(dlg, slotDlgSetTitle, uintptr(unsafe.Pointer(title)))
}

// dialogSetFileName: SetFileName(pszName) (save-dialog suggested name).
func dialogSetFileName(dlg uintptr, name *uint16) error {
	return comCallHRESULT(dlg, slotDlgSetFileName, uintptr(unsafe.Pointer(name)))
}

// dialogSetOptions: SetOptions(dwOptions).
func dialogSetOptions(dlg uintptr, options uint32) error {
	return comCallHRESULT(dlg, slotDlgSetOptions, uintptr(options))
}

// dialogGetResult: GetResult(ppsi) - the single selected shell item.
func dialogGetResult(dlg uintptr, out *uintptr) error {
	return comCallHRESULT(dlg, slotDlgGetResult, uintptr(unsafe.Pointer(out)))
}

// openDialogGetResults: GetResults(riid, ppenum) - multi-selection shell
// item array. The method takes the requested interface id (the C header's
// REFIID + void** pair; the IDL [out] alone hides it).
func openDialogGetResults(dlg uintptr, out *uintptr) error {
	return comCallHRESULT(dlg, slotOpenGetResults,
		uintptr(unsafe.Pointer(&iidShellItemArray)), uintptr(unsafe.Pointer(out)))
}

// shellItemGetDisplayName: GetDisplayName(sigdnName, ppszName); the returned
// string is COM-owned and must be CoTaskMemFree'd.
func shellItemGetDisplayName(item uintptr, sigdn uint32, out *uintptr) error {
	return comCallHRESULT(item, slotShellItemGetDisplayName, uintptr(sigdn), uintptr(unsafe.Pointer(out)))
}

// shellItemArrayGetCount: GetCount(pcelt).
func shellItemArrayGetCount(array uintptr, out *uint32) error {
	return comCallHRESULT(array, slotShellItemArrayGetCount, uintptr(unsafe.Pointer(out)))
}

// shellItemArrayGetItemAt: GetItemAt(dwIndex, ppsi).
func shellItemArrayGetItemAt(array uintptr, index uint32, out *uintptr) error {
	return comCallHRESULT(array, slotShellItemArrayGetItemAt, uintptr(index), uintptr(unsafe.Pointer(out)))
}

// coCreateInstance: CoCreateInstance(rclsid, nil, clsctx, riid, ppv).
func coCreateInstance(clsid *guid, riid *guid, out *uintptr) error {
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(clsid)), 0, CLSCTX_INPROC_SERVER,
		uintptr(unsafe.Pointer(riid)), uintptr(unsafe.Pointer(out)))
	if int32(hr) < 0 {
		return syscall.Errno(uint32(hr))
	}
	return nil
}

// ---------------------------------------------------------------------------
// Platform methods (must run on the UI thread)
// ---------------------------------------------------------------------------

// PromptForPaths shows a native open dialog and returns the selected file
// system paths (upstream prompt_for_paths). ok=false means the dialog was
// cancelled or the shell COM stack is unavailable - the port has no error
// channel here (best effort, like the other optional facilities).
//
// The dialog must run on the UI thread: callers already on it (the common
// gpui case - dialogs open from input handlers on the main thread) run it
// inline; other goroutines get it dispatched via DispatchOnMainThread and
// block on a done channel (which requires the message loop to be pumping,
// i.e. Platform.Run started).
func (p *Platform) PromptForPaths(options platform.FileDialogOptions) ([]string, bool) {
	if p.onUIThread() {
		return p.runOpenFileDialog(options)
	}
	var paths []string
	var ok bool
	done := make(chan struct{})
	p.DispatchOnMainThread(func() {
		paths, ok = p.runOpenFileDialog(options)
		close(done)
	})
	<-done
	return paths, ok
}

// PromptForPath shows a native save dialog and returns the selected path
// (upstream prompt_for_new_path). Same threading contract as
// PromptForPaths.
func (p *Platform) PromptForPath(options platform.FileDialogOptions) (string, bool) {
	if p.onUIThread() {
		return p.runSaveFileDialog(options)
	}
	var path string
	var ok bool
	done := make(chan struct{})
	p.DispatchOnMainThread(func() {
		path, ok = p.runSaveFileDialog(options)
		close(done)
	})
	<-done
	return path, ok
}

// onUIThread reports whether the calling goroutine runs on the locked UI
// thread (mainThreadID is captured in NewPlatform).
func (p *Platform) onUIThread() bool {
	tid, _, _ := procGetCurrentThreadIdSafe.Call()
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.mainThreadID != 0 && uint32(tid) == p.mainThreadID
}

// dialogOwnerHWND returns the platform's first window handle as the modal
// owner of the file dialog (0 when no window exists yet: the shell then
// shows the dialog ownerless, which is still legal). An owned dialog is
// app-modal - clicks land on the dialog, not the editor behind it - and
// opens positioned over the editor like every native app.
func (p *Platform) dialogOwnerHWND() uintptr {
	for _, w := range p.allWindows() {
		if w.hwnd != 0 {
			return uintptr(w.hwnd)
		}
	}
	return 0
}

// runOpenFileDialog drives IFileOpenDialog: CoInitializeEx, activation,
// option setup, modal Show, result extraction.
func (p *Platform) runOpenFileDialog(options platform.FileDialogOptions) ([]string, bool) {
	procCoInitializeEx.Call(0, COINIT_APARTMENTTHREADED) // S_FALSE/RPC_E_CHANGED_MODE are fine
	var dlg uintptr
	if err := coCreateInstance(&clsidFileOpenDialog, &iidIFileOpenDialog, &dlg); err != nil {
		return nil, false
	}
	defer comRelease(dlg)

	// Directories mode: folder picker (forces filesystem paths); otherwise
	// files, requiring existing entries unless the caller allows new ones
	// by combining Files+Directories (mixed selection, mirroring upstream's
	// files/directories flags).
	dialogSetOptions(dlg, openFileDialogFlags(options))
	if options.Prompt != "" {
		dialogSetTitle(dlg, utf16Ptr(options.Prompt))
	}
	hr := dialogShow(dlg, p.dialogOwnerHWND())
	if hr < 0 {
		// 0x800704C7 (ERROR_CANCELLED) means the user cancelled; every
		// other failure is equally non-fatal for the port: nil, false.
		return nil, false
	}

	if options.Multiple {
		var array uintptr
		if openDialogGetResults(dlg, &array) == nil && array != 0 {
			defer comRelease(array)
			var count uint32
			if shellItemArrayGetCount(array, &count) != nil || count == 0 {
				return nil, false
			}
			paths := make([]string, 0, count)
			for i := uint32(0); i < count; i++ {
				var item uintptr
				if shellItemArrayGetItemAt(array, i, &item) != nil || item == 0 {
					continue
				}
				if path, ok := shellItemPath(item); ok {
					paths = append(paths, path)
				}
			}
			if len(paths) == 0 {
				return nil, false
			}
			return paths, true
		}
		// Fall through to the single-result path when GetResults is
		// unavailable (older shell): still better than nothing.
	}

	var item uintptr
	if err := dialogGetResult(dlg, &item); err != nil || item == 0 {
		return nil, false
	}
	defer comRelease(item)
	path, ok := shellItemPath(item)
	if !ok {
		return nil, false
	}
	return []string{path}, true
}

// openFileDialogFlags computes FOS_* from the options (upstream passes
// files/directories/multiple booleans into the dialog the same way).
func openFileDialogFlags(options platform.FileDialogOptions) uint32 {
	flags := uint32(FOS_NOCHANGEDIR | FOS_FORCEFILESYSTEM)
	if options.Directories && !options.Files {
		flags |= FOS_PICKFOLDERS
	} else if options.Files && !options.Directories {
		flags |= FOS_FILEMUSTEXIST
	}
	if options.Multiple {
		flags |= FOS_ALLOWMULTISELECT
	}
	return flags
}

// runSaveFileDialog drives IFileSaveDialog (upstream prompt_for_new_path).
func (p *Platform) runSaveFileDialog(options platform.FileDialogOptions) (string, bool) {
	procCoInitializeEx.Call(0, COINIT_APARTMENTTHREADED)
	var dlg uintptr
	if err := coCreateInstance(&clsidFileSaveDialog, &iidIFileSaveDialog, &dlg); err != nil {
		return "", false
	}
	defer comRelease(dlg)
	dialogSetOptions(dlg, FOS_OVERWRITEPROMPT|FOS_FORCEFILESYSTEM|FOS_NOCHANGEDIR)
	if options.Prompt != "" {
		dialogSetTitle(dlg, utf16Ptr(options.Prompt))
	}
	if options.FileName != "" {
		dialogSetFileName(dlg, utf16Ptr(options.FileName))
	}
	if hr := dialogShow(dlg, p.dialogOwnerHWND()); hr < 0 {
		return "", false
	}
	var item uintptr
	if err := dialogGetResult(dlg, &item); err != nil || item == 0 {
		return "", false
	}
	defer comRelease(item)
	return shellItemPath(item)
}

// shellItemPath extracts the file system path from a shell item, freeing
// the COM-allocated string.
func shellItemPath(item uintptr) (string, bool) {
	var pwcs uintptr
	if err := shellItemGetDisplayName(item, SIGDN_FILESYSPATH, &pwcs); err != nil || pwcs == 0 {
		return "", false
	}
	defer procCoTaskMemFree.Call(pwcs)
	length := 0
	for *(*uint16)(ptrFromUintptr(pwcs + uintptr(length)*2)) != 0 {
		length++
		if length > 32768 {
			return "", false
		}
	}
	u16 := unsafe.Slice((*uint16)(ptrFromUintptr(pwcs)), length)
	return syscall.UTF16ToString(u16), true
}
