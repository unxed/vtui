package vtui

import (
	"strings"
)

// attributesToANSI returns the minimal ANSI sequence transitioning between attribute states.
func attributesToANSI(attr, lastAttr uint64, activePal *[256]uint32, profile ColorProfile, quantCache map[uint32]uint8) string {
	var b strings.Builder
	writeAttributesToANSI(&b, attr, lastAttr, activePal, profile, quantCache)
	return b.String()
}

func appendUint8(dst []byte, v uint8) []byte {
	if v < 10 {
		return append(dst, byte('0'+v))
	}
	if v < 100 {
		return append(dst, byte('0'+v/10), byte('0'+v%10))
	}
	return append(dst, byte('0'+v/100), byte('0'+(v/10)%10), byte('0'+v%10))
}

// writeAttributesToANSI merges style, fg and bg into a single CSI (~4 bytes
// shorter per component, no allocations) with identical terminal semantics.
func writeAttributesToANSI(b *strings.Builder, attr, lastAttr uint64, activePal *[256]uint32, profile ColorProfile, quantCache map[uint32]uint8) {
	var buf [64]byte
	n := formatAttributesCSI(buf[:], attr, lastAttr, activePal, profile, quantCache)
	if n > 0 {
		b.Write(buf[:n])
	}
}

// writeAttributesToBuffer writes directly to the zero-allocation byteBuffer.
func writeAttributesToBuffer(b *byteBuffer, attr, lastAttr uint64, activePal *[256]uint32, profile ColorProfile, quantCache map[uint32]uint8) {
	var buf [64]byte
	n := formatAttributesCSI(buf[:], attr, lastAttr, activePal, profile, quantCache)
	if n > 0 {
		b.Write(buf[:n])
	}
}

func formatAttributesCSI(buf []byte, attr, lastAttr uint64, activePal *[256]uint32, profile ColorProfile, quantCache map[uint32]uint8) int {
	if attr == lastAttr {
		return 0
	}

	buf[0] = '\x1b'
	buf[1] = '['
	n := 2
	first := true
	writeSep := func() {
		if !first {
			buf[n] = ';'
			n++
		}
		first = false
	}

	resetTriggered := false
	const flagsMask = (ForegroundIntensity | ForegroundDim | CommonLvbUnderscore | CommonLvbReverse | CommonLvbStrikeout)
	if (lastAttr&flagsMask)&^(attr&flagsMask) != 0 {
		buf[n] = '0'
		n++
		first = false
		lastAttr = 0
		resetTriggered = true
	}

	// SGR 1 is not emitted on syscons: scteken_te_to_sc_attr() XORs bit 3 of
	// the foreground for TF_BOLD, so bold over an already bright colour comes
	// out dark. The 90..97 codes carry brightness there on their own.
	if attr&ForegroundIntensity != 0 && lastAttr&ForegroundIntensity == 0 && !IsFreeBSDSyscons {
		writeSep()
		buf[n] = '1'
		n++
	}
	if attr&ForegroundDim != 0 && lastAttr&ForegroundDim == 0 {
		writeSep()
		buf[n] = '2'
		n++
	}
	if attr&CommonLvbUnderscore != 0 && lastAttr&CommonLvbUnderscore == 0 {
		writeSep()
		buf[n] = '4'
		n++
	}
	// SGR 7 is not emitted on syscons: teken swaps fg and bg before syscons
	// maps them, so reverse over a bright foreground lands in bit 7 of the
	// attribute byte, which the text renderer blinks. The swap is done in
	// writeColorANSI instead, which costs nothing anywhere else.
	if attr&CommonLvbReverse != 0 && lastAttr&CommonLvbReverse == 0 && !IsFreeBSDSyscons {
		writeSep()
		buf[n] = '7'
		n++
	}
	if attr&CommonLvbStrikeout != 0 && lastAttr&CommonLvbStrikeout == 0 {
		writeSep()
		buf[n] = '9'
		n++
	}

	revFlipped := IsFreeBSDSyscons && (attr^lastAttr)&CommonLvbReverse != 0

	fgMask := IsFgRGB | (0xFF << 16)
	if resetTriggered || revFlipped || attr&fgMask != lastAttr&fgMask || (attr&IsFgRGB != 0 && GetRGBFore(attr) != GetRGBFore(lastAttr)) {
		writeSep()
		n += writeColorANSI(buf[n:], false, attr, activePal, profile, quantCache)
	}

	bgMask := IsBgRGB | (0xFF << 40)
	if resetTriggered || revFlipped || attr&bgMask != lastAttr&bgMask || (attr&IsBgRGB != 0 && GetRGBBack(attr) != GetRGBBack(lastAttr)) {
		writeSep()
		n += writeColorANSI(buf[n:], true, attr, activePal, profile, quantCache)
	}

	if n > 2 {
		buf[n] = 'm'
		n++
		return n
	}
	return 0
}

// writeColorANSI appends one colour component — "P;2;R;G;B" (true colour) or
// "P;5;N" (palette), P being 38 for fg and 48 for bg — without a CSI prefix
// or trailing 'm'; returns the byte count.
func writeColorANSI(dst []byte, isBg bool, attr uint64, activePal *[256]uint32, profile ColorProfile, quantCache map[uint32]uint8) int {
	// On syscons SGR 7 is suppressed above, so the swap happens here.
	src := isBg
	if IsFreeBSDSyscons && attr&CommonLvbReverse != 0 {
		src = !src
	}

	var rgbVal uint32
	var idxVal uint8
	if src {
		rgbVal = GetRGBBack(attr)
		idxVal = GetIndexBack(attr)
	} else {
		rgbVal = GetRGBFore(attr)
		idxVal = GetIndexFore(attr)
	}

	flag := IsFgRGB
	if src {
		flag = IsBgRGB
	}
	if attr&flag != 0 {
		if profile != ColorProfileTrueColor {
			if quantCache == nil {
				idxVal = findNearestColor(rgbVal, activePal, 256)
			} else if cachedIdx, ok := quantCache[rgbVal]; ok {
				idxVal = cachedIdx
			} else {
				maxColors := 256
				if profile == ColorProfile16 {
					maxColors = 16
				}
				idxVal = findNearestColor(rgbVal, activePal, maxColors)
				quantCache[rgbVal] = idxVal
			}
			idxVal = clampSysconsBg(isBg, idxVal)
			if profile == ColorProfile16 {
				return copy(dst, idxTo16ColorANSI(isBg, idxVal))
			}
			return appendColor256(dst, isBg, idxVal)
		}
		r, g, b := rgb(rgbVal)
		return appendColorRGB(dst, isBg, r, g, b)
	}

	idxVal = clampSysconsBg(isBg, idxVal)
	if profile == ColorProfile16 {
		return copy(dst, idxTo16ColorANSI(isBg, idxVal))
	}
	return appendColor256(dst, isBg, idxVal)
}

// clampSysconsBg drops a background out of the bright half of the first 16
// palette entries. Anything syscons maps to a bg index of 8..15 sets bit 7 of
// the VGA attribute byte, and its text renderer blinks that bit.
func clampSysconsBg(isBg bool, idx uint8) uint8 {
	if IsFreeBSDSyscons && isBg && idx < 16 {
		return idx & 7
	}
	return idx
}

// appendColor256 appends "P;5;N" to dst and returns the byte count.
func appendColor256(dst []byte, isBg bool, idx uint8) int {
	p := dst[:0]
	if isBg {
		p = append(p, "48;5;"...)
	} else {
		p = append(p, "38;5;"...)
	}
	p = appendUint8(p, idx)
	return len(p)
}

// appendColorRGB appends "P;2;R;G;B" to dst and returns the byte count.
func appendColorRGB(dst []byte, isBg bool, r, g, b uint8) int {
	p := dst[:0]
	if isBg {
		p = append(p, "48;2;"...)
	} else {
		p = append(p, "38;2;"...)
	}
	p = appendUint8(p, r)
	p = append(p, ';')
	p = appendUint8(p, g)
	p = append(p, ';')
	p = appendUint8(p, b)
	return len(p)
}

// colorToANSI returns one colour component (no CSI, no 'm') as a string,
// keeping the string-based API the tests use.
func colorToANSI(isBg bool, attr uint64, activePal *[256]uint32, profile ColorProfile, quantCache map[uint32]uint8) string {
	var buf [24]byte
	n := writeColorANSI(buf[:], isBg, attr, activePal, profile, quantCache)
	return string(buf[:n])
}

var idx16FG = [...]string{
	"30", "31", "32", "33", "34", "35", "36", "37",
	"90", "91", "92", "93", "94", "95", "96", "97",
}
var idx16BG = [...]string{
	"40", "41", "42", "43", "44", "45", "46", "47",
	"100", "101", "102", "103", "104", "105", "106", "107",
}

func idxTo16ColorANSI(isBg bool, idx uint8) string {
	idx = idx & 15
	if isBg {
		if IsFreeBSDSyscons {
			// 100..107 set bit 7 of the VGA attribute byte, which syscons
			// blinks in text mode. Drop to the dark half of the palette.
			idx &= 7
		}
		return idx16BG[idx]
	}
	return idx16FG[idx]
}

func findNearestColor(rgbVal uint32, pal *[256]uint32, maxColors int) uint8 {
	if pal == nil {
		pal = &XTerm256Palette
	}
	r, g, b := rgb(rgbVal)
	var bestIdx uint8 = 0
	var bestDist int = 1000000

	for i := 0; i < maxColors; i++ {
		pr, pg, pb := rgb(pal[i])
		dr := int(r) - int(pr)
		dg := int(g) - int(pg)
		db := int(b) - int(pb)
		dist := dr*dr + dg*dg + db*db
		if dist < bestDist {
			bestDist = dist
			bestIdx = uint8(i)
			if dist == 0 {
				break
			}
		}
	}
	return bestIdx
}
