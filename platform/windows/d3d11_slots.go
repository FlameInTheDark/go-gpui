// This file intentionally has NO build constraint: it is pure data (the
// audited COM vtable tables for the Direct3D 11 interfaces this backend
// calls), so it compiles - and its unit test runs - on every platform,
// including the Linux CI where //go:build windows files are excluded.
//
// The tables record the method DECLARATION ORDER from d3d11.h / dxgi.h
// (vtable slot = slice index). Slot numbers must never be edited casually:
// a wrong slot silently calls a different method with wrong arguments,
// which is exactly how the first GPU frame crashed (OMSetRenderTargets was
// invoked with the render-target-array pointer in the NumViews parameter
// and NULL in the array parameter, making D3D11 read address 0x0).
package windows

// id3d11DeviceContextVTable is ID3D11DeviceContext's method order
// (IUnknown 0-2, ID3D11DeviceChild 3-6, then the context methods).
var id3d11DeviceContextVTable = []string{
	// IUnknown
	"QueryInterface", "AddRef", "Release",
	// ID3D11DeviceChild
	"GetDevice", "GetPrivateData", "SetPrivateData", "SetPrivateDataInterface",
	// Set / draw section
	"VSSetConstantBuffers",   // 7
	"PSSetShaderResources",   // 8
	"PSSetShader",            // 9
	"PSSetSamplers",          // 10
	"VSSetShader",            // 11
	"DrawIndexed",            // 12
	"Draw",                   // 13
	"Map",                    // 14
	"Unmap",                  // 15
	"PSSetConstantBuffers",   // 16
	"IASetInputLayout",       // 17
	"IASetVertexBuffers",     // 18
	"IASetIndexBuffer",       // 19
	"DrawIndexedInstanced",   // 20
	"DrawInstanced",          // 21
	"GSSetConstantBuffers",   // 22
	"GSSetShader",            // 23
	"IASetPrimitiveTopology", // 24
	"VSSetShaderResources",   // 25
	"VSSetSamplers",          // 26
	"Begin",                  // 27
	"End",                    // 28
	"GetData",                // 29
	"SetPredication",         // 30
	"GSSetShaderResources",   // 31
	"GSSetSamplers",          // 32
	"OMSetRenderTargets",     // 33
	"OMSetRenderTargetsAndUnorderedAccessViews", // 34
	"OMSetBlendState",               // 35
	"OMSetDepthStencilState",        // 36
	"SOSetTargets",                  // 37
	"DrawAuto",                      // 38
	"DrawIndexedInstancedIndirect",  // 39
	"DrawInstancedIndirect",         // 40
	"Dispatch",                      // 41
	"DispatchIndirect",              // 42
	"RSSetState",                    // 43
	"RSSetViewports",                // 44
	"RSSetScissorRects",             // 45
	"CopySubresourceRegion",         // 46
	"CopyResource",                  // 47
	"UpdateSubresource",             // 48
	"CopyStructureCount",            // 49
	"ClearRenderTargetView",         // 50
	"ClearUnorderedAccessViewUint",  // 51
	"ClearUnorderedAccessViewFloat", // 52
	"ClearDepthStencilView",         // 53
	"GenerateMips",                  // 54
	"SetResourceMinLOD",             // 55
	"GetResourceMinLOD",             // 56
	"ResolveSubresource",            // 57
	"ExecuteCommandList",            // 58
	"HSSetShaderResources",          // 59
	"HSSetShader",                   // 60
	"HSSetSamplers",                 // 61
	"HSSetConstantBuffers",          // 62
	"DSSetShaderResources",          // 63
	"DSSetShader",                   // 64
	"DSSetSamplers",                 // 65
	"DSSetConstantBuffers",          // 66
	"CSSetShaderResources",          // 67
	"CSSetUnorderedAccessViews",     // 68
	"CSSetShader",                   // 69
	"CSSetSamplers",                 // 70
	"CSSetConstantBuffers",          // 71
	// Get section
	"VSGetConstantBuffers",                      // 72
	"PSGetShaderResources",                      // 73
	"PSGetShader",                               // 74
	"PSGetSamplers",                             // 75
	"VSGetShader",                               // 76
	"PSGetConstantBuffers",                      // 77
	"IAGetInputLayout",                          // 78
	"IAGetVertexBuffers",                        // 79
	"IAGetIndexBuffer",                          // 80
	"GSGetConstantBuffers",                      // 81
	"GSGetShader",                               // 82
	"IAGetPrimitiveTopology",                    // 83
	"VSGetShaderResources",                      // 84
	"VSGetSamplers",                             // 85
	"GetPredication",                            // 86
	"GSGetShaderResources",                      // 87
	"GSGetSamplers",                             // 88
	"OMGetRenderTargets",                        // 89
	"OMGetRenderTargetsAndUnorderedAccessViews", // 90
	"OMGetBlendState",                           // 91
	"OMGetDepthStencilState",                    // 92
	"SOGetTargets",                              // 93
	"RSGetState",                                // 94
	"RSGetViewports",                            // 95
	"RSGetScissorRects",                         // 96
	"HSGetShaderResources",                      // 97
	"HSGetShader",                               // 98
	"HSGetSamplers",                             // 99
	"HSGetConstantBuffers",                      // 100
	"DSGetShaderResources",                      // 101
	"DSGetShader",                               // 102
	"DSGetSamplers",                             // 103
	"DSGetConstantBuffers",                      // 104
	"CSGetShaderResources",                      // 105
	"CSGetUnorderedAccessViews",                 // 106
	"CSGetShader",                               // 107
	"CSGetSamplers",                             // 108
	"CSGetConstantBuffers",                      // 109
	"ClearState",                                // 110
	"Flush",                                     // 111
	"GetType",                                   // 112
	"GetContextFlags",                           // 113
	"FinishCommandList",                         // 114
}

// id3d11DeviceVTable is ID3D11Device's method order.
var id3d11DeviceVTable = []string{
	"QueryInterface", "AddRef", "Release", // IUnknown
	"CreateBuffer",                         // 3
	"CreateTexture1D",                      // 4
	"CreateTexture2D",                      // 5
	"CreateTexture3D",                      // 6
	"CreateShaderResourceView",             // 7
	"CreateUnorderedAccessView",            // 8
	"CreateRenderTargetView",               // 9
	"CreateDepthStencilView",               // 10
	"CreateInputLayout",                    // 11
	"CreateVertexShader",                   // 12
	"CreateGeometryShader",                 // 13
	"CreateGeometryShaderWithStreamOutput", // 14
	"CreatePixelShader",                    // 15
	"CreateHullShader",                     // 16
	"CreateDomainShader",                   // 17
	"CreateComputeShader",                  // 18
	"CreateClassLinkage",                   // 19
	"CreateBlendState",                     // 20
	"CreateDepthStencilState",              // 21
	"CreateRasterizerState",                // 22
	"CreateSamplerState",                   // 23
	"CreateQuery",                          // 24
	"CreatePredicate",                      // 25
	"CreateCounter",                        // 26
	"CreateDeferredContext",                // 27
	"OpenSharedResource",                   // 28
	"CheckFormatSupport",                   // 29
	"CheckMultisampleQualityLevels",        // 30
	"CheckCounterInfo",                     // 31
	"CheckCounter",                         // 32
	"CheckFeatureSupport",                  // 33
	"GetPrivateData",                       // 34
	"SetPrivateData",                       // 35
	"SetPrivateDataInterface",              // 36
	"GetFeatureLevel",                      // 37
	"GetCreationFlags",                     // 38
	"GetDeviceRemovedReason",               // 39
	"GetImmediateContext",                  // 40
	"SetExceptionMode",                     // 41
	"GetExceptionMode",                     // 42
}

// idxgiSwapChainVTable is IDXGISwapChain's method order (IUnknown 0-2,
// IDXGIObject 3-6, IDXGIDeviceSubObject 7).
var idxgiSwapChainVTable = []string{
	"QueryInterface", "AddRef", "Release",
	"SetPrivateData",          // 3
	"SetPrivateDataInterface", // 4
	"GetPrivateData",          // 5
	"GetParent",               // 6
	"GetDevice",               // 7
	"Present",                 // 8
	"GetBuffer",               // 9
	"SetFullscreenState",      // 10
	"GetFullscreenState",      // 11
	"GetDesc",                 // 12
	"ResizeBuffers",           // 13
	"ResizeTarget",            // 14
	"GetContainingOutput",     // 15
	"GetFrameStatistics",      // 16
	"GetLastPresentCount",     // 17
}

// id3dBlobVTable is ID3DBlob's method order.
var id3dBlobVTable = []string{
	"QueryInterface", "AddRef", "Release",
	"GetBufferPointer", // 3
	"GetBufferSize",    // 4
}

// Vtable slots used by the renderer. Each constant's value is verified
// against the tables above by TestVTableSlots (d3d11_slots_test.go).
const (
	slotCtxVSSetConstantBuffers   = 7
	slotCtxPSSetShaderResources   = 8
	slotCtxPSSetShader            = 9
	slotCtxPSSetSamplers          = 10
	slotCtxVSSetShader            = 11
	slotCtxMap                    = 14
	slotCtxUnmap                  = 15
	slotCtxPSSetConstantBuffers   = 16
	slotCtxIASetInputLayout       = 17
	slotCtxIASetVertexBuffers     = 18
	slotCtxIASetPrimitiveTopology = 24
	slotCtxVSSetShaderResources   = 25
	slotCtxOMSetRenderTargets     = 33
	slotCtxOMSetBlendState        = 35
	slotCtxDrawInstanced          = 21
	slotCtxRSSetState             = 43
	slotCtxRSSetViewports         = 44
	slotCtxUpdateSubresource      = 48
	slotCtxClearRenderTargetView  = 50
	slotCtxClearDepthStencilView  = 53
	slotCtxOMSetDepthStencilState = 36
	slotCtxOMGetDepthStencilState = 92

	slotDevCreateBuffer             = 3
	slotDevCreateTexture2D          = 5
	slotDevCreateShaderResourceView = 7
	slotDevCreateRenderTargetView   = 9
	slotDevCreateDepthStencilView   = 10
	slotDevCreateInputLayout        = 11
	slotDevCreateVertexShader       = 12
	slotDevCreatePixelShader        = 15
	slotDevCreateBlendState         = 20
	slotDevCreateDepthStencilState  = 21
	slotDevCreateRasterizerState    = 22
	slotDevCreateSamplerState       = 23
	slotDevGetDeviceRemovedReason   = 39

	slotScPresent       = 8
	slotScGetBuffer     = 9
	slotScResizeBuffers = 13

	slotBlobGetBufferPointer = 3
	slotBlobGetBufferSize    = 4
)
