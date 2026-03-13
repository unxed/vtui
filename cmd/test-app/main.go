package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"golang.org/x/term"
)

func main() {
	// Включаем "продвинутый" режим терминала
	restore, err := vtinput.Enable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error enabling raw mode: %v\n", err)
		return
	}
	defer restore()

	// Скрываем курсор на время работы
	fmt.Print("\x1b[?25l")
	defer fmt.Print("\x1b[?25h")

	// Получаем начальные размеры терминала
	width, height, _ := term.GetSize(int(os.Stdin.Fd()))

	// Создаем наши главные объекты
	scr := vtui.NewScreenBuf()
	scr.AllocBuf(width, height)

	// Вычисляем координаты для центрирования на старте
	fWidth, fHeight := 40, 10
	x1 := (width - fWidth) / 2
	y1 := (height - fHeight) / 2
	frame := vtui.NewFrame(x1, y1, x1+fWidth-1, y1+fHeight-1, vtui.DoubleBox, "Test Frame")

	// Настраиваем канал для получения событий от vtinput
	reader := vtinput.NewReader(os.Stdin)
	eventChan := make(chan *vtinput.InputEvent, 1)
	go func() {
		for {
			e, err := reader.ReadEvent()
			if err != nil {
				if err != io.EOF {
					// Можно добавить логирование ошибки
				}
				close(eventChan)
				return
			}
			eventChan <- e
		}
	}()

	// Настраиваем канал для отслеживания изменения размера окна
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGWINCH)

	// --- Главный цикл приложения ---
	for {
		// 1. Отрисовка
		drawUI(scr, frame, width, height)

		// 2. Ожидание события (клавиатура или ресайз)
		select {
		case e, ok := <-eventChan:
			if !ok { // Канал закрыт, выходим
				return
			}
			if handleKeyEvent(e, frame) { // handleKeyEvent вернет true, если надо выйти
				return
			}

		case <-sigChan:
			width, height, _ = term.GetSize(int(os.Stdin.Fd()))
			scr.AllocBuf(width, height)
			// Просто перецентрируем рамку для наглядности
			frameWidth, frameHeight := 40, 10
			newX1 := (width - frameWidth) / 2
			newY1 := (height - frameHeight) / 2
			frame.SetPosition(newX1, newY1, newX1+frameWidth-1, newY1+frameHeight-1)
		}
	}
}

// drawUI отвечает за отрисовку всего интерфейса на ScreenBuf и вызов Flush.
func drawUI(scr *vtui.ScreenBuf, frame *vtui.Frame, width, height int) {
	// Очищаем буфер синим цветом (Far-style: 0x000080 или чуть светлее)
	farBlue := vtui.SetRGBBack(0, 0x0000A0)
	scr.FillRect(0, 0, width-1, height-1, ' ', farBlue)

	// Показываем наш Frame (он сохранит под собой фон и отрисует себя)
	frame.Show(scr)

	// Рисуем строку состояния внизу
	status := fmt.Sprintf(" Size: %dx%d | Use Arrows to move, ESC to quit ", width, height)
	statusInfo := strToCharInfo(status, vtui.SetRGBBoth(0, 0, 0x007BA7)) // Белый на синем
	scr.Write(0, height-1, statusInfo)

	// Отправляем все изменения в терминал
	scr.Flush()
}

// handleKeyEvent обрабатывает события клавиатуры. Возвращает true для выхода.
func handleKeyEvent(e *vtinput.InputEvent, frame *vtui.Frame) bool {
	if e.Type != vtinput.KeyEventType || !e.KeyDown {
		return false
	}

	x1, y1, x2, y2 := frame.GetPosition()

	switch e.VirtualKeyCode {
	case vtinput.VK_ESCAPE:
		return true
	case vtinput.VK_C:
		if e.ControlKeyState&vtinput.LeftCtrlPressed != 0 {
			return true
		}
	case vtinput.VK_UP:
		y1, y2 = y1-1, y2-1
	case vtinput.VK_DOWN:
		y1, y2 = y1+1, y2+1
	case vtinput.VK_LEFT:
		x1, x2 = x1-1, x2-1
	case vtinput.VK_RIGHT:
		x1, x2 = x1+1, x2+1
	}

	frame.SetPosition(x1, y1, x2, y2)
	return false
}

// Вспомогательная функция для быстрой конвертации строки в []CharInfo
func strToCharInfo(str string, attributes uint64) []vtui.CharInfo {
	runes := []rune(str)
	info := make([]vtui.CharInfo, len(runes))
	for i, r := range runes {
		info[i].Char = uint64(r)
		info[i].Attributes = attributes
	}
	return info
}