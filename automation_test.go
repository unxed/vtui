package vtui

import (
	"testing"
	"github.com/unxed/vtinput"
)

func TestAutomation_EnableDisable(t *testing.T) {
	SetDefaultPalette()
	dlg := NewDialog(0, 0, 40, 10, "Automation Test")

	chk := NewCheckbox(2, 2, "Enable Input", false)
	edit := NewEdit(2, 4, 20, "Initial")

	dlg.AddItem(chk)
	dlg.AddItem(edit)

	// Link: Edit is enabled ONLY when Checkbox is checked
	dlg.AddLink(chk, edit, LinkEnableIfChecked)

	// 1. Initial state: Checkbox is off (0), Edit must be disabled
	if !edit.IsDisabled() {
		t.Error("Edit should be disabled initially when checkbox is off")
	}

	// 2. Toggle checkbox to ON
	chk.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE})

	if chk.State != 1 {
		t.Fatalf("Checkbox toggle failed, state: %d", chk.State)
	}

	if edit.IsDisabled() {
		t.Error("Edit should be enabled after checkbox is checked")
	}

	// 3. Toggle checkbox back to OFF
	chk.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE})
	if !edit.IsDisabled() {
		t.Error("Edit should be disabled again after checkbox is unchecked")
	}
}

func TestAutomation_Visibility(t *testing.T) {
	dlg := NewDialog(0, 0, 40, 10, "Visibility Test")
	chk := NewCheckbox(2, 2, "Show Secret", false)
	txt := NewText(2, 4, "SECRET DATA", 0)

	dlg.AddItem(chk)
	dlg.AddItem(txt)

	dlg.AddLink(chk, txt, LinkShowIfChecked)

	// Initial: hidden
	if txt.IsVisible() {
		t.Error("Text should be hidden initially")
	}

	// Check -> Show
	chk.State = 1
	chk.NotifyChange()
	if !txt.IsVisible() {
		t.Error("Text should be visible when checked")
	}
}