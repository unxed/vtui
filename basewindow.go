package vtui

import (
	"fmt"

	"github.com/unxed/vtinput"
)

// BaseWindow provides generic windowing logic (moving, resizing, focus cycle).
type BaseWindow struct {
	BaseFrame
	rootGroup  *Group
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
	return bw.rootGroup.GetFocusedItem()
}
func (bw *BaseWindow) GetChildren() []UIElement {
	return bw.rootGroup.GetChildren()
}

func NewBaseWindow(x1, y1, x2, y2 int, title string) *BaseWindow {
	bw := &BaseWindow{
		frame:    NewBorderedFrame(x1, y1, x2, y2, DoubleBox, title),
		MinW:     x2 - x1 + 1,
		MinH:     y2 - y1 + 1,
		progress: -1,
	}
	// The root group lives inside the frame
	bw.rootGroup = NewGroup(x1+1, y1+1, x2-x1-1, y2-y1-1)
	// Important: we don't set owner here yet, because BaseWindow is often
	// embedded in Dialog/Window and copied. We set it in NewDialog/NewWindow.
	bw.rootGroup.WrapFocus = true
	bw.SetPosition(x1, y1, x2, y2)
	bw.lastW = x2 - x1 + 1
	bw.lastH = y2 - y1 + 1
	return bw
}

func (bw *BaseWindow) AddItem(item UIElement) {
	bw.rootGroup.AddItem(item)
	// Update minimum size based on items added, relative to the window origin
	_, _, ix2, iy2 := item.GetPosition()
	reqW := ix2 - bw.X1 + 2 // +2 for borders
	reqH := iy2 - bw.Y1 + 2
	if reqW > bw.MinW {
		bw.MinW = reqW
	}
	if reqH > bw.MinH {
		bw.MinH = reqH
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
		if bw.ShowClose {
			offset += 3
		}
		scr.Write(bw.X2-offset, bw.Y1, StringToCharInfo(zoomStr, Palette[bw.frame.ColorBoxIdx]))
	}
	if bw.Number > 0 && bw.Number <= 9 {
		numStr := fmt.Sprintf("%c%d%c", UIStrings.CloseBrackets[0], bw.Number, UIStrings.CloseBrackets[1])
		scr.Write(bw.X1+2, bw.Y1, StringToCharInfo(numStr, Palette[bw.frame.ColorBoxIdx]))
	}

	// Delegate drawing of children to the root group
	bw.rootGroup.Show(scr)
}

func (bw *BaseWindow) ProcessKey(e *vtinput.InputEvent) bool {
	if e.Type == vtinput.FocusEventType {
		bw.SetFocus(e.SetFocus)
		return true
	}

	if !e.KeyDown {
		return false
	}

	// First, let the group handle focus cycling and item-specific keys
	if bw.rootGroup.ProcessKey(e) {
		return true
	}

	// If group didn't handle it, check for window-level keys
	if e.VirtualKeyCode == vtinput.VK_F5 && bw.ShowZoom {
		bw.ToggleZoom()
		return true
	}

	switch e.VirtualKeyCode {
	case vtinput.VK_F1:
		bw.ShowHelp()
		return true
	case vtinput.VK_ESCAPE, vtinput.VK_F10:
		bw.Close()
		return true
	case vtinput.VK_RETURN:
		// Fallback for Enter: trigger default action if a focused element didn't handle it
		if bw.rootGroup.TriggerDefaultAction() {
			return true
		}
		DebugLog("DEBUG: BaseWindow found NO default action in rootGroup")
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
	if nw < bw.MinW {
		nw = bw.MinW
	}
	if nh < bw.MinH {
		nh = bw.MinH
	}
	dx := nw - bw.lastW
	dy := nh - bw.lastH
	if dx == 0 && dy == 0 {
		return
	}

	bw.X2 += dx
	bw.Y2 += dy
	DebugLog("WINDOW: Resized %q to %dx%d (Delta: %dx%d)", bw.frame.title, nw, nh, dx, dy)
	bw.frame.SetPosition(bw.X1, bw.Y1, bw.X2, bw.Y2)
	bw.rootGroup.Resize(dx, dy)

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

func (bw *BaseWindow) ProcessMouse(e *vtinput.InputEvent) bool {
	// 1. Сначала пробуем обработать клик элементами внутри окна
	if bw.rootGroup.ProcessMouse(e) {
		return true
	}
	// 2. Если элементы не обработали, пробуем операции с самим окном
	return bw.handleWindowOperations(e)
}

func (bw *BaseWindow) handleWindowOperations(e *vtinput.InputEvent) bool {
	mx, my := int(e.MouseX), int(e.MouseY)

	if bw.isDragging {
		if e.ButtonState == 0 {
			bw.isDragging = false
		} else {
			bw.MoveRelative(mx-bw.dragOffX-bw.X1, my-bw.dragOffY-bw.Y1)
		}
		return true
	}

	if bw.isResizing {
		if e.ButtonState == 0 {
			bw.isResizing = false
		} else {
			bw.ChangeSize(mx-bw.X1+1, my-bw.Y1+1)
		}
		return true
	}

	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		// Border clicks
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
		if bw.HitTest(mx, my) {
			bw.isDragging = true
			bw.dragOffX, bw.dragOffY = mx-bw.X1, my-bw.Y1
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
	bw.rootGroup.MoveRelative(dx, dy)
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
	return bw.rootGroup.HandleBroadcast(cmd, args)
}
func (bw *BaseWindow) Valid(cmd int) bool {
	return bw.rootGroup.Valid(cmd)
}

func (bw *BaseWindow) HasShadow() bool { return true }
// SetData populates UI elements from a struct using field names or `vtui` tags.
func (bw *BaseWindow) SetData(record any) {
	bw.rootGroup.SetData(record)
}

// GetData populates a struct from UI elements using field names or `vtui` tags.
func (bw *BaseWindow) GetData(record any) {
	bw.rootGroup.GetData(record)
}
