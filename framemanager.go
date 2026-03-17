package vtui

import (
	"io"
	"os"
	"os/signal"
	"syscall"

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
}

// frameManager manages the stack of frames and the main application loop.
type frameManager struct {
	frames         []Frame
	scr            *ScreenBuf
	RedrawChan     chan struct{}
	EventFilter    func(*vtinput.InputEvent) bool
	injectedEvents []*vtinput.InputEvent
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

		// 1. Rendering: draw all frames from bottom to top
		fm.scr.SetCursorVisible(false) // Сбрасываем курсор перед отрисовкой кадра
		for _, frame := range fm.frames {
			frame.Show(fm.scr)
		}
		fm.scr.Flush()

		// 2. Event waiting
		var e *vtinput.InputEvent
		injected := false

		if len(fm.injectedEvents) > 0 {
			e = fm.injectedEvents[0]
			fm.injectedEvents = fm.injectedEvents[1:]
			injected = true
		} else {
			select {
			case <-fm.RedrawChan:
				// Loop around to redraw immediately
				continue
			case ev, ok := <-eventChan:
				if !ok {
					return
				}
				e = ev
			case <-sigChan:
				width, height, _ := term.GetSize(int(os.Stdin.Fd()))
				fm.scr.AllocBuf(width, height)
				// Notify all frames about the resize
				for _, frame := range fm.frames {
					frame.ResizeConsole(width, height)
				}
				continue
			}
		}

		// Allow filtering only for real user inputs (prevent infinite macro loops)
		if !injected && fm.EventFilter != nil && fm.EventFilter(e) {
			continue
		}

		topFrame := fm.frames[len(fm.frames)-1]

		// Dispatch event to the top-most frame
		if e.Type == vtinput.KeyEventType {
			// Global Hotkey: Ctrl+Q always exits the application
			if e.VirtualKeyCode == vtinput.VK_Q && (e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed)) != 0 {
				fm.frames = nil // Clear all frames to exit
				return
			}
			topFrame.ProcessKey(e)
		} else if e.Type == vtinput.MouseEventType {
			topFrame.ProcessMouse(e)
		}

		// Check if the frame wants to close
		if topFrame.IsDone() {
			fm.Pop()
		}
	}
}
