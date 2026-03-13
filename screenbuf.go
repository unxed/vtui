package vtui

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// ScreenBuf is a complete analog of far2l/scrbuf.cpp.
// It implements double buffering to minimize terminal write operations.
type ScreenBuf struct {
	mu            sync.Mutex
	buf           []CharInfo // 'buf' is the target screen state formed by UI logic.
	shadow        []CharInfo // 'shadow' is the state last rendered in the terminal.
	width, height int

	cursorX, cursorY int
	cursorVisible    bool
	cursorSize       uint32

	lockCount int
	dirty     bool // Flag indicating that a full rewrite is required during the next Flush.
}

// NewScreenBuf creates a new ScreenBuf instance.
func NewScreenBuf() *ScreenBuf {
	return &ScreenBuf{
		dirty: true, // Initially the buffer is "dirty"
	}
}

// AllocBuf allocates or reallocates memory for the screen buffers.
func (s *ScreenBuf) AllocBuf(width, height int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if width == s.width && height == s.height {
		return
	}

	if width <= 0 || height <= 0 {
		s.buf = nil
		s.shadow = nil
		s.width = 0
		s.height = 0
		return
	}

	size := width * height
	newBuf := make([]CharInfo, size)
	newShadow := make([]CharInfo, size)

	if newBuf == nil || newShadow == nil {
		// In Go it is customary to return an error, but for a critical error such as
		// running out of memory for the screen, a panic is justified and matches
		// the behavior of far2l (abort).
		panic(fmt.Sprintf("FATAL: Failed to allocate screen buffer (%d x %d)", width, height))
	}

	s.buf = newBuf
	s.shadow = newShadow
	s.width = width
	s.height = height
	s.dirty = true // After resizing, a full redraw is needed
}

// Write writes a slice of CharInfo into the virtual buffer at specified coordinates.
func (s *ScreenBuf) Write(x, y int, text []CharInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.buf == nil || y < 0 || y >= s.height || x >= s.width {
		return
	}

	// Clipping behind the left boundary
	if x < 0 {
		if -x >= len(text) {
			return
		}
		text = text[-x:]
		x = 0
	}

	// Clipping behind the right boundary
	if x+len(text) > s.width {
		text = text[:s.width-x]
	}

	if len(text) == 0 {
		return
	}

	offset := y*s.width + x
	copy(s.buf[offset:], text)
	// Note: not comparing with shadow yet, just copying.
	// Comparison optimization will happen in Flush().
}

// ApplyColor applies specified attributes to a rectangular area.
func (s *ScreenBuf) ApplyColor(x1, y1, x2, y2 int, attributes uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.buf == nil {
		return
	}

	// Clipping by screen boundaries
	if x1 < 0 { x1 = 0 }
	if y1 < 0 { y1 = 0 }
	if x2 >= s.width { x2 = s.width - 1 }
	if y2 >= s.height { y2 = s.height - 1 }

	for y := y1; y <= y2; y++ {
		offset := y*s.width + x1
		for x := 0; x <= x2-x1; x++ {
			s.buf[offset+x].Attributes = attributes
		}
	}
}

// FillRect fills a rectangular area with specified character and attributes.
func (s *ScreenBuf) FillRect(x1, y1, x2, y2 int, char rune, attributes uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.buf == nil { return }
	if x1 < 0 { x1 = 0 }
	if y1 < 0 { y1 = 0 }
	if x2 >= s.width { x2 = s.width - 1 }
	if y2 >= s.height { y2 = s.height - 1 }
	cell := CharInfo{Char: uint64(char), Attributes: attributes}
	for y := y1; y <= y2; y++ {
		offset := y*s.width + x1
		for x := 0; x <= x2-x1; x++ {
			s.buf[offset+x] = cell
		}
	}
}

func (s *ScreenBuf) SetCursorPos(x, y int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cursorX, s.cursorY = x, y
}

func (s *ScreenBuf) SetCursorVisible(visible bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cursorVisible = visible
}


// Tables for quickly converting palettes to ANSI codes.
var (
	ansiFg = []string{"30", "34", "32", "36", "31", "35", "33", "37"}
	ansiBg = []string{"40", "44", "42", "46", "41", "45", "43", "47"}
)

// rgb extracts R, G, B components from 24-bit color (format 0xRRGGBB).
func rgb(c uint32) (r, g, b byte) {
	return byte((c >> 16) & 0xFF), byte((c >> 8) & 0xFF), byte(c & 0xFF)
}

// attributesToANSI generates the minimum ANSI sequence to transition from lastAttr to attr.
func attributesToANSI(attr, lastAttr uint64) string {
	if attr == lastAttr {
		return ""
	}

	var params []string

	// 1. Checking for flag changes (underscore, inversion)
	const simpleFlags = CommonLvbUnderscore | CommonLvbReverse
	if (attr & simpleFlags) != (lastAttr & simpleFlags) {
		params = append(params, "0")
		if attr&CommonLvbUnderscore != 0 { params = append(params, "4") }
		if attr&CommonLvbReverse != 0 { params = append(params, "7") }
	}

	// 2. Text color (Foreground)
	fgChanged := false
	if (attr & ForegroundTrueColor) != (lastAttr & ForegroundTrueColor) {
		fgChanged = true
	} else if attr&ForegroundTrueColor != 0 {
		if GetRGBFore(attr) != GetRGBFore(lastAttr) { fgChanged = true }
	} else if (attr & (ForegroundRGB | ForegroundIntensity)) != (lastAttr & (ForegroundRGB | ForegroundIntensity)) {
		fgChanged = true
	}

	if fgChanged {
		if attr&ForegroundTrueColor != 0 {
			r, g, b := rgb(GetRGBFore(attr))
			params = append(params, fmt.Sprintf("38;2;%d;%d;%d", r, g, b))
		} else {
			fg := attr & 0b1111
			if fg&ForegroundIntensity != 0 {
				params = append(params, "1", ansiFg[fg&0b0111])
			} else {
				params = append(params, "22", ansiFg[fg])
			}
		}
	}

	// 3. Background
	bgChanged := false
	if (attr & BackgroundTrueColor) != (lastAttr & BackgroundTrueColor) {
		bgChanged = true
	} else if attr&BackgroundTrueColor != 0 {
		if GetRGBBack(attr) != GetRGBBack(lastAttr) { bgChanged = true }
	} else if (attr & (BackgroundRGB | BackgroundIntensity)) != (lastAttr & (BackgroundRGB | BackgroundIntensity)) {
		bgChanged = true
	}

	if bgChanged {
		if attr&BackgroundTrueColor != 0 {
			r, g, b := rgb(GetRGBBack(attr))
			params = append(params, fmt.Sprintf("48;2;%d;%d;%d", r, g, b))
		} else {
			bg := (attr >> 4) & 0b1111
			params = append(params, ansiBg[bg&0b0111])
		}
	}

	if len(params) == 0 {
		return ""
	}

	return "\x1b[" + strings.Join(params, ";") + "m"
}

// Flush compares `buf` and `shadow` and outputs the difference to the terminal.
func (s *ScreenBuf) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.lockCount > 0 || s.buf == nil {
		return
	}

	var builder strings.Builder

	// 1. Hide the cursor to avoid flickering during rendering.
	builder.WriteString("\x1b[?25l")

	lastAttr := ^uint64(0) // Invalid value so the first cell always sets the color.
	lastX, lastY := -1, -1

	// 2. Main comparison and sequence generation loop.
	for y := 0; y < s.height; y++ {
		for x := 0; x < s.width; x++ {
			offset := y*s.width + x

			// If cell hasn't changed, skip it.
			if !s.dirty && s.buf[offset] == s.shadow[offset] {
				continue
			}

			// If there is a gap, move the cursor.
			if x != lastX+1 || y != lastY {
				// ANSI coordinates start from 1.
				builder.WriteString(fmt.Sprintf("\x1b[%d;%dH", y+1, x+1))
			}

			// Set color if changed.
			attr := s.buf[offset].Attributes
			builder.WriteString(attributesToANSI(attr, lastAttr))
			lastAttr = attr

			// Output symbol.
			// TODO: Handle composite symbols (Char > 0xFFFF)
			char := rune(s.buf[offset].Char)
			if char == 0 {
				builder.WriteByte(' ') // Render null character as space
			} else {
				builder.WriteRune(char)
			}

			lastX, lastY = x, y
		}
	}
	s.dirty = false
	copy(s.shadow, s.buf)

	// 3. Move cursor to final position and make visible if needed.
	builder.WriteString(fmt.Sprintf("\x1b[%d;%dH", s.cursorY+1, s.cursorX+1))
	if s.cursorVisible {
		builder.WriteString("\x1b[?25h")
	}

	// 4. Single write to stdout.
	if builder.Len() > 0 {
		os.Stdout.WriteString(builder.String())
	}
}