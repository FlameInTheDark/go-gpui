// Package font defines font descriptor types shared by the style and text
// systems (ported from gpui's text_system.rs descriptor types).
package font

import (
	"sort"
	"strings"
)

// FontWeight is a font weight in the CSS/OpenType range [100, 900]. 400 is
// normal, 700 is bold.
type FontWeight float32

// Common OpenType/CSS weight values.
const (
	// WeightThin is weight 100.
	WeightThin FontWeight = 100
	// WeightExtraLight is weight 200.
	WeightExtraLight FontWeight = 200
	// WeightLight is weight 300.
	WeightLight FontWeight = 300
	// WeightNormal is weight 400.
	WeightNormal FontWeight = 400
	// WeightMedium is weight 500.
	WeightMedium FontWeight = 500
	// WeightSemibold is weight 600.
	WeightSemibold FontWeight = 600
	// WeightBold is weight 700.
	WeightBold FontWeight = 700
	// WeightExtrabold is weight 800.
	WeightExtrabold FontWeight = 800
	// WeightBlack is weight 900.
	WeightBlack FontWeight = 900
)

// BOLD is the bold weight (700).
const BOLD FontWeight = 700

// FontStyle selects upright, italic or oblique.
type FontStyle uint8

const (
	// StyleNormal is the default upright style.
	StyleNormal FontStyle = 0
	// StyleItalic is italic.
	StyleItalic FontStyle = 1
	// StyleOblique is oblique.
	StyleOblique FontStyle = 2
)

// FontFeatures is an ordered list of OpenType feature tag/value pairs.
// A value of 0 disables the feature, non-zero enables it.
type FontFeatures struct {
	tags   []string
	values []uint32
}

// Feature is one tag/value pair.
type Feature struct {
	Tag   string
	Value uint32
}

// EmptyFeatures is the empty feature set.
func EmptyFeatures() FontFeatures { return FontFeatures{} }

// ParseFeatures parses a string like "ss01, calt=0, liga" into features.
// Tags must be exactly four ASCII alphanumerics; values are optional
// (default 1). Invalid entries are skipped, mirroring upstream leniency.
func ParseFeatures(s string) FontFeatures {
	var f FontFeatures
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		tag := part
		var value uint32 = 1
		if i := strings.Index(part, "="); i >= 0 {
			tag = strings.TrimSpace(part[:i])
			value = parseFeatureValue(strings.TrimSpace(part[i+1:]))
		}
		if IsValidFeatureTag(tag) {
			f.Set(tag, value)
		}
	}
	return f
}

func parseFeatureValue(s string) uint32 {
	var v uint32
	neg := false
	for i, c := range s {
		if i == 0 && c == '-' {
			neg = true
			continue
		}
		if c < '0' || c > '9' {
			return 1
		}
		v = v*10 + uint32(c-'0')
	}
	if neg {
		return 0
	}
	return v
}

// IsValidFeatureTag reports whether the tag is 4 ASCII alphanumerics.
func IsValidFeatureTag(tag string) bool {
	if len(tag) != 4 {
		return false
	}
	for i := 0; i < 4; i++ {
		c := tag[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// Set appends or updates a feature tag.
func (f *FontFeatures) Set(tag string, value uint32) {
	for i := range f.tags {
		if f.tags[i] == tag {
			f.values[i] = value
			return
		}
	}
	f.tags = append(f.tags, tag)
	f.values = append(f.values, value)
}

// Get returns the value for a tag.
func (f FontFeatures) Get(tag string) (uint32, bool) {
	for i := range f.tags {
		if f.tags[i] == tag {
			return f.values[i], true
		}
	}
	return 0, false
}

// IsEmpty reports whether no features are set.
func (f FontFeatures) IsEmpty() bool { return len(f.tags) == 0 }

// List returns the ordered feature list.
func (f FontFeatures) List() []Feature {
	out := make([]Feature, len(f.tags))
	for i := range f.tags {
		out[i] = Feature{Tag: f.tags[i], Value: f.values[i]}
	}
	return out
}

// Clone deep-copies the features.
func (f FontFeatures) Clone() FontFeatures {
	if len(f.tags) == 0 {
		return FontFeatures{}
	}
	tags := append([]string(nil), f.tags...)
	values := append([]uint32(nil), f.values...)
	return FontFeatures{tags: tags, values: values}
}

// Equal compares two feature sets by ordered content.
func (f FontFeatures) Equal(o FontFeatures) bool {
	if len(f.tags) != len(o.tags) {
		return false
	}
	for i := range f.tags {
		if f.tags[i] != o.tags[i] || f.values[i] != o.values[i] {
			return false
		}
	}
	return true
}

// Sort orders the features by tag (matching the JSON-map round trip).
func (f *FontFeatures) Sort() {
	type pair struct {
		t string
		v uint32
	}
	pairs := make([]pair, len(f.tags))
	for i := range f.tags {
		pairs[i] = pair{f.tags[i], f.values[i]}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].t < pairs[j].t })
	for i := range pairs {
		f.tags[i] = pairs[i].t
		f.values[i] = pairs[i].v
	}
}

// DisableLigatures returns features that turn off common ligatures.
func DisableLigatures() FontFeatures {
	f := FontFeatures{}
	f.Set("calt", 0)
	return f
}

// IsCaltEnabled reports the calt setting if present.
func (f FontFeatures) IsCaltEnabled() (bool, bool) {
	v, ok := f.Get("calt")
	if !ok {
		return false, false
	}
	return v != 0, true
}

// FontFallbacks is an ordered list of fallback family names.
type FontFallbacks struct {
	families []string
}

// FallbacksFrom constructs FontFallbacks.
func FallbacksFrom(families []string) FontFallbacks {
	return FontFallbacks{families: append([]string(nil), families...)}
}

// List returns the fallback list.
func (f FontFallbacks) List() []string {
	if f.families == nil {
		return nil
	}
	return append([]string(nil), f.families...)
}

// IsEmpty reports whether there are no fallbacks.
func (f FontFallbacks) IsEmpty() bool { return len(f.families) == 0 }

// Font describes a font: family, weight, style, features and fallbacks.
// It is a cache key, so two fonts that should shape differently must not
// compare equal.
type Font struct {
	Family    string
	Weight    FontWeight
	Style     FontStyle
	Features  FontFeatures
	Fallbacks *FontFallbacks
}

// New constructs a normal-weight upright font of the given family.
func New(family string) Font {
	return Font{Family: family, Weight: WeightNormal, Style: StyleNormal}
}

// Default is the system UI font descriptor.
func Default() Font { return New(".SystemUIFont") }

// Bold returns a copy with bold weight.
func (f Font) Bold() Font { f.Weight = BOLD; return f }

// Italic returns a copy with italic style.
func (f Font) Italic() Font { f.Style = StyleItalic; return f }

// WithWeight returns a copy with the given weight.
func (f Font) WithWeight(w FontWeight) Font { f.Weight = w; return f }

// WithStyle returns a copy with the given style.
func (f Font) WithStyle(s FontStyle) Font { f.Style = s; return f }

// WithFeatures returns a copy with the given features.
func (f Font) WithFeatures(feats FontFeatures) Font { f.Features = feats.Clone(); return f }

// WithFallbacks returns a copy with the given fallbacks.
func (f Font) WithFallbacks(fb FontFallbacks) Font {
	c := fb
	f.Fallbacks = &c
	return f
}

// Less orders fonts by family, then weight, then style.
func (f Font) Less(o Font) bool {
	if f.Family != o.Family {
		return f.Family < o.Family
	}
	if f.Weight != o.Weight {
		return f.Weight < o.Weight
	}
	return f.Style < o.Style
}

// SystemFamilyFor maps virtual family names to concrete families
// (upstream font_name_with_fallbacks, Linux table).
func SystemFamilyFor(name string) string {
	switch name {
	case ".SystemUIFont":
		return "Ubuntu"
	}
	return name
}

// Clone deep-copies the fallbacks.
func (f FontFallbacks) Clone() FontFallbacks {
	return FontFallbacks{families: append([]string(nil), f.families...)}
}
