package api

// SessionEvent is a lifecycle event emitted during session execution.
// Observers receive these and decide how to present them (SSE, desktop UI, etc.).
type SessionEvent struct {
	Type      string // "thinking", "tool_calls", "assistant_message", "error", "stopped", "idle", "project_created", ...
	SessionID string
	Data      any    // original payload — for run-loop events it's map[string]any (gin.H)
}

// SessionObserver receives session lifecycle events.
type SessionObserver interface {
	OnSessionEvent(event SessionEvent)
}
