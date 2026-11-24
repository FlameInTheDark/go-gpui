package text

import (
	"strings"
	"unicode"
)

// Bidirectional text support: a compact implementation of UAX #9
// (https://www.unicode.org/reports/tr9/), the pure-Go replacement for the
// BiDi reordering cosmic-text performs inside upstream's shaper.
//
// Coverage: the paragraph/embedding level is fixed by the base direction
// (rules X1-X8 collapse to the paragraph level; explicit formatting
// characters LRE/RLE/LRO/RLO/PDF/LRI/RLI/FSI/PDI are treated as boundary
// neutrals per the "retaining explicit formatting characters" notes and are
// omitted from the output), then rules W1-W7, N1-N2, I1-I2 and L1-L2 run
// exactly as specified. Two documented simplifications:
//
//   - Rule N0 (bracket pairs) is not implemented; paired brackets stay ON
//     and resolve through N1-N2 like other neutrals.
//   - Directional isolates (LRI/RLI/FSI/PDI) do not start isolating run
//     sequences; the whole line is one sequence (BD13 is collapsed). This
//     only diverges from the spec around isolate characters, which are
//     zero-width and rare in UI text.
//
// Class data (AL/AN/R/EN/ES/ET/CS ranges) mirrors the Unicode Character
// Database; other classes derive from the standard unicode package tables.

// Segment is one visually-ordered directional run of a line. Segments are
// returned in VISUAL order (left to right); Start/End are UTF-8 byte offsets
// into the logical line text, and RTL marks segments whose glyphs render
// right-to-left (so their logical order inside the range is reversed).
type Segment struct {
	Start, End int // UTF-8 byte offsets in the source line
	RTL        bool
}

// bidiClass is a UAX #9 Bidi_Class value.
type bidiClass uint8

const (
	bidiL          bidiClass = iota // Left-to-right strong
	bidiR                           // Right-to-left strong
	bidiAL                          // Arabic letter (strong RTL after W3)
	bidiEN                          // European number
	bidiAN                          // Arabic number
	bidiES                          // European separator
	bidiET                          // European terminator
	bidiCS                          // Common separator
	bidiNSM                         // Non-spacing mark
	bidiBN                          // Boundary neutral (controls, ZWJ, ...)
	bidiWS                          // Whitespace
	bidiS                           // Segment separator (TAB and friends)
	bidiB                           // Paragraph separator
	bidiON                          // Other neutral
	bidiFormatting                  // Explicit formatting char (LRE/RLE/LRO/RLO/PDF/LRI/RLI/FSI/PDI)
)

// bidiRange is an inclusive codepoint range for the class tables below.
type bidiRange struct{ lo, hi rune }

func bidiInRanges(ranges []bidiRange, r rune) bool {
	lo, hi := 0, len(ranges)-1
	for lo <= hi {
		mid := (lo + hi) / 2
		switch {
		case r < ranges[mid].lo:
			hi = mid - 1
		case r > ranges[mid].hi:
			lo = mid + 1
		default:
			return true
		}
	}
	return false
}

// bidiALRanges: exact UCD Bidi_Class=AL ranges.
var bidiALRanges = []bidiRange{
	{0x0608, 0x0608}, {0x060B, 0x060B}, {0x060D, 0x060D}, {0x061B, 0x064A}, {0x066D, 0x066F}, {0x0671, 0x06D5}, {0x06E5, 0x06E6}, {0x06EE, 0x06EF}, {0x06FA, 0x070D}, {0x070F, 0x0710}, {0x0712, 0x072F}, {0x074D, 0x07A5}, {0x07B1, 0x07B1}, {0x0860, 0x086A}, {0x0870, 0x088E}, {0x08A0, 0x08C9}, {0xFB50, 0xFBC2}, {0xFBD3, 0xFD3D}, {0xFD50, 0xFD8F}, {0xFD92, 0xFDC7}, {0xFDF0, 0xFDFC}, {0xFE70, 0xFE74}, {0xFE76, 0xFEFC}, {0x10D00, 0x10D23}, {0x10F30, 0x10F45}, {0x10F51, 0x10F59}, {0x1EC71, 0x1ECB4}, {0x1ED01, 0x1ED3D}, {0x1EE00, 0x1EE03}, {0x1EE05, 0x1EE1F}, {0x1EE21, 0x1EE22}, {0x1EE24, 0x1EE24}, {0x1EE27, 0x1EE27}, {0x1EE29, 0x1EE32}, {0x1EE34, 0x1EE37}, {0x1EE39, 0x1EE39}, {0x1EE3B, 0x1EE3B}, {0x1EE42, 0x1EE42}, {0x1EE47, 0x1EE47}, {0x1EE49, 0x1EE49}, {0x1EE4B, 0x1EE4B}, {0x1EE4D, 0x1EE4F}, {0x1EE51, 0x1EE52}, {0x1EE54, 0x1EE54}, {0x1EE57, 0x1EE57}, {0x1EE59, 0x1EE59}, {0x1EE5B, 0x1EE5B}, {0x1EE5D, 0x1EE5D}, {0x1EE5F, 0x1EE5F}, {0x1EE61, 0x1EE62}, {0x1EE64, 0x1EE64}, {0x1EE67, 0x1EE6A}, {0x1EE6C, 0x1EE72}, {0x1EE74, 0x1EE77}, {0x1EE79, 0x1EE7C}, {0x1EE7E, 0x1EE7E}, {0x1EE80, 0x1EE89}, {0x1EE8B, 0x1EE9B}, {0x1EEA1, 0x1EEA3}, {0x1EEA5, 0x1EEA9}, {0x1EEAB, 0x1EEBB},
}

// bidiANRanges: exact UCD Bidi_Class=AN ranges.
var bidiANRanges = []bidiRange{
	{0x0600, 0x0605}, {0x0660, 0x0669}, {0x066B, 0x066C}, {0x06DD, 0x06DD}, {0x0890, 0x0891}, {0x08E2, 0x08E2}, {0x10D30, 0x10D39}, {0x10E60, 0x10E7E},
}

// bidiRRanges: exact UCD Bidi_Class=R ranges.
var bidiRRanges = []bidiRange{
	{0x05BE, 0x05BE}, {0x05C0, 0x05C0}, {0x05C3, 0x05C3}, {0x05C6, 0x05C6}, {0x05D0, 0x05EA}, {0x05EF, 0x05F4}, {0x07C0, 0x07EA}, {0x07F4, 0x07F5}, {0x07FA, 0x07FA}, {0x07FE, 0x0815}, {0x081A, 0x081A}, {0x0824, 0x0824}, {0x0828, 0x0828}, {0x0830, 0x083E}, {0x0840, 0x0858}, {0x085E, 0x085E}, {0x200F, 0x200F}, {0xFB1D, 0xFB1D}, {0xFB1F, 0xFB28}, {0xFB2A, 0xFB36}, {0xFB38, 0xFB3C}, {0xFB3E, 0xFB3E}, {0xFB40, 0xFB41}, {0xFB43, 0xFB44}, {0xFB46, 0xFB4F}, {0x10800, 0x10805}, {0x10808, 0x10808}, {0x1080A, 0x10835}, {0x10837, 0x10838}, {0x1083C, 0x1083C}, {0x1083F, 0x10855}, {0x10857, 0x1089E}, {0x108A7, 0x108AF}, {0x108E0, 0x108F2}, {0x108F4, 0x108F5}, {0x108FB, 0x1091B}, {0x10920, 0x10939}, {0x1093F, 0x1093F}, {0x10980, 0x109B7}, {0x109BC, 0x109CF}, {0x109D2, 0x10A00}, {0x10A10, 0x10A13}, {0x10A15, 0x10A17}, {0x10A19, 0x10A35}, {0x10A40, 0x10A48}, {0x10A50, 0x10A58}, {0x10A60, 0x10A9F}, {0x10AC0, 0x10AE4}, {0x10AEB, 0x10AF6}, {0x10B00, 0x10B35}, {0x10B40, 0x10B55}, {0x10B58, 0x10B72}, {0x10B78, 0x10B91}, {0x10B99, 0x10B9C}, {0x10BA9, 0x10BAF}, {0x10C00, 0x10C48}, {0x10C80, 0x10CB2}, {0x10CC0, 0x10CF2}, {0x10CFA, 0x10CFF}, {0x10E80, 0x10EA9}, {0x10EAD, 0x10EAD}, {0x10EB0, 0x10EB1}, {0x10F00, 0x10F27}, {0x10F70, 0x10F81}, {0x10F86, 0x10F89}, {0x10FB0, 0x10FCB}, {0x10FE0, 0x10FF6}, {0x1E800, 0x1E8C4}, {0x1E8C7, 0x1E8CF}, {0x1E900, 0x1E943}, {0x1E94B, 0x1E94B}, {0x1E950, 0x1E959}, {0x1E95E, 0x1E95F},
}

// bidiENRanges: exact UCD Bidi_Class=EN ranges.
var bidiENRanges = []bidiRange{
	{0x0030, 0x0039}, {0x06F0, 0x06F9}, {0xFF10, 0xFF19}, {0x1D7CE, 0x1D7FF}, {0x1FBF0, 0x1FBF9},
}

// bidiESRanges: exact UCD Bidi_Class=ES ranges.
var bidiESRanges = []bidiRange{
	{0x002B, 0x002B}, {0x002D, 0x002D}, {0x207A, 0x207B}, {0x208A, 0x208B}, {0x2212, 0x2212}, {0xFB29, 0xFB29}, {0xFE62, 0xFE63}, {0xFF0B, 0xFF0B}, {0xFF0D, 0xFF0D},
}

// bidiETRanges: exact UCD Bidi_Class=ET ranges.
var bidiETRanges = []bidiRange{
	{0x0023, 0x0025}, {0x00A2, 0x00A5}, {0x00B0, 0x00B1}, {0x058F, 0x058F}, {0x0609, 0x060A}, {0x066A, 0x066A}, {0x09F2, 0x09F3}, {0x09FB, 0x09FB}, {0x0AF1, 0x0AF1}, {0x0BF9, 0x0BF9}, {0x0E3F, 0x0E3F}, {0x17DB, 0x17DB}, {0x2030, 0x2034}, {0x20A0, 0x20C0}, {0x212E, 0x212E}, {0x2213, 0x2213}, {0xA838, 0xA839}, {0xFE5F, 0xFE5F}, {0xFE69, 0xFE6A}, {0xFF03, 0xFF05}, {0xFFE0, 0xFFE1}, {0xFFE5, 0xFFE6}, {0x11FDD, 0x11FE0}, {0x1E2FF, 0x1E2FF},
}

// bidiCSRanges: exact UCD Bidi_Class=CS ranges.
var bidiCSRanges = []bidiRange{
	{0x002C, 0x002C}, {0x002E, 0x002F}, {0x003A, 0x003A}, {0x00A0, 0x00A0}, {0x060C, 0x060C}, {0x202F, 0x202F}, {0x2044, 0x2044}, {0xFE50, 0xFE50}, {0xFE52, 0xFE52}, {0xFE55, 0xFE55}, {0xFF0C, 0xFF0C}, {0xFF0E, 0xFF0F}, {0xFF1A, 0xFF1A},
}

// classifyBidi maps a rune to its UAX #9 Bidi_Class. The exact class tables
// run first because some of their members are marks or digits (e.g. the
// Arabic number marks are AN despite being Mn).
func classifyBidi(r rune) bidiClass {
	switch {
	// Explicit directional formatting (X1-X8): retained as boundary
	// neutrals (they get a neighbor's level and produce no output).
	case r == 0x202A || r == 0x202B || r == 0x202C || r == 0x202D || r == 0x202E ||
		r == 0x2066 || r == 0x2067 || r == 0x2068 || r == 0x2069:
		return bidiFormatting
	// Paragraph and segment separators.
	case r == '\n' || r == '\r' || r == 0x1C || r == 0x1D || r == 0x1E || r == 0x85 || r == 0x2029:
		return bidiB
	case r == '\t' || r == 0x0B || r == 0x1F:
		return bidiS
	// C0/C1 control characters (minus the separators above) are boundary
	// neutrals.
	case r < 0x20 || (r >= 0x7F && r <= 0x9F):
		return bidiBN
	// Exact table classes.
	case bidiInRanges(bidiANRanges, r):
		return bidiAN
	case bidiInRanges(bidiALRanges, r):
		return bidiAL
	case bidiInRanges(bidiRRanges, r):
		return bidiR
	case bidiInRanges(bidiCSRanges, r):
		return bidiCS
	case bidiInRanges(bidiESRanges, r):
		return bidiES
	case bidiInRanges(bidiETRanges, r):
		return bidiET
	case bidiInRanges(bidiENRanges, r):
		return bidiEN
	// LRM is a strong L.
	case r == 0x200E:
		return bidiL
	// Non-spacing marks (Mn/Me) are NSM. Spacing marks (Mc) are strong L.
	case unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r):
		return bidiNSM
	case unicode.Is(unicode.Mc, r):
		return bidiL
	// Remaining digits that the exact tables did not claim are L.
	case unicode.Is(unicode.Nd, r):
		return bidiL
	// Space separators (Zs minus the CS members) and Zl.
	case r == 0x2028 || unicode.Is(unicode.Zs, r):
		return bidiWS
	// Remaining format characters (ZWSP/ZWNJ/ZWJ, soft hyphen, ...).
	case unicode.Is(unicode.Cf, r):
		return bidiBN
	case unicode.IsLetter(r):
		return bidiL
	default:
		return bidiON
	}
}

// IsRTL reports whether a line's first strong character is right-to-left,
// the standard heuristic for deriving the paragraph direction of a line
// (P2/P3 with the default first-strong rule). Latin digits and neutrals are
// not strong and never decide.
func IsRTL(text string) bool {
	for _, r := range text {
		switch classifyBidi(r) {
		case bidiL:
			return false
		case bidiR, bidiAL:
			return true
		}
	}
	return false
}

// classifyLine splits text into runes with their Bidi_Class and byte offsets.
func classifyLine(text string) (classes []bidiClass, offsets []int) {
	classes = make([]bidiClass, 0, len(text))
	offsets = make([]int, 0, len(text))
	for i, r := range text {
		classes = append(classes, classifyBidi(r))
		offsets = append(offsets, i)
	}
	return classes, offsets
}

// resolveBidiLevels computes the embedding levels of a line without rune
// identities (the conformance test path): bracket pairing is skipped
// because N0 needs the actual codepoints; everything else is the full
// algorithm (bidi_full.go).
func resolveBidiLevels(classes []bidiClass, baseRTL bool) []int {
	runes := make([]rune, len(classes))
	return resolveBidiLevelsRunes(classes, runes, baseRTL)
}

// reorderBidiLevels applies rule L2 to a level array without boundary
// neutrals: from the highest level down to the paragraph's base level,
// reverse every contiguous sequence at that level or higher. Returns the
// visual order as logical indices (order[visualIndex] == logicalIndex).
func reorderBidiLevels(levels []int) []int {
	drop := make([]bool, len(levels))
	return reorderBidiLine(levels, drop, paragraphBaseLevel(levels))
}

// reorderBidiLine implements L2 exactly like the Unicode reference: the
// boundary-neutral ("x") characters are REMOVED before reordering, the
// remaining sequence is reversed at each level from the highest down to the
// paragraph's lowest odd level, and dropped positions are skipped in the
// output. drop marks the removed positions; baseLevel is the paragraph's
// embedding level (0 LTR / 1 RTL for the top-level cases this port models).
func reorderBidiLine(levels []int, drop []bool, baseLevel int) []int {
	// Compacted view: kept[i] = original index of the i-th non-dropped char.
	kept := make([]int, 0, len(levels))
	for i, d := range drop {
		if !d {
			kept = append(kept, i)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	lev := make([]int, len(kept))
	for i, orig := range kept {
		lev[i] = levels[orig]
	}
	order := make([]int, len(lev))
	for i := range order {
		order[i] = i
	}
	maxLevel := lev[0]
	for _, l := range lev {
		if l > maxLevel {
			maxLevel = l
		}
	}
	// Stop at the paragraph's lowest odd level (everything below is LTR flow).
	lowest := baseLevel
	if lowest&1 == 0 {
		lowest++
	}
	if lowest < 1 {
		lowest = 1
	}
	reverse := func(lo, hi int) {
		for i, j := lo, hi-1; i < j; i, j = i+1, j-1 {
			order[i], order[j] = order[j], order[i]
		}
	}
	for high := maxLevel; high >= lowest; high-- {
		for i := 0; i < len(lev); {
			if lev[order[i]] < high {
				i++
				continue
			}
			j := i
			for j < len(lev) && lev[order[j]] >= high {
				j++
			}
			reverse(i, j)
			i = j
		}
	}
	// Map back to original indices and remove dropped positions.
	out := make([]int, 0, len(kept))
	for _, pos := range order {
		out = append(out, kept[pos])
	}
	return out
}

// paragraphBaseLevel infers the paragraph embedding level from a level array
// (the smallest level present; 0 when all levels are equal and even).
func paragraphBaseLevel(levels []int) int {
	minLevel := -1
	for _, l := range levels {
		if minLevel < 0 || l < minLevel {
			minLevel = l
		}
	}
	if minLevel < 0 {
		return 0
	}
	return minLevel
}

// ReorderLine resolves the embedding levels of one line for the given base
// direction and returns its directional segments in visual order (UAX #9
// L1-L2). Adjacent same-direction runs are merged; the returned segments
// tile the line's visible text (boundary neutrals and explicit directional
// formatting characters are omitted). Empty input returns no segments.
func ReorderLine(text string, baseRTL bool) []Segment {
	if text == "" {
		return nil
	}
	classes, offsets := classifyLine(text)
	// Snapshot the removable positions BEFORE resolution: X6a turns
	// PDI's class into ON, but the drop decision uses the original
	// classification (explicit formatting and boundary neutrals never
	// reach the output).
	drop := make([]bool, len(classes))
	for i, c := range classes {
		drop[i] = c == bidiFormatting || c == bidiBN
	}
	runes := make([]rune, len(classes))
	for i, off := range offsets {
		runes[i] = bytesToRune(text, off)
	}
	levels := resolveBidiLevelsRunes(classes, runes, baseRTL)
	base := 0
	if baseRTL {
		base = 1
	}
	order := reorderBidiLine(levels, drop, base)

	var segs []Segment
	start, end := -1, -1
	var segRTL bool
	closeSeg := func() {
		if start >= 0 {
			segs = append(segs, Segment{Start: start, End: end, RTL: segRTL})
			start = -1
		}
	}
	for _, ix := range order {
		c := classes[ix]
		if c == bidiFormatting || c == bidiBN {
			closeSeg()
			continue
		}
		off := offsets[ix]
		size := runeLen(bytesToRune(text, off))
		rtl := levels[ix]&1 == 1
		if start >= 0 && rtl == segRTL {
			if !rtl && off == end {
				// LTR continuation: next logical char.
				end = off + size
				continue
			}
			if rtl && off+size == start {
				// RTL continuation: previous logical char (visual order
				// walks the segment right to left).
				start = off
				continue
			}
		}
		closeSeg()
		start, end, segRTL = off, off+size, rtl
	}
	closeSeg()
	return segs
}

// bytesToRune returns the rune starting at the given byte offset.
func bytesToRune(text string, offset int) rune {
	r, _ := decodeRune(text[offset:])
	return r
}

// VisualText rebuilds the visual string of a ReorderLine result: segments
// are concatenated in visual order and RTL segments are reversed per rune.
func VisualText(text string, segs []Segment) string {
	var b strings.Builder
	for _, s := range segs {
		part := text[s.Start:s.End]
		if s.RTL {
			rs := []rune(part)
			for i, j := 0, len(rs)-1; i < j; i, j = i+1, j-1 {
				rs[i], rs[j] = rs[j], rs[i]
			}
			part = string(rs)
		}
		b.WriteString(part)
	}
	return b.String()
}
