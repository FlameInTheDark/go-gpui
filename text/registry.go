// Lazy font registry: system font files are probed once per process (a
// few KB of header tables per font), and full file reads/parses happen
// only when a face is actually used. This is what keeps hundreds of
// megabytes of system fonts out of resident memory: the eager loader
// (pre-fix) read and retained every font file it walked.

package text

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/FlameInTheDark/go-gpui/text/sfnt"
)

// fontMeta is the lightweight metadata probed from a font file's
// name/OS2/post/cmap tables: a few hundred bytes per face, retained for
// the process lifetime (family matching, style/weight scoring, fallback
// coverage prefilters) without keeping any font bytes in memory.
type fontMeta struct {
	family, subfamily, fullName, psName string
	weightClass                         uint16
	isBold, isItalic, isOblique         bool
	// coverage is the cmap's code-point ranges (nil never happens for a
	// registered font: a font without a usable cmap cannot shape, and the
	// eager loader rejected it at Parse time for the same reason).
	coverage []uint32
}

// registryEntry is one probed font file.
type registryEntry struct {
	path string
	meta fontMeta
}

var (
	registryOnce sync.Once
	fontRegistry []registryEntry
)

// systemFontRegistry returns the process-wide registry of system font
// files, probing each once. Probing reads only the name/OS2/post/cmap
// tables via ReadAt — never the whole file. Fonts that fail to probe
// (unreadable, not a font, no usable cmap) are skipped, preserving the
// eager loader's per-file error tolerance. The walk visits the same
// directories in the same order as the eager loader did, so face ids,
// family resolution and fallback scan order are unchanged.
func systemFontRegistry() []registryEntry {
	registryOnce.Do(func() {
		for _, dir := range SystemFontDirs() {
			_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}
				ext := strings.ToLower(filepath.Ext(path))
				if ext != ".ttf" && ext != ".otf" {
					return nil
				}
				if meta, ok := probeFontFile(path); ok {
					fontRegistry = append(fontRegistry, registryEntry{path: path, meta: meta})
				}
				return nil
			})
		}
	})
	return fontRegistry
}

// probeFontFile reads a font file's header tables (never the whole file)
// and returns its registry metadata.
func probeFontFile(path string) (fontMeta, bool) {
	var meta fontMeta
	f, err := os.Open(path)
	if err != nil {
		return meta, false
	}
	defer f.Close()
	tables, err := sfnt.ReadTables(f, []string{"name", "OS/2", "post", "cmap"})
	if err != nil {
		return meta, false
	}
	meta.coverage = sfnt.CoverageRanges(tables["cmap"])
	if meta.coverage == nil {
		return meta, false // no usable cmap: cannot shape (Parse would fail too)
	}
	meta.family, meta.subfamily, meta.fullName, meta.psName = sfnt.ParseNameTable(tables["name"])
	meta.weightClass, meta.isBold, meta.isItalic = sfnt.OS2WeightStyle(tables["OS/2"])
	meta.isOblique = sfnt.PostItalicAngle(tables["post"]) != 0 && !meta.isItalic
	// Bold/italic fallbacks from the subfamily name (same rules as Parse).
	if meta.subfamily == "Bold" {
		meta.isBold = true
	}
	if meta.subfamily == "Italic" || meta.subfamily == "Bold Italic" {
		meta.isItalic = true
	}
	return meta, true
}

// rangesContain binary-searches sorted, merged [lo, hi] coverage pairs.
func rangesContain(ranges []uint32, r rune) bool {
	if len(ranges) == 0 || r < 0 || r > 0x10FFFF {
		return false
	}
	u := uint32(r)
	lo, hi := 0, len(ranges)/2-1
	for lo <= hi {
		mid := int(uint(lo+hi) >> 1)
		switch {
		case u < ranges[2*mid]:
			hi = mid - 1
		case u > ranges[2*mid+1]:
			lo = mid + 1
		default:
			return true
		}
	}
	return false
}

// Materialized faces are shared process-wide: every System built from the
// registry (the platform's, tests', any app-built one) resolves the same
// handful of faces, and parsing the same file per System would multiply
// retained bytes. Faces are immutable after Parse, so sharing is safe.
var (
	faceCacheMu sync.Mutex
	faceCache   = map[string]*sfnt.Font{} // path -> face (nil = known-failed)
)

// readFontFile is the full-file loader used when a registered face is
// first materialized. A package-level hook so tests can count reads.
var readFontFile = func(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// cachedFace materializes (or reuses) the parsed face of a font file.
// Failed loads are cached as nil so a broken font costs one read, not one
// read per frame.
func cachedFace(path string) *sfnt.Font {
	faceCacheMu.Lock()
	if face, ok := faceCache[path]; ok {
		faceCacheMu.Unlock()
		return face
	}
	faceCacheMu.Unlock()

	var parsed *sfnt.Font
	if data, err := readFontFile(path); err == nil {
		if f, err := sfnt.Parse(data); err == nil {
			parsed = f
		}
	}
	faceCacheMu.Lock()
	faceCache[path] = parsed
	faceCacheMu.Unlock()
	return parsed
}
