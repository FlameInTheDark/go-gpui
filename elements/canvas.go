package elements

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FlameInTheDark/go-gpui/app"
	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/style"
)

// ---------------------------------------------------------------------------
// Canvas
// ---------------------------------------------------------------------------

// Canvas runs arbitrary paint callbacks (upstream canvas element).
type Canvas struct {
	st         *style.StyleRefinement
	prepaintFn func(bounds geometry.Bounds[geometry.Pixels], w *gpui.Window)
	paintFn    func(bounds geometry.Bounds[geometry.Pixels], w *gpui.Window)
}

// NewCanvas creates a canvas with optional prepaint and paint callbacks.
// Each callback receives the canvas bounds in logical pixels and the window;
// either callback may be nil. Use paint to issue window painting operations.
func NewCanvas(prepaint func(bounds geometry.Bounds[geometry.Pixels], w *gpui.Window), paint func(bounds geometry.Bounds[geometry.Pixels], w *gpui.Window)) *Canvas {
	return &Canvas{st: style.NewStyleRefinement(), prepaintFn: prepaint, paintFn: paint}
}

// StyleRef returns the refinement.
func (c *Canvas) StyleRef() *style.StyleRefinement { return c.st }

// ElementID implements Element.
func (c *Canvas) ElementID() gpui.ElementId { return gpui.ElementId{} }

// RequestLayout implements Element.
func (c *Canvas) RequestLayout(w *gpui.Window) {
	st := style.DefaultStyle()
	st.Refine(c.st)
	w.RequestLayout(st, nil)
}

// Prepaint implements Element.
func (c *Canvas) Prepaint(w *gpui.Window, bounds geometry.Bounds[geometry.Pixels]) {
	if c.prepaintFn != nil {
		c.prepaintFn(bounds, w)
	}
}

// Paint implements Element.
func (c *Canvas) Paint(w *gpui.Window) {
	bounds := w.ElementBounds(c)
	st := style.DefaultStyle()
	st.Refine(c.st)
	w.PaintStyle(st, bounds, func() {
		if c.paintFn != nil {
			c.paintFn(bounds, w)
		}
	})
}

// ---------------------------------------------------------------------------
// Deferred (overlays)
// ---------------------------------------------------------------------------

// deferredEntry is one pending overlay draw registered for the current
// frame (upstream Window.deferred_draws).
type deferredEntry struct {
	element   gpui.Element
	offset    geometry.Point[geometry.Pixels]
	mask      geometry.Bounds[geometry.Pixels]
	textStyle style.TextStyle
	remSize   geometry.Pixels
	priority  int
	seq       int // insertion order
	drawCount int // frame the entry belongs to
	painted   bool
	owner     *Deferred
	gate      *deferredPaintGate
}

// deferredDrawRegistry tracks the pending deferred draws per window for the
// current frame. Entries are registered in prepaint order and painted from
// the first Deferred's Paint in priority order (see flushDeferredDraws).
var deferredDrawRegistry = map[*gpui.Window][]*deferredEntry{}

// Deferred paints its child after the rest of the tree (popovers,
// tooltips; upstream deferred()). Deferred children with a higher priority
// draw above lower ones; equal priority keeps insertion order.
type Deferred struct {
	child    gpui.Element
	priority int
}

// NewDeferred defers a child element.
func NewDeferred(child gpui.Element) *Deferred { return &Deferred{child: child} }

// StyleRef satisfies the forwarder surface (styles the child wrapper).
func (d *Deferred) StyleRef() *style.StyleRefinement { return nil }

// WithPriority sets the overlay priority (higher = on top).
func (d *Deferred) WithPriority(p int) *Deferred {
	d.priority = p
	return d
}

// In nests child into this overlay at priority k in one fluent call (the
// condensed equivalent of deferred(child).with_priority(k)). A nil child
// leaves the current child untouched.
func (d *Deferred) In(k int, child gpui.Element) *Deferred {
	d.priority = k
	if child != nil {
		d.child = child
	}
	return d
}

// ElementID implements Element.
func (d *Deferred) ElementID() gpui.ElementId { return gpui.ElementId{} }

// RequestLayout implements Element.
func (d *Deferred) RequestLayout(w *gpui.Window) {
	if d.child != nil {
		w.RequestLayout(style.DefaultStyle(), []gpui.Element{d.child})
	}
}

// Prepaint implements Element.
func (d *Deferred) Prepaint(w *gpui.Window, bounds geometry.Bounds[geometry.Pixels]) {
	if d.child == nil {
		return
	}
	// Prune stale entries from an earlier frame (the flush normally clears
	// them; a frame whose Deferred disappeared leaves them behind).
	dc := w.DrawCount()
	if entries := deferredDrawRegistry[w]; len(entries) > 0 && entries[0].drawCount != dc {
		delete(deferredDrawRegistry, w)
	}
	if node, ok := w.NodeOf(d.child); ok {
		w.PrepaintElement(d.child, node)
	}
	offset := w.ElementOffset()
	entry := &deferredEntry{
		element:   d.child,
		offset:    offset,
		mask:      w.ContentMask(),
		textStyle: w.TextStyle(),
		remSize:   w.RemSize(),
		priority:  d.priority,
		seq:       len(deferredDrawRegistry[w]),
		drawCount: dc,
		owner:     d,
	}
	entry.gate = &deferredPaintGate{entry: entry}
	deferredDrawRegistry[w] = append(deferredDrawRegistry[w], entry)
	// The window paints deferred children through their own Paint calls;
	// the gate replaces the child so painting stays under our control
	// (the child is painted exactly once, in priority order, by
	// flushDeferredDraws — or by the gate as a fallback).
	w.DeferDraw(entry.gate, offset, d.priority)
}

// Paint implements Element: the first deferred element painted this frame
// flushes every pending overlay in priority order (higher priority above
// lower, equal priority in insertion order — upstream deferred z semantics).
func (d *Deferred) Paint(w *gpui.Window) {
	entries := deferredDrawRegistry[w]
	for _, e := range entries {
		if e.owner == d {
			if !e.painted {
				flushDeferredDraws(w)
			}
			return
		}
	}
}

// flushDeferredDraws paints every pending overlay in priority order and
// clears the registry. Each overlay paints under the environment captured
// at its DeferDraw time: the absolute element offset and text style/rem are
// restored exactly; the content mask is intersected with the current mask
// (exact for the nested-popover case where masks nest; sibling overlays
// from disjoint subtrees with narrower masks could clip conservatively).
func flushDeferredDraws(w *gpui.Window) {
	entries := deferredDrawRegistry[w]
	delete(deferredDrawRegistry, w)
	if len(entries) == 0 {
		return
	}
	sorted := append([]*deferredEntry(nil), entries...)
	// Stable sort: lower priority paints first (higher ends on top), equal
	// priority keeps insertion order (upstream deferred priority sort).
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].priority < sorted[j].priority })
	for _, e := range sorted {
		if e.painted {
			continue
		}
		e.painted = true
		prevTextStyle := w.TextStyle()
		prevRem := w.RemSize()
		w.PushTextStyleBase(e.textStyle)
		w.SetRemSize(e.remSize)
		w.WithAbsoluteElementOffset(e.offset, func() {
			w.WithContentMask(e.mask, func() {
				w.PaintElement(e.element)
			})
		})
		w.PushTextStyleBase(prevTextStyle)
		w.SetRemSize(prevRem)
	}
}

// deferredPaintGate stands in for the deferred child in the window's
// deferred draw list. The window calls Paint on each entry between frames'
// paint phases; the gate yields to the flush when it already painted the
// child and paints the child itself (under the entry's environment, which
// the window installs per entry) when the flush never ran.
type deferredPaintGate struct {
	entry *deferredEntry
}

// ElementID implements Element.
func (g *deferredPaintGate) ElementID() gpui.ElementId { return gpui.ElementId{} }

// RequestLayout implements Element (an empty node; the real child has its
// own node from the Deferred's layout).
func (g *deferredPaintGate) RequestLayout(w *gpui.Window) {}

// Prepaint implements Element.
func (g *deferredPaintGate) Prepaint(w *gpui.Window, bounds geometry.Bounds[geometry.Pixels]) {}

// Paint implements Element.
func (g *deferredPaintGate) Paint(w *gpui.Window) {
	if g.entry.painted {
		return // already painted by the flush
	}
	// Fallback: the flush never ran (the owning Deferred never painted);
	// paint the child here, under this entry's exact environment.
	g.entry.painted = true
	w.PaintElement(g.entry.element)
}

// ---------------------------------------------------------------------------
// Image
// ---------------------------------------------------------------------------

// decodedImage is one entry of the image cache (upstream asset_cache
// CachedLoad: a shared loading/loaded cell keyed by the resource).
type decodedImage struct {
	img  image.Image   // nil until the load settles
	err  error         // decode/fetch error (renders nothing afterwards)
	done chan struct{} // closed exactly once the load settles
	gen  uint64        // cache generation this entry belongs to
}

// imageCache is the package-level decoded-image cache (upstream
// asset_cache.rs: a global, EntityMap-scoped cache). Entries are keyed by
// source (plus a baked-tint suffix) and scoped by a per-app generation so
// InvalidateImageCache can drop everything without touching live frames.
type imageCache struct {
	mu          sync.RWMutex
	entries     map[string]*decodedImage
	generations map[*app.App]uint64
}

// imgCache is the process-wide image cache.
var imgCache = &imageCache{
	entries:     map[string]*decodedImage{},
	generations: map[*app.App]uint64{},
}

// imageCacheGeneration returns the current cache generation for the
// window's app (0 for nil apps).
func imageCacheGeneration(w *gpui.Window) uint64 {
	if w == nil || w.App() == nil {
		return 0
	}
	imgCache.mu.RLock()
	defer imgCache.mu.RUnlock()
	return imgCache.generations[w.App()]
}

// imageCacheGet returns the entry for key when it exists and belongs to the
// current generation. Loading entries are returned with done still open.
func imageCacheGet(key string, gen uint64) (*decodedImage, bool) {
	imgCache.mu.RLock()
	defer imgCache.mu.RUnlock()
	e, ok := imgCache.entries[key]
	if !ok || e.gen != gen {
		return nil, false
	}
	return e, true
}

// imageCacheStore inserts entry under key if absent (first writer wins, so
// concurrent loads share one decode).
func imageCacheStore(key string, gen uint64, e *decodedImage) *decodedImage {
	imgCache.mu.Lock()
	defer imgCache.mu.Unlock()
	if existing, ok := imgCache.entries[key]; ok && existing.gen == gen {
		return existing
	}
	if e.gen != gen {
		e.gen = gen
	}
	imgCache.entries[key] = e
	return e
}

// InvalidateImageCache drops every cached decoded image belonging to the
// window's app by bumping the cache generation (upstream asset cache
// invalidation); subsequent loads decode afresh.
func InvalidateImageCache(w *gpui.Window) {
	a := w.App()
	if a == nil {
		return
	}
	imgCache.mu.Lock()
	defer imgCache.mu.Unlock()
	imgCache.generations[a]++
	gen := imgCache.generations[a]
	for key, e := range imgCache.entries {
		if e.gen != gen {
			delete(imgCache.entries, key)
		}
	}
}

// imageCacheLen returns the number of live cache entries (diagnostics/tests).
func imageCacheLen() int {
	imgCache.mu.RLock()
	defer imgCache.mu.RUnlock()
	return len(imgCache.entries)
}

// httpClient is the shared fetch client for http(s) image sources.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// decodeImageSource loads and decodes PNG/JPEG bytes for a source string.
// URIs go through http(s):// or data:; everything else is treated as a
// filesystem path, preferring the window's asset source over disk.
func decodeImageSource(source string, isURI bool, loader func(path string) ([]byte, bool)) (image.Image, error) {
	var data []byte
	var err error
	switch {
	case isURI && strings.HasPrefix(source, "data:"):
		payload := source
		if i := strings.IndexByte(payload, ','); i >= 0 {
			payload = payload[i+1:]
		}
		data, err = base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return nil, err
		}
	case isURI && (strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://")):
		resp, herr := httpClient.Get(source)
		if herr != nil {
			return nil, herr
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("img: http status %d for %s", resp.StatusCode, source)
		}
		data, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
	default:
		if loader != nil {
			if d, ok := loader(source); ok && len(d) > 0 {
				data = d
			}
		}
		if len(data) == 0 {
			data, err = os.ReadFile(source)
			if err != nil {
				return nil, err
			}
		}
	}
	return decodeImageBytes(data)
}

// decodeImageBytes sniffs PNG/JPEG magic bytes and decodes accordingly.
func decodeImageBytes(data []byte) (image.Image, error) {
	if len(data) >= 8 && data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G' {
		return png.Decode(bytes.NewReader(data))
	}
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xD8 {
		return jpeg.Decode(bytes.NewReader(data))
	}
	// Unknown format: try PNG then JPEG.
	if img, err := png.Decode(bytes.NewReader(data)); err == nil {
		return img, nil
	}
	return jpeg.Decode(bytes.NewReader(data))
}

// tintImage bakes a color multiply into a copy of img (upstream img tint,
// implemented by pre-multiplying the atlas tile pixels). Tinting
// premultiplied components by the tint keeps the result premultiplied, and
// an opaque white tint is the identity.
func tintImage(img image.Image, tint gc.Hsla) *image.RGBA {
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	rgba := tint.Rgba()
	tr, tg, tb, ta := uint32(rgba.R*0xffff), uint32(rgba.G*0xffff), uint32(rgba.B*0xffff), uint32(rgba.A*0xffff)
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, g, bl, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			out.SetRGBA(x, y, color.RGBA{
				uint8(r * tr >> 16),
				uint8(g * tg >> 16),
				uint8(bl * tb >> 16),
				uint8(a * ta >> 16),
			})
		}
	}
	return out
}

// tintCacheKey derives the tint-baked cache key for a decoded source.
func tintCacheKey(sourceKey string, tint gc.Hsla) string {
	rgba := tint.Rgba()
	return fmt.Sprintf("tint:%s|%02x%02x%02x%02x", sourceKey,
		uint8(rgba.R*255), uint8(rgba.G*255), uint8(rgba.B*255), uint8(rgba.A*255))
}

// Img displays a decoded image with an object fit. Images can be provided
// directly (NewImg) or by URI/path, in which case they are decoded
// asynchronously (upstream img: a "loading" state renders nothing until the
// background decode completes and schedules a repaint).
type Img struct {
	st        *style.StyleRefinement
	src       image.Image
	source    string // URI or path; empty when a direct image was given
	isURI     bool   // source is a URI (data:/http(s)://) rather than a path
	objectFit style.ObjectFit
	grayscale bool
	tint      *gc.Hsla
	loading   bool
}

// NewImg creates an image element.
func NewImg(src image.Image) *Img {
	return &Img{st: style.NewStyleRefinement(), src: src, objectFit: style.ObjectFitContain}
}

// NewImgFromPNG decodes PNG bytes.
func NewImgFromPNG(data []byte) (*Img, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return NewImg(img), nil
}

// NewImgFromURI creates an image element that asynchronously decodes the
// resource at uri (data:, http(s)://, or a filesystem path).
func NewImgFromURI(uri string) *Img {
	return (&Img{st: style.NewStyleRefinement(), objectFit: style.ObjectFitContain}).SourceURI(uri)
}

// NewImgFromPath creates an image element that asynchronously loads the
// file at path (through the window asset source when one is installed).
func NewImgFromPath(path string) *Img {
	return (&Img{st: style.NewStyleRefinement(), objectFit: style.ObjectFitContain}).SourcePath(path)
}

// StyleRef returns the refinement.
func (i *Img) StyleRef() *style.StyleRefinement { return i.st }

// ObjectFit sets the fit mode.
func (i *Img) ObjectFit(fit style.ObjectFit) *Img {
	i.objectFit = fit
	return i
}

// Grayscale renders without color.
func (i *Img) Grayscale(v bool) *Img {
	i.grayscale = v
	return i
}

// SourceURI loads the image asynchronously from uri (upstream
// ImageSource::Resource::Uri / SharedUri). data: URIs are decoded
// inline, http(s):// is fetched on a background executor task and any
// other value falls back to the filesystem.
func (i *Img) SourceURI(uri string) *Img {
	i.source = uri
	i.isURI = true
	i.src = nil
	return i
}

// SourcePath loads the image asynchronously from path (upstream
// ImageSource::Resource::Path): the window's asset source is preferred,
// then the filesystem.
func (i *Img) SourcePath(path string) *Img {
	i.source = path
	i.isURI = false
	i.src = nil
	return i
}

// Tint tints the image with a color multiply (upstream img tint via the
// polychrome sprite). The tint is baked when the image is uploaded and
// keyed in the cache, so switching tint reuses the shared decode.
func (i *Img) Tint(color gc.Hsla) *Img {
	i.tint = &color
	return i
}

// IsLoading reports whether an asynchronous source decode is still running
// (upstream img's loading state: nothing is rendered meanwhile).
func (i *Img) IsLoading() bool { return i.loading }

// ElementID implements Element.
func (i *Img) ElementID() gpui.ElementId { return gpui.ElementId{} }

// RequestLayout implements Element.
func (i *Img) RequestLayout(w *gpui.Window) {
	st := style.DefaultStyle()
	st.Refine(i.st)
	w.RequestLayout(st, nil)
}

// Prepaint implements Element.
func (i *Img) Prepaint(w *gpui.Window, bounds geometry.Bounds[geometry.Pixels]) {}

// resolve returns the image to draw this frame, kicking off the background
// decode for URI/path sources. It returns nil while loading or on error
// (upstream renders nothing in the loading state).
func (i *Img) resolve(w *gpui.Window) image.Image {
	if i.src != nil {
		if i.tint == nil {
			return i.src
		}
		return i.baked(w, i.src, tintCacheKey(fmt.Sprintf("direct:%p", i.src), *i.tint))
	}
	if i.source == "" {
		return nil
	}
	gen := imageCacheGeneration(w)
	decodeKey := "src:" + i.source
	// Fast path: the final image (decoded, tint baked) is already cached.
	if i.tint != nil {
		if entry, ok := imageCacheGet(tintCacheKey(decodeKey, *i.tint), gen); ok {
			select {
			case <-entry.done:
				i.loading = false
				if entry.err == nil && entry.img != nil {
					return entry.img
				}
				return nil
			default:
			}
		}
	}
	entry, ok := imageCacheGet(decodeKey, gen)
	if !ok {
		i.startLoad(w, decodeKey, gen)
		return nil
	}
	select {
	case <-entry.done:
		i.loading = false
		if entry.err != nil || entry.img == nil {
			return nil
		}
		if i.tint == nil {
			return entry.img
		}
		return i.baked(w, entry.img, tintCacheKey(decodeKey, *i.tint))
	default:
		i.loading = true
		return nil
	}
}

// baked returns the tint-baked image under key, computing and caching it on
// first use (the bake runs on the calling thread and publishes a settled
// entry).
func (i *Img) baked(w *gpui.Window, img image.Image, key string) image.Image {
	gen := imageCacheGeneration(w)
	if entry, ok := imageCacheGet(key, gen); ok {
		select {
		case <-entry.done:
			if entry.err == nil && entry.img != nil {
				return entry.img
			}
			return img
		default:
			return img // bake in flight: draw untinted rather than nothing
		}
	}
	tinted := tintImage(img, *i.tint)
	done := make(chan struct{})
	close(done)
	imageCacheStore(key, gen, &decodedImage{img: tinted, done: done})
	return tinted
}

// startLoad begins the background decode of the img source (upstream
// CachedLoad: one shared task per resource; the result is published on the
// main thread and the window is refreshed for a repaint).
func (i *Img) startLoad(w *gpui.Window, decodeKey string, gen uint64) {
	i.loading = true
	entry := imageCacheStore(decodeKey, gen, &decodedImage{done: make(chan struct{})})
	if entry == nil {
		return // another load owns the entry
	}
	select {
	case <-entry.done:
		i.loading = false
		return // settled between the get and store races
	default:
	}
	source, isURI := i.source, i.isURI
	loader := w.AssetSource()
	task := func() {
		img, err := decodeImageSource(source, isURI, loader)
		// publishResult writes the decode outcome (thread-safe: the cache
		// entry is handed over through the closed done channel).
		publishResult := func() {
			entry.img, entry.err = img, err
			close(entry.done)
		}
		if app.MainDispatcherInstalled() {
			// Publish and schedule the repaint on the main thread (upstream
			// notifies the watching views after the load resolves).
			app.DispatchOnMainThread(func() {
				publishResult()
				w.Refresh()
			})
			return
		}
		// No platform dispatcher installed (tests, single-threaded hosts):
		// publish the result without touching window state; the next frame
		// picks the image up.
		publishResult()
	}
	if a := w.App(); a != nil && a.Background() != nil {
		a.Background().Spawn(task)
	} else {
		go task()
	}
}

// Paint implements Element.
func (i *Img) Paint(w *gpui.Window) {
	img := i.resolve(w)
	if img == nil {
		return // no source, still loading, or failed: render nothing
	}
	bounds := w.ElementBounds(i)
	st := style.DefaultStyle()
	st.Refine(i.st)
	fitBounds := objectFitBounds(i.objectFit, bounds, img.Bounds().Dx(), img.Bounds().Dy())
	w.PaintQuad(st, bounds)
	w.PaintImage(fitBounds, geometry.ZeroCornersAbsolute().ToPixels(w.RemSize()), img, i.grayscale)
}

// objectFitBounds computes the drawn rect (upstream ObjectFit::get_bounds).
func objectFitBounds(fit style.ObjectFit, bounds geometry.Bounds[geometry.Pixels], iw, ih int) geometry.Bounds[geometry.Pixels] {
	imageW, imageH := float32(iw), float32(ih)
	bw, bh := bounds.Size.Width.AsF32(), bounds.Size.Height.AsF32()
	if imageW <= 0 || imageH <= 0 || bw <= 0 || bh <= 0 {
		return bounds
	}
	scaleContain := minf(bw/imageW, bh/imageH)
	var w, h float32
	switch fit {
	case style.ObjectFitFill:
		w, h = bw, bh
	case style.ObjectFitContain:
		w, h = imageW*scaleContain, imageH*scaleContain
	case style.ObjectFitCover:
		s := maxf(bw/imageW, bh/imageH)
		w, h = imageW*s, imageH*s
	case style.ObjectFitScaleDown:
		if imageW <= bw && imageH <= bh {
			w, h = imageW, imageH
		} else {
			w, h = imageW*scaleContain, imageH*scaleContain
		}
	default: // None
		w, h = imageW, imageH
	}
	return geometry.Bounds[geometry.Pixels]{
		Origin: geometry.Point[geometry.Pixels]{
			X: bounds.Origin.X + geometry.Px((bw-w)/2),
			Y: bounds.Origin.Y + geometry.Px((bh-h)/2),
		},
		Size: geometry.Size[geometry.Pixels]{Width: geometry.Px(w), Height: geometry.Px(h)},
	}
}

func minf(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func maxf(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

// ---------------------------------------------------------------------------
// Animation element
// ---------------------------------------------------------------------------

// Easing maps normalized animation time (0..1) to an animated value
// (upstream easing functions: Fn(f32) -> f32; back/elastic curves may
// overshoot the 0..1 range).
type Easing func(float32) float32

// Easing constants shared by the back/elastic families
// (easings.net reference values, also used by upstream-style curves).
const (
	easeBackS1 = float32(1.70158)
	easeBackS2 = float32(2.5949095)
	easeElastC = float32(2 * math.Pi / 3)
)

// EaseInQuad starts slowly and accelerates (delta * delta).
func EaseInQuad() Easing { return func(t float32) float32 { return t * t } }

// EaseOutQuad starts quickly and decelerates.
func EaseOutQuad() Easing { return func(t float32) float32 { return 1 - (1-t)*(1-t) } }

// EaseInOutQuad starts and ends slowly, speeding up in the middle.
func EaseInOutQuad() Easing { return EaseInOut }

// EaseInCubic accelerates cubically.
func EaseInCubic() Easing { return func(t float32) float32 { return t * t * t } }

// EaseOutCubic decelerates from a cubic start.
func EaseOutCubic() Easing {
	return func(t float32) float32 {
		u := 1 - t
		return 1 - u*u*u
	}
}

// EaseInOutCubic combines cubic in/out.
func EaseInOutCubic() Easing {
	return func(t float32) float32 {
		if t < 0.5 {
			return 4 * t * t * t
		}
		u := -2*t + 2
		return 1 - u*u*u/2
	}
}

// EaseInQuart accelerates quartically.
func EaseInQuart() Easing { return func(t float32) float32 { return t * t * t * t } }

// EaseOutQuart decelerates from a quartic start.
func EaseOutQuart() Easing {
	return func(t float32) float32 {
		u := 1 - t
		return 1 - u*u*u*u
	}
}

// EaseInOutQuart combines quartic in/out.
func EaseInOutQuart() Easing {
	return func(t float32) float32 {
		if t < 0.5 {
			return 8 * t * t * t * t
		}
		u := -2*t + 2
		return 1 - u*u*u*u/2
	}
}

// EaseInQuint accelerates quintically.
func EaseInQuint() Easing { return func(t float32) float32 { return t * t * t * t * t } }

// EaseInOutQuint combines quintic in/out (out is the existing EaseOutQuint).
func EaseInOutQuint() Easing {
	return func(t float32) float32 {
		if t < 0.5 {
			return 16 * t * t * t * t * t
		}
		u := -2*t + 2
		return 1 - u*u*u*u*u/2
	}
}

// EaseInSine eases in with a sine quarter-wave.
func EaseInSine() Easing { return func(t float32) float32 { return 1 - cosf(t*math.Pi/2) } }

// EaseOutSine eases out with a sine quarter-wave.
func EaseOutSine() Easing { return func(t float32) float32 { return sinf(t * math.Pi / 2) } }

// EaseInOutSine combines sine in/out.
func EaseInOutSine() Easing {
	return func(t float32) float32 { return -(cosf(math.Pi*t) - 1) / 2 }
}

// EaseInExpo eases in exponentially (zero until motion starts).
func EaseInExpo() Easing {
	return func(t float32) float32 {
		if t == 0 {
			return 0
		}
		return powf(2, 10*t-10)
	}
}

// EaseOutExpo eases out exponentially.
func EaseOutExpo() Easing {
	return func(t float32) float32 {
		if t == 1 {
			return 1
		}
		return 1 - powf(2, -10*t)
	}
}

// EaseInOutExpo combines exponential in/out.
func EaseInOutExpo() Easing {
	return func(t float32) float32 {
		switch {
		case t == 0:
			return 0
		case t == 1:
			return 1
		case t < 0.5:
			return powf(2, 20*t-10) / 2
		default:
			return (2 - powf(2, -20*t+10)) / 2
		}
	}
}

// EaseInCirc eases in with a circular arc.
func EaseInCirc() Easing {
	return func(t float32) float32 { return 1 - sqrtf(1-t*t) }
}

// EaseOutCirc eases out with a circular arc.
func EaseOutCirc() Easing {
	return func(t float32) float32 {
		u := t - 1
		return sqrtf(1 - u*u)
	}
}

// EaseInOutCirc combines circular in/out.
func EaseInOutCirc() Easing {
	return func(t float32) float32 {
		if t < 0.5 {
			return (1 - sqrtf(1-(2*t)*(2*t))) / 2
		}
		u := -2*t + 2
		return (sqrtf(1-u*u) + 1) / 2
	}
}

// EaseInBack eases in with a slight backwards pull (overshoots below 0).
func EaseInBack() Easing {
	c3 := easeBackS1 + 1
	return func(t float32) float32 { return c3*t*t*t - easeBackS1*t*t }
}

// EaseOutBack eases out with a slight overshoot past the target.
func EaseOutBack() Easing {
	c3 := easeBackS1 + 1
	return func(t float32) float32 {
		u := t - 1
		return 1 + c3*u*u*u + easeBackS1*u*u
	}
}

// EaseInOutBack combines back in/out.
func EaseInOutBack() Easing {
	return func(t float32) float32 {
		if t < 0.5 {
			x := 2 * t
			return ((x * x) * ((easeBackS2+1)*x - easeBackS2)) / 2
		}
		x := 2*t - 2
		return ((x*x)*((easeBackS2+1)*x+easeBackS2) + 2) / 2
	}
}

// EaseInElastic eases in with an elastic wind-up.
func EaseInElastic() Easing {
	return func(t float32) float32 {
		switch {
		case t == 0:
			return 0
		case t == 1:
			return 1
		}
		return -powf(2, 10*t-10) * sinf((10*t-10.75)*easeElastC)
	}
}

// EaseOutElastic eases out with an elastic release.
func EaseOutElastic() Easing {
	return func(t float32) float32 {
		switch {
		case t == 0:
			return 0
		case t == 1:
			return 1
		}
		return powf(2, -10*t)*sinf((10*t-0.75)*easeElastC) + 1
	}
}

// EaseInOutElastic combines elastic in/out.
func EaseInOutElastic() Easing {
	c := (2 * float32(math.Pi)) / 4.5
	return func(t float32) float32 {
		switch {
		case t == 0:
			return 0
		case t == 1:
			return 1
		case t < 0.5:
			return -(powf(2, 20*t-10) * sinf((20*t-11.125)*c)) / 2
		default:
			return (powf(2, -20*t+10)*sinf((20*t-11.125)*c))/2 + 1
		}
	}
}

// EaseOutBounce eases out with a ball-like bounce.
func EaseOutBounce() Easing {
	return func(t float32) float32 {
		const n1 = 7.5625
		const d1 = 2.75
		switch {
		case t < 1/d1:
			return n1 * t * t
		case t < 2/d1:
			u := t - 1.5/d1
			return n1*u*u + 0.75
		case t < 2.5/d1:
			u := t - 2.25/d1
			return n1*u*u + 0.9375
		default:
			u := t - 2.625/d1
			return n1*u*u + 0.984375
		}
	}
}

// EaseInBounce eases in with a ball-like bounce.
func EaseInBounce() Easing {
	out := EaseOutBounce()
	return func(t float32) float32 { return 1 - out(1-t) }
}

// EaseInOutBounce combines bounce in/out.
func EaseInOutBounce() Easing {
	out := EaseOutBounce()
	return func(t float32) float32 {
		if t < 0.5 {
			return (1 - out(1-2*t)) / 2
		}
		return (1 + out(2*t-1)) / 2
	}
}

// EaseHeartbeat is a heartbeat: a strong beat, a weaker one, then a rest.
// Starts and ends at 0 and peaks at 1 (upstream easing::heartbeat).
func EaseHeartbeat() Easing {
	beat := func(center, width, height, delta float32) float32 {
		x := (delta - center) / width
		return height * expf(-x*x)
	}
	return func(delta float32) float32 {
		return beat(0.2, 0.06, 1.0, delta) + beat(0.4, 0.05, 0.58, delta)
	}
}

// Bounce applies the given easing first forward and then in reverse
// (upstream easing::bounce).
func Bounce(easing func(float32) float32) Easing {
	return func(delta float32) float32 {
		if delta < 0.5 {
			return easing(delta * 2)
		}
		return easing((1 - delta) * 2)
	}
}

// PulsatingBetween produces a breathing alpha between min and max
// (upstream easing::pulsating_between).
func PulsatingBetween(min, max float32) Easing {
	rng := max - min
	return func(delta float32) float32 {
		t := sinf(delta * 2 * math.Pi)
		breath := (t*t*t + t) / 2
		normalized := (breath + 1) / 2
		return min + normalized*rng
	}
}

// math shims keep the easing formulas readable with float32 math.
func cosf(v float32) float32  { return float32(math.Cos(float64(v))) }
func sinf(v float32) float32  { return float32(math.Sin(float64(v))) }
func expf(v float32) float32  { return float32(math.Exp(float64(v))) }
func sqrtf(v float32) float32 { return float32(math.Sqrt(float64(v))) }
func powf(b, e float32) float32 {
	return float32(math.Pow(float64(b), float64(e)))
}

// Easing functions (upstream easing module).
func EaseLinear(t float32) float32    { return t }
func EaseQuadratic(t float32) float32 { return t * t }
func EaseInOut(t float32) float32 {
	if t < 0.5 {
		return 2 * t * t
	}
	return 1 - float32(-2*t+2)*(-2*t+2)/2
}
func EaseOutQuint(t float32) float32 {
	u := t - 1
	return u*u*u*u*u + 1
}

// Animation describes a duration-based animation.
type Animation struct {
	DurationMS int64
	Repeat     bool
	// Autoreverse mirrors the direction on every repeat (upstream-style
	// ping-pong): odd repeats run backwards. Only meaningful with Repeat.
	Autoreverse bool
	Easing      func(float32) float32
}

// NewAnimation creates an animation.
func NewAnimation(durationMS int64) *Animation {
	return &Animation{DurationMS: durationMS, Easing: EaseLinear}
}

// WithRepeat loops the animation.
func (a *Animation) WithRepeat() *Animation {
	a.Repeat = true
	return a
}

// WithAutoreverse mirrors the animation direction on every repeat: odd
// repeats run backwards (direction alternation).
func (a *Animation) WithAutoreverse() *Animation {
	a.Autoreverse = true
	return a
}

// WithEasing sets the easing curve.
func (a *Animation) WithEasing(f func(float32) float32) *Animation {
	a.Easing = f
	return a
}

// animState is the persistent animation clock.
type animState struct {
	startMS int64
	done    bool
}

// AnimationElement wraps a child with an animated value.
type AnimationElement struct {
	child     gpui.Element
	anim      *Animation
	animator  func(child gpui.Element, t float32) gpui.Element
	id        gpui.ElementId
	lastChild gpui.Element
}

// NewAnimationElement wraps a child.
func NewAnimationElement(id gpui.ElementId, child gpui.Element, anim *Animation, animator func(child gpui.Element, t float32) gpui.Element) *AnimationElement {
	return &AnimationElement{id: id, child: child, anim: anim, animator: animator}
}

// WithAnimation is the fluent API (upstream AnimationExt::with_animation).
func WithAnimation(id gpui.ElementId, anim *Animation, child gpui.Element, animator func(child gpui.Element, t float32) gpui.Element) *AnimationElement {
	return NewAnimationElement(id, child, anim, animator)
}

// ElementID implements Element.
func (a *AnimationElement) ElementID() gpui.ElementId { return a.id }

// RequestLayout implements Element: the animated child's layout node
// becomes this element's node (the animator may rebuild the child each
// frame, exactly like upstream).
func (a *AnimationElement) RequestLayout(w *gpui.Window) {
	current := a.currentChild(w)
	a.lastChild = current
	if current != nil {
		w.RequestLayoutElement(current)
	}
}

// Prepaint implements Element.
func (a *AnimationElement) Prepaint(w *gpui.Window, bounds geometry.Bounds[geometry.Pixels]) {
	if a.lastChild != nil {
		if node, ok := w.NodeOf(a.lastChild); ok {
			w.PrepaintElement(a.lastChild, node)
		}
	}
}

// Paint implements Element.
func (a *AnimationElement) Paint(w *gpui.Window) {
	if a.lastChild != nil {
		w.PaintElement(a.lastChild)
	}
}

// currentChild builds the animated child for this frame.
func (a *AnimationElement) currentChild(w *gpui.Window) gpui.Element {
	if a.child == nil || a.anim == nil {
		return nil
	}
	var t float32
	gpui.UseElementState(w, func() animState {
		return animState{startMS: w.NowMilliseconds()}
	}, func(s *animState) {
		elapsed := w.NowMilliseconds() - s.startMS
		if elapsed < 0 {
			elapsed = 0
		}
		if a.anim.DurationMS <= 0 {
			t = 1
			return
		}
		total := elapsed / a.anim.DurationMS
		progress := float32(elapsed%a.anim.DurationMS) / float32(a.anim.DurationMS)
		if a.anim.Repeat {
			if a.anim.Autoreverse && total%2 == 1 {
				// Direction alternation: odd repeats run backwards.
				progress = 1 - progress
			}
		} else if total > 0 {
			progress = 1
			s.done = true
		}
		if a.anim.Easing != nil {
			t = a.anim.Easing(progress)
		} else {
			t = progress
		}
	})
	return a.animator(a.child, t)
}

var _ = strings.TrimSpace
var _ = strconv.Itoa
