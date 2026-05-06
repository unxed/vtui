//go:build darwin && arm64

package vtui

import "reflect"

// Declare functions implemented in assembly
func trampolineXGetIMValues()
func trampolineXCreateIC()

// Retrieve their memory addresses for purego invocation
var (
	trampolineXGetIMValuesAddr = reflect.ValueOf(trampolineXGetIMValues).Pointer()
	trampolineXCreateICAddr    = reflect.ValueOf(trampolineXCreateIC).Pointer()
)
