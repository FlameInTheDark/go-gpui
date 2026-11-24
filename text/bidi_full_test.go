package text

import (
	"strings"
	"testing"
)

// stripBidiFormatting removes the explicit formatting characters our
// ReorderLine drops from the output (the python-bidi reference keeps them,
// so expected strings are compared after stripping).
func stripBidiFormatting(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case 0x202A, 0x202B, 0x202C, 0x202D, 0x202E, 0x2066, 0x2067, 0x2068, 0x2069:
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// assertVisual checks the visual ordering against a reference produced by
// python-bidi 0.6.6 (the full UAX #9 algorithm).
func assertVisual(t *testing.T, line string, baseRTL bool, expected string) {
	t.Helper()
	segs := ReorderLine(line, baseRTL)
	visual := VisualText(line, segs)
	if visual != expected {
		t.Fatalf("visual of %q (rtl=%v) = %q, want %q", line, baseRTL, visual, expected)
	}
}

// TestBidiN0BracketPairs verifies rule N0: paired brackets resolve to the
// embedding direction when their content (or the established context)
// matches, matching the python-bidi reference.
func TestBidiN0BracketPairs(t *testing.T) {
	// LTR paragraph, Hebrew inside parentheses preceded by L: the content
	// is opposite the embedding direction and the preceding strong type is
	// L (= e), so both brackets take e (N0 step 2b) — the closing bracket
	// does NOT join the Hebrew run.
	assertVisual(t, "a(בג)דה", false, "a(גב)הד")

	// Same with no preceding strong: sos = L = e, so the brackets still
	// take the embedding direction.
	assertVisual(t, "(בג)דה", false, "(גב)הד")

	// RTL paragraph with Latin inside parentheses: the brackets resolve R
	// (preceding strong is R = e), the Latin run stays LTR.
	assertVisual(t, "א(bc)ד", true, "ד)bc(א")

	// Square brackets around a single Hebrew glyph between Latin letters.
	assertVisual(t, "a[ב]c", false, "a[ב]c")

	// Hebrew on both sides, Latin inside: the inside matches the
	// embedding direction... (LTR paragraph) — N0 step 1 does not apply
	// (inside is L = e!) wait: inside 'a' is L matching e, so both
	// brackets take e (step 1).
	assertVisual(t, "ב(a)ב", false, "ב(a)ב")

	// Mixed: preceding L, inside R, following R — N0 keeps the pair
	// together on the embedding side.
	assertVisual(t, "a(ב)ב", false, "a(ב)ב")
}

// TestBidiIsolates verifies directional isolates (LRI/RLI/FSI/PDI, rules
// X5a-c/X6a + BD13): the isolated content forms its own run sequence and
// does not influence the surrounding neutrals.
func TestBidiIsolates(t *testing.T) {
	// An RLI around a Latin letter in an LTR paragraph: the letter renders
	// RTL but the surrounding text is unaffected (formatting stripped).
	assertVisual(t, "a\u2067b\u2069c", false, "abc")

	// Spaces around an isolated run stay in the paragraph flow.
	assertVisual(t, "a \u2067b\u2069 c", false, "a b c")

	// An LRI around Latin text in an RTL paragraph keeps the Latin run LTR.
	assertVisual(t, "א\u2066ab\u2069ב", true, "בabא")

	// FSI resolves its direction from the first strong character (P2/P3):
	// 'a' is L, so the content is isolated LTR and the hyphen between the
	// letters stays in the LTR flow.
	assertVisual(t, "x\u2068a-b\u2069y", false, "xa-by")
}

// TestBidiExplicitEmbeddings verifies LRE/RLE/PDF (X2/X3/X7): the embedded
// text takes the embedding level even against its natural direction.
func TestBidiExplicitEmbeddings(t *testing.T) {
	// LRE forces an even (LTR) level around Hebrew in an LTR paragraph:
	// the single glyph keeps its place but never flips the run structure.
	assertVisual(t, "a\u202Aב\u202Cc", false, "aבc")
	// RLE forces an odd level around Latin in an LTR paragraph.
	assertVisual(t, "a\u202Bb\u202Cc", false, "abc")
}

// TestBidiLevelsN0Brackets inspects levels directly: brackets around
// opposite-direction content in an LTR paragraph resolve to level 0 (the
// embedding direction), not level 1.
func TestBidiLevelsN0Brackets(t *testing.T) {
	classes, offsets := classifyLine("a(בג)דה")
	runes := make([]rune, len(classes))
	for i, off := range offsets {
		runes[i] = bytesToRune("a(בג)דה", off)
	}
	levels := resolveBidiLevelsRunes(classes, runes, false)
	// a=0, (=0 (N0), ב=1, ג=1, )=0 (N0), ד=1, ה=1
	want := []int{0, 0, 1, 1, 0, 1, 1}
	for i, w := range want {
		if levels[i] != w {
			t.Fatalf("level[%d] = %d, want %d (levels: %v)", i, levels[i], w, levels)
		}
	}
}

// TestBidiIsolateLevels inspects levels: an RLI raises the embedding level
// (least greater odd from the base) and the PDI restores it; the isolated
// L rises one more level under I2 (odd embedding level + L char).
func TestBidiIsolateLevels(t *testing.T) {
	const line = "a\u2067b\u2069c"
	classes, offsets := classifyLine(line)
	runes := make([]rune, len(classes))
	for i, off := range offsets {
		runes[i] = bytesToRune(line, off)
	}
	levels := resolveBidiLevelsRunes(classes, runes, false)
	// a=0; RLI pushes an isolate at level 1; the isolated L 'b' rises to
	// level 2 under I2; after the PDI the paragraph level returns.
	if levels[2] != 2 {
		t.Fatalf("isolated b level = %d, want 2 (levels: %v)", levels[2], levels)
	}
	if levels[4] != 0 {
		t.Fatalf("c after PDI level = %d, want 0 (levels: %v)", levels[4], levels)
	}
}
