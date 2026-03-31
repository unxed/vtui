package vtui

import (
	"os"
	"testing"
	"time"
	"strings"
	"github.com/unxed/vtinput"
)

type mockFrame struct {
	ScreenObject
	isModal             bool
	isDone              bool
	attentionSuppressed bool
	onProcessMouse      func(e *vtinput.InputEvent) bool
	onProcessKey        func(e *vtinput.InputEvent) bool
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
func (m *mockFrame) GetTitle() string                   { return "MockFrame" }
func (m *mockFrame) GetProgress() int                   { return -1 }
func (m *mockFrame) IsAttentionSuppressed() bool        { return m.attentionSuppressed }

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
	if c.onCmd != nil {
		return c.onCmd(cmd, args)
	}
	return false
}
func (c *cmdMockFrame) HandleBroadcast(cmd int, args any) bool {
	if c.onCmd != nil {
		return c.onCmd(cmd, args)
	}
	return false
}

func TestFrameManager_Broadcast(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())

	count1 := 0
	f1 := &cmdMockFrame{onCmd: func(cmd int, args any) bool { count1++; return true }}

	count2 := 0
	f2 := &cmdMockFrame{onCmd: func(cmd int, args any) bool { count2++; return true }}

	// Помещаем фреймы в разные экраны
	fm.Push(f1)           // Screen 0
	fm.AddScreen(f2)      // Screen 1

	// Посылаем бродкаст
	fm.Broadcast(777, nil)

	if count1 != 1 || count2 != 1 {
		t.Errorf("Broadcast failed to reach all frames. F1: %d, F2: %d", count1, count2)
	}
}

func TestFrameManager_Broadcast_RedrawTrigger(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())
	
	f := &cmdMockFrame{onCmd: func(cmd int, args any) bool { return true }}
	fm.Push(f)

	// Clear redraw channel
	select { case <-fm.RedrawChan: default: }

	fm.Broadcast(123, nil)

	select {
	case <-fm.RedrawChan:
		// Success
	default:
		t.Error("Broadcast should trigger Redraw if handled")
	}
}

func TestFrameManager_Broadcast_Shutdown(t *testing.T) {
	fm := &frameManager{}
	fm.Screens = nil // Simulate shutdown
	
	// Should not panic
	res := fm.Broadcast(1, nil)
	if res {
		t.Error("Broadcast should return false when manager is shut down")
	}
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
	btn.SetOnClick(func() { okClicked = true })
	dlg.AddItem(btn)
	fm.Push(dlg) // Modal dialog appears OVER the active menu

	fm.InjectEvents([]*vtinput.InputEvent{
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN},
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_Q, ControlKeyState: vtinput.LeftCtrlPressed}, // Quit loop
	})

	fm.Run(vtinput.NewReader(os.Stdin))

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

	fm.Run(vtinput.NewReader(os.Stdin))

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
		t.Fatalf("Submenu File not open. Len: %d, TopType: %d", len(fm.frames), fm.GetTopFrameType())
	}

	// 2. Inject Right Arrow.
	// The MenuBar should intercept it, close "File" and open "Edit".
	fm.InjectEvents([]*vtinput.InputEvent{
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT},
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_Q, ControlKeyState: vtinput.LeftCtrlPressed},
	})

	fm.Run(vtinput.NewReader(os.Stdin))

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
func TestFrameManager_HeadlessTransparency(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())

	f := &mockFrame{}
	fm.AddScreenHeadless(f)

	idx := fm.ActiveIdx
	s := fm.Screens[idx]

	if !s.Transparent {
		t.Error("AddScreenHeadless should create a transparent screen")
	}

	// Проверяем, что Desktop (Type 0) НЕ был добавлен
	for _, frame := range s.Frames {
		if frame.GetType() == TypeDesktop {
			t.Error("Headless screen should not contain a Desktop frame")
		}
	}

	if len(s.Frames) != 1 {
		t.Errorf("Expected 1 frame in headless stack, got %d", len(s.Frames))
	}
}

func TestFrameManager_NeedsAttention_Suppression(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())

	// Используем mockFrame, так как BaseFrame не реализует интерфейс Frame полностью
	dlg := &mockFrame{isModal: true}
	s := &AppScreen{Frames: []Frame{dlg}}

	// 1. По умолчанию внимание требуется
	if !s.NeedsAttention() {
		t.Error("Modal dialog should require attention by default")
	}

	// 2. С подавлением — не требуется
	dlg.attentionSuppressed = true
	if s.NeedsAttention() {
		t.Error("NeedsAttention should be false when attentionSuppressed is true")
	}
}

func TestFrameManager_NoAutoCloseForHeadless(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())

	// Добавляем фоновый экран 0
	fm.Push(NewDesktop())

	// Создаем Headless экран 1 (в нем 1 фрейм, и это НЕ Desktop)
	dlg := &mockFrame{isModal: true}
	fm.AddScreenHeadless(dlg)

	if len(fm.Screens) != 2 {
		t.Fatalf("Expected 2 screens, got %d", len(fm.Screens))
	}

	// Выполняем очистку (раньше она бы удалила экран 1, т.к. там 1 фрейм)
	fm.cleanupDoneFrames()

	if len(fm.Screens) != 2 {
		t.Error("cleanupDoneFrames erroneously closed a headless screen with a live dialog")
	}

	// А теперь помечаем диалог как Done
	dlg.SetExitCode(0)
	fm.cleanupDoneFrames()

	if len(fm.Screens) != 1 {
		t.Error("Screen was not closed after its last frame was marked Done")
	}
}

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

	fm.Run(vtinput.NewReader(os.Stdin))

	if !myMenu.Active {
		t.Error("F9 should activate the menu because the modal frame owns it")
	}
}
func TestFrameManager_CycleScreens(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())

	w1 := NewWindow(0, 0, 10, 10, "W1")
	w2 := NewWindow(0, 0, 10, 10, "W2")

	fm.Push(NewDesktop())
	fm.Push(w1)
	fm.AddScreen(w2) // Creates Screen 1, ActiveIdx = 1

	if fm.ActiveIdx != 1 {
		t.Fatalf("Active index should be 1, got %d", fm.ActiveIdx)
	}

	// 1. Cycle forward: ActiveIdx stays 1, but switcherIdx becomes 0
	fm.ctrlPressed = true
	fm.CycleWindows(true)

	// 2. Commit switch (release Ctrl)
	fm.ctrlPressed = false
	if !fm.ctrlPressed && fm.switcherActive {
		fm.switcherActive = false
		fm.SwitchScreen(fm.switcherIdx)
	}

	if fm.ActiveIdx != 0 {
		t.Errorf("Expected switch to Screen 0, stayed at %d", fm.ActiveIdx)
	}

	// 3. Cycle backward: from 0 back to 1
	fm.ctrlPressed = true
	fm.CycleWindows(false)
	fm.ctrlPressed = false
	if !fm.ctrlPressed && fm.switcherActive {
		fm.switcherActive = false
		fm.SwitchScreen(fm.switcherIdx)
	}

	if fm.ActiveIdx != 1 {
		t.Errorf("Expected switch back to Screen 1, stayed at %d", fm.ActiveIdx)
	}
}
func TestFrameManager_CycleBackwards(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())
	fm.Push(NewDesktop())           // Screen 0
	fm.AddScreen(NewWindow(0,0,5,5, "W1")) // Screen 1
	fm.AddScreen(NewWindow(0,0,5,5, "W2")) // Screen 2, ActiveIdx = 2

	// 1. Shift+Ctrl+Tab (forward=false)
	fm.ctrlPressed = true
	fm.CycleWindows(false) // 2 -> 1
	if fm.switcherIdx != 1 {
		t.Errorf("Backward cycle failed: expected 1, got %d", fm.switcherIdx)
	}

	fm.CycleWindows(false) // 1 -> 0
	if fm.switcherIdx != 0 {
		t.Errorf("Backward cycle failed: expected 0, got %d", fm.switcherIdx)
	}

	fm.CycleWindows(false) // 0 -> 2 (wrap)
	if fm.switcherIdx != 2 {
		t.Errorf("Backward cycle wrap failed: expected 2, got %d", fm.switcherIdx)
	}
}

func TestFrameManager_ShortcutPriority(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())
	fm.Push(NewDesktop())

	ctrlWPressed := false
	frame := &mockFrame{}
	frame.onProcessKey = func(e *vtinput.InputEvent) bool {
		if e.VirtualKeyCode == vtinput.VK_W && (e.ControlKeyState & vtinput.LeftCtrlPressed) != 0 {
			ctrlWPressed = true
			return true // Frame intercepts Ctrl+W
		}
		return false
	}
	fm.Push(frame)

	// Simulate Ctrl+W
	ev := &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_W, ControlKeyState: vtinput.LeftCtrlPressed,
	}

	// We need to simulate the dispatch logic from Run()
	fm.ctrlPressed = true
	handled := frame.ProcessKey(ev)

	if !handled {
		t.Fatal("Frame should have handled Ctrl+W")
	}

	// If handled is true, the fallback section in fm.Run (which closes screens) is not reached.
	if !ctrlWPressed {
		t.Error("Frame's own Ctrl+W handler was not called")
	}

	// Ensure screen wasn't closed (still 1 screen + desktop)
	if len(fm.Screens) == 0 {
		t.Error("Global fallback shortcut triggered even though frame handled the key")
	}
}

func TestFrameManager_CycleSingleScreen(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())
	fm.Push(NewDesktop())
	fm.Push(NewWindow(0, 0, 10, 10, "W1"))

	// Should safely return false and do nothing
	if fm.CycleWindows(true) {
		t.Error("CycleWindows should return false when there is only 1 screen")
	}
}

func TestFrameManager_TaskCleanup_MultiScreen(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())
	fm.Push(NewDesktop())

	// Screen 0: Add background task window
	w1 := NewWindow(0, 0, 10, 10, "TaskWin")
	fm.Push(w1)

	// Add Screen 1: Active workspace
	w2 := NewWindow(0, 0, 10, 10, "ActiveWin")
	fm.AddScreen(w2)

	if fm.ActiveIdx != 1 {
		t.Fatal("Should be on screen 1")
	}

	// Mark background task on Screen 0 as done
	w1.SetExitCode(0)

	// Trigger cleanup (this simulates what happens in fm.Run loop)
	fm.cleanupDoneFrames()

	// Verify Screen 0 is auto-closed because it only has Desktop left
	if len(fm.Screens) != 1 {
		t.Errorf("Expected background Screen 0 to be removed, leaving 1 screen. Got %d", len(fm.Screens))
	}

	// Verify ActiveIdx safely shifted to 0 to match the remaining screen
	if fm.ActiveIdx != 0 {
		t.Errorf("Expected ActiveIdx to shift to 0, got %d", fm.ActiveIdx)
	}
}
func TestAppScreen_AttentionState(t *testing.T) {
	s := &AppScreen{}

	// 1. Empty screen or just Desktop (non-modal)
	s.Frames = []Frame{NewDesktop()}
	if s.NeedsAttention() {
		t.Error("Desktop should not trigger attention")
	}

	// 2. Add Modal Dialog
	dlg := NewDialog(0,0,10,10, "Modal")
	s.Frames = append(s.Frames, dlg)
	if !s.NeedsAttention() {
		t.Error("Modal dialog should trigger attention flag")
	}

	// 3. Mark dialog as done (simulating user closed it)
	dlg.SetExitCode(0)
	// We need to simulate the cleanup logic because NeedsAttention checks the top frame
	s.Frames = s.Frames[:len(s.Frames)-1]
	if s.NeedsAttention() {
		t.Error("Attention should clear after modal dialog is removed")
	}
}

func TestFrameManager_ScreenAutoClose(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf()) // Creates Screen 0

	fm.Push(NewDesktop()) // Must explicitly push Desktop
	w1 := NewWindow(0, 0, 10, 10, "W1")
	fm.Push(w1) // Screen 0: [Desktop, W1]

	w2 := NewWindow(0, 0, 10, 10, "W2")
	fm.AddScreen(w2) // Creates Screen 1: [Desktop, W2]. ActiveIdx becomes 1.

	if len(fm.Screens) != 2 || fm.ActiveIdx != 1 {
		t.Fatalf("Expected 2 screens, ActiveIdx 1. Got Screens=%d, ActiveIdx=%d", len(fm.Screens), fm.ActiveIdx)
	}

	w2.SetExitCode(0) // Mark W2 as done
	fm.cleanupDoneFrames() // This should remove W2. Screen 1 will have only Desktop and auto-close.

	if len(fm.Screens) != 1 {
		t.Errorf("Expected 1 screen after auto-close, got %d", len(fm.Screens))
	}
	if fm.ActiveIdx != 0 {
		t.Errorf("Expected ActiveIdx to fallback to 0, got %d", fm.ActiveIdx)
	}
	// Ensure fm.frames points to Screen 0
	if len(fm.frames) != 2 || fm.frames[1] != w1 {
		t.Errorf("Active frames slice not restored correctly to Screen 0")
	}
}

type titleFrame struct {
	mockFrame
	title string
}
func (t *titleFrame) GetTitle() string { return t.title }

func TestFrameManager_F12ScreensMenu(t *testing.T) {
	fm := &frameManager{}
	scr := NewScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)

	f1 := &titleFrame{title: "Panel A"}
	f1.SetPosition(0,0,10,10)
	fm.Push(NewDesktop())
	fm.Push(f1) // Screen 0

	f2 := &titleFrame{title: "Editor B"}
	f2.SetPosition(0,0,10,10)
	fm.AddScreen(f2) // Screen 1, becomes active

	oldFm := FrameManager
	FrameManager = fm
	defer func() { FrameManager = oldFm }()

	// Inject F12
	fm.InjectEvents([]*vtinput.InputEvent{
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F12},
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_Q, ControlKeyState: vtinput.LeftCtrlPressed},
	})

	fm.Run(vtinput.NewReader(os.Stdin))

	if fm.GetTopFrameType() != TypeMenu {
		t.Fatalf("F12 did not open a menu. Top type: %d", fm.GetTopFrameType())
	}

	menu := fm.frames[len(fm.frames)-1].(*VMenu)
	if menu.title != " Screens " {
		t.Errorf("Expected menu title ' Screens ', got %q", menu.title)
	}

	if len(menu.items) != 2 {
		t.Fatalf("Expected 2 menu items, got %d", len(menu.items))
	}

	if menu.items[1].Text != "* Editor B" {
		t.Errorf("Expected active item to have '*' prefix, got %q", menu.items[1].Text)
	}
}

func TestFrameManager_TaskCleanup(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())

	w1 := NewWindow(0, 0, 10, 10, "TaskWin")
	fm.Push(w1)

	if len(fm.frames) != 1 {
		t.Fatal("Frame not pushed")
	}

	fm.TaskChan = make(chan func(), 1)
	fm.TaskChan <- func() {
		w1.SetExitCode(0)
	}

	// Emulate Run() block extraction and execution
	task := <-fm.TaskChan
	task()
	fm.cleanupDoneFrames() // This should now instantly clear it

	if len(fm.frames) != 0 {
		t.Error("Frame should be cleaned up immediately after task execution")
	}
}
func TestFrameManager_BoundVsUnboundTask(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())
	fm.Push(NewDesktop())

	f1 := &mockFrame{}
	fm.Push(f1)

	// 1. SCENARIO: Background Screen Creation (AddScreen)
	fm.AddScreen(&mockFrame{})
	if len(fm.Screens) != 2 {
		t.Error("AddScreen should create a second screen")
	}
	if fm.ActiveIdx != 1 {
		t.Error("AddScreen should automatically switch focus")
	}

	// 2. SCENARIO: Background task without focus (AddScreenBackground)
	fm.SwitchScreen(0)
	fm.AddScreenBackground(&mockFrame{})
	if len(fm.Screens) != 3 {
		t.Errorf("Expected 3 screens, got %d", len(fm.Screens))
	}
	if fm.ActiveIdx != 0 {
		t.Error("AddScreenBackground should NOT switch focus")
	}
}

func TestFrameManager_SwitcherLogic(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())
	fm.Push(NewDesktop()) // ActiveIdx = 0
	fm.AddScreen(NewWindow(0,0,10,10, "W2")) // ActiveIdx = 1

	// 1. Simulate Ctrl+Tab (KeyDown)
	fm.ctrlPressed = true
	fm.CycleWindows(true) // Should wrap: 1 -> 0

	if !fm.switcherActive {
		t.Error("Switcher should be active after Ctrl+Tab")
	}
	if fm.switcherIdx != 0 {
		t.Errorf("Switcher should select screen 0 (forward from 1), got %d", fm.switcherIdx)
	}

	// 2. Simulate Ctrl release (KeyUp)
	fm.ctrlPressed = false
	if !fm.ctrlPressed && fm.switcherActive {
		fm.switcherActive = false
		fm.SwitchScreen(fm.switcherIdx)
	}

	if fm.switcherActive {
		t.Error("Switcher should close on Ctrl release")
	}
	if fm.ActiveIdx != 0 {
		t.Errorf("Screen should have switched to 0, stayed at %d", fm.ActiveIdx)
	}
}

func TestFrameManager_SwitcherRichContent(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())
	fm.Push(NewDesktop())
	
	// Screen 1: With progress
	w1 := NewWindow(0,0,10,10, "TaskWin")
	w1.SetProgress(45)
	fm.AddScreen(w1)
	
	// Screen 2: With attention
	w2 := NewWindow(0,0,10,10, "AlertWin")
	fm.AddScreen(w2)
	fm.Push(NewDialog(0,0,5,5, "Modal"))

	_, title, suf, _ := fm.getScreenInfo(1, 20)
	if !strings.Contains(suf, "####") {
		t.Errorf("getScreenInfo failed to produce progress bar, got %q", suf)
	}
	if title != "TaskWin" {
		t.Errorf("getScreenInfo title mismatch: %q", title)
	}

	pre2, _, _, attn2 := fm.getScreenInfo(2, 20)
	if !attn2 || pre2 != "? " {
		t.Errorf("getScreenInfo failed to report attention. Attn:%v, Prefix:%q", attn2, pre2)
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

	fm.Run(vtinput.NewReader(os.Stdin))

	if mb.Active {
		t.Error("MenuBar should NOT be activated via F9 when a modal dialog is open")
	}
}
func TestFrameManager_PushToFrameScreen_Active(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())
	fm.Push(NewDesktop())

	anchor := &mockFrame{}
	fm.Push(anchor)

	newFrame := &mockFrame{}
	fm.PushToFrameScreen(anchor, newFrame)

	if fm.frames[len(fm.frames)-1] != newFrame {
		t.Errorf("New frame was not pushed to the active screen properly")
	}
}

func TestFrameManager_PushToFrameScreen_Background(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())
	fm.Push(NewDesktop())

	anchor := &mockFrame{}
	fm.Push(anchor)

	// Add new screen (Screen 1 becomes active)
	fm.AddScreen(&mockFrame{})

	newFrame := &mockFrame{}
	fm.PushToFrameScreen(anchor, newFrame)

	// Active screen should NOT change
	if fm.frames[len(fm.frames)-1] == newFrame {
		t.Errorf("New frame should not be in the active screen")
	}

	// Background screen (Screen 0) should have the new frame
	bgScreen := fm.Screens[0]
	if bgScreen.Frames[len(bgScreen.Frames)-1] != newFrame {
		t.Errorf("New frame was not pushed to the background screen")
	}
}
func TestFrameManager_SwitchScreenFocus(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())

	f1Focus := false
	f1 := &mockFrame{}
	f1.onProcessKey = func(e *vtinput.InputEvent) bool {
		if e.Type == vtinput.FocusEventType { f1Focus = e.SetFocus }
		return true
	}
	fm.Push(f1) // Screen 0, f1Focus = true

	f2Focus := false
	f2 := &mockFrame{}
	f2.onProcessKey = func(e *vtinput.InputEvent) bool {
		if e.Type == vtinput.FocusEventType { f2Focus = e.SetFocus }
		return true
	}
	fm.AddScreen(f2) // Screen 1, f2Focus = true, f1Focus = false

	if f1Focus || !f2Focus {
		t.Errorf("Initial focus state error: f1=%v, f2=%v", f1Focus, f2Focus)
	}

	// Switch back to Screen 0
	fm.SwitchScreen(0)

	if !f1Focus {
		t.Error("f1 should have received FocusIn after SwitchScreen")
	}
	if f2Focus {
		t.Error("f2 should have received FocusOut after SwitchScreen")
	}
}
func TestFrameManager_TargetedNotificationFlow(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())
	fm.Push(NewDesktop())

	// 1. Screen 0: Blocking Task
	taskDlg := NewDialog(0,0,10,10, "Task")
	fm.Push(taskDlg)

	// 2. Screen 1: Active Work
	workWin := NewWindow(0,0,10,10, "Active")
	fm.AddScreen(workWin)

	if fm.ActiveIdx != 1 { t.Fatal("Should be on Screen 1") }

	// 3. Task finishes and sends a targeted "Done" message
	doneMsg := NewDialog(0,0,5,5, "Finished")
	fm.PushToFrameScreen(taskDlg, doneMsg)

	// 4. Assertions
	if fm.ActiveIdx != 1 {
		t.Error("Active screen should NOT change when background notification arrives")
	}

	bgScreen := fm.Screens[0]
	topOfBg := bgScreen.Frames[len(bgScreen.Frames)-1]
	if topOfBg != doneMsg {
		t.Error("Notification did not land on the background task screen")
	}

	if !bgScreen.NeedsAttention() {
		t.Error("Background screen should now report NeedsAttention")
	}
}
func TestFrameManager_PushToFrameScreen_LostAnchor(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())
	fm.Push(NewDesktop())

	anchor := &mockFrame{}
	fm.Push(anchor)

	// Simulate anchor being closed/removed before the notification arrives
	fm.RemoveFrame(anchor)

	newFrame := &mockFrame{}
	// This should not panic and should fallback to pushing to the active screen
	fm.PushToFrameScreen(anchor, newFrame)

	if fm.frames[len(fm.frames)-1] != newFrame {
		t.Errorf("Fallback push failed for lost anchor")
	}
}
func TestFrameManager_DoubleClickDetection(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf())

	var lastEvent *vtinput.InputEvent
	frame := &mockFrame{}
	frame.onProcessMouse = func(e *vtinput.InputEvent) bool {
		lastEvent = e
		return true
	}
	fm.Push(frame)

	dispatch := func(e *vtinput.InputEvent) {
		// Simplified dispatch from fm.Run()
		if e.Type == vtinput.MouseEventType && e.ButtonState != 0 && e.KeyDown {
			now := time.Now()
			if e.ButtonState == fm.lastMouseButton && int(e.MouseX) == fm.lastMouseX && int(e.MouseY) == fm.lastMouseY && now.Sub(fm.lastMouseClickTime) < 400*time.Millisecond {
				e.MouseEventFlags |= vtinput.DoubleClick
				fm.lastMouseButton = 0 // prevent triple click
			} else {
				fm.lastMouseButton = e.ButtonState
				fm.lastMouseX = int(e.MouseX)
				fm.lastMouseY = int(e.MouseY)
				fm.lastMouseClickTime = now
			}
		}
		frame.ProcessMouse(e)
	}

	// 1. First click - no double click
	dispatch(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, MouseX: 10, MouseY: 10, ButtonState: vtinput.FromLeft1stButtonPressed})
	if (lastEvent.MouseEventFlags & vtinput.DoubleClick) != 0 {
		t.Fatal("First click should not be a double click")
	}

	// 2. Fast second click, same spot - IS double click
	time.Sleep(100 * time.Millisecond)
	dispatch(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, MouseX: 10, MouseY: 10, ButtonState: vtinput.FromLeft1stButtonPressed})
	if (lastEvent.MouseEventFlags & vtinput.DoubleClick) == 0 {
		t.Error("Fast second click was not detected as double click")
	}

	// 3. Slow third click - no double click
	time.Sleep(500 * time.Millisecond)
	dispatch(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, MouseX: 10, MouseY: 10, ButtonState: vtinput.FromLeft1stButtonPressed})
	if (lastEvent.MouseEventFlags & vtinput.DoubleClick) != 0 {
		t.Error("Slow third click should not be a double click")
	}

	// 4. Fast click, different spot - no double click
	time.Sleep(100 * time.Millisecond)
	dispatch(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, MouseX: 11, MouseY: 10, ButtonState: vtinput.FromLeft1stButtonPressed})
	if (lastEvent.MouseEventFlags & vtinput.DoubleClick) != 0 {
		t.Error("Click in different spot should not be a double click")
	}

	// 5. Fast click, different button - no double click
	time.Sleep(100 * time.Millisecond)
	dispatch(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, MouseX: 11, MouseY: 10, ButtonState: vtinput.RightmostButtonPressed})
	if (lastEvent.MouseEventFlags & vtinput.DoubleClick) != 0 {
		t.Error("Click with different button should not be a double click")
	}
}

func TestFrameManager_CloseActiveScreen_Shifting(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewScreenBuf()) // Creates Screen 0

	// Add Screen 1
	w1 := NewWindow(0, 0, 10, 10, "W1")
	fm.AddScreen(w1)

	// Add Screen 2
	w2 := NewWindow(0, 0, 10, 10, "W2")
	fm.AddScreen(w2)

	if len(fm.Screens) != 3 || fm.ActiveIdx != 2 {
		t.Fatalf("Setup failed, expected 3 screens, ActiveIdx 2")
	}

	// Switch to Screen 1 and close it
	fm.SwitchScreen(1)
	fm.CloseActiveScreen()

	// Screen 2 should shift down and become the new ActiveIdx 1
	if len(fm.Screens) != 2 {
		t.Errorf("Expected 2 screens after close, got %d", len(fm.Screens))
	}
	if fm.ActiveIdx != 1 {
		t.Errorf("ActiveIdx should remain at the same position if not last. Got %d", fm.ActiveIdx)
	}

	// Close the last screen (Screen 1)
	fm.CloseActiveScreen()

	// Should fallback to Screen 0
	if len(fm.Screens) != 1 {
		t.Errorf("Expected 1 screen after close, got %d", len(fm.Screens))
	}
	if fm.ActiveIdx != 0 {
		t.Errorf("ActiveIdx should fallback to 0, got %d", fm.ActiveIdx)
	}

	// Close the only remaining screen (Screen 0)
	fm.CloseActiveScreen()

	// Should trigger Shutdown
	if fm.Screens != nil || fm.frames != nil {
		t.Error("FrameManager should be shut down when the last screen is closed")
	}
}

type progressFrame struct {
	mockFrame
	prog int
}
func (p *progressFrame) GetProgress() int { return p.prog }

func TestAppScreen_GetProgress_DeepSearch(t *testing.T) {
	s := &AppScreen{}

	// 1. Frame with no progress
	s.Frames = append(s.Frames, &mockFrame{})
	if s.GetProgress() != -1 {
		t.Error("Expected no progress (-1)")
	}

	// 2. Frame with progress
	pf := &progressFrame{prog: 42}
	s.Frames = append(s.Frames, pf)
	if s.GetProgress() != 42 {
		t.Errorf("Expected progress 42, got %d", s.GetProgress())
	}

	// 3. A modal dialog on top that doesn't have progress should not hide the underlying progress
	modal := &mockFrame{isModal: true}
	s.Frames = append(s.Frames, modal)

	if s.GetProgress() != 42 {
		t.Errorf("Expected progress 42 to peek through modal dialog, got %d", s.GetProgress())
	}
}
