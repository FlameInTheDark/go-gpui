//go:build windows

package windows

import (
	"testing"
	"unsafe"

	"github.com/FlameInTheDark/go-gpui/render"
)

// TestStructLayouts pins the C ABI sizes/offsets of the D3D11 structs this
// backend passes across the syscall boundary. If a field is added or
// reordered, the size check fails here instead of corrupting memory in a
// native call.
func TestStructLayouts(t *testing.T) {
	if got := unsafe.Sizeof(d3d11BlendDesc{}); got != 264 {
		t.Errorf("d3d11BlendDesc size = %d, want 264 (D3D11_BLEND_DESC: 2 BOOLs + RenderTarget[8]*32)", got)
	}
	if got := unsafe.Sizeof(d3d11RenderTargetBlendDesc{}); got != 32 {
		t.Errorf("d3d11RenderTargetBlendDesc size = %d, want 32", got)
	}
	if got := unsafe.Sizeof(d3d11RasterizerDesc{}); got != 40 {
		t.Errorf("d3d11RasterizerDesc size = %d, want 40", got)
	}
	if got := unsafe.Sizeof(d3d11SamplerDesc{}); got != 52 {
		t.Errorf("d3d11SamplerDesc size = %d, want 52", got)
	}
	if got := unsafe.Sizeof(d3d11InputElementDesc{}); got != 32 {
		t.Errorf("d3d11InputElementDesc size = %d, want 32", got)
	}
	if got := unsafe.Sizeof(d3d11Viewport{}); got != 24 {
		t.Errorf("d3d11Viewport size = %d, want 24", got)
	}
}

// TestNewStructLayouts pins the structs added for the depth pre-pass, GPU
// path tiles and transparent windows. D3D11_DEPTH_STENCIL_DESC:
// BOOL DepthEnable + D3D11_DEPTH_WRITE_MASK + D3D11_COMPARISON_FUNC +
// BOOL StencilEnable = 16, UINT8 masks + 2 bytes padding = 4, two
// D3D11_DEPTH_STENCILOP_DESC (4 UINTs) = 32 -> 52 bytes total.
func TestNewStructLayouts(t *testing.T) {
	if got := unsafe.Sizeof(d3d11DepthStencilOpDesc{}); got != 16 {
		t.Errorf("d3d11DepthStencilOpDesc size = %d, want 16", got)
	}
	if got := unsafe.Sizeof(d3d11DepthStencilDesc{}); got != 52 {
		t.Errorf("d3d11DepthStencilDesc size = %d, want 52 (4+4+4+4 + 1+1+2pad + 16+16)", got)
	}
	// Field offsets inside D3D11_DEPTH_STENCIL_DESC.
	ds := d3d11DepthStencilDesc{}
	if off := unsafe.Offsetof(ds.FrontFace); off != 20 {
		t.Errorf("d3d11DepthStencilDesc.FrontFace offset = %d, want 20", off)
	}
	if got := unsafe.Sizeof(dxgiSwapChainDesc1{}); got != 48 {
		t.Errorf("dxgiSwapChainDesc1 size = %d, want 48", got)
	}
	sw := dxgiSwapChainDesc1{}
	if off := unsafe.Offsetof(sw.AlphaMode); off != 40 {
		t.Errorf("dxgiSwapChainDesc1.AlphaMode offset = %d, want 40", off)
	}
	// The frame constant buffer must be exactly 64 bytes (cbuffer Frame
	// in the HLSL packs the same fields at the same offsets).
	if got := unsafe.Sizeof(gpuFrameConstants{}); got != 64 {
		t.Errorf("gpuFrameConstants size = %d, want 64", got)
	}
	cb := gpuFrameConstants{}
	if off := unsafe.Offsetof(cb.gammaRatios); off != 32 {
		t.Errorf("gpuFrameConstants.gammaRatios offset = %d, want 32", off)
	}
	if off := unsafe.Offsetof(cb.grayscaleContrast); off != 48 {
		t.Errorf("gpuFrameConstants.grayscaleContrast offset = %d, want 48", off)
	}
	// StructuredBuffer strides must match the HLSL structs exactly
	// (see the layout contract in d3d11_shader.go).
	if got := unsafe.Sizeof(render.GpuInstance{}); got != 240 {
		t.Errorf("render.GpuInstance size = %d, want 240", got)
	}
	// PathTileGpu: uint4 header + 6 float2 rows, padded to a 16-byte
	// multiple (64) so the shader's PathTile stride matches.
	if got := unsafe.Sizeof(render.PathTileGpu{}); got != 64 {
		t.Errorf("render.PathTileGpu size = %d, want 64", got)
	}
	tile := render.PathTileGpu{}
	if off := unsafe.Offsetof(tile.TopLeft); off != 16 {
		t.Errorf("render.PathTileGpu.TopLeft offset = %d, want 16", off)
	}
	if off := unsafe.Offsetof(tile.CurvesBotRight); off != 48 {
		t.Errorf("render.PathTileGpu.CurvesBotRight offset = %d, want 48", off)
	}
	// PathPaintGpu: 4 float4 rows + 6 scalars = 88 (HLSL PathPaint).
	if got := unsafe.Sizeof(render.PathPaintGpu{}); got != 88 {
		t.Errorf("render.PathPaintGpu size = %d, want 88", got)
	}
	paint := render.PathPaintGpu{}
	if off := unsafe.Offsetof(paint.Tag); off != 64 {
		t.Errorf("render.PathPaintGpu.Tag offset = %d, want 64", off)
	}
	// The upload form of PathCurve (36-byte Go struct + 4-byte pad).
	if got := unsafe.Sizeof(gpuPathCurve{}); got != 40 {
		t.Errorf("gpuPathCurve size = %d, want 40", got)
	}
	curve := gpuPathCurve{}
	if off := unsafe.Offsetof(curve.Sign); off != 32 {
		t.Errorf("gpuPathCurve.Sign offset = %d, want 32", off)
	}
	// Structured strides must stay multiples of 16 where the task
	// requires it (tiles) and of 4 everywhere (D3D11 requirement).
	for _, stride := range []uintptr{
		unsafe.Sizeof(render.GpuInstance{}), unsafe.Sizeof(render.PathTileGpu{}),
		unsafe.Sizeof(render.PathPaintGpu{}), unsafe.Sizeof(gpuPathCurve{}),
	} {
		if stride%4 != 0 {
			t.Errorf("structured stride %d is not a multiple of 4", stride)
		}
	}
	if unsafe.Sizeof(render.PathTileGpu{})%16 != 0 {
		t.Error("PathTileGpu stride must be a multiple of 16")
	}
}
