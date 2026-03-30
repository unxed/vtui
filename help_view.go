package vtui

import (
	"os/exec"
	"runtime"
	"strings"
	"github.com/unxed/vtinput"
	"github.com/mattn/go-runewidth"
)

type HelpView struct {
	BaseWindow
	engine      *HelpEngine
	history     []string
	current     *HelpTopic
	scrollTop   int
	selectedIdx int // Index of selected link in current.Links
	scrollBar   *ScrollBar
}

func NewHelpView(engine *HelpEngine, startTopic string) *HelpView {
	hv := &HelpView{
		BaseWindow: *NewBaseWindow(0, 0, 60, 20, " Help "),
		engine:     engine,
		selectedIdx: -1,
	}
	hv.scrollBar = NewScrollBar(0, 0, 0)
	hv.scrollBar.OnScroll = func(v int) { hv.scrollTop = v }
	hv.scrollBar.PgStep = 10 // Default, will be updated in Show
	hv.Modal = true
	hv.ShowClose = true
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
		hv.history = append(hv.history, hv.current.Name)
	}
	hv.current = topic
	hv.scrollTop = 0
	hv.selectedIdx = -1
	if len(topic.Links) > 0 {
		hv.selectedIdx = 0
	}
	hv.frame.SetTitle(" Help: " + name + " ")
}

func (hv *HelpView) PopTopic() {
	if len(hv.history) == 0 {
		hv.Close()
		return
	}
	name := hv.history[len(hv.history)-1]
	hv.history = hv.history[:len(hv.history)-1]
	hv.current = hv.engine.GetTopic(name)
	hv.scrollTop = 0
	hv.selectedIdx = -1
}

func (hv *HelpView) Show(scr *ScreenBuf) {
	hv.BaseWindow.Show(scr)
	if hv.current == nil { return }

	x1, y1, x2, y2 := hv.X1+1, hv.Y1+1, hv.X2-1, hv.Y2-1
	width := x2 - x1 + 1
	height := y2 - y1 + 1

	if hv.scrollBar != nil {
		width--
	}

	// Fill background
	scr.FillRect(x1, y1, x2, y2, ' ', Palette[ColHelpText])

	// 1. Draw Sticky Headers
	for i := 0; i < hv.current.StickyRows; i++ {
		hv.renderLine(scr, x1, y1+i, hv.current.Lines[i], width, i)
	}

	// 2. Draw Scrolling Content
	contentY := y1 + hv.current.StickyRows
	contentH := height - hv.current.StickyRows
	for i := 0; i < contentH; i++ {
		lineIdx := i + hv.scrollTop + hv.current.StickyRows
		if lineIdx >= len(hv.current.Lines) { break }
		hv.renderLine(scr, x1, contentY+i, hv.current.Lines[lineIdx], width, lineIdx)
	}

	if hv.scrollBar != nil {
		totalScrollable := len(hv.current.Lines) - hv.current.StickyRows
		if totalScrollable > contentH {
			hv.scrollBar.SetParams(hv.scrollTop, 0, totalScrollable-contentH)
			hv.scrollBar.SetPosition(hv.X2-1, hv.Y1+1+hv.current.StickyRows, hv.X2-1, hv.Y2-1)
			hv.scrollBar.PgStep = contentH
			hv.scrollBar.Show(scr)
		}
	}
}

func (hv *HelpView) renderLine(scr *ScreenBuf, x, y int, line string, width int, lineIdx int) {
	isCentered := strings.HasPrefix(line, "^")
	if isCentered { line = line[1:] }

	var cells []CharInfo
	currAttr := Palette[ColHelpText]

	// State for parsing line-local formatting
	inBold := false
	inLink := false

	// Find links for this line to handle selection highlighting
	var lineLinks []int
	for i, l := range hv.current.Links {
		if l.Line == lineIdx { lineLinks = append(lineLinks, i) }
	}

	runes := []rune(line)
	linkTriggerCount := 0

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch r {
		case '#':
			inBold = !inBold
			if inBold { currAttr = Palette[ColHelpBold] } else { currAttr = Palette[ColHelpText] }
			continue
		case '~':
			inLink = !inLink
			if inLink {
				linkIdx := lineLinks[linkTriggerCount]
				if linkIdx == hv.selectedIdx {
					currAttr = Palette[ColHelpSelectedLink]
				} else {
					currAttr = Palette[ColHelpLink]
				}
				linkTriggerCount++
			} else {
				// End of link text, skip target (@... part)
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
		for _, c := range cells { if c.Char != WideCharFiller { vLen++ } }
		offX = (width - vLen) / 2
	}
	scr.Write(x+offX, y, cells)
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

	case vtinput.VK_UP:
		if hv.scrollTop > 0 {
			hv.scrollTop--
		}
		return true

	case vtinput.VK_DOWN:
		viewHeight := (hv.Y2 - hv.Y1 + 1) - 2 - hv.current.StickyRows
		if hv.scrollTop < (len(hv.current.Lines)-hv.current.StickyRows)-viewHeight {
			hv.scrollTop++
		}
		return true

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
	if hv.selectedIdx == -1 { return }
	link := hv.current.Links[hv.selectedIdx]
	height := hv.Y2 - hv.Y1 - 1 - hv.current.StickyRows
	if link.Line < hv.scrollTop + hv.current.StickyRows {
		hv.scrollTop = link.Line - hv.current.StickyRows
	} else if link.Line >= hv.scrollTop + hv.current.StickyRows + height {
		hv.scrollTop = link.Line - hv.current.StickyRows - height + 1
	}
}

func (hv *HelpView) GetType() FrameType { return TypeUser }

func (hv *HelpView) ProcessMouse(e *vtinput.InputEvent) bool {
	if e.Type != vtinput.MouseEventType {
		return false
	}

	if hv.scrollBar != nil && int(e.MouseX) == hv.scrollBar.X1 {
		contentH := (hv.Y2 - hv.Y1 + 1) - 2 - hv.current.StickyRows
		totalScrollable := len(hv.current.Lines) - hv.current.StickyRows
		if totalScrollable > contentH {
			if hv.scrollBar.ProcessMouse(e) {
				return true
			}
		}
	}

	if e.WheelDirection != 0 {
		if e.WheelDirection > 0 {
			hv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP})
		} else {
			hv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
		}
		return true
	}

	return hv.BaseWindow.ProcessMouse(e)
}
