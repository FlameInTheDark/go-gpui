package style

import (
	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/font"
	"github.com/FlameInTheDark/go-gpui/geometry"
)

// UnderlineStyle describes a text underline.
type UnderlineStyle struct {
	Thickness float32
	Color     *gc.Hsla
	Wavy      bool
}

// StrikethroughStyle describes strikethrough.
type StrikethroughStyle struct {
	Thickness float32
	Color     *gc.Hsla
}

// TextStyle is the concrete text style (the "resolved" form). It mirrors
// upstream TextStyle, which is refineable; the *Refinement form below is the
// partial-update view used while cascading.
type TextStyle struct {
	Color           gc.Hsla
	FontFamily      string
	FontFeatures    font.FontFeatures
	FontFallbacks   *font.FontFallbacks
	FontSize        geometry.AbsoluteLength
	LineHeight      geometry.DefiniteLength
	FontWeight      font.FontWeight
	FontStyle       font.FontStyle
	BackgroundColor *gc.Hsla
	Underline       *UnderlineStyle
	Strikethrough   *StrikethroughStyle
	WhiteSpace      WhiteSpace
	TextOverflow    *TextOverflow
	TextAlign       TextAlign
	LineClamp       *int
}

// DefaultTextStyle returns the upstream defaults: black 1rem text with a
// phi() line height of the system UI font.
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

// Font resolves the text style into a font descriptor.
func (ts TextStyle) ToFont() font.Font {
	return font.Font{
		Family:    ts.FontFamily,
		Weight:    ts.FontWeight,
		Style:     ts.FontStyle,
		Features:  ts.FontFeatures,
		Fallbacks: ts.FontFallbacks,
	}
}

// LineHeightInPixels resolves the line height against a rem size.
func (ts TextStyle) LineHeightInPixels(remSize geometry.Pixels) float32 {
	return ts.LineHeight.ToPixels(ts.FontSize, remSize).Round().AsF32()
}

// Refine applies a partial text style on top of this one.
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

// TextStyleRefinement is the partial (all-optional) form of TextStyle used
// by the cascade and the Styled builder API.
type TextStyleRefinement struct {
	Color              *gc.Hsla
	FontFamily         *string
	FontFeatures       *font.FontFeatures
	FontFallbacks      *font.FontFallbacks
	FontSize           *geometry.AbsoluteLength
	LineHeight         *geometry.DefiniteLength
	FontWeight         *font.FontWeight
	FontStyle          *font.FontStyle
	BackgroundColor    *gc.Hsla
	Underline          *UnderlineStyle
	ClearUnderline     bool
	Strikethrough      *StrikethroughStyle
	ClearStrikethrough bool
	WhiteSpace         *WhiteSpace
	TextOverflow       *TextOverflow
	ClearTextOverflow  bool
	TextAlign          *TextAlign
	LineClamp          *int
}

// IsEmpty reports whether nothing is set.
func (r *TextStyleRefinement) IsEmpty() bool { return r == nil || r.isEmpty() }

func (r *TextStyleRefinement) isEmpty() bool {
	return r.Color == nil && r.FontFamily == nil && r.FontFeatures == nil &&
		r.FontFallbacks == nil && r.FontSize == nil && r.LineHeight == nil &&
		r.FontWeight == nil && r.FontStyle == nil && r.BackgroundColor == nil &&
		r.Underline == nil && !r.ClearUnderline && r.Strikethrough == nil &&
		!r.ClearStrikethrough && r.WhiteSpace == nil && r.TextOverflow == nil &&
		!r.ClearTextOverflow && r.TextAlign == nil && r.LineClamp == nil
}

// Merge returns a refinement equivalent to applying r then other (other
// wins), recursing into nested values.
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
// highlights). Mirrors upstream HighlightStyle semantics.
type HighlightStyle struct {
	Color           *gc.Hsla
	FontWeight      *font.FontWeight
	FontStyle       *font.FontStyle
	BackgroundColor *gc.Hsla
	Underline       *UnderlineStyle
	Strikethrough   *StrikethroughStyle
	FadeOut         *float32
}

// Highlight applies the highlight over this text style (upstream
// TextStyle::highlight): color blends, fade multiplies, others replace.
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
