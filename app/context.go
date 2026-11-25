package app

import (
	"container/heap"
	"context"
	"reflect"
	"sync"
	"time"
)

// Context is an entity-scoped view of the App (gpui's Context<T>). It
// embeds *App.
//
// Go does not allow type parameters on methods, so the generic
// observe/subscribe APIs are package-level functions (Observe, ObserveSelf,
// Subscribe, SubscribeSelf, OnRelease).
type Context[T any] struct {
	*App
	entity EntityID
}

// EntityID returns the owning entity's id.
func (c *Context[T]) EntityID() EntityID { return c.entity }

// Entity returns the owning entity.
func (c *Context[T]) Entity() Entity[T] { return Entity[T]{id: c.entity, app: c.App} }

// WeakEntity returns a weak handle to the owning entity.
func (c *Context[T]) WeakEntity() WeakEntity[T] {
	return WeakEntity[T]{id: c.entity, app: c.App}
}

// Notify declares that this entity's state changed. Pending notifications for
// the same entity are coalesced until the notification effect is processed.
// Call it on the app thread.
func (c *Context[T]) Notify() { c.App.Notify(c.entity) }

// Updater returns a function for changing this entity and notifying its
// observers after the change. Capture it during render and call it later
// from a UI event handler. Do not call it while the same entity is already
// being updated (including inside its render function), or from a worker
// goroutine.
func (c *Context[T]) Updater() func(func(*T)) {
	entity, a := c.Entity(), c.App
	return func(change func(*T)) {
		entity.Update(a, func(state *T, cx *Context[T]) any {
			change(state)
			cx.Notify()
			return nil
		})
	}
}

// Emit queues a typed event from this entity. Subscribers match the event's
// exact dynamic type, so emitting a value and a pointer to it are distinct.
// Call it on the app thread.
func (c *Context[T]) Emit(event any) { c.App.Emit(c.entity, event) }

// Defer schedules a callback for the end of the outermost update.
func (c *Context[T]) Defer(f func(*App)) { c.App.Defer(f) }

// Spawn queues f on the foreground queue with this entity's context. Like
// App.Spawn, it must be called on the app thread and runs when Pump drains
// the queue; it is not a worker-to-UI handoff.
func (c *Context[T]) Spawn(f func(*Context[T])) {
	self := c.entity
	c.App.Spawn(func(cx *App) {
		f(&Context[T]{App: cx, entity: self})
	})
}

// NewEntity creates an entity owned by the app; build receives a Context for
// the new entity. It returns an owned handle that should eventually be
// released, and panics if the app is quitting.
func NewEntity[T any](a *App, build func(*Context[T]) T) Entity[T] {
	if a.quitting {
		panic("cannot create entities while app is quitting")
	}
	id := a.entities.reserve()
	cx := &Context[T]{App: a, entity: id}
	value := build(cx)
	a.entities.insert(id, &value)
	a.pushEffect(effectEntityCreated{entity: id, typ: reflect.TypeOf(value), window: 0})
	return Entity[T]{id: id, app: a}
}

// ReserveEntity reserves an id without a value (two-phase creation for
// parent/child cycles).
func ReserveEntity[T any](a *App) (Entity[T], func(T)) {
	id := a.entities.reserve()
	return Entity[T]{id: id, app: a}, func(value T) {
		a.entities.insert(id, &value)
		a.pushEffect(effectEntityCreated{entity: id, typ: reflect.TypeOf(value), window: 0})
	}
}

// Observe registers a callback that fires when the given entity notifies.
// The callback receives the OBSERVER's own state plus a handle to the
// observed entity, mirroring Context::observe. The observer auto-unsubscribes
// when either entity is released. Registration is activated by a deferred
// effect; notifications processed before activation are not delivered.
func Observe[T, W any](c *Context[T], entity Entity[W], f func(*T, Entity[W], *Context[T])) Subscription {
	target := entity.Weak()
	self := c.WeakEntity()
	return observeInternal(c.App, entity.ID(), func(cx *App) bool {
		observed, ok := target.Upgrade()
		if !ok {
			return false // observed released: auto-unsubscribe
		}
		if _, ok := self.Upgrade(); !ok {
			return false // observer released: auto-unsubscribe
		}
		cx.withEntity(self.ID(), func(v any) {
			f(v.(*T), observed, &Context[T]{App: cx, entity: self.ID()})
		})
		return true
	})
}

// ObserveSelf observes this entity's own notifies.
func ObserveSelf[T any](c *Context[T], f func(*T, *Context[T])) Subscription {
	self := c.WeakEntity()
	return observeInternal(c.App, c.entity, func(cx *App) bool {
		if _, ok := self.Upgrade(); !ok {
			return false
		}
		cx.withEntity(self.ID(), func(v any) {
			f(v.(*T), &Context[T]{App: cx, entity: self.ID()})
		})
		return true
	})
}

func observeInternal(a *App, targetID EntityID, handler func(*App) bool) Subscription {
	sub, activate := a.observers.insertPending(targetID, handler)
	a.Defer(func(cx *App) { _ = cx; activate() })
	return Subscription{unsubscribe: func() { *sub.state = subDropped }}
}

// Subscribe registers a typed event listener on an entity. Only events whose
// dynamic type exactly matches E invoke f. The callback receives a pointer
// to a copy of the event value. Registration is activated by a deferred effect;
// events processed before activation are not delivered. Cancel the returned
// subscription explicitly when it is no longer needed.
func Subscribe[T, W, E any](c *Context[T], entity Entity[W], f func(*E, Entity[W], *Context[T])) Subscription {
	var zero E
	eventType := eventTypeOf(zero)
	self := c.WeakEntity()
	return subscribeInternal(c.App, entity.ID(), eventType, func(cx *App, ev any) bool {
		e, ok := ev.(E)
		if !ok {
			return true
		}
		if _, ok := self.Upgrade(); !ok {
			return false
		}
		f(&e, entity, &Context[T]{App: cx, entity: self.ID()})
		return true
	})
}

// SubscribeSelf subscribes to this entity's own events. The current
// implementation releases an entity reference after each callback, although
// the weak-handle upgrade does not acquire one. This can release the entity
// prematurely; prefer Subscribe with a separate listener until this is fixed.
func SubscribeSelf[T, E any](c *Context[T], f func(*E, *Context[T])) Subscription {
	var zero E
	eventType := eventTypeOf(zero)
	self := c.WeakEntity()
	return subscribeInternal(c.App, c.entity, eventType, func(cx *App, ev any) bool {
		e, ok := ev.(E)
		if !ok {
			return true
		}
		s, ok := self.Upgrade()
		if !ok {
			return false
		}
		defer s.Release()
		f(&e, &Context[T]{App: cx, entity: self.ID()})
		return true
	})
}

func eventTypeOf[E any](zero E) reflect.Type {
	t := reflect.TypeOf(zero)
	if t == nil {
		t = reflect.TypeOf(&zero).Elem()
	}
	return t
}

func subscribeInternal(a *App, targetID EntityID, eventType reflect.Type, handler func(*App, any) bool) Subscription {
	key := entityEventKey{emitter: targetID, eventType: eventType}
	wrapped := func(cx *App) bool {
		return handler(cx, currentEvent)
	}
	sub, activate := a.eventListeners.insertPending(key, wrapped)
	a.Defer(func(cx *App) { _ = cx; activate() })
	return Subscription{unsubscribe: func() { *sub.state = subDropped }}
}

// OnRelease registers a one-shot callback that runs when the entity's last
// strong handle is dropped (before the entity is removed).
func OnRelease[T any](c *Context[T], entity Entity[T], f func(*T, *App)) Subscription {
	id := entity.ID()
	wrapped := func(cx *App) bool {
		if v, ok := cx.entities.get(id); ok {
			f(v.(*T), cx)
		}
		return false
	}
	sub := c.releaseListeners.insertActive(id, wrapped)
	return Subscription{unsubscribe: func() { *sub.state = subDropped }}
}

// ObserveGlobal registers a callback for global changes of type T.
func ObserveGlobal[T any](a *App, f func(*T, *App)) Subscription {
	var zero T
	typ := reflect.TypeOf(zero)
	wrapped := func(cx *App) bool {
		if v, ok := cx.globals[typ]; ok {
			val := v.(T)
			f(&val, cx)
			return true
		}
		return false
	}
	sub, activate := a.globalObservers.insertPending(typ, wrapped)
	a.Defer(func(cx *App) { _ = cx; activate() })
	return Subscription{unsubscribe: func() { *sub.state = subDropped }}
}

// ObserveNew registers a callback for entities of type T created after now.
func ObserveNew[T any](a *App, f func(Entity[T], *App)) Subscription {
	var zero T
	typ := reflect.TypeOf(zero)
	wrapped := func(cx *App) bool {
		if !currentNewEntitySet {
			return true
		}
		if e, ok := DowncastEntity[T](AnyEntity{id: currentNewEntity, app: cx, typ: typ}); ok {
			f(e, cx)
		}
		return true
	}
	sub, activate := a.newEntityObservers.insertPending(typ, wrapped)
	a.Defer(func(cx *App) { _ = cx; activate() })
	return Subscription{unsubscribe: func() { *sub.state = subDropped }}
}

// currentEvent carries the event payload while an Emit effect runs.
var currentEvent any

// currentNewEntity carries the created entity while an EntityCreated
// effect runs; the Set flag distinguishes "no entity" from a zero id.
var (
	currentNewEntity    EntityID
	currentNewEntitySet bool
)

// Spawn queues f for the app thread, flushing effects after it completes.
// The queue is not goroutine-safe: call Spawn on the app thread, not from a
// worker goroutine. Pump runs queued callbacks.
func (a *App) Spawn(f func(*App)) {
	a.foregroundQueue = append(a.foregroundQueue, f)
}

// Pump drains the foreground queue. The platform calls this from its event
// loop; tests call it explicitly.
func (a *App) Pump() {
	for len(a.foregroundQueue) > 0 {
		f := a.foregroundQueue[0]
		a.foregroundQueue = a.foregroundQueue[1:]
		a.Update(f)
	}
}

// ---------------------------------------------------------------------------
// Executor (port of src/executor.rs)
// ---------------------------------------------------------------------------

// Priority is the scheduling priority of a background task (upstream
// scheduler::Priority). Larger values run sooner; RealtimeAudio outranks
// everything. The zero value is PriorityIdle - always pass an explicit
// priority to SpawnWithPriority (Spawn uses PriorityDefault).
type Priority uint8

const (
	// PriorityIdle parks tasks until nothing else is queued.
	PriorityIdle Priority = iota
	// PriorityLow runs below default priority work.
	PriorityLow
	// PriorityDefault matches upstream Priority::default().
	PriorityDefault
	// PriorityHigh runs above default priority work.
	PriorityHigh
	// PriorityRealtimeAudio is reserved for latency-sensitive work.
	PriorityRealtimeAudio
)

// taskEntry is one queued background task. The heap orders by priority,
// breaking ties by enqueue order (upstream's PriorityQueue semantics).
type taskEntry struct {
	priority Priority
	seq      uint64
	run      func(ctx context.Context)
	task     *BackgroundTask
	name     string // optional label for diagnostics and tests
}

// taskHeap is a max-heap on (priority, seq): higher priority first, FIFO
// within one priority level.
type taskHeap []*taskEntry

func (h taskHeap) Len() int { return len(h) }
func (h taskHeap) Less(i, j int) bool {
	if h[i].priority != h[j].priority {
		return h[i].priority > h[j].priority
	}
	return h[i].seq < h[j].seq
}
func (h taskHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *taskHeap) Push(x any)   { *h = append(*h, x.(*taskEntry)) }
func (h *taskHeap) Pop() any     { old := *h; n := len(old); x := old[n-1]; *h = old[:n-1]; return x }

// BackgroundTask is a handle to a task spawned on the background executor
// (upstream Task<R>). The spawned function receives a context that is
// cancelled by Cancel - cancellation is cooperative, exactly like dropping
// upstream futures only cancels at await points.
type BackgroundTask struct {
	mu        sync.Mutex
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	cancelled bool
	detached  bool
}

// Done returns a channel closed when the task finished running, was
// skipped because it was cancelled before it started, or observed
// cooperative cancellation and returned.
func (t *BackgroundTask) Done() <-chan struct{} { return t.done }

// Cancel cancels the task cooperatively and does not block: a task still
// waiting in the queue is skipped entirely, a running task's context is
// cancelled and the function observes it through ctx.Done(). Wait on
// Done() to join. Cancel on a detached task is a no-op (upstream detached
// tasks cannot be cancelled either).
func (t *BackgroundTask) Cancel() {
	t.mu.Lock()
	if t.detached || t.cancelled {
		t.mu.Unlock()
		return
	}
	t.cancelled = true
	cancel := t.cancel
	t.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Detach releases ownership of the task: it is guaranteed to run to
// completion (or until an already-issued cancellation) and can no longer be
// cancelled from this handle (upstream Task::detach).
func (t *BackgroundTask) Detach() {
	t.mu.Lock()
	t.detached = true
	t.mu.Unlock()
}

// isCancelled reports whether the task must not start running.
func (t *BackgroundTask) isCancelled() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cancelled
}

// BackgroundExecutor runs closures on a four-goroutine pool, queued by
// priority. Enqueueing tasks is synchronized, but their callbacks must not
// access App or entity state concurrently with the app thread. Construct an
// executor with NewBackgroundExecutor rather than using its zero value.
type BackgroundExecutor struct {
	mu   sync.Mutex
	cond *sync.Cond
	heap taskHeap
	seq  uint64
}

// NewBackgroundExecutor creates the worker pool (four workers, matching the
// previous fixed pool size).
func NewBackgroundExecutor() *BackgroundExecutor {
	be := &BackgroundExecutor{}
	be.cond = sync.NewCond(&be.mu)
	for i := 0; i < 4; i++ {
		go be.worker()
	}
	return be
}

// worker drains the priority queue. Exactly one worker closes a task's done
// channel: the worker that either skipped it (cancelled before run) or ran
// it to completion.
func (be *BackgroundExecutor) worker() {
	for {
		be.mu.Lock()
		for len(be.heap) == 0 {
			be.cond.Wait()
		}
		entry := heap.Pop(&be.heap).(*taskEntry)
		be.mu.Unlock()

		t := entry.task
		if t.isCancelled() {
			close(t.done)
			continue
		}
		entry.run(t.ctx)
		close(t.done)
	}
}

// Spawn runs f in the background at default priority (upstream
// BackgroundExecutor::spawn). The returned handle may be ignored.
func (be *BackgroundExecutor) Spawn(f func()) *BackgroundTask {
	return be.SpawnWithPriority(PriorityDefault, func(context.Context) { f() })
}

// SpawnWithPriority queues fn with the given priority; the queue is
// priority-ordered (higher first, FIFO within a level). The function
// receives a context cancelled by BackgroundTask.Cancel - cooperative
// cancellation, mirroring upstream's await-point cancellation.
func (be *BackgroundExecutor) SpawnWithPriority(priority Priority, fn func(ctx context.Context)) *BackgroundTask {
	ctx, cancel := context.WithCancel(context.Background())
	t := &BackgroundTask{ctx: ctx, cancel: cancel, done: make(chan struct{})}
	be.mu.Lock()
	be.seq++
	heap.Push(&be.heap, &taskEntry{priority: priority, seq: be.seq, run: fn, task: t})
	be.mu.Unlock()
	be.cond.Signal()
	return t
}

// Timer invokes f after d on a background goroutine.
func (be *BackgroundExecutor) Timer(d time.Duration, f func()) {
	go func() {
		t := time.NewTimer(d)
		<-t.C
		f()
	}()
}

// Now returns the wall clock (kept here so tests can patch it later).
func (be *BackgroundExecutor) Now() time.Time { return time.Now() }

// ---------------------------------------------------------------------------
// Foreground tasks (upstream ForegroundExecutor::spawn's Task)
// ---------------------------------------------------------------------------

// ForegroundTask is a handle to a closure queued onto the app thread.
// Cancelling before the queue drains prevents the closure from running.
// Done closes when the closure starts or is cancelled, not when it finishes.
type ForegroundTask struct {
	mu        sync.Mutex
	fn        func(*App)
	cancelled bool
	closed    bool
	done      chan struct{}
}

// SpawnForeground queues f for Pump and returns a cancellable handle.
// Cancellation before the queue drains prevents f from running. Enqueuing is
// not goroutine-safe; call SpawnForeground on the app thread.
func (a *App) SpawnForeground(f func(*App)) *ForegroundTask {
	t := &ForegroundTask{fn: f, done: make(chan struct{})}
	a.foregroundQueue = append(a.foregroundQueue, func(cx *App) {
		t.invoke(cx)
	})
	return t
}

// invoke runs the closure unless it was cancelled first, then closes done.
func (t *ForegroundTask) invoke(cx *App) {
	t.mu.Lock()
	f, cancelled := t.fn, t.cancelled
	t.fn = nil
	if !t.closed {
		t.closed = true
		close(t.done)
	}
	t.mu.Unlock()
	if !cancelled && f != nil {
		f(cx)
	}
}

// Cancel cancels the task before it runs; it is a no-op once the closure
// started (there is no cooperative cancellation on the main thread - the
// closure is already running or gone).
func (t *ForegroundTask) Cancel() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cancelled || t.closed {
		return
	}
	t.cancelled = true
	t.fn = nil
	t.closed = true
	close(t.done)
}

// Done returns a channel closed when the closure starts or is cancelled.
// It does not wait for a running closure to finish.
func (t *ForegroundTask) Done() <-chan struct{} { return t.done }
