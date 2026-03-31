package vtui

import "github.com/mattn/go-runewidth"

func ShowMessage(title string, text string, buttons []string) *Dialog {
	dlg := createMessageDialog(title, text, buttons)
	FrameManager.Push(dlg)
	return dlg
}

// ShowMessageOn creates a message box targeted to a specific screen (via an anchor frame).
func ShowMessageOn(anchor Frame, title string, text string, buttons []string) *Dialog {
	// 1. Create the dialog but DON'T push it yet via the generic FrameManager.Push
	dlg := createMessageDialog(title, text, buttons)

	// 2. Target the specific screen
	FrameManager.PushToFrameScreen(anchor, dlg)
	return dlg
}

// Internal helper to avoid code duplication
func createMessageDialog(title string, text string, buttons []string) *Dialog {
	const maxDialogWidth = 60
	const padding = 4

	lines := WrapText(text, maxDialogWidth-padding)
	textWidth := 0
	for _, l := range lines {
		w := runewidth.StringWidth(l)
		if w > textWidth { textWidth = w }
	}
	if title != "" {
		tw := runewidth.StringWidth(title) + 4
		if tw > textWidth { textWidth = tw }
	}
	btnsWidth := 0
	for _, b := range buttons {
		btnsWidth += runewidth.StringWidth(b) + 5
	}
	dlgWidth := textWidth + padding
	if btnsWidth+padding > dlgWidth { dlgWidth = btnsWidth + padding }
	if dlgWidth > maxDialogWidth { dlgWidth = maxDialogWidth }
	dlgHeight := len(lines) + 4
	if len(buttons) > 0 { dlgHeight += 2 }

	scrWidth := FrameManager.GetScreenSize()
	x1 := (scrWidth - dlgWidth) / 2
	y1 := 6

	dlg := NewDialog(x1, y1, x1+dlgWidth-1, y1+dlgHeight-1, title)
	for i, l := range lines {
		lineW := runewidth.StringWidth(l)
		offX := (dlgWidth - lineW) / 2
		dlg.AddItem(NewText(x1+offX, y1+2+i, l, Palette[ColDialogText]))
	}

	if len(buttons) > 0 {
		spacing := 2
		totalBtnW := btnsWidth + (len(buttons)-1)*spacing
		currX := x1 + (dlgWidth-totalBtnW)/2
		btnY := y1 + dlgHeight - 2
		for i, b := range buttons {
			btnID := i
			btn := NewButton(currX, btnY, b)
			btn.SetOnClick(func() { dlg.SetExitCode(btnID) })
			dlg.AddItem(btn)
			currX += runewidth.StringWidth(btn.text) + spacing
		}
	}
	return dlg
}
