package vtui

import (
	"testing"
)

func TestScrollBarMath(t *testing.T) {
	// Verify scrollbar thumb positioning logic
	// Scrollbar length 12 (track 10). 100 Elements.
	length := 12
	total := 100

	// Start of the list (top = 0)
	caretPos, caretLen := CalcScrollBar(length, 0, total)
	if caretPos != 0 {
		t.Errorf("Expected caret at 0, got %d", caretPos)
	}
	if caretLen != 1 {
		t.Errorf("Expected caret length 1, got %d", caretLen)
	}

	// Middle of the list (top = 44)
	// maxTop = 100 - 12 = 88. 44 is exactly half.
	// maxCaret = 10 - 1 = 9. Half is 4.5 -> 5
	caretPos, _ = CalcScrollBar(length, 44, total)
	if caretPos != 5 {
		t.Errorf("Expected caret at 5, got %d", caretPos)
	}

	// End of the list (top = 88)
	caretPos, _ = CalcScrollBar(length, 88, total)
	if caretPos != 9 {
		t.Errorf("Expected caret at 9, got %d", caretPos)
	}
}

func TestDrawScrollBar(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 10)

	attr := Palette[ColTableBox]

	// Draw scrollbar at X=5, Y=2, length 6. 20 items, at 0.
	drawn := DrawScrollBar(scr, 5, 2, 6, 0, 20, attr)

	if !drawn {
		t.Fatal("Scrollbar should have been drawn")
	}

	// Y=2 -> Top arrow
	checkCell(t, scr, 5, 2, ScrollUpArrow, attr)

	// Y=3 -> Thumb (dark block), since we are at the beginning
	checkCell(t, scr, 5, 3, ScrollBlockDark, attr)

	// Y=4..6 -> Track (light block)
	checkCell(t, scr, 5, 4, ScrollBlockLight, attr)

	// Y=7 -> Bottom arrow
	checkCell(t, scr, 5, 7, ScrollDownArrow, attr)
}

func TestDrawScrollBar_EdgeCases(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 10)
	attr := uint64(1)

	// 1. Length less than 3 (cannot draw)
	if DrawScrollBar(scr, 0, 0, 2, 0, 10, attr) {
		t.Error("Should not draw scrollbar if length <= 2")
	}

	// 2. No items
	if DrawScrollBar(scr, 0, 0, 5, 0, 0, attr) {
		t.Error("Should not draw scrollbar if 0 items")
	}

	// 3. Items fewer than length (not needed)
	if DrawScrollBar(scr, 0, 0, 10, 0, 5, attr) {
		t.Error("Should not draw scrollbar if length >= items")
	}

	// 4. Drawing thumb at the very end
	// Track = 4 (length 6-2). Total 100 items.
	drawn := DrawScrollBar(scr, 0, 0, 6, 95, 100, attr)
	if !drawn {
		t.Fatal("Scrollbar should have been drawn")
	}
	// Top - Y=0 (Arrow)
	// Y=1, Y=2, Y=3 - Light track
	// Y=4 - Dark track (bottom of the thumb)
	checkCell(t, scr, 0, 4, ScrollBlockDark, attr)
}

func TestMathRound_DivByZero(t *testing.T) {
	res := MathRound(10, 0)
	if res != 0 {
		t.Errorf("MathRound with div 0 expected 0, got %d", res)
	}
}