package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

func TestButton_OnClick(t *testing.T) {
	b := NewButton(0, 0, "OK")
	clicked := false
	b.OnClick = func() { clicked = true }

	// Test KeyDown Space
	b.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE})
	if !clicked {
		t.Error("Button should be clicked on Space")
	}

	clicked = false
	// Test KeyDown Return (Buttons SHOULD still handle Return)
	b.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})
	if !clicked {
		t.Error("Button should be clicked on Return")
	}

	clicked = false
	// Mouse down only presses the button visually; release performs the click.
	b.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, ButtonState: vtinput.FromLeft1stButtonPressed})
	if clicked {
		t.Error("Button should not click on mouse down")
	}
	if !b.mousePressed {
		t.Error("Button should be visually pressed on mouse down")
	}
	b.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, ButtonState: 0})
	if !clicked {
		t.Error("Button should be clicked on mouse release")
	}
	if b.mousePressed || b.mouseArmed {
		t.Error("Button should reset its mouse state after release")
	}
}

func TestButton_MouseReleaseOutsideCancelsClick(t *testing.T) {
	b := NewButton(2, 1, "OK")
	clicked := false
	b.OnClick = func() { clicked = true }

	b.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX:      3, MouseY: 1,
	})
	b.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseEventFlags: vtinput.MouseMoved,
		MouseX:          20, MouseY: 5,
	})
	if b.mousePressed {
		t.Error("Button should not look pressed while pointer is outside")
	}
	b.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, ButtonState: 0,
		MouseX: 20, MouseY: 5,
	})
	if clicked {
		t.Error("Button should not click when released outside")
	}
}

func TestButton_DragBackInsideRestoresPressedStateAndClicks(t *testing.T) {
	b := NewButton(2, 1, "OK")
	clicked := false
	b.OnClick = func() { clicked = true }

	b.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, ButtonState: vtinput.FromLeft1stButtonPressed, MouseX: 3, MouseY: 1})
	b.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, ButtonState: vtinput.FromLeft1stButtonPressed, MouseEventFlags: vtinput.MouseMoved, MouseX: 20, MouseY: 5})
	b.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, ButtonState: vtinput.FromLeft1stButtonPressed, MouseEventFlags: vtinput.MouseMoved, MouseX: 3, MouseY: 1})
	if !b.mousePressed {
		t.Error("Button should look pressed again after dragging back inside")
	}
	b.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, ButtonState: 0, MouseX: 3, MouseY: 1})
	if !clicked {
		t.Error("Button should click when released inside after dragging back")
	}
}

func TestButton_HotkeyParsing(t *testing.T) {
	b := NewButton(0, 0, "Sa&ve")
	// Check that the constructor correctly extracted 'v' (lowercase)
	if b.GetHotkey() != 'v' {
		t.Errorf("Expected hotkey 'v', got %c", b.GetHotkey())
	}
	// The brackets must not leak into the caption exposed to the outside.
	if b.GetCaption() != "Save" {
		t.Errorf("Expected caption %q, got %q", "Save", b.GetCaption())
	}
	if b.GetText() != "[ Sa&ve ]" {
		t.Errorf("Expected raw text %q, got %q", "[ Sa&ve ]", b.GetText())
	}
	node := b.SemanticNode(&SemanticContext{Width: 80, Height: 25})
	if node["text"] != "Save" {
		t.Errorf("Expected semantic text %q, got %v", "Save", node["text"])
	}
	if node["hotkey"] != "v" {
		t.Errorf("Expected semantic hotkey %q, got %v", "v", node["hotkey"])
	}

	// A later SetText must re-decorate the text and refresh both the caption
	// and the hotkey.
	b.SetText("&Close")
	if b.GetCaption() != "Close" || b.GetText() != "[ &Close ]" {
		t.Errorf("Expected the caption to follow SetText, got %q and %q", b.GetCaption(), b.GetText())
	}
	if b.GetHotkey() != 'c' {
		t.Errorf("Expected hotkey 'c' after SetText, got %c", b.GetHotkey())
	}
}

func TestButton_DefaultUsesHighlightStyleWhenUnfocused(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 3)

	normal := NewButton(0, 0, "Normal")
	defaultButton := NewButton(0, 1, "Default")
	defaultButton.IsDefault = true
	normal.Show(scr)
	defaultButton.Show(scr)

	checkCell(t, scr, 2, 0, 'N', Palette[ColDialogButton])
	checkCell(t, scr, 2, 1, 'D', Palette[ColDialogHighlightButton])

	defaultButton.SetFocus(true)
	defaultButton.Show(scr)
	checkCell(t, scr, 2, 1, 'D', Palette[ColDialogSelectedButton])
}

func dialogWithButtons(t *testing.T, build func(w *BaseWindow) []*Button) (*ScreenBuf, []*Button) {
	t.Helper()
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(40, 10)
	w := NewBaseWindow(0, 0, 39, 9, "Dialog")
	buttons := build(w)
	w.Show(scr)
	return scr, buttons
}

// A dialog that flags no default button still presses its first button on
// Enter; that button has to look like the default one (f4 #320).
func TestDialog_ButtonEnterFallsBackToIsDrawnAsDefault(t *testing.T) {
	scr, buttons := dialogWithButtons(t, func(w *BaseWindow) []*Button {
		first := NewButton(2, 2, "First")
		first.OnClick = func() {}
		second := NewButton(2, 4, "Second")
		second.OnClick = func() {}
		w.AddItem(first)
		w.AddItem(second)
		return []*Button{first, second}
	})
	if !buttons[0].IsEnterDefault() || buttons[1].IsEnterDefault() {
		t.Fatalf("default flags = %v, %v; want the first only", buttons[0].IsEnterDefault(), buttons[1].IsEnterDefault())
	}
	checkCell(t, scr, 4, 2, 'F', Palette[ColDialogHighlightButton])
	checkCell(t, scr, 4, 4, 'S', Palette[ColDialogButton])
	if buttons[0].IsDefault {
		t.Fatal("the flag the dialog owns was overwritten")
	}
}

func TestDialog_FlaggedDefaultButtonWinsOverTheFallback(t *testing.T) {
	_, buttons := dialogWithButtons(t, func(w *BaseWindow) []*Button {
		first := NewButton(2, 2, "First")
		first.OnClick = func() {}
		second := NewButton(2, 4, "Second")
		second.OnClick = func() {}
		second.IsDefault = true
		w.AddItem(first)
		w.AddItem(second)
		return []*Button{first, second}
	})
	if buttons[0].IsEnterDefault() || !buttons[1].IsEnterDefault() {
		t.Fatalf("default flags = %v, %v; want the flagged second only", buttons[0].IsEnterDefault(), buttons[1].IsEnterDefault())
	}
}

func TestDialog_FallbackSkipsDisabledAndFollowsTheDialog(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(40, 10)
	SetDefaultPalette()
	w := NewBaseWindow(0, 0, 39, 9, "Dialog")
	first := NewButton(2, 2, "First")
	first.OnClick = func() {}
	first.SetDisabled(true)
	second := NewButton(2, 4, "Second")
	second.OnClick = func() {}
	w.AddItem(first)
	w.AddItem(second)
	w.Show(scr)
	if first.IsEnterDefault() || !second.IsEnterDefault() {
		t.Fatalf("with the first disabled the second must be the default: %v, %v", first.IsEnterDefault(), second.IsEnterDefault())
	}

	// The dialog then flags a default of its own: the fallback goes away.
	first.SetDisabled(false)
	first.IsDefault = true
	w.Show(scr)
	if !first.IsEnterDefault() || second.IsEnterDefault() {
		t.Fatalf("after flagging the first: %v, %v", first.IsEnterDefault(), second.IsEnterDefault())
	}
}

// What is drawn as the default is what Enter presses, whatever the layout.
func TestDialog_DrawnDefaultIsWhatEnterPresses(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(40, 10)
	w := NewBaseWindow(0, 0, 39, 9, "Dialog")
	pressed := ""
	names := []string{"A", "B", "C"}
	var buttons []*Button
	for i, name := range names {
		name := name
		b := NewButton(2, 2+i, name)
		b.OnClick = func() { pressed = name }
		buttons = append(buttons, b)
	}
	// One of them lives in a nested group, the way rows of buttons often do.
	inner := NewGroup(0, 0, 10, 3)
	inner.AddItem(buttons[1])
	w.AddItem(buttons[0])
	w.AddItem(inner)
	w.AddItem(buttons[2])
	w.Show(scr)

	drawn := ""
	for i, b := range buttons {
		if b.IsEnterDefault() {
			if drawn != "" {
				t.Fatalf("two buttons are drawn as the default: %s and %s", drawn, names[i])
			}
			drawn = names[i]
		}
	}
	if !w.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN}) {
		t.Fatal("Enter did nothing")
	}
	if pressed == "" || pressed != drawn {
		t.Fatalf("Enter pressed %q, the default is drawn on %q", pressed, drawn)
	}
}
