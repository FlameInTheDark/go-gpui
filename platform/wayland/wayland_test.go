//go:build linux || freebsd || openbsd || netbsd

package wayland

import (
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Fake compositor helpers
// ---------------------------------------------------------------------------

func writeMessage(c *net.UnixConn, objectID uint32, opcode uint16, body []byte) error {
	msg := make([]byte, 8, 8+len(body))
	binary.LittleEndian.PutUint32(msg[0:4], objectID)
	binary.LittleEndian.PutUint32(msg[4:8], uint32(8+len(body))<<16|uint32(opcode))
	msg = append(msg, body...)
	_, err := c.Write(msg)
	return err
}

func readFull(c *net.UnixConn, buf []byte) error {
	total := 0
	for total < len(buf) {
		n, err := c.Read(buf[total:])
		total += n
		if err != nil {
			return err
		}
	}
	return nil
}

func readRequest(c *net.UnixConn) (objectID uint32, opcode uint16, body []byte, err error) {
	head := make([]byte, 8)
	if err = readFull(c, head); err != nil {
		return
	}
	objectID = binary.LittleEndian.Uint32(head[0:4])
	word := binary.LittleEndian.Uint32(head[4:8])
	size := int(word >> 16)
	opcode = uint16(word & 0xFFFF)
	if size < 8 {
		err = errBadSize
		return
	}
	body = make([]byte, size-8)
	err = readFull(c, body)
	return
}

type badSizeError struct{}

func (badSizeError) Error() string { return "wayland: bad request size" }

var errBadSize = badSizeError{}

// sendGlobal sends wl_registry.global(name, interface, version).
func sendGlobal(c *net.UnixConn, name uint32, iface string, version uint32) error {
	var body []byte
	body = binary.LittleEndian.AppendUint32(body, name)
	str := append([]byte(iface), 0)
	body = binary.LittleEndian.AppendUint32(body, uint32(len(str)))
	body = append(body, str...)
	for len(body)%4 != 0 {
		body = append(body, 0)
	}
	body = binary.LittleEndian.AppendUint32(body, version)
	return writeMessage(c, idRegistry, 0, body)
}

// socketPair creates a connected unix socket pair.
func socketPair() (*net.UnixConn, *net.UnixConn, error) {
	ls, err := net.ListenUnix("unix", &net.UnixAddr{Name: "", Net: "unix"})
	if err != nil {
		return nil, nil, err
	}
	defer ls.Close()
	client, err := net.DialUnix("unix", nil, ls.Addr().(*net.UnixAddr))
	if err != nil {
		return nil, nil, err
	}
	srv, err := ls.AcceptUnix()
	if err != nil {
		return nil, nil, err
	}
	return client, srv, nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestWaylandRegistryAndBind drives the client through the startup dance
// against a fake compositor: get_registry, globals, bind, sync roundtrips.
func TestWaylandRegistryAndBind(t *testing.T) {
	clientSock, srvSock, err := socketPair()
	if err != nil {
		t.Fatalf("socketpair: %v", err)
	}

	binds := make(chan string, 8)
	syncs := make(chan int, 4)
	go func() {
		defer srvSock.Close()
		// 1. get_registry
		obj, op, _, err := readRequest(srvSock)
		if err != nil || obj != idDisplay || op != 2 {
			t.Errorf("expected get_registry, got obj=%d op=%d err=%v", obj, op, err)
			return
		}
		// 2. globals
		for i, iface := range []string{"wl_compositor", "wl_shm", "wl_seat", "xdg_wm_base"} {
			sendGlobal(srvSock, uint32(10+i), iface, 4)
		}
		// 3. serve binds and syncs until both syncs are answered.
		answered := 0
		for answered < 2 {
			obj, op, body, err := readRequest(srvSock)
			if err != nil {
				return
			}
			switch {
			case obj == idRegistry && op == 0: // bind
				strLen := int(binary.LittleEndian.Uint32(body[4:8]))
				if strLen >= 2 && 8+strLen-1 <= len(body) {
					binds <- string(body[8 : 8+strLen-1])
				}
			case obj == idDisplay && op == 1: // sync
				cbID := binary.LittleEndian.Uint32(body[0:4])
				writeMessage(srvSock, cbID, 0, []byte{42, 0, 0, 0})
				answered++
				syncs <- answered
			}
		}
		// Drain any trailing requests so the client never blocks on write.
		for {
			if _, _, _, err := readRequest(srvSock); err != nil {
				return
			}
		}
	}()

	c := &conn{
		sock:      clientSock,
		callbacks: map[uint32]chan struct{}{},
		handlers:  map[uint32]func(*event){},
	}
	done := make(chan *globals, 1)
	go func() {
		g, err := bindGlobals(c)
		if err != nil {
			t.Errorf("bindGlobals: %v", err)
			close(done)
			return
		}
		done <- g
	}()

	select {
	case g := <-done:
		if g == nil {
			t.Fatal("bindGlobals failed")
		}
		if g.compositor == 0 || g.shm == 0 || g.seat == 0 || g.wmBase == 0 {
			t.Fatalf("missing bound globals: %+v", g)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("bindGlobals timed out")
	}

	// All four interfaces must have been bound.
	seen := map[string]bool{}
	for i := 0; i < 4; i++ {
		select {
		case iface := <-binds:
			seen[iface] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for binds (got %v)", seen)
		}
	}
	for _, want := range []string{"wl_compositor", "wl_shm", "wl_seat", "xdg_wm_base"} {
		if !seen[want] {
			t.Errorf("interface %s was not bound", want)
		}
	}
}

// TestEventStringArg checks string decoding of registry globals.
func TestEventStringArg(t *testing.T) {
	ev := &event{body: make([]byte, 24)}
	binary.LittleEndian.PutUint32(ev.body[0:4], 7)  // name
	binary.LittleEndian.PutUint32(ev.body[4:8], 12) // "xdg_wm_base\0"
	copy(ev.body[8:19], "xdg_wm_base")
	if got := ev.strArg(1); got != "xdg_wm_base" {
		t.Fatalf("strArg = %q, want xdg_wm_base", got)
	}
	if got := ev.u32(0); got != 7 {
		t.Fatalf("u32(0) = %d, want 7", got)
	}
}

// TestFixed checks the 24.8 fixed-point conversion.
func TestFixed(t *testing.T) {
	if f := fixedToFloat(wlFixed(256)); f != 1 {
		t.Fatalf("fixedToFloat(256) = %v, want 1", f)
	}
	if f := fixedToFloat(wlFixed(128)); f != 0.5 {
		t.Fatalf("fixedToFloat(128) = %v, want 0.5", f)
	}
}

// TestMessageWriter verifies request framing (size/opcode word, padding).
func TestMessageWriter(t *testing.T) {
	msg := newMessage(1, 2).u32(42).finish()
	if len(msg) != 12 {
		t.Fatalf("message length = %d, want 12", len(msg))
	}
	if binary.LittleEndian.Uint32(msg[0:4]) != 1 {
		t.Fatal("object id not encoded")
	}
	word := binary.LittleEndian.Uint32(msg[4:8])
	if word>>16 != 12 || word&0xFFFF != 2 {
		t.Fatalf("size/opcode word = %#x, want size=12 opcode=2", word)
	}
	if binary.LittleEndian.Uint32(msg[8:12]) != 42 {
		t.Fatal("payload not encoded")
	}

	// A string arg must pad to 4 bytes.
	msg = newMessage(9, 2).str("hi").finish()
	if len(msg)%4 != 0 {
		t.Fatalf("string message not padded: %d", len(msg))
	}
}
