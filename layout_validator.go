package vtui

import (
	"fmt"
	"strings"
)

// LayoutError represents a specific UI design violation.
type LayoutError struct {
	Element1 UIElement
	Element2 UIElement // Optional, for overlap/proximity errors
	Message  string
}

func (e LayoutError) Error() string {
	return e.Message
}

// ValidateLayout checks a container for common TUI design mistakes.
func ValidateLayout(c Container) []error {
	var errs []error
	items := c.GetChildren()

	// Determine parent bounds
	px1, py1, px2, py2 := 0, 0, 0, 0
	if el, ok := c.(UIElement); ok {
		px1, py1, px2, py2 = el.GetPosition()
	}

	for i, item := range items {
		x1, y1, x2, y2 := item.GetPosition()
		id := item.GetId()
		if id == "" { id = fmt.Sprintf("Type:%T", item) }

		// 1. Boundary & Padding Check (Rule 3 & 4)
		// Standard padding for dialogs: 2 cells from outer edge (1 for border, 1 for "air")
		minX, minY, maxX, maxY := px1+2, py1+2, px2-2, py2-2
		
		// Special case: Separators can touch left/right borders
		if _, ok := item.(*Separator); ok {
			minX, maxX = px1, px2
		}

		if x1 < minX || y1 < minY || x2 > maxX || y2 > maxY {
			errs = append(errs, LayoutError{
				Element1: item,
				Message:  fmt.Sprintf("Element [%s] violates padding/bounds: got (%d,%d)-(%d,%d), allowed (%d,%d)-(%d,%d)", id, x1, y1, x2, y2, minX, minY, maxX, maxY),
			})
		}

		// 2. Overlap & Proximity Check (Rule 1 & 2)
		for j := i + 1; j < len(items); j++ {
			other := items[j]
			ox1, oy1, ox2, oy2 := other.GetPosition()
			oid := other.GetId()
			if oid == "" { oid = fmt.Sprintf("Type:%T", other) }

			// Check Overlap
			if x1 <= ox2 && x2 >= ox1 && y1 <= oy2 && y2 >= oy1 {
				errs = append(errs, LayoutError{
					Element1: item, Element2: other,
					Message: fmt.Sprintf("Elements [%s] and [%s] overlap", id, oid),
				})
			}

			// Check Proximity ("Air" rule)
			gapX := max(ox1-x2, x1-ox2) - 1
			gapY := max(oy1-y2, y1-oy2) - 1

			if gapX < 1 && gapY < 1 {
				_, isSep1 := item.(*Separator)
				_, isSep2 := other.(*Separator)
				_, isTxt1 := item.(*Text)
				_, isTxt2 := other.(*Text)

				// Decorative elements allowed to touch others
				_, isBf1 := item.(*BorderedFrame)
				_, isGb1 := item.(*GroupBox)
				isBox1 := isBf1 || isGb1

				_, isBf2 := other.(*BorderedFrame)
				_, isGb2 := other.(*GroupBox)
				isBox2 := isBf2 || isGb2

				// In TUIs, Separators and decorative Boxes are allowed to touch anything.
				// Also contiguous lines of text are allowed to stack.
				isAllowedToTouch := isSep1 || isSep2 || isBox1 || isBox2 || (isTxt1 && isTxt2 && gapY <= 0)

				if !isAllowedToTouch {
					errs = append(errs, LayoutError{
						Element1: item, Element2: other,
						Message: fmt.Sprintf("Elements [%s] and [%s] are too close (no air)", id, oid),
					})
				}
			}
		}

		// 3. Recurse into nested containers
		if sub, ok := item.(Container); ok {
			errs = append(errs, ValidateLayout(sub)...)
		}
	}

	return errs
}

// AssertLayout is a helper for tests to panic or fail if layout is invalid.
func AssertLayout(t interface{ Errorf(string, ...any) }, c Container) {
	errs := ValidateLayout(c)
	if len(errs) > 0 {
		var msgs []string
		for _, e := range errs {
			msgs = append(msgs, e.Error())
		}
		t.Errorf("Layout validation failed:\n%s", strings.Join(msgs, "\n"))
	}
}