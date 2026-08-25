package vtui

import (
	"fmt"
	"testing"
)

func TestClusterWidth_Combining(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"ascii", "A", 1},
		{"cjk", "世", 2},
		{"latin with acute", "e\u0301", 1},
		{"devanagari base", "क", 1},
		{"devanagari with spacing matra", "का", 1},
		{"devanagari with virama", "स\u094D", 1},
		{"emoji", "😀", 2},
		{"emoji with presentation selector", "❤\uFE0F", 2},
		{"emoji with text selector", "❤\uFE0E", 1},
		{"emoji with skin tone", "👍🏽", 2},
		{"emoji zwj family", "👨\u200D👩\u200D👦", 2},
		{"regional indicator flag", "🇩🇪", 2},
		{"keycap", "1\uFE0F\u20E3", 2},
		{"lone combining mark", "\u0301", 1},
	}
	for _, c := range cases {
		if got := ClusterWidth(c.in); got != c.want {
			t.Errorf("%s: ClusterWidth(%q) = %d, want %d", c.name, c.in, got, c.want)
		}
	}
}

// clusterList walks s through ForEachClusterAt and returns the emitted
// (text, width) pairs.
func clusterList(s string) []string {
	var out []string
	ForEachClusterAt(s, func(cluster string, width, _, _ int) {
		out = append(out, fmt.Sprintf("%s:%d", cluster, width))
	})
	return out
}

// TestClusterPaths_Equivalent is the safety net for the rune-by-rune fast
// path: for a broad corpus of names (ASCII, Cyrillic, CJK, Hangul, Japanese,
// emoji, ZWJ sequences, flags, combining marks, Indic, Thai, Arabic, symbols,
// controls) the fast path must emit exactly the same clusters, widths and
// rune offsets as the reference uniseg walk.
func TestClusterPaths_Equivalent(t *testing.T) {
	corpus := []string{
		"",
		"report_2026_final.txt",
		"отчет_2026_финал.txt",
		"报告_2026_最终版.txt",
		"한글_문서_2026_최종본.hwp",
		"ㄱㄴㄷㄹㅁㅂㅅ_자모.txt",
		"ひらがな_カタカナ_日本語.txt",
		"🚂🐐🦆🐏💐🍍💗👄  😳😎🖤",
		"👨\u200D👩\u200D👧\u200D👦 family.jpg",
		"👍🏽👍🏻👍🏿 test.md",
		"1\uFE0F\u20E3 keycap.txt",
		"🇷🇺 отчёт 🇯🇵 報告",
		"café_resumé_naïve.txt",
		"नमस्ते_दुनिया_2026.txt",
		"на\u0483звание_файла.txt",
		"か\u3099き\u309Aく.txt",
		"soft\u00ADhyphen.txt",
		"अनु\u091C्ञा_2026.txt",
		"বাংলা_ফাইল_পরীক্ষা.docx",
		"สวัสดี_โลก_2569.pdf",
		"تقريرُ_المشروعِ_2026.docx",
		"العَرَبِيَّة.txt",
		"ျမန္မာစာ.txt",
		"한국어_파일_테스트.txt",
		"日本語のテスト.txt",
		"تقرير_المشروع_2026.docx",
		"✓ ★ ♥ ⚠ ²³⁄₄",
		"ＡＢＣ 全角 テスト",
		"line1\nline2\r\ntab\tend",
		"a\uFFFD\uFFFDb",
		"x\x01y\x7Fz",
	}
	for _, s := range corpus {
		dispatched := clusterList(s)
		uniseg := clusterListRef(s)
		if len(dispatched) != len(uniseg) {
			t.Errorf("%q: dispatched %d clusters, uniseg %d\nfast: %v\nref:  %v",
				s, len(dispatched), len(uniseg), dispatched, uniseg)
			continue
		}
		for i := range dispatched {
			if dispatched[i] != uniseg[i] {
				t.Errorf("%q: cluster %d fast=%q uniseg=%q\nfast: %v\nref:  %v",
					s, i, dispatched[i], uniseg[i], dispatched, uniseg)
				break
			}
		}
		if got, want := StringWidth(s), displayWidthOf(s); got != want {
			t.Errorf("%q: StringWidth = %d, terminal display walk = %d", s, got, want)
		}
	}
}

// clusterListRef walks s with the reference uniseg walk.
func clusterListRef(s string) []string {
	var out []string
	forEachClusterUniseg(s, func(cluster string, width, _, _ int) {
		out = append(out, fmt.Sprintf("%s:%d", cluster, width))
	})
	return out
}

func displayWidthOf(s string) int {
	total := 0
	forEachDisplayCluster(s, func(_ string, w, _, _ int) {
		total += w
	})
	return total
}

func TestStringWidth_MatchesTerminalDisplayPolicy(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"hello", 5},
		{"नमस्ते", 3},
		{"বাংলা", 2},
		{"مرحبا", 5},
		{"A世B", 4},
		{"👨\u200D👩\u200D👦!", 3},
	}
	for _, c := range cases {
		if got := StringWidth(c.in); got != c.want {
			t.Errorf("StringWidth(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestRegisterCluster_RoundTrip(t *testing.T) {
	plain := RegisterCluster("A")
	if IsCompChar(plain) {
		t.Errorf("single rune cluster should stay a plain rune, got %X", plain)
	}
	if plain != uint64('A') {
		t.Errorf("expected plain rune 'A', got %X", plain)
	}

	family := "👨\u200D👩\u200D👦"
	id := RegisterCluster(family)
	if !IsCompChar(id) {
		t.Fatalf("multi rune cluster should be a composite id, got %X", id)
	}
	if id == WideCharFiller {
		t.Fatal("composite id must never collide with WideCharFiller")
	}
	if got := CellString(id); got != family {
		t.Errorf("CellString round trip: got %q, want %q", got, family)
	}
	if got := RegisterCluster(family); got != id {
		t.Errorf("registry should be stable: got %X, want %X", got, id)
	}
	if got := CellBaseRune(id); got != '👨' {
		t.Errorf("CellBaseRune: got %q, want %q", got, '👨')
	}
	if got := len(CellRunes(id)); got != 5 {
		t.Errorf("CellRunes: got %d runes, want 5", got)
	}
}

func TestCellString_SpecialCells(t *testing.T) {
	if got := CellString(WideCharFiller); got != "" {
		t.Errorf("filler should render as nothing, got %q", got)
	}
	if got := CellString(0); got != " " {
		t.Errorf("empty cell should render as a space, got %q", got)
	}
	if got := CellBaseRune(WideCharFiller); got != 0 {
		t.Errorf("filler has no base rune, got %q", got)
	}
}

func TestStringToCharInfo_KeepsMarksWithBase(t *testing.T) {
	ci := StringToCharInfo("e\u0301X", 7)
	if len(ci) != 2 {
		t.Fatalf("expected 2 cells, got %d", len(ci))
	}
	if CellString(ci[0].Char) != "e\u0301" {
		t.Errorf("cell 0: got %q, want %q", CellString(ci[0].Char), "e\u0301")
	}
	if ci[1].Char != 'X' {
		t.Errorf("cell 1: got %X, want 'X'", ci[1].Char)
	}
	for i, c := range ci {
		if c.Attributes != 7 {
			t.Errorf("cell %d lost its attributes: %d", i, c.Attributes)
		}
	}
}

func TestStringToCharInfo_Devanagari(t *testing.T) {
	// The whole point of the exercise: a Hindi word must claim exactly as many
	// terminal display cells as the rendering path gives it, or every dialog
	// around it shifts.
	ci := StringToCharInfo("नमस्ते", 0)
	if len(ci) != 3 {
		t.Fatalf("expected 3 cells for नमस्ते, got %d", len(ci))
	}
	for i, c := range ci {
		if c.Char == WideCharFiller {
			t.Errorf("cell %d: no cell of this word is wide", i)
		}
	}
}

func TestTerminalClustersKeepIndicConjunctsAtomic(t *testing.T) {
	var got []string
	var widths []int
	forEachTerminalCluster("संस्कृतम्", func(cluster string, width, _, _ int) {
		got = append(got, cluster)
		widths = append(widths, width)
	})
	want := []string{"सं", "स्कृ", "त", "म्"}
	if len(got) != len(want) {
		t.Fatalf("terminal clusters = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] || widths[i] != 1 {
			t.Errorf("cluster %d = %q width %d, want %q width 1", i, got[i], widths[i], want[i])
		}
	}
}

func TestBidiMapsUseTerminalClusterBoundaries(t *testing.T) {
	oldMode := DefaultBidiMode
	DefaultBidiMode = BidiFull
	t.Cleanup(func() { DefaultBidiMode = oldMode })

	text := "संस्कृतम् ދިވެހިބަސް"
	_, runeMap := VisualStringWithRuneMap(text)
	var clusters int
	forEachTerminalCluster(text, func(string, int, int, int) { clusters++ })
	if len(runeMap) != clusters {
		t.Fatalf("bidi rune map has %d entries for %d terminal clusters", len(runeMap), clusters)
	}
	seen := make(map[int]bool, len(runeMap))
	for _, runeIndex := range runeMap {
		if seen[runeIndex] {
			t.Fatalf("bidi rune map repeats logical cluster at rune %d: %v", runeIndex, runeMap)
		}
		seen[runeIndex] = true
	}
	if len(seen) != clusters {
		t.Fatalf("bidi rune map covers %d clusters, want %d", len(seen), clusters)
	}
}

func TestStringToCharInfo_BengaliShaping(t *testing.T) {
	ci := StringToCharInfo("বাংলা", 0)
	if len(ci) != 2 {
		t.Fatalf("expected 2 cells for বাংলা, got %d", len(ci))
	}
	for i, c := range ci {
		if c.Char == WideCharFiller {
			t.Errorf("cell %d: shaped Bengali cluster should not be wide", i)
		}
	}
}

func TestStringToCharInfo_EmojiSequenceIsOneCell(t *testing.T) {
	ci := StringToCharInfo("👨\u200D👩\u200D👦", 0)
	if len(ci) != 2 {
		t.Fatalf("expected 2 cells (one wide char), got %d", len(ci))
	}
	if !IsCompChar(ci[0].Char) {
		t.Errorf("cell 0 should hold a composite cluster, got %X", ci[0].Char)
	}
	if ci[1].Char != WideCharFiller {
		t.Errorf("cell 1 should be a filler, got %X", ci[1].Char)
	}
}

func TestStringToCharInfoHighlighted_HotkeyAfterCluster(t *testing.T) {
	// The hotkey sits after a combining sequence, so the rune index of the
	// hotkey and the cell index no longer agree.
	cells, hk := StringToCharInfoHighlighted("e\u0301&Xy", 1, 2)
	if hk != 'x' {
		t.Fatalf("hotkey: got %q, want 'x'", hk)
	}
	if len(cells) != 3 {
		t.Fatalf("expected 3 cells, got %d", len(cells))
	}
	if cells[1].Attributes != 2 {
		t.Errorf("hotkey cell should be highlighted, got attr %d", cells[1].Attributes)
	}
	if cells[0].Attributes != 1 || cells[2].Attributes != 1 {
		t.Error("only the hotkey cell should be highlighted")
	}
}

func TestFillCharInfoWithSelection_ClusterBounds(t *testing.T) {
	// "e" plus acute is 3 bytes; selecting from byte 3 must select only "X".
	data := []byte("e\u0301X")
	cells := FillCharInfoWithSelection(nil, data, 1, 2, 0, 3, 4)
	if len(cells) != 2 {
		t.Fatalf("expected 2 cells, got %d", len(cells))
	}
	if cells[0].Attributes != 1 {
		t.Errorf("cluster before selection should keep the default attr, got %d", cells[0].Attributes)
	}
	if cells[1].Attributes != 2 {
		t.Errorf("selected cluster should use the selection attr, got %d", cells[1].Attributes)
	}
}

func TestTruncateString_NeverSplitsACluster(t *testing.T) {
	if got := TruncateString("A世B", 2, ""); got != "A" {
		t.Errorf("wide char must not be half cut: got %q, want %q", got, "A")
	}
	if got := TruncateString("A世B", 3, ""); got != "A世" {
		t.Errorf("got %q, want %q", got, "A世")
	}
	if got := TruncateString("e\u0301XY", 2, ""); got != "e\u0301X" {
		t.Errorf("combining mark must travel with its base: got %q", got)
	}
	if got := TruncateString("abcdef", 4, "…"); got != "abc…" {
		t.Errorf("got %q, want %q", got, "abc…")
	}
	if got := TruncateString("abc", 10, ""); got != "abc" {
		t.Errorf("short string must be returned as is, got %q", got)
	}
}

func TestSanitizeCluster_Controls(t *testing.T) {
	if _, w := SanitizeCluster("\n"); w != 0 {
		t.Errorf("newline must not take a cell, got width %d", w)
	}
	if s, w := SanitizeCluster("\x01"); s != "·" || w != 1 {
		t.Errorf("control char: got %q width %d", s, w)
	}
	if s, _ := SanitizeCluster("\uFFFD"); s != "?" {
		t.Errorf("replacement char: got %q", s)
	}
}

func TestEmojiPresentationWideSetting(t *testing.T) {
	old := EmojiPresentationWide
	defer func() { EmojiPresentationWide = old }()

	EmojiPresentationWide = false
	if got := ClusterWidth("❤\uFE0F"); got != 1 {
		t.Errorf("with the setting off the base width wins: got %d, want 1", got)
	}
	EmojiPresentationWide = true
	if got := ClusterWidth("❤\uFE0F"); got != 2 {
		t.Errorf("with the setting on emoji presentation is wide: got %d, want 2", got)
	}
}
