package vtui

import (
	"testing"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
)

func TestHelpView_Navigation(t *testing.T) {
	memVfs := vfs.NewOSVFS(t.TempDir())
	helpPath := memVfs.Join(memVfs.GetPath(), "test.hlf")
	content := `
@Contents
~GoToNext~NextTopic@

@NextTopic
Success
`
	wc, _ := memVfs.Create(helpPath)
	wc.Write([]byte(content))
	wc.Close()

	engine := NewHelpEngine(memVfs)
	engine.LoadFile(helpPath)

	hv := NewHelpView(engine, "Contents")

	// 1. Initial state
	if hv.current.Name != "Contents" { t.Errorf("Expected Contents, got %s", hv.current.Name) }
	if hv.selectedIdx != 0 { t.Error("Link should be selected by default") }

	// 2. Press Enter to jump
	hv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})

	if hv.current.Name != "NextTopic" {
		t.Errorf("Jump failed, current is %s", hv.current.Name)
	}
	if len(hv.history) != 1 || hv.history[0] != "Contents" {
		t.Error("History not updated after jump")
	}

	// 3. Press Backspace to return
	hv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_BACK})
	if hv.current.Name != "Contents" {
		t.Error("History Back failed")
	}
}

func TestHelpView_TabWrapping(t *testing.T) {
	memVfs := vfs.NewOSVFS(t.TempDir())
	engine := NewHelpEngine(memVfs)
	topic := &HelpTopic{
		Name: "Test",
		Lines: []string{"~L1~T1@ ~L2~T2@"},
		Links: []HelpLink{
			{Text: "L1", Target: "T1", Line: 0},
			{Text: "L2", Target: "T2", Line: 0},
		},
	}
	engine.topics["Test"] = topic
	hv := NewHelpView(engine, "Test")

	// Start at 0
	if hv.selectedIdx != 0 { t.Fatal("Start index should be 0") }

	// 1. Tab to 1
	hv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if hv.selectedIdx != 1 { t.Error("Tab failed to move forward") }

	// 2. Tab to wrap back to 0
	hv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if hv.selectedIdx != 0 { t.Error("Tab failed to wrap around") }

	// 3. Shift+Tab to wrap to 1
	hv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB, ControlKeyState: vtinput.ShiftPressed})
	if hv.selectedIdx != 1 { t.Error("Shift+Tab failed to wrap around") }
}

func TestHelpView_Scrolling(t *testing.T) {
	memVfs := vfs.NewOSVFS(t.TempDir())
	engine := NewHelpEngine(memVfs)
	engine.topics["Test"] = &HelpTopic{
		Name: "Test",
		Lines: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"},
	}
	hv := NewHelpView(engine, "Test")
	hv.SetPosition(0, 0, 10, 4) // Visible content height = 3 (1 title, 1 sticky, 1 scrolling line)
	hv.current.StickyRows = 1

	// 1. Scroll Down
	hv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if hv.scrollTop != 1 { t.Errorf("Expected scrollTop 1, got %d", hv.scrollTop) }

	// 2. Scroll Up
	hv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP})
	if hv.scrollTop != 0 { t.Error("Scroll Up failed") }
}

func TestHelpView_VisualRendering(t *testing.T) {
	SetDefaultPalette()
	memVfs := vfs.NewOSVFS(t.TempDir())
	engine := NewHelpEngine(memVfs)
	engine.topics["Test"] = &HelpTopic{
		Name: "Test",
		Lines: []string{"Normal #Bold# ~Link~T@"},
		Links: []HelpLink{{Text: "Link", Target: "T", Line: 0}},
	}
	hv := NewHelpView(engine, "Test")
	hv.SetPosition(0, 0, 30, 5)
	hv.selectedIdx = -1 // Deselect link for baseline check

	scr := NewScreenBuf()
	scr.AllocBuf(32, 7)
	hv.Show(scr)

	// Check "Normal" (X=1..6 in local, X=1..6 in global because hv is at 0)
	checkCell(t, scr, 1, 1, 'N', Palette[ColHelpText])

	// Check "Bold" (starts at X=8)
	checkCell(t, scr, 8, 1, 'B', Palette[ColHelpBold])

	// Check "Link" (starts at X=13)
	checkCell(t, scr, 13, 1, 'L', Palette[ColHelpLink])

	// 2. Select link and check highlight
	hv.selectedIdx = 0
	hv.Show(scr)
	checkCell(t, scr, 13, 1, 'L', Palette[ColHelpSelectedLink])
}

func TestHelpView_History_Empty(t *testing.T) {
	memVfs := vfs.NewOSVFS(t.TempDir())
	engine := NewHelpEngine(memVfs)
	engine.topics["Test"] = &HelpTopic{Name: "Test", Lines: []string{"Text"}}
	hv := NewHelpView(engine, "Test")

	if hv.IsDone() { t.Fatal("Should not be done initially") }

	// Backspace with empty history should close help
	hv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_BACK})

	if !hv.IsDone() {
		t.Error("HelpView should close on Backspace if history is empty")
	}
}
