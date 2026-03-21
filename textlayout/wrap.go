package textlayout

import (
	"sort"
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
	fragmentCache [][]LineFragment

	// rowOffsets[i] хранит общее количество визуальных строк во всех
	// логических строках ПЕРЕД строкой i.
	rowOffsets []int
	totalRows  int
	validUntil int // Index of the last logical line with a valid calculated row offset

	tmpBuf []byte // Reusable buffer for avoiding allocations
}

func NewWrapEngine(pt *piecetable.PieceTable, li *piecetable.LineIndex) *WrapEngine {
	return &WrapEngine{
		pt:            pt,
		li:            li,
		wrapWidth:     80,
		wordWrap:      true,
		fragmentCache: nil,
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
	we.fragmentCache = nil
	we.validUntil = -1
	we.rowOffsets = nil
	we.totalRows = 0
}

func (we *WrapEngine) InvalidateFrom(logLineIdx int) {
	if logLineIdx < 0 {
		logLineIdx = 0
	}
	if we.fragmentCache != nil && logLineIdx < len(we.fragmentCache) {
		for i := logLineIdx; i < len(we.fragmentCache); i++ {
			we.fragmentCache[i] = nil
		}
	}
	if logLineIdx <= we.validUntil {
		we.validUntil = logLineIdx - 1
	}
}

// GetFragments возвращает визуальные фрагменты для одной логической строки.
func (we *WrapEngine) GetFragments(logLineIdx int) []LineFragment {
	lineCount := we.li.LineCount()
	if we.fragmentCache == nil || len(we.fragmentCache) != lineCount {
		we.fragmentCache = make([][]LineFragment, lineCount)
	}

	if logLineIdx < 0 || logLineIdx >= lineCount {
		return nil
	}

	if we.fragmentCache[logLineIdx] != nil {
		return we.fragmentCache[logLineIdx]
	}

	startOffset := we.li.GetLineOffset(logLineIdx)
	endOffset := we.pt.Size()
	if logLineIdx+1 < we.li.LineCount() {
		endOffset = we.li.GetLineOffset(logLineIdx + 1)
	}

	we.tmpBuf = we.tmpBuf[:0]
	we.tmpBuf = we.pt.AppendRange(we.tmpBuf, startOffset, endOffset-startOffset)
	lineData := we.tmpBuf
	// Убираем \n или \r\n с конца
	if len(lineData) > 0 && lineData[len(lineData)-1] == '\n' {
		lineData = lineData[:len(lineData)-1]
		if len(lineData) > 0 && lineData[len(lineData)-1] == '\r' {
			lineData = lineData[:len(lineData)-1]
		}
	}

	if !we.wordWrap || we.wrapWidth <= 0 {
		width := 0
		tmpData := lineData
		for len(tmpData) > 0 {
			r, size := utf8.DecodeRune(tmpData)
			rw := 1
			if r >= 0x7F {
				rw = runewidth.RuneWidth(r)
			}
			if rw < 0 { rw = 1 }
			width += rw
			tmpData = tmpData[size:]
		}

		frag := LineFragment{
			LogicalLineIdx:  logLineIdx,
			ByteOffsetStart: startOffset,
			ByteOffsetEnd:   startOffset + len(lineData),
			VisualWidth:     width,
		}
		we.fragmentCache[logLineIdx] = []LineFragment{frag}
		return we.fragmentCache[logLineIdx]
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
			w := 1
			if r >= 0x7F {
				w = runewidth.RuneWidth(r)
			}
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

func (we *WrapEngine) ensureRowCountCache(until int) {
	lineCount := we.li.LineCount()
	if until >= lineCount {
		until = lineCount - 1
	}
	if we.validUntil >= until && we.rowOffsets != nil && len(we.rowOffsets) == lineCount {
		return
	}

	if we.rowOffsets == nil || len(we.rowOffsets) != lineCount {
		oldOffsets := we.rowOffsets
		we.rowOffsets = make([]int, lineCount)
		if oldOffsets != nil {
			numToCopy := len(oldOffsets)
			if numToCopy > lineCount {
				numToCopy = lineCount
			}
			copy(we.rowOffsets, oldOffsets[:numToCopy])
			if we.validUntil >= numToCopy {
				we.validUntil = numToCopy - 1
			}
		} else {
			we.validUntil = -1
		}
	}

	if !we.wordWrap {
		for i := we.validUntil + 1; i < lineCount; i++ {
			we.rowOffsets[i] = i
		}
		we.totalRows = lineCount
		we.validUntil = lineCount - 1
		return
	}

	currentOffset := 0
	start := we.validUntil + 1
	if start > 0 {
		currentOffset = we.rowOffsets[start-1] + len(we.GetFragments(start-1))
	}

	for i := start; i <= until; i++ {
		we.rowOffsets[i] = currentOffset
		currentOffset += len(we.GetFragments(i))
	}
	if until > we.validUntil {
		we.validUntil = until
	}
	if we.validUntil == lineCount-1 {
		we.totalRows = currentOffset
	}
}

// GetTotalVisualRows возвращает общее количество визуальных строк в документе.
func (we *WrapEngine) GetTotalVisualRows() int {
	we.ensureRowCountCache(we.li.LineCount() - 1)
	return we.totalRows
}
// GetRowOffset возвращает индекс первой визуальной строки для данной логической строки.
func (we *WrapEngine) GetRowOffset(logLineIdx int) int {
	we.ensureRowCountCache(logLineIdx)
	if logLineIdx < 0 {
		return 0
	}
	if logLineIdx >= len(we.rowOffsets) {
		we.ensureRowCountCache(we.li.LineCount() - 1)
		return we.totalRows
	}
	return we.rowOffsets[logLineIdx]
}

// GetLogLineAtVisualRow переводит абсолютный индекс визуальной строки в индекс
// логической строки и порядковый номер фрагмента внутри неё.
func (we *WrapEngine) GetLogLineAtVisualRow(visualRow int) (logLineIdx int, fragIdx int) {
	if visualRow < 0 {
		return 0, 0
	}

	// Lazy calculation until we find the row or hit EOF
	lineCount := we.li.LineCount()
	for we.validUntil < lineCount-1 {
		var lastCalculatedRow int
		if we.validUntil >= 0 {
			lastCalculatedRow = we.rowOffsets[we.validUntil] + len(we.GetFragments(we.validUntil))
		}
		if lastCalculatedRow > visualRow {
			break
		}
		// Expand cache in chunks
		nextTarget := we.validUntil + 100
		if nextTarget >= lineCount {
			nextTarget = lineCount - 1
		}
		we.ensureRowCountCache(nextTarget)
	}

	if visualRow >= we.totalRows && we.validUntil == lineCount-1 {
		return lineCount - 1, 0
	}

	// Binary search on the valid portion of the cache
	logLineIdx = sort.Search(we.validUntil+1, func(i int) bool {
		return we.rowOffsets[i] > visualRow
	}) - 1

	if logLineIdx < 0 {
		logLineIdx = 0
	}
	fragIdx = visualRow - we.rowOffsets[logLineIdx]
	return
}

// LogicalToVisual переводит байтовый оффсет в документе в (строка, колонка) на экране.
func (we *WrapEngine) LogicalToVisual(byteOffset int) (visualRow, visualCol int) {
	logLineIdx := we.li.GetLineAtOffset(byteOffset)
	we.ensureRowCountCache(logLineIdx)
	fragments := we.GetFragments(logLineIdx)
	totalRow := we.rowOffsets[logLineIdx]

	for i, frag := range fragments {
		isLastFragOfLine := (i == len(fragments)-1)
		if byteOffset >= frag.ByteOffsetStart && (byteOffset < frag.ByteOffsetEnd || (isLastFragOfLine && byteOffset == frag.ByteOffsetEnd)) {
			// Вычисляем колонку без аллокаций
			width := 0
			if byteOffset > frag.ByteOffsetStart {
				we.tmpBuf = we.tmpBuf[:0]
				we.tmpBuf = we.pt.AppendRange(we.tmpBuf, frag.ByteOffsetStart, byteOffset-frag.ByteOffsetStart)
				data := we.tmpBuf
				for len(data) > 0 {
					r, size := utf8.DecodeRune(data)
					rw := 1
					if r >= 0x7F {
						rw = runewidth.RuneWidth(r)
					}
					if rw <= 0 { rw = 1 }
					width += rw
					data = data[size:]
				}
			}
			return totalRow + i, width
		}
	}
	return totalRow, 0
}

// VisualToLogical переводит (строка, колонка) на экране в байтовый оффсет документа.
func (we *WrapEngine) VisualToLogical(visualRow, visualCol int) int {
	logLineIdx, fragIdx := we.GetLogLineAtVisualRow(visualRow)
	fragments := we.GetFragments(logLineIdx)
	if fragments == nil {
		return 0
	}
	if fragIdx >= len(fragments) {
		fragIdx = len(fragments) - 1
	}
	frag := fragments[fragIdx]

	if frag.ByteOffsetStart >= frag.ByteOffsetEnd || visualCol <= 0 {
		return frag.ByteOffsetStart
	}

	we.tmpBuf = we.tmpBuf[:0]
	we.tmpBuf = we.pt.AppendRange(we.tmpBuf, frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart)
	lineData := we.tmpBuf
	offset := frag.ByteOffsetStart
	currentCol := 0

	for len(lineData) > 0 {
		r, size := utf8.DecodeRune(lineData)
		rw := 1
		if r >= 0x7F {
			rw = runewidth.RuneWidth(r)
		}
		if rw <= 0 {
			rw = 1
		}
		if currentCol+rw > visualCol {
			return offset
		}
		currentCol += rw
		offset += size
		lineData = lineData[size:]
	}
	return offset
}
