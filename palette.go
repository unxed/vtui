package vtui

// Palette indices (mapped to far2l's enum PaletteColors)
const (
	ColMenuText = iota
	ColMenuSelectedText
	ColMenuHighlight
	ColMenuSelectedHighlight
	ColMenuBox
	ColMenuTitle

	ColTableText
	ColTableSelectedText
	ColTableTitle
	ColTableBox
	ColTableColumnTitle

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

	ColDesktopBackground
	ColDialogEditUnchanged
	ColDialogEditSelected

	ColKeyBarNum
	ColKeyBarText
	ColMenuBarItem
	ColMenuBarSelected

	// Helper for array size
	LastPaletteColor
)

// Palette holds the current color attributes for all UI elements.
var Palette = make([]uint64, LastPaletteColor)

// SetDefaultPalette initializes the palette with standard Far Manager colors.
func SetDefaultPalette() {
	if len(Palette) < LastPaletteColor {
		Palette = make([]uint64, LastPaletteColor)
	}

	// Standard Far colors translated to 24-bit RGB for vtinput
	black := uint32(0x000000)
	white := uint32(0xFFFFFF)
	cyan := uint32(0x00A0A0)
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

	// Table (Neutral Defaults)
	Palette[ColTableText] = SetRGBBoth(0, lightGray, black)
	Palette[ColTableSelectedText] = SetRGBBoth(0, black, lightGray)
	Palette[ColTableTitle] = SetRGBBoth(0, white, black)
	Palette[ColTableBox] = SetRGBBoth(0, lightGray, black)
	Palette[ColTableColumnTitle] = SetRGBBoth(0, white, black)

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

	// Desktop
	Palette[ColDesktopBackground] = SetRGBBoth(0, lightGray, black)

	// KeyBar (Gray numbers on Black, Cyan text on Black)
	Palette[ColKeyBarNum] = SetRGBBoth(0, darkGray, black)
	Palette[ColKeyBarText] = SetRGBBoth(0, black, darkGray)

	// MenuBar (Black on LightGray, Green on LightGray for selection)
	Palette[ColMenuBarItem] = SetRGBBoth(0, black, lightGray)
	Palette[ColMenuBarSelected] = SetRGBBoth(0, black, 0x00FF00) // Green background
}
