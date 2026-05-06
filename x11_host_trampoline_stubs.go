//go:build !darwin || !arm64

package vtui

var (
	trampolineXGetIMValuesAddr uintptr = 0
	trampolineXCreateICAddr    uintptr = 0
)
