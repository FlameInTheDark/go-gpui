package sfnt

import (
	"encoding/binary"
	"os"
	"testing"
)

// buildCmap4 builds a minimal cmap table with one (platform 3, encoding 1)
// format-4 subtable covering the given inclusive [start, end] segments.
func buildCmap4(segs [][2]uint16) []byte {
	segCount := len(segs)
	subLen := 16 + 8*segCount // header + 4 arrays (endCodes, startCodes, deltas, rangeOffsets) + pad
	tableLen := 4 + 8 + subLen
	buf := make([]byte, tableLen)
	binary.BigEndian.PutUint16(buf[0:2], 0)   // version
	binary.BigEndian.PutUint16(buf[2:4], 1)   // numTables
	binary.BigEndian.PutUint16(buf[4:6], 3)   // platformID
	binary.BigEndian.PutUint16(buf[6:8], 1)   // encodingID
	binary.BigEndian.PutUint32(buf[8:12], 12) // subtable offset
	sub := buf[12:]
	binary.BigEndian.PutUint16(sub[0:2], 4) // format
	binary.BigEndian.PutUint16(sub[2:4], uint16(subLen))
	binary.BigEndian.PutUint16(sub[4:6], 0) // language
	binary.BigEndian.PutUint16(sub[6:8], uint16(2*segCount))
	binary.BigEndian.PutUint16(sub[8:10], uint16(2*segCount))
	binary.BigEndian.PutUint16(sub[10:12], 0)
	binary.BigEndian.PutUint16(sub[12:14], 0)
	// Format 4: 14-byte header, endCodes[n], reservedPad(2),
	// startCodes[n], idDeltas[n], idRangeOffsets[n].
	endCodes := sub[14:]
	startCodes := sub[16+2*segCount:]
	idDeltas := sub[16+4*segCount:]
	idRangeOffsets := sub[16+6*segCount:]
	for i, s := range segs {
		binary.BigEndian.PutUint16(endCodes[2*i:], s[1])
		binary.BigEndian.PutUint16(startCodes[2*i:], s[0])
		binary.BigEndian.PutUint16(idDeltas[2*i:], 0)
		binary.BigEndian.PutUint16(idRangeOffsets[2*i:], 0)
	}
	return buf
}

// TestCoverageRangesFormat4 verifies range extraction and merging on a
// synthetic format-4 cmap (segments are emitted unsorted and adjacent on
// purpose: the result must be sorted and merged).
func TestCoverageRangesFormat4(t *testing.T) {
	cmap := buildCmap4([][2]uint16{
		{0x43, 0x46}, // C-F
		{0x41, 0x42}, // A-B (adjacent to C-F: must merge into A-F)
		{0x430, 0x434},
	})
	got := CoverageRanges(cmap)
	want := []uint32{0x41, 0x46, 0x430, 0x434}
	if len(got) != len(want) {
		t.Fatalf("ranges = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ranges = %v, want %v", got, want)
		}
	}
}

// TestCoverageRangesNoSubtable: a cmap without a usable subtable yields
// nil (the lazy registry rejects such fonts, mirroring Parse).
func TestCoverageRangesNoSubtable(t *testing.T) {
	if got := CoverageRanges(nil); got != nil {
		t.Fatalf("CoverageRanges(nil) = %v, want nil", got)
	}
	if got := CoverageRanges([]byte{0, 0, 0, 0}); got != nil {
		t.Fatalf("empty cmap: got %v, want nil", got)
	}
}

// TestPickCmapSubtableShared verifies Parse and CoverageRanges agree on
// the selected subtable (the shared picker) on a real font.
func TestPickCmapSubtableShared(t *testing.T) {
	data, err := os.ReadFile("/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf")
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	// Extract the cmap slice the same way Parse's table walk does.
	numTables := int(binary.BigEndian.Uint16(data[4:]))
	var cmap []byte
	for i := 0; i < numTables; i++ {
		d := data[12+16*i : 12+16*i+16]
		if string(d[0:4]) == "cmap" {
			off := binary.BigEndian.Uint32(d[8:12])
			length := binary.BigEndian.Uint32(d[12:16])
			cmap = data[off : off+length]
			break
		}
	}
	if cmap == nil {
		t.Fatal("no cmap in fixture")
	}
	f, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	best, format := pickCmapSubtable(cmap)
	if format != f.cmapFormat {
		t.Fatalf("picker format %d, Parse picked %d", format, f.cmapFormat)
	}
	if &cmap[best] != &f.cmapSubtable[0] {
		t.Fatal("picker selected a different subtable than Parse")
	}
}
