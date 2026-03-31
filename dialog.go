package vtui

import (
	"github.com/unxed/vtinput"
)

// UIElement is the interface that all dialog elements must implement.
type UIElement interface {
	GetPosition() (int, int, int, int)
	SetPosition(int, int, int, int)
	GetGrowMode() GrowMode
	Show(scr *ScreenBuf)
	Hide(scr *ScreenBuf)
	SetFocus(bool)
	IsFocused() bool
	CanFocus() bool
	IsDisabled() bool
	SetDisabled(bool)
	GetHotkey() rune
	GetId() string
	GetHelp() string
	ProcessKey(e *vtinput.InputEvent) bool
	ProcessMouse(e *vtinput.InputEvent) bool
	HandleCommand(cmd int, args any) bool
	HandleBroadcast(cmd int, args any) bool
	Valid(cmd int) bool
}

// DataControl is an interface for UI elements that can store and return data.
type DataControl interface {
	SetData(value any)
	GetData() any
}

// Dialog is a modal container for UI elements.
type Dialog struct {
	BaseWindow
}

func NewDialog(x1, y1, x2, y2 int, title string) *Dialog {
	d := &Dialog{
		BaseWindow: *NewBaseWindow(x1, y1, x2, y2, title),
	}
	// Re-link the root group to the actual Dialog pointer
	d.rootGroup.SetOwner(d)
	d.frame.SetOwner(d)
	return d
}

func (d *Dialog) IsModal() bool { return true }
func (d *Dialog) GetType() FrameType { return TypeDialog }
func (d *Dialog) GetTitle() string { return d.frame.title }
func (d *Dialog) GetProgress() int {
	// If the dialog contains a text element that looks like a percentage,
	// or we can manually set it. For this demo, we'll allow manual override.
	return d.progress
}

func (d *Dialog) SetProgress(p int) {
	d.progress = p
}

