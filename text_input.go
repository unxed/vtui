package vtui

import "github.com/unxed/vtinput"

// TextInputView returns an insertion-only view of committed text. The original
// event retains its physical key and modifiers for recording and forwarding.
// Ordinary keys are returned unchanged, including legacy character shortcuts.
func TextInputView(event *vtinput.InputEvent) *vtinput.InputEvent {
	if event == nil || !event.TextInput {
		return event
	}
	view := *event
	view.VirtualKeyCode = 0
	view.VirtualScanCode = 0
	view.UnshiftedChar = view.Char
	view.ControlKeyState &^= vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed |
		vtinput.LeftAltPressed | vtinput.RightAltPressed | vtinput.ShiftPressed | vtinput.EnhancedKey
	return &view
}

func textChord(mods vtinput.ControlKeyState) bool {
	return mods&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0 &&
		mods&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0
}

func committedTextModifiers(mods vtinput.ControlKeyState) bool {
	return textChord(mods) || mods&vtinput.RightAltPressed != 0
}

func processTextRepeat(event *vtinput.InputEvent, process func(*vtinput.InputEvent) bool) bool {
	if !event.TextInput || !event.KeyDown || event.Type != vtinput.KeyEventType || event.RepeatCount <= 1 {
		return false
	}
	copy := *event
	copy.RepeatCount = 1
	for range event.RepeatCount {
		process(&copy)
	}
	return true
}

// guiTextInput pairs a native key with its committed text. A dead key never
// falls back to a shortcut: only the platform's eventual commit emits text.
type guiTextInput struct {
	pending   []guiTextPress
	composing bool
	high      uint16
	highEvent vtinput.InputEvent
}

type guiTextPress struct {
	event vtinput.InputEvent
	units int
}

func (state *guiTextInput) press(event *vtinput.InputEvent, translation int) bool {
	if translation == 0 {
		return false
	}
	if translation < 0 {
		state.composing = true
		return true
	}
	copy := *event
	copy.Char = 0
	copy.TextInput = textChord(copy.ControlKeyState) || state.composing
	state.pending = append(state.pending, guiTextPress{event: copy, units: translation})
	state.composing = false
	return true
}

func (state *guiTextInput) commit(char rune, fallback vtinput.InputEvent) *vtinput.InputEvent {
	if char < ' ' || char == 0x7f {
		return nil
	}
	units := 1
	if char > 0xffff {
		units = 2
	}
	event := state.source(fallback, units)
	event.Char = char
	return &event
}

func (state *guiTextInput) source(fallback vtinput.InputEvent, units int) vtinput.InputEvent {
	event := fallback
	// TranslateMessage posts characters to the queue. More keydowns, keyups,
	// or layout changes may precede those character messages, so pair each
	// commit with the oldest matching scan code, not the most recent key.
	for i := range state.pending {
		pending := &state.pending[i]
		if pending.event.VirtualScanCode != fallback.VirtualScanCode {
			continue
		}
		event = pending.event
		pending.units -= units
		if pending.units <= 0 {
			state.pending = append(state.pending[:i], state.pending[i+1:]...)
		}
		break
	}
	if fallback.RepeatCount != 0 {
		event.RepeatCount = fallback.RepeatCount
	}
	return event
}

func (state *guiTextInput) layoutChanged() {
	state.composing = false
	state.high = 0
}

func (state *guiTextInput) utf16(unit uint16, fallback vtinput.InputEvent, systemChar bool) *vtinput.InputEvent {
	// Control translations do not get printable slots in press, so they must
	// not consume a later printable press with the same scan code.
	if unit < ' ' || unit == 0x7f {
		return nil
	}
	source := state.source(fallback, 1)
	if unit >= 0xd800 && unit <= 0xdbff {
		state.high = unit
		state.highEvent = source
		return nil
	}
	char := rune(unit)
	if unit >= 0xdc00 && unit <= 0xdfff {
		if state.high == 0 || !sameGUITextSource(state.highEvent, source) {
			state.high = 0
			return nil
		}
		char = (rune(state.high)-0xd800)<<10 + rune(unit) - 0xdc00 + 0x10000
		source = state.highEvent
	}
	state.high = 0
	source.Char = char
	if !systemChar && committedTextModifiers(source.ControlKeyState) {
		source.TextInput = true
	}
	return &source
}

func sameGUITextSource(a, b vtinput.InputEvent) bool {
	return a.VirtualKeyCode == b.VirtualKeyCode && a.VirtualScanCode == b.VirtualScanCode &&
		a.ControlKeyState == b.ControlKeyState && a.RepeatCount == b.RepeatCount &&
		a.TextInput == b.TextInput && a.InputSource == b.InputSource
}
