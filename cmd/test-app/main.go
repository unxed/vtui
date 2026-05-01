package main

import (
	"fmt"
	"os"
	"context"
	"path/filepath"
	"time"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"golang.org/x/term"
)

// localVFS is a minimal stub to satisfy vtui dialogs without relying on external VFS.
type localVFS struct{ path string }

func (v *localVFS) GetPath() string { return v.path }
func (v *localVFS) SetPath(p string) error { v.path = p; return nil }
func (v *localVFS) Join(e ...string) string { return filepath.Join(e...) }
func (v *localVFS) Dir(p string) string { return filepath.Dir(p) }
func (v *localVFS) Base(p string) string { return filepath.Base(p) }
func (v *localVFS) ReadDir(ctx context.Context, p string, onChunk func([]vtui.FSItem)) error {
	entries, _ := os.ReadDir(p)
	var items []vtui.FSItem
	for _, e := range entries {
		items = append(items, vtui.FSItem{Name: e.Name(), IsDir: e.IsDir()})
	}
	if len(items) > 0 && onChunk != nil {
		onChunk(items)
	}
	return nil
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
	case 1001: // Custom application command
		vtui.ShowMessage(" Action ", "Command 1 executed via HandleCommand!", []string{"&Ok"})
		return true
	case 1002: // Showcase dialog
		showShowcaseDialog()
		return true
	}
	// Fallback to default window behavior (e.g. CmClose, CmZoom)
	return d.Window.HandleCommand(cmd, args)
}
func (d *DemoWindow) ProcessKey(e *vtinput.InputEvent) bool {
	// Preserve Ctrl+Q as an exit shortcut for the demo app only
	if e.VirtualKeyCode == vtinput.VK_Q && (e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed)) != 0 {
		return d.HandleCommand(vtui.CmQuit, nil)
	}
	return d.Window.ProcessKey(e)
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

func showShowcaseDialog() {
	dlg := vtui.NewCenteredDialog(56, 18, " UI Showcase ")
	dlg.ShowClose = true

	x, y := dlg.X1, dlg.Y1

	// Left side: TreeView demonstration
	dlg.AddItem(vtui.NewGroupBox(x+2, y+1, x+25, y+14, "TreeView"))
	
	root := &vtui.TreeNode{Text: "System", Expanded: true}
	etc := &vtui.TreeNode{Text: "etc", Expanded: false}
	etc.AddChild(&vtui.TreeNode{Text: "passwd"})
	etc.AddChild(&vtui.TreeNode{Text: "hosts"})
	
	usr := &vtui.TreeNode{Text: "usr", Expanded: true}
	bin := &vtui.TreeNode{Text: "bin", Expanded: true}
	bin.AddChild(&vtui.TreeNode{Text: "bash"})
	bin.AddChild(&vtui.TreeNode{Text: "cat"})
	usr.AddChild(bin)
	usr.AddChild(&vtui.TreeNode{Text: "lib", Expanded: false})
	
	root.AddChild(etc)
	root.AddChild(usr)

	tree := vtui.NewTreeView(x+3, y+2, 21, 11, root)
	dlg.AddItem(tree)

	// Right side: Disabled states demonstration
	dlg.AddItem(vtui.NewGroupBox(x+27, y+1, x+53, y+14, "Disabled State"))

	edit := vtui.NewEdit(x+29, y+4, 20, "Locked text")
	edit.SetDisabled(true)
	dlg.AddItem(vtui.NewLabel(x+29, y+3, "Disabled Edit:", edit))
	dlg.AddItem(edit)

	rg := vtui.NewRadioGroup(x+29, y+7, 1, []string{"Option 1", "Option 2"})
	rg.SetDisabled(true)
	dlg.AddItem(rg)

	btnDisabled := vtui.NewButton(x+29, y+11, "Disabled Btn")
	btnDisabled.SetDisabled(true)
	dlg.AddItem(btnDisabled)

	// Bottom: Live clock using DynamicText
	clock := vtui.NewDynamicText(x+21, y+16, 20, vtui.Palette[vtui.ColDialogText], func() string {
		return "Time: " + time.Now().Format("15:04:05")
	})
	dlg.AddItem(clock)

	// Bottom: Close button
	btnClose := vtui.NewButton(x+23, y+15, "&Close")
	btnClose.OnClick = func() { dlg.Close() }
	dlg.AddItem(btnClose)

	vtui.FrameManager.Push(dlg)
}

func main() {
	guiMode := false
	for _, arg := range os.Args {
		if arg == "--gui" {
			guiMode = true
			break
		}
	}

	setup := func() {
		width, height := 80, 25
		if vtui.FrameManager != nil {
			width = vtui.FrameManager.GetScreenSize()
			height = vtui.FrameManager.GetScreenHeight()
		}

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
			{Label: "&Options", SubItems: []vtui.MenuItem{
				{Text: "&Colors"},
				{Separator: true},
				{Text: "UI S&howcase", Command: 1002},
			}},
			{Label: "&Right", SubItems: []vtui.MenuItem{{Text: "Command &2"}}},
		}
		topMenu.SetPosition(0, 0, width-1, 0)

		// --- Status Line ---
		kb := vtui.NewKeyBar()
		kb.SetPosition(0, height-1, width-1, height-1)
		kb.SetVisible(true)
		vtui.FrameManager.KeyBar = kb

		// --- Comprehensive Window ---
		baseWin := vtui.NewWindow(0, 0, 75, 27, " vtui demo ")
		baseWin.ShowClose = true
		baseWin.Center(width, height)

		dlg := &DemoWindow{Window: baseWin}
		x1, y1 := dlg.X1, dlg.Y1
		topMenu.SetOwner(dlg)

		// Elements
		dlg.AddItem(vtui.NewGroupBox(x1+2, y1+1, x1+35, y1+5, "Execution Mode"))
		rg := vtui.NewRadioGroup(x1+4, y1+2, 1, []string{"&Fast and Dangerous", "Slow and &Stable"})
		dlg.AddItem(vtui.NewLabel(x1+3, y1+1, "&Mode:", rg)); dlg.AddItem(rg)

		combo := vtui.NewComboBox(x1+14, y1+7, 16, []string{"UTF-8", "CP866", "Win-1251"})
		dlg.AddItem(vtui.NewLabel(x1+2, y1+7, "&Encoding:", combo)); dlg.AddItem(combo)

		cmdEdit := vtui.NewEdit(x1+14, y1+9, 16, "ls -la")
		cmdEdit.History = []string{"git status", "go build", "rm -rf /", "ls -la"}
		cmdEdit.ShowHistoryButton = true
		cmdEdit.OnAction = func() {
			text := cmdEdit.GetText()
			if text != "" { cmdEdit.AddHistory(text) }
			vtui.ShowMessage(" Execute ", "Command added to history:\n"+text, []string{"&Ok"})
		}
		dlg.AddItem(vtui.NewLabel(x1+2, y1+9, "&Command:", cmdEdit)); dlg.AddItem(cmdEdit)

		cb3 := vtui.NewCheckbox(x1+2, y1+11, "3-s&tate Checkbox", true); cb3.State = 2; dlg.AddItem(cb3)

		dlg.AddItem(vtui.NewVText(x1+37, y1+2, "│CORE│", vtui.Palette[vtui.ColDialogText]))
		dlg.AddItem(vtui.NewGroupBox(x1+40, y1+1, x1+73, y1+5, "Features"))
		cg := vtui.NewCheckGroup(x1+42, y1+2, 2, []string{"Enable &AI", "A&uto-upd", "&Logging", "&Debug"})
		cg.States[1] = true; dlg.AddItem(cg)

		opMenu := vtui.NewVMenu(" Operations "); opMenu.SetPosition(x1+40, y1+7, x1+64, y1+12)
		opMenu.AddItem(vtui.MenuItem{Text: "&Copy File", Command: 1001})
		opMenu.AddItem(vtui.MenuItem{Text: "&Move File"}); opMenu.AddSeparator()
		opMenu.AddItem(vtui.MenuItem{Text: "&Delete"}); opMenu.AddItem(vtui.MenuItem{Text: "&Attributes"})
		dlg.AddItem(opMenu)

		dlg.AddItem(vtui.NewSeparator(x1, y1+14, 76, true, true))

		table := vtui.NewTable(x1+2, y1+16, 72, 7, []vtui.TableColumn{{Title: "Filename", Width: 48}, {Title: "Size", Width: 12, Alignment: vtui.AlignRight}})
		table.SetRows([]vtui.TableRow{
			fileRow{"README.md", "2 KB"},
			fileRow{"LICENSE", "1 KB"},
			fileRow{"rocket_launcher.sh", "128 KB"},
			fileRow{"data.json", "10 MB"},
		})
		table.ShowScrollBar = true; table.SetGrowMode(vtui.GrowHiX | vtui.GrowHiY); dlg.AddItem(table)

		// Bottom Buttons (Y coordinate is now inside the 27-row window)
		btnY := y1 + 25

		btnOk := vtui.NewButton(x1+16, btnY, "&Ok")
		btnOk.OnClick = func() { dlg.SetExitCode(0); desktop.SetExitCode(0) }
		btnOk.SetGrowMode(vtui.GrowLoY | vtui.GrowHiY)
		dlg.AddItem(btnOk)

		btnMsg := vtui.NewButton(x1+26, btnY, "Show &Msg")
		btnMsg.OnClick = func() { vtui.ShowMessage(" MessageBox ", "Resizing works!", []string{"&Got it"}) }
		btnMsg.SetGrowMode(vtui.GrowLoY | vtui.GrowHiY)
		dlg.AddItem(btnMsg)

		btnDir := vtui.NewButton(x1+38, btnY, "&Dir")
		btnDir.OnClick = func() { vtui.SelectDirDialog(" Choose Directory ", ".", &localVFS{path: "."}) }
		btnDir.SetGrowMode(vtui.GrowLoY | vtui.GrowHiY)
		dlg.AddItem(btnDir)

		btnFile := vtui.NewButton(x1+46, btnY, "&File")
		btnFile.OnClick = func() { vtui.SelectFileDialog(" Open File ", ".", &localVFS{path: "."}, nil) }
		btnFile.SetGrowMode(vtui.GrowLoY | vtui.GrowHiY)
		dlg.AddItem(btnFile)

		btnInp := vtui.NewButton(x1+54, btnY, "&Inp")
		btnInp.OnClick = func() {
			vtui.InputBox(" Question ", "What is your name?", "Explorer", func(s string) {
				vtui.ShowMessage(" Reply ", "Hello, "+s+"!", []string{"&Hi"})
			})
		}
		btnInp.SetGrowMode(vtui.GrowLoY | vtui.GrowHiY)
		dlg.AddItem(btnInp)

		vtui.FrameManager.MenuBar = topMenu

		go func() {
			for {
				time.Sleep(1 * time.Second)
				if vtui.FrameManager == nil || vtui.FrameManager.IsShutdown() { break }
				vtui.FrameManager.Redraw()
			}
		}()

		// IMPORTANT: Actually put the dialog into the FrameManager stack
		dlg.Center(width, height)
		vtui.FrameManager.Push(dlg)
	}

	if guiMode {
		vtui.RunInX11Window(80, 30, setup)
	} else {
		restore, err := vtui.PrepareTerminal()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return
		}
		defer restore()

		width, height, _ := term.GetSize(int(os.Stdin.Fd()))
		scr := vtui.NewScreenBuf()
		scr.AllocBuf(width, height)
		vtui.FrameManager.Init(scr)

		setup()

		reader := vtinput.NewReader(os.Stdin)
		vtui.FrameManager.Run(reader)
	}
}
