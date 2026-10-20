package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

func TestRadioButton_ProcessKeyAndSelect(t *testing.T) {
	rb := NewRadioButton(0, 0, "Option", false)
	changes := 0
	rb.OnChange = func(selected bool) {
		changes++
		if !selected {
			t.Errorf("OnChange(false), want true")
		}
	}

	if rb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, VirtualKeyCode: vtinput.VK_SPACE}) {
		t.Fatal("key-up should not be consumed")
	}
	if rb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN}) {
		t.Fatal("unrelated key should not be consumed")
	}

	if !rb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE}) {
		t.Fatal("Space should be consumed")
	}
	if !rb.Selected || changes != 1 {
		t.Fatalf("after Space: selected=%v changes=%d, want true and 1", rb.Selected, changes)
	}
	if !rb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: ' '}) {
		t.Fatal("space character should be consumed")
	}
	if changes != 1 {
		t.Fatalf("selecting an already-selected button fired OnChange %d times, want 1", changes)
	}
}

func TestRadioButton_ProcessKeyDisabledAndData(t *testing.T) {
	rb := NewRadioButton(0, 0, "Option", false)
	rb.SetDisabled(true)
	if rb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE}) {
		t.Fatal("disabled radio button consumed Space")
	}
	if rb.Selected {
		t.Fatal("disabled radio button became selected")
	}

	rb.SetData(true)
	if got, ok := rb.GetData().(bool); !ok || !got {
		t.Fatalf("GetData after SetData(true) = %v, want true", rb.GetData())
	}
	rb.SetData("not a bool")
	if got := rb.GetData(); got != true {
		t.Fatalf("SetData(non-bool) changed selection: %v", got)
	}
}

func TestRadioButton_ProcessMouseAndSelectionNoop(t *testing.T) {
	rb := NewRadioButton(0, 0, "Option", false)
	changes := 0
	rb.OnChange = func(bool) { changes++ }

	if rb.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, ButtonState: vtinput.FromLeft2ndButtonPressed}) {
		t.Fatal("right click should not be consumed")
	}
	if !rb.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, ButtonState: vtinput.FromLeft1stButtonPressed}) {
		t.Fatal("left press should be consumed")
	}
	if !rb.Selected || changes != 1 {
		t.Fatalf("after left click: selected=%v changes=%d, want true and 1", rb.Selected, changes)
	}
	rb.Select()
	if changes != 1 {
		t.Fatalf("Select on selected button fired OnChange %d times, want 1", changes)
	}

	rb.SetDisabled(true)
	if rb.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, ButtonState: vtinput.FromLeft1stButtonPressed}) {
		t.Fatal("disabled radio button consumed mouse press")
	}
}
