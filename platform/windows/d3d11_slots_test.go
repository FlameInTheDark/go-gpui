package windows

import "testing"

// TestVTableSlots verifies that every slot constant used by the Direct3D 11
// backend points at the method named in the audited vtable tables. The
// tables record the declaration order from d3d11.h / dxgi.h; the slice
// index IS the COM vtable slot.
//
// A wrong slot silently invokes a different method with mismatched
// arguments (crashes at best, subtle corruption at worst), so the mapping
// is locked here: editing either a constant or a table entry fails this
// test unless both change consistently.
func TestVTableSlots(t *testing.T) {
	cases := []struct {
		table []string
		slot  int
		name  string
	}{
		// ID3D11DeviceContext
		{id3d11DeviceContextVTable, slotCtxVSSetConstantBuffers, "VSSetConstantBuffers"},
		{id3d11DeviceContextVTable, slotCtxPSSetShaderResources, "PSSetShaderResources"},
		{id3d11DeviceContextVTable, slotCtxPSSetShader, "PSSetShader"},
		{id3d11DeviceContextVTable, slotCtxPSSetSamplers, "PSSetSamplers"},
		{id3d11DeviceContextVTable, slotCtxVSSetShader, "VSSetShader"},
		{id3d11DeviceContextVTable, slotCtxVSSetShaderResources, "VSSetShaderResources"},
		{id3d11DeviceContextVTable, slotCtxMap, "Map"},
		{id3d11DeviceContextVTable, slotCtxUnmap, "Unmap"},
		{id3d11DeviceContextVTable, slotCtxPSSetConstantBuffers, "PSSetConstantBuffers"},
		{id3d11DeviceContextVTable, slotCtxIASetInputLayout, "IASetInputLayout"},
		{id3d11DeviceContextVTable, slotCtxIASetVertexBuffers, "IASetVertexBuffers"},
		{id3d11DeviceContextVTable, slotCtxIASetPrimitiveTopology, "IASetPrimitiveTopology"},
		{id3d11DeviceContextVTable, slotCtxOMSetRenderTargets, "OMSetRenderTargets"},
		{id3d11DeviceContextVTable, slotCtxOMSetBlendState, "OMSetBlendState"},
		{id3d11DeviceContextVTable, slotCtxDrawInstanced, "DrawInstanced"},
		{id3d11DeviceContextVTable, slotCtxRSSetState, "RSSetState"},
		{id3d11DeviceContextVTable, slotCtxRSSetViewports, "RSSetViewports"},
		{id3d11DeviceContextVTable, slotCtxUpdateSubresource, "UpdateSubresource"},
		{id3d11DeviceContextVTable, slotCtxClearRenderTargetView, "ClearRenderTargetView"},
		{id3d11DeviceContextVTable, slotCtxClearDepthStencilView, "ClearDepthStencilView"},
		{id3d11DeviceContextVTable, slotCtxOMSetDepthStencilState, "OMSetDepthStencilState"},
		{id3d11DeviceContextVTable, slotCtxOMGetDepthStencilState, "OMGetDepthStencilState"},
		// ID3D11Device
		{id3d11DeviceVTable, slotDevCreateBuffer, "CreateBuffer"},
		{id3d11DeviceVTable, slotDevCreateTexture2D, "CreateTexture2D"},
		{id3d11DeviceVTable, slotDevCreateShaderResourceView, "CreateShaderResourceView"},
		{id3d11DeviceVTable, slotDevCreateRenderTargetView, "CreateRenderTargetView"},
		{id3d11DeviceVTable, slotDevCreateDepthStencilView, "CreateDepthStencilView"},
		{id3d11DeviceVTable, slotDevCreateInputLayout, "CreateInputLayout"},
		{id3d11DeviceVTable, slotDevCreateVertexShader, "CreateVertexShader"},
		{id3d11DeviceVTable, slotDevCreatePixelShader, "CreatePixelShader"},
		{id3d11DeviceVTable, slotDevCreateBlendState, "CreateBlendState"},
		{id3d11DeviceVTable, slotDevCreateDepthStencilState, "CreateDepthStencilState"},
		{id3d11DeviceVTable, slotDevCreateRasterizerState, "CreateRasterizerState"},
		{id3d11DeviceVTable, slotDevCreateSamplerState, "CreateSamplerState"},
		{id3d11DeviceVTable, slotDevGetDeviceRemovedReason, "GetDeviceRemovedReason"},
		// IDXGISwapChain
		{idxgiSwapChainVTable, slotScPresent, "Present"},
		{idxgiSwapChainVTable, slotScGetBuffer, "GetBuffer"},
		{idxgiSwapChainVTable, slotScResizeBuffers, "ResizeBuffers"},
		// ID3DBlob
		{id3dBlobVTable, slotBlobGetBufferPointer, "GetBufferPointer"},
		{id3dBlobVTable, slotBlobGetBufferSize, "GetBufferSize"},
		// IDXGIFactory2 (transparent windows)
		{idxgiFactory2VTable, slotFactory2CreateSwapChainForComposition, "CreateSwapChainForComposition"},
		// IDCompositionDevice / Target / Visual (transparent windows)
		{idCompositionDeviceVTable, slotDcompCommit, "Commit"},
		{idCompositionDeviceVTable, slotDcompCreateTargetForHwnd, "CreateTargetForHwnd"},
		{idCompositionDeviceVTable, slotDcompCreateVisual, "CreateVisual"},
		{idCompositionTargetVTable, slotDcompTargetSetRoot, "SetRoot"},
		{idCompositionVisualVTable, slotDcompVisualSetContent, "SetContent"},
	}
	for _, c := range cases {
		if c.slot < 0 || c.slot >= len(c.table) {
			t.Fatalf("slot %d out of range for %s", c.slot, c.name)
		}
		if got := c.table[c.slot]; got != c.name {
			t.Fatalf("vtable slot %d is %s, want %s", c.slot, got, c.name)
		}
	}
}

// TestVTableNamesUnique guards against typos in the tables (a duplicated
// name would shift every following entry). idxgiFactory2VTable is excluded:
// inherited interfaces legitimately repeat method names there (IDXGIFactory
// and IDXGIFactory2 both declare GetWindowAssociation); its used slots are
// still pinned by TestVTableSlots and its length by TestVTableAnchors.
func TestVTableNamesUnique(t *testing.T) {
	for _, table := range [][]string{
		id3d11DeviceContextVTable, id3d11DeviceVTable, idxgiSwapChainVTable, id3dBlobVTable,
		idCompositionDeviceVTable, idCompositionTargetVTable, idCompositionVisualVTable,
	} {
		seen := map[string]bool{}
		for i, name := range table {
			if seen[name] {
				t.Fatalf("duplicate method %q in vtable (index %d)", name, i)
			}
			seen[name] = true
		}
	}
	// IDXGIFactory2's full vtable is exactly 25 entries: IUnknown 0-2 +
	// IDXGIObject 3-6 + IDXGIFactory 7-11 + IDXGIFactory1 12-13 +
	// IDXGIFactory2 14-24 (CreateSwapChainForComposition last).
	if len(idxgiFactory2VTable) != 25 {
		t.Fatalf("idxgiFactory2VTable length = %d, want 25", len(idxgiFactory2VTable))
	}
}

// TestVTableAnchors pins a few well-known slot values that are widely
// cross-referenced (hooking tools, D3D10 heritage): if these move,
// everything else is suspect.
func TestVTableAnchors(t *testing.T) {
	anchors := []struct {
		table []string
		slot  int
		name  string
	}{
		{id3d11DeviceContextVTable, 0, "QueryInterface"},
		{id3d11DeviceContextVTable, 2, "Release"},
		{id3d11DeviceContextVTable, 12, "DrawIndexed"},
		{id3d11DeviceContextVTable, 13, "Draw"},
		{id3d11DeviceContextVTable, 14, "Map"},
		{id3d11DeviceContextVTable, 19, "IASetIndexBuffer"},
		{id3d11DeviceContextVTable, 22, "GSSetConstantBuffers"},
		{id3d11DeviceContextVTable, 30, "SetPredication"},
		{id3d11DeviceContextVTable, 41, "Dispatch"},
		{id3d11DeviceContextVTable, 49, "CopyStructureCount"},
		{id3d11DeviceContextVTable, 114, "FinishCommandList"},
		{idxgiSwapChainVTable, 8, "Present"},
		{idxgiSwapChainVTable, 9, "GetBuffer"},
		{id3d11DeviceVTable, 39, "GetDeviceRemovedReason"},
		// Inheritance anchors for the composition tables: the
		// factory's CreateSwapChain sits at 10 (well-known from DXGI
		// hooks), and the first inherited IDXGIFactory2 method lands
		// right after IDXGIFactory1::IsCurrent.
		{idxgiFactory2VTable, 10, "CreateSwapChain"},
		{idxgiFactory2VTable, 13, "IsCurrent"},
		{idxgiFactory2VTable, 24, "CreateSwapChainForComposition"},
		// IDCompositionDevice (v1) has no GetMessageNextEvent (that
		// is IDCompositionDevice2): CreateTargetForHwnd follows
		// GetFrameStatistics directly.
		{idCompositionDeviceVTable, 4, "GetFrameStatistics"},
		{idCompositionDeviceVTable, 5, "CreateTargetForHwnd"},
		{idCompositionDeviceVTable, 6, "CreateVisual"},
		{idCompositionDeviceVTable, 22, "CheckDeviceState"},
		// IDCompositionVisual: the offset/transform setters are C++
		// overload PAIRS, so each occupies its own slot and
		// SetContent is the interface's last method (the vtable of a
		// shipped COM interface is frozen, later overloads can only
		// be appended after it).
		{idCompositionVisualVTable, 3, "SetOffsetX"},
		{idCompositionVisualVTable, 5, "SetOffsetXAnimation"},
		{idCompositionVisualVTable, 7, "SetTransform"},
		{idCompositionVisualVTable, 9, "SetClip"},
		{idCompositionVisualVTable, 10, "SetContent"},
	}
	for _, a := range anchors {
		if got := a.table[a.slot]; got != a.name {
			t.Fatalf("anchor slot %d is %s, want %s", a.slot, got, a.name)
		}
	}
}
