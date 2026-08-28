//go:build linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || solaris || illumos

package vtui

import (
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// fallbackFontPaths is consulted in order, so the entries are sorted by how
// much of Unicode they carry rather than by how likely they are to exist: a
// missing file costs one failed stat, whereas a narrow font that happens to
// be listed first would answer for glyphs a later, wider font renders better.
//
// The list is deliberately long. Distributions disagree about where Noto CJK
// lives, and a Windows install outside a CJK locale has none of the Japanese
// supplemental fonts, so a short list quietly degrades to no fallback at all.
var fallbackFontPaths = []string{
	// Linux — CJK
	"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
	"/usr/share/fonts/opentype/noto/NotoSansCJK-VF.otf.ttc",
	"/usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc",
	"/usr/share/fonts/noto-cjk/NotoSansCJK-VF.otf.ttc",
	"/usr/share/fonts/google-noto-cjk/NotoSansCJK-Regular.ttc",
	"/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc",
	"/usr/share/fonts/noto/NotoSansCJK-Regular.ttc",
	"/usr/share/fonts/opentype/source-han-sans/SourceHanSans-Regular.otc",
	"/usr/share/fonts/adobe-source-han-sans/SourceHanSans-Regular.otc",
	"/usr/share/fonts/truetype/droid/DroidSansFallbackFull.ttf",
	"/usr/share/fonts/truetype/wqy/wqy-microhei.ttc",
	"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc",
	"/usr/share/fonts/wqy-microhei/wqy-microhei.ttc",
	"/usr/share/fonts/truetype/arphic/uming.ttc",
	"/usr/share/fonts/truetype/arphic/ukai.ttc",
	// Linux — emoji and general symbol coverage
	"/usr/share/fonts/google-noto-emoji-fonts/NotoEmoji-Regular.ttf",
	"/usr/share/fonts/noto/NotoEmoji-Regular.ttf",
	"/usr/share/fonts/truetype/noto/NotoEmoji-Regular.ttf",
	"/usr/share/fonts/gdouros-symbola/Symbola.ttf",
	"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	"/usr/share/fonts/TTF/DejaVuSans.ttf",
	"/usr/share/fonts/truetype/unifont/unifont.ttf",
	// Windows — CJK. Only the Yu Gothic and Microsoft families ship outside a
	// CJK locale; msgothic/msmincho/meiryo arrive with the Japanese
	// supplemental fonts and are frequently absent.
	`C:\Windows\Fonts\msyh.ttc`,
	`C:\Windows\Fonts\msjh.ttc`,
	`C:\Windows\Fonts\YuGothM.ttc`,
	`C:\Windows\Fonts\YuGothR.ttc`,
	`C:\Windows\Fonts\simsun.ttc`,
	`C:\Windows\Fonts\malgun.ttf`,
	`C:\Windows\Fonts\msgothic.ttc`,
	`C:\Windows\Fonts\msmincho.ttc`,
	`C:\Windows\Fonts\meiryo.ttc`,
	`C:\Windows\Fonts\mingliu.ttc`,
	`C:\Windows\Fonts\batang.ttc`,
	`C:\Windows\Fonts\gulim.ttc`,
	`C:\Windows\Fonts\arialuni.ttf`,
	// Windows — emoji and symbols
	`C:\Windows\Fonts\seguiemj.ttf`,
	`C:\Windows\Fonts\seguisym.ttf`,
	`C:\Windows\Fonts\segoeui.ttf`,
	// macOS
	"/System/Library/Fonts/PingFang.ttc",
	"/System/Library/Fonts/Hiragino Sans GB.ttc",
	"/System/Library/Fonts/AppleSDGothicNeo.ttc",
	"/System/Library/Fonts/STHeiti Light.ttc",
	"/System/Library/Fonts/Supplemental/Songti.ttc",
	"/System/Library/Fonts/Apple Color Emoji.ttc",
	"/Library/Fonts/Arial Unicode.ttf",
}

var runFontconfigMonospace = func() (string, error) {
	out, err := exec.Command("fc-match", "-f", "%{file}", "monospace").Output()
	return strings.TrimSpace(string(out)), err
}

const maxFontconfigEmojiPaths = 4

// runFontconfigEmoji asks fontconfig for the small set of fonts it would use
// for an emoji code point. The static fallback list covers common locations,
// but package layouts vary too much for it to find every distro's emoji font.
// Keeping this a variable makes the lookup deterministic in tests without
// making fontconfig a build-time dependency.
var runFontconfigEmoji = func() ([]string, error) {
	out, err := exec.Command("fc-match", "-s", "-f", "%{file}\n", "emoji:charset=1f600:color=false").Output()
	if err != nil {
		return nil, err
	}
	paths := parseFontconfigPaths(string(out))
	if len(paths) > maxFontconfigEmojiPaths {
		paths = paths[:maxFontconfigEmojiPaths]
	}
	return paths, nil
}

func parseFontconfigPaths(output string) []string {
	var paths []string
	seen := make(map[string]struct{})
	for _, line := range strings.Split(output, "\n") {
		path := strings.TrimSpace(line)
		if path == "" || !filepath.IsAbs(path) {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	return paths
}

// runeCoverage is a bitmap of the runes a font has glyphs for, over all of
// Unicode, so it answers definitively for every rune. An earlier version
// covered only planes 0 and 1 and answered "maybe" above that, which forced a
// re-parse of the font file for every consultation of a plane-2 CJK
// ideograph. At 136 KB per probed font the full bitmap is still noise next to
// the multi-megabyte parse it replaces.
const runeCoverageMax = 0x110000

type runeCoverage struct {
	bits [runeCoverageMax / 64]uint64
}

func (c *runeCoverage) has(r rune) bool {
	if r < 0 || r >= runeCoverageMax {
		return false
	}
	return c.bits[r>>6]&(1<<(uint(r)&63)) != 0
}

func parseFontBytes(data []byte) (*opentype.Font, error) {
	f, err := opentype.Parse(data)
	if err == nil {
		return f, nil
	}
	col, err2 := opentype.ParseCollection(data)
	if err2 == nil && col.NumFonts() > 0 {
		f, err3 := col.Font(0)
		if err3 == nil {
			return f, nil
		}
	}
	return nil, err
}

// fontFallbackChain opens a fallback font file the first time a rune actually
// needs it, and keeps in memory only the fonts that have rendered a glyph.
// The eager approach parsed every fallback font on the machine at startup —
// on a stock macOS that is ~400 MB of font files, held twice over by gg in
// the gogpu backend (~800 MB of heap), almost all of it for glyphs that never
// render.
//
// Both GUI backends share this engine; the face type and the parse mechanics
// arrive as closures. The chain itself carries the policy the backends must
// agree on:
//
//   - Fonts are consulted in list order, so the priority of fallbackFontPaths
//     is preserved exactly.
//   - A font is retained, and returned, only if it renders a glyph for the
//     rune. covers alone is not enough: a color-bitmap emoji font maps runes
//     in its cmap that x/image cannot rasterize, and selecting on the cmap
//     would blank those runes instead of falling through to the next font.
//   - A font that was parsed and not retained is condensed into a
//     runeCoverage bitmap and dropped, so from then on it answers "does this
//     font cover r" without being in memory.
//   - Every answer — including "nobody renders it" — is memoised per rune, so
//     a rune no font covers pays for the full walk exactly once.
type fontFallbackChain struct {
	mu sync.Mutex

	open    func(path string) (any, error) // parse a file into a face
	covers  func(face any, r rune) bool    // cmap probe, cheap; feeds runeCoverage
	renders func(face any, r rune) bool    // can the face produce an actual glyph image
	drop    func(face any)                 // release a face that was not retained
	logTag  string                         // backend prefix for DebugLog lines

	entries  []fontFallbackEntry
	resolved map[rune]any // memoised faceFor answers; nil = nobody renders it
}

type fontFallbackEntry struct {
	path   string
	failed bool          // unreadable or unparseable; never try again
	cov    *runeCoverage // built when the font is probed and dropped, nil until then
	face   any           // retained only once the font has rendered a glyph
}

// faceFor returns the first fallback face that renders a glyph for r, walking
// the fonts in priority order, or nil when none of them does. A repeat call
// for the same rune is a map lookup.
func (c *fontFallbackChain) faceFor(r rune) any {
	c.mu.Lock()
	defer c.mu.Unlock()
	if f, ok := c.resolved[r]; ok {
		return f
	}
	f := c.resolveLocked(r)
	if c.resolved == nil {
		c.resolved = make(map[rune]any, 256)
	}
	c.resolved[r] = f
	return f
}

func (c *fontFallbackChain) resolveLocked(r rune) any {
	for i := range c.entries {
		e := &c.entries[i]
		if e.failed {
			continue
		}
		if e.face != nil {
			if c.renders(e.face, r) {
				return e.face
			}
			continue
		}
		if e.cov != nil && !e.cov.has(r) {
			continue
		}

		t0 := time.Now()
		face, err := c.open(e.path)
		if err != nil {
			e.failed = true
			DebugLog("%s: fallback present but unusable: %s: %v", c.logTag, e.path, err)
			continue
		}
		if c.renders(face, r) {
			e.face = face
			DebugLog("%s: fallback loaded for U+%04X: %s in %v", c.logTag, r, e.path, time.Since(t0))
			return face
		}
		// Parsed and not needed. Condense into the bitmap — unless a warm()
		// sweep already has; a retained font never needs one — and drop; the
		// bitmap answers for this font from now on.
		if e.cov == nil {
			e.cov = c.coverageOf(face)
		}
		c.drop(face)
		DebugLog("%s: fallback probed for U+%04X and dropped: %s in %v", c.logTag, r, e.path, time.Since(t0))
	}
	return nil
}

func (c *fontFallbackChain) coverageOf(face any) *runeCoverage {
	cov := &runeCoverage{}
	for r := rune(0); r < runeCoverageMax; r++ {
		if r >= 0xD800 && r <= 0xDFFF {
			continue // surrogates are not runes a cmap can hold
		}
		if c.covers(face, r) {
			cov.bits[r>>6] |= 1 << (uint(r) & 63)
		}
	}
	return cov
}

// warm parses each not-yet-consulted entry once, in the background, building
// its coverage bitmap and dropping the parse. Without it the first rune the
// primary font lacks pays for parsing every fallback font ahead of the
// covering one — synchronously, mid-frame, at whatever moment the first emoji
// or CJK character happens to appear. The lock is taken per entry, so a
// concurrent faceFor interleaves between files instead of waiting out the
// whole sweep.
func (c *fontFallbackChain) warm() {
	go func() {
		for i := 0; ; i++ {
			c.mu.Lock()
			if i >= len(c.entries) {
				c.mu.Unlock()
				return
			}
			e := &c.entries[i]
			if !e.failed && e.face == nil && e.cov == nil {
				t0 := time.Now()
				if face, err := c.open(e.path); err != nil {
					e.failed = true
					DebugLog("%s: fallback present but unusable: %s: %v", c.logTag, e.path, err)
				} else {
					e.cov = c.coverageOf(face)
					c.drop(face)
					DebugLog("%s: fallback coverage warmed: %s in %v", c.logTag, e.path, time.Since(t0))
				}
			}
			c.mu.Unlock()
		}
	}()
}

// close releases every retained face and forgets the memoised answers.
// Coverage bitmaps and failure marks survive: they stay correct for the files
// on disk.
func (c *fontFallbackChain) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.entries {
		if f := c.entries[i].face; f != nil {
			c.drop(f)
			c.entries[i].face = nil
		}
	}
	c.resolved = nil
}

// newGUIFallbackChain binds the shared chain to x/image faces. covers is the
// plain cmap probe; renders actually rasterizes, because a color-bitmap emoji
// font (e.g. NotoColorEmoji) passes the cmap probe and then fails Glyph with
// ErrColoredGlyph — such a font must be passed over, not selected.
func newGUIFallbackChain(size, dpi float64) *fontFallbackChain {
	return &fontFallbackChain{
		logTag: "GUI_FONT",
		open: func(path string) (any, error) {
			return openFace(path, size, dpi)
		},
		covers: func(face any, r rune) bool {
			_, ok := face.(font.Face).GlyphAdvance(r)
			return ok
		},
		renders: func(face any, r rune) bool {
			_, _, _, _, ok := face.(font.Face).Glyph(fixed.Point26_6{}, r)
			return ok
		},
		drop: func(face any) { _ = face.(font.Face).Close() },
	}
}

// openFace builds a face the way both the primary loop and the fallback chain
// need one: same options, so primary and fallback glyphs share metrics.
func openFace(path string, size, dpi float64) (font.Face, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := parseFontBytes(data)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(f, &opentype.FaceOptions{
		Size:    size,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})
}

type fallbackFace struct {
	faces []font.Face
	chain *fontFallbackChain
}

func (f *fallbackFace) Close() error {
	var err error
	for _, face := range f.faces {
		if e := face.Close(); e != nil {
			err = e
		}
	}
	if f.chain != nil {
		f.chain.close()
	}
	return err
}

func (f *fallbackFace) Metrics() font.Metrics {
	if len(f.faces) > 0 {
		return f.faces[0].Metrics()
	}
	return font.Metrics{}
}

func (f *fallbackFace) Kern(r0, r1 rune) fixed.Int26_6 {
	if len(f.faces) > 0 {
		return f.faces[0].Kern(r0, r1)
	}
	return 0
}

// chainFace resolves r through the lazy chain, or nil when the chain is
// absent or exhausted. Each glyph method consults it only after every
// already-open face has answered "no glyph". A non-nil result is a face that
// renders r — the chain selects by rasterization, not the cmap — so the glyph
// methods can return its answer without a further ok-check.
func (f *fallbackFace) chainFace(r rune) font.Face {
	if f.chain == nil {
		return nil
	}
	face, _ := f.chain.faceFor(r).(font.Face)
	return face
}

func (f *fallbackFace) GlyphBounds(r rune) (bounds fixed.Rectangle26_6, advance fixed.Int26_6, ok bool) {
	for _, face := range f.faces {
		bounds, advance, ok = face.GlyphBounds(r)
		if ok {
			return bounds, advance, ok
		}
	}
	if face := f.chainFace(r); face != nil {
		return face.GlyphBounds(r)
	}
	if len(f.faces) > 0 {
		return f.faces[0].GlyphBounds(r)
	}
	return fixed.Rectangle26_6{}, 0, false
}

func (f *fallbackFace) GlyphAdvance(r rune) (advance fixed.Int26_6, ok bool) {
	for _, face := range f.faces {
		advance, ok = face.GlyphAdvance(r)
		if ok {
			return advance, ok
		}
	}
	if face := f.chainFace(r); face != nil {
		return face.GlyphAdvance(r)
	}
	if len(f.faces) > 0 {
		return f.faces[0].GlyphAdvance(r)
	}
	return 0, false
}

func (f *fallbackFace) Glyph(dot fixed.Point26_6, r rune) (dr image.Rectangle, mask image.Image, maskp image.Point, advance fixed.Int26_6, ok bool) {
	for _, face := range f.faces {
		dr, mask, maskp, advance, ok = face.Glyph(dot, r)
		if ok {
			return dr, mask, maskp, advance, ok
		}
	}
	if face := f.chainFace(r); face != nil {
		return face.Glyph(dot, r)
	}
	if len(f.faces) > 0 {
		return f.faces[0].Glyph(dot, r)
	}
	return image.Rectangle{}, nil, image.Point{}, 0, false
}

func getFontCandidates(fontName string) []string {
	var candidates []string
	if fontName != "" {
		candidates = append(candidates, fontName)
		if !strings.HasSuffix(strings.ToLower(fontName), ".ttf") {
			candidates = append(candidates, fontName+".ttf")
		}
		dirs := []string{
			`C:\Windows\Fonts`,
			"/usr/share/fonts/truetype",
			"/usr/share/fonts/TTF",
			"/usr/local/share/fonts",
			"/System/Library/Fonts/Supplemental",
			"/System/Library/Fonts",
		}
		for _, dir := range dirs {
			candidates = append(candidates, filepath.Join(dir, fontName))
			if !strings.HasSuffix(strings.ToLower(fontName), ".ttf") {
				candidates = append(candidates, filepath.Join(dir, fontName+".ttf"))
			}
			entries, err := os.ReadDir(dir)
			if err == nil {
				for _, e := range entries {
					if e.IsDir() {
						candidates = append(candidates, filepath.Join(dir, e.Name(), fontName))
						if !strings.HasSuffix(strings.ToLower(fontName), ".ttf") {
							candidates = append(candidates, filepath.Join(dir, e.Name(), fontName+".ttf"))
						}
					}
				}
			}
		}
	}

	if fontName == "" {
		if path := fontconfigMonospacePath(); path != "" {
			candidates = append(candidates, path)
		}
	}

	defaultPaths := []string{
		`C:\Windows\Fonts\consola.ttf`,
		`C:\Windows\Fonts\lucon.ttf`,
		`C:\Windows\Fonts\cour.ttf`,
		`C:\Windows\Fonts\arial.ttf`,
		"/usr/share/fonts/truetype/ubuntu/UbuntuMono-R.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationMono-Regular.ttf",
		"/usr/share/fonts/TTF/DejaVuSansMono.ttf",
		"/usr/share/fonts/google-noto/NotoSansMono-Regular.ttf",
		"/usr/share/fonts/adwaita-mono-fonts/AdwaitaMono-Regular.ttf",
		"/System/Library/Fonts/Supplemental/Courier New.ttf",
		"/System/Library/Fonts/Monaco.ttf",
	}
	candidates = append(candidates, defaultPaths...)
	return candidates
}

// fontconfigMonospacePath asks the platform's fontconfig database for the
// system monospace default. Fedora and other distributions commonly install
// it outside the traditional DejaVu paths.
func fontconfigMonospacePath() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	path, err := runFontconfigMonospace()
	if err != nil {
		return ""
	}
	if !filepath.IsAbs(path) {
		return ""
	}
	return path
}

// fallbackPathsForGUI adds fontconfig's runtime emoji matches to the portable
// static list. The returned order keeps the curated paths first and only
// appends paths that are not already present, so existing font priorities stay
// stable while distro-specific fonts become discoverable.
func fallbackPathsForGUI() []string {
	paths := append([]string(nil), fallbackFontPaths...)
	if runtime.GOOS != "linux" {
		return paths
	}

	dynamic, err := runFontconfigEmoji()
	if err != nil {
		return paths
	}
	seen := make(map[string]struct{}, len(paths)+len(dynamic))
	for _, path := range paths {
		seen[path] = struct{}{}
	}
	for _, path := range dynamic {
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	return paths
}

// loadBestFont attempts to find a suitable monospace TTF font on the system.
// If none is found, it falls back to a built-in bitmap font.
func loadBestFont(fontName string, size float64, dpi float64) (font.Face, int, int) {
	if size <= 0 {
		size = 18.0
	}

	var primaryFace font.Face
	var cellW, cellH int

	for _, path := range getFontCandidates(fontName) {
		face, err := openFace(path, size, dpi)
		if err != nil {
			if !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "GUI_FONT: Error loading %s: %v\n", path, err)
			}
			continue
		}

		metrics := face.Metrics()
		cellH = (metrics.Ascent + metrics.Descent).Ceil()
		advance, _ := face.GlyphAdvance('A')
		cellW = advance.Ceil()

		msg := fmt.Sprintf("GUI_FONT: Successfully loaded %s (%dx%d)", path, cellW, cellH)
		fmt.Fprintln(os.Stderr, msg)
		DebugLog("%s", msg)
		primaryFace = face
		break
	}

	if primaryFace == nil {
		// Fallback to basicfont if no TTF found
		DebugLog("GUI_FONT: CRITICAL - No TTF font found! Falling back to basicfont 7x13 (ASCII only!)")
		return basicfont.Face7x13, 7, 13
	}

	// Existence is all the startup probe checks; the files themselves are
	// opened by the chain on first use. The old loop read and parsed every
	// fallback font here, which held hundreds of megabytes for glyphs most
	// sessions never draw.
	var chain *fontFallbackChain
	for _, path := range fallbackPathsForGUI() {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		DebugLog("GUI_FONT: fallback present, deferred until first use: %s", path)
		if chain == nil {
			chain = newGUIFallbackChain(size, dpi)
		}
		chain.entries = append(chain.entries, fontFallbackEntry{path: path})
	}
	if chain != nil {
		chain.warm()
	}

	return &fallbackFace{faces: []font.Face{primaryFace}, chain: chain}, cellW, cellH
}
