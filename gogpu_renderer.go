//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris && !plan9 && !android && (amd64 || arm64)

package vtui

import (
	"image/color"
	"math"
	"os"
	"sync"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/gogpu/gg"
	_ "github.com/gogpu/gg/gpu"
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gg/text"
	"github.com/gogpu/gogpu"
)

// newGogpuFallbackChain binds the shared fallback chain (gui_font.go) to gg
// faces. gg's HasGlyph reflects what DrawString can actually draw, so covers
// and renders coincide here, unlike on the x/image side.
func newGogpuFallbackChain(size float64) *fontFallbackChain {
	hasGlyph := func(face any, r rune) bool { return face.(text.Face).HasGlyph(r) }
	return &fontFallbackChain{
		logTag: "GOGPU_DIAG_FONT",
		open: func(path string) (any, error) {
			src, err := text.NewFontSourceFromFile(path)
			if err != nil {
				return nil, err
			}
			return src.Face(size), nil
		},
		covers:  hasGlyph,
		renders: hasGlyph,
		drop:    func(any) {},
	}
}

type GogpuRenderer struct {
	mu           sync.Mutex
	host         *GogpuHost
	face         text.Face
	chain        *fontFallbackChain
	faceCache    map[rune]text.Face
	cellW, cellH int // logical cell sizes from font measurement
	cols, rows   int // dimensions of the current renderBuf

	cursorX, cursorY int
	cursorVis        bool
	cursorShape      CursorShape
	lastCursorReset  time.Time
	lastBlinkState   bool
	blinkState       bool
	lastBlinkTime    time.Time

	canvas    *ggcanvas.Canvas
	renderBuf []CharInfo
	dirty     bool

	gfxList  []ImagePlacement
	gfxCache nativeGraphicsCache
	gfxGen   uint64
	gfxKnown bool

	glyphMemo map[rune]glyphMemoEntry // font-glyph run cache, see glyphRectsCached

	// textRunBuf is the reusable scratch buffer for batched DrawString runs.
	textRunBuf []byte
	// noBatch disables DrawString batching (VTUI_GOGPU_NO_BATCH); it exists
	// so the batching cost can be measured against the per-cell path.
	noBatch bool
}

func NewGogpuRenderer(host *GogpuHost, face text.Face, cw, ch int) *GogpuRenderer {
	return &GogpuRenderer{
		host:            host,
		face:            face,
		cellW:           cw,
		cellH:           ch,
		lastCursorReset: time.Now(),
		lastBlinkState:  true,
		blinkState:      true,
		lastBlinkTime:   time.Now(),
		noBatch:         os.Getenv("VTUI_GOGPU_NO_BATCH") != "",
	}
}

// SetFallbackFontChain installs a lazily loaded fallback chain, consulted
// for runes the primary font has no glyph for. Passing nil restores
// primary-only rendering.
func (r *GogpuRenderer) SetFallbackFontChain(chain *fontFallbackChain) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chain = chain
	r.faceCache = nil
	r.glyphMemo = nil
}

// faceFor resolves the face owning a glyph for ch, memoised per rune: a cmap
// probe once per distinct rune on screen, not per cell per frame, keeps this
// off the hot path. The caller must hold r.mu (DrawToScreen, the only caller,
// holds it for the whole frame).
// gogpuBatchRune reports whether a cell may join a batched DrawString run.
// Only plain single-rune cells qualify: no registry clusters (their per-cell
// shaping must stay intact), no wide fillers, no spaces (they draw nothing),
// no box-drawing runes (they go through drawCustomChar) and never regional
// indicators — two lone RIs in one string would shape into a flag.
func gogpuBatchRune(ch uint64) bool {
	if ch == 0 || ch == WideCharFiller || IsCompChar(ch) {
		return false
	}
	if ch == ' ' {
		return false
	}
	r := rune(ch)
	if r >= runeRegionalFirst && r <= runeRegionalLast {
		return false
	}
	return !isBoxDrawRune(r)
}

// gogpuAdvFits reports whether a glyph advance in pixels occupies exactly
// `cells` grid columns of cellW pixels. A batched DrawString run is drawn
// once with the font's natural advances, which is pixel-identical to per-cell
// placement only then; anything else lets the run drift off the cell grid.
func gogpuAdvFits(adv float64, cellW, cells int) bool {
	return adv == float64(cellW*cells)
}

// gogpuAdvMatches reports whether ch can be batched in a DrawString run on
// face f. Emoji and CJK fallback faces are not scaled to the cell grid
// (measured advances of 1.8-2.5 cells are the norm) and proportional primary
// fonts advance every glyph differently, so without this gate a batched run
// would drift off the cell grid. A face without the glyph also declines.
func (r *GogpuRenderer) gogpuAdvMatches(f text.Face, ch rune, cells int) bool {
	if f == nil {
		// No font installed (tests construct nil-face renderers): nothing can
		// be measured, keep the caller's per-cell behaviour.
		return true
	}
	if !f.HasGlyph(ch) {
		return false
	}
	return gogpuAdvFits(f.Advance(cellRuneString(uint64(ch))), r.cellW, cells)
}

// gogpuTextRun extends a batched DrawString run across one colour span.
// buf is the render buffer, rowOff+x the span's first cell, start the run's
// first cell (must pass gogpuBatchRune). It appends the runes and returns the
// new buffer, the cells consumed, the resolved face, and whether the first
// cell joined at all — when false the caller draws it per-cell, as before.
// The run stops at a cell that cannot join, needs another face, or would
// drift off the cell grid.
func (r *GogpuRenderer) gogpuTextRun(buf []CharInfo, rowOff, x, start, spanW, drawCols int, curFace text.Face, run []byte) ([]byte, int, text.Face, bool) {
	pos := x + start
	first := buf[rowOff+pos]
	f := r.faceFor(CellBaseRune(first.Char))
	if f == nil {
		f = curFace
	}
	if f != curFace {
		curFace = f
	}

	consumed := 1
	if pos+1 < drawCols && buf[rowOff+pos+1].Char == WideCharFiller {
		consumed = 2
	}
	if !r.gogpuAdvMatches(f, rune(first.Char), consumed) {
		return run, 0, curFace, false
	}
	run = utf8.AppendRune(run, rune(first.Char))
	pos += consumed

	for pos < x+spanW {
		c := buf[rowOff+pos]
		if c.Char == WideCharFiller {
			pos++
			consumed++
			continue
		}
		if !gogpuBatchRune(c.Char) {
			break
		}
		cw := 1
		if pos+1 < drawCols && buf[rowOff+pos+1].Char == WideCharFiller {
			cw = 2
		}
		nf := r.faceFor(CellBaseRune(c.Char))
		if nf != curFace {
			break
		}
		if !r.gogpuAdvMatches(nf, rune(c.Char), cw) {
			break
		}
		run = utf8.AppendRune(run, rune(c.Char))
		consumed += cw
		pos += cw
	}
	return run, consumed, curFace, true
}

func (r *GogpuRenderer) faceFor(ch rune) text.Face {
	if r.face == nil || r.chain == nil {
		return r.face
	}
	if f, ok := r.faceCache[ch]; ok {
		return f
	}

	f := r.face
	if !r.face.HasGlyph(ch) {
		// The chain opens font files on demand and memoises per rune, so this
		// misses at most once per rune; a negative answer is final because the
		// chain has been walked to its end.
		if fb, ok := r.chain.faceFor(ch).(text.Face); ok {
			f = fb
		}
	}

	if r.faceCache == nil {
		r.faceCache = make(map[rune]text.Face, 256)
	}
	r.faceCache[ch] = f
	return f
}

func (r *GogpuRenderer) Render(buf, shadow []CharInfo, w, h int, force bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.cols = w
	r.rows = h

	needsRedraw := force
	if !needsRedraw {
		for i := 0; i < len(buf); i++ {
			if buf[i] != shadow[i] {
				needsRedraw = true
				break
			}
		}
	}

	now := time.Now()
	if now.Sub(r.lastBlinkTime) >= 500*time.Millisecond {
		r.blinkState = !r.blinkState
		r.lastBlinkTime = r.lastBlinkTime.Add(500 * time.Millisecond)
		if now.Sub(r.lastBlinkTime) >= 500*time.Millisecond {
			r.lastBlinkTime = now
		}
	}

	if !needsRedraw && r.cursorVis {
		if r.blinkState != r.lastBlinkState {
			needsRedraw = true
			r.lastBlinkState = r.blinkState
		}
	}

	if !needsRedraw {
		return
	}

	if len(r.renderBuf) != len(buf) {
		r.renderBuf = make([]CharInfo, len(buf))
	}
	copy(r.renderBuf, buf)
	r.dirty = true
}

func (r *GogpuRenderer) SetCursor(x, y int, visible bool, shape CursorShape) {
	r.mu.Lock()
	if r.cursorX != x || r.cursorY != y || r.cursorVis != visible || r.cursorShape != shape {
		r.cursorX, r.cursorY = x, y
		r.cursorVis = visible
		r.cursorShape = shape
		r.lastCursorReset = time.Now()
		r.lastBlinkState = true
		r.blinkState = true
		r.dirty = true
	}
	r.mu.Unlock()
}

// RenderGraphics implements GraphicsRenderer. The GPU canvas is rebuilt as a
// whole whenever it is dirty, so here we only have to remember the snapshot
// and make sure the next frame is considered dirty.
func (r *GogpuRenderer) RenderGraphics(layer *GraphicsLayer, buf, shadow []CharInfo, w, h int, force bool) {
	if layer == nil || layer.Protocol() != GraphicsNative {
		return
	}

	list, gen := layer.Snapshot(nil)

	r.mu.Lock()
	if force || !r.gfxKnown || gen != r.gfxGen {
		r.dirty = true
	}
	r.gfxList = list
	r.gfxGen = gen
	r.gfxKnown = true
	r.mu.Unlock()
}

func (r *GogpuRenderer) SetPalette(pal *[256]uint32) {}

func (r *GogpuRenderer) ResizeWindow(cols, rows int) {
	r.host.mu.Lock()
	app := r.host.app
	cw, ch := r.cellW, r.cellH
	r.host.mu.Unlock()

	if app != nil && cw > 0 && ch > 0 {
		app.RequestSize(cols*cw, rows*ch)
	}
}

// glyphMemoEntry caches a rune's mask-space rects and bbox offset from the
// cell origin (bx) and baseline (by). The baseline itself is not stored: it
// always comes from the primary font, like DrawString does for fallback text.
type glyphMemoEntry struct {
	rects []glyphRect
	bx    float64
	by    float64
}

// glyphRectsFromFace rasterizes char through the same gogpu pipeline as
// DrawString (same face, HintingFull), thresholds the mask at coverage 128
// and returns horizontal rect runs - pixel-identical to the font text.
func glyphRectsFromFace(face text.Face, char rune) (glyphMemoEntry, bool) {
	var e glyphMemoEntry
	if face == nil || !face.HasGlyph(char) {
		return e, false
	}
	src := face.Source()
	if src == nil {
		return e, false
	}
	var gid text.GlyphID
	found := false
	for g := range face.Glyphs(string(char)) {
		gid, found = g.GID, true
		break
	}
	if !found {
		return e, false
	}
	m := face.Metrics()
	ppem := m.Ascent + m.Descent
	if ppem <= 0 {
		return e, false
	}
	res, err := text.NewGlyphMaskRasterizer().RasterizeHinted(src.Parsed(), gid, ppem, 0, 0, text.HintingFull)
	if err != nil || res == nil || res.Width == 0 || res.Height == 0 {
		return e, false
	}
	const cutoff = 128
	for j := 0; j < res.Height; j++ {
		row := res.Mask[j*res.Width : (j+1)*res.Width]
		for i := 0; i < res.Width; {
			if row[i] <= cutoff {
				i++
				continue
			}
			k := i
			for k+1 < res.Width && row[k+1] > cutoff {
				k++
			}
			e.rects = append(e.rects, glyphRect{y0: j, y1: j, x0: i, x1: k})
			i = k + 1
		}
	}
	e.rects = mergeGlyphRectsV(e.rects)
	if len(e.rects) == 0 {
		return e, false
	}
	e.bx = float64(res.BearingX)
	e.by = float64(res.BearingY)
	return e, true
}

// mergeGlyphRectsV merges consecutive rows with the same x-range into taller
// rects: identical pixels, fewer single-rect fills. Shared with the generator.
// The merge runs in place: the caller's rects are consumed and reused.
func mergeGlyphRectsV(rects []glyphRect) []glyphRect {
	out := rects[:0]
	for _, r := range rects {
		if n := len(out); n > 0 && out[n-1].y1+1 == r.y0 && out[n-1].x0 == r.x0 && out[n-1].x1 == r.x1 {
			out[n-1].y1 = r.y0
			continue
		}
		out = append(out, r)
	}
	return out
}

// glyphRectsCached returns char's decomposition, memoised per rune. Negative
// results are cached too, else box runes the font lacks would be re-probed
// every frame. Uses the same faceFor chain as DrawString.
func (r *GogpuRenderer) glyphRectsCached(char rune) (glyphMemoEntry, bool) {
	if e, ok := r.glyphMemo[char]; ok {
		return e, e.rects != nil
	}
	e, ok := glyphRectsFromFace(r.faceFor(char), char)
	if r.glyphMemo == nil {
		r.glyphMemo = make(map[rune]glyphMemoEntry, 64)
	}
	r.glyphMemo[char] = e // nil rects encodes a negative entry
	return e, ok
}

// drawCustomChar draws one cell of a box/arrow/block rune.
func (r *GogpuRenderer) drawCustomChar(dc *gg.Context, char rune, x, y, w, h, ascent float64) bool {
	thick := 1.0
	fillR := func(rx, ry, rw, rh float64) {
		dc.DrawRectangle(rx, ry, rw, rh)
		dc.Fill()
	}

	mx := math.Floor(x + w/2 - thick/2)
	my := math.Floor(y + h/2 - thick/2)

	ofs := math.Floor(math.Min(w, h) / 4)
	if ofs < 1 {
		ofs = 1
	}

	switch char {
	case '─', '━':
		t := thick
		if char == '━' {
			t *= 2
		}
		fillR(x, math.Floor(y+h/2-t/2), w, t)
	case '│', '┃':
		t := thick
		if char == '┃' {
			t *= 2
		}
		fillR(math.Floor(x+w/2-t/2), y, t, h)
	case '┌', '┏':
		t := thick
		if char == '┏' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		fillR(mxx, myy, w-(mxx-x), t)
		fillR(mxx, myy, t, h-(myy-y))
	case '┐', '┓':
		t := thick
		if char == '┓' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		fillR(x, myy, mxx-x+t, t)
		fillR(mxx, myy, t, h-(myy-y))
	case '└', '┗':
		t := thick
		if char == '┗' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		fillR(mxx, myy, w-(mxx-x), t)
		fillR(mxx, y, t, myy-y+t)
	case '┘', '┛':
		t := thick
		if char == '┛' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		fillR(x, myy, mxx-x+t, t)
		fillR(mxx, y, t, myy-y+t)
	case '├', '┣':
		t := thick
		if char == '┣' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		fillR(mxx, myy, w-(mxx-x), t)
		fillR(mxx, y, t, h)
	case '┤', '┫':
		t := thick
		if char == '┫' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		fillR(x, myy, mxx-x+t, t)
		fillR(mxx, y, t, h)
	case '┬', '┳':
		t := thick
		if char == '┳' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		fillR(x, myy, w, t)
		fillR(mxx, myy, t, h-(myy-y))
	case '┴', '┻':
		t := thick
		if char == '┻' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		fillR(x, myy, w, t)
		fillR(mxx, y, t, myy-y+t)
	case '┼', '╋':
		t := thick
		if char == '╋' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		fillR(x, myy, w, t)
		fillR(mxx, y, t, h)

	// Double lines
	case '═':
		fillR(x, my-ofs, w, thick)
		fillR(x, my+ofs, w, thick)
	case '║':
		fillR(mx-ofs, y, thick, h)
		fillR(mx+ofs, y, thick, h)
	case '╔':
		fillR(mx+ofs, my-ofs, w-(mx-x+ofs), thick)
		fillR(mx-ofs, my+ofs, w-(mx-x-ofs), thick)
		fillR(mx-ofs, my+ofs, thick, (y+h)-(my+ofs))
		fillR(mx+ofs, my-ofs, thick, (y+h)-(my-ofs))
	case '╗':
		fillR(x, my-ofs, mx-x-ofs+thick, thick)
		fillR(x, my+ofs, mx-x+ofs+thick, thick)
		fillR(mx+ofs, my+ofs, thick, (y+h)-(my+ofs))
		fillR(mx-ofs, my-ofs, thick, (y+h)-(my-ofs))
	case '╚':
		fillR(mx-ofs, my-ofs, w-(mx-x-ofs), thick)
		fillR(mx+ofs, my+ofs, w-(mx-x+ofs), thick)
		fillR(mx-ofs, y, thick, (my-ofs)-y+thick)
		fillR(mx+ofs, y, thick, (my+ofs)-y+thick)
	case '╝':
		fillR(x, my-ofs, mx-x+ofs+thick, thick)
		fillR(x, my+ofs, mx-x-ofs+thick, thick)
		fillR(mx+ofs, y, thick, (my-ofs)-y+thick)
		fillR(mx-ofs, y, thick, (my+ofs)-y+thick)
	case '╠':
		fillR(mx-ofs, my-ofs, w-(mx-x-ofs), thick)
		fillR(mx+ofs, my+ofs, w-(mx-x+ofs), thick)
		fillR(mx-ofs, y, thick, h)
		fillR(mx+ofs, y, thick, h)
	case '╣':
		fillR(x, my-ofs, mx-x+ofs+thick, thick)
		fillR(x, my+ofs, mx-x-ofs+thick, thick)
		fillR(mx+ofs, y, thick, h)
		fillR(mx-ofs, y, thick, h)
	case '╦':
		fillR(x, my-ofs, w, thick)
		fillR(x, my+ofs, w, thick)
		fillR(mx-ofs, my+ofs, thick, h-(my-y+ofs))
		fillR(mx+ofs, my+ofs, thick, h-(my-y+ofs))
	case '╩':
		fillR(x, my-ofs, w, thick)
		fillR(x, my+ofs, w, thick)
		fillR(mx-ofs, y, thick, my-y-ofs+thick)
		fillR(mx+ofs, y, thick, my-y-ofs+thick)
	case '╬':
		fillR(x, my-ofs, w, thick)
		fillR(x, my+ofs, w, thick)
		fillR(mx-ofs, y, thick, h)
		fillR(mx+ofs, y, thick, h)

	// Mixed (used in VMenu)
	case '╟':
		fillR(mx+ofs, my, w-(mx-x+ofs), thick)
		fillR(mx-ofs, y, thick, h)
		fillR(mx+ofs, y, thick, h)
	case '╢':
		fillR(x, my, mx-x-ofs+thick, thick)
		fillR(mx-ofs, y, thick, h)
		fillR(mx+ofs, y, thick, h)

	// Symbols and arrows: use font-exact glyph if available, fallback to glyphTable.
	case '↑', '↓', '↕', '←', '→', '↔', '▲', '▼':
		if e, ok := r.glyphRectsCached(char); ok {
			baseY := y + ascent
			for _, rc := range e.rects {
				fillR(x+e.bx+float64(rc.x0), baseY-e.by+float64(rc.y0), float64(rc.x1-rc.x0+1), float64(rc.y1-rc.y0+1))
			}
			return true
		}
		if rects, ok := glyphTable[char]; ok {
			sx := w / glyphCellW
			sy := h / glyphCellH
			for _, rc := range rects {
				fillR(x+float64(rc.x0)*sx, y+float64(rc.y0)*sy, float64(rc.x1-rc.x0+1)*sx, float64(rc.y1-rc.y0+1)*sy)
			}
			return true
		}
		return false

	// Solid Blocks
	case '█':
		fillR(x, y, w, h)
	case '▀':
		fillR(x, y, w, h/2)
	case '▄':
		fillR(x, y+h/2, w, h/2)
	case '▌':
		fillR(x, y, w/2, h)
	case '▐':
		fillR(x+w/2, y, w/2, h)

	case '░', '▒', '▓': // scrollbar shades: 25% / 50% / 75% bar patterns
		if char == '░' {
			for bar := 2; bar < 16; bar += 4 {
				fillR(x, y+float64(bar), w, thick)
			}
		} else if char == '▒' {
			for bar := 1; bar < 16; bar += 2 {
				fillR(x, y+float64(bar), w, thick)
			}
		} else {
			for bar := 1; bar < 16; bar++ {
				if bar == 1 || bar == 6 || bar == 11 {
					continue
				}
				fillR(x, y+float64(bar), w, thick)
			}
		}
	case '▔':
		fillR(x, y, w, thick)
	case '▕':
		fillR(x+w-thick, y, thick, h)
	case '▖': // bottom-left quarter block
		fillR(x, y+h/2, w/2, h/2)
	case '▗': // top-right quarter block
		fillR(x+w/2, y, w/2, h/2)
	case '▘': // top-left quarter block
		fillR(x, y, w/2, h/2)
	case '▝': // bottom-right quarter block
		fillR(x+w/2, y+h/2, w/2, h/2)

	default:
		return false
	}

	return true
}

func (r *GogpuRenderer) Flush() {
	r.host.mu.Lock()
	app := r.host.app
	forceDirty := r.host.resizePending
	r.host.resizePending = false
	r.host.mu.Unlock()

	r.mu.Lock()
	if forceDirty {
		r.dirty = true
		r.lastCursorReset = time.Now() // Ensure cursor is solid-visible on window restore/resize
	}
	shouldRedraw := r.dirty
	r.mu.Unlock()

	if shouldRedraw && app != nil {
		go app.RequestRedraw()
	}
}

// drawFrame issues every drawing command of one frame into dc — the GPU
// canvas closure in production, a software-renderer gg.Context in the
// headless benchmarks — and returns the per-frame stats. The cell loop is
// the CPU-side per-frame cost of the gogpu backend: colour resolution,
// span detection and text batching.
func (r *GogpuRenderer) drawFrame(dc *gg.Context, w, h int) gogpuFrameStats {
	var prof gogpuFrameStats
	dc.SetRGB(0, 0, 0)
	dc.DrawRectangle(0, 0, float64(w), float64(h))
	dc.Fill()

	drawCols := r.cols
	drawRows := r.rows

	// The nil check below is not decoration: every other use of the
	// face in this function is guarded, and an unguarded Metrics()
	// call here would fault exactly the way the render thread did.
	var ascent float64
	if r.face != nil {
		dc.SetFont(r.face)
		ascent = float64(r.face.Metrics().Ascent)
	}
	// The baseline stays the primary font's for the whole frame even
	// when a fallback draws the glyph: a CJK face with its own ascent
	// would make text jump between lines that mix scripts.
	curFace := r.face

	for y := 0; y < drawRows; y++ {
		rowOff := y * drawCols
		ly := float64(y * r.cellH)
		for x := 0; x < drawCols; {
			cell := r.renderBuf[rowOff+x]
			fg, bg := r.getCellColors(cell)

			spanW := 0
			for x+spanW < drawCols {
				nextCell := r.renderBuf[rowOff+x+spanW]
				if nextCell.Char == WideCharFiller {
					spanW++
					continue
				}
				if nextCell.Attributes != cell.Attributes {
					nextFg, nextBg := r.getCellColors(nextCell)
					if nextBg != bg || nextFg != fg {
						break
					}
				}
				spanW++
			}

			lx := float64(x * r.cellW)
			spanPixW := float64(spanW * r.cellW)

			prof.spans++
			tBg := gogpuProfNow()
			dc.SetColor(bg)
			dc.DrawRectangle(lx, ly, spanPixW+1, float64(r.cellH)+1)
			dc.Fill()
			prof.bgFills++
			prof.bgTime += gogpuProfSince(tBg)
			dc.SetColor(fg)

			for sx := 0; sx < spanW; {
				idx := rowOff + x + sx
				currCell := r.renderBuf[idx]

				if currCell.Char == WideCharFiller {
					sx++
					continue
				}

				rw := 1
				if x+sx+1 < drawCols && r.renderBuf[idx+1].Char == WideCharFiller {
					rw = 2
				}

				char := CellBaseRune(currCell.Char)
				underlined := currCell.Attributes&CommonLvbUnderscore != 0

				if isBoxDrawRune(char) {
					tBox := gogpuProfNow()
					drawn := r.drawCustomChar(dc, char, lx+float64(sx*r.cellW), ly, float64(rw*r.cellW), float64(r.cellH), ascent)
					prof.boxTime += gogpuProfSince(tBox)
					if drawn {
						prof.boxChars++
						if underlined {
							drawGogpuUnderline(dc, lx+float64(sx*r.cellW), ly, float64(rw*r.cellW), float64(r.cellH), fg)
						}
						sx += rw
						continue
					}
				}

				if r.face == nil {
					if underlined {
						drawGogpuUnderline(dc, lx+float64(sx*r.cellW), ly, float64(rw*r.cellW), float64(r.cellH), fg)
					}
					sx += rw
					continue
				}

				if !underlined && !r.noBatch && gogpuBatchRune(currCell.Char) {
					tTxt := gogpuProfNow()
					run, consumed, f, batched := r.gogpuTextRun(r.renderBuf, rowOff, x, sx, spanW, drawCols, curFace, r.textRunBuf[:0])
					r.textRunBuf = run
					if batched {
						if f != curFace {
							curFace = f
							dc.SetFont(f)
						}
						dc.DrawString(unsafe.String(unsafe.SliceData(run), len(run)), lx+float64(sx*r.cellW), ly+ascent)
						prof.textTime += gogpuProfSince(tTxt)
						prof.strings++
						prof.glyphs += utf8.RuneCount(run)
						sx += consumed
						continue
					}
				}

				str := CellString(currCell.Char)
				if str != "" && str != " " {
					tTxt := gogpuProfNow()
					if f := r.faceFor(char); f != nil && f != curFace {
						curFace = f
						dc.SetFont(f)
					}
					dc.DrawString(str, lx+float64(sx*r.cellW), ly+ascent)
					prof.textTime += gogpuProfSince(tTxt)
					prof.strings++
					prof.glyphs += gogpuRuneCount(str)
				}
				if underlined {
					drawGogpuUnderline(dc, lx+float64(sx*r.cellW), ly, float64(rw*r.cellW), float64(r.cellH), fg)
				}
				sx += rw
			}

			x += spanW
		}
	}

	for i := range r.gfxList {
		p := &r.gfxList[i]
		px, py, pw, ph := placementPixelRect(p, r.cellW, r.cellH)
		if pw <= 0 || ph <= 0 {
			continue
		}
		entry := r.gfxCache.scaled(p, pw, ph)
		if entry == nil {
			continue
		}

		dc.SetRGBA(1, 1, 1, 1)
		dc.DrawImage(gg.ImageBufFromImage(entry.asImage()), float64(px), float64(py))
	}

	cursorVisible := r.cursorVis && r.blinkState

	if cursorVisible {
		dc.SetColor(color.White)
		curX, curSpan := CellSpanAt(r.renderBuf, drawCols, r.cursorX, r.cursorY)
		cx := float64(curX * r.cellW)
		cy := float64(r.cursorY * r.cellH)
		curW := float64(curSpan * r.cellW)
		if r.cursorShape == CursorShapeBlock {
			dc.DrawRectangle(cx, cy, curW, float64(r.cellH))
		} else {
			cy += float64(r.cellH) - 2
			dc.DrawRectangle(cx, cy, curW, 2)
		}
		dc.Fill()
	}
	return prof
}

func drawGogpuUnderline(dc *gg.Context, x, y, width, height float64, fg color.RGBA) {
	if width <= 0 || height <= 0 {
		return
	}
	dc.SetColor(fg)
	dc.DrawRectangle(x, y+height-1, width, 1)
	dc.Fill()
}

func (r *GogpuRenderer) DrawToScreen(ctx *gogpu.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.renderBuf) == 0 {
		return
	}

	w, h := ctx.Width(), ctx.Height()
	if w <= 0 || h <= 0 {
		return
	}

	if debugLastCtxW != w || debugLastCtxH != h {
		debugLastCtxW, debugLastCtxH = w, h
		if r.canvas != nil {
			r.canvas.Resize(w, h)
		}
		r.dirty = true
	}

	if r.canvas == nil {
		provider := r.host.app.GPUContextProvider()
		if provider == nil {
			return
		}
		r.canvas, _ = ggcanvas.New(provider, w, h)
	}

	var prof gogpuFrameStats

	// drawCanvas re-rasterizes the whole screen buffer into the canvas. OnDraw
	// fires on content changes AND on surface repaints (WM_PAINT/Expose after
	// restore, alt-tab, uncover, focus change); the canvas must repaint in
	// both cases, since ggcanvas.Render is a no-op on a clean canvas and a
	// skipped draw would leave the window black until the next change.
	// RenderTo is not used: mixing it with the primary present path flickers
	// while dragging the mouse.
	drawCanvas := func() {
		tDraw := gogpuProfNow()
		r.canvas.Draw(func(dc *gg.Context) {
			prof = r.drawFrame(dc, w, h)
		})
		prof.drawTime = gogpuProfSince(tDraw)
	}

	drawCanvas()
	r.dirty = false

	tRender := gogpuProfNow()
	r.canvas.Render(ctx.RenderTarget())
	prof.renderTime = gogpuProfSince(tRender)

	if gogpuProfileEnabled {
		prof.report(r.cols, r.rows)
	}
}

func (r *GogpuRenderer) SetWindowTitle(title string) {
	r.host.app.SetTitle(title)
}

// getCellColors decodes a cell attribute into foreground/background RGBA values.
func (r *GogpuRenderer) getCellColors(cell CharInfo) (color.RGBA, color.RGBA) {
	bg := GetRGBBack(cell.Attributes)
	if cell.Attributes&IsBgRGB == 0 {
		bg = ThemePalette[GetIndexBack(cell.Attributes)]
	}
	fg := GetRGBFore(cell.Attributes)
	if cell.Attributes&IsFgRGB == 0 {
		fg = ThemePalette[GetIndexFore(cell.Attributes)]
	}

	return color.RGBA{uint8(fg >> 16), uint8(fg >> 8), uint8(fg), 255},
		color.RGBA{uint8(bg >> 16), uint8(bg >> 8), uint8(bg), 255}
}

// needsIdleBlinkHeartbeat marks GogpuRenderer as needing the idle blink
// heartbeat in FrameManager.Run(). See softwareBlinkRenderer.
func (r *GogpuRenderer) needsIdleBlinkHeartbeat() {}
