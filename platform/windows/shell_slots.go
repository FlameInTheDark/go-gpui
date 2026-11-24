// This file intentionally has NO build constraint: it is pure data (the
// audited COM vtable tables for the shell file-dialog interfaces this
// backend calls), so it compiles - and its unit test runs - on every
// platform, including the Linux CI where //go:build windows files are
// excluded. Same convention as d3d11_slots.go.
//
// The tables record the method DECLARATION ORDER from shobjidl_core.h
// (vtable slot = slice index). Slot numbers must never be edited casually:
// a wrong slot silently calls a different method with wrong arguments -
// IFileDialog methods take disjoint parameter shapes (UINT+pointer vs
// LPCWSTR vs FILEOPENDIALOGOPTIONS), so a wrong index is a corrupted
// call at best and a crash at worst.
//
// Derivation, for auditors (all interfaces inherit IUnknown 0-2).
// Cross-checked against the shobjidl_core.idl interface declarations
// (wine-mirror/wine include/shobjidl.idl lines 2900-3200 and the
// .NET Framework FileDialogNative.cs interop agree method-for-method):
//
//	IFileDialog   <- IModalWindow (Show) <- IUnknown:
//	  0 QueryInterface, 1 AddRef, 2 Release,
//	  3 Show,                                    (IModalWindow)
//	  4 SetFileTypes, 5 SetFileTypeIndex, 6 GetFileTypeIndex,
//	  7 Advise, 8 Unadvise,
//	  9 SetOptions, 10 GetOptions,
//	  11 SetDefaultFolder, 12 SetFolder, 13 GetFolder,
//	  14 GetCurrentSelection,
//	  15 SetFileName, 16 GetFileName,
//	  17 SetTitle, 18 SetOkButtonLabel, 19 SetFileNameLabel,
//	  20 GetResult, 21 AddPlace, 22 SetDefaultExtension,
//	  23 Close, 24 SetClientGuid, 25 ClearClientData, 26 SetFilter.
//	  (There is no GetOkButtonLabel on IFileDialog - only Set; a
//	  vtable built from the MSDN documentation sidebar instead of the
//	  declaration order puts it there and shifts GetResult to 22,
//	  which actually calls SetDefaultExtension - the dialog then shows
//	  fine but every result query silently no-ops.)
//
//	IFileOpenDialog  adds 27 GetResults, 28 GetSelectedItems.
//	IFileSaveDialog  adds 27 SetSaveAsItem, 28 SetProperties,
//	                 29 SetCollectedProperties, 30 GetProperties,
//	                 31 ApplyProperties.
//
//	IShellItem: 3 BindToHandler, 4 GetParent, 5 GetDisplayName,
//	            6 GetAttributes, 7 Compare.
//	IShellItemArray: 3 BindToHandler, 4 GetPropertyStore,
//	            5 GetPropertyDescriptionList, 6 GetAttributes,
//	            7 GetCount, 8 GetItemAt, 9 EnumItems.
package windows

// iFileDialogVTable is IFileDialog's method order (IUnknown 0-2,
// IModalWindow 3, then the dialog methods).
var iFileDialogVTable = []string{
	// IUnknown
	"QueryInterface", "AddRef", "Release",
	// IModalWindow
	"Show", // 3
	// IFileDialog
	"SetFileTypes",        // 4
	"SetFileTypeIndex",    // 5
	"GetFileTypeIndex",    // 6
	"Advise",              // 7
	"Unadvise",            // 8
	"SetOptions",          // 9
	"GetOptions",          // 10
	"SetDefaultFolder",    // 11
	"SetFolder",           // 12
	"GetFolder",           // 13
	"GetCurrentSelection", // 14
	"SetFileName",         // 15
	"GetFileName",         // 16
	"SetTitle",            // 17
	"SetOkButtonLabel",    // 18
	"SetFileNameLabel",    // 19
	"GetResult",           // 20
	"AddPlace",            // 21
	"SetDefaultExtension", // 22
	"Close",               // 23
	"SetClientGuid",       // 24
	"ClearClientData",     // 25
	"SetFilter",           // 26
}

// iFileOpenDialogVTable is IFileOpenDialog's full vtable: IFileDialog
// followed by the open-dialog additions (GetResult stays the inherited
// slot 20; GetResults/GetSelectedItems land at 27/28).
var iFileOpenDialogVTable = append(
	append([]string{}, iFileDialogVTable...),
	"GetResults",       // 27
	"GetSelectedItems", // 28
)

// iFileSaveDialogVTable is IFileSaveDialog's full vtable: IFileDialog
// followed by the save-dialog additions.
var iFileSaveDialogVTable = append(
	append([]string{}, iFileDialogVTable...),
	"SetSaveAsItem",          // 27
	"SetProperties",          // 28
	"SetCollectedProperties", // 29
	"GetProperties",          // 30
	"ApplyProperties",        // 31
)

// iShellItemVTable is IShellItem's method order.
var iShellItemVTable = []string{
	"QueryInterface", "AddRef", "Release", // IUnknown
	"BindToHandler",  // 3
	"GetParent",      // 4
	"GetDisplayName", // 5
	"GetAttributes",  // 6
	"Compare",        // 7
}

// iShellItemArrayVTable is IShellItemArray's method order.
var iShellItemArrayVTable = []string{
	"QueryInterface", "AddRef", "Release", // IUnknown
	"BindToHandler",              // 3
	"GetPropertyStore",           // 4
	"GetPropertyDescriptionList", // 5
	"GetAttributes",              // 6
	"GetCount",                   // 7
	"GetItemAt",                  // 8
	"EnumItems",                  // 9
}

// Vtable slots used by the dialog backend. Each constant's value is
// verified against the tables above by TestShellVTableSlots
// (shell_slots_test.go).
const (
	slotDlgShow         = 3  // IFileDialog::Show
	slotDlgSetFileTypes = 4  // IFileDialog::SetFileTypes
	slotDlgSetOptions   = 9  // IFileDialog::SetOptions
	slotDlgSetFileName  = 15 // IFileDialog::SetFileName (save-dialog initial name)
	slotDlgSetTitle     = 17 // IFileDialog::SetTitle
	slotDlgGetResult    = 20 // IFileDialog::GetResult (single selection)

	slotOpenGetResults       = 27 // IFileOpenDialog::GetResults (multi selection)
	slotOpenGetSelectedItems = 28 // IFileOpenDialog::GetSelectedItems (unused)

	slotShellItemGetDisplayName = 5 // IShellItem::GetDisplayName

	slotShellItemArrayGetCount  = 7 // IShellItemArray::GetCount
	slotShellItemArrayGetItemAt = 8 // IShellItemArray::GetItemAt
)
