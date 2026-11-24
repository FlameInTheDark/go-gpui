// This file intentionally has NO build constraint: gammaCorrectionRatios is
// pure math, so its test runs on every platform (like TestVTableSlots).
package windows

import (
	"math"
	"testing"
)

// TestGammaCorrectionRatios pins the ported get_gamma_correction_ratios
// table at the default (1.8) and boundary gammas. Expected values were
// derived from the upstream table rows multiplied by NORM13 =
// 0x10000/(255*255)*4 and NORM24 = 0x100/255*4.
func TestGammaCorrectionRatios(t *testing.T) {
	cases := []struct {
		gamma float32
		want  [4]float32
	}{
		// gamma = 1.0: all ratios zero (row 0 of the table).
		{1.0, [4]float32{0, 0, 0, 0}},
		// gamma = 1.8 (the default): table row 8.
		{1.8, [4]float32{0.148054421, -0.89459455, 1.47590804, -0.324668258}},
		// gamma = 2.2 (upper clamp): last table row.
		{2.2, [4]float32{0.204696059, -1.39183688, 2.0007, -0.351472974}},
	}
	for _, c := range cases {
		got := gammaCorrectionRatios(c.gamma)
		for i := range got {
			if math.Abs(float64(got[i]-c.want[i])) > 1e-6 {
				t.Errorf("gammaCorrectionRatios(%v)[%d] = %v, want %v",
					c.gamma, i, got[i], c.want[i])
			}
		}
	}
	// Out-of-range gammas clamp to [1.0, 2.2] before indexing.
	low := gammaCorrectionRatios(0.5)
	if low != [4]float32{0, 0, 0, 0} {
		t.Errorf("gamma below range should clamp to 1.0, got %v", low)
	}
	high := gammaCorrectionRatios(3.0)
	want := gammaCorrectionRatios(2.2)
	if high != want {
		t.Errorf("gamma above range should clamp to 2.2, got %v want %v", high, want)
	}
}

// TestGammaRenderingParameters pins the defaults (gamma 1.8, grayscale
// contrast 1.0, subpixel contrast 0.5) and the environment overrides.
func TestGammaRenderingParameters(t *testing.T) {
	t.Setenv("GPUI_FONTS_GAMMA", "")
	t.Setenv("ZED_FONTS_GAMMA", "")
	t.Setenv("GPUI_FONTS_GRAYSCALE_CONTRAST", "")
	t.Setenv("ZED_FONTS_GRAYSCALE_ENHANCED_CONTRAST", "")
	t.Setenv("GPUI_FONTS_SUBPIXEL_CONTRAST", "")
	t.Setenv("ZED_FONTS_SUBPIXEL_ENHANCED_CONTRAST", "")

	p := newGammaRenderingParameters()
	want := gammaCorrectionRatios(1.8)
	if p.gammaRatios != want {
		t.Errorf("default gamma ratios = %v, want %v", p.gammaRatios, want)
	}
	if p.grayscaleContrast != 1.0 {
		t.Errorf("default grayscale contrast = %v, want 1.0", p.grayscaleContrast)
	}
	if p.subpixelContrast != 0.5 {
		t.Errorf("default subpixel contrast = %v, want 0.5", p.subpixelContrast)
	}

	t.Setenv("GPUI_FONTS_GAMMA", "1.3")
	t.Setenv("GPUI_FONTS_GRAYSCALE_CONTRAST", "0.25")
	t.Setenv("GPUI_FONTS_SUBPIXEL_CONTRAST", "0.75")
	p = newGammaRenderingParameters()
	if p.gammaRatios != gammaCorrectionRatios(1.3) {
		t.Errorf("GPUI_FONTS_GAMMA=1.3 ratios = %v", p.gammaRatios)
	}
	if p.grayscaleContrast != 0.25 || p.subpixelContrast != 0.75 {
		t.Errorf("contrast overrides = %v, %v", p.grayscaleContrast, p.subpixelContrast)
	}

	// Malformed values fall back to the defaults.
	t.Setenv("GPUI_FONTS_GAMMA", "not-a-number")
	if got := newGammaRenderingParameters().gammaRatios; got != gammaCorrectionRatios(1.8) {
		t.Errorf("malformed GPUI_FONTS_GAMMA should fall back to 1.8")
	}
}
