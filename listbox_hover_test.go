package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

func drawListBoxForHoverTest(t *testing.T, lb *ListBox) *ScreenBuf {
	t.Helper()
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 3)
	lb.Show(scr)
	return scr
}

func TestListBox_RoundedHoverTintsOnlyHoveredUnfocusedRow(t *testing.T) {
	roundedButtonEnv(t, "gogpu", GlyphStyleRounded)
	lb := NewListBox(0, 0, 20, 3, []string{"First", "Second", "Third"})
	base := drawListBoxForHoverTest(t, lb)

	lb.SetMouseHovered(true)
	if !lb.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, MouseX: 2, MouseY: 1, MouseEventFlags: vtinput.MouseMoved}) {
		t.Fatal("moving over a different row did not request redraw")
	}
	hovered := drawListBoxForHoverTest(t, lb)
	if sameBackground(base.GetCell(0, 1).Attributes, hovered.GetCell(0, 1).Attributes) {
		t.Fatal("row under the pointer did not receive a hover tint")
	}
	if !sameBackground(base.GetCell(0, 0).Attributes, hovered.GetCell(0, 0).Attributes) {
		t.Fatal("a row outside the pointer did receive a hover tint")
	}
	attr := hovered.GetCell(0, 1).Attributes
	if got := contrastRatioRGB(attrColorRGB(attr, false), attrColorRGB(attr, true)); got < roundedButtonMinContrast {
		t.Errorf("hover contrast = %.2f, want >= %.1f", got, roundedButtonMinContrast)
	}

	lb.SetFocus(true)
	focusedBase := drawListBoxForHoverTest(t, lb)
	lb.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, MouseX: 2, MouseY: 1, MouseEventFlags: vtinput.MouseMoved})
	focusedHover := drawListBoxForHoverTest(t, lb)
	if !sameBackground(focusedBase.GetCell(0, 1).Attributes, focusedHover.GetCell(0, 1).Attributes) {
		t.Fatal("focused list row received a hover tint")
	}

	lb.SetFocus(false)
	lb.SetMouseHovered(false)
	left := drawListBoxForHoverTest(t, lb)
	if !sameBackground(base.GetCell(0, 1).Attributes, left.GetCell(0, 1).Attributes) {
		t.Fatal("hover tint remained after leaving the list")
	}
}

func TestListBox_HoverKeepsClassicAndTextTerminalLook(t *testing.T) {
	for _, env := range []struct {
		name, backend string
	}{{"classic graphical", "gogpu"}, {"text terminal", ""}} {
		t.Run(env.name, func(t *testing.T) {
			roundedButtonEnv(t, env.backend, GlyphStyleClassic)
			lb := NewListBox(0, 0, 20, 2, []string{"First", "Second"})
			base := drawListBoxForHoverTest(t, lb)
			lb.SetMouseHovered(true)
			lb.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, MouseX: 2, MouseY: 0, MouseEventFlags: vtinput.MouseMoved})
			after := drawListBoxForHoverTest(t, lb)
			if base.GetCell(0, 0).Attributes != after.GetCell(0, 0).Attributes {
				t.Fatal("classic/text list hover changed the row attribute")
			}
		})
	}
}
