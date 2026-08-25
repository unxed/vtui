package vtui

import (
	"strings"
	"testing"
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
			if strings.Contains(e.Error(), "overlap") {
				foundOverlap = true
			}
		}
		if !foundOverlap {
			t.Error("Failed to detect overlapping buttons")
		}
	})

	t.Run("Padding violation", func(t *testing.T) {
		dlg := NewDialog(0, 0, 20, 10, "Test")
		btn := NewButton(0, 2, "Bad") // Overlaps the left border (X=0)
		dlg.AddItem(btn)

		errs := ValidateLayout(dlg)
		if len(errs) == 0 {
			t.Error("Failed to detect padding violation")
		}
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
			if strings.Contains(e.Error(), "vertical air") {
				found = true
			}
		}
		if !found {
			t.Error("Failed to detect button lacking vertical air")
		}
	})

	t.Run("Correct layout", func(t *testing.T) {
		dlg := NewDialog(0, 0, 40, 10, "Test")
		b1 := NewButton(2, 2, "B1") // ends at 7
		b2 := NewButton(9, 2, "B2") // distance 1 (X=8 is air)
		dlg.AddItem(b1)
		dlg.AddItem(b2)

		mt := &mockT{}
		AssertLayout(mt, dlg)
		if mt.failed {
			t.Error("Valid layout reported as invalid")
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
func TestLayoutValidator_FrameClearance(t *testing.T) {
	SetDefaultPalette()

	// Dialog 0..20 x 0..10, border on x=0/x=20 and y=0/y=10,
	// so the allowed area for children is (2,2)-(18,8).
	newDlg := func() *Window { return NewDialog(0, 0, 20, 10, "Test") }

	hasMsg := func(errs []error, substr string) bool {
		for _, e := range errs {
			if strings.Contains(e.Error(), substr) {
				return true
			}
		}
		return false
	}

	t.Run("Button touching the right border", func(t *testing.T) {
		dlg := newDlg()
		// "[ Test ]" is 8 columns: x1=12 -> x2=19, i.e. glued to the border.
		dlg.AddItem(NewButton(12, 2, "Test"))

		errs := ValidateLayout(dlg)
		if !hasMsg(errs, "touches the frame border") {
			t.Errorf("Failed to detect a control glued to the right border, got: %v", errs)
		}
	})

	t.Run("Button drawn on the right border", func(t *testing.T) {
		dlg := newDlg()
		dlg.AddItem(NewButton(13, 2, "Test")) // x2 == 20 == border column

		errs := ValidateLayout(dlg)
		if !hasMsg(errs, "on the frame border") {
			t.Errorf("Failed to detect a control sitting on the border, got: %v", errs)
		}
	})

	t.Run("Button crossing the right border", func(t *testing.T) {
		dlg := newDlg()
		dlg.AddItem(NewButton(15, 2, "Test")) // x2 == 22, past the border

		errs := ValidateLayout(dlg)
		if !hasMsg(errs, "sticks out of the container") {
			t.Errorf("Failed to detect a control crossing the border, got: %v", errs)
		}
	})

	t.Run("Button touching the bottom border", func(t *testing.T) {
		dlg := newDlg()
		dlg.AddItem(NewButton(2, 9, "Ok")) // y == 9, border is at y == 10

		errs := ValidateLayout(dlg)
		if !hasMsg(errs, "touches the frame border") {
			t.Errorf("Failed to detect a control glued to the bottom border, got: %v", errs)
		}
	})

	t.Run("Button touching the left border", func(t *testing.T) {
		dlg := newDlg()
		dlg.AddItem(NewButton(1, 2, "Ok"))

		errs := ValidateLayout(dlg)
		if !hasMsg(errs, "touches the frame border") {
			t.Errorf("Failed to detect a control glued to the left border, got: %v", errs)
		}
	})

	t.Run("Properly padded dialog stays valid", func(t *testing.T) {
		dlg := newDlg()
		dlg.AddItem(NewButton(2, 2, "Ok")) // "[ Ok ]" -> (2,2)-(7,2)

		if errs := ValidateLayout(dlg); len(errs) > 0 {
			t.Errorf("Correctly padded dialog reported as invalid: %v", errs)
		}
	})

	t.Run("GroupBox allows content right under its top border", func(t *testing.T) {
		dlg := NewDialog(0, 0, 40, 20, "Test")
		gb := NewGroupBox(2, 2, 30, 6, "Group")
		cb := NewCheckbox(4, 3, "Flag", false) // directly under the group border
		gb.AddItem(cb)
		dlg.AddItem(gb)

		if errs := ValidateLayout(dlg); len(errs) > 0 {
			t.Errorf("Compact group box reported as invalid: %v", errs)
		}
	})
}

func TestLayoutValidator_ContentOverflow(t *testing.T) {
	SetDefaultPalette()

	hasMsg := func(errs []error, substr string) bool {
		for _, e := range errs {
			if strings.Contains(e.Error(), substr) {
				return true
			}
		}
		return false
	}

	t.Run("Button caption grown after layout", func(t *testing.T) {
		dlg := NewDialog(0, 0, 16, 10, "Test")
		btn := NewButton(2, 2, "Ok")
		// A longer translation assigned later: SetText does not resize the box,
		// so the button paints past its own bounds and over the frame.
		btn.SetText("Alle ersetzen")
		dlg.AddItem(btn)

		errs := ValidateLayout(dlg)
		if !hasMsg(errs, "outside its own box") {
			t.Errorf("Failed to detect caption overflowing its own box, got: %v", errs)
		}
		if !hasMsg(errs, "sticks out of the container") {
			t.Errorf("Failed to detect caption overflowing the frame, got: %v", errs)
		}
	})

	t.Run("Checkbox caption grown after layout", func(t *testing.T) {
		dlg := NewDialog(0, 0, 20, 10, "Test")
		cb := NewCheckbox(2, 2, "On", false)
		cb.SetText("Unterverzeichnisse einschliessen")
		dlg.AddItem(cb)

		if errs := ValidateLayout(dlg); !hasMsg(errs, "outside its own box") {
			t.Errorf("Failed to detect checkbox caption overflow, got: %v", errs)
		}
	})

	t.Run("Clipped label is reported", func(t *testing.T) {
		dlg := NewDialog(0, 0, 30, 10, "Test")
		txt := NewText(2, 2, "Short", Palette[ColDialogText])
		txt.SetText("A much longer localized label")
		dlg.AddItem(txt)

		if errs := ValidateLayout(dlg); !hasMsg(errs, "content is clipped") {
			t.Errorf("Failed to detect a clipped label, got: %v", errs)
		}
	})

	t.Run("No false positive for a fitting caption", func(t *testing.T) {
		dlg := NewDialog(0, 0, 30, 10, "Test")
		dlg.AddItem(NewButton(2, 2, "Ok"))
		dlg.AddItem(NewCheckbox(2, 4, "Flag", false))

		if errs := ValidateLayout(dlg); len(errs) > 0 {
			t.Errorf("Fitting captions reported as invalid: %v", errs)
		}
	})
}

func TestLayoutValidator_AllLanguages(t *testing.T) {
	SetDefaultPalette()

	packs := []LanguagePack{
		{Name: "en", Strings: map[string]string{"test.Overwrite": "&Overwrite"}},
		{Name: "de", Strings: map[string]string{"test.Overwrite": "&Alle vorhandenen Dateien ersetzen"}},
	}

	build := func() Container {
		dlg := NewDialog(0, 0, 30, 10, "Test")
		dlg.AddItem(NewButton(2, 2, Msg("test.Overwrite")))
		return dlg
	}

	errs := ValidateLayoutInLanguages(packs, build)
	if len(errs) == 0 {
		t.Fatal("Failed to detect a layout that only breaks in a non-default language")
	}

	sawDE := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "[lang:de]") {
			sawDE = true
		}
		if strings.Contains(e.Error(), "[lang:en]") {
			t.Errorf("English layout should be valid, got: %v", e)
		}
	}
	if !sawDE {
		t.Errorf("Expected the German layout to be reported, got: %v", errs)
	}

	// The localization table must be restored after validation.
	if Msg("test.Overwrite") == "&Alle vorhandenen Dateien ersetzen" {
		t.Error("ValidateLayoutInLanguages leaked the last language pack into the global table")
	}
}
