package vtui

// Palette indices (mapped to far2l's enum PaletteColors)
const (
	ColMenuText = iota
	ColMenuSelectedText
	ColMenuHighlight
	ColMenuSelectedHighlight
	ColMenuBox
	ColMenuTitle

	ColPanelText
	ColPanelSelectedText
	ColPanelHighlightText
	ColPanelInfoText
	ColPanelCursor
	ColPanelSelectedCursor
	ColPanelTitle
	ColPanelSelectedTitle
	ColPanelColumnTitle
	ColPanelTotalInfo
	ColPanelSelectedInfo

	ColDialogText
	ColDialogHighlightText
	ColDialogBox
	ColDialogBoxTitle
	ColDialogHighlightBoxTitle
	ColDialogEdit
	ColDialogButton
	ColDialogSelectedButton
	ColDialogHighlightButton
	ColDialogHighlightSelectedButton

	ColCommandLineUserScreen
	ColDialogEditUnchanged
	ColDialogEditSelected

	ColPanelBox
	ColPanelScrollbar

	ColKeyBarNum
	ColKeyBarText
	ColCommandLinePrompt
	ColCommandLineText
	ColMenuBarItem
	ColMenuBarSelected

	// Helper for array size
	LastPaletteColor
)

// Palette holds the current color attributes for all UI elements.
var Palette [LastPaletteColor]uint64

// SetDefaultPalette initializes the palette with standard Far Manager colors.
func SetDefaultPalette() {
	// Standard Far colors translated to 24-bit RGB for vtinput
	black := uint32(0x000000)
	white := uint32(0xFFFFFF)
	cyan := uint32(0x00A0A0)
	blue := uint32(0x0000A0)
	yellow := uint32(0xFFFF00)
	lightGray := uint32(0xC0C0C0)
	darkGray := uint32(0x808080)

	// Menus (White on Cyan)
	Palette[ColMenuText] = SetRGBBoth(0, white, cyan)
	Palette[ColMenuSelectedText] = SetRGBBoth(0, white, black)
	Palette[ColMenuHighlight] = SetRGBBoth(0, yellow, cyan)
	Palette[ColMenuSelectedHighlight] = SetRGBBoth(0, yellow, black)
	Palette[ColMenuBox] = SetRGBBoth(0, white, cyan)
	Palette[ColMenuTitle] = SetRGBBoth(0, white, cyan)

	// Panels (LightCyan on Blue)
	Palette[ColPanelText] = SetRGBBoth(0, 0x00FFFF, blue)
	Palette[ColPanelSelectedText] = SetRGBBoth(0, yellow, blue)
	Palette[ColPanelCursor] = SetRGBBoth(0, black, cyan)
	Palette[ColPanelSelectedCursor] = SetRGBBoth(0, yellow, cyan)
	Palette[ColPanelBox] = SetRGBBoth(0, 0x00FFFF, blue)
	Palette[ColPanelColumnTitle] = SetRGBBoth(0, yellow, blue)

	// Dialogs (Black on LightGray)
	Palette[ColDialogText] = SetRGBBoth(0, black, lightGray)
	Palette[ColDialogBox] = SetRGBBoth(0, black, lightGray)
	Palette[ColDialogBoxTitle] = SetRGBBoth(0, black, lightGray)

	// Dialog Edits (Black on Cyan)
	Palette[ColDialogEdit] = SetRGBBoth(0, black, cyan)
	Palette[ColDialogEditSelected] = SetRGBBoth(0, white, black)
	Palette[ColDialogEditUnchanged] = SetRGBBoth(0, darkGray, cyan)

	// Dialog Buttons
	Palette[ColDialogButton] = SetRGBBoth(0, black, lightGray)
	Palette[ColDialogSelectedButton] = SetRGBBoth(0, black, cyan)

	// Background
	Palette[ColCommandLineUserScreen] = SetRGBBoth(0, lightGray, black)

	// KeyBar (Gray numbers on Black, Cyan text on Black)
	Palette[ColKeyBarNum] = SetRGBBoth(0, darkGray, black)
	Palette[ColKeyBarText] = SetRGBBoth(0, black, darkGray)

	// CommandLine (White on Black)
	Palette[ColCommandLinePrompt] = SetRGBBoth(0, 0x00FFFF, black)
	Palette[ColCommandLineText] = SetRGBBoth(0, white, black)

	// MenuBar (Black on LightGray, Green on LightGray for selection)
	Palette[ColMenuBarItem] = SetRGBBoth(0, black, lightGray)
	Palette[ColMenuBarSelected] = SetRGBBoth(0, black, 0x00FF00) // Green background
}
