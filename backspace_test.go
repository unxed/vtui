package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

// The reference sequences are the Notepad transcript attached to unxed/f4#546:
// Delete removes whole shaped units, Backspace peels code points.

func backspaceWalk(t *testing.T, text string, want []string) {
	t.Helper()
	e := newBoundaryEdit(text)
	e.curPos = len(e.text)
	for i, expected := range want {
		e.ProcessKey(&vtinput.InputEvent{
			Type:           vtinput.KeyEventType,
			KeyDown:        true,
			VirtualKeyCode: vtinput.VK_BACK,
		})
		if got := e.GetText(); got != expected {
			t.Fatalf("Backspace #%d on %q = %q, want %q", i+1, text, got, expected)
		}
	}
}

func TestEditBackspacePeelsDevanagariCodePoints(t *testing.T) {
	saved := DefaultBidiMode
	DefaultBidiMode = BidiDisplay
	defer func() { DefaultBidiMode = saved }()

	backspaceWalk(t, sanskritSample, []string{
		"\u0938\u0902\u0938\u094D\u0915\u0943\u0924\u092E", // संस्कृतम
		"\u0938\u0902\u0938\u094D\u0915\u0943\u0924",       // संस्कृत
		"\u0938\u0902\u0938\u094D\u0915\u0943",             // संस्कृ
		"\u0938\u0902\u0938\u094D\u0915",                   // संस्क
		"\u0938\u0902\u0938\u094D",                         // संस्
		"\u0938\u0902\u0938",                               // संस
		"\u0938\u0902",                                     // सं
		"\u0938",                                           // स
		"",
	})
}

func TestEditBackspacePeelsThaanaCodePoints(t *testing.T) {
	saved := DefaultBidiMode
	DefaultBidiMode = BidiDisplay
	defer func() { DefaultBidiMode = saved }()

	want := make([]string, 0, len(thaanaSample))
	runes := []rune(thaanaSample)
	for i := len(runes) - 1; i >= 0; i-- {
		want = append(want, string(runes[:i]))
	}
	backspaceWalk(t, thaanaSample, want)
}

func TestBackspaceKeepsEmojiSequencesAtomic(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{"zwj family", "\U0001F468\u200D\U0001F469\u200D\U0001F467"},
		{"skin tone", "\U0001F44D\U0001F3FD"},
		{"flag", "\U0001F1EF\U0001F1F5"},
		{"keycap", "1\uFE0F\u20E3"},
	}
	for _, tc := range cases {
		text := []rune(tc.text)
		if got := backspaceStart(text, 0, len(text)); got != 0 {
			t.Errorf("%s: backspaceStart = %d, want 0 (atomic)", tc.name, got)
		}
	}
}

// A decomposed Latin letter is the textbook case in the W3C write-up: the
// accents come off one at a time, the base letter last.
func TestBackspacePeelsDecomposedLatin(t *testing.T) {
	saved := DefaultBidiMode
	DefaultBidiMode = BidiDisplay
	defer func() { DefaultBidiMode = saved }()

	backspaceWalk(t, "A\u030A\u0301", []string{"A\u030A", "A", ""})
}

func TestMultiLineEditBackspacePeelsCodePoints(t *testing.T) {
	saved := DefaultBidiMode
	DefaultBidiMode = BidiDisplay
	defer func() { DefaultBidiMode = saved }()

	m := NewMultiLineEdit(0, 0, 40, 3, sanskritSample)
	m.curRow = 0
	m.curCol = len(m.lines[0])
	m.backspace()
	if got := m.GetText(); got != "\u0938\u0902\u0938\u094D\u0915\u0943\u0924\u092E" {
		t.Fatalf("MultiLineEdit backspace = %q", got)
	}
}
