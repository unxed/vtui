package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

func TestTitleButtonHover_TintsOnlyHoveredGraphicalControl(t *testing.T) {
	roundedButtonEnv(t, "gogpu", GlyphStyleRounded)
	SetDefaultPalette()
	win := NewWindow(5, 5, 45, 12, " Window ")
	scr := NewSilentScreenBuf()
	scr.AllocBuf(60, 20)
	win.Show(scr)

	closeX := win.X2 - win.frame.getControlOffset()
	zoomX := closeX - 3
	closeBefore := titleCells(scr, closeX, win.Y1)
	zoomBefore := titleCells(scr, zoomX, win.Y1)
	if !win.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, MouseX: int16(closeX + 1), MouseY: int16(win.Y1), MouseEventFlags: vtinput.MouseMoved,
	}) {
		t.Fatal("mouse movement over the close control was not handled")
	}
	win.Show(scr)
	closeAfter := titleCells(scr, closeX, win.Y1)
	zoomAfter := titleCells(scr, zoomX, win.Y1)
	assertTitleHoverTint(t, "close", closeBefore, closeAfter)
	assertTitleCellsUnchanged(t, "zoom while close hovered", zoomBefore, zoomAfter)

	win.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, MouseX: int16(zoomX + 1), MouseY: int16(win.Y1), MouseEventFlags: vtinput.MouseMoved,
	})
	win.Show(scr)
	assertTitleCellsUnchanged(t, "close after leaving", closeAfter, titleCells(scr, closeX, win.Y1))
	assertTitleHoverTint(t, "zoom", zoomBefore, titleCells(scr, zoomX, win.Y1))

	win.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, MouseX: int16(win.X1), MouseY: int16(win.Y2), MouseEventFlags: vtinput.MouseMoved,
	})
	win.Show(scr)
	assertTitleCellsUnchanged(t, "zoom after leaving", zoomBefore, titleCells(scr, zoomX, win.Y1))
}

func TestTitleButtonHover_DoesNotChangeClassicOrTextAppearance(t *testing.T) {
	for _, tc := range []struct {
		name    string
		backend string
		style   GlyphStyle
	}{
		{name: "classic graphical", backend: "gogpu", style: GlyphStyleClassic},
		{name: "rounded text terminal", style: GlyphStyleRounded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			roundedButtonEnv(t, tc.backend, tc.style)
			SetDefaultPalette()
			win := NewWindow(5, 5, 45, 12, " Window ")
			scr := NewSilentScreenBuf()
			scr.AllocBuf(60, 20)
			win.Show(scr)
			x := win.X2 - win.frame.getControlOffset()
			before := titleCells(scr, x, win.Y1)
			win.ProcessMouse(&vtinput.InputEvent{
				Type: vtinput.MouseEventType, MouseX: int16(x + 1), MouseY: int16(win.Y1), MouseEventFlags: vtinput.MouseMoved,
			})
			win.Show(scr)
			assertTitleCellsUnchanged(t, "classic/text title control", before, titleCells(scr, x, win.Y1))
		})
	}
}

func TestTitleButtonHover_DoesNotChangeCloseClickBehavior(t *testing.T) {
	roundedButtonEnv(t, "gogpu", GlyphStyleRounded)
	win := NewWindow(5, 5, 45, 12, " Window ")
	x := win.X2 - win.frame.getControlOffset() + 1
	if handled := win.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, MouseX: int16(x), MouseY: int16(win.Y1),
		ButtonState: vtinput.FromLeft1stButtonPressed, KeyDown: true,
	}); !handled {
		t.Fatal("close click was not handled")
	}
	if !win.IsDone() {
		t.Fatal("close click no longer closes the window")
	}
}

func assertTitleHoverTint(t *testing.T, name string, before, after [3]CharInfo) {
	t.Helper()
	for i := range before {
		if CellString(before[i].Char) != CellString(after[i].Char) {
			t.Errorf("%s cell %d changed text from %q to %q", name, i, CellString(before[i].Char), CellString(after[i].Char))
		}
		bg := attrColorRGB(before[i].Attributes, true)
		want := mixRGB(bg, 0xFFFFFF, 0.10)
		if relativeLuminanceRGB(bg) > roundedButtonLightLuminance {
			want = mixRGB(bg, 0x000000, 0.10)
		}
		if got := attrColorRGB(after[i].Attributes, true); got != want {
			t.Errorf("%s cell %d background = #%06x, want ten-percent tint #%06x", name, i, got, want)
		}
		if contrastRatioRGB(attrColorRGB(after[i].Attributes, false), attrColorRGB(after[i].Attributes, true)) < roundedButtonMinContrast {
			t.Errorf("%s cell %d text contrast fell below %.1f:1", name, i, roundedButtonMinContrast)
		}
	}
}

func assertTitleCellsUnchanged(t *testing.T, name string, before, after [3]CharInfo) {
	t.Helper()
	for i := range before {
		if before[i] != after[i] {
			t.Errorf("%s cell %d changed: before=%+v after=%+v", name, i, before[i], after[i])
		}
	}
}
