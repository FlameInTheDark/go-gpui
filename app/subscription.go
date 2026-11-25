package app

// Subscription is a handle for explicitly cancelling a registered callback.
// The zero value is a no-op. Discarding a Subscription does not cancel it:
// Go has no destructors, so call Cancel when the callback is no longer needed.
type Subscription struct {
	unsubscribe func()
	detached    bool
}

// NewSubscription wraps an unsubscribe function.
func NewSubscription(unsubscribe func()) Subscription {
	return Subscription{unsubscribe: unsubscribe}
}

// Cancel unsubscribes. For app subscriptions, call it on the app thread.
func (s Subscription) Cancel() {
	if s.unsubscribe != nil {
		s.unsubscribe()
	}
}

// Detach leaves the callback registered. Because cancellation is explicit in
// Go, discarding the handle also leaves the callback registered; Detach does
// not prevent a retained copy from calling Cancel.
func (s Subscription) Detach() { s.detached = true }

// Join merges two subscriptions.
func (s Subscription) Join(other Subscription) Subscription {
	return Subscription{unsubscribe: func() {
		s.Cancel()
		other.Cancel()
	}}
}
