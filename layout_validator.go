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

// LayoutRules describes how strict the layout validator is.
//
// "Clearance" is the number of completely empty cells that must remain
// between the border of a container and any element inside it. A clearance
// of 0 means an element may sit right next to the border (but never on it),
// a clearance of 1 means at least one empty cell of air is required.
type LayoutRules struct {
	// FrameClearanceX/FrameClearanceY apply to windows and dialogs.
	// The dialog border must never be touched, so both default to 1.
	FrameClearanceX int
	FrameClearanceY int

	// GroupClearanceX/GroupClearanceY apply to nested bordered containers
	// (GroupBox, BorderedFrame). Vertically a group box is usually only a
	// few rows tall, so the default there is 0: content may live directly
	// under the top border, but still never on it.
	GroupClearanceX int
	GroupClearanceY int

	// MaxWidth is the widest container we consider safe on an 80 column
	// terminal. Set to 0 to disable the check.
	MaxWidth int

	// CheckContentWidth enables detection of widgets that paint more columns
	// than their own box is wide. This is the typical failure mode after a
	// caption has been translated into a longer language.
	CheckContentWidth bool
}

// DefaultLayoutRules is used by ValidateLayout and AssertLayout.
var DefaultLayoutRules = LayoutRules{
	FrameClearanceX:   1,
	FrameClearanceY:   1,
	GroupClearanceX:   1,
	GroupClearanceY:   0,
	MaxWidth:          78,
	CheckContentWidth: true,
}

// ContentSizer may be implemented by a widget to tell the layout validator
// how many screen columns it really paints for its current content.
// Widgets that do not implement it are measured by the validator itself.
type ContentSizer interface {
	ContentWidth() int
}

// ContentClipper may be implemented by a widget that truncates its own
// content to its bounds instead of painting outside of them.
type ContentClipper interface {
	ClipsContent() bool
}

// ValidateLayout checks a container for common TUI design mistakes.
func ValidateLayout(c Container) []error {
	return ValidateLayoutWithRules(c, DefaultLayoutRules)
}

// ValidateLayoutWithRules is ValidateLayout with a custom rule set.
func ValidateLayoutWithRules(c Container, rules LayoutRules) []error {
	if c == nil {
		return nil
	}
	var errs []error
	items := c.GetChildren()

	// Layout containers are invisible geometry helpers, not paintable
	// widgets: they cannot overlap anything and need no air or clearance.
	// Their children are window items in their own right and are validated
	// directly.
	filtered := make([]UIElement, 0, len(items))
	for _, it := range items {
		switch it.(type) {
		case *VBoxLayout, *HBoxLayout, *AutoLayout:
			continue
		}
		filtered = append(filtered, it)
	}
	items = filtered

	// Determine parent bounds
	px1, py1, px2, py2 := 0, 0, 0, 0
	hasBounds := false
	if el, ok := c.(UIElement); ok {
		px1, py1, px2, py2 = el.GetPosition()
		hasBounds = true
	}

	// 0. Global Terminal Constraint: prevent dialogs from being too wide
	if hasBounds && rules.MaxWidth > 0 && px2-px1+1 > rules.MaxWidth {
		errs = append(errs, LayoutError{
			Message: fmt.Sprintf("Container width %d exceeds safe terminal limit (%d)", px2-px1+1, rules.MaxWidth),
		})
	}

	// Border thickness of this container and the required air inside it.
	border, clearX, clearY := containerClearance(c, rules)

	// First cell that is not part of the border itself.
	hardMinX, hardMaxX := px1+border, px2-border
	hardMinY, hardMaxY := py1+border, py2-border
	// First cell that also satisfies the required clearance.
	softMinX, softMaxX := hardMinX+clearX, hardMaxX-clearX
	softMinY, softMaxY := hardMinY+clearY, hardMaxY-clearY

	for i, item := range items {
		x1, y1, x2, y2 := item.GetPosition()
		id := elementID(item)

		// Measure what the widget really paints. For widgets that grow with
		// their caption this may be wider than the box they were laid out in.
		ex2 := paintedRight(item, rules)
		if rules.CheckContentWidth {
			if contentW, clips, known := contentMetrics(item); known {
				boxW := x2 - x1 + 1
				if boxW > 0 && contentW > boxW {
					if clips {
						// The widget truncates itself: nothing spills over the
						// frame, but the user loses part of the caption.
						errs = append(errs, LayoutError{
							Element1: item,
							Message: fmt.Sprintf("Element [%s] content is clipped: needs %d columns, box is %d wide (check translations)",
								id, contentW, boxW),
						})
					} else {
						errs = append(errs, LayoutError{
							Element1: item,
							Message: fmt.Sprintf("Element [%s] paints %d column(s) outside its own box: needs %d columns, box is %d wide (check translations)",
								id, ex2-x2, contentW, boxW),
						})
					}
				}
			}
		}

		// 1. Frame checks. Separators are allowed to join the border
		// horizontally, everything else must keep clear of it.
		if hasBounds {
			iHardMinX, iHardMaxX := hardMinX, hardMaxX
			iSoftMinX, iSoftMaxX := softMinX, softMaxX
			if _, isSep := item.(*Separator); isSep {
				iHardMinX, iHardMaxX = px1, px2
				iSoftMinX, iSoftMaxX = px1, px2
			}

			switch {
			case x1 < px1 || ex2 > px2 || y1 < py1 || y2 > py2:
				errs = append(errs, LayoutError{
					Element1: item,
					Message: fmt.Sprintf("Element [%s] sticks out of the container: got (%d,%d)-(%d,%d), container is (%d,%d)-(%d,%d)",
						id, x1, y1, ex2, y2, px1, py1, px2, py2),
				})
			case border > 0 && (x1 < iHardMinX || ex2 > iHardMaxX || y1 < hardMinY || y2 > hardMaxY):
				errs = append(errs, LayoutError{
					Element1: item,
					Message: fmt.Sprintf("Element [%s] is drawn on the frame border: got (%d,%d)-(%d,%d), border is at (%d,%d)-(%d,%d)",
						id, x1, y1, ex2, y2, px1, py1, px2, py2),
				})
			case x1 < iSoftMinX || ex2 > iSoftMaxX || y1 < softMinY || y2 > softMaxY:
				errs = append(errs, LayoutError{
					Element1: item,
					Message: fmt.Sprintf("Element [%s] touches the frame border: got (%d,%d)-(%d,%d), allowed (%d,%d)-(%d,%d)",
						id, x1, y1, ex2, y2, iSoftMinX, softMinY, iSoftMaxX, softMaxY),
				})
			}
		}

		// 2. Overlap & Proximity Check
		for j := i + 1; j < len(items); j++ {
			other := items[j]
			ox1, oy1, _, oy2 := other.GetPosition()
			ox2 := paintedRight(other, rules)
			oid := elementID(other)

			gapX := max(ox1-ex2, x1-ox2) - 1
			gapY := max(oy1-y2, y1-oy2) - 1

			// Overlap is always an error
			if gapX < 0 && gapY < 0 {
				errs = append(errs, LayoutError{
					Element1: item, Element2: other,
					Message: fmt.Sprintf("Elements [%s] and [%s] overlap", id, oid),
				})
				continue
			}

			// Vertical proximity: Buttons must always have air
			if gapY == 0 && gapX <= 0 {
				isBtn := func(el UIElement) bool { _, b := el.(*Button); return b }
				if (isBtn(item) || isBtn(other)) && !isDecorative(item) && !isDecorative(other) {
					errs = append(errs, LayoutError{
						Element1: item, Element2: other,
						Message: fmt.Sprintf("Button [%s] must have vertical air from [%s]", id, oid),
					})
				}
			}
		}

		// 3. Recurse into nested containers
		if sub, ok := item.(Container); ok {
			errs = append(errs, ValidateLayoutWithRules(sub, rules)...)
		}
	}

	return errs
}

// paintedRight returns the rightmost column an element actually writes to.
// For most widgets this is simply X2, but widgets that render a caption
// without clipping it (buttons, checkboxes, radio/check groups) may paint
// further to the right once the caption has been translated.
func paintedRight(el UIElement, rules LayoutRules) int {
	x1, _, x2, _ := el.GetPosition()
	if !rules.CheckContentWidth {
		return x2
	}
	contentW, clips, known := contentMetrics(el)
	if !known || clips || contentW <= 0 {
		return x2
	}
	if e := x1 + contentW - 1; e > x2 {
		return e
	}
	return x2
}

// elementID produces a readable name for error messages.
func elementID(el UIElement) string {
	if id := el.GetId(); id != "" {
		return id
	}
	if s, ok := el.(interface{ GetText() string }); ok {
		if txt := strings.TrimSpace(s.GetText()); txt != "" {
			return fmt.Sprintf("%T %q", el, TruncateString(txt, 24, "..."))
		}
	}
	return fmt.Sprintf("%T:%p", el, el)
}

func isDecorative(el UIElement) bool {
	switch el.(type) {
	case *Separator, *BorderedFrame, *GroupBox:
		return true
	}
	return false
}

// BorderedContainer is implemented by containers that paint a border on their
// own bounds. The layout validator uses it to tell how many cells of the
// container are taken by the frame itself. Containers that do not implement it
// are treated as borderless, so their bounds are the content area.
//
// BaseWindow, GroupBox, BorderedFrame and Group implement it out of the box,
// which also covers every application type embedding them.
type BorderedContainer interface {
	GetBorderThickness() int
}

// containerClearance reports the border thickness of a container and the
// amount of empty space that must remain between that border and its children.
func containerClearance(c Container, rules LayoutRules) (border, clearX, clearY int) {
	bc, ok := c.(BorderedContainer)
	if !ok {
		return 0, 0, 0
	}
	border = bc.GetBorderThickness()
	if border <= 0 {
		return 0, 0, 0
	}
	if _, isGroupBox := c.(*GroupBox); isGroupBox {
		return border, rules.GroupClearanceX, rules.GroupClearanceY
	}
	return border, rules.FrameClearanceX, rules.FrameClearanceY
}

// contentMetrics reports how many columns an element needs to paint its
// current content, and whether it clips itself to its own bounds.
// known is false when the validator cannot measure the widget.
func contentMetrics(el UIElement) (width int, clips bool, known bool) {
	if cs, ok := el.(ContentSizer); ok {
		clip := false
		if cc, ok := el.(ContentClipper); ok {
			clip = cc.ClipsContent()
		}
		return cs.ContentWidth(), clip, true
	}

	switch v := el.(type) {
	case *Button:
		// cleanText already contains the decorating brackets.
		return StringWidth(v.cleanText), false, true
	case *Checkbox:
		// The "[x] " prefix is 4 columns wide.
		return 4 + StringWidth(v.cleanText), false, true
	case *RadioGroup:
		return gridContentWidth(v.Columns, v.Items, v.colWidths), false, true
	case *CheckGroup:
		return gridContentWidth(v.Columns, v.Items, v.colWidths), false, true
	case *Text:
		// Text truncates itself in DisplayObject.
		return StringWidth(v.cleanText), true, true
	}
	return 0, false, false
}

// gridContentWidth returns how many columns a grid-based group
// (RadioGroup / CheckGroup) paints, counted from its X1.
func gridContentWidth(cols int, items []string, colWidths []int) int {
	if cols < 1 {
		cols = 1
	}
	if len(colWidths) != cols {
		colWidths = calcGridColWidths(cols, items)
	}
	widest := 0
	for i, itm := range items {
		col := i % cols
		offset := 0
		for c := 0; c < col && c < len(colWidths); c++ {
			offset += colWidths[c]
		}
		clean, _, _ := ParseAmpersandString(itm)
		// 4 columns for the "( ) " / "[ ] " prefix.
		if w := offset + 4 + StringWidth(clean); w > widest {
			widest = w
		}
	}
	return widest
}

// LanguagePack is a named set of localized strings, used to validate a
// layout against every translation the application ships with.
type LanguagePack struct {
	Name    string
	Strings map[string]string
}

// ValidateLayoutInLanguages rebuilds the UI once per language pack and
// validates the result each time. Captions differ in length between
// languages, so a dialog that fits in English may well overflow elsewhere.
//
// build must construct a fresh container every time it is called; it is
// invoked after the localization table has been switched.
func ValidateLayoutInLanguages(packs []LanguagePack, build func() Container) []error {
	return ValidateLayoutInLanguagesWithRules(packs, DefaultLayoutRules, build)
}

// ValidateLayoutInLanguagesWithRules is ValidateLayoutInLanguages with a
// custom rule set.
func ValidateLayoutInLanguagesWithRules(packs []LanguagePack, rules LayoutRules, build func() Container) []error {
	if build == nil {
		return nil
	}
	// The currently loaded table is the base every pack is overlaid onto,
	// mirroring how the application loads a translation on top of English.
	base := SnapshotStrings()
	defer ReplaceStrings(base)

	if len(packs) == 0 {
		packs = []LanguagePack{{Name: "current"}}
	}

	var errs []error
	for _, pack := range packs {
		ReplaceStrings(base)
		if len(pack.Strings) > 0 {
			AddStrings(pack.Strings)
		}

		c := build()
		if c == nil {
			continue
		}

		name := pack.Name
		if name == "" {
			name = "?"
		}
		for _, e := range ValidateLayoutWithRules(c, rules) {
			le, ok := e.(LayoutError)
			if !ok {
				errs = append(errs, LayoutError{Message: fmt.Sprintf("[lang:%s] %s", name, e.Error())})
				continue
			}
			le.Message = fmt.Sprintf("[lang:%s] %s", name, le.Message)
			errs = append(errs, le)
		}
	}
	return errs
}

// AssertLayout is a helper for tests to panic or fail if layout is invalid.
func AssertLayout(t interface{ Errorf(string, ...any) }, c Container) {
	AssertLayoutWithRules(t, c, DefaultLayoutRules)
}

// AssertLayoutWithRules is AssertLayout with a custom rule set.
func AssertLayoutWithRules(t interface{ Errorf(string, ...any) }, c Container, rules LayoutRules) {
	reportLayoutErrors(t, ValidateLayoutWithRules(c, rules))
}

// AssertLayoutInLanguages fails the test if the layout breaks in any of the
// supplied languages.
func AssertLayoutInLanguages(t interface{ Errorf(string, ...any) }, packs []LanguagePack, build func() Container) {
	reportLayoutErrors(t, ValidateLayoutInLanguages(packs, build))
}

func reportLayoutErrors(t interface{ Errorf(string, ...any) }, errs []error) {
	if len(errs) == 0 {
		return
	}
	var msgs []string
	for _, e := range errs {
		msgs = append(msgs, e.Error())
	}
	t.Errorf("Layout validation failed:\n%s", strings.Join(msgs, "\n"))
}
