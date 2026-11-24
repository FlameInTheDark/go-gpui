package sfnt

import (
	"encoding/binary"
)

// LayoutTable is a parsed GSUB or GPOS table. Lookup application is
// script-aware like upstream shapers: each script's default langsys indexes
// the feature records that apply to it (FeatureLookupsScript). A global
// tag-keyed view (FeatureLookups) remains as the fallback for fonts whose
// ScriptList does not cover the requested script.
type LayoutTable struct {
	data     []byte
	Features []FeatureRecord
	Lookups  []Lookup
	// featureMap indexes Features by tag; FeatureLookups runs per glyph
	// pair / per cluster during shaping, so the scan must be O(1).
	featureMap map[string][]uint16
	// scriptFeatures maps an OpenType script tag to the feature-record
	// indices of its default langsys.
	scriptFeatures map[string][]uint16
}

// FeatureRecord is one named feature and its lookup indices.
type FeatureRecord struct {
	Tag     string
	Lookups []uint16
}

// Lookup is one lookup subtable set with its type.
type Lookup struct {
	Type      uint16
	Flag      uint16
	Subtables []Subtable
}

// Subtable is one lookup subtable.
type Subtable struct {
	Data []byte
	// CoverageOffset is the subtable-relative coverage offset (standard
	// field position for types 1-2, 4-9 lookups).
	CoverageOffset uint16
	// Type is the resolved lookup type: the lookup's own type, or the
	// extension's target type for GSUB type 7 / GPOS type 9 wrapper
	// subtables (resolved at parse time so consumers never see wrappers).
	Type uint16
	// MarkFilteringSet is the mark-filtering glyph set index when the
	// parent lookup flag sets USE_MARK_FILTERING_SET (GSUB/GPOS spec).
	MarkFilteringSet uint16
}

// parseLayoutTable parses the common GSUB/GPOS header.
func parseLayoutTable(data []byte) *LayoutTable {
	if len(data) < 10 {
		return nil
	}
	t := &LayoutTable{data: data}
	scriptListOff := int(binary.BigEndian.Uint16(data[4:6]))
	featureListOff := int(binary.BigEndian.Uint16(data[6:8]))
	lookupListOff := int(binary.BigEndian.Uint16(data[8:10]))
	parseScriptList(t, scriptListOff)

	// Features.
	if featureListOff+2 <= len(data) {
		fl := data[featureListOff:]
		count := int(binary.BigEndian.Uint16(fl[0:2]))
		for i := 0; i < count && 2+6*i+6 <= len(fl); i++ {
			rec := fl[2+6*i : 2+6*i+6]
			tag := string(rec[0:4])
			off := int(binary.BigEndian.Uint16(rec[4:6]))
			if off+2 > len(fl) {
				continue
			}
			feat := fl[off:]
			lookupCount := int(binary.BigEndian.Uint16(feat[2:4]))
			var lookups []uint16
			for j := 0; j < lookupCount && 4+2*j+2 <= len(feat); j++ {
				lookups = append(lookups, binary.BigEndian.Uint16(feat[4+2*j:]))
			}
			t.Features = append(t.Features, FeatureRecord{Tag: tag, Lookups: lookups})
			// Feature records repeat per script/langsys; a real shaper picks
			// the record for its script. This parser has no script context,
			// so lookups union across records (deduplicated, first
			// occurrence wins for order) -- EXCEPT 'locl', whose whole
			// point is language-specific substitution: applying every
			// language's record at once would rewrite glyphs for the wrong
			// locale, so it keeps the first (default) record only.
			if t.featureMap == nil {
				t.featureMap = map[string][]uint16{}
			}
			if tag == "locl" {
				if _, exists := t.featureMap[tag]; !exists {
					t.featureMap[tag] = lookups
				}
				continue
			}
			existing := t.featureMap[tag]
			for _, li := range lookups {
				dup := false
				for _, e := range existing {
					if e == li {
						dup = true
						break
					}
				}
				if !dup {
					existing = append(existing, li)
				}
			}
			t.featureMap[tag] = existing
		}
	}

	// Lookups.
	if lookupListOff+2 <= len(data) {
		ll := data[lookupListOff:]
		count := int(binary.BigEndian.Uint16(ll[0:2]))
		for i := 0; i < count; i++ {
			if 2+2*i+2 > len(ll) {
				break
			}
			off := int(binary.BigEndian.Uint16(ll[2+2*i:]))
			if off+6 > len(ll) {
				break
			}
			lk := ll[off:]
			ltype := binary.BigEndian.Uint16(lk[0:2])
			lflag := binary.BigEndian.Uint16(lk[2:4])
			subCount := int(binary.BigEndian.Uint16(lk[4:6]))
			var lkup Lookup
			lkup.Type = ltype
			lkup.Flag = lflag
			for j := 0; j < subCount; j++ {
				if 6+2*j+2 > len(lk) {
					break
				}
				so := int(binary.BigEndian.Uint16(lk[6+2*j:]))
				if so >= len(lk) {
					continue
				}
				sub := lk[so:]
				subType := ltype
				// Extension wrappers (GSUB type 7 / GPOS type
				// 9): resolve to the wrapped subtable now so
				// downstream code only sees real lookup types.
				// Extension format 1: u16 format, u16 target
				// lookup type, u32 offset (relative to the
				// extension subtable start).
				if (ltype == 7 || ltype == 9) && len(sub) >= 8 {
					target := binary.BigEndian.Uint16(sub[2:4])
					extOff := int(binary.BigEndian.Uint32(sub[4:8]))
					if extOff > 0 && extOff < len(sub) {
						sub = sub[extOff:]
						subType = target
					}
				}
				var covOff uint16
				// Coverage offset position differs by lookup type/format.
				switch {
				case subType == 1 && len(sub) >= 2 && binary.BigEndian.Uint16(sub[0:2]) == 1:
					// PairPos format 2 has coverage at +2 as well; single
					// subtables carry coverage at +2.
					covOff = binaryAt(sub, 2)
				case subType == 2 && len(sub) >= 4:
					covOff = binaryAt(sub, 2)
				case subType == 4 || subType == 5 || subType == 6:
					covOff = binaryAt(sub, 2)
				case subType == 7 || subType == 8:
					covOff = binaryAt(sub, 2)
				case subType == 9:
					covOff = 0
				default:
					covOff = binaryAt(sub, 2)
				}
				lkup.Subtables = append(lkup.Subtables, Subtable{Data: sub, CoverageOffset: covOff, Type: subType})
			}
			t.Lookups = append(t.Lookups, lkup)
		}
	}
	return t
}

func binaryAt(b []byte, off int) uint16 {
	if off+2 > len(b) {
		return 0
	}
	return binary.BigEndian.Uint16(b[off : off+2])
}

// FeatureLookups returns the lookup indices for a feature tag (O(1) via
// the index built at parse time).
func (t *LayoutTable) FeatureLookups(tag string) []uint16 {
	if t.featureMap != nil {
		if out, ok := t.featureMap[tag]; ok && len(out) > 0 {
			return out
		}
	}
	for _, f := range t.Features {
		if f.Tag == tag {
			return f.Lookups
		}
	}
	return nil
}

// LookupByIndex returns a lookup by id.
func (t *LayoutTable) LookupByIndex(i uint16) (Lookup, bool) {
	if int(i) < len(t.Lookups) {
		return t.Lookups[i], true
	}
	return Lookup{}, false
}

// CoverageIndexAt evaluates a coverage table at data[off:] for one glyph,
// returning the coverage index (OpenType coverage table, both formats).
func CoverageIndexAt(data []byte, off int, gid uint16) (int, bool) {
	if off+4 > len(data) {
		return 0, false
	}
	cov := data[off:]
	format := binary.BigEndian.Uint16(cov[0:2])
	switch format {
	case 1:
		// The glyph array is sorted by the OpenType spec: binary search.
		count := int(binary.BigEndian.Uint16(cov[2:4]))
		lo, hi := 0, count
		for lo < hi {
			mid := int(uint(lo+hi) >> 1)
			if 4+2*mid+2 > len(cov) {
				return 0, false
			}
			g := binary.BigEndian.Uint16(cov[4+2*mid:])
			if g == gid {
				return mid, true
			}
			if g < gid {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
	case 2:
		// Range records are sorted by start glyph: binary search.
		rangeCount := int(binary.BigEndian.Uint16(cov[2:4]))
		lo, hi := 0, rangeCount
		for lo < hi {
			mid := int(uint(lo+hi) >> 1)
			o := 4 + 6*mid
			if o+6 > len(cov) {
				return 0, false
			}
			start := binary.BigEndian.Uint16(cov[o:])
			end := binary.BigEndian.Uint16(cov[o+2:])
			if gid < start {
				hi = mid
			} else if gid > end {
				lo = mid + 1
			} else {
				startCov := binary.BigEndian.Uint16(cov[o+4:])
				return int(startCov) + int(gid-start), true
			}
		}
	}
	return 0, false
}

// CoverageContainsAt reports whether the coverage table at data[off:] covers
// the glyph.
func CoverageContainsAt(data []byte, off int, gid uint16) bool {
	_, ok := CoverageIndexAt(data, off, gid)
	return ok
}

// CoverageEntry maps a glyph to a coverage index.
func CoverageIndex(sub Subtable, gid uint16) (int, bool) {
	return CoverageIndexAt(sub.Data, int(sub.CoverageOffset), gid)
}

// parseScriptList reads the ScriptList: each script's default langsys
// indexes the feature records that apply to it.
//
//	ScriptList: u16 scriptCount; ScriptRecord{ tag(4), offset(2) }
//	Script: u16 defaultLangSysOff, u16 langSysCount, LangSysRecord{...}
//	LangSys: u16 lookupOrderOff, u16 requiredFeatureIndex, u16 featureIdxCount, u16 featureIndices[]
func parseScriptList(t *LayoutTable, off int) {
	if off <= 0 || off+2 > len(t.data) {
		return
	}
	sl := t.data[off:]
	count := int(binary.BigEndian.Uint16(sl[0:2]))
	for i := 0; i < count; i++ {
		if 2+6*i+6 > len(sl) {
			break
		}
		rec := sl[2+6*i : 2+6*i+6]
		tag := string(rec[0:4])
		so := int(binary.BigEndian.Uint16(rec[4:6]))
		if so+2 > len(sl) {
			continue
		}
		script := sl[so:]
		defOff := int(binary.BigEndian.Uint16(script[0:2]))
		if defOff == 0 || defOff+4 > len(script) {
			continue
		}
		ls := script[defOff:]
		featCount := int(binary.BigEndian.Uint16(ls[4:6]))
		var idxs []uint16
		for j := 0; j < featCount; j++ {
			if 6+2*j+2 > len(ls) {
				break
			}
			idxs = append(idxs, binary.BigEndian.Uint16(ls[6+2*j:]))
		}
		if t.scriptFeatures == nil {
			t.scriptFeatures = map[string][]uint16{}
		}
		t.scriptFeatures[tag] = idxs
	}
}

// FeatureLookupsScript returns the lookup indices of one feature for a
// script's default langsys (upstream feature selection semantics). Falls
// back to the DFLT script, then to the global feature view for fonts
// without script coverage.
func (t *LayoutTable) FeatureLookupsScript(tag, script string) []uint16 {
	for _, s := range []string{script, "DFLT"} {
		if idxs, ok := t.scriptFeatures[s]; ok {
			var out []uint16
			for _, fi := range idxs {
				if int(fi) < len(t.Features) && t.Features[fi].Tag == tag {
					out = append(out, t.Features[fi].Lookups...)
				}
			}
			if len(out) > 0 {
				return out
			}
		}
	}
	return t.FeatureLookups(tag)
}
