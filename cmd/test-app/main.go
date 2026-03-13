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
	frame := vtui.NewFrame(x1, y1, x1+fWidth-1, y1+fHeight-1, vtui.DoubleBox, "Background Frame")

	// Создаем меню
	menu := vtui.NewVMenu(" Select Action ")
	menu.AddItem("Copy File")
	menu.AddItem("Move File")
	menu.AddSeparator()
	menu.AddItem("Delete File")
	menu.AddItem("Attributes")
	menu.AddSeparator()
	menu.AddItem("Exit")

	// Явно устанавливаем выбор на первый пункт
	menu.SetSelectPos(0, 1)

	// Устанавливаем фиксированную позицию меню для стабильности теста
	menu.SetPosition(x1+5, y1+2, x1+30, y1+8)
	menu.SetFocus(true) // Меню в фокусе по умолчанию

	// Добавляем текст и кнопки
	label := vtui.NewText(x1+5, y1+1, "Available actions:", vtui.SetRGBFore(0, 0xFFFFFF))
	btnOk := vtui.NewButton(x1+5, y1+10, "Ok")
	btnCancel := vtui.NewButton(x1+15, y1+10, "Cancel")

	// Список элементов для управления фокусом
	type focusable interface {
		SetFocus(bool)
		IsFocused() bool
		ProcessKey(*vtinput.InputEvent) bool
		Show(*vtui.ScreenBuf)
	}
	elements := []focusable{menu, btnOk, btnCancel}
	focusIdx := 0

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
		// Передаем все элементы для отрисовки
		scr.FillRect(0, 0, width-1, height-1, ' ', vtui.SetRGBBack(0, 0x0000A0))
		frame.Show(scr)
		label.Show(scr)
		for _, el := range elements {
			el.Show(scr)
		}

		// Статусбар
		status := fmt.Sprintf(" Size: %dx%d | Tab: Switch Focus | Arrows/ESC ", width, height)
		scr.Write(0, height-1, strToCharInfo(status, vtui.SetRGBBoth(0, 0, 0x007BA7)))
		scr.Flush()

		// 2. Ожидание события
		select {
		case e, ok := <-eventChan:
			if !ok { return }
			if e.Type != vtinput.KeyEventType || !e.KeyDown { continue }

			// Переключение фокуса по TAB
			if e.VirtualKeyCode == vtinput.VK_TAB {
				elements[focusIdx].SetFocus(false)
				focusIdx = (focusIdx + 1) % len(elements)
				elements[focusIdx].SetFocus(true)
				continue
			}

			// Передаем событие активному элементу
			if elements[focusIdx].ProcessKey(e) {
				continue
			}

			if handleKeyEvent(e, frame) {
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