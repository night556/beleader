package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"iamhuman/backend/db"
	"iamhuman/backend/llm"

	"github.com/sashabaranov/go-openai"
)

type InterveneMsg struct {
	Message string   `json:"message"`
	Images  []string `json:"images"`
}

// CoreRules is the base system prompt shared by all agents.
const CoreRules = `## Style
Be direct and concise. Start with the answer, not an introduction.
Do not write "Sure!", "I'll help with that", or "Here's the code".
When asked a question, answer it. When writing code, skip boilerplate comments.
Prefer a single dense line over a paragraph that says the same thing.

## Tool Calls
Alongside each tool call, write one brief line describing what you are about to do.
Example: "Creating the project directory structure."

## Problem Solving
Be resourceful 閳?gather what you need. If one path is blocked, find another.
If all paths lead to the same wall, say what blocks you and what you need.

## File Operations
Work in {work_dir}. Use absolute paths.
Read a file before editing it. Use edit_file for small targeted changes, write_file for creating new files or rewriting large sections.`

// MainPrompt is appended for the Main agent.
const MainPrompt = `You are the user's primary assistant.

For simple tasks (answering questions, editing a single file, explaining code), handle them directly.

For complex tasks that require multiple steps and files (the user says "build me a blog", "create a desktop app"), call create_project with a detailed prompt describing what needs to be done. The project will have a Coordinator that orchestrates the work.

When the user just wants a new conversation context 閳?a fresh place to chat, discuss ideas, or explore a topic without a specific goal 閳?call create_project with only a title and leave the prompt empty. The Coordinator will act as a conversational partner, not start building things.

When you are unsure what the user wants, call create_project with the user's original words as the prompt. The Coordinator will figure it out.

When the user asks about a project, call get_project_status to read its STATUS.md. If STATUS.md references other documents, read those for details. Do not dig into individual Worker conversations.

Use list_projects to show the user all their projects and their current state.

You manage agent templates via list_agents, create_agent, edit_agent, and delete_agent. These are reusable role definitions that Coordinators reference when spawning Workers.`

// CoordinatorPrompt is appended for the Coordinator agent.
const CoordinatorPrompt = `You are the Coordinator of this project. You manage, plan, and orchestrate — you do not execute. Your value is in understanding what needs to be done, making good decisions, and delegating to the right Worker.

## STATUS.md maintenance
STATUS.md is the project's status entry point. It records current progress, completed and pending items, key decisions, and serves as a navigation hub pointing to the project's various documents and artifacts (requirements, design docs, technical specs, API designs, etc. — whatever the project needs, not a fixed checklist).

When to update: after every Worker completes, when the user gives new requirements, or when project state changes.

How to update:
1. If STATUS.md content is still fresh in context from a recent read or write, update from memory — don't waste tokens re-reading
2. If unsure of the current content, call read_status first
3. Use write_status to write the complete updated content
4. Organize naturally based on the actual project — a small project may be a brief progress list, a large project needs sections referencing various documents

Do NOT turn it into a log or journal. Do NOT repeat the same information. Do NOT discard important past records while updating.

## How to respond
First, judge the situation:
- **Casual chat / discussion** — the user just wants to talk. Be a conversational partner, reply naturally. Don't spawn Workers.
- **Question / advice** — the user wants to understand something. Answer directly. Use web_search or web_fetch if helpful.
- **Research** — the user needs information gathered before deciding. Either answer from your own knowledge or spawn a researcher Worker.
- **Development** — the user wants something built. Spawn a Worker.

If the task evolves (e.g. conversation turns into development), adapt accordingly.

## Development workflow
Spawn Workers one at a time or in small batches. Wait for each Worker to finish and report back. Do NOT call intervene_worker immediately after spawning — the Worker will respond when done or blocked.

A Worker that has finished still holds its conversation context. If a follow-up task is closely related to what that Worker already did, use intervene_worker to give it the new task instead of spawning a fresh Worker that would need to re-learn the context.

## spawn_worker vs intervene_worker
Worker names are unique per project — spawn_worker will fail if the name already exists. Use list_workers to check before spawning.
- No Worker with that name exists → spawn_worker
- A Worker with that name already exists (running or idle) → intervene_worker
- Need a parallel Worker for a different task while one is busy → spawn_worker with a distinct name (e.g. "coder-backend", "coder-frontend")
- The Worker is currently running and you need a progress update → intervene_worker
- You want to stop a Worker → terminate_worker
- Only use delete_worker when explicitly asked by the user. Never delete Workers on your own judgment.

## When to re-think
If the user points out the same issue 2-3 times in a row, the current approach is fundamentally flawed. Don't keep patching — stop and re-analyze the problem. Terminate the stuck Worker, incorporate the user's feedback and your new understanding, then spawn a fresh Worker with clear instructions.

Key signals: "still not right", "happened again", "what's going on", "that's not what I meant" — these are telling you the direction is wrong, not asking you to try again harder.

## Research
You may spawn a researcher Worker when you need to understand something before acting. Research is a tactic, not a separate mode — use it when the path forward is unclear, skip it when straightforward.

## Asking for help
When you need the user to make a decision or clarify requirements, state clearly what you need and what the options are.

## Writing a task for a Worker
When you call spawn_worker, the task parameter becomes the Worker's first message. A good task:
- States the goal clearly and what "done" looks like
- Points to relevant files the Worker should read first (e.g. "Read shared/plan.md before starting")
- Specifies constraints (language, libraries, file paths, coding style)
- Is self-contained — the Worker should understand the job from this message alone`

// WorkerBasePrompt is appended for all Worker agents.
const WorkerBasePrompt = `You are a Worker in a project team. The Coordinator assigns you tasks via messages. Focus on executing the task you are given.

When you have completed your task, your final message should summarize what you accomplished, what files you created or changed, and any decisions you made that the Coordinator should know about.

If you discover the plan is wrong, a requirement cannot be met, or you are blocked, stop and explain the problem clearly in your response. Do not continue blindly.

If anything in the task description is unclear, ask for clarification in your response before you start working.`

// DesktopRules is appended when desktop automation tools are enabled.
const DesktopRules = `## Desktop Automation

You can control the desktop through a screenshot, analyze, and act loop. Start with a screenshot to see the current screen state. Identify the target UI elements, their positions, and any relevant surrounding context. Then take the appropriate action.

### Coordinate System
All coordinates use a normalized 0-1000 grid overlaid on the screenshot image. (0,0) is the top-left corner. (1000,1000) is the bottom-right corner. (500,500) is the exact center. These are proportional positions, not raw pixels.

For example, a button near the top-right is approximately (950, 50). A taskbar icon near the bottom-left is approximately (50, 950). Always aim for the center of your target element.

### Strategy
- Before every click, state what you are targeting and why you chose those coordinates.
- If the screen shows something different from what you expected, analyze the discrepancy and adapt. Do not repeat the same action hoping for a different result.
- After any action that changes the UI, take a screenshot to verify the result before the next step.
- For text input, prefer desktop_type_text over simulating keystrokes. It supports Chinese and Unicode characters.
- For very small targets, keyboard shortcuts are more reliable than clicking.`

// BrowserRules is appended when browser automation tools are enabled.
const BrowserRules = `## Browser Automation

### Before Acting
- Inspect page state (browser_content) before any interaction. Screenshot is for verification, not primary observation.

### After Each Action
- Verify the result 閳?"Clicked X" does not guarantee the click had effect. Compare browser_content before/after.
- If page didn't change as expected, element may be obscured, disabled, or page may have changed.

### When Blocked
- Popup/overlay detected 閳?close it before continuing. Do NOT work around it.
- Same ref fails 2-3 times 閳?abandon, try a different element or strategy.
- Element not found 閳?scroll down, then re-snapshot with browser_content.

### Interaction
- Prefer ref numbers from the latest snapshot. CSS selectors only as fallback.
- After typing into a search box, press Enter with browser_keys instead of hunting for the submit button.
- For dropdowns, use browser_select to see options before choosing.

### Extraction
- Use browser_content for text. Only screenshot as last resort for visual verification.
- Use browser_evaluate for targeted data extraction (querySelector, innerText, etc.)

### Cleanup
- When the task is complete, close open browser tabs you no longer need with browser_close. Do NOT leave tabs open 閳?they waste resources.
- If the task result should be displayed to the user (e.g., HTML content), keep that tab open.`
const CompressPrompt = `You are compressing a conversation to save context space. Your output will replace all previous messages. The assistant must be able to continue working from your summary alone.

Summarize:
- What the user asked for and what the assistant did about it
- Key information from tool outputs (file paths, errors, search results, code snippets that matter)
- What the assistant was doing right before compression

Rules:
- Preserve actionable information (file paths, error messages, relevant code)
- Discard redundant tool output, boilerplate, and noise
- Do NOT create a task plan 閳?just record what happened
- Be dense. Every sentence should carry information needed to continue.`

const MemoryExtractPrompt = `Extract reusable memories from this conversation. Facts and preferences only, not current task state.

## 閸嬪繐銈?
User's technical preferences, code style, workflow. One per line.

## 妞ゅ湱娲?
Project references (ref_id, etc.). One per line.

## 閸愬磭鐡?
Important technical or design decisions. One per line.

## 瀵板懎濮?
Things user mentioned but not done yet. One per line.

Rules:
- Only cross-conversation reusable information, ignore one-off chat
- Merge with existing memory.md, don't overwrite non-conflicting entries
- Max 10 per category, sort by importance`

type ProgressCallback func(eventType string, payload map[string]any)

type Manager struct {
	DB           *db.DB
	LLM          *llm.Client
	Config       Config
	ToolHandlers map[string]func(ctx context.Context, args string) *ToolResult
}

type Config struct {
	WorkDir       string
	MemoryPath    string
	StatePath     string
	MaxContextPct int
}

type ctxKey string

const (
	CtxKeyRoleLabel     ctxKey = "role_label"
	CtxKeyVisionEnabled ctxKey = "vision_enabled"
	CtxKeyProgress      ctxKey = "progress"
	CtxKeyToolCallID    ctxKey = "tool_call_id"
	CtxKeySessionID     ctxKey = "session_id"
)

// SendProgress sends a tool_progress event if a progress callback is in the context.
func SendProgress(ctx context.Context, content string) {
	progress, _ := ctx.Value(CtxKeyProgress).(ProgressCallback)
	if progress == nil {
		return
	}
	tcID, _ := ctx.Value(CtxKeyToolCallID).(string)
	sid, _ := ctx.Value(CtxKeySessionID).(string)
	progress("tool_progress", map[string]any{
		"tool_call_id": tcID,
		"content":      content,
		"session_id":   sid,
	})
}

func NewManager(database *db.DB, llmClient *llm.Client, cfg Config) *Manager {
	return &Manager{DB: database, LLM: llmClient, Config: cfg, ToolHandlers: make(map[string]func(ctx context.Context, args string) *ToolResult)}
}

func (m *Manager) BuildMessages(sessionID string, afterID int64, sysPrompt string, userContent string, tools []openai.Tool, visionEnabled bool) ([]openai.ChatCompletionMessage, error) {
	msgs := []openai.ChatCompletionMessage{
		{Role: "system", Content: sysPrompt},
	}

	dbMsgs, err := m.DB.GetMessages(sessionID, afterID)
	if err != nil {
		return nil, err
	}

	for _, dm := range dbMsgs {
		if dm.Hidden || dm.Role == "notice" {
			continue
		}
		// Skip empty assistant messages (no content, no tool calls)
		if dm.Role == "assistant" && dm.Content == "" && (dm.ToolCalls == "[]" || dm.ToolCalls == "") {
			continue
		}
		msg := openai.ChatCompletionMessage{
			Role:             dm.Role,
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

	return cleaned, nil
}

func (m *Manager) RunLoop(ctx context.Context, sessionID string, sysPrompt string, userContent string, toolList []openai.Tool, llmClient *llm.Client, modelContextLimit int, visionEnabled bool, pauseCh <-chan struct{}, interveneCh <-chan InterveneMsg, progress ProgressCallback) (*LoopResult, error) {
	rounds := 0
	lastPromptTokens := 0

	afterID := int64(0)
	if s, err := m.DB.GetSession(sessionID); err == nil {
		afterID = s.ContextStartID
	}

	if userContent != "" {
		m.DB.InsertMessage(&db.Message{SessionID: sessionID, Role: "user", Content: userContent})
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
				parts := []openai.ChatMessagePart{}
				if msg.Message != "" {
					parts = append(parts, openai.ChatMessagePart{Type: openai.ChatMessagePartTypeText, Text: msg.Message})
				}
				for _, img := range msg.Images {
					parts = append(parts, openai.ChatMessagePart{Type: openai.ChatMessagePartTypeImageURL, ImageURL: &openai.ChatMessageImageURL{URL: img}})
				}
				multiJSON, _ := json.Marshal(parts)
				m.DB.InsertMessage(&db.Message{SessionID: sessionID, Role: "user", Content: msg.Message, MultiContent: string(multiJSON)})
			} else {
				m.DB.InsertMessage(&db.Message{SessionID: sessionID, Role: "user", Content: msg.Message})
			}
		default:
		}

		rounds++
		msgs, err := m.BuildMessages(sessionID, afterID, sysPrompt, "", toolList, visionEnabled)
		if err != nil {
			return nil, err

		}

		if m.Config.MaxContextPct > 0 {
			estPct := estimateContextPct(msgs, modelContextLimit)
			if lastPromptTokens > 0 && modelContextLimit > 0 {
				tokenPct := lastPromptTokens * 100 / modelContextLimit
				if tokenPct > estPct {
					estPct = tokenPct
				}
			}
			if estPct > m.Config.MaxContextPct {
				lastID, lastErr := m.DB.GetLastMessageID(sessionID)
				if lastErr == nil {
					_, _, compErr := m.Compress(ctx, sessionID, afterID, llmClient)
					if compErr == nil {
						m.DB.UpdateSessionContextStart(sessionID, lastID)
						afterID = lastID
						msgs, _ = m.BuildMessages(sessionID, afterID, sysPrompt, "", toolList, visionEnabled)
						if progress != nil {
							progress("context_compressed", map[string]any{
								"session_id": sessionID,
							})
						}
					}
				}
			}
		}

		resp, err := llmClient.Chat(ctx, msgs, toolList, false)
		if err != nil {
			if ctx.Err() != nil {
				return &LoopResult{Stopped: true, Rounds: rounds}, nil
			}
			return m.handleAPIError(ctx, sessionID, sysPrompt, userContent, toolList, rounds, err)
		}

		if len(resp.Choices) == 0 {
			return &LoopResult{Completed: true, Rounds: rounds}, nil
		}

		choice := resp.Choices[0]
		assistantMsg := choice.Message

		// Skip empty responses (no content, no tool calls)
		if len(assistantMsg.ToolCalls) == 0 && strings.TrimSpace(assistantMsg.Content) == "" {
			continue
		}

		tcJSON, _ := json.Marshal(assistantMsg.ToolCalls)
		roleLabel, _ := ctx.Value(CtxKeyRoleLabel).(string)
		m.DB.InsertMessage(&db.Message{
			SessionID:        sessionID,
			Role:             "assistant",
			Content:          assistantMsg.Content,
			ToolCalls:        string(tcJSON),
			ReasoningContent: assistantMsg.ReasoningContent,
			RoleLabel:        roleLabel,
		})

		if len(assistantMsg.ToolCalls) == 0 {
			if progress != nil {
				progress("assistant_message", map[string]any{"content": assistantMsg.Content, "session_id": sessionID, "role_label": roleLabel})
			}
			return &LoopResult{Completed: true, Rounds: rounds, Content: assistantMsg.Content}, nil
		}

		if progress != nil && strings.TrimSpace(assistantMsg.Content) != "" {
			progress("assistant_message", map[string]any{"content": assistantMsg.Content, "session_id": sessionID, "role_label": roleLabel})
		}

		var shouldStop bool
		for _, tc := range assistantMsg.ToolCalls {
			if ctx.Err() != nil {
				return &LoopResult{Stopped: true, Rounds: rounds}, nil
			}

			// Notify frontend before executing each tool
			if progress != nil {
				tcSingle, _ := json.Marshal([]openai.ToolCall{tc})
				progress("tool_call", map[string]any{"tool_call_id": tc.ID, "tool_calls": string(tcSingle), "session_id": sessionID, "role_label": roleLabel})
			}

			toolCtx := context.WithValue(ctx, CtxKeyVisionEnabled, visionEnabled)
			toolCtx = context.WithValue(toolCtx, CtxKeyProgress, progress)
			toolCtx = context.WithValue(toolCtx, CtxKeyToolCallID, tc.ID)
			toolCtx = context.WithValue(toolCtx, CtxKeySessionID, sessionID)
			result := m.executeTool(toolCtx, tc)
			if result.ShouldContinue != nil && !*result.ShouldContinue {
				shouldStop = true
			}

			dbResult := *result
			dbResult.Images = nil
			dbJSON, _ := json.Marshal(dbResult)
			m.DB.InsertMessage(&db.Message{SessionID: sessionID, Role: "tool", Content: string(dbJSON), ToolCallID: tc.ID})

			if progress != nil {
				progress("tool_result", map[string]any{
					"tool_call_id": tc.ID,
					"content":      string(dbJSON),
					"session_id":   sessionID,
				})
			}

			if visionEnabled && len(result.Images) > 0 {
				label := result.ImageLabel
				if label == "" {
					label = "Screenshot"
				}
				if result.Width > 0 && result.Height > 0 {
					label = fmt.Sprintf("%s\n\nUse 0-1000 normalized coordinates over this image: (0,0)=top-left, (1000,1000)=bottom-right, (500,500)=center. Position by proportion 閳?e.g. a button 60%% from left and 10%% from top is (600,100).",
						label)
				}
				m.injectImageMessage(sessionID, result.Images, label)
			}

		}

		if shouldStop {
			return &LoopResult{Completed: true, Rounds: rounds}, nil
		}

		if resp.Usage.PromptTokens > 0 {
			lastPromptTokens = resp.Usage.PromptTokens
		}
		pct := 0
		if modelContextLimit > 0 {
			if lastPromptTokens > 0 {
				pct = lastPromptTokens * 100 / modelContextLimit
			} else {
				pct = estimateContextPct(msgs, modelContextLimit)
			}
		}
		m.DB.UpdateSessionRounds(sessionID, rounds, pct)
			if progress != nil {
				progress("context_pct", map[string]any{
					"session_id": sessionID,
					"pct":        pct,
				})
			}

		if rounds >= 30 {
			m.DB.InsertMessage(&db.Message{SessionID: sessionID, Role: "system",
				Content: "[System] 30 rounds completed. Briefly summarize progress and remaining work, then continue."})
			if progress != nil {
				progress("assistant_message", map[string]any{
					"content":    "[System] 30 rounds completed. Briefly summarize progress and remaining work, then continue.",
					"session_id": sessionID,
				})
			}
			rounds = 0
		}
	}
}

func (m *Manager) Compress(ctx context.Context, sessionID string, afterID int64, llmClient *llm.Client) (string, int64, error) {
	msgs, err := m.BuildMessages(sessionID, afterID, CompressPrompt, "Compress the above conversation.", nil, false)
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

	summary := resp.Choices[0].Message.Content

	content := "[System] 棣冩惖 Context compressed\n\n" + summary
	msgID, err := m.DB.InsertMessage(&db.Message{SessionID: sessionID, Role: "system", Content: content})
	if err != nil {
		return "", 0, err
	}

	return summary, msgID, nil
}

func (m *Manager) ExtractMemories(ctx context.Context, sessionID string, llmClient *llm.Client) error {
	msgs, err := m.DB.GetMessages(sessionID, 0)
	if err != nil {
		return err
	}

	var openaiMsgs []openai.ChatCompletionMessage
	openaiMsgs = append(openaiMsgs, openai.ChatCompletionMessage{Role: "system", Content: MemoryExtractPrompt})
	for _, dm := range msgs {
		openaiMsgs = append(openaiMsgs, openai.ChatCompletionMessage{Role: dm.Role, Content: dm.Content})
	}
	openaiMsgs = append(openaiMsgs, openai.ChatCompletionMessage{Role: "user", Content: "Extract memories from this conversation."})

	resp, err := llmClient.Chat(ctx, openaiMsgs, nil, false)
	if err != nil {
		return err
	}

	if len(resp.Choices) > 0 {
		memories := resp.Choices[0].Message.Content
		appendToFile(m.Config.MemoryPath, memories)
	}

	return nil
}

func (m *Manager) handleAPIError(ctx context.Context, sessionID, sysPrompt, userContent string, tools []openai.Tool, rounds int, err error) (*LoopResult, error) {
	return &LoopResult{
		Completed: false,
		Rounds:    rounds,
		Error:     err.Error(),
	}, nil
}

func (m *Manager) executeTool(ctx context.Context, tc openai.ToolCall) *ToolResult {
	handler, ok := m.ToolHandlers[tc.Function.Name]
	if !ok {
		return &ToolResult{Error: fmt.Sprintf("unknown tool: %s", tc.Function.Name)}
	}
	return handler(ctx, tc.Function.Arguments)
}

func (m *Manager) injectImageMessage(sessionID string, images []string, label string) {
	var parts []openai.ChatMessagePart
	parts = append(parts, openai.ChatMessagePart{
		Type: openai.ChatMessagePartTypeText,
		Text: label,
	})
	for _, img := range images {
		parts = append(parts, openai.ChatMessagePart{
			Type: openai.ChatMessagePartTypeImageURL,
			ImageURL: &openai.ChatMessageImageURL{
				URL: img,
			},
		})
	}
	multiJSON, _ := json.Marshal(parts)
	m.DB.InsertMessage(&db.Message{SessionID: sessionID, Role: "user", Content: label, MultiContent: string(multiJSON)})
}

type LoopResult struct {
	Completed bool
	Paused    bool
	Stopped   bool
	Rounds    int
	Content   string
	Error     string
}

type ToolResult struct {
	Content        string   `json:"content,omitempty"`
	Error          string   `json:"error,omitempty"`
	Images         []string `json:"images,omitempty"`
	ImageLabel     string   `json:"-"`
	Width          int      `json:"-"`
	Height         int      `json:"-"`
	ShouldContinue *bool    `json:"should_continue,omitempty"`
}

func (m *Manager) RegisterTool(name string, handler func(ctx context.Context, args string) *ToolResult) {
	m.ToolHandlers[name] = handler
}

func estimateContextPct(msgs []openai.ChatCompletionMessage, limit int) int {
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

func appendToFile(path, content string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString("\n" + content)
}
