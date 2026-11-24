package text

import (
	"github.com/FlameInTheDark/go-gpui/text/sfnt"
)

// GPOS positioning engine: single, pair (kerning), mark-to-base and
// mark-to-mark attachment, plus the contextual wrappers around them. All
// values are accumulated in design units on the shaping buffer.

// valueRecord is one GPOS value record (only the fields the port uses; the
// device/variation offsets of format 3 are ignored).
type valueRecord struct {
	xPlacement, yPlacement int
	xAdvance, yAdvance     int
}

// readValueRecord reads a value record at d[off:] and returns it with its
// byte size (0 when the format is empty).
func readValueRecord(d []byte, off int, format uint16) (valueRecord, int) {
	var v valueRecord
	size := 0
	fields := []*int{&v.xPlacement, &v.yPlacement, &v.xAdvance, &v.yAdvance}
	for i, bit := range []uint16{0x0001, 0x0002, 0x0004, 0x0008} {
		if format&bit != 0 {
			// Records pack only the fields present in the format: the
			// running size (not the bit index) is the field offset.
			if off+size+2 > len(d) {
				return v, size
			}
			*fields[i] = i16at(d, off+size)
			size += 2
		}
	}
	return v, size
}

// applyGPOSFeature applies one GPOS feature's lookups over the buffer. Each
// lookup runs as its own pass (upstream semantics): every position where the
// lookup matches is positioned.
func (sh *shaper) applyGPOSFeature(tag string) {
	for _, li := range sh.gpos.FeatureLookupsScript(tag, sh.script) {
		if _, ok := sh.gpos.LookupByIndex(li); !ok {
			continue
		}
		for pos := 0; pos < len(sh.buf); pos++ {
			// A pair match positions both glyphs: skip the second.
			if sh.applyGPOSLookup(sh.gpos, li, pos, 0) == 2 {
				pos++
			}
		}
	}
}

// applyGPOSLookup applies one GPOS lookup at pos; returns 2 when the match
// consumed two glyphs (pair), 1 for a single-glyph match, 0 for no match.
func (sh *shaper) applyGPOSLookup(table *sfnt.LayoutTable, li uint16, pos int, depth int) int {
	if depth > maxLookupNesting || pos < 0 || pos >= len(sh.buf) {
		return 0
	}
	lk, ok := table.LookupByIndex(li)
	if !ok {
		return 0
	}
	for _, sub := range lk.Subtables {
		switch sub.Type {
		case 1: // SinglePos
			if sh.gposSingle(sub, pos) {
				return 1
			}
		case 2: // PairPos
			if n := sh.gposPair(lk, sub, pos); n > 0 {
				return n
			}
		case 4: // MarkBasePos
			if sh.gposMarkBase(sub, pos) {
				return 1
			}
		case 5: // MarkLigPos
			if sh.gposMarkLig(sub, pos) {
				return 1
			}
		case 6: // MarkMarkPos
			if sh.gposMarkMark(sub, pos) {
				return 1
			}
		case 7: // ContextPos
			if sh.applyContext(table, sub, pos, lk.Flag, depth, false) {
				return 1
			}
		case 8: // ChainContextPos
			if sh.applyChainContext(table, sub, pos, lk.Flag, depth, false) {
				return 1
			}
		}
	}
	return 0
}

func (sh *shaper) gposSingle(sub sfnt.Subtable, pos int) bool {
	covIdx, ok := sfnt.CoverageIndex(sub, sh.buf[pos].gid)
	if !ok {
		return false
	}
	d := sub.Data
	format := u16at(d, 0)
	valueFormat := u16at(d, 4)
	switch format {
	case 1:
		v, _ := readValueRecord(d, 6, valueFormat)
		sh.applyValue(pos, v)
		return true
	case 2:
		count := int(u16at(d, 6))
		if covIdx >= count {
			return false
		}
		vrSize := 0
		for _, bit := range []uint16{0x0001, 0x0002, 0x0004, 0x0008} {
			if valueFormat&bit != 0 {
				vrSize += 2
			}
		}
		v, _ := readValueRecord(d, 8+covIdx*vrSize, valueFormat)
		sh.applyValue(pos, v)
		return true
	}
	return false
}

func (sh *shaper) applyValue(pos int, v valueRecord) {
	b := &sh.buf[pos]
	b.xPlacement += v.xPlacement
	b.yPlacement += v.yPlacement
	b.xAdvance += v.xAdvance
	b.yAdvance += v.yAdvance
}

// gposPair applies PairPos at pos; returns the number of consumed glyphs
// (0 = no match, 2 = pair matched).
func (sh *shaper) gposPair(lk sfnt.Lookup, sub sfnt.Subtable, pos int) int {
	d := sub.Data
	if len(d) < 16 {
		return 0
	}
	format := u16at(d, 0)
	valueFormat1 := u16at(d, 4)
	valueFormat2 := u16at(d, 6)
	vr1Size, vr2Size := 0, 0
	for _, bit := range []uint16{0x0001, 0x0002, 0x0004, 0x0008} {
		if valueFormat1&bit != 0 {
			vr1Size += 2
		}
		if valueFormat2&bit != 0 {
			vr2Size += 2
		}
	}
	next := sh.nextMatch(pos+1, lk.Flag, lk.Flag>>8)
	if next < 0 {
		return 0
	}
	switch format {
	case 1:
		covIdx, ok := sfnt.CoverageIndex(sub, sh.buf[pos].gid)
		if !ok {
			return 0
		}
		setCount := int(u16at(d, 8))
		if covIdx >= setCount {
			return 0
		}
		psOff := int(u16at(d, 10+2*covIdx))
		if psOff+2 > len(d) {
			return 0
		}
		ps := d[psOff:]
		pvCount := int(u16at(ps, 0))
		for i := 0; i < pvCount; i++ {
			rec := 2 + i*(2+vr1Size+vr2Size)
			if rec+2+vr1Size+vr2Size > len(ps) {
				break
			}
			if u16at(ps, rec) != sh.buf[next].gid {
				continue
			}
			v1, _ := readValueRecord(ps, rec+2, valueFormat1)
			v2, _ := readValueRecord(ps, rec+2+vr1Size, valueFormat2)
			sh.applyValue(pos, v1)
			sh.applyValue(next, v2)
			sh.buf[pos].gposPaired = true
			return 2
		}
	case 2:
		classDef1 := int(u16at(d, 8))
		classDef2 := int(u16at(d, 10))
		class1Count := int(u16at(d, 12))
		class2Count := int(u16at(d, 14))
		ca := sfnt.ClassDefValue(d, classDef1, sh.buf[pos].gid)
		cb := sfnt.ClassDefValue(d, classDef2, sh.buf[next].gid)
		if ca >= class1Count || cb >= class2Count {
			return 0
		}
		entry := 16 + (ca*class2Count+cb)*(vr1Size+vr2Size)
		if entry+vr1Size+vr2Size > len(d) {
			return 0
		}
		v1, _ := readValueRecord(d, entry, valueFormat1)
		v2, _ := readValueRecord(d, entry+vr1Size, valueFormat2)
		sh.applyValue(pos, v1)
		sh.applyValue(next, v2)
		sh.buf[pos].gposPaired = true
		return 2
	}
	return 0
}

// anchor is one GPOS anchor point (design units, y up).
type anchor struct{ x, y int }

// parseAnchor reads an anchor table at d[off:] (formats 1-3; device
// correction in format 3 is ignored — no hinting).
func parseAnchor(d []byte, off int) (anchor, bool) {
	if off+6 > len(d) {
		return anchor{}, false
	}
	return anchor{x: i16at(d, off+2), y: i16at(d, off+4)}, true
}

// gposMarkBase applies MarkToBase attachment at pos.
func (sh *shaper) gposMarkBase(sub sfnt.Subtable, pos int) bool {
	d := sub.Data
	if len(d) < 12 {
		return false
	}
	markCovOff := int(u16at(d, 2))
	baseCovOff := int(u16at(d, 4))
	classCount := int(u16at(d, 6))
	markArrOff := int(u16at(d, 8))
	baseArrOff := int(u16at(d, 10))
	if classCount == 0 {
		return false
	}
	markCovIdx, ok := sfnt.CoverageIndexAt(d, markCovOff, sh.buf[pos].gid)
	if !ok {
		return false
	}
	// The base is the closest preceding non-mark glyph.
	base := -1
	for i := pos - 1; i >= 0; i-- {
		if isDefaultIgnorable(sh.buf[i].src) {
			continue
		}
		if sh.face.IsMarkGlyph(sh.buf[i].gid) {
			continue
		}
		base = i
		break
	}
	if base < 0 {
		return false
	}
	baseCovIdx, ok := sfnt.CoverageIndexAt(d, baseCovOff, sh.buf[base].gid)
	if !ok {
		return false
	}
	// MarkRecord: class + mark anchor offset (from the mark array).
	if markArrOff+2+4*markCovIdx+4 > len(d) {
		return false
	}
	markClass := int(u16at(d, markArrOff+2+4*markCovIdx))
	markAnchorOff := int(u16at(d, markArrOff+2+4*markCovIdx+2))
	if markClass >= classCount {
		return false
	}
	ma, ok := parseAnchor(d, markArrOff+markAnchorOff)
	if !ok {
		return false
	}
	// BaseRecord: classCount anchors (from the base array).
	if baseArrOff+2+baseCovIdx*2*classCount+2*markClass+2 > len(d) {
		return false
	}
	baseAnchorOff := int(u16at(d, baseArrOff+2+baseCovIdx*2*classCount+2*markClass))
	ba, ok := parseAnchor(d, baseArrOff+baseAnchorOff)
	if !ok {
		return false
	}
	// Position the mark so its anchor coincides with the base's anchor.
	sh.buf[pos].attached = true
	sh.buf[pos].baseIdx = base
	sh.buf[pos].attachDX = ba.x - ma.x
	sh.buf[pos].attachDY = ba.y - ma.y
	return true
}

// gposMarkMark applies MarkToMark attachment at pos (the base of the
// attachment is the closest preceding mark glyph).
func (sh *shaper) gposMarkMark(sub sfnt.Subtable, pos int) bool {
	d := sub.Data
	if len(d) < 12 {
		return false
	}
	mark1CovOff := int(u16at(d, 2))
	mark2CovOff := int(u16at(d, 4))
	classCount := int(u16at(d, 6))
	mark1ArrOff := int(u16at(d, 8))
	mark2ArrOff := int(u16at(d, 10))
	if classCount == 0 {
		return false
	}
	markCovIdx, ok := sfnt.CoverageIndexAt(d, mark1CovOff, sh.buf[pos].gid)
	if !ok {
		return false
	}
	// The attaching base is the closest preceding mark glyph.
	base := -1
	for i := pos - 1; i >= 0; i-- {
		if isDefaultIgnorable(sh.buf[i].src) {
			continue
		}
		if sh.face.IsMarkGlyph(sh.buf[i].gid) {
			base = i
			break
		}
	}
	if base < 0 {
		return false
	}
	baseCovIdx, ok := sfnt.CoverageIndexAt(d, mark2CovOff, sh.buf[base].gid)
	if !ok {
		return false
	}
	if mark1ArrOff+2+4*markCovIdx+4 > len(d) {
		return false
	}
	markClass := int(u16at(d, mark1ArrOff+2+4*markCovIdx))
	markAnchorOff := int(u16at(d, mark1ArrOff+2+4*markCovIdx+2))
	if markClass >= classCount {
		return false
	}
	ma, ok := parseAnchor(d, mark1ArrOff+markAnchorOff)
	if !ok {
		return false
	}
	if mark2ArrOff+2+baseCovIdx*2*classCount+2*markClass+2 > len(d) {
		return false
	}
	baseAnchorOff := int(u16at(d, mark2ArrOff+2+baseCovIdx*2*classCount+2*markClass))
	ba, ok := parseAnchor(d, mark2ArrOff+baseAnchorOff)
	if !ok {
		return false
	}
	sh.buf[pos].attached = true
	sh.buf[pos].baseIdx = base
	sh.buf[pos].attachDX = ba.x - ma.x
	sh.buf[pos].attachDY = ba.y - ma.y
	return true
}

// ---------------------------------------------------------------------------
// Joining forms (Arabic et al.)

type joiningForm uint8

const (
	formNone joiningForm = iota
	formIsol
	formInit
	formMedi
	formFina
)

func joiningFormForTag(tag string) joiningForm {
	switch tag {
	case "isol":
		return formIsol
	case "init":
		return formInit
	case "medi":
		return formMedi
	case "fina":
		return formFina
	}
	return formNone
}

// joining coverage sets, derived from the font's own single-substitution
// form lookups: a glyph in the 'init' feature's coverage can start a joined
// sequence (it has an initial form); a glyph in 'fina' coverage can end one.
// This self-calibrates per font — no separate Unicode joining table needed.
type joiningInfo struct {
	initCov map[uint16]bool
	finaCov map[uint16]bool
	loaded  bool
}

func (sh *shaper) joining() *joiningInfo {
	if sh.joiningData.loaded {
		return &sh.joiningData
	}
	sh.joiningData.loaded = true
	if sh.gsub != nil {
		sh.joiningData.initCov = sh.formCoverage("init")
		sh.joiningData.finaCov = sh.formCoverage("fina")
	}
	return &sh.joiningData
}

// formCoverage unions the type-1 subtable coverages of one feature.
func (sh *shaper) formCoverage(tag string) map[uint16]bool {
	var out map[uint16]bool
	for _, li := range sh.gsub.FeatureLookupsScript(tag, sh.script) {
		lk, ok := sh.gsub.LookupByIndex(li)
		if !ok {
			continue
		}
		for _, sub := range lk.Subtables {
			if sub.Type != 1 {
				continue
			}
			out = coverageGlyphs(sub, out)
		}
	}
	return out
}

// coverageGlyphs appends the glyphs of a subtable's coverage into m.
func coverageGlyphs(sub sfnt.Subtable, m map[uint16]bool) map[uint16]bool {
	off := int(sub.CoverageOffset)
	if off+4 > len(sub.Data) {
		return m
	}
	cov := sub.Data[off:]
	if m == nil {
		m = map[uint16]bool{}
	}
	switch u16at(cov, 0) {
	case 1:
		count := int(u16at(cov, 2))
		for i := 0; i < count; i++ {
			if 4+2*i+2 > len(cov) {
				break
			}
			m[u16at(cov, 4+2*i)] = true
		}
	case 2:
		n := int(u16at(cov, 2))
		for i := 0; i < n; i++ {
			o := 4 + 6*i
			if o+6 > len(cov) {
				break
			}
			start := u16at(cov, o)
			end := u16at(cov, o+2)
			for g := start; g <= end; g++ {
				m[g] = true
				if g == 0xFFFF {
					break
				}
			}
		}
	}
	return m
}

// isJoiningTransparent reports glyphs skipped when scanning joining
// neighbors (combining marks and default-ignorables).
func (sh *shaper) isJoiningTransparent(b bufGlyph) bool {
	if isDefaultIgnorable(b.src) {
		return true
	}
	if sh.face.IsMarkGlyph(b.gid) {
		return true
	}
	return isCombiningMark(b.src)
}

// joiningFormAt returns the joining form of the glyph at pos from the
// joining state snapshotted before substitution (joinPrev/joinNext on the
// buffer glyph).
func (sh *shaper) joiningFormAt(pos int) joiningForm {
	switch {
	case sh.buf[pos].joinPrev && sh.buf[pos].joinNext:
		return formMedi
	case sh.buf[pos].joinPrev:
		return formFina
	case sh.buf[pos].joinNext:
		return formInit
	default:
		return formIsol
	}
}

// joiningNeighbor scans for the nearest non-transparent neighbor in dir
// (+1/-1), or -1.
func (sh *shaper) joiningNeighbor(pos int, dir int) int {
	for i := pos + dir; i >= 0 && i < len(sh.buf); i += dir {
		if !sh.isJoiningTransparent(sh.buf[i]) {
			return i
		}
	}
	return -1
}

// gposMarkLig applies MarkToLigature attachment at pos: the mark attaches to
// an anchor of one component of the preceding ligature glyph (the component
// recorded during ligature substitution, upstream lig_comp).
func (sh *shaper) gposMarkLig(sub sfnt.Subtable, pos int) bool {
	d := sub.Data
	if len(d) < 12 {
		return false
	}
	markCovOff := int(u16at(d, 2))
	ligCovOff := int(u16at(d, 4))
	classCount := int(u16at(d, 6))
	markArrOff := int(u16at(d, 8))
	ligArrOff := int(u16at(d, 10))
	if classCount == 0 {
		return false
	}
	markCovIdx, ok := sfnt.CoverageIndexAt(d, markCovOff, sh.buf[pos].gid)
	if !ok {
		return false
	}
	// The ligature base is the closest preceding non-mark glyph.
	base := -1
	for i := pos - 1; i >= 0; i-- {
		if isDefaultIgnorable(sh.buf[i].src) {
			continue
		}
		if sh.face.IsMarkGlyph(sh.buf[i].gid) {
			continue
		}
		base = i
		break
	}
	if base < 0 {
		return false
	}
	ligCovIdx, ok := sfnt.CoverageIndexAt(d, ligCovOff, sh.buf[base].gid)
	if !ok {
		return false
	}
	if markArrOff+2+4*markCovIdx+4 > len(d) {
		return false
	}
	markClass := int(u16at(d, markArrOff+2+4*markCovIdx))
	markAnchorOff := int(u16at(d, markArrOff+2+4*markCovIdx+2))
	if markClass >= classCount {
		return false
	}
	ma, ok := parseAnchor(d, markArrOff+markAnchorOff)
	if !ok {
		return false
	}
	// LigatureArray: ligatureCount, then LigatureAttach offsets.
	if ligArrOff+2+2*ligCovIdx+2 > len(d) {
		return false
	}
	ligAttachOff := ligArrOff + int(u16at(d, ligArrOff+2+2*ligCovIdx))
	if ligAttachOff+2 > len(d) {
		return false
	}
	componentCount := int(u16at(d, ligAttachOff))
	if componentCount == 0 {
		return false
	}
	// The component the mark belongs to: the recorded lig_comp when the mark
	// was absorbed by this ligature, else the first component.
	component := 0
	if sh.buf[pos].ligID != 0 && sh.buf[pos].ligID == sh.buf[base].ligID && sh.buf[pos].ligComp >= 0 {
		component = sh.buf[pos].ligComp
	}
	if component >= componentCount {
		component = componentCount - 1
	}
	// ComponentRecord per component: classCount anchor offsets (from the
	// LigatureAttach start).
	anchorOff := ligAttachOff + 2 + (component*classCount+markClass)*2
	if anchorOff+2 > len(d) {
		return false
	}
	ba, ok := parseAnchor(d, ligAttachOff+int(u16at(d, anchorOff)))
	if !ok {
		return false
	}
	sh.buf[pos].attached = true
	sh.buf[pos].baseIdx = base
	sh.buf[pos].attachDX = ba.x - ma.x
	sh.buf[pos].attachDY = ba.y - ma.y
	return true
}

// fallbackMarkPositioning positions marks the font did not attach (no GPOS
// data matched) the way upstream's fallback shaper does: a zero-advance
// combining mark centers over its base (half the base advance), stacked on
// top of earlier marks on the same base.
func (sh *shaper) fallbackMarkPositioning() {
	for i := range sh.buf {
		b := &sh.buf[i]
		if b.attached || isDefaultIgnorable(b.src) {
			continue
		}
		// A mark candidate: GDEF class 3, or a combining rune when the font
		// has no GDEF; it must take no horizontal space.
		isMark := sh.face.IsMarkGlyph(b.gid) || (sh.face.GDEF == nil && isCombiningMark(b.src))
		if !isMark {
			continue
		}
		if sh.face.Advance(b.gid) != 0 || b.xAdvance != 0 || b.xPlacement != 0 {
			continue
		}
		// Find the preceding base: the closest non-mark, non-ignorable glyph.
		base := -1
		for j := i - 1; j >= 0; j-- {
			if isDefaultIgnorable(sh.buf[j].src) {
				continue
			}
			if sh.face.IsMarkGlyph(sh.buf[j].gid) || (sh.face.GDEF == nil && isCombiningMark(sh.buf[j].src)) {
				continue
			}
			base = j
			break
		}
		if base < 0 {
			continue
		}
		// Center over the base; stack over previously attached marks
		// (upstream's fallback stacking keeps repeated marks apart).
		stack := 0
		for j := i - 1; j >= 0; j-- {
			if sh.buf[j].attached && sh.buf[j].baseIdx == base {
				stack++
			}
		}
		b.attached = true
		b.baseIdx = base
		b.attachDX = int(sh.face.Advance(sh.buf[base].gid)) / 2
		b.attachDY = 0
		if stack > 0 {
			b.attachDY = 350 * stack
		}
	}
}

// applyLegacyKern applies the legacy 'kern' table to pairs the GPOS kern
// feature did not position (fonts keep Latin kerning in the old table).
func (sh *shaper) applyLegacyKern() {
	if sh.gpos != nil && len(sh.gpos.FeatureLookups("kern")) > 0 {
		// GPOS kern handled what it covered; only fill the gaps.
		for i := 0; i+1 < len(sh.buf); i++ {
			if sh.buf[i].gposPaired || sh.buf[i].attached || sh.buf[i+1].attached {
				continue
			}
			if k := sh.face.Kern(sh.buf[i].gid, sh.buf[i+1].gid); k != 0 {
				sh.buf[i].xAdvance += int(k)
			}
		}
		return
	}
	for i := 0; i+1 < len(sh.buf); i++ {
		if sh.buf[i].attached || sh.buf[i+1].attached {
			continue
		}
		if k := sh.face.Kern(sh.buf[i].gid, sh.buf[i+1].gid); k != 0 {
			sh.buf[i].xAdvance += int(k)
		}
	}
}
