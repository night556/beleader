package tools

import (
	"context"
	"encoding/json"

	"iamhuman/backend/session"

	"github.com/sashabaranov/go-openai"
)

var speakTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name: "speak",
		Description: "Speak text aloud using TTS. Use for spoken confirmations and status updates. When the user asks you to read something, pass the content directly. Strip markdown and code blocks before speaking. Set continue to false when this spoken reply is the final answer and no further action is needed.",
		Parameters:  mkSpeakParams(),
	},
}

func mkSpeakParams() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "Text to speak aloud to the user.",
			},
			"continue": map[string]any{
				"type":        "boolean",
				"description": "Set to false if this is the final response and no further LLM calls are needed. Default true.",
			},
		},
		"required": []string{"text"},
	}
}

// RegisterSpeakTool registers the speak handler. Call during init.
func RegisterSpeakTool(mgr *session.Manager) {
	mgr.RegisterTool("speak", speakHandler)
}

func speakHandler(ctx context.Context, args string) *session.ToolResult {
	var p struct {
		Text     string `json:"text"`
		Continue *bool  `json:"continue,omitempty"`
	}
	json.Unmarshal([]byte(args), &p)

	if p.Text == "" {
		return &session.ToolResult{Error: "text is required"}
	}

	if notifyContent != nil {
		notifyContent("speaking", map[string]any{
			"text": p.Text,
		})
	}

	result := &session.ToolResult{Content: "Spoken."}
	if p.Continue != nil && !*p.Continue {
		f := false
		result.ShouldContinue = &f
	}
	return result
}
