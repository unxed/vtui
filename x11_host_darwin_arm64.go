//go:build darwin && arm64

package vtui

import "reflect"

// Объявляем функции, реализованные в ассемблере
func trampolineXGetIMValues()
func trampolineXCreateIC()

// Получаем их адреса в памяти для передачи в purego
var (
	trampolineXGetIMValuesAddr = reflect.ValueOf(trampolineXGetIMValues).Pointer()
	trampolineXCreateICAddr    = reflect.ValueOf(trampolineXCreateIC).Pointer()
)
