//go:build linux || freebsd || openbsd || netbsd

// Package wayland implements the go-gpui platform as a pure-Go Wayland
// client: it speaks the Wayland wire protocol directly over a unix socket
// (wl_display/wl_registry/wl_compositor/wl_shm/wl_seat/xdg_wm_base) with no
// libwayland and no cgo. Frames are software-rasterized by the render
// package into a shared-memory buffer attached to an xdg toplevel surface.
package wayland

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/keymap"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/render"
	"github.com/FlameInTheDark/go-gpui/text"
	"image"
)

// ---------------------------------------------------------------------------
// Connection and object bookkeeping
// ---------------------------------------------------------------------------

const (
	idDisplay  = 1 // wl_display
	idRegistry = 2 // wl_registry
)

// conn is the raw Wayland connection.
type conn struct {
	sock      *net.UnixConn
	pending   []byte
	fds       []int // received file descriptors, in arrival order
	mu        sync.Mutex
	nextID    uint32
	callbacks map[uint32]chan struct{}
	// handlers routes object IDs to event handlers.
	handlers map[uint32]func(*event)
}

func (c *conn) addCallback(id uint32) {
	c.mu.Lock()
	c.callbacks[id] = make(chan struct{}, 1)
	c.mu.Unlock()
}

func (c *conn) callbackDone(id uint32) bool {
	c.mu.Lock()
	ch := c.callbacks[id]
	c.mu.Unlock()
	if ch == nil {
		return true
	}
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func (c *conn) removeCallback(id uint32) {
	c.mu.Lock()
	delete(c.callbacks, id)
	c.mu.Unlock()
}

// newObject allocates the next client object ID.
func (c *conn) newObject() uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.nextID < 2 {
		c.nextID = 2
	}
	c.nextID++
	return c.nextID
}

// setHandler registers an event handler for an object ID.
func (c *conn) setHandler(id uint32, fn func(*event)) {
	c.mu.Lock()
	c.handlers[id] = fn
	c.mu.Unlock()
}

// clearHandler removes an object's handler.
func (c *conn) clearHandler(id uint32) {
	c.mu.Lock()
	delete(c.handlers, id)
	c.mu.Unlock()
}

// fill reads more bytes (and any attached FDs) from the socket.
func (c *conn) fill() error {
	buf := make([]byte, 8192)
	oob := make([]byte, 128) // room for several SCM_RIGHTS fds
	n, oobn, _, _, err := c.sock.ReadMsgUnix(buf, oob)
	if n > 0 {
		c.pending = append(c.pending, buf[:n]...)
	}
	if oobn > 0 {
		cms, ok := syscall.ParseSocketControlMessage(oob[:oobn])
		if ok == nil {
			for _, cm := range cms {
				if cm.Header.Level == syscall.SOL_SOCKET && cm.Header.Type == syscall.SCM_RIGHTS {
					fds, ferr := syscall.ParseUnixRights(&cm)
					if ferr == nil {
						c.fds = append(c.fds, fds...)
					}
				}
			}
		}
	}
	return err
}

// popFD takes the oldest received file descriptor.
func (c *conn) popFD() int {
	if len(c.fds) == 0 {
		return -1
	}
	fd := c.fds[0]
	c.fds = c.fds[1:]
	return fd
}

// drainFDs closes any unconsumed descriptors.
func (c *conn) drainFDs() {
	for _, fd := range c.fds {
		syscall.Close(fd)
	}
	c.fds = nil
}

// connectWayland opens the socket named by $WAYLAND_DISPLAY.
func connectWayland() (*conn, error) {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		return nil, errors.New("wayland: $XDG_RUNTIME_DIR is not set")
	}
	display := os.Getenv("WAYLAND_DISPLAY")
	if display == "" {
		display = "wayland-0"
	}
	path := display
	if !filepath.IsAbs(path) {
		path = filepath.Join(runtimeDir, display)
	}
	sock, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("wayland: cannot connect to %s: %w", path, err)
	}
	c := &conn{
		sock:      sock,
		callbacks: map[uint32]chan struct{}{},
		handlers:  map[uint32]func(*event){},
	}
	return c, nil
}

// dispatchEvent routes an event to its object handler.
func (c *conn) dispatchEvent(ev *event) {
	switch {
	case ev.objectID == idDisplay:
		switch ev.opcode {
		case 0: // delete_id
			id := ev.u32(0)
			c.clearHandler(id)
		case 1: // error
			code := ev.i32(1)
			msg := ev.strArg(2)
			fmt.Fprintf(os.Stderr, "wayland: protocol error on object %d: code %d: %s\n",
				ev.u32(0), code, msg)
		}
		return
	default:
		c.mu.Lock()
		handler := c.handlers[ev.objectID]
		c.mu.Unlock()
		if handler != nil {
			handler(ev)
			return
		}
		// wl_callback.done for sync roundtrips arrives on unregistered
		// callback IDs; the sync goroutine polls callbackDone.
		c.mu.Lock()
		ch := c.callbacks[ev.objectID]
		c.mu.Unlock()
		if ch != nil && ev.opcode == 0 {
			select {
			case ch <- struct{}{}:
			default:
			}
		}
	}
	c.drainFDs()
}

// ---------------------------------------------------------------------------
// Globals
// ---------------------------------------------------------------------------

// globals are the bound singletons this client needs.
type globals struct {
	compositor uint32
	shm        uint32
	seat       uint32
	wmBase     uint32
	seatCaps   uint32
}

// bindGlobals performs the startup registry dance: get_registry, two
// roundtrips to collect global events, then bind the required interfaces.
func bindGlobals(c *conn) (*globals, error) {
	g := &globals{}
	registrySeen := make(chan struct{}, 1)
	globalsDone := make(chan struct{}, 1)

	c.setHandler(idRegistry, func(ev *event) {
		switch ev.opcode {
		case 0: // global(name, interface, version)
			name := ev.u32(0)
			ifaceName := ev.strArg(1)
			version := ev.u32AfterString(1)
			switch ifaceName {
			case "wl_compositor":
				if version > 4 {
					version = 4
				}
				g.compositor = c.bind(name, "wl_compositor", version)
			case "wl_shm":
				if version > 1 {
					version = 1
				}
				g.shm = c.bind(name, "wl_shm", version)
			case "wl_seat":
				if version > 5 {
					version = 5
				}
				g.seat = c.bind(name, "wl_seat", version)
			case "xdg_wm_base":
				if version > 1 {
					version = 1
				}
				g.wmBase = c.bind(name, "xdg_wm_base", version)
			}
			select {
			case registrySeen <- struct{}{}:
			default:
			}
		case 1: // global_remove
		}
	})

	// wl_display.get_registry(registry)
	if err := c.send(newMessage(idDisplay, 2).u32(idRegistry).finish()); err != nil {
		return nil, err
	}
	if err := c.roundtrip(); err != nil {
		return nil, err
	}
	// Wait briefly for at least one global (roundtrip above should have
	// delivered them all).
	select {
	case <-registrySeen:
	default:
	}
	// Bind may create new objects; do one more roundtrip to settle.
	if err := c.roundtrip(); err != nil {
		return nil, err
	}
	close(globalsDone)

	if g.compositor == 0 || g.shm == 0 || g.wmBase == 0 {
		return nil, errors.New("wayland: missing required globals (compositor/shm/xdg_wm_base)")
	}
	return g, nil
}

// bind sends wl_registry.bind and returns the new proxy ID.
func (c *conn) bind(name uint32, iface string, version uint32) uint32 {
	id := c.newObject()
	msg := newMessage(idRegistry, 0 /* bind */).
		u32(name).
		str(iface).
		u32(version).
		u32(id)
	if err := c.send(msg.finish()); err != nil {
		return 0
	}
	return id
}

// ---------------------------------------------------------------------------
// Platform
// ---------------------------------------------------------------------------

// Platform implements platform.Platform as a Wayland client.
type Platform struct {
	conn    *conn
	globals *globals
	mu      sync.Mutex
	windows map[uint32]*Window
	quit    bool
	textSys *text.System
	// Workers enqueue callbacks; only the Run loop applies UI changes.
	dispatchQueue []func()
	seat          struct {
		pointer  uint32
		keyboard uint32
	}
	// pointerFocusSurface / keyboardFocusSurface track seat focus.
	pointerFocusSurface  uint32
	keyboardFocusSurface uint32
	// depressedMods tracks the latest wl_keyboard.modifiers mask
	// (standard xkb group-1 positions: Shift 0, Caps 1, Ctrl 2, Alt 3,
	// Logo 6).
	depressedMods uint32
}

// NewPlatform connects to the compositor and binds globals.
func NewPlatform() (*Platform, error) {
	c, err := connectWayland()
	if err != nil {
		return nil, err
	}
	g, err := bindGlobals(c)
	if err != nil {
		return nil, err
	}
	p := &Platform{conn: c, globals: g, windows: map[uint32]*Window{}}
	p.bindSeat()
	return p, nil
}

// bindSeat reads seat capabilities and creates pointer/keyboard proxies.
func (p *Platform) bindSeat() {
	if p.globals.seat == 0 {
		return
	}
	p.conn.setHandler(p.globals.seat, func(ev *event) {
		if ev.opcode == 0 { // capabilities
			caps := ev.u32(0)
			p.mu.Lock()
			p.globals.seatCaps = caps
			p.mu.Unlock()
			if caps&1 != 0 && p.seat.pointer == 0 { // pointer
				ptr := p.conn.newObject()
				p.conn.send(newMessage(p.globals.seat, 0 /* get_pointer */).u32(ptr).finish())
				p.seat.pointer = ptr
				p.conn.setHandler(ptr, p.pointerEvent)
			}
			if caps&2 != 0 && p.seat.keyboard == 0 { // keyboard
				kb := p.conn.newObject()
				p.conn.send(newMessage(p.globals.seat, 1 /* get_keyboard */).u32(kb).finish())
				p.seat.keyboard = kb
				p.conn.setHandler(kb, p.keyboardEvent)
			}
		}
	})
}

// Run processes Wayland events until Quit.
func (p *Platform) Run(onFinish func()) {
	onFinish()
	for !p.quitting() {
		p.conn.sock.SetReadDeadline(time.Now().Add(16 * time.Millisecond))
		for {
			ev, err := p.conn.readEvent()
			if err != nil {
				break
			}
			p.conn.dispatchEvent(ev)
		}
		p.conn.drainFDs()
		p.runDispatch()
		p.presentDirty()
	}
}

func (p *Platform) quitting() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.quit
}

// Quit stops the event loop.
func (p *Platform) Quit() {
	p.mu.Lock()
	p.quit = true
	p.mu.Unlock()
}

// Displays returns a single logical display.
func (p *Platform) Displays() []platform.Display {
	return []platform.Display{{
		ID:          1,
		Bounds:      geometry.BoundsPixels(0, 0, 1920, 1080),
		ScaleFactor: 1,
	}}
}

// PrimaryDisplay returns the logical display.
func (p *Platform) PrimaryDisplay() (platform.Display, bool) { return p.Displays()[0], true }

// Appearance reports light (no xdg-decoration dark detection).
func (p *Platform) Appearance() color.WindowAppearance { return color.WindowAppearanceLight }

// ReadClipboard is not supported by the core protocol (needs wp-primary/
// data-control extensions).
func (p *Platform) ReadClipboard() (platform.ClipboardItem, bool) {
	return platform.ClipboardItem{}, false
}

// WriteClipboard is not supported by the core protocol.
func (p *Platform) WriteClipboard(item platform.ClipboardItem) {}

// DispatchOnMainThread queues f for the Run loop (thread-safe).
func (p *Platform) DispatchOnMainThread(f func()) {
	p.mu.Lock()
	p.dispatchQueue = append(p.dispatchQueue, f)
	p.mu.Unlock()
}

func (p *Platform) runDispatch() {
	p.mu.Lock()
	queue := p.dispatchQueue
	p.dispatchQueue = nil
	p.mu.Unlock()
	for _, f := range queue {
		f()
	}
}

// Now returns the wall clock.
func (p *Platform) Now() time.Time { return time.Now() }

// TextSystem builds the system text system.
func (p *Platform) TextSystem() platform.TextSystem {
	if p.textSys == nil {
		p.textSys = text.NewSystemWithSystemFonts()
	}
	return p.textSys
}

// OpenWindow creates an xdg toplevel surface.
func (p *Platform) OpenWindow(options platform.WindowOptions, callbacks platform.WindowCallbacks) (platform.PlatformWindow, error) {
	c := p.conn
	g := p.globals

	surface := c.newObject()
	if err := c.send(newMessage(g.compositor, 0 /* create_surface */).u32(surface).finish()); err != nil {
		return nil, err
	}

	xdgSurface := c.newObject()
	if err := c.send(newMessage(g.wmBase, 2 /* get_xdg_surface */).u32(xdgSurface).u32(surface).finish()); err != nil {
		return nil, err
	}

	toplevel := c.newObject()
	if err := c.send(newMessage(xdgSurface, 1 /* get_toplevel */).u32(toplevel).finish()); err != nil {
		return nil, err
	}

	size := geometry.SizePixels(800, 600)
	if options.WindowBounds != nil {
		size = options.WindowBounds.Bounds.Size
	}

	w := &Window{
		p:          p,
		surface:    surface,
		xdgSurface: xdgSurface,
		toplevel:   toplevel,
		size:       size,
		scale:      1,
		callbacks:  callbacks,
		atlas:      render.NewAtlas(),
		title:      options.Title,
		visible:    true,
		active:     true,
	}

	// xdg_wm_base.ping must be answered with pong.
	c.setHandler(g.wmBase, func(ev *event) {
		if ev.opcode == 0 { // ping
			serial := ev.u32(0)
			c.send(newMessage(g.wmBase, 3 /* pong */).u32(serial).finish())
		}
	})

	// xdg_surface.configure -> ack_configure; first configure unlocks commits.
	c.setHandler(xdgSurface, func(ev *event) {
		if ev.opcode == 0 { // configure
			serial := ev.u32(0)
			c.send(newMessage(xdgSurface, 4 /* ack_configure */).u32(serial).finish())
			p.mu.Lock()
			configured := w.configured
			w.configured = true
			p.mu.Unlock()
			if !configured {
				callbacks.OnResize(w.size, w.scale)
				callbacks.OnActiveStatus(true)
				callbacks.OnVisibility(true)
			}
		}
	})

	// xdg_toplevel events.
	c.setHandler(toplevel, func(ev *event) {
		switch ev.opcode {
		case 0: // configure(width, height, states)
			width := ev.i32(0)
			height := ev.i32(1)
			if width > 0 && height > 0 {
				newSize := geometry.SizePixels(float32(width), float32(height))
				if newSize != w.size {
					w.size = newSize
					w.resizeBuffer(int(width), int(height))
					callbacks.OnResize(newSize, w.scale)
				}
			}
		case 1: // close
			callbacks.OnClose()
			p.removeWindow(w)
		}
	})

	// wl_buffer.release marks the buffer reusable.
	c.setHandler(surface, func(ev *event) {
		// wl_surface has no events below v6 except enter/leave; ignore.
		_ = ev
	})

	p.mu.Lock()
	p.windows[surface] = w
	p.mu.Unlock()

	if options.Title != "" {
		w.SetTitle(options.Title)
	}

	// The initial commit with no buffer starts the configure dance.
	if err := c.send(newMessage(surface, 6 /* commit */).finish()); err != nil {
		return nil, err
	}

	// Wait for the first configure so the first frame is not dropped.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		configured := w.configured
		p.mu.Unlock()
		if configured {
			break
		}
		c.sock.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		if ev, err := c.readEvent(); err == nil {
			c.dispatchEvent(ev)
		}
	}
	return w, nil
}

func (p *Platform) removeWindow(w *Window) {
	p.mu.Lock()
	delete(p.windows, w.surface)
	p.mu.Unlock()
	p.conn.clearHandler(w.surface)
	p.conn.clearHandler(w.xdgSurface)
	p.conn.clearHandler(w.toplevel)
}

func (p *Platform) presentDirty() {
	p.mu.Lock()
	ws := make([]*Window, 0, len(p.windows))
	for _, w := range p.windows {
		ws = append(ws, w)
	}
	p.mu.Unlock()
	for _, w := range ws {
		w.presentIfDirty()
	}
}

// windowAt finds the window that should receive pointer/keyboard events.
func (p *Platform) windowForSurface(id uint32) *Window {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.windows[id]
}

// ---------------------------------------------------------------------------
// Pointer and keyboard events
// ---------------------------------------------------------------------------

func (p *Platform) pointerEvent(ev *event) {
	var w *Window
	switch ev.opcode {
	case 0: // enter(serial, surface, sx, sy)
		w = p.windowForSurface(ev.u32(1))
		p.mu.Lock()
		p.pointerFocusSurface = ev.u32(1)
		p.mu.Unlock()
		if w != nil {
			w.dispatchMotion(ev.fixed(2), ev.fixed(3))
		}
	case 2: // motion(time, sx, sy)
		p.mu.Lock()
		focus := p.pointerFocusSurface
		p.mu.Unlock()
		if w := p.windowForSurface(focus); w != nil {
			w.dispatchMotion(ev.fixed(1), ev.fixed(2))
		}
	case 3: // button(serial, time, button, state)
		p.mu.Lock()
		focus := p.pointerFocusSurface
		p.mu.Unlock()
		w = p.windowForSurface(focus)
		if w == nil {
			return
		}
		button, ok := pointerButton(ev.u32(2))
		if !ok {
			return
		}
		press := ev.u32(3) == 1
		mods := p.currentModifiers()
		if press {
			w.clickCount = w.nextClickCount()
			w.callbacks.OnInput(platform.MouseDownEvent{
				Button: button, Position: w.lastPointer, Modifiers: mods, ClickCount: w.clickCount,
			})
		} else {
			w.callbacks.OnInput(platform.MouseUpEvent{
				Button: button, Position: w.lastPointer, Modifiers: mods, ClickCount: w.clickCount,
			})
		}
	case 4: // axis(time, axis, value) — discrete wheel: 10 units per notch
		p.mu.Lock()
		focus := p.pointerFocusSurface
		p.mu.Unlock()
		w = p.windowForSurface(focus)
		if w == nil {
			return
		}
		axis := ev.u32(1)
		value := ev.fixed(2)
		notches := value / 10
		var dx, dy float32
		if axis == 0 { // vertical
			dy = -notches * 3
		} else { // horizontal
			dx = notches * 3
		}
		w.callbacks.OnInput(platform.ScrollEvent{
			Position:  w.lastPointer,
			Delta:     platform.ScrollDeltaLines(dx, dy),
			Modifiers: p.currentModifiers(),
			Source:    platform.ScrollSourcePointer,
		})
	}
}

func pointerButton(code uint32) (platform.MouseButton, bool) {
	switch code {
	case 0x110:
		return platform.ButtonLeft, true
	case 0x111:
		return platform.ButtonRight, true
	case 0x112:
		return platform.ButtonMiddle, true
	case 0x113:
		return platform.ButtonNavigate, true
	case 0x114:
		return platform.ButtonNavigate, true
	}
	return 0, false
}

// currentModifiers maps the latest xkb modifier mask (group 1, standard
// positions) to gpui modifiers.
func (p *Platform) currentModifiers() keymap.Modifiers {
	m := p.depressedMods
	return keymap.Modifiers{
		Shift:    m&(1<<0) != 0,
		Control:  m&(1<<2) != 0,
		Alt:      m&(1<<3) != 0,
		Platform: m&(1<<6) != 0,
	}
}

func (p *Platform) keyboardEvent(ev *event) {
	switch ev.opcode {
	case 0: // keymap(format, fd, size)
		// Full xkbcommon keymap parsing is out of scope for the pure-Go
		// port; the fd is drained and closed, and key events use the
		// built-in evdev mapping (see keycodeToKey).
		fd := p.conn.popFD()
		if fd >= 0 {
			syscall.Close(fd)
		}
	case 3: // key(time, key, state) — key is an evdev keycode
		p.mu.Lock()
		focus := p.keyboardFocusSurface
		p.mu.Unlock()
		w := p.windowForSurface(focus)
		if w == nil {
			return
		}
		key, ok := keycodeToKey[ev.u32(1)]
		if !ok {
			return
		}
		mods := p.currentModifiers()
		stroke := keymap.Keystroke{Key: key, Modifiers: mods}
		if len(key) == 1 && !mods.Control && !mods.Alt && !mods.Platform {
			if mods.Shift {
				stroke.KeyChar = shiftedKey(key)
			} else {
				stroke.KeyChar = key
			}
		}
		if ev.u32(2) == 1 {
			w.callbacks.OnInput(platform.KeyDownEvent{Keystroke: stroke})
		} else {
			w.callbacks.OnInput(platform.KeyUpEvent{Keystroke: stroke})
		}
	case 4: // modifiers(serial, depressed, latched, locked, group)
		p.depressedMods = ev.u32(1)
	case 1: // enter
		p.mu.Lock()
		p.keyboardFocusSurface = ev.u32(1)
		p.mu.Unlock()
	case 2: // leave
	}
}

var _ = binary.LittleEndian
var _ = unsafe.Sizeof(0)

// ---------------------------------------------------------------------------
// Facilities not wired on Wayland yet (upstream uses the XDG desktop
// portal for dialogs; cursor-shape-v1 for cursors).
// ---------------------------------------------------------------------------

// PromptForPaths is not implemented on Wayland: always fails (upstream
// prompt_for_paths goes through the XDG portal).
func (p *Platform) PromptForPaths(options platform.FileDialogOptions) ([]string, bool) {
	return nil, false
}

// PromptForPath is not implemented on Wayland (upstream prompt_for_new_path).
func (p *Platform) PromptForPath(options platform.FileDialogOptions) (string, bool) {
	return "", false
}

// OpenURL is not implemented on Wayland (upstream open_url via portal).
func (p *Platform) OpenURL(url string) error { return nil }

// SendNotification is not implemented on Wayland (upstream notify via portal).
func (p *Platform) SendNotification(notification platform.Notification) error { return nil }

// CaptureDisplay is not implemented on Wayland.
func (p *Platform) CaptureDisplay(displayID uint32) (image.Image, bool) {
	return nil, false
}

// SetCursor is a stub: Wayland cursor shaping (cursor-shape-v1) is not
// wired yet.
func (p *Platform) SetCursor(style platform.MouseCursorStyle) {}
