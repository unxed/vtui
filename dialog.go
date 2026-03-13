package vtui

import "github.com/unxed/vtinput"

// UIElement — интерфейс, которому должны соответствовать все элементы диалога.
type UIElement interface {
	Show(scr *ScreenBuf)
	Hide(scr *ScreenBuf)
	SetFocus(bool)
	IsFocused() bool
	CanFocus() bool
	ProcessKey(e *vtinput.InputEvent) bool
	ProcessMouse(e *vtinput.InputEvent) bool
}

// Dialog — контейнер для UI-элементов, управляющий фокусом и событиями.
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

// AddItem добавляет элемент в диалог.
func (d *Dialog) AddItem(item UIElement) {
	d.items = append(d.items, item)
	// Если это первый фокусируемый элемент, отдаем ему фокус
	if d.focusIdx == -1 && item.CanFocus() {
		d.focusIdx = len(d.items) - 1
		item.SetFocus(true)
	}
}

// Show отрисовывает диалог и все его элементы.
func (d *Dialog) Show(scr *ScreenBuf) {
	d.ScreenObject.Show(scr)
	d.frame.DisplayObject(scr)
	for _, item := range d.items {
		item.Show(scr)
	}
}

// ProcessKey управляет переключением фокуса и передает события элементам.
func (d *Dialog) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown { return false }

	// 1. Обработка Tab (переключение фокуса)
	if e.VirtualKeyCode == vtinput.VK_TAB {
		d.nextFocus()
		return true
	}

	// 2. Передача события активному элементу
	if d.focusIdx != -1 {
		if d.items[d.focusIdx].ProcessKey(e) {
			return true
		}
	}

	return false
}

func (d *Dialog) nextFocus() {
	if len(d.items) == 0 { return }

	// Снимаем фокус с текущего
	if d.focusIdx != -1 {
		d.items[d.focusIdx].SetFocus(false)
	}

	// Ищем следующий подходящий элемент
	startIdx := d.focusIdx
	for {
		d.focusIdx = (d.focusIdx + 1) % len(d.items)
		if d.items[d.focusIdx].CanFocus() || d.focusIdx == startIdx {
			break
		}
	}

	d.items[d.focusIdx].SetFocus(true)
}