package style

import (
	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/font"
	"github.com/FlameInTheDark/go-gpui/geometry"
)

// UnderlineStyle describes a text underline.
type UnderlineStyle struct {
	// Thickness is the line thickness in logical pixels.
	Thickness float32
	// Color is the line color; nil uses the text color.
	Color *gc.Hsla
	// Wavy draws a squiggly line instead of a straight one.
	Wavy bool
}

// StrikethroughStyle describes a line drawn through the middle of text.
type StrikethroughStyle struct {
	// Thickness is the line thickness in logical pixels.
	Thickness float32
	// Color is the line color; nil uses the text color.
	Color *gc.Hsla
}

// TextStyle is the concrete text style (the "resolved" form). It mirrors
// upstream TextStyle, which is refineable; [TextStyleRefinement] is the
// partial-update view used while cascading. Text style is inherited: a
// descendant starts from its parent's resolved TextStyle and applies its own
// refinement on top.
//
// Start from [DefaultTextStyle]; the zero value has transparent-black text
// (alpha 0), an empty family and a zero font size.
type TextStyle struct {
	// Color is the text (foreground) color.
	Color gc.Hsla
	// FontFamily is the font family name.
	FontFamily string
	// FontFeatures are the enabled OpenType features.
	FontFeatures font.FontFeatures
	// FontFallbacks lists fallback families, nil for none.
	FontFallbacks *font.FontFallbacks
	// FontSize is the font size; rem sizes resolve against the window rem size.
	FontSize geometry.AbsoluteLength
	// LineHeight is the line height. A fraction is a multiple of FontSize.
	LineHeight geometry.DefiniteLength
	// FontWeight is the font weight.
	FontWeight font.FontWeight
	// FontStyle is normal, italic or oblique.
	FontStyle font.FontStyle
	// BackgroundColor is painted behind the glyphs, nil for none.
	BackgroundColor *gc.Hsla
	// Underline is the underline decoration, nil for none.
	Underline *UnderlineStyle
	// Strikethrough is the strikethrough decoration, nil for none.
	Strikethrough *StrikethroughStyle
	// WhiteSpace selects wrapping behavior.
	WhiteSpace WhiteSpace
	// TextOverflow selects truncation of overflowing text, nil for none.
	TextOverflow *TextOverflow
	// TextAlign is the horizontal alignment.
	TextAlign TextAlign
	// LineClamp limits the number of visible lines, nil for no limit.
	LineClamp *int
}

// DefaultTextStyle returns the upstream defaults: opaque black text in the
// system UI font (".SystemUIFont"), 1rem size, normal weight and style, a
// line height of [geometry.Phi] (about 1.618 times the font size), normal
// white-space and left alignment.
func DefaultTextStyle() TextStyle {
	return TextStyle{
		Color:        gc.Black(),
		FontFamily:   ".SystemUIFont",
		FontFeatures: font.EmptyFeatures(),
		FontSize:     geometry.RemsLength(geometry.Rems(1)),
		LineHeight:   geometry.Phi(),
		FontWeight:   font.WeightNormal,
		FontStyle:    font.StyleNormal,
		WhiteSpace:   WhiteSpaceNormal,
		TextAlign:    TextAlignLeft,
	}
}

// ToFont resolves the family, weight, style, features and fallbacks of the
// text style into a [font.Font] descriptor.
func (ts TextStyle) ToFont() font.Font {
	return font.Font{
		Family:    ts.FontFamily,
		Weight:    ts.FontWeight,
		Style:     ts.FontStyle,
		Features:  ts.FontFeatures,
		Fallbacks: ts.FontFallbacks,
	}
}

// LineHeightInPixels resolves the line height to whole logical pixels
// (rounded): a fractional line height is multiplied by FontSize, which is in
// turn resolved against remSize (the window rem size in pixels).
func (ts TextStyle) LineHeightInPixels(remSize geometry.Pixels) float32 {
	return ts.LineHeight.ToPixels(ts.FontSize, remSize).Round().AsF32()
}

// Refine applies ref on top of ts. Set fields overwrite, unset fields keep
// the current value. Underline, Strikethrough and TextOverflow can be
// removed explicitly through the Clear* flags of the refinement. ref must
// not be nil.
func (ts *TextStyle) Refine(ref *TextStyleRefinement) {
	if ref.Color != nil {
		ts.Color = *ref.Color
	}
	if ref.FontFamily != nil {
		ts.FontFamily = *ref.FontFamily
	}
	if ref.FontFeatures != nil {
		ts.FontFeatures = ref.FontFeatures.Clone()
	}
	if ref.FontFallbacks != nil {
		c := ref.FontFallbacks.Clone()
		ts.FontFallbacks = &c
	}
	if ref.FontSize != nil {
		ts.FontSize = *ref.FontSize
	}
	if ref.LineHeight != nil {
		ts.LineHeight = *ref.LineHeight
	}
	if ref.FontWeight != nil {
		ts.FontWeight = *ref.FontWeight
	}
	if ref.FontStyle != nil {
		ts.FontStyle = *ref.FontStyle
	}
	if ref.BackgroundColor != nil {
		c := *ref.BackgroundColor
		ts.BackgroundColor = &c
	}
	if ref.Underline != nil {
		u := *ref.Underline
		ts.Underline = &u
	}
	if ref.Underline == nil && ref.ClearUnderline {
		ts.Underline = nil
	}
	if ref.Strikethrough != nil {
		s := *ref.Strikethrough
		ts.Strikethrough = &s
	}
	if ref.Strikethrough == nil && ref.ClearStrikethrough {
		ts.Strikethrough = nil
	}
	if ref.WhiteSpace != nil {
		ts.WhiteSpace = *ref.WhiteSpace
	}
	if ref.TextOverflow != nil {
		to := *ref.TextOverflow
		ts.TextOverflow = &to
	}
	if ref.TextOverflow == nil && ref.ClearTextOverflow {
		ts.TextOverflow = nil
	}
	if ref.TextAlign != nil {
		ts.TextAlign = *ref.TextAlign
	}
	if ref.LineClamp != nil {
		lc := *ref.LineClamp
		ts.LineClamp = &lc
	}
}

// TextStyleRefinement is the partial (all-optional) form of [TextStyle] used
// by the cascade and the Styled builder API: a nil field means "not set".
// Fields mirror [TextStyle]. Because nil already means "inherit", the
// optional decorations carry a Clear* flag to say "explicitly none".
type TextStyleRefinement struct {
	Color           *gc.Hsla
	FontFamily      *string
	FontFeatures    *font.FontFeatures
	FontFallbacks   *font.FontFallbacks
	FontSize        *geometry.AbsoluteLength
	LineHeight      *geometry.DefiniteLength
	FontWeight      *font.FontWeight
	FontStyle       *font.FontStyle
	BackgroundColor *gc.Hsla
	Underline       *UnderlineStyle
	// ClearUnderline removes an inherited underline when Underline is nil.
	ClearUnderline bool
	Strikethrough  *StrikethroughStyle
	// ClearStrikethrough removes an inherited strikethrough when
	// Strikethrough is nil.
	ClearStrikethrough bool
	WhiteSpace         *WhiteSpace
	TextOverflow       *TextOverflow
	// ClearTextOverflow removes inherited truncation when TextOverflow is
	// nil.
	ClearTextOverflow bool
	TextAlign         *TextAlign
	LineClamp         *int
}

// IsEmpty reports whether nothing is set, including the Clear* flags. A nil
// receiver is empty.
func (r *TextStyleRefinement) IsEmpty() bool { return r == nil || r.isEmpty() }

func (r *TextStyleRefinement) isEmpty() bool {
	return r.Color == nil && r.FontFamily == nil && r.FontFeatures == nil &&
		r.FontFallbacks == nil && r.FontSize == nil && r.LineHeight == nil &&
		r.FontWeight == nil && r.FontStyle == nil && r.BackgroundColor == nil &&
		r.Underline == nil && !r.ClearUnderline && r.Strikethrough == nil &&
		!r.ClearStrikethrough && r.WhiteSpace == nil && r.TextOverflow == nil &&
		!r.ClearTextOverflow && r.TextAlign == nil && r.LineClamp == nil
}

// Merge returns a refinement equivalent to applying r and then other (other
// wins). A Clear* flag in other drops the matching value from r. If either
// side is nil the other is returned as is, not copied.
func (r *TextStyleRefinement) Merge(other *TextStyleRefinement) *TextStyleRefinement {
	if r == nil {
		return other
	}
	if other == nil {
		return r
	}
	out := *r
	if other.Color != nil {
		out.Color = other.Color
	}
	if other.FontFamily != nil {
		out.FontFamily = other.FontFamily
	}
	if other.FontFeatures != nil {
		out.FontFeatures = other.FontFeatures
	}
	if other.FontFallbacks != nil {
		out.FontFallbacks = other.FontFallbacks
	}
	if other.FontSize != nil {
		out.FontSize = other.FontSize
	}
	if other.LineHeight != nil {
		out.LineHeight = other.LineHeight
	}
	if other.FontWeight != nil {
		out.FontWeight = other.FontWeight
	}
	if other.FontStyle != nil {
		out.FontStyle = other.FontStyle
	}
	if other.BackgroundColor != nil {
		out.BackgroundColor = other.BackgroundColor
	}
	if other.Underline != nil {
		out.Underline = other.Underline
	}
	if other.ClearUnderline {
		out.ClearUnderline = true
		out.Underline = nil
	}
	if other.Strikethrough != nil {
		out.Strikethrough = other.Strikethrough
	}
	if other.ClearStrikethrough {
		out.ClearStrikethrough = true
		out.Strikethrough = nil
	}
	if other.WhiteSpace != nil {
		out.WhiteSpace = other.WhiteSpace
	}
	if other.TextOverflow != nil {
		out.TextOverflow = other.TextOverflow
	}
	if other.ClearTextOverflow {
		out.ClearTextOverflow = true
		out.TextOverflow = nil
	}
	if other.TextAlign != nil {
		out.TextAlign = other.TextAlign
	}
	if other.LineClamp != nil {
		out.LineClamp = other.LineClamp
	}
	return &out
}

// HighlightStyle is a partial style overlay for text ranges (editor
// highlights). Mirrors upstream HighlightStyle semantics; apply it with
// [TextStyle.Highlight].
type HighlightStyle struct {
	// Color is blended over the text color (alpha-over), not replaced.
	Color *gc.Hsla
	// FontWeight replaces the weight.
	FontWeight *font.FontWeight
	// FontStyle replaces the style.
	FontStyle *font.FontStyle
	// BackgroundColor replaces the text background.
	BackgroundColor *gc.Hsla
	// Underline replaces the underline.
	Underline *UnderlineStyle
	// Strikethrough replaces the strikethrough.
	Strikethrough *StrikethroughStyle
	// FadeOut reduces the resulting text alpha by this factor in [0, 1]
	// (1 makes the text fully transparent).
	FadeOut *float32
}

// Highlight returns a copy of ts with h applied (upstream
// TextStyle::highlight): the highlight color blends over the text color,
// FadeOut reduces the alpha, and the other set fields replace the current
// values. ts itself is not modified.
func (ts TextStyle) Highlight(h HighlightStyle) TextStyle {
	out := ts
	if h.Color != nil {
		out.Color = ts.Color.Blend(*h.Color)
	}
	if h.FontWeight != nil {
		out.FontWeight = *h.FontWeight
	}
	if h.FontStyle != nil {
		out.FontStyle = *h.FontStyle
	}
	if h.BackgroundColor != nil {
		c := *h.BackgroundColor
		out.BackgroundColor = &c
	}
	if h.Underline != nil {
		u := *h.Underline
		out.Underline = &u
	}
	if h.Strikethrough != nil {
		s := *h.Strikethrough
		out.Strikethrough = &s
	}
	if h.FadeOut != nil {
		out.Color.FadeOut(*h.FadeOut)
	}
	return out
}
