// Package app implements gpui's reactive core: the App context, entities,
// the effect queue (with upstream's exact flush ordering), subscriptions,
// globals and executors.
//
// Ownership model (ported from _ownership_and_data_flow.rs): the App owns
// all entity state. Entity handles are refcounted claims on an id; they
// only grant access while an App is present. cx.Notify() declares "state
// changed"; effects (observers, subscribers, defers) run in FIFO order at
// the end of the outermost update.
package app

import (
	"fmt"
	"reflect"
)

// EntityID identifies an entity: a slot index plus a generation tag, so ids
// of released entities are never reused.
type EntityID struct {
	Index uint32
	Gen   uint32
}

// AsU64 packs the id.
func (id EntityID) AsU64() uint64 { return uint64(id.Index)<<32 | uint64(id.Gen) }

// EntityIDFromU64 unpacks an id.
func EntityIDFromU64(v uint64) EntityID {
	return EntityID{Index: uint32(v >> 32), Gen: uint32(v)}
}

func (id EntityID) String() string { return fmt.Sprintf("entity#%d.%d", id.Index, id.Gen) }

// entitySlot is one slot in the entity map.
type entitySlot struct {
	value    any
	gen      uint32
	occupied bool
	leased   bool
	refcount int
}

// EntityMap is the slotmap-style entity storage owned by App.
type EntityMap struct {
	slots    []entitySlot
	freeList []uint32
	dropped  []EntityID
	// accessed tracks entities touched during the current scope (draw).
	accessed map[EntityID]bool
}

func newEntityMap() *EntityMap {
	return &EntityMap{accessed: map[EntityID]bool{}}
}

func (m *EntityMap) reserve() EntityID {
	if len(m.freeList) > 0 {
		idx := m.freeList[len(m.freeList)-1]
		m.freeList = m.freeList[:len(m.freeList)-1]
		m.slots[idx].occupied = true
		m.slots[idx].refcount = 1
		return EntityID{Index: idx, Gen: m.slots[idx].gen}
	}
	m.slots = append(m.slots, entitySlot{occupied: true, refcount: 1})
	return EntityID{Index: uint32(len(m.slots) - 1), Gen: 0}
}

func (m *EntityMap) insert(id EntityID, value any) {
	m.slots[id.Index].value = value
	m.accessed[id] = true
}

func (m *EntityMap) get(id EntityID) (any, bool) {
	if int(id.Index) >= len(m.slots) {
		return nil, false
	}
	s := &m.slots[id.Index]
	if !s.occupied || s.gen != id.Gen {
		return nil, false
	}
	m.accessed[id] = true
	return s.value, true
}

func (m *EntityMap) lease(id EntityID) any {
	if int(id.Index) >= len(m.slots) {
		return nil
	}
	s := &m.slots[id.Index]
	if !s.occupied || s.gen != id.Gen {
		return nil
	}
	if s.leased {
		panic(fmt.Sprintf("cannot update entity %v while it is already being updated", id))
	}
	s.leased = true
	v := s.value
	m.accessed[id] = true
	return v
}

func (m *EntityMap) endLease(id EntityID, value any) {
	s := &m.slots[id.Index]
	s.value = value
	s.leased = false
}

// release drops a strong reference; the entity is queued for release.
func (m *EntityMap) release(id EntityID) {
	if int(id.Index) >= len(m.slots) {
		return
	}
	s := &m.slots[id.Index]
	if !s.occupied || s.gen != id.Gen {
		return
	}
	s.refcount--
	if s.refcount <= 0 {
		m.dropped = append(m.dropped, id)
	}
}

// takeDropped drains the dropped queue.
func (m *EntityMap) takeDropped() []EntityID {
	out := m.dropped
	m.dropped = nil
	return out
}

// Entity is a strong, refcounted handle to an entity of type T owned by the
// App.
type Entity[T any] struct {
	id  EntityID
	app *App
}

// ID returns the entity id.
func (e Entity[T]) ID() EntityID { return e.id }

// IsValid reports whether the handle refers to a live app.
func (e Entity[T]) IsValid() bool { return e.app != nil }

// EntityIDOf returns the id (any entity type).
func EntityIDOf[T any](e Entity[T]) EntityID { return e.id }

// Clone bumps the refcount and returns a new handle.
func (e Entity[T]) Clone() Entity[T] {
	if e.app != nil {
		e.app.entities.bumpRefcount(e.id)
	}
	return e
}

// Release drops a strong reference.
func (e Entity[T]) Release() {
	if e.app != nil {
		e.app.entities.release(e.id)
		e.app = nil
	}
}

// Weak returns a weak handle.
func (e Entity[T]) Weak() WeakEntity[T] {
	return WeakEntity[T]{id: e.id, app: e.app}
}

// Read returns a view of the entity's state.
func (e Entity[T]) Read(cx *App) *T {
	v, ok := cx.entities.get(e.id)
	if !ok {
		panic(fmt.Sprintf("cannot read %v: entity not found", e.id))
	}
	return v.(*T)
}

// ReadWith runs f with a view of the state.
func (e Entity[T]) ReadWith(cx *App, f func(*T)) {
	f(e.Read(cx))
}

// Update leases the entity, runs f with a Context, restores and returns f's
// result.
func (e Entity[T]) Update(cx *App, f func(*T, *Context[T]) any) any {
	return cx.updateEntityErased(e.id, func(v any, cxAny *App) any {
		return f(v.(*T), &Context[T]{App: cxAny, entity: e.id})
	})
}

// UpdateIn is Update with a window available (the window is threaded by the
// caller through gpui).
func (e Entity[T]) UpdateIn(cx *App, f func(*T, *Context[T]) any) any {
	return e.Update(cx, f)
}

// Write replaces the state and notifies.
func (e Entity[T]) Write(cx *App, value T) {
	e.Update(cx, func(t *T, _ *Context[T]) any {
		*t = value
		return nil
	})
}

func (e Entity[T]) String() string { return e.id.String() }

// WeakEntity is a non-owning handle.
type WeakEntity[T any] struct {
	id  EntityID
	app *App
}

// Upgrade returns a handle if the entity is alive. Unlike Rust, this does
// NOT take a strong reference: Go has no destructors, so a discarded
// upgraded handle would leak the refcount. Ownership changes are explicit
// (Entity.Clone / Entity.Release). The returned handle is valid for
// immediate use on the app thread.
func (w WeakEntity[T]) Upgrade() (Entity[T], bool) {
	if w.app == nil {
		return Entity[T]{}, false
	}
	if !w.app.entities.isAlive(w.id) {
		return Entity[T]{}, false
	}
	return Entity[T]{id: w.id, app: w.app}, true
}

// ID returns the entity id.
func (w WeakEntity[T]) ID() EntityID { return w.id }

// AnyEntity is a type-erased strong handle.
type AnyEntity struct {
	id  EntityID
	app *App
	typ reflect.Type
}

// ID returns the entity id.
func (a AnyEntity) ID() EntityID { return a.id }

// Downcast returns a typed handle if the types match.
func DowncastEntity[T any](a AnyEntity) (Entity[T], bool) {
	var zero T
	if a.typ == reflect.TypeOf(&zero).Elem() {
		return Entity[T]{id: a.id, app: a.app}, true
	}
	return Entity[T]{}, false
}

func (m *EntityMap) bumpRefcount(id EntityID) {
	if int(id.Index) < len(m.slots) {
		s := &m.slots[id.Index]
		if s.occupied && s.gen == id.Gen {
			s.refcount++
		}
	}
}

func (m *EntityMap) isAlive(id EntityID) bool {
	if int(id.Index) >= len(m.slots) {
		return false
	}
	s := &m.slots[id.Index]
	return s.occupied && s.gen == id.Gen && s.refcount > 0
}
