//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris && !plan9 && !android && (amd64 || arm64)

package vtui

import "testing"

// TestSymChar_GogpuBatchRuneExcludesTokens confirms a symbolic glyph token
// never joins gogpu's batched-string fast path (gogpu_renderer.go), which
// appends the raw uint64 Char value cast to a rune: a SymCharFlag token's
// bit pattern is far outside Unicode, so gogpuBatchRune must decline it
// exactly as it already declines a CompCharFlag registry index. This lives
// in its own file with gogpuBatchRune's build constraint, since that
// function (and its gogpu_stub.go replacement platforms) is not available
// everywhere symchar_test.go's other tests run.
func TestSymChar_GogpuBatchRuneExcludesTokens(t *testing.T) {
	for _, sym := range allSymGlyphs {
		for part := 0; part < 3; part++ {
			tok := SymCharToken(sym, part)
			if gogpuBatchRune(tok) {
				t.Errorf("gogpuBatchRune(SymCharToken(%v, %d)) = true, want false (raw token must not join a batched run)", sym, part)
			}
		}
	}
}
