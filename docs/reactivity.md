# State, events, and goroutines

The [`app`](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui/app) package
provides app-owned entities, change notifications and typed events. To emit
and receive events, start with `Context.Emit` and `app.Subscribe` or
`app.SubscribeSelf`. For a complete typed-view application,
see the [tutorial](tutorial.md#3-move-growing-state-into-a-typed-view).

## Choosing an API

| Need | API |
| --- | --- |
| Hold app-owned, typed state | `app.NewEntity`, `Entity.Read`, `Entity.Update` |
| Report state changed | `Context.Notify`, `app.Observe`, `app.ObserveSelf` |
| Send an event with a payload | `Context.Emit`, `app.Subscribe`, `app.SubscribeSelf` |
| Change a view from a UI handler | `Context.Updater`, captured during render and called later |
| Run after the current update | `Context.Defer` |
| Queue a foreground callback from the app thread | `App.Spawn`, `App.SpawnForeground`; `App.Pump` drains the queue |
| Run work in a worker pool | `app.NewBackgroundExecutor`, `Spawn`, `SpawnWithPriority` |
| Return a worker result to a native UI loop | The platform's `DispatchOnMainThread` |

## Typed events

An event can be any Go value. The following example emits a `changed` value;
the callback is selected by that value's **exact dynamic type**. If you emit
`*changed` instead, a subscriber for `changed` will not receive it. See
[`ExampleSubscribe`](../app/example_test.go) for a runnable version. The
snippet below assumes imports of `fmt` and `github.com/FlameInTheDark/go-gpui/app`.

```go
type counter struct{ value int }
type changed struct{ value int }

a := app.NewTestApp()
e := app.NewEntity(a, func(*app.Context[counter]) counter { return counter{} })
listener := app.NewEntity(a, func(*app.Context[struct{}]) struct{} { return struct{}{} })

listener.Update(a, func(_ *struct{}, cx *app.Context[struct{}]) any {
    app.Subscribe(cx, e, func(event *changed, _ app.Entity[counter], _ *app.Context[struct{}]) {
        fmt.Println(event.value)
    })
    return nil
})
e.Update(a, func(state *counter, cx *app.Context[counter]) any {
    state.value++
    cx.Notify() // Notify is separate from Emit.
    cx.Emit(changed{value: state.value})
    return nil
})
```

For another entity, register `app.Subscribe(listenerContext, emitter, handler)`;
the handler receives a pointer to a copy of the event value, the emitting
entity handle, and the listener's context. Use `Observe` instead when the
listener needs to react to a state change rather than a particular event.
To change the listener's state from a `Subscribe` handler, use its entity's
`Update` method and call `Notify` as needed. `Observe` already supplies the
observer's state to its callback; do not recursively update that same entity.

**Current limitation:** `SubscribeSelf` releases an entity reference after
each callback even though its weak-handle upgrade does not acquire one. This
can release the entity prematurely. Prefer `Subscribe` with a separate listener
until this is fixed.

### Ordering and lifetime

- Effects are queued and processed in FIFO order when the outermost update
  finishes. `Emit` does not call the handler immediately inside the update.
- Registrations activate via a deferred effect. Events or notifications
  processed before that activation are not delivered to the new subscriber.
- Pending notifications for the same entity are coalesced until processed;
  individual emitted events are not coalesced.
- Save the returned `Subscription` if you need to call `Cancel`. Discarding
  it does **not** cancel it. `Detach` is not required to keep it alive. An
  entity's listeners are removed on release, and callbacks check the listener's
  lifetime when invoked.
- Entity handles are values, but copying one does not add a reference. Use
  `Clone` for an additional owned handle and `Release` each owned reference
  once. `WeakEntity.Upgrade` is for immediate use on the app thread and does
  not add an owned reference. `Entity.Write` does not notify; use `Update`
  and `Notify` when observers or views should react.

## Worker results and the UI thread

`App`, its entities and contexts, and the foreground queue are not safe for
concurrent use. Do not call `Update`, `Emit`, `Notify`, `Updater`, `Spawn`, or
`SpawnForeground` from a worker while the UI loop is using the app. The mutexes
on task handles do not protect the app or the foreground queue.

For a **running native backend**, capture the platform and an entity handle on
the UI thread, perform independent work in a goroutine, then dispatch the
result. The callback runs on the native event loop and may update the entity:

```go
// Called on the UI thread; plat is the native platform and view is an
// app.Entity[model] belonging to a.
weak := view.Weak()
go func() {
    result := calculate() // No app, entity, or window access here.
    plat.DispatchOnMainThread(func() {
        current, ok := weak.Upgrade()
        if !ok { return } // The view was released while work was running.
        current.Update(a, func(state *model, cx *app.Context[model]) any {
            state.result = result
            cx.Notify()
            return nil
        })
    })
}()
```

The application must keep running to process callbacks; coordinate shutdown
and cancellation in your application. The global `app.DispatchOnMainThread`
uses an installed dispatcher, but **runs inline if none is installed**. The
headless backend also dispatches inline, so it is not a cross-goroutine
handoff: in tests, send results over a Go channel and apply them on the test
goroutine that owns the app. Verify the behavior of a custom platform before
relying on its dispatcher.

`BackgroundExecutor` has four workers and a priority queue. Higher-priority
queued tasks are selected first, with FIFO selection at equal priority; running
tasks are not preempted. `SpawnWithPriority` receives a cancellation context,
and `BackgroundTask.Cancel` is cooperative for work already running. Wait on
`BackgroundTask.Done` to join. This executor does not automatically marshal
results back to the UI thread.
