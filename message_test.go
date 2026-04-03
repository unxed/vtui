package vtui

import "testing"

func TestShowMessage_Structure(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())

	title := "Warning"
	text := "This is a test message\nwith two lines."
	buttons := []string{"&Yes", "&No", "&Cancel"}

	dlg := ShowMessage(title, text, buttons)

	// Check the number of elements:
	// 2 lines of text + 3 buttons = 5 elements
	if len(dlg.rootGroup.items) != 5 {
		t.Errorf("Wrong item count. Got %d, want 5", len(dlg.rootGroup.items))
	}

	// Check the frame title
	if dlg.frame.title != title {
		t.Errorf("Wrong title. Got %q, want %q", dlg.frame.title, title)
	}

	// Check that buttons return the correct ExitCode
	for i := 0; i < 3; i++ {
		btn := dlg.rootGroup.items[2+i].(*Button)
		if btn.OnClick != nil {
			btn.OnClick()
		}
		if dlg.ExitCode != i {
			t.Errorf("Button %d failed to set exit code. Got %d", i, dlg.ExitCode)
		}
	}
}