package sfnt

import (
	"encoding/binary"
)

// Glyph classes (GDEF GlyphClassDef values).
const (
	ClassBase      = 1
	ClassLigature  = 2
	ClassMark      = 3
	ClassComponent = 4
)

// GDEF is the parsed glyph-definition table: the glyph class def (base /
// ligature / mark / component), the mark-attach class def and the mark glyph
// sets (GDEF 1.2), all used by GSUB/GPOS lookup flags and mark positioning.
type GDEF struct {
	data             []byte
	glyphClassDefOff int
	attachClassOff   int
	// markGlyphSetsOff is the MarkGlyphSetsDef offset (0 when absent).
	markGlyphSetsOff    int
	markGlyphSetCovOffs []int
}

// parseGDEF parses the GDEF header.
//
//	Version 1.0: glyphClassDef(4) attachList(6) ligCaretList(8) markAttachClassDef(10)
//	Version 1.2: + markGlyphSetsDef(12)
//	Version 1.3: + itemVarStore(14)
func parseGDEF(data []byte) *GDEF {
	if len(data) < 12 {
		return nil
	}
	g := &GDEF{
		data:             data,
		glyphClassDefOff: int(binary.BigEndian.Uint16(data[4:6])),
		attachClassOff:   int(binary.BigEndian.Uint16(data[10:12])),
	}
	if g.glyphClassDefOff == 0 && g.attachClassOff == 0 {
		return nil
	}
	if len(data) >= 14 {
		g.markGlyphSetsOff = int(binary.BigEndian.Uint16(data[12:14]))
	}
	if g.markGlyphSetsOff > 0 && g.markGlyphSetsOff+4 <= len(data) {
		mgs := data[g.markGlyphSetsOff:]
		if n := int(binary.BigEndian.Uint16(mgs[2:4])); n > 0 {
			for i := 0; i < n; i++ {
				if 4+4*i+4 > len(mgs) {
					break
				}
				g.markGlyphSetCovOffs = append(g.markGlyphSetCovOffs, g.markGlyphSetsOff+int(binary.BigEndian.Uint32(mgs[4+4*i:])))
			}
		}
	}
	return g
}

// GlyphClass returns the GDEF class of a glyph (0 when the font does not
// classify it).
func (f *Font) GlyphClass(gid uint16) int {
	if f.GDEF == nil || f.GDEF.glyphClassDefOff == 0 {
		return 0
	}
	return ClassDefValue(f.GDEF.data, f.GDEF.glyphClassDefOff, gid)
}

// MarkAttachClass returns the mark-attachment class of a glyph (0 when
// unclassified).
func (f *Font) MarkAttachClass(gid uint16) int {
	if f.GDEF == nil || f.GDEF.attachClassOff == 0 {
		return 0
	}
	return ClassDefValue(f.GDEF.data, f.GDEF.attachClassOff, gid)
}

// IsMarkGlyph reports whether the glyph is a combining mark per GDEF (class 3).
func (f *Font) IsMarkGlyph(gid uint16) bool {
	return f.GlyphClass(gid) == ClassMark
}

// InMarkFilterSet reports whether the glyph belongs to the mark glyph set
// (used by the GSUB/GPOS lookup flag USE_MARK_FILTERING_SET).
func (f *Font) InMarkFilterSet(setIdx uint16, gid uint16) bool {
	if f.GDEF == nil || int(setIdx) >= len(f.GDEF.markGlyphSetCovOffs) {
		return false
	}
	return CoverageContainsAt(f.GDEF.data, f.GDEF.markGlyphSetCovOffs[setIdx], gid)
}

// ClassDefValue evaluates a ClassDef table at data[off:] for one glyph
// (OpenType "class definition table", shared by GDEF and the class-based
// contextual lookup formats).
func ClassDefValue(data []byte, off int, gid uint16) int {
	if off+6 > len(data) {
		return 0
	}
	cd := data[off:]
	format := binary.BigEndian.Uint16(cd[0:2])
	switch format {
	case 1:
		start := binary.BigEndian.Uint16(cd[2:4])
		count := int(binary.BigEndian.Uint16(cd[4:6]))
		if gid < start || int(gid-start) >= count {
			return 0
		}
		if 6+2*int(gid-start)+2 > len(cd) {
			return 0
		}
		return int(binary.BigEndian.Uint16(cd[6+2*int(gid-start):]))
	case 2:
		n := int(binary.BigEndian.Uint16(cd[2:4]))
		// Records are sorted by start glyph: binary search.
		lo, hi := 0, n
		for lo < hi {
			mid := int(uint(lo+hi) >> 1)
			o := 4 + 6*mid
			if o+6 > len(cd) {
				return 0
			}
			start := binary.BigEndian.Uint16(cd[o:])
			end := binary.BigEndian.Uint16(cd[o+2:])
			if gid < start {
				hi = mid
			} else if gid > end {
				lo = mid + 1
			} else {
				return int(binary.BigEndian.Uint16(cd[o+4:]))
			}
		}
	}
	return 0
}
