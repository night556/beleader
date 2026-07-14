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
		ThreadID:    threadID,
		Kind:        "user_message",
		Content:     message,
		MultiContent: encodeMultiContent(images),
	})

	// Send turn to Runtime.
	resp, err := h.Runtime.SendTurn(threadID, TurnRequest{
		Message: message,
		Images:  images,
	})
	if err != nil {
		log.Printf("[session] %s: send turn error: %v", threadID, err)
		h.Notify(SessionEvent{Type: "error", SessionID: threadID, Data: map[string]any{"message": err.Error()}})
		return
	}
	defer resp.Body.Close()

	h.Notify(SessionEvent{Type: "turn_started", SessionID: threadID, Data: threadID})

	// Parse SSE events from Runtime and relay.
	ParseSSEStream(resp.Body, func(eventType string, payload map[string]any) {
		ev := SessionEvent{Type: eventType, SessionID: threadID, Data: payload}

		switch eventType {
		case "turn_started", "turn_complete", "turn_aborted":
			h.Notify(ev)

		case "response_start":
			h.Notify(ev)

		case "response_delta":
			h.Notify(ev)

		case "response_end":
			// Persist final assistant/content message.
			content, _ := payload["content"].(string)
			kind, _ := payload["kind"].(string)
			if kind == "" {
				kind = "agent_message"
			}
			h.DB.InsertMessage(&db.Message{
				ThreadID: threadID,
				Kind:     kind,
				Content:  content,
			})
			h.Notify(ev)

		case "tool_call_start":
			// Persist tool call as a message.
			toolName, _ := payload["tool_name"].(string)
			args, _ := payload["arguments"].(string)
			tcsJSON, _ := json.Marshal([]map[string]any{{
				"id":   payload["response_id"],
				"type": "function",
				"function": map[string]any{
					"name":      toolName,
					"arguments": args,
				},
			}})
			h.DB.InsertMessage(&db.Message{
				ThreadID:  threadID,
				Kind:      "tool_call",
				ToolCalls: string(tcsJSON),
			})
			h.Notify(ev)

		case "tool_call_result":
			// Persist tool result.
			output, _ := payload["output"].(string)
			h.DB.InsertMessage(&db.Message{
				ThreadID:   threadID,
				Kind:       "tool_result",
				Content:    output,
				ToolCallID: payload["response_id"].(string),
			})
			h.Notify(ev)

		case "exec_command_begin", "exec_command_output_delta", "exec_command_end":
			h.Notify(ev)

		case "error":
			h.Notify(ev)
			msg, _ := payload["message"].(string)
			h.DB.InsertMessage(&db.Message{
				ThreadID: threadID,
				Kind:     "error",
				Content:  msg,
			})
		}
	})

	_ = ctx
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
