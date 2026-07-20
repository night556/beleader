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

type SSEBroker struct {
	clients map[string]map[chan string]struct{} // threadID → subscribed channels
	mu      sync.RWMutex
}

func NewSSEBroker() *SSEBroker {
	return &SSEBroker{
		clients: make(map[string]map[chan string]struct{}),
	}
}

// Subscribe returns a channel that receives SSE events for the given thread.
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

// Unsubscribe removes the channel for the given thread and closes it.
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

// Broadcast sends an event to all channels subscribed to the given thread.
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

// OnSessionEvent implements SessionObserver.
func (b *SSEBroker) OnSessionEvent(event SessionEvent) {
	b.Broadcast(event.SessionID, SSEEvent{Type: event.Type, Payload: event.Data})
}
