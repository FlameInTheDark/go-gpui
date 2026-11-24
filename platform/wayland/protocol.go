//go:build linux || freebsd || openbsd || netbsd

package wayland

import (
	"encoding/binary"
	"errors"
	"fmt"
	"syscall"
)

// ---------------------------------------------------------------------------
// Wayland wire protocol primitives
// ---------------------------------------------------------------------------

// wlFixed is the wire fixed-point type (24.8 signed).
type wlFixed int32

func fixedToFloat(f wlFixed) float32 { return float32(f) / 256.0 }

// messageWriter builds a client request message.
type messageWriter struct {
	buf []byte
}

func newMessage(objectID uint32, opcode uint16) *messageWriter {
	w := &messageWriter{buf: make([]byte, 8, 64)}
	binary.LittleEndian.PutUint32(w.buf[0:4], objectID)
	binary.LittleEndian.PutUint32(w.buf[4:8], uint32(opcode)) // size patched later
	return w
}

func (w *messageWriter) u32(v uint32) *messageWriter {
	w.buf = binary.LittleEndian.AppendUint32(w.buf, v)
	return w
}

func (w *messageWriter) i32(v int32) *messageWriter {
	w.buf = binary.LittleEndian.AppendUint32(w.buf, uint32(v))
	return w
}

func (w *messageWriter) fixed(v wlFixed) *messageWriter { return w.i32(int32(v)) }

func (w *messageWriter) str(s string) *messageWriter {
	n := len(s) + 1
	w.buf = binary.LittleEndian.AppendUint32(w.buf, uint32(n))
	w.buf = append(w.buf, s...)
	w.buf = append(w.buf, 0)
	for len(w.buf)%4 != 0 {
		w.buf = append(w.buf, 0)
	}
	return w
}

func (w *messageWriter) finish() []byte {
	for len(w.buf)%4 != 0 {
		w.buf = append(w.buf, 0)
	}
	binary.LittleEndian.PutUint32(w.buf[4:8], uint32(len(w.buf))<<16|uint32(binary.LittleEndian.Uint32(w.buf[4:8])&0xFFFF))
	return w.buf
}

// event is a raw server event: header plus unparsed body words.
type event struct {
	objectID uint32
	opcode   uint16
	body     []byte
}

// u32 returns the body word at index i (each index = 4 bytes).
func (e *event) u32(i int) uint32 {
	off := i * 4
	if off+4 > len(e.body) {
		return 0
	}
	return binary.LittleEndian.Uint32(e.body[off : off+4])
}

func (e *event) i32(i int) int32 { return int32(e.u32(i)) }

func (e *event) fixed(i int) float32 { return fixedToFloat(wlFixed(e.i32(i))) }

// strArg decodes a string argument whose length word is at index i; the
// bytes follow the length word (NUL included).
func (e *event) strArg(i int) string {
	off := i * 4
	if off+4 > len(e.body) {
		return ""
	}
	n := int(binary.LittleEndian.Uint32(e.body[off : off+4]))
	if n <= 0 {
		return ""
	}
	start := off + 4
	end := start + n - 1 // drop NUL
	if end > len(e.body) {
		end = len(e.body)
	}
	return string(e.body[start:end])
}

// readEvent reads one event from the socket. Received file descriptors are
// collected in c.fds (the keymap descriptor is consumed by the keyboard
// handler).
func (c *conn) readEvent() (*event, error) {
	for len(c.pending) < 8 {
		if err := c.fill(); err != nil {
			return nil, err
		}
	}
	objectID := binary.LittleEndian.Uint32(c.pending[0:4])
	word := binary.LittleEndian.Uint32(c.pending[4:8])
	size := int(word >> 16)
	opcode := uint16(word & 0xFFFF)
	if size < 8 || size%4 != 0 {
		return nil, fmt.Errorf("wayland: bad event size %d", size)
	}
	for len(c.pending) < size {
		if err := c.fill(); err != nil {
			return nil, err
		}
	}
	ev := &event{
		objectID: objectID,
		opcode:   opcode,
		body:     make([]byte, size-8),
	}
	copy(ev.body, c.pending[8:size])
	c.pending = c.pending[size:]
	return ev, nil
}

// fill is implemented in wayland.go (with SCM_RIGHTS fd handling).

// send writes a request.
func (c *conn) send(msg []byte) error {
	_, err := c.sock.Write(msg)
	return err
}

// u32AfterString returns the word that follows the string argument whose
// length word is at index i (strings are padded to 4 bytes on the wire).
func (e *event) u32AfterString(i int) uint32 {
	off := i * 4
	if off+4 > len(e.body) {
		return 0
	}
	n := int(binary.LittleEndian.Uint32(e.body[off : off+4]))
	padded := (n + 3) &^ 3
	return e.u32((off + 4 + padded) / 4)
}

// roundtrip sends wl_display.sync and processes events until its callback
// fires (wl_display.get_registry is object 1 opcode 2; sync is opcode 1).
func (c *conn) roundtrip() error {
	cbID := c.newObject()
	c.addCallback(cbID)
	req := newMessage(1, 1).u32(cbID).finish()
	if err := c.send(req); err != nil {
		return err
	}
	for {
		if c.callbackDone(cbID) {
			c.removeCallback(cbID)
			return nil
		}
		ev, err := c.readEvent()
		if err != nil {
			if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
				continue
			}
			c.removeCallback(cbID)
			return err
		}
		c.dispatchEvent(ev)
	}
}

var _ = errors.Is
