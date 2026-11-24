package x11

import (
	"encoding/binary"
	"os"

	"github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/keymap"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/platform/internal/software"
	"github.com/FlameInTheDark/go-gpui/render"
)

// Window is an X11 platform window presenting scenes via PutImage.
type Window struct {
	platform    *Platform
	xid         uint32
	gc          uint32
	size        geometry.Size[geometry.Pixels]
	scale       float32
	callbacks   platform.WindowCallbacks
	atlas       *render.Atlas
	title       string
	visible     bool
	active      bool
	closed      bool
	dirty       bool
	lastImage   *render.RenderImage
	framebuffer software.Framebuffer
	bgra        []byte
	input       platform.InputHandler
}

func (w *Window) SetTitle(title string) {
	w.title = title
	w.platform.conn.setPropertyString(w.xid, "WM_NAME", "STRING", title)
	w.platform.conn.setPropertyString(w.xid, "_NET_WM_NAME", "UTF8_STRING", title)
}

func (w *Window) Title() string { return w.title }

func (w *Window) Resize(size geometry.Size[geometry.Pixels]) {
	w.size = size
}

func (w *Window) Bounds() geometry.Bounds[geometry.Pixels] {
	return geometry.BoundsPixels(0, 0, w.size.Width.AsF32(), w.size.Height.AsF32())
}

func (w *Window) ContentSize() geometry.Size[geometry.Pixels] { return w.size }

func (w *Window) ScaleFactor() float32 { return w.scale }

func (w *Window) Appearance() color.WindowAppearance { return color.WindowAppearanceLight }

func (w *Window) BackgroundAppearance() platform.WindowBackgroundAppearance {
	return platform.BackgroundOpaque
}

func (w *Window) IsMaximized() bool  { return false }
func (w *Window) ToggleFullscreen()  {}
func (w *Window) IsFullscreen() bool { return false }
func (w *Window) Minimize()          {}
func (w *Window) Activate()          { w.active = true }
func (w *Window) IsActive() bool     { return w.active }
func (w *Window) IsVisible() bool    { return w.visible }
func (w *Window) Show()              { w.visible = true }
func (w *Window) Hide()              { w.visible = false }

func (w *Window) Close() {
	if !w.closed {
		w.closed = true
		w.callbacks.OnClose()
		_ = w.platform.conn.DestroyWindow(w.xid)
	}
}

// Draw rasterizes and presents the scene.
func (w *Window) Draw(scene *render.Scene) {
	dev := geometry.SizeToDevicePixels(w.size, w.scale)
	img := w.framebuffer.Draw(scene, w.atlas, int(dev.Width.AsI32()), int(dev.Height.AsI32()))
	w.lastImage = img
	// Expose events can re-upload this frame without repacking its pixels.
	if cap(w.bgra) < len(img.Pixels) {
		w.bgra = make([]byte, len(img.Pixels))
	} else {
		w.bgra = w.bgra[:len(img.Pixels)]
	}
	for i := 0; i+4 <= len(img.Pixels); i += 4 {
		v := binary.LittleEndian.Uint32(img.Pixels[i:])
		binary.LittleEndian.PutUint32(w.bgra[i:], v<<16|v&0xFF00FF00|(v>>16)&0x000000FF)
	}
	w.dirty = true
	w.present()
}

// present uploads the frame via PutImage (X expects BGRA byte order).
func (w *Window) present() {
	if w.lastImage == nil || w.closed {
		return
	}
	img := w.lastImage
	if err := w.platform.conn.PutImageBGRX(w.xid, w.gc, img.Width, img.Height, w.bgra); err != nil {
		os.Stderr.WriteString("x11 present: " + err.Error() + "\n")
	}
	w.dirty = false
}

func (w *Window) SpriteAtlas() *render.Atlas { return w.atlas }

func (w *Window) RequestFrame(opts platform.RequestFrameOptions) {
	w.callbacks.OnRequestFrame(opts)
}

func (w *Window) SetInputHandler(handler platform.InputHandler) { w.input = handler }

func (w *Window) TakeInputHandler() platform.InputHandler { return w.input }

func (w *Window) InvalidateCharacterCoordinates(bounds geometry.Bounds[geometry.Pixels]) {}

func (w *Window) WriteClipboard(item platform.ClipboardItem) {}

func (w *Window) ReadClipboard() (platform.ClipboardItem, bool) {
	return platform.ClipboardItem{}, false
}

func (w *Window) ShowWindowMenu(position geometry.Point[geometry.Pixels]) {}

// ---------------------------------------------------------------------------
// Input translation
// ---------------------------------------------------------------------------

func buttonOf(detail byte) (platform.MouseButton, bool) {
	switch detail {
	case 1:
		return platform.ButtonLeft, true
	case 2:
		return platform.ButtonMiddle, true
	case 3:
		return platform.ButtonRight, true
	case 8:
		return platform.ButtonNavigate, true
	case 9:
		return platform.ButtonNavigate, true
	}
	return 0, false
}

func (w *Window) dispatchButton(press bool, detail byte, x, y int, state byte) {
	// Scroll buttons 4/5.
	if detail == 4 || detail == 5 {
		if !press {
			return
		}
		delta := float32(1.0)
		if detail == 5 {
			delta = -1.0
		}
		w.callbacks.OnInput(platform.ScrollEvent{
			Position: geometry.PointPixels(float32(x), float32(y)),
			Delta:    platform.ScrollDeltaLines(0, delta*3),
			Source:   platform.ScrollSourcePointer,
		})
		return
	}
	button, ok := buttonOf(detail)
	if !ok {
		return
	}
	mods := modifiersOf(state)
	pos := geometry.PointPixels(float32(x), float32(y))
	if press {
		w.callbacks.OnInput(platform.MouseDownEvent{
			Button: button, Position: pos, Modifiers: mods, ClickCount: 1,
		})
	} else {
		w.callbacks.OnInput(platform.MouseUpEvent{
			Button: button, Position: pos, Modifiers: mods, ClickCount: 1,
		})
	}
}

func (w *Window) dispatchMotion(x, y int, state byte) {
	w.callbacks.OnInput(platform.MouseMoveEvent{
		Position:  geometry.PointPixels(float32(x), float32(y)),
		Modifiers: modifiersOf(state),
	})
}

func modifiersOf(state byte) keymap.Modifiers {
	return keymap.Modifiers{
		Shift:    state&0x01 != 0,
		Control:  state&0x04 != 0,
		Alt:      state&0x08 != 0,
		Platform: state&0x40 != 0, // Mod4
	}
}

// keycodeToKey maps core keycodes to gpui key names (Latin-1 subset).
var keycodeToKey = map[byte]string{
	24: "q", 25: "w", 26: "e", 27: "r", 28: "t", 29: "y", 30: "u", 31: "i",
	32: "o", 33: "p", 34: "[", 35: "]", 36: "enter", 38: "a", 39: "s",
	40: "d", 41: "f", 42: "g", 43: "h", 44: "j", 45: "k", 46: "l", 47: ";",
	48: "'", 49: "`", 51: "\\", 52: "z", 53: "x", 54: "c", 55: "v", 56: "b",
	57: "n", 58: "m", 59: ",", 60: ".", 61: "/", 65: "space", 23: "tab",
	22: "backspace", 9: "escape", 111: "up", 116: "down", 113: "left", 114: "right",
	119: "delete", 110: "home", 115: "end", 112: "pageup", 117: "pagedown",
	66: "enter", 50: "shift", 62: "shift", 37: "control", 64: "alt", 133: "platform",
	67: "f1", 68: "f2", 69: "f3", 70: "f4", 71: "f5", 72: "f6", 73: "f7",
	74: "f8", 75: "f9", 76: "f10", 95: "f11", 96: "f12",
}

func (w *Window) dispatchKey(press bool, detail byte, state byte) {
	key, ok := keycodeToKey[detail]
	if !ok {
		return
	}
	mods := modifiersOf(state)
	stroke := keymap.Keystroke{Key: key, Modifiers: mods}
	// Printable character.
	if len(key) == 1 && !mods.Control && !mods.Alt && !mods.Platform {
		c := key
		if mods.Shift {
			c = shiftChar(c)
		}
		stroke.KeyChar = c
	}
	if press {
		w.callbacks.OnInput(platform.KeyDownEvent{Keystroke: stroke})
	} else {
		w.callbacks.OnInput(platform.KeyUpEvent{Keystroke: stroke})
	}
}

func shiftChar(c string) string {
	if len(c) != 1 {
		return c
	}
	switch c[0] {
	case 'a':
		return "A"
	case 'b':
		return "B"
	case 'c':
		return "C"
	case 'd':
		return "D"
	case 'e':
		return "E"
	case 'f':
		return "F"
	case 'g':
		return "G"
	case 'h':
		return "H"
	case 'i':
		return "I"
	case 'j':
		return "J"
	case 'k':
		return "K"
	case 'l':
		return "L"
	case 'm':
		return "M"
	case 'n':
		return "N"
	case 'o':
		return "O"
	case 'p':
		return "P"
	case 'q':
		return "Q"
	case 'r':
		return "R"
	case 's':
		return "S"
	case 't':
		return "T"
	case 'u':
		return "U"
	case 'v':
		return "V"
	case 'w':
		return "W"
	case 'x':
		return "X"
	case 'y':
		return "Y"
	case 'z':
		return "Z"
	case '1':
		return "!"
	case '2':
		return "@"
	case '3':
		return "#"
	case '4':
		return "$"
	case '5':
		return "%"
	case '6':
		return "^"
	case '7':
		return "&"
	case '8':
		return "*"
	case '9':
		return "("
	case '0':
		return ")"
	}
	return c
}

var _ = binary.BigEndian

// SupportsCustomDecorations reports client-side decorations; X11 keeps
// the WM's server-side frame.
func (w *Window) SupportsCustomDecorations() bool { return false }

// StartWindowDrag is a no-op (the WM frame owns dragging).
func (w *Window) StartWindowDrag() {}

// ToggleMaximize is a no-op on the X11 port.
func (w *Window) ToggleMaximize() {}
