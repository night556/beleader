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
	buffers map[string][]string                 // threadID → buffered msgs while no subscribers
	mu      sync.Mutex
}

func NewSSEBroker() *SSEBroker {
	return &SSEBroker{
		clients: make(map[string]map[chan string]struct{}),
		buffers: make(map[string][]string),
	}
}

// Subscribe returns a channel that receives SSE events for the given thread.
// If events were buffered while no subscriber was connected, they are flushed
// to the new channel before returning.
func (b *SSEBroker) Subscribe(threadID string) chan string {
	ch := make(chan string, 64)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.clients[threadID] == nil {
		b.clients[threadID] = make(map[chan string]struct{})
	}
	b.clients[threadID][ch] = struct{}{}
	// Flush buffered messages to this subscriber
	if msgs, ok := b.buffers[threadID]; ok && len(msgs) > 0 {
		for _, msg := range msgs {
			select {
			case ch <- msg:
			default:
			}
		}
		delete(b.buffers, threadID)
	}
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
// If there are no subscribers, the message is buffered so the first
// subscriber will receive it. On turn.completed with no subscribers,
// the buffer is cleared — the turn is done and history will be loaded
// via getMessages on the next subscribe.
func (b *SSEBroker) Broadcast(threadID string, event SSEEvent) {
	payloadJSON, err := json.Marshal(event.Payload)
	if err != nil {
		return
	}
	msg := fmt.Sprintf("event: %s\ndata: %s\n\n", event.Type, string(payloadJSON))

	b.mu.Lock()
	defer b.mu.Unlock()

	subscribers := b.clients[threadID]
	if len(subscribers) == 0 {
		if event.Type == "turn.completed" {
			delete(b.buffers, threadID)
			return
		}
		b.buffers[threadID] = append(b.buffers[threadID], msg)
		return
	}

	for ch := range subscribers {
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
