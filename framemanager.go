package vtui

import (
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	// MDI Methods
	SetPosition(x1, y1, x2, y2 int)
	GetPosition() (x1, y1, x2, y2 int)
	IsModal() bool
	GetWindowNumber() int
	SetWindowNumber(n int)
	RequestFocus() bool
	Close()
}

// frameManager manages the stack of frames and the main application loop.
type frameManager struct {
	frames         []Frame
	scr            *ScreenBuf
	RedrawChan     chan struct{}
	TaskChan       chan func()
	EventFilter    func(*vtinput.InputEvent) bool
	injectedEvents []*vtinput.InputEvent
	OnRender       func(scr *ScreenBuf)

	// Global standard UI components
	MenuBar    *MenuBar
	StatusLine *StatusLine
	KeyBar     *KeyBar

	capturedFrame Frame // Frame that currently captures mouse events (during drag/resize)
}

// FrameManager is the global instance of the frame manager.
var FrameManager = &frameManager{}

// Init initializes the FrameManager with a ScreenBuf.
func (fm *frameManager) Init(scr *ScreenBuf) {
	fm.scr = scr
	fm.frames = make([]Frame, 0, 10)
	fm.RedrawChan = make(chan struct{}, 1)
	fm.TaskChan = make(chan func(), 64)
	fm.injectedEvents = make([]*vtinput.InputEvent, 0)
	SetDefaultPalette()

	// Подписываемся на глобальную команду закрытия приложения
	GlobalEvents.Subscribe(EvCommand, func(e Event) {
		if cmd, ok := e.Data.(int); ok && cmd == cmQuit {
			fm.Shutdown()
		}
	})

	// Hide cursor globally at start
	fm.scr.SetCursorVisible(false)
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
	f.ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: true})
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
// InjectEvents adds simulated input events to the front of the queue.
func (fm *frameManager) InjectEvents(events []*vtinput.InputEvent) {
	fm.injectedEvents = append(fm.injectedEvents, events...)
}
// Shutdown clears all frames, effectively stopping the application loop.
func (fm *frameManager) Shutdown() {
	fm.frames = nil
	fm.capturedFrame = nil
}
// GetTopFrameType returns the type of the topmost frame or -1 if empty.
func (fm *frameManager) GetTopFrameType() FrameType {
	if len(fm.frames) == 0 {
		return -1
	}
	return fm.frames[len(fm.frames)-1].GetType()
}

func (fm *frameManager) GetScreenSize() int {
	if fm.scr == nil { return 80 }
	return fm.scr.width
}

// Run starts the main application event loop.
func (fm *frameManager) Run() {
	// Restore cursor visibility on exit
	defer func() {
		fm.scr.SetCursorVisible(true)
		fm.scr.Flush()
	}()
	// Configure channel for receiving vtinput events
	reader := vtinput.NewReader(os.Stdin)
	eventChan := make(chan *vtinput.InputEvent, 1)
	go func() {
		for {
			e, err := reader.ReadEvent()
			if err != nil {
				if err != io.EOF {
					// TODO: Log error
				}
				close(eventChan)
				return
			}
			eventChan <- e
		}
	}()

	// Configure channel for tracking window resizing
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGWINCH)

	// --- Main application loop ---
	for {
		if len(fm.frames) == 0 {
			return // No more frames, exit application.
		}

		// 1. Rendering
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
			fm.scr.SetCursorVisible(false)

			// Render frames from bottom to top (Painter's algorithm)
			for i, frame := range fm.frames {
				if frame.HasShadow() {
					x1, y1, x2, y2 := frame.GetPosition()
					// Fullscreen check
					if i > 0 && (x1 > 0 || y1 > 0 || x2 < fm.scr.width-1 || y2 < fm.scr.height-1) {
						fm.scr.ApplyShadow(x1+2, y2+1, x2+2, y2+1)
						fm.scr.ApplyShadow(x2+1, y1+1, x2+2, y2)
					}
				}
				frame.Show(fm.scr)
			}

			// Render Standard Global UI
			if fm.MenuBar != nil && fm.MenuBar.Active {
				fm.MenuBar.Show(fm.scr)
			}
			if fm.StatusLine != nil {
				fm.StatusLine.Show(fm.scr)
			}

			if fm.OnRender != nil {
				fm.OnRender(fm.scr)
			}
			fm.scr.Flush()
		}

		// 2. Dispatch helper
		dispatch := func(ev *vtinput.InputEvent, is_injected bool) {
			if len(fm.frames) == 0 { return }

			// Update KeyBar modifiers automatically if present
			if fm.KeyBar != nil {
				shift := (ev.ControlKeyState & vtinput.ShiftPressed) != 0
				ctrl := (ev.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)) != 0
				alt := (ev.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0
				fm.KeyBar.SetModifiers(shift, ctrl, alt)
			}

			// User-defined filter has first say
			if !is_injected && fm.EventFilter != nil && fm.EventFilter(ev) { return }

			// --- Standard Framework Event Logic ---
			if ev.Type == vtinput.KeyEventType && ev.KeyDown {
				// Global Quit (standard for vtui tools)
				if ev.VirtualKeyCode == vtinput.VK_Q && (ev.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed)) != 0 {
					fm.frames = nil
					return
				}

				// Menu Toggle (F9)
				if fm.MenuBar != nil && ev.VirtualKeyCode == vtinput.VK_F9 {
					fm.MenuBar.Active = !fm.MenuBar.Active
					return
				}

				// 1. If Menu is Active, it has priority
				if fm.MenuBar != nil && fm.MenuBar.Active {
					// Exception: if a VMenu is open, it MUST handle navigation keys
					if fm.GetTopFrameType() == TypeMenu {
						menuFrame := fm.frames[len(fm.frames)-1]
						if menuFrame.ProcessKey(ev) {
							if menuFrame.IsDone() { fm.RemoveFrame(menuFrame) }
							return
						}
					}
					// Otherwise, MenuBar processes keys (Arrows, Esc, Hotkeys)
					if ev.VirtualKeyCode == vtinput.VK_ESCAPE || ev.VirtualKeyCode == vtinput.VK_F10 {
						fm.MenuBar.Active = false
						return
					}
					if fm.MenuBar.ProcessKey(ev) { return }
					return // Modal-like: don't pass keys to dialog when menu active
				}
			}

			// 3. Regular Dispatch (MDI Hit-Testing)
			topFrame := fm.frames[len(fm.frames)-1]
			handled := false

			if ev.Type == vtinput.KeyEventType {
				handled = topFrame.ProcessKey(ev)
			} else if ev.Type == vtinput.MouseEventType {
				mx, my := int(ev.MouseX), int(ev.MouseY)

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
						} else if f.IsModal() {
							// Clicked outside a modal dialog. We must NOT pass it to windows below.
							// Optionally, we could emit a beep here.
							break
						}
					}
				}
			}

			// 3. Fallback to Menu Activation (Alt+Hotkey) if top frame didn't want the key
			if !handled && fm.MenuBar != nil && !fm.MenuBar.Active && ev.Type == vtinput.KeyEventType {
				alt := (ev.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0
				if alt && ev.Char != 0 {
					if fm.MenuBar.ProcessKey(ev) { return }
				}
			}

			if topFrame.IsDone() { fm.RemoveFrame(topFrame) }
		}

		// 3. Event waiting (Blocking)
		var e *vtinput.InputEvent
		injected := false

		if len(fm.injectedEvents) > 0 {
			e = fm.injectedEvents[0]
			fm.injectedEvents = fm.injectedEvents[1:]
			injected = true
		} else {
			select {
			case <-fm.RedrawChan: continue
			case task := <-fm.TaskChan:
				task()
				fm.Redraw()
				continue
			case <-sigChan:
				width, height, _ := term.GetSize(int(os.Stdin.Fd()))
				fm.scr.AllocBuf(width, height)
				for _, f := range fm.frames { f.ResizeConsole(width, height) }
				continue
			case ev, ok := <-eventChan:
				if !ok { return }
				e = ev
			}
		}

		// Process the first received event
		dispatch(e, injected)

		// 4. Queue "Drain"
		// If events arrive in a dense stream (insertion), process them in a batch.
		for {
			select {
			case ev, ok := <-eventChan:
				if !ok { return }
				dispatch(ev, false)
				continue
			case <-time.After(2 * time.Millisecond):
				// If nothing arrived within 2ms, consider the burst finished.
				// This is critical for "instant" Bracketed Paste, because the terminal
				// sends data in chunks, and a normal drain may be interrupted too early.
			}
			break
		}
	}
}
