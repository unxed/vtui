package vtui

import (
	"image"
	"image/color"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/soniakeys/quant/median"
)

// Sixel output backend: the lowest common denominator of terminal graphics,
// which is what makes images work on Windows Terminal, conhost and the ConPTY
// bridge. Only the modern ConPTY build (Windows Terminal 1.22+) forwards image
// escapes, so detection queries DA1 rather than assuming it.
//
// Sixel has no server-side image store, so the encoder caches the *encoded*
// bytes keyed on surface content, crop and destination size: a still picture
// or a pan inside a cached crop costs a cursor move and a byte copy, like the
// kitty backend's upload cache.

const (
	// sixelCacheLimit bounds the encoded images kept in memory: a viewer
	// shows one picture at a time, a gallery a screenful.
	sixelCacheLimit = 48

	// sixelMaxColors is the colour register count; one value is the
	// transparent sentinel, so the palette has 255 entries.
	sixelMaxColors = 255

	// sixelCellFallbackW/H is the assumed cell geometry when the terminal
	// cannot report one, matching the viewer's fallback so placement and
	// rendering agree on the aspect ratio.
	sixelCellFallbackW = 8
	sixelCellFallbackH = 16

	// sixelIndexTransparent marks a zero-alpha pixel: left unencoded so P2=1
	// keeps whatever the text layer drew there.
	sixelIndexTransparent = 0xFF
)

// sixelGrid returns the sixel raster size for a cell rectangle: with
// Pan;Pad = 1;1, a cols x rows box rasterises to cols*cw columns and rows*ch
// rows. The last band may be partial; the emit loop leaves rows beyond dh unset.
func sixelGrid(cols, rows, cw, ch int) (dw, dh int) {
	if cw <= 0 || ch <= 0 {
		cw, ch = sixelCellFallbackW, sixelCellFallbackH
	}
	dw = cols * cw
	dh = rows * ch
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}
	return dw, dh
}

// sixelCellSize resolves the cell geometry to rasterise to. conhost draws
// sixel into a fixed 10x20 virtual cell whatever the font is, so WSL sessions
// (WT_SESSION) and native Windows builds use that too. Terminals that
// rasterise sixel themselves — WezTerm in particular — keep their font cell
// even on Windows; elsewhere the reported cell wins, falling back to 8x16.
func sixelCellSize(cw, ch int) (int, int) {
	return sixelCellSizeWith(os.Getenv, runtime.GOOS, cw, ch)
}

func sixelCellSizeWith(env func(string) string, goos string, cw, ch int) (int, int) {
	if isWindowsSixelHost(env, goos) {
		return 10, 20
	}
	if cw <= 0 || ch <= 0 {
		return sixelCellFallbackW, sixelCellFallbackH
	}
	return cw, ch
}

// sixelPalette is the fixed 255-colour palette every image is quantised to:
// the 6x6x6 cube plus 39 greys. Fixed keeps encoding deterministic and
// allocation-free; the dither below hides the banding it would otherwise show.
var sixelPalette [sixelMaxColors][3]byte

// sixelPalI is the palette as int32, used by the dithering error paths.
var sixelPalI [sixelMaxColors][3]int32

// sixelLUT maps 15-bit RGB (5 bits per channel) to the nearest palette entry,
// making per-pixel quantisation a single lookup.
var sixelLUT [1 << 15]uint8

// sixelPalDef caches the ";2;R;G;B" colour triple per entry so the register
// header emits one string instead of formatting three percentages each.
var sixelPalDef [sixelMaxColors]string

func init() {
	i := 0
	for r := 0; r < 6; r++ {
		for g := 0; g < 6; g++ {
			for b := 0; b < 6; b++ {
				c := [3]byte{byte(r * 51), byte(g * 51), byte(b * 51)}
				sixelPalette[i] = c
				sixelPalI[i] = [3]int32{int32(c[0]), int32(c[1]), int32(c[2])}
				i++
			}
		}
	}
	for g := 0; g < 39; g++ {
		v := byte(g * 255 / 38)
		c := [3]byte{v, v, v}
		sixelPalette[i] = c
		sixelPalI[i] = [3]int32{int32(c[0]), int32(c[1]), int32(c[2])}
		i++
	}

	copy(sixelLUT[:], buildSixelLUT(sixelPalI[:]))

	for p := 0; p < sixelMaxColors; p++ {
		c := sixelPalette[p]
		sixelPalDef[p] = ";2;" + strconv.Itoa(int(c[0])*100/255) +
			";" + strconv.Itoa(int(c[1])*100/255) +
			";" + strconv.Itoa(int(c[2])*100/255)
	}
}

// buildSixelLUT maps 15-bit RGB (5 bits per channel) to the nearest palette entry.
func buildSixelLUT(palI [][3]int32) []uint8 {
	lut := make([]uint8, 1<<15)
	for k := range lut {
		r := int32(k>>10&31)<<3 | 4
		g := int32(k>>5&31)<<3 | 4
		b := int32(k&31)<<3 | 4
		best, bestd := 0, int32(1<<31-1)
		for j := 0; j < len(palI); j++ {
			dr := r - palI[j][0]
			dg := g - palI[j][1]
			db := b - palI[j][2]
			if d := dr*dr + dg*dg + db*db; d < bestd {
				best, bestd = j, d
				if d == 0 {
					break
				}
			}
		}
		lut[k] = uint8(best)
	}
	return lut
}

// sixelCacheEntry is one encoded image, ready to be pasted after a cursor move.
// It is a list because the layered encoder writes several DCS strings to the
// same spot; every other mode leaves exactly one in it.
type sixelCacheEntry struct {
	layers []string
}

// sixelEncoder caches encoded images so panning re-sends bytes instead of
// re-quantising the picture.
type sixelEncoder struct {
	cache map[uint64]sixelCacheEntry
	order []uint64

	// cellSize resolves the raster cell geometry; nil means sixelCellSize.
	// Tests pin it so output does not depend on the host terminal.
	cellSize func(cw, ch int) (int, int)

	// adaptive uses a per-image median-cut palette: slower, but far less
	// banding on photos with a narrow gamut.
	adaptive bool

	// trueColor gives every band its own palette, which is how a format
	// with 256 registers carries a photograph. The default; the two
	// single-palette encoders remain for a decoder that resolves its
	// registers at the end of the image rather than as it goes.
	trueColor bool

	// layered stacks several transparent images at the same spot, each with
	// a palette of its own. See encodeLayered.
	layered bool

	// layerMax and layerBudget bound that stack. Fields rather than the
	// constants they are set from, so a test can pin them and so a caller
	// on a slow link has somewhere to put a smaller number later.
	layerMax    int
	layerBudget int

	// Scratch reused across encodes; render-thread only, so never locked.
	idx      []byte
	emitBits []byte
	emitUsed []bool
}

// reuseIdx returns an idx-sized buffer, reusing the encoder's when it fits.
func (s *sixelEncoder) reuseIdx(n int) []byte {
	if cap(s.idx) < n {
		s.idx = make([]byte, n)
	}
	return s.idx[:n]
}

// reuseEmitBits returns an n-byte bit-plane buffer, reusing the encoder's.
func (s *sixelEncoder) reuseEmitBits(n int) []byte {
	if cap(s.emitBits) < n {
		s.emitBits = make([]byte, n)
	}
	return s.emitBits[:n]
}

// reuseEmitUsed returns an n-bool per-register mark, reusing the encoder's.
func (s *sixelEncoder) reuseEmitUsed(n int) []bool {
	if cap(s.emitUsed) < n {
		s.emitUsed = make([]bool, n)
	}
	return s.emitUsed[:n]
}

func newSixelEncoder() *sixelEncoder {
	return newSixelEncoderWith(os.Getenv)
}

// newSixelEncoderWith is newSixelEncoder with the environment injected so
// terminal selection and VTUI_SIXEL_PALETTE are testable without a terminal.
func newSixelEncoderWith(env func(string) string) *sixelEncoder {
	return newSixelEncoderWithOS(env, runtime.GOOS)
}

// newSixelEncoderWithOS is newSixelEncoderWith with the host OS injected as
// well. The Windows console path must be testable on the Unix CI builders.
func newSixelEncoderWithOS(env func(string) string, goos string) *sixelEncoder {
	mode := strings.ToLower(strings.TrimSpace(env("VTUI_SIXEL_PALETTE")))
	layered := mode == "layered"
	adaptive := mode == "adaptive"
	trueColor := mode == "truecolor" || mode == "per-band"
	named := mode == "adaptive" || mode == "fixed" || mode == "layered" || trueColor
	if mode == "" {
		layered = layeredSixelDefaultWith(env, goos)
		trueColor = !layered && trueColorSixelDefault(env)
		adaptive = !layered && !trueColor
	} else if !named {
		// Preserve the old escape hatch: any explicit, unknown value means
		// that the caller has chosen the per-band stream deliberately.
		trueColor = true
	}
	return &sixelEncoder{
		cache:       make(map[uint64]sixelCacheEntry),
		adaptive:    adaptive,
		layered:     layered,
		layerMax:    sixelLayerMax,
		layerBudget: sixelLayerBudget,
		trueColor:   trueColor,
	}
}

// layeredSixelDefault reports whether the terminal is known to composite
// overlapping transparent sixel images, which is what makes the layered
// encoder legal there.
//
// Windows Terminal and native OpenConsole are the cases that matter: they
// share the parser which keeps a raster indexed until it flushes, so an
// encoder that redefines a register between bands can recolour bands already
// decoded. The full-colour form is not available there. Layering is, and it
// is not a workaround the terminal merely tolerates: see
// microsoft/terminal#20020, where the one report of it breaking turned out
// to be the reporter's own encoder omitting P2=1, and where sixteen and even
// two hundred and fifty-six layers are reported working.
//
// WezTerm and foot are the terminals whose per-band behaviour has been
// verified. Unknown terminals use one adaptive palette instead of receiving a
// stream whose palette semantics have not been checked.
func layeredSixelDefault(env func(string) string) bool {
	return layeredSixelDefaultWith(env, runtime.GOOS)
}

func layeredSixelDefaultWith(env func(string) string, goos string) bool {
	return isWindowsSixelHost(env, goos)
}

// isWindowsSixelHost identifies Windows Terminal, including WSL sessions
// hosted by it, and native OpenConsole. WezTerm on Windows is excluded: it
// owns the terminal-side SIXEL renderer behind the ConPTY bridge.
func isWindowsSixelHost(env func(string) string, goos string) bool {
	return env("WT_SESSION") != "" || (goos == "windows" && !isWezTermEnv(env))
}

func trueColorSixelDefault(env func(string) string) bool {
	term := strings.ToLower(env("TERM"))
	program := strings.ToLower(env("TERM_PROGRAM"))
	return isWezTermEnv(env) || strings.Contains(term, "foot") || strings.Contains(program, "foot")
}

// adaptiveSixelPalette builds a median-cut palette of up to 255 colours with
// its LUT and ";2;R;G;B" definitions. Large rasters are subsampled first:
// palette quality barely depends on pixel count, but median-cut cost does.
func adaptiveSixelPalette(scaled *ImageSurface) ([][3]int32, []string, []uint8) {
	// Surface RGBA is straight (non-premultiplied), i.e. exactly NRGBA, so it
	// wraps as image.NRGBA without copying.
	src := image.Image(&image.NRGBA{Pix: scaled.Pix, Stride: scaled.Stride, Rect: image.Rect(0, 0, scaled.Width, scaled.Height)})
	const budget = 1 << 18
	if scaled.Width*scaled.Height > budget {
		step := 2
		for (scaled.Width/step)*(scaled.Height/step) > budget {
			step++
		}
		sample := image.NewNRGBA(image.Rect(0, 0, scaled.Width/step, scaled.Height/step))
		for y := 0; y < scaled.Height/step; y++ {
			so := y * step * scaled.Stride
			do := y * sample.Stride
			for x := 0; x < scaled.Width/step; x++ {
				copy(sample.Pix[do+x*4:do+x*4+4], scaled.Pix[so+x*step*4:so+x*step*4+4])
			}
		}
		src = sample
	}

	pal := median.Quantizer(sixelMaxColors).Quantize(make(color.Palette, 0, sixelMaxColors), src)
	palI := make([][3]int32, len(pal))
	palDef := make([]string, len(pal))
	for i, c := range pal {
		r, g, b, _ := c.RGBA()
		r8, g8, b8 := int32(r>>8), int32(g>>8), int32(b>>8)
		palI[i] = [3]int32{r8, g8, b8}
		palDef[i] = ";2;" + strconv.Itoa(int(r8)*100/255) +
			";" + strconv.Itoa(int(g8)*100/255) +
			";" + strconv.Itoa(int(b8)*100/255)
	}
	return palI, palDef, buildSixelLUT(palI)
}

// Reset is a no-op: sixel keeps no terminal-side state; a forced redraw
// re-paints the text cells and the fresh sixel draws over them.
func (s *sixelEncoder) Reset(sb kittyBuffer) {}

// Render replaces the currently visible placements with the given list.
func (s *sixelEncoder) Render(sb kittyBuffer, list []ImagePlacement, cw, ch int) {
	if s.cellSize != nil {
		cw, ch = s.cellSize(cw, ch)
	} else {
		cw, ch = sixelCellSize(cw, ch)
	}
	for i := range list {
		p := &list[i]
		if !p.Surface.Valid() || p.Cols <= 0 || p.Rows <= 0 {
			continue
		}
		sx, sy, sw, sh := p.Source()
		if sw <= 0 || sh <= 0 {
			continue
		}
		dw, dh := sixelGrid(p.Cols, p.Rows, cw, ch)
		if dw <= 0 || dh <= 0 {
			continue
		}

		key := nativeCacheKey(p.Surface.Hash(), sx, sy, sw, sh, dw, dh)
		entry, ok := s.cache[key]
		if !ok {
			entry = sixelCacheEntry{layers: s.encodeLayers(p.Surface, sx, sy, sw, sh, dw, dh)}
			s.cache[key] = entry
			s.order = append(s.order, key)
			if len(s.order) > sixelCacheLimit {
				delete(s.cache, s.order[0])
				s.order = s.order[1:]
			}
		}
		// Every layer starts at the same cell. It has to be re-stated: a
		// sixel dump leaves the text cursor at the sixel active position,
		// which is the row the raster reached, so the second layer would
		// otherwise land below the first.
		for _, layer := range entry.layers {
			s.writeCursor(sb, p.Row+1, p.Col+1)
			sb.WriteString(layer)
		}
	}
}

func (s *sixelEncoder) writeCursor(sb kittyBuffer, row, col int) {
	sb.WriteString("\x1b[")
	sixelWriteCoord(sb, row)
	sb.WriteByte(';')
	sixelWriteCoord(sb, col)
	sb.WriteByte('H')
}

// sixelWriteCoord appends a coordinate digit by digit through WriteByte so
// the cache-hit path stays allocation-free through the kittyBuffer interface.
func sixelWriteCoord(sb kittyBuffer, n int) {
	if n >= 10 {
		sixelWriteCoord(sb, n/10)
	}
	sb.WriteByte(byte('0' + n%10))
}

// sixelWriteInt appends a non-negative integer in decimal without allocating.
func sixelWriteInt(sb *strings.Builder, scratch *[16]byte, v int) {
	sb.Write(strconv.AppendInt(scratch[:0], int64(v), 10))
}

// encodeLayers returns the DCS strings to write at the placement, in order.
// Every mode but the layered one answers with a single string; see
// graphics_sixel_layered.go for the one that does not.
func (s *sixelEncoder) encodeLayers(surf *ImageSurface, sx, sy, sw, sh, dw, dh int) []string {
	if !s.layered {
		return one(s.encode(surf, sx, sy, sw, sh, dw, dh))
	}
	scaled := s.scaleFor(surf, sx, sy, sw, sh, dw, dh)
	if scaled == nil {
		return nil
	}
	return s.encodeLayered(scaled, dw, dh)
}

// scaleFor crops and scales the source to the raster the placement needs.
func (s *sixelEncoder) scaleFor(surf *ImageSurface, sx, sy, sw, sh, dw, dh int) *ImageSurface {
	src := surf
	if sx != 0 || sy != 0 || sw != surf.Width || sh != surf.Height {
		src = surf.Crop(sx, sy, sw, sh)
	}
	return ScaleSurface(src, dw, dh)
}

// encode scales the crop to the destination size, quantises it and returns one
// complete DCS string, using the fixed cube or an adaptive median-cut palette.
func (s *sixelEncoder) encode(surf *ImageSurface, sx, sy, sw, sh, dw, dh int) string {
	scaled := s.scaleFor(surf, sx, sy, sw, sh, dw, dh)
	if scaled == nil {
		return ""
	}

	if s.trueColor {
		return s.encodeTrueColor(scaled, dw, dh)
	}

	idx := s.reuseIdx(dw * dh)
	var palI [][3]int32
	var palDef []string
	if s.adaptive {
		var lut []uint8
		palI, palDef, lut = adaptiveSixelPalette(scaled)
		sixelQuantizePal(scaled, idx, lut, palI)
	} else {
		palI, palDef = sixelPalI[:], sixelPalDef[:]
		sixelQuantize(scaled, idx)
	}
	return s.emitIndexed(idx, palDef, dw, dh)
}

// one is the single-layer answer, and nothing at all when the encoder had
// nothing to say.
func one(data string) []string {
	if data == "" {
		return nil
	}
	return []string{data}
}

// emitIndexed writes one complete DCS for pixels already quantised into idx,
// where sixelIndexTransparent means "leave this pixel alone".
func (s *sixelEncoder) emitIndexed(idx []byte, palDef []string, dw, dh int) string {
	remap, regs := sixelRegisters(idx)
	if len(regs) == 0 {
		return ""
	}

	var sb strings.Builder
	var scratch [16]byte
	// Pre-size the builder; a wrong guess costs one growth instead of many.
	sb.Grow(dw*dh/2 + len(regs)*(dh/6*2+16) + 64)
	// P2=1 keeps screen content behind transparent pixels; "1;1;W;H declares
	// square pixels and the image extent.
	sb.WriteString("\x1bP0;1;8q\"1;1;")
	sixelWriteInt(&sb, &scratch, dw)
	sb.WriteByte(';')
	sixelWriteInt(&sb, &scratch, dh)
	for _, p := range regs {
		sb.WriteByte('#')
		sixelWriteInt(&sb, &scratch, remap[p])
		sb.WriteString(palDef[p])
	}

	s.sixelEmitData(&sb, idx, remap, len(regs), dw, dh)
	sb.WriteString("\x1b\\")
	return sb.String()
}

// sixelQuantize maps the surface to the fixed-palette indices with
// Floyd-Steinberg dithering.
func sixelQuantize(surf *ImageSurface, idx []byte) {
	sixelQuantizePal(surf, idx, sixelLUT[:], sixelPalI[:])
}

// sixelQuantizePal is sixelQuantize with the palette's LUT and colours
// injected so adaptive palettes reuse the same dithering. Transparent pixels
// become the sentinel and neither receive nor diffuse error.
func sixelQuantizePal(surf *ImageSurface, idx []byte, lut []uint8, palI [][3]int32) {
	w, h := surf.Width, surf.Height
	pix, stride, opaque := surf.Pix, surf.Stride, surf.Opaque

	cur := make([][3]int32, w+2)
	next := make([][3]int32, w+2)
	for y := 0; y < h; y++ {
		so := y * stride
		do := y * w
		for x := 0; x < w; x++ {
			o := so + x*4
			if !opaque {
				a := pix[o+3]
				if a == 0 {
					idx[do+x] = sixelIndexTransparent
					continue
				}
				// Half-transparent pixels draw as their own colour: sixel is
				// opaque or untouched only, and blending with unknown text
				// below is not expressible.
			}
			r := clamp255(int32(pix[o]) + cur[x+1][0]/16)
			g := clamp255(int32(pix[o+1]) + cur[x+1][1]/16)
			b := clamp255(int32(pix[o+2]) + cur[x+1][2]/16)
			pi := int(lut[lutIndex(uint8(r), uint8(g), uint8(b))])
			idx[do+x] = byte(pi)
			er := int32(r) - palI[pi][0]
			eg := int32(g) - palI[pi][1]
			eb := int32(b) - palI[pi][2]
			cur[x+2][0] += er * 7
			cur[x+2][1] += eg * 7
			cur[x+2][2] += eb * 7
			next[x][0] += er * 3
			next[x][1] += eg * 3
			next[x][2] += eb * 3
			next[x+1][0] += er * 5
			next[x+1][1] += eg * 5
			next[x+1][2] += eb * 5
			next[x+2][0] += er
			next[x+2][1] += eg
			next[x+2][2] += eb
		}
		cur, next = next, cur
		for i := range next {
			next[i] = [3]int32{}
		}
	}
}

func clamp255(v int32) int32 {
	return min(int32(255), max(int32(0), v))
}

// lutIndex packs 8-bit RGB into the 15-bit key of sixelLUT.
func lutIndex(r, g, b byte) int {
	return int(r>>3)<<10 | int(g>>3)<<5 | int(b>>3)
}

// sixelRegisters compacts the palette to the entries the image actually uses
// and returns, for each palette index, its compact register number.
func sixelRegisters(idx []byte) (remap []int, regs []int) {
	var used [256]bool
	for _, v := range idx {
		if v != sixelIndexTransparent {
			used[v] = true
		}
	}
	remap = make([]int, 256)
	for i := range remap {
		remap[i] = -1
	}
	regs = make([]int, 0, 256)
	for p := 0; p < 256; p++ {
		if used[p] {
			remap[p] = len(regs)
			regs = append(regs, p)
		}
	}
	return remap, regs
}

// sixelEmitData turns quantised pixels into sixel scanlines: one pass per
// colour register per band so each character carries one colour, RLE-compressed.
func (s *sixelEncoder) sixelEmitData(sb *strings.Builder, idx []byte, remap []int, nreg, dw, dh int) {
	nbands := (dh + 5) / 6
	workers := scaleWorkers
	// Bands are independent apart from the '-' separator, so they can run in
	// parallel; small rasters stay serial, where goroutine setup dominates.
	if nbands < 2*workers {
		sixelEmitBands(sb, idx, remap, nreg, dw, dh, 0, nbands,
			s.reuseEmitBits(dw*nreg), s.reuseEmitUsed(nreg))
		return
	}

	outs := make([]strings.Builder, workers)
	bitsAll := s.reuseEmitBits(workers * dw * nreg)
	usedAll := s.reuseEmitUsed(workers * nreg)
	bandsPer := (nbands + workers - 1) / workers
	// Pre-size per-worker builders so they grow at most once.
	for i := range outs {
		outs[i].Grow(dw*dh/(2*workers) + nreg*(bandsPer*3+16) + 64)
	}
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		b0 := w * bandsPer
		b1 := b0 + bandsPer
		if b1 > nbands {
			b1 = nbands
		}
		if b0 >= b1 {
			continue
		}
		wg.Add(1)
		go func(w, b0, b1 int) {
			defer wg.Done()
			sixelEmitBands(&outs[w], idx, remap, nreg, dw, dh, b0, b1,
				bitsAll[w*dw*nreg:(w+1)*dw*nreg], usedAll[w*nreg:(w+1)*nreg])
		}(w, b0, b1)
	}
	wg.Wait()
	for i := range outs {
		sb.WriteString(outs[i].String())
	}
}

// sixelEmitBands emits the bands in [b0, b1). The '-' separator precedes every
// band but the global first, so splitting across workers matches the serial
// pass byte for byte.
func sixelEmitBands(sb *strings.Builder, idx []byte, remap []int, nreg, dw, dh, b0, b1 int, bits []byte, bandUsed []bool) {
	var scratch [16]byte
	for band := b0; band < b1; band++ {
		if band > 0 {
			sb.WriteByte('-')
		}
		clear(bits)
		for i := range bandUsed {
			bandUsed[i] = false
		}
		for p := 0; p < 6; p++ {
			y := band*6 + p
			if y >= dh {
				break
			}
			mask := byte(1 << uint(p))
			row := idx[y*dw : (y+1)*dw]
			for x := 0; x < dw; x++ {
				if r := remap[row[x]]; r >= 0 {
					bits[r*dw+x] |= mask
					bandUsed[r] = true
				}
			}
		}

		first := true
		for r := 0; r < nreg; r++ {
			if !bandUsed[r] {
				continue
			}
			if !first {
				sb.WriteByte('$')
			}
			first = false
			sb.WriteByte('#')
			sixelWriteInt(sb, &scratch, r)
			base := r * dw
			// Blank columns still advance the sixel cursor, so each gap needs a
			// zero-bit character or everything to the right shifts left and the
			// picture tears into diagonal stripes. Trailing blanks can be
			// skipped: the following '$' or '-' returns the cursor to the edge.
			end := dw
			for end > 0 && bits[base+end-1] == 0 {
				end--
			}
			var ch byte
			cnt := 0
			for x := 0; x < end; x++ {
				b := bits[base+x]
				if b != ch {
					if cnt > 0 {
						sixelRun(sb, ch, cnt)
					}
					ch = b
					cnt = 0
				}
				cnt++
			}
			if cnt > 0 {
				sixelRun(sb, ch, cnt)
			}
		}
	}
}

// sixelRun writes one sixel character (or a repeat introducer) for cnt
// consecutive columns of the same bit pattern.
func sixelRun(sb *strings.Builder, ch byte, cnt int) {
	if cnt <= 0 {
		return
	}
	s := byte(63 + ch)
	for ; cnt > 255; cnt -= 255 {
		sb.WriteString("!255")
		sb.WriteByte(s)
	}
	switch cnt {
	case 1:
		sb.WriteByte(s)
	case 2:
		sb.WriteByte(s)
		sb.WriteByte(s)
	case 3:
		sb.WriteByte(s)
		sb.WriteByte(s)
		sb.WriteByte(s)
	default:
		var scratch [16]byte
		sb.WriteByte('!')
		sixelWriteInt(sb, &scratch, cnt)
		sb.WriteByte(s)
	}
}
