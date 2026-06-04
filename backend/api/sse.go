package api

import (
	"encoding/json"
	"sync"

	"github.com/gin-gonic/gin"
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
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	dataStr := string(data)

	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.clients {
		select {
		case ch <- dataStr:
		default:
		}
	}
}

// OnSessionEvent implements SessionObserver.
func (b *SSEBroker) OnSessionEvent(event SessionEvent) {
	if m, ok := event.Data.(map[string]any); ok {
		if _, has := m["session_id"]; !has && event.SessionID != "" {
			m["session_id"] = event.SessionID
		}
	} else if event.SessionID != "" {
		event.Data = gin.H{"session_id": event.SessionID}
	}
	b.Broadcast(SSEEvent{Type: event.Type, Payload: event.Data})
}
