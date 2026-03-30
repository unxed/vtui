package vtui

import (
	"testing"
	"github.com/unxed/vtinput"
)

// UIElement is the interface that all dialog elements must implement.
type UIElement interface {
	GetPosition() (int, int, int, int)
	SetPosition(int, int, int, int)
	GetGrowMode() GrowMode
	Show(scr *ScreenBuf)
	Hide(scr *ScreenBuf)
	SetFocus(bool)
	IsFocused() bool
	CanFocus() bool
	GetHotkey() rune
	GetId() string
	GetHelp() string
	ProcessKey(e *vtinput.InputEvent) bool
	ProcessMouse(e *vtinput.InputEvent) bool
	HandleCommand(cmd int, args any) bool
	HandleBroadcast(cmd int, args any) bool
	Valid(cmd int) bool
}

// DataControl is an interface for UI elements that can store and return data.
type DataControl interface {
	SetData(value any)
	GetData() any
}

// Dialog is a modal container for UI elements.
type Dialog struct {
	BaseWindow
}

func NewDialog(x1, y1, x2, y2 int, title string) *Dialog {
	d := &Dialog{
		BaseWindow: *NewBaseWindow(x1, y1, x2, y2, title),
	}
	return d
}

func (d *Dialog) IsModal() bool { return true }
func (d *Dialog) GetType() FrameType { return TypeDialog }
func (d *Dialog) GetTitle() string { return d.frame.title }
func (d *Dialog) GetProgress() int {
	// If the dialog contains a text element that looks like a percentage,
	// or we can manually set it. For this demo, we'll allow manual override.
	return d.progress
}

func (d *Dialog) SetProgress(p int) {
	d.progress = p
}

func TestBaseWindow_DataMapping(t *testing.T) {
	type TestData struct {
		Name      string `vtui:"user_name"`
		Admin     bool   `vtui:"is_admin"`
		Option    int    `vtui:"opt_radio"`
		Flags     uint32 // No tag, should use field name
	}

	bw := NewBaseWindow(0, 0, 40, 20, "Data Test")

	edit := NewEdit(1, 1, 20, "")
	edit.SetId("user_name")
	bw.AddItem(edit)

	chk := NewCheckbox(1, 2, "Admin", false)
	chk.SetId("is_admin")
	bw.AddItem(chk)

	rg := NewRadioGroup(1, 3, 1, []string{"O1", "O2"})
	rg.SetId("opt_radio")
	bw.AddItem(rg)

	cg := NewCheckGroup(1, 6, 1, []string{"F1", "F2"})
	cg.SetId("Flags")
	bw.AddItem(cg)

	// 1. Test SetData
	input := TestData{
		Name:   "Explorer",
		Admin:  true,
		Option: 1,
		Flags:  0x02, // Only F2 checked
	}
	bw.SetData(input)

	if edit.GetText() != "Explorer" { t.Errorf("Edit failed: %s", edit.GetText()) }
	if chk.State != 1 { t.Error("Checkbox failed") }
	if rg.Selected != 1 { t.Error("Radio failed") }
	if !cg.States[1] || cg.States[0] { t.Error("CheckGroup failed") }

	// 2. Test GetData
	edit.SetText("NewName")
	chk.State = 0
	rg.Selected = 0
	cg.States[0] = true

	var output TestData
	bw.GetData(&output)

	if output.Name != "NewName" { t.Errorf("GetData string fail: %s", output.Name) }
	if output.Admin != false { t.Error("GetData bool fail") }
	if output.Option != 0 { t.Error("GetData int fail") }
	if output.Flags != 0x03 { t.Errorf("GetData mask fail: 0x%X", output.Flags) }
}
