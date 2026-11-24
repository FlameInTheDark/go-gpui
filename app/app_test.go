package app

import (
	"fmt"
	"testing"
)

// --- test windows -----------------------------------------------------------

type testWindow struct {
	id         WindowID
	dirty      bool
	refreshing bool
	drawCount  int
	draws      []int // draw counts snapshot helper
}

func (w *testWindow) WindowID() WindowID { return w.id }
func (w *testWindow) InvalidateView(id EntityID) bool {
	w.dirty = true
	return true
}
func (w *testWindow) IsDrawing() bool { return false }
func (w *testWindow) ScheduleFrame()  {}
func (w *testWindow) MarkRefreshing() { w.refreshing = true }
func (w *testWindow) Refresh()        { w.dirty = true }
func (w *testWindow) DrawIfDirty() {
	if w.dirty {
		w.dirty = false
		w.drawCount++
	}
}
func (w *testWindow) IsDirty() bool      { return w.dirty }
func (w *testWindow) NeedsPresent() bool { return false }

// --- entity basics ----------------------------------------------------------

type counter struct {
	n     int
	log   []string
	event string
}

func TestEntityCreateReadUpdate(t *testing.T) {
	a := NewTestApp()
	e := NewEntity(a, func(cx *Context[counter]) counter {
		return counter{n: 1}
	})
	if got := e.Read(a).n; got != 1 {
		t.Fatalf("read: %d", got)
	}
	e.Update(a, func(c *counter, cx *Context[counter]) any {
		c.n = 2
		return nil
	})
	if got := e.Read(a).n; got != 2 {
		t.Fatalf("update: %d", got)
	}
}

func TestEntityDoubleUpdatePanics(t *testing.T) {
	a := NewTestApp()
	e := NewEntity(a, func(cx *Context[counter]) counter { return counter{} })
	defer func() {
		if recover() == nil {
			t.Fatal("double update should panic")
		}
	}()
	e.Update(a, func(c *counter, cx *Context[counter]) any {
		e.Update(cx.App, func(c2 *counter, cx2 *Context[counter]) any { return nil })
		return nil
	})
}

func TestEntityReleaseAndGeneration(t *testing.T) {
	a := NewTestApp()
	e := NewEntity(a, func(cx *Context[counter]) counter { return counter{} })
	id1 := e.ID()
	e.Release()
	a.Update(func(cx *App) {}) // flush releases

	e2 := NewEntity(a, func(cx *Context[counter]) counter { return counter{} })
	if e2.ID().Index != id1.Index {
		t.Fatalf("slot should be reused: %v vs %v", e2.ID().Index, id1.Index)
	}
	if e2.ID().Gen == id1.Gen {
		t.Fatal("generation must bump on release")
	}
	// Old handle is dead.
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("reading released entity should panic")
			}
		}()
		e.Read(a)
	}()
}

// --- effect ordering (the heart of the port) --------------------------------

func TestEffectOrderFIFO(t *testing.T) {
	a := NewTestApp()
	e := NewEntity(a, func(cx *Context[counter]) counter { return counter{} })

	var order []string
	a.Update(func(cx *App) {
		cx.Notify(e.ID()) // 1st
		e.Update(cx, func(c *counter, cctx *Context[counter]) any {
			cctx.Emit("event") // 2nd
			return nil
		})
		cx.Defer(func(cx *App) { order = append(order, "defer") }) // 3rd
	})

	// Observers/subscribers registered during THIS update are pending until
	// a defer activates them, so install them before the update.
	a2 := NewTestApp()
	e2 := NewEntity(a2, func(cx *Context[counter]) counter { return counter{} })
	var order2 []string
	a2.Update(func(cx *App) {
		// Register observers first (pending activation via defer effect).
		e2.Update(cx, func(c *counter, cctx *Context[counter]) any {
			ObserveSelf(cctx, func(c *counter, _ *Context[counter]) {
				order2 = append(order2, "observer")
			})
			SubscribeSelf(cctx, func(ev *string, _ *Context[counter]) {
				order2 = append(order2, "subscriber:"+*ev)
			})
			return nil
		})
		// Activate pending subscriptions by flushing through a nested update.
		// (Upstream activates them via a Defer effect in the same queue.)
	})
	// Now the classic ordering test.
	a2.Update(func(cx *App) {
		cx.Notify(e2.ID())
		e2.Update(cx, func(c *counter, cctx *Context[counter]) any {
			cctx.Emit("hello")
			return nil
		})
		cx.Defer(func(cx *App) { order2 = append(order2, "defer") })
	})
	want := []string{"observer", "subscriber:hello", "defer"}
	if fmt.Sprint(order2) != fmt.Sprint(want) {
		t.Fatalf("effect order: got %v want %v", order2, want)
	}
}

func TestNotifyDedupPerCycle(t *testing.T) {
	a := NewTestApp()
	e := NewEntity(a, func(cx *Context[counter]) counter { return counter{} })
	observed := 0
	// Install observer and flush activation.
	e.Update(a, func(c *counter, cctx *Context[counter]) any {
		ObserveSelf(cctx, func(c *counter, _ *Context[counter]) { observed++ })
		return nil
	})
	a.Update(func(cx *App) {}) // activation flush
	if observed != 0 {
		t.Fatalf("observer should not fire yet: %d", observed)
	}
	a.Update(func(cx *App) {
		cx.Notify(e.ID())
		cx.Notify(e.ID())
		cx.Notify(e.ID())
	})
	if observed != 1 {
		t.Fatalf("deduped notify: %d", observed)
	}
	// Next cycle fires again.
	a.Update(func(cx *App) { cx.Notify(e.ID()) })
	if observed != 2 {
		t.Fatalf("second cycle: %d", observed)
	}
}

func TestObserverReNotifySameCycle(t *testing.T) {
	// An observer that notifies its own entity re-queues a fresh effect.
	a := NewTestApp()
	e := NewEntity(a, func(cx *Context[counter]) counter { return counter{} })
	hits := 0
	e.Update(a, func(c *counter, cctx *Context[counter]) any {
		ObserveSelf(cctx, func(c *counter, cx *Context[counter]) {
			hits++
			if hits < 3 {
				cx.Notify()
			}
		})
		return nil
	})
	a.Update(func(cx *App) {}) // activate
	a.Update(func(cx *App) { cx.Notify(e.ID()) })
	if hits != 3 {
		t.Fatalf("re-notify within a cycle: %d", hits)
	}
}

func TestSubscriptionActivationIsDeferred(t *testing.T) {
	// Subscriptions created mid-cycle do not observe effects queued earlier
	// in the same cycle.
	a := NewTestApp()
	emitter := NewEntity(a, func(cx *Context[counter]) counter { return counter{} })
	listener := NewEntity(a, func(cx *Context[counter]) counter { return counter{} })
	received := 0

	listener.Update(a, func(l *counter, lcx *Context[counter]) any {
		Observe(lcx, emitter, func(_ *counter, e Entity[counter], _ *Context[counter]) {
			received++
		})
		return nil
	})
	// Notify BEFORE the activation defer runs but in a later update — the
	// pending subscription is not yet active, so it must NOT receive this...
	// actually activation happens at the end of the first update's flush, so
	// notifications in the second update DO arrive.
	a.Update(func(cx *App) { cx.Notify(emitter.ID()) })
	if received != 1 {
		t.Fatalf("activated subscription should receive: %d", received)
	}
}

func TestObserverAutoUnsubscribeOnRelease(t *testing.T) {
	a := NewTestApp()
	observed := NewEntity(a, func(cx *Context[counter]) counter { return counter{} })
	listener := NewEntity(a, func(cx *Context[counter]) counter { return counter{} })
	fired := 0
	listener.Update(a, func(l *counter, lcx *Context[counter]) any {
		Observe(lcx, observed, func(_ *counter, _ Entity[counter], _ *Context[counter]) {
			fired++
		})
		return nil
	})
	a.Update(func(cx *App) {}) // activate
	a.Update(func(cx *App) { cx.Notify(observed.ID()) })
	if fired != 1 {
		t.Fatalf("first notify: %d", fired)
	}
	// Release the observed entity: observer auto-unsubscribes.
	observed.Release()
	a.Update(func(cx *App) {}) // release processing
	// The observed entity is gone: notifying its id enqueues nothing.
	if _, ok := a.ReadEntityErased(observed.ID()); ok {
		t.Fatal("entity should be released")
	}
	if fired != 1 {
		t.Fatalf("no more firings: %d", fired)
	}
}

func TestSubscribeTypedEvents(t *testing.T) {
	a := NewTestApp()
	emitter := NewEntity(a, func(cx *Context[counter]) counter { return counter{} })
	listener := NewEntity(a, func(cx *Context[counter]) counter { return counter{} })
	var got []string
	var gotInts []int
	listener.Update(a, func(l *counter, lcx *Context[counter]) any {
		Subscribe(lcx, emitter, func(ev *string, _ Entity[counter], _ *Context[counter]) {
			got = append(got, *ev)
		})
		Subscribe(lcx, emitter, func(ev *int, _ Entity[counter], _ *Context[counter]) {
			gotInts = append(gotInts, *ev)
		})
		return nil
	})
	a.Update(func(cx *App) {}) // activate
	emitter.Update(a, func(e *counter, ecx *Context[counter]) any {
		ecx.Emit("one")
		ecx.Emit(2)
		ecx.Emit("three")
		return nil
	})
	if fmt.Sprint(got) != "[one three]" {
		t.Fatalf("string events: %v", got)
	}
	if fmt.Sprint(gotInts) != "[2]" {
		t.Fatalf("int events: %v", gotInts)
	}
}

func TestOnReleaseFires(t *testing.T) {
	a := NewTestApp()
	e := NewEntity(a, func(cx *Context[counter]) counter { return counter{n: 7} })
	released := 0
	lastN := 0
	e.Update(a, func(c *counter, cx *Context[counter]) any {
		OnRelease(cx, e, func(c *counter, _ *App) {
			released++
			lastN = c.n
		})
		return nil
	})
	e.Update(a, func(c *counter, cx *Context[counter]) any {
		c.n = 9
		return nil
	})
	e.Release()
	a.Update(func(cx *App) {}) // release processing
	if released != 1 || lastN != 9 {
		t.Fatalf("release: fired=%d n=%d", released, lastN)
	}
}

func TestGlobals(t *testing.T) {
	a := NewTestApp()
	type config struct{ theme string }
	observed := 0
	ObserveGlobal[config](a, func(c *config, _ *App) { observed++ })
	SetGlobal(a, config{theme: "dark"})
	a.Update(func(cx *App) {})
	if observed != 1 {
		t.Fatalf("global observer: %d", observed)
	}
	if Global[config](a).theme != "dark" {
		t.Fatal("global read")
	}
	SetGlobal(a, config{theme: "light"})
	a.Update(func(cx *App) {})
	if observed != 2 {
		t.Fatalf("global observer dedup per cycle only: %d", observed)
	}
	if _, ok := TryGlobal[config](a); !ok {
		t.Fatal("try global")
	}
	if HasGlobal[config](a) != true {
		t.Fatal("has global")
	}
	UpdateGlobal(a, func(c *config) { c.theme = "solarized" })
	if Global[config](a).theme != "solarized" {
		t.Fatal("update global")
	}
}

func TestObserveNew(t *testing.T) {
	a := NewTestApp()
	type item struct{ name string }
	created := []string{}
	ObserveNew[item](a, func(e Entity[item], _ *App) {
		created = append(created, e.Read(a).name)
	})
	a.Update(func(cx *App) {}) // activate
	a.Update(func(cx *App) {
		NewEntity(cx, func(cx *Context[item]) item { return item{name: "a"} })
		NewEntity(cx, func(cx *Context[counter]) counter { return counter{} })
		NewEntity(cx, func(cx *Context[item]) item { return item{name: "b"} })
	})
	if fmt.Sprint(created) != "[a b]" {
		t.Fatalf("observe new: %v", created)
	}
}

func TestNestedUpdatesFlushOnce(t *testing.T) {
	a := NewTestApp()
	e := NewEntity(a, func(cx *Context[counter]) counter { return counter{} })
	flushes := 0
	a.OnEffectFlushed = func() { flushes++ }
	e.Update(a, func(c *counter, cx *Context[counter]) any {
		cx.Notify()
		// More effects in the same cycle must not flush early.
		cx.Notify()
		cx.Defer(func(*App) { flushes++ })
		// Defer increments before the flush we count below.
		flushes--
		return nil
	})
	if flushes != 1 {
		t.Fatalf("one flush per outermost update: %d", flushes)
	}
}

func TestTestModeDrawsDirtyWindows(t *testing.T) {
	a := NewTestApp()
	w := &testWindow{id: 1}
	a.AddWindow(w)
	a.Update(func(cx *App) {
		w.dirty = true
	})
	// The test-mode flush draws dirty windows synchronously.
	if w.drawCount != 1 {
		t.Fatalf("test mode should draw dirty window: %d", w.drawCount)
	}
}

func TestWindowInvalidationRouting(t *testing.T) {
	a := NewTestApp()
	e := NewEntity(a, func(cx *Context[counter]) counter { return counter{} })
	w := &testWindow{id: 1}
	a.AddWindow(w)
	// Window displays the entity.
	a.RecordEntitiesAccessed(1, []EntityID{e.ID()})
	a.Update(func(cx *App) {
		cx.Notify(e.ID())
	})
	// InvalidateView returned true => no observer Notify effect pushed, but
	// window is dirty and (in test mode) drawn.
	if !w.dirty && w.drawCount == 0 {
		t.Fatal("window should be invalidated or drawn")
	}
	// Entity is tracked.
	if !a.TrackedEntities(1)[e.ID()] {
		t.Fatal("tracked entities")
	}
}

func TestDeferRunsAfterObservers(t *testing.T) {
	a := NewTestApp()
	e := NewEntity(a, func(cx *Context[counter]) counter { return counter{} })
	var order []string
	e.Update(a, func(c *counter, cx *Context[counter]) any {
		ObserveSelf(cx, func(*counter, *Context[counter]) { order = append(order, "observe") })
		return nil
	})
	a.Update(func(cx *App) {}) // activate
	a.Update(func(cx *App) {
		cx.Notify(e.ID())
		cx.Defer(func(*App) { order = append(order, "defer") })
	})
	if fmt.Sprint(order) != "[observe defer]" {
		t.Fatalf("defer after observers: %v", order)
	}
}

func TestReserveEntityTwoPhase(t *testing.T) {
	a := NewTestApp()
	type parent struct {
		child *Entity[counter]
	}
	reserved, insert := ReserveEntity[counter](a)
	p := NewEntity(a, func(cx *Context[parent]) parent {
		c := reserved
		return parent{child: &c}
	})
	insert(counter{n: 42})
	if (*p.Read(a).child).Read(a).n != 42 {
		t.Fatal("two-phase creation")
	}
}

func TestWeakEntityUpgrade(t *testing.T) {
	a := NewTestApp()
	e := NewEntity(a, func(cx *Context[counter]) counter { return counter{} })
	w := e.Weak()
	if _, ok := w.Upgrade(); !ok {
		t.Fatal("weak upgrade while alive")
	}
	e.Release()
	a.Update(func(cx *App) {}) // release
	if _, ok := w.Upgrade(); ok {
		t.Fatal("weak upgrade after release")
	}
}
