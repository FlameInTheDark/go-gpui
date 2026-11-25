package gpui

import (
	"errors"

	"github.com/FlameInTheDark/go-gpui/app"
	"github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/elements"
	"github.com/FlameInTheDark/go-gpui/geometry"
	core "github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/platform/auto"
)

type (
	// Element is a node in the UI tree. It is an alias of
	// [github.com/FlameInTheDark/go-gpui/gpui.Element]: the interface with
	// ElementID, RequestLayout, Prepaint and Paint methods that every
	// element (such as the values returned by [Div] and [Text]) implements.
	// See the gpui subpackage for the frame lifecycle.
	Element = core.Element
	// Window is the window being rendered or receiving an event. It is an
	// alias of [github.com/FlameInTheDark/go-gpui/gpui.Window], which holds
	// the frame state, focus, hit testing, actions, prompts and menus. Use
	// [Window.Refresh] to request a redraw from an event handler. Window
	// methods may only be called on the UI goroutine.
	Window = core.Window
	// ClickEvent describes pointer or keyboard activation of an element. It
	// is an alias of [github.com/FlameInTheDark/go-gpui/gpui.ClickEvent] and
	// is passed to handlers registered with OnClick on elements.
	ClickEvent = core.ClickEvent
	// WindowOptions configures a window (title, size, and similar). It is an
	// alias of [github.com/FlameInTheDark/go-gpui/platform.WindowOptions]; a
	// Title is enough to get started and zero values select platform
	// defaults.
	WindowOptions = platform.WindowOptions
)

// Run opens a native window and runs its event loop until the window closes.
//
// Run selects a backend with [github.com/FlameInTheDark/go-gpui/platform/auto],
// opens one window described by opts and blocks until the last window closes.
// Call Run from the main goroutine, because several platforms require the
// event loop to run on the process main thread.
//
// render is called on the UI goroutine each time the window draws a frame,
// including once before the window is shown, to build a fresh element tree.
// It may capture local state that UI event handlers change; handlers should
// then call [Window.Refresh]. Treat render as read-only and fast: it runs
// during the layout-request phase of the frame, so it must not call
// [Window.Draw].
//
// Run returns an error if render is nil, if backend initialization fails or
// if the window cannot be created. It returns nil after a normal close. For
// typed views, multiple windows or tests with a headless backend use the gpui
// subpackage ([github.com/FlameInTheDark/go-gpui/gpui.OpenView]).
func Run(opts WindowOptions, render func(*Window) Element) error {
	if render == nil {
		return errors.New("gpui: Run requires a render function")
	}
	p, err := auto.New()
	if err != nil {
		return err
	}
	return run(p, opts, render)
}

func run(p platform.Platform, opts WindowOptions, render func(*Window) Element) error {
	a := core.NewApplication(p)
	_, err := core.OpenView(a, opts,
		func(*Window, *app.Context[struct{}]) struct{} { return struct{}{} },
		func(_ *struct{}, w *Window, _ *app.Context[struct{}]) Element { return render(w) })
	if err != nil {
		return err
	}
	a.Run()
	return nil
}

// Div creates a box with the given children, in order. With no children it
// creates an empty box that can still be sized, styled and made interactive
// through the builder methods of [elements.Div]. The result is a new element
// on every call; build it inside the render function, not once up front.
func Div(children ...Element) *elements.Div { return elements.NewDiv().Children(children...) }

// Row creates a horizontal flex container with the given children. It is
// [Div] with display:flex and the default (row) flex direction.
func Row(children ...Element) *elements.Div { return Div(children...).Flex() }

// Column creates a vertical flex container with the given children. It is
// [Row] with the flex direction switched to column.
func Column(children ...Element) *elements.Div { return Row(children...).FlexCol() }

// Text creates a text element showing value. It inherits font, size and color
// from the surrounding text style unless overridden on the returned element.
// value is shown verbatim; there is no markup.
func Text(value string) *elements.Text { return elements.NewText(value) }

// Px returns a length of value logical pixels, for width, height and similar
// layout setters. Logical pixels are scaled by the window's HiDPI factor and
// zoom before reaching the screen.
func Px(value float32) geometry.Length { return geometry.LengthFromPixels(geometry.Px(value)) }

// RGB converts a hexadecimal 0xRRGGBB value into an opaque style color. The
// top byte of hex is ignored; use the color package for translucent colors.
func RGB(hex uint32) color.Hsla { return color.RGB(hex).Hsla() }
