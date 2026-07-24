package engine

import (
	"encoding/json"
	"strings"

	"beleader/gateway/db"

	"github.com/sashabaranov/go-openai"
)

// BuildMessages reads messages from DB and returns cleaned, ordered
// []openai.ChatCompletionMessage ready for LLM consumption.
func BuildMessages(database *db.DB, thread *db.Thread, sysPrompt, turnMeta string, tools []openai.Tool, visionEnabled bool) ([]openai.ChatCompletionMessage, error) {
	msgs := []openai.ChatCompletionMessage{
		{Role: "system", Content: sysPrompt},
	}

	afterID := thread.ContextStartID
	pinnedSet := map[int64]bool{}
	for _, id := range db.ParsePinnedIDs(thread.PinnedIDs) {
		pinnedSet[id] = true
	}

	dbMsgs, err := database.GetMessages(thread.ID, 0, 100000)
	if err != nil {
		return nil, err
	}

	for _, dm := range dbMsgs {
		if dm.ID <= afterID && !pinnedSet[dm.ID] {
			continue
		}
		if dm.Kind == "error" {
			continue
		}
		role := messageRole(dm.Kind)
		if role == "assistant" && dm.Content == "" && (dm.ToolCalls == "[]" || dm.ToolCalls == "") {
			continue
		}
		msg := openai.ChatCompletionMessage{
			Role:             role,
			Content:          dm.Content,
			ReasoningContent: dm.ReasoningContent,
		}
		if dm.MultiContent != "" {
			var parts []openai.ChatMessagePart
			if err := json.Unmarshal([]byte(dm.MultiContent), &parts); err == nil {
				if !visionEnabled {
					var texts []string
					var filtered []openai.ChatMessagePart
					for _, p := range parts {
						if p.Type == openai.ChatMessagePartTypeText {
							texts = append(texts, p.Text)
							filtered = append(filtered, p)
						}
					}
					if len(filtered) == 0 {
						continue
					}
					msg.Content = strings.Join(texts, "\n")
				} else {
					msg.MultiContent = parts
					msg.Content = ""
				}
			}
		}
		if dm.ToolCalls != "[]" && dm.ToolCalls != "" {
			var tcs []openai.ToolCall
			if err := json.Unmarshal([]byte(dm.ToolCalls), &tcs); err == nil {
				msg.ToolCalls = tcs
			}
		}
		if dm.ToolCallID != "" {
			msg.ToolCallID = dm.ToolCallID
			msg.Role = "tool"
			if strings.Contains(msg.Content, `"images":`) {
				var m map[string]any
				if err := json.Unmarshal([]byte(msg.Content), &m); err == nil {
					delete(m, "images")
					if stripped, err := json.Marshal(m); err == nil {
						msg.Content = string(stripped)
					}
				}
			}
		}
		msgs = append(msgs, msg)
	}

	// Clean orphaned tool calls/results.
	hasResult := map[string]bool{}
	for _, msg := range msgs {
		if msg.ToolCallID != "" {
			hasResult[msg.ToolCallID] = true
		}
	}
	hasCall := map[string]bool{}
	for _, msg := range msgs {
		for _, tc := range msg.ToolCalls {
			hasCall[tc.ID] = true
		}
	}

	var cleaned []openai.ChatCompletionMessage
	for _, msg := range msgs {
		if len(msg.ToolCalls) > 0 {
			valid := []openai.ToolCall{}
			for _, tc := range msg.ToolCalls {
				if hasResult[tc.ID] {
					valid = append(valid, tc)
				}
			}
			if len(valid) == 0 {
				continue
			}
			msg.ToolCalls = valid
		}
		if msg.ToolCallID != "" && !hasCall[msg.ToolCallID] {
			continue
		}
		cleaned = append(cleaned, msg)
	}

	// Reorder: tool results immediately follow their tool_calls.
	var reordered []openai.ChatCompletionMessage
	var deferred []openai.ChatCompletionMessage
	var pendingIDs map[string]bool
	for _, msg := range cleaned {
		if len(msg.ToolCalls) > 0 {
			if len(deferred) > 0 {
				reordered = append(reordered, deferred...)
				deferred = nil
			}
			reordered = append(reordered, msg)
			pendingIDs = make(map[string]bool)
			for _, tc := range msg.ToolCalls {
				pendingIDs[tc.ID] = true
			}
		} else if pendingIDs != nil && msg.ToolCallID != "" && pendingIDs[msg.ToolCallID] {
			reordered = append(reordered, msg)
			delete(pendingIDs, msg.ToolCallID)
			if len(pendingIDs) == 0 {
				pendingIDs = nil
				reordered = append(reordered, deferred...)
				deferred = nil
			}
		} else if pendingIDs != nil {
			deferred = append(deferred, msg)
		} else {
			reordered = append(reordered, msg)
		}
	}
	reordered = append(reordered, deferred...)

	// Append turn_meta to the latest user message (LLM only, not stored).
	if turnMeta != "" {
		for i := len(reordered) - 1; i >= 0; i-- {
			if reordered[i].Role == "user" {
				reordered[i].Content += turnMeta
				break
			}
		}
	}

	return reordered, nil
}

func messageRole(kind string) string {
	switch kind {
	case "user_message":
		return "user"
	case "agent_message", "tool_call":
		return "assistant"
	case "tool_result":
		return "tool"
	default:
		return "user"
	}
}

// ToolDefsToOpenAI converts ToolDefs to openai.Tool list.
func ToolDefsToOpenAI(defs []ToolDef) []openai.Tool {
	tools := make([]openai.Tool, len(defs))
	for i, d := range defs {
		params := map[string]any{"type": "object"}
		if d.Parameters != nil {
			if p, ok := d.Parameters["properties"]; ok {
				params["properties"] = p
			}
			if required, ok := d.Parameters["required"]; ok {
				params["required"] = required
			}
		}
		tools[i] = openai.Tool{
			Type: "function",
			Function: &openai.FunctionDefinition{
				Name:        d.Name,
				Description: d.Description,
				Parameters:  params,
			},
		}
	}
	return tools
}
