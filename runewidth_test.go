package vtui

import (
	"testing"
)

func TestStringToCharInfo_WideChars(t *testing.T) {
	// "A世B" -> A (1), 世 (2), B (1) -> Total visual width: 4
	str := "A世B"
	attr := uint64(123)

	ci := StringToCharInfo(str, attr)

	if len(ci) != 4 {
		t.Fatalf("Expected 4 cells, got %d", len(ci))
	}

	if ci[0].Char != 'A' {
		t.Errorf("Cell 0: expected 'A', got %c", ci[0].Char)
	}
	if ci[1].Char != '世' {
		t.Errorf("Cell 1: expected '世', got %c", ci[1].Char)
	}
	if ci[2].Char != WideCharFiller {
		t.Errorf("Cell 2: expected WideCharFiller, got %X", ci[2].Char)
	}
	if ci[3].Char != 'B' {
		t.Errorf("Cell 3: expected 'B', got %c", ci[3].Char)
	}
}

func TestParseAmpersandString(t *testing.T) {
	clean, hk, pos := ParseAmpersandString("Save &As && Exit")
	if clean != "Save As & Exit" {
		t.Errorf("Clean string mismatch: got %q", clean)
	}
	if hk != 'a' {
		t.Errorf("Hotkey mismatch: got %c", hk)
	}
	if pos != 5 { // 'S', 'a', 'v', 'e', ' ', 'A' -> pos 5
		t.Errorf("Hotkey pos mismatch: got %d", pos)
	}
}

func TestParseAmpersandString_DoubleAmpersand(t *testing.T) {
	// && должен превращаться в одиночный &, не создавая хоткея
	clean, hk, pos := ParseAmpersandString("Fish && &Chips")

	if clean != "Fish & Chips" {
		t.Errorf("Double ampersand failed: expected 'Fish & Chips', got %q", clean)
	}

	if hk != 'c' {
		t.Errorf("Expected hotkey 'c', got %c", hk)
	}

	// 'F','i','s','h',' ','&',' ','C' -> индекс 'C' это 7
	if pos != 7 {
		t.Errorf("Wrong hotkey position: expected 7, got %d", pos)
	}
}
func TestExtractHotkey(t *testing.T) {
	tests := []struct {
		input    string
		expected rune
	}{
		{"Save &As && Exit", 'a'},
		{"Fish && &Chips", 'c'},
		{"No Hotkey", 0},
		{"&Start", 's'},
		{"Trailing &", 0},
		{"&&", 0},
	}

	for _, tt := range tests {
		got := ExtractHotkey(tt.input)
		if got != tt.expected {
			t.Errorf("ExtractHotkey(%q): expected %c, got %c", tt.input, tt.expected, got)
		}
	}
}
