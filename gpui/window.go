package gpui

import (
	"hash/fnv"
	"image"
	"image/color"
	"reflect"
	"sync"
	"time"

	"github.com/FlameInTheDark/go-gpui/app"
	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gestures"
	"github.com/FlameInTheDark/go-gpui/keymap"
	"github.com/FlameInTheDark/go-gpui/layout"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/render"
	"github.com/FlameInTheDark/go-gpui/style"
	"github.com/FlameInTheDark/go-gpui/text"
)

// DrawPhase is the current frame phase.
type DrawPhase uint8

const (
	PhaseNone DrawPhase = iota
	PhasePrepaint
	PhasePaint
)

// HitboxBehavior controls mouse occlusion.
type HitboxBehavior uint8

const (
	// HitboxNormal participates normally in hit testing.
	HitboxNormal HitboxBehavior = 0
	// HitboxBlockMouse occludes mouse events for elements below.
	HitboxBlockMouse HitboxBehavior = 1
	// HitboxBlockMouseExceptScroll occludes clicks but lets scroll pass.
	HitboxBlockMouseExceptScroll HitboxBehavior = 2
)

// HitboxID identifies a hitbox for one frame.
type HitboxID uint64

// Hitbox is a mouse-interactive region (upstream Hitbox).
type Hitbox struct {
	ID          HitboxID
	Bounds      geometry.Bounds[geometry.Pixels]
	ContentMask render.ContentMask
	Behavior    HitboxBehavior
	// Key is the owning element's stable identity (ElementId.HitboxKey).
	// Pointer captures match on it so a drag survives re-renders that
	// change the tree's structure (and with it the per-frame numeric
	// ids): a resize grip keeps dragging while the panel's scrollbars
	// appear or disappear mid-gesture.
	Key string
}

// hitTest is the frame's hover state.
type hitTest struct {
	ids              []HitboxID
	hoverHitboxCount int
}

// mouseListener is a paint-registered mouse handler.
type mouseListener struct {
	kind    string // "down","up","move","exit","scroll","click"...
	button  platform.MouseButton
	hitbox  Hitbox
	handler func(ev any, phase DispatchPhase, w *Window)
	// capture-only listeners run in the capture phase.
	capture bool
	// stopAfter suppresses bubble dispatch after capture handled.
	once bool
	// depth is the element nesting at registration (children register
	// before their parents' listeners, since a div paints its children
	// first); bubble dispatch must still run innermost listeners first.
	depth int
	// seq is the registration order.
	seq int
}

// DispatchPhase is capture or bubble.
type DispatchPhase uint8

const (
	// CapturePhase runs outermost-to-innermost before bubble.
	CapturePhase DispatchPhase = 0
	// BubblePhase runs innermost-to-outermost.
	BubblePhase DispatchPhase = 1
)

// keyListener is a paint-registered key handler.
type keyListener struct {
	kind    string // "down","up"
	handler func(ev any, phase DispatchPhase, w *Window)
	context string // key context
	// focus, when set, restricts the listener to dispatches where this
	// handle held focus AT DISPATCH START (a handler that moves focus
	// mid-dispatch must not re-target later listeners of the same event).
	focus *FocusHandle
}

// frame is the double-buffered draw state (upstream Frame).
type frame struct {
	focus           *FocusHandle
	elementState    map[uint64]any
	hitboxes        []Hitbox
	mouseListeners  []mouseListener
	keyListeners    []keyListener
	actionListeners []actionListener
	scene           *render.Scene
	tabStops        []TabStop
	// cursorStyles are the paint-time cursor requests (upstream
	// Frame.cursor_styles; resolved against the mouse hit test at the end
	// of the frame).
	cursorStyles []cursorStyleRequest
	// a11yNodes are the accessibility nodes recorded during the frame
	// (upstream accesskit tree).
	a11yNodes []AccessibilityNode
}

type actionBarListener func(action any, phase DispatchPhase, w *Window)

type actionListener struct {
	typeName string
	listener actionBarListener
}

// TabStop records a focusable element's tab order.
type TabStop struct {
	Focus *FocusHandle
	Index int
}

// Window is the frame orchestrator (a condensed port of gpui's Window).
type Window struct {
	app   *app.App
	id    app.WindowID
	plat  platform.PlatformWindow
	text  *text.System
	atlas *render.Atlas

	// Root element + invalidate state.
	root        Element
	rootView    app.EntityID
	invalidator *invalidator
	dirty       bool
	refreshing  bool
	drawPhase   DrawPhase
	drawCount   int

	// Layout (rebuilt each frame).
	layoutTree      *layout.Tree
	layoutBoundsMap map[LayoutID]geometry.Bounds[geometry.Pixels]
	pendingChildren [][]LayoutID

	// Draw environment stacks.
	idPath         idPathStack
	textStyle      style.TextStyle
	textStyleStack []style.TextStyleRefinement
	textStyleBase  []style.TextStyle // folded styles
	elementOffset  geometry.Point[geometry.Pixels]
	contentMask    geometry.Bounds[geometry.Pixels] // pixel-space clip
	remSize        geometry.Pixels
	remOverride    []geometry.Pixels
	scaleFactor    float32
	elementOpacity float32

	// Element state handoff.
	renderedFrame frame
	nextFrame     frame

	// Input state.
	mousePosition  geometry.Point[geometry.Pixels]
	mouseHitTest   hitTest
	modifiers      keymap.Modifiers
	capturedHitbox *HitboxID
	// capturedKey is the captured element's stable identity (see
	// Hitbox.Key): captures match by key when present, by numeric id
	// otherwise, so drags survive structural re-renders.
	capturedKey          string
	activeListenerHitbox *Hitbox
	propagate            bool
	defaultPrevented     bool
	listenersFired       int
	lastInputKeyboard    bool
	// dispatchFocusStart snapshots the focused handle at the start of a
	// mouse dispatch: nested focusable elements use it to give the
	// innermost click-focus priority (an outer container must not steal
	// focus a deeper element already claimed during the same event).
	dispatchFocusStart FocusHandle
	// pressedButtons tracks held mouse buttons (upstream
	// platform_window mouse state). Platforms do not report the held
	// button on WM_MOUSEMOVE-style move events, but Dragging() depends
	// on it: drag-to-select and drag interactions need the stamp below.
	pressedButtons map[platform.MouseButton]bool

	// activeDrag is the drag-and-drop gesture in flight (upstream
	// cx.active_drag).
	activeDrag *DragState

	// touchGestures is the portable touch-gesture recognizer (upstream
	// window.touch_gestures); it synthesizes mouse taps, pans and
	// momentum from raw platform.TouchEvents.
	touchGestures *gestures.TouchGestureRecognizer
	// longPressTimer cancels the pending long-press offer timer.
	longPressTimer *time.Timer

	// frameTooltip is the tooltip overlay scheduled for this frame (upstream
	// next_frame.tooltip_requests).
	frameTooltip    Element
	frameTooltipPos geometry.Point[geometry.Pixels]

	// prompts is the modal prompt stack (upstream window.prompts). The top
	// prompt renders centered above everything and captures keys.
	prompts []*promptLayer

	// a11yStack tracks the open accessibility nodes during the frame
	// (nesting for WithAccessibility).
	a11yStack []int

	// debugOverlay paints frame statistics above everything when enabled
	// (SetDebugOverlay / GPUI_DEBUG=1); debugFrames is the rolling window.
	debugOverlay    bool
	debugFrames     []debugFrameStats
	debugFrameStart time.Time

	// appMenu is the window's application menu (upstream App::menubar;
	// rendered by elements.MenuBar on non-native platforms).
	appMenu []Menu
	// zoom multiplies the platform scale factor (upstream Window::zoom;
	// 1.0 = default).
	zoom float32

	// Overlay prepaint bookkeeping: the drag view or tooltip is laid out and
	// prepainted as a root during prepaint, then painted in a top layer.
	dragOverlayValid    bool
	dragOverlayNode     LayoutID
	dragOverlayOffset   geometry.Point[geometry.Pixels]
	tooltipOverlayValid bool
	tooltipOverlayNode  LayoutID

	// cursorApplier applies the resolved cursor to the OS pointer (upstream
	// cx.platform.set_cursor_style).
	cursorApplier func(platform.MouseCursorStyle)
	lastCursor    style.CursorStyle
	cursorApplied bool

	// mainDispatch posts closures onto the UI thread (upstream everything
	// runs on the main thread; hosts install platform.DispatchOnMainThread).
	mainDispatch func(f func())

	// Action pipeline (upstream dispatch_keystroke).
	keymap               *keymap.Keymap
	keyContextStack      []keymap.KeyContext // rendered contexts used at dispatch time
	prepaintKeyContexts  []keymap.KeyContext // live stack during prepaint
	nextFrameKeyContexts []keymap.KeyContext // recorded contexts for the next frame
	pendingKeystrokes    []keymap.Keystroke

	// Next-frame callbacks (upstream on_next_frame / request_animation_frame).
	nextFrameCallbacks []func(w *Window)

	// listenerDepth is the current element nesting while painting; used to
	// order mouse-listener dispatch (innermost first for bubble).
	listenerDepth int
	// nextMouseListenerSeq mints registration sequence numbers.
	nextMouseListenerSeq int

	// Focus.
	focus          *FocusHandle
	focusEnabled   bool
	focusPaths     map[FocusHandle][]FocusID
	focusPathStack []FocusID

	// Deferred draws (popovers).
	deferredDraws []deferredDraw

	// Scratch.
	currentView        app.EntityID
	currentElementNode LayoutID
	elementNode        map[Element]LayoutID
	elementBounds      map[Element]geometry.Bounds[geometry.Pixels]
	renderedFocus      *FocusHandle

	// Present callback (headless test rendering).
	afterDraw   func(*render.Scene)
	assetSource func(path string) ([]byte, bool)
	// redrawDepth bounds catch-up redraws (a frame that invalidates itself
	// during drawing must not recurse unbounded).
	redrawDepth int

	mu sync.Mutex
}

type deferredDraw struct {
	priority  int
	element   Element
	offset    geometry.Point[geometry.Pixels]
	mask      geometry.Bounds[geometry.Pixels]
	textStyle style.TextStyle
	remSize   geometry.Pixels
}

// invalidator implements app.ManagedWindow.
type invalidator struct {
	w          *Window
	dirtyViews map[app.EntityID]bool
}

func (inv *invalidator) WindowID() app.WindowID { return inv.w.id }
func (inv *invalidator) InvalidateView(id app.EntityID) bool {
	if inv.dirtyViews[id] {
		return true
	}
	if _, tracked := inv.w.app.TrackedEntities(inv.w.id)[id]; !tracked {
		return false
	}
	inv.dirtyViews[id] = true
	if inv.w.drawPhase == PhaseNone {
		inv.w.dirty = true
	}
	return true
}
func (inv *invalidator) IsDrawing() bool { return inv.w.drawPhase != PhaseNone }

// ScheduleFrame draws pending changes outside the frame walk. Called by the
// app's effect flush when a window is dirty (entity notifies, refreshes).
// Mid-draw calls defer to a catch-up redraw at the end of the active frame
// (upstream schedules the next vsync instead).
func (inv *invalidator) ScheduleFrame() {
	if inv.w.drawPhase != PhaseNone {
		inv.w.dirty = true
		return
	}
	inv.w.Draw()
}

func (inv *invalidator) MarkRefreshing() { inv.w.refreshing = true }
func (inv *invalidator) Refresh()        { inv.w.dirty = true }
func (inv *invalidator) DrawIfDirty() {
	if inv.w.dirty {
		inv.w.dirty = false
		inv.w.Draw()
	}
}
func (inv *invalidator) IsDirty() bool      { return inv.w.dirty }
func (inv *invalidator) NeedsPresent() bool { return false }

// NewWindow creates a window owned by the app.
func NewWindow(a *app.App, plat platform.PlatformWindow, textSystem *text.System) *Window {
	w := &Window{
		app:  a,
		plat: plat,
		text: textSystem,
		// The portable touch-gesture recognizer (upstream
		// window.touch_gestures), tuned per platform through
		// platform.SetGestureTuning (upstream
		// PlatformGestures::tuning).
		touchGestures: gestures.NewTouchGestureRecognizer(resolveGestureTuning()),
		// The sprite atlas is owned by the platform window so that painted
		// tiles and the rasterizer see the same storage (upstream: the
		// renderer owns the glyph atlas textures).
		atlas:       plat.SpriteAtlas(),
		scaleFactor: plat.ScaleFactor(),
		remSize:     geometry.Px(16),
		contentMask: geometry.Bounds[geometry.Pixels]{
			Origin: geometry.Point[geometry.Pixels]{},
			Size:   geometry.Size[geometry.Pixels]{Width: geometry.Px(1e9), Height: geometry.Px(1e9)},
		},
		elementOpacity:  1,
		zoom:            1,
		propagate:       true,
		focusEnabled:    true,
		elementNode:     map[Element]LayoutID{},
		elementBounds:   map[Element]geometry.Bounds[geometry.Pixels]{},
		layoutBoundsMap: map[LayoutID]geometry.Bounds[geometry.Pixels]{},
		layoutTree:      layout.NewTree(),
		nextFrame: frame{
			elementState: map[uint64]any{},
			scene:        &render.Scene{},
		},
		renderedFrame: frame{
			elementState: map[uint64]any{},
			scene:        &render.Scene{},
		},
		invalidator: &invalidator{dirtyViews: map[app.EntityID]bool{}},
		focusPaths:  map[FocusHandle][]FocusID{},
	}
	w.invalidator.w = w
	w.id = a.AddWindow(w.invalidator)
	if sf := plat.ScaleFactor(); sf > 0 {
		w.scaleFactor = sf
	}
	// Wire the OS cursor applier when the platform window exposes one
	// (upstream reset_cursor_style calls cx.platform.set_cursor_style;
	// this port sniffs the capability so hosts need no extra wiring).
	if cs, ok := plat.(interface {
		SetCursor(style platform.MouseCursorStyle)
	}); ok {
		w.SetCursorApplier(cs.SetCursor)
	}
	return w
}

// ID returns the window id.
func (w *Window) ID() app.WindowID { return w.id }

// App returns the owning app.
func (w *Window) App() *app.App { return w.app }

// PlatformWindow returns the platform window.
func (w *Window) PlatformWindow() platform.PlatformWindow { return w.plat }

// TextSystem returns the text system.
func (w *Window) TextSystem() *text.System { return w.text }

// Atlas returns the sprite atlas.
func (w *Window) Atlas() *render.Atlas { return w.atlas }

// atlasPruneInterval is how often (in frames) the window prunes stale
// atlas pages (see render.Atlas.PruneTextures).
const atlasPruneInterval = 256

// AtlasLen reports the number of cached atlas tiles (leak diagnostics).
func (w *Window) AtlasLen() int {
	if w.atlas == nil {
		return 0
	}
	return w.atlas.Len()
}

// SetRoot installs the root element (and optional view entity).
func (w *Window) SetRoot(root Element, view app.EntityID) {
	w.root = root
	w.rootView = view
}

// ScaleFactor returns the effective HiDPI scale: the platform factor
// multiplied by the window zoom (upstream Window::scale_factor includes
// zoom).
func (w *Window) ScaleFactor() float32 { return w.scaleFactor * w.zoom }

// SetScaleFactor updates the scale.
func (w *Window) SetScaleFactor(f float32) {
	if f > 0 {
		w.scaleFactor = f
	}
}

// LogicalContentSize returns the platform content size in the window's
// zoomed logical points — the coordinate space the layout runs in (upstream
// viewport_size derives from scale_factor, which includes the zoom).
func (w *Window) LogicalContentSize() geometry.Size[geometry.Pixels] {
	s := w.plat.ContentSize()
	z := w.zoom
	if z <= 0 || z == 1 {
		return s
	}
	return geometry.Size[geometry.Pixels]{
		Width:  geometry.Px(s.Width.AsF32() / z),
		Height: geometry.Px(s.Height.AsF32() / z),
	}
}

// ZoomedScrollDelta converts a scroll delta from platform logical pixels
// into the window's zoomed logical points (trackpad pixel deltas shrink
// with the zoom; line deltas were already resolved against a zoomed
// line height by the caller).
func (w *Window) ZoomedScrollDelta(d geometry.Point[geometry.Pixels]) geometry.Point[geometry.Pixels] {
	z := w.zoom
	if z <= 0 || z == 1 {
		return d
	}
	return geometry.Point[geometry.Pixels]{
		X: geometry.Px(d.X.AsF32() / z),
		Y: geometry.Px(d.Y.AsF32() / z),
	}
}

// toWindowPoint converts a platform pointer position into the window's
// zoomed logical points (upstream positions pass through scale_factor,
// which includes the zoom).
func (w *Window) toWindowPoint(p geometry.Point[geometry.Pixels]) geometry.Point[geometry.Pixels] {
	z := w.zoom
	if z <= 0 || z == 1 {
		return p
	}
	return geometry.Point[geometry.Pixels]{
		X: geometry.Px(p.X.AsF32() / z),
		Y: geometry.Px(p.Y.AsF32() / z),
	}
}

// RemSize returns the current rem size.
func (w *Window) RemSize() geometry.Pixels {
	if len(w.remOverride) > 0 {
		return w.remOverride[len(w.remOverride)-1]
	}
	return w.remSize
}

// SetRemSize sets the base rem size.
func (w *Window) SetRemSize(px geometry.Pixels) { w.remSize = px }

// WithRemSize pushes a rem override for f.
func (w *Window) WithRemSize(px geometry.Pixels, f func()) {
	w.remOverride = append(w.remOverride, px)
	defer func() { w.remOverride = w.remOverride[:len(w.remOverride)-1] }()
	f()
}

// TextStyle returns the folded current text style.
func (w *Window) TextStyle() style.TextStyle { return w.textStyle }

// WithTextStyle pushes a refinement over the current style.
func (w *Window) WithTextStyle(refinement *style.TextStyleRefinement, f func()) {
	if refinement == nil || refinement.IsEmpty() {
		f()
		return
	}
	prev := w.textStyle
	w.textStyle.Refine(refinement)
	defer func() { w.textStyle = prev }()
	f()
}

// PushTextStyleBase installs an absolute base style (window text style).
func (w *Window) PushTextStyleBase(ts style.TextStyle) {
	w.textStyle = ts
}

// ContentMask returns the current pixel-space clip.
func (w *Window) ContentMask() geometry.Bounds[geometry.Pixels] { return w.contentMask }

// WithContentMask intersects the clip for f.
func (w *Window) WithContentMask(mask geometry.Bounds[geometry.Pixels], f func()) {
	prev := w.contentMask
	if mask.Size.Width.AsF32() < 1e8 {
		w.contentMask = geometry.IntersectBounds(prev, mask)
	}
	defer func() { w.contentMask = prev }()
	f()
}

// ScaledContentMask converts the current clip to scene space.
func (w *Window) ScaledContentMask() render.ContentMask {
	return render.ContentMask{Bounds: geometry.ScaleBounds(w.contentMask, w.ScaleFactor())}
}

// WithElementOffset translates children by delta for f (upstream
// with_element_offset: pushes current + delta as the new absolute offset).
func (w *Window) WithElementOffset(delta geometry.Point[geometry.Pixels], f func()) {
	prev := w.elementOffset
	w.elementOffset = geometry.Point[geometry.Pixels]{
		X: prev.X.Add(delta.X),
		Y: prev.Y.Add(delta.Y),
	}
	defer func() { w.elementOffset = prev }()
	f()
}

// WithAbsoluteElementOffset installs an absolute origin for f (upstream
// with_absolute_element_offset, used by list items, tooltips and dragged
// elements prepainted as independent roots).
func (w *Window) WithAbsoluteElementOffset(offset geometry.Point[geometry.Pixels], f func()) {
	prev := w.elementOffset
	w.elementOffset = offset
	defer func() { w.elementOffset = prev }()
	f()
}

// ElementOffset returns the current offset (scroll).
func (w *Window) ElementOffset() geometry.Point[geometry.Pixels] { return w.elementOffset }

// WithElementOpacity multiplies opacity for f.
func (w *Window) WithElementOpacity(opacity float32, f func()) {
	prev := w.elementOpacity
	if opacity < 1 {
		w.elementOpacity *= opacity
	}
	defer func() { w.elementOpacity = prev }()
	f()
}

// ElementOpacity returns the accumulated opacity.
func (w *Window) ElementOpacity() float32 { return w.elementOpacity }

// WithElementID pushes an element id onto the path for f.
func (w *Window) WithElementID(id ElementId, f func()) {
	if id.IsZero() {
		f()
		return
	}
	w.idPath.push(id)
	defer w.idPath.pop()
	f()
}

// CurrentElementID returns the current global id.
func (w *Window) CurrentElementID() GlobalElementId {
	return GlobalElementId{ids: append([]ElementId(nil), w.idPath.ids...), hash: w.idPath.currentHash()}
}

// ---------------------------------------------------------------------------
// Element state (per-frame handoff)
// ---------------------------------------------------------------------------

// stateKeyHash mixes the element path hash with the state's type so that
// unnamed children holding their own state (e.g. a text element's line
// layout) do not clobber a parent's state stored under the same path hash.
// Upstream keys state by element identity, which carries the type with it.
func stateKeyHash[S any](path uint64) uint64 {
	h := fnv.New64a()
	h.Write([]byte(reflect.TypeOf((*S)(nil)).Elem().String()))
	return path ^ h.Sum64()
}

// WithElementState fetches or creates persistent state for the current
// element path (upstream with_element_state).
func WithElementState[S any](w *Window, f func(existing S, ok bool) S) {
	key := stateKeyHash[S](w.CurrentElementID().Hash())
	var existing S
	st, ok := w.nextFrame.elementState[key]
	if !ok {
		if st, ok = w.renderedFrame.elementState[key]; !ok {
			f(existing, false)
			return
		}
	}
	existing, ok = st.(S)
	if !ok {
		f(existing, false)
		return
	}
	_ = ok
	f(existing, true)
}

// UseElementState is the ergonomic wrapper.
func UseElementState[S any](w *Window, build func() S, use func(s *S)) {
	id := stateKeyHash[S](w.CurrentElementID().Hash())
	var state S
	if st, ok := w.nextFrame.elementState[id]; ok {
		if s, isType := st.(S); isType {
			state = s
		}
	} else if st, ok := w.renderedFrame.elementState[id]; ok {
		if s, isType := st.(S); isType {
			state = s
		}
	} else {
		state = build()
	}
	use(&state)
	w.nextFrame.elementState[id] = state
}

// UpdateElementStateAt loads, mutates and stores the element state recorded
// under a captured element path. Input listeners run between frames, outside
// the element walk, so the current path is empty; elements capture their
// GlobalElementId at paint time and pass it here (upstream listeners instead
// hold an Rc into the state, which Go's value semantics cannot express).
func UpdateElementStateAt[S any](w *Window, gid GlobalElementId, build func() S, use func(s *S)) {
	key := stateKeyHash[S](gid.Hash())
	var state S
	if st, ok := w.nextFrame.elementState[key]; ok {
		if s, isType := st.(S); isType {
			state = s
		}
	} else if st, ok := w.renderedFrame.elementState[key]; ok {
		if s, isType := st.(S); isType {
			state = s
		}
	} else {
		state = build()
	}
	use(&state)
	w.nextFrame.elementState[key] = state
}

// ---------------------------------------------------------------------------
// Layout glue
// ---------------------------------------------------------------------------

// RequestLayout registers the element's style with the layout tree,
// recursively lays out children and attaches them, and returns the node id.
// Elements call this from their RequestLayout method.
func (w *Window) RequestLayout(st style.Style, children []Element) LayoutID {
	node := LayoutID(w.layoutTree.NewNode(LayoutStyleOf(st, w.RemSize(), w.ScaleFactor())))
	var childNodes []layout.Node
	for _, child := range children {
		childNode := w.requestLayoutElement(child)
		childNodes = append(childNodes, layout.Node(childNode))
	}
	w.layoutTree.SetChildren(layout.Node(node), childNodes)
	w.currentElementNode = node
	return node
}

// RequestMeasuredLayout registers a measured leaf.
func (w *Window) RequestMeasuredLayout(st style.Style, measure func(known layout.SizeDimension, available layout.AvailableSpace) layout.SizeResult) LayoutID {
	node := LayoutID(w.layoutTree.NewLeaf(LayoutStyleOf(st, w.RemSize(), w.ScaleFactor()), measure))
	w.currentElementNode = node
	return node
}

// ComputeLayout runs the layout for a root node.
func (w *Window) ComputeLayout(node LayoutID, available layout.AvailableSpace) {
	w.layoutTree.ComputeLayout(layout.Node(node), available)
}

// ElementBounds returns an element's prepaint-computed bounds.
func (w *Window) ElementBounds(e Element) geometry.Bounds[geometry.Pixels] {
	if b, ok := w.elementBounds[e]; ok {
		return b
	}
	return geometry.Bounds[geometry.Pixels]{}
}

// ElementScrollSize returns an element's content extent from the layout tree
// (upstream taffy scrollable_overflow_rect), used to clamp scroll offsets.
func (w *Window) ElementScrollSize(e Element) geometry.Size[geometry.Pixels] {
	node, ok := w.NodeOf(e)
	if !ok {
		return geometry.Size[geometry.Pixels]{}
	}
	l := w.layoutTree.Layout(layout.Node(node))
	return geometry.Size[geometry.Pixels]{
		Width:  geometry.Px(l.ScrollWidth),
		Height: geometry.Px(l.ScrollHeight),
	}
}

// ClickEvent pairs a completed press/release (upstream ClickEvent::Mouse).
type ClickEvent struct {
	Down platform.MouseDownEvent
	Up   platform.MouseUpEvent
}

// DragEvent reports drag motion.
type DragEvent struct {
	Position geometry.Point[geometry.Pixels]
	Delta    geometry.Point[geometry.Pixels]
}

// LayoutBounds returns the computed bounds (upstream layout_bounds: the
// taffy parent-chain accumulation plus the current element offset, which
// carries scroll offsets and manually-positioned roots).
func (w *Window) LayoutBounds(node LayoutID) geometry.Bounds[geometry.Pixels] {
	l := w.layoutTree.AbsoluteBounds(layout.Node(node))
	b := geometry.Bounds[geometry.Pixels]{
		Origin: geometry.Point[geometry.Pixels]{X: geometry.Px(l.X), Y: geometry.Px(l.Y)},
		Size:   geometry.Size[geometry.Pixels]{Width: geometry.Px(l.Width), Height: geometry.Px(l.Height)},
	}
	b.Origin.X += w.elementOffset.X
	b.Origin.Y += w.elementOffset.Y
	return b
}

// ---------------------------------------------------------------------------
// Draw
// ---------------------------------------------------------------------------

// Draw runs one full frame: layout, prepaint, paint, present.
func (w *Window) Draw() {
	if w.root == nil {
		return
	}
	// Next-frame callbacks run before the frame is built (upstream
	// on_next_frame semantics).
	w.runNextFrameCallbacks()
	w.beginDebugFrame()
	w.drawPhase = PhasePrepaint
	w.beginAccessScope()

	// Reset per-frame state. elementState is carried over: present() clears
	// it and input listeners write to it between frames (upstream merges
	// those writes into the next frame's state), so it must not be reset
	// here.
	carriedElementState := w.nextFrame.elementState
	w.nextFrame = frame{
		elementState: carriedElementState,
		scene:        &render.Scene{},
		focus:        nil,
	}
	// Hitbox ids restart at 1 every frame. They must be unique WITHIN a
	// frame, and a pointer capture (CapturePointer, held by a scrollbar
	// drag across redraws) resolves against the NEXT frame's hitboxes:
	// with a monotonic counter every redraw re-numbered every hitbox and
	// the captured id silently matched nothing, so a drag that left the
	// scrollbar immediately leaked moves to the ancestors (the editor pane
	// selected text while the user held the scrollbar). Deterministic
	// per-frame numbering makes the same structural hitbox keep its id
	// while the element tree is unchanged, which is exactly the identity a
	// capture needs. The IsHovered comparison ignores ids that no longer
	// exist, so a restructured tree just drops the capture instead of
	// pointing at a random element.
	nextHitboxID = 1
	w.deferredDraws = nil
	w.frameTooltip = nil
	w.dragOverlayValid = false
	w.tooltipOverlayValid = false
	w.layoutTree.Reset()
	// elementNode and elementBounds are rebuilt each frame; entries from
	// the previous frame stay readable for input handlers until this
	// point (upstream drops the element path at the end of a frame).
	// Resetting here also bounds the maps: without it every frame's fresh
	// element pointers accumulate forever (each frame builds a new
	// element tree), leaking memory and degrading performance over time.
	clear(w.elementNode)
	clear(w.elementBounds)
	clear(w.layoutBoundsMap)
	w.pendingChildren = nil
	w.prepaintKeyContexts = nil
	w.nextFrameKeyContexts = nil

	// 1. Request layout (builds the layout tree).
	rootID := w.requestLayoutElement(w.root)
	// Auto-sized roots fill the viewport (upstream stretch_auto_size_to_fill).
	viewport := w.LogicalContentSize()
	if viewport.Width.AsF32() == 0 {
		viewport = geometry.SizePixels(800, 600)
	}
	w.layoutTree.StretchAutoSizeToFill(layout.Node(rootID),
		viewport.Width.AsF32(), viewport.Height.AsF32())
	avail := layout.DefiniteAvailable(viewport.Width.AsF32(), viewport.Height.AsF32())
	w.layoutTree.ComputeLayout(layout.Node(rootID), avail)

	// 2. Prepaint.
	w.PrepaintElement(w.root, rootID)
	// Hover state after prepaint (upstream computes mouse_hit_test here).
	w.mouseHitTest = w.hitTestOf(w.nextFrame.hitboxes, w.mousePosition)
	// Deferred draws prepaint after the root, sorted by priority (higher
	// priority paints on top; upstream deferred draw ordering).
	w.sortDeferredDraws()
	w.prepaintDeferred()

	// The active drag view (or, without one, the frame tooltip) prepaints
	// as a root above the tree (upstream draw(): prompt > active drag >
	// tooltip).
	w.prepaintOverlay()
	// Prompts prepaint above the drag/tooltip overlays so their hitboxes
	// register last (input priority) and they paint on top.
	w.prepaintPrompts()

	// Key contexts pushed during prepaint become the dispatch context
	// stack (upstream renders contexts then dispatches against them).
	w.keyContextStack = w.nextFrameKeyContexts

	// 3. Paint.
	w.drawPhase = PhasePaint
	w.paintElement(w.root)
	w.paintDeferred()
	// Hover tooltips are requested by Div during paint, after hover hit
	// testing. Give a late-requested tooltip its layout before painting it.
	if w.activeDrag == nil && w.frameTooltip != nil && !w.tooltipOverlayValid {
		w.prepaintOverlay()
	}
	// The drag view / tooltip paints in a top layer above everything.
	w.paintOverlay()
	// Prompts paint last: the modal dialog covers everything (upstream
	// prompt > drag > tooltip paint order).
	w.paintPrompts()
	// The debug HUD paints above everything.
	w.paintDebugOverlay()

	// 4. Finish the frame.
	w.sortMouseListeners(w.nextFrame.mouseListeners)
	w.nextFrame.scene.Finish()
	w.drawCount++
	// Periodic atlas pruning: stale pages (glyphs of closed buffers,
	// old font sizes) drop even when no new insertions trigger the
	// cap-based prune. The scan is cheap relative to a frame.
	if w.drawCount%atlasPruneInterval == 0 && w.atlas != nil {
		w.atlas.PruneTextures(render.AtlasMonochrome)
		w.atlas.PruneTextures(render.AtlasPolychrome)
		w.atlas.PruneTextures(render.AtlasSubpixel)
	}

	// Element-state handoff: keep accessed states.
	rendered := w.renderedFrame
	w.renderedFrame = w.nextFrame
	w.nextFrame = rendered
	w.nextFrame.elementState = map[uint64]any{}

	// A capture that no longer resolves to an element of the new frame is
	// stale (the element disappeared): drop it now rather than silencing
	// the next press.
	w.ValidateCapture()

	// Focus events.
	w.processFocusEvents()

	// Apply the resolved hover cursor through the platform (upstream
	// reset_cursor_style runs once the frame is built).
	w.applyCursorStyle()

	// Record accessed entities for invalidation routing.
	w.recordAccessedEntities()
	w.invalidator.dirtyViews = map[app.EntityID]bool{}
	w.drawPhase = PhaseNone
	w.refreshing = false
	w.dirty = false

	// 5. Present.
	scene := w.renderedFrame.scene
	if w.afterDraw != nil {
		w.afterDraw(scene)
	}
	w.plat.Draw(scene)

	// An update nested inside this frame marked the window dirty (effects
	// flushed mid-draw): draw the catch-up frame now so it is not lost.
	w.endDebugFrame()

	if w.dirty && w.drawPhase == PhaseNone && w.redrawDepth < 2 {
		w.redrawDepth++
		w.Draw()
		w.redrawDepth--
	}
}

func (w *Window) beginAccessScope() { w.app.BeginAccessScope() }

func (w *Window) recordAccessedEntities() {
	accessed := w.app.TakeAccessed()
	w.app.RecordEntitiesAccessed(w.id, accessed)
}

// noNode is the sentinel for "no layout node created yet" (node 0 is a
// valid id).
const noNode LayoutID = 0xFFFFFFFF

func (w *Window) requestLayoutElement(e Element) LayoutID {
	// Push the element's own id so state keys match across phases
	// (upstream Drawable::request_layout prepares the global element id).
	w.WithElementID(e.ElementID(), func() {
		w.currentElementNode = noNode
		e.RequestLayout(w)
		if w.currentElementNode == noNode {
			// The element created no node (e.g. an empty view): give it an
			// empty container node so layout and the walk stay consistent.
			w.currentElementNode = LayoutID(w.layoutTree.NewNode(layout.DefaultStyle()))
		}
	})
	node := w.currentElementNode
	w.elementNode[e] = node
	return node
}

// PrepaintElement walks one element with its computed bounds.
//
// Elements without an explicit id get their layout node id pushed as an
// auto id. State keyed by the path hash is per element: every element owns
// a distinct layout node, so this keeps interactive state (scroll offsets,
// hover transitions, text line caches) from colliding between all the
// anonymous divs of a window. Upstream instead refuses state for id-less
// elements; the port's anonymous scroll containers rely on it existing, so
// the node id gives them the identity upstream gets from explicit ids.
func (w *Window) PrepaintElement(e Element, node LayoutID) {
	if e.ElementID().IsZero() {
		w.WithElementID(IntElementId(uint64(node)), func() {
			w.prepaintElementInner(e, node)
		})
		return
	}
	w.WithElementID(e.ElementID(), func() {
		w.prepaintElementInner(e, node)
	})
}

func (w *Window) prepaintElementInner(e Element, node LayoutID) {
	bounds := w.LayoutBounds(node)
	w.elementBounds[e] = bounds
	e.Prepaint(w, bounds)
}

// NodeOf returns the layout node assigned to an element this frame.
func (w *Window) NodeOf(e Element) (LayoutID, bool) {
	id, ok := w.elementNode[e]
	return id, ok
}

func (w *Window) paintElement(e Element) {
	// Mirror PrepaintElement: zero-id elements push their layout node as
	// the auto id so paint-phase state (and the gid captured by listeners)
	// matches the prepaint-phase key of the same element.
	id := e.ElementID()
	if id.IsZero() {
		if node, ok := w.elementNode[e]; ok {
			id = IntElementId(uint64(node))
		}
	}
	w.WithElementID(id, func() {
		w.listenerDepth++
		e.Paint(w)
		w.listenerDepth--
	})
}

// PaintElement paints a child element with its id pushed on the path (the
// wrapper-element API; upstream Drawable::paint).
func (w *Window) PaintElement(e Element) { w.paintElement(e) }

// RequestLayoutElement requests layout for a child element, pushing its id
// and registering its node (upstream Drawable::request_layout). The created
// node becomes the caller's node when the caller creates none of its own.
func (w *Window) RequestLayoutElement(e Element) LayoutID { return w.requestLayoutElement(e) }

func (w *Window) prepaintDeferred() {
	// Deferred draws prepaint at their recorded offsets.
	for i := range w.deferredDraws {
		d := &w.deferredDraws[i]
		prevOffset := w.elementOffset
		prevMask := w.contentMask
		prevTextStyle := w.textStyle
		prevRem := w.remSize
		w.elementOffset = d.offset
		w.contentMask = d.mask
		w.textStyle = d.textStyle
		w.remSize = d.remSize
		if node, ok := w.elementNode[d.element]; ok {
			w.PrepaintElement(d.element, node)
		}
		w.elementOffset = prevOffset
		w.contentMask = prevMask
		w.textStyle = prevTextStyle
		w.remSize = prevRem
	}
}

func (w *Window) paintDeferred() {
	for i := range w.deferredDraws {
		d := &w.deferredDraws[i]
		prevOffset := w.elementOffset
		prevMask := w.contentMask
		prevTextStyle := w.textStyle
		prevRem := w.remSize
		w.elementOffset = d.offset
		w.contentMask = d.mask
		w.textStyle = d.textStyle
		w.remSize = d.remSize
		d.element.Paint(w)
		w.elementOffset = prevOffset
		w.contentMask = prevMask
		w.textStyle = prevTextStyle
		w.remSize = prevRem
	}
}

// DeferDraw schedules an element to prepaint/paint after the main tree
// (popovers, tooltips).
func (w *Window) DeferDraw(e Element, offset geometry.Point[geometry.Pixels], priority int) {
	w.deferredDraws = append(w.deferredDraws, deferredDraw{
		priority:  priority,
		element:   e,
		offset:    offset,
		mask:      w.contentMask,
		textStyle: w.textStyle,
		remSize:   w.remSize,
	})
	// Ensure the deferred element has layout.
	if _, ok := w.elementNode[e]; !ok {
		node := w.requestLayoutElement(e)
		w.layoutTree.ComputeLayout(layout.Node(node), layout.MaxContentAvailable())
	}
}

func (w *Window) processFocusEvents() {
	// Focus change hooks live on the app side in this port; the rendered
	// frame's focus handle is updated by SetFocusHandle during prepaint.
	w.renderedFocus = w.nextFrame.focus
}

// ---------------------------------------------------------------------------
// Scene helpers
// ---------------------------------------------------------------------------

// Scene returns the frame's scene (for tests).
func (w *Window) Scene() *render.Scene { return w.renderedFrame.scene }

// DrawCount returns the number of frames drawn.
func (w *Window) DrawCount() int { return w.drawCount }

// SetAfterDraw installs a callback invoked with each frame's scene (used by
// the headless platform to capture images).
func (w *Window) SetAfterDraw(f func(*render.Scene)) { w.afterDraw = f }

// RenderToImage renders the current scene to an RGBA image.
func (w *Window) RenderToImage() *render.RenderImage {
	scene := w.renderedFrame.scene
	size := w.plat.ContentSize()
	dev := geometry.SizeToDevicePixels(size, w.ScaleFactor())
	r := render.NewRasterizer(int(dev.Width.AsI32()), int(dev.Height.AsI32()), w.atlas)
	r.Clear()
	r.PaintScene(scene)
	return r.Image()
}

// ImageToRGBA converts a RenderImage to stdlib image.RGBA.
func ImageToRGBA(img *render.RenderImage) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, img.Width, img.Height))
	for y := 0; y < img.Height; y++ {
		for x := 0; x < img.Width; x++ {
			i := (y*img.Width + x) * 4
			out.SetRGBA(x, y, color.RGBA{img.Pixels[i], img.Pixels[i+1], img.Pixels[i+2], img.Pixels[i+3]})
		}
	}
	return out
}

// CurrentView returns the entity being rendered.
func (w *Window) CurrentView() app.EntityID { return w.currentView }

// WithRenderedView sets the current view for f.
func (w *Window) WithRenderedView(view app.EntityID, f func()) {
	prev := w.currentView
	w.currentView = view
	defer func() { w.currentView = prev }()
	f()
}

// Colors default palette access.
func (w *Window) DefaultColors() gc.Colors {
	return gc.ColorsForAppearance(w.plat.Appearance())
}

var (
	_ app.ManagedWindow = (*invalidator)(nil)
)

// currentElementNode is assigned by elements' RequestLayout calls.
// (Simplification: instead of returning ids from Element.RequestLayout, the
// window records the last created node.)
var _ = keymap.Modifiers{}

// renderedFocus tracks the previous frame's focus for change detection.

// NowMilliseconds returns a monotonic clock in ms for animations.
func (w *Window) NowMilliseconds() int64 {
	return w.app.Background().Now().UnixMilli()
}

// AssetSource loads asset bytes by path (installed by the platform/app).
func (w *Window) AssetSource() func(path string) ([]byte, bool) { return w.assetSource }

// SetAssetSource installs the asset loader.
func (w *Window) SetAssetSource(f func(path string) ([]byte, bool)) { w.assetSource = f }

// LastNode returns the most recently created layout node (elements'
// RequestLayout implementations use it indirectly via the walk).
func (w *Window) LastNode() LayoutID { return w.currentElementNode }

// MeasureElement lays out a single element at max-content and returns its
// size (used by the list elements to measure items).
func (w *Window) MeasureElement(e Element) geometry.Size[geometry.Pixels] {
	node := w.requestLayoutElement(e)
	w.layoutTree.ComputeLayout(layout.Node(node), layout.MaxContentAvailable())
	l := w.layoutTree.Layout(layout.Node(node))
	return geometry.Size[geometry.Pixels]{Width: geometry.Px(l.Width), Height: geometry.Px(l.Height)}
}

// LayoutElement lays out an element at a definite size.
func (w *Window) LayoutElement(e Element, available layout.AvailableSpace) {
	node := w.requestLayoutElement(e)
	w.layoutTree.ComputeLayout(layout.Node(node), available)
}

// PrepaintElementAt prepaints an element at an explicit absolute origin
// (upstream prepaint_at, used by list items and other manually-positioned
// roots). The element's bounds derive from its own layout plus the origin.
func (w *Window) PrepaintElementAt(e Element, node LayoutID, origin geometry.Point[geometry.Pixels]) {
	w.WithAbsoluteElementOffset(origin, func() {
		w.PrepaintElement(e, node)
	})
}

// SetNodeFor maps an element to a layout node for this frame.
func (w *Window) SetNodeFor(e Element, node LayoutID) {
	w.elementNode[e] = node
}

// HandleInput routes a platform input event through the window: it updates
// tracked mouse/modifier state, then dispatches mouse or key events.
func (w *Window) HandleInput(ev platform.Input) platform.DispatchEventResult {
	var result platform.DispatchEventResult
	result.Propagate = true
	redraw := false
	// Mouse input re-enables hover (upstream last_input_was_keyboard is
	// updated by every input event; a stale true value after keyboard use
	// would make IsHovered reject every hitbox and swallow all clicks).
	switch ev.(type) {
	case platform.MouseDownEvent, platform.MouseUpEvent, platform.MouseMoveEvent,
		platform.MouseExitEvent, platform.ScrollEvent, platform.PinchEvent,
		platform.TouchEvent, platform.TouchDragEvent, platform.LongPressEvent,
		platform.BackGestureEvent, platform.FileDropEvent:
		w.lastInputKeyboard = false
	}
	drawsBefore := w.drawCount
	w.app.Update(func(cx *app.App) {
		switch e := ev.(type) {
		case platform.MouseDownEvent:
			w.mousePosition = e.Position
			w.modifiers = e.Modifiers
			if w.pressedButtons == nil {
				w.pressedButtons = map[platform.MouseButton]bool{}
			}
			w.pressedButtons[e.Button] = true
			result = w.DispatchMouseEvent(e)
			redraw = w.listenersFired > 0
		case platform.MouseUpEvent:
			w.mousePosition = e.Position
			w.modifiers = e.Modifiers
			delete(w.pressedButtons, e.Button)
			result = w.DispatchMouseEvent(e)
			// Pointer capture ends with the button release (DOM
			// setPointerCapture semantics). A capture whose element vanished
			// mid-gesture (structural re-render) otherwise silences every
			// later press until some handler releases it — the "lost the
			// ability to click on anything" state.
			w.ReleasePointer()
			redraw = w.listenersFired > 0
		case platform.MouseMoveEvent:
			// Hover styling is resolved at paint time, so a move that
			// changes the hovered hitboxes (or fires move listeners) needs
			// another frame (upstream requests a frame on hover change).
			before := w.hoverKeys()
			w.mousePosition = e.Position
			w.modifiers = e.Modifiers
			result = w.DispatchMouseEvent(w.withPressedButtons(e))
			redraw = w.listenersFired > 0 || !equalHoverKeys(before, w.hoverKeys())
		case platform.MouseExitEvent:
			before := w.hoverKeys()
			result = w.DispatchMouseEvent(w.withPressedButtonsExit(e))
			redraw = !equalHoverKeys(before, w.hoverKeys())
		case platform.ScrollEvent:
			w.mousePosition = e.Position
			w.modifiers = e.Modifiers
			result = w.DispatchMouseEvent(e)
			redraw = w.listenersFired > 0
		case platform.TouchEvent:
			result = w.handleTouchEvent(e)
			redraw = w.listenersFired > 0
		case platform.PinchEvent:
			w.mousePosition = e.Position
			w.modifiers = e.Modifiers
			w.lastInputKeyboard = false
			result = w.DispatchMouseEvent(e)
			redraw = w.listenersFired > 0
		case platform.TouchDragEvent:
			w.lastInputKeyboard = false
			result = w.DispatchMouseEvent(e)
			redraw = w.listenersFired > 0
		case platform.LongPressEvent:
			w.lastInputKeyboard = false
			result = w.DispatchMouseEvent(e)
			redraw = w.listenersFired > 0
		case platform.BackGestureEvent:
			w.lastInputKeyboard = false
			w.mousePosition = e.StartPosition
			result = w.DispatchMouseEvent(e)
			redraw = w.listenersFired > 0
		case platform.FileDropEvent:
			w.mousePosition = e.Position
			w.lastInputKeyboard = false
			result = w.DispatchMouseEvent(e)
			redraw = w.listenersFired > 0
		case platform.KeyDownEvent:
			result = w.DispatchKeyEvent(e)
			redraw = w.listenersFired > 0
		case platform.KeyUpEvent:
			result = w.DispatchKeyEvent(e)
			redraw = w.listenersFired > 0
		case platform.ModifiersChangedEvent:
			w.modifiers = e.Modifiers
		}
	})
	// One input event costs at most one frame, like upstream (which
	// schedules a single vsync frame per dispatch). Handlers must not
	// call Draw() themselves: drawing mid-dispatch is re-entrant (the
	// frame swap would swap hitboxes and listeners under the iterating
	// dispatcher) and can burn several full frames per event. Handlers
	// mark the window dirty instead (Window.Refresh) or notify their
	// view; the effect flush below draws once, and the tail draw only
	// runs when nothing else drew (e.g. hover-only changes).
	if redraw && w.drawPhase == PhaseNone && w.drawCount == drawsBefore {
		w.Draw()
	}
	return result
}

// withPressedButtons stamps the tracked held button into a move event so
// listeners can rely on Dragging() (upstream reports the pressed button
// from the platform's move events; win32/X11 do not carry it).
func (w *Window) withPressedButtons(e platform.MouseMoveEvent) platform.MouseMoveEvent {
	for _, b := range []platform.MouseButton{
		platform.ButtonLeft, platform.ButtonRight, platform.ButtonMiddle, platform.ButtonNavigate,
	} {
		if w.pressedButtons[b] {
			btn := b
			e.PressedButton = &btn
			break
		}
	}
	return e
}

// withPressedButtonsExit is withPressedButtons for exit events.
func (w *Window) withPressedButtonsExit(e platform.MouseExitEvent) platform.MouseExitEvent {
	for _, b := range []platform.MouseButton{
		platform.ButtonLeft, platform.ButtonRight, platform.ButtonMiddle, platform.ButtonNavigate,
	} {
		if w.pressedButtons[b] {
			btn := b
			e.PressedButton = &btn
			break
		}
	}
	return e
}

// hoverKey identifies a hovered hitbox by geometry (hitbox ids are minted
// per frame, bounds are stable between frames).
type hoverKey struct{ x, y, w, h float32 }

// hoverKeys snapshots the hovered hitboxes of the rendered frame.
func (w *Window) hoverKeys() []hoverKey {
	var keys []hoverKey
	for i, id := range w.mouseHitTest.ids {
		if i >= w.mouseHitTest.hoverHitboxCount {
			break
		}
		for _, hb := range w.renderedFrame.hitboxes {
			if hb.ID == id {
				b := hb.Bounds
				keys = append(keys, hoverKey{
					float32(b.Origin.X), float32(b.Origin.Y),
					float32(b.Size.Width), float32(b.Size.Height),
				})
				break
			}
		}
	}
	return keys
}

func equalHoverKeys(a, b []hoverKey) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// sortMouseListeners orders the frame's mouse listeners for dispatch:
// bubble runs innermost (deepest) elements first, and among equal depth the
// latest-registered sibling first (it paints on top). Children register
// before their parents' listeners because a Div paints children before its
// own paint_listeners, so plain reverse-registration would dispatch parents
// before children - the inverse of event bubbling.
func (w *Window) sortMouseListeners(listeners []mouseListener) {
	if len(listeners) < 2 {
		return
	}
	// Single sort: bubble order (depth desc, seq desc). Capture iterates
	// the same slice in reverse (outermost, earliest first).
	for i := 1; i < len(listeners); i++ {
		l := listeners[i]
		j := i - 1
		for j >= 0 && (listeners[j].depth < l.depth ||
			(listeners[j].depth == l.depth && listeners[j].seq < l.seq)) {
			listeners[j+1] = listeners[j]
			j--
		}
		listeners[j+1] = l
	}
}

// ---------------------------------------------------------------------------
// Drag and drop (upstream AnyDrag + cx.active_drag)
// ---------------------------------------------------------------------------

// DragState describes the drag-and-drop gesture started from a div (upstream
// AnyDrag: the dragged view, the typed payload and where the pointer grabbed
// the source element).
type DragState struct {
	// Content is a short label painted under the cursor when View is nil.
	Content string
	// Payload is the typed drag payload. Drop targets discriminate payloads
	// by Type() (upstream AnyDrag::type_name / TypeId matching).
	Payload any
	// Origin is the window position where the drag gesture started (the
	// pending mouse-down position).
	Origin geometry.Point[geometry.Pixels]
	// View is the element painted under the cursor while dragging; nil
	// paints a small labeled chip with Content (built by the drag source).
	View Element
	// CursorOffset is the pointer offset inside the drag view at grab time
	// (upstream AnyDrag::cursor_offset); the view paints at
	// mouse_position - CursorOffset.
	CursorOffset geometry.Point[geometry.Pixels]
}

// Type returns the payload discriminator (upstream AnyDrag::type_name): the
// Go type name of Payload, or "" when the drag carries no payload.
func (d *DragState) Type() string {
	if d == nil || d.Payload == nil {
		return ""
	}
	return reflect.TypeOf(d.Payload).String()
}

// StartDrag begins a window drag-and-drop gesture (upstream assigning
// cx.active_drag) and schedules a frame so the drag view appears.
func (w *Window) StartDrag(d *DragState) {
	if d == nil {
		return
	}
	w.activeDrag = d
	w.Refresh()
}

// ActiveDrag returns the drag gesture in flight (upstream cx.active_drag).
func (w *Window) ActiveDrag() *DragState { return w.activeDrag }

// HasActiveDrag reports whether a drag is in flight (upstream
// cx.has_active_drag; hover callbacks and styles are suppressed during one).
func (w *Window) HasActiveDrag() bool { return w.activeDrag != nil }

// TakeActiveDrag ends the gesture and returns its state (upstream
// cx.active_drag.take(); drop targets consume the drag here).
func (w *Window) TakeActiveDrag() *DragState {
	d := w.activeDrag
	if d != nil {
		w.activeDrag = nil
		w.Refresh()
	}
	return d
}

// StopActiveDrag cancels the gesture without delivering it to a drop target.
func (w *Window) StopActiveDrag() { w.TakeActiveDrag() }

// ---------------------------------------------------------------------------
// Tooltip overlay (upstream Window::set_tooltip + prepaint_tooltip)
// ---------------------------------------------------------------------------

// SetTooltip schedules the tooltip overlay for this frame (upstream
// Window::set_tooltip). pos anchors the tooltip near the cursor: the window
// lays the element out at max-content, flips it at the window edges and
// paints it in a top layer above deferred draws. Called from element paint.
func (w *Window) SetTooltip(e Element, pos geometry.Point[geometry.Pixels]) {
	w.frameTooltip = e
	w.frameTooltipPos = pos
}

// TooltipVisible reports whether a tooltip overlay was scheduled for the
// current frame (test/diagnostic hook).
func (w *Window) TooltipVisible() bool { return w.frameTooltip != nil }

// tooltipOriginFor anchors a tooltip near the cursor: 1px below-right of the
// pointer, flipped above/left when it would overflow the window, clamped to
// the origin (upstream Window::prepaint_tooltip).
func tooltipOriginFor(anchor geometry.Point[geometry.Pixels], size, window geometry.Size[geometry.Pixels]) geometry.Point[geometry.Pixels] {
	x := anchor.X.AsF32() + 1
	y := anchor.Y.AsF32() + 1
	w, h := size.Width.AsF32(), size.Height.AsF32()
	if x+w > window.Width.AsF32() {
		x = anchor.X.AsF32() - w - 1
	}
	if y+h > window.Height.AsF32() {
		y = anchor.Y.AsF32() - h - 1
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return geometry.PointPixels(x, y)
}

// ---------------------------------------------------------------------------
// Cursor style application (upstream Window::set_cursor_style +
// reset_cursor_style)
// ---------------------------------------------------------------------------

// cursorStyleRequest pairs a cursor style with the hitbox that requested it
// (upstream CursorStyleRequest; upstream stores the hitbox id, the port
// stores the hitbox so IsHovered resolves occlusion and keyboard
// suppression).
type cursorStyleRequest struct {
	hitbox Hitbox
	style  style.CursorStyle
}

// SetCursorStyle registers the cursor shown while the pointer hovers hitbox
// (upstream Window::set_cursor_style). Call during element paint; the
// topmost hovered request wins when the frame finishes.
func (w *Window) SetCursorStyle(s style.CursorStyle, hitbox Hitbox) {
	w.nextFrame.cursorStyles = append(w.nextFrame.cursorStyles, cursorStyleRequest{hitbox: hitbox, style: s})
}

// SetCursorApplier installs the callback that applies the resolved cursor to
// the OS pointer (upstream reset_cursor_style calls
// cx.platform.set_cursor_style; this port threads the app-level platform
// through a hook because Window holds only its PlatformWindow). Hosts wire
// it at startup: w.SetCursorApplier(func(c platform.MouseCursorStyle) {
// platform.SetCursor(c) }).
func (w *Window) SetCursorApplier(f func(cursor platform.MouseCursorStyle)) {
	w.cursorApplier = f
}

// LastCursor returns the cursor resolved for the last presented frame
// (upstream resolves it inside reset_cursor_style; exposed for tests).
func (w *Window) LastCursor() style.CursorStyle { return w.lastCursor }

// applyCursorStyle resolves the frame's cursor requests against the mouse
// hit test: the topmost hovered request wins and Arrow is the fallback
// (upstream Frame::cursor_style + Window::reset_cursor_style). Identical
// consecutive resolutions skip the platform call.
func (w *Window) applyCursorStyle() {
	resolved := style.CursorArrow
	for i := len(w.renderedFrame.cursorStyles) - 1; i >= 0; i-- {
		req := w.renderedFrame.cursorStyles[i]
		if req.hitbox.IsHovered(w) {
			resolved = req.style
			break
		}
	}
	if w.cursorApplied && w.lastCursor == resolved {
		return
	}
	w.lastCursor = resolved
	w.cursorApplied = true
	if w.cursorApplier != nil {
		w.cursorApplier(platform.MouseCursorStyle(uint8(style.MouseCursorForStyle(resolved))))
	}
}

// ---------------------------------------------------------------------------
// UI-thread timers (upstream spawn + background_executor().timer().await)
// ---------------------------------------------------------------------------

// SetMainThreadDispatch installs the dispatcher ScheduleAfter posts through.
// Upstream runs all window code on the main thread; this port's timer
// goroutine never touches window state itself - it hands the continuation to
// this hook (typically platform.DispatchOnMainThread), mirroring the editor
// example's caret blink. Without a dispatcher (single-threaded headless
// hosts) ScheduleAfter runs the continuation inline.
func (w *Window) SetMainThreadDispatch(f func(f func())) { w.mainDispatch = f }

// HasMainThreadDispatch reports whether ScheduleAfter posts through a
// UI-thread dispatcher. When false, tooltip wake-up timers are not armed:
// hosts drive frames themselves (headless tests draw after the delay).
func (w *Window) HasMainThreadDispatch() bool { return w.mainDispatch != nil }

// CanScheduleTimers reports whether ScheduleAfter can deliver its
// continuation to the UI thread: either a window-level dispatcher was
// installed (SetMainThreadDispatch) or the app-level one was
// (app.SetMainThreadDispatcher). Hosts with neither run single-threaded
// frames themselves and must drive delayed repaints manually (headless
// tests), so no timer goroutines are armed for them.
func (w *Window) CanScheduleTimers() bool {
	return w.mainDispatch != nil || app.MainDispatcherInstalled()
}

// ScheduleAfter runs f on the UI thread after d (upstream spawns a timer
// task and re-enters the window through cx.update). The continuation posts
// through the window dispatcher when installed, else through the app-level
// dispatcher (which runs inline when absent, matching single-threaded hosts).
func (w *Window) ScheduleAfter(d time.Duration, f func(w *Window)) {
	time.AfterFunc(d, func() {
		if w.mainDispatch != nil {
			w.mainDispatch(func() { f(w) })
			return
		}
		app.DispatchOnMainThread(func() { f(w) })
	})
}

// ScheduleFrame draws pending changes outside the frame walk (upstream
// schedules the next vsync frame for a dirty window).
func (w *Window) ScheduleFrame() { w.invalidator.ScheduleFrame() }

// ---------------------------------------------------------------------------
// Overlay prepaint/paint (drag view + tooltip)
// ---------------------------------------------------------------------------

// sortDeferredDraws orders deferred draws by priority, lower first, so
// higher priorities paint on top (upstream deferred draw ordering). Stable:
// equal priorities keep registration order.
func (w *Window) sortDeferredDraws() {
	for i := 1; i < len(w.deferredDraws); i++ {
		d := w.deferredDraws[i]
		j := i - 1
		for j >= 0 && w.deferredDraws[j].priority > d.priority {
			w.deferredDraws[j+1] = w.deferredDraws[j]
			j--
		}
		w.deferredDraws[j+1] = d
	}
}

// prepaintOverlay lays out and prepaints the active drag view, or when no
// drag is in flight the frame tooltip, as an independent root above the
// tree (upstream draw(): prompt > active drag > tooltip; the drag
// suppresses the tooltip).
func (w *Window) prepaintOverlay() {
	if w.activeDrag != nil && w.activeDrag.View != nil {
		node := w.requestLayoutElement(w.activeDrag.View)
		w.layoutTree.ComputeLayout(layout.Node(node), layout.MaxContentAvailable())
		offset := geometry.PointPixels(
			w.mousePosition.X.AsF32()-w.activeDrag.CursorOffset.X.AsF32(),
			w.mousePosition.Y.AsF32()-w.activeDrag.CursorOffset.Y.AsF32())
		w.dragOverlayNode = node
		w.dragOverlayOffset = offset
		w.dragOverlayValid = true
		w.PrepaintElementAt(w.activeDrag.View, node, offset)
		return
	}
	if w.frameTooltip == nil {
		return
	}
	node := w.requestLayoutElement(w.frameTooltip)
	w.layoutTree.ComputeLayout(layout.Node(node), layout.MaxContentAvailable())
	l := w.layoutTree.Layout(layout.Node(node))
	size := geometry.SizePixels(l.Width, l.Height)
	origin := tooltipOriginFor(w.frameTooltipPos, size, w.LogicalContentSize())
	w.tooltipOverlayNode = node
	w.tooltipOverlayValid = true
	w.PrepaintElementAt(w.frameTooltip, node, origin)
}

// paintOverlay paints the drag view or tooltip prepared by prepaintOverlay
// in a top paint layer, above deferred draws (upstream paint order at the
// end of window.draw).
func (w *Window) paintOverlay() {
	var el Element
	var offset geometry.Point[geometry.Pixels]
	if w.dragOverlayValid && w.activeDrag != nil {
		el = w.activeDrag.View
		offset = w.dragOverlayOffset
	} else if w.tooltipOverlayValid {
		el = w.frameTooltip
		origin := tooltipOriginFor(w.frameTooltipPos, w.overlaySize(w.tooltipOverlayNode), w.LogicalContentSize())
		offset = origin
	}
	if el == nil {
		return
	}
	close := w.PaintLayer()
	w.WithAbsoluteElementOffset(offset, func() {
		w.PaintElement(el)
	})
	close()
}

// overlaySize returns the laid-out size of an overlay element's node.
func (w *Window) overlaySize(node LayoutID) geometry.Size[geometry.Pixels] {
	l := w.layoutTree.Layout(layout.Node(node))
	return geometry.SizePixels(l.Width, l.Height)
}
