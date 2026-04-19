package vtui

import (
	"testing"
)

func TestX11Host_DirtySpanLogic(t *testing.T) {
	// Мы не можем запустить реальный X-сервер, но можем проверить логику
	// отслеживания грязных строк напрямую.
	h := &X11Host{
		dirtyLines: make([]bool, 100),
	}

	// Помечаем две разрозненные группы строк
	h.dirtyLines[10] = true
	h.dirtyLines[11] = true
	h.dirtyLines[12] = true

	h.dirtyLines[50] = true
	h.dirtyLines[51] = true

	// Проверяем, как flushImage (в теории) должен их обходить.
	// Здесь мы имитируем цикл из flushImage.
	type span struct{ start, rows int }
	var spans []span

	for y := 0; y < 100; {
		if !h.dirtyLines[y] {
			y++
			continue
		}
		start := y
		end := y
		for end < 100 && h.dirtyLines[end] {
			end++
		}
		spans = append(spans, span{start, end - start})
		y = end
	}

	if len(spans) != 2 {
		t.Fatalf("Expected 2 dirty spans, got %d", len(spans))
	}

	if spans[0].start != 10 || spans[0].rows != 3 {
		t.Errorf("Span 1 mismatch: %+v", spans[0])
	}
	if spans[1].start != 50 || spans[1].rows != 2 {
		t.Errorf("Span 2 mismatch: %+v", spans[1])
	}
}

func TestX11Host_ModifierTranslation(t *testing.T) {
	h := &X11Host{}
	// ModMaskControl = 4, ModMask2 (NumLock) = 16
	mods := h.translateModifiers(4 | 16)

	const LeftCtrlPressed = 0x0008
	const NumLockOn = 0x0020

	if (mods & LeftCtrlPressed) == 0 {
		t.Error("Failed to translate Control modifier")
	}
	if (mods & NumLockOn) == 0 {
		t.Error("Failed to translate NumLock modifier (Mod2)")
	}
}