package elements

import (
	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/font"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/layout"
	"github.com/FlameInTheDark/go-gpui/style"
	"github.com/FlameInTheDark/go-gpui/text"
)

// Re-exported style enums used by the generated forwarders.
type (
	TextOverflow = style.TextOverflow
	Overflow     = style.Overflow
	BoxShadow    = style.BoxShadow
)

// Text renders a styled string (upstream elements/text.rs, condensed).
type Text struct {
	value         string
	id            gpui.ElementId
	colorOverride *gc.Hsla
	// width, when set, gives a text block a definite layout width so its
	// wrapped paint bounds match the width used during measurement.
	width *geometry.Pixels
	// runs overrides the window text style with explicit spans.
	runs []text.TextRun
	// highlight applies a highlight over ranges.
	highlights []TextHighlight
	// markedRange is the IME preedit (marked text) range, painted with
	// the platform inline-IME highlight: a translucent background over
	// the marked text plus a stronger one for the selected clause
	// (upstream InputHandler::marked_range rendering inside the text
	// element).
	markedRange   [2]int
	selectedRange [2]int
	hasMarked     bool
	hasSelected   bool
	caretIndex    int
	caretColor    *gc.Hsla
	// layout cache per frame
	layout *text.LineLayout
	// shapeMemo caches this frame's shaping: elements are measured
	// several times per frame (flex base size, cross size, final
	// layout) and then prepainted; shaping is deterministic for the
	// same inputs, so the repeated calls reuse the first result.
	memo *shapeMemo
	// meta is the run metadata of the memoized shaping (fonts +
	// lengths; colors never affect layout).
	meta []runMeta
}

// shapeMemo is the per-instance shaping cache key + result.
type shapeMemo struct {
	value    string
	fontSize float32
	wrapSet  bool
	wrapW    float32
	clamp    int
	layout   *text.LineLayout
}

// TextHighlight is a (range, style) overlay.
type TextHighlight struct {
	Start, End int
	Style      style.HighlightStyle
}

// NewText creates a text element.
func NewText(value string) *Text {
	return &Text{value: value}
}

// NewTextWithID creates a text element with an explicit id.
func NewTextWithID(id gpui.ElementId, value string) *Text {
	return &Text{value: value, id: id}
}

// WithRuns sets explicit runs.
func (t *Text) WithRuns(runs []text.TextRun) *Text {
	t.runs = runs
	return t
}

// WithCaret paints a caret at a byte boundary in single-line text. The
// caret and glyphs share the same shaping and origin. A narrow, clipped
// field pans the line as needed to keep the caret visible.
func (t *Text) WithCaret(byteIndex int, color gc.Hsla) *Text {
	t.caretIndex, t.caretColor = byteIndex, &color
	return t
}

// WithWidth sets a definite text layout width in logical pixels. This is
// useful for wrapped text in containers whose intrinsic width is otherwise
// larger than the available viewport.
func (t *Text) WithWidth(width geometry.Pixels) *Text {
	if width < 0 {
		width = 0
	}
	t.width = &width
	return t
}

// WithMarkedRange sets the IME marked-text (preedit) range and the
// selected clause inside it (upstream marked_range/selected_range on the
// input handler): the marked text paints with the inline-IME highlight
// background and the selected clause with a stronger one, like macOS
// inline composition. Ranges are byte offsets into the value.
func (t *Text) WithMarkedRange(marked [2]int, selected [2]int) *Text {
	t.markedRange = marked
	t.selectedRange = selected
	t.hasMarked = true
	if selected[0] < selected[1] {
		t.hasSelected = true
	}
	return t
}

// styleRuns resolves the runs used for painting: the explicit runs when set,
// else a single run from the window text style. WithTextColor overrides the
// color of the fallback run (it has no effect on explicit runs, which carry
// their own colors).
func (t *Text) styleRuns(ts style.TextStyle, value string) []text.TextRun {
	if t.runs != nil {
		return t.runs
	}
	r := runOf(ts, value)
	if t.colorOverride != nil {
		r.Color = *t.colorOverride
	}
	return []text.TextRun{r}
}

// WithDefaultHighlights computes runs from highlights over a base style.
func (t *Text) WithDefaultHighlights(base style.TextStyle, highlights []TextHighlight) *Text {
	t.highlights = highlights
	// Build runs eagerly like upstream.
	ts := base
	var runs []text.TextRun
	pos := 0
	for _, h := range highlights {
		if h.Start > pos {
			runs = append(runs, runOf(ts, t.value[pos:h.Start]))
		}
		hl := ts.Highlight(h.Style)
		runs = append(runs, runOf(hl, t.value[h.Start:h.End]))
		pos = h.End
	}
	if pos < len(t.value) {
		runs = append(runs, runOf(ts, t.value[pos:]))
	}
	t.runs = runs
	return t
}

func runOf(ts style.TextStyle, s string) text.TextRun {
	r := text.TextRun{Len: len(s), Font: ts.ToFont(), Color: ts.Color}
	if ts.BackgroundColor != nil {
		r.BackgroundColor = ts.BackgroundColor
	}
	if ts.Underline != nil {
		r.Underline = ts.Underline
	}
	if ts.Strikethrough != nil {
		r.Strikethrough = ts.Strikethrough
	}
	return r
}

// ElementID implements Element.
func (t *Text) ElementID() gpui.ElementId { return t.id }

// RequestLayout implements Element.
func (t *Text) RequestLayout(w *gpui.Window) {
	ts := w.TextStyle()
	st := style.DefaultStyle()
	st.Text = ts
	if t.width != nil {
		st.Size.Width = geometry.LengthFromPixels(*t.width)
	}
	if ts.WhiteSpace != style.WhiteSpaceNormal {
		st.Overflow[0] = style.OverflowHidden
	}
	if t.caretColor != nil {
		st.MinSize.Width = geometry.LengthFromPixels(0)
		st.FlexGrow = 1 // the caret can use the field's spare space after the last glyph
	}
	w.RequestMeasuredLayout(st, func(known layout.SizeDimension, available layout.AvailableSpace) layout.SizeResult {
		l := t.shape(w, ts, known, available)
		return layout.SizeResult{Width: l.Width.AsF32(), Height: lineHeights(l, ts, w.RemSize())}
	})
}

func lineHeights(l *text.LineLayout, ts style.TextStyle, rem geometry.Pixels) float32 {
	lh := ts.LineHeightInPixels(rem)
	if l.WrapLines > 1 {
		return lh * float32(l.WrapLines)
	}
	return lh
}

// shape computes the line layout (with wrap boundaries). LayoutLine
// results are shared and cached by the text system, wrap boundaries are
// cached against the shaped layout, and this frame's result is memoized on
// the element (the flexbox algorithm measures the same element several
// times per frame and then prepaints it), so repeated per-frame calls are
// cheap comparisons. The cached layout itself stays immutable; the wrap
// state is attached to a clone.
//
// ts is the text style CAPTURED when the element requested layout - the
// measure closure runs during ComputeLayout, after the ancestor div's
// WithTextStyle has been popped, so re-reading w.TextStyle() there shaped
// every styled text (TextSize/FontFamily refinements) at the default size:
// the width, wrap count and line heights then disagreed with the painted
// result (a TextSize(22) title measured as 16px, wrapped in layout and
// painted 26px-tall lines over the elements below it).
func (t *Text) shape(w *gpui.Window, ts style.TextStyle, known layout.SizeDimension, available layout.AvailableSpace) *text.LineLayout {
	sys := w.TextSystem()
	if sys == nil {
		return &text.LineLayout{}
	}
	fontSize := ts.FontSize.ToPixels(w.RemSize())

	// Wrapping width (None = unwrapped).
	var wrapWidth *geometry.Pixels
	if ts.WhiteSpace == style.WhiteSpaceNormal {
		if wd, ok := known.Width.Resolve(available.Width); ok {
			wp := geometry.Px(wd)
			wrapWidth = &wp
		} else if w, definite := available.IsDefiniteAxis(true); definite {
			wp := geometry.Px(w)
			wrapWidth = &wp
		}
	}
	maxLines := 0
	if ts.LineClamp != nil {
		maxLines = *ts.LineClamp
	}
	// Memo pre-check on run METADATA (fonts + lengths; colors never
	// affect layout). Checked before building FontRuns so repeat calls
	// skip the FontID lookups entirely. With explicit runs the metadata
	// captured last call is compared against the runs; without them the
	// single fallback run is compared against the window text style.
	if t.memo != nil && runMetaMatches(t.meta, t.runs, ts, t.value) &&
		t.memo.value == t.value && t.memo.fontSize == fontSize.AsF32() &&
		t.memo.wrapSet == (wrapWidth != nil) &&
		(!t.memo.wrapSet || t.memo.wrapW == wrapWidth.AsF32()) &&
		t.memo.clamp == maxLines {
		l := *t.memo.layout // fresh clone per call: WrapLines/Wrapped are per-caller state
		return &l
	}

	// Resolve runs (color not needed for metrics, but kept consistent).
	runs := t.styleRuns(ts, t.value)
	// FontRuns from runs (merge same-font neighbors). FontID is a cached
	// system lookup.
	var fontRuns []text.FontRun
	for _, r := range runs {
		id, _ := sys.FontID(r.Font)
		if len(fontRuns) > 0 && fontRuns[len(fontRuns)-1].FontID == id {
			fontRuns[len(fontRuns)-1].Len += r.Len
		} else {
			fontRuns = append(fontRuns, text.FontRun{Len: r.Len, FontID: id})
		}
	}

	shaped := sys.LayoutLine(t.value, fontSize, fontRuns)
	l := *shaped // clone: WrapLines/Wrapped are per-caller state

	wrapped := &text.WrappedLineLayout{Unwrapped: shaped, WrapWidth: wrapWidth}
	if wrapWidth != nil {
		wr := sys.WrapBoundariesWithIndent(shaped, t.value, *wrapWidth, maxLines)
		wrapped.WrapBoundaries = wr.Boundaries
		wrapped.Indent = wr.Indent
	}
	l.WrapLines = len(wrapped.WrapBoundaries) + 1
	l.Wrapped = wrapped

	t.meta = captureRunMeta(runs)
	t.memo = &shapeMemo{
		value:    t.value,
		fontSize: fontSize.AsF32(),
		wrapSet:  wrapWidth != nil,
		wrapW: func() float32 {
			if wrapWidth != nil {
				return wrapWidth.AsF32()
			}
			return 0
		}(),
		clamp:  maxLines,
		layout: &l,
	}
	return &l
}

// runMeta is the layout-relevant part of one run (font identity + length).
type runMeta struct {
	family string
	weight font.FontWeight
	style  font.FontStyle
	length int
}

func captureRunMeta(runs []text.TextRun) []runMeta {
	m := make([]runMeta, len(runs))
	for i, r := range runs {
		m[i] = runMeta{family: r.Font.Family, weight: r.Font.Weight, style: r.Font.Style, length: r.Len}
	}
	return m
}

// runMetaMatches reports whether the runs that produced the memo are
// layout-identical to the ones this call would use. Colors are irrelevant
// to shaping; fonts and byte lengths are not.
func runMetaMatches(meta []runMeta, runs []text.TextRun, ts style.TextStyle, value string) bool {
	if meta == nil {
		return false
	}
	if runs == nil {
		// styleRuns falls back to a single run from the text style.
		if len(meta) != 1 {
			return false
		}
		return meta[0].family == ts.FontFamily &&
			meta[0].weight == ts.FontWeight &&
			meta[0].style == ts.FontStyle &&
			meta[0].length == len(value)
	}
	if len(meta) != len(runs) {
		return false
	}
	for i := range runs {
		if meta[i].family != runs[i].Font.Family ||
			meta[i].weight != runs[i].Font.Weight ||
			meta[i].style != runs[i].Font.Style ||
			meta[i].length != runs[i].Len {
			return false
		}
	}
	return true
}

// Prepaint implements Element.
func (t *Text) Prepaint(w *gpui.Window, bounds geometry.Bounds[geometry.Pixels]) {
	gpui.UseElementState(w, func() *text.LineLayout { return nil }, func(s **text.LineLayout) {
		*s = t.shape(w, w.TextStyle(), layout.SizeDimension{Width: layout.LengthDimension(bounds.Size.Width.AsF32())}, layout.DefiniteAvailable(bounds.Size.Width.AsF32(), bounds.Size.Height.AsF32()))
		t.layout = *s
	})
}

// Paint implements Element.
func (t *Text) Paint(w *gpui.Window) {
	if t.layout == nil {
		return
	}
	// Accessibility: static text nodes carry their value (upstream
	// accesskit StaticText).
	idx := w.AppendA11yNode(gpui.AccessibilityNode{
		Role:   gpui.AccessibilityStaticText,
		Label:  t.value,
		Value:  t.value,
		Bounds: w.ElementBounds(t),
	})
	w.PushA11yParent(idx)
	w.PopA11yParent(idx)
	ts := w.TextStyle()
	bounds := w.ElementBounds(t)
	rem := w.RemSize()
	lineHeight := geometry.Px(ts.LineHeightInPixels(rem))
	sys := w.TextSystem()

	// Truncation.
	value := t.value
	truncated := false
	if ts.TextOverflow != nil && t.layout.Width.AsF32() > bounds.Size.Width.AsF32()+0.5 {
		truncated = true
		value = truncateString(value, t.layout, bounds.Size.Width, ts.TextOverflow.Affix)
		t.layout = nil
		// Reshape the truncated string.
		runs := t.styleRuns(ts, value)
		var fontRuns []text.FontRun
		for _, r := range runs {
			id, _ := sys.FontID(r.Font)
			if len(fontRuns) > 0 && fontRuns[len(fontRuns)-1].FontID == id {
				fontRuns[len(fontRuns)-1].Len += r.Len
			} else {
				fontRuns = append(fontRuns, text.FontRun{Len: r.Len, FontID: id})
			}
		}
		t.layout = sys.LayoutLine(value, ts.FontSize.ToPixels(rem), fontRuns)
	}

	// Align.
	origin := bounds.Origin
	switch ts.TextAlign {
	case style.TextAlignCenter:
		origin.X += geometry.Px((bounds.Size.Width.AsF32() - t.layout.Width.AsF32()) / 2)
	case style.TextAlignRight:
		origin.X += geometry.Px(bounds.Size.Width.AsF32() - t.layout.Width.AsF32())
	}
	const caretWidth = geometry.Pixels(1.5)
	showCaret := t.caretColor != nil && !truncated && t.layout.WrapLines <= 1
	var caretX geometry.Pixels
	if showCaret {
		idx := clampInt(t.caretIndex, 0, len(value))
		caretX = origin.X + t.layout.XForIndex(idx) - t.layout.XForIndex(0)
		if caretX > bounds.Origin.X && t.layout.Width+caretWidth > bounds.Size.Width {
			if shift := caretX - max(bounds.Origin.X, bounds.Right()-caretWidth); shift > 0 {
				origin.X -= shift
				caretX -= shift
			}
		}
	}

	// Paint each run's glyphs with its decoration run color.
	ascent := t.layout.Ascent
	descent := t.layout.Descent
	paddingTop := geometry.Px((lineHeight.AsF32() - ascent.AsF32() + descent.AsF32()) / 2)
	baseline := paddingTop + ascent

	// Walk decoration runs (the t.runs split when present, with the
	// WithTextColor override applied to the fallback).
	decoRuns := t.styleRuns(ts, value)
	if truncated && len(decoRuns) > 0 {
		// Scale runs to the truncated text.
		total := 0
		for _, r := range decoRuns {
			total += r.Len
		}
		if total > len(value) {
			scaled := len(value)
			for i := range decoRuns {
				if decoRuns[i].Len > scaled {
					decoRuns[i].Len = scaled
				}
				scaled -= decoRuns[i].Len
				if scaled < 0 {
					scaled = 0
				}
			}
		}
	}

	lineOrigin := origin
	var rowRanges [][2]int
	if t.layout.Wrapped != nil {
		rowRanges = t.layout.Wrapped.RowRanges(value)
	} else {
		rowRanges = [][2]int{{0, len(value)}}
	}
	for row := 0; row < t.layout.WrapLines && row < len(rowRanges); row++ {
		rr := rowRanges[row]
		rowIndent := geometry.Px(0)
		if row > 0 && t.layout.Wrapped != nil {
			rowIndent = geometry.Px(t.layout.Wrapped.Indent)
		}
		rowOrigin := lineOrigin
		rowOrigin.X = rowOrigin.X.Add(rowIndent)
		paintLine(w, sys, t.layout, decoRuns, rowOrigin, baseline, lineHeight, ts, rem, value, rr[0], rr[1])
		// IME marked text (preedit): a translucent highlight over
		// the marked range and a stronger one over the selected
		// clause (upstream marked_range/selected_range painting).
		if t.hasMarked {
			rowBaseX := t.layout.XForIndex(rr[0]).AsF32()
			markedX := t.layout.XForIndex(clampInt(t.markedRange[0], rr[0], rr[1])).AsF32() - rowBaseX
			markedEnd := t.layout.XForIndex(clampInt(t.markedRange[1], rr[0], rr[1])).AsF32() - rowBaseX
			if markedEnd > markedX {
				st := style.DefaultStyle()
				bg := gc.SolidBackground(gc.Hsla{H: 210, S: 1, L: 0.5, A: 0.25})
				st.Background = &bg
				w.PaintQuad(st, geometry.Bounds[geometry.Pixels]{
					Origin: geometry.Point[geometry.Pixels]{X: rowOrigin.X.Add(geometry.Px(markedX)), Y: rowOrigin.Y},
					Size:   geometry.Size[geometry.Pixels]{Width: geometry.Px(markedEnd - markedX), Height: lineHeight},
				})
			}
			if t.hasSelected {
				selX := t.layout.XForIndex(clampInt(t.selectedRange[0], rr[0], rr[1])).AsF32() - rowBaseX
				selEnd := t.layout.XForIndex(clampInt(t.selectedRange[1], rr[0], rr[1])).AsF32() - rowBaseX
				if selEnd > selX {
					st := style.DefaultStyle()
					bg := gc.SolidBackground(gc.Hsla{H: 210, S: 1, L: 0.5, A: 0.45})
					st.Background = &bg
					w.PaintQuad(st, geometry.Bounds[geometry.Pixels]{
						Origin: geometry.Point[geometry.Pixels]{X: rowOrigin.X.Add(geometry.Px(selX)), Y: rowOrigin.Y},
						Size:   geometry.Size[geometry.Pixels]{Width: geometry.Px(selEnd - selX), Height: lineHeight},
					})
				}
			}
		}
		lineOrigin.Y += lineHeight
	}
	if showCaret {
		height := min(lineHeight, t.layout.FontSize)
		st := style.DefaultStyle()
		bg := gc.SolidBackground(*t.caretColor)
		st.Background = &bg
		w.PaintQuad(st, geometry.Bounds[geometry.Pixels]{
			Origin: geometry.Point[geometry.Pixels]{X: caretX, Y: origin.Y + (lineHeight-height)/2},
			Size:   geometry.Size[geometry.Pixels]{Width: caretWidth, Height: height},
		})
	}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func paintLine(w *gpui.Window, sys *text.System, l *text.LineLayout, runs []text.TextRun,
	origin geometry.Point[geometry.Pixels], baseline, lineHeight geometry.Pixels, ts style.TextStyle, rem geometry.Pixels, value string,
	rowStart, rowEnd int) {

	// Underline y (upstream underline_y_offset).
	underlineY := geometry.Px((lineHeight.AsF32()-l.Ascent.AsF32()+l.Descent.AsF32())/2 + l.Ascent.AsF32() + l.Descent.AsF32()*0.618)

	// rowX maps an unwrapped-layout byte index to this wrapped row's
	// x: the row's glyphs keep their absolute positions; the row's own
	// first glyph anchors at the row origin.
	rowBaseX := l.XForIndex(rowStart).AsF32()
	if rowStart >= rowEnd {
		rowBaseX = 0
	}
	rowX := func(ix int) geometry.Pixels {
		return geometry.Px(l.XForIndex(ix).AsF32() - rowBaseX)
	}
	// Pass 1: run backgrounds. They must be painted BEFORE the glyphs:
	// selection and match highlights are translucent backgrounds over the
	// text, not overlays that hide it (upstream paints the background
	// layer first, then the glyph layer on top). A run is clipped to the
	// row's byte range: wrapped rows paint only their slice.
	runStart := 0
	for _, run := range runs {
		runEnd := runStart + run.Len
		if rs := maxIntLocal(runStart, rowStart); rs < runEnd {
			re := runEnd
			if rowEnd < re {
				re = rowEnd
			}
			startX := rowX(rs)
			endX := rowX(re)
			if run.BackgroundColor != nil {
				width := geometry.Px(endX.AsF32() - startX.AsF32())
				b := geometry.Bounds[geometry.Pixels]{
					Origin: geometry.Point[geometry.Pixels]{X: origin.X.Add(startX), Y: origin.Y},
					Size:   geometry.Size[geometry.Pixels]{Width: width, Height: lineHeight},
				}
				st := style.DefaultStyle()
				bg := gc.SolidBackground(*run.BackgroundColor)
				st.Background = &bg
				w.PaintQuad(st, b)
			}
		}
		runStart = runEnd
		if runStart >= len(value) && len(value) > 0 {
			break
		}
	}

	// Pass 2: glyphs and decorations, in run order (filtered to the row).
	runStart = 0
	for _, run := range runs {
		runEnd := runStart + run.Len
		rs := runStart
		if rs < rowStart {
			rs = rowStart
		}
		re := runEnd
		if rowEnd < re {
			re = rowEnd
		}
		if rs < re {
			startX := rowX(rs)
			endX := rowX(re)
			for _, sr := range l.Runs {
				for _, g := range sr.Glyphs {
					if g.Index < rs || g.Index >= re {
						continue
					}
					pen := geometry.Point[geometry.Pixels]{X: origin.X.Add(g.Position.X).Sub(geometry.Px(rowBaseX)), Y: origin.Y.Add(baseline).Add(g.Position.Y)}
					w.PaintGlyph(pen, sr.FontID, g.ID, l.FontSize, run.Color)
				}
			}
			width := geometry.Px(endX.AsF32() - startX.AsF32())
			left := geometry.Point[geometry.Pixels]{X: origin.X.Add(startX), Y: origin.Y}
			if run.Underline != nil {
				thickness := geometry.Px(run.Underline.Thickness)
				if thickness.AsF32() <= 0 {
					thickness = geometry.Px(1)
				}
				c := run.Color
				if run.Underline.Color != nil {
					c = *run.Underline.Color
				}
				w.PaintUnderline(
					geometry.Point[geometry.Pixels]{X: left.X, Y: origin.Y + underlineY},
					width, thickness, style.UnderlineStyle{Thickness: run.Underline.Thickness, Color: &c, Wavy: run.Underline.Wavy})
			}
			if run.Strikethrough != nil {
				c := run.Color
				if run.Strikethrough.Color != nil {
					c = *run.Strikethrough.Color
				}
				strikeY := origin.Y + baseline.DivScalar(2)
				w.PaintUnderline(
					geometry.Point[geometry.Pixels]{X: left.X, Y: strikeY},
					width, geometry.Px(run.Strikethrough.Thickness), style.UnderlineStyle{Thickness: run.Strikethrough.Thickness, Color: &c})
			}
		}
		runStart = runEnd
		if runStart >= len(value) && len(value) > 0 {
			break
		}
	}
	_ = font.WeightNormal
	_ = gc.Black()
}

func maxIntLocal(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// truncateString cuts the text to fit a width with an affix.
func truncateString(value string, l *text.LineLayout, width geometry.Pixels, affix string) string {
	if l == nil || width.AsF32() <= 0 {
		return value
	}
	idx, ok := l.IndexForX(width)
	if !ok {
		idx = l.Len
	}
	if idx <= 0 || idx >= len(value) {
		// Try to leave room for the affix by binary search.
		lo, hi := 0, len(value)
		for lo < hi {
			mid := (lo + hi) / 2
			if l.XForIndex(mid).AsF32() <= width.AsF32() {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		idx = lo
	}
	if idx >= len(value) {
		return value
	}
	// Reserve affix width approximately by slicing bytes to runes.
	runes := []rune(value[:idx])
	affixRunes := len([]rune(affix))
	if affixRunes < len(runes) {
		runes = runes[:len(runes)-affixRunes]
	}
	return string(runes) + affix
}

var _ = geometry.Px(0)

// WithTextColor sets the text color.
func (t *Text) WithTextColor(c gc.Hsla) *Text {
	t.colorOverride = &c
	return t
}

// Layout returns the most recent shaping of this element (test/telemetry
// hook; nil before the first paint).
func (t *Text) Layout() *text.LineLayout { return t.layout }
