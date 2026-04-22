package vtui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
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
	cursorDirty      bool

	lockCount int
	dirty     bool // Flag indicating that a full rewrite is required during the next Flush.
	clipStack []Rect

	OverlayMode      bool
	ThemePalette     *[256]uint32
	ActivePalette    *[256]uint32
	Force256Colors   bool

	HostPalette      [256]uint32
	HostPaletteValid [256]bool
	quantCache       map[uint32]uint8

	Renderer SurfaceRenderer
	Writer   io.Writer // Output destination, defaults to os.Stdout
}

// NewScreenBuf creates a new ScreenBuf instance.
func NewScreenBuf() *ScreenBuf {
	s := &ScreenBuf{
		dirty: true,
	}
	s.Renderer = &AnsiRenderer{parent: s}
	return s
}

// NewSilentScreenBuf creates a ScreenBuf that discards all output.
// Ideal for unit tests to prevent ANSI sequences from polluting the console.
func NewSilentScreenBuf() *ScreenBuf {
	return &ScreenBuf{
		dirty:  true,
		Writer: io.Discard,
	}
}

// HardReset clears the shadow buffer and forces a complete redraw of the screen.
// Essential when re-attaching to a new physical terminal.
func (s *ScreenBuf) HardReset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.shadow {
		s.shadow[i] = CharInfo{}
	}
	s.dirty = true
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
	s.clipStack = []Rect{{0, 0, width - 1, height - 1}}
}

// PushClipRect adds a new clipping rectangle by intersecting it with the current one.
func (s *ScreenBuf) PushClipRect(x1, y1, x2, y2 int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.clipStack) == 0 {
		if s.width <= 0 || s.height <= 0 {
			return
		}
		s.clipStack = []Rect{{0, 0, s.width - 1, s.height - 1}}
	}
	curr := s.clipStack[len(s.clipStack)-1]
	nx1, ny1 := max(curr.X1, x1), max(curr.Y1, y1)
	nx2, ny2 := min(curr.X2, x2), min(curr.Y2, y2)
	s.clipStack = append(s.clipStack, Rect{nx1, ny1, nx2, ny2})
}
// SetOverlayMode enables or disables Early Binding of indexed colors to RGB.
func (s *ScreenBuf) SetOverlayMode(overlay bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.OverlayMode = overlay
}

// PopClipRect removes the top clipping rectangle.
func (s *ScreenBuf) PopClipRect() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.clipStack) > 1 {
		s.clipStack = s.clipStack[:len(s.clipStack)-1]
	}
}
// ApplyShadow applies a semi-transparent shadow effect to the specified area.
func (s *ScreenBuf) ApplyShadow(x1, y1, x2, y2 int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.buf == nil || len(s.clipStack) == 0 { return }
	clip := s.clipStack[len(s.clipStack)-1]
	if x1 < clip.X1 { x1 = clip.X1 }
	if y1 < clip.Y1 { y1 = clip.Y1 }
	if x2 > clip.X2 { x2 = clip.X2 }
	if y2 > clip.Y2 { y2 = clip.Y2 }
	if x1 > x2 || y1 > y2 { return }

	for y := y1; y <= y2; y++ {
		offset := y*s.width + x1
		for x := 0; x <= x2-x1; x++ {
			attr := s.buf[offset+x].Attributes
			
			var bg uint32
			if attr&IsBgRGB != 0 {
				bg = GetRGBBack(attr)
			} else {
				idx := GetIndexBack(attr)
				if s.ThemePalette != nil { bg = s.ThemePalette[idx] } else { bg = XTerm256Palette[idx] }
			}
			
			var fg uint32
			if attr&IsFgRGB != 0 {
				fg = GetRGBFore(attr)
			} else {
				idx := GetIndexFore(attr)
				if s.ThemePalette != nil { fg = s.ThemePalette[idx] } else { fg = XTerm256Palette[idx] }
			}
			
			bg = ((bg>>16&0xFF)/2)<<16 | ((bg>>8&0xFF)/2)<<8 | ((bg&0xFF)/2)
			fg = ((fg>>16&0xFF)/2)<<16 | ((fg>>8&0xFF)/2)<<8 | ((fg&0xFF)/2)
			
			s.buf[offset+x].Attributes = SetRGBBoth(attr, fg, bg)
		}
	}
}

// Write writes a slice of CharInfo into the virtual buffer at specified coordinates.
func (s *ScreenBuf) Write(x, y int, text []CharInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.buf == nil || len(s.clipStack) == 0 {
		return
	}

	clip := s.clipStack[len(s.clipStack)-1]
	if y < clip.Y1 || y > clip.Y2 || x > clip.X2 {
		return
	}

	if x < clip.X1 {
		skip := clip.X1 - x
		if skip >= len(text) {
			return
		}
		text = text[skip:]
		x = clip.X1
	}

	if x+len(text)-1 > clip.X2 {
		text = text[:clip.X2-x+1]
	}

	if len(text) == 0 {
		return
	}

	offset := y*s.width + x
	if s.OverlayMode && s.ThemePalette != nil {
		for i, ci := range text {
			s.buf[offset+i] = CharInfo{Char: ci.Char, Attributes: s.resolveAttr(ci.Attributes)}
		}
	} else {
		copy(s.buf[offset:], text)
	}
	// Note: not comparing with shadow yet, just copying.
	// Comparison optimization will happen in Flush().
}

// resolveAttr applies OverlayMode palette resolution to the given attribute.
func (s *ScreenBuf) resolveAttr(attr uint64) uint64 {
	if s.OverlayMode && s.ThemePalette != nil {
		if attr&IsFgRGB == 0 {
			idx := GetIndexFore(attr)
			attr = SetRGBFore(attr, s.ThemePalette[idx])
		}
		if attr&IsBgRGB == 0 {
			idx := GetIndexBack(attr)
			attr = SetRGBBack(attr, s.ThemePalette[idx])
		}
	}
	return attr
}
// ApplyColor applies specified attributes to a rectangular area.
func (s *ScreenBuf) ApplyColor(x1, y1, x2, y2 int, attributes uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.buf == nil {
		return
	}

	if len(s.clipStack) == 0 { return }
	clip := s.clipStack[len(s.clipStack)-1]
	if x1 < clip.X1 { x1 = clip.X1 }
	if y1 < clip.Y1 { y1 = clip.Y1 }
	if x2 > clip.X2 { x2 = clip.X2 }
	if y2 > clip.Y2 { y2 = clip.Y2 }
	if x1 > x2 || y1 > y2 { return }

	attributes = s.resolveAttr(attributes)
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
	if s.buf == nil || len(s.clipStack) == 0 { return }
	if x1 > x2 || y1 > y2 { return }

	clip := s.clipStack[len(s.clipStack)-1]
	if x1 < clip.X1 { x1 = clip.X1 }
	if y1 < clip.Y1 { y1 = clip.Y1 }
	if x2 > clip.X2 { x2 = clip.X2 }
	if y2 > clip.Y2 { y2 = clip.Y2 }
	if x1 > x2 || y1 > y2 { return }

	attributes = s.resolveAttr(attributes)
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
	if s.cursorX != x || s.cursorY != y {
		s.cursorX, s.cursorY = x, y
		s.cursorDirty = true
	}
}

func (s *ScreenBuf) SetCursorVisible(visible bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cursorVisible != visible {
		s.cursorVisible = visible
		s.cursorDirty = true
	}
}

func (s *ScreenBuf) Width() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.width
}

func (s *ScreenBuf) Height() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.height
}
// GetCell returns the character and attributes at the specified coordinates.
// Used primarily for unit tests.
func (s *ScreenBuf) GetCell(x, y int) CharInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	if x < 0 || x >= s.width || y < 0 || y >= s.height {
		return CharInfo{}
	}
	return s.buf[y*s.width+x]
}




// rgb extracts R, G, B components from 24-bit color (format 0xRRGGBB).
func rgb(c uint32) (r, g, b byte) {
	return byte((c >> 16) & 0xFF), byte((c >> 8) & 0xFF), byte(c & 0xFF)
}
// Flush синхронизирует состояние виртуального буфера с физическим экраном через Renderer.
func (s *ScreenBuf) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.lockCount > 0 || s.buf == nil || s.Renderer == nil {
		return
	}

	var activePal *[256]uint32
	if s.ActivePalette != nil {
		activePal = s.ActivePalette
	} else if s.ThemePalette != nil {
		activePal = s.ThemePalette
	}

	s.Renderer.SetPalette(activePal)
	s.Renderer.SetCursor(s.cursorX, s.cursorY, s.cursorVisible)
	s.Renderer.Render(s.buf, s.shadow, s.width, s.height, s.dirty || s.cursorDirty)
	s.Renderer.Flush()

	s.dirty = false
	s.cursorDirty = false
	copy(s.shadow, s.buf)
}

// AnsiRenderer реализует SurfaceRenderer через ESC-последовательности.
type AnsiRenderer struct {
	parent   *ScreenBuf
	lastAttr uint64
	frameOut strings.Builder
}

func (r *AnsiRenderer) SetPalette(pal *[256]uint32) {
	if pal == nil {
		return
	}
	for i := 0; i < 256; i++ {
		if !r.parent.HostPaletteValid[i] || r.parent.HostPalette[i] != pal[i] {
			r.parent.quantCache = make(map[uint32]uint8)
			pr, pg, pb := rgb(pal[i])
			r.frameOut.WriteString(fmt.Sprintf("\x1b]4;%d;rgb:%02x/%02x/%02x\x07", i, pr, pg, pb))
			r.parent.HostPalette[i] = pal[i]
			r.parent.HostPaletteValid[i] = true
		}
	}
}

func (r *AnsiRenderer) Render(buf, shadow []CharInfo, w, h int, force bool) {
	r.frameOut.WriteString("\x1b[?25l") // Hide cursor during draw

	lastX, lastY := -1, -1
	r.lastAttr = ^uint64(0)

	var activePal *[256]uint32
	if r.parent.ActivePalette != nil {
		activePal = r.parent.ActivePalette
	} else {
		activePal = r.parent.ThemePalette
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			idx := y*w + x
			if !force && buf[idx] == shadow[idx] {
				continue
			}

			if x != lastX+1 || y != lastY {
				r.frameOut.WriteString(fmt.Sprintf("\x1b[%d;%dH", y+1, x+1))
			}

			attr := buf[idx].Attributes
			r.frameOut.WriteString(attributesToANSI(attr, r.lastAttr, activePal, r.parent.Force256Colors, r.parent.quantCache))
			r.lastAttr = attr

			char := buf[idx].Char
			if char == WideCharFiller {
				lastX, lastY = x, y
				continue
			}
			if char == 0 {
				r.frameOut.WriteByte(' ')
			} else {
				r.frameOut.WriteRune(rune(char))
			}
			lastX, lastY = x, y
		}
	}
}

func (r *AnsiRenderer) SetCursor(x, y int, vis bool) {
	r.frameOut.WriteString(fmt.Sprintf("\x1b[%d;%dH", y+1, x+1))
	if vis {
		r.frameOut.WriteString("\x1b[?25h")
	} else {
		r.frameOut.WriteString("\x1b[?25l")
	}
}

func (r *AnsiRenderer) Flush() {
	payload := r.frameOut.String()
	r.frameOut.Reset()
	if payload == "" {
		return
	}

	writeStart := time.Now()
	r.write(payload)
	writeDur := time.Since(writeStart)

	if writeDur > 10*time.Millisecond {
		DebugLog("PROFILE: Atomic Write Slow! Time:%v Bytes:%d", writeDur, len(payload))
	}
}

func (r *AnsiRenderer) write(s string) {
	if s == "" { return }
	if r.parent.Writer != nil {
		io.WriteString(r.parent.Writer, s)
	} else {
		os.Stdout.WriteString(s)
	}
}