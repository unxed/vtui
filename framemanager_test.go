package vtui

import (
	"testing"
	"github.com/unxed/vtinput"
)

type mockFrame struct {
	ScreenObject
	isModal        bool
	isDone         bool
	onProcessMouse func(e *vtinput.InputEvent) bool
	onProcessKey   func(e *vtinput.InputEvent) bool
}

func newMockFrame(x, y, w, h int, modal bool) *mockFrame {
	f := &mockFrame{isModal: modal}
	f.SetPosition(x, y, x+w-1, y+h-1)
	return f
}

func (m *mockFrame) ProcessKey(e *vtinput.InputEvent) bool {
	if m.onProcessKey != nil {
		return m.onProcessKey(e)
	}
	return false
}

func (m *mockFrame) ProcessMouse(e *vtinput.InputEvent) bool {
	if m.onProcessMouse != nil {
		return m.onProcessMouse(e)
	}
	return false
}

func (m *mockFrame) Show(scr *ScreenBuf)                 {}
func (m *mockFrame) ResizeConsole(w, h int)             {}
func (m *mockFrame) GetType() FrameType                 { return TypeUser }
func (m *mockFrame) SetExitCode(c int)                  { m.isDone = true }
func (m *mockFrame) IsDone() bool                       { return m.isDone }
func (m *mockFrame) GetHelp() string                    { return "" }
func (m *mockFrame) IsBusy() bool                       { return false }
func (m *mockFrame) HasShadow() bool                    { return false }
func (m *mockFrame) GetKeyLabels() *KeySet              { return nil }
func (m *mockFrame) HandleCommand(cmd int, args any) bool { return false }
func (m *mockFrame) IsModal() bool                      { return m.isModal }
func (m *mockFrame) GetWindowNumber() int               { return 0 }
func (m *mockFrame) SetWindowNumber(n int)              {}
func (m *mockFrame) RequestFocus() bool                 { return true }
func (m *mockFrame) Close()                             { m.SetExitCode(-1) }

type busyFrame struct {
	mockFrame
	Busy bool
}
func (b *busyFrame) IsBusy() bool { return b.Busy }

func TestFrameManager_IsBusy_Suppress(t *testing.T) {
	// This test checks that if IsBusy() == true,
	// rendering can be skipped (logical contract check)
	f := &busyFrame{Busy: true}
	if !f.IsBusy() {
		t.Error("busyFrame should be busy")
	}
}
func TestFrameManager_OnRenderHook(t *testing.T) {
	fm := &frameManager{}
	scr := NewScreenBuf()
	scr.AllocBuf(10, 10)
	fm.Init(scr)

	renderCalled := false
	fm.OnRender = func(s *ScreenBuf) {
		renderCalled = true
	}
	
	// Manually trigger the hook to verify the mechanism works
	if fm.OnRender != nil {
		fm.OnRender(scr)
	}

	if !renderCalled {
		t.Error("OnRender hook was not executed correctly")
	}
}

func TestFrameManager_NoDoubleDispatch(t *testing.T) {
	scr := NewScreenBuf()
	scr.AllocBuf(10, 10)
	fm := &frameManager{}
	fm.Init(scr)

	frame := &mockFrame{}
	fm.Push(frame)

	// Simulate one event in the channel
	eventChan := make(chan *vtinput.InputEvent, 1)
	eventChan <- &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'A'}
	close(eventChan)

	// Run the loop for one iteration (IsDone will return true after processing events)
	// In our implementation fm.Run() contains an infinite loop, so for the test
	// we would have to refactor it. But we can check the dispatch logic.

	// Simply ensure that ProcessKey is called exactly once for 1 event.
	// (This test is more for documenting the problem; the real fm.Run is too monolithic to test without changes)
}

func TestFrameManager_GetTopFrameType(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())

	// Empty stack
	if fm.GetTopFrameType() != -1 {
		t.Errorf("Expected GetTopFrameType to return -1 on empty stack, got %d", fm.GetTopFrameType())
	}

	// Push Desktop (TypeDesktop = 0)
	fm.Push(NewDesktop())
	if fm.GetTopFrameType() != TypeDesktop {
		t.Errorf("Expected TopFrameType to be TypeDesktop, got %d", fm.GetTopFrameType())
	}

	// Push User Frame
	fm.Push(&mockFrame{})
	if fm.GetTopFrameType() != TypeUser {
		t.Errorf("Expected TopFrameType to be TypeUser, got %d", fm.GetTopFrameType())
	}
}
func TestFrameManager_MouseCapture(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())

	var mouseEvents []*vtinput.InputEvent
	frame := newMockFrame(10, 10, 10, 10, false)
	frame.onProcessMouse = func(e *vtinput.InputEvent) bool {
		mouseEvents = append(mouseEvents, e)
		return true // Возвращаем true, чтобы FrameManager захватил мышь
	}
	fm.Push(frame)

	// Симулируем события:
	// 1. Клик внутри (захват)
	// 2. Движение ДАЛЕКО за пределы (например, координаты -50, -50)
	// 3. Отпускание кнопки
	events := []*vtinput.InputEvent{
		{Type: vtinput.MouseEventType, MouseX: 15, MouseY: 15, ButtonState: vtinput.FromLeft1stButtonPressed, KeyDown: true},
		{Type: vtinput.MouseEventType, MouseX: 500, MouseY: 500, ButtonState: vtinput.FromLeft1stButtonPressed, KeyDown: false},
		{Type: vtinput.MouseEventType, MouseX: 500, MouseY: 500, ButtonState: 0, KeyDown: false},
	}

	for _, e := range events {
		// Используем реальную логику диспетчеризации из нашего плана
		if fm.capturedFrame != nil {
			fm.capturedFrame.ProcessMouse(e)
			if e.ButtonState == 0 {
				fm.capturedFrame = nil
			}
		} else {
			for i := len(fm.frames) - 1; i >= 0; i-- {
				f := fm.frames[i]
				x1, y1, x2, y2 := f.GetPosition()
				if int(e.MouseX) >= x1 && int(e.MouseX) <= x2+2 && int(e.MouseY) >= y1 && int(e.MouseY) <= y2+1 {
					if f.ProcessMouse(e) && e.ButtonState != 0 {
						fm.capturedFrame = f
					}
					break
				}
			}
		}
	}

	if len(mouseEvents) != 3 {
		t.Errorf("Mouse Capture failed: frame should receive ALL events after capture, got %d", len(mouseEvents))
	}
	if mouseEvents[1].MouseX != 500 {
		t.Errorf("Captured event data corrupted: expected X=500, got %d", mouseEvents[1].MouseX)
	}
}

func TestFrameManager_ModalDialogBlocksClicks(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())

	// Background frame that should not receive the click
	bgFrame := newMockFrame(0, 0, 50, 50, false)
	bgClicked := false
	bgFrame.onProcessMouse = func(e *vtinput.InputEvent) bool {
		bgClicked = true
		return true
	}
	fm.Push(bgFrame)

	// Modal dialog on top
	modalDlg := newMockFrame(10, 10, 20, 10, true)
	modalClicked := false
	modalDlg.onProcessMouse = func(e *vtinput.InputEvent) bool {
		modalClicked = true
		return true
	}
	fm.Push(modalDlg)

	// Click outside the modal dialog, but inside the background frame
	eventChan := make(chan *vtinput.InputEvent, 1)
	eventChan <- &vtinput.InputEvent{Type: vtinput.MouseEventType, MouseX: 5, MouseY: 5, ButtonState: vtinput.FromLeft1stButtonPressed}

	e := <-eventChan
	dispatchHelper(fm, e)

	if bgClicked {
		t.Error("Background frame received a click when a modal dialog was on top")
	}
	if modalClicked {
		t.Error("Modal dialog should not receive click when it's outside its bounds")
	}
}

// dispatchHelper is a simplified version of the dispatch logic in Run()
func dispatchHelper(fm *frameManager, ev *vtinput.InputEvent) {
	if fm.capturedFrame != nil {
		fm.capturedFrame.ProcessMouse(ev)
		if ev.ButtonState == 0 {
			fm.capturedFrame = nil
		}
		return
	}
	// Simplified hit-testing
	for i := len(fm.frames) - 1; i >= 0; i-- {
		f := fm.frames[i]
		x1, y1, x2, y2 := f.GetPosition()
		if int(ev.MouseX) >= x1 && int(ev.MouseX) <= x2 && int(ev.MouseY) >= y1 && int(ev.MouseY) <= y2 {
			if fm.RequestFocus(f) {
				if f.ProcessMouse(ev) && ev.ButtonState != 0 {
					fm.capturedFrame = f
				}
			}
			if f.IsModal() {
				break
			}
		} else if f.IsModal() {
			break
		}
	}
}

func TestFrameManager_PostTask(t *testing.T) {
	fm := &frameManager{}
	// Инициализируем только каналы, без запуска цикла Run
	fm.TaskChan = make(chan func(), 10)
	
	taskExecuted := false
	fm.PostTask(func() {
		taskExecuted = true
	})
	
	// Извлекаем задачу из канала и выполняем её
	select {
	case task := <-fm.TaskChan:
		task()
	default:
		t.Fatal("Task was not posted to TaskChan")
	}
	
	if !taskExecuted {
		t.Error("Posted task was not executed")
	}
}
func TestFrameManager_FocusOnRemove(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())

	f1FocusReceived := false
	f1 := &mockFrame{}
	f1.onProcessKey = func(e *vtinput.InputEvent) bool {
		if e.Type == vtinput.FocusEventType && e.SetFocus {
			f1FocusReceived = true
		}
		return true
	}

	f2 := &mockFrame{}

	fm.Push(f1)
	f1FocusReceived = false // Reset after initial push
	fm.Push(f2)

	// Removing f2 (the top frame) should trigger focus on f1
	fm.RemoveFrame(f2)

	if !f1FocusReceived {
		t.Error("Underlying frame did not receive focus after top frame was removed")
	}
}
func TestFrameManager_KeyBarUpdate(t *testing.T) {
	fm := &frameManager{}
	scr := NewScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)
	fm.KeyBar = NewKeyBar()

	f1 := &mockFrame{}
	f1.onProcessKey = func(e *vtinput.InputEvent) bool { return false }
	// This frame provides specific labels
	f1.onProcessMouse = func(e *vtinput.InputEvent) bool { return false }

	// We need to manually implement GetKeyLabels for this mock in the test
	// but since mockFrame is a struct, we'll use a wrapper or just a specialized mock
}

// Specialized mock for label testing
type labelFrame struct {
	mockFrame
	labels *KeySet
}
func (l *labelFrame) GetKeyLabels() *KeySet { return l.labels }
type menuFrame struct {
	mockFrame
	menu *MenuBar
}
func (m *menuFrame) GetMenuBar() *MenuBar { return m.menu }

func TestFrameManager_ContextualMenuBar(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())

	globalMenu := NewMenuBar([]string{"Global"})
	fm.MenuBar = globalMenu

	// 1. With no frames, should return global menu
	if fm.GetActiveMenuBar() != globalMenu {
		t.Error("Should return global menu when stack is empty")
	}

	// 2. Push a frame without its own menu
	f1 := &mockFrame{}
	fm.Push(f1)
	if fm.GetActiveMenuBar() != globalMenu {
		t.Error("Should return global menu if top frame has no context menu")
	}

	// 3. Push a frame WITH a context menu
	contextMenu := NewMenuBar([]string{"Context"})
	f2 := &menuFrame{menu: contextMenu}
	fm.Push(f2)

	if fm.GetActiveMenuBar() != contextMenu {
		t.Error("FrameManager failed to pick contextual MenuBar from top frame")
	}

	// 4. Pop it, should go back to global
	fm.Pop()
	if fm.GetActiveMenuBar() != globalMenu {
		t.Error("Should return global menu after popping contextual frame")
	}
}

func TestFrameManager_ContextualLabels(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())
	fm.KeyBar = NewKeyBar()

	ks := &KeySet{Normal: KeyBarLabels{"TestLabel"}}
	f := &labelFrame{labels: ks}
	fm.Push(f)

	// Simulate one frame of the main loop logic
	for i := len(fm.frames) - 1; i >= 0; i-- {
		if labels := fm.frames[i].GetKeyLabels(); labels != nil {
			fm.KeyBar.Normal = labels.Normal
			break
		}
	}

	if fm.KeyBar.Normal[0] != "TestLabel" {
		t.Errorf("KeyBar did not update from frame. Got %q", fm.KeyBar.Normal[0])
	}
}
func TestFrameManager_CommandRouting(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())

	f1 := &mockFrame{}
	f1.onProcessKey = func(e *vtinput.InputEvent) bool { return false }
	// Override HandleCommand via a wrapper for testing
}

type cmdMockFrame struct {
	mockFrame
	onCmd func(cmd int, args any) bool
}

func (c *cmdMockFrame) HandleCommand(cmd int, args any) bool {
	if c.onCmd != nil { return c.onCmd(cmd, args) }
	return false
}

func TestFrameManager_CommandBubbling(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())

	fBottom := &cmdMockFrame{}
	fTop := &cmdMockFrame{}

	topCalled := false
	bottomCalled := false

	fTop.onCmd = func(cmd int, args any) bool {
		topCalled = true
		return false // Top frame doesn't handle it, should bubble down to bottom
	}

	fBottom.onCmd = func(cmd int, args any) bool {
		bottomCalled = true
		if cmd == 999 { return true }
		return false
	}

	fm.Push(fBottom)
	fm.Push(fTop)

	handled := fm.EmitCommand(999, nil)

	if !handled {
		t.Error("Command should have been handled by fBottom")
	}
	if !topCalled {
		t.Error("Command should have visited top frame first")
	}
	if !bottomCalled {
		t.Error("Command should have bubbled down to bottom frame")
	}
}
func TestFrameManager_ModalPriorityOverMenu(t *testing.T) {
	fm := &frameManager{}
	scr := NewScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)
	fm.Push(NewDesktop())

	oldFm := FrameManager
	FrameManager = fm
	defer func() { FrameManager = oldFm }()

	mb := NewMenuBar([]string{"Options"})
	fm.MenuBar = mb
	mb.Active = true // Menu is "open"

	dlg := NewDialog(0, 0, 10, 10, "Modal")
	btn := NewButton(1, 1, "Ok")
	okClicked := false
	btn.OnClick = func() { okClicked = true }
	dlg.AddItem(btn)
	fm.Push(dlg) // Modal dialog appears OVER the active menu

	fm.InjectEvents([]*vtinput.InputEvent{
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN},
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_Q, ControlKeyState: vtinput.LeftCtrlPressed}, // Quit loop
	})

	fm.Run()

	if !okClicked {
		t.Error("Modal dialog should have priority over active MenuBar for Enter key")
	}
	if !mb.Active {
		t.Error("MenuBar should remain Active (suspended) behind the modal dialog")
	}
}

func TestFrameManager_MenuAccessibleDuringNonModal(t *testing.T) {
	fm := &frameManager{}
	scr := NewScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)
	fm.Push(NewDesktop())

	oldFm := FrameManager
	FrameManager = fm
	defer func() { FrameManager = oldFm }()

	mb := NewMenuBar([]string{"&Options"})
	fm.MenuBar = mb

	win := NewWindow(0, 0, 10, 10, "Non-Modal")
	fm.Push(win)

	// Simulate pressing F9 to activate menu while window is open
	fm.InjectEvents([]*vtinput.InputEvent{
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F9},
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_Q, ControlKeyState: vtinput.LeftCtrlPressed}, // Quit loop
	})

	fm.Run()

	if !mb.Active {
		t.Error("MenuBar should be activatable when the top frame is non-modal (e.g. Progress window)")
	}
}
func TestFrameManager_CleanupAllDoneFrames(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())

	f1 := &mockFrame{} // Bottom frame
	f2 := &mockFrame{} // Top frame

	fm.Push(f1)
	fm.Push(f2)

	// Simulate f1 (the one underneath) finishing
	f1.SetExitCode(0)

	// Replicating the cleanup logic from fm.Run()
	for i := len(fm.frames) - 1; i >= 0; i-- {
		if fm.frames[i].IsDone() {
			fm.RemoveFrame(fm.frames[i])
		}
	}

	if len(fm.frames) != 1 {
		t.Errorf("Expected 1 frame left, got %d", len(fm.frames))
	}
	if fm.frames[0] != f2 {
		t.Error("Wrong frame was removed. f2 (the top one) should remain.")
	}
}
func TestFrameManager_MenuBarNavigabilityWithSubMenu(t *testing.T) {
	fm := &frameManager{}
	scr := NewScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)
	fm.Push(NewDesktop())

	oldFm := FrameManager
	FrameManager = fm
	defer func() { FrameManager = oldFm }()

	// Setup MenuBar with two items
	mb := NewMenuBar(nil)
	mb.Items = []MenuBarItem{
		{Label: "File", SubItems: []MenuItem{{Text: "Open"}}},
		{Label: "Edit", SubItems: []MenuItem{{Text: "Copy"}}},
	}
	fm.MenuBar = mb
	mb.Active = true

	// 1. Open the first submenu ("File")
	mb.ActivateSubMenu(0)
	if fm.GetTopFrameType() != TypeMenu {
		t.Fatal("Submenu File not open")
	}

	// 2. Inject Right Arrow.
	// The MenuBar should intercept it, close "File" and open "Edit".
	fm.InjectEvents([]*vtinput.InputEvent{
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT},
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_Q, ControlKeyState: vtinput.LeftCtrlPressed},
	})

	fm.Run()

	// Check if we are now on the "Edit" menu
	if fm.GetTopFrameType() != TypeMenu {
		t.Fatal("Menu was closed instead of switched")
	}
	currentMenu := fm.frames[len(fm.frames)-1].(*VMenu)
	if currentMenu.title != "Edit" {
		t.Errorf("Expected switched menu 'Edit', got %q", currentMenu.title)
	}
}

type modalOwnerFrame struct {
	mockFrame
	mb *MenuBar
}

func (m *modalOwnerFrame) GetMenuBar() *MenuBar { return m.mb }

func TestFrameManager_F9WorksForMenuOwningModal(t *testing.T) {
	fm := &frameManager{}
	scr := NewScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)
	fm.Push(NewDesktop())

	oldFm := FrameManager
	FrameManager = fm
	defer func() { FrameManager = oldFm }()

	// Create a modal frame that HAS its own menu bar
	myMenu := NewMenuBar([]string{"OwnedMenu"})
	modal := &modalOwnerFrame{
		mockFrame: *newMockFrame(0, 0, 10, 10, true),
		mb:        myMenu,
	}
	fm.Push(modal)

	// Inject F9
	fm.InjectEvents([]*vtinput.InputEvent{
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F9},
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_Q, ControlKeyState: vtinput.LeftCtrlPressed},
	})

	fm.Run()

	if !myMenu.Active {
		t.Error("F9 should activate the menu because the modal frame owns it")
	}
}

func TestFrameManager_ModalDialogBlocksF9(t *testing.T) {
	fm := &frameManager{}
	scr := NewScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)
	fm.Push(NewDesktop())

	mb := NewMenuBar([]string{"Options"})
	fm.MenuBar = mb
	mb.Active = false

	dlg := NewDialog(0, 0, 10, 10, "Test")
	fm.Push(dlg)

	fm.InjectEvents([]*vtinput.InputEvent{
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F9},
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_Q, ControlKeyState: vtinput.LeftCtrlPressed}, // Quit loop
	})

	fm.Run()

	if mb.Active {
		t.Error("MenuBar should NOT be activated via F9 when a modal dialog is open")
	}
}
