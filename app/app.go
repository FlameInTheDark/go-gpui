package app

import (
	"reflect"
	"sort"
	"sync/atomic"
)

// Effect is one deferred reaction. The queue is strictly FIFO by enqueue
// time; Notify and NotifyGlobalObservers dedup per entity/type per cycle.
type effect interface{ isEffect() }

type effectNotify struct{ emitter EntityID }
type effectEmit struct {
	emitter   EntityID
	eventType reflect.Type
	event     any
}
type effectRefreshWindows struct{}
type effectNotifyGlobal struct{ globalType reflect.Type }
type effectDefer struct{ callback func(*App) }
type effectEntityCreated struct {
	entity EntityID
	typ    reflect.Type
	window WindowID
}

func (effectNotify) isEffect()         {}
func (effectEmit) isEffect()           {}
func (effectRefreshWindows) isEffect() {}
func (effectNotifyGlobal) isEffect()   {}
func (effectDefer) isEffect()          {}
func (effectEntityCreated) isEffect()  {}

// subscriberState is the lifecycle of a subscription.
type subscriberState uint8

const (
	subPending subscriberState = iota
	subActive
	subDropped
)

type subscriber struct {
	state    *subscriberState
	callback func(*App) bool // false = unsubscribe
}

// subscriberSet mirrors gpui's SubscriberSet: keys to insertion-ordered
// callback lists, with pending activation and re-entrant retain.
type subscriberSet[K comparable] struct {
	byKey   map[K][]*subscriber
	nextID  int
	orderID map[*subscriber]int
}

func newSubscriberSet[K comparable]() *subscriberSet[K] {
	return &subscriberSet[K]{byKey: map[K][]*subscriber{}, orderID: map[*subscriber]int{}}
}

// insertActive adds an immediately-active subscriber.
func (s *subscriberSet[K]) insertActive(key K, cb func(*App) bool) *subscriber {
	sub := &subscriber{state: new(subscriberState), callback: cb}
	*sub.state = subActive
	s.nextID++
	s.orderID[sub] = s.nextID
	s.byKey[key] = append(s.byKey[key], sub)
	return sub
}

// insertPending returns a subscriber to activate later plus the activator.
func (s *subscriberSet[K]) insertPending(key K, cb func(*App) bool) (*subscriber, func()) {
	sub := &subscriber{state: new(subscriberState), callback: cb}
	*sub.state = subPending
	s.nextID++
	s.orderID[sub] = s.nextID
	s.byKey[key] = append(s.byKey[key], sub)
	return sub, func() { *sub.state = subActive }
}

// removeKey drains and returns all callbacks for a key.
func (s *subscriberSet[K]) removeKey(key K) []func(*App) bool {
	subs := s.byKey[key]
	delete(s.byKey, key)
	var out []func(*App) bool
	for _, sub := range subs {
		*sub.state = subDropped
		out = append(out, sub.callback)
	}
	return out
}

// retain invokes each active callback; a callback returning false (or a
// subscriber dropped by an earlier callback this pass) unsubscribes.
// Subscribers added during retain run later (appended, not visited now).
func (s *subscriberSet[K]) retain(key K, invoke func(func(*App) bool) bool) {
	subs := s.byKey[key]
	if len(subs) == 0 {
		return
	}
	// Take the current list out to allow re-entrancy; new subscribers go to
	// a fresh list.
	s.byKey[key] = nil
	remaining := subs[:0]
	for _, sub := range subs {
		if *sub.state == subActive {
			keep := invoke(sub.callback)
			if keep && *sub.state != subDropped {
				remaining = append(remaining, sub)
				continue
			}
			*sub.state = subDropped
			continue
		}
		if *sub.state == subPending {
			remaining = append(remaining, sub)
		}
	}
	s.byKey[key] = append(remaining, s.byKey[key]...)
}

// WindowID identifies a window.
type WindowID uint64

// ManagedWindow is the interface the gpui Window implements so the App can
// route invalidation without importing the window package.
type ManagedWindow interface {
	// WindowID returns the window's id.
	WindowID() WindowID
	// InvalidateView marks the window dirty for the given entity; returns
	// false when the entity is not tracked by this window.
	InvalidateView(id EntityID) bool
	// IsDrawing reports whether the window is mid-draw.
	IsDrawing() bool
	// ScheduleFrame requests a frame from the platform.
	ScheduleFrame()
	// MarkRefreshing marks the window for a cache-busting refresh.
	MarkRefreshing()
	// Refresh marks the window fully dirty (RefreshWindows effect).
	Refresh()
	// DrawIfDirty draws offscreen (test mode only; no-op otherwise).
	DrawIfDirty()
	// IsDirty reports pending redraws.
	IsDirty() bool
	// NeedsPresent reports pending presents.
	NeedsPresent() bool
}

// App is the root context: it owns entities, windows, globals, the effect
// queue and the subscriber sets. It is NOT safe for concurrent use; all
// access happens on the main thread (platform) or the test harness.
type App struct {
	entities *EntityMap

	windows      map[WindowID]ManagedWindow
	nextWindowID WindowID
	windowOrder  []WindowID

	pendingEffects        []effect
	pendingNotifications  map[EntityID]bool
	pendingGlobalNotifies map[reflect.Type]bool
	pendingUpdates        int

	observers          *subscriberSet[EntityID]
	eventListeners     *subscriberSet[entityEventKey]
	releaseListeners   *subscriberSet[EntityID]
	globalObservers    *subscriberSet[reflect.Type]
	newEntityObservers *subscriberSet[reflect.Type]

	globals map[reflect.Type]any

	// windowInvalidatorsByEntity maps an entity to the windows displaying it.
	windowInvalidatorsByEntity map[EntityID]map[WindowID]bool
	// trackedEntities maps a window to the entities its last frame accessed.
	trackedEntities map[WindowID]map[EntityID]bool

	foregroundQueue []func(*App)
	background      *BackgroundExecutor

	quitting    bool
	testMode    bool
	skipDrawing bool

	// OnEffectFlushed runs after each flush completes (used by tests).
	OnEffectFlushed func()
}

type entityEventKey struct {
	emitter   EntityID
	eventType reflect.Type
}

// New creates an App.
func New() *App {
	return &App{
		entities:                   newEntityMap(),
		windows:                    map[WindowID]ManagedWindow{},
		pendingNotifications:       map[EntityID]bool{},
		pendingGlobalNotifies:      map[reflect.Type]bool{},
		observers:                  newSubscriberSet[EntityID](),
		eventListeners:             newSubscriberSet[entityEventKey](),
		releaseListeners:           newSubscriberSet[EntityID](),
		globalObservers:            newSubscriberSet[reflect.Type](),
		newEntityObservers:         newSubscriberSet[reflect.Type](),
		globals:                    map[reflect.Type]any{},
		windowInvalidatorsByEntity: map[EntityID]map[WindowID]bool{},
		trackedEntities:            map[WindowID]map[EntityID]bool{},
		background:                 NewBackgroundExecutor(),
	}
}

// NewTestApp creates an App in test mode: flush_effects draws dirty windows
// synchronously.
func NewTestApp() *App {
	a := New()
	a.testMode = true
	return a
}

// Background returns the background executor.
func (a *App) Background() *BackgroundExecutor { return a.background }

// IsTestMode reports test mode.
func (a *App) IsTestMode() bool { return a.testMode }

// SkipDrawing disables test-mode drawing.
func (a *App) SkipDrawing() { a.skipDrawing = true }

// Quitting reports the shutdown state.
func (a *App) Quitting() bool { return a.quitting }

// Quit initiates shutdown.
func (a *App) Quit() { a.quitting = true }

// ---------------------------------------------------------------------------
// Entities
// ---------------------------------------------------------------------------

// newEntity is the shared creation path for the package-level NewEntity /
// ReserveEntity functions.
func (a *App) newEntity(build func(id EntityID)) EntityID {
	if a.quitting {
		panic("cannot create entities while app is quitting")
	}
	id := a.entities.reserve()
	build(id)
	return id
}

// withEntity leases an entity and runs f with its value WITHOUT wrapping in
// an update (no nested flush). Used by observer/subscription callbacks,
// mirroring how Rust's retain invokes handlers inside flush_effects.
func (a *App) withEntity(id EntityID, f func(any)) {
	v := a.entities.lease(id)
	if v == nil {
		panic(fmtEntityMissing(id))
	}
	defer a.entities.endLease(id, v)
	f(v)
}

// ReadEntityErased reads an entity without compile-time typing.
func (a *App) ReadEntityErased(id EntityID) (any, bool) {
	return a.entities.get(id)
}

// updateEntityErased leases, updates and restores.
func (a *App) updateEntityErased(id EntityID, f func(any, *App) any) any {
	v := a.entities.lease(id)
	if v == nil {
		panic(fmtEntityMissing(id))
	}
	var result any
	a.Update(func(cx *App) {
		defer a.entities.endLease(id, v)
		result = f(v, cx)
	})
	return result
}

func fmtEntityMissing(id EntityID) string {
	return "cannot update " + id.String() + ": entity not found"
}

// Notify schedules observers of the entity (deduped per cycle).
func (a *App) Notify(emitter EntityID) {
	// If any live window displays this entity, route invalidation through
	// the window (which re-schedules a notify effect).
	if invalidators, ok := a.windowInvalidatorsByEntity[emitter]; ok && len(invalidators) > 0 {
		anyWindow := false
		for wid := range invalidators {
			if w, ok := a.windows[wid]; ok {
				if w.InvalidateView(emitter) {
					anyWindow = true
				}
			}
		}
		if anyWindow {
			return
		}
	}
	a.pushEffect(effectNotify{emitter: emitter})
}

// Emit schedules subscribers of the entity with a typed event.
func (a *App) Emit(emitter EntityID, event any) {
	a.pushEffect(effectEmit{
		emitter:   emitter,
		eventType: reflect.TypeOf(event),
		event:     event,
	})
}

// pushEffect enqueues with Notify/NotifyGlobal dedup.
func (a *App) pushEffect(e effect) {
	switch eff := e.(type) {
	case effectNotify:
		if a.pendingNotifications[eff.emitter] {
			return
		}
		a.pendingNotifications[eff.emitter] = true
	case effectNotifyGlobal:
		if a.pendingGlobalNotifies[eff.globalType] {
			return
		}
		a.pendingGlobalNotifies[eff.globalType] = true
	}
	a.pendingEffects = append(a.pendingEffects, e)
}

// Defer schedules a callback to run at the end of the outermost update.
func (a *App) Defer(f func(*App)) {
	a.pushEffect(effectDefer{callback: f})
}

// RefreshWindows marks every window for a cache-busting refresh.
func (a *App) RefreshWindows() {
	a.pushEffect(effectRefreshWindows{})
}

// Update runs f as one nested update; effects flush when the outermost
// update finishes.
func (a *App) Update(f func(*App)) {
	a.startUpdate()
	f(a)
	a.finishUpdate()
}

func (a *App) startUpdate() { a.pendingUpdates++ }

func (a *App) finishUpdate() {
	a.pendingUpdates--
	if a.pendingUpdates < 0 {
		a.pendingUpdates = 0
	}
	if a.pendingUpdates == 0 {
		a.flushEffects()
	}
}

// flushEffects is a faithful port of App::flush_effects: strict FIFO with
// release processing at the top of every iteration; test mode draws dirty
// windows before parking.
func (a *App) flushEffects() {
	for {
		a.releaseDroppedEntities()

		if len(a.pendingEffects) > 0 {
			e := a.pendingEffects[0]
			a.pendingEffects = a.pendingEffects[1:]
			a.applyEffect(e)
			continue
		}

		// Queue empty.
		if a.testMode && !a.skipDrawing {
			for _, wid := range a.sortedWindowIDs() {
				if w, ok := a.windows[wid]; ok {
					w.DrawIfDirty()
				}
			}
		}

		if len(a.pendingEffects) == 0 {
			for _, wid := range a.sortedWindowIDs() {
				if w, ok := a.windows[wid]; ok {
					if w.IsDirty() || w.NeedsPresent() {
						w.ScheduleFrame()
					}
				}
			}
			if a.OnEffectFlushed != nil {
				a.OnEffectFlushed()
			}
			return
		}
	}
}

func (a *App) applyEffect(e effect) {
	switch eff := e.(type) {
	case effectNotify:
		delete(a.pendingNotifications, eff.emitter)
		a.observers.retain(eff.emitter, func(cb func(*App) bool) bool {
			return cb(a)
		})
	case effectEmit:
		prev := currentEvent
		currentEvent = eff.event
		a.eventListeners.retain(entityEventKey{emitter: eff.emitter, eventType: eff.eventType}, func(cb func(*App) bool) bool {
			return cb(a)
		})
		currentEvent = prev
	case effectRefreshWindows:
		for _, wid := range a.sortedWindowIDs() {
			if w, ok := a.windows[wid]; ok {
				w.MarkRefreshing()
				w.Refresh()
			}
		}
	case effectNotifyGlobal:
		delete(a.pendingGlobalNotifies, eff.globalType)
		a.globalObservers.retain(eff.globalType, func(cb func(*App) bool) bool {
			return cb(a)
		})
	case effectDefer:
		eff.callback(a)
	case effectEntityCreated:
		prev, prevSet := currentNewEntity, currentNewEntitySet
		currentNewEntity, currentNewEntitySet = eff.entity, true
		a.newEntityObservers.retain(eff.typ, func(cb func(*App) bool) bool {
			return cb(a)
		})
		currentNewEntity, currentNewEntitySet = prev, prevSet
	}
}

func (a *App) releaseDroppedEntities() {
	dropped := a.entities.takeDropped()
	if len(dropped) == 0 {
		return
	}
	for _, id := range dropped {
		// Fire release listeners before dropping the value.
		a.releaseListeners.retain(id, func(cb func(*App) bool) bool {
			return cb(a)
		})
		a.releaseListeners.removeKey(id)
		a.observers.removeKey(id)
		for key := range a.eventListeners.byKey {
			if key.emitter == id {
				a.eventListeners.removeKey(key)
			}
		}
		delete(a.entities.accessed, id)
		delete(a.windowInvalidatorsByEntity, id)
		if int(id.Index) < len(a.entities.slots) {
			s := &a.entities.slots[id.Index]
			if s.occupied && s.gen == id.Gen {
				s.occupied = false
				s.value = nil
				s.gen++
				a.entities.freeList = append(a.entities.freeList, id.Index)
			}
		}
	}
	// Remove the window tracking entries.
	for wid, tracked := range a.trackedEntities {
		for _, id := range dropped {
			delete(tracked, id)
		}
		a.trackedEntities[wid] = tracked
	}
}

// ---------------------------------------------------------------------------
// Windows
// ---------------------------------------------------------------------------

// AddWindow registers a window with the app.
func (a *App) AddWindow(w ManagedWindow) WindowID {
	a.nextWindowID++
	id := a.nextWindowID
	a.windows[id] = w
	a.windowOrder = append(a.windowOrder, id)
	a.trackedEntities[id] = map[EntityID]bool{}
	return id
}

// RemoveWindow unregisters a window.
func (a *App) RemoveWindow(id WindowID) {
	delete(a.windows, id)
	delete(a.trackedEntities, id)
	for e, wins := range a.windowInvalidatorsByEntity {
		delete(wins, id)
		if len(wins) == 0 {
			delete(a.windowInvalidatorsByEntity, e)
		}
	}
	for i, w := range a.windowOrder {
		if w == id {
			a.windowOrder = append(a.windowOrder[:i], a.windowOrder[i+1:]...)
			break
		}
	}
}

// Window returns a registered window.
func (a *App) Window(id WindowID) (ManagedWindow, bool) {
	w, ok := a.windows[id]
	return w, ok
}

// Windows lists windows in creation order.
func (a *App) Windows() []ManagedWindow {
	var out []ManagedWindow
	for _, id := range a.sortedWindowIDs() {
		if w, ok := a.windows[id]; ok {
			out = append(out, w)
		}
	}
	return out
}

func (a *App) sortedWindowIDs() []WindowID {
	ids := append([]WindowID(nil), a.windowOrder...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// RecordEntitiesAccessed subscribes a window to future notifies of the
// given entities (the draw phase calls this with the entities its frame
// touched).
func (a *App) RecordEntitiesAccessed(window WindowID, entities []EntityID) {
	set := map[EntityID]bool{}
	for _, e := range entities {
		set[e] = true
		if a.windowInvalidatorsByEntity[e] == nil {
			a.windowInvalidatorsByEntity[e] = map[WindowID]bool{}
		}
		a.windowInvalidatorsByEntity[e][window] = true
	}
	a.trackedEntities[window] = set
}

// TrackedEntities returns the entities a window is currently displaying.
func (a *App) TrackedEntities(window WindowID) map[EntityID]bool {
	return a.trackedEntities[window]
}

// BeginAccessScope starts recording entity accesses (per frame).
func (a *App) BeginAccessScope() {
	a.entities.accessed = map[EntityID]bool{}
}

// TakeAccessed drains the recorded accesses.
func (a *App) TakeAccessed() []EntityID {
	out := make([]EntityID, 0, len(a.entities.accessed))
	for id := range a.entities.accessed {
		out = append(out, id)
	}
	a.entities.accessed = map[EntityID]bool{}
	sort.Slice(out, func(i, j int) bool { return out[i].AsU64() < out[j].AsU64() })
	return out
}

// Shutdown tears down the app.
func (a *App) Shutdown() {
	a.quitting = true
	for id := range a.windows {
		a.RemoveWindow(id)
	}
	a.flushEffects()
}

// ---------------------------------------------------------------------------
// Main-thread dispatch (platform hook)
// ---------------------------------------------------------------------------

// mainDispatcher holds the platform's DispatchOnMainThread hook, installed
// once by the application layer (upstream: results of background loads are
// published via DispatchOnMainThread so cache writes and repaints happen on
// the main thread).
var mainDispatcher atomic.Value // func(func())

// SetMainThreadDispatcher installs the platform main-thread dispatch hook,
// e.g. app.SetMainThreadDispatcher(p.DispatchOnMainThread). Call it once at
// startup, before any background task runs.
func SetMainThreadDispatcher(dispatch func(func())) {
	mainDispatcher.Store(dispatch)
}

// MainDispatcherInstalled reports whether a platform dispatcher is present.
func MainDispatcherInstalled() bool { return mainDispatcher.Load() != nil }

// DispatchOnMainThread runs f on the main thread when a dispatcher is
// installed; otherwise f runs inline on the calling thread (single-threaded
// platforms and tests publish synchronously).
func DispatchOnMainThread(f func()) {
	if v := mainDispatcher.Load(); v != nil {
		v.(func(func()))(f)
		return
	}
	f()
}

// ---------------------------------------------------------------------------
// Globals
// ---------------------------------------------------------------------------

// SetGlobal stores a global and notifies global observers.
func SetGlobal[T any](a *App, value T) {
	a.globals[reflect.TypeOf(value)] = value
	a.pushEffect(effectNotifyGlobal{globalType: reflect.TypeOf(value)})
}

// Global returns a global, panicking when absent.
func Global[T any](a *App) T {
	var zero T
	v, ok := a.globals[reflect.TypeOf(zero)]
	if !ok {
		panic("global not set")
	}
	return v.(T)
}

// TryGlobal returns a global when present.
func TryGlobal[T any](a *App) (T, bool) {
	var zero T
	v, ok := a.globals[reflect.TypeOf(zero)]
	if !ok {
		return zero, false
	}
	return v.(T), true
}

// HasGlobal reports presence.
func HasGlobal[T any](a *App) bool {
	var zero T
	_, ok := a.globals[reflect.TypeOf(zero)]
	return ok
}

// UpdateGlobal leases a global for mutation and notifies observers.
func UpdateGlobal[T any](a *App, f func(*T)) {
	var zero T
	typ := reflect.TypeOf(zero)
	v, ok := a.globals[typ]
	if !ok {
		panic("global not set")
	}
	a.Update(func(cx *App) {
		val := v.(T)
		f(&val)
		cx.globals[typ] = val
		cx.pushEffect(effectNotifyGlobal{globalType: typ})
	})
}
