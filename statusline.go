package vtui

import "github.com/mattn/go-runewidth"

// StatusItem represents a single hotkey hint in the StatusLine.
type StatusItem struct {
	Key   string
	Label string
}

// StatusLine provides context-sensitive hotkey hints at the bottom of the screen.
// Analog of TStatusLine from Turbo Vision.
type StatusLine struct {
	Bar
	Items        map[string][]StatusItem
	Default      []StatusItem
	currentTopic string
}

func NewStatusLine() *StatusLine {
	return &StatusLine{
		Items: make(map[string][]StatusItem),
	}
}

func (sl *StatusLine) Show(scr *ScreenBuf) {
	if sl.IsLocked() {
		return
	}
	sl.ScreenObject.Show(scr)
	sl.DisplayObject(scr)
}

func (sl *StatusLine) DisplayObject(scr *ScreenBuf) {
	if !sl.IsVisible() {
		return
	}

	topic := sl.currentTopic
	items, ok := sl.Items[topic]
	if !ok {
		items = sl.Default
	}

	numAttr := Palette[ColKeyBarNum]
	textAttr := Palette[ColKeyBarText]

	sl.DrawBackground(scr, textAttr)

	currX := sl.X1
	for _, item := range items {
		// Draw Key
		keyCells := StringToCharInfo(item.Key, numAttr)
		scr.Write(currX, sl.Y1, keyCells)
		currX += runewidth.StringWidth(item.Key)

		// Draw Label
		labelCells := StringToCharInfo(item.Label, textAttr)
		scr.Write(currX, sl.Y1, labelCells)
		currX += runewidth.StringWidth(item.Label)

		// Add a space separator
		if currX <= sl.X2 {
			scr.Write(currX, sl.Y1, []CharInfo{{Char: ' ', Attributes: textAttr}})
			currX++
		}
	}
}

// UpdateContext changes the active topic and redraws if necessary.
func (sl *StatusLine) UpdateContext(topic string) {
	if sl.currentTopic != topic {
		sl.currentTopic = topic
	}
}