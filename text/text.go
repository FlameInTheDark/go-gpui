package text

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/FlameInTheDark/go-gpui/font"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/text/sfnt"
)

// FontID identifies a loaded font face (index into the system's list).
type FontID int

// FontMetrics mirrors gpui's FontMetrics (em-relative floats).
type FontMetrics struct {
	UnitsPerEm         uint16
	Ascent             float32
	Descent            float32
	LineGap            float32
	UnderlinePosition  float32
	UnderlineThickness float32
	CapHeight          float32
	XHeight            float32
	// BoundingBox is (xMin, yMin, xMax, yMax) in ems.
	BoundingBox [4]float32
}

// loadedFont is one face in the system.
//
// Faces registered from font FILES (the system registry, LoadDir) are
// lazy: metadata is probed up front and the file is read + parsed only
// when the face is first used (the UI font, the editor's mono font, any
// fallback face that actually covers a missing rune). Faces registered
// from in-memory data (AddFonts) are parsed eagerly — the bytes are
// already resident.
type loadedFont struct {
	family    string
	weight    font.FontWeight
	style     font.FontStyle
	isEmoji   bool
	features  font.FontFeatures
	fallbacks []string

	// Lazy source: exactly one is set.
	data     []byte   // in-memory font bytes (AddFonts)
	path     string   // file to read on first use (registry/LoadDir)
	coverage []uint32 // probed cmap ranges (fallback prefilter; nil for data fonts)

	// face is the parsed font, nil until first use. loadFailed latches
	// materialization errors so a broken file costs one read, not one per
	// frame.
	face       *sfnt.Font
	loadFailed bool
}

// System is the text system: font registry + shaper. It implements the
// shaping side of gpui's PlatformTextSystem contract.
type System struct {
	mu      sync.Mutex
	faces   []*loadedFont
	byKey   map[fontCacheKey]FontID
	metrics map[FontID]FontMetrics

	// Shaped-line cache. LayoutLine results are pure functions of
	// (text, fontSize, font runs), and UI frames re-layout the same strings
	// every redraw (an editor re-shapes all visible lines twice per frame);
	// without this cache each action re-runs GSUB/GPOS over every glyph.
	layoutCache   map[string]*LineLayout
	layoutCacheMU sync.RWMutex
	// wrapCache memoizes wrap boundaries per (shaped layout, wrap width,
	// max-lines). Computing boundaries is quadratic in line length with two
	// map builds per call; a wrapped frame recomputes the identical result
	// for every visible line, so this turns re-wraps into map hits. Keyed by
	// the shared *LineLayout pointer: it uniquely identifies the shaping
	// inputs (text, size, runs), and entries stay valid even after the
	// layout cache is bulk-reset (the pointer still refers to that layout).
	wrapCache   map[wrapCacheKey]WrapResult
	wrapCacheMU sync.RWMutex
	// fallbackCache memoizes per-rune fallback font resolution (a miss scans
	// every loaded face's cmap).
	fallbackCache map[fallbackKey]FontID

	// fallback stack used when a family is missing (upstream order).
	fallbackStack []string
	// stack used when a monospace family is missing (generic "Monospace",
	// Plex Mono, etc.).
	monoFallbackStack []string
}

type fallbackKey struct {
	r       rune
	primary FontID
}

// layoutCacheLimit caps the shaped-line cache; overflow clears it (the
// working set re-warms within a frame or two).
const layoutCacheLimit = 8192

type fontCacheKey struct {
	family    string
	weight    font.FontWeight
	style     font.FontStyle
	features  string
	fallbacks string
}

// NewSystem creates an empty text system.
func NewSystem() *System {
	return &System{
		byKey:         map[fontCacheKey]FontID{},
		metrics:       map[FontID]FontMetrics{},
		layoutCache:   map[string]*LineLayout{},
		wrapCache:     map[wrapCacheKey]WrapResult{},
		fallbackCache: map[fallbackKey]FontID{},
		fallbackStack: []string{
			"DejaVu Sans", "Liberation Sans", "Noto Sans", "FreeSans",
			"Ubuntu", "Cantarell", "Arial",
		},
		monoFallbackStack: []string{
			"DejaVu Sans Mono", "Liberation Mono", "Noto Sans Mono", "FreeMono",
			"Nimbus Mono PS", "Ubuntu Mono", "Consolas", "Menlo", "Courier New", "Courier",
		},
	}
}

// NewSystemWithSystemFonts creates a system over the system font
// directories.
//
// Fonts are registered from a process-wide PROBED registry: only the
// name/OS2/post/cmap header tables are read per file (a few KB), never
// the full font bytes — an eager loader here retained every system font
// file for the process lifetime (133MB+ on a typical Windows install,
// ~85% of the editor's live heap). Full reads and parses happen lazily
// when a face is first used, so resident font memory tracks the handful
// of faces actually rendered. Face order matches the eager loader's
// walk, so ids and fallback resolution are unchanged.
func NewSystemWithSystemFonts() *System {
	s := NewSystem()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range systemFontRegistry() {
		s.registerLocked(e.path, e.meta)
	}
	return s
}

// SystemFontDirs lists the default font directories for the current OS.
func SystemFontDirs() []string {
	switch runtime.GOOS {
	case "windows":
		windir := os.Getenv("WINDIR")
		if windir == "" {
			windir = `C:\Windows`
		}
		return []string{
			filepath.Join(windir, "Fonts"),
			filepath.Join(homeDir(), "AppData", "Local", "Microsoft", "Windows", "Fonts"),
		}
	case "darwin":
		return []string{
			"/System/Library/Fonts",
			"/Library/Fonts",
			filepath.Join(homeDir(), "Library", "Fonts"),
		}
	default: // Linux and BSDs
		return []string{
			"/usr/share/fonts",
			"/usr/local/share/fonts",
			"/usr/X11R6/lib/X11/fonts",
			filepath.Join(homeDir(), ".fonts"),
			filepath.Join(homeDir(), ".local/share/fonts"),
		}
	}
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

// AddFonts parses and registers in-memory font data (gpui's add_fonts).
// Data fonts are parsed eagerly: the bytes are already resident.
func (s *System) AddFonts(fontData [][]byte) error {
	for _, data := range fontData {
		if _, _, err := s.addFontData(data); err != nil {
			return err
		}
	}
	return nil
}

func (s *System) addFontData(data []byte) (FontID, bool, error) {
	face, err := sfnt.Parse(data)
	if err != nil {
		return 0, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, lf := range s.faces {
		if lf.face == face {
			return FontID(i), false, nil
		}
	}
	lf := &loadedFont{
		family:  face.FamilyName,
		weight:  font.FontWeight(face.WeightClass),
		style:   font.StyleNormal,
		isEmoji: strings.Contains(strings.ToLower(face.FullName), "emoji"),
		data:    data,
		face:    face,
	}
	if face.IsItalic || face.IsOblique() {
		lf.style = font.StyleItalic
	}
	if face.WeightClass == 0 {
		if face.IsBold {
			lf.weight = font.WeightBold
		} else {
			lf.weight = font.WeightNormal
		}
	}
	s.faces = append(s.faces, lf)
	return FontID(len(s.faces) - 1), true, nil
}

// registerLocked appends a lazy registry face (path + probed metadata).
// The style/weight scoring mirrors addFontData's rules.
func (s *System) registerLocked(path string, meta fontMeta) {
	lf := &loadedFont{
		family:   meta.family,
		weight:   font.FontWeight(meta.weightClass),
		style:    font.StyleNormal,
		isEmoji:  strings.Contains(strings.ToLower(meta.fullName), "emoji"),
		path:     path,
		coverage: meta.coverage,
	}
	if meta.isItalic || meta.isOblique {
		lf.style = font.StyleItalic
	}
	if meta.weightClass == 0 {
		if meta.isBold {
			lf.weight = font.WeightBold
		} else {
			lf.weight = font.WeightNormal
		}
	}
	s.faces = append(s.faces, lf)
}

// LoadDir registers every probed .ttf/.otf file under dir (metadata
// only; files are read lazily on first use, exactly like the system
// registry). Per-file errors are skipped.
func (s *System) LoadDir(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".ttf" && ext != ".otf" {
			return nil
		}
		if meta, ok := probeFontFile(path); ok {
			s.mu.Lock()
			s.registerLocked(path, meta)
			s.mu.Unlock()
		}
		return nil
	})
}

// AllFontNames lists unique family names.
func (s *System) AllFontNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]bool{}
	var out []string
	for _, lf := range s.faces {
		if !seen[lf.family] {
			seen[lf.family] = true
			out = append(out, lf.family)
		}
	}
	sort.Strings(out)
	return out
}

// Face exposes the parsed face of a font id, materializing it on first
// use (file faces read + parse lazily; the parsed face is shared
// process-wide via the registry cache).
func (s *System) Face(id FontID) *sfnt.Font {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.faceLocked(id)
}

func (s *System) faceLocked(id FontID) *sfnt.Font {
	if int(id) >= 0 && int(id) < len(s.faces) {
		lf := s.faces[id]
		if lf.face == nil && !lf.loadFailed {
			lf.materializeLocked()
		}
		return lf.face
	}
	return nil
}

// materializeLocked reads and parses the face's source. It runs under
// s.mu: the one-time file read is a few ms per font, and locking here
// avoids duplicate loads from concurrent shaping calls.
func (lf *loadedFont) materializeLocked() {
	var face *sfnt.Font
	switch {
	case lf.data != nil:
		face, _ = sfnt.Parse(lf.data)
	case lf.path != "":
		face = cachedFace(lf.path)
	}
	if face == nil {
		lf.loadFailed = true
	}
	lf.face = face
}

// FontID resolves a font descriptor to a loaded face, applying virtual
// family names and the fallback stack. ok=false when nothing matches.
func (s *System) FontID(f font.Font) (FontID, bool) {
	family := font.SystemFamilyFor(f.Family)
	key := fontCacheKey{
		family: family, weight: f.Weight, style: f.Style,
	}
	if len(f.Features.List()) > 0 {
		for _, feat := range f.Features.List() {
			key.features += fmt.Sprintf("%s=%d,", feat.Tag, feat.Value)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.byKey[key]; ok {
		return id, true
	}
	id, ok := s.bestMatchLocked(family, f.Weight, f.Style)
	if !ok {
		// Try the fallback stack: monospace families resolve through
		// the monospace stack so generic "Monospace" never lands on a
		// proportional face.
		stack := s.fallbackStack
		if isMonospaceFamily(family) || isMonospaceFamily(f.Family) {
			stack = s.monoFallbackStack
		}
		for _, fb := range stack {
			if id, ok = s.bestMatchLocked(fb, f.Weight, f.Style); ok {
				break
			}
		}
	}
	if !ok {
		return 0, false
	}
	s.byKey[key] = id
	return id, true
}

func (s *System) bestMatchLocked(family string, weight font.FontWeight, style font.FontStyle) (FontID, bool) {
	wantItalic := style == font.StyleItalic
	best := FontID(-1)
	bestScore := int64(1 << 62)
	for i, lf := range s.faces {
		if lf.family != family {
			continue
		}
		score := int64(0)
		isItalic := lf.style != font.StyleNormal
		if isItalic != wantItalic {
			score += 100000
		}
		wd := lf.weight - weight
		if wd < 0 {
			wd = -wd
		}
		score += int64(wd)
		if score < bestScore {
			bestScore = score
			best = FontID(i)
		}
	}
	if best < 0 {
		return 0, false
	}
	return best, true
}

// isMonospaceFamily reports whether a family name requests a monospace
// face (generic "Monospace", Plex Mono, DejaVu Sans Mono, ...).
func isMonospaceFamily(name string) bool {
	return strings.Contains(strings.ToLower(name), "mono")
}

// Metrics returns the cached metrics of a font id.
func (s *System) Metrics(id FontID) FontMetrics {
	s.mu.Lock()
	if m, ok := s.metrics[id]; ok {
		s.mu.Unlock()
		return m
	}
	face := s.faceLocked(id)
	defer s.mu.Unlock()
	var m FontMetrics
	if face != nil {
		fm := face.Metrics()
		m = FontMetrics{
			UnitsPerEm: fm.UnitsPerEm, Ascent: fm.Ascent, Descent: fm.Descent,
			LineGap: fm.LineGap, UnderlinePosition: fm.UnderlinePosition,
			UnderlineThickness: fm.UnderlineThickness, CapHeight: fm.CapHeight,
			XHeight: fm.XHeight, BoundingBox: fm.BoundingBox,
		}
	}
	s.metrics[id] = m
	return m
}

// Ascent returns the ascent in pixels at the given size.
func (s *System) Ascent(id FontID, size geometry.Pixels) geometry.Pixels {
	return geometry.Px(s.Metrics(id).Ascent * size.AsF32())
}

// Descent returns the descent in pixels (negative).
func (s *System) Descent(id FontID, size geometry.Pixels) geometry.Pixels {
	return geometry.Px(s.Metrics(id).Descent * size.AsF32())
}

// LineHeight returns ascent - descent + line gap.
func (s *System) LineHeight(id FontID, size geometry.Pixels) geometry.Pixels {
	m := s.Metrics(id)
	return geometry.Px((m.Ascent - m.Descent + m.LineGap) * size.AsF32())
}

// BaselineOffset is gpui's (line_height - ascent - descent)/2 + ascent.
func (s *System) BaselineOffset(id FontID, fontSize, lineHeight geometry.Pixels) geometry.Pixels {
	ascent := s.Ascent(id, fontSize)
	descent := s.Descent(id, fontSize)
	return geometry.Px((lineHeight.AsF32()-ascent.AsF32()-descent.AsF32())/2 + ascent.AsF32())
}

// UnderlineYOffset is gpui's (line_height-ascent-descent)/2 + ascent +
// descent*0.618.
func UnderlineYOffset(lineHeight, ascent, descent geometry.Pixels) geometry.Pixels {
	return geometry.Px((lineHeight.AsF32()-ascent.AsF32()-descent.AsF32())/2 + ascent.AsF32() + descent.AsF32()*0.618)
}

// IsEmojiFont reports whether the id is an emoji face.
func (s *System) IsEmojiFont(id FontID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return int(id) < len(s.faces) && s.faces[id].isEmoji
}

// DefaultFontID returns the first sans fallback (or any font).
func (s *System) DefaultFontID() (FontID, bool) {
	if id, ok := s.FontID(font.New(".SystemUIFont")); ok {
		return id, true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.faces) > 0 {
		return 0, true
	}
	return 0, false
}

// NewFont is a helper for external users of the text package.
func NewFont(family string) font.Font { return font.New(family) }
