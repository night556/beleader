package engine

// EventFrame is the SSE event envelope matching CodeWhale's event model.
type EventFrame struct {
	Seq        int64  `json:"seq"`
	Event      string `json:"event"`
	TurnID     string `json:"turn_id,omitempty"`
	ResponseID string `json:"response_id,omitempty"`
	Delta      string `json:"delta,omitempty"`
	Channel    string `json:"channel,omitempty"` // "text" | "reasoning"
	ToolName   string `json:"tool_name,omitempty"`
	Arguments  string `json:"arguments,omitempty"`
	Output     any    `json:"output,omitempty"`
	Command    string `json:"command,omitempty"`
	CWD        string `json:"cwd,omitempty"`
	ExitCode   int    `json:"exit_code,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Message    string `json:"message,omitempty"`
	Kind       string `json:"kind,omitempty"`    // user_message | agent_message | tool_call | tool_result
	Content    string `json:"content,omitempty"` // full content for completed messages
	Timestamp  string `json:"timestamp,omitempty"`
}

// Event type constants.
const (
	EventTurnStarted            = "turn_started"
	EventTurnComplete           = "turn_complete"
	EventTurnAborted            = "turn_aborted"
	EventResponseStart          = "response_start"
	EventResponseDelta          = "response_delta"
	EventResponseEnd            = "response_end"
	EventToolCallStart          = "tool_call_start"
	EventToolCallResult         = "tool_call_result"
	EventExecCommandBegin       = "exec_command_begin"
	EventExecCommandOutputDelta = "exec_command_output_delta"
	EventExecCommandEnd         = "exec_command_end"
	EventError                  = "error"
)
