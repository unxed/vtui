package vtui

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// ScreenBuf является полным аналогом far2l/scrbuf.cpp.
// Он реализует двойную буферизацию для минимизации операций записи в терминал.
type ScreenBuf struct {
	mu            sync.Mutex
	buf           []CharInfo // 'buf' - целевое состояние экрана, которое формирует логика UI.
	shadow        []CharInfo // 'shadow' - состояние, которое было последний раз отрисовано в терминале.
	width, height int

	cursorX, cursorY int
	cursorVisible    bool
	cursorSize       uint32

	lockCount int
	dirty     bool // Флаг, указывающий на необходимость полной перезаписи при следующем Flush.
}

// NewScreenBuf создает новый экземпляр ScreenBuf.
func NewScreenBuf() *ScreenBuf {
	return &ScreenBuf{
		dirty: true, // Изначально буфер "грязный"
	}
}

// AllocBuf выделяет или перераспределяет память для буферов экрана.
func (s *ScreenBuf) AllocBuf(width, height int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if width == s.width && height == s.height {
		return
	}

	if width <= 0 || height <= 0 {
		s.buf = nil
		s.shadow = nil
		s.width = 0
		s.height = 0
		return
	}

	size := width * height
	newBuf := make([]CharInfo, size)
	newShadow := make([]CharInfo, size)

	if newBuf == nil || newShadow == nil {
		// В Go принято возвращать ошибку, но для критической ошибки, как
		// нехватка памяти для экрана, паника оправдана и соответствует
		// поведению far2l (abort).
		panic(fmt.Sprintf("FATAL: Failed to allocate screen buffer (%d x %d)", width, height))
	}

	s.buf = newBuf
	s.shadow = newShadow
	s.width = width
	s.height = height
	s.dirty = true // После изменения размера нужна полная перерисовка
}

// Write записывает срез CharInfo в виртуальный буфер по указанным координатам.
func (s *ScreenBuf) Write(x, y int, text []CharInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.buf == nil || y < 0 || y >= s.height || x >= s.width {
		return
	}

	// Отсечение за левой границей
	if x < 0 {
		if -x >= len(text) {
			return
		}
		text = text[-x:]
		x = 0
	}

	// Отсечение за правой границей
	if x+len(text) > s.width {
		text = text[:s.width-x]
	}

	if len(text) == 0 {
		return
	}

	offset := y*s.width + x
	copy(s.buf[offset:], text)
	// Примечание: пока не сравниваем с shadow, просто копируем.
	// Оптимизация сравнения будет в Flush().
}

// ApplyColor применяет указанные атрибуты к прямоугольной области.
func (s *ScreenBuf) ApplyColor(x1, y1, x2, y2 int, attributes uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.buf == nil {
		return
	}

	// Отсечение по границам экрана
	if x1 < 0 { x1 = 0 }
	if y1 < 0 { y1 = 0 }
	if x2 >= s.width { x2 = s.width - 1 }
	if y2 >= s.height { y2 = s.height - 1 }

	for y := y1; y <= y2; y++ {
		offset := y*s.width + x1
		for x := 0; x <= x2-x1; x++ {
			s.buf[offset+x].Attributes = attributes
		}
	}
}

// FillRect заполняет прямоугольную область указанным символом и атрибутами.
func (s *ScreenBuf) FillRect(x1, y1, x2, y2 int, char rune, attributes uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.buf == nil { return }
	if x1 < 0 { x1 = 0 }
	if y1 < 0 { y1 = 0 }
	if x2 >= s.width { x2 = s.width - 1 }
	if y2 >= s.height { y2 = s.height - 1 }
	cell := CharInfo{Char: uint64(char), Attributes: attributes}
	for y := y1; y <= y2; y++ {
		offset := y*s.width + x1
		for x := 0; x <= x2-x1; x++ {
			s.buf[offset+x] = cell
		}
	}
}

func (s *ScreenBuf) SetCursorPos(x, y int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cursorX, s.cursorY = x, y
}

func (s *ScreenBuf) SetCursorVisible(visible bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cursorVisible = visible
}


// Таблицы для быстрого преобразования палитры в ANSI-коды.
var (
	ansiFg = []string{"30", "34", "32", "36", "31", "35", "33", "37"}
	ansiBg = []string{"40", "44", "42", "46", "41", "45", "43", "47"}
)

// rgb извлекает R, G, B компоненты из 24-битного цвета (формат 0xRRGGBB).
func rgb(c uint32) (r, g, b byte) {
	return byte((c >> 16) & 0xFF), byte((c >> 8) & 0xFF), byte(c & 0xFF)
}

// attributesToANSI генерирует минимальную ANSI-последовательность для перехода от lastAttr к attr.
func attributesToANSI(attr, lastAttr uint64) string {
	if attr == lastAttr {
		return ""
	}

	var params []string

	// 1. Проверка изменения флагов (подчеркивание, инверсия)
	const simpleFlags = CommonLvbUnderscore | CommonLvbReverse
	if (attr & simpleFlags) != (lastAttr & simpleFlags) {
		params = append(params, "0")
		if attr&CommonLvbUnderscore != 0 { params = append(params, "4") }
		if attr&CommonLvbReverse != 0 { params = append(params, "7") }
	}

	// 2. Цвет текста (Foreground)
	fgChanged := false
	if (attr & ForegroundTrueColor) != (lastAttr & ForegroundTrueColor) {
		fgChanged = true
	} else if attr&ForegroundTrueColor != 0 {
		if GetRGBFore(attr) != GetRGBFore(lastAttr) { fgChanged = true }
	} else if (attr & (ForegroundRGB | ForegroundIntensity)) != (lastAttr & (ForegroundRGB | ForegroundIntensity)) {
		fgChanged = true
	}

	if fgChanged {
		if attr&ForegroundTrueColor != 0 {
			r, g, b := rgb(GetRGBFore(attr))
			params = append(params, fmt.Sprintf("38;2;%d;%d;%d", r, g, b))
		} else {
			fg := attr & 0b1111
			if fg&ForegroundIntensity != 0 {
				params = append(params, "1", ansiFg[fg&0b0111])
			} else {
				params = append(params, "22", ansiFg[fg])
			}
		}
	}

	// 3. Цвет фона (Background)
	bgChanged := false
	if (attr & BackgroundTrueColor) != (lastAttr & BackgroundTrueColor) {
		bgChanged = true
	} else if attr&BackgroundTrueColor != 0 {
		if GetRGBBack(attr) != GetRGBBack(lastAttr) { bgChanged = true }
	} else if (attr & (BackgroundRGB | BackgroundIntensity)) != (lastAttr & (BackgroundRGB | BackgroundIntensity)) {
		bgChanged = true
	}

	if bgChanged {
		if attr&BackgroundTrueColor != 0 {
			r, g, b := rgb(GetRGBBack(attr))
			params = append(params, fmt.Sprintf("48;2;%d;%d;%d", r, g, b))
		} else {
			bg := (attr >> 4) & 0b1111
			params = append(params, ansiBg[bg&0b0111])
		}
	}

	if len(params) == 0 {
		return ""
	}

	return "\x1b[" + strings.Join(params, ";") + "m"
}

// Flush сравнивает `buf` и `shadow` и выводит разницу в терминал.
func (s *ScreenBuf) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.lockCount > 0 || s.buf == nil {
		return
	}

	var builder strings.Builder

	// 1. Скрываем курсор, чтобы избежать мерцания во время отрисовки.
	builder.WriteString("\x1b[?25l")

	lastAttr := ^uint64(0) // Невалидное значение, чтобы первая ячейка всегда устанавливала цвет.
	lastX, lastY := -1, -1

	// 2. Основной цикл сравнения и генерации последовательностей.
	for y := 0; y < s.height; y++ {
		for x := 0; x < s.width; x++ {
			offset := y*s.width + x

			// Если ячейка не изменилась, пропускаем.
			if !s.dirty && s.buf[offset] == s.shadow[offset] {
				continue
			}

			// Если есть разрыв, перемещаем курсор.
			if x != lastX+1 || y != lastY {
				// ANSI-координаты начинаются с 1.
				builder.WriteString(fmt.Sprintf("\x1b[%d;%dH", y+1, x+1))
			}

			// Устанавливаем цвет, если он изменился.
			attr := s.buf[offset].Attributes
			builder.WriteString(attributesToANSI(attr, lastAttr))
			lastAttr = attr

			// Выводим символ.
			// TODO: Обработка композитных символов (Char > 0xFFFF)
			char := rune(s.buf[offset].Char)
			if char == 0 {
				builder.WriteByte(' ') // Нулевой символ рисуем как пробел
			} else {
				builder.WriteRune(char)
			}

			lastX, lastY = x, y
		}
	}
	s.dirty = false
	copy(s.shadow, s.buf)

	// 3. Перемещаем курсор в его финальную позицию и делаем видимым, если нужно.
	builder.WriteString(fmt.Sprintf("\x1b[%d;%dH", s.cursorY+1, s.cursorX+1))
	if s.cursorVisible {
		builder.WriteString("\x1b[?25h")
	}

	// 4. Однократная запись в stdout.
	if builder.Len() > 0 {
		os.Stdout.WriteString(builder.String())
	}
}