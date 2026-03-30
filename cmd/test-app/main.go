package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"golang.org/x/term"
)

// localVFS is a minimal stub to satisfy vtui dialogs without relying on f4's VFS.
type localVFS struct{ path string }

func (v *localVFS) GetPath() string { return v.path }
func (v *localVFS) SetPath(p string) error { v.path = p; return nil }
func (v *localVFS) Join(e ...string) string { return filepath.Join(e...) }
func (v *localVFS) Dir(p string) string { return filepath.Dir(p) }
func (v *localVFS) Base(p string) string { return filepath.Base(p) }
func (v *localVFS) ReadDir(p string) ([]vtui.VFSItem, error) {
	entries, _ := os.ReadDir(p)
	var items []vtui.VFSItem
	for _, e := range entries {
		items = append(items, vtui.VFSItem{Name: e.Name(), IsDir: e.IsDir()})
	}
	return items, nil
}

// DemoWindow wraps vtui.Window to showcase Turbo Vision-style command routing.
type DemoWindow struct {
	*vtui.Window
}

func (d *DemoWindow) HandleCommand(cmd int, args any) bool {
	switch cmd {
	case vtui.CmQuit:
		vtui.FrameManager.Shutdown()
		return true
	case vtui.CmCopy:
		vtui.ShowMessage(" Action ", "Copy command intercepted via HandleCommand!", []string{"&Ok"})
		return true
	case 1001: // Custom application command
		vtui.ShowMessage(" Action ", "Command 1 executed via HandleCommand!", []string{"&Ok"})
		return true
	}
	// Fallback to default window behavior (e.g. CmClose, CmZoom)
	return d.Window.HandleCommand(cmd, args)
}

func (d *DemoWindow) GetKeyLabels() *vtui.KeySet {
	return &vtui.KeySet{
		Normal: vtui.KeyBarLabels{
			"Help", "Exit", "View", "Edit", "Copy", "Move", "MkDir", "Delete", "Menu", "Quit", "Plugin", "Screen",
		},
	}
}

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
		{Label: "&Left", SubItems: []vtui.MenuItem{
			{Text: "Command &1", Command: 1001},
			{Separator: true},
			{Text: "E&xit", Command: vtui.CmQuit},
		}},
		{Label: "&Files", SubItems: []vtui.MenuItem{{Text: "&Open"}, {Text: "&Save"}}},
		{Label: "&Commands", SubItems: []vtui.MenuItem{{Text: "&Search"}}},
		{Label: "&Options", SubItems: []vtui.MenuItem{{Text: "&Colors"}}},
		{Label: "&Right", SubItems: []vtui.MenuItem{{Text: "Command &2"}}},
	}
	topMenu.SetPosition(0, 0, width-1, 0)
	// Note: We removed topMenu.OnCommand. Commands are now automatically routed down the frame stack!

	// --- Status Line ---
	kb := vtui.NewKeyBar()
	kb.SetPosition(0, height-1, width-1, height-1)
	kb.SetVisible(true)
	vtui.FrameManager.KeyBar = kb

	// --- Comprehensive Window ---
	baseWin := vtui.NewWindow(0, 0, 75, 28, " vtui demo ")
	baseWin.ShowClose = true
	baseWin.Center(width, height)

	// Wrap the window to provide our custom HandleCommand logic
	dlg := &DemoWindow{Window: baseWin}
	x1, y1 := dlg.X1, dlg.Y1

	// LEFT: Input & Options
	dlg.AddItem(vtui.NewGroupBox(x1+2, y1+1, x1+35, y1+5, "Execution Mode"))

	modes := []string{"&Fast and Dangerous", "Slow and &Stable"}
	rg := vtui.NewRadioGroup(x1+4, y1+2, 1, modes)
	dlg.AddItem(vtui.NewLabel(x1+3, y1+1, "&Mode:", rg)) // Link hotkey 'M' to the group
	dlg.AddItem(rg)

	combo := vtui.NewComboBox(x1+14, y1+7, 16, []string{"UTF-8", "CP866", "Win-1251"})
	dlg.AddItem(vtui.NewLabel(x1+2, y1+7, "&Encoding:", combo))
	dlg.AddItem(combo)

	cmdEdit := vtui.NewEdit(x1+14, y1+9, 16, "ls -la")
	cmdEdit.History = []string{"git status", "go build", "rm -rf /", "ls -la"}
	cmdEdit.ShowHistoryButton = true
	cmdEdit.SetHelp("edit")
	cmdEdit.OnAction = func() {
		text := cmdEdit.GetText()
		if text != "" {
			cmdEdit.AddHistory(text)
		}
		vtui.ShowMessage(" Execute ", "Command added to history:\n"+text, []string{"&Ok"})
	}
	dlg.AddItem(vtui.NewLabel(x1+2, y1+9, "&Command:", cmdEdit))
	dlg.AddItem(cmdEdit)

	cb3 := vtui.NewCheckbox(x1+2, y1+11, "3-s&tate Checkbox", true)
	cb3.State = 2 // Demo 3-state
	dlg.AddItem(cb3)

	// RIGHT: Operations & List
	dlg.AddItem(vtui.NewVText(x1+37, y1+2, "│CORE│", vtui.Palette[vtui.ColDialogText]))

	dlg.AddItem(vtui.NewGroupBox(x1+40, y1+1, x1+73, y1+5, "Features"))
	cg := vtui.NewCheckGroup(x1+42, y1+2, 2, []string{"Enable &AI", "A&uto-upd", "&Logging", "&Debug"})
	cg.States[1] = true // auto-update enabled by default
	dlg.AddItem(cg)

	opMenu := vtui.NewVMenu(" Operations ")
	opMenu.SetPosition(x1+40, y1+7, x1+64, y1+12) // Height of 5 lines
	opMenu.AddItem(vtui.MenuItem{Text: "&Copy File", Command: vtui.CmCopy})
	opMenu.AddItem(vtui.MenuItem{Text: "&Move File"})
	opMenu.AddSeparator()
	opMenu.AddItem(vtui.MenuItem{Text: "&Delete"})
	opMenu.AddItem(vtui.MenuItem{Text: "&Attributes"})
	dlg.AddItem(opMenu)

	// FULL WIDTH SEPARATOR
	dlg.AddItem(vtui.NewSeparator(x1, y1+14, 76, true, true))

	// CENTER: Table
	tableCols := []vtui.TableColumn{
		{Title: "Filename", Width: 48},
		{Title: "Size", Width: 12, Alignment: vtui.AlignRight},
	}
	table := vtui.NewTable(x1+2, y1+16, 72, 7, tableCols)
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
	btnOk := vtui.NewButton(x1+16, y1+25, "&Ok")
	btnOk.OnClick = func() { dlg.SetExitCode(0); desktop.SetExitCode(0) }
	btnOk.SetGrowMode(vtui.GrowLoY | vtui.GrowHiY)

	btnMsg := vtui.NewButton(x1+28, y1+25, "Show &Msg")
	btnMsg.OnClick = func() {
		vtui.ShowMessage(" MessageBox ", "Resizing is enabled!\nGrab the bottom-right corner.", []string{"&Got it"})
	}
	btnMsg.SetGrowMode(vtui.GrowLoY | vtui.GrowHiY)

	btnDir := vtui.NewButton(x1+40, y1+25, "&Dir")
	btnDir.OnClick = func() {
		vtui.SelectDirDialog(" Choose Directory ", ".", &localVFS{path: "."})
	}
	btnDir.SetGrowMode(vtui.GrowLoY | vtui.GrowHiY)

	btnFile := vtui.NewButton(x1+48, y1+25, "&File")
	btnFile.OnClick = func() {
		vtui.SelectFileDialog(" Open File ", ".", &localVFS{path: "."})
	}
	btnFile.SetGrowMode(vtui.GrowLoY | vtui.GrowHiY)

	btnInp := vtui.NewButton(x1+56, y1+25, "&Inp")
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
	// vtui.FrameManager.StatusLine = sl // StatusLine removed in favor of KeyBar

	vtui.FrameManager.Push(dlg)

	reader := vtinput.NewReader(os.Stdin)
	vtui.FrameManager.Run(reader)
}
