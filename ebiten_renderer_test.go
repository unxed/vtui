//go:build (linux || windows || darwin) && !android && (amd64 || arm64)

package vtui

import (
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/unxed/vtinput"
)

func TestEbitenRenderer_AllocatesFramebufferForGrid(t *testing.T) {
	r := NewEbitenRenderer(nil, nil, 8, 16, 1)
	buf, shadow := mkGrid(10, 4, ' ', 0)

	r.Render(buf, shadow, 10, 4, true)

	if r.img == nil {
		t.Fatal("expected a framebuffer after the first Render")
	}
	if got, want := r.img.Rect.Dx(), 10*8; got != want {
		t.Errorf("framebuffer width = %d, want %d", got, want)
	}
	if got, want := r.img.Rect.Dy(), 4*16; got != want {
		t.Errorf("framebuffer height = %d, want %d", got, want)
	}
}

// A resized grid must reallocate rather than paint into the old bounds, which
// is how a shrink turns into an out-of-range write.
func TestEbitenRenderer_ReallocatesOnResize(t *testing.T) {
	r := NewEbitenRenderer(nil, nil, 8, 16, 1)

	buf, shadow := mkGrid(10, 4, ' ', 0)
	r.Render(buf, shadow, 10, 4, true)

	buf2, shadow2 := mkGrid(20, 8, ' ', 0)
	r.Render(buf2, shadow2, 20, 8, false)

	if got, want := r.img.Rect.Dx(), 20*8; got != want {
		t.Errorf("width after grow = %d, want %d", got, want)
	}

	buf3, shadow3 := mkGrid(5, 2, ' ', 0)
	r.Render(buf3, shadow3, 5, 2, false)

	if got, want := r.img.Rect.Dx(), 5*8; got != want {
		t.Errorf("width after shrink = %d, want %d", got, want)
	}
	if got, want := r.cols, 5; got != want {
		t.Errorf("cols = %d, want %d", got, want)
	}
}

// Render must survive a short buffer instead of panicking: the grid can be
// resized between AllocBuf and the next flush.
func TestEbitenRenderer_IgnoresInconsistentInput(t *testing.T) {
	r := NewEbitenRenderer(nil, nil, 8, 16, 1)
	buf, shadow := mkGrid(4, 4, ' ', 0)

	r.Render(buf, shadow, 100, 100, true) // claims far more cells than exist
	r.Render(buf, shadow, 0, 0, true)
	r.Render(nil, nil, 4, 4, true)

	if r.img != nil {
		t.Error("expected no framebuffer to be allocated from inconsistent input")
	}
}

func TestEbitenRenderer_TakeFrameClearsDirty(t *testing.T) {
	r := NewEbitenRenderer(nil, nil, 8, 16, 1)
	buf, shadow := mkGrid(4, 2, ' ', 0)
	r.Render(buf, shadow, 4, 2, true)

	pix, w, h, changed := r.takeFrame()
	if !changed {
		t.Error("first takeFrame after a forced render should report a change")
	}
	if pix == nil || w != 32 || h != 32 {
		t.Errorf("takeFrame returned %dx%d, want 32x32", w, h)
	}

	// Nothing rendered since, so the game loop must not re-upload.
	if _, _, _, changed := r.takeFrame(); changed {
		t.Error("takeFrame should report no change when nothing was rendered")
	}
}

// An unchanged grid must not dirty the framebuffer, otherwise a static screen
// uploads a texture every frame.
func TestEbitenRenderer_UnchangedGridDoesNotDirty(t *testing.T) {
	r := NewEbitenRenderer(nil, nil, 8, 16, 1)
	buf, shadow := mkGrid(8, 4, ' ', 0)

	r.Render(buf, shadow, 8, 4, true)
	r.takeFrame()

	copy(shadow, buf)
	r.blinkState = true
	r.lastBlinkTime = time.Now()

	r.Render(buf, shadow, 8, 4, false)
	if _, _, _, changed := r.takeFrame(); changed {
		t.Error("an unchanged grid should not mark the framebuffer dirty")
	}
}

func TestEbitenRenderer_CursorChangeMarksDirty(t *testing.T) {
	r := NewEbitenRenderer(nil, nil, 8, 16, 1)
	buf, shadow := mkGrid(8, 4, ' ', 0)
	r.Render(buf, shadow, 8, 4, true)
	r.takeFrame()

	r.SetCursor(3, 1, true, CursorShapeUnderline)
	if _, _, _, changed := r.takeFrame(); !changed {
		t.Error("moving the cursor should mark the framebuffer dirty")
	}

	r.takeFrame()
	r.SetCursor(3, 1, true, CursorShapeBlock)
	if _, _, _, changed := r.takeFrame(); !changed {
		t.Error("changing cursor shape should mark the framebuffer dirty")
	}
}

func TestEbitenRenderer_TitleIsReportedOnce(t *testing.T) {
	r := NewEbitenRenderer(nil, nil, 8, 16, 1)

	if _, ok := r.takeTitle(); ok {
		t.Error("no title was set, takeTitle should report nothing")
	}

	r.SetWindowTitle("vtui")
	title, ok := r.takeTitle()
	if !ok || title != "vtui" {
		t.Errorf("takeTitle = %q, %v; want \"vtui\", true", title, ok)
	}
	if _, ok := r.takeTitle(); ok {
		t.Error("a title must be reported once, not on every frame")
	}

	// Setting the same title again is not a change.
	r.SetWindowTitle("vtui")
	if _, ok := r.takeTitle(); ok {
		t.Error("setting an identical title should not report a change")
	}
}

// EbitenRenderer must satisfy the interface the ScreenBuf drives it through.
func TestEbitenRenderer_ImplementsSurfaceRenderer(t *testing.T) {
	var _ SurfaceRenderer = (*EbitenRenderer)(nil)
}

func TestEbitenKeyToVK(t *testing.T) {
	cases := []struct {
		key  ebiten.Key
		want uint16
		name string
	}{
		{ebiten.KeyEscape, vtinput.VK_ESCAPE, "Escape"},
		{ebiten.KeyEnter, vtinput.VK_RETURN, "Enter"},
		{ebiten.KeyNumpadEnter, vtinput.VK_RETURN, "NumpadEnter"},
		{ebiten.KeyTab, vtinput.VK_TAB, "Tab"},
		{ebiten.KeyBackspace, vtinput.VK_BACK, "Backspace"},
		{ebiten.KeyPageUp, vtinput.VK_PRIOR, "PageUp"},
		{ebiten.KeyPageDown, vtinput.VK_NEXT, "PageDown"},
		{ebiten.KeyArrowUp, vtinput.VK_UP, "Up"},
		{ebiten.KeyF1, vtinput.VK_F1, "F1"},
		{ebiten.KeyF12, vtinput.VK_F12, "F12"},
		{ebiten.KeyControlLeft, vtinput.VK_LCONTROL, "LeftCtrl"},
		{ebiten.KeyControlRight, vtinput.VK_RCONTROL, "RightCtrl"},
		{ebiten.KeyShiftLeft, vtinput.VK_LSHIFT, "LeftShift"},
		{ebiten.KeyAltRight, vtinput.VK_RMENU, "RightAlt"},
		{ebiten.KeyA, vtinput.VK_A, "A"},
		{ebiten.KeyZ, vtinput.VK_Z, "Z"},
		{ebiten.KeyDigit0, vtinput.VK_0, "0"},
		{ebiten.KeyDigit9, vtinput.VK_9, "9"},
	}
	for _, c := range cases {
		if got := ebitenKeyToVK(c.key, 0); got != c.want {
			t.Errorf("ebitenKeyToVK(%s) = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestEbitenKeyToVK_KeypadNavigation(t *testing.T) {
	if got := ebitenKeyToVK(ebiten.KeyNumpad2, 0); got != vtinput.VK_DOWN {
		t.Errorf("Numpad2 with NumLock off = %d, want VK_DOWN (%d)", got, vtinput.VK_DOWN)
	}
	if got := ebitenKeyToVK(ebiten.KeyNumpad2, vtinput.NumLockOn); got != vtinput.VK_NUMPAD2 {
		t.Errorf("Numpad2 with NumLock on = %d, want VK_NUMPAD2 (%d)", got, vtinput.VK_NUMPAD2)
	}
	if got := ebitenKeyToVK(ebiten.KeyNumpad2, vtinput.NumLockOn|vtinput.ShiftPressed); got != vtinput.VK_DOWN {
		t.Errorf("Numpad2 with NumLock on + Shift = %d, want VK_DOWN (%d)", got, vtinput.VK_DOWN)
	}
}

// The letter and digit ranges are mapped arithmetically, so every code point
// in them has to land on a real VK rather than merely the endpoints.
func TestEbitenKeyToVK_LetterAndDigitRanges(t *testing.T) {
	for k := ebiten.KeyA; k <= ebiten.KeyZ; k++ {
		want := vtinput.VK_A + uint16(k-ebiten.KeyA)
		if got := ebitenKeyToVK(k, 0); got != want {
			t.Fatalf("letter key %v mapped to %d, want %d", k, got, want)
		}
	}
	for k := ebiten.KeyDigit0; k <= ebiten.KeyDigit9; k++ {
		want := vtinput.VK_0 + uint16(k-ebiten.KeyDigit0)
		if got := ebitenKeyToVK(k, 0); got != want {
			t.Fatalf("digit key %v mapped to %d, want %d", k, got, want)
		}
	}
}

// Unmapped keys must return 0 so the host drops them; VK 0 sent onward would
// read as a real keystroke.
func TestEbitenKeyToVK_UnmappedIsZero(t *testing.T) {
	if got := ebitenKeyToVK(ebiten.KeyMax+1, 0); got != 0 {
		t.Errorf("out-of-range key mapped to %d, want 0", got)
	}
}

// No two distinct keys may collide on one virtual key code, except where the
// collision is deliberate.
func TestEbitenKeyToVK_NoAccidentalCollisions(t *testing.T) {
	intended := map[uint16]bool{
		vtinput.VK_RETURN: true, // Enter and NumpadEnter share it on purpose
	}
	// The check runs with NumLock on, where every key stands for itself. With
	// the lock off the keypad deliberately doubles as the navigation block, so
	// each of those codes has two keys by design; that half of the mapping is
	// pinned by TestEbitenKeyToVK_KeypadNavigation instead.
	seen := make(map[uint16]ebiten.Key)
	for k := ebiten.Key(0); k <= ebiten.KeyMax; k++ {
		vk := ebitenKeyToVK(k, vtinput.NumLockOn)
		if vk == 0 || intended[vk] {
			continue
		}
		if prev, dup := seen[vk]; dup {
			t.Errorf("keys %v and %v both map to VK %d", prev, k, vk)
		}
		seen[vk] = k
	}
}

// A caret that is not visible must not keep its row repainting: the row was
// dirtied every frame before the painted-cursor state was tracked separately
// from the logical cursor position.
func TestEbitenRenderer_HiddenCursorDoesNotDirtyForever(t *testing.T) {
	r := NewEbitenRenderer(nil, nil, 8, 16, 1)
	buf, shadow := mkGrid(8, 4, ' ', 0)

	r.SetCursor(2, 1, true, CursorShapeBlock)
	r.Render(buf, shadow, 8, 4, true)
	copy(shadow, buf)
	r.takeFrame()

	// Hide the caret. One more repaint is expected, to erase it.
	r.SetCursor(2, 1, false, CursorShapeBlock)
	r.Render(buf, shadow, 8, 4, false)
	if _, _, _, changed := r.takeFrame(); !changed {
		t.Fatal("hiding the caret should repaint its row once, to erase it")
	}

	// From then on the screen is static and must stay clean.
	for i := 0; i < 3; i++ {
		r.Render(buf, shadow, 8, 4, false)
		if _, _, _, changed := r.takeFrame(); changed {
			t.Fatalf("frame %d: hidden caret must not keep dirtying its row", i)
		}
	}
}

// A caret that moves must erase the row it left as well as paint the new one.
func TestEbitenRenderer_MovedCursorErasesOldRow(t *testing.T) {
	r := NewEbitenRenderer(nil, nil, 8, 16, 1)
	buf, shadow := mkGrid(8, 6, ' ', 0)

	r.SetCursor(1, 1, true, CursorShapeBlock)
	r.Render(buf, shadow, 8, 6, true)
	copy(shadow, buf)
	r.takeFrame()

	if !r.paintedCursor || r.paintedCursorY != 1 {
		t.Fatalf("painted caret state = (%v, row %d), want (true, row 1)", r.paintedCursor, r.paintedCursorY)
	}

	r.SetCursor(1, 4, true, CursorShapeBlock)
	r.Render(buf, shadow, 8, 6, false)
	if r.paintedCursorY != 4 {
		t.Errorf("painted caret row = %d after move, want 4", r.paintedCursorY)
	}
	if _, _, _, changed := r.takeFrame(); !changed {
		t.Error("moving the caret should dirty the framebuffer")
	}
}

// pixelAt reads a pixel out of the renderer's framebuffer.
func pixelAt(t *testing.T, r *EbitenRenderer, x, y int) (rr, g, b uint8) {
	t.Helper()
	off := y*r.img.Stride + x*4
	if off+3 >= len(r.img.Pix) {
		t.Fatalf("pixel (%d,%d) is outside the framebuffer", x, y)
	}
	return r.img.Pix[off], r.img.Pix[off+1], r.img.Pix[off+2]
}

// Frame characters must go through the geometric path, not the font, so that
// neighbouring cells join. A font glyph would leave the seam column unlit.
func TestEbitenRenderer_BoxCharsJoinAcrossCells(t *testing.T) {
	preserveTestPalette(t)
	// A nil face proves the point twice over: if the box path were not taken
	// these cells would render as nothing at all.
	r := NewEbitenRenderer(nil, nil, 8, 16, 1)

	const w, h = 4, 1
	buf, shadow := mkGrid(w, h, '─', 0)
	for i := range buf {
		buf[i].Attributes = SetIndexFore(SetIndexBack(0, 0), 15)
	}
	ThemePalette[0] = 0x000000
	ThemePalette[15] = 0xFFFFFF

	r.Render(buf, shadow, w, h, true)

	mid := 16 / 2
	for x := 0; x < w*8; x++ {
		if rr, g, b := pixelAt(t, r, x, mid); rr == 0 && g == 0 && b == 0 {
			t.Fatalf("horizontal rule has a gap at x=%d: frame chars are not "+
				"reaching the geometric path", x)
		}
	}
}

// Text runes must still reach the font path and must not be swallowed by the
// box-drawing branch.
func TestEbitenRenderer_TextRunesAreNotTreatedAsBoxes(t *testing.T) {
	r := NewEbitenRenderer(nil, nil, 8, 16, 1)
	buf, shadow := mkGrid(4, 1, 'A', 0)

	r.Render(buf, shadow, 4, 1, true)

	// With a nil face nothing is drawn, but the run must complete without the
	// geometric path claiming the cell.
	if r.img == nil {
		t.Fatal("expected a framebuffer")
	}
}

func TestEbitenRenderer_ScaleIsClampedAndStored(t *testing.T) {
	if got := NewEbitenRenderer(nil, nil, 8, 16, 0).scale; got != 1 {
		t.Errorf("scale 0 clamped to %d, want 1", got)
	}
	if got := NewEbitenRenderer(nil, nil, 8, 16, -3).scale; got != 1 {
		t.Errorf("negative scale clamped to %d, want 1", got)
	}
	if got := NewEbitenRenderer(nil, nil, 16, 32, 2).scale; got != 2 {
		t.Errorf("scale 2 stored as %d, want 2", got)
	}
}

// At scale 2 the cell is measured in real pixels, so the framebuffer is the
// full device resolution rather than a logical one that would be upscaled.
func TestEbitenRenderer_HiDPIFramebufferIsDeviceSized(t *testing.T) {
	r := NewEbitenRenderer(nil, nil, 16, 32, 2) // a 8x16 cell at scale 2
	buf, shadow := mkGrid(10, 4, ' ', 0)

	r.Render(buf, shadow, 10, 4, true)

	if got, want := r.img.Rect.Dx(), 10*16; got != want {
		t.Errorf("HiDPI framebuffer width = %d, want %d", got, want)
	}
	if got, want := r.img.Rect.Dy(), 4*32; got != want {
		t.Errorf("HiDPI framebuffer height = %d, want %d", got, want)
	}
}

// Auto-repeat: Ebitengine only reports the transition into the pressed state,
// so without synthesis a held arrow key fires exactly once.
func TestKeyRepeatFires(t *testing.T) {
	const tps = 60
	if !keyRepeatFires(1, tps) {
		t.Error("the initial press must fire")
	}
	for d := 2; d <= tps/2; d++ {
		if keyRepeatFires(d, tps) {
			t.Fatalf("fired at tick %d, inside the initial delay", d)
		}
	}

	fires := 0
	for d := tps/2 + 1; d <= tps/2+tps; d++ { // one second past the delay
		if keyRepeatFires(d, tps) {
			fires++
		}
	}
	if fires < 25 || fires > 35 {
		t.Errorf("%d repeats in the second after the delay, want about 30", fires)
	}
}

// A held key must keep firing rather than stopping after the first repeat,
// which is the bug this replaced.
func TestKeyRepeatFires_ContinuesWhileHeld(t *testing.T) {
	const tps = 60
	var ticks []int
	for d := 1; d <= tps*5; d++ {
		if keyRepeatFires(d, tps) {
			ticks = append(ticks, d)
		}
	}
	if len(ticks) < 2 {
		t.Fatalf("only %d fires in five seconds of holding", len(ticks))
	}
	if ticks[0] != 1 {
		t.Errorf("first fire at tick %d, want the initial press at 1", ticks[0])
	}
	// The gap from the initial press to the first repeat is the deliberate
	// half-second delay; every gap after it is the steady repeat rate.
	if gap := ticks[1] - ticks[0]; gap < tps/2 {
		t.Errorf("first repeat came after %d ticks, want at least %d", gap, tps/2)
	}
	for i := 2; i < len(ticks); i++ {
		if gap := ticks[i] - ticks[i-1]; gap > tps/10 {
			t.Fatalf("gap of %d ticks between repeats at tick %d", gap, ticks[i])
		}
	}
	if last := ticks[len(ticks)-1]; last < tps*4 {
		t.Errorf("repeats stopped at tick %d of %d", last, tps*5)
	}
}

func TestKeyRepeatFires_HandlesOddTPS(t *testing.T) {
	for _, tps := range []int{0, -1, 1, 15, 30, 120, 240} {
		if !keyRepeatFires(1, tps) {
			t.Errorf("tps %d: the initial press must always fire", tps)
		}
		got := 0
		for d := 1; d <= 600; d++ {
			if keyRepeatFires(d, tps) {
				got++
			}
		}
		if got == 0 {
			t.Errorf("tps %d: no repeats at all in 600 ticks", tps)
		}
	}
	if keyRepeatFires(0, 60) || keyRepeatFires(-5, 60) {
		t.Error("a key that is not held must not fire")
	}
}

// Modifiers are a sustained state, not a stream; repeating them would flood
// the queue for as long as a chord is held.
func TestIsModifierVK(t *testing.T) {
	for _, vk := range []uint16{
		vtinput.VK_LCONTROL, vtinput.VK_RCONTROL, vtinput.VK_LSHIFT, vtinput.VK_RSHIFT,
		vtinput.VK_LMENU, vtinput.VK_RMENU, vtinput.VK_LWIN, vtinput.VK_RWIN,
		vtinput.VK_CAPITAL, vtinput.VK_NUMLOCK, vtinput.VK_SCROLL,
	} {
		if !isModifierVK(vk) {
			t.Errorf("VK %d should be treated as a modifier", vk)
		}
	}
	for _, vk := range []uint16{
		vtinput.VK_LEFT, vtinput.VK_RIGHT, vtinput.VK_UP, vtinput.VK_DOWN,
		vtinput.VK_BACK, vtinput.VK_DELETE, vtinput.VK_RETURN, vtinput.VK_A, vtinput.VK_F1,
	} {
		if isModifierVK(vk) {
			t.Errorf("VK %d must be allowed to repeat", vk)
		}
	}
}

// The arrow keys, Backspace and Delete are exactly what the user holds down,
// so they must reach the repeating virtual-key path rather than the text
// stream, which never sees them.
func TestArrowsAndEditingKeysTakeTheRepeatingPath(t *testing.T) {
	for _, k := range []ebiten.Key{
		ebiten.KeyArrowLeft, ebiten.KeyArrowRight, ebiten.KeyArrowUp, ebiten.KeyArrowDown,
		ebiten.KeyBackspace, ebiten.KeyDelete, ebiten.KeyPageUp, ebiten.KeyPageDown,
		ebiten.KeyHome, ebiten.KeyEnd, ebiten.KeyTab, ebiten.KeyEnter,
	} {
		vk := ebitenKeyToVK(k, 0)
		if vk == 0 {
			t.Errorf("%v has no virtual key code", k)
			continue
		}
		if !isSpecialOrModifiedKey(vk, 0) {
			t.Errorf("%v is not treated as special, so it would fall to the text stream and never repeat", k)
		}
		if isModifierVK(vk) {
			t.Errorf("%v is misclassified as a modifier and would not repeat", k)
		}
	}
}

func TestEbitenRenderer_ImplementsGraphicsRenderer(t *testing.T) {
	var _ GraphicsRenderer = (*EbitenRenderer)(nil)
}

func TestEbitenHost_ImplementsDragBackend(t *testing.T) {
	var _ DragBackend = (*EbitenHost)(nil)

	h := &EbitenHost{}
	if !h.AcceptsDrops() {
		t.Error("the ebiten window does accept dropped files")
	}
	// Ebitengine's drop support is receive-only, so a drag out must say so
	// rather than appear to start something that never completes.
	if _, err := h.StartDrag(DragPayload{Paths: []string{"/tmp/x"}}, DropCopy); err != ErrDragUnsupported {
		t.Errorf("StartDrag error = %v, want ErrDragUnsupported", err)
	}
}

// RenderGraphics must ignore a nil or non-native layer instead of panicking.
func TestEbitenRenderer_RenderGraphicsIgnoresUnusableLayer(t *testing.T) {
	r := NewEbitenRenderer(nil, nil, 8, 16, 1)
	buf, shadow := mkGrid(4, 2, ' ', 0)
	r.Render(buf, shadow, 4, 2, true)
	r.RenderGraphics(nil, buf, shadow, 4, 2, true)
}

// Alt+letter and Alt+digit are quick-search accelerators: the application
// needs the character, not just the virtual key. Without it Alt+1 arrives
// with nothing to search for.
func TestCharForVK_SuppliesAcceleratorCharacters(t *testing.T) {
	h := &EbitenHost{lastRuneForVK: map[uint16]rune{}}

	for vk, want := range map[uint16]rune{
		vtinput.VK_0: '0', vtinput.VK_1: '1', vtinput.VK_9: '9',
		vtinput.VK_A: 'a', vtinput.VK_M: 'm', vtinput.VK_Z: 'z',
	} {
		if got := h.charForVK(vk); got != want {
			t.Errorf("charForVK(%d) = %q, want %q", vk, got, want)
		}
	}
}

// What the key really produces on this layout beats the ASCII fallback.
func TestCharForVK_PrefersTheObservedRune(t *testing.T) {
	h := &EbitenHost{lastRuneForVK: map[uint16]rune{vtinput.VK_A: 'ф'}}

	if got := h.charForVK(vtinput.VK_A); got != 'ф' {
		t.Errorf("charForVK = %q, want the observed rune 'ф'", got)
	}
	if got := h.charForVK(vtinput.VK_B); got != 'b' {
		t.Errorf("charForVK for an unobserved key = %q, want the fallback 'b'", got)
	}
}

// Keys with no character meaning must not invent one.
func TestCharForVK_LeavesNonTextKeysAlone(t *testing.T) {
	h := &EbitenHost{lastRuneForVK: map[uint16]rune{}}
	for _, vk := range []uint16{
		vtinput.VK_LEFT, vtinput.VK_F1, vtinput.VK_RETURN, vtinput.VK_ESCAPE, vtinput.VK_LMENU,
	} {
		if got := h.charForVK(vk); got != 0 {
			t.Errorf("charForVK(%d) = %q, want no character", vk, got)
		}
	}
}

// The cursor underline must follow the X11 backend, including thickening on a
// scaled display so it does not thin out to a hair.
func TestEbitenRenderer_UnderlineCursorMatchesX11Geometry(t *testing.T) {
	for _, tc := range []struct{ scale, cellH, wantRows int }{
		{1, 16, 2},
		{2, 32, 4},
	} {
		r := NewEbitenRenderer(nil, nil, 8, tc.cellH, tc.scale)
		buf, shadow := mkGrid(1, 1, ' ', 0)
		r.SetCursor(0, 0, true, CursorShapeUnderline)
		r.Render(buf, shadow, 1, 1, true)

		lit := 0
		for y := 0; y < tc.cellH; y++ {
			off := y*r.img.Stride + 0*4
			if r.img.Pix[off] != 0 || r.img.Pix[off+1] != 0 || r.img.Pix[off+2] != 0 {
				lit++
			}
		}
		if lit != tc.wantRows {
			t.Errorf("scale %d: underline is %d rows tall, want %d", tc.scale, lit, tc.wantRows)
		}
	}
}

// A block cursor fills the whole cell, unlike the underline.
func TestEbitenRenderer_BlockCursorFillsTheCell(t *testing.T) {
	r := NewEbitenRenderer(nil, nil, 8, 16, 1)
	buf, shadow := mkGrid(1, 1, ' ', 0)
	r.SetCursor(0, 0, true, CursorShapeBlock)
	r.Render(buf, shadow, 1, 1, true)

	for y := 0; y < 16; y++ {
		off := y*r.img.Stride + 0*4
		if r.img.Pix[off] == 0 && r.img.Pix[off+1] == 0 && r.img.Pix[off+2] == 0 {
			t.Fatalf("block cursor left row %d unpainted", y)
		}
	}
}

// A Ctrl or Alt chord is held for one tick so that a character arriving late
// can retract it. Without this, a fast layout switch left Alt stuck, the next
// letter went out as an Alt chord, and the character followed a tick later:
// typing "test" after Alt+Shift opened quick search and put "ttest" in it.
func TestPendingChord_DroppedWhenTextContradictsIt(t *testing.T) {
	h := newTestHost(t)

	chord := &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_T, Char: 't',
		ControlKeyState: vtinput.LeftAltPressed,
	}
	h.pendingChord = chord

	// Next tick brings a character, which no layout emits with Alt truly down.
	h.settlePendingChord(true)

	if h.pendingChord != nil {
		t.Error("the chord should have been settled")
	}
	if got := drainEvents(h); len(got) != 0 {
		t.Errorf("a contradicted chord must be dropped, but %d event(s) were sent", len(got))
	}
}

// With no contradiction the chord is a real one and must still arrive.
func TestPendingChord_SentWhenNothingContradictsIt(t *testing.T) {
	h := newTestHost(t)

	h.pendingChord = &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_T, Char: 't',
		ControlKeyState: vtinput.LeftAltPressed,
	}
	h.settlePendingChord(false)

	got := drainEvents(h)
	if len(got) != 1 {
		t.Fatalf("expected the chord to be delivered, got %d event(s)", len(got))
	}
	if got[0].VirtualKeyCode != vtinput.VK_T || got[0].ControlKeyState&vtinput.LeftAltPressed == 0 {
		t.Errorf("delivered event = %+v, want Alt+T", got[0])
	}
}

// Settling with nothing pending must be harmless: it runs on every tick.
func TestPendingChord_SettlingNothingIsHarmless(t *testing.T) {
	h := newTestHost(t)
	h.settlePendingChord(true)
	h.settlePendingChord(false)
	if got := drainEvents(h); len(got) != 0 {
		t.Errorf("settling an empty slot sent %d event(s)", len(got))
	}
}

func newTestHost(t *testing.T) *EbitenHost {
	t.Helper()
	return &EbitenHost{
		reader:        &vtinput.Reader{EventChan: make(chan *vtinput.InputEvent, 16)},
		lastRuneForVK: map[uint16]rune{},
		phantomMods:   map[ebiten.Key]bool{},
	}
}

func drainEvents(h *EbitenHost) []*vtinput.InputEvent {
	var out []*vtinput.InputEvent
	for {
		select {
		case ev := <-h.reader.EventChan:
			out = append(out, ev)
		default:
			return out
		}
	}
}

// Alt+digit is a quick-search accelerator, and every digit must behave the
// same: Alt+1 was reported broken while Alt+2 worked, so the mapping is
// pinned to absolute values rather than only to internal consistency.
func TestDigitKeysMapToExactVirtualKeys(t *testing.T) {
	h := &EbitenHost{lastRuneForVK: map[uint16]rune{}}
	for i := 0; i <= 9; i++ {
		k := ebiten.KeyDigit0 + ebiten.Key(i)
		vk := ebitenKeyToVK(k, 0)
		if want := uint16(0x30 + i); vk != want {
			t.Errorf("%v -> VK 0x%02X, want 0x%02X", k, vk, want)
		}
		if got, want := h.charForVK(vk), rune('0'+i); got != want {
			t.Errorf("%v -> char %q, want %q", k, got, want)
		}
	}
}

// A character event must name the key it came from. Ebitengine reports text
// and keys as separate streams, and a press logged as VK 0 followed by a
// release logging VK_B is a pair nothing downstream can match up.
func TestKeyBehindText_UsesTheKeyPressedThisTick(t *testing.T) {
	h := newTestHost(t)
	h.pressedBuf = []ebiten.Key{ebiten.KeyB}

	k, ok := h.keyBehindText(0)
	if !ok || k != ebiten.KeyB {
		t.Errorf("keyBehindText = %v, %v; want KeyB, true", k, ok)
	}
}

// Auto-repeat keeps producing characters with no new press, so a single held
// key is still an unambiguous source.
func TestKeyBehindText_FallsBackToTheOneHeldKey(t *testing.T) {
	h := newTestHost(t)
	h.heldBuf = []ebiten.Key{ebiten.KeyShiftLeft, ebiten.KeyB}

	k, ok := h.keyBehindText(0)
	if !ok || k != ebiten.KeyB {
		t.Errorf("keyBehindText = %v, %v; want KeyB, true (modifiers do not count)", k, ok)
	}
}

// Ambiguity must produce no attribution rather than a guess: a wrong one
// labels the key with someone else's rune and misleads every later Alt chord.
func TestKeyBehindText_DeclinesWhenAmbiguous(t *testing.T) {
	for name, h := range map[string]*EbitenHost{
		"two keys down this tick": {pressedBuf: []ebiten.Key{ebiten.KeyA, ebiten.KeyB}},
		"two keys held":           {heldBuf: []ebiten.Key{ebiten.KeyA, ebiten.KeyB}},
		"nothing at all":          {},
		"only modifiers held":     {heldBuf: []ebiten.Key{ebiten.KeyShiftLeft, ebiten.KeyControlLeft}},
	} {
		if k, ok := h.keyBehindText(0); ok {
			t.Errorf("%s: keyBehindText returned %v, want no attribution", name, k)
		}
	}
}
func TestEbitenModifiers_Locks(t *testing.T) {
	// Ensure calling ebitenModifiers doesn't panic and successfully handles lock keys.
	mods := ebitenModifiers()
	_ = mods
}

func TestEbitenHost_ResolveModifiers_Locks(t *testing.T) {
	h := &EbitenHost{}
	mods := h.resolveModifiers(false)
	_ = mods
}
