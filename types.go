package vtui

import "github.com/unxed/vtinput"

// Coord defines the coordinates in the console.
type Coord struct {
	X int16
	Y int16
}

// SmallRect defines a rectangular area in the console.
// Rect defines a generic rectangle with absolute coordinates.
type Rect struct {
	X1, Y1, X2, Y2 int
}

// SmallRect defines a rectangular area in the console.
type SmallRect struct {
	Left   int16
	Top    int16
	Right  int16
	Bottom int16
}

// CharInfo contains a character and its visual attributes (including colors).
// In far2l, Char (UnicodeChar) is uint64 (COMP_CHAR) to support composite characters.
// Let's use the same bit length.
type CharInfo struct {
	Char       uint64 // Equivalent to union with COMP_CHAR UnicodeChar
	Attributes uint64 // DWORD64 Equivalent Attributes (lower 16 bits are flags, 16-39 are Fore RGB, 40-63 are Back RGB)
}// GrowMode flags for responsive layout resizing (analogous to Turbo Vision)
type GrowMode int

const (
	GrowNone   GrowMode = 0
	GrowLoX    GrowMode = 0x01
	GrowHiX    GrowMode = 0x02
	GrowLoY    GrowMode = 0x04
	GrowHiY    GrowMode = 0x08
	GrowAll    GrowMode = 0x0f
	GrowRel    GrowMode = 0x10
)
// UIElement is the interface that all screen objects (widgets, frames, windows) implement.
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
	SetOwner(CommandHandler)
	GetOwner() CommandHandler
	GetHotkey() rune
	GetId() string
	GetHelp() string
	ProcessKey(e *vtinput.InputEvent) bool
	ProcessMouse(e *vtinput.InputEvent) bool
	HandleCommand(cmd int, args any) bool
	HandleBroadcast(cmd int, args any) bool
	Valid(cmd int) bool
	HitTest(x, y int) bool
	WantsChars() bool
	GetFocusLink() UIElement
}

// Container is an interface for elements that have child UI elements.
type Container interface {
	GetChildren() []UIElement
}

// DataControl is an interface for UI elements that can store and return data.
type DataControl interface {
	SetData(value any)
	GetData() any
}
// FocusContainer is an interface for UI elements that manage a focusable child.
type FocusContainer interface {
	GetFocusedItem() UIElement
}
