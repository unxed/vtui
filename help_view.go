package vtui

import (
	"os/exec"
	"runtime"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/vtinput"
)

type HelpView struct {
	BaseWindow
	engine  *HelpEngine
	history []helpHistoryEntry
	// current is the topic as it reads in the window now: source with the
	// lines that are longer than the text area broken at spaces (f4 #378).
	// rowSrc says which line of source each of its lines came from, and
	// wrapWidth which width that was done for.
	current     *HelpTopic
	source      *HelpTopic
	rowSrc      []int
	wrapWidth   int
	scrollTop   int
	selectedIdx int // Index of selected link in current.Links
	scrollBar   *ScrollBar
}

type helpHistoryEntry struct {
	topic       string
	selectedIdx int
	scrollTop   int
}

const helpBackButton = "[←]"

type helpViewLayout struct {
	contentX1, contentY1 int
	contentX2, contentY2 int
	contentWidth         int
	contentHeight        int
	scrollBarX           int
	showScrollBar        bool
}

func NewHelpView(engine *HelpEngine, startTopic string) *HelpView {
	hv := &HelpView{
		BaseWindow:  *NewBaseWindow(0, 0, 76, 20, " Help "),
		engine:      engine,
		selectedIdx: -1,
	}
	hv.ColorBoxIdx = ColHelpBox
	hv.ColorTitleIdx = ColHelpBoxTitle
	hv.ColorBackgroundIdx = ColHelpText

	hv.rootGroup.SetOwner(hv)
	hv.scrollBar = NewScrollBar(0, 0, 0)
	hv.scrollBar.ColorIdx = ColHelpScrollbar
	hv.scrollBar.SetOwner(hv)
	hv.scrollBar.OnScroll = func(v int) {
		hv.scrollTop = v
	}
	hv.scrollBar.PgStep = 10 // Default, will be updated in Show
	hv.Modal = true
	hv.ShowClose = true

	if FrameManager != nil && FrameManager.scr != nil {
		hv.ResizeConsole(FrameManager.scr.width, FrameManager.scr.height)
	}

	hv.SwitchTopic(startTopic)
	return hv
}

func (hv *HelpView) SwitchTopic(name string) {
	// 1. Handle External URLs
	if strings.HasPrefix(name, "http://") || strings.HasPrefix(name, "https://") {
		var err error
		switch runtime.GOOS {
		case "linux":
			err = exec.Command("xdg-open", name).Start()
		case "windows":
			err = exec.Command("rundll32", "url.dll,FileProtocolHandler", name).Start()
		case "darwin":
			err = exec.Command("open", name).Start()
		}
		if err != nil {
			DebugLog("HELP: Failed to open URL %s: %v", name, err)
		}
		return
	}

	// 2. Handle internal topics
	topic := hv.engine.GetTopic(name)
	if topic == nil {
		return
	}
	if hv.current != nil {
		hv.history = append(hv.history, helpHistoryEntry{
			topic:       hv.current.Name,
			selectedIdx: hv.selectedIdx,
			scrollTop:   hv.scrollTop,
		})
	}
	hv.applyTopic(topic)
	hv.scrollTop = 0
	hv.selectedIdx = -1
	if len(hv.current.Links) > 0 {
		hv.selectedIdx = 0
	}
	hv.frame.SetTitle(" Help: " + name + " ")
}

func (hv *HelpView) PopTopic() {
	if len(hv.history) == 0 {
		hv.Close()
		return
	}
	entry := hv.history[len(hv.history)-1]
	hv.history = hv.history[:len(hv.history)-1]
	topic := hv.engine.GetTopic(entry.topic)
	if topic == nil {
		return
	}
	hv.applyTopic(topic)
	hv.scrollTop = max(0, entry.scrollTop)
	hv.selectedIdx = entry.selectedIdx
	if hv.selectedIdx >= len(hv.current.Links) {
		// The entry was made at another width, where the topic had other rows.
		hv.selectedIdx = len(hv.current.Links) - 1
	}
	// The title names the topic on screen, and hosts resolve the current
	// topic from it (f4's help search does). Leaving the nested topic's name
	// there after going back made both wrong.
	hv.frame.SetTitle(" Help: " + entry.topic + " ")
}

// textWidth is the width of the text area, the columns between the paddings.
func (hv *HelpView) textWidth() int { return hv.X2 - hv.X1 - 3 }

// applyTopic makes topic the one on show, broken to the width of the window.
func (hv *HelpView) applyTopic(topic *HelpTopic) {
	hv.source = topic
	hv.wrapWidth = hv.textWidth()
	hv.current, hv.rowSrc = wrapHelpTopic(topic, hv.wrapWidth)
}

// CurrentTopic is the topic as it is laid out in the window: its lines are what
// is on screen, so a host that marks matches over the text (f4's help search)
// has to read them from here and not from the topic the engine holds.
func (hv *HelpView) CurrentTopic() *HelpTopic { return hv.current }

// rewrapOnResize breaks the topic again when the window has become wider or
// narrower (zoom, a resized terminal), and keeps the reader at the same place
// in the text and on the same link.
func (hv *HelpView) rewrapOnResize() {
	if hv.source == nil || hv.current == nil || hv.textWidth() == hv.wrapWidth {
		return
	}
	sticky := hv.current.StickyRows
	topSrc := 0
	if i := hv.scrollTop + sticky; i >= 0 && i < len(hv.rowSrc) {
		topSrc = hv.rowSrc[i]
	}
	var sel *HelpLink
	selSrc := 0
	if hv.selectedIdx >= 0 && hv.selectedIdx < len(hv.current.Links) {
		l := hv.current.Links[hv.selectedIdx]
		sel = &l
		if l.Line >= 0 && l.Line < len(hv.rowSrc) {
			selSrc = hv.rowSrc[l.Line]
		}
	}

	hv.applyTopic(hv.source)

	hv.scrollTop = 0
	for row, src := range hv.rowSrc {
		if row >= sticky && src >= topSrc {
			hv.scrollTop = row - sticky
			break
		}
	}
	if maxTop := len(hv.current.Lines) - sticky - hv.layout().contentHeight; hv.scrollTop > maxTop {
		hv.scrollTop = max(0, maxTop)
	}

	hv.selectedIdx = -1
	if sel != nil {
		for i, l := range hv.current.Links {
			if l.Target != sel.Target {
				continue
			}
			if hv.selectedIdx < 0 {
				hv.selectedIdx = i // the first with the same target is better than none
			}
			if l.Line < len(hv.rowSrc) && hv.rowSrc[l.Line] == selSrc {
				hv.selectedIdx = i
				break
			}
		}
	}
}

func (hv *HelpView) Show(scr *ScreenBuf) {
	hv.rewrapOnResize()
	hv.BaseWindow.Show(scr)
	if len(hv.history) > 0 {
		scr.Write(hv.X1+2, hv.Y1, StringToCharInfo(helpBackButton, Palette[hv.ColorBoxIdx]))
	}
	if hv.current == nil {
		return
	}

	layout := hv.layout()
	if layout.contentWidth <= 0 || layout.contentHeight <= 0 {
		return
	}
	x1, y1 := layout.contentX1, layout.contentY1
	width, height := layout.contentWidth, layout.contentHeight+hv.current.StickyRows

	// Fill background
	scr.PushClipRect(layout.contentX1, layout.contentY1, layout.contentX2, layout.contentY2)
	scr.FillRect(layout.contentX1, layout.contentY1, layout.contentX2, layout.contentY2, ' ', Palette[ColHelpText])

	// 1. Draw Sticky Headers
	for i := 0; i < hv.current.StickyRows; i++ {
		hv.renderLine(scr, x1, y1+i, hv.current.Lines[i], width, i)
	}

	// 2. Draw Scrolling Content
	contentY := y1 + hv.current.StickyRows
	contentH := height - hv.current.StickyRows
	for i := 0; i < contentH; i++ {
		lineIdx := i + hv.scrollTop + hv.current.StickyRows
		if lineIdx >= len(hv.current.Lines) {
			break
		}
		hv.renderLine(scr, x1, contentY+i, hv.current.Lines[lineIdx], width, lineIdx)
	}
	scr.PopClipRect()

	if layout.showScrollBar {
		hv.scrollBar.SetParams(hv.scrollTop, 0, len(hv.current.Lines)-hv.current.StickyRows-layout.contentHeight)
		hv.scrollBar.SetPosition(layout.scrollBarX, hv.Y1+1+hv.current.StickyRows, layout.scrollBarX, hv.Y2-1)
		hv.scrollBar.PgStep = layout.contentHeight
		hv.scrollBar.Show(scr)
	}
}

// TextArea reports the inclusive screen rectangle help text is drawn into,
// sticky header rows included. It leaves out the frame and the blank padding
// column inside each vertical border; the scrollbar sits on the right border
// itself, so it never narrows this area. A host painting over help text (f4
// highlights search matches) takes the text origin from here instead of
// assuming the padding.
func (hv *HelpView) TextArea() (x1, y1, x2, y2 int) {
	l := hv.layout()
	return l.contentX1, l.contentY1, l.contentX2, l.contentY2
}

func (hv *HelpView) layout() helpViewLayout {
	l := helpViewLayout{
		contentX1: hv.X1 + 2,
		contentY1: hv.Y1 + 1,
		contentX2: hv.X2 - 2,
		contentY2: hv.Y2 - 1,
		// far2l draws the help scrollbar on the right frame column
		// (ScrollBarEx(X2, ...) in help.cpp), not inside the window.
		scrollBarX: hv.X2,
	}
	stickyRows := 0
	if hv.current != nil {
		stickyRows = hv.current.StickyRows
	}
	l.contentHeight = l.contentY2 - l.contentY1 + 1 - stickyRows
	if l.contentHeight <= 0 {
		return l
	}
	if hv.current != nil {
		totalScrollable := len(hv.current.Lines) - stickyRows
		l.showScrollBar = hv.scrollBar != nil && totalScrollable > l.contentHeight
	}
	l.contentWidth = l.contentX2 - l.contentX1 + 1
	return l
}
func (hv *HelpView) renderLine(scr *ScreenBuf, x, y int, line string, width int, lineIdx int) {
	isCentered := strings.HasPrefix(line, "^")
	if isCentered {
		line = line[1:]
	}

	var cells []CharInfo
	currAttr := Palette[ColHelpText]

	inBold := false
	inLink := false

	var lineLinks []int
	for i, l := range hv.current.Links {
		if l.Line == lineIdx {
			lineLinks = append(lineLinks, i)
		}
	}

	runes := []rune(line)
	linkTriggerCount := 0

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch r {
		case '#':
			inBold = !inBold
			if inBold {
				currAttr = Palette[ColHelpBold]
			} else {
				currAttr = Palette[ColHelpText]
			}
			continue
		case '~':
			inLink = !inLink
			if inLink {
				if linkTriggerCount < len(lineLinks) {
					linkIdx := lineLinks[linkTriggerCount]
					if linkIdx == hv.selectedIdx {
						currAttr = Palette[ColHelpSelectedLink]
					} else {
						currAttr = Palette[ColHelpLink]
					}
					linkTriggerCount++
				} else {
					inLink = false
					w := runewidth.RuneWidth(r)
					cells = append(cells, CharInfo{Char: uint64(r), Attributes: currAttr})
					for j := 1; j < w; j++ {
						cells = append(cells, CharInfo{Char: WideCharFiller, Attributes: currAttr})
					}
				}
			} else {
				for i+1 < len(runes) && runes[i] != '@' {
					i++
				}
				currAttr = Palette[ColHelpText]
			}
			continue
		}

		w := runewidth.RuneWidth(r)
		cells = append(cells, CharInfo{Char: uint64(r), Attributes: currAttr})
		for j := 1; j < w; j++ {
			cells = append(cells, CharInfo{Char: WideCharFiller, Attributes: currAttr})
		}
	}

	offX := 0
	if isCentered {
		vLen := 0
		vLen = len(cells)
		if vLen > width {
			cells = fitHelpCells(cells, width)
			vLen = len(cells)
		}
		offX = (width - vLen) / 2
	} else {
		cells = fitHelpCells(cells, width)
	}
	scr.Write(x+offX, y, cells)
}

func fitHelpCells(cells []CharInfo, width int) []CharInfo {
	if width <= 0 {
		return nil
	}
	if len(cells) <= width {
		return cells
	}
	cells = cells[:width]
	// Do not leave the base cell of a wide rune without its filler cell at
	// the edge of the viewport. It would otherwise be rendered as a clipped
	// half-glyph by some terminal backends.
	if len(cells) > 0 && cells[len(cells)-1].Char != WideCharFiller &&
		runewidth.RuneWidth(rune(cells[len(cells)-1].Char)) > 1 {
		cells = cells[:len(cells)-1]
	}
	return cells
}

func (hv *HelpView) ProcessKey(e *vtinput.InputEvent) bool {
	if e.Type == vtinput.FocusEventType {
		return hv.BaseWindow.ProcessKey(e)
	}
	if !e.KeyDown {
		return false
	}

	// 1. Handle Help-specific navigation BEFORE BaseWindow focus cycling
	switch e.VirtualKeyCode {
	case vtinput.VK_F1:
		// Help is already active; do not let BaseWindow.ShowHelp nest another view.
		return true

	case vtinput.VK_TAB:
		if len(hv.current.Links) == 0 {
			return hv.BaseWindow.ProcessKey(e)
		}
		shift := (e.ControlKeyState & vtinput.ShiftPressed) != 0
		if shift {
			hv.selectedIdx--
			if hv.selectedIdx < 0 {
				hv.selectedIdx = len(hv.current.Links) - 1
			}
		} else {
			hv.selectedIdx++
			if hv.selectedIdx >= len(hv.current.Links) {
				hv.selectedIdx = 0
			}
		}
		hv.ensureLinkVisible()
		return true

	case vtinput.VK_RETURN:
		if hv.selectedIdx >= 0 && hv.selectedIdx < len(hv.current.Links) {
			hv.SwitchTopic(hv.current.Links[hv.selectedIdx].Target)
			return true
		}

	case vtinput.VK_BACK:
		hv.PopTopic()
		return true

	case vtinput.VK_ESCAPE:
		// Escape always closes the help window. Backspace is the explicit
		// history-navigation key and must remain separate from window closing.
		hv.Close()
		return true

	case vtinput.VK_UP:
		isCtrl := (e.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)) != 0
		if !isCtrl && len(hv.current.Links) > 0 {
			hv.selectedIdx--
			if hv.selectedIdx < 0 {
				hv.selectedIdx = len(hv.current.Links) - 1
			}
			hv.ensureLinkVisible()
			return true
		}
		if hv.scrollTop > 0 {
			hv.scrollTop--
		}
		return true

	case vtinput.VK_DOWN:
		isCtrl := (e.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)) != 0
		if !isCtrl && len(hv.current.Links) > 0 {
			hv.selectedIdx++
			if hv.selectedIdx >= len(hv.current.Links) {
				hv.selectedIdx = 0
			}
			hv.ensureLinkVisible()
			return true
		}
		viewHeight := (hv.Y2 - hv.Y1 + 1) - 2 - hv.current.StickyRows
		if hv.scrollTop < (len(hv.current.Lines)-hv.current.StickyRows)-viewHeight {
			hv.scrollTop++
		}
		return true

	case vtinput.VK_LEFT:
		if len(hv.current.Links) > 0 {
			hv.selectedIdx--
			if hv.selectedIdx < 0 {
				hv.selectedIdx = len(hv.current.Links) - 1
			}
			hv.ensureLinkVisible()
			return true
		}

	case vtinput.VK_RIGHT:
		if len(hv.current.Links) > 0 {
			hv.selectedIdx++
			if hv.selectedIdx >= len(hv.current.Links) {
				hv.selectedIdx = 0
			}
			hv.ensureLinkVisible()
			return true
		}

	case vtinput.VK_PRIOR: // PgUp
		viewHeight := (hv.Y2 - hv.Y1 + 1) - 2 - hv.current.StickyRows
		hv.scrollTop -= viewHeight
		if hv.scrollTop < 0 {
			hv.scrollTop = 0
		}
		return true

	case vtinput.VK_NEXT: // PgDn
		viewHeight := (hv.Y2 - hv.Y1 + 1) - 2 - hv.current.StickyRows
		maxScroll := (len(hv.current.Lines) - hv.current.StickyRows) - viewHeight
		if maxScroll < 0 {
			maxScroll = 0
		}
		hv.scrollTop += viewHeight
		if hv.scrollTop > maxScroll {
			hv.scrollTop = maxScroll
		}
		if hv.scrollTop < 0 {
			hv.scrollTop = 0
		}
		return true
	}

	return hv.BaseWindow.ProcessKey(e)
}

func (hv *HelpView) ensureLinkVisible() {
	if hv.selectedIdx == -1 {
		return
	}
	link := hv.current.Links[hv.selectedIdx]
	height := hv.Y2 - hv.Y1 - 1 - hv.current.StickyRows
	if link.Line < hv.scrollTop+hv.current.StickyRows {
		hv.scrollTop = link.Line - hv.current.StickyRows
	} else if link.Line >= hv.scrollTop+hv.current.StickyRows+height {
		hv.scrollTop = link.Line - hv.current.StickyRows - height + 1
	}
}

func (hv *HelpView) GetType() FrameType { return TypeUser }

func (hv *HelpView) scrollBy(delta int) {
	if hv.current == nil || delta == 0 {
		return
	}
	viewHeight := (hv.Y2 - hv.Y1 + 1) - 2 - hv.current.StickyRows
	maxScroll := (len(hv.current.Lines) - hv.current.StickyRows) - viewHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	hv.scrollTop += delta
	if hv.scrollTop < 0 {
		hv.scrollTop = 0
	} else if hv.scrollTop > maxScroll {
		hv.scrollTop = maxScroll
	}
}

func (hv *HelpView) ProcessMouse(e *vtinput.InputEvent) bool {
	if e.Type != vtinput.MouseEventType {
		return false
	}

	if hv.scrollBar != nil && hv.scrollBar.ProcessMouse(e) {
		return true
	}

	if len(hv.history) > 0 && e.KeyDown && e.ButtonState&vtinput.FromLeft1stButtonPressed != 0 &&
		int(e.MouseY) == hv.Y1 && int(e.MouseX) >= hv.X1+2 && int(e.MouseX) <= hv.X1+4 {
		hv.PopTopic()
		return true
	}

	if e.WheelDirection != 0 {
		if e.WheelDirection > 0 {
			hv.scrollBy(-WheelLinesPerNotch())
		} else {
			hv.scrollBy(WheelLinesPerNotch())
		}
		return true
	}

	if e.ButtonState != 0 && e.KeyDown {
		mx, my := int(e.MouseX), int(e.MouseY)
		linkIdx := hv.findLinkAt(mx, my)
		if linkIdx != -1 {
			oldSel := hv.selectedIdx
			hv.selectedIdx = linkIdx
			if hv.selectedIdx != oldSel {
				FrameManager.Redraw()
			}
		}

		isLeftDoubleClick := (e.MouseEventFlags&vtinput.DoubleClick) != 0 && (e.ButtonState&vtinput.FromLeft1stButtonPressed) != 0
		isMiddleClick := (e.ButtonState & vtinput.FromLeft2ndButtonPressed) != 0

		if isLeftDoubleClick || isMiddleClick {
			hv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})
			return true
		}

		if linkIdx != -1 {
			return true
		}
	}

	return hv.BaseWindow.ProcessMouse(e)
}

func (hv *HelpView) ResizeConsole(w, h int) {
	width := 76
	if width > w-4 {
		width = w - 4
	}
	if width < 20 {
		width = 20
	}
	height := h - 4
	if height < 5 {
		height = 5
	}
	hv.X1 = (w - width) / 2
	hv.Y1 = (h - height) / 2
	hv.X2 = hv.X1 + width - 1
	hv.Y2 = hv.Y1 + height - 1
	hv.frame.SetPosition(hv.X1, hv.Y1, hv.X2, hv.Y2)
	hv.rootGroup.SetPosition(hv.X1+1, hv.Y1+1, hv.X2-1, hv.Y2-1)
}

func (hv *HelpView) findLinkAt(mx, my int) int {
	layout := hv.layout()
	x1, y1, width := layout.contentX1, layout.contentY1, layout.contentWidth

	if width <= 0 || my < y1 || my > layout.contentY2 || mx < x1 || mx > x1+width-1 {
		return -1
	}

	// Вычисляем логический индекс строки с учетом прокрутки и липкого заголовка
	lineIdx := -1
	if my >= y1 && my < y1+hv.current.StickyRows {
		lineIdx = my - y1
	} else {
		lineIdx = my - y1 - hv.current.StickyRows + hv.scrollTop + hv.current.StickyRows
	}

	if lineIdx < 0 || lineIdx >= len(hv.current.Lines) {
		return -1
	}

	line := hv.current.Lines[lineIdx]
	isCentered := strings.HasPrefix(line, "^")
	if isCentered {
		line = line[1:]
	}

	var lineLinkIndices []int
	for i, l := range hv.current.Links {
		if l.Line == lineIdx {
			lineLinkIndices = append(lineLinkIndices, i)
		}
	}

	runes := []rune(line)
	visualX := 0
	inLink := false
	linkTriggerCount := 0

	type linkBounds struct {
		linkIdx  int
		vx1, vx2 int
	}
	var bounds []linkBounds
	var currBounds *linkBounds

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch r {
		case '#':
			continue
		case '~':
			inLink = !inLink
			if inLink {
				if linkTriggerCount < len(lineLinkIndices) {
					currBounds = &linkBounds{
						linkIdx: lineLinkIndices[linkTriggerCount],
						vx1:     visualX,
					}
					linkTriggerCount++
				}
			} else {
				if currBounds != nil {
					currBounds.vx2 = visualX - 1
					bounds = append(bounds, *currBounds)
					currBounds = nil
				}
				for i+1 < len(runes) && runes[i] != '@' {
					i++
				}
			}
			continue
		}

		w := runewidth.RuneWidth(r)
		if w <= 0 {
			w = 1
		}
		visualX += w
	}

	offX := 0
	if isCentered {
		if visualX > width {
			visualX = width
		}
		offX = (width - visualX) / 2
	}

	relMX := mx - x1 - offX
	for _, b := range bounds {
		if relMX >= b.vx1 && relMX <= b.vx2 {
			return b.linkIdx
		}
	}

	return -1
}
