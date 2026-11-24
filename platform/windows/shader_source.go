// This file intentionally has NO build constraint: it is pure data (the HLSL
// shader source string the Direct3D 11 backend compiles at runtime with
// D3DCompile), so it compiles - and the static shader audit test in
// shader_audit_test.go runs - on every platform, including the Linux CI where
// //go:build windows files are excluded, like d3d11_slots.go.
package windows

// gpuShaderSource is the HLSL port of the upstream GPU shaders
// (gpui_wgpu/src/shaders.wgsl + gpui_windows/src/shaders.hlsl), compiled at
// runtime with D3DCompile. One compilation unit provides THREE shader pairs:
//
//	vsMain / psMain        - the uber pipeline: quads (SDF corners/borders,
//	                         gradients, dash borders), shadows, underlines,
//	                         mono/subpixel/poly sprites (gamma corrected);
//	vsOpaqueQuad/psOpaqueQuad - the OPAQUE PRE-PASS: flat unshaped opaque
//	                         rectangles, no blending, depth write;
//	vsPathTile/psPathTile  - GPU path rasterization (Slug per-pixel winding).
//
// Layout contract with render/gpu.go (BuildGpuFrame):
//
//	Instance    = render.GpuInstance      (15 float4 rows, 240 bytes)
//	PathTile    = render.PathTileGpu      (64 bytes incl. 8-byte pad)
//	PathCurve   = render.PathCurve        (40 bytes incl. 4-byte pad;
//	                                         converted at upload, d3d11.go)
//	PathPaint   = render.PathPaintGpu     (88 bytes)
//
// Stride notes: D3D11 structured-buffer strides only need to be multiples of
// 4 bytes (StructuredBuffer<uint> has stride 4), so the exact upstream WGSL
// strides are reproduced here: PathCurve is 36 useful bytes + the upstream
// `pad: u32` = 40; PathTileGpu's Go-side pad keeps that upload struct on a
// 16-byte boundary (64), which the HLSL struct mirrors.
//
// All StructuredBuffer strides must match the Go structs EXACTLY - the sizes
// are pinned by TestStructLayouts (d3d11_layout_test.go).
const gpuShaderSource = `
struct Instance {
    float4 bounds;        // 0  x, y, w, h (device px)
    float4 contentMask;   // 1  x0, y0, x1, y1
    float4 radii;         // 2  tl, tr, br, bl
    float4 borderWidths;  // 3  top, right, bottom, left
    float4 color0;        // 4  background / gradient stop 0
    float4 color1;        // 5  gradient stop 1
    float4 borderColor;   // 6
    float4 shapeBounds;   // 7  shadow shape
    float4 shapeRadii;    // 8  shadow shape radii
    float4 uvMat0;        // 9  u = a*px + b*py + c
    float4 uvMat1;        // 10 v = d*px + e*py + f
    float4 misc;          // 11 bgTag, angleParam, blurRadius, inset
    float4 misc2;         // 12 primType, borderStyle, thickness, texKind
    float4 misc3;         // 13 texIndex, opacity, clipRounded, grayscale
    float4 misc4;         // 14 colorSpace, pct0, pct1, unused
};

// render.PathTileGpu: uint4 header + 6 float2 + float2 pad = 64 bytes.
struct PathTile {
    uint4  header;             // paint, curve_start, curve_count, backdrop(int)
    float2 top_left;           // 16
    float2 bottom_right;       // 24
    float2 reference;          // 32
    float2 curves_top_left;    // 40
    float2 curves_bottom_right;// 48
    float2 pad;                // 56 -> stride 64
};

// gpuPathCurve (converted from render.PathCurve at upload): 40 bytes.
struct PathCurve {
    float2 p0;
    float2 p1;
    float2 a;   // point(t) = a*t*t + b*t + p0, monotone downward
    float2 b;
    float  sign; // winding direction
    uint   pad;  // -> stride 40
};

// render.PathPaintGpu: 88 bytes.
struct PathPaint {
    float4 bounds;       // origin x,y + size w,h
    float4 contentMask;  // x0, y0, x1, y1
    float4 color0;       // prepared gradient stop 0
    float4 color1;       // prepared gradient stop 1
    uint   tag;          // 0 solid, 1 gradient, 2 slash, 3 checkerboard
    uint   colorSpace;   // 0 sRGB, 1 Oklab
    float  pct0;
    float  pct1;
    float  angle;
    uint   evenOdd;
};

// Resource plan (slots are per stage in D3D11):
//   t0 instances (VS+PS)     t1 path tiles (VS+PS)
//   t2 path curves (PS)      t3 path paints (VS+PS)
//   t4 mono atlas (PS)       t5 poly atlas (PS)     t6 subpixel atlas (PS)
StructuredBuffer<Instance> instances : register(t0);
StructuredBuffer<PathTile> pathTiles : register(t1);
StructuredBuffer<PathCurve> pathCurves : register(t2);
StructuredBuffer<PathPaint> pathPaints : register(t3);
Texture2DArray<float4> monoAtlas : register(t4);
Texture2DArray<float4> polyAtlas : register(t5);
Texture2DArray<float4> subAtlas : register(t6);
SamplerState linearSampler : register(s0);

// Frame constants - 64 bytes, mirrored by gpuFrameConstants in d3d11.go.
cbuffer Frame : register(b0) {
    float2 viewport;          // 0
    float  premultiplied;     // 8   1.0 when the swap chain is premultiplied
    uint   quadCount;         // 12
    uint   batchBase;         // 16  quad cursor (quad batches) / instance base
    float  depthScale;        // 20
    uint   tileBase;          // 24  path-tile cursor for the current batch
    uint   cbPad0;            // 28
    float4 gammaRatios;       // 32  get_gamma_correction_ratios(fonts gamma)
    float  grayscaleContrast; // 48  default 1.0
    float  subpixelContrast;  // 52  default 0.5
    uint2  cbPad1;            // 56
}

struct VSIn {
    float2 corner : POSITION;
    uint inst : SV_InstanceID;
};

struct PSIn {
    float4 pos : SV_POSITION;
    // Flat like upstream QuadVarying's @interpolate(flat): a linearly
    // interpolated constant could land an ulp below the integer at the
    // pixel-center evaluation and truncate to instance-1 on the uint cast.
    nointerpolation uint inst : INSTID;
};

struct OpaquePSIn {
    float4 pos : SV_POSITION;
    nointerpolation float4 color : COLOR0;
};

struct TilePSIn {
    float4 pos : SV_POSITION;
    nointerpolation uint tileId : TEXCOORD0;
    nointerpolation float4 curveBox : TEXCOORD1;
    // Flattened PreparedBackground (inter-stage values can't be structs).
    nointerpolation float4 bgColor0 : TEXCOORD2;
    nointerpolation float4 bgColor1 : TEXCOORD3;
    nointerpolation uint bgTag : TEXCOORD4;
};

// ---- gamma correction (ported from upstream fs_mono_sprite /
// fs_subpixel_sprite; adapted from microsoft/terminal dwrite.hlsl) ----

float colorBrightness(float3 color) {
    // REC. 601 luminance coefficients for perceived brightness
    return dot(color, float3(0.30, 0.59, 0.11));
}

float lightOnDarkContrast(float enhancedContrast, float3 color) {
    float brightness = colorBrightness(color);
    float multiplier = saturate(4.0 * (0.75 - brightness));
    return enhancedContrast * multiplier;
}

float enhanceContrastFn(float alpha, float k) {
    return alpha * (k + 1.0) / (alpha * k + 1.0);
}

float3 enhanceContrast3(float3 alpha, float k) {
    return alpha * (k + 1.0) / (alpha * k + 1.0);
}

float applyAlphaCorrection(float a, float b, float4 g) {
    float brightnessAdjustment = g.x * b + g.y;
    float correction = brightnessAdjustment * a + (g.z * b + g.w);
    return a + a * (1.0 - a) * correction;
}

float3 applyAlphaCorrection3(float3 a, float3 b, float4 g) {
    float3 brightnessAdjustment = g.x * b + g.y;
    float3 correction = brightnessAdjustment * a + (g.z * b + g.w);
    return a + a * (1.0 - a) * correction;
}

// apply_contrast_and_gamma_correction(sample, color, factor, ratios):
// "sample" is a reserved HLSL word, hence the value parameter names.
float applyContrastAndGammaCorrection(float coverage, float3 color,
                                      float enhancedContrastFactor, float4 ratios) {
    float enhancedContrast = lightOnDarkContrast(enhancedContrastFactor, color);
    float brightness = colorBrightness(color);
    float contrasted = enhanceContrastFn(coverage, enhancedContrast);
    return applyAlphaCorrection(contrasted, brightness, ratios);
}

float3 applyContrastAndGammaCorrection3(float3 coverage, float3 color,
                                        float enhancedContrastFactor, float4 ratios) {
    float enhancedContrast = lightOnDarkContrast(enhancedContrastFactor, color);
    float3 contrasted = enhanceContrast3(coverage, enhancedContrast);
    return applyAlphaCorrection3(contrasted, color, ratios);
}

// ---- alpha compositing ----

// Abstract away the final color transformation based on the target alpha
// mode (upstream blend_color): straight-alpha windows blend with
// SrcAlpha/InvSrcAlpha; premultiplied windows need the shader to multiply
// the output RGB by the final alpha and blend with One/InvSrcAlpha.
float4 blendColor(float4 color, float alphaFactor) {
    float alpha = color.a * alphaFactor;
    float multiplier = (premultiplied != 0.0) ? alpha : 1.0;
    return float4(color.rgb * multiplier, alpha);
}

float4 over(float4 below, float4 above) {
    float alpha = above.a + below.a * (1.0 - above.a);
    if (alpha == 0.0) return float4(0, 0, 0, 0);
    return float4((above.rgb * above.a + below.rgb * below.a * (1.0 - above.a)) / alpha, alpha);
}

// ---- depth ----

float quadDepth(uint quadId) {
    return saturate((float(quadId) + 1.0) * depthScale);
}

// Culled vertices collapse to a degenerate position with z = 0, which fails
// the GREATER depth compare against the 0.0-cleared buffer and never draws.
static const float4 CULLED_POSITION = float4(0.0, 0.0, 0.0, 1.0);

// ---- shared math (ports of the Go rasterizer) ----

float quadSDFImpl(float ccx, float ccy, float radius) {
    if (radius == 0.0) return max(ccx, ccy);
    float sdInset = sqrt(max(0.0, ccx) * max(0.0, ccx) + max(0.0, ccy) * max(0.0, ccy)) + min(0.0, max(ccx, ccy));
    return sdInset - radius;
}

float pickCornerRadius(float cx, float cy, float4 radii) {
    // radii = tl, tr, br, bl
    if (cx < 0.0) {
        if (cy < 0.0) return radii.x;
        return radii.w;
    }
    if (cy < 0.0) return radii.y;
    return radii.z;
}

float quadSDF(float2 p, float4 bounds, float4 radii) {
    float hw = bounds.z * 0.5;
    float hh = bounds.w * 0.5;
    float px = p.x - bounds.x - hw;
    float py = p.y - bounds.y - hh;
    float ccx = abs(px) - hw;
    float ccy = abs(py) - hh;
    float r = pickCornerRadius(px, py, radii);
    return quadSDFImpl(ccx + r, ccy + r, r);
}

float quarterEllipseSDF(float px, float py, float rx, float ry) {
    float cx = px / rx;
    float cy = py / ry;
    float unit = sqrt(cx * cx + cy * cy) - 1.0;
    return unit * (rx + ry) * -0.5;
}

// ---- backgrounds (gradients, patterns, dither) ----

float srgbToLinearC(float a) {
    if (a < 0.04045) return a / 12.92;
    return pow((a + 0.055) / 1.055, 2.4);
}

float linearToSrgbC(float a) {
    if (a < 0.0031308) return a * 12.92;
    return 1.055 * pow(a, 1.0 / 2.4) - 0.055;
}

float3 oklabToLinearSrgb(float L, float A, float B) {
    float l_ = L + 0.3963377774 * A + 0.2158037573 * B;
    float m_ = L - 0.1055613458 * A - 0.0638541728 * B;
    float s_ = L - 0.0894841775 * A - 1.2914855480 * B;
    float l = l_ * l_ * l_;
    float m = m_ * m_ * m_;
    float s = s_ * s_ * s_;
    return float3(
        4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
        -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
        -0.0041960863 * l - 0.7034186147 * m + 1.7076147010 * s);
}

float ditherOffset(float2 pix) {
    int cx = (int)pix.x & 3;
    int cy = (int)pix.y & 3;
    int z = cx ^ cy;
    int threshold = ((z & 1) << 3) | ((cy & 1) << 2) | (z & 2) | ((cy & 2) >> 1);
    return (float(threshold) + 0.5) * (1.0 / 8.0) - 1.0;
}

// PaintData is the shared source of background shading: the uber shader
// builds it from an Instance's packed rows, the path pipeline from a
// PathPaint, so both reuse one backgroundColor implementation (upstream
// background_color(prepared, background, bounds, position)).
struct PaintData {
    float4 color0;
    float4 color1;
    float tag;
    float angleOrPattern; // gradient angle / pattern height / cell size
    float colorSpace;
    float pct0;
    float pct1;
};

PaintData paintFromInstance(Instance inst) {
    PaintData pd;
    pd.color0 = inst.color0;
    pd.color1 = inst.color1;
    pd.tag = inst.misc.x;
    pd.angleOrPattern = inst.misc.y;
    pd.colorSpace = inst.misc4.x;
    pd.pct0 = inst.misc4.y;
    pd.pct1 = inst.misc4.z;
    return pd;
}

PaintData paintFromPath(PathPaint paint) {
    PaintData pd;
    pd.color0 = paint.color0;
    pd.color1 = paint.color1;
    pd.tag = float(paint.tag);
    pd.angleOrPattern = paint.angle;
    pd.colorSpace = float(paint.colorSpace);
    pd.pct0 = paint.pct0;
    pd.pct1 = paint.pct1;
    return pd;
}

float4 backgroundColor(PaintData pd, float4 bounds, float2 p) {
    float4 color = pd.color0;
    if (pd.tag == 0.0) return color;
    float w = bounds.z;
    float h = bounds.w;
    if (pd.tag == 1.0) {
        // linear gradient
        float angle = pd.angleOrPattern % 360.0;
        float radians = (angle - 90.0) * 3.14159265358979 / 180.0;
        float dirX = cos(radians);
        float dirY = sin(radians);
        if (w > h) { dirY *= h / w; } else { dirX *= w / h; }
        float extent;
        if (abs(dirX) > abs(dirY)) { extent = w; } else { extent = h; }
        float directionLength = sqrt(dirX * dirX + dirY * dirY);
        float span = pd.pct1 - pd.pct0;
        float offset = 0.5 - pd.pct0;
        float gx = 0.0, gy = 0.0, gz = 0.0;
        if (directionLength != 0.0 && extent != 0.0) {
            if (span == 0.0) {
                const float bigStep = 1e30;
                gx = dirX / (directionLength * extent) * bigStep;
                gy = dirY / (directionLength * extent) * bigStep;
                gz = offset * bigStep;
            } else {
                gx = dirX / (directionLength * extent * span);
                gy = dirY / (directionLength * extent * span);
                gz = offset / span;
            }
        }
        float cx = bounds.x + w * 0.5;
        float cy = bounds.y + h * 0.5;
        float t = saturate((p.x - cx) * gx + (p.y - cy) * gy + gz);
        float4 mixed = pd.color0 + (pd.color1 - pd.color0) * t;
        if (pd.colorSpace == 1.0) {
            // oklab channels stored in rgb
            float3 lin = oklabToLinearSrgb(mixed.r, mixed.g, mixed.b);
            color = float4(lin, mixed.a);
        } else {
            color = float4(linearToSrgbC(mixed.r), linearToSrgbC(mixed.g), linearToSrgbC(mixed.b), mixed.a);
        }
        float d = ditherOffset(p);
        color = float4(color.r + d * 1.0 / 255.0, color.g + d * 1.0 / 255.0,
                       color.b + d * 1.0 / 255.0, color.a + d * 1.5 / 255.0);
    } else if (pd.tag == 2.0) {
        // slash pattern
        float packed = pd.angleOrPattern;
        float patternWidth = (packed / 65535.0) / 255.0;
        float patternInterval = (packed % 65535.0) / 255.0;
        float patternHeight = patternWidth + patternInterval;
        float stripeAngle = 3.14159265358979 / 4.0;
        float2 sd = float2(cos(stripeAngle), sin(stripeAngle));
        float patternPeriod = patternHeight * sin(stripeAngle);
        float threshold = patternPeriod * (patternWidth / patternHeight) / 2.0;
        float rel = (p.x - bounds.x) * sd.x + (p.y - bounds.y) * sd.y;
        float pattern = rel % patternPeriod;
        if (pattern < 0.0) pattern += patternPeriod;
        float dist = min(pattern, patternPeriod - pattern) - threshold;
        color.a *= saturate(0.5 - dist);
    } else if (pd.tag == 3.0) {
        // checkerboard
        float cellSize = pd.angleOrPattern;
        float relX = p.x - bounds.x;
        float relY = p.y - bounds.y;
        float parity = floor(relX / cellSize) + floor(relY / cellSize);
        parity = parity % 2.0;
        if (parity < 0.0) parity += 2.0;
        color.a *= saturate(parity);
    }
    return color;
}

// ---- dashed borders ----

float pickDashV(float num, float w) {
    if (w <= 0.0) return 0.0;
    return num / w;
}

float cornerDashVelocity(float dv1, float dv2) {
    if (dv1 == 0.0) return dv2;
    if (dv2 == 0.0) return dv1;
    return min(dv1, dv2);
}

float dashAlphaFn(float t, float period, float dashLen, float dashVelocity, float aaThreshold) {
    if (dashVelocity == 0.0) return 1.0;
    float halfPeriod = period / 2.0;
    float halfLength = dashLen / 2.0;
    float centered = ((t + halfPeriod - halfLength) % period) - halfPeriod;
    float signedDistance = abs(centered) - halfLength;
    return saturate(aaThreshold - signedDistance / dashVelocity);
}

float dashAlpha(Instance inst, float cx, float cy, float cornerRadius, bool unrounded,
                float ccx, float ccy, bool isNearRounded, float cornerToX, float cornerToY,
                float w, float h, float aaThreshold) {
    const float dashLengthPerWidth = 2.0;
    const float dashGapPerWidth = 1.0;
    const float dashPeriodPerWidth = dashLengthPerWidth + dashGapPerWidth;
    const float dvNumerator = 1.0 / dashPeriodPerWidth;

    float t = 0.0, maxT = 0.0, dashVelocity = 0.0;
    if (unrounded) {
        bool isHorizontal = ccx < ccy;
        float dashedBorderX = max(inst.borderWidths.z, inst.borderWidths.x); // bottom, top
        float dashedBorderY = max(inst.borderWidths.y, inst.borderWidths.w); // right, left
        float borderWidth = dashedBorderY;
        if (isHorizontal) borderWidth = dashedBorderX;
        if (borderWidth <= 0.0) return 1.0;
        dashVelocity = dvNumerator / borderWidth;
        // NOTE: "point" is a reserved keyword in HLSL (geometry-shader
        // primitive type); this variable is named "pt" on purpose.
        float pt = cx + w / 2.0;
        float size = w;
        if (isHorizontal) { pt = cy + h / 2.0; size = h; }
        t = pt * dashVelocity;
        maxT = size * dashVelocity;
    } else {
        float rTR = inst.radii.y;
        float rBR = inst.radii.z;
        float rBL = inst.radii.w;
        float rTL = inst.radii.x;
        float wT = inst.borderWidths.x;
        float wR = inst.borderWidths.y;
        float wB = inst.borderWidths.z;
        float wL = inst.borderWidths.w;
        float dvT = pickDashV(dvNumerator, wT);
        float dvR = pickDashV(dvNumerator, wR);
        float dvB = pickDashV(dvNumerator, wB);
        float dvL = pickDashV(dvNumerator, wL);
        float sT = (w - rTL - rTR) * dvT;
        float sR = (h - rTR - rBR) * dvR;
        float sB = (w - rBR - rBL) * dvB;
        float sL = (h - rBL - rTL) * dvL;
        float cvTR = cornerDashVelocity(dvT, dvR);
        float cvBR = cornerDashVelocity(dvB, dvR);
        float cvBL = cornerDashVelocity(dvB, dvL);
        float cvTL = cornerDashVelocity(dvT, dvL);
        float cTR = rTR * (3.14159265358979 / 2.0) * cvTR;
        float cBR = rBR * (3.14159265358979 / 2.0) * cvBR;
        float cBL = rBL * (3.14159265358979 / 2.0) * cvBL;
        float cTL = rTL * (3.14159265358979 / 2.0) * cvTL;
        float uptoTR = sT;
        float uptoR = uptoTR + cTR;
        float uptoBR = uptoR + sR;
        float uptoB = uptoBR + cBR;
        float uptoBL = uptoB + sB;
        float uptoL = uptoBL + cBL;
        float uptoTL = uptoL + sL;
        maxT = uptoTL + cTL;

        float pointX = cx + w / 2.0;
        float pointY = cy + h / 2.0;
        if (isNearRounded) {
            float radians = atan2(ccy, ccx);
            float cornerT = radians * cornerRadius;
            if (cx >= 0.0) {
                if (cy < 0.0) {
                    dashVelocity = cvTR;
                    t = uptoR - cornerT * dashVelocity;
                } else {
                    dashVelocity = cvBR;
                    t = uptoBR + cornerT * dashVelocity;
                }
            } else {
                if (cy >= 0.0) {
                    dashVelocity = cvBL;
                    t = uptoL - cornerT * dashVelocity;
                } else {
                    dashVelocity = cvTL;
                    t = uptoTL + cornerT * dashVelocity;
                }
            }
        } else {
            bool isHorizontal = ccx < ccy;
            if (isHorizontal) {
                if (cy < 0.0) {
                    dashVelocity = dvT;
                    t = (pointX - rTL) * dashVelocity;
                } else {
                    dashVelocity = dvB;
                    t = uptoBL - (pointX - rBL) * dashVelocity;
                }
            } else {
                if (cx < 0.0) {
                    dashVelocity = dvL;
                    t = uptoTL - (pointY - rTL) * dashVelocity;
                } else {
                    dashVelocity = dvR;
                    t = uptoR + (pointY - rTR) * dashVelocity;
                }
            }
        }
    }
    float dashLength = dashLengthPerWidth / dashPeriodPerWidth;
    if (unrounded) maxT -= dashLength;
    if (maxT >= 1.0) {
        float dashCount = floor(maxT);
        float dashPeriod = maxT / dashCount;
        return dashAlphaFn(t, dashPeriod, dashLength, dashVelocity, aaThreshold);
    } else if (unrounded) {
        float dashGap = maxT - dashLength;
        if (dashGap > 0.0) {
            float dashPeriod = dashLength + dashGap;
            return dashAlphaFn(t, dashPeriod, dashLength, dashVelocity, aaThreshold);
        }
    }
    return 1.0;
}

// ---- shadows ----

float erf7(float inVal) {
    float x = clamp(inVal, -10.0, 10.0) * (2.0 / sqrt(3.14159265358979));
    float xx = x * x;
    float y = x + (0.24295 + (0.03395 + 0.0104 * xx) * xx) * (x * xx);
    return y / sqrt(1.0 + y * y);
}

float pickCorner4(float cx, float cy, float4 v) {
    // v = tl, tr, br, bl
    if (cx < 0.0) {
        if (cy < 0.0) return v.x;
        return v.w;
    }
    if (cy < 0.0) return v.y;
    return v.z;
}

float blurredRoundedRect(float px, float py, float2 shapeHalf, float sInv, float minEdge,
                         float4 r1, float4 exponent, float4 scale) {
    float cr1 = pickCorner4(px, py, r1);
    float cexp = pickCorner4(px, py, exponent);
    float cscale = pickCorner4(px, py, scale);
    float p0x = abs(px) - (shapeHalf.x - cr1);
    float p0y = abs(py) - (shapeHalf.y - cr1);
    float p1x = max(p0x, 0.0);
    float p1y = max(p0y, 0.0);
    float dPos;
    if (p1x > 0.0 && p1y > 0.0) {
        dPos = pow(pow(p1x, cexp) + pow(p1y, cexp), 1.0 / cexp);
    } else {
        dPos = p1x + p1y;
    }
    float dNeg = min(max(p0x, p0y), 0.0);
    float d = dPos + dNeg - cr1;
    return cscale * (erf7(sInv * (minEdge + d)) - erf7(sInv * d));
}

void prepareBlurredRoundedRect(float w, float h, float4 radii, float sigma,
                               out float2 shapeHalf, out float sInv, out float minEdge,
                               out float4 r1, out float4 exponent, out float4 scale) {
    float s = max(sigma * sqrt(2.0), 1e-6);
    sInv = 1.0 / s;
    minEdge = min(w, h);
    float rMax = 0.5 * minEdge;
    float4 rs = radii;
    float4 r0 = min(sqrt(rs * rs + 1.15 * 1.15 * s * s), rMax);
    r1 = min(sqrt(rs * rs + 4.0 * s * s), rMax);
    float halfExtentX = 0.5 * sInv * w;
    float halfExtentY = 0.5 * sInv * h;
    float delta = 1.25 * s * (exp(-halfExtentX * halfExtentX) - exp(-halfExtentY * halfExtentY));
    float adjX = w + min(delta, 0.0);
    float adjY = h - max(delta, 0.0);
    float4 peak = sInv * 0.5 * (max(adjX, adjY) - 0.5 * rs);
    shapeHalf = float2(0.5 * adjX, 0.5 * adjY);
    exponent = 2.0 * r1 / r0;
    scale = 0.5 * float4(erf7(peak.x), erf7(peak.y), erf7(peak.z), erf7(peak.w));
}

// ---- shading ----

// shadeQuad returns the STRAIGHT color plus the antialiasing blend factor so
// psMain can route it through blendColor (upstream fs_quad's
// blend_color(color, saturate(aa - outer_sdf))).
float4 shadeQuad(Instance inst, float2 p, out float aaFactor) {
    const float aaThreshold = 0.5;
    float w = inst.bounds.z;
    float h = inst.bounds.w;
    float hw = w * 0.5;
    float hh = h * 0.5;
    float cx = p.x - inst.bounds.x - hw;
    float cy = p.y - inst.bounds.y - hh;

    float cornerRadius = pickCornerRadius(cx, cy, inst.radii);
    float borderX = inst.borderWidths.y; // right
    if (cx < 0.0) borderX = inst.borderWidths.w; // left
    float borderY = inst.borderWidths.z; // bottom
    if (cy < 0.0) borderY = inst.borderWidths.x; // top

    float reducedX = borderX;
    if (borderX == 0.0) reducedX = -aaThreshold;
    float reducedY = borderY;
    if (borderY == 0.0) reducedY = -aaThreshold;

    float cornerToX = abs(cx) - hw;
    float cornerToY = abs(cy) - hh;
    float ccx = cornerToX + cornerRadius;
    float ccy = cornerToY + cornerRadius;
    bool isNearRounded = ccx >= 0.0 && ccy >= 0.0;

    float straightX = cornerToX + reducedX;
    float straightY = cornerToY + reducedY;
    bool isBeyondInnerStraight = straightX > 0.0 || straightY > 0.0;
    bool isWithinInnerStraight = straightX < -aaThreshold && straightY < -aaThreshold;

    bool unrounded = inst.radii.x == 0.0 && inst.radii.y == 0.0 && inst.radii.z == 0.0 && inst.radii.w == 0.0;

    float4 color;
    if (isWithinInnerStraight && !isNearRounded) {
        aaFactor = 1.0;
        return backgroundColor(paintFromInstance(inst), inst.bounds, p);
    }
    float outerSDF = quadSDFImpl(ccx, ccy, cornerRadius);
    float innerSDF;
    if (ccx <= 0.0 || ccy <= 0.0) {
        innerSDF = -max(straightX, straightY);
    } else if (isBeyondInnerStraight) {
        innerSDF = -1.0;
    } else if (reducedX == reducedY) {
        innerSDF = -(outerSDF + reducedX);
    } else {
        float erx = max(0.0, cornerRadius - reducedX);
        float ery = max(0.0, cornerRadius - reducedY);
        innerSDF = quarterEllipseSDF(ccx, ccy, erx, ery);
    }
    float borderSDF = max(innerSDF, outerSDF);
    color = backgroundColor(paintFromInstance(inst), inst.bounds, p);
    if (borderSDF < aaThreshold) {
        float4 borderColor = inst.borderColor;
        if (inst.misc2.y == 1.0) {
            borderColor.a *= dashAlpha(inst, cx, cy, cornerRadius, unrounded,
                                       ccx, ccy, isNearRounded, cornerToX, cornerToY,
                                       w, h, aaThreshold);
        }
        borderColor.a *= saturate(aaThreshold - innerSDF);
        color = over(color, borderColor);
    }
    aaFactor = saturate(aaThreshold - outerSDF);
    return color;
}

void shadeShadow(Instance inst, float2 p, out float4 color, out float alpha) {
    color = inst.color0;
    if (inst.misc.z == 0.0) {
        alpha = saturate(0.5 - quadSDF(p, inst.shapeBounds, inst.shapeRadii));
    } else {
        float2 shapeHalf; float sInv; float minEdge; float4 r1; float4 exponent; float4 scale;
        prepareBlurredRoundedRect(inst.shapeBounds.z, inst.shapeBounds.w, inst.shapeRadii,
                                  inst.misc.z, shapeHalf, sInv, minEdge, r1, exponent, scale);
        float2 c = p - (inst.shapeBounds.xy + inst.shapeBounds.zw * 0.5);
        alpha = blurredRoundedRect(c.x, c.y, shapeHalf, sInv, minEdge, r1, exponent, scale);
    }
    if (inst.misc.w != 0.0) {
        alpha = 1.0 - alpha;
        float elementDistance = quadSDF(p, inst.bounds, inst.radii);
        alpha *= saturate(0.5 - elementDistance);
    }
}

void shadeWavyUnderline(Instance inst, float2 p, out float4 color, out float alpha) {
    const float waveFrequency = 2.0;
    const float waveHeightRatio = 0.8;
    float thickness = inst.misc2.z;
    float halfThickness = thickness * 0.5;
    float bh = inst.bounds.w;
    float frequency = 3.14159265358979 * waveFrequency * thickness / bh;
    float amplitude = (thickness * waveHeightRatio) / bh;
    float stX = (p.x - inst.bounds.x) / bh;
    float stY = (p.y - inst.bounds.y) / bh - 0.5;
    float sine = sin(stX * frequency) * amplitude;
    float dSine = cos(stX * frequency) * amplitude * frequency;
    float dist = (stY - sine) / sqrt(1.0 + dSine * dSine);
    float dip = dist * bh;
    alpha = saturate(0.5 - max(-dip - halfThickness, dip - halfThickness));
    color = inst.color0;
}

// ---- the uber pipeline (vs_main / ps_main) ----

// isOpaqueRectangle feeds both vsMain (culling pre-drawn opaque quads) and
// vsOpaqueQuad (the pre-pass itself); it must be declared before both.
bool isOpaqueRectangle(Instance inst) {
    return inst.misc.x == 0.0 &&
        inst.color0.a >= 1.0 &&
        inst.radii.x == 0.0 && inst.radii.y == 0.0 &&
        inst.radii.z == 0.0 && inst.radii.w == 0.0 &&
        inst.borderWidths.x == 0.0 && inst.borderWidths.y == 0.0 &&
        inst.borderWidths.z == 0.0 && inst.borderWidths.w == 0.0;
}

PSIn vsMain(VSIn i) {
    uint id = batchBase + i.inst;
    Instance inst = instances[id];
    PSIn o;
    // Opaque rectangles were already drawn (with depth) by the pre-pass;
    // cull them here exactly like upstream vs_quad.
    if (inst.misc2.x == 0.0 && isOpaqueRectangle(inst)) {
        o.pos = CULLED_POSITION;
        o.inst = id;
        return o;
    }
    float2 p = inst.bounds.xy + i.corner * inst.bounds.zw;
    o.pos = float4(p.x / viewport.x * 2.0 - 1.0, 1.0 - p.y / viewport.y * 2.0, 0.0, 1.0);
    // Quad batches run over the full depth range and every quad carries its
    // own painter's depth (upstream vs_quad). All other primitives run at the
    // batch's fixed depth: their viewport collapses to [depth, depth], which
    // maps the z = 0 output there (upstream per-kind pipelines).
    if (inst.misc2.x == 0.0) {
        o.pos.z = quadDepth(id);
    }
    o.inst = id;
    return o;
}

float4 psMain(PSIn input) : SV_Target {
    Instance inst = instances[input.inst];
    float2 p = input.pos.xy; // pixel centers (x+0.5, y+0.5)

    if (p.x < inst.contentMask.x || p.y < inst.contentMask.y ||
        p.x > inst.contentMask.z || p.y > inst.contentMask.w) {
        discard;
    }

    float primType = inst.misc2.x;
    float texKind = inst.misc2.w;
    float4 result = float4(0, 0, 0, 0);

    if (texKind == 1.0) {
        // Monochrome sprite, gamma corrected (upstream fs_mono_sprite).
        float2 uv = float2(
            inst.uvMat0.x * p.x + inst.uvMat0.y * p.y + inst.uvMat0.z,
            inst.uvMat1.x * p.x + inst.uvMat1.y * p.y + inst.uvMat1.z);
        float cov = monoAtlas.Sample(linearSampler, float3(uv, inst.misc3.x)).r;
        float alphaCorrected = applyContrastAndGammaCorrection(
            cov, inst.color0.rgb, grayscaleContrast, gammaRatios);
        result = blendColor(inst.color0, alphaCorrected);
    } else if (texKind == 3.0) {
        // Subpixel (ClearType) sprite. Upstream blends dual-source when the
        // backend supports it and falls back to the monochrome path
        // otherwise; this backend compiles the documented fallback: average
        // the RGB coverage, then apply the same contrast/gamma correction
        // with the subpixel enhanced-contrast factor.
        float2 uv = float2(
            inst.uvMat0.x * p.x + inst.uvMat0.y * p.y + inst.uvMat0.z,
            inst.uvMat1.x * p.x + inst.uvMat1.y * p.y + inst.uvMat1.z);
        float4 sv = subAtlas.Sample(linearSampler, float3(uv, inst.misc3.x));
        float cov = (sv.r + sv.g + sv.b) / 3.0;
        float alphaCorrected = applyContrastAndGammaCorrection(
            cov, inst.color0.rgb, subpixelContrast, gammaRatios);
        result = blendColor(inst.color0, alphaCorrected);
    } else if (texKind == 2.0) {
        // Polychrome sprite (upstream fs_poly_sprite).
        float2 uv = float2(
            inst.uvMat0.x * p.x + inst.uvMat0.y * p.y + inst.uvMat0.z,
            inst.uvMat1.x * p.x + inst.uvMat1.y * p.y + inst.uvMat1.z);
        float4 s = polyAtlas.Sample(linearSampler, float3(uv, inst.misc3.x));
        if (inst.misc3.w != 0.0) {
            float lum = 0.2126 * s.r + 0.7152 * s.g + 0.0722 * s.b;
            s = float4(lum, lum, lum, s.a);
        }
        float factor = inst.misc3.y;
        if (inst.misc3.z != 0.0) {
            factor *= saturate(0.5 - quadSDF(p, inst.bounds, inst.radii));
        }
        result = blendColor(s, factor);
    } else if (primType == 0.0) {
        float aaFactor;
        float4 color = shadeQuad(inst, p, aaFactor);
        result = blendColor(color, aaFactor);
    } else if (primType == 1.0) {
        float4 color; float alpha;
        shadeShadow(inst, p, color, alpha);
        result = blendColor(color, alpha);
    } else if (primType == 2.0) {
        // Solid underline: alpha is pre-squared on the CPU side (matching the
        // software renderer), so the blend factor is 1.
        result = blendColor(inst.color0, 1.0);
    } else {
        float4 color; float alpha;
        shadeWavyUnderline(inst, p, color, alpha);
        result = blendColor(color, alpha);
    }

    if (result.a <= 0.0) discard;
    return result;
}

// ---- opaque pre-pass (vs_opaque_quad / fs_opaque_quad) ----

// Draw every quad LAST first (upstream vs_opaque_quad reads
// instances[quadCount - 1 - instanceID] and the CPU sets batchBase to
// quadCount - 1); non-opaque quads collapse to CULLED_POSITION.
OpaquePSIn vsOpaqueQuad(VSIn i) {
    uint quadId = quadCount - 1u - i.inst;
    Instance inst = instances[quadId];
    OpaquePSIn o;
    if (!isOpaqueRectangle(inst)) {
        o.pos = CULLED_POSITION;
        o.color = float4(0, 0, 0, 0);
        return o;
    }
    // clip_to_mask: intersect the bounds with the content mask so the opaque
    // pass neither paints nor writes depth outside it (upstream clips the
    // same way).
    float2 origin = max(inst.bounds.xy, inst.contentMask.xy);
    float2 corner = min(inst.bounds.xy + inst.bounds.zw, inst.contentMask.zw);
    float2 size = max(corner - origin, float2(0.0, 0.0));
    float2 p = origin + i.corner * size;
    o.pos = float4(p.x / viewport.x * 2.0 - 1.0, 1.0 - p.y / viewport.y * 2.0,
                   quadDepth(quadId), 1.0);
    o.color = inst.color0;
    return o;
}

float4 psOpaqueQuad(OpaquePSIn input) : SV_Target {
    return input.color;
}

// ---- path tiles (Slug winding rasterization; vs_path_tile / fs_path_tile) ----

TilePSIn vsPathTile(VSIn i) {
    uint tileId = tileBase + i.inst;
    PathTile tile = pathTiles[tileId];
    PathPaint paint = pathPaints[tile.header.x];

    // Neighboring tiles store their shared edge as the same number, so
    // clipping each corner against the paint's content mask gives them
    // bit-identical edges (upstream vs_path_tile).
    float2 maskTopLeft = paint.contentMask.xy;
    float2 maskBottomRight = paint.contentMask.zw;
    float2 topLeft = clamp(tile.top_left, maskTopLeft, maskBottomRight);
    float2 bottomRight = clamp(tile.bottom_right, topLeft, maskBottomRight);
    float2 p = float2(
        (i.corner.x != 0.0) ? bottomRight.x : topLeft.x,
        (i.corner.y != 0.0) ? bottomRight.y : topLeft.y);

    TilePSIn o;
    o.pos = float4(p.x / viewport.x * 2.0 - 1.0, 1.0 - p.y / viewport.y * 2.0, 0.0, 1.0);
    o.tileId = tileId;
    o.curveBox = float4(tile.curves_top_left, tile.curves_bottom_right);
    o.bgColor0 = paint.color0;
    o.bgColor1 = paint.color1;
    o.bgTag = paint.tag;
    return o;
}

// Solves a*t^2 + b*t + c = 0 for the root in [0, 1] on a monotone piece.
float monotoneQuadraticRoot(float a, float b, float c, float direction) {
    float discriminant = max(b * b - 4.0 * a * c, 0.0);
    float denominator = b + direction * sqrt(discriminant);
    if (denominator == 0.0) {
        return 0.0;
    }
    return saturate(-2.0 * c / denominator);
}

// Coverage estimate adapted from Slug (Eric Lengyel, MIT license): accumulates
// the curve's winding along the pixel's leftward and upward rays. Like Slug,
// a crossing within half a pixel of the center counts fractionally.
void addCurveWinding(PathCurve curve, float2 center, inout float2 winding, inout float2 weight) {
    float minX = min(curve.p0.x, curve.p1.x);
    float maxX = max(curve.p0.x, curve.p1.x);

    if (curve.p0.y <= center.y && center.y < curve.p1.y) {
        if (maxX <= center.x - 0.5) {
            winding.x += curve.sign;
        } else {
            float t = monotoneQuadraticRoot(curve.a.y, curve.b.y, curve.p0.y - center.y, 1.0);
            float x = (curve.a.x * t + curve.b.x) * t + curve.p0.x;
            float dist = center.x - x;
            winding.x += curve.sign * saturate(dist + 0.5);
            weight.x = max(weight.x, saturate(1.0 - 2.0 * abs(dist)));
        }
    }

    if (minX <= center.x && center.x < maxX) {
        float direction = (curve.p1.x > curve.p0.x) ? 1.0 : -1.0;
        if (curve.p1.y <= center.y - 0.5) {
            winding.y -= curve.sign * direction;
        } else if (curve.p0.y < center.y + 0.5) {
            float t = monotoneQuadraticRoot(curve.a.x, curve.b.x, curve.p0.x - center.x, direction);
            float y = (curve.a.y * t + curve.b.y) * t + curve.p0.y;
            float dist = center.y - y;
            winding.y -= curve.sign * direction * saturate(dist + 0.5);
            weight.y = max(weight.y, saturate(1.0 - 2.0 * abs(dist)));
        }
    }
}

// The backdrop gives the unlisted curves' winding along a leftward ray from
// the tile's reference corner. Because paths are closed, chains of unlisted
// curves can only begin or end where a listed curve does, so the winding at
// the current pixel is the backdrop corrected for each listed endpoint
// between its ray and the pixel.
void correctForEndpoint(float2 endpoint, float windStep, float2 reference, float2 center,
                        inout float2 winding) {
    if (endpoint.x <= reference.x) {
        winding.x -= windStep * (float(endpoint.y <= center.y) - float(endpoint.y <= reference.y));
    }
    if (endpoint.y <= reference.y && endpoint.x <= center.x) {
        winding.y += windStep;
    }
}

float4 psPathTile(TilePSIn input) : SV_Target {
    float2 center = floor(input.pos.xy) + 0.5;
    // Fast path (upstream fs_path_tile): pixels outside the tile's curve box
    // of a solid fill are exactly the background color.
    if (input.bgTag == 0u &&
        (center.x < input.curveBox.x || center.y < input.curveBox.y ||
         center.x > input.curveBox.z || center.y > input.curveBox.w)) {
        return blendColor(input.bgColor0, 1.0);
    }
    PathTile tile = pathTiles[input.tileId];
    PathPaint paint = pathPaints[tile.header.x];

    float2 winding = float2(float(int(tile.header.w)), 0.0); // backdrop (i32)
    float2 weight = float2(0.0, 0.0);
    [loop]
    for (uint k = 0u; k < tile.header.z; k++) {
        PathCurve curve = pathCurves[tile.header.y + k];
        if (min(curve.p0.x, curve.p1.x) >= center.x + 0.5) {
            break;
        }
        if (curve.p0.y < center.y + 0.5) {
            addCurveWinding(curve, center, winding, weight);
            correctForEndpoint(curve.p0, curve.sign, tile.reference, center, winding);
            correctForEndpoint(curve.p1, -curve.sign, tile.reference, center, winding);
        }
    }

    // Weight the winding computed by each ray by its nearest crossing's
    // distance from the pixel center, using Slug's technique.
    float coverage = max(
        abs(dot(winding, weight)) / max(weight.x + weight.y, 1.0 / 65536.0),
        min(abs(winding.x), abs(winding.y)));
    if (paint.evenOdd != 0u) {
        coverage = 1.0 - abs(1.0 - frac(coverage * 0.5) * 2.0);
    } else {
        coverage = saturate(coverage);
    }

    if (coverage == 0.0) {
        return float4(0, 0, 0, 0);
    }

    PaintData pd;
    pd.color0 = input.bgColor0;
    pd.color1 = input.bgColor1;
    pd.tag = float(input.bgTag);
    pd.angleOrPattern = paint.angle;
    pd.colorSpace = float(paint.colorSpace);
    pd.pct0 = paint.pct0;
    pd.pct1 = paint.pct1;
    float4 color = backgroundColor(pd, paint.bounds, input.pos.xy);
    return blendColor(color, coverage);
}
`
