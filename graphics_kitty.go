package vtui

import (
	"encoding/base64"
	"strconv"
)

const (
	// kittyChunkSize caps base64 payload per escape, as the protocol mandates.
	kittyChunkSize = 4096

	// kittyIDBase keeps our image identifiers away from the low numbers
	// other clients running in the same terminal tend to pick.
	kittyIDBase = 0x76740000

	// kittyCacheLimit bounds uploaded images kept alive in the terminal;
	// each costs terminal-side memory.
	kittyCacheLimit = 48
)

// kittyEncoder tracks uploaded images so panning/scrolling re-sends only
// the cheap placement commands instead of the pixels.
type kittyEncoder struct {
	uploaded map[uint64]uint32
	order    []uint64
	nextID   uint32

	// hasPlaced records that at least one placement is live, so the next
	// render knows to clear the old placements first; only the flag matters.
	hasPlaced bool
}

type kittyBuffer interface {
	Write([]byte) (int, error)
	WriteString(string) (int, error)
	WriteByte(byte) error
}

func newKittyEncoder() *kittyEncoder {
	return &kittyEncoder{
		uploaded: make(map[uint64]uint32),
		nextID:   kittyIDBase,
	}
}

// Reset drops every uploaded image, both locally and in the terminal.
func (k *kittyEncoder) Reset(sb kittyBuffer) {
	if sb != nil && (k.hasPlaced || len(k.uploaded) > 0) {
		sb.WriteString("\x1b_Ga=d,d=A,q=2\x1b\\")
	}
	k.uploaded = make(map[uint64]uint32)
	k.order = k.order[:0]
	k.hasPlaced = false
}

// Render replaces the currently visible placements with the given list.
func (k *kittyEncoder) Render(sb kittyBuffer, list []ImagePlacement) {
	k.removePlacements(sb)
	pid := uint32(0)
	for i := range list {
		p := &list[i]
		if !p.Surface.Valid() || p.Cols <= 0 || p.Rows <= 0 {
			continue
		}
		pid++
		k.emit(sb, p, pid)
	}
}

func (k *kittyEncoder) removePlacements(sb kittyBuffer) {
	if !k.hasPlaced {
		return
	}
	// WezTerm composites kitty images per cell, so a stale placement shows
	// through the next picture; one atomic clear beats per-placement deletes.
	sb.WriteString("\x1b_Ga=d,d=a,q=2\x1b\\")
	k.hasPlaced = false
}

func (k *kittyEncoder) emit(sb kittyBuffer, p *ImagePlacement, pid uint32) {
	sx, sy, sw, sh := p.Source()
	if sw <= 0 || sh <= 0 {
		return
	}
	// The whole surface is uploaded once; panning and zooming only change the
	// source rectangle of the placement, never the pixels.
	key := p.Surface.Hash()
	id, known := k.uploaded[key]
	if !known {
		id = k.nextID
		k.nextID++
		k.uploaded[key] = id
		k.order = append(k.order, key)
		k.evict(sb)
		k.upload(sb, p.Surface, id)
	}
	k.place(sb, p, sx, sy, sw, sh, id, pid)
	k.hasPlaced = true
}

func (k *kittyEncoder) evict(sb kittyBuffer) {
	for len(k.order) > kittyCacheLimit {
		oldest := k.order[0]
		k.order = k.order[1:]
		id, ok := k.uploaded[oldest]
		if !ok {
			continue
		}
		delete(k.uploaded, oldest)
		sb.WriteString("\x1b_Ga=d,d=I,q=2,i=")
		sb.WriteString(strconv.FormatUint(uint64(id), 10))
		sb.WriteString("\x1b\\")
	}
}

func (k *kittyEncoder) upload(sb kittyBuffer, surf *ImageSurface, id uint32) {
	if surf.Width <= 0 || surf.Height <= 0 {
		return
	}
	// Opaque images have a constant alpha byte, so they are sent as RGB
	// (f=24): a quarter smaller payload at identical quality; real
	// transparency keeps RGBA (f=32). rawChunk is a multiple of 3 (base64
	// triples) and of both pixel sizes, so every chunk base64s exactly and
	// never splits a pixel.
	format, bpp := "f=32", 4
	if surf.Opaque {
		format, bpp = "f=24", 3
	}
	const rawChunk = kittyChunkSize / 4 * 3
	raw := make([]byte, 0, rawChunk)
	var enc [kittyChunkSize]byte
	first := true
	remaining := surf.Width * surf.Height * bpp
	y, x := 0, 0 // pixel cursor (x is in pixels)

	writeChunk := func(final bool) {
		m := byte('1')
		if final {
			m = '0'
		}
		sb.WriteString("\x1b_G")
		if first {
			sb.WriteString("a=t,q=2,")
			sb.WriteString(format)
			sb.WriteString(",t=d,i=")
			sb.WriteString(strconv.FormatUint(uint64(id), 10))
			sb.WriteString(",s=")
			sb.WriteString(strconv.Itoa(surf.Width))
			sb.WriteString(",v=")
			sb.WriteString(strconv.Itoa(surf.Height))
			sb.WriteString(",m=")
			first = false
		} else {
			sb.WriteString("m=")
		}
		sb.WriteByte(m)
		sb.WriteByte(';')
		n := base64.StdEncoding.EncodedLen(len(raw))
		base64.StdEncoding.Encode(enc[:n], raw)
		sb.Write(enc[:n])
		sb.WriteString("\x1b\\")
		raw = raw[:0]
	}

	for remaining > 0 {
		rowBytes := (surf.Width - x) * bpp
		n := rawChunk - len(raw)
		if n > rowBytes {
			n = rowBytes
		}
		if n > remaining {
			n = remaining
		}
		o := y*surf.Stride + x*4
		if bpp == 4 {
			raw = append(raw, surf.Pix[o:o+n]...)
		} else {
			// Strip the alpha byte from each source pixel.
			start := len(raw)
			raw = raw[:start+n]
			for j := 0; j < n/3; j++ {
				s := o + j*4
				d := start + j*3
				raw[d] = surf.Pix[s]
				raw[d+1] = surf.Pix[s+1]
				raw[d+2] = surf.Pix[s+2]
			}
		}
		x += n / bpp
		remaining -= n
		if x >= surf.Width {
			x = 0
			y++
		}
		if len(raw) == rawChunk {
			writeChunk(remaining == 0)
		}
	}
	if len(raw) > 0 {
		writeChunk(true)
	}
}

func (k *kittyEncoder) place(sb kittyBuffer, p *ImagePlacement, sx, sy, sw, sh int, id, pid uint32) {
	sb.WriteString("\x1b[")
	sb.WriteString(strconv.Itoa(p.Row + 1))
	sb.WriteByte(';')
	sb.WriteString(strconv.Itoa(p.Col + 1))
	sb.WriteByte('H')

	sb.WriteString("\x1b_Ga=p,q=2,C=1,i=")
	sb.WriteString(strconv.FormatUint(uint64(id), 10))
	sb.WriteString(",p=")
	sb.WriteString(strconv.FormatUint(uint64(pid), 10))
	if sx != 0 || sy != 0 || sw != p.Surface.Width || sh != p.Surface.Height {
		sb.WriteString(",x=")
		sb.WriteString(strconv.Itoa(sx))
		sb.WriteString(",y=")
		sb.WriteString(strconv.Itoa(sy))
		sb.WriteString(",w=")
		sb.WriteString(strconv.Itoa(sw))
		sb.WriteString(",h=")
		sb.WriteString(strconv.Itoa(sh))
	}
	sb.WriteString(",c=")
	sb.WriteString(strconv.Itoa(p.Cols))
	sb.WriteString(",r=")
	sb.WriteString(strconv.Itoa(p.Rows))
	sb.WriteString(",z=")
	sb.WriteString(strconv.Itoa(p.ZIndex))
	sb.WriteString("\x1b\\")
}

// RenderGraphics implements GraphicsRenderer for the ANSI text backend.
func (r *AnsiRenderer) RenderGraphics(layer *GraphicsLayer, buf, shadow []CharInfo, w, h int, force bool) {
	if layer == nil {
		return
	}

	proto := layer.Protocol()
	if proto != r.gfxProto {
		if r.gfxFar2l != nil {
			r.gfxFar2l.Reset(&r.frameOut)
		}
		r.gfxKitty = nil
		r.gfxSixel = nil
		r.gfxFar2l = nil
		r.gfxProto = proto
		force = true
	}

	gen := layer.Generation()
	if !force && gen == r.gfxGen && !layer.DirtyUnder(buf, shadow, w, h) {
		return
	}
	r.gfxGen = gen

	switch proto {
	case GraphicsKitty:
		if r.gfxKitty == nil {
			r.gfxKitty = newKittyEncoder()
		}
		if force {
			r.gfxKitty.Reset(&r.frameOut)
		}
		r.gfxList, _ = layer.Snapshot(r.gfxList)
		r.gfxKitty.Render(&r.frameOut, r.gfxList)
	case GraphicsSixel:
		if r.gfxSixel == nil {
			r.gfxSixel = newSixelEncoder()
		}
		if force {
			r.gfxSixel.Reset(&r.frameOut)
		}
		cw, ch := layer.CellSize()
		r.gfxList, _ = layer.Snapshot(r.gfxList)
		r.gfxSixel.Render(&r.frameOut, r.gfxList, cw, ch)
	case GraphicsExternal:
		if ext := layer.External(); ext != nil {
			cw, ch := layer.CellSize()
			r.gfxList, _ = layer.Snapshot(r.gfxList)
			ext.RenderExternal(r.gfxList, cw, ch, w, h)
		}
	case GraphicsFar2l:
		if r.gfxFar2l == nil {
			r.gfxFar2l = newFar2lEncoder()
		}
		r.gfxList, _ = layer.Snapshot(r.gfxList)
		r.gfxFar2l.Render(&r.frameOut, r.gfxList)
	}

	// Both protocols move the text cursor (kitty places relative to it, sixel
	// leaves it below the image), so PrepareFlush must re-emit the cursor
	// report this frame.
	if proto == GraphicsKitty || proto == GraphicsSixel {
		r.termCursorInvalid = true
	}
}
