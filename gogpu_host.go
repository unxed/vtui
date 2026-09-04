//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris && !plan9 && !android && (amd64 || arm64)

package vtui

import (
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gogpu/gg/text"
	"github.com/gogpu/gogpu"
	"github.com/gogpu/gpucontext"
	"github.com/unxed/vtinput"
)

var (
	debugLastMouseX, debugLastMouseY float64 = -1, -1
	debugLastCtxW, debugLastCtxH     int     = -1, -1
)

// gogpuCmdIsCtrl folds the Super modifier into Ctrl. On macOS the Command key
// is the shortcut modifier — Cmd+C must reach the application as Ctrl+C or no
// clipboard shortcut works at all — while on Windows and Linux the Super/Win
// key belongs to the OS and must stay out of the application's way.
var gogpuCmdIsCtrl = runtime.GOOS == "darwin"

// gogpuAltComposesText says the platform makes a chord type a character of its
// own instead of leaving the key its own.
//
// macOS is the one that does. Option is a compose modifier there, so
// [NSEvent characters] for Option+T is "†" and for Option+S is "ß", and the
// backend hands that over as ordinary text input. An accelerator reading it as
// the chord's character searches for "†" instead of for "t" — the terminal
// never has this problem, because Alt+T reaches an application as ESC t.
//
// Windows and Linux compose nothing for a plain Alt chord: WM_SYSCHAR and
// xkbcommon both report the key's own character for it. Alt with Ctrl is
// AltGr there, and composing is its entire purpose (AltGr+E is €). Their text
// is right, so it stays as it is, and the character stays out of the key
// event — filling both would type the chord twice.
var gogpuAltComposesText = runtime.GOOS == "darwin"

type GogpuHost struct {
	mu              sync.Mutex
	app             *gogpu.App
	reader          *vtinput.Reader
	scr             *ScreenBuf
	cols, rows      int
	cellW, cellH    int
	face            text.Face
	mouseBtn        uint32
	currentMods     vtinput.ControlKeyState
	pendingKeyEvent *vtinput.InputEvent
	pendingKeyTimer *time.Timer
	lastRuneForVK   map[uint16]rune
	lastVK          uint16
	// suppressTextInput drops the text belonging to the keystroke just
	// handled, because the keystroke has already been delivered as a virtual
	// key: a keypad key in navigation mode whose digit some platforms hand
	// over anyway, or a chord whose composed symbol macOS reports as text.
	// One-shot; see gogpuKeystrokeSwallowsText.
	suppressTextInput bool
	// The side flags pick which side syncMods reports for a chord. The Ctrl
	// pair only matters where the Cmd fold is off; with it on, each Ctrl bit
	// derives from the event's own modifier flags instead.
	lCtrl, rCtrl   bool
	lAlt, rAlt     bool
	lShift, rShift bool
	// superDown mirrors the Super (Command) modifier of the last key event.
	// With Cmd folded into Ctrl, macOS still reports the plain character for
	// the shortcut key ([NSEvent characters] for Cmd+C is "c"), and that text
	// must not additionally arrive as typed input.
	superDown bool

	// Cached sizes to prevent deadlocks and speed up GetTerminalSize
	lastAppW, lastAppH int
	resizePending      bool
	// dragOut is the gesture waiting for the main loop to hand it to
	// gogpu, or nil. One pointer, so one gesture at a time.
	dragOut *gogpuDragRequest
}

func (h *GogpuHost) sendEvent(ev *vtinput.InputEvent) {
	if h.reader == nil || h.reader.EventChan == nil {
		return
	}
	select {
	case h.reader.EventChan <- ev:
	default:
		// Drop intermediate mouse move events when queue is full to prevent clogging
		if ev.Type == vtinput.MouseEventType && (ev.MouseEventFlags&vtinput.MouseMoved) != 0 {
			return
		}
		// For non-move critical events, attempt a secondary non-blocking send without spawning goroutines
		select {
		case h.reader.EventChan <- ev:
		default:
			DebugLog("GOGPU_HOST: dropped event due to full buffer: %s", ev.String())
		}
	}
}

// charForVK returns the character a modified key should carry: the one this
// key was last seen to type on the current layout, or else the character its
// virtual key is named after.
//
// The layout memory is what keeps Alt+ф searching for "ф" instead of for the
// "a" engraved on the same key; the fallback covers the accelerator pressed
// before its key has ever been typed unmodified, which is the usual case for
// one.
func (h *GogpuHost) charForVK(vk uint16) rune {
	if r := h.lastRuneForVK[vk]; r != 0 {
		return r
	}
	return defaultRuneForVK(vk)
}

// gogpuKeystrokeSwallowsText reports whether the text following this key press
// is a leftover to drop rather than typing: the digit of a keypad key already
// delivered as navigation, or — where the platform composes for chords — the
// composition of a chord already delivered by virtual key code ("†" for
// Option+T).
//
// The decision is made here, on the key press itself, and carried by the
// one-shot suppressTextInput flag. Text that arrives without a key press of
// its own — the character viewer, an IME commit — never had a chord press
// before it, so nothing marks it for dropping no matter what modifier state
// was left behind.
func gogpuKeystrokeSwallowsText(key gpucontext.Key, vk uint16, mods vtinput.ControlKeyState) bool {
	if isGogpuKeypadKey(key) && gogpuNumpadRune(vk) == 0 {
		return true
	}
	// Delete has no text of its own, but some xkbcommon layouts (seen on
	// Arch/KDE Plasma) still fire OnTextInput for it with the ASCII DEL
	// character (0x7F), which most fonts render as a blank box — it then
	// gets typed right after Delete has already removed a character. See
	// f4 issue #519.
	if key == gpucontext.KeyDelete {
		return true
	}
	return gogpuAltComposesText && mods&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed|
		vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0
}

// handleTextInput takes the text the platform made of a keystroke and decides
// whether it is typing — in which case it becomes the character of the key
// press waiting for it, or an event of its own — or the leftovers of a chord
// that has already been delivered.
func (h *GogpuHost) handleTextInput(text string) {
	DebugLog("GOGPU_HOST_EVENT: OnTextInput text=%q", text)
	h.mu.Lock()
	defer h.mu.Unlock()

	runes := []rune(text)
	if len(runes) == 0 {
		return
	}

	// This text belongs to a keystroke already delivered whole — a keypad
	// key resolved to navigation, or a chord whose composition this is (see
	// gogpuKeystrokeSwallowsText). The key press flushed any pending event
	// before setting the flag, so there is nothing here to pair the text
	// with either. Dropping it also keeps a composed character out of
	// lastRuneForVK below: a key labelled "†" would go on standing for it
	// in every later chord and key repeat.
	if h.suppressTextInput {
		h.suppressTextInput = false
		DebugLog("GOGPU_HOST_EVENT: dropped text %q, keystroke already delivered as a key", text)
		return
	}

	// A Cmd chord is a shortcut, not typing. It already went out as a
	// Ctrl-modified key event, but macOS reports the plain character for
	// it anyway; the other platforms deliver no printable text for their
	// Ctrl chords, so none may be delivered here either.
	if gogpuCmdIsCtrl && h.superDown {
		DebugLog("GOGPU_HOST_EVENT: dropped text %q, Super held (Cmd shortcut)", text)
		return
	}

	if h.lastRuneForVK == nil {
		h.lastRuneForVK = make(map[uint16]rune)
	}

	if h.pendingKeyEvent != nil {
		if h.pendingKeyTimer != nil {
			h.pendingKeyTimer.Stop()
		}
		h.pendingKeyEvent.Char = runes[0]
		h.lastRuneForVK[h.pendingKeyEvent.VirtualKeyCode] = runes[0]
		h.sendEvent(h.pendingKeyEvent)
		h.pendingKeyEvent = nil

		for i := 1; i < len(runes); i++ {
			h.sendEvent(&vtinput.InputEvent{
				Type:            vtinput.KeyEventType,
				KeyDown:         true,
				Char:            runes[i],
				ControlKeyState: h.currentMods,
			})
		}
		return
	}

	// Control characters stay out of the layout memory: macOS reports "\r"
	// for Return, and a key learned as one would carry it into every later
	// chord through charForVK.
	if h.lastVK != 0 && runes[0] >= ' ' && runes[0] != 0x7f {
		h.lastRuneForVK[h.lastVK] = runes[0]
	}
	for _, r := range runes {
		h.sendEvent(&vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         true,
			Char:            r,
			ControlKeyState: h.currentMods,
		})
	}
}

func (h *GogpuHost) syncMods(key gpucontext.Key, mods gpucontext.Modifiers, isDown bool) vtinput.ControlKeyState {
	vk := gogpuKeyToVK(key, 0)
	switch vk {
	case vtinput.VK_LCONTROL:
		h.lCtrl = isDown
	case vtinput.VK_RCONTROL:
		h.rCtrl = isDown
	case vtinput.VK_LMENU:
		h.lAlt = isDown
	case vtinput.VK_RMENU:
		h.rAlt = isDown
	case vtinput.VK_LSHIFT:
		h.lShift = isDown
	case vtinput.VK_RSHIFT:
		h.rShift = isDown
	}

	h.superDown = mods.HasSuper()
	h.syncModifierStateLocked(vk, mods, isDown)

	var sysMods vtinput.ControlKeyState
	if mods.HasShift() {
		sysMods |= vtinput.ShiftPressed
	}
	if gogpuCmdIsCtrl {
		// Channel split: Cmd is the left Ctrl channel, physical Ctrl the
		// right one. Both bits may be set at once; consumers OR them.
		if mods.HasSuper() {
			sysMods |= vtinput.LeftCtrlPressed
		}
		if mods.HasControl() {
			sysMods |= vtinput.RightCtrlPressed
		}
	} else if mods.HasControl() {
		if h.rCtrl {
			sysMods |= vtinput.RightCtrlPressed
		} else {
			sysMods |= vtinput.LeftCtrlPressed
		}
	}
	if mods.HasAlt() {
		if h.rAlt {
			sysMods |= vtinput.RightAltPressed
		} else {
			sysMods |= vtinput.LeftAltPressed
		}
	}

	// Lock states. gpucontext has no Has* helper for these, but the platform
	// layer fills both bits on every event. Without them isNumLockEffectiveGogpu
	// always sees NumLock as off, and the keypad can never reach numeric mode.
	// X11 and Ebitengine already report the locks; the framework masks them out
	// where they must not affect matching (see FrameManager and Edit).
	if mods&gpucontext.ModCapsLock != 0 {
		sysMods |= vtinput.CapsLockOn
	}
	if mods&gpucontext.ModNumLock != 0 {
		sysMods |= vtinput.NumLockOn
	}

	h.currentMods = sysMods
	return sysMods
}

func isGogpuModifierVK(vk uint16) bool {
	switch vk {
	case vtinput.VK_SHIFT, vtinput.VK_LSHIFT, vtinput.VK_RSHIFT,
		vtinput.VK_CONTROL, vtinput.VK_LCONTROL, vtinput.VK_RCONTROL,
		vtinput.VK_MENU, vtinput.VK_LMENU, vtinput.VK_RMENU,
		vtinput.VK_LWIN, vtinput.VK_RWIN, vtinput.VK_APPS,
		vtinput.VK_CAPITAL, vtinput.VK_NUMLOCK, vtinput.VK_SCROLL:
		return true
	default:
		return false
	}
}

// syncModifierStateLocked repairs the side-tracking flags from gogpu's
// aggregate modifier mask. A modifier release can be lost during a focus
// transition, while an ordinary key event still gives us a reliable snapshot
// of which modifier groups are active. Modifier key presses remain
// authoritative when the platform reports its mask one event late.
func (h *GogpuHost) syncModifierStateLocked(vk uint16, mods gpucontext.Modifiers, isDown bool) {
	assumeActive := !isGogpuModifierVK(vk)
	keepShift := isDown && (vk == vtinput.VK_SHIFT || vk == vtinput.VK_LSHIFT || vk == vtinput.VK_RSHIFT)
	keepAlt := isDown && (vk == vtinput.VK_MENU || vk == vtinput.VK_LMENU || vk == vtinput.VK_RMENU)

	if !mods.HasShift() && !keepShift {
		h.lShift, h.rShift = false, false
	} else if mods.HasShift() && assumeActive && !h.lShift && !h.rShift {
		h.lShift = true
	}
	if !mods.HasAlt() && !keepAlt {
		h.lAlt, h.rAlt = false, false
	} else if mods.HasAlt() && assumeActive && !h.lAlt && !h.rAlt {
		h.lAlt = true
	}

	keepCtrl := isDown && (vk == vtinput.VK_CONTROL || vk == vtinput.VK_LCONTROL || vk == vtinput.VK_RCONTROL)
	if gogpuCmdIsCtrl {
		// macOS has two Ctrl channels: Command is left Ctrl and physical Ctrl
		// is right Ctrl. Keep their tracking independent when the aggregate
		// platform flags change one channel at a time.
		keepCommand := isDown && (vk == vtinput.VK_LCONTROL || vk == vtinput.VK_CONTROL)
		keepPhysical := isDown && (vk == vtinput.VK_RCONTROL || vk == vtinput.VK_CONTROL)
		if !mods.HasSuper() && !keepCommand {
			h.lCtrl = false
		} else if mods.HasSuper() && assumeActive && !h.lCtrl {
			h.lCtrl = true
		}
		if !mods.HasControl() && !keepPhysical {
			h.rCtrl = false
		} else if mods.HasControl() && assumeActive && !h.rCtrl {
			h.rCtrl = true
		}
	} else if !mods.HasControl() && !keepCtrl {
		h.lCtrl, h.rCtrl = false, false
	} else if mods.HasControl() && assumeActive && !h.lCtrl && !h.rCtrl {
		h.lCtrl = true
	}
}

func (h *GogpuHost) resetKeyboardStateLocked() {
	if h.pendingKeyTimer != nil {
		h.pendingKeyTimer.Stop()
		h.pendingKeyTimer = nil
	}
	h.pendingKeyEvent = nil
	h.lastVK = 0
	h.suppressTextInput = false
	h.currentMods = 0
	h.lCtrl, h.rCtrl = false, false
	h.lAlt, h.rAlt = false, false
	h.lShift, h.rShift = false, false
	h.superDown = false
}

func (h *GogpuHost) handleFocus(focused bool) {
	h.mu.Lock()
	if !focused {
		h.resetKeyboardStateLocked()
	}
	h.mu.Unlock()
	defer func() {
		// App shutdown can close the reader while the backend is dispatching
		// its final focus notification.
		_ = recover()
	}()
	h.sendEvent(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: focused})
}

func RunGogpuHost(cols, rows int, fontName string, fontSize float64, setupApp func()) error {
	// DX12: use naga DXIL backend instead of HLSL->FXC
	// to avoid 2-6s shader compilation via d3dcompiler_47.dll
	if os.Getenv("GOGPU_DX12_DXIL") == "" {
		api := os.Getenv("GOGPU_GRAPHICS_API")
		if api == "" || strings.EqualFold(api, "dx12") || strings.EqualFold(api, "d3d12") || strings.EqualFold(api, "directx") {
			os.Setenv("GOGPU_DX12_DXIL", "1")
		}
	}
	face, fallbackChain, cellW, cellH := loadGogpuFont(fontName, fontSize)

	fmt.Fprintf(os.Stdout, "GOGPU_HOST: Starting RunGogpuHost %dx%d (Cell: %dx%d)\n", cols, rows, cellW, cellH)
	DebugLog("GOGPU_HOST: Starting RunGogpuHost %dx%d (Cell: %dx%d)", cols, rows, cellW, cellH)

	config := gogpu.DefaultConfig().
		WithTitle(AppName).
		WithSize(cols*cellW, rows*cellH)

	fmt.Fprintln(os.Stdout, "GOGPU_HOST: Creating gogpu.App...")
	app := gogpu.NewApp(config)
	fmt.Fprintln(os.Stdout, "GOGPU_HOST: gogpu.App created successfully")

	host := &GogpuHost{
		app:      app,
		cols:     cols,
		rows:     rows,
		cellW:    cellW,
		cellH:    cellH,
		face:     face,
		lastAppW: cols * cellW,
		lastAppH: rows * cellH,
	}

	scr := NewScreenBuf()
	host.scr = scr
	scr.AllocBuf(cols, rows)
	renderer := NewGogpuRenderer(host, face, cellW, cellH)
	renderer.SetFallbackFontChain(fallbackChain)
	scr.Renderer = renderer
	scr.Graphics().SetProtocol(GraphicsNative)
	scr.Graphics().SetCellSize(cellW, cellH)

	FrameManager.Init(scr)

	pr, _ := io.Pipe()
	reader := vtinput.NewReader(pr, true)
	host.reader = reader

	// Not a close request. gogpu's App.OnClose runs inside App.shutdown(),
	// on the render thread, after the main loop has already ended and just
	// before the renderer is destroyed (gogpu app.go:444). Emitting CmQuit
	// from here made the application open its exit confirmation dialog
	// during teardown — including teardown caused by a panic, which made a
	// crash on the draw thread look like a user initiated quit in the logs.
	//
	// Nothing has to be emitted: closing the window clears the last window,
	// gogpu stops the main loop (quitOnLastWindowClosed defaults to true),
	// App.Run returns, and RunGogpuHost returns to its caller, which is the
	// ordinary exit path with its deferred cleanup intact.
	app.OnClose(func() {
		DebugLog("GOGPU_HOST: app is shutting down, renderer teardown follows")
	})
	app.EventSource().OnFocus(host.handleFocus)
	// Files dropped on the window by other applications arrive here; the
	// drag and drop core takes them from the backend to whatever the
	// application registered as its target.
	app.OnDragDrop(host.handleFileDrop)
	SetDragBackend(host)
	logGogpuDragEnvironment()
	// A drag out has to begin on this loop: on Windows and X11 gogpu's
	// drag source is a modal loop of its own, and everywhere the window
	// belongs to this thread.
	app.OnUpdate(func(float64) {
		defer LogAndRepanic("gogpu OnUpdate")
		host.pumpDragOut()
	})

	app.EventSource().OnKeyPress(func(key gpucontext.Key, mods gpucontext.Modifiers) {
		host.mu.Lock()
		currMods := host.syncMods(key, mods, true)

		vk := gogpuKeyToVK(key, currMods)
		if vk != 0 {
			DebugLog("GOGPU_HOST_EVENT: OnKeyPress key=%v, vk=%d", key, vk)
		} else {
			DebugLog("GOGPU_HOST_EVENT: OnKeyPress UNMAPPED key=%v", key)
		}

		// A keystroke already delivered whole must not also type: a keypad
		// key that resolved to navigation would type its digit, a chord that
		// the platform composes would type its symbol ("†" for Option+T).
		// Every other key press reassigns the flag, and text only ever
		// follows its own key press.
		host.suppressTextInput = gogpuKeystrokeSwallowsText(key, vk, currMods)

		if host.pendingKeyEvent != nil {
			if host.pendingKeyTimer != nil {
				host.pendingKeyTimer.Stop()
			}
			if host.pendingKeyEvent.Char == 0 && host.lastRuneForVK != nil {
				host.pendingKeyEvent.Char = host.lastRuneForVK[host.pendingKeyEvent.VirtualKeyCode]
			}
			host.sendEvent(host.pendingKeyEvent)
			host.pendingKeyEvent = nil
		}

		if vk != 0 {
			host.lastVK = vk
			ev := &vtinput.InputEvent{
				Type:            vtinput.KeyEventType,
				KeyDown:         true,
				VirtualKeyCode:  vk,
				ControlKeyState: currMods,
			}
			if isGogpuEnhancedNavKey(key) {
				ev.ControlKeyState |= vtinput.EnhancedKey
			}

			if isSpecialOrModifiedKey(vk, currMods) {
				// An Alt chord carries its character too, so that an
				// accelerator such as Alt+letter knows what it stands for.
				// The X11 and Wayland hosts put the virtual key and the
				// character in one event, and the ebiten host fills this in
				// exactly as here.
				//
				// Only Alt chords, and only where the platform composes
				// their text. Everywhere else that text arrives as an event
				// of its own carrying the right character, and filling this
				// event too would deliver the same keystroke twice. Ctrl and
				// Cmd chords stay Char-less on every platform: Group matches
				// hotkeys on Char without requiring Alt, so a Char here
				// would make Cmd+D press a &Delete button that Windows and
				// Linux never see it press.
				if gogpuAltComposesText && ev.Char == 0 &&
					currMods&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0 {
					ev.Char = host.charForVK(vk)
				}
				host.sendEvent(ev)
			} else {
				if host.lastRuneForVK != nil {
					ev.Char = host.lastRuneForVK[vk]
				}
				// The keypad names its own character, and unlike the letter
				// rows it does not depend on the layout. Seeding it here keeps
				// the digit working where the platform sends no text for the
				// keypad at all; a character that does arrive still wins,
				// which matters for layouts whose decimal key yields a comma.
				if ev.Char == 0 {
					ev.Char = gogpuNumpadRune(vk)
				}
				host.pendingKeyEvent = ev
				host.pendingKeyTimer = time.AfterFunc(10*time.Millisecond, func() {
					host.mu.Lock()
					defer host.mu.Unlock()
					if host.pendingKeyEvent != nil {
						if host.pendingKeyEvent.Char == 0 && host.lastRuneForVK != nil {
							host.pendingKeyEvent.Char = host.lastRuneForVK[host.pendingKeyEvent.VirtualKeyCode]
						}
						host.sendEvent(host.pendingKeyEvent)
						host.pendingKeyEvent = nil
					}
				})
			}
		}
		host.mu.Unlock()
	})

	app.EventSource().OnTextInput(host.handleTextInput)

	app.EventSource().OnKeyRelease(func(key gpucontext.Key, mods gpucontext.Modifiers) {
		host.mu.Lock()
		currMods := host.syncMods(key, mods, false)

		vk := gogpuKeyToVK(key, currMods)
		if vk == 0 {
			DebugLog("GOGPU_HOST_EVENT: OnKeyRelease UNMAPPED key=%v", key)
		}

		if host.pendingKeyEvent != nil {
			if host.pendingKeyTimer != nil {
				host.pendingKeyTimer.Stop()
			}
			if host.pendingKeyEvent.Char == 0 && host.lastRuneForVK != nil {
				host.pendingKeyEvent.Char = host.lastRuneForVK[host.pendingKeyEvent.VirtualKeyCode]
			}
			host.sendEvent(host.pendingKeyEvent)
			host.pendingKeyEvent = nil
		}

		host.mu.Unlock()

		if vk == 0 {
			return
		}
		// The FrameManager derives Ctrl-held from a Ctrl key-up's virtual key
		// alone, ignoring ControlKeyState, so a key-up on one Ctrl channel
		// while the other still holds Ctrl would read as a full release
		// mid-chord — the Switcher commits its selection on exactly that.
		// With the channel split this fires only when Cmd and a physical
		// Ctrl are genuinely held together; gogpu's darwin layer reports a
		// modifier release only once the aggregate flag clears, so keys
		// sharing one channel never produce a mid-hold release. Only where
		// the fold is active: macOS reports the post-change flags on the
		// modifier's own event, while on Wayland the modifiers event trails
		// the key event, so a genuine final Ctrl release still carries the
		// Ctrl bit and would be swallowed here.
		if gogpuCmdIsCtrl && (vk == vtinput.VK_LCONTROL || vk == vtinput.VK_RCONTROL) &&
			currMods&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0 {
			DebugLog("GOGPU_HOST_EVENT: withheld Ctrl key-up, Ctrl still held (key=%v)", key)
			return
		}
		host.sendEvent(&vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         false,
			VirtualKeyCode:  vk,
			ControlKeyState: currMods,
		})
	})

	app.EventSource().OnMousePress(func(button gpucontext.MouseButton, x, y float64) {
		var btn uint32
		switch button {
		case gpucontext.MouseButtonLeft:
			btn = uint32(vtinput.FromLeft1stButtonPressed)
		case gpucontext.MouseButtonRight:
			btn = uint32(vtinput.RightmostButtonPressed)
		case gpucontext.MouseButtonMiddle:
			btn = uint32(vtinput.FromLeft2ndButtonPressed)
		default:
			btn = uint32(vtinput.FromLeft1stButtonPressed)
		}

		host.mu.Lock()
		host.mouseBtn = btn
		cW := host.cellW
		cH := host.cellH
		host.mu.Unlock()

		host.sendEvent(&vtinput.InputEvent{
			Type:        vtinput.MouseEventType,
			MouseX:      int16(x / float64(cW)),
			MouseY:      int16(y / float64(cH)),
			KeyDown:     true,
			ButtonState: btn,
		})
	})

	app.EventSource().OnMouseRelease(func(button gpucontext.MouseButton, x, y float64) {
		host.mu.Lock()
		host.mouseBtn = 0
		cW := host.cellW
		cH := host.cellH
		host.mu.Unlock()

		host.sendEvent(&vtinput.InputEvent{
			Type:        vtinput.MouseEventType,
			MouseX:      int16(x / float64(cW)),
			MouseY:      int16(y / float64(cH)),
			KeyDown:     false,
			ButtonState: 0,
		})
	})

	app.EventSource().OnMouseMove(func(x, y float64) {
		host.mu.Lock()
		btn := host.mouseBtn
		cW := host.cellW
		cH := host.cellH
		host.mu.Unlock()

		if btn != 0 {
			host.sendEvent(&vtinput.InputEvent{
				Type:            vtinput.MouseEventType,
				MouseX:          int16(x / float64(cW)),
				MouseY:          int16(y / float64(cH)),
				MouseEventFlags: vtinput.MouseMoved,
				ButtonState:     btn,
				ControlKeyState: host.currentMods,
			})
		}
	})

	app.EventSource().OnScroll(func(dx float64, dy float64) {
		host.mu.Lock()
		cW := host.cellW
		cH := host.cellH
		host.mu.Unlock()

		mx, my := app.Input().Mouse().Position()

		// One wheel notch produces one event; consumers decide how many
		// lines a notch scrolls (see WheelLinesPerNotch).
		steps := int(math.Abs(dy))
		if steps == 0 {
			return
		}
		dir := -1
		if dy < 0 {
			dir = 1
		}
		for i := 0; i < steps; i++ {
			host.sendEvent(&vtinput.InputEvent{
				Type:           vtinput.MouseEventType,
				MouseX:         int16(float64(mx) / float64(cW)),
				MouseY:         int16(float64(my) / float64(cH)),
				WheelDirection: dir,
			})
		}
		// Request a redraw to ensure the UI updates instantly in event-driven mode
		app.RequestRedraw()
	})

	var infoLogged sync.Once
	app.OnDraw(func(dc *gogpu.Context) {
		defer LogAndRepanic("gogpu OnDraw")

		w, h := dc.Width(), dc.Height()

		host.mu.Lock()
		sizeChanged := (host.lastAppW != w || host.lastAppH != h)
		host.lastAppW, host.lastAppH = w, h
		if sizeChanged {
			host.resizePending = true
		}
		host.mu.Unlock()

		if sizeChanged && host.reader != nil && host.reader.EventChan != nil {
			host.sendEvent(&vtinput.InputEvent{Type: vtinput.ResizeEventType})
		}

		infoLogged.Do(func() {
			if provider := app.GPUContextProvider(); provider != nil {
				info := provider.AdapterInfo()
				fmt.Fprintf(os.Stdout, "GOGPU_HOST_ON_DRAW: Adapter confirmed: %q, Type: %v\n", info.Name, info.Type)
				DebugLog("GOGPU_HOST_ON_DRAW: Adapter confirmed: %q, Type: %v", info.Name, info.Type)
			}
		})

		if gogpuRenderer, ok := host.scr.Renderer.(*GogpuRenderer); ok {
			gogpuRenderer.DrawToScreen(dc)
		}
	})

	GetTerminalSize = func() (int, int, error) {
		host.mu.Lock()
		defer host.mu.Unlock()

		w, h := host.lastAppW, host.lastAppH

		if host.cellW > 0 && host.cellH > 0 && w > 0 && h > 0 {
			c := w / host.cellW
			r := h / host.cellH
			if c != host.cols || r != host.rows {
				host.cols = c
				host.rows = r
			}
		}
		return host.cols, host.rows, nil
	}

	setupApp()
	// After setupApp: the application installs the debug log sink during
	// setup, so a backend announced before it is logged nowhere.
	SetActiveBackend("gogpu")
	// One wheel notch arrives as a single event now; keep widgets scrolling
	// the system-configured number of lines per notch as before.
	setWheelNotchLines(getSystemScrollLines())

	go func() {
		w, h := app.Size()
		fw, fh := app.PhysicalSize()
		fmt.Fprintf(os.Stdout, "GOGPU_HOST: Before Run(). App Size (Log): %dx%d. App PhysicalSize: %dx%d. ScaleFactor: %f\n", w, h, fw, fh, app.ScaleFactor())
		DebugLog("GOGPU_HOST: Before Run(). App Size (Log): %dx%d. App PhysicalSize: %dx%d. ScaleFactor: %f", w, h, fw, fh, app.ScaleFactor())

		provider := app.GPUContextProvider()
		if provider != nil {
			info := provider.AdapterInfo()
			fmt.Fprintf(os.Stdout, "GOGPU_HOST: Adapter: Name=%q, Type=%v\n", info.Name, info.Type)
			DebugLog("GOGPU_HOST: Adapter: Name=%q, Type=%v", info.Name, info.Type)
		}

		fmt.Fprintln(os.Stdout, "GOGPU_HOST: FrameManager starting...")
		DebugLog("GOGPU_HOST: FrameManager starting...")
		FrameManager.Run(reader)
		fmt.Fprintln(os.Stdout, "GOGPU_HOST: FrameManager exited. Forcing app shutdown to prevent blue screen hang.")
		DebugLog("GOGPU_HOST: FrameManager exited. Forcing app shutdown to prevent blue screen hang.")
		os.Exit(0)
	}()

	fmt.Fprintln(os.Stdout, "GOGPU_HOST: Calling app.Run()...")
	err := app.Run()
	fmt.Fprintf(os.Stdout, "GOGPU_HOST: app.Run() exited with: %v\n", err)
	return err
}

// isGogpuFaceSafe does a minimal feature probe on a gg/text.Face.
// The GPU GlyphMask path later calls into FontSource.Parsed/copyCheck.
// A face that survives Metrics() can still have a nil internal
// FontSource and panic on the first DrawString. We catch that class
// of failure early and fall back to the primary face.
func isGogpuFaceSafe(f text.Face) (ok bool) {
	if f == nil {
		return false
	}
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	_ = f.Metrics()
	// Touch a couple of common code points that the UI actually draws.
	_ = f.Advance("A")
	_ = f.Advance("字")
	ok = true
	return
}

// loadGogpuFont returns the primary face plus a lazily loaded chain of the
// fallback fonts that cover the runes the primary lacks. The fallbacks are
// kept as separate faces rather than folded into a gg text.MultiFace on
// purpose: a MultiFace has no single FontSource, and the GPU glyph-mask
// engine refuses such a face and sends every DrawString down the CPU path.
// Selecting the face per rune keeps each face a plain single-source one, so
// both the primary and the fallbacks stay on the GPU path.
func loadGogpuFont(fontName string, size float64) (text.Face, *fontFallbackChain, int, int) {
	if size <= 0 {
		size = 18.0
	}
	var primaryFace text.Face
	var cellW, cellH int

	for _, p := range getFontCandidates(fontName) {
		if _, err := os.Stat(p); err == nil {
			src, err := text.NewFontSourceFromFile(p)
			if err == nil {
				face := src.Face(size)
				metrics := face.Metrics()
				adv := face.Advance("A")
				cellH = int(metrics.Ascent + metrics.Descent + 0.5)
				cellW = int(adv + 0.5)
				if cellW == 0 {
					cellW = 8
				}
				if cellH == 0 {
					cellH = 16
				}
				fmt.Fprintf(os.Stdout, "GOGPU_DIAG_FONT: Loaded File=%s RequestSize=%.1f, Cell: %dx%d\n", p, size, cellW, cellH)
				DebugLog("GOGPU_DIAG_FONT: File=%s RequestSize=%.1f", p, size)
				DebugLog("GOGPU_DIAG_FONT: Metrics: Ascent=%.2f Descent=%.2f LineGap=%.2f AdvanceA=%.2f",
					float64(metrics.Ascent), float64(metrics.Descent), float64(metrics.LineGap), adv)
				DebugLog("GOGPU_DIAG_FONT: Calculated Cell: %dx%d", cellW, cellH)
				primaryFace = face
				break
			}
		}
	}

	if primaryFace == nil {
		return nil, nil, 8, 16
	}

	// An escape hatch that does not need a rebuild: if the chain ever draws
	// something worse than a .notdef box, the old behaviour is one variable
	// away.
	noFallback := os.Getenv("VTUI_GOGPU_NO_FALLBACK") != ""

	// The fallbacks cannot be attached to the face yet — see the note below on
	// MultiFace — but which of them exist on this machine is worth recording.
	// "No CJK font installed" and "CJK font installed and never consulted" are
	// different bugs with identical symptoms, and the log is the only place
	// they can be told apart.
	//
	// Existence is all the startup probe checks now. The old probe also parsed
	// every file, which cost ~800 MB of heap on a stock macOS (the fallback
	// list is ~400 MB of font files and gg holds each twice) in sessions that
	// never drew a single glyph from them. Whether gg can actually open a file
	// is still logged — by the chain's warm() sweep, shortly after startup.
	var chain *fontFallbackChain
	if noFallback {
		DebugLog("GOGPU_DIAG_FONT: fallback chain disabled by VTUI_GOGPU_NO_FALLBACK")
	} else {
		for _, p := range fallbackPathsForGUI() {
			if _, err := os.Stat(p); err != nil {
				continue
			}
			DebugLog("GOGPU_DIAG_FONT: fallback present, deferred until first use: %s", p)
			if chain == nil {
				chain = newGogpuFallbackChain(size)
			}
			chain.entries = append(chain.entries, fontFallbackEntry{path: p})
		}
		if chain != nil {
			chain.warm()
		}
	}

	// text.MultiFace stays out of this backend deliberately. It reports no
	// FontSource, gg's glyph-mask engine rejects such a face (nil guard added
	// upstream in gg 5aa3005, v0.50.15) and falls back to the CPU text path
	// for every DrawString — with one DrawString per cell that is not a
	// slowdown but a hang. The renderer walks the returned fallbacks itself
	// instead; see GogpuRenderer.faceFor.
	//
	// This can be revisited when gg lands per-font-run GPU support (ADR-065),
	// and only if it measures better than the chain below.
	return primaryFace, chain, cellW, cellH
}

func isNumLockEffectiveGogpu(mods vtinput.ControlKeyState) bool {
	numLock := (mods & vtinput.NumLockOn) != 0
	shift := (mods & vtinput.ShiftPressed) != 0
	return numLock != shift
}

// isGogpuKeypadKey reports whether a key belongs to the numeric keypad.
//
// Only the keys whose meaning NumLock changes are listed. The operators and
// Enter are the same key either way, so text arriving for them is genuine and
// must not be dropped.
func isGogpuKeypadKey(k gpucontext.Key) bool {
	switch k {
	case gpucontext.KeyNumpad0, gpucontext.KeyNumpad1, gpucontext.KeyNumpad2,
		gpucontext.KeyNumpad3, gpucontext.KeyNumpad4, gpucontext.KeyNumpad5,
		gpucontext.KeyNumpad6, gpucontext.KeyNumpad7, gpucontext.KeyNumpad8,
		gpucontext.KeyNumpad9, gpucontext.KeyNumpadDecimal:
		return true
	}
	return false
}

// isGogpuEnhancedNavKey is isEbitenEnhancedNavKey for the gogpu key set,
// except the four arrows stay plain: the picture viewer pans with them, and
// flagging them would send them down the directory walk instead.
func isGogpuEnhancedNavKey(k gpucontext.Key) bool {
	switch k {
	case gpucontext.KeyHome, gpucontext.KeyEnd, gpucontext.KeyPageUp, gpucontext.KeyPageDown,
		gpucontext.KeyInsert, gpucontext.KeyDelete:
		return true
	}
	return false
}

// gogpuNumpadRune returns the character a keypad virtual key stands for, or
// zero for the navigation codes the same keys produce with NumLock off.
//
// The caller uses zero as the test for "this keystroke types nothing", so the
// two jobs are one function: it both fills in the character and names the keys
// whose text has to be thrown away.
func gogpuNumpadRune(vk uint16) rune {
	if vk >= vtinput.VK_NUMPAD0 && vk <= vtinput.VK_NUMPAD9 {
		return rune('0' + (vk - vtinput.VK_NUMPAD0))
	}
	switch vk {
	case vtinput.VK_DECIMAL:
		return '.'
	case vtinput.VK_ADD:
		return '+'
	case vtinput.VK_SUBTRACT:
		return '-'
	case vtinput.VK_MULTIPLY:
		return '*'
	case vtinput.VK_DIVIDE:
		return '/'
	case vtinput.VK_SEPARATOR:
		return ','
	}
	return 0
}

func gogpuKeyToVK(k gpucontext.Key, mods vtinput.ControlKeyState) uint16 {
	switch k {
	case gpucontext.KeyEscape:
		return vtinput.VK_ESCAPE
	case gpucontext.KeyF1:
		return vtinput.VK_F1
	case gpucontext.KeyF2:
		return vtinput.VK_F2
	case gpucontext.KeyF3:
		return vtinput.VK_F3
	case gpucontext.KeyF4:
		return vtinput.VK_F4
	case gpucontext.KeyF5:
		return vtinput.VK_F5
	case gpucontext.KeyF6:
		return vtinput.VK_F6
	case gpucontext.KeyF7:
		return vtinput.VK_F7
	case gpucontext.KeyF8:
		return vtinput.VK_F8
	case gpucontext.KeyF9:
		return vtinput.VK_F9
	case gpucontext.KeyF10:
		return vtinput.VK_F10
	case gpucontext.KeyF11:
		return vtinput.VK_F11
	case gpucontext.KeyF12:
		return vtinput.VK_F12
	case gpucontext.KeyInsert:
		return vtinput.VK_INSERT
	case gpucontext.KeyDelete:
		return vtinput.VK_DELETE
	case gpucontext.KeyHome:
		return vtinput.VK_HOME
	case gpucontext.KeyEnd:
		return vtinput.VK_END
	case gpucontext.KeyPageUp:
		return vtinput.VK_PRIOR
	case gpucontext.KeyPageDown:
		return vtinput.VK_NEXT
	case gpucontext.KeyUp:
		return vtinput.VK_UP
	case gpucontext.KeyDown:
		return vtinput.VK_DOWN
	case gpucontext.KeyLeft:
		return vtinput.VK_LEFT
	case gpucontext.KeyRight:
		return vtinput.VK_RIGHT
	case gpucontext.KeyBackspace:
		return vtinput.VK_BACK
	case gpucontext.KeyEnter:
		return vtinput.VK_RETURN
	case gpucontext.KeyTab:
		return vtinput.VK_TAB
	case gpucontext.KeySpace:
		return vtinput.VK_SPACE

	// Numeric keypad.
	//
	// gogpu reports the physical key, so the same Key arrives whatever NumLock
	// says; the split into digits and navigation has to happen here. Shift
	// inverts the lock, which is what isNumLockEffectiveGogpu computes. On
	// Windows the platform layer has already resolved the lock and simply never
	// sends KeyNumpadN in navigation mode, so the same rule holds there too.
	case gpucontext.KeyNumpad0:
		if isNumLockEffectiveGogpu(mods) {
			return vtinput.VK_NUMPAD0
		}
		return vtinput.VK_INSERT
	case gpucontext.KeyNumpad1:
		if isNumLockEffectiveGogpu(mods) {
			return vtinput.VK_NUMPAD1
		}
		return vtinput.VK_END
	case gpucontext.KeyNumpad2:
		if isNumLockEffectiveGogpu(mods) {
			return vtinput.VK_NUMPAD2
		}
		return vtinput.VK_DOWN
	case gpucontext.KeyNumpad3:
		if isNumLockEffectiveGogpu(mods) {
			return vtinput.VK_NUMPAD3
		}
		return vtinput.VK_NEXT
	case gpucontext.KeyNumpad4:
		if isNumLockEffectiveGogpu(mods) {
			return vtinput.VK_NUMPAD4
		}
		return vtinput.VK_LEFT
	case gpucontext.KeyNumpad5:
		if isNumLockEffectiveGogpu(mods) {
			return vtinput.VK_NUMPAD5
		}
		return vtinput.VK_CLEAR
	case gpucontext.KeyNumpad6:
		if isNumLockEffectiveGogpu(mods) {
			return vtinput.VK_NUMPAD6
		}
		return vtinput.VK_RIGHT
	case gpucontext.KeyNumpad7:
		if isNumLockEffectiveGogpu(mods) {
			return vtinput.VK_NUMPAD7
		}
		return vtinput.VK_HOME
	case gpucontext.KeyNumpad8:
		if isNumLockEffectiveGogpu(mods) {
			return vtinput.VK_NUMPAD8
		}
		return vtinput.VK_UP
	case gpucontext.KeyNumpad9:
		if isNumLockEffectiveGogpu(mods) {
			return vtinput.VK_NUMPAD9
		}
		return vtinput.VK_PRIOR
	case gpucontext.KeyNumpadDecimal:
		if isNumLockEffectiveGogpu(mods) {
			return vtinput.VK_DECIMAL
		}
		return vtinput.VK_DELETE
	case gpucontext.KeyNumpadAdd:
		return vtinput.VK_ADD
	case gpucontext.KeyNumpadSubtract:
		return vtinput.VK_SUBTRACT
	case gpucontext.KeyNumpadMultiply:
		return vtinput.VK_MULTIPLY
	case gpucontext.KeyNumpadDivide:
		return vtinput.VK_DIVIDE
	case gpucontext.KeyNumpadComma:
		return vtinput.VK_SEPARATOR
	case gpucontext.KeyNumpadEqual:
		return vtinput.VK_OEM_PLUS
	case gpucontext.KeyNumpadEnter:
		return vtinput.VK_RETURN

	// Lock keys. The keypad is unusable without NumLock reaching the
	// application, and the other two travel with it on every other backend.
	case gpucontext.KeyNumLock:
		return vtinput.VK_NUMLOCK
	case gpucontext.KeyCapsLock:
		return vtinput.VK_CAPITAL
	case gpucontext.KeyScrollLock:
		return vtinput.VK_SCROLL
	case gpucontext.KeyPause:
		return vtinput.VK_PAUSE
	case gpucontext.KeyCancel:
		// Win32 reports Ctrl+Break as VK_CANCEL. Keep it distinct from
		// the plain Pause key so f4 can translate the chord to ETX.
		return vtinput.VK_CANCEL

	// Where Super acts as Ctrl (macOS Command), the modifiers arrive on two
	// channels, the way far2l separates them: both Cmd keys as VK_LCONTROL
	// carrying LeftCtrlPressed, both physical Ctrl keys as VK_RCONTROL
	// carrying RightCtrlPressed. The keys never share a virtual key, so a
	// release in one channel cannot be mistaken for a release in the other,
	// and nothing in the framework assigns meaning to Ctrl sides (see the
	// KeyBar doc comment).
	case gpucontext.KeyLeftControl:
		if gogpuCmdIsCtrl {
			return vtinput.VK_RCONTROL
		}
		return vtinput.VK_LCONTROL
	case gpucontext.KeyRightControl:
		return vtinput.VK_RCONTROL
	case gpucontext.KeyLeftShift:
		return vtinput.VK_LSHIFT
	case gpucontext.KeyRightShift:
		return vtinput.VK_RSHIFT
	case gpucontext.KeyLeftAlt:
		return vtinput.VK_LMENU
	case gpucontext.KeyRightAlt:
		return vtinput.VK_RMENU
	case gpucontext.KeyLeftSuper:
		if gogpuCmdIsCtrl {
			return vtinput.VK_LCONTROL
		}
		return vtinput.VK_LWIN
	case gpucontext.KeyRightSuper:
		if gogpuCmdIsCtrl {
			return vtinput.VK_LCONTROL
		}
		return vtinput.VK_RWIN
	case gpucontext.KeyA:
		return vtinput.VK_A
	case gpucontext.KeyB:
		return vtinput.VK_B
	case gpucontext.KeyC:
		return vtinput.VK_C
	case gpucontext.KeyD:
		return vtinput.VK_D
	case gpucontext.KeyE:
		return vtinput.VK_E
	case gpucontext.KeyF:
		return vtinput.VK_F
	case gpucontext.KeyG:
		return vtinput.VK_G
	case gpucontext.KeyH:
		return vtinput.VK_H
	case gpucontext.KeyI:
		return vtinput.VK_I
	case gpucontext.KeyJ:
		return vtinput.VK_J
	case gpucontext.KeyK:
		return vtinput.VK_K
	case gpucontext.KeyL:
		return vtinput.VK_L
	case gpucontext.KeyM:
		return vtinput.VK_M
	case gpucontext.KeyN:
		return vtinput.VK_N
	case gpucontext.KeyO:
		return vtinput.VK_O
	case gpucontext.KeyP:
		return vtinput.VK_P
	case gpucontext.KeyQ:
		return vtinput.VK_Q
	case gpucontext.KeyR:
		return vtinput.VK_R
	case gpucontext.KeyS:
		return vtinput.VK_S
	case gpucontext.KeyT:
		return vtinput.VK_T
	case gpucontext.KeyU:
		return vtinput.VK_U
	case gpucontext.KeyV:
		return vtinput.VK_V
	case gpucontext.KeyW:
		return vtinput.VK_W
	case gpucontext.KeyX:
		return vtinput.VK_X
	case gpucontext.KeyY:
		return vtinput.VK_Y
	case gpucontext.KeyZ:
		return vtinput.VK_Z
	case gpucontext.Key0:
		return vtinput.VK_0
	case gpucontext.Key1:
		return vtinput.VK_1
	case gpucontext.Key2:
		return vtinput.VK_2
	case gpucontext.Key3:
		return vtinput.VK_3
	case gpucontext.Key4:
		return vtinput.VK_4
	case gpucontext.Key5:
		return vtinput.VK_5
	case gpucontext.Key6:
		return vtinput.VK_6
	case gpucontext.Key7:
		return vtinput.VK_7
	case gpucontext.Key8:
		return vtinput.VK_8
	case gpucontext.Key9:
		return vtinput.VK_9
	case gpucontext.KeyGrave:
		return vtinput.VK_OEM_3
	case gpucontext.KeyMinus:
		return vtinput.VK_OEM_MINUS
	case gpucontext.KeyEqual:
		return vtinput.VK_OEM_PLUS
	case gpucontext.KeyLeftBracket:
		return vtinput.VK_OEM_4
	case gpucontext.KeyRightBracket:
		return vtinput.VK_OEM_6
	case gpucontext.KeyBackslash:
		return vtinput.VK_OEM_5
	case gpucontext.KeySemicolon:
		return vtinput.VK_OEM_1
	case gpucontext.KeyApostrophe:
		return vtinput.VK_OEM_7
	case gpucontext.KeyComma:
		return vtinput.VK_OEM_COMMA
	case gpucontext.KeyPeriod:
		return vtinput.VK_OEM_PERIOD
	case gpucontext.KeySlash:
		return vtinput.VK_OEM_2
	}
	return 0
}
