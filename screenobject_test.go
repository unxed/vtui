package vtui

import (
	"testing"
	"github.com/unxed/vtinput"
)

type mockOwner struct {
	ScreenObject
	commandHandled bool
	lastCmd        int
	lastArgs       any
}

func (m *mockOwner) HandleCommand(cmd int, args any) bool {
	m.commandHandled = true
	m.lastCmd = cmd
	m.lastArgs = args
	return true
}

func TestScreenObject_FireAction(t *testing.T) {
	owner := &mockOwner{}
	so := &ScreenObject{}
	so.SetOwner(owner)

	// 1. Test OnClick priority
	clicked := false
	owner.commandHandled = false
	onClick := func() { clicked = true }
	so.Command = 123

	handled := so.FireAction(onClick, nil)

	if !handled {
		t.Error("FireAction should return true when OnClick is handled")
	}
	if !clicked {
		t.Error("OnClick was not called")
	}
	if owner.commandHandled {
		t.Error("Command should not be handled when OnClick is present")
	}

	// 2. Test Command bubbling
	clicked = false
	owner.commandHandled = false
	so.Command = 456

	handled = so.FireAction(nil, "test_args")

	if !handled {
		t.Error("FireAction should return true when command is handled by owner")
	}
	if clicked {
		t.Error("OnClick should not be called when it's nil")
	}
	if !owner.commandHandled {
		t.Error("Command was not bubbled up to owner")
	}
	if owner.lastCmd != 456 || owner.lastArgs != "test_args" {
		t.Errorf("Command data mismatch: cmd=%d, args=%v", owner.lastCmd, owner.lastArgs)
	}

	// 3. Test no action
	owner.commandHandled = false
	so.Command = 0
	handled = so.FireAction(nil, nil)
	if handled {
		t.Error("FireAction should return false when there is nothing to do")
	}
}

func TestCommandBubbling_MultiLevel(t *testing.T) {
	// Regression test for "broken menu" bug.
	// Flow: VMenu (item clicked) -> MenuBar (owner) -> PanelsFrame (owner)
	
	finalTarget := &mockOwner{}
	
	mb := NewMenuBar([]string{"File"})
	mb.SetOwner(finalTarget)
	
	vm := NewVMenu("Sub")
	vm.SetOwner(mb)
	vm.AddItem(MenuItem{Text: "Action", Command: 777})
	vm.SetSelectPos(0)
	
	// Simulate Enter on menu item
	vm.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, 
		KeyDown: true, 
		VirtualKeyCode: vtinput.VK_RETURN,
	})
	
	if !finalTarget.commandHandled {
		t.Error("Multi-level bubbling failed: Command did not reach the final owner")
	}
	if finalTarget.lastCmd != 777 {
		t.Errorf("Wrong command reached owner: expected 777, got %d", finalTarget.lastCmd)
	}
}
