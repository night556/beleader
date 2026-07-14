package api

// SessionEvent is a lifecycle event emitted during session execution.
type SessionEvent struct {
	Type      string
	SessionID string
	Data      any
}

// SessionObserver receives session lifecycle events.
type SessionObserver interface {
	OnSessionEvent(event SessionEvent)
}
