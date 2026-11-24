// Package x11 provides a pure-Go X11 platform backend: a from-scratch X11
// protocol client (no libxcb/xlib bindings) implementing the window,
// input and PutImage-present surface gpui needs. Rendering goes through
// the shared software rasterizer.
package x11

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/render"
	"github.com/FlameInTheDark/go-gpui/text"
	"image"
)

// atom ids for the properties we set/read.
const (
	atomNone = 0
)

var atomNames = []string{
	"WM_PROTOCOLS", "WM_DELETE_WINDOW", "WM_NAME", "WM_CLASS", "_NET_WM_NAME",
	"_NET_WM_PID", "UTF8_STRING", "STRING", "WM_NORMAL_HINTS", "CARDINAL",
	"WM_STATE", "_NET_WM_STATE", "ATOM", "WM_TAKE_FOCUS", "_MOTIF_WM_HINTS",
}

const (
	atomWMProtocols = iota
	atomWMDeleteWindow
	atomWMName
	atomWMClass
	atomNetWMName
	atomNetWMPid
	atomUTF8String
	atomSTRING
	atomWMNormalHints
	atomCARDINAL
	atomWMState
	atomNetWMState
	atomATOM
	atomWMTakeFocus
	atomMotifWMHints
)

// Connection is a minimal X11 protocol client over a Unix/TCP socket.
type Connection struct {
	conn          net.Conn
	seq           uint16
	screen        ScreenInfo
	atoms         map[string]uint32
	mu            sync.Mutex
	depth         byte
	visualID      uint32
	ridBase       uint32 // resource-id-base from the setup reply
	ridMask       uint32 // resource-id-mask from the setup reply
	idCounter     uint32
	pendingEvents []xEvent
	lastError     error
}

// allocID mints a resource id from the server-provided base and mask
// (upstream xcb_generate_id).
func (c *Connection) allocID() uint32 {
	c.idCounter++
	if c.ridMask == 0 {
		return 0x00400000 + c.idCounter
	}
	return c.ridBase | (c.idCounter & c.ridMask)
}

// ScreenInfo describes the default screen.
type ScreenInfo struct {
	RootWindow  uint32
	WidthPx     uint16
	HeightPx    uint16
	WhitePixel  uint32
	BlackPixel  uint32
	Depth       byte
	VisualID    uint32
	FormatDepth byte
}

type xEvent struct {
	code byte
	data []byte
}

// Connect opens the display named by $DISPLAY.
func Connect() (*Connection, error) {
	display := os.Getenv("DISPLAY")
	if display == "" {
		return nil, errors.New("x11: $DISPLAY is not set")
	}
	var conn net.Conn
	var err error
	dispNum := 0
	if strings.HasPrefix(display, "/") {
		conn, err = net.Dial("unix", display)
	} else {
		host := ""
		rest := display
		if i := strings.Index(display, ":"); i >= 0 {
			host = display[:i]
			rest = display[i+1:]
		}
		if i := strings.Index(rest, "."); i >= 0 {
			rest = rest[:i]
		}
		dispNum, _ = strconv.Atoi(rest)
		if host == "" || host == "unix" {
			path := fmt.Sprintf("/tmp/.X11-unix/X%d", dispNum)
			conn, err = net.Dial("unix", path)
			if err != nil {
				return nil, fmt.Errorf("x11: cannot connect to %s: %w", path, err)
			}
		} else {
			conn, err = net.Dial("tcp", fmt.Sprintf("%s:6000", host))
		}
	}
	if err != nil {
		return nil, err
	}
	c := &Connection{conn: conn, atoms: map[string]uint32{}}
	if err := c.handshake(dispNum); err != nil {
		conn.Close()
		return nil, err
	}
	return c, nil
}

// handshake performs the X11 setup exchange with MIT-MAGIC-COOKIE-1 auth.
func (c *Connection) handshake(dispNum int) error {
	// Load the authority cookie.
	authName, authData, _ := loadXAuth(dispNum)

	// Byte order 'B' (big): the rest of this client (requests and reply
	// parsing) is big-endian, so the connection declares big-endian and the
	// server replies in kind. Protocol 11.0.
	req := make([]byte, 0, 64)
	req = append(req, 'B', 0)
	req = binary.BigEndian.AppendUint16(req, 11) // major
	req = binary.BigEndian.AppendUint16(req, 0)  // minor
	req = binary.BigEndian.AppendUint16(req, uint16(len(authName)))
	req = binary.BigEndian.AppendUint16(req, uint16(len(authData)))
	req = binary.BigEndian.AppendUint16(req, 0) // unused
	req = append(req, authName...)
	req = append(req, authData...)
	// Pad to 4 bytes.
	for len(req)%4 != 0 {
		req = append(req, 0)
	}
	if _, err := c.conn.Write(req); err != nil {
		return err
	}

	// Read the reply.
	head := make([]byte, 8)
	if _, err := io.ReadFull(c.conn, head); err != nil {
		return err
	}
	if head[0] != 1 {
		// Error or failure.
		if head[0] == 0 {
			// Read reason.
			reasonLen := int(head[1])
			rest := make([]byte, reasonLen+2)
			_, _ = io.ReadFull(c.conn, rest)
			return fmt.Errorf("x11: setup failed: %s", string(rest[:reasonLen]))
		}
		return fmt.Errorf("x11: unexpected setup reply %d", head[0])
	}
	// Success: 8 bytes header + additional data. The length field (bytes
	// 6-7, big-endian as declared by our 'B' byte order) counts 4-byte units.
	additional := (int(head[6])<<8 | int(head[7])) * 4
	body := make([]byte, additional)
	if _, err := io.ReadFull(c.conn, body); err != nil {
		return err
	}
	// body offsets (after the 8-byte setup header): 0-3 release, 4-7
	// resource-id-base, 8-11 resource-id-mask, 12-15 motion-buffer-size,
	// 16-17 vendor-len, 18-19 max-request-length, 20 screens, 21 formats,
	// 22 image-byte-order, ..., 28-31 unused, 32+ vendor, then formats,
	// then screens.
	vendorLen := int(binary.BigEndian.Uint16(body[16:18]))
	formatsLen := int(body[21])
	off := 32 + vendorLen
	if pad := off % 4; pad != 0 {
		off += 4 - pad
	}
	// Skip pixmap formats (8 bytes each).
	off += formatsLen * 8
	// First screen: 4 root, 4 default-colormap, 4 white, 4 black,
	// 4 current-input-masks, 2 width/height px, 2 width/height mm,
	// 2 min-installed-maps, 2 max-installed-maps, 4 root-visual, ...
	// then allowed-depths.
	if off+40 > len(body) {
		return errors.New("x11: short setup")
	}
	// The server replies in the byte order we declared ('B'), so parse BE.
	screen := body[off:]
	c.ridBase = binary.BigEndian.Uint32(body[4:8])
	c.ridMask = binary.BigEndian.Uint32(body[8:12])
	c.screen.RootWindow = binary.BigEndian.Uint32(screen[0:4])
	c.screen.WhitePixel = binary.BigEndian.Uint32(screen[8:12])
	c.screen.BlackPixel = binary.BigEndian.Uint32(screen[12:16])
	// Screen layout: 0-3 root, 4-7 default-colormap, 8-11 white,
	// 12-15 black, 16-19 current-input-masks, 20-21 width-in-pixels,
	// 22-23 height-in-pixels, 24-27 width/height-in-mm, 28-31
	// min/max-installed-maps, 32-35 root-visual.
	c.screen.WidthPx = binary.BigEndian.Uint16(screen[20:22])
	c.screen.HeightPx = binary.BigEndian.Uint16(screen[22:24])
	c.screen.VisualID = binary.BigEndian.Uint32(screen[32:36])
	c.visualID = c.screen.VisualID
	// Parse allowed depths to find 24-bit TrueColor. Each DEPTH is 8
	// bytes (depth, pad, visuals-count u16, 4 pad) followed by
	// visualsLen VISUALTYPEs of 24 bytes (first field: visual id u32).
	depthsOff := 40
	for {
		if depthsOff+8 > len(screen) {
			break
		}
		depth := screen[depthsOff]
		visualsLen := int(binary.BigEndian.Uint16(screen[depthsOff+2 : depthsOff+4]))
		depthsOff += 8
		if depth == 24 && visualsLen > 0 {
			c.screen.Depth = 24
			c.visualID = binary.BigEndian.Uint32(screen[depthsOff : depthsOff+4])
			c.screen.FormatDepth = 24
			break
		}
		depthsOff += visualsLen * 24
	}
	if c.screen.Depth == 0 {
		c.screen.Depth = 24
	}
	return nil
}

// loadXAuth reads ~/.Xauthority for the display cookie.
func loadXAuth(dispNum int) (name string, data []byte, err error) {
	path := os.Getenv("XAUTHORITY")
	if path == "" {
		home, _ := os.UserHomeDir()
		path = home + "/.Xauthority"
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	// Family 256 (Wild), 252 (local) etc.; match any entry for display.
	off := 0
	for off+4 <= len(raw) {
		family := int(binary.BigEndian.Uint16(raw[off : off+2]))
		addrLen := int(binary.BigEndian.Uint16(raw[off+2 : off+4]))
		off += 4
		if off+addrLen+2 > len(raw) {
			break
		}
		addr := raw[off : off+addrLen]
		off += addrLen
		numLen := int(binary.BigEndian.Uint16(raw[off : off+2]))
		off += 2
		if off+numLen+2 > len(raw) {
			break
		}
		num := raw[off : off+numLen]
		off += numLen
		nameLen := int(binary.BigEndian.Uint16(raw[off : off+2]))
		off += 2
		if off+nameLen+2 > len(raw) {
			break
		}
		authName := string(raw[off : off+nameLen])
		off += nameLen
		dataLen := int(binary.BigEndian.Uint16(raw[off : off+2]))
		off += 2
		if off+dataLen > len(raw) {
			break
		}
		authData := raw[off : off+dataLen]
		off += dataLen
		n, _ := strconv.Atoi(string(num))
		if (family == 256 || family == 252 || family == 0) && (n == dispNum || numLen == 0) && len(authData) > 0 {
			return authName, authData, nil
		}
		_ = addr
	}
	return "", nil, errors.New("no matching cookie")
}

// Close closes the connection.
func (c *Connection) Close() error { return c.conn.Close() }

// request writes a fully-formed request. The 2-byte length field at
// [2:4] must already contain the request length in 4-byte units; build
// requests with newRequest().finalize() so this holds. Requests sent
// without expecting a reply still consume a sequence number on the wire,
// tracked via bumpSeq.
func (c *Connection) writeRequest(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(data)%4 != 0 {
		return fmt.Errorf("x11: request not padded to 4 bytes (len %d)", len(data))
	}
	_, err := c.conn.Write(data)
	return err
}

// bumpSeq advances the wire sequence number for a request that expects no
// reply (every request consumes one sequence number).
func (c *Connection) bumpSeq() {
	c.mu.Lock()
	c.seq++
	c.mu.Unlock()
}

// requestBuilder assembles a request with automatic length handling (the
// moral equivalent of xcb's request structs).
type requestBuilder struct {
	buf []byte
}

// newRequest starts a request: opcode and the second header byte.
func newRequest(opcode, second byte) *requestBuilder {
	b := &requestBuilder{buf: make([]byte, 0, 64)}
	b.buf = append(b.buf, opcode, second, 0, 0) // length placeholder
	return b
}

func (b *requestBuilder) u16(v uint16) *requestBuilder {
	b.buf = binary.BigEndian.AppendUint16(b.buf, v)
	return b
}

func (b *requestBuilder) u32(v uint32) *requestBuilder {
	b.buf = binary.BigEndian.AppendUint32(b.buf, v)
	return b
}

func (b *requestBuilder) i16(v int16) *requestBuilder {
	b.buf = binary.BigEndian.AppendUint16(b.buf, uint16(v))
	return b
}

func (b *requestBuilder) byte(v byte) *requestBuilder {
	b.buf = append(b.buf, v)
	return b
}

func (b *requestBuilder) bytes(p []byte) *requestBuilder {
	b.buf = append(b.buf, p...)
	return b
}

func (b *requestBuilder) str(s string) *requestBuilder {
	b.buf = append(b.buf, s...)
	return b
}

// padTo4 pads the buffer to a 4-byte multiple.
func (b *requestBuilder) padTo4() *requestBuilder {
	for len(b.buf)%4 != 0 {
		b.buf = append(b.buf, 0)
	}
	return b
}

// finalize writes the request length (4-byte units) into the header and
// returns the buffer.
func (b *requestBuilder) finalize() []byte {
	binary.BigEndian.PutUint16(b.buf[2:4], uint16(len(b.buf)/4))
	return b.buf
}

func (c *Connection) nextSeq() uint16 {
	c.mu.Lock()
	c.seq++
	s := c.seq
	c.mu.Unlock()
	return s
}

// InternAtom interns a name and returns its id (opcode 16).
func (c *Connection) InternAtom(name string, onlyIfExists bool) (uint32, error) {
	only := byte(0)
	if onlyIfExists {
		only = 1
	}
	req := newRequest(16, only).
		u16(uint16(len(name))). // name length
		u16(0).                 // unused
		str(name).
		padTo4().
		finalize()
	seq := c.nextSeq()
	if err := c.writeRequest(req); err != nil {
		return 0, err
	}
	reply, err := c.readReply(seq)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(reply[8:12]), nil
}

// readReply reads events/replies until the reply with seq arrives.
func (c *Connection) readReply(seq uint16) ([]byte, error) {
	// First drain buffered events.
	for {
		if len(c.pendingEvents) > 0 {
			ev := c.pendingEvents[0]
			c.pendingEvents = c.pendingEvents[1:]
			_ = ev
		}
		break
	}
	buf := make([]byte, 32)
	for {
		if _, err := io.ReadFull(c.conn, buf); err != nil {
			return nil, err
		}
		switch buf[0] {
		case 1: // reply
			if binary.BigEndian.Uint16(buf[2:4]) == seq {
				length := int(binary.BigEndian.Uint32(buf[4:8]))
				if length > 0 {
					extra := make([]byte, length*4)
					if _, err := io.ReadFull(c.conn, extra); err != nil {
						return nil, err
					}
					return append(buf, extra...), nil
				}
				return buf, nil
			}
			// Out-of-order reply (shouldn't happen single-threaded); skip.
		case 0: // error
			return nil, fmt.Errorf("x11: error code %d seq %d", buf[1], binary.BigEndian.Uint16(buf[2:4]))
		default: // event
			ev := make([]byte, 32)
			copy(ev, buf)
			c.pendingEvents = append(c.pendingEvents, xEvent{code: buf[0], data: ev})
		}
	}
}

// CreateWindow creates a window and returns its id (opcode 1).
func (c *Connection) CreateWindow(x, y, width, height int, title string) (uint32, error) {
	c.mu.Lock()
	c.idCounter++
	wid := c.ridBase | (c.idCounter & c.ridMask)
	c.mu.Unlock()

	// CreateWindow: depth 24 (matching the chosen visual), class
	// INPUT_OUTPUT(1), visual from the setup, with background pixel and
	// event mask values.
	const (
		cwBackPixel = 1 << 1
		cwEventMask = 1 << 11
	)
	// Exposure|PointerMotion|KeyPress|KeyRelease|ButtonPress|ButtonRelease|
	// StructureNotify|FocusChange|EnterWindow|LeaveWindow
	const eventMask = 1<<0 | 1<<1 | 1<<2 | 1<<3 | 1<<4 | 1<<5 | 1<<6 |
		1<<7 | 1<<8 | 1<<10 | 1<<11 | 1<<17 | 1<<18
	req := newRequest(1, 24). // depth
					u32(wid).
					u32(c.screen.RootWindow).
					i16(int16(x)).
					i16(int16(y)).
					u16(uint16(width)).
					u16(uint16(height)).
					u16(0). // border width
					u16(1). // class InputOutput
					u32(c.visualID).
					u32(cwBackPixel | cwEventMask).
					u32(c.screen.BlackPixel).
					u32(eventMask).
					finalize()
	if err := c.writeRequest(req); err != nil {
		return 0, err
	}
	c.bumpSeq()

	// WM_PROTOCOLS with WM_DELETE_WINDOW (ChangeProperty, format 32).
	if del, ok := c.atoms["WM_DELETE_WINDOW"]; ok {
		if proto, ok2 := c.atoms["WM_PROTOCOLS"]; ok2 {
			const atomATOM = 4        // predefined ATOM atom
			req := newRequest(18, 0). // ChangeProperty, Replace
							u32(wid).
							u32(proto).
							u32(atomATOM).
							byte(32). // format
							byte(0).byte(0).byte(0).
							u32(1). // one atom
							u32(del).
							finalize()
			if err := c.writeRequest(req); err != nil {
				return 0, err
			}
			c.bumpSeq()
		}
	}

	// WM_NAME + _NET_WM_NAME.
	c.setPropertyString(wid, "WM_NAME", "STRING", title)
	c.setPropertyString(wid, "_NET_WM_NAME", "UTF8_STRING", title)
	return wid, nil
}

func (c *Connection) setPropertyString(wid uint32, propName, typeName, value string) {
	prop, ok1 := c.atoms[propName]
	typ, ok2 := c.atoms[typeName]
	if !ok1 || !ok2 {
		return
	}
	// ChangeProperty (opcode 18), mode Replace, format 8, data = the
	// raw string bytes; the length field counts format units (bytes).
	req := newRequest(18, 0).
		u32(wid).
		u32(prop).
		u32(typ).
		byte(8). // format
		byte(0).byte(0).byte(0).
		u32(uint32(len(value))).
		str(value).
		padTo4().
		finalize()
	_ = c.writeRequest(req)
	c.bumpSeq()
}

// MapWindow maps the window (opcode 8).
func (c *Connection) MapWindow(wid uint32) error {
	req := newRequest(8, 0).u32(wid).finalize()
	if err := c.writeRequest(req); err != nil {
		return err
	}
	c.bumpSeq()
	return nil
}

// DestroyWindow destroys the window (opcode 4).
func (c *Connection) DestroyWindow(wid uint32) error {
	req := newRequest(4, 0).u32(wid).finalize()
	if err := c.writeRequest(req); err != nil {
		return err
	}
	c.bumpSeq()
	return nil
}

// PutImageBGRX writes a full-window BGRA (X byte order) frame via PutImage
// (opcode 72), split into chunks that stay within the 16-bit request length.
func (c *Connection) PutImageBGRX(wid, gc uint32, width, height int, data []byte) error {
	// Max request length is 65535 4-byte units (~256KB); keep chunks
	// smaller and row-aligned.
	const maxChunk = 60000
	bpp := 4
	rowBytes := width * bpp
	if rowBytes == 0 || len(data) < rowBytes*height {
		return fmt.Errorf("x11: PutImage bad buffer (want %d bytes, have %d)", rowBytes*height, len(data))
	}
	y := 0
	for y < height {
		rows := maxChunk / rowBytes
		if rows < 1 {
			rows = 1
		}
		if y+rows > height {
			rows = height - y
		}
		chunk := data[y*rowBytes : (y+rows)*rowBytes]
		req := newRequest(72, 2). // PutImage, ZPixmap
						u32(wid).
						u32(gc).
						u16(uint16(width)).
						u16(uint16(rows)).
						i16(0).          // dst-x
						i16(int16(y)).   // dst-y
						byte(0).         // left-pad
						byte(24).        // depth
						byte(0).byte(0). // pad
						bytes(chunk).
						padTo4().
						finalize()
		if err := c.writeRequest(req); err != nil {
			return err
		}
		c.bumpSeq()
		y += rows
	}
	return nil
}

// CreateGC creates a graphics context for PutImage (opcode 55).
func (c *Connection) CreateGC(drawable uint32) (uint32, error) {
	c.mu.Lock()
	c.idCounter++
	gc := c.ridBase | (c.idCounter & c.ridMask)
	c.mu.Unlock()
	// No attribute values; the mask is 0.
	req := newRequest(55, 0).
		u32(gc).
		u32(drawable).
		u32(0). // value mask
		finalize()
	if err := c.writeRequest(req); err != nil {
		return 0, err
	}
	c.bumpSeq()
	return gc, nil
}

// PollEvent reads one buffered event or returns nil.
func (c *Connection) PollEvent() *xEvent {
	if len(c.pendingEvents) > 0 {
		ev := c.pendingEvents[0]
		c.pendingEvents = c.pendingEvents[1:]
		return &ev
	}
	return nil
}

// Flush writes any pending bytes.
func (c *Connection) Flush() error { return nil }

// ReadEvents blocks (with timeout) reading events into the buffer.
func (c *Connection) ReadEvents(timeout time.Duration) {
	if deadline, ok := c.conn.(interface{ SetReadDeadline(time.Time) error }); ok {
		_ = deadline.SetReadDeadline(time.Now().Add(timeout))
	}
	buf := make([]byte, 32)
	for {
		if _, err := io.ReadFull(c.conn, buf); err != nil {
			return
		}
		if buf[0] == 1 || buf[0] == 0 {
			// replies/errors while polling: skip
			continue
		}
		ev := make([]byte, 32)
		copy(ev, buf)
		c.pendingEvents = append(c.pendingEvents, xEvent{code: buf[0], data: ev})
		if timeout > 0 {
			// One batch is enough per poll.
			if deadline, ok := c.conn.(interface{ SetReadDeadline(time.Time) error }); ok {
				_ = deadline.SetReadDeadline(time.Now().Add(time.Millisecond))
			}
			buf2 := make([]byte, 32)
			if _, err := io.ReadFull(c.conn, buf2); err != nil {
				return
			}
			if buf2[0] > 1 {
				ev2 := make([]byte, 32)
				copy(ev2, buf2)
				c.pendingEvents = append(c.pendingEvents, xEvent{code: buf2[0], data: ev2})
			}
			return
		}
	}
}

// Screen returns the screen info.
func (c *Connection) Screen() ScreenInfo { return c.screen }

// PendingCount returns buffered event count.
func (c *Connection) PendingCount() int { return len(c.pendingEvents) }

// Platform implements platform.Platform over the X11 connection.
type Platform struct {
	conn       *Connection
	windows    map[uint32]*Window
	mu         sync.Mutex
	quit       bool
	textSys    *text.System
	lastWindow *Window

	// dispatchQueue holds main-thread callbacks (drained by Run).
	dispatchMu    sync.Mutex
	dispatchQueue []func()
}

// NewPlatform connects to X11 and interns atoms.
func NewPlatform() (*Platform, error) {
	conn, err := Connect()
	if err != nil {
		return nil, err
	}
	p := &Platform{conn: conn, windows: map[uint32]*Window{}}
	for _, name := range atomNames {
		id, err := conn.InternAtom(name, false)
		if err == nil {
			conn.atoms[name] = id
		}
	}
	return p, nil
}

// Run starts the event loop.
func (p *Platform) Run(onFinish func()) {
	onFinish()
	for !p.quit {
		p.conn.ReadEvents(16 * time.Millisecond)
		p.processEvents()
		p.runDispatch()
		p.presentDirty()
	}
	p.runDispatch()
}

// runDispatch drains queued main-thread callbacks (so DispatchOnMainThread
// is safe to call from any goroutine, e.g. async process completion).
func (p *Platform) runDispatch() {
	p.dispatchMu.Lock()
	queue := p.dispatchQueue
	p.dispatchQueue = nil
	p.dispatchMu.Unlock()
	for _, f := range queue {
		f()
	}
}

// processEvents drains buffered events. Event layouts follow the X11 core
// protocol; events forwarded by SendEvent have bit 0x80 set in the response
// type, so the code is masked first.
func (p *Platform) processEvents() {
	for {
		ev := p.conn.PollEvent()
		if ev == nil {
			return
		}
		code := ev.code & 0x7f
		switch code {
		case 12: // Expose: window [4:8]
			wid := binary.BigEndian.Uint32(ev.data[4:8])
			if w := p.windowByXID(wid); w != nil {
				w.dirty = true
			}
		case 22: // ConfigureNotify: window [8:12], size [20:24]
			wid := binary.BigEndian.Uint32(ev.data[8:12])
			width := int(binary.BigEndian.Uint16(ev.data[20:22]))
			height := int(binary.BigEndian.Uint16(ev.data[22:24]))
			if w := p.windowByXID(wid); w != nil {
				newSize := geometry.SizePixels(float32(width), float32(height))
				if newSize != w.size {
					w.size = newSize
					w.callbacks.OnResize(newSize, w.scale)
				}
			}
		case 33: // ClientMessage: window [4:8], type [8:12], data [12:32]
			wid := binary.BigEndian.Uint32(ev.data[4:8])
			msgType := binary.BigEndian.Uint32(ev.data[8:12])
			if msgType == p.conn.atoms["WM_PROTOCOLS"] {
				proto := binary.BigEndian.Uint32(ev.data[12:16])
				if proto == p.conn.atoms["WM_DELETE_WINDOW"] {
					if w := p.windowByXID(wid); w != nil {
						w.callbacks.OnClose()
					}
				}
			}
		case 4, 5: // ButtonPress/Release: detail [1], event window
			// [12:16], event-x/y [24:28] (window-relative), state [28:30]
			wid := binary.BigEndian.Uint32(ev.data[12:16])
			detail := ev.data[1]
			x := int(int16(binary.BigEndian.Uint16(ev.data[24:26])))
			y := int(int16(binary.BigEndian.Uint16(ev.data[26:28])))
			if w := p.windowByXID(wid); w != nil {
				w.dispatchButton(code == 4, detail, x, y, ev.data[28])
			}
		case 6: // MotionNotify: event window [12:16], event-x/y [24:28]
			wid := binary.BigEndian.Uint32(ev.data[12:16])
			x := int(int16(binary.BigEndian.Uint16(ev.data[24:26])))
			y := int(int16(binary.BigEndian.Uint16(ev.data[26:28])))
			if w := p.windowByXID(wid); w != nil {
				w.dispatchMotion(x, y, ev.data[28])
			}
		case 2, 3: // KeyPress/KeyRelease: keycode [1], event window [12:16],
			// state [28:30]
			wid := binary.BigEndian.Uint32(ev.data[12:16])
			detail := ev.data[1]
			state := ev.data[28]
			if w := p.windowByXID(wid); w != nil {
				w.dispatchKey(code == 2, detail, state)
			}
		}
	}
}

func (p *Platform) presentDirty() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, w := range p.windows {
		if w.dirty {
			w.present()
		}
	}
}

func (p *Platform) windowByXID(wid uint32) *Window {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.windows[wid]
}

// Quit stops the loop.
func (p *Platform) Quit() { p.quit = true }

// Quitting reports the quit state.
func (p *Platform) Quitting() bool { return p.quit }

// Displays returns the screen.
func (p *Platform) Displays() []platform.Display {
	s := p.conn.Screen()
	return []platform.Display{{
		ID:          1,
		Bounds:      geometry.BoundsPixels(0, 0, float32(s.WidthPx), float32(s.HeightPx)),
		ScaleFactor: 1,
	}}
}

// PrimaryDisplay returns the screen.
func (p *Platform) PrimaryDisplay() (platform.Display, bool) { return p.Displays()[0], true }

// Appearance reports light.
func (p *Platform) Appearance() color.WindowAppearance { return color.WindowAppearanceLight }

// Clipboard stubs.
func (p *Platform) ReadClipboard() (platform.ClipboardItem, bool) {
	return platform.ClipboardItem{}, false
}
func (p *Platform) WriteClipboard(item platform.ClipboardItem) {}

// DispatchOnMainThread queues f for the Run loop (thread-safe).
func (p *Platform) DispatchOnMainThread(f func()) {
	p.dispatchMu.Lock()
	p.dispatchQueue = append(p.dispatchQueue, f)
	p.dispatchMu.Unlock()
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

// OpenWindow creates an X11 window.
func (p *Platform) OpenWindow(options platform.WindowOptions, callbacks platform.WindowCallbacks) (platform.PlatformWindow, error) {
	size := geometry.SizePixels(800, 600)
	if options.WindowBounds != nil {
		size = options.WindowBounds.Bounds.Size
	}
	title := options.Title
	wid, err := p.conn.CreateWindow(100, 100, int(size.Width.AsF32()), int(size.Height.AsF32()), title)
	if err != nil {
		return nil, err
	}
	gc, err := p.conn.CreateGC(wid)
	if err != nil {
		return nil, err
	}
	w := &Window{
		platform:  p,
		xid:       wid,
		gc:        gc,
		size:      size,
		scale:     1,
		callbacks: callbacks,
		atlas:     render.NewAtlas(),
	}
	p.mu.Lock()
	p.windows[wid] = w
	p.mu.Unlock()
	p.conn.MapWindow(wid)
	callbacks.OnResize(size, 1)
	callbacks.OnActiveStatus(true)
	callbacks.OnVisibility(true)
	return w, nil
}

// ---------------------------------------------------------------------------
// Facilities not wired on X11 yet (stubs keep the Platform interface
// uniform; upstream gpui uses the XDG desktop portal for these).
// ---------------------------------------------------------------------------

// PromptForPaths is not implemented on X11: always fails (upstream
// prompt_for_paths goes through the XDG portal).
func (p *Platform) PromptForPaths(options platform.FileDialogOptions) ([]string, bool) {
	return nil, false
}

// PromptForPath is not implemented on X11 (upstream prompt_for_new_path).
func (p *Platform) PromptForPath(options platform.FileDialogOptions) (string, bool) {
	return "", false
}

// OpenURL is not implemented on X11 (upstream open_url via xdg-open).
func (p *Platform) OpenURL(url string) error { return nil }

// SendNotification is not implemented on X11.
func (p *Platform) SendNotification(notification platform.Notification) error { return nil }

// CaptureDisplay is not implemented on X11.
func (p *Platform) CaptureDisplay(displayID uint32) (image.Image, bool) {
	return nil, false
}

// SetCursor is a stub: X11 cursor shaping (xcursor) is not wired yet.
func (p *Platform) SetCursor(style platform.MouseCursorStyle) {}
