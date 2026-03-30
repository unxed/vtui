package vtui

import (
	"fmt"
	"strings"
)

// attributesToANSI генерирует минимальную ANSI-последовательность для перехода между состояниями аттрибутов.
func attributesToANSI(attr, lastAttr uint64, activePal *[256]uint32, force256 bool, quantCache map[uint32]uint8) string {
	if attr == lastAttr {
		return ""
	}

	var params []string

	resetTriggered := false
	const flagsMask = (ForegroundIntensity | ForegroundDim | CommonLvbUnderscore | CommonLvbReverse | CommonLvbStrikeout)
	if (lastAttr&flagsMask)&^(attr&flagsMask) != 0 {
		params = append(params, "0")
		lastAttr = 0
		resetTriggered = true
	}

	// 1. Style Flags
	if attr&ForegroundIntensity != 0 && lastAttr&ForegroundIntensity == 0 {
		params = append(params, "1")
	}
	if attr&ForegroundDim != 0 && lastAttr&ForegroundDim == 0 {
		params = append(params, "2")
	}
	if attr&CommonLvbUnderscore != 0 && lastAttr&CommonLvbUnderscore == 0 {
		params = append(params, "4")
	}
	if attr&CommonLvbReverse != 0 && lastAttr&CommonLvbReverse == 0 {
		params = append(params, "7")
	}
	if attr&CommonLvbStrikeout != 0 && lastAttr&CommonLvbStrikeout == 0 {
		params = append(params, "9")
	}

	// 2. Foreground Color
	fgMask := IsFgRGB | (0xFF << 16)
	if resetTriggered || attr&fgMask != lastAttr&fgMask || (attr&IsFgRGB != 0 && GetRGBFore(attr) != GetRGBFore(lastAttr)) {
		params = append(params, colorToANSI(false, attr, activePal, force256, quantCache))
	}

	// 3. Background Color
	bgMask := IsBgRGB | (0xFF << 40)
	if resetTriggered || attr&bgMask != lastAttr&bgMask || (attr&IsBgRGB != 0 && GetRGBBack(attr) != GetRGBBack(lastAttr)) {
		params = append(params, colorToANSI(true, attr, activePal, force256, quantCache))
	}

	if len(params) == 0 {
		return ""
	}

	return "\x1b[" + strings.Join(params, ";") + "m"
}

func colorToANSI(isBg bool, attr uint64, activePal *[256]uint32, force256 bool, quantCache map[uint32]uint8) string {
	isRGBFlag := IsFgRGB
	cmd := 38
	var rgbVal uint32
	var idxVal uint8

	if isBg {
		isRGBFlag = IsBgRGB
		cmd = 48
	}

	isRGB := (attr & isRGBFlag) != 0

	if isRGB {
		if isBg {
			rgbVal = GetRGBBack(attr)
		} else {
			rgbVal = GetRGBFore(attr)
		}

		if force256 {
			if cachedIdx, ok := quantCache[rgbVal]; ok {
				idxVal = cachedIdx
			} else {
				idxVal = findNearestColor(rgbVal, activePal)
				quantCache[rgbVal] = idxVal
			}
			return fmt.Sprintf("%d;5;%d", cmd, idxVal)
		}

		r, g, b := rgb(rgbVal)
		return fmt.Sprintf("%d;2;%d;%d;%d", cmd, r, g, b)
	} else {
		if isBg {
			idxVal = GetIndexBack(attr)
		} else {
			idxVal = GetIndexFore(attr)
		}
		return fmt.Sprintf("%d;5;%d", cmd, idxVal)
	}
}

func findNearestColor(rgbVal uint32, pal *[256]uint32) uint8 {
	if pal == nil {
		pal = &XTerm256Palette
	}
	r, g, b := rgb(rgbVal)
	var bestIdx uint8 = 0
	var bestDist int = 1000000

	for i := 0; i < 256; i++ {
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
