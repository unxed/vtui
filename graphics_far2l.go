package vtui

import (
	"strconv"

	"github.com/unxed/vtinput"
)

const (
	far2lImageCategory = 'i'
	far2lImageSet      = 's'
	far2lImageDelete   = 'd'
	far2lImageRGBA     = 0
	far2lMaxCell       = 1<<16 - 2 // -1 is reserved by FARTTY for "unchanged".
)

// far2lEncoder maps vtui placements to the native FARTTY image channel. The
// channel is asynchronous here: waiting for replies from RenderGraphics
// would re-enter the frame manager while ScreenBuf is holding its mutex.
type far2lEncoder struct {
	active map[uint32]struct{}
}

func newFar2lEncoder() *far2lEncoder {
	return &far2lEncoder{active: make(map[uint32]struct{})}
}

func (e *far2lEncoder) Reset(out *byteBuffer) {
	if e == nil {
		return
	}
	for id := range e.active {
		e.delete(out, id)
	}
	e.active = make(map[uint32]struct{})
}

func (e *far2lEncoder) Render(out *byteBuffer, list []ImagePlacement) {
	if e == nil || out == nil {
		return
	}

	next := make(map[uint32]struct{}, len(list))
	for _, p := range list {
		if !far2lPlacementValid(&p) {
			continue
		}
		next[p.ID] = struct{}{}
	}
	for id := range e.active {
		if _, ok := next[id]; !ok {
			e.delete(out, id)
		}
	}
	for i := range list {
		p := &list[i]
		if !far2lPlacementValid(p) {
			continue
		}
		e.set(out, p)
	}
	e.active = next
}

func far2lPlacementValid(p *ImagePlacement) bool {
	if p == nil || p.ID == 0 || p.Surface == nil || !p.Surface.Valid() ||
		p.Cols <= 0 || p.Rows <= 0 || p.Col < 0 || p.Row < 0 ||
		p.Col > far2lMaxCell || p.Row > far2lMaxCell ||
		p.Cols > far2lMaxCell-p.Col+1 || p.Rows > far2lMaxCell-p.Row+1 {
		return false
	}
	_, _, sw, sh := p.Source()
	maxUint32 := uint64(^uint32(0))
	return sw > 0 && sh > 0 && uint64(sw) <= maxUint32 && uint64(sh) <= maxUint32 &&
		uint64(sw)*uint64(sh) <= uint64(^uint32(0))/4
}

func far2lImageIdentity(id uint32) string {
	return "vtui-image-" + strconv.FormatUint(uint64(id), 10)
}

func (e *far2lEncoder) set(out *byteBuffer, p *ImagePlacement) {
	sx, sy, sw, sh := p.Source()
	cropped := p.Surface.Crop(sx, sy, sw, sh)
	if cropped == nil || !cropped.Valid() {
		return
	}
	right := p.Col + p.Cols - 1
	bottom := p.Row + p.Rows - 1

	stk := &vtinput.Far2lStack{}
	// Far2l's StackSerializer is LIFO, so push in the reverse order of the
	// IMAGE_SET documentation: data, dimensions, area, flags, identity,
	// subcommand, category.
	stk.PushBytes(cropped.Pix)
	stk.PushU32(uint32(cropped.Height)) //nolint:gosec // far2lPlacementValid bounds the dimensions
	stk.PushU32(uint32(cropped.Width))  //nolint:gosec // far2lPlacementValid bounds the dimensions
	stk.PushU16(uint16(bottom))         //nolint:gosec // far2lPlacementValid bounds the area
	stk.PushU16(uint16(right))          //nolint:gosec // far2lPlacementValid bounds the area
	stk.PushU16(uint16(p.Row))          //nolint:gosec // far2lPlacementValid bounds the area
	stk.PushU16(uint16(p.Col))          //nolint:gosec // far2lPlacementValid bounds the area
	stk.PushU64(far2lImageRGBA)
	stk.PushString(far2lImageIdentity(p.ID))
	stk.PushU8(far2lImageSet)
	stk.PushU8(far2lImageCategory)
	_, _ = out.Write(far2lInteractionPayload(stk))
}

func (e *far2lEncoder) delete(out *byteBuffer, id uint32) {
	stk := &vtinput.Far2lStack{}
	stk.PushString(far2lImageIdentity(id))
	stk.PushU8(far2lImageDelete)
	stk.PushU8(far2lImageCategory)
	_, _ = out.Write(far2lInteractionPayload(stk))
}
