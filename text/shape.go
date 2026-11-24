package text

import (
	"math"
	"strconv"

	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/font"
	"github.com/FlameInTheDark/go-gpui/geometry"
	st "github.com/FlameInTheDark/go-gpui/style"
)

// TabWidthEm is the tab stop size in spaces of the current font (the upstream editor's
// default tab_size).
const TabWidthEm = 4

// ShapedGlyph is one positioned glyph (gpui's ShapedGlyph).
type ShapedGlyph struct {
	ID       uint32
	Position geometry.Point[geometry.Pixels]
	Index    int // UTF-8 byte offset in the line
	IsEmoji  bool
}

// ShapedRun groups glyphs sharing a font.
type ShapedRun struct {
	FontID FontID
	Glyphs []ShapedGlyph
}

// FontRun is a shaping input span (gpui's FontRun).
type FontRun struct {
	Len    int // UTF-8 bytes
	FontID FontID
}

// LineLayout is the shaped result of one line (gpui's LineLayout).
type LineLayout struct {
	FontSize  geometry.Pixels
	Width     geometry.Pixels
	Ascent    geometry.Pixels
	Descent   geometry.Pixels
	Runs      []ShapedRun
	Len       int // UTF-8 bytes of the source text
	WrapLines int // 1 when unwrapped
	Wrapped   *WrappedLineLayout

	// BidiSegments carries the visual-order directional segments when the
	// line was reordered (nil for pure-LTR lines, which keep the fast
	// logical-order index maps). BidiBaseRTL is the resolved base
	// direction (upstream: the shaper's bidi pass result).
	BidiSegments []Segment
	BidiBaseRTL  bool
}

// LayoutLine shapes a line of text with the given font runs. This is the
// pure-Go replacement for the harfbuzz/cosmic shaper: itemization picks
// per-character fonts (primary first, sticky fallback spans), GSUB
// substitutes ligatures/features, GPOS + 'kern' apply kerning, and glyph
// positions accumulate pen deltas like upstream.
//
// Results are cached: UI frames re-layout the same strings every redraw
// (an editor re-shapes all visible lines twice per frame), and the shaping
// cost is linear in line length with per-pair GPOS table walks. The
// returned *LineLayout is SHARED between callers and must be treated as
// immutable; callers that need to attach wrap state must clone first (see
// elements/text.go). Cache keys include everything the pure shaping reads;
// wrap width is applied by the caller and never reaches this cache.
func (s *System) LayoutLine(text string, fontSize geometry.Pixels, runs []FontRun) *LineLayout {
	key := layoutCacheKey(text, fontSize, runs)

	s.layoutCacheMU.RLock()
	if l, ok := s.layoutCache[key]; ok {
		s.layoutCacheMU.RUnlock()
		return l
	}
	s.layoutCacheMU.RUnlock()

	l := s.layoutLineUncached(text, fontSize, runs)

	s.layoutCacheMU.Lock()
	if len(s.layoutCache) >= layoutCacheLimit {
		s.layoutCache = map[string]*LineLayout{} // bulk reset; re-warms cheaply
	}
	s.layoutCache[key] = l
	s.layoutCacheMU.Unlock()
	return l
}

// wrapCacheKey identifies one wrap-boundaries computation. The shaped
// layout pointer stands in for its shaping inputs (text, size, font runs);
// entries remain valid across layout-cache resets because the pointer
// still identifies that exact shaped line.
type wrapCacheKey struct {
	line     *LineLayout
	wrapW    float32
	maxLines int
}

// WrapBoundaries returns the wrap boundaries for a shaped line at the
// given width, memoized per (layout, width, maxLines). Computing
// boundaries is quadratic in line length; without the cache a wrapped
// editor frame recomputed the identical result for every visible line on
// every redraw.
// WrapResult bundles a line's wrap boundaries with the continuation-row
// indent (upstream IndentAdjustment::SameIndent).
type WrapResult struct {
	Boundaries []WrapBoundary
	Indent     float32
}

func (s *System) WrapBoundaries(l *LineLayout, text string, wrapWidth geometry.Pixels, maxLines int) []WrapBoundary {
	return s.WrapBoundariesWithIndent(l, text, wrapWidth, maxLines).Boundaries
}

// WrapBoundariesWithIndent is WrapBoundaries plus the continuation-row
// indent (both cached together).
func (s *System) WrapBoundariesWithIndent(l *LineLayout, text string, wrapWidth geometry.Pixels, maxLines int) WrapResult {
	key := wrapCacheKey{line: l, wrapW: wrapWidth.AsF32(), maxLines: maxLines}

	s.wrapCacheMU.RLock()
	if b, ok := s.wrapCache[key]; ok {
		s.wrapCacheMU.RUnlock()
		return b
	}
	s.wrapCacheMU.RUnlock()

	w := &WrappedLineLayout{Unwrapped: l}
	boundaries := w.ComputeWrapBoundaries(text, wrapWidth, maxLines)
	result := WrapResult{Boundaries: boundaries, Indent: w.Indent}

	s.wrapCacheMU.Lock()
	if len(s.wrapCache) >= layoutCacheLimit {
		s.wrapCache = map[wrapCacheKey]WrapResult{} // bulk reset; re-warms cheaply
	}
	s.wrapCache[key] = result
	s.wrapCacheMU.Unlock()
	return result
}

// layoutCacheKey builds the cache key: size, font-run fingerprint, text.
// The fingerprint encodes run ids and byte lengths so distinct run splits
// never collide. Built with strconv appends into a reusable buffer: this
// runs for every visible text element on every frame (even on cache hits),
// so the previous fmt.Sprintf path showed up in frame profiles.
func layoutCacheKey(text string, fontSize geometry.Pixels, runs []FontRun) string {
	b := make([]byte, 0, 24+len(text)+8*len(runs))
	b = strconv.AppendInt(b, int64(int32(fontSize.AsF32()*4)), 10)
	if len(runs) == 1 {
		b = append(b, '|')
		b = strconv.AppendInt(b, int64(runs[0].FontID), 10)
		b = append(b, '|')
		b = strconv.AppendInt(b, int64(len(text)), 10)
		b = append(b, '|')
		return string(append(b, text...))
	}
	for _, r := range runs {
		b = append(b, '|')
		b = strconv.AppendInt(b, int64(r.FontID), 10)
		b = append(b, ':')
		b = strconv.AppendInt(b, int64(r.Len), 10)
	}
	b = append(b, '|')
	return string(append(b, text...))
}

func (s *System) layoutLineUncached(text string, fontSize geometry.Pixels, runs []FontRun) *LineLayout {
	// Expand font runs to per-rune font ids with fallback.
	runeFonts := s.itemize(text, runs)

	// Build clusters: one shaping unit per grapheme-ish cluster (base rune
	// + combining marks).
	clusters := splitClusters(text)

	layout := &LineLayout{FontSize: fontSize, Len: len(text)}
	maxAscent, maxDescent := geometry.Px(0), geometry.Px(0)

	// glyphEntry is one positioned glyph awaiting run assignment; attached
	// marks carry their base's entry index plus pixel offsets.
	type glyphEntry struct {
		run *ShapedRun
		g   ShapedGlyph
		pos float32
		adv float32
		// orig is the entry's original index (stable across the BiDi
		// reorder, used to remap mark-base references).
		orig int
		// Attachment: base is the index into entries (-1 unattached); dx/dy
		// are pixel offsets from the base's position (dy negative = up).
		base int
		dx   float32
		dy   float32
	}
	var entries []glyphEntry
	var runOrder []*ShapedRun

	// shapeSpan runs the OpenType pipeline over a same-font cluster span and
	// appends its glyphs to entries at the current pen position.
	shapeSpan := func(fontID FontID, cls []cluster, pen float32) float32 {
		if len(cls) == 0 {
			return pen
		}
		face := s.Face(fontID)
		if face == nil {
			return pen
		}
		run := &ShapedRun{FontID: fontID}
		runOrder = append(runOrder, run)

		sh := newShaper(face)
		for _, cl := range cls {
			for _, r := range text[cl.start:cl.end] {
				sh.buf = append(sh.buf, bufGlyph{gid: face.GlyphID(r), cluster: cl.start, src: r})
				if sh.script == "" {
					sh.script = scriptForRune(r)
				}
			}
		}
		sh.shape()

		upem := float32(face.UnitsPerEm)
		if upem == 0 {
			upem = 1000
		}
		scale := fontSize.AsF32() / upem
		isEmoji := s.IsEmojiFont(fontID)

		entryOf := make([]int, len(sh.buf))
		for i := range sh.buf {
			b := sh.buf[i]
			// Default-ignorable code points that survived shaping (an
			// unconsumed ZWJ or variation selector) never render.
			if isDefaultIgnorable(b.src) {
				entryOf[i] = -1
				continue
			}
			adv := (float32(face.Advance(b.gid)) + float32(b.xAdvance)) * scale
			if b.attached {
				// Attached marks take their position from the base; they do
				// not advance the pen.
				adv = 0
			}
			entryOf[i] = len(entries)
			entries = append(entries, glyphEntry{
				run: run,
				g: ShapedGlyph{
					ID:       uint32(b.gid),
					Position: geometry.Point[geometry.Pixels]{X: geometry.Px(pen + float32(b.xPlacement)*scale), Y: geometry.Px(-float32(b.yPlacement) * scale)},
					Index:    b.cluster,
					IsEmoji:  isEmoji,
				},
				pos:  pen + float32(b.xPlacement)*scale,
				adv:  adv,
				orig: len(entries),
				base: -1,
				dy:   -float32(b.yPlacement) * scale,
			})
			if b.attached {
				if e := entryOf[b.baseIdx]; e >= 0 {
					entries[len(entries)-1].base = e
					entries[len(entries)-1].dx = float32(b.attachDX)*scale + float32(b.xPlacement)*scale
					entries[len(entries)-1].dy = -(float32(b.attachDY) + float32(b.yPlacement)) * scale
				}
			}
			pen += adv
		}
		return pen
	}

	// Group consecutive same-font clusters into spans; tabs and BiDi
	// formatting controls interrupt a span (tabs jump the pen, controls are
	// invisible).
	pen := float32(0)
	spanFont := FontID(-1)
	var spanClusters []cluster
	flushSpan := func() {
		pen = shapeSpan(spanFont, spanClusters, pen)
		spanClusters = nil
	}
	for _, cl := range clusters {
		if isBidiFormattingString(text[cl.start:cl.end]) {
			continue
		}
		base := []rune(text[cl.start:cl.end])
		if len(base) == 1 && base[0] == '\t' {
			flushSpan()
			spanFont = FontID(-1)
			// Tabs advance to the next tab stop (TabWidthEm spaces, aligned
			// to the line origin); fonts have no usable '\t' glyph.
			fontID := runeFonts[cl.start]
			if face := s.Face(fontID); face != nil {
				m := s.Metrics(fontID)
				if a := geometry.Px(m.Ascent * fontSize.AsF32()); a > maxAscent {
					maxAscent = a
				}
				if d := geometry.Px(-m.Descent * fontSize.AsF32()); d > maxDescent {
					maxDescent = d
				}
				space := face.GlyphID(' ')
				spaceAdv := float32(face.Advance(space)) / float32(face.UnitsPerEm) * fontSize.AsF32()
				if spaceAdv <= 0 {
					spaceAdv = fontSize.AsF32() / 2
				}
				tabWidth := TabWidthEm * spaceAdv
				stops := pen / tabWidth
				pen = (float32(math.Floor(float64(stops))) + 1) * tabWidth
			}
			continue
		}
		fontID := runeFonts[cl.start]
		if fontID != spanFont {
			flushSpan()
			spanFont = fontID
		}
		spanClusters = append(spanClusters, cl)
		if face := s.Face(fontID); face != nil {
			m := s.Metrics(fontID)
			if a := geometry.Px(m.Ascent * fontSize.AsF32()); a > maxAscent {
				maxAscent = a
			}
			if d := geometry.Px(-m.Descent * fontSize.AsF32()); d > maxDescent {
				maxDescent = d
			}
		}
	}
	flushSpan()

	// --- BiDi: reorder into visual order (upstream: cosmic-text's
	// bidi pass inside the shaper). The base direction comes from the
	// first strong character; ReorderLine yields the visual segment
	// order. Pure-LTR lines keep the shaped order untouched; lines
	// containing RTL segments are laid out visually: segments
	// left-to-right, RTL segments' glyphs right-to-left. ---
	baseRTL := IsRTL(text)
	segs := ReorderLine(text, baseRTL)
	needsReorder := false
	for _, sg := range segs {
		if sg.RTL {
			needsReorder = true
		}
	}
	if needsReorder || baseRTL {
		layout.BidiSegments = segs
		layout.BidiBaseRTL = baseRTL
		inSegment := make([]int, 0, len(entries))
		visual := make([]glyphEntry, 0, len(entries))
		for _, sg := range segs {
			inSegment = inSegment[:0]
			for i := range entries {
				if entries[i].g.Index >= sg.Start && entries[i].g.Index < sg.End {
					inSegment = append(inSegment, i)
				}
			}
			if sg.RTL {
				// Right-to-left: the logically-first glyph lands rightmost.
				for j := len(inSegment) - 1; j >= 0; j-- {
					visual = append(visual, entries[inSegment[j]])
				}
			} else {
				for _, j := range inSegment {
					visual = append(visual, entries[j])
				}
			}
		}
		// Any entries outside the segments (e.g. tabs dropped from
		// segments) keep relative order at the end.
		covered := make([]bool, len(entries))
		for k := range visual {
			covered[visual[k].orig] = true
		}
		for i := range entries {
			if !covered[i] {
				visual = append(visual, entries[i])
			}
		}
		// Re-walk the visual order left-to-right. Attached marks do not
		// participate: they are positioned relative to their bases after
		// the walk, so cluster base+mark pairs stay attached regardless of
		// the direction. Base references are remapped through the
		// reorder (visual[k] came from entries[j]).
		remap := make([]int, len(entries))
		for i := range remap {
			remap[i] = -1
		}
		for k := range visual {
			remap[visual[k].orig] = k
		}
		p := float32(0)
		for i := range visual {
			if visual[i].base >= 0 {
				continue
			}
			visual[i].pos = p
			p += visual[i].adv
		}
		for i := range visual {
			if visual[i].base >= 0 {
				if nb := remap[visual[i].base]; nb >= 0 {
					visual[i].pos = visual[nb].pos + visual[i].dx
				}
			}
		}
		entries = visual
	} else {
		// No reorder: attached marks still resolve against their
		// bases' positions (the emit pass left them at the pen).
		for i := range entries {
			if entries[i].base >= 0 && entries[i].base < i {
				entries[i].pos = entries[entries[i].base].pos + entries[i].dx
			}
		}
	}

	// Fill runs in order (glyphs carry shaped, kern-adjusted positions;
	// attached marks resolve their x from the base and their y offset).
	for i := range entries {
		entries[i].g.Position.X = geometry.Px(entries[i].pos)
		entries[i].g.Position.Y = geometry.Px(entries[i].dy)
		entries[i].run.Glyphs = append(entries[i].run.Glyphs, entries[i].g)
	}
	layout.Runs = nil
	for _, r := range runOrder {
		if len(r.Glyphs) > 0 {
			layout.Runs = append(layout.Runs, *r)
		}
	}

	layout.Width = geometry.Px(pen)
	layout.Ascent = maxAscent
	layout.Descent = -maxDescent
	return layout
}

// itemize assigns each rune a font id: the run's font if it covers the
// rune, else the first covering fallback (sticky spans like cosmic-text).
func (s *System) itemize(text string, runs []FontRun) []FontID {
	out := make([]FontID, len(text))
	var fallbacks []FontID
	// Pre-resolve fallback candidates: the run fonts + system stack.
	for _, r := range runs {
		fallbacks = append(fallbacks, r.FontID)
	}

	i := 0
	for _, run := range runs {
		start := i
		end := start + run.Len
		if end > len(text) {
			end = len(text)
		}
		current := run.FontID
		for bi := start; bi < end; {
			r, size := decodeRune(text[bi:])
			if size == 0 {
				break
			}
			if s.covers(current, r) {
				out[bi] = current
			} else {
				// Sticky: keep the current fallback if it still covers;
				// else pick the first covering candidate.
				if !s.coversSticky(current, r) {
					if next, ok := s.pickFallback(fallbacks, run.FontID, r); ok {
						current = next
					} else if id, ok := s.systemFallback(r); ok {
						current = id
					}
				}
				out[bi] = current
			}
			for j := bi + 1; j < bi+size && j < end; j++ {
				out[j] = current
			}
			bi += size
		}
		i = end
		if i >= len(text) {
			break
		}
	}
	// Tail bytes without a run: default font.
	for j := 0; j < len(out); j++ {
		if out[j] == 0 && !s.covers(0, rune(text[j])) {
			if id, ok := s.DefaultFontID(); ok {
				out[j] = id
			}
		}
	}
	return out
}

func (s *System) covers(id FontID, r rune) bool {
	face := s.Face(id)
	if face == nil {
		return false
	}
	// Format controls (tab, LF, CR) stay with the run's font: most cmaps
	// have no entry for them, and letting them fall back to another face
	// would break tab-stop math (the tab advance must come from the same
	// font as the surrounding text).
	if r == '\t' || r == '\n' || r == '\r' {
		return true
	}
	return face.HasGlyph(r)
}

func (s *System) coversSticky(id FontID, r rune) bool {
	return s.covers(id, r)
}

func (s *System) pickFallback(candidates []FontID, primary FontID, r rune) (FontID, bool) {
	// Memoize resolution: an uncovered rune otherwise rescans every
	// candidate face's cmap on every occurrence. Cache keys include the
	// primary, since ASCII coverage short-circuits to it.
	key := fallbackKey{r: r, primary: primary}
	s.mu.Lock()
	if id, ok := s.fallbackCache[key]; ok {
		s.mu.Unlock()
		return id, true
	}
	s.mu.Unlock()

	id, ok := s.pickFallbackUncached(candidates, primary, r)
	if ok {
		s.mu.Lock()
		// Bounded: identical to the layout-cache reset strategy.
		if len(s.fallbackCache) >= layoutCacheLimit {
			s.fallbackCache = map[fallbackKey]FontID{}
		}
		s.fallbackCache[key] = id
		s.mu.Unlock()
	}
	return id, ok
}

func (s *System) pickFallbackUncached(candidates []FontID, primary FontID, r rune) (FontID, bool) {
	// ASCII and covered runes stay with the primary font.
	if r < 0x80 && s.covers(primary, r) {
		return primary, true
	}
	for _, id := range candidates {
		if s.covers(id, r) {
			return id, true
		}
	}
	return 0, false
}

// systemFallback scans every registered face for coverage.
//
// Faces are lazy, so the scan prefilters with the probed cmap coverage
// ranges: faces whose ranges exclude the rune are skipped WITHOUT reading
// their files (an eager scan would materialize every system font). Faces
// that pass the prefilter are materialized and checked exactly (ranges
// are conservative; the cmap may still map the rune to glyph 0).
func (s *System) systemFallback(r rune) (FontID, bool) {
	// Prefer emoji faces for emoji-ish runes.
	if r >= 0x1F000 {
		s.mu.Lock()
		first := -1
		for i, lf := range s.faces {
			if lf.isEmoji && s.faceMightCoverLocked(i, r) {
				first = i
				break
			}
		}
		s.mu.Unlock()
		if first >= 0 && s.covers(FontID(first), r) {
			return FontID(first), true
		}
	}
	s.mu.Lock()
	var candidates []int
	for i := range s.faces {
		if s.faceMightCoverLocked(i, r) {
			candidates = append(candidates, i)
		}
	}
	s.mu.Unlock()
	for _, i := range candidates {
		if s.covers(FontID(i), r) {
			return FontID(i), true
		}
	}
	return 0, false
}

// faceMightCoverLocked reports whether face i plausibly covers r without
// materializing it: exact when the face is already loaded, probed-range
// based otherwise. Caller holds s.mu.
func (s *System) faceMightCoverLocked(i int, r rune) bool {
	lf := s.faces[i]
	if lf.face != nil {
		return lf.face.HasGlyph(r)
	}
	return rangesContain(lf.coverage, r)
}

// decodeRune decodes one UTF-8 rune (tolerant).
func decodeRune(s string) (rune, int) {
	if len(s) == 0 {
		return 0, 0
	}
	b := s[0]
	switch {
	case b < 0x80:
		return rune(b), 1
	case b < 0xC0:
		return rune(b), 1 // invalid byte
	case b < 0xE0:
		if len(s) < 2 {
			return rune(b), 1
		}
		return rune(b&0x1F)<<6 | rune(s[1]&0x3F), 2
	case b < 0xF0:
		if len(s) < 3 {
			return rune(b), 1
		}
		return rune(b&0x0F)<<12 | rune(s[1]&0x3F)<<6 | rune(s[2]&0x3F), 3
	default:
		if len(s) < 4 {
			return rune(b), 1
		}
		return rune(b&0x07)<<18 | rune(s[1]&0x3F)<<12 | rune(s[2]&0x3F)<<6 | rune(s[3]&0x3F), 4
	}
}

// cluster is a shaping unit: base rune + trailing combining marks.
type cluster struct {
	start, end int // byte range
}

// splitClusters groups combining marks with their base runes (a compact
// grapheme approximation: Mn/Me/Mc ranges + ZWJ sequences).
func splitClusters(text string) []cluster {
	var out []cluster
	i := 0
	for i < len(text) {
		r, size := decodeRune(text[i:])
		if size == 0 {
			break
		}
		start := i
		i += size
		// Absorb combining marks and ZWJ continuations.
		for i < len(text) {
			m, mSize := decodeRune(text[i:])
			if isCombiningMark(m) || m == 0x200D {
				i += mSize
			} else {
				break
			}
		}
		_ = r
		out = append(out, cluster{start: start, end: i})
	}
	return out
}

// isCombiningMark approximates Unicode Mn/Me/Mc.
func isCombiningMark(r rune) bool {
	switch {
	case r >= 0x0300 && r <= 0x036F, // combining diacriticals
		r >= 0x0483 && r <= 0x0489,
		r >= 0x0591 && r <= 0x05BD,
		r >= 0x0610 && r <= 0x061A,
		r >= 0x064B && r <= 0x065F,
		r >= 0x0670 && r <= 0x0670,
		r >= 0x06D6 && r <= 0x06DC,
		r >= 0x0E31 && r <= 0x0E3A,
		r >= 0x0E47 && r <= 0x0E4E,
		r >= 0x1AB0 && r <= 0x1AFF,
		r >= 0x1DC0 && r <= 0x1DFF,
		r >= 0x20D0 && r <= 0x20F0,
		r >= 0xFE00 && r <= 0xFE0F, // variation selectors
		r >= 0xFE20 && r <= 0xFE2F:
		return true
	}
	return false
}

// TextRun is gpui's TextRun: one styled span for painting.
type TextRun struct {
	Len             int // UTF-8 bytes
	Font            font.Font
	Color           gc.Hsla
	BackgroundColor *gc.Hsla
	Underline       *st.UnderlineStyle
	Strikethrough   *st.StrikethroughStyle
}

// scriptForRune maps a rune to its OpenType script tag (the first strong
// script of a span selects its feature records; common/inherited characters
// return "" so marks and punctuation defer to the surrounding text).
func scriptForRune(r rune) string {
	switch {
	case r >= 0x0041 && r <= 0x007A, // Basic Latin letters
		r >= 0x00C0 && r <= 0x024F, // Latin-1 Supplement + Extended A/B
		r >= 0x1E00 && r <= 0x1EFF: // Latin Extended Additional
		return "latn"
	case r >= 0x0370 && r <= 0x03FF:
		return "grek"
	case r >= 0x0400 && r <= 0x04FF:
		return "cyrl"
	case r >= 0x0590 && r <= 0x05FF:
		return "hebr"
	case r >= 0x0600 && r <= 0x06FF, // Arabic
		r >= 0x0750 && r <= 0x077F,
		r >= 0xFB50 && r <= 0xFDFF, // presentation forms
		r >= 0xFE70 && r <= 0xFEFF:
		return "arab"
	case r >= 0x0900 && r <= 0x097F:
		return "deva"
	case r >= 0x0E00 && r <= 0x0E7F:
		return "thai"
	case r >= 0x3040 && r <= 0x30FF:
		return "kana"
	case r >= 0x3400 && r <= 0x4DBF, r >= 0x4E00 && r <= 0x9FFF:
		return "hani"
	}
	return ""
}
