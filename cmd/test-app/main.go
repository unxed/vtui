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

	// 1. Text and Edit
	label := vtui.NewText(x1+2, y1+1, "Input field:", vtui.SetRGBFore(0, 0xFFFFFF))
	edit := vtui.NewEdit(x1+15, y1+1, 40, "f4 project")

	// 2. Table with mock files
	tableCols := []vtui.TableColumn{
		{Title: "Name", Width: 25},
		{Title: "Size", Width: 10, Alignment: vtui.AlignRight},
		{Title: "Date", Width: 12},
	}
	table := vtui.NewTable(x1+2, y1+3, 56, 8, tableCols)
	table.SetRows([]vtui.TableRow{
		fileRow{"kernel.go", "4 KB", "2024-03-10"},
		fileRow{"ui_table.go", "12 KB", "2024-03-11"},
		fileRow{"main.go", "2 KB", "2024-03-12"},
		fileRow{"README.md", "1 KB", "2024-03-01"},
		fileRow{"go.mod", "128 B", "2024-01-01"},
		fileRow{"LICENSE", "1 KB", "2024-01-01"},
		fileRow{"very_long_filename_test.txt", "0 B", "2024-03-13"},
	})

	// 3. VMenu
	menu := vtui.NewVMenu(" Operations ")
	menu.SetPosition(x1+2, y1+12, x1+30, y1+17)
	menu.AddItem("Copy File")
	menu.AddItem("Move File")
	menu.AddSeparator()
	menu.AddItem("Delete File")
	btnOk := vtui.NewButton(x1+10, y1+19, "Ok")
	btnCancel := vtui.NewButton(x1+30, y1+19, "Cancel")

	// Set button actions to close the dialog
	btnCancel.OnClick = func() {
		dlg.SetExitCode(-1)
		desktop.SetExitCode(-1)
	}
	btnOk.OnClick = func() {
		dlg.SetExitCode(0)
		desktop.SetExitCode(0)
	}

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