package vtui

import (
	"testing"
)

func useAutoLayoutTestScreen(t *testing.T) {
	t.Helper()
	oldFM := FrameManager
	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)
	FrameManager = fm
	t.Cleanup(func() {
		fm.Shutdown()
		FrameManager = oldFM
	})
}

func TestAutoLayout_BasicPinAndStack(t *testing.T) {
	useAutoLayoutTestScreen(t)
	SetDefaultPalette()
	dlg := NewCenteredDialog(50, 15, " AutoLayout Test ")

	lbl := NewLabel(0, 0, "Name:", nil)
	edit := NewEdit(0, 0, 10, "John")
	btnOk := NewButton(0, 0, "&Save")
	btnCancel := NewButton(0, 0, "&Cancel")

	dlg.AddItem(lbl)
	dlg.AddItem(edit)
	dlg.AddItem(btnOk)
	dlg.AddItem(btnCancel)

	layout := NewAutoLayout(dlg.X1+2, dlg.Y1+2, 50-4, 15-4)
	layout.
		PinTop(lbl, 0).PinLeft(lbl, 0).
		StackVertical(1, lbl, edit).
		FillWidth(edit, 0, 0).
		PinBottom(btnOk, 0).PinBottom(btnCancel, 0).
		StackHorizontal(2, btnOk, btnCancel).
		CenterHorizontalGroup(btnOk, btnCancel)

	layout.Apply()

	ex1, ey1, ex2, _ := edit.GetPosition()
	if ex1 != dlg.X1+2 {
		t.Errorf("expected edit Left %d, got %d", dlg.X1+2, ex1)
	}
	if ex2-ex1+1 != 46 {
		t.Errorf("expected edit Width 46, got %d", ex2-ex1+1)
	}
	if ey1 != dlg.Y1+4 {
		t.Errorf("expected edit Top %d, got %d", dlg.Y1+4, ey1)
	}

	AssertLayout(t, dlg)
}

func TestAutoLayout_ApportionWidthsEqualColumns(t *testing.T) {
	useAutoLayoutTestScreen(t)
	SetDefaultPalette()
	dlg := NewCenteredDialog(72, 10, " Equal Columns ")

	c1 := NewEdit(0, 0, 10, "Col 1")
	c2 := NewEdit(0, 0, 10, "Col 2")
	c3 := NewEdit(0, 0, 10, "Col 3")

	dlg.AddItem(c1)
	dlg.AddItem(c2)
	dlg.AddItem(c3)

	layout := NewAutoLayout(dlg.X1+2, dlg.Y1+2, 68, 6)
	layout.
		PinTop(c1, 0).AlignTop(c1, c2, c3).
		PinLeft(c1, 0).
		StackHorizontal(0, c1, c2, c3).
		ApportionWidths(68, c1, c2, c3)

	layout.Apply()

	w1 := c1.X2 - c1.X1 + 1
	w2 := c2.X2 - c2.X1 + 1
	w3 := c3.X2 - c3.X1 + 1

	if w1+w2+w3 != 68 {
		t.Errorf("expected total width 68, got %d (%d+%d+%d)", w1+w2+w3, w1, w2, w3)
	}
	if c2.X1 != c1.X2+1 || c3.X1 != c2.X2+1 {
		t.Errorf("expected zero gap between columns: c1.X2=%d, c2.X1=%d, c2.X2=%d, c3.X1=%d", c1.X2, c2.X1, c2.X2, c3.X1)
	}
}

func TestAutoLayout_SnapToGridAndEqualize(t *testing.T) {
	useAutoLayoutTestScreen(t)
	SetDefaultPalette()
	dlg := NewCenteredDialog(60, 10, " Snapping Test ")

	b1 := NewButton(0, 0, "B1")
	b2 := NewButton(0, 0, "B2")

	dlg.AddItem(b1)
	dlg.AddItem(b2)

	layout := NewAutoLayout(dlg.X1+2, dlg.Y1+2, 56, 6)
	layout.
		PinTop(b1, 0).AlignTop(b1, b2).
		PinLeft(b1, 0).
		StackHorizontal(2, b1, b2).
		EqualizeWidthsGroup(b1, b2).
		SnapWidthToGrid(b1, 2)

	layout.Apply()

	w1 := b1.X2 - b1.X1 + 1
	w2 := b2.X2 - b2.X1 + 1

	if w1%2 != 0 {
		t.Errorf("expected b1 width snapped to even number, got %d", w1)
	}
	if w1 != w2 {
		t.Errorf("expected equalized widths, got w1=%d, w2=%d", w1, w2)
	}

	AssertLayout(t, dlg)
}

func TestAutoLayout_ResizeReSolve(t *testing.T) {
	useAutoLayoutTestScreen(t)
	SetDefaultPalette()
	dlg := NewCenteredDialog(50, 10, " Resize Test ")

	edit := NewEdit(0, 0, 10, "Resizable")
	dlg.AddItem(edit)

	layout := NewAutoLayout(dlg.X1+2, dlg.Y1+2, 46, 6)
	layout.PinTop(edit, 0).FillWidth(edit, 0, 0)
	layout.Apply()

	initW := edit.X2 - edit.X1 + 1
	if initW != 46 {
		t.Fatalf("expected initial width 46, got %d", initW)
	}

	layout.SetPosition(dlg.X1+2, dlg.Y1+2, dlg.X1+65, dlg.Y1+7)

	newW := edit.X2 - edit.X1 + 1
	if newW != 64 {
		t.Errorf("expected re-solved width 64 after resize, got %d", newW)
	}
}
