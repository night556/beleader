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
	clients    map[chan string]struct{}
	mu         sync.RWMutex
	register   chan chan string
	unregister chan chan string
}

func NewSSEBroker() *SSEBroker {
	b := &SSEBroker{
		clients:    make(map[chan string]struct{}),
		register:   make(chan chan string),
		unregister: make(chan chan string),
	}
	go b.run()
	return b
}

func (b *SSEBroker) run() {
	for {
		select {
		case ch := <-b.register:
			b.mu.Lock()
			b.clients[ch] = struct{}{}
			b.mu.Unlock()
		case ch := <-b.unregister:
			b.mu.Lock()
			delete(b.clients, ch)
			close(ch)
			b.mu.Unlock()
		}
	}
}

func (b *SSEBroker) Subscribe() chan string {
	ch := make(chan string, 64)
	b.register <- ch
	return ch
}

func (b *SSEBroker) Unsubscribe(ch chan string) {
	b.unregister <- ch
}

func (b *SSEBroker) Broadcast(event SSEEvent) {
	payloadJSON, err := json.Marshal(event.Payload)
	if err != nil {
		return
	}
	msg := fmt.Sprintf("event: %s\ndata: %s\n\n", event.Type, string(payloadJSON))

	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

// OnSessionEvent implements SessionObserver.
func (b *SSEBroker) OnSessionEvent(event SessionEvent) {
	var m map[string]any
	switch v := event.Data.(type) {
	case map[string]any:
		m = v
	}
	if m != nil {
		if _, has := m["session_id"]; !has && event.SessionID != "" {
			m["session_id"] = event.SessionID
		}
	} else if event.SessionID != "" {
		event.Data = map[string]any{"session_id": event.SessionID}
	}
	b.Broadcast(SSEEvent{Type: event.Type, Payload: event.Data})
}
