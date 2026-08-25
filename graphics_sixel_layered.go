package vtui

// Full colour through several palettes instead of through one that changes.
//
// Sixel carries 256 colour registers. The usual way past that limit is to
// redefine a register between bands, which is what encodeTrueColor does and
// what a decoder resolving registers as it reads them honours. Windows
// Terminal does not: it keeps the raster indexed until it flushes, so a
// redefinition can recolour bands that were decoded already.
//
// The other way past the limit needs nothing of the decoder except the part
// of the specification everybody implements. Send the picture several times
// at the same spot, each time with a palette of its own and with P2=1 so
// untouched pixels stay untouched, and each image lands on top of the one
// before. Two hundred and fifty-six registers per layer, as many layers as
// the budget allows.
//
// **Every layer covers every pixel it is responsible for, and the first layer
// covers all of them.** That is the property worth keeping: the picture is
// complete after layer one and only gets better, so stopping early -- because
// the budget ran out, or because the terminal is slow -- leaves a correct
// image rather than a holed one. Layer k+1 repaints exactly the pixels layer
// k got wrong, which is also why it is cheap: a sparse layer is mostly runs
// of transparency, and a run of transparency is three bytes.
//
// Everything here is written into one frame. vtui already wraps a frame in
// synchronised output (mode 2026), so the reader never sees a half-built
// stack -- and, conversely, layers must not be spread across frames, because
// then they would be in different synchronisation blocks and the picture
// would visibly assemble itself.

import "strings"

const (
	// sixelLayerMax is how many DCS images one placement may become. Three
	// is 765 colours, which takes a photograph's banding below what the
	// eye finds on a terminal-sized picture; the gain from a fourth is
	// small and its cost is not.
	sixelLayerMax = 3

	// sixelLayerBudget bounds the whole stack. A layer that would take it
	// past this is not sent, and what has been sent already is a complete
	// picture. The number is deliberately generous next to a single-layer
	// image of the same picture: the reference point is
	// microsoft/terminal#20020, where a megabyte a frame at 1280x540 with
	// thirty-two layers was called a reasonable frame size, and a viewer
	// sends one frame rather than twenty-four a second.
	sixelLayerBudget = 3 << 20

	// sixelLayerTolerance is the squared RGB distance at which a pixel is
	// considered badly served by its palette entry and handed to the next
	// layer. Sixteen units per channel is roughly where a flat gradient
	// starts showing a band.
	sixelLayerTolerance = 3 * 16 * 16

	// sixelLayerFloor stops the stack when the pixels still wrong are too
	// few to be worth a DCS header and a palette: a layer has a fixed cost
	// of its own, and below this it buys nothing anybody can see.
	sixelLayerFloor = 64
)

// encodeLayered returns the stack for one placement, outermost first.
func (s *sixelEncoder) encodeLayered(scaled *ImageSurface, dw, dh int) []string {
	n := dw * dh
	if n <= 0 {
		return nil
	}

	// mask says which pixels this layer is responsible for. It starts as
	// every pixel that is not transparent and shrinks to the ones the
	// layer before got wrong.
	mask := make([]bool, n)
	remaining := 0
	pix, stride, opaque := scaled.Pix, scaled.Stride, scaled.Opaque
	for y := 0; y < dh; y++ {
		so := y * stride
		do := y * dw
		for x := 0; x < dw; x++ {
			if opaque || pix[so+x*4+3] != 0 {
				mask[do+x] = true
				remaining++
			}
		}
	}
	if remaining == 0 {
		return nil
	}

	layers := make([]string, 0, s.layerMax)
	total := 0
	idx := s.reuseIdx(n)
	for len(layers) < s.layerMax && remaining > 0 {
		palI, palDef, lut := adaptiveSixelPalette(packMasked(scaled, mask, remaining))
		remaining = sixelQuantizeLayer(scaled, idx, lut, palI, mask, sixelLayerTolerance)

		data := s.emitIndexed(idx, palDef, dw, dh)
		if data == "" {
			break
		}
		if len(layers) > 0 && total+len(data) > s.layerBudget {
			// What is already on the screen is a whole picture, so
			// stopping here costs accuracy and nothing else.
			break
		}
		layers = append(layers, data)
		total += len(data)

		if remaining < sixelLayerFloor {
			break
		}
	}
	return layers
}

// packMasked gathers the marked pixels into a compact opaque surface.
//
// A median cut cares about the distribution of colours and not at all about
// where they were, which is what makes this legal: the pixels a layer is
// responsible for are laid out in reading order into a rectangle of no
// particular meaning, and the quantiser sees their colours and nothing else.
// The tail is padded by repeating the last colour rather than with a
// transparent black that would earn itself a register.
func packMasked(src *ImageSurface, mask []bool, count int) *ImageSurface {
	if count <= 0 {
		return src
	}
	w := packedRowWidth(count)
	h := (count + w - 1) / w
	out := &ImageSurface{
		Width:  w,
		Height: h,
		Stride: w * 4,
		Pix:    make([]byte, w*h*4),
		Opaque: true,
	}

	d := 0
	var last [4]byte
	last[3] = 0xFF
	for y := 0; y < src.Height; y++ {
		so := y * src.Stride
		mo := y * src.Width
		for x := 0; x < src.Width; x++ {
			if !mask[mo+x] {
				continue
			}
			o := so + x*4
			last[0], last[1], last[2] = src.Pix[o], src.Pix[o+1], src.Pix[o+2]
			copy(out.Pix[d:d+4], last[:])
			d += 4
		}
	}
	for d < len(out.Pix) {
		copy(out.Pix[d:d+4], last[:])
		d += 4
	}
	return out
}

// packedRowWidth keeps the packed rectangle roughly square-ish without
// costing a square root: the only thing that matters is that it is a valid
// image, and a very long single row is one.
func packedRowWidth(count int) int {
	const maxRow = 1024
	if count < maxRow {
		return count
	}
	return maxRow
}

// sixelQuantizeLayer maps the marked pixels to their nearest palette entry and
// returns how many of them the palette served badly. Those keep their mark and
// become the next layer's responsibility; the rest are cleared.
//
// No dithering, unlike sixelQuantizePal. Dithering trades a colour error for a
// spatial one, which is the right trade when there is one palette and no way
// to do better -- but here there is a way to do better, and a diffused error
// is one that cannot be measured per pixel, so there would be nothing left to
// decide the next layer with.
func sixelQuantizeLayer(surf *ImageSurface, idx []byte, lut []uint8, palI [][3]int32, mask []bool, tol int32) int {
	w, h := surf.Width, surf.Height
	pix, stride := surf.Pix, surf.Stride

	remaining := 0
	for y := 0; y < h; y++ {
		so := y * stride
		do := y * w
		for x := 0; x < w; x++ {
			if !mask[do+x] {
				idx[do+x] = sixelIndexTransparent
				continue
			}
			o := so + x*4
			r, g, b := pix[o], pix[o+1], pix[o+2]
			pi := int(lut[lutIndex(r, g, b)])
			idx[do+x] = byte(pi)

			er := int32(r) - palI[pi][0]
			eg := int32(g) - palI[pi][1]
			eb := int32(b) - palI[pi][2]
			if er*er+eg*eg+eb*eb > tol {
				remaining++
				continue
			}
			mask[do+x] = false
		}
	}
	return remaining
}

// sixelLayerBytes is the size of a stack, for tests and for anybody measuring
// what this costs on the wire.
func sixelLayerBytes(layers []string) int {
	n := 0
	for _, l := range layers {
		n += len(l)
	}
	return n
}

// sixelLayerCount counts the DCS introducers in a stream, which is how a test
// says "this went out as three images" without parsing sixel.
func sixelLayerCount(s string) int {
	return strings.Count(s, "\x1bP")
}
