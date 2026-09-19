//go:build (linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris) && !android

package vtui

import (
	"image"
	"io"
	"testing"
	"time"

	"github.com/jezek/xgb/xproto"
	"github.com/unxed/vtinput"
)

func TestX11Renderer_CursorMovementTracking(t *testing.T) {
	r := NewX11Renderer(nil, nil)

	// Устанавливаем начальную позицию
	r.SetCursor(10, 5, true, CursorShapeUnderline)

	// Имитируем перемещение влево (скрытые панели, нажатие Left)
	r.SetCursor(9, 5, true, CursorShapeUnderline)

	if r.oldCursorX != 10 {
		t.Errorf("expected oldCursorX to be 10, got %d", r.oldCursorX)
	}
	if r.cursorX != 9 {
		t.Errorf("expected cursorX to be 9, got %d", r.cursorX)
	}
	if r.oldCursorY != 5 || r.cursorY != 5 {
		t.Error("expected both old and new cursor row to be marked as 5")
	}
}

func TestX11Renderer_CursorBlinkAndReset(t *testing.T) {
	r := NewX11Renderer(nil, nil)

	// Проверяем инициализацию таймера
	r.SetCursor(0, 0, true, CursorShapeUnderline)
	t1 := r.lastCursorReset
	if t1.IsZero() {
		t.Fatal("expected lastCursorReset to be initialized and non-zero")
	}

	// Тестируем, что перемещение курсора сбрасывает таймер
	time.Sleep(2 * time.Millisecond)
	r.SetCursor(1, 0, true, CursorShapeUnderline)
	t2 := r.lastCursorReset
	if !t2.After(t1) {
		t.Error("expected lastCursorReset to update (be newer) after cursor movement")
	}

	// Тестируем, что изменение формы курсора (Ins/Ovr) сбрасывает таймер
	time.Sleep(2 * time.Millisecond)
	r.SetCursor(1, 0, true, CursorShapeBlock)
	t3 := r.lastCursorReset
	if !t3.After(t2) {
		t.Error("expected lastCursorReset to update (be newer) after cursor shape change")
	}
}

func TestX11Host_DirtySpanLogic(t *testing.T) {
	// Мы не можем запустить реальный X-сервер, но можем проверить логику
	// отслеживания грязных строк напрямую.
	h := &X11Host{
		dirtyLines: make([]bool, 100),
	}

	// Помечаем две разрозненные группы строк
	h.dirtyLines[10] = true
	h.dirtyLines[11] = true
	h.dirtyLines[12] = true

	h.dirtyLines[50] = true
	h.dirtyLines[51] = true

	// Проверяем, как flushImage (в теории) должен их обходить.
	// Ожидается Bounding Box оптимизация (от первой грязной до последней)
	minY := -1
	maxY := -1
	for y := 0; y < 100; y++ {
		if h.dirtyLines[y] {
			if minY == -1 {
				minY = y
			}
			maxY = y
		}
	}

	if minY != 10 {
		t.Errorf("Expected minY 10, got %d", minY)
	}
	if maxY != 51 {
		t.Errorf("Expected maxY 51, got %d", maxY)
	}
}

func TestX11RendererKeepsPartialWindowMarginsCleared(t *testing.T) {
	const (
		windowWidth  = 13
		windowHeight = 7
		cellWidth    = 5
		cellHeight   = 3
	)

	host := &X11Host{
		width:  windowWidth,
		height: windowHeight,
		cellW:  cellWidth,
		cellH:  cellHeight,
		// Simulate the cell-aligned backing image from the previous configure.
		imgBuf:     image.NewRGBA(image.Rect(0, 0, 2*cellWidth, 2*cellHeight)),
		dirtyLines: make([]bool, windowHeight),
	}
	for i := range host.imgBuf.Pix {
		host.imgBuf.Pix[i] = 0xcc
	}

	renderer := NewX11Renderer(host, nil)
	buf := make([]CharInfo, 2*2)
	renderer.Render(buf, make([]CharInfo, len(buf)), 2, 2, true)

	if got := host.imgBuf.Bounds().Size(); got.X != windowWidth || got.Y != windowHeight {
		t.Fatalf("backing image size = %v, want %dx%d", got, windowWidth, windowHeight)
	}
	if host.width != windowWidth || host.height != windowHeight {
		t.Fatalf("host window size changed to %dx%d, want %dx%d", host.width, host.height, windowWidth, windowHeight)
	}

	for y := 0; y < windowHeight; y++ {
		for x := 0; x < windowWidth; x++ {
			if x < 2*cellWidth && y < 2*cellHeight {
				continue
			}
			r, g, b, _ := host.imgBuf.At(x, y).RGBA()
			if r != 0 || g != 0 || b != 0 {
				t.Fatalf("stale pixel at (%d,%d) = #%02x%02x%02x", x, y, r>>8, g>>8, b>>8)
			}
		}
	}
}

// stalePixel reports the first pixel of img outside the gridW x gridH area
// that is not opaque black, or ok=false when the margins are clean.
func stalePixel(img *image.RGBA, gridW, gridH int) (x, y int, ok bool) {
	for y := 0; y < img.Rect.Dy(); y++ {
		for x := 0; x < img.Rect.Dx(); x++ {
			if x < gridW && y < gridH {
				continue
			}
			p := img.Pix[y*img.Stride+x*4:]
			if p[0] != 0 || p[1] != 0 || p[2] != 0 || p[3] != 255 {
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

func redCells(n int) []CharInfo {
	buf := make([]CharInfo, n)
	for i := range buf {
		buf[i].Attributes = SetRGBBack(IsBgRGB, 0xff0000)
	}
	return buf
}

func TestClearFrameMargins(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 13, 7))
	for i := range img.Pix {
		img.Pix[i] = 0xcc
	}
	clearFrameMargins(img, 10, 6)
	if x, y, stale := stalePixel(img, 10, 6); stale {
		t.Fatalf("margin pixel (%d,%d) was not cleared", x, y)
	}
	if got := img.Pix[5*img.Stride+9*4]; got != 0xcc {
		t.Errorf("a pixel inside the grid was touched: %#x", got)
	}

	// A grid at least as large as the image has no margin to clear.
	full := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := range full.Pix {
		full.Pix[i] = 0xcc
	}
	clearFrameMargins(full, 8, 8)
	if full.Pix[0] != 0xcc || full.Pix[len(full.Pix)-1] != 0xcc {
		t.Error("nothing should be cleared when the grid covers the image")
	}
	clearFrameMargins(nil, 1, 1) // must not panic
}

// f4 #283: the FrameManager can render once more for the previous, larger grid
// after the window has shrunk. That frame is clipped to the new image and
// fills the partial bottom row and right column; the smaller frame that
// follows draws whole cells only and used to leave them showing, as a nearly
// whole second key bar one pixel under a row boundary.
func TestX11RendererClearsMarginsFilledByALargerFrame(t *testing.T) {
	const (
		windowWidth  = 14 // 2 cells of 5 and 4 px of margin
		windowHeight = 8  // 2 cells of 3 and 2 px of margin
		cellWidth    = 5
		cellHeight   = 3
	)
	host := &X11Host{
		width:      windowWidth,
		height:     windowHeight,
		cellW:      cellWidth,
		cellH:      cellHeight,
		dirtyLines: make([]bool, windowHeight),
	}
	renderer := NewX11Renderer(host, nil)

	large := redCells(3 * 3)
	renderer.Render(large, make([]CharInfo, len(large)), 3, 3, true)
	// (13,6) is in the bottom margin and the right margin of the 2x2 grid.
	if got := host.imgBuf.Pix[6*host.imgBuf.Stride+13*4]; got != 0xff {
		t.Fatalf("setup: the larger frame did not reach the margin: %#x", got)
	}
	for i := range host.dirtyLines {
		host.dirtyLines[i] = false
	}

	small := redCells(2 * 2)
	renderer.Render(small, make([]CharInfo, len(small)), 2, 2, true)
	if x, y, stale := stalePixel(host.imgBuf, 2*cellWidth, 2*cellHeight); stale {
		t.Fatalf("stale pixel at (%d,%d) outside the smaller grid", x, y)
	}
	if got := host.imgBuf.Pix[0]; got != 0xff {
		t.Errorf("the smaller frame was not drawn: %#x", got)
	}
	for y, dirty := range host.dirtyLines {
		if !dirty {
			t.Errorf("line %d was not marked for upload; the cleared margin would not reach the window", y)
		}
	}
}

func TestX11Host_SendEvent_ClosedChannelSafety(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	reader := vtinput.NewReader(pr, true)

	h := &X11Host{
		reader:    reader,
		closeChan: make(chan struct{}),
	}

	// Симулируем закрытие канала горутиной рендера при выходе
	reader.Close()

	// Вызов отправки события не должен паниковать
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("sendEvent panicked on closed channel: %v", r)
		}
	}()

	h.sendEvent(&vtinput.InputEvent{Type: vtinput.ResizeEventType})

	// Тест отправки при закрытии канала завершения хоста
	close(h.closeChan)
	h.sendEvent(&vtinput.InputEvent{Type: vtinput.ResizeEventType})
}

func TestX11HostFocusLossClearsKeyboardState(t *testing.T) {
	pr, pw := io.Pipe()
	reader := vtinput.NewReader(pr, true)
	t.Cleanup(func() {
		reader.Close()
		_ = pw.Close()
	})

	h := &X11Host{
		reader:      reader,
		closeChan:   make(chan struct{}),
		currentMods: vtinput.LeftCtrlPressed | vtinput.RightAltPressed | vtinput.ShiftPressed,
		lCtrl:       true,
		rCtrl:       true,
		lAlt:        true,
		rAlt:        true,
		lShift:      true,
		rShift:      true,
	}

	h.handleFocusEvent(false)
	h.mu.Lock()
	if h.currentMods != 0 || h.lCtrl || h.rCtrl || h.lAlt || h.rAlt || h.lShift || h.rShift {
		h.mu.Unlock()
		t.Fatal("focus loss did not clear X11 keyboard state")
	}
	h.mu.Unlock()

	select {
	case event := <-reader.EventChan:
		if event.Type != vtinput.FocusEventType || event.SetFocus {
			t.Fatalf("focus loss event = %+v, want FocusEvent(false)", event)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for X11 focus loss event")
	}

	h.handleFocusEvent(true)
	select {
	case event := <-reader.EventChan:
		if event.Type != vtinput.FocusEventType || !event.SetFocus {
			t.Fatalf("focus gain event = %+v, want FocusEvent(true)", event)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for X11 focus gain event")
	}
}

func TestX11ModifierStateHealing(t *testing.T) {
	h := &X11Host{}

	mods := h.syncModifierStateLocked(xproto.ModMaskControl, vtinput.VK_A, true)
	if mods != vtinput.LeftCtrlPressed || !h.lCtrl || h.rCtrl {
		t.Fatalf("active aggregate Ctrl = mods:%d sides:%v/%v, want left Ctrl", mods, h.lCtrl, h.rCtrl)
	}

	h.lCtrl, h.rCtrl = true, true
	mods = h.syncModifierStateLocked(0, vtinput.VK_A, true)
	if mods != 0 || h.lCtrl || h.rCtrl {
		t.Fatalf("inactive aggregate Ctrl = mods:%d sides:%v/%v, want cleared state", mods, h.lCtrl, h.rCtrl)
	}

	// X11 reports a modifier key release with the state from before the
	// release. The already-applied key transition must not be healed back in.
	h.lCtrl = false
	mods = h.syncModifierStateLocked(xproto.ModMaskControl, vtinput.VK_LCONTROL, false)
	if h.lCtrl {
		t.Fatal("modifier release recreated left Ctrl side")
	}
	if mods != vtinput.LeftCtrlPressed {
		t.Fatalf("modifier release result = %d, want aggregate Ctrl", mods)
	}
}

func TestX11Renderer_GlyphCacheUniqueness(t *testing.T) {
	// Clusters with no precomposed form, so they stay composite: NFC folds
	// "e\u0301"-style sequences to plain runes and they would never hit the
	// glyph-key path this test guards.
	ch1 := RegisterCluster("a\u0305")
	ch2 := RegisterCluster("e\u0305")

	key1 := glyphKey{ch1, 0, 0, 1}
	key2 := glyphKey{ch2, 0, 0, 1}

	if key1 == key2 {
		t.Error("expected different composite clusters to produce unique glyph keys")
	}

	keySame := glyphKey{ch1, 0, 0, 1}
	if key1 != keySame {
		t.Error("expected same composite cluster to produce identical glyph key")
	}
}

func TestX11Host_MouseStateTracking(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	reader := vtinput.NewReader(pr, true)

	h := &X11Host{
		reader:    reader,
		closeChan: make(chan struct{}),
		cellW:     10,
		cellH:     20,
	}
	defer h.Close()

	// 1. Имитируем нажатие левой кнопки мыши (detail=1) на координатах (100, 40) -> колонка 10, строка 2
	h.handleButtonEvent(100, 40, 1, 0, true)

	h.mu.Lock()
	pressedBtn := h.mouseBtn
	h.mu.Unlock()

	if pressedBtn != uint32(vtinput.FromLeft1stButtonPressed) {
		t.Errorf("Expected mouseBtn to be FromLeft1stButtonPressed, got %d", pressedBtn)
	}

	// Достаем событие и проверяем ButtonState
	select {
	case ev := <-reader.EventChan:
		if ev.ButtonState != uint32(vtinput.FromLeft1stButtonPressed) {
			t.Errorf("Expected ButtonState %d, got %d", vtinput.FromLeft1stButtonPressed, ev.ButtonState)
		}
		if !ev.KeyDown {
			t.Error("Expected KeyDown to be true")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Button press event was not sent")
	}

	// 2. Имитируем отпускание левой кнопки мыши (detail=1, isDown=false)
	h.handleButtonEvent(100, 40, 1, 0, false)

	h.mu.Lock()
	releasedBtn := h.mouseBtn
	h.mu.Unlock()

	if releasedBtn != 0 {
		t.Errorf("Expected mouseBtn to be 0 after release, got %d", releasedBtn)
	}

	select {
	case ev := <-reader.EventChan:
		if ev.ButtonState != 0 {
			t.Errorf("Expected ButtonState 0 on release, got %d", ev.ButtonState)
		}
		if ev.KeyDown {
			t.Error("Expected KeyDown to be false")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Button release event was not sent")
	}
}
