package vtui

import (
	"fmt"
	"unicode"
	"reflect"

	"github.com/unxed/vtinput"
)

// BaseWindow provides generic windowing logic (moving, resizing, focus cycle).
type BaseWindow struct {
	BaseFrame
	items      []UIElement
	focusIdx   int
	frame      *BorderedFrame
	isDragging bool
	isResizing bool
	dragOffX   int
	dragOffY   int
	lastW      int
	lastH      int
	MinW       int
	MinH       int
	ShowClose  bool
	ShowZoom   bool
	SavedBounds *Rect
	progress   int
}

func (bw *BaseWindow) GetFocusedItem() UIElement {
	if bw.focusIdx >= 0 && bw.focusIdx < len(bw.items) {
		return bw.items[bw.focusIdx]
	}
	return nil
}

func NewBaseWindow(x1, y1, x2, y2 int, title string) *BaseWindow {
	bw := &BaseWindow{
		items:    []UIElement{},
		focusIdx: -1,
		frame:    NewBorderedFrame(x1, y1, x2, y2, DoubleBox, title),
		MinW:     x2 - x1 + 1,
		MinH:     y2 - y1 + 1,
		progress: -1,
	}
	bw.SetPosition(x1, y1, x2, y2)
	bw.lastW = x2 - x1 + 1
	bw.lastH = y2 - y1 + 1
	return bw
}

func (bw *BaseWindow) AddItem(item UIElement) {
	bw.items = append(bw.items, item)
	_, _, ix2, iy2 := item.GetPosition()
	reqW := ix2 - bw.X1 + 1
	reqH := iy2 - bw.Y1 + 1
	if reqW > bw.MinW { bw.MinW = reqW }
	if reqH > bw.MinH { bw.MinH = reqH }
	if bw.focusIdx == -1 && item.CanFocus() && !item.IsDisabled() {
		bw.focusIdx = len(bw.items) - 1
		item.SetFocus(true)
	}
}

func (bw *BaseWindow) Show(scr *ScreenBuf) {
	bw.ScreenObject.Show(scr)
	bw.frame.ShowClose = bw.ShowClose

	// Draw active frame color if this window has focus
	if bw.IsFocused() {
		bw.frame.ColorBoxIdx = ColDialogBox
		bw.frame.ColorTitleIdx = ColDialogHighlightBoxTitle
	} else {
		bw.frame.ColorBoxIdx = ColDialogBox
		bw.frame.ColorTitleIdx = ColDialogBoxTitle
	}

	bw.frame.DisplayObject(scr)

	if bw.ShowZoom {
		zoomStr := string(UIStrings.CloseBrackets[0]) + string(UIStrings.ZoomSymbol) + string(UIStrings.CloseBrackets[1])
		offset := 4
		if bw.ShowClose { offset += 3 }
		scr.Write(bw.X2-offset, bw.Y1, StringToCharInfo(zoomStr, Palette[bw.frame.ColorBoxIdx]))
	}
	if bw.Number > 0 && bw.Number <= 9 {
		numStr := fmt.Sprintf("%c%d%c", UIStrings.CloseBrackets[0], bw.Number, UIStrings.CloseBrackets[1])
		scr.Write(bw.X1+2, bw.Y1, StringToCharInfo(numStr, Palette[bw.frame.ColorBoxIdx]))
	}

	scr.PushClipRect(bw.X1+1, bw.Y1+1, bw.X2-1, bw.Y2-1)
	defer scr.PopClipRect()
	for _, item := range bw.items {
		item.Show(scr)
	}
}

func (bw *BaseWindow) ProcessKey(e *vtinput.InputEvent) bool {
	if e.Type == vtinput.FocusEventType {
		bw.SetFocus(e.SetFocus)
		GlobalEvents.Publish(Event{
			Type:   EvFocus,
			Sender: bw,
			Data:   e.SetFocus,
		})
		return true
	}

	if bw.focusIdx != -1 {
		if bw.items[bw.focusIdx].ProcessKey(e) {
			return true
		}
		if e.KeyDown && (e.VirtualKeyCode == vtinput.VK_SPACE || e.VirtualKeyCode == vtinput.VK_RETURN) {
			if rb, ok := bw.items[bw.focusIdx].(*RadioButton); ok {
				bw.selectRadio(rb)
				return true
			}
		}
	}

	if !e.KeyDown { return false }

	if e.VirtualKeyCode == vtinput.VK_F5 && bw.ShowZoom {
		bw.ToggleZoom()
		return true
	}

	if e.Char != 0 {
		charLower := unicode.ToLower(e.Char)
		alt := (e.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0
		allowWithoutAlt := true
		if bw.focusIdx != -1 {
			if _, isEdit := bw.items[bw.focusIdx].(*Edit); isEdit {
				allowWithoutAlt = false
			} else if cb, isCombo := bw.items[bw.focusIdx].(*ComboBox); isCombo && !cb.DropdownOnly {
				allowWithoutAlt = false
			}
		}

		if alt || allowWithoutAlt {
			for i, item := range bw.items {
				hk := item.GetHotkey()
				if hk != 0 && hk == charLower {
					target := item
					targetIdx := i
					for hops := 0; hops < len(bw.items); hops++ {
						if txt, ok := target.(*Text); ok && txt.FocusLink != nil {
							target = txt.FocusLink
							for j, other := range bw.items {
								if other == target {
									targetIdx = j
									break
								}
							}
							continue
						}
						break
					}
					if target.CanFocus() && !target.IsDisabled() {
						if bw.focusIdx != -1 {
							bw.items[bw.focusIdx].SetFocus(false)
						}
						bw.focusIdx = targetIdx
						target.SetFocus(true)
					}
					if _, isBtn := target.(*Button); isBtn {
						target.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE})
					} else if _, isChk := target.(*Checkbox); isChk {
						target.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE})
					} else if rb, isRad := target.(*RadioButton); isRad {
						bw.selectRadio(rb)
					}
					return true
				}
			}
		}
	}

	switch e.VirtualKeyCode {
	case vtinput.VK_F1:
		bw.ShowHelp()
		return true
	case vtinput.VK_ESCAPE, vtinput.VK_F10:
		bw.Close()
		return true
	case vtinput.VK_TAB:
		ctrl := (e.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)) != 0
		if ctrl {
			return false // Let FrameManager handle Ctrl+Tab window cycling
		}
		shift := (e.ControlKeyState & vtinput.ShiftPressed) != 0
		if shift {
			bw.changeFocus(-1)
		} else {
			bw.changeFocus(1)
		}
		return true

	case vtinput.VK_UP, vtinput.VK_LEFT:
		bw.changeFocus(-1)
		return true

	case vtinput.VK_DOWN, vtinput.VK_RIGHT:
		bw.changeFocus(1)
		return true

	case vtinput.VK_RETURN:
		// If Enter was not handled by the focused item (like an Edit with OnAction),
		// try to find the first available button and click it.
		for _, item := range bw.items {
			if btn, ok := item.(*Button); ok {
				if btn.OnClick != nil {
					btn.OnClick()
					return true
				}
			}
		}
	}

	return false
}

func (bw *BaseWindow) ResizeConsole(w, h int) {
	dw, dh := bw.X2-bw.X1+1, bw.Y2-bw.Y1+1
	nx1 := (w - dw) / 2
	ny1 := (h - dh) / 2
	bw.MoveRelative(nx1-bw.X1, ny1-bw.Y1)
}

func (bw *BaseWindow) Center(scrW, scrH int) {
	dw, dh := bw.X2-bw.X1+1, bw.Y2-bw.Y1+1
	nx1 := (scrW - dw) / 2
	ny1 := (scrH - dh) / 2
	bw.MoveRelative(nx1-bw.X1, ny1-bw.Y1)
}

func (bw *BaseWindow) ChangeSize(nw, nh int) {
	if nw < bw.MinW { nw = bw.MinW }
	if nh < bw.MinH { nh = bw.MinH }
	dx := nw - bw.lastW
	dy := nh - bw.lastH
	if dx == 0 && dy == 0 { return }

	bw.X2 += dx
	bw.Y2 += dy
	bw.frame.SetPosition(bw.X1, bw.Y1, bw.X2, bw.Y2)

	for _, item := range bw.items {
		gm := item.GetGrowMode()
		ix1, iy1, ix2, iy2 := item.GetPosition()
		if (gm & GrowLoX) != 0 { ix1 += dx }
		if (gm & GrowHiX) != 0 { ix2 += dx }
		if (gm & GrowLoY) != 0 { iy1 += dy }
		if (gm & GrowHiY) != 0 { iy2 += dy }
		item.SetPosition(ix1, iy1, ix2, iy2)
	}
	bw.lastW = nw
	bw.lastH = nh
}

func (bw *BaseWindow) ToggleZoom() {
	if bw.SavedBounds != nil {
		bw.ChangeSize(bw.SavedBounds.X2-bw.SavedBounds.X1+1, bw.SavedBounds.Y2-bw.SavedBounds.Y1+1)
		bw.MoveRelative(bw.SavedBounds.X1-bw.X1, bw.SavedBounds.Y1-bw.Y1)
		bw.SavedBounds = nil
	} else {
		bw.SavedBounds = &Rect{X1: bw.X1, Y1: bw.Y1, X2: bw.X2, Y2: bw.Y2}
		w := FrameManager.GetScreenSize()
		h := 25
		if FrameManager.scr != nil {
			h = FrameManager.scr.height
		}
		bw.MoveRelative(-bw.X1, -bw.Y1)
		bw.ChangeSize(w, h-1)
	}
}

func (bw *BaseWindow) changeFocus(direction int) {
	if len(bw.items) == 0 { return }
	
	// If starting from scratch, decide where to begin checking
	if bw.focusIdx == -1 {
		if direction > 0 {
			bw.focusIdx = len(bw.items) - 1
		} else {
			bw.focusIdx = 0
		}
	} else {
		bw.items[bw.focusIdx].SetFocus(false)
	}

	// Determine starting point for the cycle check (where we started before movement)
	startIdx := bw.focusIdx

	for {
		bw.focusIdx += direction
		if bw.focusIdx < 0 { bw.focusIdx = len(bw.items) - 1 }
		if bw.focusIdx >= len(bw.items) { bw.focusIdx = 0 }

		if bw.items[bw.focusIdx].CanFocus() && !bw.items[bw.focusIdx].IsDisabled() {
			bw.items[bw.focusIdx].SetFocus(true)
			return
		}

		if bw.focusIdx == startIdx {
			// Completed a full circle and found no focusable items
			bw.focusIdx = -1
			return
		}
	}
}

func (bw *BaseWindow) selectRadio(rb *RadioButton) {
	if rb.Selected { return }
	for _, item := range bw.items {
		if other, ok := item.(*RadioButton); ok {
			other.Selected = false
		}
	}
	rb.Selected = true
}

func (bw *BaseWindow) ProcessMouse(e *vtinput.InputEvent) bool {
	mx, my := int(e.MouseX), int(e.MouseY)

	if bw.isDragging {
		if !e.KeyDown && e.ButtonState == 0 {
			bw.isDragging = false
			return true
		}
		dx := mx - bw.dragOffX
		dy := my - bw.dragOffY
		if dx != bw.X1 || dy != bw.Y1 {
			bw.MoveRelative(dx-bw.X1, dy-bw.Y1)
		}
		return true
	}

	if bw.isResizing {
		if !e.KeyDown && e.ButtonState == 0 {
			bw.isResizing = false
			return true
		}
		newW := mx - bw.X1 + 1
		newH := my - bw.Y1 + 1
		if newW < bw.MinW { newW = bw.MinW }
		if newH < bw.MinH { newH = bw.MinH }
		bw.ChangeSize(newW, newH)
		return true
	}

	for i := len(bw.items) - 1; i >= 0; i-- {
		item := bw.items[i]
		x1, y1, x2, y2 := item.GetPosition()
		if mx >= x1 && mx <= x2 && my >= y1 && my <= y2 {
			if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
				if item.CanFocus() && !item.IsDisabled() && bw.focusIdx != i {
					if bw.focusIdx != -1 {
						bw.items[bw.focusIdx].SetFocus(false)
					}
					bw.focusIdx = i
					item.SetFocus(true)
				}
			}
			if item.ProcessMouse(e) { return true }
			if rb, ok := item.(*RadioButton); ok && e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
				bw.selectRadio(rb)
				return true
			}
			return true
		}
	}

	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		if bw.ShowClose && my == bw.Y1 && mx >= bw.X2-4 && mx <= bw.X2-2 {
			bw.Close()
			return true
		}
		offset := 4
		if bw.ShowClose { offset += 3 }
		if bw.ShowZoom && my == bw.Y1 && mx >= bw.X2-offset && mx <= bw.X2-offset+2 {
			bw.ToggleZoom()
			return true
		}
		if mx == bw.X2 && my == bw.Y2 {
			bw.isResizing = true
			return true
		}
		if mx >= bw.X1 && mx <= bw.X2 && my >= bw.Y1 && my <= bw.Y2 {
			bw.isDragging = true
			bw.dragOffX = mx - bw.X1
			bw.dragOffY = my - bw.Y1
			return true
		}
	}
	return false
}

func (bw *BaseWindow) MoveRelative(dx, dy int) {
	bw.X1 += dx
	bw.X2 += dx
	bw.Y1 += dy
	bw.Y2 += dy
	bw.frame.SetPosition(bw.X1, bw.Y1, bw.X2, bw.Y2)
	for _, item := range bw.items {
		ix1, iy1, ix2, iy2 := item.GetPosition()
		if so, ok := item.(interface{ SetPosition(int, int, int, int) }); ok {
			so.SetPosition(ix1+dx, iy1+dy, ix2+dx, iy2+dy)
		}
	}
}

// HandleCommand implements Turbo Vision style command routing for Windows/Dialogs.
func (bw *BaseWindow) HandleCommand(cmd int, args any) bool {
	// 1. Handle standard window commands
	switch cmd {
	case CmOK, CmDefault:
		if !bw.Valid(cmd) {
			return true // Consumed but blocked by validation
		}
		bw.SetExitCode(cmd)
		return true
	case CmClose, CmCancel:
		bw.Close()
		return true
	case CmZoom:
		if bw.ShowZoom {
			bw.ToggleZoom()
			return true
		}
	}

	// 3. Bubble up to BaseFrame (which bubbles to owner)
	return bw.BaseFrame.HandleCommand(cmd, args)
}
func (bw *BaseWindow) HandleBroadcast(cmd int, args any) bool {
	handled := false
	for _, item := range bw.items {
		if item.HandleBroadcast(cmd, args) {
			handled = true
		}
	}
	return handled
}
func (bw *BaseWindow) Valid(cmd int) bool {
	for _, item := range bw.items {
		if !item.Valid(cmd) {
			return false
		}
	}
	return true
}

func (bw *BaseWindow) HasShadow() bool { return true }
// SetData populates UI elements from a struct using field names or `vtui` tags.
func (bw *BaseWindow) SetData(record any) {
	val := reflect.ValueOf(record)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return
	}

	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)
		id := fieldType.Tag.Get("vtui")
		if id == "" {
			id = fieldType.Name
		}

		for _, item := range bw.items {
			if item.GetId() == id {
				if dc, ok := item.(DataControl); ok {
					dc.SetData(field.Interface())
				}
				break
			}
		}
	}
}

// GetData populates a struct from UI elements using field names or `vtui` tags.
func (bw *BaseWindow) GetData(record any) {
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

		for _, item := range bw.items {
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
