package vtui

import (
	"testing"
	"strings"
)

// mockT is a simple spy for testing AssertLayout without failing the real test
type mockT struct {
	failed bool
}
func (m *mockT) Errorf(format string, args ...any) { m.failed = true }

func TestLayoutValidator_Logic(t *testing.T) {
	SetDefaultPalette()

	t.Run("Overlap detection", func(t *testing.T) {
		dlg := NewDialog(0, 0, 20, 10, "Test")
		b1 := NewButton(2, 2, "B1") // x1:2, x2:7
		b2 := NewButton(6, 2, "B2") // Overlaps
		dlg.AddItem(b1)
		dlg.AddItem(b2)
		
		errs := ValidateLayout(dlg)
		foundOverlap := false
		for _, e := range errs {
			if strings.Contains(e.Error(), "overlap") { foundOverlap = true }
		}
		if !foundOverlap { t.Error("Failed to detect overlapping buttons") }
	})

	t.Run("Padding violation", func(t *testing.T) {
		dlg := NewDialog(0, 0, 20, 10, "Test")
		btn := NewButton(0, 2, "Bad") // Overlaps the left border (X=0)
		dlg.AddItem(btn)

		errs := ValidateLayout(dlg)
		if len(errs) == 0 { t.Error("Failed to detect padding violation") }
	})

	t.Run("Glued elements (horizontal air required)", func(t *testing.T) {
		dlg := NewDialog(0, 0, 30, 20, "Test")
		b1 := NewButton(2, 2, "B1") // x1:2, x2:7
		b2 := NewButton(8, 2, "B2") // x1:8, touching b1 horizontally
		dlg.AddItem(b1)
		dlg.AddItem(b2)

		errs := ValidateLayout(dlg)
		found := false
		for _, e := range errs {
			if strings.Contains(e.Error(), "horizontally") { found = true }
		}
		if !found { t.Error("Failed to detect horizontal air violation") }
	})

	t.Run("Compact TUI (vertical touch allowed for labels)", func(t *testing.T) {
		dlg := NewDialog(0, 0, 30, 20, "Test")
		l1 := NewText(2, 2, "Line 1", 0)
		l2 := NewText(2, 3, "Line 2", 0) // Touching vertically
		dlg.AddItem(l1)
		dlg.AddItem(l2)

		errs := ValidateLayout(dlg)
		if len(errs) > 0 {
			t.Errorf("Vertical touch should be allowed for non-buttons, got: %v", errs)
		}
	})

	t.Run("Button vertical air requirement", func(t *testing.T) {
		dlg := NewDialog(0, 0, 30, 20, "Test")
		l1 := NewText(2, 2, "Label", 0)
		b1 := NewButton(2, 3, "Btn") // Touching label vertically
		dlg.AddItem(l1)
		dlg.AddItem(b1)

		errs := ValidateLayout(dlg)
		found := false
		for _, e := range errs {
			if strings.Contains(e.Error(), "vertical air") { found = true }
		}
		if !found { t.Error("Failed to detect button lacking vertical air") }
	})

	t.Run("Correct layout", func(t *testing.T) {
		dlg := NewDialog(0, 0, 40, 10, "Test")
		b1 := NewButton(2, 2, "B1") // ends at 7
		b2 := NewButton(9, 2, "B2") // distance 1 (X=8 is air)
		dlg.AddItem(b1)
		dlg.AddItem(b2)

		mt := &mockT{}
		AssertLayout(mt, dlg)
		if mt.failed { t.Error("Valid layout reported as invalid") }
	})
	t.Run("Recursive nested layout", func(t *testing.T) {
		// We use GroupBox because it is a real Container that the validator recurses into.
		dlg := NewDialog(0, 0, 60, 20, "Nested")

		// gb at (2,2). Validator requires 2 cells padding from gb edge for children.
		// Allowed children area: (2+2, 2+2) = (4,4).
		gb := NewGroupBox(dlg.X1+2, dlg.Y1+2, dlg.X1+30, dlg.Y1+12, "Group")
		b1 := NewButton(0, 0, "B1")
		b2 := NewButton(0, 0, "B2")

		vbox := NewVBoxLayout(gb.X1+2, gb.Y1+2, gb.X2-gb.X1-4, gb.Y2-gb.Y1-4)
		vbox.Add(b1, Margins{}, AlignLeft)
		vbox.Add(b2, Margins{Top: 1}, AlignLeft)
		vbox.Apply()

		gb.AddItem(b1)
		gb.AddItem(b2)
		dlg.AddItem(gb)

		// edit at (35,2) is safe for dlg (padding 2 means allowed starts at 2,2)
		edit := NewEdit(dlg.X1+35, dlg.Y1+2, 20, "E1")
		dlg.AddItem(edit)

		errs := ValidateLayout(dlg)
		if len(errs) > 0 {
			t.Errorf("Valid recursive layout reported as invalid: %v", errs)
		}
	})
	t.Run("Separator touching elements", func(t *testing.T) {
		dlg := NewDialog(0, 0, 40, 10, "Separator Test")
		sep := NewSeparator(0, 4, 40, true, true)
		btn := NewButton(10, 5, "Below") // Touching separator vertically (gapY=0)
		dlg.AddItem(sep)
		dlg.AddItem(btn)

		errs := ValidateLayout(dlg)
		if len(errs) > 0 {
			t.Errorf("Separator touching elements should be allowed, but got: %v", errs)
		}
	})
	t.Run("Frame touching elements", func(t *testing.T) {
		dlg := NewDialog(0, 0, 40, 20, "Frame Test")
		// Create a GroupBox (BorderedFrame) that fills most of the dialog
		gb := NewGroupBox(2, 2, 38, 10, "Group")
		// Put a button exactly 1 cell below the frame (touching, gapY=0)
		btn := NewButton(10, 11, "Below")

		dlg.AddItem(gb)
		dlg.AddItem(btn)

		errs := ValidateLayout(dlg)
		if len(errs) > 0 {
			t.Errorf("Frame touching elements should be allowed, but got: %v", errs)
		}
	})
	t.Run("Recursive nested layout", func(t *testing.T) {
		// We use GroupBox because it is a real Container that the validator recurses into.
		dlg := NewDialog(0, 0, 60, 20, "Nested")

		// gb at (2,2). Validator requires 2 cells padding from gb edge for children.
		// Allowed children area: (2+2, 2+2) = (4,4).
		gb := NewGroupBox(dlg.X1+2, dlg.Y1+2, dlg.X1+30, dlg.Y1+12, "Group")
		b1 := NewButton(0, 0, "B1")
		b2 := NewButton(0, 0, "B2")

		vbox := NewVBoxLayout(gb.X1+2, gb.Y1+2, gb.X2-gb.X1-4, gb.Y2-gb.Y1-4)
		vbox.Add(b1, Margins{}, AlignLeft)
		vbox.Add(b2, Margins{Top: 1}, AlignLeft)
		vbox.Apply()

		gb.AddItem(b1)
		gb.AddItem(b2)
		dlg.AddItem(gb)

		// edit at (35,2) is safe for dlg (padding 2 means allowed starts at 2,2)
		edit := NewEdit(dlg.X1+35, dlg.Y1+2, 20, "E1")
		dlg.AddItem(edit)

		errs := ValidateLayout(dlg)
		if len(errs) > 0 {
			t.Errorf("Valid recursive layout reported as invalid: %v", errs)
		}
	})
}