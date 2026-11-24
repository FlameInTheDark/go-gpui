//go:build windows

package windows

import (
	"fmt"
	"math"
	"syscall"
	"unsafe"
)

// comVTable returns the vtable pointer of a COM interface.
func comVTable(iface uintptr) uintptr {
	return *(*uintptr)(unsafe.Pointer(iface))
}

// comCall invokes vtable method index with args (this passed implicitly as
// the first argument; callers do not pass it themselves).
// Native out-parameters passed as uintptr must not point into a Go stack
// that can move while preparing the syscall.
//
//go:uintptrescapes
func comCall(iface uintptr, method int, args ...uintptr) uintptr {
	vtbl := comVTable(iface)
	fn := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(method)*unsafe.Sizeof(uintptr(0))))
	r1, _, _ := syscall.SyscallN(fn, append([]uintptr{iface}, args...)...)
	return r1
}

// comCallHRESULT is comCall for methods returning HRESULT (negative = error).
//
//go:uintptrescapes
func comCallHRESULT(iface uintptr, method int, args ...uintptr) error {
	if hr := int32(comCall(iface, method, args...)); hr < 0 {
		return fmt.Errorf("d3d11: method %d failed with 0x%08x", method, uint32(hr))
	}
	return nil
}

// comRelease releases a COM interface (safe on 0).
func comRelease(iface uintptr) {
	if iface != 0 {
		comCall(iface, 2) // IUnknown::Release
	}
}

// blobSize returns an ID3DBlob's byte length.
func blobSize(b uintptr) int { return int(comCall(b, slotBlobGetBufferSize)) }

// blobPtr returns an ID3DBlob's buffer pointer.
func blobPtr(b uintptr) unsafe.Pointer { return unsafe.Pointer(comCall(b, slotBlobGetBufferPointer)) }

// ---------------------------------------------------------------------------
// Typed COM calls. The signatures mirror the d3d11.h / dxgi.h declarations
// so the compiler checks argument order and pointer widths - the class of
// bug where a raw uintptr varargs call silently swaps parameters (which
// crashed the first GPU frame: OMSetRenderTargets received the array
// pointer as NumViews and NULL as the array, faulting at address 0x0)
// cannot recur. Parameters the renderer never uses are fixed to zero.
// ---------------------------------------------------------------------------

// --- ID3D11DeviceContext ---

// ctxVSSetConstantBuffers: VSSetConstantBuffers(StartSlot, NumBuffers, ppConstantBuffers).
func ctxVSSetConstantBuffers(ctx uintptr, startSlot, numBuffers uint32, ppConstantBuffers *uintptr) {
	comCall(ctx, slotCtxVSSetConstantBuffers, uintptr(startSlot), uintptr(numBuffers),
		uintptr(unsafe.Pointer(ppConstantBuffers)))
}

// ctxPSSetConstantBuffers: PSSetConstantBuffers(StartSlot, NumBuffers, ppConstantBuffers).
func ctxPSSetConstantBuffers(ctx uintptr, startSlot, numBuffers uint32, ppConstantBuffers *uintptr) {
	comCall(ctx, slotCtxPSSetConstantBuffers, uintptr(startSlot), uintptr(numBuffers),
		uintptr(unsafe.Pointer(ppConstantBuffers)))
}

// ctxPSSetShaderResources: PSSetShaderResources(StartSlot, NumViews, ppShaderResourceViews).
func ctxPSSetShaderResources(ctx uintptr, startSlot, numViews uint32, ppShaderResourceViews *uintptr) {
	comCall(ctx, slotCtxPSSetShaderResources, uintptr(startSlot), uintptr(numViews),
		uintptr(unsafe.Pointer(ppShaderResourceViews)))
}

// ctxVSSetShaderResources: VSSetShaderResources(StartSlot, NumViews, ppShaderResourceViews).
// Shader-resource bindings are PER STAGE: the instance StructuredBuffer at
// t0 is read by vsMain (quad expansion) AND psMain (per-pixel shading), so
// the SRV must be bound to both the vertex- and pixel-shader slots.
func ctxVSSetShaderResources(ctx uintptr, startSlot, numViews uint32, ppShaderResourceViews *uintptr) {
	comCall(ctx, slotCtxVSSetShaderResources, uintptr(startSlot), uintptr(numViews),
		uintptr(unsafe.Pointer(ppShaderResourceViews)))
}

// ctxPSSetSamplers: PSSetSamplers(StartSlot, NumSamplers, ppSamplers).
func ctxPSSetSamplers(ctx uintptr, startSlot, numSamplers uint32, ppSamplers *uintptr) {
	comCall(ctx, slotCtxPSSetSamplers, uintptr(startSlot), uintptr(numSamplers),
		uintptr(unsafe.Pointer(ppSamplers)))
}

// ctxVSSetShader: VSSetShader(pVertexShader, nil, 0).
func ctxVSSetShader(ctx uintptr, vs uintptr) {
	comCall(ctx, slotCtxVSSetShader, vs, 0, 0)
}

// ctxPSSetShader: PSSetShader(pPixelShader, nil, 0).
func ctxPSSetShader(ctx uintptr, ps uintptr) {
	comCall(ctx, slotCtxPSSetShader, ps, 0, 0)
}

// ctxMap: Map(pResource, Subresource, MapType, MapFlags, pMappedResource).
// Map returns HRESULT; a successful call must also supply a writable mapping.
func ctxMap(ctx uintptr, resource uintptr, subresource uint32, mapType uint32,
	mapFlags uint32, out *d3d11MappedSubresource) error {
	*out = d3d11MappedSubresource{}
	err := comCallHRESULT(ctx, slotCtxMap, resource, uintptr(subresource),
		uintptr(mapType), uintptr(mapFlags), uintptr(unsafe.Pointer(out)))
	if err == nil && out.pData == nil {
		return fmt.Errorf("d3d11: Map succeeded but returned no data")
	}
	return err
}

// ctxUnmap: Unmap(pResource, Subresource).
func ctxUnmap(ctx uintptr, resource uintptr, subresource uint32) {
	comCall(ctx, slotCtxUnmap, resource, uintptr(subresource))
}

// ctxOMSetRenderTargets: OMSetRenderTargets(NumViews, ppRenderTargetViews, nil).
func ctxOMSetRenderTargets(ctx uintptr, numViews uint32, ppRenderTargetViews *uintptr) {
	comCall(ctx, slotCtxOMSetRenderTargets, uintptr(numViews),
		uintptr(unsafe.Pointer(ppRenderTargetViews)), 0)
}

// ctxOMSetBlendState: OMSetBlendState(pBlendState, BlendFactor, SampleMask).
func ctxOMSetBlendState(ctx uintptr, blendState uintptr, blendFactor *[4]float32, sampleMask uint32) {
	comCall(ctx, slotCtxOMSetBlendState, blendState, uintptr(unsafe.Pointer(blendFactor)),
		uintptr(sampleMask))
}

// ctxClearRenderTargetView: ClearRenderTargetView(pRenderTargetView, ColorRGBA).
func ctxClearRenderTargetView(ctx uintptr, rtv uintptr, colorRGBA *[4]float32) {
	comCall(ctx, slotCtxClearRenderTargetView, rtv, uintptr(unsafe.Pointer(colorRGBA)))
}

// ctxRSSetState: RSSetState(pRasterizerState).
func ctxRSSetState(ctx uintptr, rasterizerState uintptr) {
	comCall(ctx, slotCtxRSSetState, rasterizerState)
}

// ctxRSSetViewports: RSSetViewports(NumViewports, pViewports).
func ctxRSSetViewports(ctx uintptr, numViewports uint32, pViewports *d3d11Viewport) {
	comCall(ctx, slotCtxRSSetViewports, uintptr(numViewports), uintptr(unsafe.Pointer(pViewports)))
}

// ctxIASetInputLayout: IASetInputLayout(pInputLayout).
func ctxIASetInputLayout(ctx uintptr, inputLayout uintptr) {
	comCall(ctx, slotCtxIASetInputLayout, inputLayout)
}

// ctxIASetVertexBuffers: IASetVertexBuffers(StartSlot, NumBuffers,
// ppVertexBuffers, pStrides, pOffsets). Strides and offsets are UINT arrays
// (4 bytes each), so the pointers must reference uint32 values.
func ctxIASetVertexBuffers(ctx uintptr, startSlot, numBuffers uint32, ppVertexBuffers *uintptr,
	pStrides, pOffsets *uint32) {
	comCall(ctx, slotCtxIASetVertexBuffers, uintptr(startSlot), uintptr(numBuffers),
		uintptr(unsafe.Pointer(ppVertexBuffers)), uintptr(unsafe.Pointer(pStrides)),
		uintptr(unsafe.Pointer(pOffsets)))
}

// ctxIASetPrimitiveTopology: IASetPrimitiveTopology(Topology).
func ctxIASetPrimitiveTopology(ctx uintptr, topology uint32) {
	comCall(ctx, slotCtxIASetPrimitiveTopology, uintptr(topology))
}

// ctxDrawInstanced: DrawInstanced(VertexCountPerInstance, InstanceCount,
// StartVertexLocation, StartInstanceLocation).
func ctxDrawInstanced(ctx uintptr, vertexCountPerInstance, instanceCount,
	startVertexLocation, startInstanceLocation uint32) {
	comCall(ctx, slotCtxDrawInstanced, uintptr(vertexCountPerInstance), uintptr(instanceCount),
		uintptr(startVertexLocation), uintptr(startInstanceLocation))
}

// ctxUpdateSubresource: UpdateSubresource(pDstResource, DstSubresource,
// pDstBox, pSrcData, SrcRowPitch, SrcDepthPitch). A nil pDstBox means the
// whole subresource.
func ctxUpdateSubresource(ctx uintptr, dst uintptr, dstSubresource uint32, pDstBox *d3d11Box,
	pSrcData unsafe.Pointer, srcRowPitch, srcDepthPitch uint32) {
	comCall(ctx, slotCtxUpdateSubresource, dst, uintptr(dstSubresource),
		uintptr(unsafe.Pointer(pDstBox)), uintptr(pSrcData),
		uintptr(srcRowPitch), uintptr(srcDepthPitch))
}

// ctxOMSetRenderTargetsDSV: OMSetRenderTargets(NumViews, ppRenderTargetViews,
// pDepthStencilView). The D3D11 renderer binds a depth buffer so the opaque
// pre-pass can depth-reject blended fragments; pDSV may be 0 (no depth).
func ctxOMSetRenderTargetsDSV(ctx uintptr, numViews uint32, ppRenderTargetViews *uintptr,
	depthStencilView uintptr) {
	comCall(ctx, slotCtxOMSetRenderTargets, uintptr(numViews),
		uintptr(unsafe.Pointer(ppRenderTargetViews)), depthStencilView)
}

// ctxOMSetDepthStencilState: OMSetDepthStencilState(pDepthStencilState,
// StencilRef). The opaque pre-pass uses a write-all state; the blended
// passes use a read-only one (both GREATER compare, see d3d11.go).
func ctxOMSetDepthStencilState(ctx uintptr, depthStencilState uintptr, stencilRef uint32) {
	comCall(ctx, slotCtxOMSetDepthStencilState, depthStencilState, uintptr(stencilRef))
}

// ctxClearDepthStencilView: ClearDepthStencilView(pDepthStencilView,
// ClearFlags, Depth, Stencil). The depth buffer clears to 0.0 because the
// depth convention is "greater = closer" (upstream wgpu Clear(0.0) with
// CompareFunction::Greater).
func ctxClearDepthStencilView(ctx uintptr, dsv uintptr, clearFlags uint32, depth float32, stencil uint8) {
	comCall(ctx, slotCtxClearDepthStencilView, dsv, uintptr(clearFlags),
		uintptr(math.Float32bits(depth)), uintptr(stencil))
}

// --- ID3D11Device ---

// devCreateBuffer: CreateBuffer(pDesc, pInitData, ppBuffer).
func devCreateBuffer(dev uintptr, desc *d3d11BufferDesc, initData *d3d11SubresourceData, out *uintptr) error {
	return comCallHRESULT(dev, slotDevCreateBuffer, uintptr(unsafe.Pointer(desc)),
		uintptr(unsafe.Pointer(initData)), uintptr(unsafe.Pointer(out)))
}

// devCreateTexture2D: CreateTexture2D(pDesc, pInitData, ppTexture2D).
func devCreateTexture2D(dev uintptr, desc *d3d11Texture2DDesc, initData *d3d11SubresourceData, out *uintptr) error {
	return comCallHRESULT(dev, slotDevCreateTexture2D, uintptr(unsafe.Pointer(desc)),
		uintptr(unsafe.Pointer(initData)), uintptr(unsafe.Pointer(out)))
}

// devCreateShaderResourceView: CreateShaderResourceView(pResource, pDesc, ppView).
func devCreateShaderResourceView(dev uintptr, resource uintptr, desc unsafe.Pointer, out *uintptr) error {
	return comCallHRESULT(dev, slotDevCreateShaderResourceView, resource,
		uintptr(desc), uintptr(unsafe.Pointer(out)))
}

// devCreateRenderTargetView: CreateRenderTargetView(pResource, pDesc, ppView).
func devCreateRenderTargetView(dev uintptr, resource uintptr, desc unsafe.Pointer, out *uintptr) error {
	return comCallHRESULT(dev, slotDevCreateRenderTargetView, resource,
		uintptr(desc), uintptr(unsafe.Pointer(out)))
}

// devCreateInputLayout: CreateInputLayout(pInputElementDescs, NumElements,
// pShaderBytecodeWithInputSignature, BytecodeLength, ppInputLayout).
func devCreateInputLayout(dev uintptr, elements *d3d11InputElementDesc, numElements uint32,
	bytecode []byte, out *uintptr) error {
	return comCallHRESULT(dev, slotDevCreateInputLayout, uintptr(unsafe.Pointer(elements)),
		uintptr(numElements), uintptr(unsafe.Pointer(&bytecode[0])), uintptr(len(bytecode)),
		uintptr(unsafe.Pointer(out)))
}

// devCreateVertexShader: CreateVertexShader(pShaderBytecode, BytecodeLength, ppVertexShader).
func devCreateVertexShader(dev uintptr, bytecode []byte, out *uintptr) error {
	return comCallHRESULT(dev, slotDevCreateVertexShader, uintptr(unsafe.Pointer(&bytecode[0])),
		uintptr(len(bytecode)), 0, uintptr(unsafe.Pointer(out)))
}

// devCreatePixelShader: CreatePixelShader(pShaderBytecode, BytecodeLength, ppPixelShader).
func devCreatePixelShader(dev uintptr, bytecode []byte, out *uintptr) error {
	return comCallHRESULT(dev, slotDevCreatePixelShader, uintptr(unsafe.Pointer(&bytecode[0])),
		uintptr(len(bytecode)), 0, uintptr(unsafe.Pointer(out)))
}

// devCreateBlendState: CreateBlendState(pBlendStateDesc, ppBlendState).
// CreateBlendState takes exactly TWO arguments - passing an extra zero moved
// ppBlendState to a third parameter slot that D3D never reads, so the output
// pointer stayed NULL and OMSetBlendState(NULL) reset the pipeline to its
// DEFAULT blend state (blending disabled): every alpha<1 pixel rendered
// opaque (solid selection bars, black-square shadows, aliased text).
func devCreateBlendState(dev uintptr, desc *d3d11BlendDesc, out *uintptr) error {
	return comCallHRESULT(dev, slotDevCreateBlendState, uintptr(unsafe.Pointer(desc)),
		uintptr(unsafe.Pointer(out)))
}

// devCreateRasterizerState: CreateRasterizerState(pRasterizerStateDesc, ppRasterizerState)
// takes exactly two arguments (see devCreateBlendState).
func devCreateRasterizerState(dev uintptr, desc *d3d11RasterizerDesc, out *uintptr) error {
	return comCallHRESULT(dev, slotDevCreateRasterizerState, uintptr(unsafe.Pointer(desc)),
		uintptr(unsafe.Pointer(out)))
}

// devCreateSamplerState: CreateSamplerState(pSamplerDesc, ppSamplerState)
// takes exactly two arguments (see devCreateBlendState).
func devCreateSamplerState(dev uintptr, desc *d3d11SamplerDesc, out *uintptr) error {
	return comCallHRESULT(dev, slotDevCreateSamplerState, uintptr(unsafe.Pointer(desc)),
		uintptr(unsafe.Pointer(out)))
}

// devCreateDepthStencilView: CreateDepthStencilView(pResource, pDesc,
// ppDepthStencilView). A nil pDesc derives format/mip from the resource.
func devCreateDepthStencilView(dev uintptr, resource uintptr, desc unsafe.Pointer, out *uintptr) error {
	return comCallHRESULT(dev, slotDevCreateDepthStencilView, resource,
		uintptr(desc), uintptr(unsafe.Pointer(out)))
}

// devCreateDepthStencilState: CreateDepthStencilState(pDepthStencilDesc,
// ppDepthStencilState) takes exactly two arguments (see devCreateBlendState).
func devCreateDepthStencilState(dev uintptr, desc *d3d11DepthStencilDesc, out *uintptr) error {
	return comCallHRESULT(dev, slotDevCreateDepthStencilState, uintptr(unsafe.Pointer(desc)),
		uintptr(unsafe.Pointer(out)))
}

// --- IDXGISwapChain ---

// scPresent: Present(SyncInterval, Flags).
func scPresent(sc uintptr, syncInterval, flags uint32) error {
	return comCallHRESULT(sc, slotScPresent, uintptr(syncInterval), uintptr(flags))
}

// scGetBuffer: GetBuffer(Buffer, riid, ppSurface).
func scGetBuffer(sc uintptr, buffer uint32, riid *guid, out *uintptr) error {
	return comCallHRESULT(sc, slotScGetBuffer, uintptr(buffer),
		uintptr(unsafe.Pointer(riid)), uintptr(unsafe.Pointer(out)))
}

// scResizeBuffers: ResizeBuffers(BufferCount, Width, Height, NewFormat, SwapChainFlags).
func scResizeBuffers(sc uintptr, bufferCount, width, height uint32, format uint32, flags uint32) error {
	return comCallHRESULT(sc, slotScResizeBuffers, uintptr(bufferCount), uintptr(width),
		uintptr(height), uintptr(format), uintptr(flags))
}

// --- IDXGIFactory2 (transparent windows) ---

// factory2CreateSwapChainForComposition:
// CreateSwapChainForComposition(pDevice, pDesc, ppSwapChain). The device is
// passed as the IUnknown of an ID3D11Device; the returned swap chain is an
// IDXGISwapChain1, usable through the plain IDXGISwapChain slots above.
func factory2CreateSwapChainForComposition(factory uintptr, device uintptr,
	desc *dxgiSwapChainDesc1, out *uintptr) error {
	return comCallHRESULT(factory, slotFactory2CreateSwapChainForComposition,
		device, uintptr(unsafe.Pointer(desc)), uintptr(unsafe.Pointer(out)))
}

// --- IDCompositionDevice / Target / Visual (dcomp.dll, transparent windows) ---

// dcompCreateDevice: DCompositionCreateDevice(pDXGIDevice, riid, ppDevice).
// This is a dcomp.dll EXPORT (not a COM method), so it is invoked through
// the DLL proc, not a vtable call. A nil dxgiDevice makes dcomp create its
// own DXGI device; the composition tree only needs the swap chain, which is
// set as the visual's content.
func dcompCreateDevice(dxgiDevice uintptr, riid *guid, out *uintptr) error {
	if err := procDCompositionCreateDevice.Find(); err != nil {
		return fmt.Errorf("dcomp.dll unavailable: %w", err)
	}
	hr, _, _ := procDCompositionCreateDevice.Call(dxgiDevice,
		uintptr(unsafe.Pointer(riid)), uintptr(unsafe.Pointer(out)))
	if int32(hr) < 0 {
		return fmt.Errorf("DCompositionCreateDevice failed: 0x%08x", uint32(hr))
	}
	return nil
}

// dcompCommit: IDCompositionDevice::Commit().
func dcompCommit(dev uintptr) {
	comCall(dev, slotDcompCommit)
}

// dcompCreateTargetForHwnd: IDCompositionDevice::CreateTargetForHwnd(hwnd,
// Topmost, ppTarget). The target binds the composition tree to the window.
func dcompCreateTargetForHwnd(dev uintptr, hwnd syscall.Handle, topmost bool, out *uintptr) error {
	var top uint32
	if topmost {
		top = 1
	}
	return comCallHRESULT(dev, slotDcompCreateTargetForHwnd, uintptr(hwnd), uintptr(top),
		uintptr(unsafe.Pointer(out)))
}

// dcompCreateVisual: IDCompositionDevice::CreateVisual(ppVisual).
func dcompCreateVisual(dev uintptr, out *uintptr) error {
	return comCallHRESULT(dev, slotDcompCreateVisual, uintptr(unsafe.Pointer(out)))
}

// dcompTargetSetRoot: IDCompositionTarget::SetRoot(pVisual).
func dcompTargetSetRoot(target uintptr, visual uintptr) {
	comCall(target, slotDcompTargetSetRoot, visual)
}

// dcompVisualSetContent: IDCompositionVisual::SetContent(pContent). The
// content is the swap chain (IUnknown).
func dcompVisualSetContent(visual uintptr, content uintptr) {
	comCall(visual, slotDcompVisualSetContent, content)
}
