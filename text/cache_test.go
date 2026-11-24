package text

import (
	"testing"

	"github.com/FlameInTheDark/go-gpui/geometry"
)

// TestLayoutLineCache verifies the shaped-line cache: identical inputs
// return the shared layout, and the cache entry is not corrupted by the
// wrap-state mutation callers apply to their clones (elements/text.go).
func TestLayoutLineCache(t *testing.T) {
	sys := NewSystem()
	if err := sys.LoadDir("/usr/share/fonts/truetype"); err != nil && len(sys.faces) == 0 {
		t.Skipf("no system fonts: %v", err)
	}
	if _, ok := sys.DefaultFontID(); !ok {
		t.Skip("no default font")
	}
	const line = "func Fib(n int) int { // cached"
	runs := []FontRun{{Len: len(line), FontID: 0}}
	size := geometry.Px(14)

	a := sys.LayoutLine(line, size, runs)
	b := sys.LayoutLine(line, size, runs)
	if a != b {
		t.Fatal("LayoutLine returned different layouts for identical input (cache miss)")
	}

	// Simulate a caller cloning and mutating (as elements.Text.shape does):
	c := *a
	c.WrapLines = 3
	c.Wrapped = &WrappedLineLayout{Unwrapped: a}
	if a.WrapLines != 0 || a.Wrapped != nil {
		t.Fatalf("cached layout was mutated through the clone: WrapLines=%d Wrapped=%v", a.WrapLines, a.Wrapped)
	}

	// A different size must not alias the first cache entry.
	d := sys.LayoutLine(line, geometry.Px(16), runs)
	if d == a {
		t.Fatal("different font size shared a cache entry")
	}
	// Distinct text must not alias either.
	e := sys.LayoutLine(line+"x", size, runs)
	if e == a || e.Width == a.Width {
		t.Fatal("different text shared a cache entry")
	}
}
