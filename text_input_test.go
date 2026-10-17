package vtui

import (
	"reflect"
	"testing"

	"github.com/unxed/vtinput"
)

func confirmedText(vk uint16, char rune) *vtinput.InputEvent {
	return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, TextInput: true,
		VirtualKeyCode: vk, VirtualScanCode: 18, Char: char, UnshiftedChar: 'e', RepeatCount: 1,
		ControlKeyState: vtinput.LeftCtrlPressed | vtinput.RightAltPressed | vtinput.ShiftPressed | vtinput.CapsLockOn | vtinput.EnhancedKey,
		InputSource:     "test-native"}
}

func TestTextInputViewPreservesOriginal(t *testing.T) {
	if TextInputView(nil) != nil {
		t.Fatal("nil event acquired a text view")
	}
	event := confirmedText(vtinput.VK_E, '€')
	original := *event
	view := TextInputView(event)
	if view == event || !reflect.DeepEqual(*event, original) {
		t.Fatal("text view changed original physical event")
	}
	if view.VirtualKeyCode != 0 || view.VirtualScanCode != 0 || view.UnshiftedChar != '€' || view.ControlKeyState != vtinput.CapsLockOn || !view.TextInput || view.InputSource != event.InputSource {
		t.Fatalf("incorrect insertion view: %+v", view)
	}
	event.TextInput = false
	if TextInputView(event) != event {
		t.Fatal("ordinary shortcut was rewritten")
	}
}

func TestGUITextInputRuneCommitConsumesUTF16Units(t *testing.T) {
	var state guiTextInput
	key := confirmedText(vtinput.VK_E, 0)
	state.press(key, 2)
	plain := *key
	plain.VirtualKeyCode, plain.ControlKeyState, plain.TextInput = vtinput.VK_A, 0, false
	state.press(&plain, 1)
	fallback := vtinput.InputEvent{VirtualScanCode: key.VirtualScanCode}
	for _, char := range []rune{0, '\n', 0x7f} {
		if event := state.commit(char, fallback); event != nil {
			t.Fatalf("control character emitted: %+v", event)
		}
	}
	if event := state.commit('😀', fallback); event.Char != '😀' || !event.TextInput || event.VirtualKeyCode != vtinput.VK_E {
		t.Fatalf("supplementary commit lost source: %+v", event)
	}
	if event := state.commit('a', fallback); event.TextInput || event.VirtualKeyCode != vtinput.VK_A {
		t.Fatalf("supplementary rune did not consume both UTF-16 units: %+v", event)
	}
	if len(state.pending) != 0 {
		t.Fatal("consumed presses retained")
	}
	// An IME can commit without any physical press in the queue.
	fallback = *confirmedText(0, 0)
	fallback.RepeatCount = 2
	if event := state.commit('界', fallback); event.Char != '界' || event.VirtualKeyCode != 0 ||
		event.RepeatCount != 2 || event.InputSource != fallback.InputSource || !event.TextInput {
		t.Fatalf("keyless fallback lost metadata: %+v", event)
	}
}

func TestGUITextInputCommitMatchesScanWithoutConsumingOtherKeys(t *testing.T) {
	var state guiTextInput
	first := confirmedText(vtinput.VK_Q, 0)
	first.VirtualScanCode = 16
	second := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_A, VirtualScanCode: 30, RepeatCount: 1}
	state.press(first, 1)
	state.press(second, 1)
	// A different scan may commit first, or an IME may have no matching key.
	if event := state.commit('界', vtinput.InputEvent{}); event.VirtualKeyCode != 0 || event.Char != '界' {
		t.Fatalf("keyless commit stole a queued key: %+v", event)
	}
	if event := state.commit('a', vtinput.InputEvent{VirtualScanCode: 30}); event.VirtualKeyCode != vtinput.VK_A || event.TextInput {
		t.Fatalf("commit paired with wrong scan: %+v", event)
	}
	if event := state.commit('@', vtinput.InputEvent{VirtualScanCode: 16}); event.VirtualKeyCode != vtinput.VK_Q || !event.TextInput {
		t.Fatalf("earlier source was consumed by a different key: %+v", event)
	}
	if len(state.pending) != 0 {
		t.Fatal("consumed presses retained")
	}
}

func TestConfirmedTextCannotToggleControls(t *testing.T) {
	checkbox := NewCheckbox(0, 0, "&Option", false)
	radio := NewRadioButton(0, 0, "&Option", false)
	group := NewRadioGroup(0, 0, 1, []string{"First", "Second"})
	group.focusIdx = 1
	for _, tc := range []struct {
		name     string
		process  func(*vtinput.InputEvent) bool
		selected func() bool
	}{
		{"checkbox", checkbox.ProcessKey, func() bool { return checkbox.State != 0 }},
		{"radio button", radio.ProcessKey, func() bool { return radio.Selected }},
		{"radio group", group.ProcessKey, func() bool { return group.Selected != 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := confirmedText(vtinput.VK_SPACE, ' ')
			if tc.process(event) || tc.selected() {
				t.Fatal("committed space selected a control")
			}
			event.TextInput, event.ControlKeyState = false, 0
			if !tc.process(event) || !tc.selected() {
				t.Fatal("ordinary Space no longer selects the control")
			}
		})
	}
}

func TestConfirmedTextCannotZoomWindow(t *testing.T) {
	window := NewBaseWindow(2, 2, 30, 10, "Text")
	window.ShowZoom = true
	event := confirmedText(vtinput.VK_F5, '€')
	if window.ProcessKey(event) || window.SavedBounds != nil {
		t.Fatal("committed text activated F5 zoom")
	}
	event.TextInput, event.ControlKeyState = false, 0
	if !window.ProcessKey(event) || window.SavedBounds == nil {
		t.Fatal("ordinary F5 no longer zooms the window")
	}
}

func TestConfirmedTextSkipsListShortcutAndSelection(t *testing.T) {
	list := NewListBox(0, 0, 20, 5, []string{"First", "Second"})
	list.MultiSelect = true
	called := 0
	list.OnKeyDown = func(*vtinput.InputEvent) bool { called++; return false }
	event := confirmedText(vtinput.VK_INSERT, '€')
	list.ProcessKey(event)
	if called != 0 || len(list.SelectedMap) != 0 || list.SelectPos != 0 {
		t.Fatal("committed text triggered a list command")
	}
	event.TextInput, event.ControlKeyState = false, 0
	list.ProcessKey(event)
	if called != 1 || !list.SelectedMap[0] || list.SelectPos != 1 {
		t.Fatal("ordinary Insert no longer selects and advances")
	}
}

func TestConfirmedTextRepeatEditsMenuFilterOnly(t *testing.T) {
	menu := newFilterMenu("€€€uro", "Other")
	menu.FilterOnType = true
	menu.OnKeyDown = func(*vtinput.InputEvent) bool { t.Fatal("text reached shortcut callback"); return false }
	event := confirmedText(vtinput.VK_RETURN, '€')
	event.RepeatCount = 3
	original := *event
	if !menu.ProcessKey(event) || menu.FilterText() != "€€€" || menu.IsDone() {
		t.Fatalf("repeated text did not stay in filter: %q, done=%v", menu.FilterText(), menu.IsDone())
	}
	if !reflect.DeepEqual(*event, original) {
		t.Fatal("menu mutated the source event")
	}
}

func TestGUITextInputPreservesPhysicalMetadataAndSurrogates(t *testing.T) {
	var state guiTextInput
	key := confirmedText(vtinput.VK_E, 0)
	key.TextInput = false
	key.RepeatCount = 3
	if !state.press(key, 2) {
		t.Fatal("layout text was not reserved")
	}
	if event := state.utf16(0xd83d, vtinput.InputEvent{RepeatCount: 3, VirtualScanCode: key.VirtualScanCode}, false); event != nil {
		t.Fatal("high surrogate emitted separately")
	}
	event := state.utf16(0xde00, vtinput.InputEvent{RepeatCount: 3, VirtualScanCode: key.VirtualScanCode}, false)
	if event == nil || event.Char != '😀' || !event.TextInput || event.VirtualKeyCode != key.VirtualKeyCode || event.VirtualScanCode != key.VirtualScanCode || event.RepeatCount != 3 || event.ControlKeyState != key.ControlKeyState || event.InputSource != key.InputSource {
		t.Fatalf("UTF-16 commit lost data: %+v", event)
	}
	edit := NewEdit(0, 0, 20, "")
	edit.ProcessKey(event)
	if edit.GetText() != "😀😀😀" {
		t.Fatalf("repeat insertion: %q", edit.GetText())
	}
	if event.RepeatCount != 3 {
		t.Fatal("repeat count mutated")
	}
}

func TestGUITextInputDeadKeyAndPlainTyping(t *testing.T) {
	var state guiTextInput
	key := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_OEM_6}
	state.press(key, -1)
	key.VirtualKeyCode = vtinput.VK_SPACE
	state.press(key, 1)
	event := state.commit('~', vtinput.InputEvent{})
	if !event.TextInput || event.Char != '~' {
		t.Fatalf("composition: %+v", event)
	}
	key.VirtualKeyCode = vtinput.VK_A
	state.press(key, 1)
	if event = state.commit('a', vtinput.InputEvent{}); event.TextInput {
		t.Fatal("ordinary character shortcuts disabled after composition")
	}
}

func TestGUITextInputBatchedPressesKeepSourceIdentity(t *testing.T) {
	var state guiTextInput
	altGr := confirmedText(vtinput.VK_Q, 0)
	altGr.VirtualScanCode = 16
	plain := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_A, VirtualScanCode: 30, RepeatCount: 1, InputSource: "win32-gui"}
	state.press(altGr, 1)
	state.press(plain, 1)
	// A non-text shortcut between keydowns/commits must not overwrite either.
	if state.press(&vtinput.InputEvent{VirtualKeyCode: vtinput.VK_F1}, 0) {
		t.Fatal("shortcut reserved as text")
	}
	event := state.commit('@', vtinput.InputEvent{VirtualScanCode: 16})
	if !event.TextInput || event.VirtualKeyCode != vtinput.VK_Q || event.ControlKeyState != altGr.ControlKeyState {
		t.Fatalf("AltGr associated with later key: %+v", event)
	}
	event = state.commit('a', vtinput.InputEvent{VirtualScanCode: 30})
	if event.TextInput || event.VirtualKeyCode != vtinput.VK_A || event.ControlKeyState != 0 {
		t.Fatalf("plain typing inherited AltGr: %+v", event)
	}
	if len(state.pending) != 0 {
		t.Fatal("consumed presses retained")
	}
}

func TestGUITextInputRepeatedScanAndMultiCharacterCommit(t *testing.T) {
	var state guiTextInput
	key := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_A, VirtualScanCode: 30}
	state.press(key, -1)
	state.press(key, 2) // Failed composition commits accent and base character.
	state.press(key, 1) // Another physical press of the same key in this batch.
	for i, char := range []rune{'~', 'a', 'a'} {
		event := state.commit(char, vtinput.InputEvent{VirtualScanCode: 30})
		if event.VirtualKeyCode != vtinput.VK_A || event.TextInput != (i < 2) {
			t.Fatalf("commit %d paired incorrectly: %+v", i, event)
		}
	}
	if len(state.pending) != 0 {
		t.Fatal("multi-character press consumed incorrectly")
	}
}

func TestGUITextInputLayoutChangePreservesQueuedCommits(t *testing.T) {
	var state guiTextInput
	old := confirmedText(vtinput.VK_Q, 0)
	state.press(old, 1)
	state.layoutChanged()
	plain := *old
	plain.ControlKeyState = 0
	plain.TextInput = false
	state.press(&plain, 1)
	first := state.commit('@', vtinput.InputEvent{VirtualScanCode: old.VirtualScanCode})
	second := state.commit('q', vtinput.InputEvent{VirtualScanCode: old.VirtualScanCode})
	if !first.TextInput || first.ControlKeyState != old.ControlKeyState || second.TextInput || second.ControlKeyState != 0 {
		t.Fatalf("layout change lost key-time snapshots: first=%+v second=%+v", first, second)
	}
}

func TestGUITextInputOrdinaryAltRemainsAccelerator(t *testing.T) {
	for _, mod := range []vtinput.ControlKeyState{vtinput.LeftAltPressed, vtinput.RightAltPressed} {
		var state guiTextInput
		key := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true,
			VirtualKeyCode: vtinput.VK_Q, VirtualScanCode: 16, ControlKeyState: mod}
		if !state.press(key, 1) {
			t.Fatal("Alt character not reserved for its SYSCHAR")
		}
		event := state.commit('q', vtinput.InputEvent{VirtualScanCode: 16})
		if event.TextInput || event.VirtualKeyCode != vtinput.VK_Q || event.ControlKeyState != mod {
			t.Fatalf("Alt accelerator changed: %+v", event)
		}
		if len(state.pending) != 0 {
			t.Fatal("accelerator commit delivered twice")
		}
	}
}

func TestGUITextInputRightAltRequiresCharacterConfirmation(t *testing.T) {
	for _, systemChar := range []bool{false, true} {
		var state guiTextInput
		key := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true,
			VirtualKeyCode: vtinput.VK_Q, VirtualScanCode: 16, ControlKeyState: vtinput.RightAltPressed}
		state.press(key, 1)
		event := state.utf16('q', vtinput.InputEvent{VirtualScanCode: 16}, systemChar)
		if event == nil || event.TextInput == systemChar || event.ControlKeyState != vtinput.RightAltPressed {
			t.Fatalf("system character %v: incorrect confirmation: %+v", systemChar, event)
		}
	}
}

func TestGUITextInputControlMessageDoesNotConsumePrintablePress(t *testing.T) {
	var state guiTextInput
	ctrlC := &vtinput.InputEvent{VirtualKeyCode: vtinput.VK_C, VirtualScanCode: 46, ControlKeyState: vtinput.LeftCtrlPressed}
	if state.press(ctrlC, 0) {
		t.Fatal("Ctrl+C turned into text")
	}
	plainC := *ctrlC
	plainC.ControlKeyState = 0
	state.press(&plainC, 1)
	if event := state.utf16(3, vtinput.InputEvent{VirtualScanCode: 46}, false); event != nil {
		t.Fatal("control character emitted")
	}
	event := state.utf16('c', vtinput.InputEvent{VirtualScanCode: 46, ControlKeyState: vtinput.LeftCtrlPressed}, false)
	if event == nil || event.VirtualKeyCode != vtinput.VK_C || event.ControlKeyState != 0 || event.TextInput {
		t.Fatalf("control message consumed plain source: %+v", event)
	}
}

func TestGUITextInputLaterModifiersDoNotMarkEarlierPlainText(t *testing.T) {
	var state guiTextInput
	state.press(&vtinput.InputEvent{VirtualKeyCode: vtinput.VK_A, VirtualScanCode: 30}, 1)
	state.press(confirmedText(vtinput.VK_Q, 0), 1)
	event := state.commit('a', vtinput.InputEvent{VirtualScanCode: 30, TextInput: true, ControlKeyState: vtinput.LeftCtrlPressed | vtinput.RightAltPressed})
	if event.TextInput || event.ControlKeyState != 0 || event.VirtualKeyCode != vtinput.VK_A {
		t.Fatalf("later modifiers changed prior source: %+v", event)
	}
}

func TestGUITextInputInterruptedSurrogateDoesNotPoisonQueue(t *testing.T) {
	var state guiTextInput
	key := confirmedText(vtinput.VK_E, 0)
	state.press(key, 2)
	fallback := vtinput.InputEvent{VirtualScanCode: key.VirtualScanCode}
	state.utf16(0xd83d, fallback, false)
	state.layoutChanged()
	if event := state.utf16(0xde00, fallback, false); event != nil {
		t.Fatal("interrupted surrogate emitted")
	}
	if len(state.pending) != 0 {
		t.Fatal("interrupted surrogate left a stale source")
	}
	plain := *key
	plain.ControlKeyState = 0
	plain.TextInput = false
	state.press(&plain, 1)
	if event := state.utf16('e', fallback, false); event.TextInput || event.Char != 'e' {
		t.Fatalf("stale surrogate affected new key: %+v", event)
	}
}

func TestGUITextInputSurrogatesRequireSameSource(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*vtinput.InputEvent)
	}{
		{"virtual key", func(e *vtinput.InputEvent) { e.VirtualKeyCode = vtinput.VK_Q }},
		{"modifiers", func(e *vtinput.InputEvent) { e.ControlKeyState ^= vtinput.ShiftPressed }},
		{"repeat", func(e *vtinput.InputEvent) { e.RepeatCount++ }},
		{"input source", func(e *vtinput.InputEvent) { e.InputSource = "different" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var state guiTextInput
			first := confirmedText(vtinput.VK_E, 0)
			second := *first
			tc.change(&second)
			state.press(first, 1)
			state.press(&second, 1)
			fallback := vtinput.InputEvent{VirtualScanCode: first.VirtualScanCode}
			state.utf16(0xd83d, fallback, false)
			if event := state.utf16(0xde00, fallback, false); event != nil {
				t.Fatalf("surrogates from different presses combined: %+v", event)
			}
			if state.high != 0 || len(state.pending) != 0 {
				t.Fatal("mismatched pair retained stale source")
			}
		})
	}
}

func TestConfirmedTextBypassesEditorShortcuts(t *testing.T) {
	edit := NewEdit(0, 0, 20, "")
	edit.History = []string{"history"}
	edit.ProcessKey(confirmedText(vtinput.VK_E, '€'))
	if edit.GetText() != "€" {
		t.Fatalf("history shortcut took text: %q", edit.GetText())
	}
	multi := NewMultiLineEdit(0, 0, 20, 5, "")
	event := confirmedText(vtinput.VK_A, 'Ą')
	event.RepeatCount = 2
	multi.ProcessKey(event)
	if multi.GetText() != "ĄĄ" {
		t.Fatalf("multiline shortcut/repeat took text: %q", multi.GetText())
	}
}

func TestConfirmedTextCannotActivateDialogOrMenu(t *testing.T) {
	group := NewGroup(0, 0, 30, 8)
	button := NewButton(1, 1, "&€")
	clicked := false
	button.OnClick = func() { clicked = true }
	group.AddItem(button)
	group.ProcessKey(confirmedText(vtinput.VK_SPACE, '€'))
	if clicked {
		t.Fatal("confirmed text activated a button or accelerator")
	}
	bar := NewMenuBar(nil)
	bar.Items = []MenuBarItem{{Label: "&€", SubItems: []MenuItem{{Text: "Action"}}}}
	if bar.ProcessKey(confirmedText(vtinput.VK_E, '€')) || bar.Active {
		t.Fatal("confirmed text activated menu bar")
	}
	menu := newFilterMenu("&€ action")
	menu.DisableFilter = true
	menu.OnKeyDown = func(*vtinput.InputEvent) bool { t.Fatal("confirmed text reached shortcut callback"); return true }
	menu.ProcessKey(confirmedText(vtinput.VK_E, '€'))
	if menu.IsDone() {
		t.Fatal("confirmed text invoked menu entry")
	}
}

func TestConfirmedTextCanEditMenuFilter(t *testing.T) {
	menu := newFilterMenu("€uro", "Other")
	menu.FilterOnType = true
	menu.ProcessKey(confirmedText(vtinput.VK_F, '€'))
	if menu.FilterText() != "€" {
		t.Fatalf("filter did not receive committed text: %q", menu.FilterText())
	}
}

func TestConfirmedTextBypassesFrameManagerFallback(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	frame := newMockFrame(0, 0, 40, 20, false)
	called := false
	frame.onHandleCommand = func(int, any) bool { called = true; return true }
	fm.Push(frame)
	event := confirmedText(vtinput.VK_N, 'ñ')
	fm.dispatchEvent(event, false)
	if called {
		t.Fatal("committed text triggered Ctrl+N workspace command")
	}
}
