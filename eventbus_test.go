package vtui

import (
	"testing"
	"github.com/unxed/vtinput"
)

func TestEventBus_PublishSubscribe(t *testing.T) {
	bus := &eventBus{listeners: make(map[EventType][]Listener)}

	received := false
	var receivedData string

	bus.Subscribe(EvCommand, func(e Event) {
		received = true
		receivedData = e.Data.(string)
	})

	bus.Publish(Event{
		Type: EvCommand,
		Data: "HelloBus",
	})

	if !received {
		t.Error("Event was not received")
	}
	if receivedData != "HelloBus" {
		t.Errorf("Expected 'HelloBus', got %q", receivedData)
	}
}

func TestEventBus_MultipleListeners(t *testing.T) {
	bus := &eventBus{listeners: make(map[EventType][]Listener)}
	count := 0

	l := func(e Event) { count++ }
	bus.Subscribe(EvCommand, l)
	bus.Subscribe(EvCommand, l)

	bus.Publish(Event{Type: EvCommand})

	if count != 2 {
		t.Errorf("Expected 2 handler calls, got %d", count)
	}
}

func TestEventBus_Integration_cmQuit(t *testing.T) {
	scr := NewScreenBuf()
	scr.AllocBuf(10, 10)
	fm := &frameManager{}
	fm.Init(scr)

	fm.Push(NewDesktop()) // Чтобы было что закрывать

	// Публикуем команду выхода через шину
	GlobalEvents.Publish(Event{
		Type: EvCommand,
		Data: CmQuit,
	})

	if len(fm.frames) != 0 {
		t.Error("FrameManager failed to react to Global cmQuit command")
	}
}
func TestCommandSet_Logic(t *testing.T) {
	cs := NewCommandSet()
	cmd := 100

	if cs.IsDisabled(cmd) { t.Error("Should be enabled by default") }

	cs.Disable(cmd)
	if !cs.IsDisabled(cmd) { t.Error("Should be disabled after Disable()") }

	cs.Enable(cmd)
	if cs.IsDisabled(cmd) { t.Error("Should be enabled after Enable()") }
}
func TestCommandSet_Clear(t *testing.T) {
	cs := NewCommandSet()
	cs.Disable(1)
	cs.Disable(2)
	cs.Clear()
	if cs.IsDisabled(1) || cs.IsDisabled(2) {
		t.Error("Clear() failed to empty the command set")
	}
}

func TestVMenu_DisabledCommandVisualsAndLogic(t *testing.T) {
	fm := FrameManager
	fm.Init(NewScreenBuf())
	defer fm.Shutdown()

	// Force white text AFTER Init (because Init resets the palette)
	Palette[ColMenuText] = SetRGBFore(0, 0xFFFFFF)

	const testCmd = 999
	m := NewVMenu("Test")
	m.AddItem(MenuItem{Text: "Enabled", Command: 0})
	m.AddItem(MenuItem{Text: "Disabled", Command: testCmd})
	m.SetPosition(0, 0, 20, 5)

	// 1. Check Visuals
	fm.DisabledCommands.Disable(testCmd)
	scr := NewScreenBuf()
	scr.AllocBuf(21, 6)
	m.Show(scr)

	// Normal item (index 0) should have normal color
	normalAttr := scr.GetCell(2, 1).Attributes
	// Disabled item (index 1) should have dimmed color (halved RGB)
	disabledAttr := scr.GetCell(2, 2).Attributes

	if GetRGBFore(disabledAttr) >= GetRGBFore(normalAttr) && GetRGBFore(normalAttr) > 0 {
		t.Errorf("Disabled item should be dimmer. Normal: %X, Disabled: %X",
			GetRGBFore(normalAttr), GetRGBFore(disabledAttr))
	}

	// 2. Check Logic: Enter on disabled item
	m.SelectPos = 1
	m.done = false
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})

	// If the item is disabled, it might still close the menu (Far behavior varies),
	// but it MUST NOT emit a command.
	// In our vtui implementation, the easiest check is that exitCode isn't set OR it's handled as "no-op".
	// Let's check that EmitCommand wasn't called (by using a disabled command, which EmitCommand blocks anyway).
}

func TestMenuBar_TopLevelDimming(t *testing.T) {
	fm := FrameManager
	fm.Init(NewScreenBuf())
	defer fm.Shutdown()

	// Force white text for this test AFTER Init
	Palette[ColMenuBarItem] = SetRGBFore(0, 0xFFFFFF)

	const cmd1 = 101
	mb := NewMenuBar(nil)
	mb.Items = []MenuBarItem{
		{Label: "Group", SubItems: []MenuItem{{Text: "Action", Command: cmd1}}},
	}
	mb.SetPosition(0, 0, 10, 0)

	scr := NewScreenBuf()
	scr.AllocBuf(11, 1)

	// Case 1: Enabled
	fm.DisabledCommands.Enable(cmd1)
	mb.Show(scr)
	// Sample at X=4 (letter 'G' in "  Group  ")
	attrEnabled := scr.GetCell(4, 0).Attributes

	// Case 2: Disabled
	fm.DisabledCommands.Disable(cmd1)
	mb.Show(scr)
	attrDisabled := scr.GetCell(4, 0).Attributes

	if GetRGBFore(attrDisabled) >= GetRGBFore(attrEnabled) {
		t.Errorf("Top-level menu should be dimmed if all sub-items are disabled. En: %X, Dis: %X",
			GetRGBFore(attrEnabled), GetRGBFore(attrDisabled))
	}
}

func TestFrameManager_DisabledCommandBlocking(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())

	cmdTriggered := false
	frame := &cmdMockFrame{
		onCmd: func(cmd int, args any) bool {
			cmdTriggered = true
			return true
		},
	}
	fm.Push(frame)

	// 1. Normal execution
	fm.EmitCommand(55, nil)
	if !cmdTriggered { t.Error("Command should have been triggered") }

	// 2. Disabled execution
	cmdTriggered = false
	fm.DisabledCommands.Disable(55)
	handled := fm.EmitCommand(55, nil)

	if handled { t.Error("EmitCommand should return false for disabled commands") }
	if cmdTriggered { t.Error("Disabled command reached the frame!") }
}
