package vtui

import "testing"

func TestText_Truncation(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 1)

	// Width is 5 (0 to 4)
	txt := NewText(0, 0, "Longer than five", 0)
	txt.X2 = 4
	txt.SetVisible(true)

	txt.Show(scr)

	// Check that only "Longe" is written
	checkCell(t, scr, 0, 0, 'L', Palette[ColDialogText])
	checkCell(t, scr, 4, 0, 'e', Palette[ColDialogText])

	// Cell at X=5 should be empty (zero-char)
	if cell := scr.GetCell(5, 0); cell.Char != 0 {
		t.Errorf("Text overflow: expected empty cell at X=5, got %c", rune(cell.Char))
	}
}