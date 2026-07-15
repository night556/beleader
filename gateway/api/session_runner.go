package api

import (
	"context"
	"encoding/json"
	"log"

	"beleader/gateway/db"
)

func (h *Handler) runSession(threadID string, agent *db.Agent, message string, images []string) {
	ctx, cancel := context.WithCancel(context.Background())
	h.mu.Lock()
	h.cancelFuncs[threadID] = cancel
	h.pauseChs[threadID] = make(chan struct{}, 1)
	h.interveneChs[threadID] = make(chan struct{}, 1)
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.cancelFuncs, threadID)
		delete(h.pauseChs, threadID)
		delete(h.interveneChs, threadID)
		h.mu.Unlock()
	}()

	// Persist user message.
	h.DB.InsertMessage(&db.Message{
		ThreadID:     threadID,
		Kind:         "user_message",
		Content:      message,
		MultiContent: encodeMultiContent(images),
	})

	// Send turn to Runtime.
	resp, err := h.Runtime.SendTurn(ctx, threadID, TurnRequest{
		Message: message,
		Images:  images,
		Model:   h.buildModelMap(),
	})
	if err != nil {
		log.Printf("[session] %s: send turn error: %v", threadID, err)
		h.Notify(SessionEvent{Type: "error", SessionID: threadID, Data: map[string]any{"message": err.Error()}})
		return
	}
	defer resp.Body.Close()

	// Parse SSE events from Runtime and relay.
	ParseSSEStream(resp.Body, func(eventType string, envelope map[string]any) {
		// Extract inner payload from the RuntimeEventRecord envelope.
		payload, _ := envelope["payload"].(map[string]any)
		if payload == nil {
			payload = map[string]any{}
		}

		ev := SessionEvent{Type: eventType, SessionID: threadID, Data: payload}

		switch eventType {
		case "turn.started", "turn.completed":
			h.Notify(ev)

		case "item.started":
			item, _ := payload["item"].(map[string]any)
			if item != nil {
				kind, _ := item["kind"].(string)
				if kind == "tool_call" {
					metadata, _ := item["metadata"].(map[string]any)
					toolName, _ := metadata["tool_name"].(string)
					toolUseID, _ := metadata["tool_use_id"].(string)
					tcsJSON, _ := json.Marshal([]map[string]any{{
						"id":   toolUseID,
						"type": "function",
						"function": map[string]any{
							"name": toolName,
						},
					}})
					h.DB.InsertMessage(&db.Message{
						ThreadID:  threadID,
						Kind:      "tool_call",
						ToolCalls: string(tcsJSON),
					})
				}
			}
			h.Notify(ev)

		case "item.delta":
			h.Notify(ev)

		case "item.completed":
			item, _ := payload["item"].(map[string]any)
			if item != nil {
				kind, _ := item["kind"].(string)
				detail, _ := item["detail"].(string)
				switch kind {
				case "agent_message":
					h.DB.InsertMessage(&db.Message{
						ThreadID: threadID,
						Kind:     "agent_message",
						Content:  detail,
					})
				case "tool_call":
					metadata, _ := item["metadata"].(map[string]any)
					toolUseID, _ := metadata["tool_use_id"].(string)
					// Only persist individual tool execution results (skip batch completions).
					if toolUseID != "" {
						dbContent := detail
						if m, ok := parseJSONMap(detail); ok {
							delete(m, "images")
							if b, err := json.Marshal(m); err == nil {
								dbContent = string(b)
							}
						}
						h.DB.InsertMessage(&db.Message{
							ThreadID:   threadID,
							Kind:       "tool_result",
							Content:    dbContent,
							ToolCallID: toolUseID,
						})
					}
				}
			}
			h.Notify(ev)

		case "item.failed":
			item, _ := payload["item"].(map[string]any)
			if item != nil {
				detail, _ := item["detail"].(string)
				h.DB.InsertMessage(&db.Message{
					ThreadID: threadID,
					Kind:     "error",
					Content:  detail,
				})
			}
			h.Notify(ev)
		}
	})

	log.Printf("[session] %s: done", threadID)
}

func encodeMultiContent(images []string) string {
	if len(images) == 0 {
		return ""
	}
	parts := make([]map[string]any, len(images))
	for i, img := range images {
		parts[i] = map[string]any{
			"type":      "image_url",
			"image_url": map[string]string{"url": img},
		}
	}
	b, _ := json.Marshal(parts)
	return string(b)
}

func parseJSONMap(s string) (map[string]any, bool) {
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, false
	}
	return m, true
}
