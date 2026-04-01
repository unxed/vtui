package vtui

// Basic color and attribute constants (matching WinCompat.h)
const (
	IsFgRGB             uint64 = 0x0100 // Flag: Foreground is 24-bit RGB. If false, it's an 8-bit index.
	IsBgRGB             uint64 = 0x0200 // Flag: Background is 24-bit RGB. If false, it's an 8-bit index.

	ForegroundIntensity uint64 = 0x0008 // Retained for SGR Bold style
	BackgroundIntensity uint64 = 0x0080 // Retained for style flags

	ExplicitLineBreak   uint64 = 0x0400 // Don't concatenate next line if this char is last
	ImportantLineChar   uint64 = 0x0800 // Dont skip this character when recomposing

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
	return (attr & 0xFFFFFF000000FFFF) | ForegroundTrueColor | ((uint64(rgb) & 0xFFFFFF) << 16)
}

// SetRGBBack sets 24-bit RGB background color into attributes, adding BackgroundTrueColor flag.
func SetRGBBack(attr uint64, rgb uint32) uint64 {
	return (attr & 0x000000FFFFFFFFFF) | BackgroundTrueColor | ((uint64(rgb) & 0xFFFFFF) << 40)
}

// SetRGBBoth sets both RGB colors into attributes at once.
func SetRGBBoth(attr uint64, rgbFore uint32, rgbBack uint32) uint64 {
	return (attr & 0xFFFF) | ForegroundTrueColor | BackgroundTrueColor |
		((uint64(rgbFore) & 0xFFFFFF) << 16) | ((uint64(rgbBack) & 0xFFFFFF) << 40)
}// GetIndexFore extracts the 8-bit foreground index from attributes.
func GetIndexFore(attr uint64) uint8 {
	return uint8((attr >> 16) & 0xFF)
}

// GetIndexBack extracts the 8-bit background index from attributes.
func GetIndexBack(attr uint64) uint8 {
	return uint8((attr >> 40) & 0xFF)
}

// SetIndexFore sets the 8-bit foreground index, clearing the IsFgRGB flag.
func SetIndexFore(attr uint64, idx uint8) uint64 {
	return (attr & 0xFFFFFF000000FFFF) & ^IsFgRGB | (uint64(idx) << 16)
}

// SetIndexBack sets the 8-bit background index, clearing the IsBgRGB flag.
func SetIndexBack(attr uint64, idx uint8) uint64 {
	return (attr & 0x000000FFFFFFFFFF) & ^IsBgRGB | (uint64(idx) << 40)
}

// SetIndexBoth sets both foreground and background 8-bit indices at once.
func SetIndexBoth(attr uint64, idxFore, idxBack uint8) uint64 {
	return SetIndexBack(SetIndexFore(attr, idxFore), idxBack)
}
// DimColor reduces the brightness of the foreground color to visually indicate a disabled state.
func DimColor(attr uint64) uint64 {
	if attr&IsFgRGB != 0 {
		fg := GetRGBFore(attr)
		r, g, b := (fg>>16)&0xFF, (fg>>8)&0xFF, fg&0xFF
		return SetRGBFore(attr, (r/2)<<16|(g/2)<<8|(b/2))
	}
	return SetIndexFore(attr, 8) // 8 is DarkGray in standard ANSI
}
