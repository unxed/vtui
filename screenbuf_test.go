package vtui

import (
	"testing"
	"os"
)

func TestAttributesToANSI(t *testing.T) {
	// 1. Simple Bold + Index Red
	attr := ForegroundIntensity | SetIndexFore(0, 9)
	got := attributesToANSI(attr, 0, nil, ColorProfileTrueColor, nil)
	// Expected: 1 (Bold) and 38;5;9 as separate chunks
	want := "\x1b[1m\x1b[38;5;9m"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// 2. TrueColor mapping (when ColorProfile is TrueColor)
	orange := uint32(0xFF8700)
	attrTC := SetRGBFore(0, orange)
	gotTC := attributesToANSI(attrTC, 0, nil, ColorProfileTrueColor, nil)
	wantTC := "\x1b[38;2;255;135;0m"
	if gotTC != wantTC {
		t.Errorf("TrueColor fallback: got %q, want %q", gotTC, wantTC)
	}

	// 3. Flag removal (Reset)
	attr1 := CommonLvbUnderscore
	attr2 := SetIndexFore(0, 4)
	gotReset := attributesToANSI(attr2, attr1, nil, ColorProfileTrueColor, nil)
	// attr1 has underscore, attr2 does NOT. Should trigger reset '0'.
	if gotReset[:4] != "\x1b[0m" {
		t.Errorf("Reset expected, got %q", gotReset)
	}
}
func TestAttributesToANSI_ResetBug(t *testing.T) {
	// Simulate transition: (Bold + Black FG + Cyan BG) -> (Normal + Black FG + Cyan BG)
	// Index 0 is Black. Removing Bold triggers an SGR 0 (reset).
	attr1 := ForegroundIntensity | SetIndexBoth(0, 0, 3)
	attr2 := SetIndexBoth(0, 0, 3)

	got := attributesToANSI(attr2, attr1, nil, ColorProfileTrueColor, nil)

	// Since we trigger a reset, the terminal forgets the Foreground color.
	// We MUST emit the Foreground color (38;5;0) again even though it numerically matches lastAttr=0.
	if !contains(got, "38;5;0") {
		t.Errorf("Foreground color missing after reset! Got: %q", got)
	}
}

func TestAttributesToANSI_FullSplitting(t *testing.T) {
	// Verify that style, foreground, and background are all split into separate escape sequences.
	// This is critical for FreeBSD console compatibility.
	attr := ForegroundIntensity | SetIndexFore(0, 1) | SetIndexBack(0, 2)
	got := attributesToANSI(attr, 0, nil, ColorProfileTrueColor, nil)

	// We expect three separate \x1b[...]m blocks
	want := "\x1b[1m\x1b[38;5;1m\x1b[48;5;2m"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDetectColorProfile_FreeBSD(t *testing.T) {
	// Clean environment
	os.Unsetenv("DISPLAY")
	os.Unsetenv("SSH_CLIENT")
	os.Unsetenv("TMUX")
	os.Unsetenv("WAYLAND_DISPLAY")
	os.Unsetenv("COLORTERM")
	os.Setenv("TERM", "xterm-256color")

	// 1. Bare FreeBSD console should force 16 colors
	if p := detectColorProfile("freebsd"); p != ColorProfile16 {
		t.Errorf("Bare FreeBSD console: expected 16 colors, got %v", p)
	}

	// 2. FreeBSD under TMUX should allow 256 colors
	os.Setenv("TMUX", "/tmp/tmux-1000/default,123,0")
	if p := detectColorProfile("freebsd"); p != ColorProfile256 {
		t.Errorf("FreeBSD under TMUX: expected 256 colors, got %v", p)
	}

	// 3. FreeBSD under SSH should allow 256 colors
	os.Unsetenv("TMUX")
	os.Setenv("SSH_CLIENT", "1.2.3.4 1234 22")
	if p := detectColorProfile("freebsd"); p != ColorProfile256 {
		t.Errorf("FreeBSD under SSH: expected 256 colors, got %v", p)
	}
}

func TestScreenBuf_OverlayMode(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(5, 5)

	// Setup a custom theme palette
	var theme [256]uint32
	theme[5] = 0x112233 // Arbitrary RGB color mapped to index 5
	scr.ThemePalette = &theme

	attrIndex := SetIndexFore(0, 5)

	// 1. OverlayMode = false -> Early Binding should NOT happen
	scr.SetOverlayMode(false)
	scr.Write(0, 0, StringToCharInfo("A", attrIndex))
	cell1 := scr.GetCell(0, 0)
	if cell1.Attributes&IsFgRGB != 0 {
		t.Error("OverlayMode=false should keep index (IsFgRGB must be false)")
	}

	// 2. OverlayMode = true -> Early Binding SHOULD happen
	scr.SetOverlayMode(true)
	scr.Write(1, 0, StringToCharInfo("B", attrIndex))
	cell2 := scr.GetCell(1, 0)
	if cell2.Attributes&IsFgRGB == 0 {
		t.Error("OverlayMode=true should convert index to RGB (IsFgRGB must be true)")
	}
	if GetRGBFore(cell2.Attributes) != 0x112233 {
		t.Errorf("OverlayMode=true should use ThemePalette, got %X", GetRGBFore(cell2.Attributes))
	}
}

func TestScreenBuf_Quantization(t *testing.T) {
	var pal [256]uint32
	pal[10] = 0xFF0000 // Pure Red
	pal[20] = 0x00FF00 // Pure Green

	// RGB color that is close to red, but not exactly
	rgbAttr := SetRGBFore(0, 0xEE0000)

	// Quantization requested (ColorProfile256)
	quantCache := make(map[uint32]uint8)
	ansi := colorToANSI(false, rgbAttr, &pal, ColorProfile256, quantCache)

	// Should quantize to index 10 (the closest match in our dummy palette)
	want := "38;5;10"
	if !contains(ansi, want) {
		t.Errorf("Quantization failed. Expected to contain %q, got %q", want, ansi)
	}

	// Make sure the cache was populated
	if quantCache[0xEE0000] != 10 {
		t.Error("Quantization cache was not updated")
	}
}

func TestScreenBuf_16ColorProfile(t *testing.T) {
	// RGB color
	rgbAttr := SetRGBFore(0, 0xFF0000)
	quantCache := make(map[uint32]uint8)
	ansi := colorToANSI(false, rgbAttr, nil, ColorProfile16, quantCache)

	// In 16-color profile, Red is index 1 (or 9 for bright red).
	// The 16-color fallback for index 9 should be "91" (90 + 1).
	if !contains(ansi, "91") && !contains(ansi, "31") {
		t.Errorf("16-color profile failed for foreground. Got %q", ansi)
	}

	// Background color (e.g. index 4 - Blue)
	bgAttr := SetIndexBack(0, 4)
	ansiBg := colorToANSI(true, bgAttr, nil, ColorProfile16, quantCache)
	if !contains(ansiBg, "44") {
		t.Errorf("16-color profile failed for background index. Expected 44, got %q", ansiBg)
	}
}

func TestScreenBuf_ColorTransitions(t *testing.T) {
	// Check transition from TrueColor to indexed palette
	tcAttr := SetRGBFore(0, 0xFF0000)
	palAttr := SetIndexFore(0, 4) // Regular blue index

	got := attributesToANSI(palAttr, tcAttr, nil, ColorProfileTrueColor, nil)

	// Since we changed color type (TrueColor -> Index), explicit code 38;5;4 must be triggered.
	if !contains(got, "38;5;4") {
		t.Errorf("Transition to palette failed, ANSI: %q", got)
	}
}

func TestAttributesToANSI_Styles(t *testing.T) {
	// Bold + Strikeout
	attr := ForegroundIntensity | CommonLvbStrikeout
	got := attributesToANSI(attr, 0, nil, ColorProfileTrueColor, nil)

	// Note: result might vary depending on whether we treat 0 as having black/black or no color.
	// But let's verify flags at least.
	if !contains(got, "1") || !contains(got, "9") {
		t.Errorf("Styles missing in %q", got)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestScreenBuf_Clipping(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 10)
	attr := uint64(111)

	// Default clip should be the whole screen
	scr.Write(0, 0, StringToCharInfo("ABC", attr))
	checkCell(t, scr, 0, 0, 'A', attr)

	// Push a clip rect (5, 5) to (10, 10)
	scr.PushClipRect(5, 5, 10, 10)

	// Try to write outside (left)
	scr.Write(2, 5, StringToCharInfo("HELLO", attr))
	// 'H', 'E', 'L' should be clipped. 'L', 'O' should be printed at 5 and 6
	checkCell(t, scr, 2, 5, 0, 0)
	checkCell(t, scr, 5, 5, 'L', attr)
	checkCell(t, scr, 6, 5, 'O', attr)

	// Try to write outside (right)
	scr.Write(8, 6, StringToCharInfo("WORLD", attr))
	// 'W', 'O', 'R' should be at 8, 9, 10. 'L', 'D' should be clipped
	checkCell(t, scr, 8, 6, 'W', attr)
	checkCell(t, scr, 10, 6, 'R', attr)
	checkCell(t, scr, 11, 6, 0, 0)

	// Try to fill rect crossing bounds
	scr.FillRect(2, 7, 15, 8, 'X', attr)
	checkCell(t, scr, 4, 7, 0, 0)
	checkCell(t, scr, 5, 7, 'X', attr)
	checkCell(t, scr, 10, 7, 'X', attr)
	checkCell(t, scr, 11, 7, 0, 0)

	// Pop clip rect
	scr.PopClipRect()

	// Now we can write outside again
	scr.Write(0, 9, StringToCharInfo("END", attr))
	checkCell(t, scr, 0, 9, 'E', attr)
}

func TestScreenBuf_ApplyShadow_Clipping(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 10)
	
	// Fill with white
	white := SetRGBBoth(0, 0xFFFFFF, 0xFFFFFF)
	scr.FillRect(0, 0, 9, 9, ' ', white)
	
	// Set clip rect to top half
	scr.PushClipRect(0, 0, 9, 4)
	
	// Apply shadow to bottom half (should be clipped and do nothing)
	scr.ApplyShadow(0, 5, 9, 9)
	
	// Cell at (5,5) must still be white
	if scr.GetCell(5, 5).Attributes != white {
		t.Error("ApplyShadow was not clipped correctly")
	}
	
	// Apply shadow partially inside clip
	scr.ApplyShadow(0, 0, 9, 9)
	
	// Cell at (2,2) should be darker than white
	if GetRGBFore(scr.GetCell(2, 2).Attributes) >= 0xFFFFFF {
		t.Error("Shadow not applied inside clip rect")
	}
}
func TestScreenBuf_WidthHeightConcurrency(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)

	done := make(chan bool)
	go func() {
		for i := 0; i < 1000; i++ {
			scr.AllocBuf(100+i, 30+i)
		}
		done <- true
	}()

	for i := 0; i < 1000; i++ {
		_ = scr.Width()
		_ = scr.Height()
	}

	<-done
}
