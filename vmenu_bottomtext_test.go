package vtui

import (
	"strings"
	"testing"

	"github.com/unxed/vtinput"
)

// rowText reads one painted row of a menu, border to border.
func rowText(scr *ScreenBuf, m *VMenu, y int) string {
	var sb strings.Builder
	for x := m.X1; x <= m.X2; x++ {
		sb.WriteRune(rune(scr.GetCell(x, y).Char))
	}
	return sb.String()
}

func TestVMenu_BottomTextShowsSelectedDescription(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())
	m := NewVMenu("Config")
	for i, s := range []string{"one", "two", "three", "four", "five", "six"} {
		m.AddItem(MenuItem{Text: s, Description: strings.Repeat(s+" ", 3) + "end" + string(rune('A'+i))})
	}
	// 12 rows: border, 7 list rows, separator, 2 text rows, border.
	m.SetPosition(0, 0, 19, 11)
	m.SetBottomTextLines(2)
	if m.ViewHeight != 7 {
		t.Fatalf("ViewHeight = %d, want the list to stop above the separator (7)", m.ViewHeight)
	}
	if got := m.BottomTextWidth(); got != 16 {
		t.Fatalf("BottomTextWidth = %d, want 16", got)
	}

	m.SetSelectPos(1)
	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 12)
	m.Show(scr)
	if sep := rowText(scr, m, 8); strings.ContainsAny(sep, "abcdefghijklmnopqrstuvwxyz") {
		t.Errorf("separator row = %q, want a rule and no item text", sep)
	}
	text := rowText(scr, m, 9) + rowText(scr, m, 10)
	if !strings.Contains(text, "two two two") || !strings.Contains(text, "endB") {
		t.Errorf("bottom text = %q, want the wrapped description of the selected item", text)
	}
	for y := 1; y < 8; y++ {
		if strings.Contains(rowText(scr, m, y), "endB") {
			t.Errorf("row %d shows the description inside the list", y)
		}
	}

	// The area follows the selection.
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	scr = NewSilentScreenBuf()
	scr.AllocBuf(20, 12)
	m.Show(scr)
	text = rowText(scr, m, 9) + rowText(scr, m, 10)
	if !strings.Contains(text, "three") || strings.Contains(text, "two") {
		t.Errorf("bottom text after Down = %q, want the description of the new selection", text)
	}
}

func TestVMenu_BottomTextAreaIsNotAListRow(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())
	m := NewVMenu("Config")
	for i := 0; i < 20; i++ {
		m.AddItem(MenuItem{Text: "item", Description: "text"})
	}
	m.SetBottomTextLines(3)
	m.SetPosition(0, 0, 29, 11) // list rows 1..6, separator 7, text 8..10
	if m.ViewHeight != 6 {
		t.Fatalf("ViewHeight = %d, want 6", m.ViewHeight)
	}
	for _, y := range []int{7, 8, 10} {
		if idx := m.GetClickIndex(y); idx != -1 {
			t.Errorf("GetClickIndex(%d) = %d, want no item under the bottom area", y, idx)
		}
	}
	if idx := m.GetClickIndex(6); idx != 5 {
		t.Errorf("GetClickIndex(6) = %d, want the last list row (5)", idx)
	}

	m.SetBottomTextLines(0)
	if m.ViewHeight != 10 {
		t.Errorf("ViewHeight after removing the area = %d, want 10", m.ViewHeight)
	}
}
