package gpui

import (
	"errors"
	"sync/atomic"

	"github.com/FlameInTheDark/go-gpui/app"
	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/style"
	"github.com/FlameInTheDark/go-gpui/text"
)

// Application wires a platform, an App and window creation (upstream
// Application + App::open_window). It is the entry point for programs that
// need more than [github.com/FlameInTheDark/go-gpui.Run]: several windows,
// an explicit or headless platform, or typed views.
//
// Create one with [NewApplication], open at least one window with
// [OpenView] (or [OpenWindow]) and then call [Application.Run]. The
// application quits its event loop when the last window it opened closes.
// All methods except [Application.WindowCount] must be called on the UI
// goroutine.
type Application struct {
	// Platform is the backend that creates windows and runs the event loop.
	Platform platform.Platform
	// App is the entity and effect runtime shared by all windows of the
	// application.
	App *app.App
	// Text is the text system shared by all windows. It is the platform's
	// own system when that is a *text.System, otherwise a system using the
	// installed system fonts.
	Text *text.System

	openWindows int32
}

// NewApplication creates an application over the platform p. p must not be
// nil. No window is opened and no event loop is started; the text system is
// taken from p when it provides a *text.System and is otherwise created from
// system fonts, which can be slow on first use.
func NewApplication(p platform.Platform) *Application {
	a := &Application{Platform: p, App: app.New()}
	if ts, ok := p.TextSystem().(*text.System); ok {
		a.Text = ts
	}
	if a.Text == nil {
		a.Text = text.NewSystemWithSystemFonts()
	}
	return a
}

// Run runs the platform event loop and blocks until it ends, which happens
// when the last window opened through this application closes (or the
// platform is told to quit). Open at least one window first and call Run from
// the same goroutine that initialized the backend and opened it; on most
// platforms this must be the process main goroutine.
func (a *Application) Run() { a.Platform.Run(func() {}) }

// WindowCount reports the number of windows opened through this application
// that have not been closed yet. It is safe to call from any goroutine.
func (a *Application) WindowCount() int32 { return atomic.LoadInt32(&a.openWindows) }

// WindowHandle is a typed handle to an open window's root view. It is
// returned by [OpenView], [OpenWindow] and their *With variants.
type WindowHandle[T any] struct {
	// View is the entity holding the root state of type T.
	View app.Entity[T]
	// Window is the window the view is rendered into.
	Window *Window
}

// Update runs f with the root view state, the window and the entity context
// of the handle. f runs synchronously on the calling goroutine, which must be
// the UI goroutine, and a [app.Context.Notify] call inside f marks the view
// for redraw. It must not be called while the same entity is already being
// updated (for example from within a render function of that view).
func (h WindowHandle[T]) Update(a *Application, f func(view *T, w *Window, cx *app.Context[T])) {
	h.View.Update(a.App, func(v *T, cx *app.Context[T]) any {
		f(v, h.Window, cx)
		return nil
	})
}

// viewElement adapts a Render entity into an Element (upstream ViewElement).
type viewElement struct {
	view      app.EntityID
	child     Element
	lastChild Element
}

func (v *viewElement) ElementID() ElementId { return EntityElementId(v.view) }

func (v *viewElement) RequestLayout(w *Window) {
	if el := v.current(w); el != nil {
		node := w.requestLayoutElement(el)
		v.lastChild = el
		w.SetNodeFor(v, node)
	}
}

func (v *viewElement) current(w *Window) Element {
	if v.child != nil {
		return v.child
	}
	return nil
}

func (v *viewElement) Prepaint(w *Window, bounds geometry.Bounds[geometry.Pixels]) {
	if v.lastChild != nil {
		if node, ok := w.NodeOf(v.lastChild); ok {
			w.PrepaintElement(v.lastChild, node)
		}
	}
}

func (v *viewElement) Paint(w *Window) {
	if v.lastChild != nil {
		w.WithRenderedView(v.view, func() {
			v.lastChild.Paint(w)
		})
	}
}

// OpenWindow opens a window with an entity of type T as its root. buildRoot
// constructs the state once; if *T implements [Render], its Render method is
// used automatically each frame, otherwise the window draws nothing (the root
// element is nil). For an explicit typed render function, use [OpenView].
//
// It returns an error if buildRoot is nil or the platform cannot create the
// window (see [OpenViewWith]). The first frame has been drawn when it returns
// successfully.
func OpenWindow[T any](a *Application, opts platform.WindowOptions, buildRoot func(w *Window, cx *app.Context[T]) T) (WindowHandle[T], error) {
	return OpenWindowWith[T](a, opts, buildRoot, nil)
}

// OpenWindowWith is [OpenWindow] with a hook to customize the platform
// callbacks (e.g. install OnShouldClose for unsaved-change prompts)
// before the platform window is created. customize may be nil. It receives
// the default callbacks and may replace any of them; see [OpenViewWith] for
// what the defaults do.
func OpenWindowWith[T any](a *Application, opts platform.WindowOptions, buildRoot func(w *Window, cx *app.Context[T]) T, customize func(callbacks *platform.WindowCallbacks)) (WindowHandle[T], error) {
	return OpenViewWith(a, opts, buildRoot, func(v *T, w *Window, cx *app.Context[T]) Element {
		if r, ok := any(v).(Render); ok {
			return r.Render(w, cx.App)
		}
		return nil
	}, customize)
}

// OpenView opens a window, constructs its root state once, and draws its
// first frame. build creates the root state of type T (it runs once, inside
// an app update, with the new window and the entity context). render builds
// the element tree from that state each frame; it runs during the
// layout-request phase on the UI goroutine and should treat the state as
// read-only (mutate it through [app.Context.Updater] from handlers). A method
// expression such as (*Counter).Render can be passed as render.
//
// OpenView returns an error if build or render is nil, or if the platform
// fails to create the window. On success the returned [WindowHandle] gives
// access to the state entity and window. The first frame is drawn before
// OpenView returns, so render has already been called once. Call it on the
// UI goroutine (normally from main before [Application.Run]).
func OpenView[T any](a *Application, opts platform.WindowOptions, build func(*Window, *app.Context[T]) T, render func(*T, *Window, *app.Context[T]) Element) (WindowHandle[T], error) {
	return OpenViewWith(a, opts, build, render, nil)
}

// OpenViewWith is [OpenView] with a hook to customize platform callbacks
// before creating the window. customize may be nil. It is handed a pointer to
// the default [platform.WindowCallbacks]; leave callbacks you do not need to
// change intact, because the defaults route input to [Window.HandleInput],
// redraw on resize and quit the application when the last window closes.
// Replacing OnClose without calling the original skips that bookkeeping, and
// the default OnShouldClose allows every close request.
//
// Callbacks may fire during window creation, before the [Window] exists; the
// defaults ignore such early calls. The initial scale factor is applied
// explicitly after creation. If the platform rejects the window, its error is
// returned and no state is created.
func OpenViewWith[T any](a *Application, opts platform.WindowOptions, build func(*Window, *app.Context[T]) T, render func(*T, *Window, *app.Context[T]) Element, customize func(*platform.WindowCallbacks)) (WindowHandle[T], error) {
	if build == nil || render == nil {
		return WindowHandle[T]{}, errors.New("gpui: OpenView requires a state constructor and a render function")
	}
	// w is captured by the callbacks; it is created right after the platform
	// window returns. Platform backends may fire callbacks (including the
	// initial OnResize) during OpenWindow, before w exists; those early calls
	// are no-ops and the initial size/scale is applied explicitly below.
	var w *Window
	onResize := func(size geometry.Size[geometry.Pixels], scale float32) {
		if w != nil {
			w.SetScaleFactor(scale)
			w.Draw()
		}
	}
	callbacks := platform.WindowCallbacks{
		OnRequestFrame: func(o platform.RequestFrameOptions) {},
		OnInput: func(ev platform.Input) platform.DispatchEventResult {
			if w == nil {
				return platform.DispatchEventResult{Propagate: true}
			}
			return w.HandleInput(ev)
		},
		OnActiveStatus: func(bool) {},
		OnVisibility:   func(bool) {},
		OnHoverStatus:  func(bool) {},
		OnResize:       onResize,
		OnMoved:        func() {},
		OnShouldClose:  func() bool { return true },
		OnClose: func() {
			// Closing the last window quits the event loop (upstream
			// exits when the final window is released).
			if n := atomic.AddInt32(&a.openWindows, -1); n <= 0 {
				a.Platform.Quit()
			}
		},
		OnAppearance: func(gc.WindowAppearance) {},
	}
	if customize != nil {
		customize(&callbacks)
	}
	pw, err := a.Platform.OpenWindow(opts, callbacks)
	if err != nil {
		return WindowHandle[T]{}, err
	}
	atomic.AddInt32(&a.openWindows, 1)

	w = NewWindow(a.App, pw, a.Text)
	w.PushTextStyleBase(style.DefaultTextStyle())

	// Build the root view inside an update so EntityCreated observers fire.
	var view app.Entity[T]
	a.App.Update(func(cx *app.App) {
		view = app.NewEntity(cx, func(cx *app.Context[T]) T {
			return build(w, cx)
		})
	})

	// The root produces a fresh element tree each frame (upstream ViewElement).
	w.SetRootView(view.ID(), func(w *Window, cx *app.App) Element {
		var el Element
		view.Update(cx, func(v *T, vcx *app.Context[T]) any {
			el = render(v, w, vcx)
			return nil
		})
		return el
	})

	// Apply the platform window's initial scale factor explicitly: the
	// backend's initial OnResize fired before w existed (see above).
	w.SetScaleFactor(pw.ScaleFactor())

	// Initial draw (upstream draws once before returning).
	w.Draw()

	return WindowHandle[T]{View: view, Window: w}, nil
}

// SetRootView installs render as the window root, replacing any previous
// root. view is the entity id used as the root element's identity and as the
// current view while painting (pass the id of the entity whose state render
// reads, so entity notifications invalidate the window). render is called on
// every draw during layout request and may return nil to draw nothing. It is
// an alternative to [Window.SetRoot] for simple cases and is what [OpenView] uses
// internally. Call it on the UI goroutine, outside a draw.
func (w *Window) SetRootView(view app.EntityID, render func(w *Window, cx *app.App) Element) {
	w.root = &callbackView{view: view, render: render}
	w.rootView = view
}

// callbackView renders via a callback.
type callbackView struct {
	view   app.EntityID
	render func(w *Window, cx *app.App) Element
	child  Element
}

func (c *callbackView) ElementID() ElementId { return EntityElementId(c.view) }

func (c *callbackView) RequestLayout(w *Window) {
	c.child = nil
	c.child = c.render(w, w.App())
	if c.child != nil {
		node := w.requestLayoutElement(c.child)
		w.SetNodeFor(c, node)
	}
}

func (c *callbackView) Prepaint(w *Window, bounds geometry.Bounds[geometry.Pixels]) {
	if c.child != nil {
		if node, ok := w.NodeOf(c.child); ok {
			w.PrepaintElement(c.child, node)
		}
	}
}

func (c *callbackView) Paint(w *Window) {
	if c.child != nil {
		w.WithRenderedView(c.view, func() {
			// Paint through the window wrapper so the child's element id is
			// pushed on the id path: paint-phase state keys must match
			// prepaint-phase keys (PrepaintElement pushes the same id).
			w.PaintElement(c.child)
		})
	}
}
