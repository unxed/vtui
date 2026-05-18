package vtui

import (
	"fmt"
	"strings"
)

// attributesToANSI генерирует минимальную ANSI-последовательность для перехода между состояниями аттрибутов.
func attributesToANSI(attr, lastAttr uint64, activePal *[256]uint32, profile ColorProfile, quantCache map[uint32]uint8) string {
	if attr == lastAttr {
		return ""
	}

	var sb strings.Builder

	resetTriggered := false
	const flagsMask = (ForegroundIntensity | ForegroundDim | CommonLvbUnderscore | CommonLvbReverse | CommonLvbStrikeout)
	if (lastAttr&flagsMask)&^(attr&flagsMask) != 0 {
		sb.WriteString("\x1b[0m")
		lastAttr = 0
		resetTriggered = true
	}

	// 1. Style Flags
	var styles []string
	if attr&ForegroundIntensity != 0 && lastAttr&ForegroundIntensity == 0 {
		styles = append(styles, "1")
	}
	if attr&ForegroundDim != 0 && lastAttr&ForegroundDim == 0 {
		styles = append(styles, "2")
	}
	if attr&CommonLvbUnderscore != 0 && lastAttr&CommonLvbUnderscore == 0 {
		styles = append(styles, "4")
	}
	if attr&CommonLvbReverse != 0 && lastAttr&CommonLvbReverse == 0 {
		styles = append(styles, "7")
	}
	if attr&CommonLvbStrikeout != 0 && lastAttr&CommonLvbStrikeout == 0 {
		styles = append(styles, "9")
	}

	if len(styles) > 0 {
		sb.WriteString("\x1b[" + strings.Join(styles, ";") + "m")
	}

	// 2. Foreground Color
	fgMask := IsFgRGB | (0xFF << 16)
	if resetTriggered || attr&fgMask != lastAttr&fgMask || (attr&IsFgRGB != 0 && GetRGBFore(attr) != GetRGBFore(lastAttr)) {
		sb.WriteString("\x1b[" + colorToANSI(false, attr, activePal, profile, quantCache) + "m")
	}

	// 3. Background Color
	bgMask := IsBgRGB | (0xFF << 40)
	if resetTriggered || attr&bgMask != lastAttr&bgMask || (attr&IsBgRGB != 0 && GetRGBBack(attr) != GetRGBBack(lastAttr)) {
		sb.WriteString("\x1b[" + colorToANSI(true, attr, activePal, profile, quantCache) + "m")
	}

	return sb.String()
}

func colorToANSI(isBg bool, attr uint64, activePal *[256]uint32, profile ColorProfile, quantCache map[uint32]uint8) string {
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

		if profile != ColorProfileTrueColor {
			if cachedIdx, ok := quantCache[rgbVal]; ok {
				idxVal = cachedIdx
			} else {
				maxColors := 256
				if profile == ColorProfile16 {
					maxColors = 16
				}
				idxVal = findNearestColor(rgbVal, activePal, maxColors)
				quantCache[rgbVal] = idxVal
			}
			if profile == ColorProfile16 {
				return idxTo16ColorANSI(isBg, idxVal)
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

		if profile == ColorProfile16 {
			return idxTo16ColorANSI(isBg, idxVal)
		}
		return fmt.Sprintf("%d;5;%d", cmd, idxVal)
	}
}

func idxTo16ColorANSI(isBg bool, idx uint8) string {
	if idx > 15 {
		idx = idx % 16 // safe fallback
	}
	if isBg {
		if idx < 8 {
			return fmt.Sprintf("%d", 40+idx)
		}
		return fmt.Sprintf("%d", 100+(idx-8))
	} else {
		if idx < 8 {
			return fmt.Sprintf("%d", 30+idx)
		}
		return fmt.Sprintf("%d", 90+(idx-8))
	}
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
