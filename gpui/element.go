// Package gpui is the core of go-gpui: the Window (frame lifecycle, layout
// integration, hitboxes, focus, event dispatch, painting) and the Element
// model, ported from the upstream editor's gpui window.rs/element.rs. Concrete elements
// live in the elements package.
package gpui

import (
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/FlameInTheDark/go-gpui/app"
	"github.com/FlameInTheDark/go-gpui/geometry"
)

// LayoutID identifies a layout-tree node for one frame.
type LayoutID = uint32

// ElementId identifies an element within its parent (upstream ElementId).
type ElementId struct {
	kind    uint8 // 0 int, 1 name, 2 entity, 3 named-int
	integer uint64
	name    string
	entity  app.EntityID
}

// Package-internal: the element identity of a hitbox (stable across
// re-renders when the element carries a string id). InsertKeyedHitbox
// sets it; pointer captures resolve against it so a drag survives
// frames where the tree's structure changed (a resize grip keeps its
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

// IntElementId builds an integer id.
func IntElementId(v uint64) ElementId { return ElementId{kind: 0, integer: v} }

// NameElementId builds a name id.
func NameElementId(name string) ElementId { return ElementId{kind: 1, name: name} }

// EntityElementId builds an entity id.
func EntityElementId(e app.EntityID) ElementId { return ElementId{kind: 2, entity: e} }

// NamedIntElementId builds a name+integer id.
func NamedIntElementId(name string, v uint64) ElementId {
	return ElementId{kind: 3, name: name, integer: v}
}

// IsZero reports an unset id.
func (id ElementId) IsZero() bool { return id.kind == 0 && id.integer == 0 && id.name == "" }

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

// EmptyPathHash is the root hash seed.
const EmptyPathHash = 0

// ExtendPathHash folds one more id into a path hash.
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

// GlobalElementId is the full path of ids from the root plus its hash.
type GlobalElementId struct {
	ids  []ElementId
	hash uint64
}

// NewGlobalElementId builds a global id.
func NewGlobalElementId(ids []ElementId, hash uint64) GlobalElementId {
	return GlobalElementId{ids: ids, hash: hash}
}

// Hash returns the path hash.
func (g GlobalElementId) Hash() uint64 { return g.hash }

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
type Element interface {
	// ElementID returns the element's optional id.
	ElementID() ElementId
	// RequestLayout registers the element with the layout tree; the layout
	// node id is tracked by the window (see Window.RequestLayout).
	RequestLayout(w *Window)
	// Prepaint runs with the element's computed bounds.
	Prepaint(w *Window, bounds geometry.Bounds[geometry.Pixels])
	// Paint records primitives and registers listeners.
	Paint(w *Window)
}

// Render is implemented by views (entities that produce elements).
type Render interface {
	Render(w *Window, cx *app.App) Element
}

// ParentElement is implemented by containers.
type ParentElement interface {
	Children() []Element
}

// emptyElement paints nothing.
type emptyElement struct{}

func (emptyElement) ElementID() ElementId                                   { return ElementId{} }
func (emptyElement) RequestLayout(w *Window)                                {}
func (emptyElement) Prepaint(w *Window, b geometry.Bounds[geometry.Pixels]) {}
func (emptyElement) Paint(w *Window)                                        {}

// Empty returns an element that draws nothing.
func Empty() Element { return emptyElement{} }
