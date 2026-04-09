package vtui

import "testing"

func TestProgressBar_Rendering(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 1)

	pb := NewProgressBar(0, 0, 10)
	pb.SetVisible(true)

	// 1. Test 50% on width 10 (should be 5 blocks)
	pb.Percent = 50
	pb.Show(scr)

	colFill := Palette[ColDialogEdit]
	colEmpty := Palette[ColDialogText]

	for x := 0; x < 5; x++ {
		checkCell(t, scr, x, 0, '█', colFill)
	}
	for x := 5; x < 10; x++ {
		checkCell(t, scr, x, 0, '░', colEmpty)
	}

	// 2. Test Clamping
	pb.SetPercent(150)
	if pb.Percent != 100 {
		t.Errorf("Expected clamping to 100, got %d", pb.Percent)
	}
	pb.SetPercent(-50)
	if pb.Percent != 0 {
		t.Errorf("Expected clamping to 0, got %d", pb.Percent)
	}
}