# Text and fonts

For ordinary labels, start with `gpui.Text` or
[`elements.NewText`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/elements#NewText).
Text inherits the surrounding text style unless overridden. Use `WithRuns`
for differently styled spans and `WithCaret` for a caret; their positions and
run lengths use **UTF-8 byte offsets**, not rune counts. The
[task-list example](../examples/tutorial/main.go) uses text and a caret for a
small input control.

The lower-level text stack is useful for custom drawing, editors, font
inspection or measuring text:

| Package or type | Purpose |
| --- | --- |
| [`font.Font`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/font#Font) | Family, weight, style and related font selection properties. |
| [`text.System`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/text#System) | Font registry, resolution, shaping and line-layout caches. |
| [`text.LineLayout`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/text#LineLayout) | Shaped runs, line metrics and byte-index/position mapping. |
| [`text/sfnt`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/text/sfnt) | Parsing font tables and glyph outlines. |

`text.NewSystemWithSystemFonts()` discovers system fonts. For a bundled font,
load its bytes and call `System.AddFonts`; check the returned error. Resolve a
descriptor with `System.FontID`, then shape text with `System.LayoutLine` and
`FontRun` values. The [text package documentation](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/text)
includes a shaping example and details of fallback, bidirectional text and
wrapping. Layouts returned by `LayoutLine` are cached and shared: treat them as
immutable.

Font availability and rendering differ across machines. Bundle fonts when
consistent output matters, and avoid assuming a particular system font exists
in a portable test. The [architecture guide](architecture.md#the-text-stack)
describes how shaping, rasterization and the sprite atlas fit together.
