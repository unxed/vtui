package vtui

import "github.com/unxed/vtinput"

// CommandHandler defines an object that can process or route commands.
type CommandHandler interface {
	HandleCommand(cmd int, args any) bool
	IsLocked() bool
	GetHelp() string
}

// ScreenObject is the base class for all visible UI elements,
// analog of ScreenObject from scrobj.hpp.
type ScreenObject struct {
	X1, Y1, X2, Y2 int
	owner          CommandHandler
	visible        bool
	focused        bool
	canFocus       bool
	lockCount      int
	helpTopic      string
	growMode       GrowMode
	hotkey         rune
	Id             string
	disabled       bool
	Command        int
	text           string
	cleanText      string
	hotkeyPos      int
}

// GetHotkey returns the assigned hotkey rune for the object.
func (so *ScreenObject) GetHotkey() rune {
	return so.hotkey
}
func (so *ScreenObject) SetText(s string) {
	so.text = s
	clean, hk, pos := ParseAmpersandString(s)
	so.cleanText = clean
	so.hotkey = hk
	so.hotkeyPos = pos
}

func (so *ScreenObject) GetText() string {
	return so.text
}
func (so *ScreenObject) GetId() string {
	return so.Id
}

func (so *ScreenObject) SetId(id string) {
	so.Id = id
}

func (so *ScreenObject) SetOwner(owner CommandHandler) {
	so.owner = owner
}

func (so *ScreenObject) GetOwner() CommandHandler {
	return so.owner
}

func (so *ScreenObject) SetGrowMode(gm GrowMode) {
	so.growMode = gm
}

func (so *ScreenObject) GetGrowMode() GrowMode {
	return so.growMode
}

// HitTest returns true if the coordinates fall within the object's bounding box.
func (so *ScreenObject) HitTest(x, y int) bool {
	return x >= so.X1 && x <= so.X2 && y >= so.Y1 && y <= so.Y2
}
func (so *ScreenObject) WantsChars() bool {
	return false
}
func (so *ScreenObject) GetFocusLink() UIElement {
	return nil
}
// SetPosition sets the object's coordinates.
// Important: this does not trigger a redraw.
func (so *ScreenObject) SetPosition(x1, y1, x2, y2 int) {
	if so.X1 == x1 && so.Y1 == y1 && so.X2 == x2 && so.Y2 == y2 {
		return
	}
	// Visibility status becomes invalid on move
	so.visible = false
	so.X1, so.Y1, so.X2, so.Y2 = x1, y1, x2, y2
}

// GetPosition returns current object coordinates.
func (so *ScreenObject) GetPosition() (int, int, int, int) {
	return so.X1, so.Y1, so.X2, so.Y2
}

// Show makes the object visible.
func (so *ScreenObject) Show(scr *ScreenBuf) {
	if so.IsLocked() {
		return
	}
	so.visible = true
}

// Hide hides the object.
func (so *ScreenObject) Hide(scr *ScreenBuf) {
	so.visible = false
}

// IsVisible returns true if the object is visible.
func (so *ScreenObject) IsVisible() bool {
	return so.visible
}
// SetVisible manually sets the visibility flag.
func (so *ScreenObject) SetVisible(v bool) {
	so.visible = v
}

// SetFocus sets or removes focus from the object.
func (so *ScreenObject) SetFocus(f bool) {
	if so.focused != f {
		so.focused = f
		// Optional: Log focus changes only for focusable items to avoid spam
		if so.canFocus {
			DebugLog("FOCUS: [%p] set to %v (ID: %s)", so, f, so.Id)
		}
	}
}

// IsFocused returns the focus state of the object.
func (so *ScreenObject) IsFocused() bool {
	return so.focused
}

// SetCanFocus sets whether the object can accept focus.
func (so *ScreenObject) SetCanFocus(c bool) {
	so.canFocus = c
}

// CanFocus returns true if the object can be focused.
func (so *ScreenObject) CanFocus() bool {
	return so.canFocus
}
// IsDisabled returns true if the object is explicitly disabled.
func (so *ScreenObject) IsDisabled() bool {
	return so.disabled
}

// SetDisabled enables or disables the object.
func (so *ScreenObject) SetDisabled(d bool) {
	so.disabled = d
	if d {
		so.focused = false
	}
}

// Lock increases the lock counter. A locked object is not redrawn.
func (so *ScreenObject) Lock() {
	so.lockCount++
}

// Unlock decreases the lock counter.
func (so *ScreenObject) Unlock() {
	if so.lockCount > 0 {
		so.lockCount--
	}
}

// IsLocked returns true if the object or its owner is locked.
func (so *ScreenObject) IsLocked() bool {
	if so.lockCount > 0 {
		return true
	}
	if so.owner != nil {
		return so.owner.IsLocked()
	}
	return false
}
func (so *ScreenObject) HandleBroadcast(cmd int, args any) bool {
	return false
}
func (so *ScreenObject) Valid(cmd int) bool {
	return true
}

// ProcessKey (stub) will be overridden in child classes.
func (so *ScreenObject) ProcessKey(key *vtinput.InputEvent) bool {
	return false
}

// ProcessMouse is a default empty implementation.
func (so *ScreenObject) ProcessMouse(mouse *vtinput.InputEvent) bool {
	return false
}

// ResizeConsole (stub) will be overridden to react to resizing.
func (so *ScreenObject) ResizeConsole() {
	// Default empty implementation.
}

// SetHelp sets the help topic for this object.
func (so *ScreenObject) SetHelp(topic string) {
	so.helpTopic = topic
}

// GetHelp returns the help topic for this object.
// If the topic is empty, it searches in the owner object.
func (so *ScreenObject) GetHelp() string {
	if so.helpTopic != "" {
		return so.helpTopic
	}
	if so.owner != nil {
		return so.owner.GetHelp()
	}
	return ""
}

// ShowHelp triggers the help system for this object.
// For now, it just logs the topic to the debug log.
func (so *ScreenObject) ShowHelp() {
	topic := so.GetHelp()
	if topic == "" {
		topic = UIStrings.DefaultHelp
	}
	DebugLog("HELP SYSTEM: Requested topic '%s'", topic)
	// In the future, this will push a HelpFrame to FrameManager.
}
func (so *ScreenObject) HasShadow() bool {
	return false
}
func (so *ScreenObject) GetKeyLabels() *KeySet {
	return nil
}
func (so *ScreenObject) GetMenuBar() *MenuBar {
	return nil
}
// ResolveColor simplifies selecting the right palette index based on focus and disabled states.
func (so *ScreenObject) ResolveColor(normIdx, selIdx int) uint64 {
	attr := Palette[normIdx]
	if so.IsFocused() {
		attr = Palette[selIdx]
	}
	if so.IsDisabled() {
		return DimColor(attr)
	}
	return attr
}

// ResolveColors resolves both base and highlight colors simultaneously.
func (so *ScreenObject) ResolveColors(normIdx, selNormIdx, highIdx, selHighIdx int) (uint64, uint64) {
	return so.ResolveColor(normIdx, selNormIdx), so.ResolveColor(highIdx, selHighIdx)
}
// FireAction centralizes the logic for executing an optional callback or emitting the internal Command.
// It gives priority to the callback.
func (so *ScreenObject) FireAction(callback func(), args any) bool {
	if callback != nil {
		callback()
		return true
	}
	if so.Command != 0 {
		return so.HandleCommand(so.Command, args)
	}
	return false
}
// HandleCommand is the default implementation for command routing.
// It bubbles the command up to the owner.
func (so *ScreenObject) HandleCommand(cmd int, args any) bool {
	if so.owner != nil {
		DebugLog("CMD: [%p] Bubbling ID %d to owner [%p]", so, cmd, so.owner)
		return so.owner.HandleCommand(cmd, args)
	}
	DebugLog("CMD: [%p] ID %d dropped (no owner)", so, cmd)
	return false
}
