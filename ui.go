// Package gpui provides the starting point for building desktop UIs in Go.
// Use [Run] for a single native window, or see the gpui subpackage for typed
// views, explicit platform selection, and multi-window applications.
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
	// Element is a node in the UI tree.
	Element = core.Element
	// Window is the window being rendered or receiving an event.
	Window = core.Window
	// ClickEvent describes pointer or keyboard activation of an element.
	ClickEvent = core.ClickEvent
	// WindowOptions configures a window. A Title is enough to get started.
	WindowOptions = platform.WindowOptions
)

// Run opens a native window and runs its event loop until the window closes.
// Call Run from main. render is called on the UI goroutine to build a fresh
// element tree; it may capture local state that its UI event handlers change.
// Errors from backend initialization or window creation are returned.
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

// Div creates a box with optional children.
func Div(children ...Element) *elements.Div { return elements.NewDiv().Children(children...) }

// Row creates a horizontal flex container with optional children.
func Row(children ...Element) *elements.Div { return Div(children...).Flex() }

// Column creates a vertical flex container with optional children.
func Column(children ...Element) *elements.Div { return Row(children...).FlexCol() }

// Text creates plain text that inherits the surrounding text style.
func Text(value string) *elements.Text { return elements.NewText(value) }

// Px is a logical pixel length for width, height, and similar layout setters.
func Px(value float32) geometry.Length { return geometry.LengthFromPixels(geometry.Px(value)) }

// RGB converts a hexadecimal 0xRRGGBB value into an opaque style color.
func RGB(hex uint32) color.Hsla { return color.RGB(hex).Hsla() }
