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
	width, height := scr.width, scr.height
	scr.FillRect(0, 0, width-1, height-1, ' ', SetRGBBack(0, 0x0000A0))
	// Add a little hint that the app is alive
	msg := " f4 project - Press Ctrl+Q to exit "
	scr.Write((width-len(msg))/2, height-1, StringToCharInfo(msg, SetRGBBoth(0, 0xAAAAAA, 0x0000A0)))
}

// Desktop doesn't handle any specific keys, but could handle global hotkeys in the future.
func (d *Desktop) ProcessKey(e *vtinput.InputEvent) bool {
	// Global exit on Ctrl+Q for example
	if e.VirtualKeyCode == vtinput.VK_ESCAPE || e.VirtualKeyCode == vtinput.VK_F10 {
		d.SetExitCode(-1)
		return true
	}
	return false
}

func (d *Desktop) ProcessMouse(e *vtinput.InputEvent) bool {
	return false
}

func (d *Desktop) ResizeConsole(w, h int) {
	// The desktop automatically adapts to the screen size during Show() via scr.width/height
}

func (d *Desktop) GetType() FrameType {
	return TypeDesktop
}

func (d *Desktop) SetExitCode(code int) {
	d.done = true
	d.exitCode = code
}

func (d *Desktop) IsDone() bool {
	return d.done
}