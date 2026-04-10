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
	// 0. Global Terminal Constraint: prevent dialogs from being too wide
	if px2-px1+1 > 78 {
		errs = append(errs, LayoutError{
			Message: fmt.Sprintf("Container width %d exceeds safe terminal limit (78)", px2-px1+1),
		})
	}

	for i, item := range items {
		x1, y1, x2, y2 := item.GetPosition()
		id := item.GetId()
		if id == "" {
			id = fmt.Sprintf("Type:%T", item)
		}

		// 1. Boundary & Padding Check
		minX, minY, maxX, maxY := px1+2, py1+2, px2-2, py2-2
		if _, ok := item.(*Separator); ok {
			minX, maxX = px1, px2
		}

		if x1 < minX || y1 < minY || x2 > maxX || y2 > maxY {
			errs = append(errs, LayoutError{
				Element1: item,
				Message:  fmt.Sprintf("Element [%s] violates padding/bounds: got (%d,%d)-(%d,%d), allowed (%d,%d)-(%d,%d)", id, x1, y1, x2, y2, minX, minY, maxX, maxY),
			})
		}

		// 2. Overlap & Proximity Check
		for j := i + 1; j < len(items); j++ {
			other := items[j]
			ox1, oy1, ox2, oy2 := other.GetPosition()
			oid := other.GetId()
			if oid == "" {
				oid = fmt.Sprintf("Type:%T", other)
			}

			gapX := max(ox1-x2, x1-ox2) - 1
			gapY := max(oy1-y2, y1-oy2) - 1

			// Overlap is always an error
			if gapX < 0 && gapY < 0 {
				errs = append(errs, LayoutError{
					Element1: item, Element2: other,
					Message: fmt.Sprintf("Elements [%s] and [%s] overlap", id, oid),
				})
				continue
			}

			isDeco := func(el UIElement) bool {
				_, s := el.(*Separator); _, f := el.(*BorderedFrame); _, g := el.(*GroupBox)
				return s || f || g
			}

			// Horizontal proximity: mandatory 1 cell air for non-decorative items
			if gapX == 0 && gapY <= 0 && !isDeco(item) && !isDeco(other) {
				errs = append(errs, LayoutError{
					Element1: item, Element2: other,
					Message: fmt.Sprintf("Elements [%s] and [%s] are too close horizontally (need 1 cell air)", id, oid),
				})
			}

			// Vertical proximity: Buttons must always have air
			if gapY == 0 && gapX <= 0 {
				isBtn := func(el UIElement) bool { _, b := el.(*Button); return b }
				if (isBtn(item) || isBtn(other)) && !isDeco(item) && !isDeco(other) {
					errs = append(errs, LayoutError{
						Element1: item, Element2: other,
						Message: fmt.Sprintf("Button [%s] must have vertical air from [%s]", id, oid),
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