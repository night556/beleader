package engine

import (
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Message is an in-memory message in a Thread.
type Message struct {
	ID               int64              `json:"id"`
	Kind             string             `json:"kind"` // user_message | agent_message | tool_call | tool_result
	Content          string             `json:"content"`
	ToolCalls        []ToolCall         `json:"tool_calls,omitempty"`
	ToolCallID       string             `json:"tool_call_id,omitempty"`
	MultiContent     []MultiContentPart `json:"multi_content,omitempty"`
	ReasoningContent string             `json:"reasoning_content,omitempty"`
	Hidden           bool               `json:"hidden,omitempty"`
	CreatedAt        time.Time          `json:"created_at"`
}

// ToolCall represents a tool call from the assistant.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall represents a function call within a tool call.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// MultiContentPart represents a part of a multimodal message.
type MultiContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL *struct {
		URL string `json:"url"`
	} `json:"image_url,omitempty"`
}

// ModelConfig is the LLM model configuration for a thread.
type ModelConfig struct {
	BaseURL      string `json:"base_url"`
	APIKey       string `json:"api_key"`
	Model        string `json:"model"`
	ContextLimit    int    `json:"context_limit"`
	Vision          bool   `json:"vision"`
	ReasoningEffort string `json:"reasoning_effort"`
}

// Thread represents a conversation thread.
type Thread struct {
	ID             string         `json:"id"`
	SystemPrompt   string         `json:"system_prompt"`
	Model          ModelConfig    `json:"model"`
	ToolDefs       []ToolDef      `json:"tools"`
	DataDir        string         `json:"-"` // thread data directory: {dataDir}/threads/{id}/
	WorkspaceDir   string         `json:"workspace_dir"`
	Metadata       map[string]any `json:"metadata"`
	MaxContextPct  int            `json:"max_context_pct"`
	Messages       []Message      `json:"-"` // persisted in messages.jsonl
	ContextStartID int64          `json:"context_start_id"`
	PinnedIDs      []int64        `json:"pinned_ids"`
	TotalTokens    int            `json:"total_tokens"`
	CreatedAt      time.Time      `json:"created_at"`

	// OnMessageAppend is called after a message is added to Messages.
	// Set by the server layer to persist to messages.jsonl.
	OnMessageAppend func(msg *Message)

	mu sync.RWMutex
}

// ToolDef is a lightweight tool definition passed from Gateway.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// NewThread creates a new Thread with the given configuration.
func NewThread(systemPrompt string, model ModelConfig, toolDefs []ToolDef, dataDir string, maxContextPct int, metadata map[string]any, history []Message) *Thread {
	t := &Thread{
		ID:            uuid.New().String(),
		SystemPrompt:  systemPrompt,
		Model:         model,
		ToolDefs:      toolDefs,
		DataDir:       dataDir,
		WorkspaceDir:  dataDir + "/workspace",
		Metadata:      metadata,
		MaxContextPct: maxContextPct,
		Messages:      history,
		CreatedAt:     time.Now(),
	}
	for i := range t.Messages {
		if t.Messages[i].ID == 0 {
			t.Messages[i].ID = int64(i + 1)
		}
	}
	return t
}

// AddMessage appends a message and returns its ID.
func (t *Thread) AddMessage(msg Message) int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	msg.ID = int64(len(t.Messages) + 1)
	msg.CreatedAt = time.Now()
	t.Messages = append(t.Messages, msg)

	if t.OnMessageAppend != nil {
		t.OnMessageAppend(&msg)
	}
	return msg.ID
}

// GetMessages returns messages after the given ID.
func (t *Thread) GetMessages(afterID int64) []Message {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var msgs []Message
	for _, m := range t.Messages {
		if m.ID > afterID {
			msgs = append(msgs, m)
		}
	}
	return msgs
}

// GetAllMessages returns all messages (read lock).
func (t *Thread) GetAllMessages() []Message {
	t.mu.RLock()
	defer t.mu.RUnlock()
	msgs := make([]Message, len(t.Messages))
	copy(msgs, t.Messages)
	return msgs
}

// LastMessageID returns the ID of the last message.
func (t *Thread) LastMessageID() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if len(t.Messages) == 0 {
		return 0
	}
	return t.Messages[len(t.Messages)-1].ID
}

// SetContextStart sets the context start ID (messages before this are compressed).
func (t *Thread) SetContextStart(id int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ContextStartID = id
}

// AddTokens adds to the total token count.
func (t *Thread) AddTokens(n int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.TotalTokens += n
}

// PinTurnCount is the number of recent turns to preserve during compression.
const PinTurnCount = 4

// ComputePinWindow scans messages from the end and returns IDs of all messages
// that fall within the last PinTurnCount user turns (inclusive of the turn start).
func (t *Thread) ComputePinWindow() []int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if len(t.Messages) == 0 {
		return nil
	}

	userCount := 0
	cutoff := 0
	for i := len(t.Messages) - 1; i >= 0; i-- {
		if t.Messages[i].Kind == "user_message" {
			userCount++
			if userCount >= PinTurnCount {
				cutoff = i
				break
			}
		}
	}

	var ids []int64
	for i := cutoff; i < len(t.Messages); i++ {
		ids = append(ids, t.Messages[i].ID)
	}
	return ids
}

// PruneCompressed removes messages before ContextStartID that are not in PinnedIDs.
// This frees memory after compression — the full history lives in messages.jsonl.
func (t *Thread) PruneCompressed() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.ContextStartID == 0 {
		return
	}

	pinned := make(map[int64]bool, len(t.PinnedIDs))
	for _, id := range t.PinnedIDs {
		pinned[id] = true
	}

	keep := t.Messages[:0]
	for _, m := range t.Messages {
		if m.ID >= t.ContextStartID || pinned[m.ID] || m.Kind == "notice" && strings.HasPrefix(m.Content, "[System]") {
			keep = append(keep, m)
		}
	}
	t.Messages = keep
}
