package x11

import (
	"encoding/binary"
	"fmt"
	"time"
)

// SendMask overrides the SendEvent event mask used by the test-input
// helpers (0 = creator delivery, the window-manager path). Set it before
// the first send.
var SendMask uint32

// X11 core event codes used by the test-input helpers.
const (
	eventKeyPress      = 2
	eventKeyRelease    = 3
	eventButtonPress   = 4
	eventButtonRelease = 5

	maskKeyPress      = 1 << 0
	maskKeyRelease    = 1 << 1
	maskButtonPress   = 1 << 2
	maskButtonRelease = 1 << 3
)

// QueryTreeChildren returns the child windows of the given window (root
// returns top-level windows). It is used by test tooling to discover live
// go-gpui windows under Xvfb.
func (c *Connection) QueryTreeChildren(window uint32) ([]uint32, error) {
	req := newRequest(15, 0).u32(window)
	seq := c.nextSeq()
	if err := c.writeRequest(req.finalize()); err != nil {
		return nil, err
	}
	reply, err := c.readReply(seq)
	if err != nil {
		return nil, err
	}
	if len(reply) < 32 {
		return nil, fmt.Errorf("x11: short QueryTree reply")
	}
	n := int(binary.BigEndian.Uint16(reply[16:18]))
	children := make([]uint32, n)
	for i := 0; i < n && 32+4*i+4 <= len(reply); i++ {
		children[i] = binary.BigEndian.Uint32(reply[32+4*i:])
	}
	return children, nil
}

// sendTestEvent builds and delivers a synthetic core event to the window
// via SendEvent (opcode 25). A zero event mask delivers to the client that
// created the destination window — the same mechanism window managers use
// to deliver WM_DELETE_WINDOW — so it works regardless of what the target
// selected.
func (c *Connection) sendTestEventMasked(window uint32, mask uint32, event [32]byte) error {
	req := newRequest(25, 0). // SendEvent (second byte = propagate = 0)
					u32(window). // destination
					u32(mask).   // event mask (0 = creator delivery)
					bytes(event[:])
	if err := c.writeRequest(req.finalize()); err != nil {
		return err
	}
	return c.Flush()
}

// SendTestClick synthesizes a left-button press + release pair at
// window-relative coordinates on the given window. The go-gpui x11 backend
// processes these like real clicks, which lets tests drive interactive
// windows under Xvfb without a display server input extension.
func (c *Connection) SendTestClick(window uint32, x, y int16) error {
	if err := c.SendTestButton(window, eventButtonPress, 1, x, y, 0x100); err != nil {
		return err
	}
	return c.SendTestButton(window, eventButtonRelease, 1, x, y, 0x100)
}

// SendTestButton synthesizes one button event. state carries the modifier
// mask (0x100 = button 1 held).
func (c *Connection) SendTestButton(window uint32, code byte, detail byte, x, y int16, state uint16) error {
	var ev [32]byte
	ev[0] = code | 0x80 // send-event marker
	ev[1] = detail
	binary.BigEndian.PutUint32(ev[4:], 0) // CurrentTime
	binary.BigEndian.PutUint32(ev[8:], c.screen.RootWindow)
	binary.BigEndian.PutUint32(ev[12:], window) // event window
	binary.BigEndian.PutUint32(ev[16:], 0)      // no child
	binary.BigEndian.PutUint16(ev[20:], uint16(x))
	binary.BigEndian.PutUint16(ev[22:], uint16(y))
	binary.BigEndian.PutUint16(ev[24:], uint16(x))
	binary.BigEndian.PutUint16(ev[26:], uint16(y))
	binary.BigEndian.PutUint16(ev[28:], state)
	return c.sendTestEventMasked(window, SendMask, ev)
}

// SendTestClientMessage delivers a 32-bit ClientMessage to the window (used
// to test delivery itself, e.g. WM_PROTOCOLS/WM_DELETE_WINDOW).
func (c *Connection) SendTestClientMessage(window uint32, messageType uint32, data [5]uint32) error {
	var ev [32]byte
	ev[0] = 33 | 0x80 // ClientMessage | send-event marker
	ev[1] = 32        // format
	binary.BigEndian.PutUint32(ev[4:], window)
	binary.BigEndian.PutUint32(ev[8:], messageType)
	for i, d := range data {
		binary.BigEndian.PutUint32(ev[12+4*i:], d)
	}
	return c.sendTestEventMasked(window, SendMask, ev)
}

// Atom returns an interned atom id by name (0 when unknown).
func (c *Connection) Atom(name string) uint32 { return c.atoms[name] }

// SendTestKey synthesizes a key press or release with the given core
// keycode and modifier state.
func (c *Connection) SendTestKey(window uint32, keycode byte, press bool, state uint16) error {
	code := byte(eventKeyPress)
	if !press {
		code = eventKeyRelease
	}
	var ev [32]byte
	ev[0] = code | 0x80
	ev[1] = keycode
	binary.BigEndian.PutUint32(ev[4:], 0)
	binary.BigEndian.PutUint32(ev[8:], c.screen.RootWindow)
	binary.BigEndian.PutUint32(ev[12:], window)
	binary.BigEndian.PutUint32(ev[16:], 0)
	binary.BigEndian.PutUint16(ev[20:], 0)
	binary.BigEndian.PutUint16(ev[22:], 0)
	binary.BigEndian.PutUint16(ev[24:], 0)
	binary.BigEndian.PutUint16(ev[26:], 0)
	binary.BigEndian.PutUint16(ev[28:], state)
	return c.sendTestEventMasked(window, SendMask, ev)
}

// ---------------------------------------------------------------------------
// XTEST fake input
// ---------------------------------------------------------------------------

// XTEST core event types (the same constants the XTestFakeInput request
// takes in its type field).
const (
	xtestKeyPress      = 2
	xtestKeyRelease    = 3
	xtestButtonPress   = 4
	xtestButtonRelease = 5
	xtestMotionNotify  = 6
)

// queryExtension returns the extension's major opcode (0 when the
// extension is not present). QueryExtension is core opcode 98.
func (c *Connection) queryExtension(name string) (byte, error) {
	req := newRequest(98, 0).
		u16(uint16(len(name))).
		u16(0).
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
	if len(reply) < 12 {
		return 0, fmt.Errorf("x11: short QueryExtension reply")
	}
	if reply[8] == 0 {
		return 0, nil // not present
	}
	return reply[9], nil
}

// translateToRoot maps window-relative (x, y) to root coordinates
// (TranslateCoordinates, opcode 40, target = root).
func (c *Connection) translateToRoot(window uint32, x, y int16) (int16, int16, error) {
	req := newRequest(40, 0).
		u32(window).
		u32(c.screen.RootWindow).
		i16(x).
		i16(y).
		padTo4().
		finalize()
	seq := c.nextSeq()
	if err := c.writeRequest(req); err != nil {
		return 0, 0, err
	}
	reply, err := c.readReply(seq)
	if err != nil {
		return 0, 0, err
	}
	if len(reply) < 16 {
		return 0, 0, fmt.Errorf("x11: short TranslateCoordinates reply")
	}
	if reply[1] == 0 {
		return 0, 0, fmt.Errorf("x11: TranslateCoordinates: windows on different screens")
	}
	return int16(binary.BigEndian.Uint16(reply[12:14])), int16(binary.BigEndian.Uint16(reply[14:16])), nil
}

// xtestFakeInput sends one XTestFakeInput request (XTEST 2.2): the server
// injects the event through its real input pipeline, so windows receive it
// exactly like hardware input (SendEvent, in contrast, cannot deliver
// synthetic button/key events to the window's creator).
//
// Wire format (xtestproto.h xXTestFakeInputReq, 36 bytes):
//
//	reqType, X_XTestFakeInput(2), length | type(1) detail(1) pad(2) |
//	time(4) | root(4) | pad1(4) | pad2(4) | rootX(2) rootY(2) |
//	pad3(4) | pad4(2) pad5(1) deviceid(1)
func (c *Connection) xtestFakeInput(extOpcode byte, evType, detail byte, rootX, rootY int16) error {
	req := newRequest(extOpcode, 2). // 2 = X_XTestFakeInput
						byte(evType).
						byte(detail).
						u16(0).                   // pad0
						u32(0).                   // time = CurrentTime
						u32(c.screen.RootWindow). // root
						u32(0).                   // pad1
						u32(0).                   // pad2
						i16(rootX).
						i16(rootY).
						u32(0). // pad3
						u16(0). // pad4
						byte(0).
						byte(0). // deviceid 0 = default
						finalize()
	if err := c.writeRequest(req); err != nil {
		return err
	}
	c.bumpSeq()
	return c.Flush()
}

// SendTestClickXTest moves the pointer to window-relative (x, y) and clicks
// button 1 via the XTEST extension (real events, including the hover
// motion). Falls back to SendEvent when XTEST is unavailable.
func (c *Connection) SendTestClickXTest(window uint32, x, y int16) error {
	ext, err := c.queryExtension("XTEST")
	if err != nil || ext == 0 {
		return c.SendTestClick(window, x, y)
	}
	rx, ry, err := c.translateToRoot(window, x, y)
	if err != nil {
		return err
	}
	if err := c.xtestFakeInput(ext, xtestMotionNotify, 0, rx, ry); err != nil {
		return err
	}
	time.Sleep(60 * time.Millisecond)
	if err := c.xtestFakeInput(ext, xtestButtonPress, 1, rx, ry); err != nil {
		return err
	}
	time.Sleep(40 * time.Millisecond)
	return c.xtestFakeInput(ext, xtestButtonRelease, 1, rx, ry)
}

// SendTestMotionXTest moves the pointer to window-relative (x, y) via XTEST
// (hover testing).
func (c *Connection) SendTestMotionXTest(window uint32, x, y int16) error {
	ext, err := c.queryExtension("XTEST")
	if err != nil || ext == 0 {
		return nil
	}
	rx, ry, err := c.translateToRoot(window, x, y)
	if err != nil {
		return err
	}
	return c.xtestFakeInput(ext, xtestMotionNotify, 0, rx, ry)
}
