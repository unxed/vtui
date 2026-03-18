package main

import (
	"fmt"
	"os"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"golang.org/x/term"
)

type fileRow struct {
	name string
	size string
}

func (f fileRow) GetCellText(col int) string {
	if col == 0 { return f.name }
	return f.size
}

func main() {
	restore, err := vtinput.Enable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}
	defer restore()

	width, height, _ := term.GetSize(int(os.Stdin.Fd()))
	scr := vtui.NewScreenBuf()
	scr.AllocBuf(width, height)
	vtui.FrameManager.Init(scr)

	// --- Layers ---
	desktop := vtui.NewDesktop()
	vtui.FrameManager.Push(desktop)

	// --- Menu Bar ---
	topMenu := vtui.NewMenuBar(nil)
	topMenu.Items = []vtui.MenuBarItem{
		{Label: "&Left", SubItems: []vtui.MenuItem{{Text: "Command &1"}, {Separator: true}, {Text: "E&xit"}}},
		{Label: "&Files", SubItems: []vtui.MenuItem{{Text: "&Open"}, {Text: "&Save"}}},
		{Label: "&Commands", SubItems: []vtui.MenuItem{{Text: "&Search"}}},
		{Label: "&Options", SubItems: []vtui.MenuItem{{Text: "&Colors"}}},
		{Label: "&Right", SubItems: []vtui.MenuItem{{Text: "Command &2"}}},
	}
	topMenu.SetPosition(0, 0, width-1, 0)
	topMenu.OnCommand = func(menuIdx, itemIdx int) {
		if menuIdx == 0 && itemIdx == 2 {
			vtui.FrameManager.Shutdown()
		}
	}

	// --- Status Line ---
	sl := vtui.NewStatusLine()
	sl.SetPosition(0, height-1, width-1, height-1)
	sl.Default = []vtui.StatusItem{
		{Key: "F1", Label: "Help"},
		{Key: "F9", Label: "Menu"},
		{Key: "F10", Label: "Quit"},
		{Key: "Drag ↘", Label: "Resize"},
	}
	sl.Items["edit"] = []vtui.StatusItem{
		{Key: "F1", Label: "Help"},
		{Key: "F9", Label: "Menu"},
		{Key: "Alt-Down", Label: "History"},
		{Key: "Ctrl-C", Label: "Copy"},
	}

	// --- Comprehensive Dialog ---
	dlg := vtui.NewDialog(0, 0, 63, 25, " vtui 3.0 Kitchen Sink ")
	dlg.ShowClose = true
	dlg.Center(width, height)
	x1, y1 := dlg.X1, dlg.Y1

	// LEFT: Input & Options
	dlg.AddItem(vtui.NewLabel(x1+2, y1+2, "Select &mode:", nil))
	rb1 := vtui.NewRadioButton(x1+4, y1+3, "&Fast and Dangerous")
	rb1.Selected = true
	dlg.AddItem(rb1)
	dlg.AddItem(vtui.NewRadioButton(x1+4, y1+4, "Slow and &Stable"))

	combo := vtui.NewComboBox(x1+13, y1+6, 16, []string{"UTF-8", "CP866", "Win-1251"})
	dlg.AddItem(vtui.NewLabel(x1+2, y1+6, "&Encoding:", combo))
	dlg.AddItem(combo)

	cmdEdit := vtui.NewEdit(x1+13, y1+8, 16, "ls -la")
	cmdEdit.History = []string{"git status", "go build", "rm -rf /", "ls -la"}
	cmdEdit.ShowHistoryButton = true
	cmdEdit.SetHelp("edit")
	cmdEdit.OnAction = func() {
		text := cmdEdit.GetText()
		cmdEdit.AddHistory(text)
		vtui.ShowMessage(" Execute ", "Command added to history:\n"+text, []string{"&Ok"})
	}
	dlg.AddItem(vtui.NewLabel(x1+2, y1+8, "&Command:", cmdEdit))
	dlg.AddItem(cmdEdit)

	// RIGHT: Operations & List
	dlg.AddItem(vtui.NewVText(x1+30, y1+2, "│CORE│", vtui.Palette[vtui.ColDialogText]))
	dlg.AddItem(vtui.NewLabel(x1+34, y1+2, "S&ettings:", nil))
	dlg.AddItem(vtui.NewCheckbox(x1+36, y1+3, "Enable &AI", false))
	dlg.AddItem(vtui.NewCheckbox(x1+36, y1+4, "A&uto-update", true))

	opMenu := vtui.NewVMenu(" Operations ")
	opMenu.SetPosition(x1+34, y1+6, x1+58, y1+11) // Height of 5 lines
	opMenu.AddItem("&Copy File")
	opMenu.AddItem("&Move File")
	opMenu.AddSeparator()
	opMenu.AddItem("&Delete")
	opMenu.AddItem("&Attributes")
	dlg.AddItem(opMenu)

	recentFiles := []string{"main.go", "edit.go", "dialog.go", "table.go", "pty.go", "vfs.go", "sum.go"}
	lb := vtui.NewListBox(x1+34, y1+13, 24, 3, recentFiles)
	dlg.AddItem(vtui.NewLabel(x1+34, y1+12, "&Recent:", lb))
	dlg.AddItem(lb)

	// CENTER: Table
	tableCols := []vtui.TableColumn{
		{Title: "Filename", Width: 35},
		{Title: "Size", Width: 12, Alignment: vtui.AlignRight},
	}
	table := vtui.NewTable(x1+2, y1+17, 58, 5, tableCols)
	table.SetRows([]vtui.TableRow{
		fileRow{"README.md", "2 KB"},
		fileRow{"LICENSE", "1 KB"},
		fileRow{"rocket_launcher.sh", "128 KB"},
		fileRow{"data.json", "10 MB"},
	})
	table.ShowScrollBar = true
	table.SetGrowMode(vtui.GrowHiX | vtui.GrowHiY)
	dlg.AddItem(table)

	// BOTTOM: Buttons
	btnOk := vtui.NewButton(x1+12, y1+23, "&Ok")
	btnOk.OnClick = func() { dlg.SetExitCode(0); desktop.SetExitCode(0) }
	btnOk.SetGrowMode(vtui.GrowLoY | vtui.GrowHiY)

	btnMsg := vtui.NewButton(x1+24, y1+23, "Show &Msg")
	btnMsg.OnClick = func() {
		vtui.ShowMessage(" MessageBox ", "Resizing is enabled!\nGrab the bottom-right corner.", []string{"&Got it"})
	}
	btnMsg.SetGrowMode(vtui.GrowLoY | vtui.GrowHiY)

	btnDir := vtui.NewButton(x1+36, y1+23, "&Dir")
	btnDir.OnClick = func() {
		vtui.SelectDirDialog(" Choose Directory ", ".", vtui.NewOSVFS("."))
	}
	btnDir.SetGrowMode(vtui.GrowLoY | vtui.GrowHiY)

	btnFile := vtui.NewButton(x1+44, y1+23, "&File")
	btnFile.OnClick = func() {
		vtui.SelectFileDialog(" Open File ", ".", vtui.NewOSVFS("."))
	}
	btnFile.SetGrowMode(vtui.GrowLoY | vtui.GrowHiY)

	btnInp := vtui.NewButton(x1+52, y1+23, "&Inp")
	btnInp.OnClick = func() {
		vtui.InputBox(" Question ", "What is your name?", "Explorer", func(s string) {
			vtui.ShowMessage(" Reply ", "Hello, "+s+"!", []string{"&Hi"})
		})
	}
	btnInp.SetGrowMode(vtui.GrowLoY | vtui.GrowHiY)

	dlg.AddItem(btnOk)
	dlg.AddItem(btnMsg)
	dlg.AddItem(btnDir)
	dlg.AddItem(btnFile)
	dlg.AddItem(btnInp)

	// Assign components to the Framework to enable standard behaviors
	vtui.FrameManager.MenuBar = topMenu
	vtui.FrameManager.StatusLine = sl

	vtui.FrameManager.Push(dlg)


	vtui.FrameManager.Run()
}
