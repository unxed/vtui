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
	frame    *Frame
}

func NewDialog(x1, y1, x2, y2 int, title string) *Dialog {
	d := &Dialog{
		items:    []UIElement{},
		focusIdx: -1,
		frame:    NewFrame(x1, y1, x2, y2, DoubleBox, title),
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
	d.frame.DisplayObject(scr)
	for _, item := range d.items {
		item.Show(scr)
	}
}

// ProcessKey manages focus switching and passes events to elements.
func (d *Dialog) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown { return false }

	// 1. Handle Tab (focus switching)
	if e.VirtualKeyCode == vtinput.VK_TAB {
		d.nextFocus()
		return true
	}

	// 2. Pass the event to the active element
	if d.focusIdx != -1 {
		if d.items[d.focusIdx].ProcessKey(e) {
			return true
		}
	}

	return false
}

func (d *Dialog) nextFocus() {
	if len(d.items) == 0 { return }

	// Remove focus from the current element
	if d.focusIdx != -1 {
		d.items[d.focusIdx].SetFocus(false)
	}

	// Find the next suitable element
	startIdx := d.focusIdx
	for {
		d.focusIdx = (d.focusIdx + 1) % len(d.items)
		if d.items[d.focusIdx].CanFocus() || d.focusIdx == startIdx {
			break
		}
	}

	d.items[d.focusIdx].SetFocus(true)
}// ProcessMouse обрабатывает события мыши, передавая их соответствующему элементу.
func (d *Dialog) ProcessMouse(e *vtinput.InputEvent) bool {
	mx, my := int(e.MouseX), int(e.MouseY)

	// Проверяем, попал ли клик в какой-либо элемент диалога
	for i, item := range d.items {
		x1, y1, x2, y2 := item.GetPosition()
		if mx >= x1 && mx <= x2 && my >= y1 && my <= y2 {

			// Если элемент может принимать фокус и был клик левой кнопкой — передаем фокус
			if item.CanFocus() && d.focusIdx != i && e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
				if d.focusIdx != -1 {
					d.items[d.focusIdx].SetFocus(false)
				}
				d.focusIdx = i
				item.SetFocus(true)
			}

			// Пробрасываем событие элементу
			if item.ProcessMouse(e) {
				return true
			}
		}
	}

	return false
}
