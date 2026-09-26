package vtui

import (
	"fmt"
	"math"
	"strings"
)

// ColorPair is one foreground/background combination to validate: either a
// single palette slot's own text-on-background pair, or two colors that sit
// next to each other on screen (for example a dialog box drawn over a
// panel). Colors are packed 0xRRGGBB; any bits above that are ignored.
type ColorPair struct {
	Name   string
	FG, BG uint32
}

// ColorError describes one perceptual problem found in a ColorPair.
type ColorError struct {
	Pair    ColorPair
	Message string
}

func (e ColorError) Error() string {
	return e.Message
}

// ColorRules describes how strict the color scheme validator is.
//
// The two checks approach visual discomfort from opposite directions.
// MinContrastRatio catches colors that are too close in luminance to read
// comfortably — the classic light-yellow-on-light-gray hotkey bug (about
// 1.2:1) is far below WCAG AA's 4.5:1 for normal text. HarshChromaThreshold
// and HarshHueDeltaDeg catch the opposite problem that current models of
// human color perception also flag: two highly saturated colors of very
// different hue sitting right next to each other, which reads as harsh or
// "eye-gouging" even when their luminance contrast is technically fine (a
// saturated yellow on a saturated cyan is the textbook example).
type ColorRules struct {
	// MinContrastRatio is the WCAG relative-luminance contrast ratio every
	// pair must reach. 0 disables this check.
	MinContrastRatio float64

	// HarshChromaThreshold is the CIE L*a*b* chroma (C*, roughly how
	// saturated a color looks) at or above which a color counts as
	// "saturated" for the clash check below. 0 disables this check.
	HarshChromaThreshold float64

	// HarshHueDeltaDeg is the minimum hue-angle difference, in degrees
	// around the a*b* plane, between two saturated colors before their
	// combination is flagged as a harsh clash.
	HarshHueDeltaDeg float64
}

// DefaultColorRules is used by ValidateColors and AssertColors.
var DefaultColorRules = ColorRules{
	MinContrastRatio:     4.5,
	HarshChromaThreshold: 40,
	HarshHueDeltaDeg:     60,
}

// ValidateColors checks each pair against DefaultColorRules.
func ValidateColors(pairs []ColorPair) []error {
	return ValidateColorsWithRules(pairs, DefaultColorRules)
}

// ValidateColorsWithRules is ValidateColors with a custom rule set.
func ValidateColorsWithRules(pairs []ColorPair, rules ColorRules) []error {
	var errs []error
	for _, pair := range pairs {
		if rules.MinContrastRatio > 0 {
			ratio := contrastRatioRGB(pair.FG, pair.BG)
			if ratio < rules.MinContrastRatio {
				errs = append(errs, ColorError{
					Pair: pair,
					Message: fmt.Sprintf("[%s] insufficient contrast: #%06x on #%06x is %.2f:1, need at least %.2f:1",
						pair.Name, pair.FG, pair.BG, ratio, rules.MinContrastRatio),
				})
			}
		}

		if rules.HarshChromaThreshold > 0 && rules.HarshHueDeltaDeg > 0 {
			_, a1, b1 := rgbToLab(pair.FG)
			_, a2, b2 := rgbToLab(pair.BG)
			c1 := math.Hypot(a1, b1)
			c2 := math.Hypot(a2, b2)
			if c1 >= rules.HarshChromaThreshold && c2 >= rules.HarshChromaThreshold {
				if hue := hueDeltaDeg(a1, b1, a2, b2); hue >= rules.HarshHueDeltaDeg {
					errs = append(errs, ColorError{
						Pair: pair,
						Message: fmt.Sprintf("[%s] harsh color clash: #%06x next to #%06x are both highly saturated (C*=%.0f/%.0f) with a %.0f° hue difference",
							pair.Name, pair.FG, pair.BG, c1, c2, hue),
					})
				}
			}
		}
	}
	return errs
}

// PaletteColorPairs builds one ColorPair per non-empty slot of a vtui-style
// packed palette (see SetRGBBoth/SetIndexBoth), pairing each slot's own
// foreground and background. names labels each index for error messages; a
// nil slice, or one shorter than a given index, falls back to a numeric
// label for that slot. Indexed (non true-color) colors are resolved through
// ThemePalette. A slot left at the zero value is treated as unset and
// skipped, matching how the rest of vtui treats a zero palette entry.
func PaletteColorPairs(palette []uint64, names []string) []ColorPair {
	pairs := make([]ColorPair, 0, len(palette))
	for i, attr := range palette {
		if attr == 0 {
			continue
		}
		var fg, bg uint32
		if attr&IsFgRGB != 0 {
			fg = GetRGBFore(attr)
		} else {
			fg = ThemePalette[GetIndexFore(attr)]
		}
		if attr&IsBgRGB != 0 {
			bg = GetRGBBack(attr)
		} else {
			bg = ThemePalette[GetIndexBack(attr)]
		}

		var name string
		if i < len(names) {
			name = names[i]
		}
		if name == "" {
			name = fmt.Sprintf("palette[%d]", i)
		}
		pairs = append(pairs, ColorPair{Name: name, FG: fg, BG: bg})
	}
	return pairs
}

// AssertColors is a helper for tests to fail if any pair violates
// DefaultColorRules.
func AssertColors(t interface{ Errorf(string, ...any) }, pairs []ColorPair) {
	AssertColorsWithRules(t, pairs, DefaultColorRules)
}

// AssertColorsWithRules is AssertColors with a custom rule set.
func AssertColorsWithRules(t interface{ Errorf(string, ...any) }, pairs []ColorPair, rules ColorRules) {
	reportColorErrors(t, ValidateColorsWithRules(pairs, rules))
}

func reportColorErrors(t interface{ Errorf(string, ...any) }, errs []error) {
	if len(errs) == 0 {
		return
	}
	var msgs []string
	for _, e := range errs {
		msgs = append(msgs, e.Error())
	}
	t.Errorf("Color validation failed:\n%s", strings.Join(msgs, "\n"))
}

// --- Color math --------------------------------------------------------
//
// Compact, self-contained implementations of the standard formulas so this
// file has no dependency beyond the standard library. The perceptual
// distance used by the harsh-clash check is plain CIE76 (Euclidean distance
// in L*a*b*) rather than the more elaborate CIEDE2000: for the coarse
// "is this combination harsh" question asked here that is an adequate
// approximation, at a fraction of the code and with no extra state to get
// wrong.

// srgbToLinear undoes the sRGB gamma curve for one channel in 0..1.
func srgbToLinear(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// rgbChannel extracts one 8-bit channel from a packed 0xRRGGBB color,
// scaled to 0..1. shift is 16 for red, 8 for green, 0 for blue.
func rgbChannel(rgb uint32, shift uint) float64 {
	return float64((rgb>>shift)&0xFF) / 255.0
}

// relativeLuminanceRGB is the WCAG relative luminance of a packed color.
func relativeLuminanceRGB(rgb uint32) float64 {
	r := srgbToLinear(rgbChannel(rgb, 16))
	g := srgbToLinear(rgbChannel(rgb, 8))
	b := srgbToLinear(rgbChannel(rgb, 0))
	return 0.2126*r + 0.7152*g + 0.0722*b
}

// contrastRatioRGB is the WCAG contrast ratio between two packed colors.
func contrastRatioRGB(a, b uint32) float64 {
	la, lb := relativeLuminanceRGB(a), relativeLuminanceRGB(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// rgbToLab converts a packed 0xRRGGBB color to CIE L*a*b* (D65 white
// point). L is 0..100; a and b are roughly -128..127.
func rgbToLab(rgb uint32) (l, a, b float64) {
	r := srgbToLinear(rgbChannel(rgb, 16))
	g := srgbToLinear(rgbChannel(rgb, 8))
	bl := srgbToLinear(rgbChannel(rgb, 0))

	x := (r*0.4124 + g*0.3576 + bl*0.1805) / 0.95047
	y := r*0.2126 + g*0.7152 + bl*0.0722
	z := (r*0.0193 + g*0.1192 + bl*0.9505) / 1.08883

	f := func(t float64) float64 {
		if t > 0.008856 {
			return math.Cbrt(t)
		}
		return 7.787*t + 16.0/116.0
	}

	l = 116.0*f(y) - 16.0
	a = 500.0 * (f(x) - f(y))
	b = 200.0 * (f(y) - f(z))
	return l, a, b
}

// hueDeltaDeg returns the angle, in degrees (0..180), between two points in
// the a*b* plane — how different two colors' hues are once lightness is
// factored out. Chroma near zero has no well-defined hue; callers only use
// this once both colors have already passed a chroma threshold, where the
// angle is meaningful.
func hueDeltaDeg(a1, b1, a2, b2 float64) float64 {
	h1 := math.Atan2(b1, a1)
	h2 := math.Atan2(b2, a2)
	d := math.Abs(h1 - h2)
	if d > math.Pi {
		d = 2*math.Pi - d
	}
	return d * 180.0 / math.Pi
}
