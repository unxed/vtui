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
