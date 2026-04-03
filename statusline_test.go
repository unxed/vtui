package vtui

import "testing"

func TestStatusLine_Rendering(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(40, 1)

	sl := NewStatusLine()
	sl.Default = []StatusItem{
		{Key: "F1", Label: "Help"},
		{Key: "Alt-X", Label: "Exit"},
	}
	sl.SetPosition(0, 0, 39, 0)
	sl.Show(scr)

	// Check rendering of 'F1'
	checkCell(t, scr, 0, 0, 'F', Palette[ColKeyBarNum])
	checkCell(t, scr, 1, 0, '1', Palette[ColKeyBarNum])

	// Check rendering of 'Help'
	checkCell(t, scr, 2, 0, 'H', Palette[ColKeyBarText])
}

func TestStatusLine_ContextUpdate(t *testing.T) {
	sl := NewStatusLine()
	sl.Items["Dialog"] = []StatusItem{{Key: "Enter", Label: "OK"}}
	sl.UpdateContext("Dialog")

	if sl.currentTopic != "Dialog" {
		t.Errorf("Expected topic 'Dialog', got '%s'", sl.currentTopic)
	}
}

func TestStatusLine_Truncation(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 1) // Very narrow screen

	sl := NewStatusLine()
	sl.Default = []StatusItem{
		{Key: "F1", Label: "Very Long Help Label That Should Be Truncated"},
	}
	sl.SetPosition(0, 0, 9, 0)

	// Should not panic
	sl.Show(scr)

	// Check that at least the start is visible
	checkCell(t, scr, 0, 0, 'F', Palette[ColKeyBarNum])
}
