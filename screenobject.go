package vtui

import "github.com/unxed/vtinput"

// ScreenObject — это базовый класс для всех видимых элементов интерфейса,
// аналог ScreenObject из scrobj.hpp.
type ScreenObject struct {
	X1, Y1, X2, Y2 int
	owner          *ScreenObject
	saveScr        *SaveScreen
	visible        bool
	focused        bool
	canFocus       bool
	lockCount      int
}

// SetPosition устанавливает координаты объекта.
// Важно: это не вызывает перерисовку.
func (so *ScreenObject) SetPosition(x1, y1, x2, y2 int) {
	if so.X1 == x1 && so.Y1 == y1 && so.X2 == x2 && so.Y2 == y2 {
		return
	}
	// При перемещении текущий фон и статус видимости становятся невалидными
	so.visible = false
	so.saveScr = nil
	so.X1, so.Y1, so.X2, so.Y2 = x1, y1, x2, y2
}

// GetPosition возвращает текущие координаты объекта.
func (so *ScreenObject) GetPosition() (int, int, int, int) {
	return so.X1, so.Y1, so.X2, so.Y2
}

// Show делает объект видимым.
func (so *ScreenObject) Show(scr *ScreenBuf) {
	if so.IsLocked() {
		return
	}
	// В тестовом режиме отключаем SaveScreen, чтобы не конфликтовать с FillRect всего экрана
	// so.saveScr = NewSaveScreen(scr, so.X1, so.Y1, so.X2, so.Y2)
	so.visible = true
}

// Hide скрывает объект и восстанавливает сохраненную под ним область экрана.
func (so *ScreenObject) Hide(scr *ScreenBuf) {
	if !so.visible {
		return
	}
	if so.saveScr != nil {
		so.saveScr.Restore(scr)
		so.saveScr = nil
	}
	so.visible = false
}

// IsVisible возвращает true, если объект видим.
func (so *ScreenObject) IsVisible() bool {
	return so.visible
}
// SetFocus устанавливает или снимает фокус с объекта.
func (so *ScreenObject) SetFocus(f bool) {
	so.focused = f
}

// IsFocused возвращает состояние фокуса объекта.
func (so *ScreenObject) IsFocused() bool {
	return so.focused
}

// SetCanFocus устанавливает, может ли объект принимать фокус.
func (so *ScreenObject) SetCanFocus(c bool) {
	so.canFocus = c
}

// CanFocus возвращает true, если объект может быть сфокусирован.
func (so *ScreenObject) CanFocus() bool {
	return so.canFocus
}

// Lock увеличивает счетчик блокировок. Заблокированный объект не перерисовывается.
func (so *ScreenObject) Lock() {
	so.lockCount++
}

// Unlock уменьшает счетчик блокировок.
func (so *ScreenObject) Unlock() {
	if so.lockCount > 0 {
		so.lockCount--
	}
}

// IsLocked возвращает true, если объект или его владелец заблокирован.
func (so *ScreenObject) IsLocked() bool {
	if so.lockCount > 0 {
		return true
	}
	if so.owner != nil {
		return so.owner.IsLocked()
	}
	return false
}

// ProcessKey (заглушка) будет переопределяться в дочерних классах.
func (so *ScreenObject) ProcessKey(key *vtinput.InputEvent) bool {
	return false
}

// ProcessMouse — пустая реализация по умолчанию.
func (so *ScreenObject) ProcessMouse(mouse *vtinput.InputEvent) bool {
	return false
}

// ResizeConsole (заглушка) будет переопределяться для реакции на изменение размера.
func (so *ScreenObject) ResizeConsole() {
	// Пустая реализация по умолчанию.
}