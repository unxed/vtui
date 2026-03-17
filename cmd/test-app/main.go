package main

import (
	"fmt"
	"os"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"golang.org/x/term"
)

// fileRow implements vtui.TableRow for testing
type fileRow struct {
	name string
	size string
	date string
}

func (f fileRow) GetCellText(col int) string {
	switch col {
	case 0: return f.name
	case 1: return f.size
	case 2: return f.date
	}
	return ""
}

func main() {
	// Enable advanced terminal mode
	restore, err := vtinput.Enable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error enabling raw mode: %v\n", err)
		return
	}
	defer restore()

	// Hide cursor during execution and restore it on exit
	fmt.Print("\x1b[?25l")
	defer fmt.Print("\x1b[?25h")

	// Get initial terminal size and create the screen buffer
	width, height, _ := term.GetSize(int(os.Stdin.Fd()))
	scr := vtui.NewScreenBuf()
	scr.AllocBuf(width, height)

	// --- Initialize FrameManager ---
	vtui.FrameManager.Init(scr)

	// Create and push the root Desktop frame
	desktop := vtui.NewDesktop()
	vtui.FrameManager.Push(desktop)

	// --- Create a Dialog to show ---
	dlgWidth, dlgHeight := 60, 22
	x1 := (width - dlgWidth) / 2
	y1 := (height - dlgHeight) / 2
	dlg := vtui.NewDialog(x1, y1, x1+dlgWidth-1, y1+dlgHeight-1, " UI Components Test ")
	dlg.SetHelp("MainDialogTopic")

	// 1. Text and Edit
	label := vtui.NewText(x1+2, y1+1, "Input field:", vtui.SetRGBFore(0, 0xFFFFFF))
	edit := vtui.NewEdit(x1+15, y1+1, 40, "vtui")

	// 2. Table with mock files (высота 6, чтобы не залезать на нижние контролы)
	tableCols := []vtui.TableColumn{
		{Title: "Name", Width: 25},
		{Title: "Size", Width: 10, Alignment: vtui.AlignRight},
		{Title: "Date", Width: 12},
	}
	table := vtui.NewTable(x1+2, y1+3, 56, 6, tableCols)
	table.SetRows([]vtui.TableRow{
		fileRow{"kernel.go", "4 KB", "2024-03-10"},
		fileRow{"🚀 rocket.exe", "12 KB", "2024-03-11"},
		fileRow{"日本語ファイル.txt", "2 KB", "2024-03-12"},
		fileRow{"main.go", "2 KB", "2024-03-12"},
		fileRow{"README.md", "1 KB", "2024-03-01"},
		fileRow{"LICENSE", "1 KB", "2024-01-01"},
	})

	// 3. Радиокнопки и Чекбоксы (на новых строках Y=10, 11)
	dlg.AddItem(vtui.NewText(x1+2, y1+10, "Options:", vtui.Palette[vtui.ColDialogText]))
	rb1 := vtui.NewRadioButton(x1+15, y1+10, "Option A")
	rb1.Selected = true
	dlg.AddItem(rb1)
	dlg.AddItem(vtui.NewRadioButton(x1+30, y1+10, "Option B"))

	dlg.AddItem(vtui.NewText(x1+2, y1+11, "Checks:", vtui.Palette[vtui.ColDialogText]))
	dlg.AddItem(vtui.NewCheckbox(x1+15, y1+11, "Normal", false))
	dlg.AddItem(vtui.NewCheckbox(x1+30, y1+11, "3-State", true))
	// Add ComboBox to test-app
	dlg.AddItem(vtui.NewText(x1+2, y1+12, "Combo:", vtui.Palette[vtui.ColDialogText]))
	comboItems := []string{"Red", "Green", "Blue", "Alpha"}
	dlg.AddItem(vtui.NewComboBox(x1+15, y1+12, 20, comboItems))

	// 4. VMenu
	menu := vtui.NewVMenu(" Operations ")
	menu.SetHelp("MenuOperationsTopic")
	menu.SetPosition(x1+2, y1+13, x1+30, y1+18)
	menu.AddItem("Copy File")
	menu.AddItem("Move File")
	menu.AddSeparator()
	menu.AddItem("Delete File")

	// 5. Кнопки
	btnOk := vtui.NewButton(x1+10, y1+20, "Ok")
	btnCancel := vtui.NewButton(x1+30, y1+20, "Cancel")

	btnCancel.OnClick = func() {
		dlg.SetExitCode(-1)
		desktop.SetExitCode(-1)
	}
	btnOk.OnClick = func() {
		dlg.SetExitCode(0)
		desktop.SetExitCode(0)
	}

	// Добавляем всё в правильном порядке (Z-order)
	dlg.AddItem(label)
	dlg.AddItem(edit)
	dlg.AddItem(table)
	dlg.AddItem(menu)
	dlg.AddItem(btnOk)
	dlg.AddItem(btnCancel)

	// Push the dialog onto the frame stack
	vtui.FrameManager.Push(dlg)

	// Start the main application loop
	vtui.FrameManager.Run()
}