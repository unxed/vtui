package vtui

import (
	"testing"
)

func TestFilterValidator(t *testing.T) {
	v := &FilterValidator{ValidChars: "0123456789ABCDEF"}

	if !v.IsValidInput("123AB") {
		t.Error("Should accept hex string")
	}
	if v.IsValidInput("123AG") {
		t.Error("Should reject non-hex character G")
	}
}

func TestIntRangeValidator(t *testing.T) {
	v := &IntRangeValidator{Min: -2, Max: 10}
	for _, tc := range []struct {
		input string
		valid bool
	}{
		{"-2", true}, {"10", true}, {"-3", false}, {"11", false}, {"nope", false},
	} {
		if got := v.Validate(tc.input); got != tc.valid {
			t.Errorf("Validate(%q) = %v, want %v", tc.input, got, tc.valid)
		}
	}
	for _, tc := range []struct {
		input string
		valid bool
	}{
		{"", true}, {"-", true}, {"12", true}, {"1x", false},
	} {
		if got := v.IsValidInput(tc.input); got != tc.valid {
			t.Errorf("IsValidInput(%q) = %v, want %v", tc.input, got, tc.valid)
		}
	}
}

func TestRegexValidator(t *testing.T) {
	v := &RegexValidator{Pattern: `^[a-z]+$`}
	if !v.Validate("hello") {
		t.Error("RegexValidator rejected a matching string")
	}
	if v.Validate("Hello1") {
		t.Error("RegexValidator accepted a non-matching string")
	}
	if !v.IsValidInput("partial") {
		t.Error("RegexValidator should allow partial input")
	}
}

func TestOctalValidator(t *testing.T) {
	v := &OctalValidator{MaxDigits: 3}
	for _, tc := range []struct {
		input   string
		final   bool
		partial bool
	}{
		{"", true, true}, {"755", true, true}, {"7550", false, false}, {"789", false, false}, {"77x", false, false},
	} {
		if got := v.Validate(tc.input); got != tc.final {
			t.Errorf("Validate(%q) = %v, want %v", tc.input, got, tc.final)
		}
		if got := v.IsValidInput(tc.input); got != tc.partial {
			t.Errorf("IsValidInput(%q) = %v, want %v", tc.input, got, tc.partial)
		}
	}
}

func TestLookupValidator(t *testing.T) {
	v := &LookupValidator{
		List:       []string{"UTF-8", "CP866", "Windows-1251"},
		IgnoreCase: true,
	}

	if !v.Validate("utf-8") {
		t.Error("Lookup failed with IgnoreCase=true")
	}
	if v.Validate("ASCII") {
		t.Error("Lookup should fail for item not in list")
	}
}

func TestMaskValidator(t *testing.T) {
	// Pattern: 2 digits, a dash, 3 letters
	v := &MaskValidator{Mask: "##-???"}

	if !v.IsValidInput("12") {
		t.Error("Partial valid input rejected")
	}
	if v.IsValidInput("1A") {
		t.Error("Mask violation (# expected digit) not detected")
	}
	if !v.Validate("12-ABC") {
		t.Error("Full valid string rejected")
	}
	if v.Validate("12-AB") {
		t.Error("Incomplete string should fail Validate")
	}
}

func TestMaskValidator_Uppercase(t *testing.T) {
	v := &MaskValidator{Mask: "&&&"}

	// Real test of Edit integration would require a mock InputEvent,
	// here we just test the logic of check.
	if !v.check("ABC", false) {
		t.Error("Upper case letters should be valid for '&'")
	}
}

func TestMaskValidator_SpecialMarkers(t *testing.T) {
	// ! - Any (Upper), @ - Any
	v := &MaskValidator{Mask: "!@#"}

	// 1. Valid inputs
	if !v.IsValidInput("A12") {
		t.Error("Valid input rejected")
	}
	if !v.IsValidInput("!%1") {
		t.Error("Symbols should be allowed by ! and @")
	}

	// 2. Marker # constraint
	if v.IsValidInput("AB A") {
		t.Error("Letter in digit slot (#) should be rejected")
	}

	// 3. Length constraint
	if v.IsValidInput("ABCD") {
		t.Error("Input exceeding mask length should be rejected")
	}
}

func TestMaskValidator_LiteralEscaping(t *testing.T) {
	// Test that non-marker characters in mask are treated as literals
	v := &MaskValidator{Mask: "Ref-####"}

	if !v.IsValidInput("Ref-1") {
		t.Error("Valid prefix rejected")
	}
	if v.IsValidInput("Rex-1") {
		t.Error("Mismatching literal 'x' instead of 'f' should be rejected")
	}
}
