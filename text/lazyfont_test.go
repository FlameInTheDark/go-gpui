package text

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/FlameInTheDark/go-gpui/font"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/text/sfnt"
)

// Font fixtures used by the lazy-loading tests. Families and coverage
// differ: DejaVu Sans covers Latin/Cyrillic but no CJK; Liberation Serif
// is a distinct proportional family; Noto Serif SC adds CJK coverage.
const (
	fixtureDejaVu       = "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"
	fixtureLiberation   = "/usr/share/fonts/truetype/liberation/LiberationSerif-Regular.ttf"
	fixtureNotoSerifSC  = "/usr/share/fonts/truetype/chinese/NotoSansSC-Regular.ttf"
	fixtureDejaVuFamily = "DejaVu Sans"
)

// copyFixture copies a font into dir under a walkable .ttf name.
func copyFixture(t *testing.T, src, dir, name string) string {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Skipf("fixture %s unavailable: %v", src, err)
	}
	defer in.Close()
	out, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	return out.Name()
}

// readRecorder instruments readFontFile: which paths were fully read and
// how many bytes each returned.
type readRecorder struct {
	reads map[string]int
	bytes map[string]int
}

func recordReads(rec *readRecorder) func() {
	old := readFontFile
	readFontFile = func(path string) ([]byte, error) {
		data, err := old(path)
		if err == nil {
			rec.reads[path]++
			rec.bytes[path] = len(data)
		}
		return data, err
	}
	return func() { readFontFile = old }
}

// TestLazyFontMaterialization guards the memory fix: registering a font
// directory must NOT read the font files; only the faces actually used
// for shaping are read (here: one file for one family), and the others
// stay un-read.
func TestLazyFontMaterialization(t *testing.T) {
	dir := t.TempDir()
	a := copyFixture(t, fixtureDejaVu, dir, "a-dejavu.ttf")
	b := copyFixture(t, fixtureLiberation, dir, "b-liberation.ttf")

	sys := NewSystem()
	if err := sys.LoadDir(dir); err != nil {
		t.Fatal(err)
	}

	// Registry metadata must be available without any full-file reads.
	names := sys.AllFontNames()
	if !contains(names, fixtureDejaVuFamily) || !contains(names, "Liberation Serif") {
		t.Fatalf("probed families missing: %v", names)
	}

	rec := &readRecorder{reads: map[string]int{}, bytes: map[string]int{}}
	restore := recordReads(rec)
	defer restore()

	id, ok := sys.FontID(font.New(fixtureDejaVuFamily))
	if !ok {
		t.Fatalf("family %q unresolved", fixtureDejaVuFamily)
	}
	line := "Hello, lazy world"
	l := sys.LayoutLine(line, geometry.Px(16), []FontRun{{Len: len(line), FontID: id}})
	if l == nil || len(l.Runs) == 0 {
		t.Fatalf("no shaped runs")
	}
	glyphs := 0
	for _, r := range l.Runs {
		glyphs += len(r.Glyphs)
	}
	if glyphs == 0 {
		t.Fatalf("no glyphs shaped")
	}

	// Exactly one file materialized: the requested family's. ASCII text
	// never triggers a fallback scan, so the second font stays un-read.
	if len(rec.reads) != 1 {
		t.Fatalf("expected exactly 1 full font read after shaping ASCII, got %d: %v", len(rec.reads), rec.reads)
	}
	if n := rec.reads[a]; n != 1 {
		t.Fatalf("expected 1 read of %s, got %d", a, n)
	}
	if n := rec.reads[b]; n != 0 {
		t.Fatalf("unused font was read %d times (must stay lazy)", n)
	}
	// The full read must equal the file size (a real materialization, not
	// a partial probe).
	if fi, err := os.Stat(a); err == nil && int64(rec.bytes[a]) != fi.Size() {
		t.Fatalf("materialization read %d bytes, file is %d", rec.bytes[a], fi.Size())
	}
}

// TestLazyFallbackMaterializesOnDemand: a rune the primary font lacks
// materializes exactly the covering fallback face; faces whose probed
// coverage excludes the rune are never read (the prefilter).
func TestLazyFallbackMaterializesOnDemand(t *testing.T) {
	dir := t.TempDir()
	a := copyFixture(t, fixtureDejaVu, dir, "a-dejavu.ttf")
	c := copyFixture(t, fixtureLiberation, dir, "b-liberation.ttf")
	cjk := copyFixture(t, fixtureNotoSerifSC, dir, "c-noto-sc.ttf")

	sys := NewSystem()
	if err := sys.LoadDir(dir); err != nil {
		t.Fatal(err)
	}

	rec := &readRecorder{reads: map[string]int{}, bytes: map[string]int{}}
	restore := recordReads(rec)
	defer restore()

	id, ok := sys.FontID(font.New(fixtureDejaVuFamily))
	if !ok {
		t.Fatalf("family %q unresolved", fixtureDejaVuFamily)
	}
	const line = "中"
	l := sys.LayoutLine(line, geometry.Px(16), []FontRun{{Len: len(line), FontID: id}})
	if l == nil {
		t.Fatal("nil layout")
	}
	glyphs := 0
	for _, r := range l.Runs {
		glyphs += len(r.Glyphs)
	}
	if glyphs == 0 {
		t.Fatal("fallback produced no glyphs")
	}

	// DejaVu materialized as the run font; the CJK face materialized via
	// the fallback scan; Liberation (no CJK coverage) must NEVER be read —
	// the probed-range prefilter skipped it without loading the file.
	if n := rec.reads[a]; n != 1 {
		t.Errorf("primary font read %d times, want 1", n)
	}
	if n := rec.reads[cjk]; n != 1 {
		t.Errorf("CJK fallback font read %d times, want 1", n)
	}
	if n := rec.reads[c]; n != 0 {
		t.Errorf("non-covering font read %d times: prefilter must skip it", n)
	}
}

// TestProbeFontFile checks the header probe: correct family, style bits
// and coverage semantics — and that probing a truncated header fails
// instead of returning garbage.
func TestProbeFontFile(t *testing.T) {
	if _, err := os.Stat(fixtureDejaVu); err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	meta, ok := probeFontFile(fixtureDejaVu)
	if !ok {
		t.Fatal("probe rejected a valid font")
	}
	if meta.family != fixtureDejaVuFamily {
		t.Fatalf("family = %q, want %q", meta.family, fixtureDejaVuFamily)
	}
	if meta.weightClass != 400 {
		t.Fatalf("weightClass = %d, want 400", meta.weightClass)
	}
	if meta.isBold || meta.isItalic || meta.isOblique {
		t.Fatalf("regular face flagged styled: %+v", meta)
	}
	if !rangesContain(meta.coverage, 'A') || !rangesContain(meta.coverage, 'Ж') {
		t.Fatal("coverage must include Latin and Cyrillic")
	}
	if rangesContain(meta.coverage, '中') {
		t.Fatal("DejaVu has no CJK coverage; ranges must exclude U+4E2D")
	}

	// Probing a 64-byte truncated font must fail (bounds-validated reads).
	data, err := os.ReadFile(fixtureDejaVu)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sfnt.ReadTables(shortReader{data: data[:64]}, []string{"name"}); err == nil {
		t.Fatal("probe of a truncated font must fail")
	}
}

// shortReader exposes only the first n bytes of data via ReadAt.
type shortReader struct {
	data []byte
}

func (s shortReader) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(s.data)) {
		return 0, io.EOF
	}
	n := copy(p, s.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// TestFontMemoryFootprint guards the RAM regression: building the system
// font registry and shaping a typical line must leave only a few MB of
// font data resident. The eager loader retained every system font file
// (133MB+ on a typical Windows install; the sandbox here carries
// ~685MB of fonts) for the process lifetime.
func TestFontMemoryFootprint(t *testing.T) {
	var st0 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&st0)

	rec := &readRecorder{reads: map[string]int{}, bytes: map[string]int{}}
	restore := recordReads(rec)
	defer restore()

	sys := NewSystemWithSystemFonts()
	id, ok := sys.FontID(font.New(fixtureDejaVuFamily))
	if !ok {
		t.Skipf("family %q unresolved", fixtureDejaVuFamily)
	}
	// A line mixing covered and fallback runes, like real editor content.
	line := "The quick brown fox jumps over the lazy dog 中"
	sys.LayoutLine(line, geometry.Px(16), []FontRun{{Len: len(line), FontID: id}})
	sys.FontID(font.New("DejaVu Sans Mono"))

	runtime.GC()
	var st1 runtime.MemStats
	runtime.ReadMemStats(&st1)

	sys.mu.Lock()
	faces := len(sys.faces)
	sys.mu.Unlock()
	delta := int64(st1.HeapAlloc) - int64(st0.HeapAlloc)
	var readBytes int64
	for _, n := range rec.bytes {
		readBytes += int64(n)
	}
	t.Logf("registry: %d faces, %d full-file reads (%.1f MB), heap delta %.1f MB",
		faces, len(rec.reads), float64(readBytes)/1e6, float64(delta)/1e6)

	// Guards (generous to stay robust across font sets): a whole editor
	// session materializes only a handful of faces — never the registry.
	if len(rec.reads) > 8 {
		t.Errorf("too many full font reads (%d): unused faces must stay lazy", len(rec.reads))
	}
	if readBytes > 64<<20 {
		t.Errorf("materialized fonts total %.1f MB: unused faces must stay lazy", float64(readBytes)/1e6)
	}
	if delta > 96<<20 {
		t.Errorf("text system heap delta %.1f MB exceeds the 96 MB guard", float64(delta)/1e6)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if strings.EqualFold(x, want) {
			return true
		}
	}
	return false
}
