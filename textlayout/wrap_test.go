package textlayout

import (
	"bytes"
	"reflect"
	"testing"
	"time"
	"github.com/unxed/vtui/piecetable"
)

func TestWrapEngine_SimpleWrap(t *testing.T) {
	pt := piecetable.New([]byte("The quick brown fox jumps over the lazy dog"))
	li := piecetable.NewLineIndex()
	li.Rebuild(pt)

	we := NewWrapEngine(pt, li)
	we.SetWidth(10)

	frags := we.GetFragments(0)

	// Пояснение: "The quick " (10), "brown fox " (10), "jumps over " (11, пробел в конце), "the lazy " (9), "dog" (3)
	expectedTexts := []string{"The quick ", "brown fox ", "jumps over ", "the lazy ", "dog"}
	if len(frags) != len(expectedTexts) {
		t.Fatalf("Expected %d fragments, got %d. Frags: %+v", len(expectedTexts), len(frags), frags)
	}

	for i, frag := range frags {
		data, _ := pt.GetRange(frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart)
		text := string(data)
		if text != expectedTexts[i] {
			t.Errorf("Frag %d: expected %q, got %q", i, expectedTexts[i], text)
		}
	}
}

func TestWrapEngine_NoWrap(t *testing.T) {
	pt := piecetable.New([]byte("This is a very long line that should not be wrapped."))
	li := piecetable.NewLineIndex()
	li.Rebuild(pt)

	we := NewWrapEngine(pt, li)
	we.SetWidth(20)
	we.ToggleWrap(false)

	frags := we.GetFragments(0)
	if len(frags) != 1 {
		t.Fatalf("Expected 1 fragment when word wrap is off, got %d", len(frags))
	}

	data, _ := pt.GetRange(frags[0].ByteOffsetStart, frags[0].ByteOffsetEnd-frags[0].ByteOffsetStart)
	text := string(data)
	if text != "This is a very long line that should not be wrapped." {
		t.Errorf("Fragment text mismatch: got %q", text)
	}
}

func TestWrapEngine_UnicodeWrap(t *testing.T) {
	text := "A世B世C D"
	pt := piecetable.New([]byte(text))
	li := piecetable.NewLineIndex()
	li.Rebuild(pt)

	we := NewWrapEngine(pt, li)
	we.SetWidth(4)

	frags := we.GetFragments(0)

	// 1. "A世B" (4)
	// 2. "世C " (4) - пробел в конце
	// 3. "D"    (1)
	expectedTexts := []string{"A世B", "世C ", "D"}
	if len(frags) != 3 {
		t.Fatalf("Expected 3 fragments for unicode string, got %d. Fragments: %+v", len(frags), frags)
	}

	for i, frag := range frags {
		data, _ := pt.GetRange(frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart)
		text := string(data)
		if text != expectedTexts[i] {
			t.Errorf("Frag %d: expected %q, got %q", i, expectedTexts[i], text)
		}
	}
}

func TestWrapEngine_LongWord(t *testing.T) {
	pt := piecetable.New([]byte("supercalifragilisticexpialidocious"))
	li := piecetable.NewLineIndex()
	li.Rebuild(pt)
	we := NewWrapEngine(pt, li)
	we.SetWidth(10)

	frags := we.GetFragments(0)
	expectedTexts := []string{"supercalif", "ragilistic", "expialidoc", "ious"}

	if len(frags) != 4 {
		t.Fatalf("Expected 4 fragments for long word, got %d", len(frags))
	}

	for i, frag := range frags {
		data, _ := pt.GetRange(frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart)
		text := string(data)
		if !reflect.DeepEqual(text, expectedTexts[i]) {
			t.Errorf("Frag %d: expected %q, got %q", i, expectedTexts[i], text)
		}
	}
}

func TestWrapEngine_Navigation(t *testing.T) {
	// Строка: "01234 67890", ширина 5.
	// Фрагменты: "01234 ", "67890"
	pt := piecetable.New([]byte("01234 67890"))
	li := piecetable.NewLineIndex()
	li.Rebuild(pt)
	we := NewWrapEngine(pt, li)
	we.SetWidth(5)

	// 1. Тест LogicalToVisual
	// Оффсет 6 (символ '6').
	row, col := we.LogicalToVisual(6)
	if row != 1 || col != 0 {
		t.Errorf("LogicalToVisual(6): expected (1, 0), got (%d, %d)", row, col)
	}

	// 2. Тест VisualToLogical
	// Вторая строка ("67890"), колонка 1 (символ '7')
	offset := we.VisualToLogical(1, 1)
	if offset != 7 {
		t.Errorf("VisualToLogical(1, 1): expected offset 7, got %d", offset)
	}
}

func TestWrapEngine_Performance10MB(t *testing.T) {
	// Создаем 10 МБ текста
	chunk := "The quick brown fox jumps over the lazy dog. " // 45 bytes
	count := (10 * 1024 * 1024) / len(chunk)
	data := make([]byte, 0, count*len(chunk))
	for i := 0; i < count; i++ {
		data = append(data, chunk...)
	}

	pt := piecetable.New(data)
	li := piecetable.NewLineIndex()
	li.Rebuild(pt)
	we := NewWrapEngine(pt, li)
	we.SetWidth(80)

	// Тест 1: С пробелами (Word Wrap)
	start := time.Now()
	frags := we.GetFragments(0)
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("Performance (with spaces) too slow: %v", elapsed)
	}
	t.Logf("10MB with spaces parsed into %d fragments in %v", len(frags), elapsed)

	// Тест 2: Без пробелов (Hard Wrap)
	we.InvalidateCache()
	we.pt = piecetable.New(bytes.Repeat([]byte("A"), 10*1024*1024))
	we.li.Rebuild(we.pt)

	start = time.Now()
	frags = we.GetFragments(0)
	elapsed = time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("Performance (hard wrap) too slow: %v", elapsed)
	}
	t.Logf("10MB without spaces parsed into %d fragments in %v", len(frags), elapsed)
}
func TestWrapEngine_ExtremeCorners(t *testing.T) {
	// 1. Окно шириной 1, символ шириной 2
	pt := piecetable.New([]byte("世"))
	li := piecetable.NewLineIndex()
	li.Rebuild(pt)
	we := NewWrapEngine(pt, li)
	we.SetWidth(1) // Меньше ширины символа

	frags := we.GetFragments(0)
	if len(frags) != 1 || frags[0].VisualWidth != 2 {
		t.Errorf("Narrow window CJK: expected width 2, got %v", frags[0].VisualWidth)
	}

	// 2. Сверхдлинное слово без пробелов
	pt2 := piecetable.New([]byte("1234567890"))
	li2 := piecetable.NewLineIndex()
	li2.Rebuild(pt2)
	we.pt = pt2
	we.li = li2
	we.SetWidth(3)

	frags2 := we.GetFragments(0)
	// Ожидаем: "123", "456", "789", "0"
	if len(frags2) != 4 {
		t.Errorf("Long word break: expected 4 frags, got %d", len(frags2))
	}

	// 3. Сохранение отступов (ведущих пробелов)
	pt3 := piecetable.New([]byte("    Line with indentation"))
	li3 := piecetable.NewLineIndex()
	li3.Rebuild(pt3)
	we.pt = pt3
	we.li = li3
	we.SetWidth(10)

	frags3 := we.GetFragments(0)
	// Ожидаем "    Line " (пробел после Line влезает в 10 символов)
	data3, _ := pt3.GetRange(frags3[0].ByteOffsetStart, frags3[0].ByteOffsetEnd-frags3[0].ByteOffsetStart)
	text := string(data3)
	if text != "    Line " {
		t.Errorf("Indentation preserved: expected '    Line ', got %q", text)
	}
}

func TestWrapEngine_MultipleSpaces(t *testing.T) {
	pt := piecetable.New([]byte("word1    word2"))
	li := piecetable.NewLineIndex()
	li.Rebuild(pt)
	we := NewWrapEngine(pt, li)
	we.SetWidth(10)

	frags := we.GetFragments(0)
	// 1. "word1     " (9 символов: "word1" + 4 пробела) - это влезает в 10.
	// 2. "word2"
	if len(frags) != 2 {
		t.Fatalf("Expected 2 fragments for multiple spaces, got %d", len(frags))
	}
	d1, _ := pt.GetRange(frags[0].ByteOffsetStart, frags[0].ByteOffsetEnd-frags[0].ByteOffsetStart)
	d2, _ := pt.GetRange(frags[1].ByteOffsetStart, frags[1].ByteOffsetEnd-frags[1].ByteOffsetStart)
	text1 := string(d1)
	text2 := string(d2)
	if text1 != "word1    " || text2 != "word2" {
		t.Errorf("Multiple spaces failed. Got %q and %q", text1, text2)
	}
}
func TestWrapEngine_EndOfLineCursor(t *testing.T) {
	pt := piecetable.New([]byte("abc"))
	li := piecetable.NewLineIndex()
	li.Rebuild(pt)
	we := NewWrapEngine(pt, li)
	we.SetWidth(10)

	// The cursor is often placed at offset == length(text) to type at the end.
	// We want to ensure LogicalToVisual correctly maps this to the end of the first row.
	row, col := we.LogicalToVisual(3)

	if row != 0 || col != 3 {
		t.Errorf("Cursor at EOF on first line: expected (0, 3), got (%d, %d)", row, col)
	}
}
