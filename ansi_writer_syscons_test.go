package vtui

import (
	"strings"
	"testing"
)

// Issue #91. On FreeBSD syscons in a VGA text mode bit 7 of the attribute byte
// is the hardware blink bit, so nothing we emit may end up setting it.
// scteken_te_to_sc_attr() sets it from a background index of 8..15, and from
// TF_REVERSE whenever the foreground is bright, because teken swaps fg and bg
// before syscons maps them. SGR 1 is a separate problem: syscons XORs bit 3 of
// the foreground for TF_BOLD, so bold over a bright colour comes out dark.
func TestSysconsNeverEmitsBlinkingAttributes(t *testing.T) {
	old := IsFreeBSDSyscons
	defer func() { IsFreeBSDSyscons = old }()

	var pal [256]uint32
	copy(pal[:], ThemePalette[:])
	cases := []struct {
		name   string
		attr   uint64
		banned []string
		wanted string
	}{
		{
			name:   "bright background",
			attr:   SetIndexBack(SetIndexFore(0, 7), 12),
			banned: []string{"104"},
			wanted: "44",
		},
		{
			name:   "bright grey background of the key bar",
			attr:   SetIndexBack(SetIndexFore(0, 15), 8),
			banned: []string{"100"},
			wanted: "40",
		},
		{
			name: "reverse over a bright foreground",
			attr: SetIndexBack(SetIndexFore(0, 14), 1) | CommonLvbReverse,
			// The swap moves blue to the foreground and the bright cyan to
			// the background, where it is clamped to the dark half.
			banned: []string{";7;", "[7;", "100", "101", "102", "103", "104", "105", "106", "107"},
			wanted: "31;46",
		},
		{
			name:   "bold with a bright foreground",
			attr:   SetIndexBack(SetIndexFore(0, 11), 0) | ForegroundIntensity,
			banned: []string{"[1;", ";1;"},
			wanted: "93",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			IsFreeBSDSyscons = true
			got := attributesToANSI(tc.attr, ^uint64(0), &pal, ColorProfile16, nil)
			for _, b := range tc.banned {
				if strings.Contains(got, b) {
					t.Errorf("emitted %q, which contains the banned %q", got, b)
				}
			}
			if !strings.Contains(got, tc.wanted) {
				t.Errorf("emitted %q, want it to contain %q", got, tc.wanted)
			}

			// Off syscons nothing changes.
			IsFreeBSDSyscons = false
			plain := attributesToANSI(tc.attr, ^uint64(0), &pal, ColorProfile16, nil)
			if plain == got {
				t.Logf("note: %q is identical with and without the policy", got)
			}
		})
	}
}
