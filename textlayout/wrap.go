package textlayout

import (
    "unicode/utf8"

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
	if width < 1 { width = 1 } // Ширина не может быть меньше 1
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
	bytePos := 0
	dataLen := len(lineData)

	for bytePos < dataLen {
		visualWidth := 0
		fragStartByte := bytePos
		lastSpaceEnd := -1
		lastSpaceWidth := 0

		scanPos := bytePos
		for scanPos < dataLen {
			r, size := utf8.DecodeRune(lineData[scanPos:])
			w := runewidth.RuneWidth(r)
			if w < 0 { w = 1 }

			if visualWidth+w > we.wrapWidth {
				if r == ' ' {
					// Пробел не влезает, но мы его забираем в конец этой строки
					scanPos += size
					visualWidth += w
				} else if lastSpaceEnd != -1 {
					// Word Wrap: откатываемся к последнему пробелу
					scanPos = lastSpaceEnd
					visualWidth = lastSpaceWidth
				} else if scanPos == fragStartByte {
					// Даже один символ не влез (CJK в узком окне) - поглощаем его
					scanPos += size
					visualWidth = w
				}
				break
			}

			visualWidth += w
			scanPos += size
			if r == ' ' {
				lastSpaceEnd = scanPos
				lastSpaceWidth = visualWidth
			}
		}

		fragments = append(fragments, LineFragment{
			LogicalLineIdx:  logLineIdx,
			ByteOffsetStart: startOffset + fragStartByte,
			ByteOffsetEnd:   startOffset + scanPos,
			VisualWidth:     visualWidth,
		})
		bytePos = scanPos
	}

	if len(fragments) == 0 {
		fragments = append(fragments, LineFragment{LogicalLineIdx: logLineIdx, ByteOffsetStart: startOffset, ByteOffsetEnd: startOffset})
	}

	we.fragmentCache[logLineIdx] = fragments
	return fragments
}

// LogicalToVisual переводит байтовый оффсет в документе в (строка, колонка) на экране.
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
	if visualRow < 0 {
		return 0
	}
	currRow := 0
	for i := 0; i < we.li.LineCount(); i++ {
		fragments := we.GetFragments(i)
		if visualRow < currRow+len(fragments) {
			frag := fragments[visualRow-currRow]

			// Если фрагмент пустой (например, пустая строка), возвращаем его начало
			if frag.ByteOffsetStart == frag.ByteOffsetEnd || visualCol <= 0 {
				return frag.ByteOffsetStart
			}

			lineData := string(we.pt.GetRange(frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart))
			runes := []rune(lineData)

			offset := frag.ByteOffsetStart
			w := 0
			for _, r := range runes {
				rw := runewidth.RuneWidth(r)
				if rw <= 0 {
					rw = 1
				}
				// Если мы достигли или перешли нужную колонку — возвращаем текущий байт
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
