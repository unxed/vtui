//go:build (linux || windows || darwin) && !android && (amd64 || arm64)

package vtui

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/unxed/vtinput"
)

// EbitenHost drives a vtui application inside an Ebitengine window.
//
// The point of this backend is that it needs no cgo on Linux, Windows and
// macOS alike, so vtui's GUI mode does not depend on a single graphics stack.
// Ebitengine reaches the platform through purego, which means the whole thing
// still cross-compiles with CGO_ENABLED=0.
type EbitenHost struct {
	mu sync.Mutex

	renderer *EbitenRenderer
	reader   *vtinput.Reader
	scr      *ScreenBuf

	cols, rows   int
	cellW, cellH int

	// scale is the display scale factor. Ebitengine's window API and the
	// framebuffer disagree about units on a HiDPI screen: SetWindowSize takes
	// device-independent pixels while the cells are measured in real ones, so
	// every crossing between the two goes through this.
	scale int

	// Last window size seen by Layout, in logical pixels.
	winW, winH int

	// Mouse state, kept so that a drag reports the button that started it and
	// so that a move with no button down is not sent at all.
	mouseBtn   uint32
	lastMouseX int
	lastMouseY int

	// Buffers reused every frame to keep the input path allocation-free.
	keyBuf     []ebiten.Key
	charBuf    []rune
	pressedBuf []ebiten.Key
	heldBuf    []ebiten.Key

	// phantomMods holds modifier keys Ebitengine still reports as pressed but
	// which demonstrably are not. Switching keyboard layout with Alt+Shift is
	// grabbed by the desktop, which swallows the release, and GLFW never
	// learns the key came back up: from then on every keystroke looks like an
	// Alt chord and typing stops working entirely. An entry is dropped once
	// the key is genuinely reported up again.
	phantomMods map[ebiten.Key]bool

	// lastRuneForVK remembers which character a physical key produced when it
	// was pressed unmodified, so a later Alt chord on the same key can carry
	// that character. Alt+letter and Alt+digit are quick-search accelerators
	// and need the rune, not just the virtual key.
	lastRuneForVK map[uint16]rune

	focused bool

	// pendingChord holds a Ctrl or Alt chord for one tick before sending it.
	// The evidence that a modifier is stuck is a printable character, and that
	// character can arrive a tick after the key that produced it, by which
	// time an immediately sent chord has already been acted on. Waiting one
	// tick means a chord that turns out to be phantom is dropped instead of
	// retracted, which is not possible.
	pendingChord *vtinput.InputEvent
	pendingKeys  []ebiten.Key

	pendingSize struct {
		w, h  int
		valid bool
	}
}

func (h *EbitenHost) sendEvent(ev *vtinput.InputEvent) {
	if h.reader == nil || h.reader.EventChan == nil {
		return
	}
	select {
	case h.reader.EventChan <- ev:
	default:
		// A full queue means the app is behind. Mouse motion is the only
		// event stream dense enough to cause that and the only one where
		// dropping an intermediate sample costs nothing, so it goes first.
		if ev.Type == vtinput.MouseEventType && (ev.MouseEventFlags&vtinput.MouseMoved) != 0 {
			return
		}
		select {
		case h.reader.EventChan <- ev:
		default:
			DebugLog("EBITEN_HOST: dropped event, queue full: %s", ev.String())
		}
	}
}

// resolveModifiers reads the modifier state and filters out keys that are
// stuck down.
//
// Ebitengine has no authoritative per-event modifier state the way X11 does,
// so a swallowed release cannot simply be corrected from a later event. What
// it does give is a contradiction to work from: if the platform produced a
// printable character this frame, then Ctrl and Alt cannot really be held,
// because no layout emits plain text while they are. That is enough to catch
// the stuck key, and the suspicion is remembered until Ebitengine reports the
// key up, so the state does not flip back on the next frame.
func (h *EbitenHost) resolveModifiers(sawText bool) vtinput.ControlKeyState {
	if h.phantomMods == nil {
		h.phantomMods = make(map[ebiten.Key]bool)
	}
	for k := range h.phantomMods {
		if !ebiten.IsKeyPressed(k) {
			delete(h.phantomMods, k)
		}
	}
	if sawText {
		for _, k := range [...]ebiten.Key{
			ebiten.KeyControlLeft, ebiten.KeyControlRight,
			ebiten.KeyAltLeft, ebiten.KeyAltRight,
		} {
			if ebiten.IsKeyPressed(k) && !h.phantomMods[k] {
				DebugLog("EBITEN_HOST: %v reported held while text arrived; treating it as stuck", k)
				h.phantomMods[k] = true
			}
		}
	}

	var mods vtinput.ControlKeyState
	pressed := func(k ebiten.Key) bool { return ebiten.IsKeyPressed(k) && !h.phantomMods[k] }
	if pressed(ebiten.KeyShiftLeft) || pressed(ebiten.KeyShiftRight) {
		mods |= vtinput.ShiftPressed
	}
	if pressed(ebiten.KeyControlLeft) {
		mods |= vtinput.LeftCtrlPressed
	}
	if pressed(ebiten.KeyControlRight) {
		mods |= vtinput.RightCtrlPressed
	}
	if pressed(ebiten.KeyAltLeft) {
		mods |= vtinput.LeftAltPressed
	}
	if pressed(ebiten.KeyAltRight) {
		mods |= vtinput.RightAltPressed
	}
	if ebiten.IsCapsLockOn() {
		mods |= vtinput.CapsLockOn
	}
	if ebiten.IsNumLockOn() {
		mods |= vtinput.NumLockOn
	}
	return mods
}

// settlePendingChord decides the fate of a chord held over from last tick.
//
// A printable character this tick means the modifier was never really down,
// so the chord was an artefact of a swallowed key release and must not reach
// the application at all. Dropping it is only possible because it was held
// back; once sent, a chord cannot be retracted.
func (h *EbitenHost) settlePendingChord(sawText bool) {
	if h.pendingChord == nil {
		return
	}
	chord := h.pendingChord
	h.pendingChord = nil
	if sawText {
		DebugLog("EBITEN_HOST: dropping chord vk=%d, text arrived so the modifier was stuck",
			chord.VirtualKeyCode)
		return
	}
	h.sendEvent(chord)
}

// keyBehindText names the key a character came from, when exactly one key can
// be responsible.
//
// Ebitengine reports text and keys as separate streams with no link between
// them, so this is attribution rather than fact. A key that went down this
// tick is the obvious candidate; failing that, a single held non-modifier key
// covers auto-repeat, where the character keeps arriving with no new press.
// Anything more ambiguous is left alone, since a wrong attribution would
// label the key with someone else's rune and mislead every later Alt chord.
func (h *EbitenHost) keyBehindText(mods vtinput.ControlKeyState) (ebiten.Key, bool) {
	if len(h.pressedBuf) == 1 {
		return h.pressedBuf[0], true
	}
	if len(h.pressedBuf) > 1 {
		return 0, false
	}

	var found ebiten.Key
	n := 0
	for _, k := range h.heldBuf {
		vk := ebitenKeyToVK(k, mods)
		if vk == 0 || isModifierVK(vk) {
			continue
		}
		found = k
		n++
		if n > 1 {
			return 0, false
		}
	}
	return found, n == 1
}

// charForVK returns the character a modified key should carry.
//
// It prefers what the key actually produced when pressed unmodified on this
// keyboard, and falls back to the ASCII the virtual key names. Without this a
// quick-search accelerator such as Alt+1 arrives with no character at all and
// the application has nothing to search for.
func (h *EbitenHost) charForVK(vk uint16) rune {
	if r, ok := h.lastRuneForVK[vk]; ok && r != 0 {
		return r
	}
	return defaultRuneForVK(vk)
}

// requestSize records a resize asked for by the UI. The actual call happens on
// the game loop, because Ebitengine's window functions want the main thread.
func (h *EbitenHost) requestSize(w, h2 int) {
	h.mu.Lock()
	h.pendingSize.w, h.pendingSize.h, h.pendingSize.valid = w, h2, true
	h.mu.Unlock()
}

// ebitenGame is the ebiten.Game implementation. Update translates input,
// Draw uploads whatever the renderer has rasterised.
type ebitenGame struct {
	host *EbitenHost
	tex  *ebiten.Image

	// presented says the screen already holds a frame. Ebitengine is a fixed
	// tick loop with no render-on-demand, but with clearing disabled it keeps
	// the previous contents, so once a frame is up an unchanged UI needs
	// neither an upload nor a blit and Draw becomes free.
	presented bool
}

func (g *ebitenGame) Update() error {
	h := g.host

	h.mu.Lock()
	if h.pendingSize.valid {
		w, ph := h.pendingSize.w, h.pendingSize.h
		h.pendingSize.valid = false
		sc := h.scale
		h.mu.Unlock()
		if sc < 1 {
			sc = 1
		}
		ebiten.SetWindowSize(w/sc, ph/sc)
	} else {
		h.mu.Unlock()
	}

	if title, ok := h.renderer.takeTitle(); ok {
		ebiten.SetWindowTitle(WindowTitleWithBackend(title))
	}

	// Focus changes reset the modifier picture. The framework already treats a
	// focus event as a cue to drop its own modifier state, and a chord held
	// while the window went away is not held when it comes back.
	if focused := ebiten.IsFocused(); focused != h.focused {
		h.focused = focused
		h.phantomMods = nil
		h.sendEvent(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: focused})
	}

	// Text is collected before the keys, because a printable character is the
	// evidence that decides whether a modifier is really down.
	h.charBuf = ebiten.AppendInputChars(h.charBuf[:0])
	sawText := len(h.charBuf) > 0
	mods := h.resolveModifiers(sawText)

	// Settle last tick's chord now that this tick's text is known.
	h.settlePendingChord(sawText)

	// Which physical keys went down this frame, used below to attribute an
	// incoming character to the key that produced it.
	h.pressedBuf = inpututil.AppendJustPressedKeys(h.pressedBuf[:0])

	// Keys first, then text. A modified or special key is delivered by virtual
	// key code; an unmodified printable key is left to the text stream below,
	// because only the platform knows what character the current layout
	// produces for that physical key. Sending both would double every
	// keystroke.
	//
	// Repeat is synthesised from how long each key has been held. Ebitengine
	// reports only the transition into the pressed state, so without this a
	// held arrow key moves the cursor exactly once. Printable keys are left
	// out: the platform already repeats those through the text stream, and
	// adding to it would repeat them twice.
	tps := ebiten.TPS()
	// Held keys go in their own buffer: keyBuf is reused for released keys
	// below, and the text loop still needs to know what was down.
	h.heldBuf = inpututil.AppendPressedKeys(h.heldBuf[:0])
	for _, k := range h.heldBuf {
		vk := ebitenKeyToVK(k, mods)
		if vk == 0 || !isSpecialOrModifiedKey(vk, mods) {
			continue
		}
		d := inpututil.KeyPressDuration(k)
		if isModifierVK(vk) {
			// A modifier is a state, not a stream: report it once when it
			// goes down and then leave it alone.
			if d != 1 {
				continue
			}
		} else if !keyRepeatFires(d, tps) {
			continue
		}
		ev := &vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         true,
			VirtualKeyCode:  vk,
			Char:            h.charForVK(vk),
			ControlKeyState: mods,
		}
		if isEbitenEnhancedNavKey(k) {
			ev.ControlKeyState |= vtinput.EnhancedKey
		}
		DebugLog("EBITEN_HOST: key vk=%d char=%q mods=%d held=%d", vk, ev.Char, mods, d)

		// Only Ctrl and Alt chords wait. An inherently special key such as an
		// arrow or a function key cannot be contradicted by text, and delaying
		// it would cost repeat responsiveness for nothing.
		if mods&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed|vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0 &&
			!isModifierVK(vk) {
			if h.pendingChord != nil {
				h.sendEvent(h.pendingChord)
			}
			h.pendingChord = ev
			continue
		}
		h.sendEvent(ev)
	}

	h.keyBuf = inpututil.AppendJustReleasedKeys(h.keyBuf[:0])
	for _, k := range h.keyBuf {
		vk := ebitenKeyToVK(k, mods)
		if vk == 0 {
			continue
		}
		h.sendEvent(&vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         false,
			VirtualKeyCode:  vk,
			ControlKeyState: mods,
		})
	}

	// Text input. Ctrl and Alt combinations are filtered out: some platforms
	// still emit a control character for them, and the virtual key event above
	// has already covered that keystroke.
	if mods&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed|vtinput.LeftAltPressed|vtinput.RightAltPressed) == 0 {
		for i, r := range h.charBuf {
			if r < 0x20 || r == 0x7f {
				continue
			}
			// Attribute the character to the key that produced it, when that
			// can be said without guessing. It gives the event a virtual key
			// to go with the character, so a press and its release describe
			// the same key; a press reported as VK 0 followed by a release
			// reporting VK_B is a pair nothing downstream can match up.
			//
			// The same attribution teaches which rune this key yields on this
			// layout, so a later Alt chord on it can carry that rune.
			var vk uint16
			if i == 0 && len(h.charBuf) == 1 {
				if k, ok := h.keyBehindText(mods); ok {
					vk = ebitenKeyToVK(k, mods)
					if vk != 0 {
						h.lastRuneForVK[vk] = r
					}
				}
			}
			h.sendEvent(&vtinput.InputEvent{
				Type:            vtinput.KeyEventType,
				KeyDown:         true,
				VirtualKeyCode:  vk,
				Char:            r,
				ControlKeyState: mods,
			})
		}
	}

	g.updateMouse(mods)
	g.pollDroppedFiles(mods)
	return nil
}

func (g *ebitenGame) updateMouse(mods vtinput.ControlKeyState) {
	h := g.host

	h.mu.Lock()
	cw, ch := h.cellW, h.cellH
	h.mu.Unlock()
	if cw <= 0 || ch <= 0 {
		return
	}

	px, py := ebiten.CursorPosition()
	cx, cy := px/cw, py/ch

	for _, b := range [...]struct {
		btn  ebiten.MouseButton
		mask uint32
	}{
		{ebiten.MouseButtonLeft, uint32(vtinput.FromLeft1stButtonPressed)},
		{ebiten.MouseButtonMiddle, uint32(vtinput.FromLeft2ndButtonPressed)},
		{ebiten.MouseButtonRight, uint32(vtinput.RightmostButtonPressed)},
	} {
		if inpututil.IsMouseButtonJustPressed(b.btn) {
			h.mu.Lock()
			h.mouseBtn |= b.mask
			btn := h.mouseBtn
			h.mu.Unlock()
			h.sendEvent(&vtinput.InputEvent{
				Type:            vtinput.MouseEventType,
				MouseX:          int16(cx),
				MouseY:          int16(cy),
				KeyDown:         true,
				ButtonState:     btn,
				ControlKeyState: mods,
			})
		}
		if inpututil.IsMouseButtonJustReleased(b.btn) {
			h.mu.Lock()
			h.mouseBtn &^= b.mask
			btn := h.mouseBtn
			h.mu.Unlock()
			h.sendEvent(&vtinput.InputEvent{
				Type:            vtinput.MouseEventType,
				MouseX:          int16(cx),
				MouseY:          int16(cy),
				KeyDown:         false,
				ButtonState:     btn,
				ControlKeyState: mods,
			})
		}
	}

	// Motion is reported per cell, not per pixel: the UI works in cells, and
	// a pixel-granular stream would flood the queue during a drag.
	h.mu.Lock()
	moved := cx != h.lastMouseX || cy != h.lastMouseY
	h.lastMouseX, h.lastMouseY = cx, cy
	btn := h.mouseBtn
	h.mu.Unlock()

	if moved && btn != 0 {
		h.sendEvent(&vtinput.InputEvent{
			Type:            vtinput.MouseEventType,
			MouseX:          int16(cx),
			MouseY:          int16(cy),
			MouseEventFlags: vtinput.MouseMoved,
			ButtonState:     btn,
			ControlKeyState: mods,
		})
	}

	if _, dy := ebiten.Wheel(); dy != 0 {
		steps := int(dy)
		if steps == 0 {
			if dy > 0 {
				steps = 1
			} else {
				steps = -1
			}
		}
		if steps < 0 {
			steps = -steps
		}
		// One wheel notch produces one event; consumers decide how many
		// lines a notch scrolls (see WheelLinesPerNotch).
		dir := -1
		if dy > 0 {
			dir = 1
		}
		for i := 0; i < steps; i++ {
			h.sendEvent(&vtinput.InputEvent{
				Type:            vtinput.MouseEventType,
				MouseX:          int16(cx),
				MouseY:          int16(cy),
				WheelDirection:  dir,
				ControlKeyState: mods,
			})
		}
	}
}

func (g *ebitenGame) Draw(screen *ebiten.Image) {
	pix, w, h, changed := g.host.renderer.takeFrame()
	if pix == nil || w <= 0 || h <= 0 {
		return
	}

	if g.tex == nil || g.tex.Bounds().Dx() != w || g.tex.Bounds().Dy() != h {
		g.tex = ebiten.NewImage(w, h)
		changed = true
		g.presented = false
	}

	// Nothing moved and the screen already shows the last frame: skip both the
	// upload and the blit. This is what keeps an idle file manager off the GPU.
	if !changed && g.presented {
		return
	}
	if changed {
		g.tex.WritePixels(pix)
	}
	screen.DrawImage(g.tex, nil)
	g.presented = true
}

// Layout maps the window size onto the character grid and tells the running
// application when that grid changed.
func (g *ebitenGame) Layout(outsideWidth, outsideHeight int) (int, int) {
	h := g.host

	h.mu.Lock()
	scale := h.scale
	if scale < 1 {
		scale = 1
	}
	// The logical screen is requested at full device resolution so that the
	// framebuffer maps one to one onto physical pixels; anything less would
	// have Ebitengine upscale text that was rasterised sharp.
	pixW, pixH := outsideWidth*scale, outsideHeight*scale

	changed := pixW != h.winW || pixH != h.winH
	h.winW, h.winH = pixW, pixH
	if h.cellW > 0 && h.cellH > 0 {
		cols, rows := pixW/h.cellW, pixH/h.cellH
		if cols < 1 {
			cols = 1
		}
		if rows < 1 {
			rows = 1
		}
		if cols != h.cols || rows != h.rows {
			h.cols, h.rows = cols, rows
			changed = true
		} else {
			changed = false
		}
	}
	h.mu.Unlock()

	if changed {
		h.sendEvent(&vtinput.InputEvent{Type: vtinput.ResizeEventType})
	}
	return pixW, pixH
}

// RunEbitenHost opens an Ebitengine window and runs the application in it.
// It blocks until the window closes, and must be called from the main
// goroutine because that is where Ebitengine insists on running its loop.
func RunEbitenHost(cols, rows int, fontName string, fontSize float64, setupApp func()) error {
	// The scale factor has to be known before the font is measured, because a
	// HiDPI screen needs the face rasterised at the larger size. Scaling a
	// face built for 96dpi up afterwards is what makes text look soft.
	// DeviceScaleFactor is readable before RunGame; if the window later moves
	// to a monitor with a different factor the cells keep their pixel size,
	// which is a visible but not a broken result and is left for later.
	var monitorScale float64 = 1.0
	if m := ebiten.Monitor(); m != nil {
		monitorScale = m.DeviceScaleFactor()
	}
	scale := int(monitorScale + 0.5)
	if scale < 1 {
		scale = 1
	}

	face, cellW, cellH := loadBestFont(fontName, fontSize*float64(scale), 72)
	if cellW <= 0 || cellH <= 0 {
		cellW, cellH = 7*scale, 13*scale
	}
	DebugLog("EBITEN_HOST: starting %dx%d cells, cell size %dx%d, scale %d", cols, rows, cellW, cellH, scale)

	host := &EbitenHost{
		cols:       cols,
		rows:       rows,
		cellW:      cellW,
		cellH:      cellH,
		scale:      scale,
		winW:       cols * cellW,
		winH:       rows * cellH,
		lastMouseX: -1,
		lastMouseY: -1,

		phantomMods:   make(map[ebiten.Key]bool),
		lastRuneForVK: make(map[uint16]rune),
		focused:       true,
	}

	renderer := NewEbitenRenderer(host, face, cellW, cellH, scale)
	host.renderer = renderer

	scr := NewScreenBuf()
	scr.AllocBuf(cols, rows)
	scr.Renderer = renderer
	// Without this the layer keeps whatever protocol was detected for a
	// terminal, and the viewer falls back to showing a JPEG as text even
	// though this backend can draw it.
	scr.Graphics().SetProtocol(GraphicsNative)
	host.scr = scr
	FrameManager.Init(scr)

	// vtinput normally parses a terminal byte stream. Here the events are
	// synthesised directly, so the reader is handed a pipe that never
	// produces anything and is used purely for its event channel.
	pr, _ := io.Pipe()
	reader := vtinput.NewReader(pr, true)
	host.reader = reader

	GetTerminalSize = func() (int, int, error) {
		host.mu.Lock()
		defer host.mu.Unlock()
		return host.cols, host.rows, nil
	}

	// Copy and paste reach the OS through the shared clipboard helpers, which
	// need nothing backend specific: the native path on Windows, and wl-copy,
	// xclip, xsel or pbcopy elsewhere. Only the terminal escape fallback has
	// to go, since this is a window and there is no terminal to receive it.
	DisableTerminalClipboard()
	SetDragBackend(host)

	setupApp()

	// Announced after setupApp on purpose. The application installs the debug
	// log sink during setup, so everything logged before that point is written
	// nowhere and the backend line never appears in a bug report.
	SetActiveBackend("ebiten",
		fmt.Sprintf("cell %dx%d, scale %d, font %q", cellW, cellH, scale, fontName),
		"cgo-free: Ebitengine over purego")
	// One wheel notch arrives as a single event now; keep widgets scrolling
	// the system-configured number of lines per notch as before.
	setWheelNotchLines(getSystemScrollLines())

	ebiten.SetWindowTitle(WindowTitleWithBackend(AppName))
	ebiten.SetWindowSize(cols*cellW/scale, rows*cellH/scale)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	// The screen is fully repainted from our own framebuffer every frame, so
	// letting Ebitengine clear it first would only waste a pass.
	ebiten.SetScreenClearedEveryFrame(false)

	game := &ebitenGame{host: host}

	go func() {
		defer LogAndRepanic("ebiten FrameManager")
		FrameManager.Run(reader)
		DebugLog("EBITEN_HOST: FrameManager exited, shutting down")
		os.Exit(0)
	}()

	err := ebiten.RunGame(game)
	DebugLog("EBITEN_HOST: RunGame returned: %v", err)
	return err
}
