package vtui

import (
	"context"
)

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
	ReadDir(ctx context.Context, path string, onChunk func([]VFSItem)) error
	Join(elem ...string) string
	Dir(path string) string
	Base(path string) string
}

// SelectDirDialog creates a standard directory selection dialog.
func SelectDirDialog(title string, initialPath string, vfs VFSMinimal) *Window {
	width := 50
	height := 18
	dlg := NewCenteredDialog(width, height, title)
	dlg.ShowClose = true

	pathEdit := NewEdit(0, 0, 10, initialPath)
	pathEdit.SetDisabled(true)
	lb := NewListBox(0, 0, 10, 8, []string{".."})

	updateList := func(p string, targetToSelect string) {
		currentItems := []string{".."}
		vfs.ReadDir(context.Background(), p, func(chunk []VFSItem) {
			for _, e := range chunk {
				if e.IsDir && e.Name != ".." { currentItems = append(currentItems, e.Name) }
			}
			FrameManager.PostTask(func() {
				if dlg.IsDone() { return }
				lb.Items = currentItems
				lb.UpdateRows()
				if targetToSelect != "" {
					lb.SelectName(targetToSelect)
				} else {
					lb.SetSelectPos(0) // Default to ".."
				}
				FrameManager.Redraw()
			})
		})
	}

	lb.OnSelect = func(idx int) {
		if idx < 0 || idx >= len(lb.Items) { return }
		if lb.Items[idx] == ".." { pathEdit.SetText(vfs.Dir(vfs.GetPath())) } else { pathEdit.SetText(vfs.Join(vfs.GetPath(), lb.Items[idx])) }
	}

	lb.OnAction = func(idx int) {
		if idx < 0 || idx >= len(lb.Items) { return }
		selected := lb.Items[idx]
		oldPath := vfs.GetPath()
		if err := vfs.SetPath(vfs.Join(oldPath, selected)); err == nil {
			target := ""
			if selected == ".." { target = vfs.Base(oldPath) }
			updateList(vfs.GetPath(), target)
			pathEdit.SetText(vfs.GetPath())
		}
	}

	btnOk := NewButton(0, 0, "&Ok")
	btnCancel := NewButton(0, 0, "&Cancel")
	btnOk.OnClick = func() { dlg.SetExitCode(1) }
	btnCancel.OnClick = func() { dlg.SetExitCode(-1) }

	dlg.AddItem(pathEdit); dlg.AddItem(lb); dlg.AddItem(btnOk); dlg.AddItem(btnCancel)

	vbox := NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	vbox.Add(pathEdit, Margins{Bottom: 1}, AlignFill)
	vbox.Add(lb, Margins{Bottom: 1}, AlignFill)
	hbox := NewHBoxLayout(0, 0, width-4, 1)
	hbox.HorizontalAlign = AlignCenter
	hbox.Spacing = 2
	hbox.Add(btnOk, Margins{}, AlignTop)
	hbox.Add(btnCancel, Margins{}, AlignTop)
	vbox.Add(hbox, Margins{}, AlignFill)
	vbox.Apply()

	updateList(vfs.GetPath(), "")
	FrameManager.Push(dlg)
	return dlg
}

// SelectFileDialog creates a standard file selection dialog.
func SelectFileDialog(title string, initialPath string, vfs VFSMinimal) *Window {
	width := 55
	height := 20
	dlg := NewCenteredDialog(width, height, title)
	dlg.ShowClose = true

	lblPath := NewLabel(0, 0, "Path:", nil)
	pathEdit := NewEdit(0, 0, 10, initialPath)
	pathEdit.SetDisabled(true)

	lblFile := NewLabel(0, 0, "&File:", nil)
	fileEdit := NewEdit(0, 0, 10, "")
	lblFile.FocusLink = fileEdit

	lb := NewListBox(0, 0, 10, 6, []string{".."})
	isDirMap := make(map[string]bool)

	updateList := func(p string, targetToSelect string) {
		var allEntries []VFSItem
		vfs.ReadDir(context.Background(), p, func(chunk []VFSItem) {
			allEntries = append(allEntries, chunk...)
			FrameManager.PostTask(func() {
				if dlg.IsDone() { return }
				items := []string{".."}
				isDirMap = make(map[string]bool); isDirMap[".."] = true
				for _, e := range allEntries { if e.IsDir { items = append(items, e.Name); isDirMap[e.Name] = true } }
				for _, e := range allEntries { if !e.IsDir { items = append(items, e.Name); isDirMap[e.Name] = false } }
				lb.Items = items
				lb.UpdateRows()
				if targetToSelect != "" { lb.SelectName(targetToSelect) } else { lb.SetSelectPos(0) }
				FrameManager.Redraw()
			})
		})
	}

	lb.OnSelect = func(idx int) {
		if idx >= 0 && idx < len(lb.Items) && !isDirMap[lb.Items[idx]] { fileEdit.SetText(lb.Items[idx]) }
	}
	lb.OnAction = func(idx int) {
		if idx < 0 || idx >= len(lb.Items) { return }
		selected := lb.Items[idx]
		if isDirMap[selected] {
			oldPath := vfs.GetPath()
			if err := vfs.SetPath(vfs.Join(oldPath, selected)); err == nil {
				target := ""
				if selected == ".." { target = vfs.Base(oldPath) }
				updateList(vfs.GetPath(), target)
				pathEdit.SetText(vfs.GetPath())
			}
		} else { dlg.SetExitCode(1) }
	}

	btnOk := NewButton(0, 0, "&Ok"); btnCancel := NewButton(0, 0, "&Cancel")
	btnOk.OnClick = func() { dlg.SetExitCode(1) }
	btnCancel.OnClick = func() { dlg.SetExitCode(-1) }

	dlg.AddItem(lblPath); dlg.AddItem(pathEdit); dlg.AddItem(lb)
	dlg.AddItem(lblFile); dlg.AddItem(fileEdit); dlg.AddItem(btnOk); dlg.AddItem(btnCancel)

	vbox := NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	rowPath := NewHBoxLayout(0, 0, width-4, 1)
	rowPath.Add(lblPath, Margins{Right: 1}, AlignTop); rowPath.Add(pathEdit, Margins{}, AlignFill)
	vbox.Add(rowPath, Margins{}, AlignFill)
	vbox.Add(lb, Margins{Top: 1, Bottom: 1}, AlignFill)
	rowFile := NewHBoxLayout(0, 0, width-4, 1)
	rowFile.Add(lblFile, Margins{Right: 1}, AlignTop); rowFile.Add(fileEdit, Margins{}, AlignFill)
	vbox.Add(rowFile, Margins{}, AlignFill)
	rowBtns := NewHBoxLayout(0, 0, width-4, 1)
	rowBtns.HorizontalAlign = AlignCenter; rowBtns.Spacing = 2
	rowBtns.Add(btnOk, Margins{}, AlignTop); rowBtns.Add(btnCancel, Margins{}, AlignTop)
	vbox.Add(rowBtns, Margins{Top: 1}, AlignFill)
	vbox.Apply()

	updateList(vfs.GetPath(), "")
	FrameManager.Push(dlg)
	return dlg
}

// InputBox creates a simple one-line text input dialog.
func InputBox(title, prompt, defaultText string, onOk func(string)) *Window {
	width := 40
	height := 8
	dlg := NewCenteredDialog(width, height, title)
	dlg.ShowClose = true

	edit := NewEdit(0, 0, 10, defaultText)
	lbl := NewLabel(0, 0, prompt, edit)
	btnOk := NewButton(0, 0, "&Ok")
	btnCancel := NewButton(0, 0, "&Cancel")

	btnOk.OnClick = func() {
		if onOk != nil { onOk(edit.GetText()) }
		dlg.SetExitCode(1)
	}
	btnCancel.OnClick = func() { dlg.SetExitCode(-1) }

	dlg.AddItem(lbl); dlg.AddItem(edit)
	dlg.AddItem(btnOk); dlg.AddItem(btnCancel)

	// Layout construction
	vbox := NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	vbox.Add(lbl, Margins{}, AlignLeft)
	vbox.Add(edit, Margins{Top: 1}, AlignFill)

	rowBtns := NewHBoxLayout(0, 0, width-4, 1)
	rowBtns.HorizontalAlign = AlignCenter
	rowBtns.Spacing = 2
	rowBtns.Add(btnOk, Margins{}, AlignTop)
	rowBtns.Add(btnCancel, Margins{}, AlignTop)

	vbox.Add(rowBtns, Margins{Top: 1}, AlignFill)
	vbox.Apply()

	FrameManager.Push(dlg)
	return dlg
}