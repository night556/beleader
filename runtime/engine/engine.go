package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"beleader/runtime/llm"

	"github.com/sashabaranov/go-openai"
)

// BuildMessages reads messages from the thread and returns cleaned, ordered
// []openai.ChatCompletionMessage ready for LLM consumption.
func BuildMessages(thread *Thread, afterID int64, sysPrompt string, tools []openai.Tool, visionEnabled bool) ([]openai.ChatCompletionMessage, error) {
	msgs := []openai.ChatCompletionMessage{
		{Role: "system", Content: sysPrompt},
	}

	for _, dm := range thread.GetMessages(afterID) {
		if dm.Hidden || dm.Kind == "notice" || dm.Kind == "error" {
			continue
		}
		role := messageRole(dm.Kind)
		if role == "assistant" && dm.Content == "" && len(dm.ToolCalls) == 0 {
			continue
		}
		msg := openai.ChatCompletionMessage{
			Role:             role,
			Content:          dm.Content,
			ReasoningContent: dm.ReasoningContent,
		}
		if len(dm.MultiContent) > 0 {
			parts := make([]openai.ChatMessagePart, len(dm.MultiContent))
			for i, p := range dm.MultiContent {
				parts[i] = openai.ChatMessagePart{Type: openai.ChatMessagePartType(p.Type)}
				if p.Type == "text" {
					parts[i].Text = p.Text
				} else if p.Type == "image_url" && p.ImageURL != nil {
					parts[i].ImageURL = &openai.ChatMessageImageURL{URL: p.ImageURL.URL}
				}
			}
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
		if len(dm.ToolCalls) > 0 {
			tcs := make([]openai.ToolCall, len(dm.ToolCalls))
			for i, tc := range dm.ToolCalls {
				tcs[i] = openai.ToolCall{
					ID:   tc.ID,
					Type: openai.ToolType(tc.Type),
					Function: openai.FunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
			msg.ToolCalls = tcs
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

// RunLoop runs the LLM agent loop on the thread.
func (e *Engine) RunLoop(ctx context.Context, thread *Thread, sysPrompt string, userContent string, toolList []openai.Tool, llmClient *llm.Client, modelContextLimit int, visionEnabled bool, pauseCh <-chan struct{}, interveneCh <-chan InterveneMsg, emit ProgressCallback) (*LoopResult, error) {
	rounds := 0
	lastPromptTokens := 0
	currentTotalTokens := thread.TotalTokens
	afterID := thread.ContextStartID
	responseID := ""

	if userContent != "" {
		thread.AddMessage(Message{Kind: "user_message", Content: userContent})
	}

	for {
		select {
		case <-pauseCh:
			return &LoopResult{Paused: true, Rounds: rounds}, nil
		case <-ctx.Done():
			return &LoopResult{Stopped: true, Rounds: rounds}, nil
		default:
		}

		select {
		case msg := <-interveneCh:
			if len(msg.Images) > 0 {
				parts := []MultiContentPart{}
				if msg.Message != "" {
					parts = append(parts, MultiContentPart{Type: "text", Text: msg.Message})
				}
				for _, img := range msg.Images {
					parts = append(parts, MultiContentPart{Type: "image_url", ImageURL: &struct{ URL string `json:"url"` }{URL: img}})
				}
				thread.AddMessage(Message{Kind: "user_message", Content: msg.Message, MultiContent: parts})
			} else {
				thread.AddMessage(Message{Kind: "user_message", Content: msg.Message})
			}
		default:
		}

		rounds++
		msgs, err := BuildMessages(thread, afterID, sysPrompt, toolList, visionEnabled)
		if err != nil {
			return nil, err
		}

		if thread.MaxContextPct > 0 {
			estPct := estimateContextPctOpenAI(msgs, modelContextLimit)
			if lastPromptTokens > 0 && modelContextLimit > 0 {
				tokenPct := lastPromptTokens * 100 / modelContextLimit
				if tokenPct > estPct {
					estPct = tokenPct
				}
			}
			if estPct > thread.MaxContextPct {
				lastID := thread.LastMessageID()
				_, _, compErr := e.Compress(ctx, thread, afterID, llmClient)
				if compErr == nil {
					currentTotalTokens = thread.TotalTokens
					thread.SetContextStart(lastID)
					afterID = lastID
					msgs, _ = BuildMessages(thread, afterID, sysPrompt, toolList, visionEnabled)
				}
			}
		}

		// Emit response_start.
		responseID = fmt.Sprintf("resp_%s_%d", thread.ID, rounds)
		emit(EventFrame{
			Event:      EventResponseStart,
			ResponseID: responseID,
		})

		resp, err := llmClient.ChatStream(ctx, msgs, toolList, func(delta string) error {
			emit(EventFrame{
				Event:      EventResponseDelta,
				ResponseID: responseID,
				Delta:      delta,
				Channel:    "text",
			})
			return nil
		})
		if err != nil {
			if ctx.Err() != nil {
				emit(EventFrame{
					Event:      EventResponseEnd,
					ResponseID: responseID,
				})
				return &LoopResult{Stopped: true, Rounds: rounds}, nil
			}
			emit(EventFrame{
				Event:      EventResponseEnd,
				ResponseID: responseID,
			})
			emit(EventFrame{
				Event:   EventError,
				Message: err.Error(),
			})
			return &LoopResult{Completed: false, Rounds: rounds, Error: err.Error()}, nil
		}

		if len(resp.Choices) == 0 {
			emit(EventFrame{
				Event:      EventResponseEnd,
				ResponseID: responseID,
			})
			return &LoopResult{Completed: true, Rounds: rounds}, nil
		}

		choice := resp.Choices[0]
		assistantMsg := choice.Message

		if len(assistantMsg.ToolCalls) == 0 && strings.TrimSpace(assistantMsg.Content) == "" {
			emit(EventFrame{
				Event:      EventResponseEnd,
				ResponseID: responseID,
			})
			continue
		}

		// Convert tool calls.
		tcs := make([]ToolCall, len(assistantMsg.ToolCalls))
		for i, tc := range assistantMsg.ToolCalls {
			tcs[i] = ToolCall{
				ID:   tc.ID,
				Type: string(tc.Type),
				Function: FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			}
		}

		// Store message with kind.
		msgKind := "agent_message"
		if len(tcs) > 0 {
			msgKind = "tool_call"
		}
		thread.AddMessage(Message{
			Kind:             msgKind,
			Content:          assistantMsg.Content,
			ToolCalls:        tcs,
			ReasoningContent: assistantMsg.ReasoningContent,
		})

		// Emit response_end for this response.
		emit(EventFrame{
			Event:      EventResponseEnd,
			ResponseID: responseID,
			Content:    assistantMsg.Content,
			Kind:       msgKind,
		})

		if len(assistantMsg.ToolCalls) == 0 {
			return &LoopResult{Completed: true, Rounds: rounds, Content: assistantMsg.Content}, nil
		}

		// Execute tool calls.
		var shouldStop bool
		for _, tc := range assistantMsg.ToolCalls {
			if ctx.Err() != nil {
				return &LoopResult{Stopped: true, Rounds: rounds}, nil
			}

			emit(EventFrame{
				Event:      EventToolCallStart,
				ResponseID: responseID,
				ToolName:   tc.Function.Name,
				Arguments:  tc.Function.Arguments,
			})

			toolCtx := context.WithValue(ctx, CtxKeyVisionEnabled, visionEnabled)
			toolCtx = context.WithValue(toolCtx, CtxKeyProgress, emit)
			toolCtx = context.WithValue(toolCtx, CtxKeyToolCallID, tc.ID)
			toolCtx = context.WithValue(toolCtx, CtxKeyThreadID, thread.ID)
			toolCtx = context.WithValue(toolCtx, CtxKeyWorkDir, thread.WorkspaceDir)
			toolCtx = context.WithValue(toolCtx, CtxKeyThreadDir, thread.DataDir)

			result := e.executeTool(toolCtx, ToolCall{
				ID:   tc.ID,
				Type: string(tc.Type),
				Function: FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			})
			if result.ShouldContinue != nil && !*result.ShouldContinue {
				shouldStop = true
			}

			dbResult := *result
			dbResult.Images = nil
			dbJSON, _ := json.Marshal(dbResult)
			thread.AddMessage(Message{Kind: "tool_result", Content: string(dbJSON), ToolCallID: tc.ID})

			emit(EventFrame{
				Event:      EventToolCallResult,
				ResponseID: responseID,
				ToolName:   tc.Function.Name,
				Output:     string(dbJSON),
			})

			if visionEnabled && len(result.Images) > 0 {
				label := result.ImageLabel
				if label == "" {
					label = "Screenshot"
				}
				if result.Width > 0 && result.Height > 0 {
					label = fmt.Sprintf("%s\n\nUse 0-1000 normalized coordinates over this image: (0,0)=top-left, (1000,1000)=bottom-right, (500,500)=center. Position by proportion — e.g. a button 60%% from left and 10%% from top is (600,100).",
						label)
				}
				e.injectImageMessage(thread, result.Images, label)
			}
		}

		if shouldStop {
			return &LoopResult{Completed: true, Rounds: rounds}, nil
		}

		if resp.Usage.PromptTokens > 0 {
			lastPromptTokens = resp.Usage.PromptTokens
		}
		if resp.Usage.TotalTokens > 0 {
			currentTotalTokens += int(resp.Usage.TotalTokens)
		} else {
			currentTotalTokens += estimateTokensForRoundOpenAI(msgs, assistantMsg)
		}
		thread.TotalTokens = currentTotalTokens

		if rounds >= 30 {
			thread.AddMessage(Message{Kind: "notice",
				Content: "[System] 30 rounds completed. Briefly summarize progress and remaining work, then continue."})
			rounds = 0
		}
	}
}

// Compress compresses the conversation history using the LLM and stores the summary.
func (e *Engine) Compress(ctx context.Context, thread *Thread, afterID int64, llmClient *llm.Client) (string, int64, error) {
	msgs, err := BuildMessages(thread, afterID, CompressPrompt, nil, false)
	if err != nil {
		return "", 0, err
	}

	resp, err := llmClient.Chat(ctx, msgs, nil, false)
	if err != nil {
		return "", 0, err
	}

	if len(resp.Choices) == 0 {
		return "", 0, fmt.Errorf("no response from LLM")
	}

	if resp.Usage.TotalTokens > 0 {
		thread.AddTokens(int(resp.Usage.TotalTokens))
	}

	summary := resp.Choices[0].Message.Content
	content := "[System] Context compressed\n\n" + summary
	msgID := thread.AddMessage(Message{Kind: "notice", Content: content})
	return summary, msgID, nil
}

func (e *Engine) injectImageMessage(thread *Thread, images []string, label string) {
	parts := []MultiContentPart{
		{Type: "text", Text: label},
	}
	for _, img := range images {
		parts = append(parts, MultiContentPart{
			Type:     "image_url",
			ImageURL: &struct{ URL string `json:"url"` }{URL: img},
		})
	}
	thread.AddMessage(Message{Kind: "user_message", Content: label, MultiContent: parts})
}

// ToolDefsToOpenAI converts runtime ToolDefs to openai.Tool list.
func ToolDefsToOpenAI(defs []ToolDef) []openai.Tool {
	tools := make([]openai.Tool, len(defs))
	for i, d := range defs {
		params := map[string]any{"type": "object"}
		if d.Parameters != nil {
			params["properties"] = d.Parameters["properties"]
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

func estimateContextPctOpenAI(msgs []openai.ChatCompletionMessage, limit int) int {
	total := 0
	for _, m := range msgs {
		total += len(m.Content)
		for _, tc := range m.ToolCalls {
			total += len(tc.Function.Arguments)
		}
	}
	if limit == 0 {
		return 0
	}
	pct := total * 100 / (limit * 2)
	if pct > 100 {
		pct = 100
	}
	return pct
}

func estimateTokensForRoundOpenAI(msgs []openai.ChatCompletionMessage, completion openai.ChatCompletionMessage) int {
	total := 0
	for _, m := range msgs {
		total += len(m.Content)
		for _, tc := range m.ToolCalls {
			total += len(tc.Function.Arguments)
		}
	}
	total += len(completion.Content)
	for _, tc := range completion.ToolCalls {
		total += len(tc.Function.Arguments)
	}
	return total / 2
}
