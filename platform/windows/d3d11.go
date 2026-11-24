//go:build windows

package windows

// d3d11.go implements a Direct3D 11 renderer driven entirely through
// syscalls (no cgo), mirroring upstream gpui_wgpu's record_frame pipeline:
//
//   - an OPAQUE PRE-PASS draws every fully opaque, unshaped rectangle
//     first (no blending, depth write), so later transparent fragments that
//     were painted beneath them are depth-rejected;
//   - the frame's batches then draw in painter's order: quad batches run
//     over the full depth range (each quad writes its own depth in the
//     vertex shader), every other batch runs at the fixed depth of the quad
//     cursor at that point (viewport MinDepth == MaxDepth);
//   - paths are rasterized per-pixel on the GPU (Slug winding tiles);
//   - text sprites are contrast/gamma corrected with the upstream
//     dwrite.hlsl functions;
//   - transparent windows render into a flip-model composition swap chain
//     (DirectComposition) with premultiplied blending.
//
// The scene -> instance conversion lives in render/gpu.go so it stays
// platform independent and unit-testable; this file owns the D3D objects.

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"github.com/FlameInTheDark/go-gpui/render"
)

// ---------------------------------------------------------------------------
// DLLs and procedures
// ---------------------------------------------------------------------------

var (
	d3d11          = syscall.NewLazyDLL("d3d11.dll")
	dxgi           = syscall.NewLazyDLL("dxgi.dll")
	d3dcompiler    = syscall.NewLazyDLL("d3dcompiler_47.dll")
	procD3DCompile = func() *syscall.LazyProc {
		for _, name := range []string{"d3dcompiler_47.dll", "d3dcompiler_46.dll", "d3dcompiler_43.dll"} {
			dll := syscall.NewLazyDLL(name)
			p := dll.NewProc("D3DCompile")
			if err := p.Find(); err == nil {
				return p
			}
		}
		return d3dcompiler.NewProc("D3DCompile")
	}()

	procD3D11CreateDeviceAndSwapChain = d3d11.NewProc("D3D11CreateDeviceAndSwapChain")
	// Transparent windows need a composition swap chain, which
	// D3D11CreateDeviceAndSwapChain cannot create: the device is created
	// alone and the swap chain comes from IDXGIFactory2 below.
	procD3D11CreateDevice = d3d11.NewProc("D3D11CreateDevice")

	procCreateDXGIFactory1 = dxgi.NewProc("CreateDXGIFactory1")

	// DirectComposition (transparent windows). Missing dcomp.dll
	// (Windows 7) fails the transparent path cleanly into the software
	// fallback.
	dcomp                        = syscall.NewLazyDLL("dcomp.dll")
	procDCompositionCreateDevice = dcomp.NewProc("DCompositionCreateDevice")
)

// ---------------------------------------------------------------------------
// Constants (verified against d3d11.h / dxgi.h)
// ---------------------------------------------------------------------------

const (
	d3d11SDKVersion     = 7
	d3d11DriverHardware = 1
	d3d11DriverWarp     = 5 // D3D_DRIVER_TYPE_WARP (CPU rasterizer, RDP-safe)
	featureLevel11_0    = 0xb000

	d3d11CreateDeviceBGRA = 0x20

	dxgiFormatUnknown       = 0
	dxgiFormatR8G8B8A8UNorm = 28 // 0x1c
	dxgiFormatB8G8R8A8UNorm = 87 // 0x57
	dxgiFormatR8UNorm       = 61 // 0x3d
	// Depth attachment (upstream wgpu: Depth32Float; the native Windows
	// D3D renderer uses D32_FLOAT. No stencil is used, but D24S8 is
	// universally supported, including on WARP).
	dxgiFormatD24UNormS8Uint = 45

	dxgiUsageRenderTargetOutput = 0x20
	dxgiSwapEffectFlipDiscard   = 4
	// Composition swap chains require FLIP_SEQUENTIAL and premultiplied
	// alpha (upstream create_swap_chain_for_composition; note
	// DXGI_ALPHA_MODE_PREMULTIPLIED = 1, STRAIGHT = 2, IGNORE = 3).
	dxgiSwapEffectFlipSequential = 3
	dxgiAlphaModePremultiplied   = 1

	d3d11UsageDefault   = 0
	d3d11UsageDynamic   = 2
	d3d11CPUAccessWrite = 0x10000

	d3d11BindVertexBuffer   = 0x1
	d3d11BindShaderResource = 0x8
	d3d11BindConstantBuffer = 0x4  // D3D11_BIND_CONSTANT_BUFFER
	d3d11BindRenderTarget   = 0x20 // D3D11_BIND_RENDER_TARGET
	d3d11BindDepthStencil   = 0x40 // D3D11_BIND_DEPTH_STENCIL

	d3d11ResourceMiscBufferStructured = 0x40

	d3d11MapWriteDiscard = 4

	d3d11ClearDepth = 0x1 // D3D11_CLEAR_DEPTH

	d3d11BlendZero        = 1
	d3d11BlendOne         = 2
	d3d11BlendSrcAlpha    = 5
	d3d11BlendInvSrcAlpha = 6
	d3d11BlendOpAdd       = 1

	d3d11DepthWriteMaskZero = 0
	d3d11DepthWriteMaskAll  = 1
	// Depth convention: greater = closer (upstream CompareFunction::Greater
	// with a depth clear of 0.0). NOTE: D3D11_COMPARISON_GREATER is 5;
	// 8 is ALWAYS.
	d3d11ComparisonGreater = 5
	d3d11ComparisonAlways  = 8
	d3d11StencilOpKeep     = 1

	d3d11CullNone  = 1
	d3d11FillSolid = 3

	d3d11FilterMinMagMipLinear = 0x15
	d3d11AddressClamp          = 3

	d3d11TopologyTriangleStrip = 5

	d3d11InputPerVertexData   = 0
	d3d11InputPerInstanceData = 1

	d3d11SRVTexture2DArray = 5

	d3d11ColorWriteEnableAll = 0xf
)

// ---------------------------------------------------------------------------
// Structures (C layouts, windows/amd64)
// ---------------------------------------------------------------------------

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	iidID3D11Texture2D = guid{
		Data1: 0x6f15aaf2, Data2: 0xd208, Data3: 0x4e89,
		Data4: [8]byte{0x9a, 0xb4, 0x48, 0x95, 0x35, 0xd3, 0x4f, 0x9c},
	}
	// IID_IDXGIFactory2 (dxgi.h: 770aae78-f26f-4dba-a429-253e83a1ec2c).
	iidIDXGIFactory2 = guid{
		Data1: 0x770aae78, Data2: 0xf26f, Data3: 0x4dba,
		Data4: [8]byte{0xa4, 0x29, 0x25, 0x3e, 0x83, 0xa1, 0xec, 0x2c},
	}
	// IID_IDCompositionDevice (dcomp.h: c37ea93a-e7aa-450d-b16f-9746cb0407d3),
	// the interface DCompositionCreateDevice returns.
	iidIDCompositionDevice = guid{
		Data1: 0xc37ea93a, Data2: 0xe7aa, Data3: 0x450d,
		Data4: [8]byte{0xb1, 0x6f, 0x97, 0x46, 0xcb, 0x04, 0x07, 0xd3},
	}
)

type dxgiRational struct{ Num, Denom uint32 }

type dxgiModeDesc struct {
	Width            uint32
	Height           uint32
	RefreshRate      dxgiRational
	Format           uint32
	ScanlineOrdering uint32
	Scaling          uint32
}

type dxgiSampleDesc struct{ Count, Quality uint32 }

type dxgiSwapChainDesc struct {
	BufferDesc   dxgiModeDesc
	SampleDesc   dxgiSampleDesc
	BufferUsage  uint32
	BufferCount  uint32
	OutputWindow syscall.Handle
	Windowed     uint32
	SwapEffect   uint32
	Flags        uint32
}

// dxgiSwapChainDesc1 is DXGI_SWAP_CHAIN_DESC1 (48 bytes: all members are
// 32-bit or packed 2x32, no padding on amd64).
type dxgiSwapChainDesc1 struct {
	Width       uint32         // 0
	Height      uint32         // 4
	Format      uint32         // 8
	Stereo      uint32         // 12
	SampleDesc  dxgiSampleDesc // 16
	BufferUsage uint32         // 24
	BufferCount uint32         // 28
	Scaling     uint32         // 32 (DXGI_SCALING_STRETCH = 0)
	SwapEffect  uint32         // 36
	AlphaMode   uint32         // 40
	Flags       uint32         // 44
}

type d3d11BufferDesc struct {
	ByteWidth           uint32
	Usage               uint32
	BindFlags           uint32
	CPUAccessFlags      uint32
	MiscFlags           uint32
	StructureByteStride uint32
}

type d3d11SubresourceData struct {
	pSysMem     unsafe.Pointer
	SysMemPitch uint32
	SysMemSlice uint32
}

type d3d11Texture2DDesc struct {
	Width      uint32
	Height     uint32
	MipLevels  uint32
	ArraySize  uint32
	Format     uint32
	SampleDesc dxgiSampleDesc
	Usage      uint32
	BindFlags  uint32
	CPUAccess  uint32
	MiscFlags  uint32
}

type d3d11Tex2DArraySRV struct {
	MostDetailedMip uint32
	MipLevels       uint32
	FirstArraySlice uint32
	ArraySize       uint32
}

type d3d11SRVDesc struct {
	Format         uint32
	ViewDimension  uint32
	Texture2DArray d3d11Tex2DArraySRV
}

// srvDescFull is a Buffer-dimension SRV desc (union overlaid).
type srvDescFull struct {
	Format        uint32
	ViewDimension uint32
	FirstElement  uint32
	NumElements   uint32
}

type d3d11InputElementDesc struct {
	SemanticName      *byte
	SemanticIndex     uint32
	Format            uint32
	InputSlot         uint32
	AlignedByteOffset uint32
	InputSlotClass    uint32
	InstanceDataRate  uint32
}

type d3d11RenderTargetBlendDesc struct {
	BlendEnable           uint32
	SrcBlend              uint32
	DestBlend             uint32
	BlendOp               uint32
	SrcBlendAlpha         uint32
	DestBlendAlpha        uint32
	BlendOpAlpha          uint32
	RenderTargetWriteMask uint8
	_                     [3]byte // padding to 32 bytes
}

type d3d11BlendDesc struct {
	AlphaToCoverageEnable  uint32
	IndependentBlendEnable uint32
	// D3D11_BLEND_DESC declares RenderTarget[8]; the array is always
	// read in full (232 bytes), so a shorter Go struct would make D3D
	// read past the end of the heap allocation.
	RenderTarget [8]d3d11RenderTargetBlendDesc
}

// d3d11DepthStencilOpDesc is D3D11_DEPTH_STENCILOP_DESC (16 bytes).
type d3d11DepthStencilOpDesc struct {
	StencilFailOp      uint32
	StencilDepthFailOp uint32
	StencilPassOp      uint32
	StencilFunc        uint32
}

// d3d11DepthStencilDesc is D3D11_DEPTH_STENCIL_DESC: 4+4+4+4 (BOOL/enums) +
// 1+1 (masks) + 2 (padding) + 16 + 16 = 52 bytes (pinned by TestStructLayouts).
type d3d11DepthStencilDesc struct {
	DepthEnable      uint32
	DepthWriteMask   uint32
	DepthFunc        uint32
	StencilEnable    uint32
	StencilReadMask  uint8
	StencilWriteMask uint8
	_                uint16
	FrontFace        d3d11DepthStencilOpDesc
	BackFace         d3d11DepthStencilOpDesc
}

type d3d11SamplerDesc struct {
	Filter         uint32
	AddressU       uint32
	AddressV       uint32
	AddressW       uint32
	MipLODBias     float32
	MaxAnisotropy  uint32
	ComparisonFunc uint32
	BorderColor    [4]float32
	MinLOD         float32
	MaxLOD         float32
}

type d3d11RasterizerDesc struct {
	FillMode              uint32
	CullMode              uint32
	FrontCounterClockwise uint32
	DepthBias             int32
	DepthBiasClamp        float32
	SlopeScaledDepthBias  float32
	DepthClipEnable       uint32
	ScissorEnable         uint32
	MultisampleEnable     uint32
	AntialiasedLineEnable uint32
}

type d3d11Viewport struct {
	TopLeftX, TopLeftY, Width, Height, MinDepth, MaxDepth float32
}

type d3d11Box struct {
	Left, Top, Front, Right, Bottom, Back uint32
}

type d3d11MappedSubresource struct {
	pData      unsafe.Pointer
	RowPitch   uint32
	DepthPitch uint32
}

// gpuPathCurve is the upload form of render.PathCurve: the Go struct is
// 36 bytes (4 x Point[float32] + Sign); the HLSL PathCurve has a trailing
// pad so its stride is 40. Converted at upload time by record().
type gpuPathCurve struct {
	P0, P1, A, B [2]float32
	Sign         float32
	Pad          uint32
}

// gpuFrameConstants is the 64-byte frame constant buffer (cbuffer Frame in
// d3d11_shader.go; field offsets must match the HLSL exactly).
type gpuFrameConstants struct {
	viewportX, viewportY, premultiplied float32    // 0, 4, 8
	quadCount                           uint32     // 12
	batchBase                           uint32     // 16 quad cursor (quad batches) / instance base
	depthScale                          float32    // 20
	tileBase                            uint32     // 24 path-tile cursor
	pad0                                uint32     // 28
	gammaRatios                         [4]float32 // 32
	grayscaleContrast                   float32    // 48
	subpixelContrast                    float32    // 52
	pad1, pad2                          uint32     // 56, 60
}

// ---------------------------------------------------------------------------
// Renderer
// ---------------------------------------------------------------------------

// d3d11Renderer owns the GPU objects for one window.
type d3d11Renderer struct {
	device    uintptr // ID3D11Device
	context   uintptr // ID3D11DeviceContext
	swapChain uintptr // IDXGISwapChain(1)
	rtv       uintptr // ID3D11RenderTargetView

	// Depth buffer for the opaque pre-pass (D3D11 depth: greater=closer,
	// cleared to 0.0, GREATER compare).
	depthTex uintptr // ID3D11Texture2D (D24_UNORM_S8_UINT)
	dsv      uintptr // ID3D11DepthStencilView

	// DirectComposition objects (transparent windows only).
	dcomp       uintptr // IDCompositionDevice
	dcompTarget uintptr // IDCompositionTarget
	dcompVisual uintptr // IDCompositionVisual
	transparent bool

	vb            uintptr // ID3D11Buffer (4 corners)
	frameCB       uintptr // ID3D11Buffer (64-byte frame constants)
	instanceBuf   uintptr // ID3D11Buffer (structured, dynamic, shared)
	instanceCap   int
	instanceCount int // instances recorded for the current scene (all kinds)

	// Path buffers (structured, dynamic, per kind).
	tileBuf, tileSRV   uintptr
	tileCap            int
	curveBuf, curveSRV uintptr
	curveCap           int
	paintBuf, paintSRV uintptr
	paintCap           int

	// Frame data recorded for the last submitted scene version.
	quadCount                                               int
	depthScale                                              float32
	shadowCount, underCount, monoCount, subCount, polyCount int
	tileCount, curveCount, paintCount                       int
	batches                                                 []render.GpuBatch
	sceneVersion                                            uint64

	vs, ps         uintptr // uber pipeline
	vsOpaque, psOp uintptr // opaque pre-pass pipeline
	vsTile, psTile uintptr // path-tile pipeline
	layout         uintptr // ID3D11InputLayout (shared: identical signatures)

	blendStraight uintptr // SrcAlpha/InvSrcAlpha (opaque windows)
	blendPremult  uintptr // One/InvSrcAlpha (premultiplied windows)
	blendNone     uintptr // blending disabled (opaque pre-pass)
	depthWrite    uintptr // depth ON, write ALL (opaque pre-pass)
	depthRead     uintptr // depth ON, write ZERO (blended passes)
	raster        uintptr // ID3D11RasterizerState
	sampler       uintptr // ID3D11SamplerState
	instSRV       uintptr // SRV for instanceBuf

	// atlas mirrors: one Texture2DArray per kind (slices = atlas pages).
	monoTex, polyTex, subTex uintptr
	monoSRV, polySRV, subSRV uintptr
	monoCap, polyCap, subCap int

	// Font gamma parameters (upstream RenderingParameters).
	gamma gammaRenderingParameters

	width, height int
	semanticPOS   [8]byte // "POSITION\0"

	scratchUpload []byte
}

// rendererDrivers returns the driver types to try, in order. GPUI_RENDERER
// overrides the default: "hardware", "warp" or "gdi" (the caller checks
// the latter and never builds a GPU renderer).
func rendererDriver(name string) ([]uintptr, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "gdi", "software", "cpu":
		return nil, false
	case "warp":
		return []uintptr{d3d11DriverWarp}, true
	case "d3d11", "hardware", "gpu":
		return []uintptr{d3d11DriverHardware}, true
	default: // "" or "auto"
		return []uintptr{d3d11DriverHardware, d3d11DriverWarp}, true
	}
}

// newD3D11Renderer creates the device, swap chain and pipeline for hwnd.
// transparent selects the composition path: device + IDXGIFactory2
// CreateSwapChainForComposition + DirectComposition target/visual, with
// premultiplied alpha. It must be called from the message-pump thread (the
// swap chain's window receives messages during creation and present;
// creating or presenting while the thread is not pumping messages can
// deadlock with DWM).
func newD3D11Renderer(hwnd syscall.Handle, width, height int, transparent bool) (*d3d11Renderer, error) {
	if err := procD3D11CreateDeviceAndSwapChain.Find(); err != nil && procD3D11CreateDevice.Find() != nil {
		return nil, fmt.Errorf("d3d11.dll unavailable: %w", err)
	}
	drivers, ok := rendererDriver(os.Getenv("GPUI_RENDERER"))
	if !ok {
		return nil, fmt.Errorf("GPU rendering disabled (GPUI_RENDERER=gdi)")
	}
	var lastErr error
	for _, driver := range drivers {
		r, err := newD3D11RendererWithDriver(driver, hwnd, width, height, transparent)
		if err == nil {
			return r, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func newD3D11RendererWithDriver(driver uintptr, hwnd syscall.Handle, width, height int,
	transparent bool) (*d3d11Renderer, error) {
	r := &d3d11Renderer{
		width: width, height: height,
		transparent: transparent,
		gamma:       newGammaRenderingParameters(),
	}
	copy(r.semanticPOS[:], "POSITION")

	if transparent {
		if err := r.createCompositionSwapChain(hwnd, driver, width, height); err != nil {
			return nil, err
		}
	} else {
		sc := dxgiSwapChainDesc{
			BufferDesc:   dxgiModeDesc{Format: dxgiFormatB8G8R8A8UNorm},
			SampleDesc:   dxgiSampleDesc{Count: 1, Quality: 0},
			BufferUsage:  dxgiUsageRenderTargetOutput,
			BufferCount:  2,
			OutputWindow: hwnd,
			Windowed:     1,
			SwapEffect:   dxgiSwapEffectFlipDiscard,
		}
		var fl uint32 = featureLevel11_0
		var device, context, swapChain uintptr
		if hr, _, _ := procD3D11CreateDeviceAndSwapChain.Call(
			0, // default adapter
			driver,
			0, // no software rasterizer handle
			d3d11CreateDeviceBGRA,
			uintptr(unsafe.Pointer(&fl)), 1,
			d3d11SDKVersion,
			uintptr(unsafe.Pointer(&sc)),
			uintptr(unsafe.Pointer(&swapChain)),
			uintptr(unsafe.Pointer(&device)),
			0, // no feature level out
			uintptr(unsafe.Pointer(&context)),
		); int32(hr) < 0 {
			return nil, fmt.Errorf("d3d11: D3D11CreateDeviceAndSwapChain(driver %d) failed: 0x%08x", uint(driver), uint32(hr))
		}
		r.device, r.context, r.swapChain = device, context, swapChain
	}

	if err := r.createBackbuffer(); err != nil {
		r.Close()
		return nil, err
	}
	if err := r.createDepthBuffer(); err != nil {
		r.Close()
		return nil, err
	}
	if err := r.createPipeline(); err != nil {
		r.Close()
		return nil, err
	}
	return r, nil
}

// createCompositionSwapChain builds the transparent-window presentation
// stack (upstream create_swap_chain_for_composition + get_comp_device):
//
//	D3D11CreateDevice (no swap chain)
//	CreateDXGIFactory1(IID_IDXGIFactory2)
//	IDXGIFactory2::CreateSwapChainForComposition (FLIP_SEQUENTIAL,
//	                                             premultiplied alpha)
//	DCompositionCreateDevice -> CreateTargetForHwnd(hwnd, topmost)
//	  -> CreateVisual -> visual.SetContent(swapchain) -> target.SetRoot
//	  -> device.Commit
func (r *d3d11Renderer) createCompositionSwapChain(hwnd syscall.Handle, driver uintptr, width, height int) error {
	if err := procD3D11CreateDevice.Find(); err != nil {
		return fmt.Errorf("d3d11.dll unavailable: %w", err)
	}
	var fl uint32 = featureLevel11_0
	var device, context uintptr
	if hr, _, _ := procD3D11CreateDevice.Call(
		0, driver, 0, d3d11CreateDeviceBGRA,
		uintptr(unsafe.Pointer(&fl)), 1, d3d11SDKVersion,
		uintptr(unsafe.Pointer(&device)),
		0, // no feature level out
		uintptr(unsafe.Pointer(&context)),
	); int32(hr) < 0 {
		return fmt.Errorf("d3d11: D3D11CreateDevice(driver %d) failed: 0x%08x", uint(driver), uint32(hr))
	}
	r.device, r.context = device, context

	if err := procCreateDXGIFactory1.Find(); err != nil {
		return fmt.Errorf("dxgi.dll unavailable: %w", err)
	}
	var factory uintptr
	if hr, _, _ := procCreateDXGIFactory1.Call(
		uintptr(unsafe.Pointer(&iidIDXGIFactory2)),
		uintptr(unsafe.Pointer(&factory)),
	); int32(hr) < 0 {
		return fmt.Errorf("dxgi: CreateDXGIFactory1(IID_IDXGIFactory2) failed: 0x%08x", uint32(hr))
	}
	defer comRelease(factory)

	scd := dxgiSwapChainDesc1{
		Width:       uint32(width),
		Height:      uint32(height),
		Format:      dxgiFormatB8G8R8A8UNorm,
		SampleDesc:  dxgiSampleDesc{Count: 1, Quality: 0},
		BufferUsage: dxgiUsageRenderTargetOutput,
		BufferCount: 2,
		// Composition swap chains only support DXGI_SCALING_STRETCH (0).
		Scaling:    0,
		SwapEffect: dxgiSwapEffectFlipSequential,
		AlphaMode:  dxgiAlphaModePremultiplied,
		Flags:      0,
	}
	if err := factory2CreateSwapChainForComposition(factory, device, &scd, &r.swapChain); err != nil {
		return fmt.Errorf("dxgi: CreateSwapChainForComposition failed: %w", err)
	}

	if err := dcompCreateDevice(0, &iidIDCompositionDevice, &r.dcomp); err != nil {
		return err
	}
	// Topmost = TRUE matches upstream's composition targets (the visual
	// hosts the whole window content).
	if err := dcompCreateTargetForHwnd(r.dcomp, hwnd, true, &r.dcompTarget); err != nil {
		return fmt.Errorf("dcomp: CreateTargetForHwnd failed: %w", err)
	}
	if err := dcompCreateVisual(r.dcomp, &r.dcompVisual); err != nil {
		return fmt.Errorf("dcomp: CreateVisual failed: %w", err)
	}
	dcompVisualSetContent(r.dcompVisual, r.swapChain)
	dcompTargetSetRoot(r.dcompTarget, r.dcompVisual)
	dcompCommit(r.dcomp)
	return nil
}

func (r *d3d11Renderer) createBackbuffer() error {
	var tex uintptr
	if err := scGetBuffer(r.swapChain, 0, &iidID3D11Texture2D, &tex); err != nil {
		return err
	}
	defer comRelease(tex)
	if err := devCreateRenderTargetView(r.device, tex, nil, &r.rtv); err != nil {
		return err
	}
	return nil
}

// createDepthBuffer allocates the depth attachment for the opaque pre-pass.
// Recreated on resize together with the backbuffer.
func (r *d3d11Renderer) createDepthBuffer() error {
	desc := d3d11Texture2DDesc{
		Width: uint32(r.width), Height: uint32(r.height),
		MipLevels: 1, ArraySize: 1,
		Format:     dxgiFormatD24UNormS8Uint,
		SampleDesc: dxgiSampleDesc{Count: 1, Quality: 0},
		Usage:      d3d11UsageDefault,
		BindFlags:  d3d11BindDepthStencil,
	}
	var tex uintptr
	if err := devCreateTexture2D(r.device, &desc, nil, &tex); err != nil {
		return err
	}
	r.depthTex = tex
	// A nil desc derives the format from the (fully typed) resource.
	return devCreateDepthStencilView(r.device, tex, nil, &r.dsv)
}

func (r *d3d11Renderer) releaseDepth() {
	comRelease(r.dsv)
	comRelease(r.depthTex)
	r.dsv, r.depthTex = 0, 0
}

// d3dCompileShader compiles one shader via D3DCompile.
func d3dCompileShader(src string, entry, target string) ([]byte, error) {
	if err := procD3DCompile.Find(); err != nil {
		return nil, fmt.Errorf("d3dcompiler unavailable: %w", err)
	}
	srcBytes := []byte(src)
	entryPtr, _ := syscall.BytePtrFromString(entry)
	targetPtr, _ := syscall.BytePtrFromString(target)
	var code, errMsg uintptr
	hr, _, _ := procD3DCompile.Call(
		uintptr(unsafe.Pointer(&srcBytes[0])),
		uintptr(len(srcBytes)),
		0, 0, 0,
		uintptr(unsafe.Pointer(entryPtr)),
		uintptr(unsafe.Pointer(targetPtr)),
		0, 0,
		uintptr(unsafe.Pointer(&code)),
		uintptr(unsafe.Pointer(&errMsg)),
	)
	if int32(hr) < 0 {
		defer comRelease(errMsg)
		if errMsg != 0 {
			msg := unsafe.Slice((*byte)(blobPtr(errMsg)), blobSize(errMsg))
			return nil, fmt.Errorf("D3DCompile: %s", string(msg))
		}
		return nil, fmt.Errorf("D3DCompile failed: 0x%08x", uint32(hr))
	}
	defer comRelease(errMsg)
	defer comRelease(code)
	out := make([]byte, blobSize(code))
	copy(out, unsafe.Slice((*byte)(blobPtr(code)), len(out)))
	return out, nil
}

func (r *d3d11Renderer) createPipeline() error {
	// THREE shader pairs from ONE source (entry points verified by the
	// compiler itself: D3DCompile fails when an entry point is missing).
	vsBlob, err := d3dCompileShader(gpuShaderSource, "vsMain", "vs_5_0")
	if err != nil {
		return err
	}
	psBlob, err := d3dCompileShader(gpuShaderSource, "psMain", "ps_5_0")
	if err != nil {
		return err
	}
	vsOpaqueBlob, err := d3dCompileShader(gpuShaderSource, "vsOpaqueQuad", "vs_5_0")
	if err != nil {
		return err
	}
	psOpaqueBlob, err := d3dCompileShader(gpuShaderSource, "psOpaqueQuad", "ps_5_0")
	if err != nil {
		return err
	}
	vsTileBlob, err := d3dCompileShader(gpuShaderSource, "vsPathTile", "vs_5_0")
	if err != nil {
		return err
	}
	psTileBlob, err := d3dCompileShader(gpuShaderSource, "psPathTile", "ps_5_0")
	if err != nil {
		return err
	}
	if err := devCreateVertexShader(r.device, vsBlob, &r.vs); err != nil {
		return err
	}
	if err := devCreatePixelShader(r.device, psBlob, &r.ps); err != nil {
		return err
	}
	if err := devCreateVertexShader(r.device, vsOpaqueBlob, &r.vsOpaque); err != nil {
		return err
	}
	if err := devCreatePixelShader(r.device, psOpaqueBlob, &r.psOp); err != nil {
		return err
	}
	if err := devCreateVertexShader(r.device, vsTileBlob, &r.vsTile); err != nil {
		return err
	}
	if err := devCreatePixelShader(r.device, psTileBlob, &r.psTile); err != nil {
		return err
	}

	// Input layout: one float2 POSITION per vertex. All three vertex
	// shaders share the same input signature, so one layout serves
	// every pipeline.
	elements := [1]d3d11InputElementDesc{{
		SemanticName:      &r.semanticPOS[0],
		Format:            16, // DXGI_FORMAT_R32G32_FLOAT
		InputSlot:         0,
		AlignedByteOffset: 0,
		InputSlotClass:    d3d11InputPerVertexData,
	}}
	if err := devCreateInputLayout(r.device, &elements[0], 1, vsBlob, &r.layout); err != nil {
		return err
	}

	// Static vertex buffer: 4 corners (triangle strip).
	corners := [4][2]float32{{0, 0}, {1, 0}, {0, 1}, {1, 1}}
	vbd := d3d11BufferDesc{ByteWidth: uint32(unsafe.Sizeof(corners)), BindFlags: d3d11BindVertexBuffer}
	init := d3d11SubresourceData{pSysMem: unsafe.Pointer(&corners[0])}
	if err := devCreateBuffer(r.device, &vbd, &init, &r.vb); err != nil {
		return err
	}

	// Frame constant buffer (64 bytes, see gpuFrameConstants).
	fbd := d3d11BufferDesc{ByteWidth: uint32(unsafe.Sizeof(gpuFrameConstants{})), BindFlags: d3d11BindConstantBuffer,
		Usage: d3d11UsageDynamic, CPUAccessFlags: d3d11CPUAccessWrite}
	finit := d3d11SubresourceData{pSysMem: unsafe.Pointer(&gpuFrameConstants{})}
	if err := devCreateBuffer(r.device, &fbd, &finit, &r.frameCB); err != nil {
		return err
	}

	// Blend states. Straight alpha for opaque windows; premultiplied for
	// composition swap chains (the shader premultiplies its output when
	// the CB premultiplied flag is set, upstream blend_color); blending
	// disabled for the opaque pre-pass (upstream blend: None).
	straight := d3d11BlendDesc{}
	straight.RenderTarget[0].BlendEnable = 1
	straight.RenderTarget[0].SrcBlend = d3d11BlendSrcAlpha
	straight.RenderTarget[0].DestBlend = d3d11BlendInvSrcAlpha
	straight.RenderTarget[0].BlendOp = d3d11BlendOpAdd
	straight.RenderTarget[0].SrcBlendAlpha = d3d11BlendOne
	straight.RenderTarget[0].DestBlendAlpha = d3d11BlendInvSrcAlpha
	straight.RenderTarget[0].BlendOpAlpha = d3d11BlendOpAdd
	straight.RenderTarget[0].RenderTargetWriteMask = d3d11ColorWriteEnableAll
	if err := devCreateBlendState(r.device, &straight, &r.blendStraight); err != nil {
		return err
	}
	premult := d3d11BlendDesc{}
	premult.RenderTarget[0].BlendEnable = 1
	premult.RenderTarget[0].SrcBlend = d3d11BlendOne
	premult.RenderTarget[0].DestBlend = d3d11BlendInvSrcAlpha
	premult.RenderTarget[0].BlendOp = d3d11BlendOpAdd
	premult.RenderTarget[0].SrcBlendAlpha = d3d11BlendOne
	premult.RenderTarget[0].DestBlendAlpha = d3d11BlendInvSrcAlpha
	premult.RenderTarget[0].BlendOpAlpha = d3d11BlendOpAdd
	premult.RenderTarget[0].RenderTargetWriteMask = d3d11ColorWriteEnableAll
	if err := devCreateBlendState(r.device, &premult, &r.blendPremult); err != nil {
		return err
	}
	none := d3d11BlendDesc{}
	none.RenderTarget[0].BlendEnable = 0
	none.RenderTarget[0].SrcBlend = d3d11BlendOne
	none.RenderTarget[0].DestBlend = d3d11BlendZero
	none.RenderTarget[0].BlendOp = d3d11BlendOpAdd
	none.RenderTarget[0].SrcBlendAlpha = d3d11BlendOne
	none.RenderTarget[0].DestBlendAlpha = d3d11BlendZero
	none.RenderTarget[0].BlendOpAlpha = d3d11BlendOpAdd
	none.RenderTarget[0].RenderTargetWriteMask = d3d11ColorWriteEnableAll
	if err := devCreateBlendState(r.device, &none, &r.blendNone); err != nil {
		return err
	}

	// Depth-stencil states: the opaque pre-pass writes depth (mask ALL),
	// the blended passes only test it (mask ZERO). Both use the GREATER
	// compare with a 0.0-cleared buffer: greater = closer (upstream
	// CompareFunction::Greater). Stencil is disabled; the op descs are
	// filled with KEEP/ALWAYS like upstream create_depth_stencil_state.
	stencilOp := d3d11DepthStencilOpDesc{
		StencilFailOp:      d3d11StencilOpKeep,
		StencilDepthFailOp: d3d11StencilOpKeep,
		StencilPassOp:      d3d11StencilOpKeep,
		StencilFunc:        d3d11ComparisonAlways,
	}
	mkDepth := func(writeMask uint32) d3d11DepthStencilDesc {
		return d3d11DepthStencilDesc{
			DepthEnable:      1,
			DepthWriteMask:   writeMask,
			DepthFunc:        d3d11ComparisonGreater,
			StencilEnable:    0,
			StencilReadMask:  0xff, // D3D11_DEFAULT_STENCIL_READ_MASK
			StencilWriteMask: 0xff, // D3D11_DEFAULT_STENCIL_WRITE_MASK
			FrontFace:        stencilOp,
			BackFace:         stencilOp,
		}
	}
	writeAll := mkDepth(d3d11DepthWriteMaskAll)
	writeZero := mkDepth(d3d11DepthWriteMaskZero)
	if err := devCreateDepthStencilState(r.device, &writeAll, &r.depthWrite); err != nil {
		return err
	}
	if err := devCreateDepthStencilState(r.device, &writeZero, &r.depthRead); err != nil {
		return err
	}

	raster := d3d11RasterizerDesc{FillMode: d3d11FillSolid, CullMode: d3d11CullNone,
		DepthClipEnable: 1}
	if err := devCreateRasterizerState(r.device, &raster, &r.raster); err != nil {
		return err
	}

	sampler := d3d11SamplerDesc{Filter: d3d11FilterMinMagMipLinear,
		AddressU: d3d11AddressClamp, AddressV: d3d11AddressClamp, AddressW: d3d11AddressClamp,
		MaxLOD: 3.402823466e+38}
	if err := devCreateSamplerState(r.device, &sampler, &r.sampler); err != nil {
		return err
	}
	return nil
}

// ensureStructured grows a dynamic structured buffer (the shared growth
// pattern of ensureInstances). The buffer binds as a Buffer SRV.
func (r *d3d11Renderer) ensureStructured(count int, stride uint32, bufPtr, srvPtr *uintptr, capPtr *int) error {
	if *bufPtr != 0 && count <= *capPtr {
		return nil
	}
	cap := *capPtr * 2
	if cap < 64 {
		cap = 64
	}
	for cap < count {
		cap *= 2
	}
	comRelease(*srvPtr)
	comRelease(*bufPtr)
	*srvPtr, *bufPtr = 0, 0
	bd := d3d11BufferDesc{
		ByteWidth:           uint32(cap) * stride,
		Usage:               d3d11UsageDynamic,
		BindFlags:           d3d11BindShaderResource,
		CPUAccessFlags:      d3d11CPUAccessWrite,
		MiscFlags:           d3d11ResourceMiscBufferStructured,
		StructureByteStride: stride,
	}
	if err := devCreateBuffer(r.device, &bd, nil, bufPtr); err != nil {
		return err
	}
	// Buffer SRV: Format=UNKNOWN, Dimension=BUFFER(1), {FirstElement, NumElements}.
	full := srvDescFull{Format: dxgiFormatUnknown, ViewDimension: 1, NumElements: uint32(cap)}
	if err := devCreateShaderResourceView(r.device, *bufPtr, unsafe.Pointer(&full), srvPtr); err != nil {
		return err
	}
	*capPtr = cap
	return nil
}

// ensureInstances grows the shared instance structured buffer.
func (r *d3d11Renderer) ensureInstances(count int) error {
	return r.ensureStructured(count, uint32(len(render.GpuInstance{})*16),
		&r.instanceBuf, &r.instSRV, &r.instanceCap)
}

// ensurePathBuffers grows the path tile/curve/paint structured buffers.
func (r *d3d11Renderer) ensurePathBuffers(tiles, curves, paints int) error {
	if err := r.ensureStructured(tiles, uint32(unsafe.Sizeof(render.PathTileGpu{})),
		&r.tileBuf, &r.tileSRV, &r.tileCap); err != nil {
		return err
	}
	if err := r.ensureStructured(curves, uint32(unsafe.Sizeof(gpuPathCurve{})),
		&r.curveBuf, &r.curveSRV, &r.curveCap); err != nil {
		return err
	}
	return r.ensureStructured(paints, uint32(unsafe.Sizeof(render.PathPaintGpu{})),
		&r.paintBuf, &r.paintSRV, &r.paintCap)
}

// createArraySRV builds one Texture2DArray (slices = atlas pages) for one
// atlas kind and its shader-resource view.
func (r *d3d11Renderer) createArraySRV(count int, format uint32, texOut, srvOut *uintptr) error {
	desc := d3d11Texture2DDesc{
		Width: render.AtlasTextureSize, Height: render.AtlasTextureSize,
		MipLevels: 1, ArraySize: uint32(count),
		Format: format, SampleDesc: dxgiSampleDesc{Count: 1},
		Usage: d3d11UsageDefault, BindFlags: d3d11BindShaderResource,
	}
	if err := devCreateTexture2D(r.device, &desc, nil, texOut); err != nil {
		return err
	}
	srv := d3d11SRVDesc{Format: format, ViewDimension: d3d11SRVTexture2DArray}
	srv.Texture2DArray = d3d11Tex2DArraySRV{MipLevels: 0xffffffff, ArraySize: uint32(count)}
	return devCreateShaderResourceView(r.device, *texOut, unsafe.Pointer(&srv), srvOut)
}

// uploadAtlas mirrors dirty CPU atlas regions into the GPU texture arrays.
func (r *d3d11Renderer) uploadAtlas(atlas *render.Atlas) error {
	monoCount := atlas.TextureCount(render.AtlasMonochrome)
	polyCount := atlas.TextureCount(render.AtlasPolychrome)
	subCount := atlas.TextureCount(render.AtlasSubpixel)
	// Rebuild the mirrors whenever page counts CHANGE, not only grow: the
	// CPU atlas auto-prunes stale pages (AtlasMaxTexturesPerKind) and
	// remaps the surviving pages' indices — the GPU arrays must be rebuilt
	// wholesale or stale indices sample dropped pages (upstream evicts
	// textures in the renderers the same way).
	if monoCount != r.monoCap || polyCount != r.polyCap || subCount != r.subCap {
		if err := r.recreateAtlasArrays(monoCount, polyCount, subCount, atlas); err != nil {
			return err
		}
	}
	for _, d := range atlas.TakeDirtyRegions() {
		if d.NewTexture {
			continue // already uploaded wholesale by recreateAtlasArrays
		}
		kind := d.Kind
		data, texW, _, _, ok := atlas.Texture(kind, d.TextureIndex)
		if !ok {
			continue
		}
		tex := r.polyTex
		bpp := 4
		switch kind {
		case render.AtlasMonochrome:
			tex, bpp = r.monoTex, 1
		case render.AtlasSubpixel:
			tex = r.subTex
		}
		if tex == 0 {
			continue
		}
		// Gather the sub-rectangle rows into contiguous memory.
		n := d.W * d.H * bpp
		if len(r.scratchUpload) < n {
			r.scratchUpload = make([]byte, n)
		}
		row := 0
		for y := d.Y; y < d.Y+d.H; y++ {
			src := (y*texW + d.X) * bpp
			copy(r.scratchUpload[row:row+d.W*bpp], data[src:src+d.W*bpp])
			row += d.W * bpp
		}
		box := d3d11Box{Left: uint32(d.X), Top: uint32(d.Y), Front: 0,
			Right: uint32(d.X + d.W), Bottom: uint32(d.Y + d.H), Back: 1}
		ctxUpdateSubresource(r.context, tex, uint32(d.TextureIndex), &box,
			unsafe.Pointer(&r.scratchUpload[0]), uint32(d.W*bpp), 0)
	}
	return nil
}

// recreateAtlasArrays rebuilds the texture arrays when page counts grow and
// uploads every existing page.
func (r *d3d11Renderer) recreateAtlasArrays(monoCount, polyCount, subCount int, atlas *render.Atlas) error {
	comRelease(r.monoSRV)
	comRelease(r.polySRV)
	comRelease(r.subSRV)
	comRelease(r.monoTex)
	comRelease(r.polyTex)
	comRelease(r.subTex)
	r.monoSRV, r.polySRV, r.subSRV = 0, 0, 0
	r.monoTex, r.polyTex, r.subTex = 0, 0, 0
	r.monoCap, r.polyCap, r.subCap = monoCount, polyCount, subCount

	if monoCount > 0 {
		if err := r.createArraySRV(monoCount, dxgiFormatR8UNorm, &r.monoTex, &r.monoSRV); err != nil {
			return err
		}
	}
	if polyCount > 0 {
		if err := r.createArraySRV(polyCount, dxgiFormatR8G8B8A8UNorm, &r.polyTex, &r.polySRV); err != nil {
			return err
		}
	}
	if subCount > 0 {
		if err := r.createArraySRV(subCount, dxgiFormatR8G8B8A8UNorm, &r.subTex, &r.subSRV); err != nil {
			return err
		}
	}

	// Upload every existing page in full.
	for _, kind := range []render.AtlasKind{render.AtlasMonochrome, render.AtlasPolychrome, render.AtlasSubpixel} {
		tex := r.monoTex
		if kind == render.AtlasPolychrome {
			tex = r.polyTex
		} else if kind == render.AtlasSubpixel {
			tex = r.subTex
		}
		if tex == 0 {
			continue
		}
		for i := 0; i < atlas.TextureCount(kind); i++ {
			data, texW, _, mono, ok := atlas.Texture(kind, i)
			if !ok || len(data) == 0 {
				continue
			}
			bpp := 4
			if mono {
				bpp = 1
			}
			ctxUpdateSubresource(r.context, tex, uint32(i), nil,
				unsafe.Pointer(&data[0]), uint32(texW*bpp), 0)
		}
	}
	return nil
}

// uploadFrameCB maps the dynamic constant buffer and stores one frame's
// constants. Called once per pass/batch (the base fields change per batch).
func (r *d3d11Renderer) uploadFrameCB(c *gpuFrameConstants) error {
	var mapped d3d11MappedSubresource
	if err := ctxMap(r.context, r.frameCB, 0, d3d11MapWriteDiscard, 0, &mapped); err != nil {
		return fmt.Errorf("upload frame constants: %w", err)
	}
	*(*gpuFrameConstants)(mapped.pData) = *c
	ctxUnmap(r.context, r.frameCB, 0)
	return nil
}

// Render draws scene and presents it. The ONLY caller is the message
// pump's WM_PAINT handler: DXGI requires the calling thread to pump
// messages while presenting, so scene recording, swap chain resizes and
// the present itself all happen here, never from window-proc input
// handlers or from OpenWindow before the loop runs.
//
// Every error carries the device-removed reason when one is pending: the
// WM_PAINT fallback decision (software vs. one D3D11 recreation attempt)
// depends on knowing WHY the GPU path failed.
func (r *d3d11Renderer) Render(scene *render.Scene, version uint64, atlas *render.Atlas, width, height int, transparent bool) (err error) {
	defer func() {
		if err != nil && r.device != 0 {
			if reason := int32(comCall(r.device, slotDevGetDeviceRemovedReason)); reason < 0 {
				err = fmt.Errorf("%w; device removed reason 0x%08x", err, uint32(reason))
			}
		}
	}()
	if r.device == 0 {
		return fmt.Errorf("d3d11: device released")
	}
	if width <= 0 || height <= 0 {
		return nil
	}
	if err := r.resize(width, height); err != nil {
		return err // device lost: caller falls back to software
	}
	// GPUI reuses two scene objects. If paints are coalesced, the same
	// pointer can now describe a completely different frame.
	if version != r.sceneVersion && scene != nil {
		if err := r.record(scene, atlas); err != nil {
			return err
		}
		r.sceneVersion = version
	}

	// Pipeline state shared by every pass. The instance/path
	// StructuredBuffers are bound to BOTH the vertex and the pixel
	// stages: D3D11 resource slots are per stage, and vsMain/vsOpaque
	// read instances (quad expansion) while vsPathTile reads path tiles
	// and paints.
	ctxRSSetState(r.context, r.raster)
	ctxIASetInputLayout(r.context, r.layout)
	stride, offset := uint32(8), uint32(0) // sizeof(float2 corner)
	ctxIASetVertexBuffers(r.context, 0, 1, &r.vb, &stride, &offset)
	ctxIASetPrimitiveTopology(r.context, d3d11TopologyTriangleStrip)
	cbs := [1]uintptr{r.frameCB}
	ctxVSSetConstantBuffers(r.context, 0, 1, &cbs[0])
	ctxPSSetConstantBuffers(r.context, 0, 1, &cbs[0])
	vsSRVs := [4]uintptr{r.instSRV, r.tileSRV, r.curveSRV, r.paintSRV}
	ctxVSSetShaderResources(r.context, 0, 4, &vsSRVs[0])
	srvs := [7]uintptr{r.instSRV, r.tileSRV, r.curveSRV, r.paintSRV,
		r.monoSRV, r.polySRV, r.subSRV}
	ctxPSSetShaderResources(r.context, 0, 7, &srvs[0])
	ctxPSSetSamplers(r.context, 0, 1, &r.sampler)

	// Render target + depth. The argument ORDER of OMSetRenderTargets is
	// critical: the RTV array pointer is the SECOND parameter (passing it
	// first once made D3D11 dereference NULL).
	rtvs := [1]uintptr{r.rtv}
	ctxOMSetRenderTargetsDSV(r.context, 1, &rtvs[0], r.dsv)
	clear := [4]float32{0, 0, 0, 1}
	if transparent {
		// Premultiplied composition: transparent black.
		clear = [4]float32{0, 0, 0, 0}
	}
	ctxClearRenderTargetView(r.context, r.rtv, &clear)
	if r.dsv != 0 {
		ctxClearDepthStencilView(r.context, r.dsv, d3d11ClearDepth, 0.0, 0)
	}

	premult := float32(0)
	if transparent {
		premult = 1
	}
	base := gpuFrameConstants{
		viewportX:         float32(width),
		viewportY:         float32(height),
		premultiplied:     premult,
		quadCount:         uint32(r.quadCount),
		depthScale:        r.depthScale,
		gammaRatios:       r.gamma.gammaRatios,
		grayscaleContrast: r.gamma.grayscaleContrast,
		subpixelContrast:  r.gamma.subpixelContrast,
	}
	blendFactor := [4]float32{}
	fullVP := d3d11Viewport{Width: float32(width), Height: float32(height), MaxDepth: 1}

	// 1) Opaque pre-pass (upstream draw_instances with the opaque_quads
	// pipeline): every quad, last first; the vertex shader culls
	// non-opaque rectangles to CULLED_POSITION, writes flat colors with
	// blending disabled and depth write ON.
	ctxRSSetViewports(r.context, 1, &fullVP)
	ctxOMSetBlendState(r.context, r.blendNone, &blendFactor, 0xffffffff)
	ctxOMSetDepthStencilState(r.context, r.depthWrite, 0)
	ctxVSSetShader(r.context, r.vsOpaque)
	ctxPSSetShader(r.context, r.psOp)
	opaqueCB := base
	if err := r.uploadFrameCB(&opaqueCB); err != nil {
		return err
	}
	if r.quadCount > 0 {
		ctxDrawInstanced(r.context, 4, uint32(r.quadCount), 0, 0)
	}

	// 2) Blended batches in painter's order (upstream record_frame's
	// batch loop): quad batches run over the full depth range; every
	// other batch runs at the fixed depth of the quad cursor at that
	// point (viewport MinDepth == MaxDepth collapses all shader depths).
	blend := r.blendStraight
	if transparent {
		blend = r.blendPremult
	}
	ctxOMSetBlendState(r.context, blend, &blendFactor, 0xffffffff)
	ctxOMSetDepthStencilState(r.context, r.depthRead, 0)
	ctxVSSetShader(r.context, r.vs)
	ctxPSSetShader(r.context, r.ps)

	// Per-kind cursors index the shared instance buffer; the quads
	// occupy [0, quadCount), then shadows, underlines, mono, sub, poly.
	quadCursor, shadowCursor, underCursor := 0, 0, 0
	monoCursor, subCursor, polyCursor, tileCursor := 0, 0, 0, 0
	tilePipeline := false
	for _, b := range r.batches {
		count := int(b.Count)
		if count <= 0 {
			continue
		}
		cb := base
		if b.Kind == render.GpuBatchQuads {
			// Quad batches fetch from the quads list; batchBase is
			// also the global quad cursor for per-quad depths.
			cb.batchBase = uint32(quadCursor)
			quadCursor += count
			ctxRSSetViewports(r.context, 1, &fullVP)
			if tilePipeline {
				ctxVSSetShader(r.context, r.vs)
				ctxPSSetShader(r.context, r.ps)
				tilePipeline = false
			}
		} else {
			vp := d3d11Viewport{Width: float32(width), Height: float32(height),
				MinDepth: b.Depth, MaxDepth: b.Depth}
			ctxRSSetViewports(r.context, 1, &vp)
			if b.Kind == render.GpuBatchPaths {
				cb.batchBase = 0
				cb.tileBase = uint32(tileCursor)
				tileCursor += count
				if !tilePipeline {
					ctxVSSetShader(r.context, r.vsTile)
					ctxPSSetShader(r.context, r.psTile)
					tilePipeline = true
				}
			} else {
				if tilePipeline {
					ctxVSSetShader(r.context, r.vs)
					ctxPSSetShader(r.context, r.ps)
					tilePipeline = false
				}
				switch b.Kind {
				case render.GpuBatchShadows:
					cb.batchBase = uint32(r.quadCount + shadowCursor)
					shadowCursor += count
				case render.GpuBatchUnderlines:
					cb.batchBase = uint32(r.quadCount + r.shadowCount + underCursor)
					underCursor += count
				case render.GpuBatchMono:
					cb.batchBase = uint32(r.quadCount + r.shadowCount + r.underCount + monoCursor)
					monoCursor += count
				case render.GpuBatchSub:
					cb.batchBase = uint32(r.quadCount + r.shadowCount + r.underCount + r.monoCount + subCursor)
					subCursor += count
				case render.GpuBatchPoly:
					cb.batchBase = uint32(r.quadCount + r.shadowCount + r.underCount + r.monoCount + r.subCount + polyCursor)
					polyCursor += count
				default:
					continue
				}
			}
		}
		if err := r.uploadFrameCB(&cb); err != nil {
			return err
		}
		ctxDrawInstanced(r.context, 4, uint32(count), 0, 0)
	}

	// Present with vsync (bounded: the message loop is pumping).
	// SyncInterval 1 locks every frame to the display's refresh rate
	// (the compositor's vblank); GPUI_VSYNC=0 presents immediately for
	// benchmarking.
	if err := scPresent(r.swapChain, r.presentInterval(), 0); err != nil {
		return fmt.Errorf("present swap chain: %w", err)
	}
	return nil
}

// presentInterval resolves the Present sync interval (1 = vsync default).
func (r *d3d11Renderer) presentInterval() uint32 {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GPUI_VSYNC"))) {
	case "0", "off", "false", "no":
		return 0
	}
	return 1
}

// resetFrame drops the recorded frame data (upload failure recovery).
// Clearing the version ensures the next render records the scene again.
func (r *d3d11Renderer) resetFrame() {
	r.quadCount = 0
	r.shadowCount, r.underCount, r.monoCount, r.subCount, r.polyCount = 0, 0, 0, 0, 0
	r.tileCount, r.curveCount, r.paintCount = 0, 0, 0
	r.instanceCount = 0
	r.batches = nil
	r.sceneVersion = 0
}

// record converts the scene into the GPU frame structure and uploads every
// per-kind instance list plus the path tile/curve/paint buffers.
func (r *d3d11Renderer) record(scene *render.Scene, atlas *render.Atlas) error {
	frame := render.BuildGpuFrame(scene, atlas)
	r.quadCount = frame.QuadCount
	r.depthScale = frame.DepthScale
	r.shadowCount = len(frame.Shadows)
	r.underCount = len(frame.Underines)
	r.monoCount = len(frame.Mono)
	r.subCount = len(frame.Sub)
	r.polyCount = len(frame.Poly)
	r.tileCount = len(frame.PathTiles)
	r.curveCount = len(frame.PathCurves)
	r.paintCount = len(frame.PathPaints)
	r.batches = frame.Batches
	r.instanceCount = r.quadCount + r.shadowCount + r.underCount + r.monoCount + r.subCount + r.polyCount

	if err := r.uploadAtlas(atlas); err != nil {
		r.resetFrame()
		return err
	}

	if r.instanceCount > 0 {
		if err := r.ensureInstances(r.instanceCount); err != nil {
			r.resetFrame()
			return err
		}
		var mapped d3d11MappedSubresource
		if err := ctxMap(r.context, r.instanceBuf, 0, d3d11MapWriteDiscard, 0, &mapped); err != nil {
			r.resetFrame()
			return fmt.Errorf("upload scene instances: %w", err)
		}
		dst := unsafe.Slice((*byte)(mapped.pData), r.instanceCount*gpuInstanceStrideBytes)
		off := 0
		// Upload order MUST match the batch bases computed in Render:
		// quads, shadows, underlines, mono, sub, poly.
		for _, list := range [][]render.GpuInstance{
			frame.Quads, frame.Shadows, frame.Underines,
			frame.Mono, frame.Sub, frame.Poly,
		} {
			if len(list) == 0 {
				continue
			}
			src := unsafe.Slice((*byte)(unsafe.Pointer(&list[0])), len(list)*gpuInstanceStrideBytes)
			copy(dst[off:], src)
			off += len(src)
		}
		ctxUnmap(r.context, r.instanceBuf, 0)
	}

	if r.tileCount > 0 || r.curveCount > 0 || r.paintCount > 0 {
		if err := r.ensurePathBuffers(r.tileCount, r.curveCount, r.paintCount); err != nil {
			r.resetFrame()
			return err
		}
		if r.tileCount > 0 {
			if err := r.uploadStructured(r.tileBuf, r.tileCount,
				func(dst unsafe.Pointer) {
					tiles := unsafe.Slice((*render.PathTileGpu)(dst), r.tileCount)
					copy(tiles, frame.PathTiles)
				}); err != nil {
				r.resetFrame()
				return err
			}
		}
		if r.curveCount > 0 {
			// PathCurve needs its 4-byte pad for the shader's
			// 40-byte stride; convert while uploading.
			if err := r.uploadStructured(r.curveBuf, r.curveCount,
				func(dst unsafe.Pointer) {
					curves := unsafe.Slice((*gpuPathCurve)(dst), r.curveCount)
					for i, c := range frame.PathCurves {
						curves[i] = gpuPathCurve{
							P0:   [2]float32{c.P0.X, c.P0.Y},
							P1:   [2]float32{c.P1.X, c.P1.Y},
							A:    [2]float32{c.A.X, c.A.Y},
							B:    [2]float32{c.B.X, c.B.Y},
							Sign: c.Sign,
						}
					}
				}); err != nil {
				r.resetFrame()
				return err
			}
		}
		if r.paintCount > 0 {
			if err := r.uploadStructured(r.paintBuf, r.paintCount,
				func(dst unsafe.Pointer) {
					paints := unsafe.Slice((*render.PathPaintGpu)(dst), r.paintCount)
					copy(paints, frame.PathPaints)
				}); err != nil {
				r.resetFrame()
				return err
			}
		}
	}
	return nil
}

// uploadStructured maps a dynamic structured buffer and fills it with the
// writer callback.
func (r *d3d11Renderer) uploadStructured(buf uintptr, count int, write func(dst unsafe.Pointer)) error {
	var mapped d3d11MappedSubresource
	if err := ctxMap(r.context, buf, 0, d3d11MapWriteDiscard, 0, &mapped); err != nil {
		return err
	}
	write(mapped.pData)
	ctxUnmap(r.context, buf, 0)
	return nil
}

const gpuInstanceStrideBytes = len(render.GpuInstance{}) * 4 * 4

// resize rebuilds the back buffer and depth attachment for a new client size.
func (r *d3d11Renderer) resize(width, height int) error {
	if r.device == 0 {
		return nil
	}
	if width <= 0 || height <= 0 {
		r.width, r.height = 0, 0
		return nil
	}
	if width == r.width && height == r.height {
		return nil
	}
	// The immediate context retains its own reference to the old RTV. DXGI
	// rejects ResizeBuffers unless both that reference and ours are released.
	ctxOMSetRenderTargetsDSV(r.context, 0, nil, 0)
	comRelease(r.rtv)
	r.rtv = 0
	r.releaseDepth()
	// ResizeBuffers(0, ...) keeps the buffer count (works for composition
	// swap chains too).
	if err := scResizeBuffers(r.swapChain, 0, uint32(width), uint32(height), dxgiFormatUnknown, 0); err != nil {
		r.width, r.height = 0, 0
		return err
	}
	r.width, r.height = width, height
	if err := r.createBackbuffer(); err != nil {
		return err
	}
	return r.createDepthBuffer()
}

// Close releases every GPU object.
func (r *d3d11Renderer) Close() {
	comRelease(r.rtv)
	r.releaseDepth()
	comRelease(r.tileSRV)
	comRelease(r.tileBuf)
	comRelease(r.curveSRV)
	comRelease(r.curveBuf)
	comRelease(r.paintSRV)
	comRelease(r.paintBuf)
	comRelease(r.instSRV)
	comRelease(r.instanceBuf)
	comRelease(r.vb)
	comRelease(r.frameCB)
	comRelease(r.vs)
	comRelease(r.ps)
	comRelease(r.vsOpaque)
	comRelease(r.psOp)
	comRelease(r.vsTile)
	comRelease(r.psTile)
	comRelease(r.layout)
	comRelease(r.blendStraight)
	comRelease(r.blendPremult)
	comRelease(r.blendNone)
	comRelease(r.depthWrite)
	comRelease(r.depthRead)
	comRelease(r.raster)
	comRelease(r.sampler)
	comRelease(r.monoSRV)
	comRelease(r.polySRV)
	comRelease(r.subSRV)
	comRelease(r.monoTex)
	comRelease(r.polyTex)
	comRelease(r.subTex)
	// The composition tree must be torn down before the swap chain.
	comRelease(r.dcompVisual)
	comRelease(r.dcompTarget)
	comRelease(r.dcomp)
	comRelease(r.swapChain)
	// The immediate context is owned by the device; release it last.
	comRelease(r.context)
	comRelease(r.device)
	r.rtv = 0
	r.releaseDepth()
	r.tileSRV, r.tileBuf, r.curveSRV, r.curveBuf, r.paintSRV, r.paintBuf = 0, 0, 0, 0, 0, 0
	r.tileCap, r.curveCap, r.paintCap = 0, 0, 0
	r.instSRV, r.instanceBuf, r.vb, r.frameCB = 0, 0, 0, 0
	r.vs, r.ps, r.vsOpaque, r.psOp, r.vsTile, r.psTile = 0, 0, 0, 0, 0, 0
	r.layout = 0
	r.blendStraight, r.blendPremult, r.blendNone = 0, 0, 0
	r.depthWrite, r.depthRead, r.raster, r.sampler = 0, 0, 0, 0
	r.monoSRV, r.polySRV, r.subSRV = 0, 0, 0
	r.monoTex, r.polyTex, r.subTex = 0, 0, 0
	r.dcompVisual, r.dcompTarget, r.dcomp = 0, 0, 0
	r.instanceCap = 0
	r.resetFrame()
	r.swapChain, r.context, r.device = 0, 0, 0
}
