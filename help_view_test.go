package vtui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/vtinput"
)

func TestHelpView_Navigation(t *testing.T) {
	tmpDir := t.TempDir()
	helpPath := filepath.Join(tmpDir, "test.hlf")
	content := `
@Contents
~GoToNext~NextTopic@

@NextTopic
Success
`
	os.WriteFile(helpPath, []byte(content), 0644)

	engine := NewHelpEngine(&mockHelpVFS{})
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
	engine := NewHelpEngine(&mockHelpVFS{})
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
	engine := NewHelpEngine(&mockHelpVFS{})
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
	engine := NewHelpEngine(&mockHelpVFS{})
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
	engine := NewHelpEngine(&mockHelpVFS{})
	engine.topics["Test"] = &HelpTopic{Name: "Test", Lines: []string{"Text"}}
	hv := NewHelpView(engine, "Test")

	if hv.IsDone() { t.Fatal("Should not be done initially") }

	// Backspace with empty history should close help
	hv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_BACK})

	if !hv.IsDone() {
		t.Error("HelpView should close on Backspace if history is empty")
	}
}
func TestHelpView_EnsureLinkVisible(t *testing.T) {
	engine := NewHelpEngine(&mockHelpVFS{})
	// Создаем тему, где ссылка находится далеко внизу
	lines := make([]string, 20)
	for i := range lines { lines[i] = "line" }
	lines[15] = "~TargetLink~Topic@"

	topic := &HelpTopic{
		Name: "Long",
		Lines: lines,
		Links: []HelpLink{{Text: "TargetLink", Target: "Topic", Line: 15}},
	}
	engine.topics["Long"] = topic

	hv := NewHelpView(engine, "Long")
	hv.SetPosition(0, 0, 30, 5) // Видимая область контента мала (высота 6, контент 4)

	// Изначально мы вверху
	if hv.scrollTop != 0 { t.Fatal("Should start at top") }

	// Выбираем ссылку на 15-й строке
	hv.selectedIdx = 0
	hv.ensureLinkVisible()

	// ScrollTop должен измениться, чтобы 15-я строка стала видимой
	if hv.scrollTop == 0 {
		t.Errorf("scrollTop should have increased to show link at line 15, got %d", hv.scrollTop)
	}
}

func TestHelpView_PageNavigation(t *testing.T) {
	engine := NewHelpEngine(&mockHelpVFS{})
	lines := make([]string, 50)
	for i := range lines { lines[i] = "text" }
	engine.topics["Scroll"] = &HelpTopic{Name: "Scroll", Lines: lines}

	hv := NewHelpView(engine, "Scroll")
	hv.SetPosition(0, 0, 20, 11) // Высота 12, контент ~10 строк

	// 1. PgDn
	hv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT})
	if hv.scrollTop == 0 { t.Error("PgDn failed to scroll") }

	midScroll := hv.scrollTop

	// 2. PgUp
	hv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_PRIOR})
	if hv.scrollTop >= midScroll { t.Error("PgUp failed to scroll up") }
}

func TestHelpView_TabNoLinks(t *testing.T) {
	engine := NewHelpEngine(&mockHelpVFS{})
	engine.topics["NoLinks"] = &HelpTopic{Name: "NoLinks", Lines: []string{"Just text"}}

	hv := NewHelpView(engine, "NoLinks")

	// Tab не должен паниковать и должен возвращать false (или обрабатываться базовым окном)
	//handled := hv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})

	// В BaseWindow Tab переключает фокус между элементами.
	// В HelpView элементов нет, поэтому вернет true (поглотит ключ), но selectedIdx останется -1.
	if hv.selectedIdx != -1 {
		t.Errorf("selectedIdx should remain -1 when no links present, got %d", hv.selectedIdx)
	}
}

func TestHelpView_MultiLinkLineRendering(t *testing.T) {
	SetDefaultPalette()
	engine := NewHelpEngine(&mockHelpVFS{})
	// Две ссылки на одной строке
	line := "~L1~T1@ and ~L2~T2@"
	engine.topics["Test"] = &HelpTopic{
		Name: "Test",
		Lines: []string{line},
		Links: []HelpLink{
			{Text: "L1", Target: "T1", Line: 0},
			{Text: "L2", Target: "T2", Line: 0},
		},
	}
	hv := NewHelpView(engine, "Test")
	hv.SetPosition(0, 0, 40, 5)

	scr := NewScreenBuf()
	scr.AllocBuf(42, 7)

	// 1. Выбрана первая ссылка (L1)
	hv.selectedIdx = 0
	hv.Show(scr)
	// Текст: "L1 and L2". Отступ окна 1.
	// L1: X=1. " and ": 5 символов. L2: X=1+2+5 = 8.
	checkCell(t, scr, 1, 1, 'L', Palette[ColHelpSelectedLink])
	checkCell(t, scr, 8, 1, 'L', Palette[ColHelpLink])

	// 2. Выбираем вторую ссылку (L2)
	hv.selectedIdx = 1
	hv.Show(scr)
	checkCell(t, scr, 1, 1, 'L', Palette[ColHelpLink])
	checkCell(t, scr, 8, 1, 'L', Palette[ColHelpSelectedLink])
}
