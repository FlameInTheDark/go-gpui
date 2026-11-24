package text

import (
	"strings"
	"unicode"
)

// BreakOpportunities returns the byte offsets where a line may break
// (UAX #14 line break opportunities, covering the common cases: after
// spaces, between words, after hyphens; no break inside CJK is approximated
// by allowing breaks between CJK chars like most terminals).
func BreakOpportunities(s string) []int {
	var out []int
	rs := []rune(s)
	byteIx := 0
	prev := rune(0)
	prevClass := classOf(prev)
	for i, r := range rs {
		_ = i
		cls := classOf(r)
		if canBreakBetween(prevClass, cls, prev, r) {
			out = append(out, byteIx)
		}
		prev = r
		prevClass = cls
		byteIx += runeLen(r)
	}
	// The end of text is always a break opportunity.
	out = append(out, byteIx)
	return out
}

func runeLen(r rune) int {
	switch {
	case r < 0x80:
		return 1
	case r < 0x800:
		return 2
	case r < 0x10000:
		return 3
	default:
		return 4
	}
}

type breakClass uint8

const (
	clOther breakClass = iota
	clSpace
	clLetter
	clCJK
	clHyphen
	clPunctOpen
	clPunctClose
	clDigit
	clIdeographicPunct
	clZWJ
)

func classOf(r rune) breakClass {
	switch {
	case r == 0x200D:
		return clZWJ
	case r == ' ' || r == '\t' || r == 0x00A0:
		return clSpace
	case r == '-' || r == 0x2010 || r == 0x2011:
		return clHyphen
	case unicode.IsDigit(r):
		return clDigit
	case unicode.IsLetter(r):
		if isCJK(r) {
			return clCJK
		}
		return clLetter
	case unicode.Is(unicode.P, r) || unicode.Is(unicode.S, r):
		switch r {
		case '(', '[', '{', '"', '\'', 0x2018, 0x201C, 0x300C:
			return clPunctOpen
		case ')', ']', '}', ',', '.', ';', ':', '!', '?', 0x2019, 0x201D, 0x3001, 0x3002:
			return clPunctClose
		}
		return clPunctClose
	case unicode.Is(unicode.Mn, r):
		return clOther
	default:
		return clOther
	}
}

func isCJK(r rune) bool {
	switch {
	case r >= 0x2E80 && r <= 0x9FFF, // CJK radicals .. Yi
		r >= 0xAC00 && r <= 0xD7AF, // Hangul syllables
		r >= 0xF900 && r <= 0xFAFF,
		r >= 0x20000 && r <= 0x2FA1F:
		return true
	}
	return false
}

// canBreakBetween implements the core UAX#14 pair rules for the classes we
// track. LB2/LB3: no break at start; always at end.
func canBreakBetween(prev, cur breakClass, prevR, curR rune) bool {
	if prev == clOther && prevR == 0 {
		return false // start of text
	}
	// Do not break before combining marks or after ZWJ.
	if cur == clOther && unicode.Is(unicode.Mn, curR) {
		return false
	}
	if prev == clZWJ {
		return false
	}
	// No break inside numbers.
	if prev == clDigit && cur == clDigit {
		return false
	}
	if prev == clLetter && cur == clDigit {
		return false
	}
	if prev == clDigit && cur == clLetter {
		return false
	}
	// No break after an open punctuation.
	if prev == clPunctOpen {
		return false
	}
	// No break before close punctuation or ideographic punctuation.
	if cur == clPunctClose || cur == clIdeographicPunct {
		return false
	}
	// Break after spaces (trailing space rule).
	if prev == clSpace {
		return true
	}
	// No break before a space (break happens after it).
	if cur == clSpace {
		return false
	}
	// Break after hyphens (except before digits, e.g. minus).
	if prev == clHyphen {
		return true
	}
	// CJK allows breaks between characters (except before close punct).
	if prev == clCJK || cur == clCJK {
		return true
	}
	// Break between letters and punctuation boundaries.
	if prev == clLetter && cur == clLetter {
		return false
	}
	// Default: break between letter -> punct-open etc.
	if prev == clLetter && cur == clPunctOpen {
		return false
	}
	if prev == clPunctClose && (cur == clLetter || cur == clDigit || cur == clPunctOpen) {
		return true
	}
	if prev == clLetter && cur == clOther {
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// WrapWithIndent (upstream line_wrapper.rs IndentAdjustment, condensed to a
// pure string-level helper without font metrics)
// ---------------------------------------------------------------------------

// WrapWithIndent wraps each line of text to width and prefixes every
// continuation line with the first line's leading indent (spaces/tabs), the
// string-level equivalent of upstream LineWrapper::wrap_line with
// IndentAdjustment::SameIndent (line_wrapper.rs, the default adjustment:
// "Match the original line's leading whitespace").
//
// Exact rule implemented (upstream measures real glyph widths and returns
// Boundary{byte index, indent columns}; this port has no font, so it counts
// columns instead — a space is one column, a tab is indentWidth columns, and
// width is in those columns):
//
//  1. Each input line (split on '\n') is wrapped independently at UAX #14
//     break opportunities (BreakOpportunities; long words are hard-split).
//     The first row keeps the leading indent as-is because it is part of the
//     text; continuation rows get the leading indent prefixed, and the
//     indent counts toward the row width, like upstream adds indent_width to
//     the row width after each wrap.
//  2. Up/down adjustment (upstream base_indent = min(MAX_INDENT,
//     first_non_whitespace_ix - last_wrap_ix)): when the leading indent
//     fills an entire row on its own (indent columns >= width, or the line
//     is all whitespace), the first visual row is blank-ish and the
//     continuation unindents by one level — one indentWidth worth of
//     columns is dropped from the indent (upstream gives that continuation
//     row indent 0, because the wrap happened before any non-whitespace
//     character). While the remaining indent would still fill a whole row,
//     it is unindented another level, so wrapped rows never degrade into
//     the cascading whitespace-only rows upstream's comment warns about.
//  3. Rows are right-trimmed of spaces/tabs; a whitespace-only row keeps
//     its (width-limited) whitespace so the blank-ish first indent row
//     stays visible.
//
// Short lines produce no continuation: a line that fits within width is
// returned unchanged. An empty text yields one empty row.
func WrapWithIndent(text string, width float32, indentWidth float32) []string {
	if width < 1 {
		width = 1
	}
	level := indentWidth
	if level < 1 {
		level = 1
	}
	var out []string
	for _, line := range strings.Split(text, "\n") {
		out = append(out, wrapIndentLine(line, width, level)...)
	}
	if out == nil {
		out = []string{""}
	}
	return out
}

// wrapIndentLine wraps a single physical line, applying the indent rules
// documented on WrapWithIndent.
func wrapIndentLine(line string, width, level float32) []string {
	// Leading indent: the run of spaces/tabs before the first non-whitespace
	// character (upstream first_non_whitespace_ix).
	ix := 0
	for ix < len(line) && (line[ix] == ' ' || line[ix] == '\t') {
		ix++
	}
	indent, rest := line[:ix], line[ix:]
	indentCols := stringColumns(indent, level)

	// Blank or all-whitespace line: a single row, no continuation (upstream
	// wraps all-whitespace lines with indent 0; the port keeps the row
	// verbatim).
	if rest == "" {
		return []string{line}
	}

	// Continuation indent (upstream IndentAdjustment::SameIndent keeps the
	// line's leading whitespace on every continuation row).
	contIndent, contCols := indent, indentCols
	if indentCols >= width {
		// The first visual row is blank-ish: unindent one level, then keep
		// unindenting while the remainder would still fill a row (rule 2 in
		// the WrapWithIndent doc comment).
		contCols = max(indentCols-level, 0)
		contIndent = indentPrefix(indent, contCols, level)
		for contCols > 0 && width-contCols < 1 {
			contCols = max(contCols-level, 0)
			contIndent = indentPrefix(indent, contCols, level)
		}
	}

	// First row budget: the indent is part of the text, so the first row
	// fits width - indentCols columns of content. Continuation rows fit
	// width - contCols columns after the prefixed indent (upstream adds the
	// indent to the row width after each wrap).
	var rows []string
	budget := width - indentCols
	if budget < 1 {
		// Blank-ish first row: keep the visible indent prefix (up to width
		// columns) as the first row, then continue with the unindented
		// indent.
		rows = append(rows, indentPrefix(indent, min(indentCols, width), level))
		budget = width - contCols
		rows = append(rows, wrapRows(rest, budget, level)...)
	} else {
		rows = wrapRows(rest, budget, level)
		// The first row carries the line's own leading indent (it was
		// stripped above for measuring; upstream keeps it in the text).
		rows[0] = indent + rows[0]
	}
	// Continuation rows carry the indent (SameIndent).
	for i := 1; i < len(rows); i++ {
		rows[i] = contIndent + rows[i]
	}
	return rows
}

// wrapRows wraps s greedily at break opportunities, right-trimming each row.
// Whitespace-only rows keep their content so blank indent rows stay visible.
func wrapRows(s string, budget, level float32) []string {
	if budget < 1 {
		budget = 1
	}
	var rows []string
	start := 0
	for start < len(s) {
		best := 0
		for _, c := range BreakOpportunities(s[start:]) {
			if c == 0 {
				continue
			}
			if stringColumns(s[start:start+c], level) <= budget {
				best = c
			} else {
				break
			}
		}
		if best == 0 {
			// No break opportunity fits (or an oversized single word): hard
			// split at the budget boundary; an oversized grapheme stays
			// intact even if it exceeds the width (upstream keeps oversized
			// graphemes intact too).
			best = byteIndexForColumns(s[start:], budget, level)
		}
		chunk := s[start : start+best]
		row := strings.TrimRight(chunk, " \t")
		if row == "" && strings.TrimRight(chunk, " \t") != strings.Trim(chunk, " \t") {
			// Whitespace-only chunk: keep it so blank rows stay visible.
			row = chunk
		}
		rows = append(rows, row)
		start += best
		// Breaks land after spaces, so skip the remaining run before the
		// next row (the carried space is dropped, like upstream trims the
		// carried suffix).
		for start < len(s) && (s[start] == ' ' || s[start] == '\t') {
			start++
		}
	}
	return rows
}

// stringColumns measures s in columns: a tab counts level columns, any
// other rune one column.
func stringColumns(s string, level float32) float32 {
	cols := float32(0)
	for _, r := range s {
		if r == '\t' {
			cols += level
		} else {
			cols++
		}
	}
	return cols
}

// byteIndexForColumns returns the byte offset of the first rune that would
// exceed budget columns (at least one rune, so the wrapper always advances).
func byteIndexForColumns(s string, budget, level float32) int {
	cols := float32(0)
	for i, r := range s {
		w := float32(1)
		if r == '\t' {
			w = level
		}
		if cols+w > budget && i > 0 {
			return i
		}
		cols += w
	}
	return len(s)
}

// indentPrefix cuts the indent to at most cols columns, keeping whole runes.
func indentPrefix(indent string, cols, level float32) string {
	c := float32(0)
	for i, r := range indent {
		w := float32(1)
		if r == '\t' {
			w = level
		}
		if c+w > cols {
			return indent[:i]
		}
		c += w
	}
	return indent
}
