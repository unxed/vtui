package vtui

import "testing"

func TestWindowsLineBreakConversion(t *testing.T) {
	tests := []struct {
		name       string
		unix, wins string
	}{
		{"empty", "", ""},
		{"no breaks", "abc", "abc"},
		{"single LF", "a\nb", "a\r\nb"},
		{"leading and trailing LF", "\na\n", "\r\na\r\n"},
		{"consecutive LF", "a\n\nb", "a\r\n\r\nb"},
		{"unicode and emoji", "каталог\n日本語\n😀\tтаб", "каталог\r\n日本語\r\n😀\tтаб"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := toWindowsLineBreaks(tc.unix); got != tc.wins {
				t.Errorf("toWindowsLineBreaks(%q) = %q, want %q", tc.unix, got, tc.wins)
			}
			if got := fromWindowsLineBreaks(tc.wins); got != tc.unix {
				t.Errorf("fromWindowsLineBreaks(%q) = %q, want %q", tc.wins, got, tc.unix)
			}
		})
	}
}

func TestToWindowsLineBreaksKeepsExistingCRLF(t *testing.T) {
	for _, in := range []string{"a\r\nb", "\r\n", "a\r\nb\r\n"} {
		if got := toWindowsLineBreaks(in); got != in {
			t.Errorf("toWindowsLineBreaks(%q) = %q, want it unchanged", in, got)
		}
	}
	if got, want := toWindowsLineBreaks("a\r\nb\nc"), "a\r\nb\r\nc"; got != want {
		t.Errorf("mixed text: got %q, want %q", got, want)
	}
}

func TestLineBreakConversionKeepsLoneCR(t *testing.T) {
	for _, in := range []string{"\r", "a\rb", "a\r", "\r\r"} {
		if got := toWindowsLineBreaks(in); got != in {
			t.Errorf("toWindowsLineBreaks(%q) = %q, want it unchanged", in, got)
		}
		if got := fromWindowsLineBreaks(in); got != in {
			t.Errorf("fromWindowsLineBreaks(%q) = %q, want it unchanged", in, got)
		}
	}
	if got, want := toWindowsLineBreaks("a\r\n"), "a\r\n"; got != want {
		t.Errorf("CR before LF: got %q, want %q", got, want)
	}
	if got, want := fromWindowsLineBreaks("a\r\r\nb"), "a\r\nb"; got != want {
		t.Errorf("CR before CRLF: got %q, want %q", got, want)
	}
}

func TestFromWindowsLineBreaksMixed(t *testing.T) {
	if got, want := fromWindowsLineBreaks("a\r\nb\nc\rd\r\n"), "a\nb\nc\rd\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
