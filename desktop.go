package vtui

import (
	"github.com/unxed/vtinput"
)

// Desktop is the root object that draws the background. It is always at the bottom of the frame stack.
type Desktop struct {
	ScreenObject
	done     bool
	exitCode int
}

func NewDesktop() *Desktop {
	return &Desktop{}
}

func (d *Desktop) Show(scr *ScreenBuf) {
	// Desktop background should be rendered using indexed colors (usually Index 1)
	// without Early Binding to RGB, to allow user terminal colors to show through.
	prevOverlay := scr.OverlayMode
	scr.SetOverlayMode(false)
	defer func() { scr.SetOverlayMode(prevOverlay) }()

	width, height := scr.width, scr.height
	bgAttr := Palette[ColDesktopBackground]
	scr.FillRect(0, 0, width-1, height-1, ' ', bgAttr)
}

// Desktop doesn't handle any specific keys, but could handle global hotkeys in the future.
func (d *Desktop) ProcessKey(e *vtinput.InputEvent) bool {
	// Fallback exit keys when no other window is focused
	if e.VirtualKeyCode == vtinput.VK_ESCAPE || e.VirtualKeyCode == vtinput.VK_F10 {
		return FrameManager.EmitCommand(CmQuit, nil)
	}
	return false
}

func (d *Desktop) HandleCommand(cmd int, args any) bool {
	if cmd == CmQuit {
		FrameManager.Shutdown()
		return true
	}
	return d.ScreenObject.HandleCommand(cmd, args)
}

func (d *Desktop) ProcessMouse(e *vtinput.InputEvent) bool {
	return false
}

func (d *Desktop) ResizeConsole(w, h int) {
	// The desktop automatically adapts to the screen size during Show() via scr.width/height
}
func (d *Desktop) GetTitle() string {
	return "Desktop"
}
func (d *Desktop) GetProgress() int {
	return -1
}

func (d *Desktop) GetType() FrameType {
	return TypeDesktop
}

func (d *Desktop) SetExitCode(code int) {
	d.done = true
	d.exitCode = code
}

func (d *Desktop) IsDone() bool { return d.done }
func (d *Desktop) IsBusy() bool { return false }
func (d *Desktop) IsModal() bool { return false }
func (d *Desktop) GetWindowNumber() int { return 0 }
func (d *Desktop) SetWindowNumber(n int) {}
func (d *Desktop) RequestFocus() bool { return false }
func (d *Desktop) Close() { d.SetExitCode(-1) }
func (d *Desktop) HasShadow() bool { return false }
