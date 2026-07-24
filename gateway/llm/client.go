package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
)

var LogWriter io.Writer = os.Stdout

type Client struct {
	*openai.Client
	model   string
	baseURL string
	apiKey  string
}

func New(baseURL, apiKey, model string) *Client {
	cfg := openai.DefaultConfig(apiKey)
	cfg.BaseURL = baseURL
	return &Client{
		Client:  openai.NewClientWithConfig(cfg),
		model:   model,
		baseURL: baseURL,
		apiKey:  apiKey,
	}
}

func (c *Client) Model() string {
	return c.model
}

func (c *Client) ChatStream(ctx context.Context, messages []openai.ChatCompletionMessage, tools []openai.Tool, onChunk func(deltaContent string) error, onReasoningChunk func(deltaReasoning string) error, onThinkingDone func(), reasoningEffort string) (*openai.ChatCompletionResponse, error) {
	var toolNames []string
	for _, t := range tools {
		if t.Function != nil {
			toolNames = append(toolNames, t.Function.Name)
		}
	}
	fmt.Fprintf(LogWriter, "\n%s\n", strings.Repeat("━", 60))
	fmt.Fprintf(LogWriter, "[LLM REQ] %s | model=%s | msgs=%d | tools=%d [%s] (stream)\n",
		time.Now().Format("15:04:05"),
		c.model,
		len(messages),
		len(toolNames),
		strings.Join(toolNames, ", "))

	for i, m := range messages {
		content := truncate(m.Content, 200)
		if content == "" && len(m.MultiContent) > 0 {
			var parts []string
			for _, p := range m.MultiContent {
				switch p.Type {
				case "text":
					parts = append(parts, truncate(p.Text, 100))
				case "image_url":
					parts = append(parts, "[image]")
				default:
					parts = append(parts, "["+string(p.Type)+"]")
				}
			}
			content = strings.Join(parts, " ")
		}
		tcInfo := ""
		if len(m.ToolCalls) > 0 {
			var tcNames []string
			for _, tc := range m.ToolCalls {
				var args string
				if tc.Function.Arguments != "" {
					args = truncate(tc.Function.Arguments, 80)
				}
				tcNames = append(tcNames, fmt.Sprintf("%s(%s)", tc.Function.Name, args))
			}
			tcInfo = fmt.Sprintf(" [tc:%d %s]", len(m.ToolCalls), strings.Join(tcNames, "; "))
		}
		fmt.Fprintf(LogWriter, "  [%d] %-9s %s%s\n", i, m.Role, content, tcInfo)
	}

	start := time.Now()

	// Build request body — use map for custom provider fields.
	reqBody := map[string]any{
		"model":    c.model,
		"messages": messages,
		"tools":    tools,
		"stream":   true,
		"stream_options": map[string]any{
			"include_usage": true,
		},
	}
	applyReasoningEffort(reqBody, reasoningEffort, detectProvider(c.baseURL))

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		fmt.Fprintf(LogWriter, "[LLM ERR] %s | elapsed=%v | %v\n", time.Now().Format("15:04:05"), time.Since(start), err)
		fmt.Fprintf(LogWriter, "%s\n\n", strings.Repeat("━", 60))
		return nil, fmt.Errorf("chat completion stream: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(httpResp.Body, 4096))
		fmt.Fprintf(LogWriter, "[LLM ERR] %s | elapsed=%v | HTTP %d: %s\n", time.Now().Format("15:04:05"), time.Since(start), httpResp.StatusCode, string(body))
		fmt.Fprintf(LogWriter, "%s\n\n", strings.Repeat("━", 60))
		return nil, fmt.Errorf("chat completion stream: HTTP %d: %s", httpResp.StatusCode, string(body))
	}

	var fullContent string
	var reasoningContent string
	tcAcc := make(map[int]*openai.ToolCall)
	var usage openai.Usage
	var thinkingDone bool

	scanner := bufio.NewScanner(httpResp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk openai.ChatCompletionStreamResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if chunk.Usage != nil && chunk.Usage.TotalTokens > 0 {
			usage = *chunk.Usage
		}

		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta

		if delta.Content != "" {
			// First content delta after reasoning → thinking phase is done
			if !thinkingDone && reasoningContent != "" && onThinkingDone != nil {
				thinkingDone = true
				onThinkingDone()
			}
			fullContent += delta.Content
			if onChunk != nil {
				if err := onChunk(delta.Content); err != nil {
					return nil, err
				}
			}
		}

		if delta.ReasoningContent != "" {
			reasoningContent += delta.ReasoningContent
			if onReasoningChunk != nil {
				if err := onReasoningChunk(delta.ReasoningContent); err != nil {
					return nil, err
				}
			}
		}

		for _, tcDelta := range delta.ToolCalls {
			idx := 0
			if tcDelta.Index != nil {
				idx = *tcDelta.Index
			}
			if tcAcc[idx] == nil {
				tcAcc[idx] = &openai.ToolCall{ID: tcDelta.ID, Type: tcDelta.Type}
			}
			if tcDelta.ID != "" {
				tcAcc[idx].ID = tcDelta.ID
			}
			if tcDelta.Function.Name != "" {
				tcAcc[idx].Function.Name = tcDelta.Function.Name
			}
			tcAcc[idx].Function.Arguments += tcDelta.Function.Arguments
		}
	}

	elapsed := time.Since(start)

	var toolCalls []openai.ToolCall
	for i := 0; i < len(tcAcc); i++ {
		if tc, ok := tcAcc[i]; ok {
			toolCalls = append(toolCalls, *tc)
		}
	}

	fmt.Fprintf(LogWriter, "[LLM RES] %s | elapsed=%v (stream)\n", time.Now().Format("15:04:05"), elapsed)
	if fullContent != "" {
		fmt.Fprintf(LogWriter, "  content: %s\n", truncate(fullContent, 300))
	}
	if len(toolCalls) > 0 {
		for _, tc := range toolCalls {
			fmt.Fprintf(LogWriter, "  tool_call: %s(%s)\n", tc.Function.Name, truncate(tc.Function.Arguments, 150))
		}
	}
	if usage.TotalTokens > 0 {
		fmt.Fprintf(LogWriter, "  tokens: %d (p:%d c:%d)\n", usage.TotalTokens, usage.PromptTokens, usage.CompletionTokens)
	}
	fmt.Fprintf(LogWriter, "%s\n", strings.Repeat("━", 60))

	return &openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{Message: openai.ChatCompletionMessage{Content: fullContent, ReasoningContent: reasoningContent, ToolCalls: toolCalls}},
		},
		Usage: usage,
	}, nil
}

func (c *Client) Chat(ctx context.Context, messages []openai.ChatCompletionMessage, tools []openai.Tool, stream bool) (*openai.ChatCompletionResponse, error) {
	var toolNames []string
	for _, t := range tools {
		if t.Function != nil {
			toolNames = append(toolNames, t.Function.Name)
		}
	}
	fmt.Fprintf(LogWriter, "\n%s\n", strings.Repeat("━", 60))
	fmt.Fprintf(LogWriter, "[LLM REQ] %s | model=%s | msgs=%d | tools=%d [%s]\n",
		time.Now().Format("15:04:05"),
		c.model,
		len(messages),
		len(toolNames),
		strings.Join(toolNames, ", "))

	for i, m := range messages {
		content := truncate(m.Content, 200)
		if content == "" && len(m.MultiContent) > 0 {
			var parts []string
			for _, p := range m.MultiContent {
				switch p.Type {
				case "text":
					parts = append(parts, truncate(p.Text, 100))
				case "image_url":
					parts = append(parts, "[image]")
				default:
					parts = append(parts, "["+string(p.Type)+"]")
				}
			}
			content = strings.Join(parts, " ")
		}
		tcInfo := ""
		if len(m.ToolCalls) > 0 {
			var tcNames []string
			for _, tc := range m.ToolCalls {
				var args string
				if tc.Function.Arguments != "" {
					args = truncate(tc.Function.Arguments, 80)
				}
				tcNames = append(tcNames, fmt.Sprintf("%s(%s)", tc.Function.Name, args))
			}
			tcInfo = fmt.Sprintf(" [tc:%d %s]", len(m.ToolCalls), strings.Join(tcNames, "; "))
		}
		fmt.Fprintf(LogWriter, "  [%d] %-9s %s%s\n", i, m.Role, content, tcInfo)
	}

	start := time.Now()
	resp, err := c.Client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    c.model,
		Messages: messages,
		Tools:    tools,
		Stream:   stream,
	})
	elapsed := time.Since(start)

	if err != nil {
		fmt.Fprintf(LogWriter, "[LLM ERR] %s | elapsed=%v | %v\n", time.Now().Format("15:04:05"), elapsed, err)
		fmt.Fprintf(LogWriter, "%s\n\n", strings.Repeat("━", 60))
		return nil, fmt.Errorf("chat completion: %w", err)
	}

	tokens := ""
	if resp.Usage.TotalTokens > 0 {
		tokens = fmt.Sprintf(" | tokens=%d(p:%d c:%d)", resp.Usage.TotalTokens, resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
	}
	fmt.Fprintf(LogWriter, "[LLM RES] %s | elapsed=%v%s\n", time.Now().Format("15:04:05"), elapsed, tokens)

	if len(resp.Choices) > 0 {
		c := resp.Choices[0]
		if c.Message.Content != "" {
			fmt.Fprintf(LogWriter, "  content: %s\n", truncate(c.Message.Content, 300))
		}
		if len(c.Message.ToolCalls) > 0 {
			for _, tc := range c.Message.ToolCalls {
				fmt.Fprintf(LogWriter, "  tool_call: %s(%s)\n", tc.Function.Name, truncate(tc.Function.Arguments, 150))
			}
		}
		if c.FinishReason != "" {
			fmt.Fprintf(LogWriter, "  finish: %s\n", c.FinishReason)
		}
	}
	fmt.Fprintf(LogWriter, "%s\n", strings.Repeat("━", 60))

	return &resp, nil
}

func detectProvider(baseURL string) string {
	u := strings.ToLower(baseURL)
	switch {
	case strings.Contains(u, "api.deepseek.com"):
		return "deepseek"
	case strings.Contains(u, "api.openai.com"):
		return "openai"
	case strings.Contains(u, "api.anthropic.com"):
		return "anthropic"
	case strings.Contains(u, "generativelanguage.googleapis.com"):
		return "gemini"
	default:
		return "openai"
	}
}

func applyReasoningEffort(body map[string]any, effort, provider string) {
	if effort == "" || effort == "off" {
		// Disable thinking. DeepSeek defaults to thinking enabled; other
		// providers silently ignore this parameter.
		body["thinking"] = map[string]string{"type": "disabled"}
		return
	}
	body["reasoning_effort"] = effort
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "")
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
