package vtui

import (
	"github.com/mattn/go-runewidth"
	"unicode"

	"github.com/unxed/vtinput"
)

// MenuItem represents a single menu item.
type MenuItem struct {
	// AccentPrefix is drawn immediately before Text using the menu highlight
	// color. It is useful for non-hotkey metadata such as stable item numbers.
	AccentPrefix string
	Text         string
	Shortcut     string // Optional right-aligned hotkey hint (e.g. "F3")
	Command      int    // TV-style Command ID to emit when selected
	OnClick      func() // Closure called when selected
	UserData     any
	Separator    bool
	// SubItems turns the item into a nested menu: selecting it opens a
	// second VMenu beside this one instead of firing an action. An item
	// with SubItems is a heading, so its Command and OnClick are never
	// used, and it carries the submenu marker where a Shortcut would go.
	SubItems []MenuItem
}

// VMenu implements a vertical menu with navigation support.
type VMenu struct {
	ScrollView
	title    string
	Items    []MenuItem
	done     bool
	exitCode int
	// selectAtOpen is SelectPos as of the last ClearDone. Browsing moves
	// SelectPos live (arrows, mouse hover), so cancelling has to put it
	// back: dialogs read SelectPos as the confirmed choice, and without the
	// restore an Esc'd dropdown silently commits whatever row the user
	// happened to stop on.
	selectAtOpen   int
	OnAction       func(int)
	OnKeyDown      func(*vtinput.InputEvent) bool
	HideShadow     bool
	mouseSelecting bool
	BoxType        int

	// parentMenu and activeSub link a chain of nested menus. Only the
	// deepest one is on top of the frame stack and sees input, so closing
	// or confirming has to walk the chain explicitly: a submenu left
	// behind would keep painting over the screen with nothing to close it.
	parentMenu *VMenu
	activeSub  *VMenu

	// Palette entries the menu paints with. They default to the Menu.* group;
	// a ComboBox points them at Dialog.Combo.* so its dropdown stands apart
	// from the dialog underneath it.
	ColorTextIdx              int
	ColorSelectedTextIdx      int
	ColorHighlightIdx         int
	ColorSelectedHighlightIdx int
	ColorBoxIdx               int
	ColorTitleIdx             int
}

// NewVMenu creates a new vertical menu instance.
func NewVMenu(title string) *VMenu {
	clean, _, _ := ParseAmpersandString(title)
	m := &VMenu{
		title:                     clean,
		Items:                     []MenuItem{},
		ColorTextIdx:              ColMenuText,
		ColorSelectedTextIdx:      ColMenuSelectedText,
		ColorHighlightIdx:         ColMenuHighlight,
		ColorSelectedHighlightIdx: ColMenuSelectedHighlight,
		ColorBoxIdx:               ColMenuBox,
		ColorTitleIdx:             ColMenuTitle,
		BoxType:                   DoubleBox,
	}
	m.canFocus = true
	m.Wrap = true
	m.WheelArea = WheelAreaMenu
	m.IsSelectable = func(i int) bool {
		return i >= 0 && i < len(m.Items) && !m.Items[i].Separator
	}
	m.ShowScrollBar = true
	m.MarginTop = 1
	m.MarginBottom = 1
	m.InitScrollBar(m)
	return m
}

// AddItem adds a new item to the menu.
func (m *VMenu) AddItem(item MenuItem) {
	m.Items = append(m.Items, item)
	m.ItemCount = len(m.Items)
	if len(m.Items) == 1 {
		if m.Items[0].Separator {
			m.SelectPos = 0
		} else {
			m.SetSelectPos(0)
		}
	}
}

// AddSeparator adds a separator line.
func (m *VMenu) AddSeparator() {
	m.Items = append(m.Items, MenuItem{Separator: true})
	m.ItemCount = len(m.Items)
}

func (m *VMenu) GetItemCount() int { return len(m.Items) }

// menuItemHint returns the text drawn right-aligned on a menu row: the
// shortcut, or the marker that says the row opens a nested menu.
func menuItemHint(item MenuItem) string {
	if item.Shortcut != "" {
		return item.Shortcut
	}
	if len(item.SubItems) > 0 {
		return SubMenuMarker
	}
	return ""
}

// menuItemsWidth returns the box width the items need: the widest row plus
// its hint column, never below minWidth.
func menuItemsWidth(items []MenuItem, minWidth int) int {
	width := minWidth
	for _, item := range items {
		if item.Separator {
			continue
		}
		clean, _, _ := ParseAmpersandString(" " + item.Text)
		w := StringWidth(clean)
		if hint := menuItemHint(item); hint != "" {
			w += StringWidth(hint + " ")
		}
		w += 4 // Minimum visual padding between text and shortcut/border
		if w > width {
			width = w
		}
	}
	return width
}

// HasSubMenu reports whether the item at index opens a nested menu.
func (m *VMenu) HasSubMenu(index int) bool {
	return index >= 0 && index < len(m.Items) && len(m.Items[index].SubItems) > 0
}

// OpenSubMenu drops the nested menu of the item at index next to its row,
// to the right of this menu or -- when the screen edge is in the way -- to
// its left. It reports whether a menu was opened.
func (m *VMenu) OpenSubMenu(index int) bool {
	if !m.HasSubMenu(index) || FrameManager == nil {
		return false
	}
	m.CloseSubMenu()

	item := m.Items[index]
	sub := NewVMenu(item.Text)
	sub.SetOwner(m)
	sub.parentMenu = m
	sub.HideShadow = m.HideShadow
	sub.BoxType = m.BoxType
	sub.ColorTextIdx = m.ColorTextIdx
	sub.ColorSelectedTextIdx = m.ColorSelectedTextIdx
	sub.ColorHighlightIdx = m.ColorHighlightIdx
	sub.ColorSelectedHighlightIdx = m.ColorSelectedHighlightIdx
	sub.ColorBoxIdx = m.ColorBoxIdx
	sub.ColorTitleIdx = m.ColorTitleIdx
	for _, nested := range item.SubItems {
		if nested.Separator {
			sub.AddSeparator()
		} else {
			sub.AddItem(nested)
		}
	}
	// Picking a row in the nested menu is picking a row in this one, so the
	// listener that closes the whole construction still hears about it.
	sub.OnAction = func(int) {
		if m.OnAction != nil {
			m.OnAction(m.SelectPos)
		}
	}

	screenW, screenH := FrameManager.GetScreenSize(), FrameManager.GetScreenHeight()
	width := menuItemsWidth(item.SubItems, 20)
	// The nested box shares the border column of the parent rather than
	// standing a gap away from it.
	x1 := m.X2
	if x1+width-1 > screenW-1 {
		x1 = m.X1 - width + 1
	}
	if x1 < 0 {
		x1 = 0
	}
	x2 := x1 + width - 1
	if x2 > screenW-1 {
		x2 = screenW - 1
	}

	// The first nested row lines up with the row that opened it.
	y1 := m.Y1 + m.MarginTop + (index - m.TopPos) - 1
	y2 := y1 + sub.GetItemCount() + 1
	if y2 > screenH-1 {
		y1 -= y2 - (screenH - 1)
		y2 = screenH - 1
	}
	if y1 < 0 {
		y1 = 0
	}
	if y2 > screenH-1 {
		y2 = screenH - 1
	}
	if y2 < y1+2 {
		y2 = y1 + 2
	}
	sub.SetPosition(x1, y1, x2, y2)

	m.activeSub = sub
	FrameManager.Push(sub)
	return true
}

// CloseSubMenu closes the nested menu opened from this one, deepest first.
func (m *VMenu) CloseSubMenu() {
	sub := m.activeSub
	if sub == nil {
		return
	}
	m.activeSub = nil
	sub.CloseSubMenu()
	sub.done = true
	sub.exitCode = -1
	if FrameManager != nil {
		FrameManager.RemoveFrame(sub)
	}
}

// closeAncestors dismisses the menus this one was opened from. A nested
// menu that finishes -- an item chosen, F10, a click outside -- ends the
// whole construction, and the parents are not on top to notice it
// themselves.
func (m *VMenu) closeAncestors() {
	for parent := m.parentMenu; parent != nil; parent = parent.parentMenu {
		parent.activeSub = nil
		parent.done = true
		parent.exitCode = -1
		if FrameManager != nil {
			FrameManager.RemoveFrame(parent)
		}
	}
}

// ProcessKey processes navigation keys.
func (m *VMenu) ProcessKey(e *vtinput.InputEvent) bool {
	if m.IsDisabled() || !e.KeyDown {
		return false
	}

	if m.OnKeyDown != nil && m.OnKeyDown(e) {
		return true
	}

	isSubMenu := false
	if m.owner != nil {
		_, isSubMenu = m.owner.(*MenuBar)
	}

	switch e.VirtualKeyCode {
	case vtinput.VK_LEFT:
		if m.parentMenu != nil {
			// One level back, not out of the menu bar entirely.
			m.parentMenu.CloseSubMenu()
			return true
		}
		if isSubMenu {
			FrameManager.EmitCommand(CmMenuLeft, nil)
			return true
		}
		return false // Boundary exit
	case vtinput.VK_RIGHT:
		if m.HasSubMenu(m.SelectPos) {
			m.OpenSubMenu(m.SelectPos)
			return true
		}
		if isSubMenu {
			FrameManager.EmitCommand(CmMenuRight, nil)
			return true
		}
		if m.parentMenu != nil {
			// Inside a nested menu Right is the open gesture; with nothing
			// to open it must not walk the selection sideways.
			return true
		}
		// If last item in standalone menu, let focus cycle (unless wrapping is on)
		if m.SelectPos == m.ItemCount-1 && !m.Wrap {
			return false
		}
		return m.HandleKey(e)
	case vtinput.VK_UP:
		if m.SelectPos == 0 && !isSubMenu && !m.Wrap {
			return false
		}
		return m.HandleKey(e)
	case vtinput.VK_DOWN:
		if m.SelectPos == m.ItemCount-1 && !isSubMenu && !m.Wrap {
			return false
		}
		return m.HandleKey(e)
	// PgUp/PgDn fall through to HandleKey like Home/End do: HandleNavKey
	// pages via PageBy, which clamps at the list ends even though Wrap is on.
	case vtinput.VK_ESCAPE, vtinput.VK_F10:
		if m.parentMenu != nil && e.VirtualKeyCode == vtinput.VK_ESCAPE {
			m.parentMenu.CloseSubMenu()
			return true
		}
		m.SetExitCode(-1)
		return FrameManager.GetTopFrame() == Frame(m)
	case vtinput.VK_RETURN:
		if m.HasSubMenu(m.SelectPos) {
			m.OpenSubMenu(m.SelectPos)
			return true
		}
		if m.SelectPos >= 0 && m.SelectPos < m.ItemCount {
			// Virtual consumers size the menu via ItemCount without backing
			// Items; such rows carry no command to fire, but the selection is
			// still confirmed through OnAction and the exit code.
			if m.SelectPos < len(m.Items) {
				item := m.Items[m.SelectPos]
				if item.Separator {
					return true
				}
				if FrameManager.DisabledCommands.IsDisabled(item.Command) {
					return true
				}

				// 1. Fire the actual action (bubbles through owner)
				oldCmd := m.Command
				m.Command = item.Command
				m.FireAction(item.OnClick, item.UserData)
				m.Command = oldCmd
			}

			// 2. Notify listener (may close the menu)
			if m.OnAction != nil {
				m.OnAction(m.SelectPos)
			}

			m.SetExitCode(m.SelectPos)
			return true
		}
		return true
	}

	if e.Char != 0 {
		charLower := unicode.ToLower(e.Char)
		xlatLower := unicode.ToLower(GlobalXlator.Translate(e.Char))
		for i, item := range m.Items {
			if item.Separator {
				continue
			}
			hk := ExtractHotkey(item.Text)
			if hk != 0 && (hk == charLower || hk == xlatLower) {
				if FrameManager.DisabledCommands.IsDisabled(item.Command) {
					return true
				}
				m.SetSelectPos(i)
				if m.HasSubMenu(i) {
					m.OpenSubMenu(i)
					return true
				}

				oldCmd := m.Command
				m.Command = item.Command
				m.FireAction(item.OnClick, item.UserData)
				m.Command = oldCmd

				if m.OnAction != nil {
					m.OnAction(i)
				}

				m.SetExitCode(i)
				return true
			}
		}
	}

	return m.HandleKey(e)
}

func (m *VMenu) ResizeConsole(w, h int) {
	// For standalone VMenus, we might want to keep them centered
}
func (m *VMenu) GetTitle() string {
	return m.title
}
func (m *VMenu) GetProgress() int {
	return -1
}

func (m *VMenu) GetType() FrameType {
	return TypeMenu
}

func (m *VMenu) SetExitCode(code int) {
	m.mouseSelecting = false
	m.CloseSubMenu()
	m.closeAncestors()
	m.done = true
	m.exitCode = code
	if code == -1 {
		// Cancelled: undo the browsing highlight (see selectAtOpen).
		m.SetSelectPos(m.selectAtOpen)
		FrameManager.EmitCommand(CmMenuClose, nil)
	}
}

func (m *VMenu) IsDone() bool {
	return m.done
}
func (m *VMenu) IsBusy() bool          { return false }
func (m *VMenu) IsModal() bool         { return true }
func (m *VMenu) GetWindowNumber() int  { return 0 }
func (m *VMenu) SetWindowNumber(n int) {}
func (m *VMenu) RequestFocus() bool    { return true }
func (m *VMenu) Close()                { m.SetExitCode(-1) }
func (m *VMenu) HasShadow() bool       { return !m.HideShadow }

// ClearDone resets the menu state, allowing it to be shown again.
func (m *VMenu) ClearDone() {
	m.mouseSelecting = false
	m.done = false
	m.exitCode = -1
	m.selectAtOpen = m.SelectPos
}

// BeginMouseSelection transfers the opening press to the popup.
func (m *VMenu) BeginMouseSelection() { m.mouseSelecting = true }

// ProcessMouse handles mouse wheel scrolling, menu item hover, and clicks.
func (m *VMenu) ProcessMouse(e *vtinput.InputEvent) bool {
	if m.IsDisabled() || e.Type != vtinput.MouseEventType {
		return false
	}
	if m.mouseSelecting {
		index := m.GetClickIndex(int(e.MouseY))
		inside := int(e.MouseX) > m.X1 && int(e.MouseX) < m.X2 && index >= 0 && index < len(m.Items) && !m.Items[index].Separator
		if inside {
			m.SetSelectPos(index)
		}
		if IsMouseRelease(e) {
			m.mouseSelecting = false
			if inside {
				click := *e
				click.KeyDown = true
				click.ButtonState = vtinput.FromLeft1stButtonPressed
				click.MouseEventFlags = 0
				return m.ProcessMouse(&click)
			}
		}
		return true
	}
	if m.HandleMouseScroll(e) {
		return true
	}

	if (e.MouseEventFlags & vtinput.MouseMoved) != 0 {
		mx := int(e.MouseX)
		if mx <= m.X1 || mx >= m.X2 {
			return false
		}

		hoverIdx := m.GetClickIndex(int(e.MouseY))
		if hoverIdx == -1 {
			return false
		}
		// Rows past len(Items) belong to virtual consumers that only set
		// ItemCount; they are plain selectable rows, not separators.
		if hoverIdx >= len(m.Items) || !m.Items[hoverIdx].Separator {
			m.SetSelectPos(hoverIdx)
		}
		return true
	}

	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		clickIdx := m.GetClickIndex(int(e.MouseY))
		if clickIdx != -1 && (clickIdx >= len(m.Items) || !m.Items[clickIdx].Separator) {
			m.SetSelectPos(clickIdx)
			if m.HasSubMenu(clickIdx) {
				m.OpenSubMenu(clickIdx)
				return true
			}
			// Virtual rows (ItemCount beyond len(Items)) have no command to
			// fire; the click still selects and confirms them.
			if clickIdx < len(m.Items) {
				item := m.Items[clickIdx]
				if FrameManager.DisabledCommands.IsDisabled(item.Command) {
					return true
				}

				// Fire Action BEFORE calling OnAction/SetExitCode
				oldCmd := m.Command
				m.Command = item.Command
				m.FireAction(item.OnClick, item.UserData)
				m.Command = oldCmd
			}

			if m.OnAction != nil {
				m.OnAction(clickIdx)
			}
			m.SetExitCode(clickIdx)
			return true
		}
	}
	return false
}

// Show prepares the background and calls the render method.
func (m *VMenu) Show(scr *ScreenBuf) {
	m.ScreenObject.Show(scr)
	m.DisplayObject(scr)
}

// DisplayObject renders the frame and menu items.
func (m *VMenu) DisplayObject(scr *ScreenBuf) {
	if !m.IsVisible() {
		return
	}
	p := NewPainter(scr)

	// 1. Frame and Background
	p.Fill(m.X1, m.Y1, m.X2, m.Y2, ' ', Palette[m.ColorTextIdx])
	p.DrawBox(m.X1, m.Y1, m.X2, m.Y2, Palette[m.ColorBoxIdx], m.BoxType)

	// far2l paints a menu title with Menu.Title whether the menu holds focus
	// or not, so there is no separate focused variant here.
	p.DrawTitle(m.X1, m.Y1, m.X2, m.title, Palette[m.ColorTitleIdx])

	colText := Palette[m.ColorTextIdx]
	colSel := Palette[m.ColorSelectedTextIdx]
	colBox := Palette[m.ColorBoxIdx]
	height := m.Y2 - m.Y1 - 1
	if height < 0 {
		height = 0
	}

	colHigh := Palette[m.ColorHighlightIdx]
	colSelHigh := Palette[m.ColorSelectedHighlightIdx]

	// 3. Rendering items
	for i := 0; i < height; i++ {
		itemIdx := i + m.TopPos
		currY := m.Y1 + 1 + i
		if currY >= m.Y2 {
			break
		}
		if itemIdx >= len(m.Items) {
			continue
		}

		item := m.Items[itemIdx]
		isDisabled := !item.Separator && FrameManager.DisabledCommands.IsDisabled(item.Command)

		attr := colText
		if isDisabled {
			attr = DimColor(attr)
		} else if itemIdx == m.SelectPos {
			attr = colSel
		}

		if item.Separator {
			if m.BoxType == SingleBox {
				symbols := getBoxSymbols(SingleBox)
				p.DrawLine(m.X1, currY, m.X2, currY, symbols[bsH], colBox, false, false)
				scr.Write(m.X1, currY, []CharInfo{{Char: uint64(symbols[bsHCrossLeft]), Attributes: colBox}})
				scr.Write(m.X2, currY, []CharInfo{{Char: uint64(symbols[bsHCrossRight]), Attributes: colBox}})
			} else {
				p.DrawLine(m.X1, currY, m.X2, currY, boxSymbols[bsH], colBox, true, true)
			}
			continue
		}

		// Resolve item colors
		isSel := itemIdx == m.SelectPos
		isDisabled = FrameManager.DisabledCommands.IsDisabled(item.Command)

		itemAttr := colText
		hiAttr := colHigh
		if isSel {
			itemAttr, hiAttr = colSel, colSelHigh
		}
		if isDisabled {
			itemAttr, hiAttr = DimColor(itemAttr), DimColor(hiAttr)
		}

		// Calculate layout
		//clean, _, _ := ParseAmpersandString(item.Text)
		//vLenText := StringWidth(clean) + 1 // +1 for leading space
		hintText := ""
		vLenHint := 0
		if hint := menuItemHint(item); hint != "" {
			hintText = hint + " "
			vLenHint = StringWidth(hintText)
		}

		// Draw background and text
		p.Fill(m.X1+1, currY, m.X2-1, currY, ' ', itemAttr)
		textX := m.X1 + 1
		p.DrawString(textX, currY, " ", itemAttr)
		textX++
		if item.AccentPrefix != "" {
			p.DrawString(textX, currY, item.AccentPrefix, hiAttr)
			textX += runewidth.StringWidth(item.AccentPrefix)
		}
		p.DrawControlText(textX, currY, item.Text, itemAttr, hiAttr)
		if hintText != "" {
			p.DrawString(m.X2-vLenHint, currY, hintText, itemAttr)
		}
	}

	// 4. Scrollbar
	m.DrawScrollBar(scr)
}
