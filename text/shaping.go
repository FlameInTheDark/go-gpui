package text

import (
	"encoding/binary"

	"github.com/FlameInTheDark/go-gpui/text/sfnt"
)

// This file implements the OpenType layout engine that drives shaping:
// GSUB substitution (single, multiple, alternate, ligature, context,
// chained context — extension wrappers are unwrapped at parse time) and GPOS
// positioning (single, pair, mark-to-base, mark-to-mark, context, chained
// context), with lookup-flag aware matching (ignore base/ligature/mark,
// mark attachment types, mark filtering sets).
//
// The engine operates on a glyph buffer (HarfBuzz-style): each glyph carries
// its source cluster and rune, accumulated GPOS adjustments, and — once a
// mark-to-base/mark-to-mark lookup matched — an attachment to its base glyph
// (anchor deltas in font design units).

// maxLookupNesting bounds contextual lookup recursion (upstream shapers cap
// nesting depth at 6).
const maxLookupNesting = 6

// bufGlyph is one glyph in the shaping buffer.
type bufGlyph struct {
	gid     uint16
	cluster int
	src     rune
	// GPOS accumulated adjustments in design units.
	xPlacement, yPlacement int
	xAdvance, yAdvance     int
	// Attachment set by mark positioning: offset (design units, y up) from
	// the base glyph's origin; baseIdx indexes the buffer (-1 unattached).
	attached bool
	baseIdx  int
	attachDX int
	attachDY int
	// Ligature bookkeeping (upstream lig_id/lig_comp): marks absorbed by a
	// ligature substitution remember which component they belong to, so
	// MarkToLigature positioning can pick the right component anchor.
	ligID   int
	ligComp int
	// Joining state (Arabic et al.), snapshotted from the pre-substitution
	// glyph identities before GSUB runs: substitution swaps glyph ids, so
	// re-deriving joining from the mutated buffer would break mid-word forms.
	joinPrev bool
	joinNext bool
	// gposPaired records that a GPOS PairPos lookup positioned this glyph
	// as the left side of a pair (suppresses the legacy 'kern' fallback).
	gposPaired bool
}

// shaper shapes one same-font span.
type shaper struct {
	face *sfnt.Font
	gsub *sfnt.LayoutTable
	gpos *sfnt.LayoutTable
	// script is the span's OpenType script tag (rune-derived); feature
	// records are selected per script like upstream shapers.
	script string
	buf    []bufGlyph
	// joiningData caches the font-derived joining coverage sets.
	joiningData joiningInfo
	// ligCounter generates unique ligature ids.
	ligCounter int
}

func newShaper(face *sfnt.Font) *shaper {
	return &shaper{face: face, gsub: face.GSUB, gpos: face.GPOS}
}

// gsubFeatureOrder is the application order of supported GSUB features
// (following the recommended order: decomposition first, then required
// ligatures, joining forms, then contextual/discretionary ligatures).
var gsubFeatureOrder = []string{
	"ccmp", "locl", "rlig", "isol", "init", "medi", "fina", "calt", "clig", "liga",
}

// gposFeatureOrder is the application order of supported GPOS features.
var gposFeatureOrder = []string{"kern", "mark", "mkmk"}

// shape runs the full GSUB + GPOS pipeline over the buffer.
func (sh *shaper) shape() {
	if sh.gsub != nil {
		// Snapshot the joining state from the base glyph identities before
		// any substitution (upstream computes Arabic joining from characters
		// before shaping; substituted form glyphs are not in the form
		// coverages, so joining must not be re-derived mid-pipeline).
		sh.snapshotJoining()
		for _, tag := range gsubFeatureOrder {
			if len(sh.gsub.FeatureLookups(tag)) > 0 {
				sh.applyGSUBFeature(tag)
			}
		}
	}
	if sh.gpos != nil {
		for _, tag := range gposFeatureOrder {
			if len(sh.gpos.FeatureLookups(tag)) > 0 {
				sh.applyGPOSFeature(tag)
			}
		}
		// Fonts whose GPOS kern feature leaves pairs unpositioned still
		// carry the legacy 'kern' table.
		sh.applyLegacyKern()
	}
	// Fonts without mark-attachment data for every mark still need their
	// combining marks over their bases (upstream fallback positioning).
	sh.fallbackMarkPositioning()
}

// snapshotJoining records, per glyph, whether it joins with its neighbors
// (dual/right/left-joining letters take positional forms; the flags survive
// substitutions like a font's own chain-context rules would).
func (sh *shaper) snapshotJoining() {
	j := sh.joining()
	if j.initCov == nil && j.finaCov == nil {
		return
	}
	for i := range sh.buf {
		cur := sh.buf[i].gid
		if j.finaCov[cur] {
			if p := sh.joiningNeighbor(i, -1); p >= 0 && j.initCov[sh.buf[p].gid] {
				sh.buf[i].joinPrev = true
			}
		}
		if j.initCov[cur] {
			if n := sh.joiningNeighbor(i, 1); n >= 0 && j.finaCov[sh.buf[n].gid] {
				sh.buf[i].joinNext = true
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Skipping (lookup flags)

func isDefaultIgnorable(r rune) bool {
	switch {
	case r == 0x200B || r == 0x200C || r == 0x200D, // ZWSP ZWNJ ZWJ
		r == 0x200E || r == 0x200F, // LRM RLM
		r >= 0x202A && r <= 0x202E, // bidi embeddings
		r >= 0x2060 && r <= 0x2064, // word joiner / invisible ops
		r >= 0x2066 && r <= 0x2069, // bidi isolates
		r >= 0xFE00 && r <= 0xFE0F, // variation selectors
		r == 0xFEFF:                // BOM / zero width no-break
		return true
	}
	return false
}

// skippable reports whether a glyph is skipped by a lookup's matching rules
// (ignore flags, mark attachment type, mark filtering set). Note that
// default-ignorable code points participate in matching like any other
// glyph (ZWJ is a ligature component in emoji fonts); they are dropped from
// the output after shaping instead.
func (sh *shaper) skippable(b bufGlyph, flag uint16, filterSet uint16) bool {
	if flag == 0 {
		return false
	}
	cls := sh.face.GlyphClass(b.gid)
	if flag&0x0001 != 0 && cls == sfnt.ClassBase {
		return true
	}
	if flag&0x0002 != 0 && cls == sfnt.ClassLigature {
		return true
	}
	if cls == sfnt.ClassMark {
		if flag&0x0004 != 0 {
			return true
		}
		if flag&0xFF00 != 0 && sh.face.MarkAttachClass(b.gid) != int(flag>>8) {
			return true
		}
		if flag&0x0010 != 0 && !sh.face.InMarkFilterSet(filterSet, b.gid) {
			return true
		}
	}
	return false
}

// nextMatch returns the next buffer index >= from that participates in
// matching under the lookup flag, or -1.
func (sh *shaper) nextMatch(from int, flag uint16, filterSet uint16) int {
	for i := from; i < len(sh.buf); i++ {
		if !sh.skippable(sh.buf[i], flag, filterSet) {
			return i
		}
	}
	return -1
}

// prevMatch returns the previous buffer index <= from that participates in
// matching under the lookup flag, or -1.
func (sh *shaper) prevMatch(from int, flag uint16, filterSet uint16) int {
	for i := from; i >= 0; i-- {
		if !sh.skippable(sh.buf[i], flag, filterSet) {
			return i
		}
	}
	return -1
}

func u16at(d []byte, off int) uint16 {
	if off+2 > len(d) {
		return 0
	}
	return binary.BigEndian.Uint16(d[off : off+2])
}

func i16at(d []byte, off int) int {
	if off+2 > len(d) {
		return 0
	}
	return int(int16(binary.BigEndian.Uint16(d[off : off+2])))
}

func u32at(d []byte, off int) uint32 {
	if off+4 > len(d) {
		return 0
	}
	return binary.BigEndian.Uint32(d[off : off+4])
}

// ---------------------------------------------------------------------------
// GSUB

// applyGSUBFeature applies one feature's lookups across the buffer.
func (sh *shaper) applyGSUBFeature(tag string) {
	lookups := sh.gsub.FeatureLookups(tag)
	form := joiningFormForTag(tag)
	for pos := 0; pos < len(sh.buf); {
		advanced := false
		for _, li := range lookups {
			lk, ok := sh.gsub.LookupByIndex(li)
			if !ok || len(lk.Subtables) == 0 {
				continue
			}
			// Joining-form features with plain single-substitution lookups
			// apply only to glyphs whose computed joining form matches
			// (contextual lookups gate themselves).
			if form != formNone && allType1(lk) && sh.joiningFormAt(pos) != form {
				continue
			}
			np, applied := sh.applyGSUBLookup(li, pos, 0)
			if applied {
				pos = np
				advanced = true
				break
			}
		}
		if !advanced {
			pos++
		}
	}
}

func allType1(lk sfnt.Lookup) bool {
	for _, sub := range lk.Subtables {
		if sub.Type != 1 {
			return false
		}
	}
	return true
}

// applyGSUBLookup applies one lookup at a buffer position. Returns the next
// position and whether anything matched.
func (sh *shaper) applyGSUBLookup(li uint16, pos int, depth int) (int, bool) {
	if depth > maxLookupNesting || pos < 0 || pos >= len(sh.buf) {
		return pos, false
	}
	lk, ok := sh.gsub.LookupByIndex(li)
	if !ok {
		return pos, false
	}
	for _, sub := range lk.Subtables {
		switch sub.Type {
		case 1:
			if np, ok := sh.gsubSingle(sub, pos); ok {
				return np, true
			}
		case 2:
			if np, ok := sh.gsubMultiple(sub, pos); ok {
				return np, true
			}
		case 3:
			if np, ok := sh.gsubAlternate(sub, pos); ok {
				return np, true
			}
		case 4:
			if np, ok := sh.gsubLigature(lk, sub, pos); ok {
				return np, true
			}
		case 5:
			if ok := sh.applyContext(sh.gsub, sub, pos, lk.Flag, depth, true); ok {
				return pos + 1, true
			}
		case 6:
			if ok := sh.applyChainContext(sh.gsub, sub, pos, lk.Flag, depth, true); ok {
				return pos + 1, true
			}
		}
	}
	return pos, false
}

func (sh *shaper) gsubSingle(sub sfnt.Subtable, pos int) (int, bool) {
	gid := sh.buf[pos].gid
	covIdx, ok := sfnt.CoverageIndex(sub, gid)
	if !ok {
		return pos, false
	}
	format := u16at(sub.Data, 0)
	switch format {
	case 1:
		delta := u16at(sub.Data, 4)
		sh.buf[pos].gid = gid + delta
		return pos + 1, true
	case 2:
		count := int(u16at(sub.Data, 4))
		if covIdx < count {
			sh.buf[pos].gid = u16at(sub.Data, 6+2*covIdx)
			return pos + 1, true
		}
	}
	return pos, false
}

func (sh *shaper) gsubMultiple(sub sfnt.Subtable, pos int) (int, bool) {
	covIdx, ok := sfnt.CoverageIndex(sub, sh.buf[pos].gid)
	if !ok {
		return pos, false
	}
	count := int(u16at(sub.Data, 4))
	if covIdx >= count {
		return pos, false
	}
	seqOff := int(u16at(sub.Data, 6+2*covIdx))
	if seqOff+2 > len(sub.Data) {
		return pos, false
	}
	seq := sub.Data[seqOff:]
	n := int(u16at(seq, 0))
	if n == 0 {
		// Empty substitution deletes the glyph (legal per the spec).
		sh.buf = append(sh.buf[:pos], sh.buf[pos+1:]...)
		return pos, true
	}
	if n == 1 {
		sh.buf[pos].gid = u16at(seq, 2)
		return pos + 1, true
	}
	// One-to-many: the outputs share the input's cluster.
	out := make([]bufGlyph, 0, len(sh.buf)+n-1)
	out = append(out, sh.buf[:pos]...)
	for i := 0; i < n; i++ {
		g := sh.buf[pos]
		g.gid = u16at(seq, 2+2*i)
		out = append(out, g)
	}
	out = append(out, sh.buf[pos+1:]...)
	sh.buf = out
	return pos + n, true
}

func (sh *shaper) gsubAlternate(sub sfnt.Subtable, pos int) (int, bool) {
	covIdx, ok := sfnt.CoverageIndex(sub, sh.buf[pos].gid)
	if !ok {
		return pos, false
	}
	count := int(u16at(sub.Data, 4))
	if covIdx >= count {
		return pos, false
	}
	setOff := int(u16at(sub.Data, 6+2*covIdx))
	if setOff+2 > len(sub.Data) {
		return pos, false
	}
	set := sub.Data[setOff:]
	n := int(u16at(set, 0))
	if n == 0 {
		return pos, false
	}
	// The first alternate is the default choice.
	sh.buf[pos].gid = u16at(set, 2)
	return pos + 1, true
}

func (sh *shaper) gsubLigature(lk sfnt.Lookup, sub sfnt.Subtable, pos int) (int, bool) {
	covIdx, ok := sfnt.CoverageIndex(sub, sh.buf[pos].gid)
	if !ok {
		return pos, false
	}
	setCount := int(u16at(sub.Data, 4))
	if covIdx >= setCount {
		return pos, false
	}
	setOff := int(u16at(sub.Data, 6+2*covIdx))
	if setOff+2 > len(sub.Data) {
		return pos, false
	}
	set := sub.Data[setOff:]
	ligCount := int(u16at(set, 0))
	for j := 0; j < ligCount; j++ {
		ligOff := int(u16at(set, 2+2*j))
		if ligOff+4 > len(set) {
			break
		}
		lig := set[ligOff:]
		ligGlyph := u16at(lig, 0)
		compCount := int(u16at(lig, 2))
		if compCount < 1 || compCount > 8 {
			continue
		}
		// Match components 1..compCount-1 after pos, honoring the lookup
		// flag's skip rules. Marks skipped between components join the
		// component they follow (upstream lig_id/lig_comp), so a later
		// MarkToLigature lookup positions them on the right component.
		idx := pos
		matched := true
		ligID := -1
		for c := 1; c < compCount; c++ {
			prev := idx
			idx = sh.nextMatch(idx+1, lk.Flag, lk.Flag>>8)
			if idx < 0 || sh.buf[idx].gid != u16at(lig, 2+2*c) {
				matched = false
				break
			}
			if ligID < 0 {
				sh.ligCounter++
				ligID = sh.ligCounter
			}
			for m := prev + 1; m < idx; m++ {
				if sh.face.IsMarkGlyph(sh.buf[m].gid) {
					sh.buf[m].ligID = ligID
					sh.buf[m].ligComp = c - 1
				}
			}
		}
		if !matched {
			continue
		}
		// Substitute: the ligature glyph keeps the first component's cluster;
		// consumed components merge into it.
		sh.buf[pos].gid = ligGlyph
		if ligID < 0 {
			sh.ligCounter++
			ligID = sh.ligCounter
		}
		sh.buf[pos].ligID = ligID
		sh.buf[pos].ligComp = -1
		for k := idx; k > pos; k-- {
			sh.buf = append(sh.buf[:k], sh.buf[k+1:]...)
		}
		return pos + 1, true
	}
	return pos, false
}

// ---------------------------------------------------------------------------
// Contextual lookups (shared matcher for GSUB and GPOS)

type lookupRecord struct {
	seq, lookup uint16
}

func parseLookupRecords(d []byte, off int, count int) []lookupRecord {
	out := make([]lookupRecord, 0, count)
	for i := 0; i < count; i++ {
		if off+4*i+4 > len(d) {
			break
		}
		out = append(out, lookupRecord{seq: u16at(d, off+4*i), lookup: u16at(d, off+4*i+2)})
	}
	return out
}

// applyNestedLookup applies a nested lookup by index for the given table.
func (sh *shaper) applyNestedLookup(table *sfnt.LayoutTable, li uint16, pos int, depth int, gsub bool) bool {
	if depth > maxLookupNesting || pos < 0 || pos >= len(sh.buf) {
		return false
	}
	if _, ok := table.LookupByIndex(li); !ok {
		return false
	}
	if gsub {
		_, applied := sh.applyGSUBLookup(li, pos, depth+1)
		return applied
	}
	return sh.applyGPOSLookup(table, li, pos, depth+1) > 0
}

// applyRecords applies nested lookups at sequence offsets from pos.
func (sh *shaper) applyRecords(table *sfnt.LayoutTable, records []lookupRecord, pos int, depth int, gsub bool) bool {
	applied := false
	for _, rec := range records {
		at := pos + int(rec.seq)
		if at >= 0 && at < len(sh.buf) {
			if sh.applyNestedLookup(table, rec.lookup, at, depth+1, gsub) {
				applied = true
			}
		}
	}
	return applied
}

// applyContext applies a Context lookup (format 1/2/3) at pos.
func (sh *shaper) applyContext(table *sfnt.LayoutTable, sub sfnt.Subtable, pos int, flag uint16, depth int, gsub bool) bool {
	d := sub.Data
	if len(d) < 4 {
		return false
	}
	format := u16at(d, 0)
	switch format {
	case 1:
		covIdx, ok := sfnt.CoverageIndex(sub, sh.buf[pos].gid)
		if !ok {
			return false
		}
		ruleSetCount := int(u16at(d, 4))
		if covIdx >= ruleSetCount {
			return false
		}
		rsOff := int(u16at(d, 6+2*covIdx))
		if rsOff+2 > len(d) {
			return false
		}
		rs := d[rsOff:]
		ruleCount := int(u16at(rs, 0))
		for r := 0; r < ruleCount; r++ {
			ruleOff := int(u16at(rs, 2+2*r))
			if ruleOff+4 > len(rs) {
				break
			}
			rule := rs[ruleOff:]
			glyphCount := int(u16at(rule, 0))
			if glyphCount < 1 {
				continue
			}
			recCount := int(u16at(rule, 2))
			if sh.matchGlyphSequence(rule, 4, glyphCount-1, pos, flag) {
				records := parseLookupRecords(rule, 4+2*(glyphCount-1), recCount)
				return sh.applyRecords(table, records, pos, depth, gsub)
			}
		}
	case 2:
		// The class of the first glyph selects the rule set.
		classDefOff := int(u16at(d, 4))
		cls := sfnt.ClassDefValue(d, classDefOff, sh.buf[pos].gid)
		setCount := int(u16at(d, 6))
		if cls >= setCount {
			return false
		}
		setOff := int(u16at(d, 8+2*cls))
		if setOff+2 > len(d) {
			return false
		}
		set := d[setOff:]
		ruleCount := int(u16at(set, 0))
		for r := 0; r < ruleCount; r++ {
			ruleOff := int(u16at(set, 2+2*r))
			if ruleOff+4 > len(set) {
				break
			}
			rule := set[ruleOff:]
			glyphCount := int(u16at(rule, 0))
			if glyphCount < 1 {
				continue
			}
			recCount := int(u16at(rule, 2))
			if sh.matchClassSequence(d, classDefOff, rule, 4, glyphCount-1, pos, flag) {
				records := parseLookupRecords(rule, 4+2*(glyphCount-1), recCount)
				return sh.applyRecords(table, records, pos, depth, gsub)
			}
		}
	case 3:
		glyphCount := int(u16at(d, 2))
		recCount := int(u16at(d, 4))
		if glyphCount < 1 {
			return false
		}
		if !sfnt.CoverageContainsAt(d, int(u16at(d, 6)), sh.buf[pos].gid) {
			return false
		}
		idx := pos
		for g := 1; g < glyphCount; g++ {
			idx = sh.nextMatch(idx+1, flag, flag>>8)
			if idx < 0 {
				return false
			}
			if !sfnt.CoverageContainsAt(d, int(u16at(d, 6+2*g)), sh.buf[idx].gid) {
				return false
			}
		}
		records := parseLookupRecords(d, 6+2*glyphCount, recCount)
		return sh.applyRecords(table, records, pos, depth, gsub)
	}
	return false
}

// applyChainContext applies a ChainContext lookup (format 1/2/3) at pos.
func (sh *shaper) applyChainContext(table *sfnt.LayoutTable, sub sfnt.Subtable, pos int, flag uint16, depth int, gsub bool) bool {
	d := sub.Data
	if len(d) < 4 {
		return false
	}
	format := u16at(d, 0)
	switch format {
	case 1:
		covIdx, ok := sfnt.CoverageIndex(sub, sh.buf[pos].gid)
		if !ok {
			return false
		}
		setCount := int(u16at(d, 4))
		if covIdx >= setCount {
			return false
		}
		rsOff := int(u16at(d, 6+2*covIdx))
		if rsOff+2 > len(d) {
			return false
		}
		rs := d[rsOff:]
		ruleCount := int(u16at(rs, 0))
		for r := 0; r < ruleCount; r++ {
			ruleOff := int(u16at(rs, 2+2*r))
			if ruleOff+2 > len(rs) {
				break
			}
			rule := rs[ruleOff:]
			// Parse the chain rule.
			off := 0
			backCount := int(u16at(rule, off))
			off += 2
			var backtrack []uint16
			for i := 0; i < backCount; i++ {
				backtrack = append(backtrack, u16at(rule, off+2*i))
			}
			off += 2 * backCount
			inCount := int(u16at(rule, off))
			off += 2
			if inCount < 1 {
				continue
			}
			var input []uint16
			for i := 0; i < inCount-1; i++ {
				input = append(input, u16at(rule, off+2*i))
			}
			off += 2 * (inCount - 1)
			lookCount := int(u16at(rule, off))
			off += 2
			var lookahead []uint16
			for i := 0; i < lookCount; i++ {
				lookahead = append(lookahead, u16at(rule, off+2*i))
			}
			off += 2 * lookCount
			recCount := int(u16at(rule, off))
			off += 2
			if sh.matchChain(backtrack, input, lookahead, pos, flag) {
				records := parseLookupRecords(rule, off, recCount)
				return sh.applyRecords(table, records, pos, depth, gsub)
			}
		}
	case 2:
		// Backtrack/input/lookahead are class sequences; the input class of
		// the first glyph selects the rule set.
		backClassDef := int(u16at(d, 4))
		inClassDef := int(u16at(d, 6))
		lookClassDef := int(u16at(d, 8))
		cls := sfnt.ClassDefValue(d, inClassDef, sh.buf[pos].gid)
		setCount := int(u16at(d, 10))
		if cls >= setCount {
			return false
		}
		setOff := int(u16at(d, 12+2*cls))
		if setOff+2 > len(d) {
			return false
		}
		set := d[setOff:]
		ruleCount := int(u16at(set, 0))
		for r := 0; r < ruleCount; r++ {
			ruleOff := int(u16at(set, 2+2*r))
			if ruleOff+2 > len(set) {
				break
			}
			rule := set[ruleOff:]
			off := 0
			backCount := int(u16at(rule, off))
			off += 2
			var backtrack []uint16
			for i := 0; i < backCount; i++ {
				backtrack = append(backtrack, u16at(rule, off+2*i))
			}
			off += 2 * backCount
			inCount := int(u16at(rule, off))
			off += 2
			if inCount < 1 {
				continue
			}
			var input []uint16
			for i := 0; i < inCount-1; i++ {
				input = append(input, u16at(rule, off+2*i))
			}
			off += 2 * (inCount - 1)
			lookCount := int(u16at(rule, off))
			off += 2
			var lookahead []uint16
			for i := 0; i < lookCount; i++ {
				lookahead = append(lookahead, u16at(rule, off+2*i))
			}
			off += 2 * lookCount
			recCount := int(u16at(rule, off))
			off += 2
			if sh.matchChainClasses(d, backtrack, input, lookahead, backClassDef, inClassDef, lookClassDef, pos, flag) {
				records := parseLookupRecords(rule, off, recCount)
				return sh.applyRecords(table, records, pos, depth, gsub)
			}
		}
	case 3:
		// Coverage arrays: backtrack (logical order, last is nearest pos),
		// input, lookahead.
		backCount := int(u16at(d, 2))
		idx := pos
		for b := backCount; b >= 1; b-- {
			idx = sh.prevMatch(idx-1, flag, flag>>8)
			if idx < 0 {
				return false
			}
			if !sfnt.CoverageContainsAt(d, int(u16at(d, 4+2*(b-1))), sh.buf[idx].gid) {
				return false
			}
		}
		o := 4 + 2*backCount
		inCount := int(u16at(d, o))
		if inCount < 1 {
			return false
		}
		if !sfnt.CoverageContainsAt(d, int(u16at(d, o+2)), sh.buf[pos].gid) {
			return false
		}
		idx = pos
		for g := 1; g < inCount; g++ {
			idx = sh.nextMatch(idx+1, flag, flag>>8)
			if idx < 0 {
				return false
			}
			if !sfnt.CoverageContainsAt(d, int(u16at(d, o+2+2*g)), sh.buf[idx].gid) {
				return false
			}
		}
		o2 := o + 2 + 2*inCount
		lookCount := int(u16at(d, o2))
		for l := 0; l < lookCount; l++ {
			idx = sh.nextMatch(idx+1, flag, flag>>8)
			if idx < 0 {
				return false
			}
			if !sfnt.CoverageContainsAt(d, int(u16at(d, o2+2+2*l)), sh.buf[idx].gid) {
				return false
			}
		}
		recCount := int(u16at(d, o2+2+2*lookCount))
		records := parseLookupRecords(d, o2+2+2*lookCount+2, recCount)
		return sh.applyRecords(table, records, pos, depth, gsub)
	}
	return false
}

// matchGlyphSequence matches input glyph ids at rule[seqOff:] after pos.
func (sh *shaper) matchGlyphSequence(rule []byte, seqOff int, count int, pos int, flag uint16) bool {
	idx := pos
	for c := 0; c < count; c++ {
		idx = sh.nextMatch(idx+1, flag, flag>>8)
		if idx < 0 || idx >= len(sh.buf) || sh.buf[idx].gid != u16at(rule, seqOff+2*c) {
			return false
		}
	}
	return true
}

// matchClassSequence matches input classes at rule[seqOff:] after pos.
// Class 0 in a rule matches glyphs whose class is 0 (not in the class def).
func (sh *shaper) matchClassSequence(d []byte, classDefOff int, rule []byte, seqOff int, count int, pos int, flag uint16) bool {
	idx := pos
	for c := 0; c < count; c++ {
		idx = sh.nextMatch(idx+1, flag, flag>>8)
		if idx < 0 || idx >= len(sh.buf) {
			return false
		}
		if sfnt.ClassDefValue(d, classDefOff, sh.buf[idx].gid) != int(u16at(rule, seqOff+2*c)) {
			return false
		}
	}
	return true
}

// matchChain matches glyph-sequence backtrack (walking left from pos, in
// reverse), input (after pos) and lookahead sequences.
func (sh *shaper) matchChain(backtrack, input, lookahead []uint16, pos int, flag uint16) bool {
	idx := pos
	for b := len(backtrack); b >= 1; b-- {
		idx = sh.prevMatch(idx-1, flag, flag>>8)
		if idx < 0 || sh.buf[idx].gid != backtrack[b-1] {
			return false
		}
	}
	idx = pos
	for c := 0; c < len(input); c++ {
		idx = sh.nextMatch(idx+1, flag, flag>>8)
		if idx < 0 || idx >= len(sh.buf) || sh.buf[idx].gid != input[c] {
			return false
		}
	}
	for l := 0; l < len(lookahead); l++ {
		idx = sh.nextMatch(idx+1, flag, flag>>8)
		if idx < 0 || idx >= len(sh.buf) || sh.buf[idx].gid != lookahead[l] {
			return false
		}
	}
	return true
}

// matchChainClasses matches class-sequence backtrack/input/lookahead.
func (sh *shaper) matchChainClasses(d []byte, backtrack, input, lookahead []uint16, backClassDef, inClassDef, lookClassDef int, pos int, flag uint16) bool {
	idx := pos
	for b := len(backtrack); b >= 1; b-- {
		idx = sh.prevMatch(idx-1, flag, flag>>8)
		if idx < 0 || sfnt.ClassDefValue(d, backClassDef, sh.buf[idx].gid) != int(backtrack[b-1]) {
			return false
		}
	}
	idx = pos
	for c := 0; c < len(input); c++ {
		idx = sh.nextMatch(idx+1, flag, flag>>8)
		if idx < 0 || idx >= len(sh.buf) {
			return false
		}
		if sfnt.ClassDefValue(d, inClassDef, sh.buf[idx].gid) != int(input[c]) {
			return false
		}
	}
	for l := 0; l < len(lookahead); l++ {
		idx = sh.nextMatch(idx+1, flag, flag>>8)
		if idx < 0 || idx >= len(sh.buf) {
			return false
		}
		if sfnt.ClassDefValue(d, lookClassDef, sh.buf[idx].gid) != int(lookahead[l]) {
			return false
		}
	}
	return true
}
