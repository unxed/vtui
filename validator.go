package vtui

import (
	"fmt"
	"regexp"
	"strconv"
)

// Validator is an interface for validating string input in Edit controls.
type Validator interface {
	Validate(s string) bool
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

// RegexValidator checks if input matches a regular expression.
type RegexValidator struct {
	Pattern      string
	ErrorMessage string
}

func (v *RegexValidator) Validate(s string) bool {
	matched, _ := regexp.MatchString(v.Pattern, s)
	return matched
}

func (v *RegexValidator) Error(owner Frame) {
	msg := v.ErrorMessage
	if msg == "" {
		msg = "Input does not match required format."
	}
	ShowMessageOn(owner, " Invalid Input ", msg, []string{"&Ok"})
}