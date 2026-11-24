// This file intentionally has NO build constraint: it is pure data (the
// audited COM vtable tables for the DXGI factory and DirectComposition
// interfaces the transparent-window path calls), so it compiles - and its
// unit test runs - on every platform, including the Linux CI where
// //go:build windows files are excluded.
//
// Like d3d11_slots.go, each table records the method DECLARATION ORDER from
// dxgi1_2.h / dcomp.h (vtable slot = slice index). Slot numbers must never be
// edited casually: a wrong slot silently calls a different method with wrong
// arguments.
//
// NOTE: idxgiFactory2VTable deliberately repeats inherited method names
// (GetWindowAssociation appears in both IDXGIFactory and IDXGIFactory2), so
// it is NOT covered by TestVTableNamesUnique; TestVTableSlots still pins
// every slot it uses.
package windows

// idxgiFactory2VTable is IDXGIFactory2's method order. Derivation
// (dxgi.h + dxgi1_2.h):
//
//	IUnknown          0-2  QueryInterface, AddRef, Release
//	IDXGIObject       3-6  SetPrivateData, SetPrivateDataInterface,
//	                       GetPrivateData, GetParent
//	IDXGIFactory      7-11 EnumAdapters, MakeWindowAssociation,
//	                       GetWindowAssociation, CreateSwapChain,
//	                       CreateSoftwareAdapter
//	IDXGIFactory1    12-13 EnumAdapters1, IsCurrent
//	IDXGIFactory2    14-24 GetWindowAssociation, CreateSwapChainForHwnd,
//	                       CreateSwapChainForCoreWindow,
//	                       GetSharedResourceAdapterLuid,
//	                       RegisterStereoStatusWindow,
//	                       RegisterStereoStatusEvent,
//	                       UnregisterStereoStatus,
//	                       RegisterOcclusionStatusWindow,
//	                       RegisterOcclusionStatusEvent,
//	                       UnregisterOcclusionStatus,
//	                       CreateSwapChainForComposition
var idxgiFactory2VTable = []string{
	"QueryInterface", "AddRef", "Release",
	"SetPrivateData",                // 3
	"SetPrivateDataInterface",       // 4
	"GetPrivateData",                // 5
	"GetParent",                     // 6
	"EnumAdapters",                  // 7
	"MakeWindowAssociation",         // 8
	"GetWindowAssociation",          // 9  (IDXGIFactory)
	"CreateSwapChain",               // 10
	"CreateSoftwareAdapter",         // 11
	"EnumAdapters1",                 // 12
	"IsCurrent",                     // 13
	"GetWindowAssociation",          // 14 (IDXGIFactory2 - legitimate repeat)
	"CreateSwapChainForHwnd",        // 15
	"CreateSwapChainForCoreWindow",  // 16
	"GetSharedResourceAdapterLuid",  // 17
	"RegisterStereoStatusWindow",    // 18
	"RegisterStereoStatusEvent",     // 19
	"UnregisterStereoStatus",        // 20
	"RegisterOcclusionStatusWindow", // 21
	"RegisterOcclusionStatusEvent",  // 22
	"UnregisterOcclusionStatus",     // 23
	"CreateSwapChainForComposition", // 24
}

// idCompositionDeviceVTable is IDCompositionDevice's method order (dcomp.h,
// the interface DCompositionCreateDevice returns, IID
// c37ea93a-e7aa-450d-b16f-9746cb0407d3). Derivation:
//
//	IUnknown        0-2  QueryInterface, AddRef, Release
//	(own methods)   3-22 Commit, GetFrameStatistics, CreateTargetForHwnd,
//	                     CreateVisual, CreateClip, CreateAnimation,
//	                     CreateEffectGroup, CreateTransform,
//	                     CreateTranslateTransform, CreateScaleTransform,
//	                     CreateRotateTransform, CreateSkewTransform,
//	                     CreateMatrixTransform, CreateTransformGroup,
//	                     CreateTranslateTransform3D, CreateScaleTransform3D,
//	                     CreateRotateTransform3D, CreateMatrixTransform3D,
//	                     CreateEffect, CheckDeviceState
//
// (The newer IDCompositionDevice2 inserts GetMessageNextEvent and
// WaitForMessageCompletion and shifts CreateTargetForHwnd to 7; that
// interface is NOT what DCompositionCreateDevice returns, so those methods
// are deliberately absent here.)
var idCompositionDeviceVTable = []string{
	"QueryInterface", "AddRef", "Release",
	"Commit",                     // 3
	"GetFrameStatistics",         // 4
	"CreateTargetForHwnd",        // 5
	"CreateVisual",               // 6
	"CreateClip",                 // 7
	"CreateAnimation",            // 8
	"CreateEffectGroup",          // 9
	"CreateTransform",            // 10
	"CreateTranslateTransform",   // 11
	"CreateScaleTransform",       // 12
	"CreateRotateTransform",      // 13
	"CreateSkewTransform",        // 14
	"CreateMatrixTransform",      // 15
	"CreateTransformGroup",       // 16
	"CreateTranslateTransform3D", // 17
	"CreateScaleTransform3D",     // 18
	"CreateRotateTransform3D",    // 19
	"CreateMatrixTransform3D",    // 20
	"CreateEffect",               // 21
	"CheckDeviceState",           // 22
}

// idCompositionTargetVTable is IDCompositionTarget's method order (dcomp.h:
// IUnknown 0-2, then SetRoot).
var idCompositionTargetVTable = []string{
	"QueryInterface", "AddRef", "Release",
	"SetRoot", // 3
}

// idCompositionVisualVTable is IDCompositionVisual's method order (dcomp.h).
// Derivation: the interface declares C++ OVERLOADS, and each overload
// occupies its own vtable slot in declaration order:
//
//	IUnknown                        0-2  QueryInterface, AddRef, Release
//	SetOffsetX(float)               3
//	SetOffsetY(float)               4
//	SetOffsetX(IDCompositionAnimation)  5
//	SetOffsetY(IDCompositionAnimation)  6
//	SetTransform(IDCompositionTransform)    7
//	SetTransform(IDCompositionTransform3D)  8
//	SetClip(IDCompositionClip)      9
//	SetContent(IUnknown)            10
//
// SetContent is the LAST method (slot 10). The overloads can never shift it:
// a shipped COM interface's vtable is frozen, so any overload added later
// (e.g. the Windows 8.1 IDCompositionClip2 clip) could only be appended
// after SetContent. The names below disambiguate the overloads the way the
// C-mode header macros do; the slots, not the names, are what matters.
var idCompositionVisualVTable = []string{
	"QueryInterface", "AddRef", "Release",
	"SetOffsetX",          // 3 (float)
	"SetOffsetY",          // 4 (float)
	"SetOffsetXAnimation", // 5 (IDCompositionAnimation)
	"SetOffsetYAnimation", // 6 (IDCompositionAnimation)
	"SetTransform",        // 7 (IDCompositionTransform)
	"SetTransform3D",      // 8 (IDCompositionTransform3D)
	"SetClip",             // 9
	"SetContent",          // 10
}

// Vtable slots used by the transparent-window (DirectComposition) path.
// Each constant is verified against the tables above by TestVTableSlots.
const (
	slotFactory2CreateSwapChainForComposition = 24

	slotDcompCommit              = 3
	slotDcompCreateTargetForHwnd = 5
	slotDcompCreateVisual        = 6

	slotDcompTargetSetRoot = 3

	// IDCompositionVisual: slots 3-6 are the offset/transform setters'
	// overloads; SetContent is the interface's last method.
	slotDcompVisualSetContent = 10
)
