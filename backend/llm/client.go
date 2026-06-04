package llm

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
)

// LogWriter receives LLM request/response logs. Defaults to os.Stdout.
// Set to a file or lumberjack logger to redirect.
var LogWriter io.Writer = os.Stdout

type Client struct {
	*openai.Client
	model string
}

func New(baseURL, apiKey, model string) *Client {
	cfg := openai.DefaultConfig(apiKey)
	cfg.BaseURL = baseURL
	return &Client{
		Client: openai.NewClientWithConfig(cfg),
		model:  model,
	}
}

func (c *Client) Model() string {
	return c.model
}

func (c *Client) Chat(ctx context.Context, messages []openai.ChatCompletionMessage, tools []openai.Tool, stream bool) (*openai.ChatCompletionResponse, error) {
	// ── Log request ──
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

	// ── Log response ──
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

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "")
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
