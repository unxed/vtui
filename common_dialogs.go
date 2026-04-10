package vtui

import (
	"context"
	"github.com/mattn/go-runewidth"
)

// FSItem represents a generic file or directory entry for UI dialogs.
type FSItem struct {
	Name  string
	IsDir bool
}

// FSProvider is a subset of file operations required by UI dialogs.
// This keeps vtui independent of the actual file manager implementation.
type FSProvider interface {
	GetPath() string
	SetPath(path string) error
	ReadDir(ctx context.Context, path string, onChunk func([]FSItem)) error
	Join(elem ...string) string
	Dir(path string) string
	Base(path string) string
}

// SelectDirDialog creates a standard directory selection dialog.
func SelectDirDialog(title string, initialPath string, vfs FSProvider) *Window {
	width := 50
	height := 18
	dlg := NewCenteredDialog(width, height, title)
	dlg.ShowClose = true

	pathEdit := NewEdit(0, 0, 10, initialPath)
	pathEdit.SetDisabled(true)
	lb := NewListBox(0, 0, 10, 8, []string{".."})

	updateList := func(p string, targetToSelect string) {
		go func() {
			currentItems := []string{".."}
			vfs.ReadDir(context.Background(), p, func(chunk []FSItem) {
				for _, e := range chunk {
					if e.IsDir && e.Name != ".." { currentItems = append(currentItems, e.Name) }
				}
			})
			FrameManager.PostTask(func() {
				if dlg.IsDone() { return }
				lb.Items = currentItems
				lb.UpdateRows()
				if targetToSelect != "" { lb.SelectName(targetToSelect) } else { lb.SetSelectPos(0) }
				FrameManager.Redraw()
			})
		}()
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

	btnOk := NewButton(0, 0, Msg("vtui.Ok"))
	btnCancel := NewButton(0, 0, Msg("vtui.Cancel"))
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
func ShowMessage(title string, text string, buttons []string) *Window {
	dlg := createMessageDialog(title, text, buttons)
	FrameManager.Push(dlg)
	return dlg
}

// ShowMessageOn creates a message box targeted to a specific screen (via an anchor frame).
func ShowMessageOn(anchor Frame, title string, text string, buttons []string) *Window {
	// 1. Create the dialog but DON'T push it yet via the generic FrameManager.Push
	dlg := createMessageDialog(title, text, buttons)

	// 2. Target the specific screen
	FrameManager.PushToFrameScreen(anchor, dlg)
	return dlg
}

// Internal helper to avoid code duplication
func createMessageDialog(title string, text string, buttons []string) *Window {
	const maxDialogWidth = 72 // Comfortably fits within 80 columns
	const sidePadding = 4

	// 1. Calculate button dimensions
	btnsWidth := 0
	for _, b := range buttons {
		clean, _, _ := ParseAmpersandString(b)
		btnsWidth += runewidth.StringWidth(clean) + 4
	}
	spacing := 2
	totalBtnsWidth := 0
	if len(buttons) > 0 {
		totalBtnsWidth = btnsWidth + (len(buttons)-1)*spacing
	}

	// Determine if buttons fit horizontally
	stackButtons := (totalBtnsWidth + sidePadding) > maxDialogWidth

	// 2. Finalize Dialog width
	lines := WrapText(text, maxDialogWidth-sidePadding)
	textWidth := 0
	for _, l := range lines {
		w := runewidth.StringWidth(l)
		if w > textWidth { textWidth = w }
	}

	dlgWidth := textWidth + sidePadding
	if !stackButtons && totalBtnsWidth+sidePadding > dlgWidth {
		dlgWidth = totalBtnsWidth + sidePadding
	}
	if title != "" {
		tw := runewidth.StringWidth(title) + 6
		if tw > dlgWidth { dlgWidth = tw }
	}
	if dlgWidth > maxDialogWidth { dlgWidth = maxDialogWidth }

	// 3. Finalize Dialog height
	// Borders (2) + Padding (2) + Lines (len)
	dlgHeight := len(lines) + 4
	if len(buttons) > 0 {
		if stackButtons {
			// Each stacked button adds 1 row for itself and 1 row for its top margin
			dlgHeight += (len(buttons) * 2)
		} else {
			// Horizontal layout adds 1 row for the gap and 1 row for the buttons
			dlgHeight += 2
		}
	}

	dlg := NewCenteredDialog(dlgWidth, dlgHeight, title)

	// 4. Use Layout Engine for positioning
	vbox := NewVBoxLayout(dlg.X1+2, dlg.Y1+2, dlgWidth-4, dlgHeight-4)

	for _, l := range lines {
		txt := NewText(0, 0, l, Palette[ColDialogText])
		vbox.Add(txt, Margins{}, AlignCenter)
		dlg.AddItem(txt)
	}

	if len(buttons) > 0 {
		if stackButtons {
			// Add stacked buttons
			for i, b := range buttons {
				btnID := i
				btn := NewButton(0, 0, b)
				btn.OnClick = func() { dlg.SetExitCode(btnID) }
				vbox.Add(btn, Margins{Top: 1}, AlignCenter)
				dlg.AddItem(btn)
			}
		} else {
			// Add horizontal button row
			hbox := NewHBoxLayout(0, 0, dlgWidth-4, 1)
			hbox.HorizontalAlign = AlignCenter
			hbox.Spacing = spacing
			for i, b := range buttons {
				btnID := i
				btn := NewButton(0, 0, b)
				btn.OnClick = func() { dlg.SetExitCode(btnID) }
				hbox.Add(btn, Margins{}, AlignTop)
				dlg.AddItem(btn)
			}
			vbox.Add(hbox, Margins{Top: 1}, AlignFill)
		}
	}

	vbox.Apply()
	return dlg
}

// SelectFileDialog creates a standard file selection dialog.
func SelectFileDialog(title string, initialPath string, vfs FSProvider) *Window {
	width := 55
	height := 20
	dlg := NewCenteredDialog(width, height, title)
	dlg.ShowClose = true

	lblPath := NewLabel(0, 0, Msg("vtui.Path"), nil)
	pathEdit := NewEdit(0, 0, 10, initialPath)
	pathEdit.SetDisabled(true)

	lblFile := NewLabel(0, 0, Msg("vtui.File"), nil)
	fileEdit := NewEdit(0, 0, 10, "")
	lblFile.FocusLink = fileEdit

	lb := NewListBox(0, 0, 10, 6, []string{".."})
	isDirMap := make(map[string]bool)

	updateList := func(p string, targetToSelect string) {
		go func() {
			var allEntries []FSItem
			vfs.ReadDir(context.Background(), p, func(chunk []FSItem) {
				allEntries = append(allEntries, chunk...)
			})
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
		}()
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

	btnOk := NewButton(0, 0, Msg("vtui.Ok")); btnCancel := NewButton(0, 0, Msg("vtui.Cancel"))
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
	height := 9 // Increased height to fit all elements with margins
	dlg := NewCenteredDialog(width, height, title)
	dlg.ShowClose = true

	edit := NewEdit(0, 0, 10, defaultText)
	lbl := NewLabel(0, 0, prompt, edit)
	btnOk := NewButton(0, 0, Msg("vtui.Ok"))
	btnCancel := NewButton(0, 0, Msg("vtui.Cancel"))

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