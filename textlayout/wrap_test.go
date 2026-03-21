package textlayout

import (
	"reflect"
	"testing"
	"github.com/unxed/vtui/piecetable"
)

func TestWrapEngine_SimpleWrap(t *testing.T) {
	pt := piecetable.New([]byte("The quick brown fox jumps over the lazy dog"))
	li := piecetable.NewLineIndex()
	li.Rebuild(pt)

	we := NewWrapEngine(pt, li)
	we.SetWidth(10)

	frags := we.GetFragments(0)

	// Ожидаем, что пробелы "прилипнут" к концам строк
	expectedTexts := []string{"The quick ", "brown fox ", "jumps over ", "the lazy ", "dog"}
	if len(frags) != len(expectedTexts) {
		t.Fatalf("Expected %d fragments, got %d", len(expectedTexts), len(frags))
	}

	for i, frag := range frags {
		text := string(pt.GetRange(frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart))
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

	text := string(pt.GetRange(frags[0].ByteOffsetStart, frags[0].ByteOffsetEnd-frags[0].ByteOffsetStart))
	if text != "This is a very long line that should not be wrapped." {
		t.Errorf("Fragment text mismatch: got %q", text)
	}
}

func TestWrapEngine_UnicodeWrap(t *testing.T) {
	// 世 - 2 cells wide. Вход: "A世B世C D" (A=1, 世=2, B=1, 世=2, C=1, пробел=1, D=1)
	// Ширина 4.
	// Ожидаемый результат:
	// 1. "A世B" (ширина 4)
	// 2. "世C " (ширина 4, пробел в конце)
	// 3. "D"    (ширина 1)
	text := "A世B世C D"
	pt := piecetable.New([]byte(text))
	li := piecetable.NewLineIndex()
	li.Rebuild(pt)

	we := NewWrapEngine(pt, li)
	we.SetWidth(4)

	frags := we.GetFragments(0)

	expectedTexts := []string{"A世B", "世C ", "D"}
	if len(frags) != 3 {
		t.Fatalf("Expected 3 fragments for unicode string, got %d. Fragments: %+v", len(frags), frags)
	}

	for i, frag := range frags {
		text := string(pt.GetRange(frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart))
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
		text := string(pt.GetRange(frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart))
		if !reflect.DeepEqual(text, expectedTexts[i]) {
			t.Errorf("Frag %d: expected %q, got %q", i, expectedTexts[i], text)
		}
	}
}

func TestWrapEngine_Navigation(t *testing.T) {
	// Строка: "01234 67890" (пробел на 5-й позиции)
	pt := piecetable.New([]byte("01234 67890"))
	li := piecetable.NewLineIndex()
	li.Rebuild(pt)
	we := NewWrapEngine(pt, li)
	we.SetWidth(5)

	// Ожидаемые фрагменты при ширине 5: "01234 ", "67890"

	// 1. Тест LogicalToVisual
	// Позиция '6' (оффсет 6)
	row, col := we.LogicalToVisual(6)
	if row != 1 || col != 0 {
		t.Errorf("LogicalToVisual(6): expected (1, 0), got (%d, %d)", row, col)
	}

	// 2. Тест VisualToLogical
	// Клик на вторую строку, вторую колонку (символ '7')
	offset := we.VisualToLogical(1, 1)
	if offset != 7 {
		t.Errorf("VisualToLogical(1, 1): expected offset 7, got %d", offset)
	}
}
