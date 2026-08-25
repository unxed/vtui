package vtui

import (
	"context"
	"sort"
	"strings"
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
	fm := FrameManager
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
					if e.IsDir && e.Name != ".." {
						currentItems = append(currentItems, e.Name)
					}
				}
			})
			fm.PostTask(func() {
				if dlg.IsDone() {
					return
				}
				lb.Items = currentItems
				// Sort folders alphabetically (ignoring "..")
				if len(currentItems) > 2 {
					sort.Slice(currentItems[1:], func(i, j int) bool {
						return strings.ToLower(currentItems[i+1]) < strings.ToLower(currentItems[j+1])
					})
				}
				lb.Items = currentItems
				lb.UpdateRows()
				if targetToSelect != "" {
					lb.SelectName(targetToSelect)
				} else {
					lb.SetSelectPos(0)
				}
				fm.Redraw()
			})
		}()
	}

	lb.OnSelect = func(idx int) {
		if idx < 0 || idx >= len(lb.Items) {
			return
		}
		if lb.Items[idx] == ".." {
			pathEdit.SetText(vfs.Dir(vfs.GetPath()))
		} else {
			pathEdit.SetText(vfs.Join(vfs.GetPath(), lb.Items[idx]))
		}
	}

	lb.OnAction = func(idx int) {
		if idx < 0 || idx >= len(lb.Items) {
			return
		}
		selected := lb.Items[idx]
		oldPath := vfs.GetPath()
		if err := vfs.SetPath(vfs.Join(oldPath, selected)); err == nil {
			target := ""
			if selected == ".." {
				target = vfs.Base(oldPath)
			}
			updateList(vfs.GetPath(), target)
			pathEdit.SetText(vfs.GetPath())
		}
	}

	btnOk := NewButton(0, 0, Msg("vtui.Ok"))
	btnCancel := NewButton(0, 0, Msg("vtui.Cancel"))
	btnOk.OnClick = func() { dlg.SetExitCode(1) }
	btnCancel.OnClick = func() { dlg.SetExitCode(-1) }

	dlg.AddItem(pathEdit)
	dlg.AddItem(lb)
	dlg.AddItem(btnOk)
	dlg.AddItem(btnCancel)

	layout := NewAutoLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	layout.
		PinTop(pathEdit, 0).FillWidth(pathEdit, 0, 0).
		StackVertical(1, pathEdit, lb).FillWidth(lb, 0, 0).
		PinBottom(btnOk, 0).PinBottom(btnCancel, 0).
		StackHorizontal(2, btnOk, btnCancel).
		CenterHorizontalGroup(btnOk, btnCancel).
		StackVertical(1, lb, btnOk)
	layout.Apply()

	updateList(vfs.GetPath(), "")
	fm.Push(dlg)
	return dlg
}

// MessageKind selects the visual style of a message dialog.
//
// Message dialogs come in two flavours:
//   - MessageInfo — the ordinary blue/dialog palette; use for questions,
//     choices, and neutral notifications ("File already open, what now?").
//   - MessageWarn — the red WarnDialog palette; use for genuinely alarming
//     situations: destructive confirmations ("Delete N files?"), errors,
//     data-loss risks ("Unsaved changes will be lost").
//
// Prefer ShowMessageEx / ShowMessageOnEx when the semantics are known.
// The legacy ShowMessage / ShowMessageOn keep working — they infer the
// kind from a small set of well-known titles (see legacyKindFromTitle),
// which is retained for backward compatibility only.
type MessageKind int

const (
	MessageInfo MessageKind = iota
	MessageWarn
)

// legacyKindFromTitle preserves the historical behaviour of ShowMessage:
// dialogs whose trimmed title equals "Warning", "Error", or "Confirm" are
// rendered with the warning palette. New code should pass MessageKind
// explicitly via ShowMessageEx / ShowMessageOnEx and stop relying on
// this string match, which couples the visual style to the (potentially
// localised) title text.
func legacyKindFromTitle(title string) MessageKind {
	trimmed := strings.TrimSpace(title)
	if trimmed == "Warning" || trimmed == "Error" || trimmed == "Confirm" {
		return MessageWarn
	}
	return MessageInfo
}

// ShowMessage displays a message dialog whose visual style is guessed
// from the title (see legacyKindFromTitle). Kept for backward
// compatibility; prefer ShowMessageEx in new code.
func ShowMessage(title string, text string, buttons []string) *Window {
	return ShowMessageEx(title, text, buttons, legacyKindFromTitle(title))
}

// ShowMessageOn is the anchored variant of ShowMessage — see there for
// the caveats. Prefer ShowMessageOnEx in new code.
func ShowMessageOn(anchor Frame, title string, text string, buttons []string) *Window {
	return ShowMessageOnEx(anchor, title, text, buttons, legacyKindFromTitle(title))
}

// ShowMessageEx displays a message dialog with an explicit visual kind.
// The title no longer influences the palette — callers control the look
// via kind, which decouples wording from styling and lets warnings stay
// warnings regardless of localisation.
func ShowMessageEx(title string, text string, buttons []string, kind MessageKind) *Window {
	dlg := createMessageDialog(title, text, buttons, kind)
	FrameManager.Push(dlg)
	return dlg
}

// ShowMessageOnEx is the anchored variant of ShowMessageEx: the dialog
// is pushed onto the screen owned by `anchor` instead of the current
// top screen.
func ShowMessageOnEx(anchor Frame, title string, text string, buttons []string, kind MessageKind) *Window {
	dlg := createMessageDialog(title, text, buttons, kind)
	FrameManager.PushToFrameScreen(anchor, dlg)
	return dlg
}

// Internal helper to avoid code duplication.
// kind is the sole source of truth for IsWarning; title-based inference
// is done upstream (see legacyKindFromTitle) for backward compatibility.
func createMessageDialog(title string, text string, buttons []string, kind MessageKind) *Window {
	const maxDialogWidth = 72 // Comfortably fits within 80 columns
	const sidePadding = 4

	// 1. Calculate button dimensions
	btnsWidth := 0
	for _, b := range buttons {
		clean, _, _ := ParseAmpersandString(b)
		btnsWidth += StringWidth(clean) + 4
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
		w := StringWidth(l)
		if w > textWidth {
			textWidth = w
		}
	}

	dlgWidth := textWidth + sidePadding
	if !stackButtons && totalBtnsWidth+sidePadding > dlgWidth {
		dlgWidth = totalBtnsWidth + sidePadding
	}
	if title != "" {
		tw := StringWidth(title) + 6
		if tw > dlgWidth {
			dlgWidth = tw
		}
	}
	if dlgWidth > maxDialogWidth {
		dlgWidth = maxDialogWidth
	}

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

	maxScreenHeight := 25
	if FrameManager != nil && FrameManager.scr != nil && FrameManager.scr.height > 0 {
		maxScreenHeight = FrameManager.scr.height
	}

	if dlgHeight > maxScreenHeight-2 {
		dlgHeight = maxScreenHeight - 2
		allowedLines := dlgHeight - 4
		if len(buttons) > 0 {
			if stackButtons {
				allowedLines -= (len(buttons) * 2)
			} else {
				allowedLines -= 2
			}
		}
		if allowedLines < 1 {
			allowedLines = 1
		}
		if len(lines) > allowedLines {
			lines = lines[:allowedLines]
			if len(lines) > 0 {
				lines[len(lines)-1] = "... (truncated)"
			}
		}
	}

	dlg := NewCenteredDialog(dlgWidth, dlgHeight, title)
	if kind == MessageWarn {
		dlg.IsWarning = true
	}

	if dlgHeight > maxScreenHeight-2 {
		dlgHeight = maxScreenHeight - 2
		allowedLines := dlgHeight - 4
		if len(buttons) > 0 {
			if stackButtons {
				allowedLines -= (len(buttons) * 2)
			} else {
				allowedLines -= 2
			}
		}
		if allowedLines < 1 {
			allowedLines = 1
		}
		if len(lines) > allowedLines {
			lines = lines[:allowedLines]
			if len(lines) > 0 {
				lines[len(lines)-1] = "... (truncated)"
			}
		}
	}

	layout := NewAutoLayout(dlg.X1+2, dlg.Y1+2, dlgWidth-4, dlgHeight-4)

	var lastTextUI UIElement
	for _, l := range lines {
		txt := NewText(0, 0, l, Palette[ColDialogText])
		layout.CenterHorizontal(txt)
		dlg.AddItem(txt)
		if lastTextUI != nil {
			layout.StackVertical(0, lastTextUI, txt)
		} else {
			layout.PinTop(txt, 0)
		}
		lastTextUI = txt
	}

	if len(buttons) > 0 {
		btnElems := make([]UIElement, len(buttons))
		for i, b := range buttons {
			btnID := i
			btn := NewButton(0, 0, b)
			btn.OnClick = func() { dlg.SetExitCode(btnID) }
			dlg.AddItem(btn)
			btnElems[i] = btn
		}

		if stackButtons {
			for i, btn := range btnElems {
				layout.CenterHorizontal(btn)
				if i == 0 {
					if lastTextUI != nil {
						layout.StackVertical(1, lastTextUI, btn)
					} else {
						layout.PinTop(btn, 0)
					}
				} else {
					layout.StackVertical(1, btnElems[i-1], btn)
				}
			}
		} else {
			if len(btnElems) == 1 {
				layout.CenterHorizontal(btnElems[0])
			} else {
				layout.StackHorizontal(spacing, btnElems...)
				layout.CenterHorizontalGroup(btnElems[0], btnElems[len(btnElems)-1])
			}
			for _, btn := range btnElems {
				layout.PinBottom(btn, 0)
			}
		}
	}

	layout.Apply()
	return dlg
}

// SelectFileDialog creates a standard file selection dialog.
func SelectFileDialog(title string, initialPath string, vfs FSProvider, onOk func(string)) *Window {
	fm := FrameManager
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
			fm.PostTask(func() {
				if dlg.IsDone() {
					return
				}
				items := []string{".."}
				// Sort entries: Directories first, then files, both alphabetically
				sort.Slice(allEntries, func(i, j int) bool {
					if allEntries[i].IsDir != allEntries[j].IsDir {
						return allEntries[i].IsDir
					}
					return strings.ToLower(allEntries[i].Name) < strings.ToLower(allEntries[j].Name)
				})
				items = []string{".."}
				isDirMap = make(map[string]bool)
				isDirMap[".."] = true
				for _, e := range allEntries {
					if e.IsDir {
						items = append(items, e.Name)
						isDirMap[e.Name] = true
					}
				}
				for _, e := range allEntries {
					if !e.IsDir {
						items = append(items, e.Name)
						isDirMap[e.Name] = false
					}
				}
				lb.Items = items
				lb.UpdateRows()
				if targetToSelect != "" {
					lb.SelectName(targetToSelect)
				} else {
					lb.SetSelectPos(0)
				}
				fm.Redraw()
			})
		}()
	}

	lb.OnSelect = func(idx int) {
		if idx >= 0 && idx < len(lb.Items) && !isDirMap[lb.Items[idx]] {
			fileEdit.SetText(lb.Items[idx])
		}
	}
	lb.OnAction = func(idx int) {
		if idx < 0 || idx >= len(lb.Items) {
			return
		}
		selected := lb.Items[idx]
		if isDirMap[selected] {
			oldPath := vfs.GetPath()
			if err := vfs.SetPath(vfs.Join(oldPath, selected)); err == nil {
				target := ""
				if selected == ".." {
					target = vfs.Base(oldPath)
				}
				updateList(vfs.GetPath(), target)
				pathEdit.SetText(vfs.GetPath())
			}
		} else {
			dlg.SetExitCode(1)
		}
	}

	btnOk := NewButton(0, 0, Msg("vtui.Ok"))
	btnCancel := NewButton(0, 0, Msg("vtui.Cancel"))
	btnOk.OnClick = func() {
		if onOk != nil {
			onOk(vfs.Join(vfs.GetPath(), fileEdit.GetText()))
		}
		dlg.SetExitCode(1)
	}
	btnCancel.OnClick = func() { dlg.SetExitCode(-1) }

	dlg.AddItem(lblPath)
	dlg.AddItem(pathEdit)
	dlg.AddItem(lb)
	dlg.AddItem(lblFile)
	dlg.AddItem(fileEdit)
	dlg.AddItem(btnOk)
	dlg.AddItem(btnCancel)

	layout := NewAutoLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	layout.
		PinTop(lblPath, 0).PinLeft(lblPath, 0).
		AlignTop(lblPath, pathEdit).
		StackHorizontal(1, lblPath, pathEdit).PinRight(pathEdit, 0).
		StackVertical(1, pathEdit, lb).FillWidth(lb, 0, 0).
		StackVertical(1, lb, fileEdit).
		PinLeft(lblFile, 0).AlignTop(lblFile, fileEdit).
		StackHorizontal(1, lblFile, fileEdit).PinRight(fileEdit, 0).
		PinBottom(btnOk, 0).PinBottom(btnCancel, 0).
		StackHorizontal(2, btnOk, btnCancel).
		CenterHorizontalGroup(btnOk, btnCancel).
		StackVertical(1, fileEdit, btnOk)
	layout.Apply()

	updateList(vfs.GetPath(), "")
	fm.Push(dlg)
	return dlg
}

// InputBox creates a simple one-line text input dialog.
func InputBox(title, prompt, defaultText string, onOk func(string)) *Window {
	return InputBoxOn(nil, title, prompt, defaultText, onOk)
}

// InputBoxOn creates a simple one-line text input dialog tied to a specific anchor screen.
func InputBoxOn(anchor Frame, title, prompt, defaultText string, onOk func(string)) *Window {
	width := 40
	height := 9 // Increased height to fit all elements with margins
	dlg := NewCenteredDialog(width, height, title)
	dlg.ShowClose = true

	edit := NewEdit(0, 0, 10, defaultText)
	lbl := NewLabel(0, 0, prompt, edit)
	btnOk := NewButton(0, 0, Msg("vtui.Ok"))
	btnCancel := NewButton(0, 0, Msg("vtui.Cancel"))

	btnOk.OnClick = func() {
		if onOk != nil {
			onOk(edit.GetText())
		}
		dlg.SetExitCode(1)
	}
	btnCancel.OnClick = func() { dlg.SetExitCode(-1) }

	dlg.AddItem(lbl)
	dlg.AddItem(edit)
	dlg.AddItem(btnOk)
	dlg.AddItem(btnCancel)

	layout := NewAutoLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	layout.
		PinTop(lbl, 0).PinLeft(lbl, 0).
		StackVertical(1, lbl, edit).FillWidth(edit, 0, 0).
		PinBottom(btnOk, 0).PinBottom(btnCancel, 0).
		StackHorizontal(2, btnOk, btnCancel).
		CenterHorizontalGroup(btnOk, btnCancel)
	layout.Apply()

	if anchor != nil {
		FrameManager.PushToFrameScreen(anchor, dlg)
	} else {
		FrameManager.Push(dlg)
	}
	return dlg
}
