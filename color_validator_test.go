package vtui

import (
	"math"
	"strings"
	"testing"
)

// TestValidateColors_FlagsLightYellowOnLightGray encodes the concrete
// regression named in f4#363: hotkey/highlight text rendered as a light
// yellow on a light gray background, as happened in far2l's default dark
// scheme. Pure WCAG contrast already catches this without any perceptual
// color-difference math: the pair sits at roughly 1.7:1, far below the
// 4.5:1 normal-text threshold.
func TestValidateColors_FlagsLightYellowOnLightGray(t *testing.T) {
	pairs := []ColorPair{
		{Name: "Dialog.HighlightText", FG: 0xFFFF00, BG: 0xC0C0C0},
	}
	errs := ValidateColors(pairs)
	if len(errs) == 0 {
		t.Fatal("expected the light-yellow-on-light-gray hotkey pair to be flagged, got no errors")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "insufficient contrast") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an insufficient-contrast error, got: %v", errs)
	}
}

// TestValidateColors_GoodSchemePasses makes sure the validator does not flag
// everything: plain, readable, unsaturated combinations must come back
// clean under the default rules.
func TestValidateColors_GoodSchemePasses(t *testing.T) {
	pairs := []ColorPair{
		{Name: "Dialog.Text", FG: 0x000000, BG: 0xC0C0C0}, // black on light gray
		{Name: "Panel.Text", FG: 0xD3D7CF, BG: 0x2E3436},  // light gray on dark gray
		{Name: "Editor.Text", FG: 0xFFFFFF, BG: 0x000080}, // white on navy
	}
	if errs := ValidateColors(pairs); len(errs) != 0 {
		t.Errorf("expected a well-behaved scheme to pass, got: %v", errs)
	}
}

// TestValidateColors_FlagsHarshSaturatedClash covers the axis pure luminance
// contrast misses entirely: two highly saturated colors of very different
// hue, "eye-gouging" even though their WCAG contrast ratio is comfortably
// above the readability threshold.
func TestValidateColors_FlagsHarshSaturatedClash(t *testing.T) {
	pairs := []ColorPair{
		// Saturated yellow on saturated blue: WCAG contrast is a healthy
		// ~8:1 (well above 4.5), so this pair must be flagged for its harsh
		// clash, not for insufficient contrast.
		{Name: "Test.HarshPair", FG: 0xFFFF00, BG: 0x0000FF},
	}
	errs := ValidateColors(pairs)
	if len(errs) == 0 {
		t.Fatal("expected the saturated yellow-on-blue pair to be flagged as a harsh clash")
	}
	for _, e := range errs {
		if strings.Contains(e.Error(), "insufficient contrast") {
			t.Errorf("pair has fine WCAG contrast, should not be flagged for that: %v", e)
		}
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "harsh color clash") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a harsh-color-clash error, got: %v", errs)
	}
}

// TestValidateColorsWithRules_ThresholdsAreHonored spot-checks that both
// checks can be independently disabled and that the contrast threshold is
// configurable, the way LayoutRules' fields work for the layout validator.
func TestValidateColorsWithRules_ThresholdsAreHonored(t *testing.T) {
	pairs := []ColorPair{{Name: "p", FG: 0xFFFF00, BG: 0xC0C0C0}}

	// Disabling the contrast check entirely must silence the regression case.
	rules := DefaultColorRules
	rules.MinContrastRatio = 0
	if errs := ValidateColorsWithRules(pairs, rules); len(errs) != 0 {
		t.Errorf("MinContrastRatio = 0 should disable the contrast check, got: %v", errs)
	}

	// A lax enough ratio must let a mediocre pair through.
	rules = DefaultColorRules
	rules.MinContrastRatio = 1.0
	if errs := ValidateColorsWithRules(pairs, rules); len(errs) != 0 {
		t.Errorf("a 1.0:1 minimum should accept every pair, got: %v", errs)
	}
}

// TestPaletteColorPairs_SkipsUnsetSlotsAndNamesByIndex exercises the
// adapter between a raw vtui palette (as used by Palette/SetRGBBoth) and the
// pair-based validator API.
func TestPaletteColorPairs_SkipsUnsetSlotsAndNamesByIndex(t *testing.T) {
	palette := make([]uint64, 3)
	palette[0] = 0 // unset: must be skipped
	palette[1] = SetRGBBoth(0, 0x123456, 0x654321)
	palette[2] = SetIndexBoth(0, 1, 2) // resolved via ThemePalette

	names := []string{"Slot0", "Slot1"} // shorter than the palette: index 2 falls back

	pairs := PaletteColorPairs(palette, names)
	if len(pairs) != 2 {
		t.Fatalf("expected 2 pairs (slot 0 skipped), got %d: %+v", len(pairs), pairs)
	}

	if pairs[0].Name != "Slot1" || pairs[0].FG != 0x123456 || pairs[0].BG != 0x654321 {
		t.Errorf("unexpected RGB pair: %+v", pairs[0])
	}

	wantFG, wantBG := ThemePalette[1], ThemePalette[2]
	if pairs[1].Name != "palette[2]" || pairs[1].FG != wantFG || pairs[1].BG != wantBG {
		t.Errorf("unexpected indexed pair: %+v (want fg=#%06x bg=#%06x)", pairs[1], wantFG, wantBG)
	}
}

// TestAssertColors_ReportsThroughErrorfInterface makes sure the test helper
// mirrors AssertLayout: it must call Errorf exactly when there are errors.
func TestAssertColors_ReportsThroughErrorfInterface(t *testing.T) {
	var calls int
	fake := &fakeT{errorf: func(string, ...any) { calls++ }}

	AssertColors(fake, []ColorPair{{Name: "ok", FG: 0x000000, BG: 0xFFFFFF}})
	if calls != 0 {
		t.Errorf("a passing pair must not call Errorf, got %d calls", calls)
	}

	AssertColors(fake, []ColorPair{{Name: "bad", FG: 0xFFFF00, BG: 0xC0C0C0}})
	if calls != 1 {
		t.Errorf("a failing pair must call Errorf once, got %d calls", calls)
	}
}

type fakeT struct {
	errorf func(string, ...any)
}

func (f *fakeT) Errorf(format string, args ...any) { f.errorf(format, args...) }

// --- Color math sanity checks -------------------------------------------

func TestContrastRatioRGB_KnownValues(t *testing.T) {
	if got := contrastRatioRGB(0xFFFFFF, 0x000000); math.Abs(got-21.0) > 0.05 {
		t.Errorf("white/black contrast = %.3f, want ~21", got)
	}
	if got := contrastRatioRGB(0x808080, 0x808080); math.Abs(got-1.0) > 1e-9 {
		t.Errorf("identical color contrast = %.3f, want 1", got)
	}
}

func TestRgbToLab_KnownValues(t *testing.T) {
	cases := []struct {
		rgb     uint32
		l, a, b float64
	}{
		{0xFFFFFF, 100.0, 0.0, 0.0},
		{0x000000, 0.0, 0.0, 0.0},
		{0xFF0000, 53.24, 80.09, 67.20},
	}
	for _, tc := range cases {
		l, a, b := rgbToLab(tc.rgb)
		if math.Abs(l-tc.l) > 0.1 || math.Abs(a-tc.a) > 0.1 || math.Abs(b-tc.b) > 0.1 {
			t.Errorf("#%06x -> Lab(%.2f, %.2f, %.2f), want (%.2f, %.2f, %.2f)", tc.rgb, l, a, b, tc.l, tc.a, tc.b)
		}
	}
}

func TestHueDeltaDeg_WrapsAroundCorrectly(t *testing.T) {
	// Two points near the +a axis and -a axis are 180 degrees apart either
	// way you measure, never reported as 0 or as more than 180.
	if got := hueDeltaDeg(10, 1, -10, -1); math.Abs(got-180.0) > 1.0 {
		t.Errorf("opposite hues = %.1f degrees, want ~180", got)
	}
	if got := hueDeltaDeg(10, 0, 10, 0); got != 0 {
		t.Errorf("identical hues = %.1f degrees, want 0", got)
	}
}
