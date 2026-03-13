package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"golang.org/x/term"
)

func main() {
	// Enable advanced terminal mode
	restore, err := vtinput.Enable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error enabling raw mode: %v\n", err)
		return
	}
	defer restore()

	// Hide cursor during execution
	fmt.Print("\x1b[?25l")
	defer fmt.Print("\x1b[?25h")

	// Get initial terminal size
	width, height, _ := term.GetSize(int(os.Stdin.Fd()))

	// Create main objects
	scr := vtui.NewScreenBuf()
	scr.AllocBuf(width, height)

	// Calculate start centering coordinates
	fWidth, fHeight := 40, 14
	x1 := (width - fWidth) / 2
	y1 := (height - fHeight) / 2
	// Create dialog
	dlg := vtui.NewDialog(x1, y1, x1+fWidth-1, y1+fHeight-1, " Action Dialog ")

	// Create menu
	menu := vtui.NewVMenu(" Select Action ")
	menu.AddItem("Copy File")
	menu.AddItem("Move File")
	menu.AddSeparator()
	menu.AddItem("Delete File")
	menu.AddItem("Attributes")
	menu.AddSeparator()
	menu.AddItem("Exit")
	menu.SetPosition(x1+5, y1+2, x1+30, y1+8)

	// Create text, edit field and buttons
	label := vtui.NewText(x1+5, y1+1, "Enter task name:", vtui.SetRGBFore(0, 0xFFFFFF))
	edit := vtui.NewEdit(x1+5, y1+2, 20, "f4 project")

	// Move menu down so it doesn't overlap with the title
	menu.SetPosition(x1+5, y1+5, x1+30, y1+10)

	// Move buttons to the bottom of the new high dialog
	btnOk := vtui.NewButton(x1+5, y1+12, "Ok")
	btnCancel := vtui.NewButton(x1+15, y1+12, "Cancel")

	// Setup Button Actions
	shouldExit := false
	btnCancel.OnClick = func() {
		shouldExit = true
	}
	btnOk.OnClick = func() {
		edit.SetFocus(true) // Just moving focus to show it works
	}

	// Assemble everything into the dialog
	dlg.AddItem(label)
	dlg.AddItem(edit)
	dlg.AddItem(menu)
	dlg.AddItem(btnOk)
	dlg.AddItem(btnCancel)

	// Configure channel for receiving vtinput events
	reader := vtinput.NewReader(os.Stdin)
	eventChan := make(chan *vtinput.InputEvent, 1)
	go func() {
		for {
			e, err := reader.ReadEvent()
			if err != nil {
				if err != io.EOF {
					// Log error
				}
				close(eventChan)
				return
			}
			eventChan <- e
		}
	}()

	// Configure channel for tracking window resizing
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGWINCH)

	// --- Main application loop ---
	for {
		// 1. Rendering
		scr.FillRect(0, 0, width-1, height-1, ' ', vtui.SetRGBBack(0, 0x0000A0))
		dlg.Show(scr)

		// Status bar
		status := fmt.Sprintf(" Size: %dx%d | Tab: Switch Focus | Arrows/ESC ", width, height)
		scr.Write(0, height-1, strToCharInfo(status, vtui.SetRGBBoth(0, 0, 0x007BA7)))
		scr.Flush()

		// 2. Event waiting
		select {
		case e, ok := <-eventChan:
			if !ok { return }

			if e.Type == vtinput.KeyEventType {
				if dlg.ProcessKey(e) {
					// Check if a button action triggered exit
					if shouldExit { return }
					continue
				}
				// Global keys (Esc / Resize)
				if handleKeyEvent(e, dlg) {
					return
				}
			} else if e.Type == vtinput.MouseEventType {
				if dlg.ProcessMouse(e) {
					if shouldExit { return }
					continue
				}
			}

		case <-sigChan:
			width, height, _ = term.GetSize(int(os.Stdin.Fd()))
			scr.AllocBuf(width, height)
			// Re-center dialog considering the new height
			dlgWidth, dlgHeight := 40, 14
			newX1 := (width - dlgWidth) / 2
			newY1 := (height - dlgHeight) / 2
			dlg.SetPosition(newX1, newY1, newX1+dlgWidth-1, newY1+dlgHeight-1)
		}
	}
}

// handleKeyEvent handles keyboard events. Returns true for exit.
func handleKeyEvent(e *vtinput.InputEvent, dlg *vtui.Dialog) bool {
	if e.Type != vtinput.KeyEventType || !e.KeyDown {
		return false
	}

	if dlg == nil {
		return false
	}

	x1, y1, x2, y2 := dlg.GetPosition()

	switch e.VirtualKeyCode {
	case vtinput.VK_ESCAPE:
		return true
	case vtinput.VK_C:
		if e.ControlKeyState&vtinput.LeftCtrlPressed != 0 {
			return true
		}
	case vtinput.VK_UP:
		y1, y2 = y1-1, y2-1
	case vtinput.VK_DOWN:
		y1, y2 = y1+1, y2+1
	case vtinput.VK_LEFT:
		x1, x2 = x1-1, x2-1
	case vtinput.VK_RIGHT:
		x1, x2 = x1+1, x2+1
	}

	dlg.SetPosition(x1, y1, x2, y2)
	return false
}

// Helper function for quick string conversion to []CharInfo
func strToCharInfo(str string, attributes uint64) []vtui.CharInfo {
	runes := []rune(str)
	info := make([]vtui.CharInfo, len(runes))
	for i, r := range runes {
		info[i].Char = uint64(r)
		info[i].Attributes = attributes
	}
	return info
}