package vtui

import (
	"fmt"
	"regexp"
	"strconv"
	"unicode"
	"strings"
)

// Validator is an interface for validating string input in Edit controls.
// It supports both final validation (Validate) and real-time filtering (IsValidInput).
type Validator interface {
	// Validate checks the final content of the field (e.g. on OK).
	Validate(s string) bool
	// IsValidInput checks if the string is valid while the user is typing.
	// This can be used to block invalid characters or enforce a partial mask.
	IsValidInput(s string) bool
	// Error shows a message box describing the validation failure.
	Error(owner Frame)
}

// IntRangeValidator checks if input is an integer within [Min, Max].
type IntRangeValidator struct {
	Min, Max int
	Title    string
}

func (v *IntRangeValidator) Validate(s string) bool {
	val, err := strconv.Atoi(s)
	if err != nil {
		return false
	}
	return val >= v.Min && val <= v.Max
}

func (v *IntRangeValidator) Error(owner Frame) {
	msg := fmt.Sprintf("Value must be an integer\nbetween %d and %d.", v.Min, v.Max)
	title := " Invalid Input "
	if v.Title != "" { title = v.Title }
	ShowMessageOn(owner, title, msg, []string{"&Ok"})
}
func (v *IntRangeValidator) IsValidInput(s string) bool {
	if s == "" || s == "-" { // Allow empty or minus sign while typing
		return true
	}
	_, err := strconv.Atoi(s)
	return err == nil
}

// RegexValidator checks if input matches a regular expression.
type RegexValidator struct {
	Pattern      string
	ErrorMessage string
}

func (v *RegexValidator) Validate(s string) bool {
	matched, _ := regexp.MatchString(v.Pattern, s)
	return matched
}

func (v *RegexValidator) IsValidInput(s string) bool {
	return true // Hard to check partial regex reliably without a specific library
}

func (v *RegexValidator) Error(owner Frame) {
	msg := v.ErrorMessage
	if msg == "" {
		msg = "Input does not match required format."
	}
	ShowMessageOn(owner, " Invalid Input ", msg, []string{"&Ok"})
}// FilterValidator restricts input to a specific set of characters.
type FilterValidator struct {
	ValidChars   string
	ErrorMessage string
}

func (v *FilterValidator) Validate(s string) bool {
	return v.IsValidInput(s)
}

func (v *FilterValidator) IsValidInput(s string) bool {
	for _, r := range s {
		if !strings.ContainsRune(v.ValidChars, r) {
			return false
		}
	}
	return true
}

func (v *FilterValidator) Error(owner Frame) {
	msg := v.ErrorMessage
	if msg == "" {
		msg = "Input contains invalid characters."
	}
	ShowMessageOn(owner, " Invalid Input ", msg, []string{"&Ok"})
}

// LookupValidator checks if input is present in a list of allowed values.
type LookupValidator struct {
	List         []string
	IgnoreCase   bool
	ErrorMessage string
}

func (v *LookupValidator) Validate(s string) bool {
	for _, item := range v.List {
		if v.IgnoreCase {
			if strings.EqualFold(s, item) {
				return true
			}
		} else {
			if s == item {
				return true
			}
		}
	}
	return false
}

func (v *LookupValidator) IsValidInput(s string) bool {
	return true // Cannot check partial list match accurately
}

func (v *LookupValidator) Error(owner Frame) {
	msg := v.ErrorMessage
	if msg == "" {
		msg = "Value is not in the list of allowed items."
	}
	ShowMessageOn(owner, " Invalid Input ", msg, []string{"&Ok"})
}

// MaskValidator enforces a specific input pattern.
// # - Digit, ? - Letter, & - Letter (Upper), ! - Any (Upper), @ - Any.
type MaskValidator struct {
	Mask         string
	ErrorMessage string
}

func (v *MaskValidator) Validate(s string) bool {
	if len([]rune(s)) != len([]rune(v.Mask)) {
		return false
	}
	return v.check(s, false)
}

func (v *MaskValidator) IsValidInput(s string) bool {
	return v.check(s, true)
}

func (v *MaskValidator) check(s string, partial bool) bool {
	runes := []rune(s)
	mask := []rune(v.Mask)
	if !partial && len(runes) != len(mask) {
		return false
	}
	if len(runes) > len(mask) {
		return false
	}
	for i, r := range runes {
		m := mask[i]
		switch m {
		case '#':
			if !unicode.IsDigit(r) { return false }
		case '?':
			if !unicode.IsLetter(r) { return false }
		case '&':
			if !unicode.IsLetter(r) { return false }
		case '!':
			// Any char is allowed, uppercase check handled during typing usually
		case '@':
			// Any char allowed
		default:
			if r != m { return false }
		}
	}
	return true
}

func (v *MaskValidator) Error(owner Frame) {
	msg := v.ErrorMessage
	if msg == "" {
		msg = fmt.Sprintf("Input must match the pattern:\n%s", v.Mask)
	}
	ShowMessageOn(owner, " Invalid Input ", msg, []string{"&Ok"})
}
