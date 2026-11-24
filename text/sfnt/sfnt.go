// Package sfnt is a pure-Go parser for TrueType/OpenType fonts (the tables
// gpui needs: head, hhea, maxp, hmtx, cmap, glyf, loca, kern, OS/2, post,
// name and the GSUB/GPOS shaping tables). TrueType (glyf) outlines are
// supported for rasterization; CFF outlines are parsed for metrics only.
package sfnt

import (
	"encoding/binary"
	"fmt"
)

// Font is a parsed font file.
type Font struct {
	Data             []byte
	UnitsPerEm       uint16
	IndexToLocFormat int32 // 0 = short, 1 = long

	// head
	XMin, YMin, XMax, YMax int16

	// hhea
	Ascender, Descender, LineGap int16
	NumberOfHMetrics             uint16

	// OS/2 (may be absent)
	TypoAscender, TypoDescender, TypoLineGap int16
	USE_TYPO_METRICS                         bool
	WeightClass                              uint16
	IsFixedPitch                             bool

	// post
	PostItalicAngle                               int32
	PostIsFixedPitch                              bool
	PostUnderlinePosition, PostUnderlineThickness int16

	// name
	FamilyName       string
	SubfamilyName    string
	FullName         string
	PostScriptName   string
	IsBold, IsItalic bool

	// cmap
	cmapSubtable []byte
	cmapFormat   int

	// glyf/loca
	loca []byte
	glyf []byte
	// CFF presence
	isCFF bool
	cff   *CFF

	hmtx []byte

	// kern (format 0)
	kernPairs map[uint32]int16

	// GSUB / GPOS / GDEF
	GSUB *LayoutTable
	GPOS *LayoutTable
	GDEF *GDEF
}

// Table directories.
type tableDir struct {
	tag      [4]byte
	checksum uint32
	offset   uint32
	length   uint32
}

// Parse parses a font file (TTF/OTF; the first font of a collection).
func Parse(data []byte) (*Font, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("sfnt: too short")
	}
	tag := binary.BigEndian.Uint32(data[0:4])
	base := uint32(0)
	switch tag {
	case 0x00010000, 0x74727565: // TrueType
	case 0x4F54544F: // 'OTTO' CFF
	case 0x74746366: // 'ttcf' collection: use first font
		base = binary.BigEndian.Uint32(data[12:16])
		if tag := binary.BigEndian.Uint32(data[base:]); tag != 0x00010000 && tag != 0x4F54544F {
			return nil, fmt.Errorf("sfnt: bad collection")
		}
	default:
		return nil, fmt.Errorf("sfnt: not a font (tag %08x)", tag)
	}
	numTables := int(binary.BigEndian.Uint16(data[base+4:]))
	if int(base)+12+16*numTables > len(data) {
		return nil, fmt.Errorf("sfnt: bad table count")
	}
	tables := map[string][]byte{}
	for i := 0; i < numTables; i++ {
		d := data[base+uint32(12+16*i) : base+uint32(12+16*i+16)]
		name := string(d[0:4])
		off := binary.BigEndian.Uint32(d[8:12])
		length := binary.BigEndian.Uint32(d[12:16])
		if int(off)+int(length) > len(data) {
			return nil, fmt.Errorf("sfnt: table %s out of range", name)
		}
		tables[name] = data[off : off+length]
	}

	f := &Font{Data: data}
	if head := tables["head"]; len(head) >= 54 {
		f.UnitsPerEm = binary.BigEndian.Uint16(head[18:20])
		f.XMin = int16(binary.BigEndian.Uint16(head[36:38]))
		f.YMin = int16(binary.BigEndian.Uint16(head[38:40]))
		f.XMax = int16(binary.BigEndian.Uint16(head[40:42]))
		f.YMax = int16(binary.BigEndian.Uint16(head[42:44]))
		f.IndexToLocFormat = int32(int16(binary.BigEndian.Uint16(head[50:52])))
	} else {
		return nil, fmt.Errorf("sfnt: missing head")
	}
	if hhea := tables["hhea"]; len(hhea) >= 36 {
		f.Ascender = int16(binary.BigEndian.Uint16(hhea[4:6]))
		f.Descender = int16(binary.BigEndian.Uint16(hhea[6:8]))
		f.LineGap = int16(binary.BigEndian.Uint16(hhea[8:10]))
		f.NumberOfHMetrics = binary.BigEndian.Uint16(hhea[34:36])
	}
	f.hmtx = tables["hmtx"]

	if os2 := tables["OS/2"]; len(os2) >= 78 {
		f.WeightClass = binary.BigEndian.Uint16(os2[4:6])
		fsSel := binary.BigEndian.Uint16(os2[62:64])
		f.IsItalic = fsSel&(1<<0) != 0
		f.IsBold = fsSel&(1<<5) != 0
		f.USE_TYPO_METRICS = fsSel&(1<<7) != 0
		f.TypoAscender = int16(binary.BigEndian.Uint16(os2[68:70]))
		f.TypoDescender = int16(binary.BigEndian.Uint16(os2[70:72]))
		f.TypoLineGap = int16(binary.BigEndian.Uint16(os2[72:74]))
	}
	if post := tables["post"]; len(post) >= 32 {
		f.PostItalicAngle = int32(binary.BigEndian.Uint32(post[4:8]))
		f.PostUnderlinePosition = int16(binary.BigEndian.Uint16(post[8:10]))
		f.PostUnderlineThickness = int16(binary.BigEndian.Uint16(post[10:12]))
		f.PostIsFixedPitch = binary.BigEndian.Uint32(post[12:16]) != 0
	}

	if name := tables["name"]; name != nil {
		f.FamilyName, f.SubfamilyName, f.FullName, f.PostScriptName = parseNames(name)
	}
	// Bold/italic fallbacks from the subfamily name.
	if f.SubfamilyName == "Bold" {
		f.IsBold = true
	}
	if f.SubfamilyName == "Italic" || f.SubfamilyName == "Bold Italic" {
		f.IsItalic = true
	}

	if err := f.parseCmap(tables["cmap"]); err != nil {
		return nil, err
	}

	f.loca = tables["loca"]
	f.glyf = tables["glyf"]
	if cffData := tables["CFF "]; cffData != nil {
		f.isCFF = true
		cff, err := parseCFF(cffData)
		if err == nil {
			f.cff = cff
		}
	}

	f.parseKern(tables["kern"])

	if gsub := tables["GSUB"]; gsub != nil {
		f.GSUB = parseLayoutTable(gsub)
	}
	if gpos := tables["GPOS"]; gpos != nil {
		f.GPOS = parseLayoutTable(gpos)
	}
	if gdef := tables["GDEF"]; gdef != nil {
		f.GDEF = parseGDEF(gdef)
	}
	return f, nil
}

// IsCFF reports a CFF-outline font.
func (f *Font) IsCFF() bool { return f.isCFF }

func parseNames(name []byte) (family, subfamily, fullName, psName string) {
	if len(name) < 6 {
		return
	}
	count := int(binary.BigEndian.Uint16(name[2:4]))
	strOff := int(binary.BigEndian.Uint16(name[4:6]))
	pick := func(want []uint16) string {
		best := ""
		bestRank := -1
		for i := 0; i < count && 6+12*i+12 <= len(name); i++ {
			rec := name[6+12*i : 6+12*i+12]
			platform := binary.BigEndian.Uint16(rec[0:2])
			_ = binary.BigEndian.Uint16(rec[2:4])
			nameID := binary.BigEndian.Uint16(rec[6:8])
			length := int(binary.BigEndian.Uint16(rec[8:10]))
			offset := int(binary.BigEndian.Uint16(rec[10:12]))
			match := false
			for _, w := range want {
				if nameID == w {
					match = true
				}
			}
			if !match || strOff+offset+length > len(name) {
				continue
			}
			raw := name[strOff+offset : strOff+offset+length]
			rank := 0
			var s string
			if platform == 3 { // Windows: UTF-16BE
				rank = 2
				b := make([]byte, 0, len(raw)/2)
				for j := 0; j+1 < len(raw); j += 2 {
					b = append(b, byte(binary.BigEndian.Uint16(raw[j:j+2])))
				}
				s = string(b)
			} else if platform == 1 { // Mac: ASCII
				rank = 1
				s = string(raw)
			}
			if rank > bestRank {
				best, bestRank = s, rank
			}
		}
		return best
	}
	family = pick([]uint16{1})
	subfamily = pick([]uint16{2})
	fullName = pick([]uint16{4})
	psName = pick([]uint16{6})
	return
}

func (f *Font) parseCmap(cmap []byte) error {
	best, format := pickCmapSubtable(cmap)
	if best < 0 {
		return fmt.Errorf("sfnt: no usable cmap subtable")
	}
	f.cmapFormat = format
	f.cmapSubtable = cmap[best:]
	return nil
}

// pickCmapSubtable selects the highest-scoring usable cmap subtable,
// returning its byte offset within cmap and its format (-1 when none).
// Shared by Parse (glyph mapping) and CoverageRanges (lazy-registry
// coverage prefilters).
func pickCmapSubtable(cmap []byte) (best int, bestFormat int) {
	if len(cmap) < 4 {
		return -1, 0
	}
	numTables := int(binary.BigEndian.Uint16(cmap[2:4]))
	best = -1
	bestScore := -1
	for i := 0; i < numTables && 4+8*i+8 <= len(cmap); i++ {
		rec := cmap[4+8*i : 4+8*i+8]
		platform := binary.BigEndian.Uint16(rec[0:2])
		encoding := binary.BigEndian.Uint16(rec[2:4])
		offset := int(binary.BigEndian.Uint32(rec[4:8]))
		score := 0
		switch {
		case platform == 3 && encoding == 10:
			score = 6
		case platform == 3 && encoding == 1:
			score = 5
		case platform == 0:
			score = 4
		case platform == 3 && encoding == 0:
			score = 3
		case platform == 1 && encoding == 0:
			score = 2
		}
		if score > bestScore && offset < len(cmap) {
			format := int(binary.BigEndian.Uint16(cmap[offset : offset+2]))
			switch format {
			case 0, 4, 6, 12:
				bestScore = score
				best = offset
				bestFormat = format
			}
		}
	}
	return best, bestFormat
}

// GlyphID maps a rune to a glyph id (0 = missing).
func (f *Font) GlyphID(r rune) uint16 {
	b := f.cmapSubtable
	if len(b) < 2 {
		return 0
	}
	format := int(binary.BigEndian.Uint16(b[0:2]))
	switch format {
	case 0:
		if len(b) >= 262 && r < 256 {
			return uint16(b[6+r])
		}
	case 4:
		if len(b) < 14 {
			return 0
		}
		segCount := int(binary.BigEndian.Uint16(b[6:8])) / 2
		endCodes := 14
		startCodes := endCodes + 2 + 2*segCount
		idDeltas := startCodes + 2*segCount
		idRangeOffsets := idDeltas + 2*segCount
		for i := 0; i < segCount; i++ {
			end := uint32(binary.BigEndian.Uint16(b[endCodes+2*i:]))
			if uint32(r) > end {
				continue
			}
			start := uint32(binary.BigEndian.Uint16(b[startCodes+2*i:]))
			if uint32(r) < start {
				return 0
			}
			delta := uint32(binary.BigEndian.Uint16(b[idDeltas+2*i:]))
			ro := int(binary.BigEndian.Uint16(b[idRangeOffsets+2*i:]))
			if ro == 0 {
				return uint16((uint32(r) + delta) & 0xFFFF)
			}
			glyphIdxAddr := idRangeOffsets + 2*i + ro + 2*int(uint32(r)-start)
			if glyphIdxAddr+2 > len(b) {
				return 0
			}
			g := binary.BigEndian.Uint16(b[glyphIdxAddr:])
			if g == 0 {
				return 0
			}
			return uint16((uint32(g) + delta) & 0xFFFF)
		}
	case 6:
		if len(b) < 10 {
			return 0
		}
		first := uint32(binary.BigEndian.Uint16(b[6:8]))
		count := uint32(binary.BigEndian.Uint16(b[8:10]))
		if uint32(r) < first || uint32(r) >= first+count {
			return 0
		}
		off := 10 + 2*int(uint32(r)-first)
		if off+2 > len(b) {
			return 0
		}
		return binary.BigEndian.Uint16(b[off:])
	case 12:
		if len(b) < 16 {
			return 0
		}
		nGroups := int(binary.BigEndian.Uint32(b[12:16]))
		for i := 0; i < nGroups; i++ {
			off := 16 + 12*i
			if off+12 > len(b) {
				return 0
			}
			start := binary.BigEndian.Uint32(b[off:])
			end := binary.BigEndian.Uint32(b[off+4:])
			if uint32(r) < start {
				return 0
			}
			if uint32(r) <= end {
				startGID := binary.BigEndian.Uint32(b[off+8:])
				return uint16(startGID + uint32(r) - start)
			}
		}
	}
	return 0
}

// HasGlyph reports whether the font provides a glyph for the rune.
func (f *Font) HasGlyph(r rune) bool {
	if r < 0 {
		return false
	}
	// U+FFFD or .notdef fallbacks: treat glyph 0 as missing except for the
	// replacement char itself.
	gid := f.GlyphID(r)
	if gid == 0 {
		return r == 0xFFFD
	}
	return true
}

// Advance returns the advance width in font units.
func (f *Font) Advance(gid uint16) int16 {
	if f.NumberOfHMetrics == 0 || f.hmtx == nil {
		return 0
	}
	if gid >= f.NumberOfHMetrics {
		gid = f.NumberOfHMetrics - 1
	}
	off := int(gid) * 4
	if off+2 > len(f.hmtx) {
		return 0
	}
	return int16(binary.BigEndian.Uint16(f.hmtx[off : off+2]))
}

// Metrics returns the font metrics in font units, preferring OS/2 typo
// metrics when the font requests them (matching skrifa/swash behavior).
type Metrics struct {
	UnitsPerEm         uint16
	Ascent             float32 // positive
	Descent            float32 // negative
	LineGap            float32
	UnderlinePosition  float32 // negative (below baseline)
	UnderlineThickness float32
	CapHeight          float32
	XHeight            float32
	BoundingBox        [4]float32
}

// Metrics computes scaled-normalized metrics (per em).
func (f *Font) Metrics() Metrics {
	upem := float32(f.UnitsPerEm)
	if upem == 0 {
		upem = 1000
	}
	var m Metrics
	m.UnitsPerEm = f.UnitsPerEm
	if f.USE_TYPO_METRICS && f.TypoAscender != 0 {
		m.Ascent = float32(f.TypoAscender) / upem
		m.Descent = float32(f.TypoDescender) / upem
		m.LineGap = float32(f.TypoLineGap) / upem
	} else {
		m.Ascent = float32(f.Ascender) / upem
		m.Descent = float32(f.Descender) / upem
		m.LineGap = float32(f.LineGap) / upem
	}
	if f.PostUnderlinePosition != 0 {
		m.UnderlinePosition = float32(f.PostUnderlinePosition) / upem
		m.UnderlineThickness = float32(f.PostUnderlineThickness) / upem
	} else {
		m.UnderlinePosition = -0.1
		m.UnderlineThickness = 0.05
	}
	if f.cff != nil {
		if c := f.cff.topDICT; c != nil {
			_ = c
		}
	}
	m.BoundingBox = [4]float32{
		float32(f.XMin) / upem, float32(f.YMin) / upem,
		float32(f.XMax) / upem, float32(f.YMax) / upem,
	}
	// CapHeight/XHeight from OS/2 fields 87.. if version >= 2.
	m.CapHeight = 0.7
	m.XHeight = 0.52
	return m
}

func (f *Font) parseKern(kern []byte) {
	if len(kern) < 4 {
		return
	}
	f.kernPairs = map[uint32]int16{}
	nTables := int(binary.BigEndian.Uint16(kern[2:4]))
	off := 4
	for i := 0; i < nTables; i++ {
		if off+6 > len(kern) {
			return
		}
		length := int(binary.BigEndian.Uint16(kern[off+2 : off+4]))
		coverage := binary.BigEndian.Uint16(kern[off+4 : off+6])
		format := coverage >> 8
		if format == 0 && off+14 <= len(kern) {
			nPairs := int(binary.BigEndian.Uint16(kern[off+6 : off+8]))
			for p := 0; p < nPairs; p++ {
				po := off + 14 + 6*p
				if po+6 > len(kern) {
					break
				}
				left := binary.BigEndian.Uint16(kern[po:])
				right := binary.BigEndian.Uint16(kern[po+2:])
				value := int16(binary.BigEndian.Uint16(kern[po+4:]))
				f.kernPairs[uint32(left)<<16|uint32(right)] = value
			}
		}
		off += length
		if length <= 0 {
			return
		}
	}
}

// Kern returns the format-0 'kern' pair adjustment for two glyph ids.
func (f *Font) Kern(left, right uint16) int16 {
	if f.kernPairs == nil {
		return 0
	}
	return f.kernPairs[uint32(left)<<16|uint32(right)]
}

// IsOblique reports an oblique style (from post italic angle).
func (f *Font) IsOblique() bool {
	return f.PostItalicAngle != 0 && !f.IsItalic
}
