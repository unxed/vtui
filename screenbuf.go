package vtui

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
	"unsafe"
)

type ColorProfile int

const (
	ColorProfileTrueColor ColorProfile = iota
	ColorProfile256
	ColorProfile16
)

func DetectColorProfile() ColorProfile {
	return detectColorProfile(runtime.GOOS)
}

func detectColorProfile(goos string) ColorProfile {
	if goos == "windows" {
		return ColorProfileTrueColor
	}

	// Detect bare FreeBSD console (no X11, no SSH, no TMUX, no Wayland)
	if goos == "freebsd" && os.Getenv("DISPLAY") == "" && os.Getenv("SSH_CLIENT") == "" && os.Getenv("TMUX") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return ColorProfile16
	}

	// Special case for FreeBSD: many users have TERM=xterm in console,
	// but the vt(4) driver only supports 16 colors and has a tiny SGR buffer.
	if goos == "freebsd" && os.Getenv("DISPLAY") == "" && os.Getenv("SSH_CLIENT") == "" && os.Getenv("TMUX") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return ColorProfile16
	}

	colorTerm := os.Getenv("COLORTERM")
	if colorTerm == "truecolor" || colorTerm == "24bit" {
		return ColorProfileTrueColor
	}
	term := os.Getenv("TERM")
	if strings.Contains(term, "256color") {
		return ColorProfile256
	}
	if term == "linux" || term == "xterm-clear" || strings.HasPrefix(term, "cons") {
		return ColorProfile16
	}
	// Fallback for general 'xterm' and others. Most modern terminals support 256 colors.
	return ColorProfile256
}

var IsFreeBSDConsole bool

// IsFreeBSDSyscons narrows IsFreeBSDConsole to the syscons driver, the only
// one that aliases a bright background onto the VGA blink bit. See
// console_freebsd.go for the driver sources this is based on.
var IsFreeBSDSyscons bool

func init() {
	IsFreeBSDConsole = (runtime.GOOS == "freebsd" &&
		os.Getenv("DISPLAY") == "" &&
		os.Getenv("SSH_CLIENT") == "" &&
		os.Getenv("TMUX") == "" &&
		os.Getenv("WAYLAND_DISPLAY") == "")
	IsFreeBSDSyscons = detectFreeBSDSyscons()
}

// ScreenBuf implements double buffering to minimize terminal write operations.
type ScreenBuf struct {
	mu sync.Mutex
	// writeMu serialises delivery of finished frames to the output. It is
	// deliberately separate from mu: a terminal that has stopped draining
	// blocks the write for as long as it likes, and mu must not be held
	// across that. See Flush.
	writeMu       sync.Mutex
	buf           []CharInfo // 'buf' is the target screen state formed by UI logic.
	shadow        []CharInfo // 'shadow' is the state last rendered in the terminal.
	width, height int

	cursorX, cursorY int
	cursorVisible    bool
	cursorShape      CursorShape
	cursorSize       uint32
	cursorDirty      bool

	lockCount int
	dirty     bool // Flag indicating that a full rewrite is required during the next Flush.
	clipStack []Rect

	OverlayMode   bool
	ThemePalette  *[256]uint32
	ActivePalette *[256]uint32
	ColorProfile  ColorProfile

	HostPalette      [256]uint32
	HostPaletteValid [256]bool
	quantCache       map[uint32]uint8

	Renderer SurfaceRenderer
	Writer   io.Writer // Output destination, defaults to os.Stdout

	graphics GraphicsLayer
}

// SessionConfig configures I/O streams and initial dimensions for a UI session.
type SessionConfig struct {
	Out    io.Writer
	In     io.Reader
	Width  int
	Height int
}

// SetOutput changes the writer destination for flushed frame output.
func (s *ScreenBuf) SetOutput(w io.Writer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Writer = w
}

// WritePassthrough writes bytes straight to the terminal output, bypassing the
// shadow buffer. Takes writeMu so it can never interleave with a frame.
func (s *ScreenBuf) WritePassthrough(p []byte) {
	if s == nil || len(p) == 0 {
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	w := io.Writer(os.Stdout)
	if s.Writer != nil {
		w = s.Writer
	}
	const writeChunk = 8192
	for len(p) > 0 {
		n := len(p)
		if n > writeChunk {
			n = writeChunk
		}
		w.Write(p[:n])
		p = p[n:]
	}
}

// WritePassthrough writes raw bytes directly to the active ScreenBuf's output,
// bypassing the shadow buffer and serializing with frame rendering.
func WritePassthrough(p []byte) {
	if FrameManager != nil && FrameManager.scr != nil {
		FrameManager.scr.WritePassthrough(p)
	}
}

// NewScreenBuf creates a new ScreenBuf instance.
func NewScreenBuf() *ScreenBuf {
	s := &ScreenBuf{
		dirty:        true,
		ColorProfile: DetectColorProfile(),
		quantCache:   make(map[uint32]uint8),
	}
	s.Renderer = &AnsiRenderer{parent: s}
	return s
}

// NewSilentScreenBuf creates a ScreenBuf that discards all output.
// Ideal for unit tests to prevent ANSI sequences from polluting the console.
func NewSilentScreenBuf() *ScreenBuf {
	return &ScreenBuf{
		dirty:        true,
		Writer:       io.Discard,
		ColorProfile: DetectColorProfile(),
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
	s.graphics.Invalidate()
}

// InvalidateHostPalette forgets which colors the terminal was last told to
// use, so that the next Flush re-sends all 256 OSC 4 sequences.
//
// HostPalette/HostPaletteValid describe the state of the *terminal*, not of
// this process. Anything that resets that state behind our back — Suspend
// sending OSC 104, or the daemon re-attaching to a different terminal
// altogether — must say so here, otherwise SetPalette sees a palette it
// believes is already loaded, sends nothing, and the session runs with
// whatever colors the terminal happened to have.
func (s *ScreenBuf) InvalidateHostPalette() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invalidateHostPaletteLocked()
}

func (s *ScreenBuf) invalidateHostPaletteLocked() {
	for i := range s.HostPaletteValid {
		s.HostPaletteValid[i] = false
	}
	s.quantCache = make(map[uint32]uint8)
}

// ClearBuf resets every cell of the pending buffer to a zero CharInfo.
// Used by the FrameManager when the bottom of the painted frame stack is
// transparent: nothing below will paint the background, so cells vacated
// by moved frames must not retain stale content.
func (s *ScreenBuf) ClearBuf() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.buf {
		s.buf[i] = CharInfo{}
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
	if s.buf == nil || len(s.clipStack) == 0 {
		return
	}
	clip := s.clipStack[len(s.clipStack)-1]
	if x1 < clip.X1 {
		x1 = clip.X1
	}
	if y1 < clip.Y1 {
		y1 = clip.Y1
	}
	if x2 > clip.X2 {
		x2 = clip.X2
	}
	if y2 > clip.Y2 {
		y2 = clip.Y2
	}
	if x1 > x2 || y1 > y2 {
		return
	}

	for y := y1; y <= y2; y++ {
		offset := y*s.width + x1
		for x := 0; x <= x2-x1; x++ {
			attr := s.buf[offset+x].Attributes

			var bg uint32
			if attr&IsBgRGB != 0 {
				bg = GetRGBBack(attr)
			} else {
				idx := GetIndexBack(attr)
				if s.ThemePalette != nil {
					bg = s.ThemePalette[idx]
				} else {
					bg = XTerm256Palette[idx]
				}
			}

			var fg uint32
			if attr&IsFgRGB != 0 {
				fg = GetRGBFore(attr)
			} else {
				idx := GetIndexFore(attr)
				if s.ThemePalette != nil {
					fg = s.ThemePalette[idx]
				} else {
					fg = XTerm256Palette[idx]
				}
			}

			// Use 3/8 (0.375) factor: in 16-color quantization (Win32 console / DOS),
			// dark blue desktop (0x0000A0) drops to Black (0) creating crisp Far-style shadows,
			// while light gray dialogs (0xC0C0C0) drop to DarkGray (8).
			bg = (((bg>>16&0xFF)*3)/8)<<16 | (((bg>>8&0xFF)*3)/8)<<8 | (((bg & 0xFF) * 3) / 8)
			fg = (((fg>>16&0xFF)*3)/8)<<16 | (((fg>>8&0xFF)*3)/8)<<8 | (((fg & 0xFF) * 3) / 8)

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

	if len(s.clipStack) == 0 {
		return
	}
	clip := s.clipStack[len(s.clipStack)-1]
	if x1 < clip.X1 {
		x1 = clip.X1
	}
	if y1 < clip.Y1 {
		y1 = clip.Y1
	}
	if x2 > clip.X2 {
		x2 = clip.X2
	}
	if y2 > clip.Y2 {
		y2 = clip.Y2
	}
	if x1 > x2 || y1 > y2 {
		return
	}

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
	if s.buf == nil || len(s.clipStack) == 0 {
		return
	}
	if x1 > x2 || y1 > y2 {
		return
	}

	clip := s.clipStack[len(s.clipStack)-1]
	if x1 < clip.X1 {
		x1 = clip.X1
	}
	if y1 < clip.Y1 {
		y1 = clip.Y1
	}
	if x2 > clip.X2 {
		x2 = clip.X2
	}
	if y2 > clip.Y2 {
		y2 = clip.Y2
	}
	if x1 > x2 || y1 > y2 {
		return
	}

	attributes = s.resolveAttr(attributes)
	cell := CharInfo{Char: uint64(char), Attributes: attributes}
	for y := y1; y <= y2; y++ {
		offset := y*s.width + x1
		for x := 0; x <= x2-x1; x++ {
			s.buf[offset+x] = cell
		}
	}
}

// SetCursorPos moves the caret. It deliberately says nothing about whether
// the caret is visible: that is SetCursorVisible's business, and callers
// disagree on the order they call the two in (Edit shows then positions,
// EditorView and MultiLineEdit position then show), so a position setter
// that also hid the caret produced different results in different widgets
// for the same out-of-range coordinate. Out-of-range positions are clamped
// to the screen instead; a caret cannot be placed off-screen, but asking
// for that no longer silently turns it off. See f4 issue #518.
func (s *ScreenBuf) SetCursorPos(x, y int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.width <= 0 || s.height <= 0 {
		return
	}
	if x < 0 {
		x = 0
	} else if x >= s.width {
		x = s.width - 1
	}
	if y < 0 {
		y = 0
	} else if y >= s.height {
		y = s.height - 1
	}
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

func (s *ScreenBuf) SetCursorShape(shape CursorShape) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cursorShape != shape {
		s.cursorShape = shape
		s.cursorDirty = true
	}
}

// GetCursorStateForTesting returns the internal cursor states for verification in unit tests.
func (s *ScreenBuf) GetCursorStateForTesting() (x, y int, visible bool, shape CursorShape) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cursorX, s.cursorY, s.cursorVisible, s.cursorShape
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

// CellSpanAt reports which character occupies the cell at (x, y) and how many
// columns it claims. startX is the column that character begins at, which is x
// itself unless x landed on the filler half of a double width character, and
// span is never less than one. Out of range coordinates answer as a plain one
// column cell so that callers need no second bounds check.
//
// Renderers use this instead of measuring the character they are about to
// draw: the layout engine has already decided how many cells the cluster gets,
// and a renderer that measures again can disagree with it.
func CellSpanAt(buf []CharInfo, width, x, y int) (startX, span int) {
	if width <= 0 || x < 0 || y < 0 || x >= width {
		return x, 1
	}
	rowOff := y * width
	if rowOff < 0 || rowOff+width > len(buf) {
		return x, 1
	}

	startX = x
	for startX > 0 && buf[rowOff+startX].Char == WideCharFiller {
		startX--
	}
	span = 1
	for startX+span < width && buf[rowOff+startX+span].Char == WideCharFiller {
		span++
	}
	return startX, span
}

// Dump записывает содержимое буфера в поток в формате, оптимизированном для нейросетей.
// Сначала идет текстовое превью, затем детальные данные атрибутов с RLE-сжатием.
func (s *ScreenBuf) Dump(w io.Writer) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fmt.Fprintf(w, "VTUI_SCREEN_DUMP_V1 %dx%d\n", s.width, s.height)
	fmt.Fprintln(w, "--- TEXT PREVIEW ---")
	for y := 0; y < s.height; y++ {
		var line strings.Builder
		for x := 0; x < s.width; x++ {
			line.WriteString(CellString(s.buf[y*s.width+x].Char))
		}
		fmt.Fprintln(w, line.String())
	}

	fmt.Fprintln(w, "--- CELL METADATA (RLE) ---")
	fmt.Fprintln(w, "Format: [AttrHex]xRepeatCount ...")
	for y := 0; y < s.height; y++ {
		fmt.Fprintf(w, "R%d: ", y)
		count := 0
		var lastAttr uint64 = 0
		first := true

		for x := 0; x < s.width; x++ {
			attr := s.buf[y*s.width+x].Attributes
			if first {
				lastAttr = attr
				count = 1
				first = false
				continue
			}
			if attr == lastAttr {
				count++
			} else {
				fmt.Fprintf(w, "[%016X]x%d ", lastAttr, count)
				lastAttr = attr
				count = 1
			}
		}
		fmt.Fprintf(w, "[%016X]x%d\n", lastAttr, count)
	}
}

// rgb extracts R, G, B components from 24-bit color (format 0xRRGGBB).
func rgb(c uint32) (r, g, b byte) {
	return byte((c >> 16) & 0xFF), byte((c >> 8) & 0xFF), byte(c & 0xFF)
}

// Flush синхронизирует состояние виртуального буфера с физическим экраном через Renderer.
//
// The frame is composed while holding mu and delivered after releasing it.
// Writing to a terminal is not a bounded operation: if whatever sits on the
// other end of the tty stops reading, the write blocks until it resumes, and
// a full-screen frame on a large terminal (~31 KB on 246x70) is far bigger
// than a pty buffer. Holding mu across that would freeze every goroutine
// that touches the screen, not just the render loop.
//
// writeMu is taken first and covers both phases, so concurrent Flushes still
// reach the terminal whole and in order. Lock order is always writeMu -> mu.
func (s *ScreenBuf) Flush() {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if deliver := s.composeFrame(); deliver != nil {
		deliver()
	}
}

// composeFrame renders the pending state and returns a closure that delivers
// the result, or nil when there is nothing to deliver (or when the renderer
// writes on its own, as the GUI backends do). Everything it touches is
// protected by mu; the returned closure touches none of it.
func (s *ScreenBuf) composeFrame() func() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.lockCount > 0 || s.buf == nil || s.Renderer == nil {
		return nil
	}
	if s.graphics.TakeRepaintRequest() {
		s.dirty = true
	}

	var activePal *[256]uint32
	if s.ActivePalette != nil {
		activePal = s.ActivePalette
	} else if s.ThemePalette != nil {
		activePal = s.ThemePalette
	}

	s.Renderer.SetPalette(activePal)
	s.Renderer.SetCursor(s.cursorX, s.cursorY, s.cursorVisible, s.cursorShape)
	s.Renderer.Render(s.buf, s.shadow, s.width, s.height, s.dirty)
	if gr, ok := s.Renderer.(GraphicsRenderer); ok {
		gr.RenderGraphics(&s.graphics, s.buf, s.shadow, s.width, s.height, s.dirty)
	}
	var deliver func()
	if fp, ok := s.Renderer.(framePreparer); ok {
		deliver = fp.PrepareFlush()
	} else {
		s.Renderer.Flush()
	}

	s.dirty = false
	s.cursorDirty = false
	copy(s.shadow, s.buf)

	return deliver
}

// framePreparer is implemented by renderers that can separate composing a
// frame (which reads the screen buffer and so must happen under ScreenBuf.mu)
// from delivering it (which must not). Renderers that don't implement it —
// the GUI backends, which hand pixels to a windowing library rather than
// bytes to a tty — keep being flushed inline.
type framePreparer interface {
	// PrepareFlush finishes the frame and returns a closure that writes it
	// out, or nil if there is nothing to write.
	PrepareFlush() func()
}

// byteBuffer is a reusable byte slice buffer with zero allocations across frames.
type byteBuffer []byte

func (b *byteBuffer) Len() int {
	return len(*b)
}

func (b *byteBuffer) Cap() int {
	return cap(*b)
}

func (b *byteBuffer) Reset() {
	*b = (*b)[:0]
}

func (b *byteBuffer) Grow(n int) {
	if cap(*b)-len(*b) < n {
		newBuf := make([]byte, len(*b), 2*cap(*b)+n)
		copy(newBuf, *b)
		*b = newBuf
	}
}

func (b *byteBuffer) Write(p []byte) (int, error) {
	*b = append(*b, p...)
	return len(p), nil
}

func (b *byteBuffer) WriteByte(c byte) error {
	*b = append(*b, c)
	return nil
}

func (b *byteBuffer) WriteString(s string) (int, error) {
	*b = append(*b, s...)
	return len(s), nil
}

func (b *byteBuffer) WriteRune(r rune) (int, error) {
	if r < 0x80 {
		*b = append(*b, byte(r))
		return 1, nil
	}
	var buf [utf8.UTFMax]byte
	n := utf8.EncodeRune(buf[:], r)
	*b = append(*b, buf[:n]...)
	return n, nil
}

func (b *byteBuffer) String() string {
	if len(*b) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(*b), len(*b))
}

func (b *byteBuffer) Bytes() []byte {
	return *b
}

// AnsiRenderer implements SurfaceRenderer via ESC sequences.
type AnsiRenderer struct {
	parent   *ScreenBuf
	lastAttr uint64
	frameOut byteBuffer

	cursorX, cursorY int
	cursorVis        bool
	cursorShape      CursorShape

	lastSentCursorX, lastSentCursorY int
	lastSentCursorVis                bool
	lastSentCursorShape              CursorShape
	termCursorInvalid                bool
	firstInit                        bool

	gfxProto GraphicsProtocol
	gfxGen   uint64
	gfxKitty *kittyEncoder
	gfxSixel *sixelEncoder
	gfxFar2l *far2lEncoder
	gfxList  []ImagePlacement
}

func (r *AnsiRenderer) SetPalette(pal *[256]uint32) {
	if IsFreeBSDConsole {
		// The FreeBSD system consoles have a fixed palette and do not parse
		// OSC 4.  syscons would print the payload after the unknown ESC ]
		// introducer, turning one palette update into a screenful of garbage.
		return
	}
	if r.parent.quantCache == nil {
		r.parent.quantCache = make(map[uint32]uint8)
	}
	if pal == nil {
		return
	}
	// All changed entries go into a single OSC 4 message (xterm accepts
	// multiple ;index;spec pairs). One message per frame instead of up to
	// 256: terminals that repaint per OSC message (iTerm2) take seconds to
	// chew through individual messages, which showed up as a multi-second
	// visible freeze on startup and exit.
	changed := false
	for i := 0; i < 256; i++ {
		if !r.parent.HostPaletteValid[i] || r.parent.HostPalette[i] != pal[i] {
			if !changed {
				changed = true
				r.frameOut.WriteString("\x1b]4")
			}
			pr, pg, pb := rgb(pal[i])
			r.frameOut.WriteString(fmt.Sprintf(";%d;rgb:%02x/%02x/%02x", i, pr, pg, pb))
			r.parent.HostPalette[i] = pal[i]
			r.parent.HostPaletteValid[i] = true
		}
	}
	if changed {
		r.frameOut.WriteString("\x07")
		r.parent.quantCache = make(map[uint32]uint8)
	}
}

// isGhostProneText reports Unicode that can shift width through font
// fallback; box/block runes are fixed single-column cells that never shift.
func isGhostProneText(ch uint64) bool {
	if ch < 0x80 {
		return false
	}
	if ch >= 0x2500 && ch <= 0x259F {
		return false
	}
	return true
}

// cellAdvanceTrusted reports whether every terminal can be relied upon to
// advance its cursor by exactly the columns vtui gave this cell: printable
// ASCII, the box drawing and block element ranges, and the single column
// letters of Latin, Greek and Cyrillic. Everything else (a composite cluster,
// a wide character, an emoji, any script a terminal shapes) may be measured
// differently by the terminal than by go-runewidth, and the renderer resyncs
// the cursor after it. wide is true when the cell is followed by a filler:
// go-runewidth widens Cyrillic in an East Asian locale, the terminal need not.
func cellAdvanceTrusted(ch uint64, wide bool) bool {
	switch {
	case wide:
		return false
	case ch < 0x80:
		return true
	case ch >= 0x2500 && ch <= 0x259F:
		return true
	case IsCompChar(ch):
		return false
	case ch >= 0xA0 && ch <= 0x052F:
		// Latin-1 Supplement through Cyrillic Supplement; the combining
		// diacritical marks block sits in the middle of that range.
		return ch < 0x0300 || ch > 0x036F
	}
	return false
}

func (r *AnsiRenderer) Render(buf, shadow []CharInfo, w, h int, force bool) {
	needsDraw := force
	if !needsDraw {
		for i := 0; i < w*h; i++ {
			if buf[i] != shadow[i] {
				needsDraw = true
				break
			}
		}
	}

	if !needsDraw {
		return
	}

	if !IsFreeBSDConsole {
		r.frameOut.WriteString("\x1b[?2026h\x1b[?25l") // Atomic update + hide cursor during draw
	}

	r.termCursorInvalid = true
	lastX, lastY := -1, -1
	resync := false
	r.lastAttr = ^uint64(0)

	var activePal *[256]uint32
	if r.parent.ActivePalette != nil {
		activePal = r.parent.ActivePalette
	} else {
		activePal = r.parent.ThemePalette
	}

	for y := 0; y < h; y++ {
		rowOff := y * w
		firstDiff, lastDiff := -1, -1
		rowHasGhost := false

		if force {
			firstDiff = 0
			lastDiff = w - 1
			rowHasGhost = true
		} else {
			for x := 0; x < w; x++ {
				bCell := buf[rowOff+x]
				sCell := shadow[rowOff+x]
				if bCell != sCell {
					if firstDiff == -1 {
						firstDiff = x
					}
					lastDiff = x
				}
				if isGhostProneText(bCell.Char) || isGhostProneText(sCell.Char) || IsCompChar(bCell.Char) || IsCompChar(sCell.Char) {
					rowHasGhost = true
				}
			}
		}

		if firstDiff == -1 {
			continue
		}

		// Font fallback can shift columns of text Unicode, so such rows
		// redraw in full to wipe out ghost remnants; ASCII and box-drawing
		// rows keep the sparse diff.
		var startX, endX int
		if rowHasGhost {
			startX = 0
			endX = w - 1
		} else {
			startX = firstDiff
			endX = lastDiff
		}

		for x := startX; x <= endX; x++ {
			idx := rowOff + x
			if !force && !rowHasGhost && buf[idx] == shadow[idx] {
				continue
			}

			char := buf[idx].Char
			if char == WideCharFiller {
				// The right half of a wide character. If its left half was
				// just written the terminal cursor already sits past it;
				// otherwise leave lastX alone so the next cell repositions.
				if lastY == y && lastX == x-1 {
					lastX = x
				}
				continue
			}

			if x != lastX+1 || y != lastY || resync {
				if y == lastY && !rowHasGhost && !resync {
					if x > lastX+1 {
						r.writeRelCursor(x-lastX-1, 'C')
					} else {
						r.writeRelCursor(lastX+1-x, 'D')
					}
				} else {
					r.writeCursorPos(y+1, x+1)
				}
				resync = false
			}

			attr := buf[idx].Attributes
			if attr != r.lastAttr {
				writeAttributesToBuffer(&r.frameOut, attr, r.lastAttr, activePal, r.parent.ColorProfile, r.parent.quantCache)
				r.lastAttr = attr
			}

			if char == 0 {
				r.frameOut.WriteByte(' ')
			} else if char < 0x80 {
				r.frameOut.WriteByte(byte(char))
			} else if IsCompChar(char) {
				r.frameOut.WriteString(CellString(char))
			} else {
				r.frameOut.WriteRune(rune(char))
			}
			// A terminal advances by the columns *it* measures for the text,
			// not by the cells vtui allotted, and no two width tables fully
			// agree (Indic conjuncts, ambiguous width, emoji sequences, the
			// Unicode version). Text is written as a stream, so one column
			// of disagreement used to drag everything after it on the row
			// along: that is the leaning dialog of unxed/f4#546. After any
			// cell where the two could differ, the next cell is placed with
			// an absolute cursor position, which confines the damage to the
			// cell itself.
			if !cellAdvanceTrusted(char, x+1 < w && buf[idx+1].Char == WideCharFiller) {
				resync = true
			}
			lastX, lastY = x, y
		}
	}
}

func (r *AnsiRenderer) SetCursor(x, y int, vis bool, shape CursorShape) {
	r.cursorX = x
	r.cursorY = y
	r.cursorVis = vis
	r.cursorShape = shape
}

// writeCursorPos emits CSI Y;X H without allocating.
func (r *AnsiRenderer) writeCursorPos(row, col int) {
	var buf [16]byte
	b := strconv.AppendInt(append(buf[:0], "\x1b["...), int64(row), 10)
	b = append(b, ';')
	b = strconv.AppendInt(b, int64(col), 10)
	b = append(b, 'H')
	r.frameOut.Write(b)
}

// writeRelCursor emits CSI n C (right) / CSI n D (left); n is omitted for 1.
func (r *AnsiRenderer) writeRelCursor(n int, dir byte) {
	var buf [12]byte
	b := append(buf[:0], "\x1b["...)
	if n != 1 {
		b = strconv.AppendInt(b, int64(n), 10)
	}
	b = append(b, dir)
	r.frameOut.Write(b)
}

func (r *AnsiRenderer) SetWindowTitle(title string) {
	if IsFreeBSDConsole {
		return
	}
	// Titles are not part of a frame: write under writeMu so an update can
	// neither resend the pending frame nor race the render loop.
	r.parent.writeMu.Lock()
	defer r.parent.writeMu.Unlock()
	r.write(fmt.Sprintf("\x1b]0;%s\x07", title))
}

// Flush composes the frame and writes it out immediately.
func (r *AnsiRenderer) Flush() {
	deliver := r.PrepareFlush()
	if deliver == nil {
		return
	}
	r.parent.writeMu.Lock()
	defer r.parent.writeMu.Unlock()
	deliver()
}

// PrepareFlush appends the cursor state and mode 2026 termination to the pending frame.
func (r *AnsiRenderer) PrepareFlush() func() {
	if !r.firstInit || r.termCursorInvalid || r.cursorX != r.lastSentCursorX || r.cursorY != r.lastSentCursorY || r.cursorVis != r.lastSentCursorVis || r.cursorShape != r.lastSentCursorShape {
		r.writeCursorPos(r.cursorY+1, r.cursorX+1)
		if IsFreeBSDConsole {
			if r.cursorVis {
				r.frameOut.WriteString("\x1b[=0S")
			} else {
				r.frameOut.WriteString("\x1b[=1S")
			}
		} else if r.cursorVis {
			r.frameOut.WriteString("\x1b[?25h")
			// On a classic Windows console the shape goes through
			// SetConsoleCursorInfo below and DECSCUSR must stay off the
			// stream: see cursorStyleViaConsoleAPI.
			if ManageCursorStyle && !cursorStyleViaConsoleAPI() {
				if os.Getenv("TERM") == "linux" {
					if r.cursorShape == CursorShapeBlock {
						r.frameOut.WriteString("\x1b[?6c")
					} else {
						r.frameOut.WriteString("\x1b[?3c")
					}
				} else {
					if r.cursorShape == CursorShapeBlock {
						r.frameOut.WriteString("\x1b[1 q\x1b]1337;CursorShape=0\x07")
					} else {
						r.frameOut.WriteString("\x1b[3 q\x1b]1337;CursorShape=2\x07")
					}
				}
			}
		} else {
			r.frameOut.WriteString("\x1b[?25l")
		}
		if !r.firstInit || r.cursorVis != r.lastSentCursorVis || r.cursorShape != r.lastSentCursorShape {
			SetCursorStyleOS(r.cursorVis, r.cursorShape)
		}
		r.lastSentCursorX = r.cursorX
		r.lastSentCursorY = r.cursorY
		r.lastSentCursorVis = r.cursorVis
		r.lastSentCursorShape = r.cursorShape
		r.termCursorInvalid = false
		r.firstInit = true
	}

	if r.frameOut.Len() > 0 && !IsFreeBSDConsole {
		r.frameOut.WriteString("\x1b[?2026l")
	}

	payloadLen := r.frameOut.Len()
	if payloadLen == 0 {
		return nil
	}

	payload := r.frameOut.String()
	r.frameOut.Reset()

	return func() {
		writeStart := time.Now()
		r.write(payload)
		writeDur := time.Since(writeStart)

		if writeDur > 10*time.Millisecond {
			DebugLog("PROFILE: Atomic Write Slow! Time:%v Bytes:%d", writeDur, payloadLen)
		}
	}
}

func (r *AnsiRenderer) write(s string) {
	if s == "" {
		return
	}
	w := io.Writer(os.Stdout)
	if r.parent.Writer != nil {
		w = r.parent.Writer
	}
	// Hand large frames over in chunks so a relay reading the tty (WSL,
	// ConPTY) can keep up; a byte lost mid-frame becomes U+FFFD on screen.
	//
	// Advance by what was actually written, not by what was offered: a
	// short write used to drop the rest of its chunk silently, and since
	// every painted frame opens with ESC[?25l and only restores the caret
	// with ESC[?25h at the very end, a frame truncated in the middle left
	// the caret hidden until something changed its state again. Give up on
	// a hard error rather than spinning on a dead tty. See f4 issue #518.
	const writeChunk = 8192
	for len(s) > 0 {
		n := len(s)
		if n > writeChunk {
			n = writeChunk
		}
		written, err := io.WriteString(w, s[:n])
		if written > 0 {
			s = s[written:]
		}
		if err != nil {
			return
		}
		if written == 0 {
			return
		}
	}
}

// GetCursorPos returns the current virtual cursor position.
func (s *ScreenBuf) GetCursorPos() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cursorX, s.cursorY
}

// ScreenRow reads a stretch of one row back out of the screen.
func ScreenRow(scr *ScreenBuf, y, x1, x2 int) string {
	if x1 > x2 {
		return ""
	}
	var sb strings.Builder
	for x := x1; x <= x2; x++ {
		cell := scr.GetCell(x, y)
		if cell.Char == WideCharFiller {
			continue
		}
		if cell.Char == 0 {
			sb.WriteByte(' ')
		} else if IsCompChar(cell.Char) {
			sb.WriteString(CellString(cell.Char))
		} else {
			sb.WriteRune(rune(cell.Char))
		}
	}
	return sb.String()
}
