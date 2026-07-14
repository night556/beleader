package engine

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ── Event record (persisted + streamed) ──

// RuntimeEventRecord is the SSE event envelope matching CodeWhale's event model.
// Envelope fields carry routing metadata; event-specific data goes in Payload.
type RuntimeEventRecord struct {
	SchemaVersion int            `json:"schema_version"`
	Seq           int64          `json:"seq"`
	Timestamp     string         `json:"timestamp"`
	ThreadID      string         `json:"thread_id"`
	TurnID        string         `json:"turn_id,omitempty"`
	ItemID        string         `json:"item_id,omitempty"`
	Event         string         `json:"event"`
	Payload       map[string]any `json:"payload"`
}

// ── Turn record ──

// TurnRecord is the turn-level metadata, persisted to turns/<id>.json.
type TurnRecord struct {
	ID           string   `json:"id"`
	ThreadID     string   `json:"thread_id"`
	Status       string   `json:"status"` // in_progress | completed | interrupted
	InputSummary string   `json:"input_summary"`
	StartedAt    string   `json:"started_at,omitempty"`
	EndedAt      string   `json:"ended_at,omitempty"`
	DurationMs   int64    `json:"duration_ms,omitempty"`
	ItemIDs      []string `json:"item_ids"`
}

// ── Item record ──

// TurnItemRecord is the item-level content record.
type TurnItemRecord struct {
	ID        string         `json:"id"`
	TurnID    string         `json:"turn_id"`
	Kind      string         `json:"kind"`   // user_message | agent_message | tool_call | command_execution | error
	Status    string         `json:"status"` // in_progress | completed | failed | interrupted
	Summary   string         `json:"summary"`
	Detail    string         `json:"detail,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	StartedAt string         `json:"started_at,omitempty"`
	EndedAt   string         `json:"ended_at,omitempty"`
}

// NewItemID generates a unique item ID.
func NewItemID() string {
	return "item_" + uuid.New().String()[:8]
}

// NewTurnID generates a unique turn ID.
func NewTurnID() string {
	return "turn_" + uuid.New().String()[:8]
}

// ItemKind constants matching CodeWhale's TurnItemKind.
const (
	ItemKindUserMessage      = "user_message"
	ItemKindAgentMessage     = "agent_message"
	ItemKindToolCall         = "tool_call"
	ItemKindCommandExecution = "command_execution"
	ItemKindError            = "error"
)

// ItemStatus constants.
const (
	ItemStatusInProgress  = "in_progress"
	ItemStatusCompleted   = "completed"
	ItemStatusFailed      = "failed"
	ItemStatusInterrupted = "interrupted"
)

// TurnStatus constants.
const (
	TurnStatusInProgress  = "in_progress"
	TurnStatusCompleted   = "completed"
	TurnStatusInterrupted = "interrupted"
)

// Event type constants matching CodeWhale's event names.
const (
	EventTurnStarted   = "turn.started"
	EventTurnCompleted = "turn.completed"
	EventItemStarted   = "item.started"
	EventItemDelta     = "item.delta"
	EventItemCompleted = "item.completed"
	EventItemFailed    = "item.failed"
)

// MakeEvent is a convenience constructor for RuntimeEventRecord.
func MakeEvent(event, threadID, turnID, itemID string, payload map[string]any) RuntimeEventRecord {
	return RuntimeEventRecord{
		SchemaVersion: 1,
		Timestamp:     time.Now().UTC().Format(time.RFC3339Nano),
		ThreadID:      threadID,
		TurnID:        turnID,
		ItemID:        itemID,
		Event:         event,
		Payload:       payload,
	}
}

// StartItem creates an item.started event with an in_progress TurnItemRecord.
func StartItem(itemID, turnID, kind, summary string, metadata map[string]any) RuntimeEventRecord {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item := TurnItemRecord{
		ID:        itemID,
		TurnID:    turnID,
		Kind:      kind,
		Status:    ItemStatusInProgress,
		Summary:   summary,
		Metadata:  metadata,
		StartedAt: now,
	}
	return RuntimeEventRecord{
		SchemaVersion: 1,
		Timestamp:     now,
		Event:         EventItemStarted,
		Payload:       map[string]any{"item": item},
	}
}

// CompleteItem creates an item.completed event with a completed TurnItemRecord.
func CompleteItem(itemID, turnID, kind, detail string, metadata map[string]any) RuntimeEventRecord {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	summary := detail
	if len(summary) > 280 {
		summary = summary[:280]
	}
	item := TurnItemRecord{
		ID:        itemID,
		TurnID:    turnID,
		Kind:      kind,
		Status:    ItemStatusCompleted,
		Summary:   summary,
		Detail:    detail,
		Metadata:  metadata,
		EndedAt:   now,
	}
	return RuntimeEventRecord{
		SchemaVersion: 1,
		Timestamp:     now,
		Event:         EventItemCompleted,
		ItemID:        itemID,
		Payload:       map[string]any{"item": item},
	}
}

// FailItem creates an item.failed event.
func FailItem(itemID, turnID, kind, detail string) RuntimeEventRecord {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item := TurnItemRecord{
		ID:      itemID,
		TurnID:  turnID,
		Kind:    kind,
		Status:  ItemStatusFailed,
		Detail:  detail,
		EndedAt: now,
	}
	return RuntimeEventRecord{
		SchemaVersion: 1,
		Timestamp:     now,
		Event:         EventItemFailed,
		ItemID:        itemID,
		Payload:       map[string]any{"item": item},
	}
}

// DeltaEvent creates an item.delta event.
func DeltaEvent(itemID, kind, delta string) RuntimeEventRecord {
	return RuntimeEventRecord{
		SchemaVersion: 1,
		Timestamp:     time.Now().UTC().Format(time.RFC3339Nano),
		Event:         EventItemDelta,
		ItemID:        itemID,
		Payload: map[string]any{
			"delta": delta,
			"kind":  kind,
		},
	}
}

// abbreviate truncates a string for summaries.
func abbreviate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// Ensure fmt is used.
var _ = fmt.Sprintf
