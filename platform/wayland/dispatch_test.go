//go:build linux || freebsd || openbsd || netbsd

package wayland

import "testing"

func TestDispatchQueuesWorkerCallbacks(t *testing.T) {
	p := &Platform{}
	var called bool
	ready := make(chan struct{})
	go func() {
		p.DispatchOnMainThread(func() { called = true })
		close(ready)
	}()
	<-ready
	if called {
		t.Fatal("callback ran on the worker instead of waiting for the event loop")
	}
	p.runDispatch()
	if !called {
		t.Fatal("event loop did not run the queued callback")
	}
}
