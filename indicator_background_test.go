package vtui

import "testing"

func TestDialogIndicatorBackgroundRender(t *testing.T) {
	saved := append([]uint64(nil), Palette...)
	defer func() { Palette = saved }()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(40, 8)
	controls := []UIElement{NewCheckbox(1, 1, "Label", false), NewRadioButton(1, 3, "Label", true)}
	for iteration := 0; iteration < 2; iteration++ {
		Palette[ColDialogText] = SetRGBBoth(0, uint32(0xabcdef+iteration), uint32(0x334455+iteration))
		Palette[ColDialogSelectedButton] = SetRGBBoth(0, 0xffffff, uint32(0x556677+iteration))
		Palette[ColDialogHighlightText] = SetRGBBoth(0, 0xeeee00, 0x334455)
		Palette[ColDialogHighlightSelectedButton] = SetRGBBoth(0, 0xdddd00, 0x556677)
		for _, background := range []uint64{0, SetRGBBack(0, uint32(0x112233+iteration)), SetIndexBack(0, 4)} {
			Palette[ColDialogIndicatorBackground] = background
			for _, focused := range []bool{false, true} {
				for _, disabled := range []bool{false, true} {
					for _, control := range controls {
						control.SetFocus(focused)
						control.SetDisabled(disabled)
						control.Show(scr)
						x, y, _, _ := control.GetPosition()
						label := scr.GetCell(x+4, y).Attributes
						want := label
						if !control.IsFocused() && background != 0 {
							if background&IsBgRGB != 0 {
								want = SetRGBBack(label, GetRGBBack(background))
							} else {
								want = SetIndexBack(label, GetIndexBack(background))
							}
						}
						for dx := 0; dx < 3; dx++ {
							if scr.GetCell(x+dx, y).Attributes != want {
								t.Fatalf("%T focused=%v disabled=%v bg=%x got=%x want=%x", control, control.IsFocused(), disabled, background, scr.GetCell(x+dx, y).Attributes, want)
							}
						}
						if scr.GetCell(x+3, y).Attributes != label {
							t.Fatal("background spilled onto spacing")
						}
					}
				}
			}
		}
	}
}
