package vtui

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestScreenBuf_CursorDirtyState(t *testing.T) {
	scr := NewScreenBuf()
	scr.AllocBuf(80, 25)

	// Начальное состояние
	scr.SetCursorPos(10, 10)
	scr.SetCursorVisible(true)
	scr.SetCursorShape(CursorShapeUnderline)

	if !scr.cursorDirty {
		t.Error("expected cursorDirty to be true after initial setup")
	}

	// Сбрасываем флаг (имитируем Flush)
	scr.cursorDirty = false

	// Проверяем, что изменение позиции взводит флаг
	scr.SetCursorPos(11, 10)
	if !scr.cursorDirty {
		t.Error("expected cursorDirty to be true after cursor position change")
	}
	scr.cursorDirty = false

	// Проверяем, что изменение видимости взводит флаг
	scr.SetCursorVisible(false)
	if !scr.cursorDirty {
		t.Error("expected cursorDirty to be true after cursor visibility change")
	}
	scr.cursorDirty = false

	// Проверяем, что изменение формы взводит флаг
	scr.SetCursorShape(CursorShapeBlock)
	if !scr.cursorDirty {
		t.Error("expected cursorDirty to be true after cursor shape change")
	}
}

func TestAttributesToANSI(t *testing.T) {
	// 1. Simple Bold + Index Red
	attr := ForegroundIntensity | SetIndexFore(0, 9)
	got := attributesToANSI(attr, 0, nil, ColorProfileTrueColor, nil)
	// Expected: Bold and 38;5;9 in one CSI
	want := "\x1b[1;38;5;9m"
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
	// attr1 has underscore, attr2 does NOT. Reset is emitted as the first
	// SGR parameter of the combined sequence, so it starts "\x1b[0;".
	if !strings.HasPrefix(gotReset, "\x1b[0;") {
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
	// Verify that style, foreground, and background are combined into a single CSI.
	attr := ForegroundIntensity | SetIndexFore(0, 1) | SetIndexBack(0, 2)
	got := attributesToANSI(attr, 0, nil, ColorProfileTrueColor, nil)

	// We expect one shared \x1b[...]m block (shorter than separate sequences,
	// identical terminal semantics)
	want := "\x1b[1;38;5;1;48;5;2m"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
func TestAnsiRenderer_SetWindowTitle(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 10)
	scr.Renderer = &AnsiRenderer{parent: scr}

	ansiRenderer, ok := scr.Renderer.(*AnsiRenderer)
	if !ok {
		t.Fatal("Renderer is not AnsiRenderer")
	}

	var buf bytes.Buffer
	scr.Writer = &buf
	ansiRenderer.SetWindowTitle("New Title")

	expected := "\x1b]0;New Title\x07"
	if !strings.Contains(buf.String(), expected) {
		t.Errorf("Expected window title sequence %q in output, got %q", expected, buf.String())
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

// blockingWriter stalls its first Write until release is closed, standing in
// for a terminal that has stopped draining its input.
type blockingWriter struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	<-w.release
	return len(p), nil
}

// TestFlush_DoesNotHoldScreenLockDuringWrite pins down the invariant behind
// the #429 investigation: a write to the terminal is unbounded in time (a pty
// nobody reads from blocks it indefinitely, and a full frame on a large
// terminal is far bigger than the pty buffer), so ScreenBuf.mu must not be
// held across it. Previously Flush took mu for its whole body, which turned a
// stalled terminal into a freeze of every goroutine touching the screen.
func TestFlush_DoesNotHoldScreenLockDuringWrite(t *testing.T) {
	scr := NewScreenBuf()
	w := &blockingWriter{entered: make(chan struct{}), release: make(chan struct{})}
	scr.Writer = w
	scr.AllocBuf(20, 5)

	flushed := make(chan struct{})
	go func() {
		scr.Flush()
		close(flushed)
	}()

	select {
	case <-w.entered:
	case <-time.After(5 * time.Second):
		close(w.release)
		t.Fatal("Flush never reached the write")
	}

	// The write is now stuck. Any other reader of the screen must still get
	// through.
	reached := make(chan struct{})
	go func() {
		scr.GetCell(0, 0)
		close(reached)
	}()

	select {
	case <-reached:
	case <-time.After(5 * time.Second):
		close(w.release)
		t.Fatal("GetCell blocked while a terminal write was in progress: " +
			"ScreenBuf.mu is being held across the write")
	}

	close(w.release)
	<-flushed
}

// TestInvalidateHostPalette_ForcesResend guards the other half of the same
// bug report: HostPalette/HostPaletteValid track the state of the terminal,
// not of this process. Suspend sends OSC 104 and the daemon re-attaches to
// terminals it has never painted, both of which drop the palette on the far
// end. Without invalidation SetPalette sees entries it believes are already
// loaded and sends nothing, leaving the session in the terminal's own colors.
func TestInvalidateHostPalette_ForcesResend(t *testing.T) {
	scr := NewScreenBuf()
	var buf bytes.Buffer
	scr.Writer = &buf
	scr.AllocBuf(8, 2)

	pal := XTerm256Palette
	scr.ThemePalette = &pal

	scr.Flush()
	if !strings.Contains(buf.String(), "\x1b]4;") {
		t.Fatal("first flush sent no OSC 4 sequences")
	}

	// Nothing changed on either side: the palette must not be resent.
	buf.Reset()
	scr.Flush()
	if strings.Contains(buf.String(), "\x1b]4;") {
		t.Error("palette resent although neither side changed")
	}

	buf.Reset()
	scr.InvalidateHostPalette()
	scr.Flush()
	if !strings.Contains(buf.String(), "\x1b]4;") {
		t.Error("palette not resent after InvalidateHostPalette")
	}
}

// TestAnsiRenderer_RelativeCursorMoves verifies that sparse cells on the
// same row are reached with short relative moves (CSI n C) instead of
// absolute "\x1b[Y;XH" positioning, and that the stream stays equivalent.
func TestAnsiRenderer_RelativeCursorMoves(t *testing.T) {
	r := &AnsiRenderer{parent: &ScreenBuf{ColorProfile: ColorProfileTrueColor}}
	w, h := 12, 1
	buf := make([]CharInfo, w*h)
	shadow := make([]CharInfo, w*h)
	for i := range buf {
		buf[i] = CharInfo{Char: 'a'}
		shadow[i] = buf[i]
	}
	attr := SetRGBBoth(0, 0xff0000, 0x0033ff)
	buf[0] = CharInfo{Char: 'X', Attributes: attr}
	buf[3] = CharInfo{Char: 'Y', Attributes: attr}
	buf[9] = CharInfo{Char: 'Z', Attributes: attr}

	r.Render(buf, shadow, w, h, false)
	got := r.frameOut.String()

	var want strings.Builder
	want.WriteString("\x1b[?2026h\x1b[?25l")
	want.WriteString("\x1b[1;1H")
	var lastAttr uint64 = ^uint64(0)
	writeAttributesToANSI(&want, attr, lastAttr, nil, ColorProfileTrueColor, nil)
	want.WriteString("X")
	want.WriteString("\x1b[2C") // col 1 -> 3
	// attr unchanged between X and Y: no new CSI
	want.WriteString("Y")
	want.WriteString("\x1b[5C") // col 4 -> 9
	want.WriteString("Z")
	if got != want.String() {
		t.Fatalf("render\ngot  %q\nwant %q", got, want.String())
	}
}

// TestAnsiRenderer_RuneWriting guards the allocation-free write path for
// plain runes (space, ASCII, non-ASCII and wide): WriteRune must emit the
// exact same UTF-8 as the CellString call it replaced. Composite cells
// still go through CellString.
func TestAnsiRenderer_RuneWriting(t *testing.T) {
	r := &AnsiRenderer{parent: &ScreenBuf{ColorProfile: ColorProfileTrueColor}}
	w, h := 5, 1
	buf := make([]CharInfo, w*h)
	shadow := make([]CharInfo, w*h)
	attr := SetRGBBoth(0, 0xff0000, 0x0033ff)
	cases := []uint64{0, uint64('a'), uint64('Ж'), uint64('中'), WideCharFiller}
	for i, ch := range cases {
		buf[i] = CharInfo{Char: ch, Attributes: attr}
		shadow[i] = CharInfo{Char: ch, Attributes: attr}
	}

	r.Render(buf, shadow, w, h, true)
	got := r.frameOut.String()

	var want strings.Builder
	want.WriteString("\x1b[?2026h\x1b[?25l")
	want.WriteString("\x1b[1;1H")
	writeAttributesToANSI(&want, attr, ^uint64(0), nil, ColorProfileTrueColor, nil)
	want.WriteString(" aЖ中")
	if got != want.String() {
		t.Fatalf("render\ngot  %q\nwant %q", got, want.String())
	}
}

func TestAnsiRenderer_FreeBSDConsoleAvoidsPrivateModesAndOSC(t *testing.T) {
	oldFreeBSDConsole := IsFreeBSDConsole
	IsFreeBSDConsole = true
	defer func() { IsFreeBSDConsole = oldFreeBSDConsole }()

	scr := NewScreenBuf()
	var output bytes.Buffer
	scr.Writer = &output
	palette := XTerm256Palette
	scr.ThemePalette = &palette
	scr.AllocBuf(2, 1)
	scr.Write(0, 0, []CharInfo{{Char: 'O'}, {Char: 'K'}})
	scr.SetCursorPos(1, 0)
	scr.SetCursorVisible(true)
	scr.Flush()

	got := output.String()
	for _, unsupported := range []string{"\x1b[?", "\x1b]"} {
		if strings.Contains(got, unsupported) {
			t.Errorf("FreeBSD console frame contains unsupported sequence prefix %q: %q", unsupported, got)
		}
	}
	if !strings.Contains(got, "\x1b[=0S") {
		t.Errorf("FreeBSD console frame did not restore its native visible cursor: %q", got)
	}
}

func TestScreenRow(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 2)
	family := RegisterCluster("👨‍👩‍👧‍👦")
	scr.Write(0, 0, []CharInfo{
		{Char: 'H'},
		{Char: 'i'},
		{Char: 0},
		{Char: family},
		{Char: WideCharFiller},
		{Char: '!'},
	})

	got := ScreenRow(scr, 0, 0, 5)
	want := "Hi 👨‍👩‍👧‍👦!"
	if got != want {
		t.Errorf("ScreenRow = %q, want %q", got, want)
	}
}

// TestAnsiRenderer_UnicodeOverwriteClearsTrailingGarbage verifies that updating
// a row that held wide/unicode characters overwrites ghost glyph remnants.
func TestAnsiRenderer_UnicodeOverwriteClearsTrailingGarbage(t *testing.T) {
	r := &AnsiRenderer{parent: &ScreenBuf{ColorProfile: ColorProfileTrueColor}}
	w, h := 10, 1
	shadow := make([]CharInfo, w*h)
	buf := make([]CharInfo, w*h)

	// shadow: wide unicode at col 0, filler, "test", then spaces
	shadow[0] = CharInfo{Char: '📁'}
	shadow[1] = CharInfo{Char: WideCharFiller}
	shadow[2] = CharInfo{Char: 't'}
	shadow[3] = CharInfo{Char: 'e'}
	shadow[4] = CharInfo{Char: 's'}
	shadow[5] = CharInfo{Char: 't'}
	for i := 6; i < 10; i++ {
		shadow[i] = CharInfo{Char: ' '}
	}

	// buf: "hi" at cols 0..1, then spaces
	buf[0] = CharInfo{Char: 'h'}
	buf[1] = CharInfo{Char: 'i'}
	for i := 2; i < 10; i++ {
		buf[i] = CharInfo{Char: ' '}
	}

	r.Render(buf, shadow, w, h, false)
	got := r.frameOut.String()

	// Full rewrite must cover the trailing spaces to wipe the old "test" and wide remnants.
	if !strings.Contains(got, "hi") || !strings.Contains(got, "\x1b[1;1H") {
		t.Fatalf("Expected render to contain position and text, got %q", got)
	}
	if !strings.Contains(got, "hi    ") {
		t.Errorf("Expected trailing spaces to overwrite previous content, got %q", got)
	}
}
func TestScreenBuf_WritePassthrough(t *testing.T) {
	scr := NewSilentScreenBuf()
	var buf bytes.Buffer
	scr.Writer = &buf
	scr.AllocBuf(10, 5)

	payload := []byte("hello passthrough output\x1b[0m")
	scr.WritePassthrough(payload)

	if !bytes.Equal(buf.Bytes(), payload) {
		t.Fatalf("WritePassthrough = %q, want %q", buf.String(), string(payload))
	}
}

func TestWritePassthrough_Global(t *testing.T) {
	scr := NewSilentScreenBuf()
	var buf bytes.Buffer
	scr.Writer = &buf
	scr.AllocBuf(10, 5)

	oldFM := FrameManager
	fm := &frameManager{}
	fm.Init(scr)
	FrameManager = fm
	defer func() { FrameManager = oldFM }()

	payload := []byte("global passthrough test")
	WritePassthrough(payload)

	if !bytes.Equal(buf.Bytes(), payload) {
		t.Fatalf("global WritePassthrough = %q, want %q", buf.String(), string(payload))
	}
}

func TestScreenBuf_WritePassthrough_Concurrency(t *testing.T) {
	scr := NewSilentScreenBuf()
	var buf bytes.Buffer
	scr.Writer = &buf
	scr.AllocBuf(20, 5)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				scr.WritePassthrough([]byte("pt"))
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				scr.Flush()
			}
		}()
	}
	wg.Wait()
}

// renderRow renders one row of cells and returns the bytes the renderer
// would send to the terminal.
func renderRow(t *testing.T, cells []CharInfo, w int) string {
	t.Helper()
	r := &AnsiRenderer{parent: &ScreenBuf{ColorProfile: ColorProfileTrueColor}}
	buf := make([]CharInfo, w)
	for i := range buf {
		if i < len(cells) {
			buf[i] = cells[i]
		}
	}
	r.Render(buf, make([]CharInfo, w), w, 1, true)
	return r.frameOut.String()
}

func countCursorPositions(s string) int {
	n := 0
	for i := 0; i+1 < len(s); i++ {
		if s[i] != 0x1b || s[i+1] != '[' {
			continue
		}
		for j := i + 2; j < len(s); j++ {
			if c := s[j]; c >= '@' && c <= '~' {
				if c == 'H' {
					n++
				}
				break
			}
		}
	}
	return n
}

func TestAnsiRenderer_ResyncsAfterUntrustedCells(t *testing.T) {
	// A row of text whose width the terminal cannot disagree about is
	// written as one stream: a single cursor positioning at its start.
	if got := countCursorPositions(renderRow(t, StringToCharInfo("abcdef", 0), 6)); got != 1 {
		t.Errorf("ASCII row used %d cursor positionings, want 1", got)
	}
	if got := countCursorPositions(renderRow(t, StringToCharInfo("привет", 0), 6)); got != 1 {
		t.Errorf("Cyrillic row used %d cursor positionings, want 1", got)
	}

	// After a cell whose advance the terminal may measure differently, the
	// next cell is placed absolutely, so a disagreement cannot drag the
	// rest of the row along (unxed/f4#546).
	cases := []struct {
		name string
		text string
		// the column, one based, the renderer must jump back to
		wantCol int
	}{
		{"devanagari", "abcकde", 5},
		{"wide character", "世cd", 3},
		{"composite cluster", "ab\u0915\u094D\u0915cd", 5},
		{"emoji", "ab😀cd", 5},
	}
	for _, c := range cases {
		got := renderRow(t, StringToCharInfo(c.text, 0), 8)
		want := fmt.Sprintf("\x1b[1;%dH", c.wantCol)
		if !strings.Contains(got, want) {
			t.Errorf("%s: %q lacks a resync at %q", c.name, got, want)
		}
	}
}

func TestAnsiRenderer_WideCharFillerDoesNotAdvanceTwice(t *testing.T) {
	// The filler of a wide character is skipped, not written, so the
	// renderer must not believe the terminal moved past it on its own
	// unless the wide character itself was just sent.
	cells := StringToCharInfo("世界", 0)
	got := renderRow(t, cells, 4)
	if strings.Contains(got, "\x1b[1;2H") || strings.Contains(got, "\x1b[1;4H") {
		t.Errorf("renderer addressed a filler column: %q", got)
	}
	if !strings.Contains(got, "\x1b[1;3H") {
		t.Errorf("second wide character was not resynced: %q", got)
	}
}
