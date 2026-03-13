package vtui

// Coord defines the coordinates in the console.
type Coord struct {
	X int16
	Y int16
}

// SmallRect defines a rectangular area in the console.
type SmallRect struct {
	Left   int16
	Top    int16
	Right  int16
	Bottom int16
}

// CharInfo contains a character and its visual attributes (including colors).
// In far2l, Char (UnicodeChar) is uint64 (COMP_CHAR) to support composite characters.
// Let's use the same bit length.
type CharInfo struct {
	Char       uint64 // Equivalent to union with COMP_CHAR UnicodeChar
	Attributes uint64 // DWORD64 Equivalent Attributes (lower 16 bits are flags, 16-39 are Fore RGB, 40-63 are Back RGB)
}