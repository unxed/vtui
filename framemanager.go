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
}

// frameManager manages the stack of frames and the main application loop.
type frameManager struct {
	frames         []Frame
	scr            *ScreenBuf
	RedrawChan     chan struct{}
	EventFilter    func(*vtinput.InputEvent) bool
	injectedEvents []*vtinput.InputEvent
	OnRender       func(scr *ScreenBuf)

	// Global standard UI components
	MenuBar    *MenuBar
	StatusLine *StatusLine
	KeyBar     *KeyBar
}

// FrameManager is the global instance of the frame manager.
var FrameManager = &frameManager{}

// Init initializes the FrameManager with a ScreenBuf.
func (fm *frameManager) Init(scr *ScreenBuf) {
	fm.scr = scr
	fm.frames = make([]Frame, 0, 10)
	fm.RedrawChan = make(chan struct{}, 1)
	fm.injectedEvents = make([]*vtinput.InputEvent, 0)
	SetDefaultPalette()
	// Hide cursor globally at start
	fm.scr.SetCursorVisible(false)
}

// Push adds a new frame to the top of the stack.
func (fm *frameManager) Push(f Frame) {
	fm.frames = append(fm.frames, f)
}

// Pop removes the top-most frame from the stack.
func (fm *frameManager) Pop() {
	if len(fm.frames) > 0 {
		fm.frames = fm.frames[:len(fm.frames)-1]
	}
}
// Redraw triggers an asynchronous redraw request.
func (fm *frameManager) Redraw() {
	select {
	case fm.RedrawChan <- struct{}{}:
	default:
	}
}
// InjectEvents adds simulated input events to the front of the queue.
func (fm *frameManager) InjectEvents(events []*vtinput.InputEvent) {
	fm.injectedEvents = append(fm.injectedEvents, events...)
}
// Shutdown clears all frames, effectively stopping the application loop.
func (fm *frameManager) Shutdown() {
	fm.frames = nil
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

		// If the frame is "busy" (e.g., mass insertion in progress), skip drawing
		// and Flush to avoid flickering and save CPU.
		if !topFrame.IsBusy() {
			fm.scr.SetCursorVisible(false)
			for _, frame := range fm.frames {
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
						if fm.frames[len(fm.frames)-1].ProcessKey(ev) {
							if fm.frames[len(fm.frames)-1].IsDone() { fm.Pop() }
							return
						}
					}
					// Otherwise, MenuBar processes keys (Arrows, Esc, Hotkeys)
					if ev.VirtualKeyCode == vtinput.VK_ESCAPE {
						fm.MenuBar.Active = false
						return
					}
					if fm.MenuBar.ProcessKey(ev) { return }
					return // Modal-like: don't pass keys to dialog when menu active
				}
			}

			// 2. Regular Dispatch to Top Frame
			topFrame := fm.frames[len(fm.frames)-1]
			handled := false
			if ev.Type == vtinput.KeyEventType {
				handled = topFrame.ProcessKey(ev)
			} else if ev.Type == vtinput.MouseEventType {
				handled = topFrame.ProcessMouse(ev)
			}

			// 3. Fallback to Menu Activation (Alt+Hotkey) if top frame didn't want the key
			if !handled && fm.MenuBar != nil && !fm.MenuBar.Active && ev.Type == vtinput.KeyEventType {
				alt := (ev.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0
				if alt && ev.Char != 0 {
					if fm.MenuBar.ProcessKey(ev) { return }
				}
			}

			if topFrame.IsDone() { fm.Pop() }
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
