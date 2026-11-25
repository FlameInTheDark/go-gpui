// Package text implements gpui's text system in pure Go: a font registry,
// OpenType shaping, bidirectional reordering, line breaking, wrapped-line
// layout with index/pixel mapping, and a glyph rasterizer. It is the
// pure-Go replacement for the harfbuzz/cosmic-text stack used by upstream
// GPUI and ports its LineLayout / WrappedLineLayout model faithfully.
//
// # Overview
//
// The central type is [System]. It owns a registry of font faces, resolves
// [font.Font] descriptors to a [FontID], shapes strings into [LineLayout]
// values, and caches the results:
//
//	sys := text.NewSystemWithSystemFonts()
//	id, ok := sys.FontID(font.New("Arial"))
//	if !ok {
//		return // no matching face and no usable fallback
//	}
//	line := "Hello, world"
//	layout := sys.LayoutLine(line, geometry.Px(16),
//		[]text.FontRun{{Len: len(line), FontID: id}})
//	x := layout.XForIndex(5) // pixel x of the caret before byte 5
//
// # Concepts
//
// Font registry. Faces come from three sources: the process-wide probed
// registry of operating-system fonts ([NewSystemWithSystemFonts]), font
// directories ([System.LoadDir]) and in-memory data ([System.AddFonts]).
// File-backed faces are lazy: only the name, OS/2, post and cmap tables
// are read when they are registered, and the full file is read and parsed
// the first time the face is used. Parsed faces are shared process-wide
// and are immutable.
//
// Font resolution. [System.FontID] maps a descriptor to the registered face
// of that family whose weight and italic-ness score best, applying the
// virtual family names understood by [font.SystemFamilyFor] and, when the
// family is missing, a built-in fallback stack (a separate monospace stack
// is used for family names containing "mono").
//
// Shaping. [System.LayoutLine] itemizes text into same-font spans (falling
// back per rune to another face that covers it), splits it into clusters,
// runs the bidirectional algorithm (UAX #9, see [ReorderLine]), and then
// applies the font's GSUB substitutions, GPOS positioning (single, pair,
// mark-to-base, mark-to-mark, mark-to-ligature and contextual lookups,
// honouring GDEF glyph classes and lookup flags) and the legacy kern table.
// Tabs advance to the next multiple of [TabWidthEm] space widths. Glyph
// positions are in pixels measured from the line origin; every glyph also
// carries the UTF-8 byte offset of its cluster.
//
// Line layout. [LineLayout] answers the editor questions: which byte index
// is at pixel x ([LineLayout.IndexForX], [LineLayout.ClosestIndexForX]) and
// where is a byte index ([LineLayout.XForIndex]). These are bidi-aware.
// [WrappedLineLayout] adds soft-wrap boundaries and maps between byte
// indices and (x, y) positions across wrapped rows.
//
// Line breaking. [BreakOpportunities] computes UAX #14-style break
// opportunities and [System.WrapBoundaries] chooses where a shaped line
// wraps at a given width. [WrapWithIndent] is a simpler string-level
// wrapper for fixed-width columns (terminals, plain text views).
//
// Rasterization. [RasterizeGlyph] renders one glyph outline to an 8-bit
// coverage mask; the render package stores such masks in its atlas.
//
// # Caching and concurrency
//
// Registry state and face loading are guarded by a mutex; the shaped-line
// cache and the wrap cache each have their own RWMutex. Both caches are
// bounded: when one reaches its limit
// it is cleared wholesale. A *LineLayout returned by [System.LayoutLine] is
// shared between callers and must be treated as immutable. Wrap results are
// keyed by that pointer, so wrap computation is cached per (layout, width,
// max lines).
//
// # Known limitations
//
// The font features and fallback lists carried by [font.Font] are not
// consulted by the shaper: the GSUB/GPOS feature set is fixed. Bidi
// handling is bracket-aware (N0) but treats directional isolates
// approximately; see [ReorderLine].
package text
