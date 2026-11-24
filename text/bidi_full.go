package text

// Full UAX #9 implementation: X rules with embeddings and isolates, BD13
// isolating run sequences, N0 bracket pairs (BD14-BD16) and per-sequence
// W/N/I resolution (complementing bidi.go's classification, L1/L2 and the
// visual-order helpers).

// bidiBracketPair maps a paired bracket codepoint to its partner
// (Unicode BidiBrackets.txt; canonical equivalents 2329/232A are folded
// onto 3008/3009 so they pair with the CJK family like canonical matching).
var bidiBracketPair = map[rune]rune{
	0x0028: 0x0029, 0x0029: 0x0028, // ( )
	0x005B: 0x005D, 0x005D: 0x005B, // [ ]
	0x007B: 0x007D, 0x007D: 0x007B, // { }
	0x2045: 0x2046, 0x2046: 0x2045, // ⁅ ⁆
	0x207D: 0x207E, 0x207E: 0x207D, // ⁽ ⁾
	0x208D: 0x208E, 0x208E: 0x208D, // ₍ ₎
	0x2308: 0x2309, 0x2309: 0x2308, // ⌈ ⌉
	0x230A: 0x230B, 0x230B: 0x230A, // ⌊ ⌋
	0x2768: 0x2769, 0x2769: 0x2768, // ❨ ❩
	0x276A: 0x276B, 0x276B: 0x276A, // ❪ ❫
	0x276C: 0x276D, 0x276D: 0x276C, // ❬ ❭
	0x276E: 0x276F, 0x276F: 0x276E, // ❮ ❯
	0x2770: 0x2771, 0x2771: 0x2770, // ❰ ❱
	0x2772: 0x2773, 0x2773: 0x2772, // ❲ ❳
	0x2774: 0x2775, 0x2775: 0x2774, // ❴ ❵
	0x27C5: 0x27C6, 0x27C6: 0x27C5, // ⟅ ⟆
	0x27E6: 0x27E7, 0x27E7: 0x27E6, // ⟦ ⟧
	0x27E8: 0x27E9, 0x27E9: 0x27E8, // ⟨ ⟩
	0x27EA: 0x27EB, 0x27EB: 0x27EA, // ⟪ ⟫
	0x27EC: 0x27ED, 0x27ED: 0x27EC, // ⟬ ⟭
	0x27EE: 0x27EF, 0x27EF: 0x27EE, // ⟮ ⟯
	0x2983: 0x2984, 0x2984: 0x2983, // ⦃ ⦄
	0x2985: 0x2986, 0x2986: 0x2985, // ⦅ ⦆
	0x2987: 0x2988, 0x2988: 0x2987, // ⦇ ⦈
	0x2989: 0x298A, 0x298A: 0x2989, // ⦉ ⦊
	0x298B: 0x298C, 0x298C: 0x298B, // ⦋ ⦌
	0x298D: 0x2990, 0x2990: 0x298D, // ⦍ ⦐ (mirrored pairing)
	0x298E: 0x298F, 0x298F: 0x298E, // ⦎ ⦏ (mirrored pairing)
	0x2991: 0x2992, 0x2992: 0x2991, // ⦑ ⦒
	0x2993: 0x2994, 0x2994: 0x2993, // ⦓ ⦔
	0x2995: 0x2996, 0x2996: 0x2995, // ⦕ ⦖
	0x2997: 0x2998, 0x2998: 0x2997, // ⦗ ⦘
	0x29D8: 0x29D9, 0x29D9: 0x29D8, // ⧘ ⧙
	0x29DA: 0x29DB, 0x29DB: 0x29DA, // ⧚ ⧛
	0x29FC: 0x29FD, 0x29FD: 0x29FC, // ⧼ ⧽
	0x2E22: 0x2E23, 0x2E23: 0x2E22, // ⸢ ⸣
	0x2E24: 0x2E25, 0x2E25: 0x2E24, // ⸤ ⸥
	0x2E26: 0x2E27, 0x2E27: 0x2E26, // ⸦ ⸧
	0x2E28: 0x2E29, 0x2E29: 0x2E28, // ⸨ ⸩
	0x2329: 0x3009, 0x232A: 0x3008, // canonical equivalents of 〈 〉
	0x3008: 0x3009, 0x3009: 0x3008, // 〈 〉
	0x300A: 0x300B, 0x300B: 0x300A, // 《 》
	0x300C: 0x300D, 0x300D: 0x300C, // 「 」
	0x300E: 0x300F, 0x300F: 0x300E, // 『 』
	0x3010: 0x3011, 0x3011: 0x3010, // 【 】
	0x3014: 0x3015, 0x3015: 0x3014, // 〔 〕
	0x3016: 0x3017, 0x3017: 0x3016, // 〖 〗
	0x3018: 0x3019, 0x3019: 0x3018, // 〘 〙
	0x301A: 0x301B, 0x301B: 0x301A, // 〚 〛
	0x301D: 0x301E, 0x301E: 0x301D, // 〝 〞
	0xFD3E: 0xFD3F, 0xFD3F: 0xFD3E, // ﴾ ﴿
	0xFE17: 0xFE18, 0xFE18: 0xFE17, // ︗ ︘
	0xFE35: 0xFE36, 0xFE36: 0xFE35, // ︵ ︶
	0xFE37: 0xFE38, 0xFE38: 0xFE37, // ︷ ︸
	0xFE39: 0xFE3A, 0xFE3A: 0xFE39, // ︹ ︺
	0xFE3B: 0xFE3C, 0xFE3C: 0xFE3B, // ︻ ︼
	0xFE3D: 0xFE3E, 0xFE3E: 0xFE3D, // ︽ ︾
	0xFE3F: 0xFE40, 0xFE40: 0xFE3F, // ︿ ﹀
	0xFE41: 0xFE42, 0xFE42: 0xFE41, // ﹁ ﹂
	0xFE43: 0xFE44, 0xFE44: 0xFE43, // ﹃ ﹄
	0xFE47: 0xFE48, 0xFE48: 0xFE47, // ﹇ ﹈
	0xFE59: 0xFE5A, 0xFE5A: 0xFE59, // ﹙ ﹚
	0xFE5B: 0xFE5C, 0xFE5C: 0xFE5B, // ﹛ ﹜
	0xFE5D: 0xFE5E, 0xFE5E: 0xFE5D, // ﹝ ﹞
	0xFF08: 0xFF09, 0xFF09: 0xFF08, // （ ）
	0xFF3B: 0xFF3D, 0xFF3D: 0xFF3B, // ［ ］
	0xFF5B: 0xFF5D, 0xFF5D: 0xFF5B, // ｛ ｝
	0xFF5F: 0xFF60, 0xFF60: 0xFF5F, // ｟ ｠
	0xFF62: 0xFF63, 0xFF63: 0xFF62, // ｢ ｣
}

// isBracketOpen reports whether r is an opening bracket (the lower
// codepoint of its pair, with the mirrored ⦍/⦐ and ⦎/⦏ exceptions).
func isBracketOpen(r rune) bool {
	switch r {
	case 0x298D, 0x298F: // ⦍ opens (pairs with ⦐); ⦎ opens (pairs with ⦏)
		return true
	case 0x2990, 0x298E:
		return false
	}
	partner, ok := bidiBracketPair[r]
	return ok && r < partner
}

// bidiLevelLimit is the maximum embedding depth (X1).
const bidiLevelLimit = 125

// bidiSequence is one isolating run sequence (BD13): indices into the line
// (not necessarily contiguous) plus its sos/eos resolved classes.
type bidiSequence struct {
	indices  []int
	sos      bidiClass
	eos      bidiClass
	embedRTL bool
}

// resolveBidiLevelsRunes computes embedding levels for a line: X rules
// (embeddings, overrides, isolates), then W/N/I rules per isolating run
// sequence (BD13) and L1. classes is the classified line; runes the
// corresponding codepoints (needed by N0 bracket pairing). The classes
// slice is mutated during resolution.
func resolveBidiLevelsRunes(classes []bidiClass, runes []rune, baseRTL bool) []int {
	base := 0
	if baseRTL {
		base = 1
	}
	n := len(classes)
	levels := make([]int, n)
	for i := range levels {
		levels[i] = base
	}
	if n == 0 {
		return levels
	}

	// --- X1-X8: the directional status stack (embeddings + isolates) ---
	type stackEntry struct {
		level     int
		override  bidiClass // bidiL/bidiR override, or 0 for neutral
		isIsolate bool
	}
	stack := []stackEntry{{level: base}}
	for i, c := range classes {
		top := &stack[len(stack)-1]
		switch c {
		case bidiFormatting:
			r := runes[i]
			switch r {
			case 0x202A, 0x202D: // LRE, LLO
				if len(stack) < bidiLevelLimit {
					lvl := top.level + 1
					if lvl&1 == 1 {
						lvl++
					}
					ov := bidiClass(0)
					if r == 0x202D {
						ov = bidiL
					}
					stack = append(stack, stackEntry{level: lvl, override: ov})
				}
				levels[i] = stack[len(stack)-1].level
			case 0x202B, 0x202E: // RLE, RLO
				if len(stack) < bidiLevelLimit {
					lvl := top.level + 1
					if lvl&1 == 0 {
						lvl++
					}
					ov := bidiClass(0)
					if r == 0x202E {
						ov = bidiR
					}
					stack = append(stack, stackEntry{level: lvl, override: ov})
				}
				levels[i] = stack[len(stack)-1].level
			case 0x202C: // PDF: pop the last non-isolate entry (X7).
				if len(stack) > 1 && !stack[len(stack)-1].isIsolate {
					stack = stack[:len(stack)-1]
				}
				levels[i] = stack[len(stack)-1].level
			case 0x2066, 0x2067, 0x2068: // LRI, RLI, FSI (X5a-c)
				isRTL := r == 0x2067
				if r == 0x2068 {
					isRTL = fsiBaseIsRTL(classes, runes, i+1)
				}
				lvl := top.level + 1
				if isRTL {
					if lvl&1 == 0 {
						lvl++
					}
				} else if lvl&1 == 1 {
					lvl++
				}
				if len(stack) < bidiLevelLimit {
					stack = append(stack, stackEntry{level: lvl, isIsolate: true})
				}
				levels[i] = stack[len(stack)-1].level
			case 0x2069: // PDI (X6a)
				for len(stack) > 1 && !stack[len(stack)-1].isIsolate {
					stack = stack[:len(stack)-1]
				}
				if len(stack) > 1 {
					stack = stack[:len(stack)-1]
				}
				levels[i] = stack[len(stack)-1].level
				classes[i] = bidiON
			}
		case bidiBN:
			levels[i] = top.level
		case bidiB, bidiS:
			// X8: separators take the paragraph level.
			levels[i] = base
		default:
			levels[i] = top.level
			if top.override != 0 {
				classes[i] = top.override
			}
		}
	}

	// --- BD13: isolating run sequences ---
	seqs := buildIsolatingRunSequences(classes, runes, levels, base)

	// --- W rules per sequence ---
	for si := range seqs {
		resolveWeakTypes(&seqs[si], classes)
	}

	// Snapshot for L1: after X/W, before N (the N rules resolve WS/S/B to
	// strong types, but L1 must still see the originals).
	origClasses := append([]bidiClass(nil), classes...)

	// --- N + I rules per sequence ---
	for si := range seqs {
		resolveBracketsAndNeutrals(&seqs[si], classes, runes)
		resolveImplicitLevels(&seqs[si], classes, levels)
	}

	// --- L1: reset trailing whitespace and separators to the base level ---
	prevLevel := base
	for i, c := range origClasses {
		if c == bidiFormatting || c == bidiBN {
			levels[i] = prevLevel
		} else {
			prevLevel = levels[i]
		}
	}
	for i := 0; i < n; {
		if origClasses[i] == bidiWS || origClasses[i] == bidiBN || origClasses[i] == bidiFormatting {
			j := i
			for j < n && (origClasses[j] == bidiWS || origClasses[j] == bidiBN || origClasses[j] == bidiFormatting) {
				j++
			}
			if j == n || origClasses[j] == bidiS || origClasses[j] == bidiB {
				for k := i; k < j; k++ {
					levels[k] = base
				}
			}
			i = j
			continue
		}
		if origClasses[i] == bidiS || origClasses[i] == bidiB {
			levels[i] = base
		}
		i++
	}
	return levels
}

// fsiBaseIsRTL implements P2/P3 for an FSI: scan forward for the first
// strong type (L/R/AL), skipping nested isolate initiators and their
// content up to the matching PDIs (P2).
func fsiBaseIsRTL(classes []bidiClass, runes []rune, from int) bool {
	depth := 0
	for i := from; i < len(classes); i++ {
		if classes[i] == bidiFormatting {
			switch runes[i] {
			case 0x2066, 0x2067, 0x2068:
				depth++
				continue
			case 0x2069:
				if depth > 0 {
					depth--
				}
				continue
			}
		}
		if depth > 0 {
			continue
		}
		switch classes[i] {
		case bidiL:
			return false
		case bidiR, bidiAL:
			return true
		}
	}
	return false
}

// buildIsolatingRunSequences implements BD13: level runs merged across
// isolate boundaries — a run ending with an isolate initiator joins the
// run containing its matching PDI (and the runs between) into one
// isolating run sequence.
func buildIsolatingRunSequences(classes []bidiClass, runes []rune, levels []int, base int) []bidiSequence {
	n := len(classes)

	// Level runs (maximal same-level spans).
	type levelRun struct{ start, end int }
	var runs []levelRun
	for i := 0; i < n; {
		j := i
		for j+1 < n && levels[j+1] == levels[i] {
			j++
		}
		runs = append(runs, levelRun{i, j})
		i = j + 1
	}
	if len(runs) == 0 {
		return nil
	}
	runOf := make([]int, n)
	for ri, r := range runs {
		for i := r.start; i <= r.end; i++ {
			runOf[i] = ri
		}
	}

	// Union-find over runs.
	seqOfRun := make([]int, len(runs))
	for i := range seqOfRun {
		seqOfRun[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for seqOfRun[i] != i {
			seqOfRun[i] = seqOfRun[seqOfRun[i]]
			i = seqOfRun[i]
		}
		return i
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			seqOfRun[rb] = ra
		}
	}

	for ri, r := range runs {
		last := r.end
		if classes[last] != bidiFormatting {
			continue
		}
		ru := runes[last]
		if ru != 0x2066 && ru != 0x2067 && ru != 0x2068 {
			continue
		}
		// Find the matching PDI (depth counting over isolate initiators).
		depth := 1
		pdi := -1
		for j := last + 1; j < n; j++ {
			if classes[j] == bidiFormatting {
				switch runes[j] {
				case 0x2066, 0x2067, 0x2068:
					depth++
				case 0x2069:
					depth--
					if depth == 0 {
						pdi = j
					}
				}
			}
			if pdi >= 0 {
				break
			}
		}
		if pdi < 0 {
			// Unmatched initiator: the sequence extends to the end of the
			// line (merge forward).
			for k := ri + 1; k < len(runs); k++ {
				union(ri, k)
			}
			continue
		}
		for k := ri; k <= runOf[pdi]; k++ {
			union(ri, k)
		}
	}

	// Collect sequences in order of first run.
	var order []int
	seenRoot := map[int]bool{}
	for ri := range runs {
		root := find(ri)
		if !seenRoot[root] {
			seenRoot[root] = true
			order = append(order, root)
		}
	}
	byRoot := map[int][]int{}
	for ri := range runs {
		byRoot[find(ri)] = append(byRoot[find(ri)], ri)
	}

	var seqs []bidiSequence
	for _, root := range order {
		ris := byRoot[root]
		var indices []int
		for _, ri := range ris {
			for i := runs[ri].start; i <= runs[ri].end; i++ {
				indices = append(indices, i)
			}
		}
		first := runs[ris[0]]
		last := runs[ris[len(ris)-1]]
		seqLevel := levels[first.start]
		// sos/eos (X10): the higher of the sequence's level and the
		// adjacent level (X9-removed characters skipped); R when odd.
		sosLevel := base
		for i := first.start - 1; i >= 0; i-- {
			if !x9Removed(classes[i], runes[i]) {
				sosLevel = levels[i]
				break
			}
		}
		eosLevel := base
		for i := last.end + 1; i < n; i++ {
			if !x9Removed(classes[i], runes[i]) {
				eosLevel = levels[i]
				break
			}
		}
		seqs = append(seqs, bidiSequence{
			indices:  indices,
			sos:      classForLevel(bidiMaxInt(seqLevel, sosLevel)),
			eos:      classForLevel(bidiMaxInt(seqLevel, eosLevel)),
			embedRTL: seqLevel&1 == 1,
		})
	}
	return seqs
}

// x9Removed reports whether the position is removed by X9 (LRE/RLE/LRO/
// RLO/PDF) or is BN.
func x9Removed(c bidiClass, r rune) bool {
	if c == bidiBN {
		return true
	}
	if c == bidiFormatting {
		switch r {
		case 0x202A, 0x202B, 0x202C, 0x202D, 0x202E:
			return true
		}
	}
	return false
}

func classForLevel(l int) bidiClass {
	if l&1 == 1 {
		return bidiR
	}
	return bidiL
}

func bidiMaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// resolveWeakTypes applies W1-W7 across one isolating run sequence, with
// the sequence's sos standing in for the context before the first
// character and eos after the last.
func resolveWeakTypes(seq *bidiSequence, classes []bidiClass) {
	idx := seq.indices
	if len(idx) == 0 {
		return
	}

	// W1: NSM takes the previous character's class (sos at the start).
	// BN and X9-removed characters are transparent: the "previous
	// character" skips them (TR9: removed characters do not participate).
	prev := seq.sos
	for _, i := range idx {
		if classes[i] == bidiBN || classes[i] == bidiFormatting {
			continue
		}
		if classes[i] == bidiNSM {
			classes[i] = prev
		} else {
			prev = classes[i]
		}
	}

	// W2: EN becomes AN when the last strong type is AL (sos is L/R, so the
	// search starts empty). W3: AL becomes R.
	lastStrongAL := false
	for _, i := range idx {
		if classes[i] == bidiBN || classes[i] == bidiFormatting {
			continue
		}
		c := classes[i]
		if c == bidiAL {
			classes[i] = bidiR
			lastStrongAL = true
			continue
		}
		if c == bidiEN && lastStrongAL {
			classes[i] = bidiAN
			continue
		}
		switch c {
		case bidiL, bidiR:
			lastStrongAL = false
		}
	}

	// W4: a single ES/CS between two numbers of the same kind takes that
	// number type. W5: a run of ETs adjacent to an EN becomes EN (either
	// side: ETs accumulate in a pending buffer and convert when the EN
	// arrives, or turn ON otherwise). W6: the remaining separators and ETs
	// become ON.
	prevEff := seq.sos
	var pendingET []int
	flushET := func(toEN bool) {
		for _, j := range pendingET {
			if toEN {
				classes[j] = bidiEN
			} else {
				classes[j] = bidiON
			}
		}
		pendingET = pendingET[:0]
	}
	for k, i := range idx {
		c := classes[i]
		if c == bidiBN || c == bidiFormatting {
			continue
		}
		switch c {
		case bidiEN:
			// W5: an EN converts the ET run that precedes it.
			flushET(true)
		case bidiES, bidiCS:
			// The separator ends any pending ET run: the ET is no longer
			// adjacent to the EN (W5 adjacency), so it resolves to ON.
			flushET(false)
			next := seq.eos
			for j := k + 1; j < len(idx); j++ {
				nc := classes[idx[j]]
				if nc == bidiBN || nc == bidiFormatting {
					continue
				}
				next = nc
				break
			}
			switch {
			case prevEff == bidiEN && c == bidiES && next == bidiEN:
				classes[i] = bidiEN
			case prevEff == bidiEN && c == bidiCS && next == bidiEN:
				classes[i] = bidiEN
			case prevEff == bidiAN && c == bidiCS && next == bidiAN:
				classes[i] = bidiAN
			default:
				classes[i] = bidiON
			}
		case bidiET:
			if prevEff == bidiEN {
				classes[i] = bidiEN
			} else {
				pendingET = append(pendingET, i)
			}
		default:
			// A non-ET, non-EN character ends any pending ET run: W6.
			flushET(false)
		}
		if classes[i] != bidiET {
			prevEff = classes[i]
		}
	}
	// Trailing ET run at the sequence end: W6.
	flushET(false)

	// W7: EN becomes L when the last strong type is L (sos counts).
	lastStrongL := seq.sos == bidiL
	for _, i := range idx {
		switch classes[i] {
		case bidiEN:
			if lastStrongL {
				classes[i] = bidiL
			}
		case bidiL, bidiR:
			lastStrongL = classes[i] == bidiL
		}
	}
}

// strongTypeForN0 maps a resolved class to its N0 strong type: EN and AN
// are treated as R (TR9 N0 note); L stays L; others are not strong.
func strongTypeForN0(c bidiClass) (bidiClass, bool) {
	switch c {
	case bidiL:
		return bidiL, true
	case bidiR, bidiAL, bidiEN, bidiAN:
		return bidiR, true
	}
	return 0, false
}

// resolveBracketsAndNeutrals applies N0 (bracket pairs), then N1/N2, over
// one isolating run sequence.
func resolveBracketsAndNeutrals(seq *bidiSequence, classes []bidiClass, runes []rune) {
	idx := seq.indices
	if len(idx) == 0 {
		return
	}
	e := bidiL
	if seq.embedRTL {
		e = bidiR
	}
	o := bidiR
	if e == bidiR {
		o = bidiL
	}
	isNI := func(c bidiClass) bool {
		return c == bidiWS || c == bidiS || c == bidiB || c == bidiON || c == bidiBN
	}

	inSeq := make([]bool, len(classes))
	for _, i := range idx {
		inSeq[i] = true
	}

	// --- N0: bracket pairs (BD16 stack pairing within the sequence) ---
	type pair struct{ open, close int }
	var pairs []pair
	var stack []int
	for _, i := range idx {
		if classes[i] != bidiON {
			// BD14/BD15: brackets keep their Bidi type; after X6 an
			// overridden bracket is no longer ON and does not pair.
			continue
		}
		r := runes[i]
		partner, ok := bidiBracketPair[r]
		if !ok {
			continue
		}
		if isBracketOpen(r) {
			stack = append(stack, i)
		} else {
			// Closing: pop until the matching opener (unmatched openers
			// are discarded, BD16).
			for len(stack) > 0 && runes[stack[len(stack)-1]] != partner {
				stack = stack[:len(stack)-1]
			}
			if len(stack) > 0 {
				pairs = append(pairs, pair{stack[len(stack)-1], i})
				stack = stack[:len(stack)-1]
			}
		}
	}

	for _, p := range pairs {
		// 1. Inspect the enclosed strong types (sequence members only; the
		// sequence may not be contiguous).
		var insideStrong bidiClass
		for i := p.open + 1; i < p.close; i++ {
			if !inSeq[i] {
				continue
			}
			if s, ok := strongTypeForN0(classes[i]); ok {
				if s == e {
					insideStrong = e
					break
				}
				if insideStrong == 0 {
					insideStrong = s
				}
			}
		}
		if insideStrong == e {
			classes[p.open] = e
			classes[p.close] = e
			continue
		}
		if insideStrong != 0 {
			// 2. Established context: scan backwards from the opening
			// bracket (sequence order) over neutrals for the first strong
			// type; sos when none.
			preceding := seq.sos
		scan:
			for _, i := range idx {
				if i >= p.open {
					break
				}
				if !isNI(classes[i]) {
					if s, ok := strongTypeForN0(classes[i]); ok {
						preceding = s
						break scan
					}
				}
			}
			if preceding == o {
				// Context established with the opposite direction.
				classes[p.open] = o
				classes[p.close] = o
			} else {
				classes[p.open] = e
				classes[p.close] = e
			}
		}
		// No enclosed strong types: leave for N1/N2.
	}

	// --- N1/N2: neutral runs ---
	isStrongish := func(c bidiClass) bidiClass {
		if c == bidiEN || c == bidiAN {
			return bidiR
		}
		return c
	}
	prev := seq.sos
	for k := 0; k < len(idx); {
		i := idx[k]
		if !isNI(classes[i]) {
			prev = classes[i]
			k++
			continue
		}
		j := k
		for j < len(idx) && isNI(classes[idx[j]]) {
			j++
		}
		next := seq.eos
		if j < len(idx) {
			next = classes[idx[j]]
		}
		left, right := isStrongish(prev), isStrongish(next)
		resolved := e
		if left == bidiL && right == bidiL {
			resolved = bidiL
		} else if left == bidiR && right == bidiR {
			resolved = bidiR
		}
		for m := k; m < j; m++ {
			classes[idx[m]] = resolved
		}
		if j < len(idx) {
			prev = classes[idx[j]]
		}
		k = j
	}
}

// resolveImplicitLevels applies I1/I2 across one isolating run sequence.
func resolveImplicitLevels(seq *bidiSequence, classes []bidiClass, levels []int) {
	for _, i := range seq.indices {
		c := classes[i]
		if c == bidiFormatting || c == bidiBN {
			continue
		}
		if levels[i]&1 == 0 {
			switch c {
			case bidiR:
				levels[i]++
			case bidiAN, bidiEN:
				levels[i] += 2
			}
		} else {
			switch c {
			case bidiL, bidiEN, bidiAN:
				levels[i]++
			}
		}
	}
}

// isBidiFormattingString reports whether every rune of s is a BiDi
// formatting character (embeddings, overrides, isolates, PDF): the shaper
// skips them — they are zero-width controls excluded from the visual
// segments.
func isBidiFormattingString(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch r {
		case 0x202A, 0x202B, 0x202C, 0x202D, 0x202E, 0x2066, 0x2067, 0x2068, 0x2069:
		default:
			return false
		}
	}
	return true
}
