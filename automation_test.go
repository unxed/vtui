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

func TestAutomation_InverseActions(t *testing.T) {
	dlg := NewDialog(0, 0, 40, 10, "Inverse Test")
	chk := NewCheckbox(2, 2, "Disable Others", false)
	btn := NewButton(2, 4, "Action")
	lbl := NewLabel(2, 6, "Notice", nil)

	dlg.AddItem(chk); dlg.AddItem(btn); dlg.AddItem(lbl)

	dlg.AddLink(chk, btn, LinkDisableIfChecked)
	dlg.AddLink(chk, lbl, LinkHideIfChecked)

	// 1. Initial: Checkbox OFF -> Button Enabled, Label Visible
	if btn.IsDisabled() || !lbl.IsVisible() {
		t.Error("Initial state fail: items should be active")
	}

	// 2. Toggle ON -> Button Disabled, Label Hidden
	chk.State = 1
	chk.NotifyChange()

	if !btn.IsDisabled() {
		t.Error("Button should be disabled when checkbox is checked")
	}
	if lbl.IsVisible() {
		t.Error("Label should be hidden when checkbox is checked")
	}
}

func TestAutomation_BitmaskHandling(t *testing.T) {
	// Tests if syncLinks handles uint32 from CheckGroup correctly
	dlg := NewDialog(0, 0, 40, 10, "Bitmask Test")
	cg := NewCheckGroup(2, 2, 1, []string{"Option 1"})
	edit := NewEdit(2, 4, 10, "")
	dlg.AddItem(cg); dlg.AddItem(edit)

	dlg.AddLink(cg, edit, LinkEnableIfChecked)

	// Initial: mask is 0 -> disabled
	if !edit.IsDisabled() { t.Error("Should be disabled initially") }

	// Set first bit -> enabled
	cg.SetData(uint32(1))
	cg.NotifyChange()

	if edit.IsDisabled() { t.Error("Should be enabled when bitmask is non-zero") }
}
