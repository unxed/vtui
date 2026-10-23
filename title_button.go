package vtui

import "strconv"

// Title-bar buttons (f4 #285).
//
// The close button, the zoom button and the window number all sit on the top
// border of a window or dialog. They used to be drawn in four places, each
// wrapping its symbol in UIStrings.CloseBrackets on its own, so a style change
// had to be repeated everywhere (and was forgotten for the Help window, which
// still draws its own). TitleButtonText is the one place that decides how such
// a button looks; Painter.DrawTitleButton is the one place that puts it on the
// screen.
//
// A title button always occupies exactly 3 cells whatever the style, so the
// hit areas that BaseWindow.ProcessMouse computes from the border offsets and
// the frame's layout never move:
//
//   - classic style (and every text terminal, whatever style the application
//     selected): "[x]", the brackets taken from UIStrings.CloseBrackets;
//   - rounded style on a graphical backend (roundedButtonsActive): " x ", the
//     brackets replaced by blank cells in the same colours.

// TitleButtonText returns the 3-cell text a title-bar button with the given
// 1-cell label is drawn with in the current style. Hosts that paint their own
// decorations on a window border (the Help window) use it, or
// Painter.DrawTitleButton, instead of wrapping the label in brackets
// themselves.
func TitleButtonText(label string) string {
	if roundedButtonsActive() {
		return " " + label + " "
	}
	return string(UIStrings.CloseBrackets[0]) + label + string(UIStrings.CloseBrackets[1])
}

// DrawTitleButton draws a title-bar button with the given label at (x, y); the
// button occupies 3 cells starting at x. attr applies to all of them.
func (p *Painter) DrawTitleButton(x, y int, label string, attr uint64) {
	p.scr.Write(x, y, StringToCharInfo(TitleButtonText(label), attr))
}

// DrawTitleButtonWithHover draws a title-bar button and applies the subtle
// graphical hover tint when hovered. The button remains exactly three cells
// wide; text terminals and the classic style keep their existing appearance.
func (p *Painter) DrawTitleButtonWithHover(x, y int, label string, attr uint64, hovered bool) {
	if hovered && roundedButtonsActive() {
		attr = roundedTitleButtonHoverAttr(attr)
	}
	p.DrawTitleButton(x, y, label, attr)
}

// DrawZoomButton draws the zoom button of a window at (x, y).
func (p *Painter) DrawZoomButton(x, y int, attr uint64) {
	p.DrawTitleButton(x, y, string(UIStrings.ZoomSymbol), attr)
}

// DrawZoomButtonWithHover draws the zoom control with its hover state.
func (p *Painter) DrawZoomButtonWithHover(x, y int, attr uint64, hovered bool) {
	p.DrawTitleButtonWithHover(x, y, string(UIStrings.ZoomSymbol), attr, hovered)
}

// DrawWindowNumber draws the number of a workspace window (1..9) at (x, y),
// framed like a title button.
func (p *Painter) DrawWindowNumber(x, y, number int, attr uint64) {
	p.DrawTitleButton(x, y, strconv.Itoa(number), attr)
}
