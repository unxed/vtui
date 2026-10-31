//go:build linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || solaris || illumos

package vtui

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// maxCmapTableSize bounds what one cmap read may allocate. The largest cmap on
// a stock macOS (Hiragino Sans GB) is about 330 KB; a table far beyond that is
// a damaged file, and the parse path is the place to reject it.
const maxCmapTableSize = 8 << 20

// errColorBitmapFont is cmapCoverage's answer for a font whose glyphs are
// color bitmaps. Neither renderer draws one: gg and x/image both rasterize
// outlines only.
var errColorBitmapFont = errors.New("glyphs are color bitmaps")

// cmapCoverage reads which runes a font file maps to a glyph from its cmap
// table alone: the file header, the table directory and the cmap itself, a few
// kilobytes to a few hundred, never the glyph data. A collection answers for
// its first face, the one parseFontBytes and gg open.
//
// It is what the fallback chain probes a font with. Parsing the whole file to
// learn the same thing read 400 MB of fallback fonts on a stock macOS at every
// GUI start, and gg held each of them twice while it did.
func cmapCoverage(path string) (*runeCoverage, error) {
	f, err := os.Open(path) // #nosec G304 -- the path comes from the font lists and discovery
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return fontCoverage(f)
}

// fontCoverage is cmapCoverage over an open file.
func fontCoverage(r io.ReaderAt) (*runeCoverage, error) {
	tables, err := readTableDirectory(r)
	if err != nil {
		return nil, err
	}
	if colorBitmapFont(tables) {
		return nil, errColorBitmapFont
	}
	cmap, ok := tables["cmap"]
	if !ok {
		return nil, errors.New("no cmap table")
	}
	if cmap.length > maxCmapTableSize {
		return nil, fmt.Errorf("cmap table of %d bytes", cmap.length)
	}
	table := make([]byte, cmap.length)
	if err := readAtFull(r, table, cmap.off); err != nil {
		return nil, err
	}
	return parseCmapCoverage(table)
}

// colorBitmapFont reports whether a font's glyphs are color bitmaps: an sbix
// (Apple) or CBDT (Google) table carrying more data than its outlines. Such a
// font maps every emoji in its cmap and draws none of them here, while
// keeping it costs the whole file: Apple Color Emoji is 183 MB, which gg held
// twice for the first emoji on screen. The comparison, rather than the mere
// presence of the table, keeps an outline font with a few bitmap glyphs.
//
// What is lost is the handful of outline glyphs such a font carries besides
// its bitmaps: in Apple Color Emoji the regional indicator letters and the
// keycap mark; its ♀ ♂ ⚕ and digits are in the fonts listed before it.
func colorBitmapFont(tables map[string]sfntTable) bool {
	bitmaps := int64(tables["sbix"].length) + int64(tables["CBDT"].length)
	outlines := int64(tables["glyf"].length) + int64(tables["CFF "].length) + int64(tables["CFF2"].length)
	return bitmaps > outlines
}

// readAtFull fills p from off, accepting the io.EOF a ReaderAt may return
// together with a full read at the end of the file.
func readAtFull(r io.ReaderAt, p []byte, off int64) error {
	n, err := r.ReadAt(p, off)
	if n == len(p) {
		return nil
	}
	if err == nil || errors.Is(err, io.EOF) {
		return io.ErrUnexpectedEOF
	}
	return err
}

// sfntTable is where a table sits in a font file.
type sfntTable struct {
	off    int64
	length uint32
}

// readTableDirectory returns the tables of an sfnt file (TrueType, OpenType,
// or the first face of a collection of either) by tag.
func readTableDirectory(r io.ReaderAt) (map[string]sfntTable, error) {
	var hdr [12]byte
	if err := readAtFull(r, hdr[:], 0); err != nil {
		return nil, err
	}
	base := int64(0)
	if string(hdr[:4]) == "ttcf" {
		if binary.BigEndian.Uint32(hdr[8:12]) == 0 {
			return nil, errors.New("font collection without fonts")
		}
		var first [4]byte
		if err := readAtFull(r, first[:], 12); err != nil {
			return nil, err
		}
		base = int64(binary.BigEndian.Uint32(first[:]))
		if err := readAtFull(r, hdr[:], base); err != nil {
			return nil, err
		}
	}
	switch string(hdr[:4]) {
	case "\x00\x01\x00\x00", "OTTO", "true":
	default:
		return nil, fmt.Errorf("not an sfnt font (tag %q)", hdr[:4])
	}

	numTables := int(binary.BigEndian.Uint16(hdr[4:6]))
	dir := make([]byte, numTables*16)
	if err := readAtFull(r, dir, base+12); err != nil {
		return nil, err
	}
	tables := make(map[string]sfntTable, numTables)
	for i := 0; i < numTables; i++ {
		rec := dir[i*16 : i*16+16]
		tables[string(rec[:4])] = sfntTable{
			off:    int64(binary.BigEndian.Uint32(rec[8:12])),
			length: binary.BigEndian.Uint32(rec[12:16]),
		}
	}
	return tables, nil
}

// parseCmapCoverage builds the coverage of a cmap table.
//
// It is the union of every Unicode subtable the font carries, where gg and
// x/image each pick one. The chain uses coverage only to pass a font over, so
// the union errs in the safe direction: a rune it claims that the chosen
// subtable lacks costs one parse that finds no glyph, while a rune it missed
// would skip a font that renders it. In practice a font's Unicode subtables
// agree, and the union equals what the parsers see.
func parseCmapCoverage(cmap []byte) (*runeCoverage, error) {
	if len(cmap) < 4 {
		return nil, errors.New("cmap table too short")
	}
	numRecords := int(binary.BigEndian.Uint16(cmap[2:4]))
	if len(cmap) < 4+numRecords*8 {
		return nil, errors.New("cmap encoding records truncated")
	}
	cov := &runeCoverage{}
	found := false
	done := make(map[uint32]bool, numRecords) // records often share a subtable
	for i := 0; i < numRecords; i++ {
		rec := cmap[4+i*8 : 4+i*8+8]
		platform := binary.BigEndian.Uint16(rec[0:2])
		encoding := binary.BigEndian.Uint16(rec[2:4])
		off := binary.BigEndian.Uint32(rec[4:8])
		if !unicodeCmapEncoding(platform, encoding) || done[off] || int64(off) >= int64(len(cmap)) {
			continue
		}
		done[off] = true
		if addCmapSubtable(cov, cmap[off:]) {
			found = true
		}
	}
	if !found {
		return nil, errors.New("no Unicode cmap subtable in a supported format")
	}
	return cov, nil
}

// unicodeCmapEncoding reports whether a cmap encoding record maps Unicode code
// points: any Unicode-platform encoding except 5, which holds variation
// sequences (format 14) rather than a mapping, and the Windows BMP and full
// repertoire encodings.
func unicodeCmapEncoding(platform, encoding uint16) bool {
	switch platform {
	case 0:
		return encoding != 5
	case 3:
		return encoding == 1 || encoding == 10
	}
	return false
}

// addCmapSubtable adds the runes one subtable maps to a glyph other than
// .notdef, and reports whether it knew the format. sub runs from the subtable
// to the end of the cmap: format 4's length field is 16 bits and overflows in
// the large CJK fonts, so it is not trusted, the same as in gg.
//
// The formats are the ones gg reads. Format 13 (many-to-one) is left out on
// purpose: gg does not read it, and macOS's LastResort font claims all of
// Unicode through it without drawing a single rune the chain could use.
func addCmapSubtable(cov *runeCoverage, sub []byte) bool {
	if len(sub) < 2 {
		return false
	}
	switch binary.BigEndian.Uint16(sub[0:2]) {
	case 4:
		return addCmapFormat4(cov, sub)
	case 6:
		return addCmapFormat6(cov, sub)
	case 12:
		return addCmapFormat12(cov, sub)
	}
	return false
}

// addCmapFormat4: segment mapping to delta values, the BMP subtable.
func addCmapFormat4(cov *runeCoverage, sub []byte) bool {
	if len(sub) < 14 {
		return false
	}
	segCount := int(binary.BigEndian.Uint16(sub[6:8])) / 2
	endOff := 14
	startOff := endOff + segCount*2 + 2 // reservedPad
	deltaOff := startOff + segCount*2
	rangeOff := deltaOff + segCount*2
	if len(sub) < rangeOff+segCount*2 {
		return false
	}
	for i := 0; i < segCount; i++ {
		end := rune(binary.BigEndian.Uint16(sub[endOff+i*2:]))
		start := rune(binary.BigEndian.Uint16(sub[startOff+i*2:]))
		delta := rune(binary.BigEndian.Uint16(sub[deltaOff+i*2:]))
		ro := int(binary.BigEndian.Uint16(sub[rangeOff+i*2:]))
		for c := start; c <= end; c++ {
			var gid rune
			if ro == 0 {
				gid = (c + delta) & 0xFFFF
			} else {
				// idRangeOffset counts from its own position in the table.
				pos := rangeOff + i*2 + ro + int(c-start)*2
				if pos+2 > len(sub) {
					continue
				}
				if gid = rune(binary.BigEndian.Uint16(sub[pos:])); gid != 0 {
					gid = (gid + delta) & 0xFFFF
				}
			}
			if gid != 0 {
				cov.set(c)
			}
		}
	}
	return true
}

// addCmapFormat6: a trimmed table, one glyph per code from firstCode on.
func addCmapFormat6(cov *runeCoverage, sub []byte) bool {
	if len(sub) < 10 {
		return false
	}
	first := rune(binary.BigEndian.Uint16(sub[6:8]))
	count := int(binary.BigEndian.Uint16(sub[8:10]))
	if len(sub) < 10+count*2 {
		return false
	}
	for i, c := 0, first; i < count; i, c = i+1, c+1 {
		if binary.BigEndian.Uint16(sub[10+i*2:]) != 0 {
			cov.set(c)
		}
	}
	return true
}

// addCmapFormat12: segmented coverage, groups of codes whose glyphs count up
// from the group's first.
func addCmapFormat12(cov *runeCoverage, sub []byte) bool {
	if len(sub) < 16 {
		return false
	}
	n := int64(binary.BigEndian.Uint32(sub[12:16]))
	if avail := int64(len(sub)-16) / 12; n > avail {
		n = avail
	}
	for i := int64(0); i < n; i++ {
		g := sub[16+i*12 : 16+i*12+12]
		start := binary.BigEndian.Uint32(g[0:4])
		end := min(binary.BigEndian.Uint32(g[4:8]), runeCoverageMax-1)
		gid := binary.BigEndian.Uint32(g[8:12])
		for c := start; c <= end; c++ {
			// Glyphs count up from gid, so only the first code can be .notdef.
			if gid != 0 || c != start {
				cov.set(rune(c)) // #nosec G115 -- c <= end < runeCoverageMax
			}
		}
	}
	return true
}
