package app

// Subscription cancels a registered observer/subscription when released.
// The zero value is a no-op. Callers must keep it or call Detach to leak
// deliberately (Go has no destructors, so cancellation is explicit).
type Subscription struct {
	unsubscribe func()
	detached    bool
}

// NewSubscription wraps an unsubscribe function.
func NewSubscription(unsubscribe func()) Subscription {
	return Subscription{unsubscribe: unsubscribe}
}

// Cancel unsubscribes.
func (s Subscription) Cancel() {
	if s.unsubscribe != nil {
		s.unsubscribe()
	}
}

// Detach makes the subscription live until its emitter dies.
func (s Subscription) Detach() { s.detached = true }

// Join merges two subscriptions.
func (s Subscription) Join(other Subscription) Subscription {
	return Subscription{unsubscribe: func() {
		s.Cancel()
		other.Cancel()
	}}
}
