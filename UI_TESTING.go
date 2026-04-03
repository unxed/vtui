# UI Testing & Layout Validation

To maintain a consistent, professional "Desktop TUI" look and feel, `vtui` enforces automated layout validation. **Every new dialog or window added to the framework MUST have a corresponding layout validation test.**

## The Golden Rules of `vtui` Design

The `LayoutValidator` checks for three common visual bugs:

1.  **No Overlaps:** UI elements must never occupy the same cell. Even a 1-character overlap is considered a failure.
2.  **Mandatory "Air":** Interactive elements (Buttons, Edits, Checkboxes) must have at least **1 empty cell** of space between them in all directions. "Glued" elements are hard to read and navigate.
    *   *Exception:* Contiguous lines of a single text block (paragraph) are allowed to touch vertically.
3.  **Padding & Bounds:** Content must not touch the window borders. There must be at least **1 cell of padding** between any element and the frame border.

## How to Write a Layout Test

To validate a dialog, use the `vtui.AssertLayout(t, dlg)` helper.

### Boilerplate for a Dialog Test:

```go
func TestMyNewDialog_Layout(t *testing.T) {
    vtui.SetDefaultPalette()

    // 1. Setup a silent environment with fixed dimensions
    scr := vtui.NewSilentScreenBuf()
    scr.AllocBuf(80, 25)
    vtui.FrameManager.Init(scr)

    // 2. Construct your dialog
    dlg := NewMyDialog(...)

    // 3. Run the validator
    vtui.AssertLayout(t, dlg)
}
```

## Common Failure Messages & Fixes

*   **"Elements [X] and [Y] overlap"**: Your coordinates are too close. If using manual coordinates, check your `X2/Y2` calculations. If using the Layout Engine, check your margins.
*   **"Elements [X] and [Y] are too close (no air)"**: Elements are touching. Add a space or increase the margin between them.
*   **"Element [X] violates padding/bounds"**: The element is either sticking out of the window or is touching the border. Increase the Window size or move the element further from the edge.

## AI Instructions
When generating code for new dialogs, you **MUST** include a test file using `AssertLayout`. If the validation fails, you must adjust the dialog dimensions or margins until the test passes.