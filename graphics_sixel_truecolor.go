package vtui

// Full colour sixel, the way Windows Terminal does it.
//
// A sixel image has 256 colour registers and that is usually taken to mean 256
// colours. It does not. A register is not a slot in a palette that is fixed
// once at the top of the image: it is a variable, and redefining it partway
// through changes what every later reference to it means. So an encoder can
// give **each band its own palette** — a band being six pixel rows, the unit
// sixel works in — and an image of any number of colours comes out of a format
// that never holds more than 256 at a time.
//
// This is what Windows Terminal emits and what f4's own decoder was written to
// read: it resolves a register at the moment it is used rather than at the end,
// which is the whole reason it can show a Windows Terminal image at all. A
// decoder that resolves at the end sees only the last band's palette and
// paints the picture in it. Those exist, so `VTUI_SIXEL_PALETTE=adaptive` and
// `=fixed` keep the single-palette encoders for anybody who meets one.
//
// The palette of a band is built from the pixels of that band and nothing
// else, which is why it is good: six rows of a photograph span a narrow part
// of the colour space, and 255 registers spent inside it beat 255 spent across
// the whole image by a wide margin.

import (
	"strconv"
	"strings"
	"sync"
)

// sixelBandShiftMax is where giving up starts: at eight bits of reduction
// every colour is the same colour. It is never reached by a real picture.
const sixelBandShiftMax = 8

// sixelBandBuckets is the open-addressing table used to collect a band's
// colours. Sized well above the register count so the table stays sparse and
// probing stays short; a power of two so the mask is a mask.
const sixelBandBuckets = 4096

// sixelBand collects one band's colours and hands back a palette and the
// per-pixel register indices. Everything in it is reused between bands, so a
// picture of two hundred bands allocates once.
type sixelBand struct {
	// key is the packed reduced colour of a bucket, plus one so that zero
	// means empty. sums accumulate the true colours that landed in it, so
	// the register can be their average rather than the corner of whatever
	// box the reduction drew.
	key   [sixelBandBuckets]uint32
	sumR  [sixelBandBuckets]uint32
	sumG  [sixelBandBuckets]uint32
	sumB  [sixelBandBuckets]uint32
	count [sixelBandBuckets]uint32
	reg   [sixelBandBuckets]int32

	// idx is the register of every pixel of the band, row-major, with
	// sixelIndexTransparent where there is nothing to draw.
	idx []byte

	// pal is the band's registers, in the order they were first seen.
	pal []uint32

	// touched is which buckets are dirty. It outlives the band on purpose:
	// the next band clears exactly these and nothing else. Losing it
	// between bands leaves the previous band's colours in the table, whose
	// register numbers point into a palette that has just been emptied —
	// the band then draws nothing at all.
	touched []uint32

	bits    []byte
	scratch [16]byte
}

// reset clears only the buckets that were used, which is what makes a band
// cheap: clearing four thousand entries per band would cost more than the
// pixels do.
func (b *sixelBand) reset(touched []uint32) {
	for _, h := range touched {
		b.key[h] = 0
		b.sumR[h], b.sumG[h], b.sumB[h] = 0, 0, 0
		b.count[h] = 0
		b.reg[h] = -1
	}
}

// sixelBandHash spreads a packed colour over the bucket table.
func sixelBandHash(c uint32) uint32 {
	c *= 2654435761
	return (c ^ (c >> 16)) & (sixelBandBuckets - 1)
}

// collect fills the band's palette and indices from the pixels of rows
// [y0, y1), reducing precision only as far as it must to fit the registers.
//
// The reduction is by bits, which is cheap and can be retried in one pass, and
// then undone in quality by averaging: a bucket's register is the mean of the
// pixels that fell into it, so a coarse box still yields a colour that is
// actually in the picture. Uniform quantisation with the boxes' own corners
// would band visibly; this does not.
func (b *sixelBand) collect(pix []byte, stride, dw, y0, y1, maxReg int, opaque bool) bool {
	if cap(b.idx) < dw*6 {
		b.idx = make([]byte, dw*6)
	}
	b.idx = b.idx[:dw*(y1-y0)]

	for shift := 0; shift <= sixelBandShiftMax; shift++ {
		b.reset(b.touched)
		b.touched = b.touched[:0]
		b.pal = b.pal[:0]
		overflow := false

		for y := y0; y < y1 && !overflow; y++ {
			row := pix[y*stride:]
			out := b.idx[(y-y0)*dw:]
			for x := 0; x < dw; x++ {
				p := row[x*4:]
				if !opaque && p[3] == 0 {
					out[x] = sixelIndexTransparent
					continue
				}
				r, g, bl := p[0], p[1], p[2]
				packed := uint32(r>>shift)<<16 | uint32(g>>shift)<<8 | uint32(bl>>shift)
				k := packed + 1

				h := sixelBandHash(packed)
				for {
					switch b.key[h] {
					case 0:
						if len(b.pal) >= maxReg {
							overflow = true
						} else {
							b.key[h] = k
							b.reg[h] = int32(len(b.pal))
							b.pal = append(b.pal, packed)
							b.touched = append(b.touched, h)
						}
					case k:
					default:
						h = (h + 1) & (sixelBandBuckets - 1)
						continue
					}
					break
				}
				if overflow {
					break
				}
				b.sumR[h] += uint32(r)
				b.sumG[h] += uint32(g)
				b.sumB[h] += uint32(bl)
				b.count[h]++
				out[x] = byte(b.reg[h])
			}
		}
		if !overflow {
			// The palette holds packed reduced colours; replace each
			// with the average of the pixels that produced it.
			for _, h := range b.touched {
				if n := b.count[h]; n > 0 {
					r := b.sumR[h] / n
					g := b.sumG[h] / n
					bl := b.sumB[h] / n
					b.pal[b.reg[h]] = r<<16 | g<<8 | bl
				}
			}
			return true
		}
	}
	return false
}

// emit writes one band: the register definitions it needs, then the data.
func (b *sixelBand) emit(sb *strings.Builder, dw, rows int) {
	nreg := len(b.pal)
	if nreg == 0 {
		return
	}
	if cap(b.bits) < dw*nreg {
		b.bits = make([]byte, dw*nreg)
	}
	b.bits = b.bits[:dw*nreg]
	clear(b.bits)

	for y := 0; y < rows; y++ {
		mask := byte(1 << uint(y))
		row := b.idx[y*dw : (y+1)*dw]
		for x := 0; x < dw; x++ {
			if r := row[x]; r != sixelIndexTransparent {
				b.bits[int(r)*dw+x] |= mask
			}
		}
	}

	// The definitions go here, in the band, not at the top of the image.
	// That is the whole trick, and it is also the one thing a decoder that
	// resolves registers at the end of the image cannot follow.
	for i, c := range b.pal {
		sb.WriteByte('#')
		sixelWriteInt(sb, &b.scratch, i)
		sb.WriteString(";2;")
		sb.WriteString(strconv.Itoa(sixelPercent(int(c >> 16 & 0xFF))))
		sb.WriteByte(';')
		sb.WriteString(strconv.Itoa(sixelPercent(int(c >> 8 & 0xFF))))
		sb.WriteByte(';')
		sb.WriteString(strconv.Itoa(sixelPercent(int(c & 0xFF))))
	}

	first := true
	for r := 0; r < nreg; r++ {
		base := r * dw
		// Trailing blanks are dropped: the '$' or '-' that follows puts
		// the cursor back at the edge anyway. Blanks in the middle are
		// not, because they still advance it.
		end := dw
		for end > 0 && b.bits[base+end-1] == 0 {
			end--
		}
		if end == 0 {
			continue
		}
		if !first {
			sb.WriteByte('$')
		}
		first = false
		sb.WriteByte('#')
		sixelWriteInt(sb, &b.scratch, r)

		var ch byte
		cnt := 0
		for x := 0; x < end; x++ {
			if v := b.bits[base+x]; v != ch {
				if cnt > 0 {
					sixelRun(sb, ch, cnt)
				}
				ch = v
				cnt = 0
			}
			cnt++
		}
		if cnt > 0 {
			sixelRun(sb, ch, cnt)
		}
	}
}

// sixelPercent converts a channel to the hundredths sixel carries colour in,
// rounding rather than truncating. Truncating loses up to two and a half
// levels on the way out and as much again on the way back, which is visible
// as a shift towards black on a picture that is otherwise exact — and this
// encoder exists to be exact.
func sixelPercent(v int) int { return (v*100 + 127) / 255 }

// encodeTrueColor writes the whole image with a palette per band.
func (s *sixelEncoder) encodeTrueColor(scaled *ImageSurface, dw, dh int) string {
	nbands := (dh + 5) / 6

	var sb strings.Builder
	var scratch [16]byte
	sb.Grow(dw*dh/2 + nbands*sixelMaxColors*8 + 64)
	sb.WriteString("\x1bP0;1;8q\"1;1;")
	sixelWriteInt(&sb, &scratch, dw)
	sb.WriteByte(';')
	sixelWriteInt(&sb, &scratch, dh)

	work := func(out *strings.Builder, band *sixelBand, b0, b1 int) {
		for i := b0; i < b1; i++ {
			if i > 0 {
				out.WriteByte('-')
			}
			y0 := i * 6
			y1 := y0 + 6
			if y1 > dh {
				y1 = dh
			}
			if !band.collect(scaled.Pix, scaled.Stride, dw, y0, y1, sixelMaxColors, scaled.Opaque) {
				continue
			}
			band.emit(out, dw, y1-y0)
		}
	}

	workers := scaleWorkers
	if nbands < 2*workers {
		work(&sb, &sixelBand{}, 0, nbands)
		sb.WriteString("\x1b\\")
		return sb.String()
	}

	outs := make([]strings.Builder, workers)
	bandsPer := (nbands + workers - 1) / workers
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
		outs[w].Grow(dw*dh/(2*workers) + bandsPer*sixelMaxColors*8 + 64)
		wg.Add(1)
		go func(w, b0, b1 int) {
			defer wg.Done()
			work(&outs[w], &sixelBand{}, b0, b1)
		}(w, b0, b1)
	}
	wg.Wait()
	for i := range outs {
		sb.WriteString(outs[i].String())
	}
	sb.WriteString("\x1b\\")
	return sb.String()
}
