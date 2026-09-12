package vtui

import (
	"fmt"

	"github.com/unxed/vtinput"
)

// BaseWindow provides generic windowing logic (moving, resizing, focus cycle).
type BaseWindow struct {
	BaseFrame
	rootGroup          *Group
	frame              *BorderedFrame
	isDragging         bool
	isResizing         bool
	dragOffX           int
	dragOffY           int
	lastW              int
	lastH              int
	MinW               int
	MinH               int
	ShowClose          bool
	ShowZoom           bool
	SavedBounds        *Rect
	progress           int
	IsWarning          bool
	ColorBoxIdx        int
	ColorTitleIdx      int
	ColorBackgroundIdx int
	initialFocusItem   UIElement
}

func (bw *BaseWindow) GetFocusedItem() UIElement {
	return bw.rootGroup.GetFocusedItem()
}
func (bw *BaseWindow) GetChildren() []UIElement {
	return bw.rootGroup.GetChildren()
}

// GetBorderThickness reports how many cells of the window bounds are taken by
// its own frame. It implements BorderedContainer for the layout validator and
// is inherited by every application type embedding BaseWindow.
func (bw *BaseWindow) GetBorderThickness() int {
	if bw.frame == nil {
		return 0
	}
	return bw.frame.GetBorderThickness()
}
func (bw *BaseWindow) SetFocusedItem(item UIElement) {
	bw.rootGroup.SetFocusedItem(item)
}

func NewBaseWindow(x1, y1, x2, y2 int, title string) *BaseWindow {
	bw := &BaseWindow{
		frame:              NewBorderedFrame(x1, y1, x2, y2, DoubleBox, title),
		MinW:               x2 - x1 + 1,
		MinH:               y2 - y1 + 1,
		progress:           -1,
		ColorBoxIdx:        ColDialogBox,
		ColorTitleIdx:      ColDialogBoxTitle,
		ColorBackgroundIdx: ColDialogText,
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
func (bw *BaseWindow) SetPosition(x1, y1, x2, y2 int) {
	bw.ScreenObject.SetPosition(x1, y1, x2, y2)
	if bw.frame != nil {
		bw.frame.SetPosition(x1, y1, x2, y2)
	}
	if bw.rootGroup != nil {
		bw.rootGroup.SetPosition(x1+1, y1+1, x2-1, y2-1)
	}
	bw.lastW = x2 - x1 + 1
	bw.lastH = y2 - y1 + 1
}

func (bw *BaseWindow) GetPaletteIndex(baseIdx int) int {
	if !bw.IsWarning {
		return baseIdx
	}
	switch baseIdx {
	case ColDialogText:
		return ColWarnText
	case ColDialogHighlightText:
		return ColWarnHighlightText
	case ColDialogBox:
		return ColWarnBox
	case ColDialogBoxTitle:
		return ColWarnBoxTitle
	case ColDialogHighlightBoxTitle:
		return ColWarnHighlightBoxTitle
	case ColDialogEdit, ColDialogEditUnchanged, ColDialogEditSelected, ColDialogComboText, ColDialogComboSelectedText:
		return ColWarnEdit
	case ColDialogComboHighlight, ColDialogComboSelectedHighlight:
		return ColWarnHighlightText
	case ColDialogComboBox:
		return ColWarnBox
	case ColDialogButton:
		return ColWarnButton
	case ColDialogSelectedButton:
		return ColWarnSelectedButton
	case ColDialogHighlightButton:
		return ColWarnHighlightButton
	case ColDialogHighlightSelectedButton:
		return ColWarnHighlightSelectedButton
	}
	return baseIdx
}

func (bw *BaseWindow) ReleaseMouseCapture() { bw.rootGroup.ReleaseMouseCapture() }

func (bw *BaseWindow) SetFocus(f bool) {
	bw.ScreenObject.SetFocus(f)
	bw.rootGroup.SetFocus(f)
	if f && bw.initialFocusItem == nil {
		bw.initialFocusItem = bw.GetFocusedItem()
	}
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

// AddLink delegates the automation link to the root group.
func (bw *BaseWindow) AddLink(src, target UIElement, action LinkAction) {
	bw.rootGroup.AddLink(src, target, action)
}

func (bw *BaseWindow) Show(scr *ScreenBuf) {
	bw.ScreenObject.Show(scr)
	bw.frame.ShowClose = bw.ShowClose

	boxIdx := bw.ColorBoxIdx
	if boxIdx == 0 {
		boxIdx = ColDialogBox
	}
	titleIdx := bw.ColorTitleIdx
	if titleIdx == 0 {
		titleIdx = ColDialogBoxTitle
	}
	bgIdx := bw.ColorBackgroundIdx
	if bgIdx == 0 {
		bgIdx = ColDialogText
	}

	// far2l paints a dialog title with Dialog.Box.Title no matter which window
	// holds focus; Dialog.Box.Title.Highlight is the hotkey colour inside a
	// title, not a focused variant. Panels track focus through their own frame
	// colours instead.
	bw.frame.ColorBoxIdx = bw.GetPaletteIndex(boxIdx)
	bw.frame.ColorTitleIdx = bw.GetPaletteIndex(titleIdx)
	bw.frame.ColorBackgroundIdx = bw.GetPaletteIndex(bgIdx)

	bw.frame.DisplayObject(scr)

	if bw.ShowZoom {
		zoomStr := string(UIStrings.CloseBrackets[0]) + string(UIStrings.ZoomSymbol) + string(UIStrings.CloseBrackets[1])
		offset := bw.frame.getControlOffset()
		if bw.ShowClose {
			offset += 3
		}
		controlAttr := withForeground(Palette[bw.frame.ColorBoxIdx], Palette[bw.frame.ColorTitleIdx])
		scr.Write(bw.X2-offset, bw.Y1, StringToCharInfo(zoomStr, controlAttr))
	}
	if bw.Number > 0 && bw.Number <= 9 {
		numStr := fmt.Sprintf("%c%d%c", UIStrings.CloseBrackets[0], bw.Number, UIStrings.CloseBrackets[1])
		scr.Write(bw.X1+2, bw.Y1, StringToCharInfo(numStr, Palette[bw.frame.ColorBoxIdx]))
	}

	// A short modal dialog can retain full-size contents and scroll them
	// through its smaller visible viewport. Normal windows keep their existing
	// drawing behaviour.
	if bw.Modal {
		scr.PushClipRect(bw.X1+1, bw.Y1+1, bw.X2-1, bw.Y2-1)
		bw.rootGroup.Show(scr)
		scr.PopClipRect()
		return
	}
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
	case vtinput.VK_PRIOR:
		// Jump to initial focus item (PgUp)
		if bw.initialFocusItem != nil {
			for i, item := range bw.rootGroup.items {
				if item == bw.initialFocusItem {
					bw.rootGroup.setFocus(i)
					return true
				}
			}
		}
		// Fallback: jump to the first focusable element
		for i, item := range bw.rootGroup.items {
			if item.CanFocus() && !item.IsDisabled() {
				bw.rootGroup.setFocus(i)
				return true
			}
		}
	case vtinput.VK_NEXT:
		// Jump to default button (PgDn)
		for i, item := range bw.rootGroup.items {
			if btn, ok := item.(*Button); ok && btn.IsDefault && !btn.IsDisabled() {
				bw.rootGroup.setFocus(i)
				return true
			}
		}
		// Fallback: jump to first non-disabled button
		for i, item := range bw.rootGroup.items {
			if btn, ok := item.(*Button); ok && !btn.IsDisabled() {
				bw.rootGroup.setFocus(i)
				return true
			}
		}
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
	// 1. Во время драга дети не должны перехватывать события
	if bw.isDragging || bw.isResizing {
		return bw.handleWindowOperations(e)
	}
	// 2. Сначала пробуем обработать клик элементами внутри окна
	if bw.rootGroup.ProcessMouse(e) {
		return true
	}
	// 3. Если элементы не обработали, пробуем операции с самим окном
	return bw.handleWindowOperations(e)
}

func (bw *BaseWindow) handleWindowOperations(e *vtinput.InputEvent) bool {
	mx, my := int(e.MouseX), int(e.MouseY)

	if bw.isDragging {
		if IsMouseRelease(e) {
			bw.isDragging = false
		} else {
			bw.MoveRelative(mx-bw.dragOffX-bw.X1, my-bw.dragOffY-bw.Y1)
		}
		return true
	}

	if bw.isResizing {
		if IsMouseRelease(e) {
			bw.isResizing = false
		} else {
			bw.ChangeSize(mx-bw.X1+1, my-bw.Y1+1)
		}
		return true
	}

	if e.ButtonState == vtinput.FromLeft1stButtonPressed && IsMousePress(e) {
		offset := bw.frame.getControlOffset()

		// Border clicks
		if bw.ShowClose && my == bw.Y1 && mx >= bw.X2-offset && mx <= bw.X2-offset+2 {
			bw.Close()
			return true
		}

		if bw.ShowClose {
			offset += 3
		}
		if bw.ShowZoom && my == bw.Y1 && mx >= bw.X2-offset && mx <= bw.X2-offset+2 {
			bw.ToggleZoom()
			return true
		}
		if mx == bw.X2 && my == bw.Y2 {
			bw.isResizing = true
			return true
		}
		if bw.HitTest(mx, my) {
			if FrameManager != nil {
				for i := len(FrameManager.frames) - 1; i >= 0; i-- {
					f := FrameManager.frames[i]
					fx1, fy1, fx2, fy2 := f.GetPosition()
					if fx1 == bw.X1 && fy1 == bw.Y1 && fx2 == bw.X2 && fy2 == bw.Y2 {
						FrameManager.PopFramesAbove(f)
						break
					}
				}
			}
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

// setViewportSize changes only the visible window bounds. Unlike ChangeSize,
// it deliberately does not resize or reflow child controls: a modal dialog
// uses it to expose a scrolling viewport for controls that do not fit.
func (bw *BaseWindow) setViewportSize(width, height int) {
	if width < 3 {
		width = 3
	}
	if height < 3 {
		height = 3
	}
	bw.X2 = bw.X1 + width - 1
	bw.Y2 = bw.Y1 + height - 1
	bw.ScreenObject.SetPosition(bw.X1, bw.Y1, bw.X2, bw.Y2)
	if bw.frame != nil {
		bw.frame.SetPosition(bw.X1, bw.Y1, bw.X2, bw.Y2)
	}
	if bw.rootGroup != nil {
		bw.rootGroup.SetPosition(bw.X1+1, bw.Y1+1, bw.X2-1, bw.Y2-1)
	}
	bw.lastW, bw.lastH = width, height
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
func (bw *BaseWindow) SetTitle(title string) {
	if bw.frame != nil {
		bw.frame.SetTitle(title)
	}
}
func (bw *BaseWindow) GetTitle() string {
	if bw.frame != nil {
		return bw.frame.title
	}
	return ""
}
