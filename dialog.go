package vtui

import "github.com/unxed/vtinput"

// UIElement is the interface that all dialog elements must implement.
type UIElement interface {
	GetPosition() (int, int, int, int)
	Show(scr *ScreenBuf)
	Hide(scr *ScreenBuf)
	SetFocus(bool)
	IsFocused() bool
	CanFocus() bool
	ProcessKey(e *vtinput.InputEvent) bool
	ProcessMouse(e *vtinput.InputEvent) bool
}

// Dialog is a container for UI elements that manages focus and events.
type Dialog struct {
	ScreenObject
	items    []UIElement
	focusIdx int
	frame    *BorderedFrame
	done     bool
	exitCode int
}

func NewDialog(x1, y1, x2, y2 int, title string) *Dialog {
	d := &Dialog{
		items:    []UIElement{},
		focusIdx: -1,
		frame:    NewBorderedFrame(x1, y1, x2, y2, DoubleBox, title),
	}
	d.SetPosition(x1, y1, x2, y2)
	return d
}

// AddItem adds an element to the dialog.
func (d *Dialog) AddItem(item UIElement) {
	d.items = append(d.items, item)
	// If this is the first focusable element, give it focus
	if d.focusIdx == -1 && item.CanFocus() {
		d.focusIdx = len(d.items) - 1
		item.SetFocus(true)
	}
}

// Show renders the dialog and all its elements.
func (d *Dialog) Show(scr *ScreenBuf) {
	d.ScreenObject.Show(scr)
	d.drawShadow(scr)
	d.frame.DisplayObject(scr)
	for _, item := range d.items {
		item.Show(scr)
	}
}

func (d *Dialog) drawShadow(scr *ScreenBuf) {
	// Тень в Far — это смещение +2 по X и +1 по Y.
	// Рисуем только если тень влезает в границы буфера.
	shAttr := Palette[ColShadow]

	// Вертикальная часть тени (справа)
	// От Y1+1 до Y2+1, в колонках X2+1 и X2+2
	scr.FillRect(d.X2+1, d.Y1+1, d.X2+2, d.Y2+1, ' ', shAttr)

	// Горизонтальная часть тени (снизу)
	// От X1+2 до X2, в строке Y2+1
	scr.FillRect(d.X1+2, d.Y2+1, d.X2, d.Y2+1, ' ', shAttr)
}

// ProcessKey manages focus switching and passes events to elements.
func (d *Dialog) ProcessKey(e *vtinput.InputEvent) bool {

	// 1. Pass the event to the active element first
	if d.focusIdx != -1 {
		if d.items[d.focusIdx].ProcessKey(e) {
			return true
		}

		// Специальная обработка RadioButton: если элемент не поглотил нажатие Space/Enter,
		// но это радиокнопка, мы активируем её и сбрасываем остальные.
		if e.KeyDown && (e.VirtualKeyCode == vtinput.VK_SPACE || e.VirtualKeyCode == vtinput.VK_RETURN) {
			if rb, ok := d.items[d.focusIdx].(*RadioButton); ok {
				d.selectRadio(rb)
				return true
			}
		}
	}

	// 2. Handle global dialog keys
	if !e.KeyDown { return false }

	DebugLog("Dialog.ProcessKey: VK=%X Char=%d FocusIdx=%d", e.VirtualKeyCode, e.Char, d.focusIdx)

	switch e.VirtualKeyCode {
	case vtinput.VK_F1:
		d.ShowHelp()
		return true
	case vtinput.VK_ESCAPE, vtinput.VK_F10:
		DebugLog("Dialog: Close signal")
		d.SetExitCode(-1)
		return true
	case vtinput.VK_TAB:
		shift := (e.ControlKeyState & vtinput.ShiftPressed) != 0
		if shift {
			d.changeFocus(-1) // Назад
		} else {
			d.changeFocus(1) // Вперед
		}
		return true
	}

	return false
}

func (d *Dialog) ResizeConsole(w, h int) {
	// Center the dialog on the new screen size
	dw, dh := d.X2-d.X1+1, d.Y2-d.Y1+1
	x1 := (w - dw) / 2
	y1 := (h - dh) / 2
	d.SetPosition(x1, y1, x1+dw-1, y1+dh-1)
	// Important: We'd need to reposition all internal items here too.
}

func (d *Dialog) GetType() FrameType {
	return TypeDialog
}

func (d *Dialog) SetExitCode(code int) {
	d.done = true
	d.exitCode = code
}

func (d *Dialog) IsDone() bool {
	return d.done
}
func (d *Dialog) IsBusy() bool { return false }

func (d *Dialog) changeFocus(direction int) {
	if len(d.items) == 0 { return }

	// 1. Снимаем фокус с текущего элемента
	if d.focusIdx != -1 {
		d.items[d.focusIdx].SetFocus(false)
	}

	// 2. Ищем следующий/предыдущий фокусируемый элемент
	startIdx := d.focusIdx
	for {
		d.focusIdx += direction
		if d.focusIdx < 0 {
			d.focusIdx = len(d.items) - 1
		}
		if d.focusIdx >= len(d.items) {
			d.focusIdx = 0
		}

		if d.items[d.focusIdx].CanFocus() || d.focusIdx == startIdx {
			break
		}
	}

	d.items[d.focusIdx].SetFocus(true)
}

func (d *Dialog) selectRadio(rb *RadioButton) {
	if rb.Selected { return }
	for _, item := range d.items {
		if other, ok := item.(*RadioButton); ok {
			other.Selected = false
		}
	}
	rb.Selected = true
}

// ProcessMouse handles mouse events, passing them to the appropriate element.
func (d *Dialog) ProcessMouse(e *vtinput.InputEvent) bool {
	mx, my := int(e.MouseX), int(e.MouseY)

	// Check whether the click hit any element of the dialog.
	// Iterate over the elements in reverse order (Z-order: top first)
	for i := len(d.items) - 1; i >= 0; i-- {
		item := d.items[i]
		x1, y1, x2, y2 := item.GetPosition()
		if mx >= x1 && mx <= x2 && my >= y1 && my <= y2 {

			// If it's a left button click, change focus.
			if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
				if item.CanFocus() && d.focusIdx != i {
					if d.focusIdx != -1 {
						d.items[d.focusIdx].SetFocus(false)
					}
					d.focusIdx = i
					item.SetFocus(true)
				}
			}

			// Always propagate an event to the element under the mouse
			if item.ProcessMouse(e) {
				return true
			}
			// Специальная обработка клика по RadioButton
			if rb, ok := item.(*RadioButton); ok && e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
				d.selectRadio(rb)
				return true
			}

			// If an element absorbs a click (even if it returns false),
			// prevent clicks through it (important for overlapping elements)
			return true
		}
	}

	return false
}
