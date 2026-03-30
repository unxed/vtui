package vtui

import (
	"os"
	"os/signal"
	"syscall"
	"time"
	"strings"
	"runtime/debug"

	"github.com/unxed/vtinput"
	"golang.org/x/term"
)

// FrameType defines the type of a frame for introspection.
type FrameType int

const (
	TypeDesktop FrameType = iota
	TypeDialog
	TypeMenu
	TypeUser
)

// Frame is the interface that all top-level screen objects (windows, dialogs, menus) must implement.
type Frame interface {
	ProcessKey(e *vtinput.InputEvent) bool
	ProcessMouse(e *vtinput.InputEvent) bool
	Show(scr *ScreenBuf)
	ResizeConsole(w, h int)
	GetType() FrameType
	SetExitCode(code int)
	IsDone() bool
	GetHelp() string
	IsBusy() bool // If true, FrameManager may skip the rendering phase
	HasShadow() bool
	GetKeyLabels() *KeySet
	HandleCommand(cmd int, args any) bool // Turbo Vision style command routing
	HandleBroadcast(cmd int, args any) bool
	Valid(cmd int) bool

	// MDI Methods
	GetMenuBar() *MenuBar
	SetPosition(x1, y1, x2, y2 int)
	GetPosition() (x1, y1, x2, y2 int)
	IsModal() bool
	GetWindowNumber() int
	SetWindowNumber(n int)
	RequestFocus() bool
	Close()
	GetTitle() string
	GetProgress() int // Returns 0-100, or -1 if no progress
}

// AppScreen represents an isolated workspace with its own frame stack.
type AppScreen struct {
	Frames        []Frame
	CapturedFrame Frame
	Transparent   bool // Если true, под этим экраном будет рисоваться предыдущий
}

func (s *AppScreen) GetTitle() string {
	for i := len(s.Frames) - 1; i >= 0; i-- {
		if s.Frames[i].GetType() >= TypeUser {
			return s.Frames[i].GetTitle()
		}
	}
	return "Workspace"
}

func (s *AppScreen) GetProgress() int {
	for i := len(s.Frames) - 1; i >= 0; i-- {
		if p := s.Frames[i].GetProgress(); p >= 0 {
			return p
		}
	}
	return -1
}

func (s *AppScreen) NeedsAttention() bool {
	if len(s.Frames) == 0 { return false }
	top := s.Frames[len(s.Frames)-1]
	// Проверяем флаг подавления внимания
	suppressed := false
	if bf, ok := top.(interface{ IsAttentionSuppressed() bool }); ok {
		suppressed = bf.IsAttentionSuppressed()
	}
	return top.IsModal() && !suppressed && top.GetType() != TypeMenu
}

// frameManager manages multiple screens and the main application loop.
type frameManager struct {
	Screens   []*AppScreen
	ActiveIdx int

	frames         []Frame // Points to the active screen's frame stack
	scr            *ScreenBuf
	RedrawChan     chan struct{}
	TaskChan       chan func()
	EventFilter    func(*vtinput.InputEvent) bool
	injectedEvents []*vtinput.InputEvent
	OnRender       func(scr *ScreenBuf)

	// Global standard UI components
	DisabledCommands CommandSet
	MenuBar    *MenuBar
	StatusLine *StatusLine
	KeyBar     *KeyBar

	capturedFrame Frame // Points to the active screen's captured frame

	// Switcher State
	ctrlPressed      bool
	switcherActive   bool
	switcherIdx      int
	running          bool

	lastMouseClickTime time.Time
	lastMouseX, lastMouseY int
	lastMouseButton uint32
}

// FrameManager is the global instance of the frame manager.
var FrameManager = &frameManager{}

func (fm *frameManager) SyncCurrentScreen() {
	if len(fm.Screens) > 0 {
		fm.Screens[fm.ActiveIdx].Frames = fm.frames
		fm.Screens[fm.ActiveIdx].CapturedFrame = fm.capturedFrame
		DebugLog("FM: SyncCurrentScreen() - Screen %d has %d frames", fm.ActiveIdx, len(fm.frames))
	}
}

func (fm *frameManager) GetActiveFrames(sIdx int) []Frame {
	if sIdx == fm.ActiveIdx {
		return fm.frames
	}
	if sIdx >= 0 && sIdx < len(fm.Screens) {
		return fm.Screens[sIdx].Frames
	}
	return nil
}

func (fm *frameManager) SwitchScreen(idx int) {
	if idx == fm.ActiveIdx && len(fm.frames) > 0 {
		return
	}

	// 1. Notify current screen it's losing focus
	if len(fm.frames) > 0 {
		fm.frames[len(fm.frames)-1].ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: false})
	}

	fm.SyncCurrentScreen()
	fm.ActiveIdx = idx
	fm.frames = fm.Screens[idx].Frames
	fm.capturedFrame = fm.Screens[idx].CapturedFrame

	// 2. Notify new screen it's gaining focus
	if len(fm.frames) > 0 {
		fm.frames[len(fm.frames)-1].ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: true})
	}

	fm.Redraw()
}

func (fm *frameManager) AddScreen(f Frame) {
	// If we are already shutting down or in an inconsistent state, bail out.
	if fm.Screens == nil { return }

	fm.SyncCurrentScreen()
	newScreen := &AppScreen{Frames: make([]Frame, 0, 10)}
	newScreen.Frames = append(newScreen.Frames, NewDesktop())
	newScreen.Frames = append(newScreen.Frames, f)
	fm.Screens = append(fm.Screens, newScreen)
	fm.SwitchScreen(len(fm.Screens) - 1)
	fm.Redraw()
}

func (fm *frameManager) AddScreenHeadless(f Frame) {
	if fm.Screens == nil { return }
	fm.SyncCurrentScreen()
	// Создаем абсолютно чистый стэк без Desktop
	newScreen := &AppScreen{
		Frames:      make([]Frame, 0, 5),
		Transparent: true,
	}
	newScreen.Frames = append(newScreen.Frames, f)
	fm.Screens = append(fm.Screens, newScreen)
	fm.ActiveIdx = len(fm.Screens) - 1
	fm.frames = newScreen.Frames
	fm.capturedFrame = nil
	f.ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: true})
	fm.Redraw()
}

func (fm *frameManager) AddScreenBackground(f Frame) {
	fm.SyncCurrentScreen()
	newScreen := &AppScreen{Frames: make([]Frame, 0, 10)}
	newScreen.Frames = append(newScreen.Frames, NewDesktop())
	newScreen.Frames = append(newScreen.Frames, f)
	fm.Screens = append(fm.Screens, newScreen)
	// Notice: We intentionally do not call fm.SwitchScreen here
}

func (fm *frameManager) CloseActiveScreen() {
	if len(fm.Screens) <= 1 {
		fm.Shutdown()
		return
	}
	fm.Screens = append(fm.Screens[:fm.ActiveIdx], fm.Screens[fm.ActiveIdx+1:]...)
	newIdx := fm.ActiveIdx
	if newIdx >= len(fm.Screens) {
		newIdx = len(fm.Screens) - 1
	}
	fm.ActiveIdx = newIdx
	fm.frames = fm.Screens[newIdx].Frames
	fm.capturedFrame = fm.Screens[newIdx].CapturedFrame
	fm.Redraw()
}

// GetActiveMenuBar returns the menu bar of the topmost frame that provides one,
// or the global MenuBar if none do.
func (fm *frameManager) GetActiveMenuBar() *MenuBar {
	for i := len(fm.frames) - 1; i >= 0; i-- {
		if mb := fm.frames[i].GetMenuBar(); mb != nil {
			return mb
		}
	}
	return fm.MenuBar
}

// Init initializes the FrameManager with a ScreenBuf.
func (fm *frameManager) Init(scr *ScreenBuf) {
	fm.scr = scr
	fm.frames = make([]Frame, 0, 10)
	fm.Screens = []*AppScreen{{Frames: fm.frames}}
	fm.ActiveIdx = 0
	fm.RedrawChan = make(chan struct{}, 1)
	fm.TaskChan = make(chan func(), 64)
	fm.injectedEvents = make([]*vtinput.InputEvent, 0)
	SetDefaultPalette()

	// Подписываемся на глобальную команду закрытия приложения
	GlobalEvents.Subscribe(EvCommand, func(e Event) {
		if cmd, ok := e.Data.(int); ok && cmd == CmQuit {
			fm.Shutdown()
		}
	})

	fm.scr.ThemePalette = &ThemePalette

	// Hide cursor globally at start
	fm.scr.SetCursorVisible(false)

	// Reset terminal palette to default to clear state from possible previous crashes
	os.Stdout.WriteString("\x1b]104\x07")
}

// Push adds a new frame to the top of the stack and assigns a number if it's non-modal.
func (fm *frameManager) Push(f Frame) {
	if !f.IsModal() && f.GetType() != TypeDesktop {
		// Find a free number from 1 to 9
		used := make(map[int]bool)
		for _, frame := range fm.frames {
			if frame.GetWindowNumber() > 0 {
				used[frame.GetWindowNumber()] = true
			}
		}
		for i := 1; i <= 9; i++ {
			if !used[i] {
				f.SetWindowNumber(i)
				break
			}
		}
	}

	if len(fm.frames) > 0 {
		fm.frames[len(fm.frames)-1].ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: false})
	}

	fm.frames = append(fm.frames, f)
	fm.SyncCurrentScreen() // Ensure the Screen object is aware of the new frame immediately
	f.ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: true})
}

// PushToFrameScreen adds a frame to the screen that contains the anchor frame.
func (fm *frameManager) PushToFrameScreen(anchor Frame, f Frame) {
	fm.SyncCurrentScreen()
	for i, s := range fm.Screens {
		for _, existing := range s.Frames {
			if existing == anchor {
				if i == fm.ActiveIdx {
					// Target is active screen, use standard Push to ensure proper focus and slice update
					fm.Push(f)
				} else {
					// Target is background screen
					s.Frames = append(s.Frames, f)
					// Initialize focus state for the new frame
					f.ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: true})
				}
				return
			}
		}
	}
	// Fallback to active screen if anchor is lost
	fm.Push(f)
}

// Flash provides visual feedback for screen transitions (fork/close).
func (fm *frameManager) Flash() {
	if fm.scr == nil {
		return
	}
	prevOverlay := fm.scr.OverlayMode
	fm.scr.SetOverlayMode(false)

	// Pure black blink
	fm.scr.FillRect(0, 0, fm.scr.width-1, fm.scr.height-1, ' ', SetRGBBoth(0, 0, 0))
	fm.scr.Flush()

	time.Sleep(30 * time.Millisecond)

	fm.scr.SetOverlayMode(prevOverlay)
	fm.Redraw()
}

// Broadcast sends a command to ALL frames in ALL screens, bypassing focus and modality.
// Returns true if at least one element handled the broadcast.
func (fm *frameManager) Broadcast(cmd int, args any) bool {
	if fm.Screens == nil {
		return false
	}
	handled := false
	for _, s := range fm.Screens {
		for i := len(s.Frames) - 1; i >= 0; i-- {
			if s.Frames[i].HandleBroadcast(cmd, args) {
				handled = true
			}
		}
	}
	if handled {
		fm.Redraw()
	}
	return handled
}

// RequestFocus moves the given frame to the top of the stack (brings it to front).
// Returns false if a modal dialog blocks the focus change.
func (fm *frameManager) RequestFocus(f Frame) bool {
	// If there's a modal dialog on top, we cannot change focus
	for i := len(fm.frames) - 1; i >= 0; i-- {
		if fm.frames[i] == f {
			break
		}
		if fm.frames[i].IsModal() {
			return false
		}
	}

	idx := -1
	for i, frame := range fm.frames {
		if frame == f {
			idx = i
			break
		}
	}

	if idx == -1 || idx == len(fm.frames)-1 {
		return true // Already on top or not found
	}

	// Tell current top frame it lost focus
	fm.frames[len(fm.frames)-1].ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: false})

	// Move the frame to the end of the slice
	fm.frames = append(fm.frames[:idx], fm.frames[idx+1:]...)
	fm.frames = append(fm.frames, f)

	// Tell new top frame it got focus
	f.ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: true})

	fm.Redraw()
	return true
}

// Pop removes the top-most frame from the stack.
func (fm *frameManager) Pop() {
	if len(fm.frames) > 0 {
		top := fm.frames[len(fm.frames)-1]
		if fm.capturedFrame == top {
			fm.capturedFrame = nil
		}
		fm.frames = fm.frames[:len(fm.frames)-1]
		if len(fm.frames) > 0 {
			fm.frames[len(fm.frames)-1].ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: true})
		}
	}
}
// RemoveFrame deletes a specific frame from the stack, regardless of its position.
func (fm *frameManager) RemoveFrame(f Frame) {
	isTop := len(fm.frames) > 0 && fm.frames[len(fm.frames)-1] == f
	for i, frame := range fm.frames {
		if frame == f {
			if fm.capturedFrame == f {
				fm.capturedFrame = nil
			}
			fm.frames = append(fm.frames[:i], fm.frames[i+1:]...)
			fm.SyncCurrentScreen() // Critical: update the slice header in Screens array
			if isTop && len(fm.frames) > 0 {
				fm.frames[len(fm.frames)-1].ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: true})
			}
			return
		}
	}
}
// Redraw triggers an asynchronous redraw request.
func (fm *frameManager) Redraw() {
	select {
	case fm.RedrawChan <- struct{}{}:
	default:
	}
}
// PostTask schedules a function to be executed safely on the main UI thread.
func (fm *frameManager) PostTask(task func()) {
	if fm.TaskChan != nil {
		fm.TaskChan <- task
	}
}
// EmitCommand broadcasts a command starting from the top-most frame
// and going down the stack until a frame handles it. (Turbo Vision style)
func (fm *frameManager) EmitCommand(cmd int, args any) bool {
	if fm.DisabledCommands.IsDisabled(cmd) {
		DebugLog("COMMAND: %d is DISABLED, ignoring", cmd)
		return false
	}
	DebugLog("COMMAND: Emitting %d", cmd)
	// First, if MenuBar is active, give it a chance
	activeMenu := fm.GetActiveMenuBar()
	if activeMenu != nil && activeMenu.Active {
		if activeMenu.HandleCommand(cmd, args) {
			DebugLog("COMMAND: Handled by MenuBar")
			return true
		}
	}

	// Route down the frame stack
	for i := len(fm.frames) - 1; i >= 0; i-- {
		DebugLog("COMMAND: Checking frame %d (type %d)", i, fm.frames[i].GetType())
		if fm.frames[i].HandleCommand(cmd, args) {
			DebugLog("COMMAND: Handled by frame %d", i)
			fm.Redraw()
			return true
		}
	}
	DebugLog("COMMAND: No one handled %d", cmd)
	return false
}

// InjectEvents adds simulated input events to the front of the queue.
func (fm *frameManager) InjectEvents(events []*vtinput.InputEvent) {
	fm.injectedEvents = append(fm.injectedEvents, events...)
}

// Shutdown clears all frames, effectively stopping the application loop.
func (fm *frameManager) Shutdown() {
	fm.Screens = nil
	fm.frames = nil
	fm.capturedFrame = nil
}
// IsShutdown returns true if the FrameManager has been shut down explicitly.
func (fm *frameManager) IsShutdown() bool {
	return fm.Screens == nil
}

// CycleWindows updates the selection in the switcher overlay
func (fm *frameManager) CycleWindows(forward bool) bool {
	if len(fm.Screens) < 2 {
		return false
	}

	if !fm.switcherActive {
		fm.switcherActive = true
		fm.switcherIdx = fm.ActiveIdx
	}

	if forward {
		fm.switcherIdx = (fm.switcherIdx + 1) % len(fm.Screens)
	} else {
		fm.switcherIdx = (fm.switcherIdx - 1 + len(fm.Screens)) % len(fm.Screens)
	}
	fm.Redraw()
	return true
}

func (fm *frameManager) renderSwitcher(scr *ScreenBuf) {
	if !fm.switcherActive || len(fm.Screens) < 2 { return }
	
	menuW := 60
	menuH := len(fm.Screens) + 2
	x := (scr.width - menuW) / 2
	y := (scr.height - menuH) / 2
	
	attr := Palette[ColMenuText]
	selAttr := Palette[ColMenuSelectedText]
	boxAttr := Palette[ColMenuBox]
	attnAttr := SetRGBBoth(0, 0xFFFFFF, 0xFF0000)

	scr.FillRect(x, y, x+menuW-1, y+menuH-1, ' ', attr)
	sym := getBoxSymbols(DoubleBox)
	scr.Write(x, y, StringToCharInfo(string(sym[bsTL])+strings.Repeat(string(sym[bsH]), menuW-2)+string(sym[bsTR]), boxAttr))
	scr.Write(x, y+menuH-1, StringToCharInfo(string(sym[bsBL])+strings.Repeat(string(sym[bsH]), menuW-2)+string(sym[bsBR]), boxAttr))
	for i := 1; i < menuH-1; i++ {
		scr.Write(x, y+i, StringToCharInfo(string(sym[bsV]), boxAttr))
		scr.Write(x+menuW-1, y+i, StringToCharInfo(string(sym[bsV]), boxAttr))
	}

	maxTitleLen := menuW - 19
	for i := range fm.Screens {
		itemAttr := attr
		if i == fm.switcherIdx { itemAttr = selAttr }
		
		pre, tit, suf, needsAttn := fm.getScreenInfo(i, maxTitleLen)
		if i == fm.switcherIdx { pre = "> " }

		rowText := pre + tit + suf
		scr.Write(x+1, y+1+i, StringToCharInfo(rowText+strings.Repeat(" ", menuW-2-len([]rune(rowText))), itemAttr))
		if needsAttn {
			scr.Write(x+1, y+1+i, StringToCharInfo("!", attnAttr))
		}
	}
}

func (fm *frameManager) getScreenInfo(idx int, maxTitleLen int) (prefix, title, suffix string, needsAttn bool) {
	s := fm.Screens[idx]
	rawTitle := s.GetTitle()
	needsAttn = s.NeedsAttention()
	isCurrent := (idx == fm.ActiveIdx)

	prefix = "  "
	if isCurrent && needsAttn {
		prefix = "? "
	} else if isCurrent {
		prefix = "* "
	} else if needsAttn {
		prefix = "! "
	}

	suffix = ""
	if p := s.GetProgress(); p >= 0 {
		barLen := 10
		filled := (p * barLen) / 100
		bar := "["
		for b := 0; b < barLen; b++ {
			if b < filled { bar += "#" } else { bar += "." }
		}
		suffix = " " + bar + "]"
	}

	title = TruncateMiddle(rawTitle, maxTitleLen)
	return
}

func (fm *frameManager) showScreensMenu() {
	fm.SyncCurrentScreen()
	menu := NewVMenu(" Screens ")

	scrW := fm.GetScreenSize()
	scrH := 25
	if fm.scr != nil { scrH = fm.scr.height }

	menuW := (scrW * 60) / 100
	if menuW < 40 { menuW = 40 }
	if menuW > 100 { menuW = 100 }

	maxTitleLen := menuW - 19
	if maxTitleLen < 10 { maxTitleLen = 10 }

	for i := range fm.Screens {
		pre, tit, suf, _ := fm.getScreenInfo(i, maxTitleLen)
		menu.AddItem(MenuItem{Text: pre + tit + suf, UserData: i})
	}

	menu.OnSelect = func(idx int) {
		fm.SwitchScreen(idx)
	}

	menuH := len(fm.Screens) + 2
	if menuH > 15 { menuH = 15 }
	x := (scrW - menuW) / 2
	y := (scrH - menuH) / 2
	menu.SetPosition(x, y, x+menuW-1, y+menuH-1)
	fm.Push(menu)
}

func (fm *frameManager) cleanupDoneFrames() {
	fm.SyncCurrentScreen()

	for sIdx := len(fm.Screens) - 1; sIdx >= 0; sIdx-- {
		s := fm.Screens[sIdx]
		wasModified := false
		for i := len(s.Frames) - 1; i >= 0; i-- {
			if s.Frames[i].IsDone() {
				if s.CapturedFrame == s.Frames[i] { s.CapturedFrame = nil }
				s.Frames = append(s.Frames[:i], s.Frames[i+1:]...)
				wasModified = true
				DebugLog("FM: Frame removed from Screen %d. Remaining: %d", sIdx, len(s.Frames))
			}
		}

		// Экран считается мертвым, если:
		// 1. В нем вообще нет фреймов.
		// 2. В нем остался только Desktop, и МЫ ТОЛЬКО ЧТО закрыли в нем
		//    последнее окно (wasModified), и это НЕ единственный экран.
		isDead := len(s.Frames) == 0
		if !isDead && wasModified && len(s.Frames) == 1 && s.Frames[0].GetType() == TypeDesktop && len(fm.Screens) > 1 {
			isDead = true
		}

		if isDead && len(fm.Screens) > 1 {
			DebugLog("FM: Closing dead Screen %d (Active was %d)", sIdx, fm.ActiveIdx)
			fm.Screens = append(fm.Screens[:sIdx], fm.Screens[sIdx+1:]...)
			if fm.ActiveIdx >= sIdx && fm.ActiveIdx > 0 {
				fm.ActiveIdx--
			}
		}
	}

	if len(fm.Screens) > 0 {
		if fm.ActiveIdx >= len(fm.Screens) {
			fm.ActiveIdx = len(fm.Screens) - 1
		}
		fm.frames = fm.Screens[fm.ActiveIdx].Frames
		fm.capturedFrame = fm.Screens[fm.ActiveIdx].CapturedFrame
	} else {
		fm.Shutdown()
	}
}
func (fm *frameManager) cleanupOrphanedMenus() {
	activeMenu := fm.GetActiveMenuBar()
	if activeMenu != nil && !activeMenu.Active && activeMenu.activeSubMenu != nil {
		activeMenu.closeSub()
	}
}
// GetTopFrameType returns the type of the topmost frame or -1 if empty.
func (fm *frameManager) GetTopFrameType() FrameType {
	if len(fm.frames) == 0 {
		DebugLog("FRAMEWORK: GetTopFrameType(), len(fm.frames) == 0")
		return -1
	}
	return fm.frames[len(fm.frames)-1].GetType()
}
func (fm *frameManager) GetTopFrame() Frame {
	if len(fm.frames) == 0 {
		return nil
	}
	return fm.frames[len(fm.frames)-1]
}

func (fm *frameManager) GetScreenSize() int {
	if fm.scr == nil { return 80 }
	return fm.scr.width
}

// Stop signals the main loop to exit.
func (fm *frameManager) Stop() {
	DebugLog("FM: Stop() requested. Deactivating menus and exiting loop.")
	// Safety: deactivate top menu before leaving to avoid stale sub-menus on return
	if fm.MenuBar != nil {
		fm.MenuBar.Active = false
	}
	fm.running = false
	// Wake up the select loop immediately
	select {
	case fm.RedrawChan <- struct{}{}:
	default:
	}
}

// Run starts the main application event loop.
func (fm *frameManager) Run(reader *vtinput.Reader) {
	fm.running = true
	// Restore cursor visibility on exit
	defer func() {
		if r := recover(); r != nil {
			DebugLog("FATAL PANIC in FrameManager: %v\n%s", r, debug.Stack())
		}
		fm.running = false
		fm.scr.SetCursorVisible(true)
		fm.scr.Flush()
	}()

	eventChan := make(chan *vtinput.InputEvent, 1)
	stopChan := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopChan:
				return
			default:
				e, err := reader.ReadEvent()
				if err != nil {
					close(eventChan)
					return
				}
				eventChan <- e
			}
		}
	}()
	defer close(stopChan)

	// Configure channel for tracking window resizing
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGWINCH)

	// --- Main application loop ---
	// Persistent timer to avoid allocations in the drain loop
	idleTimer := time.NewTimer(time.Hour)
	if !idleTimer.Stop() {
		select {
		case <-idleTimer.C:
		default:
		}
	}
	DebugLog("FM: Entering Run loop. MenuBar set: %v, KeyBar set: %v", fm.MenuBar != nil, fm.KeyBar != nil)
	for fm.running {
		if len(fm.frames) == 0 {
			DebugLog("FM: No frames left, exiting Run loop.")
			return
		}

		// 1. Rendering
		if len(fm.frames) == 0 {
			return
		}
		topFrame := fm.frames[len(fm.frames)-1]

		// Update global status line context automatically
		if fm.StatusLine != nil {
			topic := ""
			// Priority: Focused item's help -> Frame's help -> Menu context
			if fm.MenuBar != nil && fm.MenuBar.Active {
				topic = "menu"
			} else {
				if dlg, ok := topFrame.(*Dialog); ok {
					if foc := dlg.GetFocusedItem(); foc != nil {
						topic = foc.GetHelp()
					}
				}
				if topic == "" {
					topic = topFrame.GetHelp()
				}
			}
			fm.StatusLine.UpdateContext(topic)
		}

		// Update KeyBar content from the active frame
		if fm.KeyBar != nil {
			// Find the topmost frame that provides key labels
			for i := len(fm.frames) - 1; i >= 0; i-- {
				if ks := fm.frames[i].GetKeyLabels(); ks != nil {
					fm.KeyBar.Normal = ks.Normal
					fm.KeyBar.Shift = ks.Shift
					fm.KeyBar.Ctrl = ks.Ctrl
					fm.KeyBar.Alt = ks.Alt
					break
				}
			}
		}

		// If the frame is "busy" (e.g., mass insertion in progress), skip drawing
		// and Flush to avoid flickering and save CPU.
		if !topFrame.IsBusy() {
			// Cleanup orphaned menus safely outside the frames iteration loop
			// to avoid "index out of range" during rendering.
			fm.cleanupOrphanedMenus()

			fm.scr.SetCursorVisible(false)
			fm.scr.ActivePalette = nil
			// By default, we use OverlayMode (Early Binding) for host UI elements.
			// Desktop and TerminalView will explicitly disable it during their render.
			fm.scr.SetOverlayMode(true)

			// 1. Находим "базовый" экран (первый непрозрачный, идя назад от активного)
			baseIdx := fm.ActiveIdx
			for baseIdx > 0 && fm.Screens[baseIdx].Transparent {
				baseIdx--
			}

			// 2. Отрисовываем стэк экранов от базового до текущего
			for sIdx := baseIdx; sIdx <= fm.ActiveIdx; sIdx++ {
				frames := fm.GetActiveFrames(sIdx)
				for _, frame := range frames {
					if frame.HasShadow() {
						x1, y1, x2, y2 := frame.GetPosition()
						isFullScreen := x1 <= 0 && y1 <= 0 && x2 >= fm.scr.width-1 && y2 >= fm.scr.height-1
						if !isFullScreen {
							fm.scr.ApplyShadow(x1+2, y2+1, x2+2, y2+1)
							fm.scr.ApplyShadow(x2+1, y1+1, x2+2, y2)
						}
					}
					frame.Show(fm.scr)
				}
			}

			fm.renderSwitcher(fm.scr)

			// Render Standard Global UI
			// Render Standard Global UI
			activeMenu := fm.GetActiveMenuBar()
			if activeMenu != nil && activeMenu.Active {
				activeMenu.Show(fm.scr)
			}
			if fm.KeyBar != nil {
				fm.KeyBar.Show(fm.scr)
			}
			if fm.StatusLine != nil {
				fm.StatusLine.Show(fm.scr)
			}

			if fm.OnRender != nil {
				fm.OnRender(fm.scr)
			}

			// Draw Global Attention Indicator [!] if background screens need input
			hasHiddenAttention := false
			for i, s := range fm.Screens {
				if i != fm.ActiveIdx && s.NeedsAttention() {
					hasHiddenAttention = true
					break
				}
			}
			if hasHiddenAttention {
				attr := SetRGBBoth(0, 0xFFFFFF, 0xFF0000) // White on Red
				fm.scr.Write(fm.scr.width-3, 0, StringToCharInfo("[!]", attr))
			}

			fm.scr.Flush()
		}

		// 2. Dispatch helper
		dispatch := func(ev *vtinput.InputEvent, is_injected bool) {
			if len(fm.frames) == 0 {
				return
			}

			// Generate DoubleClick flag from sequence of clicks
			if ev.Type == vtinput.MouseEventType && ev.ButtonState != 0 && ev.KeyDown {
				now := time.Now()
				if ev.ButtonState == fm.lastMouseButton && int(ev.MouseX) == fm.lastMouseX && int(ev.MouseY) == fm.lastMouseY && now.Sub(fm.lastMouseClickTime) < 400*time.Millisecond {
					ev.MouseEventFlags |= vtinput.DoubleClick
					fm.lastMouseButton = 0 // prevent triple click
					DebugLog("FM: DoubleClick generated at (%d,%d)", ev.MouseX, ev.MouseY)
				} else {
					fm.lastMouseButton = ev.ButtonState
					fm.lastMouseX = int(ev.MouseX)
					fm.lastMouseY = int(ev.MouseY)
					fm.lastMouseClickTime = now
				}
			}

			topFrame := fm.frames[len(fm.frames)-1]
			activeMenu := fm.GetActiveMenuBar()

			// Update KeyBar modifiers automatically if present
			if fm.KeyBar != nil {
				shift := (ev.ControlKeyState & vtinput.ShiftPressed) != 0
				ctrl := (ev.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)) != 0
				alt := (ev.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0
				fm.KeyBar.SetModifiers(shift, ctrl, alt)
			}

			// User-defined filter has first say
			if !is_injected && fm.EventFilter != nil && fm.EventFilter(ev) {
				return
			}

			// Track Ctrl state for Switcher logic
			if ev.Type == vtinput.KeyEventType {
				fm.ctrlPressed = (ev.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)) != 0

				// Commit Switcher selection on Ctrl release
				if !fm.ctrlPressed && fm.switcherActive {
					fm.switcherActive = false
					fm.SwitchScreen(fm.switcherIdx)
				}
			}

			// --- Menu Interception ---
			if ev.Type == vtinput.KeyEventType && ev.KeyDown {
				// 1. If Menu is Active, it has priority.
				// We allow it even if topFrame is modal, provided topFrame IS the menu itself
				// or the frame that owns the menu.
				isMenuRelated := topFrame.GetType() == TypeMenu || topFrame.GetMenuBar() == activeMenu
				if activeMenu != nil && activeMenu.Active && (!topFrame.IsModal() || isMenuRelated) {
					// Exception: if a VMenu is open, it MUST handle navigation keys
					if fm.GetTopFrameType() == TypeMenu {
						menuFrame := fm.frames[len(fm.frames)-1]
						if menuFrame.ProcessKey(ev) {
							if menuFrame.IsDone() {
								fm.RemoveFrame(menuFrame)
							}
							return
						}
					}
					// Otherwise, MenuBar processes keys (Arrows, Esc, Hotkeys)
					if ev.VirtualKeyCode == vtinput.VK_ESCAPE || ev.VirtualKeyCode == vtinput.VK_F10 {
						activeMenu.Active = false
						return
					}
					if activeMenu.ProcessKey(ev) {
						return
					}
					return // Don't pass keys to background frames when menu is active
				}
			}

			// 3. Regular Dispatch (MDI Hit-Testing)
			handled := false

			if ev.Type == vtinput.KeyEventType || ev.Type == vtinput.PasteEventType || ev.Type == vtinput.FocusEventType {
				handled = topFrame.ProcessKey(ev)
			} else if ev.Type == vtinput.MouseEventType {
				mx, my := int(ev.MouseX), int(ev.MouseY)
				if ev.ButtonState != 0 || ev.WheelDirection != 0 {
					DebugLog("FM: Mouse Event at (%d,%d) State:%X Wheel:%d", mx, my, ev.ButtonState, ev.WheelDirection)
				}

				// 3.1. Active Mouse Capture (Dragging/Resizing)
				if fm.capturedFrame != nil {
					handled = fm.capturedFrame.ProcessMouse(ev)
					if ev.ButtonState == 0 {
						fm.capturedFrame = nil // Release capture
					}
				} else {
					// 3.2. Mouse Hit-Testing: check frames from top to bottom
					for i := len(fm.frames) - 1; i >= 0; i-- {
						f := fm.frames[i]

						// Desktop always gets mouse if nothing above it handled it
						if f.GetType() == TypeDesktop {
							handled = f.ProcessMouse(ev)
							if handled && ev.ButtonState != 0 {
								fm.capturedFrame = f
							}
							break
						}

						x1, y1, x2, y2 := f.GetPosition()
						if mx >= x1 && mx <= x2+2 && my >= y1 && my <= y2+1 {
							DebugLog("FM: Hit-test SUCCESS for frame %d type %d. Pos: (%d,%d)-(%d,%d)", i, f.GetType(), x1, y1, x2, y2)
							// Click is within this frame (or its shadow)
							if i != len(fm.frames)-1 {
								// Try to bring it to front before passing the event
								if fm.RequestFocus(f) {
									handled = f.ProcessMouse(ev)
								}
							} else {
								handled = f.ProcessMouse(ev)
							}

							// If a frame handled a click, it captures the mouse until release
							if handled && ev.ButtonState != 0 {
								fm.capturedFrame = f
							}

							// If the frame is modal, it eats the click even if it didn't handle it
							// (to prevent clicking on windows behind it)
							if f.IsModal() || handled {
								break
							}
						} else {
							// For troubleshooting sizing issues
							if ev.ButtonState != 0 {
								// DebugLog("FM: Hit-test miss for frame %d type %d. Bounds: (%d,%d)-(%d,%d)", i, f.GetType(), x1, y1, x2, y2)
							}
						}

						if f.IsModal() {
							break
						}
					}
				}
			}

			// 3. Fallbacks (F9, Alt+Hotkey, Global Shortcuts) if top frame didn't want the key
			if !handled && ev.Type == vtinput.KeyEventType && ev.KeyDown {
				// Global Quit (standard for vtui tools)
				if ev.VirtualKeyCode == vtinput.VK_Q && fm.ctrlPressed {
					fm.Shutdown()
					return
				}

				// Window Cycling (Ctrl+Tab / Ctrl+Shift+Tab)
				if ev.VirtualKeyCode == vtinput.VK_TAB && (fm.ctrlPressed || fm.switcherActive) {
					shift := (ev.ControlKeyState & vtinput.ShiftPressed) != 0
					// Only consume the event if cycling is actually possible
					if fm.CycleWindows(!shift) {
						return
					}
				}

				// Ctrl+N - Fork Active Frame into new Screen
				if ev.VirtualKeyCode == vtinput.VK_N && fm.ctrlPressed {
					fm.Flash()
					// We need a way to clone the top-level frame.
					// For now, let's trigger a Command that Panels can handle.
					fm.EmitCommand(CmResize, "fork") // Temporary hack or use specialized Command
					return
				}

				// Ctrl+W - Close Active Screen
				if ev.VirtualKeyCode == vtinput.VK_W && fm.ctrlPressed {
					fm.Flash()
					fm.CloseActiveScreen()
					return
				}

				// F12 - Screens Menu (Window List)
				// We must ignore NumLock, CapsLock, and EnhancedKey flags
				modifierMask := uint32(vtinput.LeftAltPressed | vtinput.RightAltPressed | vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed | vtinput.ShiftPressed)
				if ev.VirtualKeyCode == vtinput.VK_F1 && (ev.ControlKeyState&modifierMask) == 0 {
					topic := topFrame.GetHelp()
					if dlg, ok := topFrame.(*Dialog); ok {
						if foc := dlg.GetFocusedItem(); foc != nil && foc.GetHelp() != "" {
							topic = foc.GetHelp()
						}
					}
					if topic != "" && GlobalHelpEngine != nil {
						hv := NewHelpView(GlobalHelpEngine, topic)
						fm.Push(hv)
						return
					}
				}
				if ev.VirtualKeyCode == vtinput.VK_F12 && (ev.ControlKeyState&modifierMask) == 0 {
					if fm.GetTopFrameType() != TypeMenu {
						fm.showScreensMenu()
						return
					}
				}

				// Allow F9 if not modal, OR if the modal frame itself has a menu
				canActivateMenu := !topFrame.IsModal() || topFrame.GetMenuBar() != nil
				if ev.VirtualKeyCode == vtinput.VK_F9 {
					if activeMenu == nil {
						DebugLog("FM: F9 pressed but activeMenu is NIL")
					} else if activeMenu.Active {
						DebugLog("FM: F9 pressed but Menu is already active")
					} else if !canActivateMenu {
						DebugLog("FM: F9 pressed but Menu activation blocked by modal frame")
					} else {
						DebugLog("FM: F9 accepted, activating menu")
						activeMenu.Active = true
						if len(activeMenu.Items) > 0 {
							if activeMenu.SelectPos < 0 || activeMenu.SelectPos >= len(activeMenu.Items) {
								activeMenu.SelectPos = 0
							}
							activeMenu.ActivateSubMenu(activeMenu.SelectPos)
						}
						return
					}
				}
				if activeMenu != nil && !activeMenu.Active && canActivateMenu {
					alt := (ev.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0
					if alt && ev.Char != 0 {
						if activeMenu.ProcessKey(ev) {
							return
						}
					}
				}
			}

			// 4. Cleanup: Remove all frames that are marked as done.
			fm.cleanupDoneFrames()
		}

		// 3. Event waiting (Blocking)
		var e *vtinput.InputEvent
		injected := false
		loopAgain := false

		if len(fm.injectedEvents) > 0 {
			e = fm.injectedEvents[0]
			fm.injectedEvents = fm.injectedEvents[1:]
			injected = true
		} else {
			select {
			case <-fm.RedrawChan:
				loopAgain = true
			case task := <-fm.TaskChan:
				task()
				fm.cleanupDoneFrames()
				fm.Redraw()
				loopAgain = true
			case <-sigChan:
				width, height, _ := term.GetSize(int(os.Stdin.Fd()))
				fm.scr.AllocBuf(width, height)
				for _, f := range fm.frames {
					f.ResizeConsole(width, height)
				}
				fm.Redraw()
				loopAgain = true
			case ev, ok := <-eventChan:
				if !ok {
					DebugLog("FM: eventChan closed, exiting Run() // in Event waiting (Blocking)")
					return
				}
				e = ev
			}
		}

		if loopAgain {
			continue
		}

		if e.Type == vtinput.KeyEventType && e.KeyDown {
			DebugLog("KEY: Vk=%X Char=%d ActiveFrames=%d", e.VirtualKeyCode, e.Char, len(fm.frames))
		}

		dispatch(e, injected)

		// 4. Queue "Drain"
		// Burst process pending events (e.g. fast typing or paste)
		for fm.running && !fm.IsShutdown() {
			idleTimer.Reset(2 * time.Millisecond)
			select {
			case ev, ok := <-eventChan:
				if !idleTimer.Stop() {
					select { case <-idleTimer.C: default: }
				}
				if !ok { return }

				// If previous event in this burst closed the window/app, stop immediately.
				if len(fm.frames) > 0 {
					dispatch(ev, false)
				}
				continue
			case <-idleTimer.C:
			}
			break
		}
	}
}
