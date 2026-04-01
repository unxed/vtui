package vtui

import (
	"reflect"
	"unicode"

	"github.com/unxed/vtinput"
)

// Group is a container for UI elements, handling layout, focus, and event propagation.
// It implements the UIElement interface, allowing groups to be nested.
type Group struct {
	ScreenObject
	items     []UIElement
	focusIdx  int
	WrapFocus bool
}

// NewGroup creates a new Group container.
func NewGroup(x, y, w, h int) *Group {
	g := &Group{
		items:     make([]UIElement, 0),
		focusIdx:  -1,
		WrapFocus: false,
	}
	g.SetPosition(x, y, x+w-1, y+h-1)
	return g
}

func (g *Group) GetFocusedItem() UIElement {
	if g.focusIdx >= 0 && g.focusIdx < len(g.items) {
		return g.items[g.focusIdx]
	}
	return nil
}

// AddItem adds a UI element to the group.
func (g *Group) AddItem(item UIElement) {
	g.items = append(g.items, item)
	// Set the group as the owner of the added item to enable command bubbling
	item.SetPosition(item.GetPosition()) // Trigger potential internal updates
	if so, ok := item.(interface{ SetOwner(CommandHandler) }); ok {
		so.SetOwner(g)
	}
	if g.focusIdx == -1 && item.CanFocus() && !item.IsDisabled() {
		g.focusIdx = len(g.items) - 1
		item.SetFocus(true)
	}
}

// DisplayObject draws all child elements of the group.
func (g *Group) DisplayObject(scr *ScreenBuf) {
	if !g.IsVisible() {
		return
	}
	// The clip rect is assumed to be set by the parent (e.g., BaseWindow).
	for _, item := range g.items {
		item.Show(scr)
	}
}

// Show makes the group and its children visible.
func (g *Group) Show(scr *ScreenBuf) {
	g.ScreenObject.Show(scr)
	g.DisplayObject(scr)
}

// ProcessKey handles keyboard events, delegating to the focused child or managing focus changes.
func (g *Group) ProcessKey(e *vtinput.InputEvent) bool {
	// 1. Give priority to currently focused child.
	// This ensures that text input goes to Edit fields, buttons handle space/enter, etc.
	if g.focusIdx != -1 {
		focusedItem := g.items[g.focusIdx]

		// Delegate the key processing.
		if focusedItem.ProcessKey(e) {
			return true // The focused item consumed the key event.
		}
		// If focusedItem.ProcessKey(e) returned false, it means the item did NOT consume the key.
		// Now the parent group can attempt to handle it (e.g. for global navigation keys like TAB/Arrows
		// that the child didn't want, or if the child is a nested group that signals "wrapped out").
	}

	if !e.KeyDown { // Only process KeyDown events from here onwards for general group navigation/hotkeys.
		return false
	}

	// Handle hotkeys (Alt+char or char if no focusable text-input element)
	if e.Char != 0 {
		charLower := unicode.ToLower(e.Char)
		alt := (e.ControlKeyState&(vtinput.LeftAltPressed|vtinput.RightAltPressed)) != 0
		allowWithoutAlt := true
		if g.focusIdx != -1 {
			if _, isEdit := g.items[g.focusIdx].(*Edit); isEdit {
				allowWithoutAlt = false
			} else if cb, isCombo := g.items[g.focusIdx].(*ComboBox); isCombo && !cb.DropdownOnly {
				allowWithoutAlt = false
			}
		}

		if alt || allowWithoutAlt {
			if g.ActivateHotkey(charLower) {
				return true
			}
		}
	}

	// Handle focus navigation for THIS group (`g`).
	// This is reached if:
	//    - No focused item (or nested group) consumed the event.
	//    - Hotkey wasn't activated.
	switch e.VirtualKeyCode {
	case vtinput.VK_TAB:
		ctrl := (e.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)) != 0
		if ctrl {
			return false // Let FrameManager handle Ctrl+Tab
		}
		shift := (e.ControlKeyState & vtinput.ShiftPressed) != 0
		if shift {
			return g.changeFocus(-1)
		} else {
			return g.changeFocus(1)
		}
	case vtinput.VK_UP, vtinput.VK_LEFT:
		return g.changeFocus(-1)
	case vtinput.VK_DOWN, vtinput.VK_RIGHT:
		return g.changeFocus(1)
	}

	return false
}

// ProcessMouse handles mouse events by hit-testing child elements.
func (g *Group) ProcessMouse(e *vtinput.InputEvent) bool {
	mx, my := int(e.MouseX), int(e.MouseY)
	for i := len(g.items) - 1; i >= 0; i-- {
		item := g.items[i]
		if item.HitTest(mx, my) {
			if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
				if item.CanFocus() && !item.IsDisabled() && g.focusIdx != i {
					g.setFocus(i)
				}
			}
			if item.ProcessMouse(e) {
				return true
			}
			return true // Event was within an item, consume it
		}
	}
	return false
}

func (g *Group) changeFocus(direction int) bool {
	if len(g.items) == 0 {
		return false
	}

	startIdx := g.focusIdx
	if startIdx == -1 {
		if direction > 0 {
			startIdx = -1
		} else {
			startIdx = len(g.items)
		}
	}

	for i := 1; i <= len(g.items); i++ {
		nextIdx := startIdx + i*direction
		if nextIdx >= len(g.items) || nextIdx < 0 {
			if !g.WrapFocus {
				return false
			}
			nextIdx = (nextIdx%len(g.items) + len(g.items)) % len(g.items)
		}

		item := g.items[nextIdx]
		if item.CanFocus() && !item.IsDisabled() {
			g.setFocus(nextIdx)
			return true
		}
	}
	return false
}

// setFocus changes the focused item within the group.
func (g *Group) setFocus(index int) {
	if g.focusIdx != -1 && g.focusIdx < len(g.items) {
		g.items[g.focusIdx].SetFocus(false)
	}
	g.focusIdx = index
	if g.focusIdx != -1 {
		g.items[g.focusIdx].SetFocus(true)
	}
}

// SetFocus handles focus delegation for the group.
func (g *Group) SetFocus(f bool) {
	g.ScreenObject.SetFocus(f)
	if f {
		if g.focusIdx == -1 {
			g.changeFocus(1)
		} else if g.focusIdx < len(g.items) {
			g.items[g.focusIdx].SetFocus(true)
		}
	} else {
		if g.focusIdx != -1 && g.focusIdx < len(g.items) {
			g.items[g.focusIdx].SetFocus(false)
		}
		g.focusIdx = -1
	}
}
// ActivateHotkey finds and activates an element by its hotkey.
func (g *Group) ActivateHotkey(hk rune) bool {
	for i, item := range g.items {
		if item.GetHotkey() == hk {
			target := item
			targetIdx := i
			// Follow FocusLink chain
			for {
				if txt, ok := target.(*Text); ok && txt.FocusLink != nil {
					target = txt.FocusLink
					found := false
					for j, other := range g.items {
						if other == target {
							targetIdx = j
							found = true
							break
						}
					}
					if !found { break } // Link points to an item not in this group
				} else {
					break
				}
			}

			if target.CanFocus() && !target.IsDisabled() {
				g.setFocus(targetIdx)
			}
			// Trigger action for buttons/checkboxes
			if b, isBtn := target.(*Button); isBtn {
				if b.Command != 0 {
					g.HandleCommand(b.Command, nil)
				}
			} else if _, isChk := target.(*Checkbox); isChk {
				target.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE})
			}
			return true
		}

		// Recurse into nested groups
		if subGroup, ok := item.(interface{ ActivateHotkey(rune) bool }); ok {
			if subGroup.ActivateHotkey(hk) {
				if item.CanFocus() && !item.IsDisabled() {
					g.setFocus(i)
				}
				return true
			}
		}
	}
	return false
}

// MoveRelative moves the group and all its children.
func (g *Group) MoveRelative(dx, dy int) {
	g.X1 += dx
	g.X2 += dx
	g.Y1 += dy
	g.Y2 += dy
	for _, item := range g.items {
		ix1, iy1, ix2, iy2 := item.GetPosition()
		item.SetPosition(ix1+dx, iy1+dy, ix2+dx, iy2+dy)
	}
}

// Resize resizes the group and applies GrowMode to its children.
func (g *Group) Resize(dx, dy int) {
	g.X2 += dx
	g.Y2 += dy
	for _, item := range g.items {
		gm := item.GetGrowMode()
		ix1, iy1, ix2, iy2 := item.GetPosition()
		if (gm & GrowLoX) != 0 {
			ix1 += dx
		}
		if (gm & GrowHiX) != 0 {
			ix2 += dx
		}
		if (gm & GrowLoY) != 0 {
			iy1 += dy
		}
		if (gm & GrowHiY) != 0 {
			iy2 += dy
		}
		item.SetPosition(ix1, iy1, ix2, iy2)
	}
}

func (g *Group) SetData(record any) {
	val := reflect.ValueOf(record)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return
	}
	typ := val.Type()

	// Extract flat map of all ID -> DataControl recursively
	controls := make(map[string]DataControl)
	var collect func(grp *Group)
	collect = func(grp *Group) {
		for _, item := range grp.items {
			if dc, ok := item.(DataControl); ok && item.GetId() != "" {
				controls[item.GetId()] = dc
			}
			if subGrp, ok := item.(*Group); ok {
				collect(subGrp)
			}
			if gb, ok := item.(*GroupBox); ok {
				collect(&gb.Group)
			}
		}
	}
	collect(g)

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)
		id := fieldType.Tag.Get("vtui")
		if id == "" {
			id = fieldType.Name
		}
		if dc, ok := controls[id]; ok {
			dc.SetData(field.Interface())
		}
	}
}

func (g *Group) GetData(record any) {
	val := reflect.ValueOf(record)
	if val.Kind() != reflect.Ptr || val.Elem().Kind() != reflect.Struct {
		return
	}
	val = val.Elem()
	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)
		id := fieldType.Tag.Get("vtui")
		if id == "" {
			id = fieldType.Name
		}
		for _, item := range g.items {
			if item.GetId() == id {
				if dc, ok := item.(DataControl); ok {
					itemVal := reflect.ValueOf(dc.GetData())
					if itemVal.Type().AssignableTo(field.Type()) {
						field.Set(itemVal)
					}
				}
				break
			}
		}
	}
}

// FindDefaultButton recursively searches for a button marked as IsDefault,
// falling back to the first actionable button if none is strictly default.
func (g *Group) FindDefaultButton() *Button {
	var firstBtn *Button
	var search func(grp *Group) *Button

	search = func(grp *Group) *Button {
		for _, item := range grp.items {
			if btn, ok := item.(*Button); ok && !btn.IsDisabled() {
				if btn.IsDefault { return btn }
				if firstBtn == nil && (btn.OnClick != nil || btn.Command != 0) {
					firstBtn = btn
				}
			} else if subGrp, ok := item.(*Group); ok {
				if b := search(subGrp); b != nil && b.IsDefault { return b }
			} else if gb, ok := item.(*GroupBox); ok {
				if b := search(&gb.Group); b != nil && b.IsDefault { return b }
			}
		}
		return nil
	}

	if def := search(g); def != nil {
		return def
	}
	return firstBtn
}

// HandleBroadcast propagates broadcast events to all children.
func (g *Group) HandleBroadcast(cmd int, args any) bool {
	handled := false
	for _, item := range g.items {
		if item.HandleBroadcast(cmd, args) {
			handled = true
		}
	}
	return handled
}

// Valid checks if all children are valid.
func (g *Group) Valid(cmd int) bool {
	for _, item := range g.items {
		if !item.Valid(cmd) {
			return false
		}
	}
	return true
}