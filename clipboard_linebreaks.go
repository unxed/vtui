package vtui

import "strings"

// The Windows clipboard keeps CRLF line breaks and vtui programs work with
// LF, so the Windows clipboard code converts at the boundary. The functions
// are plain string transforms without a build tag so that Linux CI covers
// them; Unix clipboards never call them and stay unchanged.

// toWindowsLineBreaks turns every LF into CRLF. A LF that already follows a
// CR is left alone, so CRLF text is not doubled, and a CR that stands alone
// is kept as it is.
func toWindowsLineBreaks(text string) string {
	if !strings.Contains(text, "\n") {
		return text
	}
	var b strings.Builder
	b.Grow(len(text) + strings.Count(text, "\n"))
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c == '\n' && (i == 0 || text[i-1] != '\r') {
			b.WriteByte('\r')
		}
		b.WriteByte(c)
	}
	return b.String()
}

// fromWindowsLineBreaks turns every CRLF into LF. A CR that is not followed
// by LF is kept as it is.
func fromWindowsLineBreaks(text string) string {
	return strings.ReplaceAll(text, "\r\n", "\n")
}
