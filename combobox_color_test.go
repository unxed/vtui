package vtui

import "testing"

// A dropdown must take its colours from the combo group rather than from the
// menu group, so that a theme can keep it distinct from the dialog behind it.
func TestComboBox_DropdownUsesComboPalette(t *testing.T) {
	cb := NewComboBox(0, 0, 10, []string{"one", "two"})
	if cb.Menu.BoxType != SingleBox {
		t.Fatalf("dropdown box type = %d, want SingleBox", cb.Menu.BoxType)
	}

	cases := []struct {
		name      string
		got, want int
	}{
		{"text", cb.Menu.ColorTextIdx, ColDialogComboText},
		{"selected text", cb.Menu.ColorSelectedTextIdx, ColDialogComboSelectedText},
		{"highlight", cb.Menu.ColorHighlightIdx, ColDialogComboHighlight},
		{"selected highlight", cb.Menu.ColorSelectedHighlightIdx, ColDialogComboSelectedHighlight},
		{"box", cb.Menu.ColorBoxIdx, ColDialogComboBox},
		{"title", cb.Menu.ColorTitleIdx, ColDialogComboTitle},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("dropdown %s index = %d, want %d", tc.name, tc.got, tc.want)
		}
	}
	if cb.Menu.ScrollBar == nil {
		t.Fatal("dropdown has no scrollbar")
	}
	if cb.Menu.ScrollBar.ColorIdx != ColDialogComboScrollbar {
		t.Errorf("dropdown scrollbar index = %d, want %d",
			cb.Menu.ScrollBar.ColorIdx, ColDialogComboScrollbar)
	}
}

// The visible symptom the indices above are there to prevent: a dropdown
// painted in the dialog's own background, indistinguishable from it.
func TestComboBox_DropdownStandsApartFromDialog(t *testing.T) {
	preserveTestPalette(t)
	SetDefaultPalette()
	Palette[ColDialogText] = SetRGBBoth(0, 0x2E3436, 0xD3D7CF)
	Palette[ColDialogComboText] = SetRGBBoth(0, 0xEEEEEC, 0x06989A)
	Palette[ColDialogComboSelectedText] = SetRGBBoth(0, 0xEEEEEC, 0x2E3436)

	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 8)

	cb := NewComboBox(0, 0, 10, []string{"one", "two"})
	cb.Menu.SetPosition(0, 0, 10, 4)
	cb.Menu.Show(scr)
	single := getBoxSymbols(SingleBox)
	for _, corner := range []struct {
		x, y int
		want rune
	}{
		{0, 0, single[bsTL]},
		{10, 0, single[bsTR]},
		{0, 4, single[bsBL]},
		{10, 4, single[bsBR]},
	} {
		if got := rune(scr.GetCell(corner.x, corner.y).Char); got != corner.want {
			t.Errorf("dropdown corner (%d,%d) = %q, want %q", corner.x, corner.y, got, corner.want)
		}
	}

	// A menu selects its first item on construction, so row 1 is the selected
	// one and row 2 an ordinary item. Both must come from the combo group;
	// neither may fall back to the dialog background behind the dropdown.
	if got := GetRGBBack(scr.GetCell(2, 2).Attributes); got != 0x06989A {
		t.Errorf("dropdown background = #%06x, want the combo colour #06989a", got)
	}
	if got := GetRGBBack(scr.GetCell(2, 1).Attributes); got != 0x2E3436 {
		t.Errorf("selected row background = #%06x, want #2e3436", got)
	}
}

// A plain menu keeps the Menu.* group.
func TestVMenu_DefaultsToMenuPalette(t *testing.T) {
	m := NewVMenu("Menu")
	if m.BoxType != DoubleBox {
		t.Fatalf("plain menu box type = %d, want DoubleBox", m.BoxType)
	}
	if m.ColorTextIdx != ColMenuText || m.ColorBoxIdx != ColMenuBox || m.ColorTitleIdx != ColMenuTitle {
		t.Errorf("plain menu should default to the Menu.* entries, got %d/%d/%d",
			m.ColorTextIdx, m.ColorBoxIdx, m.ColorTitleIdx)
	}
}
func TestComboBox_DropdownOnlyFocusedFillsSelectionContinuously(t *testing.T) {
	preserveTestPalette(t)
	SetDefaultPalette()
	Palette[ColDialogComboText] = SetRGBBoth(0, 0xEEEEEC, 0x37322C)
	Palette[ColDialogComboSelectedText] = SetRGBBoth(0, 0x1E1A16, 0xE6B450)
	Palette[ColDialogComboSelectedHighlight] = SetRGBBoth(0, 0xA04020, 0xE6B450)

	scr := NewSilentScreenBuf()
	scr.AllocBuf(30, 3)

	cb := NewComboBox(1, 1, 20, []string{"One", "Two"})
	cb.DropdownOnly = true
	cb.Edit.SetText("One")
	cb.SetFocus(true)
	cb.Show(scr)

	selectedBg := GetRGBBack(Palette[ColDialogComboSelectedText])

	for x := cb.X1; x < cb.X2; x++ {
		cellBg := GetRGBBack(scr.GetCell(x, cb.Y1).Attributes)
		if cellBg != selectedBg {
			t.Errorf("text cell X=%d background = #%06x, want selected background #%06x", x, cellBg, selectedBg)
		}
	}

	arrowBg := GetRGBBack(scr.GetCell(cb.X2, cb.Y1).Attributes)
	if arrowBg != selectedBg {
		t.Errorf("arrow cell X=%d background = #%06x, want selected background #%06x", cb.X2, arrowBg, selectedBg)
	}
}

func TestComboBox_UnfocusedArrowIsVisible(t *testing.T) {
	preserveTestPalette(t)
	SetDefaultPalette()
	Palette[ColDialogComboText] = SetRGBBoth(0, 0xEEEEEC, 0x37322C)
	Palette[ColDialogComboHighlight] = SetRGBBoth(0, 0xE6CF70, 0x37322C)

	scr := NewSilentScreenBuf()
	scr.AllocBuf(30, 3)

	cb := NewComboBox(1, 1, 20, []string{"One", "Two"})
	cb.Show(scr)

	arrowCell := scr.GetCell(cb.X2, cb.Y1)
	arrowFg := GetRGBFore(arrowCell.Attributes)
	arrowBg := GetRGBBack(arrowCell.Attributes)

	if arrowFg == arrowBg {
		t.Errorf("arrow foreground #%06x equals background #%06x (invisible arrow)", arrowFg, arrowBg)
	}
	if arrowFg != 0xE6CF70 {
		t.Errorf("arrow foreground = #%06x, want highlight color #e6cf70", arrowFg)
	}
	if arrowBg != 0x37322C {
		t.Errorf("arrow background = #%06x, want combo background #37322c", arrowBg)
	}
}
