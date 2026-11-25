package gpui

import (
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/FlameInTheDark/go-gpui/app"
	"github.com/FlameInTheDark/go-gpui/geometry"
)

// LayoutID identifies a layout-tree node for one frame. Ids are minted by
// [Window.RequestLayout] and friends, are only meaningful within the frame
// that created them, and index the window's layout tree. The zero value is a
// valid id, not a "none" marker.
type LayoutID = uint32

// ElementId identifies an element within its parent (upstream ElementId).
// An id can be an integer, a name, an entity id or a name plus integer. Ids
// are pushed on the window's id path while an element is laid out, prepainted
// and painted, and the hash of the whole path keys persistent per-element
// state. Give elements that hold state (scroll offsets, text caches, clicked
// state) a stable id so the state survives frames; elements without an id get
// an automatic positional id derived from their layout node. The zero value is
// "no id" (see [ElementId.IsZero]).
type ElementId struct {
	kind    uint8 // 0 int, 1 name, 2 entity, 3 named-int
	integer uint64
	name    string
	entity  app.EntityID
}

// HitboxKey returns the identity string used for keyed hitboxes: it is
// stable across re-renders when the element carries a name id ("n:name" or
// "n:name:N") and empty for integer and entity ids, which are positional and
// therefore opt out of keyed capture. Pass the result to
// [Window.InsertKeyedHitbox]; pointer captures resolve against it so a drag
// survives frames where the tree's structure changed (a resize grip keeps its
// identity while the panel's scrollbars appear or disappear).
func (id ElementId) HitboxKey() string {
	switch id.kind {
	case 1:
		return "n:" + id.name
	case 3:
		return fmt.Sprintf("n:%s:%d", id.name, id.integer)
	default:
		// Integer ids are positional: not stable across restructures, so
		// they opt out of keyed capture (the numeric id path handles them).
		return ""
	}
}

// IntElementId builds an integer id. Note that IntElementId(0) is the zero
// id and reports [ElementId.IsZero].
func IntElementId(v uint64) ElementId { return ElementId{kind: 0, integer: v} }

// NameElementId builds a name id. An empty name yields the zero id.
func NameElementId(name string) ElementId { return ElementId{kind: 1, name: name} }

// EntityElementId builds an id identifying the view entity e.
func EntityElementId(e app.EntityID) ElementId { return ElementId{kind: 2, entity: e} }

// NamedIntElementId builds a name+integer id, convenient for list items
// ("row", 3). Distinct names or integers produce distinct ids.
func NamedIntElementId(name string, v uint64) ElementId {
	return ElementId{kind: 3, name: name, integer: v}
}

// IsZero reports whether the id is unset. Elements with a zero id are not
// pushed on the id path by the window until it substitutes an automatic id.
func (id ElementId) IsZero() bool { return id.kind == 0 && id.integer == 0 && id.name == "" }

// String returns a human-readable form of the id (the name, "entity:<id>",
// "name-N" or the integer), for logs and diagnostics.
func (id ElementId) String() string {
	switch id.kind {
	case 1:
		return id.name
	case 2:
		return fmt.Sprintf("entity:%s", id.entity)
	case 3:
		return fmt.Sprintf("%s-%d", id.name, id.integer)
	default:
		return fmt.Sprintf("%d", id.integer)
	}
}

// hashElementId hashes one id deterministically.
func hashElementId(id ElementId) uint64 {
	h := fnv.New64a()
	var buf [8]byte
	buf[0] = id.kind
	_, _ = h.Write(buf[:1])
	v := id.integer ^ id.entity.AsU64()
	for i := 0; i < 8; i++ {
		buf[i] = byte(v >> (8 * i))
	}
	_, _ = h.Write(buf[:])
	_, _ = h.Write([]byte(id.name))
	return h.Sum64()
}

// EmptyPathHash is the hash of the empty id path, the seed that the first
// id of a path is folded into.
const EmptyPathHash = 0

// ExtendPathHash folds id into the path hash prefix and returns the hash of
// the extended path. It is deterministic and has no side effects, so it is
// safe to call from any goroutine.
func ExtendPathHash(prefix uint64, id ElementId) uint64 {
	return (prefix ^ hashElementId(id)) * 0x100000001b3
}

// idPathStack tracks element ids and prefix hashes during a frame.
type idPathStack struct {
	ids   []ElementId
	hashs []uint64 // hashs[i] = full hash of ids[0..i]
}

func (s *idPathStack) push(id ElementId) {
	var prev uint64
	if len(s.hashs) > 0 {
		prev = s.hashs[len(s.hashs)-1]
	}
	s.ids = append(s.ids, id)
	s.hashs = append(s.hashs, ExtendPathHash(prev, id))
}

func (s *idPathStack) pop() {
	if len(s.ids) == 0 {
		return
	}
	s.ids = s.ids[:len(s.ids)-1]
	s.hashs = s.hashs[:len(s.hashs)-1]
}

func (s *idPathStack) currentHash() uint64 {
	if len(s.hashs) == 0 {
		return EmptyPathHash
	}
	return s.hashs[len(s.hashs)-1]
}

// GlobalElementId is the full path of ids from the root plus its hash. Obtain
// the current one with [Window.CurrentElementID] during a frame. Capture it at
// paint time when an input handler (which runs outside the frame, with an
// empty id path) needs to update the element's state later, and pass it to
// [UpdateElementStateAt].
type GlobalElementId struct {
	ids  []ElementId
	hash uint64
}

// NewGlobalElementId builds a global id from a path of ids and the hash of
// that path (see [ExtendPathHash]). The slice is retained, not copied.
func NewGlobalElementId(ids []ElementId, hash uint64) GlobalElementId {
	return GlobalElementId{ids: ids, hash: hash}
}

// Hash returns the hash of the whole path; it is the key under which
// per-element state is stored.
func (g GlobalElementId) Hash() uint64 { return g.hash }

// String returns the ids of the path joined with dots, for diagnostics.
func (g GlobalElementId) String() string {
	parts := make([]string, len(g.ids))
	for i, id := range g.ids {
		parts[i] = id.String()
	}
	return strings.Join(parts, ".")
}

// Element is the drawing unit (upstream Element + ElementObject). Elements
// are rebuilt every frame from view state; persistent per-element state
// (scroll offsets, clicked state, focus handles) lives in the Window's
// elementState map keyed by the element's global id.
//
// The window calls the methods once per frame, in order, on the UI goroutine
// and never concurrently: RequestLayout for the whole tree, then Prepaint for
// the whole tree, then Paint for the whole tree. An element value should be
// used in a single frame only; element values are used as map keys by the
// window, so implementations must be comparable (pointer receivers are the
// norm).
type Element interface {
	// ElementID returns the element's optional id. Return the zero
	// [ElementId] for an anonymous element; the window then derives an
	// automatic id from the element's layout node.
	ElementID() ElementId
	// RequestLayout registers the element with the layout tree by calling
	// [Window.RequestLayout] (container) or [Window.RequestMeasuredLayout]
	// (leaf with intrinsic size) exactly once; the layout node id is tracked
	// by the window. An element that registers no node gets an empty
	// container node. Do not paint or register hitboxes here.
	RequestLayout(w *Window)
	// Prepaint runs with the element's computed bounds in window logical
	// pixels (after layout). Register hitboxes, tab stops, focus handles and
	// key contexts here, and prepaint children with [Window.PrepaintElement].
	// Do not record primitives here.
	Prepaint(w *Window, bounds geometry.Bounds[geometry.Pixels])
	// Paint records primitives and registers listeners ([Window.OnMouseEvent],
	// [Window.OnKeyEvent], [OnAction]). Paint children with
	// [Window.PaintElement] so their ids are pushed on the id path. Listeners
	// registered here live until the next frame replaces them.
	Paint(w *Window)
}

// Render is implemented by views (entities that produce elements). If *T
// implements Render, [OpenWindow] uses its Render method to draw the root
// entity of type T. Render is called during the layout-request phase with the
// window and the app; it should build and return a tree without side effects
// on the window and may return nil to draw nothing.
type Render interface {
	Render(w *Window, cx *app.App) Element
}

// ParentElement is implemented by container elements that expose their
// children. Children returns the child elements in paint order; the slice
// must not be modified by the caller.
type ParentElement interface {
	Children() []Element
}

// emptyElement paints nothing.
type emptyElement struct{}

func (emptyElement) ElementID() ElementId                                   { return ElementId{} }
func (emptyElement) RequestLayout(w *Window)                                {}
func (emptyElement) Prepaint(w *Window, b geometry.Bounds[geometry.Pixels]) {}
func (emptyElement) Paint(w *Window)                                        {}

// Empty returns an element that draws nothing, has the zero id and takes up
// an empty layout node. Use it as a placeholder child.
func Empty() Element { return emptyElement{} }
