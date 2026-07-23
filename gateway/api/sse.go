package api

import (
	"encoding/json"
	"fmt"
	"sync"
)

type SSEEvent struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

// SSEBroker is a simple pub/sub for live SSE events.
// Events are always persisted to DB (with auto-increment seq) by the
// emit callback. On SSE connect, missed events are replayed from DB.
// The broker only delivers events that arrive after subscription.
type SSEBroker struct {
	clients map[string]map[chan string]struct{} // threadID → subscribed channels
	mu      sync.RWMutex
}

func NewSSEBroker() *SSEBroker {
	return &SSEBroker{
		clients: make(map[string]map[chan string]struct{}),
	}
}

func (b *SSEBroker) Subscribe(threadID string) chan string {
	ch := make(chan string, 64)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.clients[threadID] == nil {
		b.clients[threadID] = make(map[chan string]struct{})
	}
	b.clients[threadID][ch] = struct{}{}
	return ch
}

func (b *SSEBroker) Unsubscribe(threadID string, ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if m, ok := b.clients[threadID]; ok {
		delete(m, ch)
		if len(m) == 0 {
			delete(b.clients, threadID)
		}
	}
	close(ch)
}

func (b *SSEBroker) Broadcast(threadID string, event SSEEvent) {
	payloadJSON, err := json.Marshal(event.Payload)
	if err != nil {
		return
	}
	msg := fmt.Sprintf("event: %s\ndata: %s\n\n", event.Type, string(payloadJSON))

	b.mu.RLock()
	defer b.mu.RUnlock()

	for ch := range b.clients[threadID] {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (b *SSEBroker) OnSessionEvent(event SessionEvent) {
	b.Broadcast(event.SessionID, SSEEvent{Type: event.Type, Payload: event.Data})
}