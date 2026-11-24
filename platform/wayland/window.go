//go:build linux || freebsd || openbsd || netbsd

package wayland

import (
	"encoding/binary"
	"net"
	"os"
	"syscall"
	"time"

	"github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/platform/internal/software"
	"github.com/FlameInTheDark/go-gpui/render"
)

// ---------------------------------------------------------------------------
// Window
// ---------------------------------------------------------------------------

// Window is an xdg toplevel presenting software-rasterized frames through a
// wl_shm buffer.
type Window struct {
	p          *Platform
	surface    uint32
	xdgSurface uint32
	toplevel   uint32
	size       geometry.Size[geometry.Pixels]
	scale      float32
	callbacks  platform.WindowCallbacks
	atlas      *render.Atlas
	title      string
	visible    bool
	active     bool
	closed     bool
	configured bool

	// shm buffer state
	poolFD     int
	poolMapped []byte
	poolID     uint32
	bufferID   uint32
	bufW       int
	bufH       int
	bufferBusy bool

	lastImage   *render.RenderImage
	framebuffer software.Framebuffer
	lastPointer geometry.Point[geometry.Pixels]
	clickCount  int
	lastClickAt int64

	input platform.InputHandler
}

// sendWithFD writes a request carrying a file descriptor.
func (c *conn) sendWithFD(msg []byte, fd int) error {
	_rights := syscall.UnixRights(fd)
	_, _, err := c.sock.WriteMsgUnix(msg, _rights, nil)
	return err
}

var _ = net.DialUnix

// ensureBuffer creates or resizes the shm pool and buffer.
func (w *Window) ensureBuffer(width, height int) error {
	c := w.p.conn
	if width <= 0 || height <= 0 {
		return nil
	}
	if w.poolFD >= 0 && w.bufW == width && w.bufH == height && w.bufferID != 0 {
		return nil
	}
	w.destroyBuffer()

	stride := width * 4
	size := stride * height

	// Create a temp file in the runtime dir (guaranteed shm-backed).
	f, err := os.CreateTemp(os.Getenv("XDG_RUNTIME_DIR"), "go-gpui-shm-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Truncate(int64(size)); err != nil {
		f.Close()
		return err
	}
	poolFD := int(f.Fd())

	// wl_shm.create_pool(new_id, fd, size)
	poolID := c.newObject()
	msg := newMessage(w.p.globals.shm, 0).u32(poolID).u32(0 /*fd placeholder*/).i32(int32(size))
	if err := c.sendWithFD(msg.finish(), poolFD); err != nil {
		f.Close()
		return err
	}

	// wl_shm_pool.create_buffer(new_id, offset, w, h, stride, format)
	bufID := c.newObject()
	if err := c.send(newMessage(poolID, 0).
		u32(bufID).u32(0).i32(int32(width)).i32(int32(height)).i32(int32(stride)).u32(0 /* ARGB8888 */).
		finish()); err != nil {
		f.Close()
		return err
	}

	mapped, err := syscall.Mmap(poolFD, 0, size, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		f.Close()
		return err
	}

	w.poolFD = poolFD
	w.poolMapped = mapped
	w.poolID = poolID
	w.bufferID = bufID
	w.bufW, w.bufH = width, height

	// wl_buffer.release -> reusable
	c.setHandler(bufID, func(ev *event) {
		if ev.opcode == 0 {
			w.bufferBusy = false
		}
	})
	// The fd is dup'ed by the compositor; ours can be closed after mmap.
	f.Close()
	return nil
}

func (w *Window) destroyBuffer() {
	c := w.p.conn
	if w.poolMapped != nil {
		syscall.Munmap(w.poolMapped)
		w.poolMapped = nil
	}
	if w.poolFD >= 0 {
		syscall.Close(w.poolFD)
		w.poolFD = -1
	}
	if w.bufferID != 0 {
		c.send(newMessage(w.bufferID, 0 /* destroy */).finish())
		c.clearHandler(w.bufferID)
		w.bufferID = 0
	}
	if w.poolID != 0 {
		c.send(newMessage(w.poolID, 1 /* destroy */).finish())
		w.poolID = 0
	}
	w.bufW, w.bufH = 0, 0
}

func (w *Window) resizeBuffer(width, height int) {
	_ = w.ensureBuffer(width, height)
}

// Draw rasterizes the scene and marks the frame dirty.
func (w *Window) Draw(scene *render.Scene) {
	if w.closed || !w.configured {
		return
	}
	dev := geometry.SizeToDevicePixels(w.size, w.scale)
	w.lastImage = w.framebuffer.Draw(scene, w.atlas, int(dev.Width.AsI32()), int(dev.Height.AsI32()))
	w.presentIfDirty()
}

// presentIfDirty attaches the latest frame to the surface and commits.
func (w *Window) presentIfDirty() {
	if w.lastImage == nil || w.closed || !w.configured {
		return
	}
	img := w.lastImage
	if err := w.ensureBuffer(img.Width, img.Height); err != nil {
		os.Stderr.WriteString("wayland present: " + err.Error() + "\n")
		return
	}
	if w.bufferBusy || w.poolMapped == nil {
		return // wait for wl_buffer.release
	}

	// ARGB8888: premultiplied, little-endian B,G,R,A byte order.
	src := img.Pixels
	dst := w.poolMapped
	n := len(dst)
	if len(src) < n {
		n = len(src)
	}
	for i := 0; i+4 <= n; i += 4 {
		r8, g8, b8, a8 := src[i], src[i+1], src[i+2], src[i+3]
		dst[i] = uint8(uint16(b8) * uint16(a8) / 255)
		dst[i+1] = uint8(uint16(g8) * uint16(a8) / 255)
		dst[i+2] = uint8(uint16(r8) * uint16(a8) / 255)
		dst[i+3] = a8
	}

	c := w.p.conn
	// wl_surface.attach(buffer, 0, 0)
	c.send(newMessage(w.surface, 1).u32(w.bufferID).i32(0).i32(0).finish())
	// wl_surface.damage_buffer(0, 0, w, h)
	c.send(newMessage(w.surface, 9).i32(0).i32(0).i32(int32(img.Width)).i32(int32(img.Height)).finish())
	// wl_surface.commit
	c.send(newMessage(w.surface, 6).finish())
	w.bufferBusy = true
}

// dispatchMotion reports pointer motion in logical pixels.
func (w *Window) dispatchMotion(x, y float32) {
	w.lastPointer = geometry.PointPixels(x, y)
	w.callbacks.OnInput(platform.MouseMoveEvent{Position: w.lastPointer})
}

// nextClickCount implements gpui's 1-2-3 click cadence.
func (w *Window) nextClickCount() int {
	now := time.Now().UnixMilli()
	if now-w.lastClickAt < 500 {
		w.clickCount++
	} else {
		w.clickCount = 1
	}
	if w.clickCount > 3 {
		w.clickCount = 1
	}
	w.lastClickAt = now
	return w.clickCount
}

// ---------------------------------------------------------------------------
// PlatformWindow interface
// ---------------------------------------------------------------------------

func (w *Window) SetTitle(title string) {
	w.title = title
	w.p.conn.send(newMessage(w.toplevel, 2 /* set_title */).str(title).finish())
}

func (w *Window) Title() string { return w.title }

func (w *Window) Resize(size geometry.Size[geometry.Pixels]) {
	w.size = size
	w.resizeBuffer(int(size.Width.AsF32()), int(size.Height.AsF32()))
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

func (w *Window) IsMaximized() bool { return false }

func (w *Window) ToggleFullscreen() {
	w.p.conn.send(newMessage(w.toplevel, 11 /* set_fullscreen */).u32(0).finish())
}

func (w *Window) IsFullscreen() bool { return false }

func (w *Window) Minimize() {
	w.p.conn.send(newMessage(w.toplevel, 13 /* set_minimized */).finish())
}

func (w *Window) Activate() { w.active = true }

func (w *Window) IsActive() bool { return w.active }

func (w *Window) IsVisible() bool { return w.visible }

func (w *Window) Show() { w.visible = true }

func (w *Window) Hide() { w.visible = false }

func (w *Window) Close() {
	if !w.closed {
		w.closed = true
		w.destroyBuffer()
		// xdg_toplevel.destroy, xdg_surface.destroy, wl_surface.destroy
		c := w.p.conn
		c.send(newMessage(w.toplevel, 0).finish())
		c.send(newMessage(w.xdgSurface, 0).finish())
		c.send(newMessage(w.surface, 0).finish())
		w.callbacks.OnClose()
		w.p.removeWindow(w)
	}
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
// Keycodes (Linux evdev, as delivered by wl_keyboard.key)
// ---------------------------------------------------------------------------

// keycodeToKey maps evdev keycodes to gpui key names.
var keycodeToKey = map[uint32]string{
	15: "tab", 16: "q", 17: "w", 18: "e", 19: "r", 20: "t", 21: "y", 22: "u",
	23: "i", 24: "o", 25: "p", 26: "[", 27: "]", 28: "enter", 29: "control",
	30: "a", 31: "s", 32: "d", 33: "f", 34: "g", 35: "h", 36: "j", 37: "k",
	38: "l", 39: ";", 40: "'", 41: "`", 42: "shift", 43: "\\", 44: "z",
	45: "x", 46: "c", 47: "v", 48: "b", 49: "n", 50: "m", 51: ",", 52: ".",
	53: "/", 54: "shift", 55: "numpad*", 56: "alt", 57: "space", 58: "capslock",
	59: "f1", 60: "f2", 61: "f3", 62: "f4", 63: "f5", 64: "f6", 65: "f7",
	66: "f8", 67: "f9", 68: "f10", 87: "f11", 88: "f12",
	96: "enter", 97: "control", 98: "numpad/", 99: "sysreq", 100: "alt",
	102: "home", 103: "up", 104: "pageup", 105: "left", 106: "right",
	107: "end", 108: "down", 109: "pagedown", 110: "insert", 111: "delete",
	125: "platform", 127: "platform",
}

// shiftedKey returns the shifted character of a single-char key.
func shiftedKey(key string) string {
	if len(key) != 1 {
		return key
	}
	switch key[0] {
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
	case ';':
		return ":"
	case '\'':
		return "\""
	case '`':
		return "~"
	case '\\':
		return "|"
	case ',':
		return "<"
	case '.':
		return ">"
	case '/':
		return "?"
	case '[':
		return "{"
	case ']':
		return "}"
	case '=':
		return "+"
	case '-':
		return "_"
	}
	return key
}

var _ = binary.LittleEndian

// SupportsCustomDecorations reports client-side decorations; the Wayland
// port keeps the compositor's frame.
func (w *Window) SupportsCustomDecorations() bool { return false }

// StartWindowDrag is a no-op (the compositor frame owns dragging).
func (w *Window) StartWindowDrag() {}

// ToggleMaximize is a no-op on the Wayland port.
func (w *Window) ToggleMaximize() {}
