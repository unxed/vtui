package textlayout

import (
	"github.com/unxed/vtui/piecetable"
	"github.com/mattn/go-runewidth"
)

// LineFragment описывает один визуальный кусок логической строки после свертки.
type LineFragment struct {
	LogicalLineIdx  int // Номер оригинальной строки (до \n)
	ByteOffsetStart int // Смещение начала фрагмента (от начала всего файла/буфера)
	ByteOffsetEnd   int // Смещение конца фрагмента
	VisualWidth     int // Ширина фрагмента в колонках терминала (учитывая CJK)
}

// WrapEngine отвечает за вычисление визуальной разметки текста.
type WrapEngine struct {
	pt         *piecetable.PieceTable
	li         *piecetable.LineIndex
	wrapWidth  int
	wordWrap   bool
	fragmentCache map[int][]LineFragment
}

func NewWrapEngine(pt *piecetable.PieceTable, li *piecetable.LineIndex) *WrapEngine {
	return &WrapEngine{
		pt:         pt,
		li:         li,
		wrapWidth:  80,
		wordWrap:   true,
		fragmentCache: make(map[int][]LineFragment),
	}
}

// SetWidth устанавливает ширину для свертки. При изменении сбрасывает кэш.
func (we *WrapEngine) SetWidth(width int) {
	if width != we.wrapWidth {
		we.wrapWidth = width
		we.InvalidateCache()
	}
}

// ToggleWrap включает/выключает перенос по словам.
func (we *WrapEngine) ToggleWrap(wrap bool) {
	if wrap != we.wordWrap {
		we.wordWrap = wrap
		we.InvalidateCache()
	}
}

// InvalidateCache сбрасывает кэш фрагментов.
func (we *WrapEngine) InvalidateCache() {
	we.fragmentCache = make(map[int][]LineFragment)
}

// GetFragments возвращает визуальные фрагменты для одной логической строки.
func (we *WrapEngine) GetFragments(logLineIdx int) []LineFragment {
	if frags, ok := we.fragmentCache[logLineIdx]; ok {
		return frags
	}

	if logLineIdx < 0 || logLineIdx >= we.li.LineCount() {
		return nil
	}

	startOffset := we.li.GetLineOffset(logLineIdx)
	endOffset := we.pt.Size()
	if logLineIdx+1 < we.li.LineCount() {
		endOffset = we.li.GetLineOffset(logLineIdx + 1)
	}

	lineData := we.pt.GetRange(startOffset, endOffset-startOffset)
	// Убираем \n или \r\n с конца
	if len(lineData) > 0 && lineData[len(lineData)-1] == '\n' {
		lineData = lineData[:len(lineData)-1]
		if len(lineData) > 0 && lineData[len(lineData)-1] == '\r' {
			lineData = lineData[:len(lineData)-1]
		}
	}

	if !we.wordWrap || we.wrapWidth <= 0 {
		width := runewidth.StringWidth(string(lineData))
		frag := LineFragment{
			LogicalLineIdx:  logLineIdx,
			ByteOffsetStart: startOffset,
			ByteOffsetEnd:   startOffset + len(lineData),
			VisualWidth:     width,
		}
		we.fragmentCache[logLineIdx] = []LineFragment{frag}
		return []LineFragment{frag}
	}

	var fragments []LineFragment
	lineRunes := []rune(string(lineData))

	currentBytePos := 0
	startRuneIdx := 0

	for startRuneIdx < len(lineRunes) {
		visualWidth := 0
		endRuneIdx := startRuneIdx

		for endRuneIdx < len(lineRunes) {
			r := lineRunes[endRuneIdx]
			w := runewidth.RuneWidth(r)

			if visualWidth+w > we.wrapWidth {
				// Если символ, вызвавший переполнение — пробел, мы просто включаем его
				// в этот фрагмент (в конец строки) и завершаем фрагмент.
				if r == ' ' {
					endRuneIdx++
					visualWidth += w
					break
				}

				// Защита от бесконечного цикла (символ шире всей доступной области)
				if endRuneIdx == startRuneIdx {
					endRuneIdx++
					visualWidth += w
				} else {
					// Ищем последний пробел для «мягкого» переноса слова
					lastSpace := -1
					for i := endRuneIdx - 1; i >= startRuneIdx; i-- {
						if lineRunes[i] == ' ' {
							lastSpace = i
							break
						}
					}
					if lastSpace != -1 {
						endRuneIdx = lastSpace + 1 // Включаем пробел в конец строки
					}
				}
				break
			}
			visualWidth += w
			endRuneIdx++
		}

		fragText := lineRunes[startRuneIdx:endRuneIdx]
		fragByteLen := len(string(fragText))

		frag := LineFragment{
			LogicalLineIdx: logLineIdx,
			ByteOffsetStart: startOffset + currentBytePos,
			ByteOffsetEnd: startOffset + currentBytePos + fragByteLen,
			VisualWidth: runewidth.StringWidth(string(fragText)),
		}
		fragments = append(fragments, frag)

		startRuneIdx = endRuneIdx
		// Пропускаем пробелы в начале следующего фрагмента
		for startRuneIdx < len(lineRunes) && lineRunes[startRuneIdx] == ' ' {
			startRuneIdx++
		}
		currentBytePos = len(string(lineRunes[:startRuneIdx]))
	}

	if len(fragments) == 0 { // Для пустых строк
		fragments = append(fragments, LineFragment{LogicalLineIdx: logLineIdx, ByteOffsetStart: startOffset})
	}

	we.fragmentCache[logLineIdx] = fragments
	return fragments
}// LogicalToVisual переводит байтовый оффсет в документе в (строка, колонка) на экране.
func (we *WrapEngine) LogicalToVisual(byteOffset int) (visualRow, visualCol int) {
	logLineIdx := we.li.GetLineAtOffset(byteOffset)
	fragments := we.GetFragments(logLineIdx)

	// Считаем визуальный ряд от начала документа
	totalRow := 0
	for i := 0; i < logLineIdx; i++ {
		totalRow += len(we.GetFragments(i))
	}

	for i, frag := range fragments {
		// Используем < для конца фрагмента, чтобы оффсет на границе
		// переходил на следующую строку.
		// Исключение — самый конец последней строки файла.
		isLastFrag := (logLineIdx == we.li.LineCount()-1) && (i == len(fragments)-1)

		if byteOffset >= frag.ByteOffsetStart && (byteOffset < frag.ByteOffsetEnd || (isLastFrag && byteOffset == frag.ByteOffsetEnd)) {
			textBefore := string(we.pt.GetRange(frag.ByteOffsetStart, byteOffset-frag.ByteOffsetStart))
			return totalRow + i, runewidth.StringWidth(textBefore)
		}
	}

	// Если мы в конце фрагмента, но не в конце файла — ищем следующий фрагмент
	return totalRow, 0
}

// VisualToLogical переводит (строка, колонка) на экране в байтовый оффсет документа.
func (we *WrapEngine) VisualToLogical(visualRow, visualCol int) int {
	if visualRow < 0 { return 0 }
	currRow := 0
	for i := 0; i < we.li.LineCount(); i++ {
		fragments := we.GetFragments(i)
		if visualRow < currRow+len(fragments) {
			frag := fragments[visualRow-currRow]

			lineData := string(we.pt.GetRange(frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart))
			runes := []rune(lineData)

			offset := frag.ByteOffsetStart
			w := 0
			for _, r := range runes {
				rw := runewidth.RuneWidth(r)
				// Если текущий символ уже не влезает в visualCol — останавливаемся
				if w+rw > visualCol {
					return offset
				}
				w += rw
				offset += len(string(r))
			}
			return offset
		}
		currRow += len(fragments)
	}
	return we.pt.Size()
}
