//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris && !plan9 && !android && (amd64 || arm64)

package vtui

import (
	"testing"

	"github.com/gogpu/gg/text"
)

func TestGogpuBatchRune(t *testing.T) {
	cases := []struct {
		name string
		ch   uint64
		want bool
	}{
		{"ascii", 'a', true},
		{"cyrillic", uint64('ф'), true},
		{"wide emoji", '🚂', true},
		{"space", ' ', false},
		{"empty", 0, false},
		{"filler", WideCharFiller, false},
		{"compound cluster", RegisterCluster("👨‍👩‍👧‍👦"), false},
		{"lone regional indicator", uint64(0x1F1E6), false},
		{"box drawing", uint64('│'), false},
	}
	for _, tc := range cases {
		if got := gogpuBatchRune(tc.ch); got != tc.want {
			t.Errorf("%s: gogpuBatchRune(%X) = %v, want %v", tc.name, tc.ch, got, tc.want)
		}
	}
}

func TestGogpuTextRun(t *testing.T) {
	r := &GogpuRenderer{} // nil face: every rune resolves to the same face
	family := RegisterCluster("👨‍👩‍👧‍👦")
	cases := []struct {
		name      string
		cells     []CharInfo
		wantText  string
		wantCons  int
		wantBatch bool
	}{
		{"plain ascii", []CharInfo{{Char: 'a'}, {Char: 'b'}, {Char: 'c'}}, "abc", 3, true},
		{"wide emoji run", []CharInfo{{Char: '🚂'}, {Char: WideCharFiller}, {Char: '🐐'}, {Char: WideCharFiller}}, "🚂🐐", 4, true},
		{"wide then plain", []CharInfo{{Char: '🚂'}, {Char: WideCharFiller}, {Char: 'x'}}, "🚂x", 3, true},
		{"compound breaks", []CharInfo{{Char: 'a'}, {Char: family}, {Char: 'b'}}, "a", 1, true},
		{"space breaks", []CharInfo{{Char: 'a'}, {Char: ' '}, {Char: 'b'}}, "a", 1, true},
		{"regional breaks", []CharInfo{{Char: uint64(0x1F1E6)}, {Char: uint64(0x1F1E7)}}, string([]rune{0x1F1E6}), 1, true},
		{"box breaks", []CharInfo{{Char: 'a'}, {Char: '│'}, {Char: 'b'}}, "a", 1, true},
		{"empty stops", []CharInfo{{Char: 'a'}, {Char: 0}, {Char: 'b'}}, "a", 1, true},
	}
	for _, tc := range cases {
		var run []byte
		var consumed int
		var f text.Face
		run, consumed, f, batched := r.gogpuTextRun(tc.cells, 0, 0, 0, len(tc.cells), len(tc.cells), f, run)
		if got := string(run); got != tc.wantText {
			t.Errorf("%s: run text = %q, want %q", tc.name, got, tc.wantText)
		}
		if consumed != tc.wantCons {
			t.Errorf("%s: consumed = %d cells, want %d", tc.name, consumed, tc.wantCons)
		}
		if batched != tc.wantBatch {
			t.Errorf("%s: batched = %v, want %v", tc.name, batched, tc.wantBatch)
		}
	}
}

// TestGogpuAdvFits covers the drift gate: advances of exactly one cell (two
// for wide cells) join; everything else — measured emoji/CJK fallback
// advances (~1.8-2.5 cells), subpixel advances, proportional glyphs — stays
// per-cell.
func TestGogpuAdvFits(t *testing.T) {
	cases := []struct {
		name  string
		adv   float64
		cellW int
		cells int
		want  bool
	}{
		{"monospace latin", 13.0, 13, 1, true},
		{"wide monospace", 26.0, 13, 2, true},
		{"emoji fallback drift", 33.0, 13, 2, false},
		{"cjk fallback drift", 24.0, 13, 2, false},
		{"proportional narrow", 5.0, 16, 1, false},
		{"proportional wide", 23.0, 16, 1, false},
		{"subpixel advance", 12.96875, 13, 1, false},
		{"exact only", 12.9999, 13, 1, false},
	}
	for _, tc := range cases {
		if got := gogpuAdvFits(tc.adv, tc.cellW, tc.cells); got != tc.want {
			t.Errorf("%s: gogpuAdvFits(%v, %d, %d) = %v, want %v", tc.name, tc.adv, tc.cellW, tc.cells, got, tc.want)
		}
	}
}

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
func TestGogpuRenderer_Flush(t *testing.T) {
	host := &GogpuHost{}
	host.resizePending = true

	r := NewGogpuRenderer(host, nil, 8, 16)
	r.dirty = false

	r.Flush()

	// After Flush:
	// 1. host.resizePending should be false
	if host.resizePending {
		t.Error("GogpuRenderer.Flush: expected host.resizePending to be false")
	}

	// 2. r.dirty should be true (because of forceDirty/resizePending)
	r.mu.Lock()
	dirty := r.dirty
	r.mu.Unlock()
	if !dirty {
		t.Error("GogpuRenderer.Flush: expected r.dirty to be true because of resizePending")
	}
}

func TestGogpuRenderer_CursorShapeState(t *testing.T) {
	host := &GogpuHost{}
	r := NewGogpuRenderer(host, nil, 8, 16)

	r.SetCursor(1, 2, true, CursorShapeBlock)
	if r.cursorShape != CursorShapeBlock {
		t.Errorf("Expected CursorShapeBlock, got %v", r.cursorShape)
	}

	r.SetCursor(1, 2, true, CursorShapeUnderline)
	if r.cursorShape != CursorShapeUnderline {
		t.Errorf("Expected CursorShapeUnderline, got %v", r.cursorShape)
	}
}
