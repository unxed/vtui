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
// 1.2:1) is far below WCAG AA's 4.5:1 for normal text. HarshChromaThreshold,
// HarshHueDeltaDeg and HarshMaxLightnessDelta catch the opposite problem
// that current models of human color perception also flag: two highly
// saturated colors of very different hue sitting right next to each other,
// which reads as harsh or "eye-gouging" even when their luminance contrast
// is technically fine (a saturated yellow on a saturated blue is the
// textbook example).
//
// Chroma and hue delta alone are not enough to tell that apart from a
// completely ordinary bright-foreground-on-dark-background pair, though:
// bright cyan text (#00ffff, C*≈50) on a navy panel background (#0000a0,
// C*≈94) is a classic, comfortable terminal color choice — Norton
// Commander/far2l-style — yet both colors are "saturated" by that
// definition and their hues sit 110° apart, well past a flat 60° cutoff.
// What actually separates that pair from a genuine clash (saturated yellow
// directly on saturated blue, hues 157° apart) is lightness: the clash
// pairs found in practice are close to isoluminant (ΔL* well under 40 —
// pure red on pure green, magenta on cyan, and so on), matching the known
// perceptual effect that two saturated, hue-opposed colors "vibrate"
// uncomfortably mainly when neither one reads as clearly lighter or darker
// than the other. Once there is a wide lightness gap (ΔL* in the high 60s
// or more, as with cyan-on-navy at 73 or the Classic scheme's
// yellow-on-navy selection highlight at 79), the pair instead reads as
// ordinary light-on-dark text, and the vibrating-clash effect does not
// apply even though the hue delta is, if anything, larger than in the
// genuine clash cases. HarshMaxLightnessDelta encodes that gap: it exempts
// a pair from the clash check once its two colors are far enough apart in
// lightness, rather than trying to fix the false positive by loosening
// chroma or hue delta (which would just stop catching real clashes, since
// real clashes and this false positive share very similar chroma and hue
// numbers).
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
	// combination is a candidate harsh clash (subject to
	// HarshMaxLightnessDelta below).
	HarshHueDeltaDeg float64

	// HarshMaxLightnessDelta caps how far apart, in CIE L* (0..100), two
	// otherwise-clashing colors may be before the clash check is skipped.
	// A wide lightness gap means the pair reads as light text on a dark
	// (or dark text on a light) surface rather than as two isoluminant
	// saturated colors vibrating against each other — see the ColorRules
	// doc comment for the empirical basis. 0 disables this exemption, so
	// every pair that clears the chroma and hue-delta gates is flagged
	// regardless of lightness.
	HarshMaxLightnessDelta float64
}

// DefaultColorRules is used by ValidateColors and AssertColors.
var DefaultColorRules = ColorRules{
	MinContrastRatio:       4.5,
	HarshChromaThreshold:   40,
	HarshHueDeltaDeg:       60,
	HarshMaxLightnessDelta: 69,
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
			l1, a1, b1 := rgbToLab(pair.FG)
			l2, a2, b2 := rgbToLab(pair.BG)
			c1 := math.Hypot(a1, b1)
			c2 := math.Hypot(a2, b2)
			lightnessGap := math.Abs(l1 - l2)
			if c1 >= rules.HarshChromaThreshold && c2 >= rules.HarshChromaThreshold &&
				(rules.HarshMaxLightnessDelta <= 0 || lightnessGap <= rules.HarshMaxLightnessDelta) {
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
