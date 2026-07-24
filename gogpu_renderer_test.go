//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris

package vtui

import (
	"testing"
)

func TestGogpuRenderer_CursorDirtyOnStateChange(t *testing.T) {
	r := NewGogpuRenderer(nil, nil, 8, 16)
	r.dirty = false

	// Смена позиции курсора должна взводить флаг dirty для обхода раннего выхода
	r.SetCursor(5, 5, true, CursorShapeUnderline)
	if !r.dirty {
		t.Error("GogpuRenderer: expected dirty to be true after cursor position change")
	}
	r.dirty = false

	// Смена формы курсора (Ins/Ovr) также должна помечать буфер грязным
	r.SetCursor(5, 5, true, CursorShapeBlock)
	if !r.dirty {
		t.Error("GogpuRenderer: expected dirty to be true after cursor shape change")
	}
}
