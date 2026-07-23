package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"beleader/gateway/db"
	"beleader/gateway/llm"

	"github.com/google/uuid"
	"github.com/pkoukk/tiktoken-go"
	"github.com/sashabaranov/go-openai"
)

// Engine runs the LLM agent loop on threads.
type Engine struct {
	DB         *db.DB
	LLM        *llm.Client
	ToolRouter ToolRouter
}

// ToolRouter routes tool calls to local or remote executors.
type ToolRouter interface {
	Execute(ctx context.Context, thread *db.Thread, tc openai.ToolCall) *ToolResult
}

// NewEngine creates a new Engine.
func NewEngine(database *db.DB, llmClient *llm.Client, router ToolRouter) *Engine {
	return &Engine{DB: database, LLM: llmClient, ToolRouter: router}
}

// RunLoop runs the LLM agent loop on the thread.
func (e *Engine) RunLoop(
	ctx context.Context,
	thread *db.Thread,
	sysPrompt string,
	turnMeta string,
	userContent string,
	images []string,
	toolList []openai.Tool,
	llmClient *llm.Client,
	modelContextLimit int,
	visionEnabled bool,
	reasoningEffort string,
	emit ProgressCallback,
) (*LoopResult, error) {
	turnID := "turn_" + uuid.New().String()[:8]
	rounds := 0
	var turnUsage TokenUsage
	lastPromptTokens := 0

	// Insert user message if provided.
	if userContent != "" {
		msg := &db.Message{
			ThreadID: thread.ID,
			TurnID:   turnID,
			Kind:     "user_message",
			Content:  userContent,
		}
		if len(images) > 0 {
			parts := make([]map[string]any, 0, len(images)+1)
			if userContent != "" {
				parts = append(parts, map[string]any{"type": "text", "text": userContent})
			}
			for _, img := range images {
				parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]string{"url": img}})
			}
			b, _ := json.Marshal(parts)
			msg.MultiContent = string(b)
		}
		e.DB.InsertMessage(msg)
	}

	// Emit turn.started
	emit("turn.started", turnID, "", map[string]any{
		"turn": map[string]any{"id": turnID, "status": "in_progress"},
	})

	for {
		select {
		case <-ctx.Done():
			emit("turn.completed", turnID, "", map[string]any{"status": "interrupted"})
			return &LoopResult{Stopped: true, Rounds: rounds, Usage: turnUsage}, nil
		default:
		}

		rounds++

		// Reload thread from DB to get latest state (context_start_id, pinned_ids, etc.)
		thread, err := e.DB.GetThread(thread.ID)
		if err != nil {
			return nil, err
		}

		msgs, err := BuildMessages(e.DB, thread, sysPrompt, turnMeta, toolList, visionEnabled)
		if err != nil {
			return nil, err
		}

		// Context compression
		if thread.MaxContextPct > 0 && lastPromptTokens > 0 && modelContextLimit > 0 {
			tokenPct := lastPromptTokens * 100 / modelContextLimit
			if tokenPct > thread.MaxContextPct {
				pinned := computePinWindow(e.DB, thread.ID)
				if len(pinned) > 0 {
					e.DB.UpdateThread(thread.ID, map[string]any{
						"context_start_id": pinned[0] - 1,
						"pinned_ids":       db.MarshalPinnedIDs(pinned),
					})
				}
				_, compErr := e.compress(ctx, thread, llmClient, turnID)
				if compErr == nil {
					beforeTokens := 0
					for _, m := range msgs {
						beforeTokens += countTokens(m.Content)
					}
					msgs, _ = BuildMessages(e.DB, thread, sysPrompt, turnMeta, toolList, visionEnabled)
					afterTokens := 0
					for _, m := range msgs {
						afterTokens += countTokens(m.Content)
					}
					emit("context.compressed", turnID, "", map[string]any{
						"before_tokens": beforeTokens,
						"after_tokens":  afterTokens,
					})
				}
			}
		}

		// LLM call
		agentItemID := "item_" + uuid.New().String()[:8]
		emit("item.started", turnID, agentItemID, map[string]any{
			"item": map[string]any{"id": agentItemID, "kind": "agent_message"},
		})

		resp, err := llmClient.ChatStream(ctx, msgs, toolList, func(delta string) error {
			emit("item.delta", turnID, agentItemID, map[string]any{"delta": delta, "kind": "agent_message"})
			return nil
		}, func(reasoningDelta string) error {
			emit("item.delta", turnID, agentItemID, map[string]any{"delta": reasoningDelta, "kind": "thinking"})
			return nil
		}, reasoningEffort)
		if err != nil {
			if ctx.Err() != nil {
				emit("item.completed", turnID, agentItemID, map[string]any{
					"item": map[string]any{"id": agentItemID, "kind": "agent_message", "content": ""},
				})
				emit("turn.completed", turnID, "", map[string]any{"status": "interrupted"})
				return &LoopResult{Stopped: true, Rounds: rounds, Usage: turnUsage}, nil
			}
			emit("item.failed", turnID, agentItemID, map[string]any{
				"item": map[string]any{"id": agentItemID, "kind": "agent_message", "detail": err.Error()},
			})
			emit("turn.completed", turnID, "", map[string]any{"status": "completed"})
			return &LoopResult{Completed: false, Rounds: rounds, Usage: turnUsage, Error: err.Error()}, nil
		}

		if len(resp.Choices) == 0 {
			emit("item.completed", turnID, agentItemID, map[string]any{
				"item": map[string]any{"id": agentItemID, "kind": "agent_message", "content": ""},
			})
			emit("turn.completed", turnID, "", map[string]any{"status": "completed"})
			return &LoopResult{Completed: true, Rounds: rounds, Usage: turnUsage}, nil
		}

		choice := resp.Choices[0]
		assistantMsg := choice.Message

		if len(assistantMsg.ToolCalls) == 0 && strings.TrimSpace(assistantMsg.Content) == "" {
			emit("item.completed", turnID, agentItemID, map[string]any{
				"item": map[string]any{"id": agentItemID, "kind": "agent_message", "content": ""},
			})
			continue
		}

		// Store agent message
		msgKind := "agent_message"
		tcsJSON := "[]"
		if len(assistantMsg.ToolCalls) > 0 {
			msgKind = "tool_call"
			tcsBytes, _ := json.Marshal(assistantMsg.ToolCalls)
			tcsJSON = string(tcsBytes)
		}
		usageJSON := ""
		if resp.Usage.TotalTokens > 0 {
			u := TokenUsage{
				Prompt: resp.Usage.PromptTokens, Completion: resp.Usage.CompletionTokens,
				Total: resp.Usage.TotalTokens,
			}
			if resp.Usage.PromptTokensDetails != nil {
				u.Cached = resp.Usage.PromptTokensDetails.CachedTokens
			}
			usageJSON = MarshalJSON(u)
			turnUsage.Prompt += u.Prompt
			turnUsage.Completion += u.Completion
			turnUsage.Total += u.Total
			turnUsage.Cached += u.Cached
			lastPromptTokens = u.Prompt
		}

		e.DB.InsertMessage(&db.Message{
			ThreadID:         thread.ID,
			TurnID:           turnID,
			Kind:             msgKind,
			Content:          assistantMsg.Content,
			ToolCalls:        tcsJSON,
			ReasoningContent: assistantMsg.ReasoningContent,
			Usage:            usageJSON,
		})

		itemMeta := map[string]any{}
		if usageJSON != "" {
			itemMeta["usage"] = usageJSON
		}
		emit("item.completed", turnID, agentItemID, map[string]any{
			"item": map[string]any{
				"id":       agentItemID,
				"kind":     "agent_message",
				"content":  assistantMsg.Content,
				"metadata": itemMeta,
			},
		})

		if len(assistantMsg.ToolCalls) == 0 {
			// Update thread total tokens
			e.DB.UpdateThread(thread.ID, map[string]any{"total_tokens": thread.TotalTokens + turnUsage.Total})
			emit("turn.completed", turnID, "", map[string]any{"status": "completed", "usage": MarshalJSON(turnUsage)})
			return &LoopResult{Completed: true, Rounds: rounds, Usage: turnUsage, Content: assistantMsg.Content}, nil
		}

		// Execute tool calls
		var shouldStop bool
		for _, tc := range assistantMsg.ToolCalls {
			if ctx.Err() != nil {
				emit("turn.completed", turnID, "", map[string]any{"status": "interrupted"})
				return &LoopResult{Stopped: true, Rounds: rounds, Usage: turnUsage}, nil
			}

			toolItemID := "item_" + uuid.New().String()[:8]
			toolMeta := map[string]any{
				"tool_use_id": tc.ID,
				"tool_name":   tc.Function.Name,
				"arguments":   tc.Function.Arguments,
			}
			emit("item.started", turnID, toolItemID, map[string]any{
				"item": map[string]any{
					"id":       toolItemID,
					"kind":     "tool_call",
					"summary":  tc.Function.Name,
					"metadata": toolMeta,
				},
			})

			// Reload thread (it may have been updated by spawn_worker etc.)
			currentThread, _ := e.DB.GetThread(thread.ID)
			result := e.ToolRouter.Execute(ctx, currentThread, tc)
			if result.ShouldContinue != nil && !*result.ShouldContinue {
				shouldStop = true
			}

			// Store tool result
			dbContent := result.Content
			if result.Error != "" {
				dbContent = result.Content
				if dbContent == "" {
					dbContent = result.Error
				}
			}
			dbResult := map[string]any{"content": dbContent}
			if result.Error != "" {
				dbResult["error"] = result.Error
			}
			dbJSON, _ := json.Marshal(dbResult)

			e.DB.InsertMessage(&db.Message{
				ThreadID:   thread.ID,
				TurnID:     turnID,
				Kind:       "tool_result",
				Content:    string(dbJSON),
				ToolCallID: tc.ID,
			})

			emit("item.completed", turnID, toolItemID, map[string]any{
				"item": map[string]any{
					"id":       toolItemID,
					"kind":     "tool_call",
					"detail":   string(dbJSON),
					"metadata": toolMeta,
				},
			})

			// Inject images if vision enabled
			if visionEnabled && len(result.Images) > 0 {
				e.injectImageMessage(thread.ID, result.Images, "Screenshot", turnID)
			}
		}

		if shouldStop {
			e.DB.UpdateThread(thread.ID, map[string]any{"total_tokens": thread.TotalTokens + turnUsage.Total})
			emit("turn.completed", turnID, "", map[string]any{"status": "completed", "usage": MarshalJSON(turnUsage)})
			return &LoopResult{Completed: true, Rounds: rounds, Usage: turnUsage}, nil
		}
	}
}

// Compress compresses the conversation history using the LLM.
func (e *Engine) compress(ctx context.Context, thread *db.Thread, llmClient *llm.Client, turnID string) (string, error) {
	msgs, err := BuildMessages(e.DB, thread, CompressPrompt, "", nil, false)
	if err != nil {
		return "", err
	}

	resp, err := llmClient.Chat(ctx, msgs, nil, false)
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from LLM")
	}

	summary := resp.Choices[0].Message.Content
	content := "[System] Context compressed\n\n" + summary
	e.DB.InsertMessage(&db.Message{
		ThreadID: thread.ID,
		TurnID:   turnID,
		Kind:     "notice",
		Content:  content,
	})
	return summary, nil
}

func (e *Engine) injectImageMessage(threadID string, images []string, label string, turnID string) {
	parts := []map[string]any{
		{"type": "text", "text": label},
	}
	for _, img := range images {
		parts = append(parts, map[string]any{
			"type":      "image_url",
			"image_url": map[string]string{"url": img},
		})
	}
	b, _ := json.Marshal(parts)
	e.DB.InsertMessage(&db.Message{
		ThreadID:     threadID,
		TurnID:       turnID,
		Kind:         "user_message",
		Content:      label,
		MultiContent: string(b),
	})
}

// computePinWindow scans messages from the end and returns IDs of all messages
// that fall within the last PinTurnCount user turns.
func computePinWindow(database *db.DB, threadID string) []int64 {
	const pinTurnCount = 4
	msgs, _ := database.GetMessages(threadID, 0)
	if len(msgs) == 0 {
		return nil
	}
	userCount := 0
	cutoff := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Kind == "user_message" {
			userCount++
			if userCount >= pinTurnCount {
				cutoff = i
				break
			}
		}
	}
	var ids []int64
	for i := cutoff; i < len(msgs); i++ {
		ids = append(ids, msgs[i].ID)
	}
	return ids
}

// ── Token estimation (tiktoken) ──

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
