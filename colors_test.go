package vtui

import "testing"

func TestColors_IndexAndRGB(t *testing.T) {
	// Test 1: SetIndexFore
	attr := uint64(0)
	attr = SetIndexFore(attr, 42)
	if attr&IsFgRGB != 0 {
		t.Error("SetIndexFore should not set IsFgRGB flag")
	}
	if GetIndexFore(attr) != 42 {
		t.Errorf("GetIndexFore expected 42, got %d", GetIndexFore(attr))
	}

	// Test 2: SetRGBFore overwrites index and sets flag
	attr = SetRGBFore(attr, 0xAABBCC)
	if attr&IsFgRGB == 0 {
		t.Error("SetRGBFore should set IsFgRGB flag")
	}
	if GetRGBFore(attr) != 0xAABBCC {
		t.Errorf("GetRGBFore expected AABBCC, got %X", GetRGBFore(attr))
	}

	// Test 3: SetIndexBack clears IsBgRGB
	attr = SetRGBBack(attr, 0x112233)
	attr = SetIndexBack(attr, 7)
	if attr&IsBgRGB != 0 {
		t.Error("SetIndexBack should clear IsBgRGB flag")
	}
	if GetIndexBack(attr) != 7 {
		t.Errorf("GetIndexBack expected 7, got %d", GetIndexBack(attr))
	}

	// Test 4: SetIndexBoth
	attr = SetIndexBoth(0, 5, 6)
	if GetIndexFore(attr) != 5 || GetIndexBack(attr) != 6 {
		t.Error("SetIndexBoth failed")
	}
	if attr&IsFgRGB != 0 || attr&IsBgRGB != 0 {
		t.Error("SetIndexBoth should not set RGB flags")
	}
}

func TestDimColor(t *testing.T) {
	// RGB Test
	rgbAttr := SetRGBFore(0, 0xAA6622)
	dimmed := DimColor(rgbAttr)
	if GetRGBFore(dimmed) != 0x553311 {
		t.Errorf("DimColor RGB failed, got %X", GetRGBFore(dimmed))
	}

	// With an RGB background the foreground fades into it, so dark text on a
	// light background gets lighter instead of staying as dark as it was.
	onLight := SetRGBBack(SetRGBFore(0, 0x2E3436), 0xD3D7CF)
	if got := GetRGBFore(DimColor(onLight)); got != 0x808582 {
		t.Errorf("DimColor over a light background = %06X, want 808582", got)
	}
	if got := GetRGBFore(DimColor(SetRGBBack(SetRGBFore(0, 0xEEEEEC), 0x333333))); got != 0x90908F {
		t.Errorf("DimColor over a dark background = %06X, want 90908F", got)
	}
	// Over a DarkGray background DarkGray would vanish.
	if got := GetIndexFore(DimColor(SetIndexBoth(0, 15, 8))); got != 7 {
		t.Errorf("DimColor over DarkGray = %d, want 7", got)
	}

	// Index Test (ANSI fallback)
	idxAttr := SetIndexFore(0, 15) // White
	dimmedIdx := DimColor(idxAttr)
	if GetIndexFore(dimmedIdx) != 8 { // Should become DarkGray (8)
		t.Errorf("DimColor Index failed, got %d", GetIndexFore(dimmedIdx))
	}
}

func TestDefaultColorAttributes(t *testing.T) {
	base := SetIndexBoth(0, 2, 4)
	both := SetDefaultBack(SetDefaultFore(base))
	if both&ForegroundDefault == 0 || both&BackgroundDefault == 0 {
		t.Fatalf("default flags not set: %#x", both)
	}
	// Readers that cannot express "default" keep seeing an ordinary index.
	if GetIndexFore(both) != 7 || GetIndexBack(both) != 0 || both&(IsFgRGB|IsBgRGB) != 0 {
		t.Errorf("default attr reads as fg %d bg %d", GetIndexFore(both), GetIndexBack(both))
	}
	// Any other colour setter leaves "default".
	if a := SetIndexFore(both, 3); a&ForegroundDefault != 0 || a&BackgroundDefault == 0 {
		t.Errorf("SetIndexFore: %#x", a)
	}
	if a := SetRGBBack(both, 0x102030); a&BackgroundDefault != 0 || a&ForegroundDefault == 0 {
		t.Errorf("SetRGBBack: %#x", a)
	}
	if a := SetRGBBoth(both, 1, 2); a&(ForegroundDefault|BackgroundDefault) != 0 {
		t.Errorf("SetRGBBoth: %#x", a)
	}
	// Inversion moves the flag with its colour.
	inv := InvertColors(SetDefaultBack(SetIndexFore(0, 5)))
	if inv&ForegroundDefault == 0 || inv&BackgroundDefault != 0 || GetIndexBack(inv) != 5 {
		t.Errorf("InvertColors: %#x", inv)
	}
}

func TestDefaultColorANSI(t *testing.T) {
	def := SetDefaultBack(SetDefaultFore(0))
	for _, profile := range []ColorProfile{ColorProfile16, ColorProfile256, ColorProfileTrueColor} {
		if got := attributesToANSI(def, SetIndexBoth(0, 1, 2), nil, profile, nil); got != "\x1b[39;49m" {
			t.Errorf("profile %v: default over index = %q", profile, got)
		}
		if got := attributesToANSI(SetIndexBoth(0, 1, 2), def, nil, profile, nil); got == "" || got == "\x1b[39;49m" {
			t.Errorf("profile %v: index over default = %q, want explicit colours", profile, got)
		}
		if got := attributesToANSI(def, def, nil, profile, nil); got != "" {
			t.Errorf("profile %v: unchanged default = %q", profile, got)
		}
	}
	// Only the background changes to default.
	if got := attributesToANSI(SetDefaultBack(SetIndexFore(0, 1)), SetIndexBoth(0, 1, 2), nil, ColorProfileTrueColor, nil); got != "\x1b[49m" {
		t.Errorf("background alone = %q", got)
	}
}
