package vtui

// Basic color and attribute constants (matching WinCompat.h)
const (
	IsFgRGB uint64 = 0x0100 // Flag: Foreground is 24-bit RGB. If false, it's an 8-bit index.
	IsBgRGB uint64 = 0x0200 // Flag: Background is 24-bit RGB. If false, it's an 8-bit index.

	ForegroundIntensity uint64 = 0x0008 // Retained for SGR Bold style
	BackgroundIntensity uint64 = 0x0080 // Retained for style flags

	// ForegroundDefault / BackgroundDefault mark a side as "the terminal's own
	// default colour" (SGR 39 / 49) instead of a palette index or RGB value. The
	// index the setters leave beside the flag (7 for the foreground, 0 for the
	// background) is what everything that cannot express "default" -- the GUI
	// renderers, colour maths, ThemePalette lookups -- keeps reading, so only
	// the ANSI writer treats the flag specially. Every other colour setter
	// clears it. Bits 0x0010 and 0x0020 were never used by the attribute.
	ForegroundDefault uint64 = 0x0010
	BackgroundDefault uint64 = 0x0020

	ExplicitLineBreak uint64 = 0x0400 // Don't concatenate next line if this char is last
	ImportantLineChar uint64 = 0x0800 // Dont skip this character when recomposing

	ForegroundDim       uint64 = 0x1000 // Extra flag for dim text
	CommonLvbStrikeout  uint64 = 0x2000 // Strikeout.
	CommonLvbReverse    uint64 = 0x4000 // Reverse fore/back ground attribute.
	CommonLvbUnderscore uint64 = 0x8000 // Underscore.

	// Deprecated aliases for compatibility
	ForegroundTrueColor = IsFgRGB
	BackgroundTrueColor = IsBgRGB
)

// GetRGBFore extracts 24-bit RGB text color from attributes (bits 16-39).
func GetRGBFore(attr uint64) uint32 {
	return uint32((attr >> 16) & 0xFFFFFF)
}

// GetRGBBack extracts 24-bit RGB background color from attributes (bits 40-63).
func GetRGBBack(attr uint64) uint32 {
	return uint32((attr >> 40) & 0xFFFFFF)
}

// SetRGBFore sets 24-bit RGB text color into attributes, adding ForegroundTrueColor flag.
func SetRGBFore(attr uint64, rgb uint32) uint64 {
	return (attr&0xFFFFFF000000FFFF)&^ForegroundDefault | ForegroundTrueColor | ((uint64(rgb) & 0xFFFFFF) << 16)
}

// SetRGBBack sets 24-bit RGB background color into attributes, adding BackgroundTrueColor flag.
func SetRGBBack(attr uint64, rgb uint32) uint64 {
	return (attr&0x000000FFFFFFFFFF)&^BackgroundDefault | BackgroundTrueColor | ((uint64(rgb) & 0xFFFFFF) << 40)
}

// SetRGBBoth sets both RGB colors into attributes at once.
func SetRGBBoth(attr uint64, rgbFore uint32, rgbBack uint32) uint64 {
	return (attr&0xFFFF)&^(ForegroundDefault|BackgroundDefault) | ForegroundTrueColor | BackgroundTrueColor |
		((uint64(rgbFore) & 0xFFFFFF) << 16) | ((uint64(rgbBack) & 0xFFFFFF) << 40)
} // GetIndexFore extracts the 8-bit foreground index from attributes.
func GetIndexFore(attr uint64) uint8 {
	return uint8((attr >> 16) & 0xFF)
}

// GetIndexBack extracts the 8-bit background index from attributes.
func GetIndexBack(attr uint64) uint8 {
	return uint8((attr >> 40) & 0xFF)
}

// SetIndexFore sets the 8-bit foreground index, clearing the IsFgRGB flag.
func SetIndexFore(attr uint64, idx uint8) uint64 {
	return (attr&0xFFFFFF000000FFFF) & ^(IsFgRGB|ForegroundDefault) | (uint64(idx) << 16)
}

// SetIndexBack sets the 8-bit background index, clearing the IsBgRGB flag.
func SetIndexBack(attr uint64, idx uint8) uint64 {
	return (attr&0x000000FFFFFFFFFF) & ^(IsBgRGB|BackgroundDefault) | (uint64(idx) << 40)
}

// SetDefaultFore makes the foreground the terminal's default colour (SGR 39).
// Where "default" cannot be expressed it reads as palette index 7.
func SetDefaultFore(attr uint64) uint64 {
	return SetIndexFore(attr, 7) | ForegroundDefault
}

// SetDefaultBack makes the background the terminal's default colour (SGR 49).
// Where "default" cannot be expressed it reads as palette index 0.
func SetDefaultBack(attr uint64) uint64 {
	return SetIndexBack(attr, 0) | BackgroundDefault
}

// SetIndexBoth sets both foreground and background 8-bit indices at once.
func SetIndexBoth(attr uint64, idxFore, idxBack uint8) uint64 {
	return SetIndexBack(SetIndexFore(attr, idxFore), idxBack)
}

// InvertColors swaps the foreground and background colors of the attribute,
// preserving each color's mode (palette index vs 24-bit RGB). Style flags
// (bold, underscore, etc.) are kept as-is.
func InvertColors(attr uint64) uint64 {
	fgIdx, bgIdx := GetIndexFore(attr), GetIndexBack(attr)
	fgRGB, bgRGB := GetRGBFore(attr), GetRGBBack(attr)
	fgIsRGB := attr&IsFgRGB != 0
	bgIsRGB := attr&IsBgRGB != 0
	fgDefault := attr&ForegroundDefault != 0
	bgDefault := attr&BackgroundDefault != 0
	if bgIsRGB {
		attr = SetRGBFore(attr, bgRGB)
	} else {
		attr = SetIndexFore(attr, bgIdx)
	}
	if fgIsRGB {
		attr = SetRGBBack(attr, fgRGB)
	} else {
		attr = SetIndexBack(attr, fgIdx)
	}
	// "Default" travels with the side it was on: the inverted foreground is
	// the old background's default, and the other way round.
	if bgDefault {
		attr |= ForegroundDefault
	}
	if fgDefault {
		attr |= BackgroundDefault
	}
	return attr
}

// DimColor shows a disabled state by moving the foreground half way to the
// background, so it fades whichever way round the colours are. Halving the
// foreground alone made dark text on a light dialog (the usual button) nearly
// unchanged, so a disabled button looked as live as an enabled one (f4#918).
// Without an RGB background to blend with, an RGB foreground is halved and an
// indexed one becomes DarkGray, or LightGray over a DarkGray background.
func DimColor(attr uint64) uint64 {
	if attr&IsFgRGB != 0 {
		fg := GetRGBFore(attr)
		r, g, b := (fg>>16)&0xFF, (fg>>8)&0xFF, fg&0xFF
		if attr&IsBgRGB != 0 {
			bg := GetRGBBack(attr)
			br, bgG, bb := (bg>>16)&0xFF, (bg>>8)&0xFF, bg&0xFF
			return SetRGBFore(attr, ((r+br)/2)<<16|((g+bgG)/2)<<8|(b+bb)/2)
		}
		return SetRGBFore(attr, (r/2)<<16|(g/2)<<8|(b/2))
	}
	if attr&IsBgRGB == 0 && GetIndexBack(attr) == 8 {
		return SetIndexFore(attr, 7)
	}
	return SetIndexFore(attr, 8) // 8 is DarkGray in standard ANSI
}

// DialogIndicatorAttr changes only the background of the three-character mark.
// Focus retains the normal selection palette; zero means legacy inheritance.
func DialogIndicatorAttr(normal uint64, focused bool) uint64 {
	background := Palette[ColDialogIndicatorBackground]
	if focused || background == 0 {
		return normal
	}
	if background&IsBgRGB != 0 {
		return SetRGBBack(normal, GetRGBBack(background))
	}
	return SetIndexBack(normal, GetIndexBack(background))
}
