package vtui

import "testing"

func TestSetDefaultPalette(t *testing.T) {
	// Сбрасываем палитру, чтобы убедиться, что функция её заполняет
	Palette = [LastPaletteColor]uint64{}

	SetDefaultPalette()

	// Проверяем, что базовый индекс не остался нулевым
	if Palette[ColMenuText] == 0 {
		t.Error("Palette was not initialized correctly")
	}

	// Проверяем конкретный цвет (MenuText должен быть White on Cyan)
	// Cyan = 0x00A0A0, White = 0xFFFFFF
	expectedMenuText := SetRGBBoth(0, 0xFFFFFF, 0x00A0A0)
	if Palette[ColMenuText] != expectedMenuText {
		t.Errorf("Expected MenuText color %X, got %X", expectedMenuText, Palette[ColMenuText])
	}

	// Проверяем цвет панелей (LightCyan on Blue)
	// Blue = 0x0000A0, LightCyan = 0x00FFFF
	expectedPanelText := SetRGBBoth(0, 0x00FFFF, 0x0000A0)
	if Palette[ColPanelText] != expectedPanelText {
		t.Errorf("Expected PanelText color %X, got %X", expectedPanelText, Palette[ColPanelText])
	}
}