package vtui

import (
	"strings"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/vtinput"
)

// ChatRole tells ChatWindow which side of the conversation a ChatTurn is on,
// so it can pick a header label and an arrow glyph for it.
type ChatRole int

const (
	// ChatRoleSelf is the local user's own turns ("sent" messages).
	ChatRoleSelf ChatRole = iota
	// ChatRolePeer is the other side of the conversation: a bot, an AI
	// model, a remote chat participant, whatever the host is talking to.
	ChatRolePeer
)

// ChatTurn is one message in a ChatWindow's history.
type ChatTurn struct {
	Role ChatRole
	Text string
	Time time.Time
}

// ChatLink is a navigable target found on screen inside the message area: a
// markdown [text](target) link, or a bare URL matching one of ChatWindow's
// URLSchemes. Row/Col are relative to the message area's top-left cell.
type ChatLink struct {
	Row, Col, Width int
	Target          string
}

// ChatStatusBar is an optional strip ChatWindow shows between the message
// area and the input box, for host state that is outside a generic chat
// widget's business — f4's AI assistant panel (f4 #615, the panel this
// control was extracted from) uses it for "these files are attached" and
// "the model's patch is ready to apply". A ChatWindow with no StatusBar set
// simply has no such strip and gives the extra row to the message area.
type ChatStatusBar interface {
	// Label renders the strip's current text within maxW cells. An empty
	// string hides the strip for this frame and returns its row to the
	// message area.
	Label(maxW int) string
	// Activate runs when Enter is pressed while the strip has focus.
	Activate()
}

type chatWinLine struct {
	cells   []CharInfo
	targets []string
}

// ChatWindow is a bordered chat control: a scrollable, wrapped message
// history with markdown-link and bare-URL navigation, over a multi-line
// input box — the reusable shape of f4's AI assistant panel (f4 #615).
//
// A host wires it up like any other composite vtui control: create it with
// NewChatWindow, then style Frame and Input directly (as with any
// BorderedFrame/MultiLineEdit) and set the Color*Idx fields below to its own
// palette. Push turns into the Turns field and call Show every frame, the
// same way a list-like control expects its data refreshed before drawing.
type ChatWindow struct {
	ScreenObject

	// Frame is the control's border and title.
	Frame *BorderedFrame
	// Input is the multi-line box at the bottom the user types into.
	Input *MultiLineEdit

	// ColorTextIdx is the message area's text/background color.
	ColorTextIdx int
	// ColorHeaderIdx colors each turn's "▸ Label  15:04" header line.
	ColorHeaderIdx int
	// ColorLinkIdx colors markdown and bare-URL link text.
	ColorLinkIdx int
	// ColorLinkFocusIdx supplies the background blended into the currently
	// focused link's cells, and doubles as the StatusBar's label color while
	// the status bar has focus.
	ColorLinkFocusIdx int
	// ColorStatusBarIdx is the StatusBar label's color while unfocused.
	ColorStatusBarIdx int
	// ColorTitleIdx/ColorTitleFocusedIdx are written to Frame.ColorTitleIdx
	// by SetFocus, so the border title can change color with focus the way
	// f4's panel titles do. Leave them equal to opt out.
	ColorTitleIdx        int
	ColorTitleFocusedIdx int
	// ColorHighlightBgRefIdx is the background HighlightLang's syntax
	// highlighter leaves untouched on a token (its own idea of "default");
	// ChatWindow swaps that background for ColorTextIdx's so highlighted
	// text still blends into the message area instead of showing through
	// with a foreign background.
	ColorHighlightBgRefIdx int

	// SelfLabel/PeerLabel are the header text for ChatRoleSelf/ChatRolePeer
	// turns, e.g. "You" and a bot's name.
	SelfLabel string
	PeerLabel string
	// TimeFormat is appended to every header via time.Time.Format; "" omits
	// the timestamp entirely.
	TimeFormat string

	// HighlightLang, when set, is passed to GetHighlighter to colorize
	// message text (f4 passes a synthetic "chat.md" to get markdown-ish
	// highlighting from whatever highlighter is registered for .md).
	HighlightLang string
	// URLSchemes lists the bare-URL prefixes ChatWindow autolinks inside
	// message text outside of markdown [text](url) syntax, e.g.
	// []string{"https://", "ai://"}. Markdown links are always recognized
	// regardless of scheme.
	URLSchemes []string
	// WheelLines is how many lines a single mouse wheel notch scrolls.
	WheelLines int

	// Busy, while true, appends a trailing status line built from
	// BusyLabel — the "typing…" indicator.
	Busy      bool
	BusyLabel string

	// Turns is the conversation history to render. The host refreshes it
	// (e.g. from its own session/backend) before every Show, the same way a
	// list control's items are kept current by its owner.
	Turns []ChatTurn

	// StatusBar, if set, draws an extra strip above Input; see
	// ChatStatusBar.
	StatusBar ChatStatusBar

	// OnSend fires when the user presses Enter over non-blank input text.
	// ChatWindow clears Input and scrolls to bottom right after.
	OnSend func(text string)
	// OnActivateLink fires when a focused link's Enter is pressed, or a
	// link is double-clicked / middle-clicked.
	OnActivateLink func(target string)
	// OnLinkAltKey fires on F5 while a link has focus, for a host-specific
	// secondary action (f4 uses it to copy the linked file between panels).
	// Its return value is currently unused but mirrors OnActivateLink's
	// shape for symmetry and future use.
	OnLinkAltKey func(target string, e *vtinput.InputEvent) bool
	// OnStatusBarKey, if set, is offered every key event while the status
	// bar has focus, before ChatWindow's own Enter/arrow handling — a hook
	// for host-specific bar hotkeys (f4 uses it for F3 "preview the
	// patch"). Returning true marks the key handled.
	OnStatusBarKey func(e *vtinput.InputEvent) bool
	// ExtraLink, if set, is offered each already-wrapped source line of
	// message text and may turn it into an appended link line: for example
	// f4 uses it to turn a ```lang:filename fenced code header into a jump
	// link. Returning ok == false leaves the line alone.
	ExtraLink func(line string) (label, target string, ok bool)

	topPos         int
	lines          []chatWinLine
	visibleLinks   []ChatLink
	focusedLinkIdx int // -1: Input has focus. -2: StatusBar has focus. >=0: that link has focus.
}

// NewChatWindow creates a chat control occupying (x1,y1)-(x2,y2). Like
// NewBorderedFrame/NewMultiLineEdit, colors default to vtui's built-in
// palette entries; a themed host is expected to override the Color*Idx
// fields (and Frame's/Input's own) the same way it would for any other
// composite control.
func NewChatWindow(x1, y1, x2, y2 int, title string) *ChatWindow {
	cw := &ChatWindow{
		Frame:                  NewBorderedFrame(x1, y1, x2, y2, SingleBox, title),
		Input:                  NewMultiLineEdit(0, 0, 10, 3, ""),
		ColorTextIdx:           ColDialogText,
		ColorHeaderIdx:         ColDialogBoxTitle,
		ColorLinkIdx:           ColHelpLink,
		ColorLinkFocusIdx:      ColHelpSelectedLink,
		ColorStatusBarIdx:      ColDialogHighlightText,
		ColorHighlightBgRefIdx: ColDialogText,
		TimeFormat:             "15:04",
		WheelLines:             3,
		focusedLinkIdx:         -1,
	}
	cw.ColorTitleIdx = cw.Frame.ColorTitleIdx
	cw.ColorTitleFocusedIdx = cw.Frame.ColorTitleIdx
	cw.SetPosition(x1, y1, x2, y2)
	return cw
}

// SetPosition places the control and lays out Frame and Input inside it.
// Below a height of 10 rows, Input shrinks to 2 rows to leave room for the
// message area.
func (cw *ChatWindow) SetPosition(x1, y1, x2, y2 int) {
	cw.ScreenObject.SetPosition(x1, y1, x2, y2)
	cw.Frame.SetPosition(x1, y1, x2, y2)

	inputH := 4
	if y2-y1 < 10 {
		inputH = 2
	}
	cw.Input.SetPosition(x1+1, y2-inputH, x2-1, y2-1)
}

// SetFocus gives or removes focus from the control. It forwards focus to
// Input unless a link or the status bar currently has it, and swaps
// Frame.ColorTitleIdx between ColorTitleIdx/ColorTitleFocusedIdx.
func (cw *ChatWindow) SetFocus(f bool) {
	cw.ScreenObject.SetFocus(f)
	if f {
		cw.Frame.ColorTitleIdx = cw.ColorTitleFocusedIdx
	} else {
		cw.Frame.ColorTitleIdx = cw.ColorTitleIdx
	}
	if f && cw.focusedLinkIdx == -1 {
		cw.Input.SetFocus(true)
	} else {
		cw.Input.SetFocus(false)
	}
}

// ScrollToBottom recomputes wrapped lines and scrolls to show the last page
// of the message history. Hosts call it right after appending a turn.
func (cw *ChatWindow) ScrollToBottom() {
	cw.updateLines()
	h := cw.Input.Y1 - cw.Y1 - 2
	maxTop := len(cw.lines) - h
	if maxTop > 0 {
		cw.topPos = maxTop
	}
}

// StatusBarFocused reports whether the status bar currently has focus.
func (cw *ChatWindow) StatusBarFocused() bool { return cw.focusedLinkIdx == -2 }

// LinkFocused reports whether a message-area link currently has focus.
func (cw *ChatWindow) LinkFocused() bool { return cw.focusedLinkIdx >= 0 }

// FocusedLink returns the currently focused link, if any.
func (cw *ChatWindow) FocusedLink() (ChatLink, bool) {
	if cw.focusedLinkIdx < 0 || cw.focusedLinkIdx >= len(cw.visibleLinks) {
		return ChatLink{}, false
	}
	return cw.visibleLinks[cw.focusedLinkIdx], true
}

func (cw *ChatWindow) barAvailWidth() int {
	return cw.X2 - cw.X1 - 3
}

func (cw *ChatWindow) statusBarLabel() string {
	if cw.StatusBar == nil {
		return ""
	}
	return cw.StatusBar.Label(cw.barAvailWidth())
}

func (cw *ChatWindow) headerText(t ChatTurn) string {
	label := cw.PeerLabel
	arrow := "▾ "
	if t.Role == ChatRoleSelf {
		label = cw.SelfLabel
		arrow = "▸ "
	}
	s := arrow + label
	if cw.TimeFormat != "" {
		s += "  " + t.Time.Format(cw.TimeFormat)
	}
	return s
}

// ProcessKey handles scrolling, link/status-bar focus navigation, and
// sending Input's text on Enter. It returns false (letting a host route the
// key elsewhere, e.g. panel switching on Tab) whenever it does not apply.
func (cw *ChatWindow) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown || !cw.IsFocused() {
		return false
	}

	ctrl := (e.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)) != 0
	alt := (e.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0
	shift := (e.ControlKeyState & vtinput.ShiftPressed) != 0

	hasBar := cw.statusBarLabel() != ""

	if cw.focusedLinkIdx == -2 {
		// Focus is on the status bar.
		if cw.OnStatusBarKey != nil && cw.OnStatusBarKey(e) {
			return true
		}
		if e.VirtualKeyCode == vtinput.VK_RETURN {
			if cw.StatusBar != nil {
				cw.StatusBar.Activate()
			}
			return true
		}
		if e.VirtualKeyCode == vtinput.VK_DOWN || e.VirtualKeyCode == vtinput.VK_RIGHT {
			cw.focusedLinkIdx = -1
			cw.Input.SetFocus(true)
			cw.redraw()
			return true
		}
		if e.VirtualKeyCode == vtinput.VK_UP || e.VirtualKeyCode == vtinput.VK_LEFT {
			if len(cw.visibleLinks) > 0 {
				cw.focusedLinkIdx = len(cw.visibleLinks) - 1
			} else {
				cw.focusedLinkIdx = -1
				cw.Input.SetFocus(true)
			}
			cw.redraw()
			return true
		}
		if e.VirtualKeyCode == vtinput.VK_ESCAPE {
			cw.focusedLinkIdx = -1
			cw.Input.SetFocus(true)
			cw.redraw()
			return true
		}
	}

	if cw.focusedLinkIdx >= 0 {
		// Focus is on a response link.
		if e.VirtualKeyCode == vtinput.VK_RETURN {
			if cw.focusedLinkIdx < len(cw.visibleLinks) && cw.OnActivateLink != nil {
				cw.OnActivateLink(cw.visibleLinks[cw.focusedLinkIdx].Target)
			}
			return true
		}
		if e.VirtualKeyCode == vtinput.VK_F5 {
			if cw.focusedLinkIdx < len(cw.visibleLinks) && cw.OnLinkAltKey != nil {
				cw.OnLinkAltKey(cw.visibleLinks[cw.focusedLinkIdx].Target, e)
			}
			return true
		}
		if e.VirtualKeyCode == vtinput.VK_ESCAPE {
			cw.focusedLinkIdx = -1
			cw.Input.SetFocus(true)
			cw.redraw()
			return true
		}
		if e.VirtualKeyCode == vtinput.VK_RIGHT || e.VirtualKeyCode == vtinput.VK_DOWN {
			cw.focusedLinkIdx++
			if cw.focusedLinkIdx >= len(cw.visibleLinks) {
				if hasBar {
					cw.focusedLinkIdx = -2
				} else {
					cw.focusedLinkIdx = -1
					cw.Input.SetFocus(true)
				}
			}
			cw.redraw()
			return true
		}
		if e.VirtualKeyCode == vtinput.VK_LEFT || e.VirtualKeyCode == vtinput.VK_UP {
			cw.focusedLinkIdx--
			if cw.focusedLinkIdx < 0 {
				cw.focusedLinkIdx = 0
			}
			cw.redraw()
			return true
		}
	}

	// Transition from Input's row 0 to the status bar or a response link.
	if cw.focusedLinkIdx == -1 && !ctrl && !alt && !shift {
		row, col := cw.Input.CursorPos()
		if (e.VirtualKeyCode == vtinput.VK_UP && row == 0) || (e.VirtualKeyCode == vtinput.VK_LEFT && row == 0 && col == 0) {
			if hasBar {
				cw.focusedLinkIdx = -2
				cw.Input.SetFocus(false)
				cw.redraw()
				return true
			} else if len(cw.visibleLinks) > 0 {
				cw.focusedLinkIdx = len(cw.visibleLinks) - 1
				cw.Input.SetFocus(false)
				cw.redraw()
				return true
			}
		}
	}

	if e.VirtualKeyCode == vtinput.VK_RETURN && !ctrl && !alt && cw.focusedLinkIdx == -1 {
		if shift {
			ePlain := *e
			ePlain.ControlKeyState &^= vtinput.ShiftPressed
			return cw.Input.ProcessKey(&ePlain)
		}
		text := cw.Input.GetText()
		if strings.TrimSpace(text) != "" {
			if cw.OnSend != nil {
				cw.OnSend(text)
			}
			cw.Input.SetText("")
			cw.ScrollToBottom()
		}
		return true
	}

	h := cw.Input.Y1 - cw.Y1 - 2
	if h < 1 {
		h = 1
	}

	switch e.VirtualKeyCode {
	case vtinput.VK_PRIOR:
		cw.topPos -= h
		if cw.topPos < 0 {
			cw.topPos = 0
		}
		cw.redraw()
		return true
	case vtinput.VK_NEXT:
		cw.topPos += h
		maxTop := len(cw.lines) - h
		if maxTop < 0 {
			maxTop = 0
		}
		if cw.topPos > maxTop {
			cw.topPos = maxTop
		}
		cw.redraw()
		return true
	case vtinput.VK_UP:
		if !ctrl && !alt && !shift {
			if cw.focusedLinkIdx == -1 {
				row, _ := cw.Input.CursorPos()
				if row == 0 && cw.topPos > 0 {
					cw.topPos--
					cw.redraw()
					return true
				}
			}
		}
	case vtinput.VK_DOWN:
		if !ctrl && !alt && !shift {
			if cw.focusedLinkIdx == -1 {
				row, _ := cw.Input.CursorPos()
				if row == cw.Input.LineCount()-1 {
					maxTop := len(cw.lines) - h
					if maxTop < 0 {
						maxTop = 0
					}
					if cw.topPos < maxTop {
						cw.topPos++
						cw.redraw()
						return true
					}
				}
			}
		}
	case vtinput.VK_LEFT, vtinput.VK_RIGHT:
		if shift {
			if cw.focusedLinkIdx == -1 {
				cw.Input.ProcessKey(e)
			}
			return true
		}
	}

	if cw.focusedLinkIdx == -1 {
		if cw.Input.ProcessKey(e) {
			return true
		}
	}

	return false
}

func (cw *ChatWindow) redraw() {
	if FrameManager != nil {
		FrameManager.Redraw()
	}
}

// ProcessMouse handles clicks on links, wheel scrolling, and delegates to
// Input first so text selection/caret placement inside it keeps working.
func (cw *ChatWindow) ProcessMouse(e *vtinput.InputEvent) bool {
	if cw.Input.ProcessMouse(e) {
		cw.focusedLinkIdx = -1
		cw.Input.SetFocus(true)
		return true
	}
	if e.Type == vtinput.MouseEventType {
		if e.WheelDirection != 0 {
			wheel := cw.WheelLines
			if wheel <= 0 {
				wheel = 3
			}
			if e.WheelDirection > 0 {
				cw.topPos -= wheel
				if cw.topPos < 0 {
					cw.topPos = 0
				}
			} else {
				cw.topPos += wheel
				h := cw.Input.Y1 - cw.Y1 - 2
				maxTop := len(cw.lines) - h
				if maxTop < 0 {
					maxTop = 0
				}
				if cw.topPos > maxTop {
					cw.topPos = maxTop
				}
			}
			cw.redraw()
			return true
		}
		if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
			mx, my := int(e.MouseX), int(e.MouseY)
			if mx > cw.X1 && mx < cw.X2 && my > cw.Y1 && my < cw.Input.Y1-1 {
				// Clicked in the message area.
				cw.Input.SetFocus(false)
				cw.focusedLinkIdx = -1
				row := my - cw.Y1 - 1
				col := mx - cw.X1 - 1
				for i, link := range cw.visibleLinks {
					if link.Row == row && col >= link.Col && col < link.Col+link.Width {
						cw.focusedLinkIdx = i
						if e.MouseEventFlags&vtinput.DoubleClick != 0 && cw.OnActivateLink != nil {
							cw.OnActivateLink(link.Target)
						}
						break
					}
				}
				cw.redraw()
				return true
			}
		}
	}
	return false
}

// updateLines re-wraps Turns (and the busy indicator, if any) into on-screen
// cells, parsing markdown [text](url) links, bare URLSchemes autolinks, and
// ExtraLink's synthetic links along the way.
func (cw *ChatWindow) updateLines() {
	w := cw.X2 - cw.X1 - 1
	if w <= 0 {
		return
	}

	var lines []chatWinLine

	attr := Palette[cw.ColorTextIdx]
	headerAttr := Palette[cw.ColorHeaderIdx]
	linkAttr := Palette[cw.ColorLinkIdx]

	var highlighter Highlighter
	if cw.HighlightLang != "" {
		highlighter = GetHighlighter(cw.HighlightLang, "")
	}
	var hlState any
	bgAttr := attr

	appendWrapped := func(runes []rune, attrs []uint64, targets []string) {
		col := 0
		var currentCells []CharInfo
		var currentTargets []string

		for i := 0; i < len(runes); i++ {
			r := runes[i]
			rw := runewidth.RuneWidth(r)
			if rw <= 0 {
				rw = 1
			}

			if col+rw > w-2 {
				lines = append(lines, chatWinLine{cells: currentCells, targets: currentTargets})
				currentCells = nil
				currentTargets = nil
				col = 0
			}

			charVal := RegisterCluster(string(r))
			for j := 0; j < rw; j++ {
				currentCells = append(currentCells, CharInfo{Char: charVal, Attributes: attrs[i]})
				currentTargets = append(currentTargets, targets[i])
				charVal = uint64(WideCharFiller)
			}
			col += rw
		}
		if len(currentCells) > 0 {
			lines = append(lines, chatWinLine{cells: currentCells, targets: currentTargets})
		}
	}

	makePlainLine := func(text string, attr uint64) chatWinLine {
		var cells []CharInfo
		var targets []string
		for _, r := range text {
			rw := runewidth.RuneWidth(r)
			if rw <= 0 {
				rw = 1
			}
			charVal := RegisterCluster(string(r))
			for j := 0; j < rw; j++ {
				cells = append(cells, CharInfo{Char: charVal, Attributes: attr})
				targets = append(targets, "")
				charVal = uint64(WideCharFiller)
			}
		}
		return chatWinLine{cells: cells, targets: targets}
	}

	for _, t := range cw.Turns {
		lines = append(lines, makePlainLine(cw.headerText(t), headerAttr))

		for _, p := range strings.Split(t.Text, "\n") {
			var lineSyntax []uint64
			if highlighter != nil {
				lineSyntax, hlState = highlighter.Highlight(p, hlState, bgAttr)
			}

			runesSrc := []rune("  " + p)
			syntaxPad := []uint64{attr, attr}
			fullSyntax := append(syntaxPad, lineSyntax...)

			var runes []rune
			var attrs []uint64
			var targets []string

			i := 0
			for i < len(runesSrc) {
				// Parse markdown inline links [text](url).
				if runesSrc[i] == '[' {
					closeBracket := -1
					for j := i + 1; j < len(runesSrc); j++ {
						if runesSrc[j] == ']' {
							closeBracket = j
							break
						}
					}
					if closeBracket != -1 && closeBracket+1 < len(runesSrc) && runesSrc[closeBracket+1] == '(' {
						closeParen := -1
						for j := closeBracket + 2; j < len(runesSrc); j++ {
							if runesSrc[j] == ')' {
								closeParen = j
								break
							}
						}
						if closeParen != -1 {
							text := string(runesSrc[i+1 : closeBracket])
							target := string(runesSrc[closeBracket+2 : closeParen])
							for _, r := range text {
								runes = append(runes, r)
								attrs = append(attrs, linkAttr)
								targets = append(targets, target)
							}
							i = closeParen + 1
							continue
						}
					}
				}

				// Parse bare URLSchemes autolinks, preferring the longest
				// matching scheme when more than one applies at i.
				matchedLen := 0
				for _, scheme := range cw.URLSchemes {
					sr := []rune(scheme)
					if len(sr) == 0 || len(sr) <= matchedLen {
						continue
					}
					if i+len(sr) <= len(runesSrc) && string(runesSrc[i:i+len(sr)]) == scheme {
						matchedLen = len(sr)
					}
				}
				if matchedLen > 0 {
					end := i
					for end < len(runesSrc) && runesSrc[end] > 32 && runesSrc[end] != ')' && runesSrc[end] != ']' {
						end++
					}
					target := string(runesSrc[i:end])
					for _, r := range target {
						runes = append(runes, r)
						attrs = append(attrs, linkAttr)
						targets = append(targets, target)
					}
					i = end
					continue
				}

				// Default syntax mapping.
				curAttr := attr
				if i < len(fullSyntax) {
					curAttr = fullSyntax[i]
					// If the highlighter left its own default background,
					// blend it with the message area's own background.
					if curAttr&IsBgRGB == 0 && GetIndexBack(curAttr) == GetIndexBack(Palette[cw.ColorHighlightBgRefIdx]) {
						curAttr = SetIndexBack(curAttr, GetIndexBack(attr))
					}
				}
				runes = append(runes, runesSrc[i])
				attrs = append(attrs, curAttr)
				targets = append(targets, "")
				i++
			}

			// Let the host append a synthetic link after the line, e.g.
			// f4 turns a ```lang:filename fenced code header into a jump
			// link to that output file.
			if cw.ExtraLink != nil {
				if label, target, ok := cw.ExtraLink(p); ok {
					tr := []rune("  " + label)
					for j := 0; j < len(tr); j++ {
						if j < 2 {
							runes = append(runes, tr[j])
							attrs = append(attrs, attr)
							targets = append(targets, "")
						} else {
							runes = append(runes, tr[j])
							attrs = append(attrs, linkAttr)
							targets = append(targets, target)
						}
					}
				}
			}

			appendWrapped(runes, attrs, targets)
		}
		lines = append(lines, chatWinLine{})
	}

	if cw.Busy && cw.BusyLabel != "" {
		lines = append(lines, makePlainLine("▸ "+cw.BusyLabel, headerAttr))
	}

	cw.lines = lines
}

// Show draws Frame, the message history (scrolled to topPos), the optional
// StatusBar strip, and Input, in that order.
func (cw *ChatWindow) Show(scr *ScreenBuf) {
	cw.Frame.Show(scr)
	cw.updateLines()

	x1, y1 := cw.X1+1, cw.Y1+1
	x2 := cw.X2 - 1

	attrBox := Palette[cw.Frame.ColorBoxIdx]
	NewPainter(scr).DrawLine(cw.X1+1, cw.Input.Y1-1, cw.X2-1, cw.Input.Y1-1, '─', attrBox, false, false)
	scr.Write(cw.X1, cw.Input.Y1-1, StringToCharInfo("├", attrBox))
	scr.Write(cw.X2, cw.Input.Y1-1, StringToCharInfo("┤", attrBox))

	label := cw.statusBarLabel()
	if label != "" {
		attr := Palette[cw.ColorStatusBarIdx]
		if cw.IsFocused() && cw.focusedLinkIdx == -2 {
			attr = Palette[cw.ColorLinkFocusIdx]
		}
		NewPainter(scr).DrawString(cw.X1+2, cw.Input.Y1-1, label, attr)
	}

	h := cw.Input.Y1 - cw.Y1 - 2
	if h > 0 {
		maxTop := len(cw.lines) - h
		if maxTop < 0 {
			maxTop = 0
		}
		if cw.topPos > maxTop {
			cw.topPos = maxTop
		}

		cw.visibleLinks = nil
		for i := 0; i < h; i++ {
			idx := cw.topPos + i
			if idx >= len(cw.lines) {
				break
			}
			line := cw.lines[idx]

			NewPainter(scr).Fill(x1, y1+i, x2, y1+i, ' ', Palette[cw.ColorTextIdx])

			col := 0
			for j := 0; j < len(line.cells); j++ {
				target := line.targets[j]

				if target != "" {
					startCol := col
					for j < len(line.cells) && line.targets[j] == target {
						if cw.IsFocused() && cw.focusedLinkIdx == len(cw.visibleLinks) {
							line.cells[j].Attributes = SetIndexBack(line.cells[j].Attributes, GetIndexBack(Palette[cw.ColorLinkFocusIdx]))
						}
						j++
						col++
					}
					cw.visibleLinks = append(cw.visibleLinks, ChatLink{
						Row: i, Col: startCol, Width: col - startCol, Target: target,
					})
					j-- // Step back since the outer loop will increment.
				} else {
					col++
				}
			}

			writeLen := len(line.cells)
			if writeLen > x2-x1+1 {
				writeLen = x2 - x1 + 1
			}
			scr.Write(x1, y1+i, line.cells[:writeLen])
		}
	}

	if cw.focusedLinkIdx >= len(cw.visibleLinks) {
		cw.focusedLinkIdx = -1
		if cw.IsFocused() {
			cw.Input.SetFocus(true)
		}
	}

	cw.Input.Show(scr)
}
