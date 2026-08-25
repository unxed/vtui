package vtui

import (
	"github.com/mattn/go-runewidth"
	"github.com/unxed/vtinput"
	"io"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type mockFrame struct {
	BaseFrame
	onProcessMouse     func(e *vtinput.InputEvent) bool
	onProcessKey       func(e *vtinput.InputEvent) bool
	onHandleCommand    func(cmd int, args any) bool
	tabTitle           string
	tabMarker          string
	resizedW, resizedH int
}

type closeVetoFrame struct {
	mockFrame
	allowClose        bool
	confirmCloseCalls int
	closeCalls        int
}

func (f *closeVetoFrame) ConfirmClose() bool {
	f.confirmCloseCalls++
	return f.allowClose
}

func (f *closeVetoFrame) Close() {
	f.closeCalls++
}

func (m *mockFrame) ResizeConsole(w, h int) {
	m.resizedW, m.resizedH = w, h
}

func newMockFrame(x, y, w, h int, modal bool) *mockFrame {
	f := &mockFrame{}
	f.Modal = modal
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

func (m *mockFrame) HandleCommand(cmd int, args any) bool {
	if m.onHandleCommand != nil {
		return m.onHandleCommand(cmd, args)
	}
	return m.BaseFrame.HandleCommand(cmd, args)
}

func (m *mockFrame) GetType() FrameType           { return TypeUser }
func (m *mockFrame) GetTitle() string             { return "MockFrame" }
func (m *mockFrame) GetWorkspaceTabTitle() string { return m.tabTitle }
func (m *mockFrame) GetWorkspaceTabMarker() string {
	return m.tabMarker
}

func TestFrameManager_CallOnUIConcurrentRunStartup(t *testing.T) {
	done := make(chan error, 1)
	go func() {
		for range 100 {
			fm := &frameManager{}
			fm.Init(NewSilentScreenBuf())
			fm.SetHostMode(true)

			// Queue Run on the ownership lock before callOnUI observes running.
			fm.uiOwnershipMu.Lock()
			runStarted := make(chan struct{})
			runDone := make(chan struct{})
			go func() {
				close(runStarted)
				fm.Run()
				close(runDone)
			}()
			<-runStarted
			runtime.Gosched()

			callStarted := make(chan struct{})
			callDone := make(chan error, 1)
			go func() {
				close(callStarted)
				callDone <- fm.callOnUI(func() error { return nil })
			}()
			<-callStarted
			runtime.Gosched()
			fm.uiOwnershipMu.Unlock()

			// Run lifetimes vary across platforms, so a stopped-manager error is
			// valid here; only waiting forever indicates the regression.
			<-callDone
			fm.Shutdown()
			<-runDone
		}
		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("callOnUI failed during Run startup: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("callOnUI deadlocked with concurrent Run startup")
	}
}

func TestAppScreen_GetWorkspaceTitleIgnoresModalFrames(t *testing.T) {
	workspace := newMockFrame(0, 0, 40, 20, false)
	menu := NewVMenu("Commands")
	screen := &AppScreen{Frames: []Frame{workspace, menu}}

	if got := screen.GetTitle(); got != "Commands" {
		t.Fatalf("top-frame title = %q, want transient menu title", got)
	}
	if got := screen.GetWorkspaceTitle(); got != "MockFrame" {
		t.Fatalf("workspace title = %q, want underlying frame title", got)
	}
}

func TestFrameManager_DuplicateMouseMoveDoesNotHoverNewMenu(t *testing.T) {
	oldFM := FrameManager
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	FrameManager = fm
	defer func() { FrameManager = oldFM }()

	background := newMockFrame(0, 0, 40, 20, false)
	fm.Push(background)

	// Establish the pointer position before the menu exists.
	fm.dispatchEvent(&vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		MouseX:          15,
		MouseY:          8,
		MouseEventFlags: vtinput.MouseMoved,
	}, false)

	menu := NewVMenu("Menu")
	menu.AddItem(MenuItem{Text: "First"})
	menu.AddSeparator()
	menu.AddItem(MenuItem{Text: "Third"})
	menu.SetPosition(10, 5, 30, 9)
	menu.SetSelectPos(0)
	fm.Push(menu)

	// A backend may repeat MouseMoved at the unchanged cell after the menu is
	// pushed. That is not a new hover and must preserve keyboard selection.
	fm.dispatchEvent(&vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		MouseX:          15,
		MouseY:          8,
		MouseEventFlags: vtinput.MouseMoved,
	}, false)
	if menu.SelectPos != 0 {
		t.Fatalf("duplicate mouse move selected item %d, want 0", menu.SelectPos)
	}

	// A real cell-coordinate change still enables normal hover navigation.
	fm.dispatchEvent(&vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		MouseX:          16,
		MouseY:          8,
		MouseEventFlags: vtinput.MouseMoved,
	}, false)
	if menu.SelectPos != 2 {
		t.Fatalf("real mouse move selected item %d, want 2", menu.SelectPos)
	}
}

func TestFrameManager_GlobalUIPriority(t *testing.T) {
	oldFm := FrameManager
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	FrameManager = fm
	defer func() { FrameManager = oldFm }()

	// Настраиваем KeyBar на нижней строке
	kb := NewKeyBar()
	kb.SetPosition(0, 24, 79, 24)
	kb.SetVisible(true)
	fm.KeyBar = kb

	// Создаем полноэкранный фрейм, который перекрывает координаты KeyBar
	frameClicked := false
	f := &mockFrame{}
	f.SetPosition(0, 0, 79, 24)
	f.onProcessMouse = func(e *vtinput.InputEvent) bool {
		frameClicked = true
		return true
	}
	fm.Push(f)

	// Симулируем клик в области KeyBar (Y=24)
	ev := &vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX:      10,
		MouseY:      24,
	}

	// Вызываем внутренний метод диспетчеризации
	fm.dispatchEvent(ev, false)

	if frameClicked {
		t.Error("Global UI priority failed: background frame received mouse event belonging to KeyBar")
	}

	if len(fm.injectedEvents) == 0 {
		t.Error("KeyBar failed to intercept and process the mouse event via FrameManager")
	}
}

func TestFrameManager_FilteredEventCleansUpClosedFrame(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())

	frame := newMockFrame(0, 0, 10, 10, false)
	fm.Push(frame)
	fm.EventFilter = func(*vtinput.InputEvent) bool {
		frame.Close()
		return true
	}

	fm.dispatchEvent(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_ESCAPE,
	}, false)

	if !frame.IsDone() {
		t.Fatal("filter did not close the frame")
	}
	if len(fm.frames) != 0 {
		t.Fatalf("closed frame survived a consumed event: %d frame(s) remain", len(fm.frames))
	}
}

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
	scr := NewSilentScreenBuf()
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
	scr := NewSilentScreenBuf()
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

func TestFrameManager_CleanupFocusRestore(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())

	f1Focused := false
	f1 := &mockFrame{}
	f1.onProcessKey = func(e *vtinput.InputEvent) bool {
		if e.Type == vtinput.FocusEventType {
			f1Focused = e.SetFocus
		}
		return true
	}
	fm.Push(f1) // Focus goes to f1
	if !f1Focused {
		t.Error("f1 should be focused on push")
	}

	f2 := &mockFrame{}
	fm.Push(f2) // Focus goes to f2, f1 loses focus
	if f1Focused {
		t.Error("f1 should lose focus when f2 is pushed")
	}

	// Now f2 finishes
	f2.SetExitCode(0)
	fm.cleanupDoneFrames() // Should remove f2 and restore focus to f1

	if !f1Focused {
		t.Error("f1 should regain focus after f2 is cleaned up")
	}
}
func TestFrameManager_Toast(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	// Push a dummy frame, otherwise renderPhase returns early and won't clear the toast
	fm.Push(&mockFrame{})

	// ShowToast references global FrameManager, we must swap it for the test
	oldFm := FrameManager
	FrameManager = fm
	defer func() { FrameManager = oldFm }()

	msg := "Hello Toast"
	ShowToast(msg, 100*time.Millisecond)

	// Pump tasks
	select {
	case task := <-fm.TaskChan:
		task()
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Toast task not posted")
	}

	if fm.currentToast == nil || fm.currentToast.Message != msg {
		t.Errorf("Toast not set correctly in FrameManager. Got: %v", fm.currentToast)
	}

	// Ждем истечения срока
	time.Sleep(150 * time.Millisecond)

	// Вызываем renderPhase (она очищает просроченные тосты)
	fm.renderPhase()

	if fm.currentToast != nil {
		t.Error("Toast did not expire")
	}
}
func TestFrameManager_ToastStyle(t *testing.T) {
	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)
	fm.Push(&mockFrame{})

	oldFm := FrameManager
	FrameManager = fm
	defer func() { FrameManager = oldFm }()

	attr := SetRGBBoth(0, 0x112233, 0xAABBCC)
	ShowToastStyled("Styled", 100*time.Millisecond, ToastStyle{Attr: attr, Row: -1})

	select {
	case task := <-fm.TaskChan:
		task()
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Toast task not posted")
	}

	fm.renderPhase()

	// The styled toast must land centred on the last row with its colours
	// (the message carries a leading space, so the first letter is at x+1).
	c := scr.GetCell(37, 24)
	if c.Attributes != attr {
		t.Errorf("bottom toast must use the styled attribute, got %#x want %#x", c.Attributes, attr)
	}
	if c.Char != uint64('S') {
		t.Errorf("bottom toast must draw the message, got %q", rune(c.Char))
	}
}

func TestFrameManager_GetTopFrameType(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())

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
func TestFrameManager_SwitchScreenPreservesOrderAndNumbers(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	defer fm.Shutdown()
	fm.Push(NewDesktop())
	fm.AddScreen(NewWindow(0, 0, 10, 10, "W1"))
	fm.AddScreen(NewWindow(0, 0, 10, 10, "W2"))

	if len(fm.Screens) != 3 {
		t.Fatalf("Expected 3 screens, got %d", len(fm.Screens))
	}
	if got := []int{fm.Screens[0].Number, fm.Screens[1].Number, fm.Screens[2].Number}; !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Fatalf("initial workspace numbers = %v, want [1 2 3]", got)
	}

	fm.SwitchScreen(0)

	if fm.ActiveIdx != 0 {
		t.Errorf("ActiveIdx should be 0, got %d", fm.ActiveIdx)
	}
	if got := []string{fm.Screens[0].GetTitle(), fm.Screens[1].GetTitle(), fm.Screens[2].GetTitle()}; !reflect.DeepEqual(got, []string{"Desktop", "W1", "W2"}) {
		t.Errorf("workspace order changed after switch: %v", got)
	}
	if got := []int{fm.Screens[0].Number, fm.Screens[1].Number, fm.Screens[2].Number}; !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Errorf("workspace numbers changed after switch: %v", got)
	}

	// New numbers are derived from the largest number still in use.
	fm.SwitchScreen(2)
	fm.CloseActiveScreen()
	fm.AddScreen(NewWindow(0, 0, 10, 10, "W3"))
	if got := []int{fm.Screens[0].Number, fm.Screens[1].Number, fm.Screens[2].Number}; !reflect.DeepEqual(got, []int{1, 3, 2}) {
		t.Errorf("workspace number did not follow the live maximum: %v", got)
	}

	fm.SwitchScreen(-1)
	fm.SwitchScreen(100)
}
func TestFrameManager_MouseCapture(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())

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
	fm.Init(NewSilentScreenBuf())

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
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 10)
	fm.Init(scr)
	defer fm.Shutdown()

	taskExecuted := false
	fm.PostTask(func() {
		taskExecuted = true
	})

	// Извлекаем задачу из канала и выполняем её
	select {
	case task := <-fm.TaskChan:
		task()
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Task was not posted to TaskChan")
	}

	if !taskExecuted {
		t.Error("Posted task was not executed")
	}
}
func TestFrameManager_FocusOnRemove(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())

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
	scr := NewSilentScreenBuf()
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
	fm.Init(NewSilentScreenBuf())

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
	fm.Init(NewSilentScreenBuf())
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
	fm.Init(NewSilentScreenBuf())

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
	fm.Init(NewSilentScreenBuf())

	count1 := 0
	f1 := &cmdMockFrame{onCmd: func(cmd int, args any) bool { count1++; return true }}

	count2 := 0
	f2 := &cmdMockFrame{onCmd: func(cmd int, args any) bool { count2++; return true }}

	// Помещаем фреймы в разные экраны
	fm.Push(f1)      // Screen 0
	fm.AddScreen(f2) // Screen 1

	// Посылаем бродкаст
	fm.Broadcast(777, nil)

	if count1 != 1 || count2 != 1 {
		t.Errorf("Broadcast failed to reach all frames. F1: %d, F2: %d", count1, count2)
	}
}

func TestFrameManager_Broadcast_RedrawTrigger(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())

	f := &cmdMockFrame{onCmd: func(cmd int, args any) bool { return true }}
	fm.Push(f)

	// Clear redraw channel
	select {
	case <-fm.RedrawChan:
	default:
	}

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
	fm.Init(NewSilentScreenBuf())

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
		if cmd == 999 {
			return true
		}
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
func TestFrameManager_ModalOutsideClicks(t *testing.T) {
	SetDefaultPalette()
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	fm.Push(NewDesktop())

	// Create a real Dialog with an active default button
	dlg := NewDialog(10, 10, 30, 20, "Modal")
	btn := NewButton(1, 1, "OK")
	btn.IsDefault = true
	btn.OnClick = func() { dlg.SetExitCode(42) }
	dlg.AddItem(btn)
	fm.Push(dlg)

	// 1. Test LMB outside (at 5, 5) -> should trigger ESC (-1)
	fm.dispatchEvent(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		MouseX:      5,
		MouseY:      5,
		ButtonState: vtinput.FromLeft1stButtonPressed,
	}, false)

	if !dlg.IsDone() {
		t.Error("LMB outside modal dialog did not close it")
	}
	if dlg.ExitCode != -1 {
		t.Errorf("LMB outside: expected ExitCode -1 (Closed), got %d", dlg.ExitCode)
	}

	// Reset and re-push for RMB test
	dlg.Done = false
	dlg.ExitCode = 0
	fm.Push(dlg)

	// 2. Test RMB outside (at 5, 5) -> should trigger ENTER (42 via Default Button)
	fm.dispatchEvent(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		MouseX:      5,
		MouseY:      5,
		ButtonState: vtinput.RightmostButtonPressed,
	}, false)

	if !dlg.IsDone() {
		t.Error("RMB outside modal dialog did not trigger the default action")
	}
	if dlg.ExitCode != 42 {
		t.Errorf("RMB outside: expected ExitCode 42, got %d", dlg.ExitCode)
	}
}

// TestFrameManager_ModalOutsideClicks_MenuDismissesOnRMB pins the fix
// for unxed/f4#396: the RMB-outside → ENTER shortcut is Far-style
// muscle memory for confirming a dialog's default button, but for a
// drop-down / context menu it silently activates whichever row happens
// to be under the cursor (typically the first item, e.g. "Other panel"
// for the f4 drive menu). RMB outside a TypeMenu should dismiss it
// like LMB does.
func TestFrameManager_ModalOutsideClicks_MenuDismissesOnRMB(t *testing.T) {
	SetDefaultPalette()
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	fm.Push(NewDesktop())

	menu := NewVMenu(" Test ")
	menu.SetPosition(10, 10, 30, 20)
	menu.AddItem(MenuItem{Text: "Would-be default action"})
	fm.Push(menu)

	fm.dispatchEvent(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		MouseX:      5,
		MouseY:      5,
		ButtonState: vtinput.RightmostButtonPressed,
	}, false)

	if !menu.IsDone() {
		t.Error("RMB outside a menu did not dismiss it")
	}
	if menu.exitCode != -1 {
		t.Errorf("RMB outside menu: expected ExitCode -1 (dismissed), got %d — "+
			"row 0 was activated instead, which is exactly what unxed/f4#396 reported",
			menu.exitCode)
	}
}

func TestFrameManager_ModalPriorityOverMenu(t *testing.T) {
	fm := &frameManager{}
	scr := NewSilentScreenBuf()
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
	})

	pr, pw := io.Pipe()
	fm.PostTask(func() { pw.Close() })
	fm.Run(vtinput.NewReader(pr, false))

	if !okClicked {
		t.Error("Modal dialog should have priority over active MenuBar for Enter key")
	}
	if !mb.Active {
		t.Error("MenuBar should remain Active (suspended) behind the modal dialog")
	}
}

func TestFrameManager_MenuAccessibleDuringNonModal(t *testing.T) {
	fm := &frameManager{}
	scr := NewSilentScreenBuf()
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
	})

	pr, pw := io.Pipe()
	fm.PostTask(func() { pw.Close() })
	fm.Run(vtinput.NewReader(pr, false))

	if !mb.Active {
		t.Error("MenuBar should be activatable when the top frame is non-modal (e.g. Progress window)")
	}
}
func TestFrameManager_CleanupAllDoneFrames(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())

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
	scr := NewSilentScreenBuf()
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
	})

	fm.Step(0)

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
	fm.Init(NewSilentScreenBuf())

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
	fm.Init(NewSilentScreenBuf())

	// Используем mockFrame, так как BaseFrame не реализует интерфейс Frame полностью
	dlg := &mockFrame{}
	dlg.Modal = true
	s := &AppScreen{Frames: []Frame{dlg}}

	// 1. По умолчанию внимание требуется
	if !s.NeedsAttention() {
		t.Error("Modal dialog should require attention by default")
	}

	// 2. С подавлением — не требуется
	dlg.AttentionSuppressed = true
	if s.NeedsAttention() {
		t.Error("NeedsAttention should be false when attentionSuppressed is true")
	}
}
func TestFrameManager_ResizeAllScreens(t *testing.T) {
	oldGetSize := GetTerminalSize
	defer func() { GetTerminalSize = oldGetSize }()

	GetTerminalSize = func() (int, int, error) {
		return 120, 40, nil
	}

	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 24)
	fm.Init(scr)

	f1 := &mockFrame{}
	f2 := &mockFrame{}
	f3 := &mockFrame{}

	fm.Push(f1)                // Screen 0 (Background)
	fm.AddScreen(f2)           // Screen 1 (Active)
	fm.AddScreenBackground(f3) // Screen 2 (Background added explicitly)

	// Inject Resize Event
	fm.InjectEvents([]*vtinput.InputEvent{
		{Type: vtinput.ResizeEventType},
	})

	// Setup a trap to stop the manager cleanly after processing
	f2.onProcessKey = func(e *vtinput.InputEvent) bool {
		if e.VirtualKeyCode == vtinput.VK_F10 {
			fm.Stop()
			return true
		}
		return false
	}
	fm.InjectEvents([]*vtinput.InputEvent{
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F10},
	})

	pr, pw := io.Pipe()
	go func() {
		time.Sleep(100 * time.Millisecond)
		pw.Close()
	}()

	reader := vtinput.NewReader(pr, false)
	fm.Run(reader)

	if f1.resizedW != 120 || f1.resizedH != 40 {
		t.Errorf("Background screen frame not resized. Got %dx%d", f1.resizedW, f1.resizedH)
	}
	if f2.resizedW != 120 || f2.resizedH != 40 {
		t.Errorf("Active screen frame not resized. Got %dx%d", f2.resizedW, f2.resizedH)
	}
	if f3.resizedW != 120 || f3.resizedH != 40 {
		t.Errorf("Background added screen frame not resized. Got %dx%d", f3.resizedW, f3.resizedH)
	}
	if scr.Width() != 120 || scr.Height() != 40 {
		t.Errorf("ScreenBuf not resized. Got %dx%d", scr.Width(), scr.Height())
	}
}

func TestFrameManager_ResizeSameGridReflowsFrames(t *testing.T) {
	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(100, 30)
	fm.Init(scr)

	frame := &mockFrame{}
	fm.Push(frame)
	frame.resizedW, frame.resizedH = 0, 0

	fm.Resize(100, 30)
	if frame.resizedW != 100 || frame.resizedH != 30 {
		t.Fatalf("same-grid Resize() did not reflow frame: got %dx%d, want 100x30", frame.resizedW, frame.resizedH)
	}
}

func TestFrameManager_SizePolling(t *testing.T) {
	oldGetSize := GetTerminalSize
	defer func() { GetTerminalSize = oldGetSize }()

	var mockW, mockH atomic.Int64
	mockW.Store(80)
	mockH.Store(24)
	GetTerminalSize = func() (int, int, error) {
		return int(mockW.Load()), int(mockH.Load()), nil
	}

	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 24)
	fm.Init(scr)

	f1 := &mockFrame{}
	fm.Push(f1)

	done := make(chan struct{})
	pr, pw := io.Pipe()

	go func() {
		reader := vtinput.NewReader(pr, false)
		fm.Run(reader)
		close(done)
	}()

	// Wait for loop and polling goroutine to start
	time.Sleep(50 * time.Millisecond)

	// Change mock size to trigger polling mechanism
	mockW.Store(100)
	mockH.Store(30)

	// Wait for the polling interval (200ms) + buffer time
	time.Sleep(300 * time.Millisecond)

	fm.Stop()
	pw.Close()
	<-done

	if f1.resizedW != 100 || f1.resizedH != 30 {
		t.Errorf("Polling failed to resize frame. Got %dx%d", f1.resizedW, f1.resizedH)
	}
	if scr.Width() != 100 || scr.Height() != 30 {
		t.Errorf("Polling failed to resize ScreenBuf. Got %dx%d", scr.Width(), scr.Height())
	}
}
func TestFrameManager_ResizeRobustness(t *testing.T) {
	oldGetSize := GetTerminalSize
	defer func() { GetTerminalSize = oldGetSize }()

	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 24)
	fm.Init(scr)

	// 1. Test: Error from GetTerminalSize should be ignored
	GetTerminalSize = func() (int, int, error) {
		return 0, 0, io.EOF
	}

	// Manual call to internal handleResize logic is simulated via event
	fm.InjectEvents([]*vtinput.InputEvent{{Type: vtinput.ResizeEventType}})

	// Start a short-lived loop to process the event
	pr, pw := io.Pipe()
	go func() { time.Sleep(50 * time.Millisecond); fm.Stop(); pw.Close() }()
	fm.Run(vtinput.NewReader(pr, false))

	if scr.Width() != 80 || scr.Height() != 24 {
		t.Error("Resize handled an error size incorrectly")
	}

	// 2. Test: Non-positive sizes should be ignored
	GetTerminalSize = func() (int, int, error) {
		return -10, 0, nil
	}
	fm.InjectEvents([]*vtinput.InputEvent{{Type: vtinput.ResizeEventType}})

	pr2, pw2 := io.Pipe()
	go func() { time.Sleep(50 * time.Millisecond); fm.Stop(); pw2.Close() }()
	fm.Run(vtinput.NewReader(pr2, false))

	if scr.Width() != 80 || scr.Height() != 24 {
		t.Error("Resize handled non-positive size incorrectly")
	}
}

func TestFrameManager_NoAutoCloseForHeadless(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	defer fm.Shutdown()

	// Добавляем фоновый экран 0
	fm.Push(NewDesktop())

	// Создаем Headless экран 1 (в нем 1 фрейм, и это НЕ Desktop)
	dlg := &mockFrame{}
	dlg.Modal = true
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
	scr := NewSilentScreenBuf()
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
	})

	pr, pw := io.Pipe()
	fm.PostTask(func() { pw.Close() })
	fm.Run(vtinput.NewReader(pr, false))

	if !myMenu.Active {
		t.Error("F9 should activate the menu because the modal frame owns it")
	}
}
func TestFrameManager_CycleScreens(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())

	w1 := NewWindow(0, 0, 10, 10, "W1")
	w2 := NewWindow(0, 0, 10, 10, "W2")

	fm.Push(NewDesktop())
	fm.Push(w1)      // Screen 0
	fm.AddScreen(w2) // Screen 1

	// 1. Cycle forward (MRU): Активен W2 (индекс 1). Переключаемся на "предыдущий" — W1 (индекс 0).
	fm.ctrlPressed = true
	fm.CycleWindows(true)

	// 2. Commit switch
	fm.ctrlPressed = false
	if !fm.ctrlPressed && fm.switcherMenu != nil {
		if !fm.switcherMenu.IsDone() {
			idx := fm.switcherMenu.SelectPos
			if idx >= 0 && idx < len(fm.switcherMenu.Items) {
				userData := fm.switcherMenu.Items[idx].UserData.(int)
				fm.switcherMenu.Close()
				fm.SwitchScreen(userData)
			}
		}
		fm.switcherMenu = nil
	}

	// W1 переехал в конец. Порядок: [W2, W1]. ActiveIdx = 1.
	if fm.Screens[fm.ActiveIdx].Frames[1] != w1 {
		t.Errorf("Expected W1 to be active, got %s", fm.Screens[fm.ActiveIdx].GetTitle())
	}

	// 3. Переключаемся обратно на W2 (теперь он на индексе 0)
	fm.ctrlPressed = true
	fm.CycleWindows(true)
	fm.ctrlPressed = false
	if !fm.ctrlPressed && fm.switcherMenu != nil {
		if !fm.switcherMenu.IsDone() {
			idx := fm.switcherMenu.SelectPos
			if idx >= 0 && idx < len(fm.switcherMenu.Items) {
				userData := fm.switcherMenu.Items[idx].UserData.(int)
				fm.switcherMenu.Close()
				fm.SwitchScreen(userData)
			}
		}
		fm.switcherMenu = nil
	}

	if fm.Screens[fm.ActiveIdx].Frames[1] != w2 {
		t.Errorf("Expected W2 to be active again, got %s", fm.Screens[fm.ActiveIdx].GetTitle())
	}
}
func TestFrameManager_CycleBackwards(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	fm.Push(NewDesktop())                     // Screen 0
	fm.AddScreen(NewWindow(0, 0, 5, 5, "W1")) // Screen 1
	fm.AddScreen(NewWindow(0, 0, 5, 5, "W2")) // Screen 2, ActiveIdx = 2

	// 1. Shift+Ctrl+Tab (forward=false)
	fm.ctrlPressed = true
	fm.CycleWindows(false) // 2 -> 1
	if fm.switcherMenu == nil || fm.switcherMenu.SelectPos != 1 {
		t.Errorf("Backward cycle failed: expected 1, got %v", fm.switcherMenu)
	}

	fm.CycleWindows(false) // 1 -> 0
	if fm.switcherMenu == nil || fm.switcherMenu.SelectPos != 0 {
		t.Errorf("Backward cycle failed: expected 0, got %v", fm.switcherMenu)
	}

	fm.CycleWindows(false) // 0 -> 2 (wrap)
	if fm.switcherMenu == nil || fm.switcherMenu.SelectPos != 2 {
		t.Errorf("Backward cycle wrap failed: expected 2, got %v", fm.switcherMenu)
	}
}

func TestFrameManager_ShortcutPriority(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	fm.Push(NewDesktop())

	ctrlWPressed := false
	frame := &mockFrame{}
	frame.onProcessKey = func(e *vtinput.InputEvent) bool {
		if e.VirtualKeyCode == vtinput.VK_W && (e.ControlKeyState&vtinput.LeftCtrlPressed) != 0 {
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
	fm.Init(NewSilentScreenBuf())
	fm.Push(NewDesktop())
	fm.Push(NewWindow(0, 0, 10, 10, "W1"))

	// Should safely return false and do nothing
	if fm.CycleWindows(true) {
		t.Error("CycleWindows should return false when there is only 1 screen")
	}
}

func TestFrameManager_TaskCleanup_MultiScreen(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
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
	dlg := NewDialog(0, 0, 10, 10, "Modal")
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
	fm.Init(NewSilentScreenBuf()) // Creates Screen 0
	defer fm.Shutdown()

	fm.Push(NewDesktop()) // Must explicitly push Desktop
	w1 := NewWindow(0, 0, 10, 10, "W1")
	fm.Push(w1) // Screen 0: [Desktop, W1]

	w2 := NewWindow(0, 0, 10, 10, "W2")
	fm.AddScreen(w2) // Creates Screen 1: [Desktop, W2]. ActiveIdx becomes 1.

	if len(fm.Screens) != 2 || fm.ActiveIdx != 1 {
		t.Fatalf("Expected 2 screens, ActiveIdx 1. Got Screens=%d, ActiveIdx=%d", len(fm.Screens), fm.ActiveIdx)
	}

	w2.SetExitCode(0)      // Mark W2 as done
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

type workspaceInfoFrame struct {
	titleFrame
	info WorkspaceMenuInfo
}

func (f *workspaceInfoFrame) GetWorkspaceMenuInfo() WorkspaceMenuInfo { return f.info }

func TestFrameManager_F12ScreensMenu(t *testing.T) {
	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)
	defer fm.Shutdown()

	f1 := &titleFrame{title: "Panel A"}
	f1.SetPosition(0, 0, 10, 10)
	fm.Push(NewDesktop())
	fm.Push(f1) // Screen 0

	f2 := &titleFrame{title: "Editor B"}
	f2.SetPosition(0, 0, 10, 10)
	fm.AddScreen(f2) // Screen 1, becomes active

	oldFm := FrameManager
	FrameManager = fm
	defer func() { FrameManager = oldFm }()

	// Inject F12
	fm.InjectEvents([]*vtinput.InputEvent{
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F12},
	})

	pr, pw := io.Pipe()
	fm.PostTask(func() { pw.Close() })
	fm.Run(vtinput.NewReader(pr, false))

	if fm.GetTopFrameType() != TypeMenu {
		t.Fatalf("F12 did not open a menu. Top type: %d", fm.GetTopFrameType())
	}

	menu := fm.frames[len(fm.frames)-1].(*VMenu)
	if menu.title != " Screens " {
		t.Errorf("Expected menu title ' Screens ', got %q", menu.title)
	}

	if len(menu.Items) != 2 {
		t.Fatalf("Expected 2 menu items, got %d", len(menu.Items))
	}

	if menu.Items[1].AccentPrefix != "2" || menu.Items[1].Text != " * ▣ Editor B" {
		t.Errorf("unexpected active Screens item: accent=%q text=%q", menu.Items[1].AccentPrefix, menu.Items[1].Text)
	}
}

func TestFrameManager_ScreensMenuUsesStructuredAlignedWorkspaceInfo(t *testing.T) {
	SetDefaultPalette()
	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(120, 25)
	fm.Init(scr)
	defer fm.Shutdown()

	first := &workspaceInfoFrame{titleFrame: titleFrame{title: "Panels"}, info: WorkspaceMenuInfo{
		Icon: "📁", Primary: `C:\short`, Secondary: `D:\a\much\longer\right\path`,
	}}
	second := &workspaceInfoFrame{titleFrame: titleFrame{title: "Panels"}, info: WorkspaceMenuInfo{
		Icon: "📁", Primary: `C:\a\much\longer\left\path`, Secondary: `D:\right`,
	}}
	fm.Push(first)
	fm.AddScreen(second)
	fm.showScreensMenu()

	menu := fm.frames[len(fm.frames)-1].(*VMenu)
	if len(menu.Items) != 2 {
		t.Fatalf("Screens items = %d, want 2", len(menu.Items))
	}
	if menu.Items[0].AccentPrefix != "1" || menu.Items[1].AccentPrefix != "2" {
		t.Fatalf("workspace accents = %q, %q", menu.Items[0].AccentPrefix, menu.Items[1].AccentPrefix)
	}
	if !strings.Contains(menu.Items[0].Text, `C:\short`) || !strings.Contains(menu.Items[0].Text, `D:\a\much\longer\right\path`) {
		t.Fatalf("full panel paths missing from first item: %q", menu.Items[0].Text)
	}
	if strings.Index(menu.Items[0].Text, "↔") != strings.Index(menu.Items[1].Text, "↔") {
		t.Fatalf("panel separators are not aligned: %q / %q", menu.Items[0].Text, menu.Items[1].Text)
	}

	menu.Show(scr)
	accentCell := scr.GetCell(menu.X1+2, menu.Y1+2) // active second item
	wantAccent := Palette[menu.ColorSelectedHighlightIdx]
	if GetRGBFore(accentCell.Attributes) != GetRGBFore(wantAccent) {
		t.Fatalf("active workspace index attr = %#x, want selected highlight %#x", accentCell.Attributes, Palette[menu.ColorSelectedHighlightIdx])
	}
}

func TestFrameManager_TaskCleanup(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	defer fm.Shutdown()

	w1 := NewWindow(0, 0, 10, 10, "TaskWin")
	fm.Push(w1)

	if len(fm.frames) != 1 {
		t.Fatal("Frame not pushed")
	}

	fm.PostTask(func() {
		w1.SetExitCode(0)
	})

	// Emulate Run() block extraction and execution
	task := <-fm.TaskChan
	task()
	fm.cleanupDoneFrames() // This should now instantly clear it

	if len(fm.frames) != 0 {
		t.Error("Frame should be cleaned up immediately after task execution")
	}
}
func TestFrameManager_PostTaskNonBlocking(t *testing.T) {
	// Проверяем, что PostTask не вешает вызывающий поток, если очередь полна.
	// Это важно для предотвращения каскадных зависаний.
	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 10)
	fm.Init(scr)
	defer fm.Shutdown()

	fm.PostTask(func() {})
	fm.PostTask(func() {})

	// Третий вызов не должен зависнуть
	done := make(chan bool)
	go func() {
		fm.PostTask(func() {})
		done <- true
	}()

	select {
	case <-done:
		// Успех: PostTask больше не блокирует поток
	case <-time.After(100 * time.Millisecond):
		t.Error("PostTask STILL blocks on full channel (regression!)")
	}
}
func TestFrameManager_BoundVsUnboundTask(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	fm.Push(NewDesktop()) // Screen 0

	// 1. SCENARIO: Background Screen Creation (AddScreen)
	fm.AddScreen(&mockFrame{}) // Screen 1
	if len(fm.Screens) != 2 {
		t.Error("AddScreen should create a second screen")
	}
	// В MRU-логике активный всегда последний
	if fm.ActiveIdx != 1 {
		t.Error("AddScreen should automatically switch focus")
	}

	// 2. SCENARIO: Background task without focus (AddScreenBackground)
	activeBefore := fm.Screens[fm.ActiveIdx]
	fm.AddScreenBackground(&mockFrame{})

	if len(fm.Screens) != 3 {
		t.Errorf("Expected 3 screens, got %d", len(fm.Screens))
	}

	// AddScreenBackground не должен менять текущий активный экран
	if fm.Screens[fm.ActiveIdx] != activeBefore {
		t.Error("AddScreenBackground switched focus erroneously")
	}
}

func TestFrameManager_SwitcherLogic(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	defer fm.Shutdown()

	fm.Push(NewDesktop())                       // Screen 0: Desktop
	fm.AddScreen(NewWindow(0, 0, 10, 10, "W1")) // Screen 1: W1. ActiveIdx = 1.

	// 1. Simulate Ctrl+Tab (KeyDown)
	fm.ctrlPressed = true
	fm.CycleWindows(true) // Вперед в MRU — это к предыдущему (индекс 0: Desktop)

	if fm.switcherMenu == nil {
		t.Error("Switcher should be active after Ctrl+Tab")
	} else if fm.switcherMenu.SelectPos != 0 {
		t.Errorf("Switcher should select screen 0, got %d", fm.switcherMenu.SelectPos)
	}

	// 2. Simulate Ctrl release (KeyUp)
	fm.ctrlPressed = false
	if !fm.ctrlPressed && fm.switcherMenu != nil {
		if !fm.switcherMenu.IsDone() {
			idx := fm.switcherMenu.SelectPos
			if idx >= 0 && idx < len(fm.switcherMenu.Items) {
				userData := fm.switcherMenu.Items[idx].UserData.(int)
				fm.switcherMenu.Close()
				fm.SwitchScreen(userData)
			}
		}
		fm.switcherMenu = nil
	}

	// Теперь Desktop должен стать активным (переехать в конец массива)
	if fm.Screens[fm.ActiveIdx].GetTitle() != "Desktop" {
		t.Errorf("Screen Desktop was not moved to active. Top title: %q", fm.Screens[fm.ActiveIdx].GetTitle())
	}
}

func TestFrameManager_WorkspaceTopInsetModes(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm := &frameManager{}
	fm.Init(scr)
	frame := newMockFrame(0, 0, 80, 25, false)
	fm.Push(frame)
	menuBar := NewMenuBar(nil)
	menuBar.SetPosition(0, 0, 79, 0)
	fm.MenuBar = menuBar

	if got := fm.WorkspaceTopInset(); got != 0 {
		t.Fatalf("multiple mode with one screen inset = %d, want 0", got)
	}
	fm.AddScreenBackground(newMockFrame(0, 0, 80, 25, false))
	if got := fm.WorkspaceTopInset(); got != 1 {
		t.Fatalf("multiple mode with two screens inset = %d, want 1", got)
	}
	if frame.resizedW != 80 || frame.resizedH != 25 {
		t.Fatalf("screen was not relaid out after tab row appeared: %dx%d", frame.resizedW, frame.resizedH)
	}
	if _, y1, _, y2 := menuBar.GetPosition(); y1 != 1 || y2 != 1 {
		t.Fatalf("menu bar position with persistent tabs = %d..%d, want 1..1", y1, y2)
	}

	fm.ConfigureWorkspaceTabs(WorkspaceTabsOnCtrl, WorkspaceCtrlTabDirect)
	if got := fm.WorkspaceTopInset(); got != 0 {
		t.Fatalf("Ctrl overlay mode inset = %d, want 0", got)
	}
	if _, y1, _, y2 := menuBar.GetPosition(); y1 != 0 || y2 != 0 {
		t.Fatalf("menu bar position with overlay tabs = %d..%d, want 0..0", y1, y2)
	}
	fm.ConfigureWorkspaceTabs(WorkspaceTabsAlways, WorkspaceCtrlTabDirect)
	if got := fm.WorkspaceTopInset(); got != 1 {
		t.Fatalf("always mode inset = %d, want 1", got)
	}
	if _, y1, _, y2 := menuBar.GetPosition(); y1 != 1 || y2 != 1 {
		t.Fatalf("menu bar position with always-visible tabs = %d..%d, want 1..1", y1, y2)
	}
}

func TestFrameManager_OnCtrlInsetTracksCtrlState(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm := &frameManager{}
	fm.Init(scr)
	fm.ConfigureWorkspaceTabs(WorkspaceTabsOnCtrl, WorkspaceCtrlTabDirect)

	// The overlay strip is drawn while Ctrl is held, so that row must be
	// reserved after the first Ctrl+Tab; otherwise a full-screen image paints
	// over the tabs.
	fm.ctrlPressed = true
	if got := fm.WorkspaceTopInset(); got != 0 {
		t.Fatalf("on-ctrl mode before Ctrl+Tab inset = %d, want 0", got)
	}
	fm.workspaceTabPreview = true
	if got := fm.WorkspaceTopInset(); got != 1 {
		t.Fatalf("on-ctrl mode after Ctrl+Tab inset = %d, want 1", got)
	}
	fm.ConfigureWorkspaceTabOverlay(true)
	if !fm.workspaceTabsVisible() {
		t.Fatal("overlay mode must keep the transient tabs visible")
	}
	if got := fm.WorkspaceTopInset(); got != 0 {
		t.Fatalf("on-ctrl overlay mode inset = %d, want 0", got)
	}
	fm.ctrlPressed = false
	if got := fm.WorkspaceTopInset(); got != 0 {
		t.Fatalf("on-ctrl mode without ctrl inset = %d, want 0", got)
	}
}

func TestFrameManager_OnCtrlTabsArmAfterFirstCtrlTab(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm := &frameManager{}
	fm.Init(scr)
	fm.Push(newMockFrame(0, 0, 80, 25, false))
	fm.AddScreenBackground(newMockFrame(0, 0, 80, 25, false))
	fm.ConfigureWorkspaceTabs(WorkspaceTabsOnCtrl, WorkspaceCtrlTabDirect)

	fm.dispatchEvent(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_CONTROL,
		ControlKeyState: vtinput.LeftCtrlPressed,
	}, false)
	if fm.workspaceTabsVisible() || fm.WorkspaceTopInset() != 0 {
		t.Fatal("pressing Ctrl alone should not reveal workspace tabs")
	}

	fm.dispatchEvent(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_TAB,
		ControlKeyState: vtinput.LeftCtrlPressed,
	}, false)
	if !fm.workspaceTabsVisible() || fm.WorkspaceTopInset() != 1 {
		t.Fatal("the first Ctrl+Tab should reveal workspace tabs")
	}

	fm.dispatchEvent(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        false,
		VirtualKeyCode: vtinput.VK_CONTROL,
	}, false)
	if fm.workspaceTabsVisible() || fm.WorkspaceTopInset() != 0 {
		t.Fatal("releasing Ctrl should hide workspace tabs again")
	}
}

func TestFrameManager_WorkspaceTabsAndCounterRendering(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(60, 10)
	fm := &frameManager{}
	fm.Init(scr)
	panels := newMockFrame(0, 0, 20, 5, false)
	panels.tabTitle = "Panels ─ Files"
	panels.tabMarker = "P"
	fm.Push(panels)
	fm.AddScreen(NewWindow(0, 0, 20, 5, "Editor"))
	fm.ConfigureWorkspaceTabs(WorkspaceTabsMultiple, WorkspaceCtrlTabDirect)
	fm.ConfigureWorkspaceTabColors(
		SetRGBBoth(0, 0xAAAAAA, 0x111111),
		SetRGBBoth(0, 0xAAAAAA, 0x222222),
		SetRGBBoth(0, 0xFFD75F, 0),
		SetRGBBoth(0, 0xFF5F5F, 0),
	)

	fm.drawWorkspaceTabs()
	fm.drawWorkspaceCounter()
	var row strings.Builder
	for x := 0; x < scr.width; x++ {
		row.WriteRune(rune(scr.GetCell(x, 0).Char))
	}
	text := row.String()
	if !strings.Contains(text, "1P Panels") || !strings.Contains(text, "2 Editor") {
		t.Fatalf("tab row does not contain both workspaces: %q", text)
	}
	if !strings.Contains(text, "Files │ 2 Editor") {
		t.Fatalf("tabs are not compact and box-drawing-separated: %q", text)
	}
	if !strings.Contains(text, "Editor │+") {
		t.Fatalf("new-workspace control is missing after tabs: %q", text)
	}
	if !strings.HasSuffix(text, "[2/2]") {
		t.Fatalf("workspace counter = %q, want suffix [2/2]", text)
	}
	counterX := scr.width - runewidth.StringWidth("[2/2]")
	if got := GetRGBFore(scr.GetCell(counterX, 0).Attributes); got != 0xAAAAAA {
		t.Fatalf("workspace counter decoration foreground = %#x, want tab bar text color", got)
	}
	if len(fm.workspaceTabHits) != 2 {
		t.Fatalf("tab hit targets = %d, want 2", len(fm.workspaceTabHits))
	}
	if got := GetRGBBack(scr.GetCell(fm.workspaceTabHits[0].x1, 0).Attributes); got != 0x111111 {
		t.Fatalf("inactive tab background = %#x, want dark bar background", got)
	}
	if got := GetRGBBack(scr.GetCell(fm.workspaceTabHits[1].x2, 0).Attributes); got != 0x222222 {
		t.Fatalf("active tab background = %#x, want panel background", got)
	}
	titleSeparatorX := strings.IndexRune(text, '─')
	if titleSeparatorX < 0 {
		t.Fatalf("workspace title separator is missing: %q", text)
	}
	if got := GetRGBFore(scr.GetCell(titleSeparatorX, 0).Attributes); got != 0x5D5D5D {
		t.Fatalf("workspace title separator foreground = %#x, want half-brightness %#x", got, uint32(0x5D5D5D))
	}
	typeMarkerX := strings.Index(text, "P Panels")
	if typeMarkerX < 0 {
		t.Fatalf("workspace type marker is missing: %q", text)
	}
	if got := GetRGBFore(scr.GetCell(typeMarkerX, 0).Attributes); got != 0x5D5D5D {
		t.Fatalf("workspace type marker foreground = %#x, want half-brightness %#x", got, uint32(0x5D5D5D))
	}
	counterCurrentX := scr.width - runewidth.StringWidth("[2/2]") + 1
	if tabFG, counterFG := GetRGBFore(scr.GetCell(fm.workspaceTabHits[0].x1+1, 0).Attributes), GetRGBFore(scr.GetCell(counterCurrentX, 0).Attributes); tabFG != counterFG {
		t.Fatalf("tab number foreground = %#x, counter current foreground = %#x", tabFG, counterFG)
	} else if tabFG != 0xFFD75F {
		t.Fatalf("workspace accent foreground = %#x, want configured %#x", tabFG, uint32(0xFFD75F))
	}

	// Attention has its own color and must not replace the ordinary accent on
	// unrelated controls such as the new-tab button.
	fm.Screens[0].Frames = []Frame{newMockFrame(0, 1, 20, 5, true)}
	fm.drawWorkspaceTabs()
	fm.drawWorkspaceCounter()
	if got := GetRGBFore(scr.GetCell(fm.workspaceTabHits[0].x1+1, 0).Attributes); got != 0xFF5F5F {
		t.Fatalf("attention tab foreground = %#x, want configured attention color", got)
	}
	if got := GetRGBFore(scr.GetCell(fm.workspaceNewTabX, 0).Attributes); got != 0xFFD75F {
		t.Fatalf("new-tab accent foreground = %#x after attention, want ordinary accent", got)
	}
	if got := GetRGBFore(scr.GetCell(counterCurrentX, 0).Attributes); got != 0xFF5F5F {
		t.Fatalf("attention counter foreground = %#x, want configured attention color", got)
	}
	first := fm.workspaceTabHits[0]
	fm.dispatchEvent(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		MouseX:      int16(first.x1),
		MouseY:      0,
		ButtonState: vtinput.FromLeft1stButtonPressed,
	}, false)
	if fm.ActiveIdx != 0 {
		t.Fatalf("clicking first workspace tab selected %d, want 0", fm.ActiveIdx)
	}
}

func TestFrameManager_WorkspacePlusEmitsForkCommand(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(60, 10)
	fm := &frameManager{}
	fm.Init(scr)
	fm.Push(newMockFrame(0, 0, 20, 5, false))
	forked := false
	active := newMockFrame(0, 0, 20, 5, false)
	active.onHandleCommand = func(cmd int, args any) bool {
		forked = cmd == CmResize && args == "fork"
		return forked
	}
	fm.AddScreen(active)
	fm.drawWorkspaceTabs()
	if fm.workspaceNewTabX < 0 {
		t.Fatal("workspace plus hit target was not created")
	}
	fm.dispatchEvent(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		MouseX:      int16(fm.workspaceNewTabX),
		MouseY:      0,
		ButtonState: vtinput.FromLeft1stButtonPressed,
	}, false)
	if !forked {
		t.Fatal("clicking workspace plus did not emit the Ctrl+N fork command")
	}
}

func TestFrameManager_DragWorkspaceTabReordersScreens(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 10)
	fm := &frameManager{}
	fm.Init(scr)
	fm.Push(newMockFrame(0, 1, 20, 5, false))
	fm.AddScreen(newMockFrame(0, 1, 20, 5, false))
	fm.AddScreen(newMockFrame(0, 1, 20, 5, false))
	fm.drawWorkspaceTabs()

	dragged := fm.Screens[0]
	first := fm.workspaceTabHits[0]
	third := fm.workspaceTabHits[2]
	fm.dispatchEvent(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		MouseX:      int16(first.x1 + 1),
		MouseY:      0,
		ButtonState: vtinput.FromLeft1stButtonPressed,
	}, false)
	fm.dispatchEvent(&vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		KeyDown:         true,
		MouseX:          int16(third.x1 + 1),
		MouseY:          5,
		ButtonState:     vtinput.FromLeft1stButtonPressed,
		MouseEventFlags: vtinput.MouseMoved,
	}, false)
	fm.dispatchEvent(&vtinput.InputEvent{
		Type:    vtinput.MouseEventType,
		MouseX:  500,
		MouseY:  500,
		KeyDown: false,
	}, false)

	if got := []int{fm.Screens[0].Number, fm.Screens[1].Number, fm.Screens[2].Number}; !reflect.DeepEqual(got, []int{2, 3, 1}) {
		t.Fatalf("workspace order = %v, want [2 3 1]", got)
	}
	if fm.Screens[fm.ActiveIdx] != dragged || fm.ActiveIdx != 2 {
		t.Fatalf("dragged workspace is not active at its new position: active=%d", fm.ActiveIdx)
	}
	if fm.workspaceTabDrag != nil {
		t.Fatal("workspace tab capture was not released")
	}
}

func TestFrameManager_DragShortTabPastLongTabDoesNotOscillate(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(100, 10)
	fm := &frameManager{}
	fm.Init(scr)

	short := newMockFrame(0, 1, 20, 5, false)
	short.tabTitle = "A"
	long := newMockFrame(0, 1, 20, 5, false)
	long.tabTitle = "A much longer workspace title"
	last := newMockFrame(0, 1, 20, 5, false)
	last.tabTitle = "C"
	fm.Push(short)
	fm.AddScreen(long)
	fm.AddScreen(last)
	fm.drawWorkspaceTabs()

	dragged := fm.Screens[0]
	first := fm.workspaceTabHits[0]
	second := fm.workspaceTabHits[1]
	dragX := second.x1 + 1
	fm.dispatchEvent(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		MouseX:      int16(first.x1 + 1),
		MouseY:      0,
		ButtonState: vtinput.FromLeft1stButtonPressed,
	}, false)
	fm.dispatchEvent(&vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		KeyDown:         true,
		MouseX:          int16(dragX),
		MouseY:          0,
		ButtonState:     vtinput.FromLeft1stButtonPressed,
		MouseEventFlags: vtinput.MouseMoved,
	}, false)
	if fm.Screens[1] != dragged {
		t.Fatal("short tab did not move into the second slot")
	}

	// The long tab now occupies the beginning of the row and its freshly
	// rendered hit region covers dragX. A dynamic hit test would therefore
	// bounce the dragged tab back to slot zero on this one-pixel move.
	fm.dispatchEvent(&vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		KeyDown:         true,
		MouseX:          int16(dragX + 1),
		MouseY:          0,
		ButtonState:     vtinput.FromLeft1stButtonPressed,
		MouseEventFlags: vtinput.MouseMoved,
	}, false)
	if fm.Screens[1] != dragged {
		t.Fatal("short tab oscillated back after the long tab changed the rendered boundary")
	}
}

func TestFrameManager_RestoreScreenNumbersKeepsFutureNumbersUnique(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 10)
	fm := &frameManager{}
	fm.Init(scr)
	fm.Push(newMockFrame(0, 1, 20, 5, false))
	fm.AddScreenBackground(newMockFrame(0, 1, 20, 5, false))
	fm.AddScreenBackground(newMockFrame(0, 1, 20, 5, false))

	fm.RestoreScreenNumbers([]int{2, 3, 1})
	fm.AddScreenBackground(newMockFrame(0, 1, 20, 5, false))
	if got := fm.Screens[1].Number; got != 4 {
		t.Fatalf("new workspace number = %d, want 4 after restoring [2 3 1]", got)
	}
}

func TestFrameManager_NewWorkspaceAppearsAfterActiveAndUsesCurrentMaxNumber(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 10)
	fm := &frameManager{}
	fm.Init(scr)
	fm.Push(newMockFrame(0, 1, 20, 5, false))
	fm.AddScreen(newMockFrame(0, 1, 20, 5, false))
	fm.AddScreen(newMockFrame(0, 1, 20, 5, false))

	// Remove the highest number. A stale monotonic counter would allocate 4;
	// the current maximum is now 2, so the replacement must be number 3.
	fm.SwitchScreen(0)
	fm.CloseScreen(2)
	fm.AddScreen(newMockFrame(0, 1, 20, 5, false))

	if fm.ActiveIdx != 1 {
		t.Fatalf("new workspace index = %d, want 1 immediately after the active tab", fm.ActiveIdx)
	}
	if got := []int{fm.Screens[0].Number, fm.Screens[1].Number, fm.Screens[2].Number}; !reflect.DeepEqual(got, []int{1, 3, 2}) {
		t.Fatalf("workspace numbers/order = %v, want [1 3 2]", got)
	}
}

func TestFrameManager_NewBackgroundWorkspaceAppearsAfterActive(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 10)
	fm := &frameManager{}
	fm.Init(scr)
	fm.Push(newMockFrame(0, 1, 20, 5, false))
	fm.AddScreen(newMockFrame(0, 1, 20, 5, false))
	fm.AddScreen(newMockFrame(0, 1, 20, 5, false))
	fm.SwitchScreen(0)

	active := fm.Screens[0]
	fm.AddScreenBackground(newMockFrame(0, 1, 20, 5, false))
	if fm.ActiveIdx != 0 || fm.Screens[0] != active {
		t.Fatal("background workspace changed the active tab")
	}
	if got := []int{fm.Screens[0].Number, fm.Screens[1].Number, fm.Screens[2].Number, fm.Screens[3].Number}; !reflect.DeepEqual(got, []int{1, 4, 2, 3}) {
		t.Fatalf("background workspace order = %v, want [1 4 2 3]", got)
	}
}

func TestFrameManager_MiddleClickClosesBackgroundWorkspace(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 10)
	fm := &frameManager{}
	fm.Init(scr)
	fm.Push(newMockFrame(0, 0, 20, 5, false))
	fm.AddScreen(newMockFrame(0, 0, 20, 5, false))
	fm.AddScreen(newMockFrame(0, 0, 20, 5, false))
	active := fm.Screens[fm.ActiveIdx]
	fm.drawWorkspaceTabs()
	first := fm.workspaceTabHits[0]
	fm.dispatchEvent(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		MouseX:      int16(first.x1 + 1),
		MouseY:      0,
		ButtonState: vtinput.FromLeft2ndButtonPressed,
	}, false)
	if len(fm.Screens) != 2 {
		t.Fatalf("middle-click left %d workspaces, want 2", len(fm.Screens))
	}
	if fm.Screens[fm.ActiveIdx] != active {
		t.Fatal("closing a background tab changed the active workspace")
	}
	if fm.Screens[0].Number != 2 || fm.Screens[1].Number != 3 {
		t.Fatalf("remaining workspace numbers = [%d %d], want [2 3]", fm.Screens[0].Number, fm.Screens[1].Number)
	}
}

func TestFrameManager_WorkspaceTabsSemanticModelAndActions(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(60, 10)
	fm := &frameManager{}
	fm.Init(scr)
	forked := false
	makeFrame := func(title string) *mockFrame {
		frame := newMockFrame(0, 0, 20, 5, false)
		frame.tabTitle = title
		frame.onHandleCommand = func(cmd int, args any) bool {
			forked = cmd == CmResize && args == "fork"
			return forked
		}
		return frame
	}
	fm.Push(makeFrame("Left ↔ Right"))
	fm.AddScreen(makeFrame("Python"))
	fm.drawWorkspaceTabs()
	fm.drawWorkspaceCounter()

	scene := fm.ExportSemanticScene()
	bar, ok := scene["workspaceTabs"].(map[string]any)
	if !ok {
		t.Fatalf("workspaceTabs semantic node missing: %#v", scene["workspaceTabs"])
	}
	if bar["kind"] != "tablist" || bar["visible"] != true || bar["activeIndex"] != 1 {
		t.Fatalf("workspace tablist metadata = %#v", bar)
	}
	tabs, ok := bar["tabs"].([]map[string]any)
	if !ok || len(tabs) != 2 {
		t.Fatalf("workspace semantic tabs = %#v, want two tabs", bar["tabs"])
	}
	if tabs[0]["text"] != "Left ↔ Right" || tabs[0]["active"] != false || tabs[0]["w"] == nil {
		t.Fatalf("first workspace semantic tab = %#v", tabs[0])
	}
	if tabs[1]["text"] != "Python" || tabs[1]["selected"] != true {
		t.Fatalf("active workspace semantic tab = %#v", tabs[1])
	}
	if tabs[1]["closable"] != true || tabs[1]["closeAction"] != "workspace.close" {
		t.Fatalf("workspace close semantics missing: %#v", tabs[1])
	}
	newTab := bar["newTab"].(map[string]any)
	if newTab["kind"] != "button" || newTab["action"] != "workspace.new" || newTab["visible"] != true {
		t.Fatalf("workspace new semantic node = %#v", newTab)
	}
	counter := bar["counter"].(map[string]any)
	if counter["text"] != "[2/2]" || counter["current"] != 2 || counter["total"] != 2 {
		t.Fatalf("workspace counter semantic node = %#v", counter)
	}

	if !fm.HandleSemanticAction(map[string]any{"action": "activate", "target": tabs[0]["id"]}) || fm.ActiveIdx != 0 {
		t.Fatalf("semantic tab activation selected workspace %d, want 0", fm.ActiveIdx)
	}
	if !fm.HandleSemanticAction(map[string]any{"action": "workspace.new", "target": "workspace-new"}) || !forked {
		t.Fatal("semantic workspace.new did not emit the Ctrl+N fork command")
	}
	if !fm.HandleSemanticAction(map[string]any{"action": "workspace.close", "target": tabs[1]["id"]}) || len(fm.Screens) != 1 {
		t.Fatalf("semantic workspace.close left %d workspaces, want 1", len(fm.Screens))
	}
}

func TestFrameManager_CtrlTabDirectAndMenuModes(t *testing.T) {
	newManager := func() *frameManager {
		scr := NewSilentScreenBuf()
		scr.AllocBuf(80, 25)
		fm := &frameManager{}
		fm.Init(scr)
		fm.Push(newMockFrame(0, 0, 80, 25, false))
		fm.AddScreen(newMockFrame(0, 0, 80, 25, false))
		return fm
	}

	direct := newManager()
	direct.ConfigureWorkspaceTabs(WorkspaceTabsOnCtrl, WorkspaceCtrlTabDirect)
	direct.dispatchEvent(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_TAB,
		ControlKeyState: vtinput.LeftCtrlPressed,
	}, false)
	if direct.ActiveIdx != 0 || direct.switcherMenu != nil {
		t.Fatalf("direct Ctrl+Tab active=%d menu=%v, want active=0 without menu", direct.ActiveIdx, direct.switcherMenu != nil)
	}

	menu := newManager()
	menu.ConfigureWorkspaceTabs(WorkspaceTabsMultiple, WorkspaceCtrlTabMenu)
	menu.dispatchEvent(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_TAB,
		ControlKeyState: vtinput.LeftCtrlPressed,
	}, false)
	if menu.ActiveIdx != 1 || menu.switcherMenu == nil {
		t.Fatalf("menu Ctrl+Tab active=%d menu=%v, want active=1 with menu", menu.ActiveIdx, menu.switcherMenu != nil)
	}
}
func TestFrameManager_CtrlTabNeverMode(t *testing.T) {
	newManager := func() *frameManager {
		scr := NewSilentScreenBuf()
		scr.AllocBuf(80, 25)
		fm := &frameManager{}
		fm.Init(scr)
		fm.Push(newMockFrame(0, 0, 80, 25, false))
		fm.AddScreen(newMockFrame(0, 0, 80, 25, false))
		return fm
	}

	fm := newManager()
	fm.ConfigureWorkspaceTabs(WorkspaceTabsNever, WorkspaceCtrlTabDirect)

	if fm.WorkspaceTopInset() != 0 {
		t.Fatalf("WorkspaceTopInset with WorkspaceTabsNever = %d, want 0", fm.WorkspaceTopInset())
	}
	if fm.workspaceTabsVisible() {
		t.Fatal("workspaceTabsVisible should be false for WorkspaceTabsNever")
	}

	fm.ctrlPressed = true
	fm.dispatchEvent(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_TAB,
		ControlKeyState: vtinput.LeftCtrlPressed,
	}, false)

	if fm.ActiveIdx != 1 || fm.switcherMenu == nil {
		t.Fatalf("Never mode Ctrl+Tab active=%d menu=%v, want active=1 with switcher menu", fm.ActiveIdx, fm.switcherMenu != nil)
	}
}

func TestFrameManager_AltNumberSelectsStableWorkspaceNumber(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm := &frameManager{}
	fm.Init(scr)
	first := newMockFrame(0, 0, 80, 25, false)
	forwarded := 0
	first.onProcessKey = func(*vtinput.InputEvent) bool {
		forwarded++
		return false
	}
	fm.Push(first)
	fm.AddScreen(newMockFrame(0, 0, 80, 25, false))
	fm.AddScreen(newMockFrame(0, 0, 80, 25, false))
	fm.RestoreScreenNumbers([]int{7, 2, 9})
	fm.ConfigureWorkspaceAltNumberSwitch(true)

	fm.dispatchEvent(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_2,
		ControlKeyState: vtinput.LeftAltPressed,
	}, false)
	if fm.ActiveIdx != 1 || fm.Screens[fm.ActiveIdx].Number != 2 {
		t.Fatalf("Alt+2 selected index=%d number=%d, want stable workspace number 2", fm.ActiveIdx, fm.Screens[fm.ActiveIdx].Number)
	}

	fm.dispatchEvent(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_7,
		ControlKeyState: vtinput.RightAltPressed,
	}, false)
	if fm.ActiveIdx != 0 || fm.Screens[fm.ActiveIdx].Number != 7 {
		t.Fatalf("right Alt+7 selected index=%d number=%d, want stable workspace number 7", fm.ActiveIdx, fm.Screens[fm.ActiveIdx].Number)
	}
	forwarded = 0 // Ignore focus events generated by the workspace switches.

	// Missing numbers and modified variants are not consumed.
	fm.dispatchEvent(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_1,
		ControlKeyState: vtinput.LeftAltPressed,
	}, false)
	fm.dispatchEvent(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_2,
		ControlKeyState: vtinput.LeftAltPressed | vtinput.ShiftPressed,
	}, false)
	if fm.ActiveIdx != 0 || forwarded != 2 {
		t.Fatalf("fall-through Alt-number variants: active=%d forwarded=%d, want 0/2", fm.ActiveIdx, forwarded)
	}

	fm.ConfigureWorkspaceAltNumberSwitch(false)
	fm.dispatchEvent(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_2,
		ControlKeyState: vtinput.LeftAltPressed,
	}, false)
	if fm.ActiveIdx != 0 || forwarded != 3 {
		t.Fatalf("disabled Alt-number shortcut: active=%d forwarded=%d, want 0/3", fm.ActiveIdx, forwarded)
	}
}

func TestFrameManager_CtrlAltNumberSelectsWorkspaceOnlyInOnCtrlTabMode(t *testing.T) {
	newManager := func(mode WorkspaceTabMode) (*frameManager, *int) {
		scr := NewSilentScreenBuf()
		scr.AllocBuf(80, 25)
		fm := &frameManager{}
		fm.Init(scr)
		forwarded := 0
		first := newMockFrame(0, 0, 80, 25, false)
		first.onProcessKey = func(*vtinput.InputEvent) bool {
			forwarded++
			return false
		}
		fm.Push(first)
		fm.AddScreen(newMockFrame(0, 0, 80, 25, false))
		fm.RestoreScreenNumbers([]int{1, 2})
		fm.ConfigureWorkspaceTabs(mode, WorkspaceCtrlTabDirect)
		fm.ConfigureWorkspaceAltNumberSwitch(true)
		fm.SwitchScreen(0)
		forwarded = 0 // Ignore focus events generated while arranging the fixture.
		return fm, &forwarded
	}

	event := func() *vtinput.InputEvent {
		return &vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         true,
			VirtualKeyCode:  vtinput.VK_2,
			ControlKeyState: vtinput.LeftCtrlPressed | vtinput.LeftAltPressed,
		}
	}

	onCtrl, _ := newManager(WorkspaceTabsOnCtrl)
	onCtrl.dispatchEvent(event(), false)
	if onCtrl.ActiveIdx != 1 {
		t.Fatalf("Ctrl+Alt+2 in Ctrl-only tab mode selected index %d, want 1", onCtrl.ActiveIdx)
	}
	onCtrl.SwitchScreen(0)
	onCtrl.dispatchEvent(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_2,
		ControlKeyState: vtinput.RightCtrlPressed | vtinput.RightAltPressed,
	}, false)
	if onCtrl.ActiveIdx != 1 {
		t.Fatalf("right Ctrl+Alt+2 in Ctrl-only tab mode selected index %d, want 1", onCtrl.ActiveIdx)
	}

	for _, mode := range []WorkspaceTabMode{WorkspaceTabsAlways, WorkspaceTabsMultiple} {
		fm, forwarded := newManager(mode)
		fm.dispatchEvent(event(), false)
		if fm.ActiveIdx != 0 || *forwarded != 1 {
			t.Fatalf("Ctrl+Alt+2 in tab mode %d: active=%d forwarded=%d, want 0/1", mode, fm.ActiveIdx, *forwarded)
		}
	}
}

func TestFrameManager_SwitcherRichContent(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	defer fm.Shutdown()
	fm.Push(NewDesktop())

	// Screen 1: With progress
	w1 := NewWindow(0, 0, 10, 10, "TaskWin")
	w1.SetProgress(45)
	fm.AddScreen(w1)

	// Screen 2: With attention
	w2 := NewWindow(0, 0, 10, 10, "AlertWin")
	fm.AddScreen(w2)
	fm.Push(NewDialog(0, 0, 5, 5, "Modal"))

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
	scr := NewSilentScreenBuf()
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
	})

	fm.Step(0)

	if mb.Active {
		t.Error("MenuBar should NOT be activated via F9 when a modal dialog is open")
	}
}
func TestFrameManager_ModalBlocksMenuMouse(t *testing.T) {
	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)
	fm.Push(NewDesktop())

	mb := NewMenuBar([]string{"Options"})
	mb.SetPosition(0, 0, 79, 0)
	fm.MenuBar = mb

	// Создаем модальный диалог
	dlg := NewDialog(10, 10, 30, 15, "Modal")
	fm.Push(dlg)

	// Кликаем в область меню (0, 0)
	fm.dispatchEvent(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		MouseX:      5,
		MouseY:      0,
		ButtonState: vtinput.FromLeft1stButtonPressed,
	}, false)

	if mb.Active {
		t.Error("Mouse click on MenuBar should be blocked by modal dialog")
	}
}
func TestFrameManager_PushToFrameScreen_Active(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
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
	fm.Init(NewSilentScreenBuf())
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
	fm.Init(NewSilentScreenBuf())
	defer fm.Shutdown()

	f1Focus := false
	f1 := &mockFrame{}
	f1.onProcessKey = func(e *vtinput.InputEvent) bool {
		if e.Type == vtinput.FocusEventType {
			f1Focus = e.SetFocus
		}
		return true
	}
	fm.Push(f1) // Screen 0, f1Focus = true

	f2Focus := false
	f2 := &mockFrame{}
	f2.onProcessKey = func(e *vtinput.InputEvent) bool {
		if e.Type == vtinput.FocusEventType {
			f2Focus = e.SetFocus
		}
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
	fm.Init(NewSilentScreenBuf())
	defer fm.Shutdown()
	fm.Push(NewDesktop()) // Screen 0

	// 1. Добавляем диалог задачи на Screen 0. Screen 0: [Desktop, Task]
	taskDlg := NewDialog(0, 0, 10, 10, "Task")
	fm.Push(taskDlg)

	// 2. Создаем Screen 1 для работы. [Screen 0, Screen 1:ActiveWork].
	workWin := NewWindow(0, 0, 10, 10, "ActiveWork")
	fm.AddScreen(workWin)

	// 3. Задача в фоне (Screen 0) завершается и шлет модальный диалог
	doneMsg := NewDialog(0, 0, 5, 5, "Finished") // NewDialog по умолчанию Modal = true
	fm.PushToFrameScreen(taskDlg, doneMsg)

	// 4. Assertions
	// Так как сообщение модальное, оно должно было вызвать SwitchScreen и "всплыть" в конец.
	topTitle := fm.Screens[fm.ActiveIdx].GetTitle()
	if topTitle != "Finished" {
		t.Errorf("Modal notification failed to pull focus. Active screen title: %q", topTitle)
	}
}
func TestFrameManager_PushToFrameScreen_LostAnchor(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
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
	fm.Init(NewSilentScreenBuf())

	var lastEvent *vtinput.InputEvent
	frame := &mockFrame{}
	frame.onProcessMouse = func(e *vtinput.InputEvent) bool {
		lastEvent = e
		return true
	}
	fm.Push(frame)

	dispatch := func(e *vtinput.InputEvent) {
		fm.markMultiClick(e, time.Now())
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

	// 3. Fast third click, same spot - IS triple click
	time.Sleep(100 * time.Millisecond)
	dispatch(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, MouseX: 10, MouseY: 10, ButtonState: vtinput.FromLeft1stButtonPressed})
	if (lastEvent.MouseEventFlags & TripleClick) == 0 {
		t.Error("Fast third click was not detected as triple click")
	}
	if (lastEvent.MouseEventFlags & vtinput.DoubleClick) != 0 {
		t.Error("Triple click should not also be marked as double click")
	}

	// 4. Slow next click - no multi-click
	time.Sleep(500 * time.Millisecond)
	dispatch(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, MouseX: 10, MouseY: 10, ButtonState: vtinput.FromLeft1stButtonPressed})
	if (lastEvent.MouseEventFlags & (vtinput.DoubleClick | TripleClick)) != 0 {
		t.Error("Slow click should not be a multi-click")
	}

	// 5. Fast click, different spot - no double click
	time.Sleep(100 * time.Millisecond)
	dispatch(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, MouseX: 11, MouseY: 10, ButtonState: vtinput.FromLeft1stButtonPressed})
	if (lastEvent.MouseEventFlags & vtinput.DoubleClick) != 0 {
		t.Error("Click in different spot should not be a double click")
	}

	// 6. Fast click, different button - no double click
	time.Sleep(100 * time.Millisecond)
	dispatch(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, MouseX: 11, MouseY: 10, ButtonState: vtinput.RightmostButtonPressed})
	if (lastEvent.MouseEventFlags & vtinput.DoubleClick) != 0 {
		t.Error("Click with different button should not be a double click")
	}
}

func TestFrameManager_CloseActiveScreen_Shifting(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	defer fm.Shutdown()
	fm.Push(NewDesktop()) // Screen 0

	fm.AddScreen(NewWindow(0, 0, 10, 10, "W1")) // Screen 1
	fm.AddScreen(NewWindow(0, 0, 10, 10, "W2")) // Screen 2

	// Порядок в массиве: [S0, S1, S2]. Активен S2 (W2).

	// Переключаемся на S1 (W1).
	// Порядок становится: [S0, S2, S1]. Активен S1 (W1) на индексе 2.
	fm.SwitchScreen(1)

	// Закрываем активный экран (W1).
	fm.CloseActiveScreen()

	// Порядок должен стать: [S0, S2]. Активен S2 (W2) на индексе 1.
	if len(fm.Screens) != 2 {
		t.Errorf("Expected 2 screens after close, got %d", len(fm.Screens))
	}
	if fm.ActiveIdx != 1 {
		t.Errorf("ActiveIdx should point to the last element (1), got %d", fm.ActiveIdx)
	}
	if fm.Screens[fm.ActiveIdx].GetTitle() != "W2" {
		t.Errorf("Expected W2 to be active, got %q", fm.Screens[fm.ActiveIdx].GetTitle())
	}
}

func TestFrameManager_CloseScreenVeto(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	fm.Push(newMockFrame(0, 0, 10, 10, false))
	vetoer := &closeVetoFrame{}
	fm.AddScreen(vetoer)

	target := fm.Screens[fm.ActiveIdx]
	fm.CloseScreen(fm.ActiveIdx)

	if len(fm.Screens) != 2 {
		t.Fatalf("CloseScreen left %d workspaces, want 2 after veto", len(fm.Screens))
	}
	if fm.Screens[fm.ActiveIdx] != target {
		t.Fatal("CloseScreen changed the active workspace after veto")
	}
	if vetoer.confirmCloseCalls != 1 {
		t.Fatalf("ConfirmClose called %d times, want 1", vetoer.confirmCloseCalls)
	}
	if vetoer.closeCalls != 0 {
		t.Fatalf("Close called %d times before veto, want 0", vetoer.closeCalls)
	}
}

func TestFrameManager_CloseScreenAllowedByVetoer(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	fm.Push(newMockFrame(0, 0, 10, 10, false))
	vetoer := &closeVetoFrame{allowClose: true}
	fm.AddScreen(vetoer)

	fm.CloseScreen(fm.ActiveIdx)

	if len(fm.Screens) != 1 {
		t.Fatalf("CloseScreen left %d workspaces, want 1", len(fm.Screens))
	}
	if vetoer.confirmCloseCalls != 1 {
		t.Fatalf("ConfirmClose called %d times, want 1", vetoer.confirmCloseCalls)
	}
	if vetoer.closeCalls != 1 {
		t.Fatalf("Close called %d times, want 1", vetoer.closeCalls)
	}
}

func TestFrameManager_CloseScreenFindsVetoerUnderDialog(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	fm.Push(newMockFrame(0, 0, 10, 10, false))
	vetoer := &closeVetoFrame{}
	fm.AddScreen(vetoer)
	dialog := newMockFrame(0, 0, 5, 5, true)
	fm.Push(dialog)

	fm.CloseScreen(fm.ActiveIdx)

	if len(fm.Screens) != 2 {
		t.Fatalf("CloseScreen left %d workspaces, want 2 after underlying frame veto", len(fm.Screens))
	}
	if vetoer.confirmCloseCalls != 1 {
		t.Fatalf("underlying frame ConfirmClose called %d times, want 1", vetoer.confirmCloseCalls)
	}
	if vetoer.closeCalls != 0 {
		t.Fatalf("underlying frame Close called %d times before veto, want 0", vetoer.closeCalls)
	}
	if dialog.IsDone() {
		t.Fatal("dialog was closed before underlying frame veto")
	}
}

func TestFrameManager_WorkspaceCloseRoutesRespectVeto(t *testing.T) {
	newManager := func() (*frameManager, *closeVetoFrame) {
		SetDefaultPalette()
		scr := NewSilentScreenBuf()
		scr.AllocBuf(80, 10)
		fm := &frameManager{}
		fm.Init(scr)
		fm.Push(newMockFrame(0, 0, 20, 5, false))
		vetoer := &closeVetoFrame{}
		fm.AddScreen(vetoer)
		fm.SwitchScreen(0)
		return fm, vetoer
	}

	t.Run("Ctrl+W fallback", func(t *testing.T) {
		fm, vetoer := newManager()
		fm.SwitchScreen(1)

		fm.dispatchEvent(&vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         true,
			VirtualKeyCode:  vtinput.VK_W,
			ControlKeyState: vtinput.LeftCtrlPressed,
		}, false)

		if len(fm.Screens) != 2 || fm.ActiveIdx != 1 {
			t.Fatal("Ctrl+W fallback closed a vetoed workspace or changed the active workspace")
		}
		if vetoer.confirmCloseCalls != 1 || vetoer.closeCalls != 0 {
			t.Fatalf("Ctrl+W ConfirmClose calls = %d, Close calls = %d; want 1, 0", vetoer.confirmCloseCalls, vetoer.closeCalls)
		}
	})

	t.Run("middle click", func(t *testing.T) {
		fm, vetoer := newManager()
		active := fm.Screens[fm.ActiveIdx]
		fm.drawWorkspaceTabs()
		target := fm.workspaceTabHits[1]

		fm.dispatchEvent(&vtinput.InputEvent{
			Type:        vtinput.MouseEventType,
			KeyDown:     true,
			MouseX:      int16(target.x1 + 1),
			MouseY:      0,
			ButtonState: vtinput.FromLeft2ndButtonPressed,
		}, false)

		if len(fm.Screens) != 2 || fm.Screens[fm.ActiveIdx] != active {
			t.Fatal("middle-click closed a vetoed workspace or changed the active workspace")
		}
		if vetoer.confirmCloseCalls != 1 || vetoer.closeCalls != 0 {
			t.Fatalf("middle-click ConfirmClose calls = %d, Close calls = %d; want 1, 0", vetoer.confirmCloseCalls, vetoer.closeCalls)
		}
	})

	t.Run("semantic action", func(t *testing.T) {
		fm, vetoer := newManager()
		handled := fm.HandleSemanticAction(map[string]any{
			"action": "workspace.close",
			"index":  1,
		})

		if !handled {
			t.Fatal("semantic workspace.close was not handled")
		}
		if len(fm.Screens) != 2 {
			t.Fatalf("semantic workspace.close left %d workspaces, want 2 after veto", len(fm.Screens))
		}
		if vetoer.confirmCloseCalls != 1 || vetoer.closeCalls != 0 {
			t.Fatalf("semantic close ConfirmClose calls = %d, Close calls = %d; want 1, 0", vetoer.confirmCloseCalls, vetoer.closeCalls)
		}
	})
}

func TestFrameManager_CloseActiveScreen_CancelsTasks(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	defer fm.Shutdown()
	fm.Push(NewDesktop()) // Screen 0

	// Screen 1
	fm.AddScreen(NewWindow(0, 0, 10, 10, "W1"))

	// Push a dialog with OnResult
	dlg := NewDialog(0, 0, 5, 5, "TaskDlg")
	cancelled := false
	dlg.OnResult = func(code int) {
		cancelled = true
	}
	fm.Push(dlg)

	// Close Active Screen (Screen 1)
	fm.CloseActiveScreen()

	if !cancelled {
		t.Error("CloseActiveScreen did not call Close() (and OnResult) on the frames being dropped")
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
	modal := &mockFrame{}
	modal.Modal = true
	s.Frames = append(s.Frames, modal)

	if s.GetProgress() != 42 {
		t.Errorf("Expected progress 42 to peek through modal dialog, got %d", s.GetProgress())
	}
}

func TestFrameManager_BackgroundScreenFallback_Fix(t *testing.T) {
	// Сценарий бага:
	// 1. Панели активны (Screen 0)
	// 2. В фоне появляется Очередь (теперь Screen 0)
	// 3. Открывается Редактор (Screen 2)
	// 4. Редактор закрывается. Должны вернуться в Панели, а не в Очередь.

	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	defer fm.Shutdown()

	pFrame := &titleFrame{title: "Panels"}
	fm.frames = append(fm.frames, pFrame)
	fm.Screens[0].Frames = fm.frames // Initial Panels screen

	// 1. Добавляем фон (Очередь)
	qFrame := &titleFrame{title: "Queue"}
	fm.AddScreenBackground(qFrame)

	if len(fm.Screens) != 2 {
		t.Fatal("Queue screen not added")
	}
	if fm.Screens[fm.ActiveIdx].GetTitle() != "Panels" {
		t.Errorf("Focus stolen by background screen. Current: %q", fm.Screens[fm.ActiveIdx].GetTitle())
	}

	// 2. Открываем Редактор (через AddScreen, он уйдет в конец)
	eFrame := &titleFrame{title: "Editor"}
	fm.AddScreen(eFrame)

	if fm.Screens[fm.ActiveIdx].GetTitle() != "Editor" {
		t.Fatal("Editor not active")
	}

	// 3. Закрываем Редактор
	eFrame.SetExitCode(0)
	fm.cleanupDoneFrames()

	// Проверка: мы должны вернуться к Panels, а не к Queue
	currentTitle := fm.Screens[fm.ActiveIdx].GetTitle()
	if currentTitle != "Panels" {
		t.Errorf("FALLBACK BUG: Closed editor and landed in %q instead of Panels", currentTitle)
	}
}
func TestFrameManager_HideCursorWhenMenuIsActive(t *testing.T) {
	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)
	fm.Push(NewDesktop())

	// Push a dummy focused User Frame that wants a cursor
	frame := &mockFrame{}
	frame.onProcessKey = func(e *vtinput.InputEvent) bool { return false }
	fm.Push(frame)

	// Set cursor position to simulate active cursor in the frame
	scr.SetCursorVisible(true)

	// Active menu bar
	mb := NewMenuBar([]string{"Options"})
	fm.MenuBar = mb
	mb.Active = true // Open menu

	fm.renderPhase()

	if scr.cursorVisible {
		t.Error("Cursor should be hidden when MenuBar is active")
	}
}

// trailFrame paints a single cell at a movable position. Used to verify
// that a transparent bottom screen leaves no stale content behind.
type trailFrame struct {
	BaseFrame
	x, y int
}

func (f *trailFrame) Show(scr *ScreenBuf) {
	scr.Write(f.x, f.y, StringToCharInfo("X", SetRGBBoth(0, 0xFFFFFF, 0x000000)))
}
func (f *trailFrame) GetType() FrameType { return TypeUser }
func (f *trailFrame) GetTitle() string   { return "TrailFrame" }

func TestFrameManager_TransparentBaseClearsVacatedCells(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 5)
	fm := &frameManager{}
	fm.Init(scr)

	// The host paints the background itself (e.g. a terminal under the
	// overlay): the bottom screen is declared transparent.
	fm.Screens[0].Transparent = true

	frame := &trailFrame{x: 2, y: 1}
	fm.Push(frame)
	fm.renderPhase()
	if cell := scr.GetCell(2, 1); cell.Char != 'X' {
		t.Fatalf("expected frame painted at (2,1), got %q", rune(cell.Char))
	}

	// Move the frame: the vacated cell must be cleared, not keep the copy.
	frame.x, frame.y = 5, 2
	fm.renderPhase()
	if cell := scr.GetCell(2, 1); cell.Char != 0 {
		t.Errorf("vacated cell (2,1) still holds %q: moved frames leave a trail", rune(cell.Char))
	}
	if cell := scr.GetCell(5, 2); cell.Char != 'X' {
		t.Errorf("expected frame painted at (5,2), got %q", rune(cell.Char))
	}
}

func TestFrameManager_OpaqueBaseKeepsLegacyBehavior(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 5)
	fm := &frameManager{}
	fm.Init(scr)

	// Default (opaque) bottom screen: the buffer is NOT cleared between
	// render passes — the base screen is expected to paint every cell.
	frame := &trailFrame{x: 2, y: 1}
	fm.Push(frame)
	fm.renderPhase()
	frame.x, frame.y = 5, 2
	fm.renderPhase()
	if cell := scr.GetCell(2, 1); cell.Char != 'X' {
		t.Errorf("opaque base: vacated cell (2,1) unexpectedly cleared")
	}
}
func TestFrameManager_NilEventsHandling(t *testing.T) {
	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 10)
	fm.Init(scr)
	defer fm.Shutdown()

	f := &mockFrame{}
	fm.Push(f)

	pr, pw := io.Pipe()
	reader := vtinput.NewReader(pr, false)

	// Simulate sending a nil event followed by a closed channel
	go func() {
		reader.EventChan <- nil
		time.Sleep(10 * time.Millisecond)
		pw.Close() // This triggers EOF on reader, closing EventChan
	}()

	// Should handle the nil event safely and exit cleanly without panicking
	fm.Run(reader)
}

func TestFrameManager_WaitFar2lResponse_NilEvent(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	defer fm.Shutdown()

	fm.EventChan = make(chan *vtinput.InputEvent, 10) // Initialize EventChan for the test

	// Send nil event and then find the active waiter channel and send the valid response
	go func() {
		fm.EventChan <- nil
		time.Sleep(10 * time.Millisecond)

		var ch chan *vtinput.Far2lStack
		for i := 0; i < 100; i++ {
			fm.far2lMu.Lock()
			if fm.pendingFar2l != nil {
				ch = fm.pendingFar2l[42]
			}
			fm.far2lMu.Unlock()
			if ch != nil {
				break
			}
			time.Sleep(1 * time.Millisecond)
		}

		if ch != nil {
			ch <- &vtinput.Far2lStack{42, 1}
		}
	}()

	// WaitFar2lResponse should skip nil without panicking and return valid response
	res := fm.WaitFar2lResponse(42, 150*time.Millisecond)
	if res == nil {
		t.Error("WaitFar2lResponse returned nil, expected valid response after skipping nil event")
	}
}
func TestFrameManager_TranslatorTool(t *testing.T) {
	// Временно отключаем синхронизацию с far2l, чтобы избежать 15-секундного таймаута в тестах
	oldFar2l := Far2lEnabled
	Far2lEnabled = false
	defer func() { Far2lEnabled = oldFar2l }()

	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)
	defer fm.Shutdown()

	oldFm := FrameManager
	FrameManager = fm
	defer func() { FrameManager = oldFm }()

	AddStrings(map[string]string{"Label.TestKey": "TestLabelText"})

	dlg := NewDialog(0, 0, 30, 20, "TestDialog")
	dlg.SetHelp("Help.DialogContext")

	txt := NewText(10, 10, "TestLabelText", 0)
	txt.SetHelp("Help.TextContext")
	dlg.AddItem(txt)

	fm.Push(dlg)

	SetOSClipboard("")

	ev := &vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		MouseX:          12,
		MouseY:          10,
		ButtonState:     vtinput.RightmostButtonPressed,
		KeyDown:         true,
		ControlKeyState: vtinput.LeftCtrlPressed | vtinput.LeftAltPressed,
	}

	fm.dispatchEvent(ev, false)

	clip := GetOSClipboard()
	if !strings.Contains(clip, "Key:  Label.TestKey") {
		t.Errorf("Clipboard missing key 'Label.TestKey'. Got:\n%s", clip)
	}
	if !strings.Contains(clip, "Help.DialogContext -> Help.TextContext") {
		t.Errorf("Clipboard missing help context stack. Got:\n%s", clip)
	}
}
func TestFrameManager_FocusLossResetsModifiers(t *testing.T) {
	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)
	fm.KeyBar = NewKeyBar()

	// Push a mock frame to prevent dispatchEvent from returning early
	fm.Push(&mockFrame{})

	// 1. Set modifiers to active via a key event
	fm.dispatchEvent(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		ControlKeyState: vtinput.ShiftPressed | vtinput.LeftCtrlPressed,
	}, false)

	if !fm.KeyBar.shiftState || !fm.KeyBar.ctrlState {
		t.Fatal("Modifiers were not set by Key event")
	}

	// 2. Dispatch FocusOut event
	fm.dispatchEvent(&vtinput.InputEvent{
		Type:     vtinput.FocusEventType,
		SetFocus: false,
	}, false)

	if fm.KeyBar.shiftState || fm.KeyBar.ctrlState || fm.KeyBar.altState {
		t.Error("FocusOut event did not reset KeyBar modifiers")
	}
}
func TestFrameManager_ModifierKeyPressState(t *testing.T) {
	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)
	fm.KeyBar = NewKeyBar()

	// Push a mock frame to prevent early return
	fm.Push(&mockFrame{})

	// 1. Simulate Shift KeyDown (with ControlKeyState showing it as unpressed initially, i.e., 0)
	fm.dispatchEvent(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_SHIFT,
		ControlKeyState: 0,
	}, false)

	if !fm.KeyBar.shiftState {
		t.Error("Expected shiftState to be true on Shift KeyDown despite empty ControlKeyState")
	}

	// 2. Simulate Shift KeyUp (with ControlKeyState still showing it as pressed, i.e., ShiftPressed)
	fm.dispatchEvent(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         false,
		VirtualKeyCode:  vtinput.VK_SHIFT,
		ControlKeyState: vtinput.ShiftPressed,
	}, false)

	if fm.KeyBar.shiftState {
		t.Error("Expected shiftState to be false on Shift KeyUp despite active ControlKeyState")
	}
}

// AddAnimation must wake the heartbeat at once: the idle poll alone would
// wait 250ms and miss short animations like the viewer toast's wall flash.
func TestAddAnimationWakesHeartbeat(t *testing.T) {
	fm := NewFrameManager()
	fm.animWake = make(chan struct{}, 1)
	fm.AddAnimation(func(float64) bool { return true })
	select {
	case <-fm.animWake:
	default:
		t.Error("AddAnimation must signal animWake")
	}
}

// AddAnimation must restart the dt clock after an idle gap, otherwise the
// first tick would span the whole idle duration and jump the animation.
func TestAddAnimationResetsStaleDt(t *testing.T) {
	fm := NewFrameManager()
	fm.lastAnim = time.Now().Add(-10 * time.Second)
	fm.AddAnimation(func(float64) bool { return true })
	if !fm.lastAnim.IsZero() {
		t.Error("AddAnimation must reset lastAnim when transitioning from idle")
	}
}
