package vtui

// VFSItem represents a generic file or directory entry for UI dialogs.
type VFSItem struct {
	Name  string
	IsDir bool
}

// VFSMinimal is a subset of file operations required by UI dialogs.
// This keeps vtui independent of the actual file manager implementation.
type VFSMinimal interface {
	GetPath() string
	SetPath(path string) error
	ReadDir(path string) ([]VFSItem, error)
	Join(elem ...string) string
	Dir(path string) string
	Base(path string) string
}

// SelectDirDialog creates a standard directory selection dialog.
func SelectDirDialog(title string, initialPath string, vfs VFSMinimal) *Dialog {
	width := 50
	height := 18
	scrW := FrameManager.GetScreenSize()
	x1 := (scrW - width) / 2
	y1 := 4

	dlg := NewDialog(x1, y1, x1+width-1, y1+height-1, title)
	dlg.ShowClose = true

	pathEdit := NewEdit(x1+2, y1+2, width-4, initialPath)
	dlg.AddItem(pathEdit)

	// List of directories
	var items []string
	updateList := func(p string) {
		entries, _ := vfs.ReadDir(p)
		items = []string{".."}
		for _, e := range entries {
			if e.IsDir {
				items = append(items, e.Name)
			}
		}
	}
	updateList(vfs.GetPath())

	lb := NewListBox(x1+2, y1+4, width-4, height-8, items)
	dlg.AddItem(lb)

	lb.ChangeCommand = dlg.AddCallback(func(args any) {
		idx := args.(int)
		if idx < 0 || idx >= len(items) { return }
		selected := items[idx]
		var previewPath string
		if selected == ".." {
			previewPath = vfs.Dir(vfs.GetPath())
		} else {
			previewPath = vfs.Join(vfs.GetPath(), selected)
		}
		pathEdit.SetText(previewPath)
	})

	lb.ActionCommand = dlg.AddCallback(func(args any) {
		idx := args.(int)
		if idx < 0 || idx >= len(items) { return }
		selected := items[idx]
		oldPath := vfs.GetPath()
		var newPath string
		isGoingUp := selected == ".."

		if isGoingUp {
			newPath = vfs.Dir(oldPath)
		} else {
			newPath = vfs.Join(oldPath, selected)
		}

		if err := vfs.SetPath(newPath); err == nil {
			updateList(vfs.GetPath())
			lb.Items = items
			lb.SelectPos = 0

			if isGoingUp {
				// Find where we came from
				prevDirName := vfs.Base(oldPath)
				for i, name := range items {
					if name == prevDirName {
						lb.SelectPos = i
						break
					}
				}
			}

			lb.TopPos = 0
			lb.EnsureVisible()
			pathEdit.SetText(vfs.GetPath())
		}
	})

	btnOk := NewButton(x1+10, y1+height-2, "&Ok")
	btnOk.Command = dlg.AddCommand(func() { dlg.SetExitCode(1) })
	dlg.AddItem(btnOk)

	btnCancel := NewButton(x1+width-20, y1+height-2, "&Cancel")
	btnCancel.Command = dlg.AddCommand(func() { dlg.SetExitCode(-1) })
	dlg.AddItem(btnCancel)

	FrameManager.Push(dlg)
	return dlg
}

// SelectFileDialog creates a standard file selection dialog.
func SelectFileDialog(title string, initialPath string, vfs VFSMinimal) *Dialog {
	width := 55
	height := 20
	scrW := FrameManager.GetScreenSize()
	x1 := (scrW - width) / 2
	y1 := 3

	dlg := NewDialog(x1, y1, x1+width-1, y1+height-1, title)
	dlg.ShowClose = true

	// 1. Current Path Preview
	dlg.AddItem(NewLabel(x1+2, y1+2, "Path:", nil))
	pathEdit := NewEdit(x1+8, y1+2, width-11, initialPath)
	dlg.AddItem(pathEdit)

	var items []string
	var isDirMap map[string]bool

	updateList := func(p string) {
		entries, _ := vfs.ReadDir(p)
		items = []string{".."}
		isDirMap = make(map[string]bool)
		isDirMap[".."] = true

		// Folders first
		for _, e := range entries {
			if e.IsDir {
				items = append(items, e.Name)
				isDirMap[e.Name] = true
			}
		}
		// Then files
		for _, e := range entries {
			if !e.IsDir {
				items = append(items, e.Name)
				isDirMap[e.Name] = false
			}
		}
	}
	updateList(vfs.GetPath())

	// 2. File List
	lb := NewListBox(x1+2, y1+4, width-4, height-10, items)
	dlg.AddItem(lb)

	// 3. Filename input
	dlg.AddItem(NewLabel(x1+2, y1+height-4, "&File:", nil))
	fileEdit := NewEdit(x1+8, y1+height-4, width-11, "")
	dlg.AddItem(fileEdit)

	lb.ChangeCommand = dlg.AddCallback(func(args any) {
		idx := args.(int)
		if idx < 0 || idx >= len(items) { return }
		selected := items[idx]
		if !isDirMap[selected] {
			fileEdit.SetText(selected)
		}
	})

	lb.ActionCommand = dlg.AddCallback(func(args any) {
		idx := args.(int)
		if idx < 0 || idx >= len(items) { return }
		selected := items[idx]
		oldPath := vfs.GetPath()

		if isDirMap[selected] {
			var newPath string
			if selected == ".." {
				newPath = vfs.Dir(oldPath)
			} else {
				newPath = vfs.Join(oldPath, selected)
			}
			if err := vfs.SetPath(newPath); err == nil {
				updateList(vfs.GetPath())
				lb.Items = items
				lb.SelectPos = 0
				lb.TopPos = 0
				pathEdit.SetText(vfs.GetPath())
				if selected == ".." {
					prevDir := vfs.Base(oldPath)
					for i, name := range items {
						if name == prevDir { lb.SelectPos = i; break }
					}
				}
				lb.EnsureVisible()
			}
		} else {
			// Selecting a file via Enter
			dlg.SetExitCode(1)
		}
	})

	btnOk := NewButton(x1+width/2-12, y1+height-2, "&Ok")
	btnOk.Command = dlg.AddCommand(func() { dlg.SetExitCode(1) })
	dlg.AddItem(btnOk)

	btnCancel := NewButton(x1+width/2+2, y1+height-2, "&Cancel")
	btnCancel.Command = dlg.AddCommand(func() { dlg.SetExitCode(-1) })
	dlg.AddItem(btnCancel)

	FrameManager.Push(dlg)
	return dlg
}

// InputBox creates a simple one-line text input dialog.
func InputBox(title, prompt, defaultText string, onOk func(string)) *Dialog {
	width := 40
	height := 8
	scrW := FrameManager.GetScreenSize()
	x1 := (scrW - width) / 2
	y1 := 8

	dlg := NewDialog(x1, y1, x1+width-1, y1+height-1, title)
	dlg.ShowClose = true

	edit := NewEdit(x1+2, y1+3, width-4, defaultText)
	// Use NewLabel to link the prompt hotkey to the edit field
	dlg.AddItem(NewLabel(x1+2, y1+2, prompt, edit))
	dlg.AddItem(edit)

	btnOk := NewButton(x1+8, y1+5, "&Ok")
	btnOk.Command = dlg.AddCommand(func() {
		if onOk != nil { onOk(edit.GetText()) }
		dlg.SetExitCode(1)
	})
	dlg.AddItem(btnOk)

	btnCancel := NewButton(x1+width-18, y1+5, "&Cancel")
	btnCancel.Command = dlg.AddCommand(func() { dlg.SetExitCode(-1) })
	dlg.AddItem(btnCancel)

	FrameManager.Push(dlg)
	return dlg
}