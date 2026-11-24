package sfnt

import (
	"encoding/binary"
	"fmt"
	"io"
	"sort"
)

// probeTableCap bounds single table reads during probing: the name/OS2/
// post/cmap tables never approach this size, and the cap keeps a corrupt
// table directory from turning a probe into a full-file read.
const probeTableCap = 8 << 20

// probeMaxTables bounds the table-directory size accepted while probing
// (real fonts carry well under a hundred tables).
const probeMaxTables = 1024

// ReadTables reads the named tables of the first font in f WITHOUT
// loading the whole file: the 12-byte header and table directory are
// parsed first, then only the requested tables are read into small
// private copies via ReadAt. It powers the lazy font registry: metadata
// (family, weight, style, cmap coverage) is probed from every system
// font, while the full parse of a file waits until its face is actually
// used. Tables are validated against the file bounds the same way Parse
// validates them; a font that fails here would fail Parse too.
func ReadTables(f io.ReaderAt, want []string) (map[string][]byte, error) {
	var header [12]byte
	if _, err := f.ReadAt(header[:], 0); err != nil {
		return nil, err
	}
	base := uint32(0)
	switch binary.BigEndian.Uint32(header[0:4]) {
	case 0x00010000, 0x74727565: // TrueType
	case 0x4F54544F: // 'OTTO' CFF
	case 0x74746366: // 'ttcf' collection: first font (matches Parse)
		var baseOff [4]byte
		if _, err := f.ReadAt(baseOff[:], 12); err != nil {
			return nil, err
		}
		base = binary.BigEndian.Uint32(baseOff[:])
		var tag [4]byte
		if _, err := f.ReadAt(tag[:], int64(base)); err != nil {
			return nil, err
		}
		if t := binary.BigEndian.Uint32(tag[:]); t != 0x00010000 && t != 0x4F54544F {
			return nil, fmt.Errorf("sfnt: bad collection")
		}
	default:
		return nil, fmt.Errorf("sfnt: not a font")
	}
	var count [2]byte
	if _, err := f.ReadAt(count[:], int64(base)+4); err != nil {
		return nil, err
	}
	numTables := int(binary.BigEndian.Uint16(count[:]))
	if numTables <= 0 || numTables > probeMaxTables {
		return nil, fmt.Errorf("sfnt: bad table count")
	}
	dir := make([]byte, 16*numTables)
	if _, err := f.ReadAt(dir, int64(base)+12); err != nil {
		return nil, err
	}
	tables := map[string][]byte{}
	for i := 0; i < numTables; i++ {
		d := dir[16*i : 16*i+16]
		name := string(d[0:4])
		wanted := false
		for _, w := range want {
			if name == w {
				wanted = true
				break
			}
		}
		if !wanted {
			continue
		}
		off := binary.BigEndian.Uint32(d[8:12])
		length := binary.BigEndian.Uint32(d[12:16])
		if length == 0 || length > probeTableCap {
			continue // treat oversized/empty as absent
		}
		buf := make([]byte, length)
		if n, err := f.ReadAt(buf, int64(off)); err != nil || uint32(n) != length {
			return nil, fmt.Errorf("sfnt: table %s out of range", name)
		}
		tables[name] = buf
	}
	return tables, nil
}

// ParseNameTable extracts the family/subfamily/full/PostScript names from
// a 'name' table (the same selection logic Parse uses).
func ParseNameTable(name []byte) (family, subfamily, fullName, psName string) {
	return parseNames(name)
}

// OS2WeightStyle parses usWeightClass and the fsSelection bold/italic
// bits from an 'OS/2' table (absent tables leave the zero values).
func OS2WeightStyle(os2 []byte) (weight uint16, isBold, isItalic bool) {
	if len(os2) >= 78 {
		weight = binary.BigEndian.Uint16(os2[4:6])
		fsSel := binary.BigEndian.Uint16(os2[62:64])
		isBold = fsSel&(1<<5) != 0
		isItalic = fsSel&(1<<0) != 0
	}
	return
}

// PostItalicAngle parses the italic angle from a 'post' table (0 when
// absent, matching Parse).
func PostItalicAngle(post []byte) int32 {
	if len(post) >= 8 {
		return int32(binary.BigEndian.Uint32(post[4:8]))
	}
	return 0
}

// CoverageRanges extracts the cmap's code-point coverage as sorted,
// merged, inclusive [lo, hi] pairs (lo0, hi0, lo1, hi1, ...). It returns
// nil when the cmap has no usable subtable. The ranges are a CONSERVATIVE
// prefilter — a rune inside a range may still map to glyph 0 in format-4
// idRangeOffset tables — which is fine for its only use: deciding which
// fonts a fallback scan can skip without loading them.
func CoverageRanges(cmap []byte) []uint32 {
	best, format := pickCmapSubtable(cmap)
	if best < 0 {
		return nil
	}
	sub := cmap[best:]
	var pairs []uint32
	switch format {
	case 0:
		pairs = []uint32{0, 0xFF}
	case 4:
		if len(sub) >= 14 {
			segCount := int(binary.BigEndian.Uint16(sub[6:8])) / 2
			endCodes := 14
			startCodes := endCodes + 2 + 2*segCount
			for i := 0; i < segCount; i++ {
				so, eo := startCodes+2*i, endCodes+2*i
				if so+2 > len(sub) || eo+2 > len(sub) {
					break
				}
				start := uint32(binary.BigEndian.Uint16(sub[so:]))
				end := uint32(binary.BigEndian.Uint16(sub[eo:]))
				if end >= start {
					pairs = append(pairs, start, end)
				}
			}
		}
	case 6:
		if len(sub) >= 10 {
			first := uint32(binary.BigEndian.Uint16(sub[6:8]))
			cnt := uint32(binary.BigEndian.Uint16(sub[8:10]))
			if cnt > 0 {
				pairs = append(pairs, first, first+cnt-1)
			}
		}
	case 12:
		if len(sub) >= 16 {
			nGroups := int(binary.BigEndian.Uint32(sub[12:16]))
			for i := 0; i < nGroups; i++ {
				off := 16 + 12*i
				if off+12 > len(sub) {
					break
				}
				start := binary.BigEndian.Uint32(sub[off:])
				end := binary.BigEndian.Uint32(sub[off+4:])
				if end >= start {
					pairs = append(pairs, start, end)
				}
			}
		}
	}
	return mergeRanges(pairs)
}

// mergeRanges sorts [lo, hi] pairs and merges overlapping/adjacent ones.
func mergeRanges(pairs []uint32) []uint32 {
	if len(pairs) == 0 {
		return nil
	}
	type span struct{ lo, hi uint32 }
	spans := make([]span, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		spans = append(spans, span{pairs[i], pairs[i+1]})
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].lo != spans[j].lo {
			return spans[i].lo < spans[j].lo
		}
		return spans[i].hi < spans[j].hi
	})
	out := make([]span, 0, len(spans))
	for _, s := range spans {
		if n := len(out); n > 0 && s.lo <= out[n-1].hi+1 {
			if s.hi > out[n-1].hi {
				out[n-1].hi = s.hi
			}
			continue
		}
		out = append(out, s)
	}
	merged := make([]uint32, 0, 2*len(out))
	for _, s := range out {
		merged = append(merged, s.lo, s.hi)
	}
	return merged
}
