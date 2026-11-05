package vtui

import "testing"

func drawEditForHoverTest(e *Edit) *ScreenBuf {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 1)
	e.Show(scr)
	return scr
}

func TestEdit_HoverTintsUnfocusedFieldInRoundedStyle(t *testing.T) {
	roundedButtonEnv(t, "gogpu", GlyphStyleRounded)
	SetDefaultPalette()
	e := NewEdit(0, 0, 12, "text")
	base := drawEditForHoverTest(e).GetCell(0, 0).Attributes

	e.SetMouseHovered(true)
	after := drawEditForHoverTest(e).GetCell(0, 0).Attributes
	if attrColorRGB(base, true) == attrColorRGB(after, true) {
		t.Fatal("hover did not tint the unfocused edit field")
	}
	if got := contrastRatioRGB(attrColorRGB(after, false), attrColorRGB(after, true)); got < roundedButtonMinContrast {
		t.Errorf("hover contrast = %.2f, want >= %.1f", got, roundedButtonMinContrast)
	}
	// Blank cells past the text are tinted too.
	if drawEditForHoverTest(e).GetCell(8, 0).Attributes == drawEditForHoverTest(NewEdit(0, 0, 12, "text")).GetCell(8, 0).Attributes {
		t.Error("blank part of the field was not tinted")
	}

	e.SetMouseHovered(false)
	if got := drawEditForHoverTest(e).GetCell(0, 0).Attributes; got != base {
		t.Error("leaving the field did not remove the tint")
	}
}

func TestEdit_HoverNotAppliedWhenFocusedDisabledOrClassic(t *testing.T) {
	roundedButtonEnv(t, "gogpu", GlyphStyleRounded)
	SetDefaultPalette()

	focused := NewEdit(0, 0, 12, "text")
	focused.SetFocus(true)
	base := drawEditForHoverTest(focused).GetCell(0, 0).Attributes
	focused.SetMouseHovered(true)
	if got := drawEditForHoverTest(focused).GetCell(0, 0).Attributes; got != base {
		t.Error("focused field must not be tinted by hover")
	}

	disabled := NewEdit(0, 0, 12, "text")
	disabled.SetDisabled(true)
	disabled.SetMouseHovered(true)
	if disabled.mouseHovered {
		t.Error("disabled field must not track hover")
	}

	SetGlyphStyle(GlyphStyleClassic)
	classic := NewEdit(0, 0, 12, "text")
	base = drawEditForHoverTest(classic).GetCell(0, 0).Attributes
	classic.SetMouseHovered(true)
	if got := drawEditForHoverTest(classic).GetCell(0, 0).Attributes; got != base {
		t.Error("classic style must not be tinted by hover")
	}
}
