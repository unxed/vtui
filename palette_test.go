package vtui

import "testing"

func TestSetDefaultPalette(t *testing.T) {
	// Reset palette to ensure the function fills it
	Palette = make([]uint64, LastPaletteColor)

	SetDefaultPalette()

	// Check that the base index didn't remain zero
	if Palette[ColMenuText] == 0 {
		t.Error("Palette was not initialized correctly")
	}

	// Check specific color (MenuText should be White on Cyan)
	// Cyan = 0x00A0A0, White = 0xFFFFFF
	expectedMenuText := SetRGBBoth(0, 0xFFFFFF, 0x00A0A0)
	if Palette[ColMenuText] != expectedMenuText {
		t.Errorf("Expected MenuText color %X, got %X", expectedMenuText, Palette[ColMenuText])
	}

	// Check highlight colors are initialized
	if Palette[ColDialogHighlightText] == 0 {
		t.Error("ColDialogHighlightText was not initialized")
	}
	if Palette[ColDialogHighlightButton] == 0 {
		t.Error("ColDialogHighlightButton was not initialized")
	}

	// Check table color (LightGray on Blue)
	// Blue = 0x0000A0, LightGray = 0xC0C0C0
	expectedTableText := SetRGBBoth(0, 0xC0C0C0, 0x0000A0)
	if Palette[ColTableText] != expectedTableText {
		t.Errorf("Expected TableText color %X, got %X", expectedTableText, Palette[ColTableText])
	}
}