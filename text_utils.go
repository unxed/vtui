package vtui

import "strings"

// WrapText splits a string into an array of strings not exceeding maxWidth.
// Respects \n line breaks and tries to split by spaces.
func WrapText(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{text}
	}

	var result []string
	paragraphs := strings.Split(text, "\n")

	for _, p := range paragraphs {
		words := strings.Fields(p)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}

		var currentLine strings.Builder
		currentLineWidth := 0

		for _, word := range words {
			wordWidth := StringWidth(word)

			// If a word is inherently longer than maxWidth, split it forcefully
			if wordWidth > maxWidth {
				if currentLineWidth > 0 {
					result = append(result, currentLine.String())
					currentLine.Reset()
					currentLineWidth = 0
				}

				result = append(result, splitWordByWidth(word, maxWidth)...)
				continue
			}

			// Check if the word fits in the current line
			spaceWidth := 0
			if currentLineWidth > 0 {
				spaceWidth = 1
			}

			if currentLineWidth+spaceWidth+wordWidth > maxWidth {
				result = append(result, currentLine.String())
				currentLine.Reset()
				currentLine.WriteString(word)
				currentLineWidth = wordWidth
			} else {
				if spaceWidth > 0 {
					currentLine.WriteByte(' ')
				}
				currentLine.WriteString(word)
				currentLineWidth += spaceWidth + wordWidth
			}
		}
		if currentLineWidth > 0 {
			result = append(result, currentLine.String())
		}
	}

	return result
} // TruncateMiddle shortens a string by removing characters from the middle
// and replacing them with "...".
func TruncateMiddle(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen || maxLen < 5 {
		return s
	}

	// Calculate how many characters to keep on each side
	// Subtract 3 for the "..." ellipsis
	half := (maxLen - 3) / 2
	start := string(runes[:half])
	end := string(runes[len(runes)-(maxLen-3-half):])

	return start + "..." + end
}

func splitWordByWidth(word string, maxWidth int) []string {
	var result []string
	var current strings.Builder
	currentWidth := 0
	forEachDisplayCluster(word, func(cluster string, width, _, _ int) {
		if currentWidth > 0 && currentWidth+width > maxWidth {
			result = append(result, current.String())
			current.Reset()
			currentWidth = 0
		}
		current.WriteString(cluster)
		currentWidth += width
	})
	if current.Len() > 0 {
		result = append(result, current.String())
	}
	return result
}
