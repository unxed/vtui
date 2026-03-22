package vtui

// Bar — базовый структурный примитив для однострочных горизонтальных панелей.
type Bar struct {
	ScreenObject
}

// SetPosition переопределяет метод ScreenObject, чтобы гарантировать высоту в 1 строку.
func (b *Bar) SetPosition(x1, y1, x2, y2 int) {
	b.ScreenObject.SetPosition(x1, y1, x2, y1)
}

// DrawBackground заполняет всю полосу бара указанным атрибутом.
func (b *Bar) DrawBackground(scr *ScreenBuf, attr uint64) {
	if b.IsVisible() {
		scr.FillRect(b.X1, b.Y1, b.X2, b.Y1, ' ', attr)
	}
}