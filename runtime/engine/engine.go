package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"beleader/runtime/llm"

	"github.com/pkoukk/tiktoken-go"
	"github.com/sashabaranov/go-openai"
)

// BuildMessages reads messages from the thread and returns cleaned, ordered
// []openai.ChatCompletionMessage ready for LLM consumption.
// Messages with ID > afterID are included, plus messages whose ID is in pinnedIDs.
func BuildMessages(thread *Thread, afterID int64, pinnedIDs []int64, sysPrompt string, tools []openai.Tool, visionEnabled bool) ([]openai.ChatCompletionMessage, error) {
	msgs := []openai.ChatCompletionMessage{
		{Role: "system", Content: sysPrompt},
	}

	pinnedSet := make(map[int64]bool, len(pinnedIDs))
	for _, id := range pinnedIDs {
		pinnedSet[id] = true
	}

	for _, dm := range thread.GetAllMessages() {
		if !(dm.ID > afterID || pinnedSet[dm.ID]) {
			continue
		}
		if dm.Hidden || dm.Kind == "error" {
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

	// Append turn_meta to the latest user message (LLM only, not stored).
	if thread.TurnMeta != "" {
		for i := len(reordered) - 1; i >= 0; i-- {
			if reordered[i].Role == "user" {
				reordered[i].Content += thread.TurnMeta
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

// RunLoop runs the LLM agent loop on the thread.
// turnID is the current turn identifier for item lifecycle events.
func (e *Engine) RunLoop(ctx context.Context, thread *Thread, turnID string, sysPrompt string, userContent string, toolList []openai.Tool, llmClient *llm.Client, modelContextLimit int, visionEnabled bool, emit ProgressCallback, bgCheck func() []BackgroundResult) (*LoopResult, error) {
	rounds := 0
	var turnUsage TokenUsage
	lastPromptTokens := 0
	currentTotalTokens := thread.TotalTokens
	afterID := thread.ContextStartID

	if userContent != "" {
		itemID := NewItemID()
		emit(StartItem(itemID, turnID, ItemKindUserMessage, abbreviate(userContent, 200), nil))
		thread.AddMessage(Message{Kind: "user_message", Content: userContent})
		emit(CompleteItem(itemID, turnID, ItemKindUserMessage, userContent, nil))
	}

	for {
		select {
		case <-ctx.Done():
			return &LoopResult{Stopped: true, Rounds: rounds, Usage: turnUsage}, nil
		default:
		}

		rounds++
		msgs, err := BuildMessages(thread, afterID, thread.PinnedIDs, sysPrompt, toolList, visionEnabled)
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
				// Compute pin window — last N turns stay verbatim.
				pinned := thread.ComputePinWindow()
				if len(pinned) > 0 {
					thread.ContextStartID = pinned[0] - 1
				} else {
					thread.ContextStartID = thread.LastMessageID()
				}
				thread.PinnedIDs = pinned

				_, _, compErr := e.Compress(ctx, thread, thread.ContextStartID, llmClient)
				if compErr == nil {
					currentTotalTokens = thread.TotalTokens
					thread.PruneCompressed()
					afterID = thread.ContextStartID
					msgs, _ = BuildMessages(thread, afterID, thread.PinnedIDs, sysPrompt, toolList, visionEnabled)
				}
			}
		}

		// item.started for this LLM response
		agentItemID := NewItemID()
		emit(StartItem(agentItemID, turnID, ItemKindAgentMessage, "", nil))

		resp, err := llmClient.ChatStream(ctx, msgs, toolList, func(delta string) error {
			emit(DeltaEvent(agentItemID, ItemKindAgentMessage, delta))
			return nil
			}, func(reasoningDelta string) error {
			emit(ThinkingDeltaEvent(agentItemID, reasoningDelta))
			return nil
		}, thread.Model.ReasoningEffort)
		if err != nil {
			if ctx.Err() != nil {
				emit(CompleteItem(agentItemID, turnID, ItemKindAgentMessage, "", nil))
				return &LoopResult{Stopped: true, Rounds: rounds, Usage: turnUsage}, nil
			}
			emit(FailItem(agentItemID, turnID, ItemKindAgentMessage, err.Error()))
			return &LoopResult{Completed: false, Rounds: rounds, Usage: turnUsage, Error: err.Error()}, nil
		}

		if len(resp.Choices) == 0 {
			emit(CompleteItem(agentItemID, turnID, ItemKindAgentMessage, "", nil))
			return &LoopResult{Completed: true, Rounds: rounds, Usage: turnUsage}, nil
		}

		choice := resp.Choices[0]
		assistantMsg := choice.Message

		if len(assistantMsg.ToolCalls) == 0 && strings.TrimSpace(assistantMsg.Content) == "" {
			emit(CompleteItem(agentItemID, turnID, ItemKindAgentMessage, "", nil))
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
		msgKind := ItemKindAgentMessage
		if len(tcs) > 0 {
			msgKind = ItemKindToolCall
		}
		msgUsage := &TokenUsage{
			Prompt:     resp.Usage.PromptTokens,
			Completion: resp.Usage.CompletionTokens,
			Total:      resp.Usage.TotalTokens,
		}
		if resp.Usage.PromptTokensDetails != nil {
			msgUsage.Cached = resp.Usage.PromptTokensDetails.CachedTokens
		}
		thread.AddMessage(Message{
			Kind:             msgKind,
			Content:          assistantMsg.Content,
			ToolCalls:        tcs,
			ReasoningContent: assistantMsg.ReasoningContent,
			Usage:            msgUsage,
		})
		
		// item.completed for this LLM response
		emit(CompleteItem(agentItemID, turnID, ItemKindAgentMessage, assistantMsg.Content, map[string]any{
			"usage": msgUsage,
		}))

		if len(assistantMsg.ToolCalls) == 0 {
			return &LoopResult{Completed: true, Rounds: rounds, Usage: turnUsage, Content: assistantMsg.Content}, nil
		}

		// Execute tool calls
		var shouldStop bool
		for _, tc := range assistantMsg.ToolCalls {
			if ctx.Err() != nil {
				return &LoopResult{Stopped: true, Rounds: rounds, Usage: turnUsage}, nil
			}

			toolItemID := NewItemID()
			toolMeta := map[string]any{
				"tool_use_id": tc.ID,
				"tool_name":   tc.Function.Name,
				"arguments":   tc.Function.Arguments,
			}

			// item.started for tool_call
			emit(StartItem(toolItemID, turnID, ItemKindToolCall, tc.Function.Name, toolMeta))

			toolCtx := context.WithValue(ctx, CtxKeyVisionEnabled, visionEnabled)
			toolCtx = context.WithValue(toolCtx, CtxKeyProgress, emit)
			toolCtx = context.WithValue(toolCtx, CtxKeyToolCallID, tc.ID)
			toolCtx = context.WithValue(toolCtx, CtxKeyThreadID, thread.ID)
			toolCtx = context.WithValue(toolCtx, CtxKeyTurnID, turnID)
			toolCtx = context.WithValue(toolCtx, CtxKeyItemID, toolItemID)
			toolCtx = context.WithValue(toolCtx, CtxKeyWorkDir, thread.WorkspaceDir)
			toolCtx = context.WithValue(toolCtx, CtxKeyThreadDir, thread.DataDir)
			toolCtx = context.WithValue(toolCtx, CtxKeyRestrictWorkspace, thread.RestrictWorkspace)

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

			// item.completed for tool_call
			emit(CompleteItem(toolItemID, turnID, ItemKindToolCall, string(dbJSON), toolMeta))

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

		// Inject any completed background command results as user_message.
		// (not tool_result — background commands have no matching tool_call)
		var bgResults []BackgroundResult
		if bgCheck != nil {
			bgResults = bgCheck()
		}
		for _, r := range bgResults {
			content := fmt.Sprintf("[Background command completed]\nCommand: %s\nExit code: %d\n\n%s",
				r.Command, r.ExitCode, r.Output)
			if r.Error != "" {
				content = fmt.Sprintf("[Background command completed]\nCommand: %s\nExit code: %d\nError: %s\n\n%s",
					r.Command, r.ExitCode, r.Error, r.Output)
			}
			thread.AddMessage(Message{Kind: "user_message", Content: content})
		}

		if shouldStop {
			return &LoopResult{Completed: true, Rounds: rounds, Usage: turnUsage}, nil
		}

		if len(bgResults) > 0 {
			continue
		}

		if resp.Usage.PromptTokens > 0 {
			lastPromptTokens = resp.Usage.PromptTokens
		}
		if resp.Usage.TotalTokens > 0 {
			currentTotalTokens += int(resp.Usage.TotalTokens)
			turnUsage.Prompt += resp.Usage.PromptTokens
			turnUsage.Completion += resp.Usage.CompletionTokens
			turnUsage.Total += resp.Usage.TotalTokens
			if resp.Usage.PromptTokensDetails != nil {
				turnUsage.Cached += resp.Usage.PromptTokensDetails.CachedTokens
			}
		} else {
			currentTotalTokens += estimateTokensForRoundOpenAI(msgs, assistantMsg)
		}
		thread.TotalTokens = currentTotalTokens

	}
}

// Compress compresses the conversation history using the LLM and stores the summary.
func (e *Engine) Compress(ctx context.Context, thread *Thread, afterID int64, llmClient *llm.Client) (string, int64, error) {
	msgs, err := BuildMessages(thread, afterID, nil, CompressPrompt, nil, false)
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

var (
	tokenEncOnce sync.Once
	tokenEncoder *tiktoken.Tiktoken
	tokenEncErr  error
)

func getTokenEncoder() (*tiktoken.Tiktoken, error) {
	tokenEncOnce.Do(func() {
		tokenEncoder, tokenEncErr = tiktoken.GetEncoding("cl100k_base")
	})
	return tokenEncoder, tokenEncErr
}

func countTokens(s string) int {
	if s == "" {
		return 0
	}
	enc, err := getTokenEncoder()
	if err != nil {
		return len(s) / 4
	}
	return len(enc.Encode(s, nil, nil))
}

func countMessagesTokens(msgs []openai.ChatCompletionMessage) int {
	total := 0
	for _, m := range msgs {
		total += countTokens(m.Content)
		for _, tc := range m.ToolCalls {
			total += countTokens(tc.Function.Arguments)
		}
	}
	return total
}

func estimateContextPctOpenAI(msgs []openai.ChatCompletionMessage, limit int) int {
	total := countMessagesTokens(msgs)
	if limit == 0 {
		return 0
	}
	pct := total * 100 / limit
	if pct > 100 {
		pct = 100
	}
	return pct
}

func estimateTokensForRoundOpenAI(msgs []openai.ChatCompletionMessage, completion openai.ChatCompletionMessage) int {
	total := countMessagesTokens(msgs)
	total += countTokens(completion.Content)
	for _, tc := range completion.ToolCalls {
		total += countTokens(tc.Function.Arguments)
	}
	return total
}
