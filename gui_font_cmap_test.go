//go:build linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || solaris || illumos

package vtui

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// sfntFile lays out an sfnt file holding the given tables, in the order given.
func sfntFile(tables ...[2]string) []byte {
	return sfntFileAt(0, tables...)
}

// sfntFileAt is sfntFile for a font that starts at base within a larger file
// (a collection): table offsets in an sfnt count from the start of the file.
func sfntFileAt(base int, tables ...[2]string) []byte {
	var out bytes.Buffer
	be := func(v any) { _ = binary.Write(&out, binary.BigEndian, v) }
	be(uint32(0x00010000))
	be(uint16(len(tables))) // #nosec G115 -- test fixture: a few tables
	be([3]uint16{})         // searchRange, entrySelector, rangeShift: unused here
	off := base + 12 + 16*len(tables)
	for _, t := range tables {
		out.WriteString(t[0])
		be(uint32(0))         // checksum
		be(uint32(off))       // #nosec G115 -- test fixture: a few hundred bytes
		be(uint32(len(t[1]))) // #nosec G115 -- test fixture: a few hundred bytes
		off += len(t[1])
	}
	for _, t := range tables {
		out.WriteString(t[1])
	}
	return out.Bytes()
}

type cmapRecord struct {
	platform, encoding uint16
	subtable           []byte
}

func cmapTable(records ...cmapRecord) string {
	var out bytes.Buffer
	be := func(v any) { _ = binary.Write(&out, binary.BigEndian, v) }
	be(uint16(0))
	be(uint16(len(records))) // #nosec G115 -- test fixture: a few records
	off := 4 + 8*len(records)
	for _, r := range records {
		be(r.platform)
		be(r.encoding)
		be(uint32(off)) // #nosec G115 -- test fixture: a few hundred bytes
		off += len(r.subtable)
	}
	for _, r := range records {
		out.Write(r.subtable)
	}
	return out.String()
}

type format4Segment struct {
	start, end uint16
	delta      int16
	glyphs     []uint16 // non-nil: mapped through glyphIdArray instead of delta
}

func cmapFormat4(segs ...format4Segment) []byte {
	segs = append(segs, format4Segment{start: 0xFFFF, end: 0xFFFF, delta: 1})
	n := len(segs)
	var glyphArray []uint16
	rangeOffsets := make([]uint16, n)
	for i, s := range segs {
		if s.glyphs == nil {
			continue
		}
		// From idRangeOffset[i] to glyphIdArray[len(glyphArray)].
		rangeOffsets[i] = uint16(2*(n-i) + 2*len(glyphArray)) // #nosec G115 -- test fixture: a few segments
		glyphArray = append(glyphArray, s.glyphs...)
	}
	var out bytes.Buffer
	be := func(v any) { _ = binary.Write(&out, binary.BigEndian, v) }
	be([]uint16{4, 0, 0, uint16(2 * n), 0, 0, 0}) // #nosec G115 -- test fixture: a few segments
	for _, s := range segs {
		be(s.end)
	}
	be(uint16(0))
	for _, s := range segs {
		be(s.start)
	}
	for _, s := range segs {
		be(s.delta)
	}
	be(rangeOffsets)
	be(glyphArray)
	return out.Bytes()
}

func cmapGroups(format uint16, groups ...[3]uint32) []byte {
	var out bytes.Buffer
	be := func(v any) { _ = binary.Write(&out, binary.BigEndian, v) }
	be(format)
	be(uint16(0))
	be(uint32(16 + 12*len(groups))) // #nosec G115 -- test fixture: a few groups
	be(uint32(0))
	be(uint32(len(groups))) // #nosec G115 -- test fixture: a few groups
	for _, g := range groups {
		be(g)
	}
	return out.Bytes()
}

func coverageOfBytes(t *testing.T, data []byte) *runeCoverage {
	t.Helper()
	cov, err := fontCoverage(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("fontCoverage: %v", err)
	}
	return cov
}

func coveredRunes(cov *runeCoverage) []rune {
	var out []rune
	for r := rune(0); r < runeCoverageMax; r++ {
		if cov.has(r) {
			out = append(out, r)
		}
	}
	return out
}

func TestCmapCoverage_Format4(t *testing.T) {
	sub := cmapFormat4(
		format4Segment{start: 'A', end: 'C', delta: -'A' + 1},
		// The middle code maps to glyph 0 through the array: not covered.
		format4Segment{start: 'x', end: 'z', glyphs: []uint16{7, 0, 9}},
	)
	data := sfntFile([2]string{"head", "...."}, [2]string{"cmap", cmapTable(cmapRecord{3, 1, sub})})
	got := string(coveredRunes(coverageOfBytes(t, data)))
	if got != "ABCxz" {
		t.Errorf("covered = %q, want %q", got, "ABCxz")
	}
}

func TestCmapCoverage_UnionOfUnicodeSubtables(t *testing.T) {
	bmp := cmapFormat4(format4Segment{start: '0', end: '1', delta: 5})
	full := cmapGroups(12, [3]uint32{0x1F600, 0x1F601, 40})
	// Not counted: Mac Roman and variation sequences map no Unicode, and gg
	// reads no format 13 (LastResort claims all of Unicode through one).
	macRoman := cmapFormat4(format4Segment{start: 'M', end: 'M', delta: 1})
	sequences := cmapGroups(12, [3]uint32{'S', 'S', 1})
	manyToOne := cmapGroups(13, [3]uint32{0x2190, 0x2191, 3})
	data := sfntFile([2]string{"cmap", cmapTable(
		cmapRecord{3, 1, bmp},
		cmapRecord{3, 10, full},
		cmapRecord{1, 0, macRoman},
		cmapRecord{0, 5, sequences},
		cmapRecord{0, 6, manyToOne},
	)})
	got := coveredRunes(coverageOfBytes(t, data))
	want := []rune{'0', '1', 0x1F600, 0x1F601}
	if string(got) != string(want) {
		t.Errorf("covered = %U, want %U", got, want)
	}
}

func TestCmapCoverage_CollectionReadsTheFirstFace(t *testing.T) {
	first := cmapTable(cmapRecord{3, 1, cmapFormat4(format4Segment{start: 'F', end: 'F', delta: 1})})
	second := cmapTable(cmapRecord{3, 1, cmapFormat4(format4Segment{start: 'S', end: 'S', delta: 1})})
	const header = 12 + 2*4
	face0 := sfntFileAt(header, [2]string{"cmap", first})
	face1 := sfntFileAt(header+len(face0), [2]string{"cmap", second})

	var ttc bytes.Buffer
	ttc.WriteString("ttcf")
	_ = binary.Write(&ttc, binary.BigEndian, []uint32{0x00010000, 2, header, uint32(header + len(face0))}) // #nosec G115 -- test fixture: a few hundred bytes
	ttc.Write(face0)
	ttc.Write(face1)

	if got := string(coveredRunes(coverageOfBytes(t, ttc.Bytes()))); got != "F" {
		t.Errorf("covered = %q, want the first face's %q", got, "F")
	}
}

// withCmapRecord returns a one-table font whose cmap record is rewritten:
// the table directory claims off and length, whatever the file holds.
func withCmapRecord(off, length uint32) []byte {
	data := sfntFile([2]string{"cmap", cmapTable(cmapRecord{3, 1, cmapFormat4(format4Segment{start: 'A', end: 'A', delta: 1})})})
	binary.BigEndian.PutUint32(data[12+8:], off)
	binary.BigEndian.PutUint32(data[12+12:], length)
	return data
}

// fontWithSubtable is a font whose only cmap subtable is sub, raw.
func fontWithSubtable(sub []byte) []byte {
	return sfntFile([2]string{"cmap", cmapTable(cmapRecord{3, 1, sub})})
}

func ttcHeader(fields ...uint32) []byte {
	var out bytes.Buffer
	out.WriteString("ttcf")
	_ = binary.Write(&out, binary.BigEndian, fields)
	return out.Bytes()
}

type failingReaderAt struct{ err error }

func (r failingReaderAt) ReadAt([]byte, int64) (int, error) { return 0, r.err }

// The scan reads whatever files the lists and discovery name, so damage must
// end in an error, which sends the file to the parse, and never in a panic.
func TestCmapCoverage_RejectsWhatIsNotAFont(t *testing.T) {
	noCmap := sfntFile([2]string{"head", "...."})
	format4 := cmapFormat4(format4Segment{start: 'A', end: 'B', delta: 1})
	for name, data := range map[string][]byte{
		"text":                       []byte("not a font, just text"),
		"empty":                      nil,
		"no cmap":                    noCmap,
		"unknown format":             fontWithSubtable([]byte{0, 2, 0, 0}),
		"truncated":                  noCmap[:20],
		"collection without fonts":   ttcHeader(0x00010000, 0),
		"collection cut after count": ttcHeader(0x00010000, 1),
		"collection face past EOF":   ttcHeader(0x00010000, 1, 4096),
		"cmap over the size limit":   withCmapRecord(28, maxCmapTableSize+1),
		"cmap past EOF":              withCmapRecord(4096, 64),
		"cmap too short":             sfntFile([2]string{"cmap", "\x00"}),
		"encoding records cut":       sfntFile([2]string{"cmap", "\x00\x00\x00\x05"}),
		"subtable of one byte":       sfntFile([2]string{"cmap", "\x00\x00\x00\x01\x00\x03\x00\x01\x00\x00\x00\x0c\x00"}),
		"format 4 header cut":        fontWithSubtable(format4[:10]),
		"format 4 arrays cut":        fontWithSubtable(format4[:16]),
		"format 6 header cut":        fontWithSubtable([]byte{0, 6, 0, 0}),
		"format 6 entries cut":       fontWithSubtable([]byte{0, 6, 0, 0, 0, 0, 0, 'a', 0, 9, 0, 1}),
		"format 12 header cut":       fontWithSubtable(cmapGroups(12)[:12]),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := fontCoverage(bytes.NewReader(data)); err == nil {
				t.Error("want an error")
			}
		})
	}

	ioErr := errors.New("device gone")
	if _, err := fontCoverage(failingReaderAt{ioErr}); !errors.Is(err, ioErr) {
		t.Errorf("a read error = %v, want it passed through", err)
	}
}

func TestCmapCoverage_Format6(t *testing.T) {
	// firstCode 'a', glyphs 3, 0, 5: 'b' maps to .notdef.
	sub := []byte{0, 6, 0, 16, 0, 0, 0, 'a', 0, 3, 0, 3, 0, 0, 0, 5}
	if got := string(coveredRunes(coverageOfBytes(t, fontWithSubtable(sub)))); got != "ac" {
		t.Errorf("covered = %q, want %q", got, "ac")
	}
}

// Damage inside a subtable the scan can still bound costs the runes it
// touches, not the font.
func TestCmapCoverage_ToleratesDamageInsideASubtable(t *testing.T) {
	t.Run("idRangeOffset past the table", func(t *testing.T) {
		sub := cmapFormat4(
			format4Segment{start: 'A', end: 'A', delta: 1},
			format4Segment{start: 'x', end: 'x', glyphs: []uint16{7}},
		)
		// segCount 3: idRangeOffset[1] sits at 14 + 3*2*3 + 2 + 2.
		binary.BigEndian.PutUint16(sub[14+3*2*3+2+2:], 0xFFF0)
		if got := string(coveredRunes(coverageOfBytes(t, fontWithSubtable(sub)))); got != "A" {
			t.Errorf("covered = %q, want only %q", got, "A")
		}
	})
	t.Run("more format 12 groups claimed than present", func(t *testing.T) {
		sub := cmapGroups(12, [3]uint32{0x1F600, 0x1F601, 1})
		binary.BigEndian.PutUint32(sub[12:], 1000)
		got := coveredRunes(coverageOfBytes(t, fontWithSubtable(sub)))
		if want := []rune{0x1F600, 0x1F601}; string(got) != string(want) {
			t.Errorf("covered = %U, want %U", got, want)
		}
	})
}

// A color bitmap font maps its emoji and draws none of them through gg or
// x/image; the scan refuses it so that the chain never opens it. An outline
// font with a few bitmap glyphs stays a font.
func TestCmapCoverage_RefusesColorBitmapFonts(t *testing.T) {
	cmap := cmapTable(cmapRecord{3, 10, cmapGroups(12, [3]uint32{0x1F600, 0x1F64F, 1})})
	big, small := strings.Repeat("b", 4096), strings.Repeat("o", 64)
	for name, tc := range map[string]struct {
		data    []byte
		refused bool
	}{
		"sbix over a token glyf (Apple)":  {sfntFile([2]string{"cmap", cmap}, [2]string{"glyf", small}, [2]string{"sbix", big}), true},
		"CBDT without outlines (Noto)":    {sfntFile([2]string{"cmap", cmap}, [2]string{"CBDT", big}), true},
		"outlines with a few bitmaps":     {sfntFile([2]string{"cmap", cmap}, [2]string{"glyf", big}, [2]string{"sbix", small}), false},
		"CFF outlines with a few bitmaps": {sfntFile([2]string{"cmap", cmap}, [2]string{"CFF ", big}, [2]string{"CBDT", small}), false},
	} {
		t.Run(name, func(t *testing.T) {
			cov, err := fontCoverage(bytes.NewReader(tc.data))
			if tc.refused {
				if !errors.Is(err, errColorBitmapFont) {
					t.Fatalf("err = %v, want errColorBitmapFont", err)
				}
				return
			}
			if err != nil || !cov.has(0x1F600) {
				t.Fatalf("an outline font must keep its coverage: err = %v", err)
			}
		})
	}
}

// The scan must see exactly what the parser sees, on real fonts: the chain
// passes a font over on the scan's word.
func TestCmapCoverage_MatchesTheParserOnGoFonts(t *testing.T) {
	for name, data := range map[string][]byte{"goregular": goregular.TTF, "gomono": gomono.TTF} {
		t.Run(name, func(t *testing.T) {
			cov := coverageOfBytes(t, data)
			f, err := opentype.Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			var buf sfnt.Buffer
			n := 0
			for r := rune(0); r < runeCoverageMax; r++ {
				if r >= 0xD800 && r <= 0xDFFF {
					continue
				}
				gi, err := f.GlyphIndex(&buf, r)
				parsed := err == nil && gi != 0
				if parsed != cov.has(r) {
					t.Fatalf("U+%04X: parser says %v, scan says %v", r, parsed, cov.has(r))
				}
				if parsed {
					n++
				}
			}
			if n < 500 {
				t.Fatalf("only %d runes covered; the comparison proves nothing", n)
			}
		})
	}
}

func TestCmapCoverage_ReadsAFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Go-Regular.ttf")
	if err := os.WriteFile(path, goregular.TTF, 0o600); err != nil {
		t.Fatal(err)
	}
	cov, err := cmapCoverage(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cov.has('A') || cov.has(0x4E00) {
		t.Errorf("Go Regular: has('A') = %v, has(U+4E00) = %v", cov.has('A'), cov.has(0x4E00))
	}
	if _, err := cmapCoverage(filepath.Join(t.TempDir(), "absent.ttf")); err == nil {
		t.Error("a missing file must be an error")
	}
}

// scannedChain is newFakeChain with a scan that answers from the same table
// and counts its calls; paths listed in unreadable fail the scan.
func scannedChain(t *testing.T, fonts map[string]string, unreadable ...string) (*fontFallbackChain, *int, *atomic.Int32) {
	t.Helper()
	chain, opens := newFakeChain(t, fonts)
	previous := discoverFallbackPaths
	discoverFallbackPaths = func(rune) []string { return nil }
	t.Cleanup(func() { discoverFallbackPaths = previous })
	scans := &atomic.Int32{}
	chain.scan = func(path string) (*runeCoverage, error) {
		scans.Add(1)
		for _, p := range unreadable {
			if p == path {
				return nil, errors.New("unreadable cmap")
			}
		}
		cov := &runeCoverage{}
		for _, r := range fonts[path] {
			cov.set(r)
		}
		return cov, nil
	}
	return chain, opens, scans
}

func TestChain_PassesOverAFontWithoutOpeningIt(t *testing.T) {
	chain, opens, _ := scannedChain(t, map[string]string{
		"/fonts/CJK.ttc":     "葉↑",
		"/fonts/Symbols.ttf": "▸",
	})
	chain.entries = []fontFallbackEntry{{path: "/fonts/CJK.ttc"}, {path: "/fonts/Symbols.ttf"}}

	face, ok := chain.faceFor('▸').(*fakeFace)
	if !ok || face.path != "/fonts/Symbols.ttf" {
		t.Fatalf("faceFor('▸') = %#v, want the symbol font", chain.faceFor('▸'))
	}
	if *opens != 1 {
		t.Errorf("fonts opened = %d, want 1: the CJK font does not cover ▸ and must stay closed", *opens)
	}
	if chain.entries[0].face != nil {
		t.Error("the CJK font was retained")
	}
}

func TestChain_WarmScansWithoutOpening(t *testing.T) {
	chain, opens, scans := scannedChain(t, map[string]string{"/a.ttf": "a", "/b.ttf": "b", "/c.ttf": "c"})
	chain.entries = []fontFallbackEntry{{path: "/a.ttf"}, {path: "/b.ttf"}, {path: "/c.ttf"}}

	chain.warm()
	deadline := time.Now().Add(5 * time.Second)
	for {
		chain.mu.Lock()
		done := true
		for _, e := range chain.entries {
			done = done && e.cov != nil
		}
		chain.mu.Unlock()
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("warm did not give every entry its coverage")
		}
		time.Sleep(time.Millisecond)
	}
	if *opens != 0 {
		t.Errorf("warm opened %d fonts, want none", *opens)
	}
	if scans.Load() != 3 {
		t.Errorf("warm scanned %d fonts, want 3", scans.Load())
	}
}

func TestRuneCoverage_SetSkipsWhatACmapCannotHold(t *testing.T) {
	cov := &runeCoverage{}
	for _, r := range []rune{-1, 0xD800, 0xDFFF, runeCoverageMax, 'a', 0x10FFFF} {
		cov.set(r)
	}
	if got := coveredRunes(cov); string(got) != string([]rune{'a', 0x10FFFF}) {
		t.Errorf("covered = %U, want only U+0061 and U+10FFFF", got)
	}
}

func TestChain_WarmParsesWhatTheScanCannotRead(t *testing.T) {
	chain, opens, _ := scannedChain(t, map[string]string{"/odd.ttf": "x", "/b.ttf": "b"}, "/odd.ttf")
	chain.entries = []fontFallbackEntry{{path: "/odd.ttf"}, {path: "/b.ttf"}}

	chain.warm()
	deadline := time.Now().Add(5 * time.Second)
	for {
		chain.mu.Lock()
		done := chain.entries[0].cov != nil && chain.entries[1].cov != nil
		chain.mu.Unlock()
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("warm did not give every entry its coverage")
		}
		time.Sleep(time.Millisecond)
	}
	chain.mu.Lock()
	defer chain.mu.Unlock()
	if !chain.entries[0].unscanned || !chain.entries[0].cov.has('x') {
		t.Errorf("the unreadable font must be parsed for its coverage: %+v", chain.entries[0])
	}
	if *opens != 1 {
		t.Errorf("warm opened %d fonts, want only the one the scan could not read", *opens)
	}
}

func TestChain_UnreadableCoverageFallsBackToTheParse(t *testing.T) {
	chain, opens, scans := scannedChain(t, map[string]string{"/odd.ttf": "x", "/next.ttf": "y"}, "/odd.ttf")
	chain.entries = []fontFallbackEntry{{path: "/odd.ttf"}, {path: "/next.ttf"}}

	if face, _ := chain.faceFor('y').(*fakeFace); face == nil || face.path != "/next.ttf" {
		t.Fatalf("faceFor('y') = %#v, want /next.ttf", face)
	}
	odd := chain.entries[0]
	if !odd.unscanned || odd.cov == nil || !odd.cov.has('x') {
		t.Fatalf("the font scan could not read must get its coverage from the parse: %+v", odd)
	}
	if *opens != 2 {
		t.Errorf("fonts opened = %d, want 2 (the unreadable one, parsed once, and /next.ttf)", *opens)
	}

	// Its coverage is known now: a rune it lacks neither scans nor opens it again.
	before := scans.Load()
	chain.faceFor('q')
	if *opens != 2 || scans.Load() != before {
		t.Errorf("after a miss: opens = %d, scans = %d, want 2 and %d", *opens, scans.Load(), before)
	}
}

func TestChain_NeverOpensAColorBitmapFont(t *testing.T) {
	chain, opens, scans := scannedChain(t, map[string]string{"/Emoji.ttc": "😀♀", "/Symbols.ttf": "♀"})
	scan := chain.scan
	chain.scan = func(path string) (*runeCoverage, error) {
		if path == "/Emoji.ttc" {
			scans.Add(1)
			return nil, errColorBitmapFont
		}
		return scan(path)
	}
	chain.entries = []fontFallbackEntry{{path: "/Emoji.ttc"}, {path: "/Symbols.ttf"}}

	if face, _ := chain.faceFor('♀').(*fakeFace); face == nil || face.path != "/Symbols.ttf" {
		t.Fatalf("faceFor('♀') = %#v, want the symbol font after the emoji font", face)
	}
	if chain.faceFor('😀') != nil {
		t.Error("nothing can draw 😀 here, so nothing may be returned for it")
	}
	emoji := chain.entries[0]
	if emoji.unscanned || emoji.cov == nil {
		t.Errorf("the emoji font must be passed over by its scan, not left to the parse: %+v", emoji)
	}
	if *opens != 1 {
		t.Errorf("fonts opened = %d, want only the symbol font", *opens)
	}
	if scans.Load() != 2 {
		t.Errorf("scans = %d, want one per font", scans.Load())
	}
}
