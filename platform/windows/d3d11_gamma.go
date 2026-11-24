// This file intentionally has NO build constraint: it is pure data/math (the
// font gamma-correction parameters the Direct3D 11 renderer uploads in the
// frame constant buffer), so it compiles - and its unit test runs - on every
// platform, like d3d11_slots.go.
package windows

import (
	"math"
	"os"
	"strconv"
)

// gammaCorrectionRatios ports gpui's get_gamma_correction_ratios (adapted
// from microsoft/terminal's dwrite.cpp): the subpixel/antialiasing alpha
// correction ratios for a display gamma in [1.0, 2.2].
//
// The table rows are the GAMMA_INCORRECT_TARGET_RATIOS entries (already
// divided by 4, as upstream). The returned vector combines them with the
// NORM13/NORM24 normalizers, matching upstream exactly.
func gammaCorrectionRatios(gamma float32) [4]float32 {
	var gammaIncorrectTargetRatios = [13][4]float32{
		{0.0000 / 4.0, 0.0000 / 4.0, 0.0000 / 4.0, 0.0000 / 4.0},   // gamma = 1.0
		{0.0166 / 4.0, -0.0807 / 4.0, 0.2227 / 4.0, -0.0751 / 4.0}, // gamma = 1.1
		{0.0350 / 4.0, -0.1760 / 4.0, 0.4325 / 4.0, -0.1370 / 4.0}, // gamma = 1.2
		{0.0543 / 4.0, -0.2821 / 4.0, 0.6302 / 4.0, -0.1876 / 4.0}, // gamma = 1.3
		{0.0739 / 4.0, -0.3963 / 4.0, 0.8167 / 4.0, -0.2287 / 4.0}, // gamma = 1.4
		{0.0933 / 4.0, -0.5161 / 4.0, 0.9926 / 4.0, -0.2616 / 4.0}, // gamma = 1.5
		{0.1121 / 4.0, -0.6395 / 4.0, 1.1588 / 4.0, -0.2877 / 4.0}, // gamma = 1.6
		{0.1300 / 4.0, -0.7649 / 4.0, 1.3159 / 4.0, -0.3080 / 4.0}, // gamma = 1.7
		{0.1469 / 4.0, -0.8911 / 4.0, 1.4644 / 4.0, -0.3234 / 4.0}, // gamma = 1.8
		{0.1627 / 4.0, -1.0170 / 4.0, 1.6051 / 4.0, -0.3347 / 4.0}, // gamma = 1.9
		{0.1773 / 4.0, -1.1420 / 4.0, 1.7385 / 4.0, -0.3426 / 4.0}, // gamma = 2.0
		{0.1908 / 4.0, -1.2652 / 4.0, 1.8650 / 4.0, -0.3476 / 4.0}, // gamma = 2.1
		{0.2031 / 4.0, -1.3864 / 4.0, 1.9851 / 4.0, -0.3501 / 4.0}, // gamma = 2.2
	}
	const norm13 = float32(0x10000) / (255.0 * 255.0) * 4.0
	const norm24 = float32(0x100) / 255.0 * 4.0

	index := int(math.Round(float64(gamma) * 10.0))
	if index < 10 {
		index = 10
	}
	if index > 22 {
		index = 22
	}
	index -= 10
	ratios := gammaIncorrectTargetRatios[index]
	return [4]float32{
		ratios[0] * norm13,
		ratios[1] * norm24,
		ratios[2] * norm13,
		ratios[3] * norm24,
	}
}

// gammaRenderingParameters carries the font rendering parameters uploaded in
// the frame constant buffer (upstream RenderingParameters + GammaParams).
type gammaRenderingParameters struct {
	gammaRatios       [4]float32
	grayscaleContrast float32
	subpixelContrast  float32
}

// newGammaRenderingParameters resolves the parameters from the environment,
// defaulting to gamma 1.8, grayscale contrast 1.0 and subpixel contrast 0.5
// like upstream RenderingParameters::new. GPUI_FONTS_GAMMA and friends are
// checked first; upstream's ZED_FONTS_* names are honored as fallbacks.
func newGammaRenderingParameters() gammaRenderingParameters {
	gamma := envFloat(envFloat(1.8, "GPUI_FONTS_GAMMA"), "ZED_FONTS_GAMMA")
	if gamma < 1.0 {
		gamma = 1.0
	}
	if gamma > 2.2 {
		gamma = 2.2
	}
	grayscale := envFloat(envFloat(1.0, "GPUI_FONTS_GRAYSCALE_CONTRAST"), "ZED_FONTS_GRAYSCALE_ENHANCED_CONTRAST")
	if grayscale < 0 {
		grayscale = 0
	}
	subpixel := envFloat(envFloat(0.5, "GPUI_FONTS_SUBPIXEL_CONTRAST"), "ZED_FONTS_SUBPIXEL_ENHANCED_CONTRAST")
	if subpixel < 0 {
		subpixel = 0
	}
	return gammaRenderingParameters{
		gammaRatios:       gammaCorrectionRatios(gamma),
		grayscaleContrast: grayscale,
		subpixelContrast:  subpixel,
	}
}

// envFloat parses the first set environment variable as a float, falling
// back to def when unset or malformed.
func envFloat(def float32, names ...string) float32 {
	for _, name := range names {
		v, ok := os.LookupEnv(name)
		if !ok {
			continue
		}
		f, err := strconv.ParseFloat(v, 32)
		if err != nil {
			// Malformed values fall through to the next name / default.
			continue
		}
		return float32(f)
	}
	return def
}
