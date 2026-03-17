package main

import (
	"fmt"
	"os"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"golang.org/x/term"
)

// fileRow реализует vtui.TableRow для тестирования таблицы
type fileRow struct {
	name string
	size string
	date string
}

func (f fileRow) GetCellText(col int) string {
	switch col {
	case 0:
		return f.name
	case 1:
		return f.size
	case 2:
		return f.date
	}
	return ""
}

func main() {
	// 1. Включаем расширенный режим терминала
	restore, err := vtinput.Enable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error enabling raw mode: %v\n", err)
		return
	}
	defer restore()

	// Скрываем курсор и восстанавливаем его при выходе
	fmt.Print("\x1b[?25l")
	defer fmt.Print("\x1b[?25h")

	// 2. Получаем размер терминала и создаем экранный буфер
	width, height, _ := term.GetSize(int(os.Stdin.Fd()))
	scr := vtui.NewScreenBuf()
	scr.AllocBuf(width, height)

	// 3. Инициализируем FrameManager
	vtui.FrameManager.Init(scr)

	// Создаем фоновый слой Desktop
	desktop := vtui.NewDesktop()
	vtui.FrameManager.Push(desktop)

	// 4. Создаем демонстрационный диалог
	dlgWidth, dlgHeight := 60, 22
	x1 := (width - dlgWidth) / 2
	y1 := (height - dlgHeight) / 2
	dlg := vtui.NewDialog(x1, y1, x1+dlgWidth-1, y1+dlgHeight-1, " UI Components Test ")
	dlg.SetHelp("MainDialogTopic")

	// --- Определение и добавление элементов в порядке навигации (Tab) ---

	// 1. Текстовое поле ввода
	labelEdit := vtui.NewText(x1+2, y1+1, "Input field:", vtui.SetRGBFore(0, 0xFFFFFF))
	edit := vtui.NewEdit(x1+15, y1+1, 40, "vtui")
	dlg.AddItem(labelEdit)
	dlg.AddItem(edit)

	// 2. Таблица (высота 6 строк)
	tableCols := []vtui.TableColumn{
		{Title: "Name", Width: 25},
		{Title: "Size", Width: 10, Alignment: vtui.AlignRight},
		{Title: "Date", Width: 12},
	}
	table := vtui.NewTable(x1+2, y1+3, 56, 6, tableCols)
	table.SetRows([]vtui.TableRow{
		fileRow{"kernel.go", "4 KB", "2024-03-10"},
		fileRow{"🚀 rocket.exe", "12 KB", "2024-03-11"},
		fileRow{"日本語.txt", "2 KB", "2024-03-12"},
		fileRow{"main.go", "2 KB", "2024-03-12"},
	})
	dlg.AddItem(table)

	// 3. Радиокнопки (одна группа)
	dlg.AddItem(vtui.NewText(x1+2, y1+10, "Options:", vtui.Palette[vtui.ColDialogText]))
	rb1 := vtui.NewRadioButton(x1+15, y1+10, "Option A")
	rb1.Selected = true
	dlg.AddItem(rb1)
	dlg.AddItem(vtui.NewRadioButton(x1+30, y1+10, "Option B"))

	// 4. Чекбоксы (обычный и с тремя состояниями)
	dlg.AddItem(vtui.NewText(x1+2, y1+11, "Checks:", vtui.Palette[vtui.ColDialogText]))
	dlg.AddItem(vtui.NewCheckbox(x1+15, y1+11, "Normal", false))
	dlg.AddItem(vtui.NewCheckbox(x1+30, y1+11, "3-State", true))

	// 5. Поле ввода пароля (PasswordMode)
	dlg.AddItem(vtui.NewText(x1+2, y1+12, "Pass:", vtui.Palette[vtui.ColDialogText]))
	pedit := vtui.NewEdit(x1+15, y1+12, 20, "secret")
	pedit.PasswordMode = true
	dlg.AddItem(pedit)

	// 6. ComboBox (Выпадающий список)
	dlg.AddItem(vtui.NewText(x1+2, y1+13, "Combo:", vtui.Palette[vtui.ColDialogText]))
	comboItems := []string{"Red", "Green", "Blue", "Alpha", "Cyan", "Magenta"}
	dlg.AddItem(vtui.NewComboBox(x1+15, y1+13, 20, comboItems))

	// 7. Вертикальное меню (VMenu)
	menu := vtui.NewVMenu(" Operations ")
	menu.SetHelp("MenuOperationsTopic")
	menu.SetPosition(x1+2, y1+15, x1+30, y1+19)
	menu.AddItem("Copy File")
	menu.AddItem("Move File")
	menu.AddSeparator()
	menu.AddItem("Delete File")
	dlg.AddItem(menu)

	// 8. Кнопки управления
	btnOk := vtui.NewButton(x1+10, y1+20, "Ok")
	btnCancel := vtui.NewButton(x1+30, y1+20, "Cancel")

	// Действия при нажатии на кнопки
	btnCancel.OnClick = func() {
		dlg.SetExitCode(-1)
		desktop.SetExitCode(-1)
	}
	btnOk.OnClick = func() {
		dlg.SetExitCode(0)
		desktop.SetExitCode(0)
	}

	dlg.AddItem(btnOk)
	dlg.AddItem(btnCancel)

	// 5. Добавляем диалог в стек FrameManager и запускаем цикл событий
	vtui.FrameManager.Push(dlg)
	vtui.FrameManager.Run()
}
