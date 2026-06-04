package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"iamhuman/backend/session"

	"github.com/sashabaranov/go-openai"
)

var DesktopEnabled bool

var lastNativeW, lastNativeH int
var lastOffsetX, lastOffsetY int

func mkTool(name, desc string, props map[string]any, required []string) openai.Tool {
	params := map[string]any{
		"type": "object",
	}
	if props != nil {
		params["properties"] = props
	}
	if required != nil {
		params["required"] = required
	}
	return openai.Tool{
		Type: "function",
		Function: &openai.FunctionDefinition{
			Name:        name,
			Description: desc,
			Parameters:  params,
		},
	}
}

var desktopTools = []openai.Tool{
	mkTool("desktop_screenshot", "Capture a screenshot of the entire screen, a region, or a specific window. Use this to understand what's currently visible before deciding where to click or type.",
		map[string]any{
			"region":      map[string]any{"type": "string", "description": "Region as 'x,y,w,h' (optional, defaults to full screen)"},
			"window":      map[string]any{"type": "string", "description": "Capture a specific window by title (partial match)"},
			"description": map[string]any{"type": "string", "description": "One line describing why this screenshot is needed, e.g. 'locate the settings button'. This text labels the image."},
		}, nil),

	mkTool("desktop_click", "Click at (x, y) in 0-1000 normalized coordinates over the screenshot image. 0=left/top, 1000=right/bottom, 500=center.",
		map[string]any{
			"x":      map[string]any{"type": "integer", "description": "X coordinate in 0-1000 range. 0=left edge, 1000=right edge, 500=center."},
			"y":      map[string]any{"type": "integer", "description": "Y coordinate in 0-1000 range. 0=top edge, 1000=bottom edge, 500=center."},
			"button": map[string]any{"type": "string", "description": "Mouse button: left, right, center. Default: left"},
		}, []string{"x", "y"}),

	mkTool("desktop_double_click", "Double-click at (x, y) in 0-1000 normalized coordinates over the screenshot image.",
		map[string]any{
			"x": map[string]any{"type": "integer", "description": "X coordinate in 0-1000 range. 0=left edge, 1000=right edge, 500=center."},
			"y": map[string]any{"type": "integer", "description": "Y coordinate in 0-1000 range. 0=top edge, 1000=bottom edge, 500=center."},
		}, []string{"x", "y"}),

	mkTool("desktop_move", "Move the mouse cursor to (x, y) in 0-1000 normalized coordinates over the screenshot image.",
		map[string]any{
			"x": map[string]any{"type": "integer", "description": "X coordinate in 0-1000 range. 0=left edge, 1000=right edge, 500=center."},
			"y": map[string]any{"type": "integer", "description": "Y coordinate in 0-1000 range. 0=top edge, 1000=bottom edge, 500=center."},
		}, []string{"x", "y"}),

	mkTool("desktop_drag", "Drag the mouse from (x, y) to (to_x, to_y) in 0-1000 normalized coordinates over the screenshot image.",
		map[string]any{
			"x":    map[string]any{"type": "integer", "description": "Start X coordinate in 0-1000 range"},
			"y":    map[string]any{"type": "integer", "description": "Start Y coordinate in 0-1000 range"},
			"to_x": map[string]any{"type": "integer", "description": "End X coordinate in 0-1000 range"},
			"to_y": map[string]any{"type": "integer", "description": "End Y coordinate in 0-1000 range"},
		}, []string{"x", "y", "to_x", "to_y"}),

	mkTool("desktop_scroll", "Scroll the mouse wheel at (x, y) in 0-1000 normalized coordinates. dx/dy are scroll ticks (not normalized).",
		map[string]any{
			"x":  map[string]any{"type": "integer", "description": "X coordinate in 0-1000 range"},
			"y":  map[string]any{"type": "integer", "description": "Y coordinate in 0-1000 range"},
			"dx": map[string]any{"type": "integer", "description": "Horizontal scroll delta"},
			"dy": map[string]any{"type": "integer", "description": "Vertical scroll delta (positive=down, negative=up)"},
		}, []string{"x", "y", "dx", "dy"}),

	mkTool("desktop_type_text", "Type text at the current cursor/focus position. Supports all languages including Chinese (Unicode). Use this for entering text — do NOT use run_command to type.",
		map[string]any{
			"text": map[string]any{"type": "string", "description": "Text to type. Supports Chinese and any Unicode characters."},
		}, []string{"text"}),

	mkTool("desktop_key_tap", "Press a key or key combination (e.g. 'enter', 'ctrl+c', 'alt+tab').",
		map[string]any{
			"keys": map[string]any{"type": "string", "description": "Key or combo, e.g. 'enter', 'ctrl+c', 'alt+tab'"},
		}, []string{"keys"}),

	mkTool("desktop_clipboard_read", "Read the current text content of the system clipboard.", nil, nil),

	mkTool("desktop_clipboard_write", "Write text to the system clipboard.",
		map[string]any{
			"text": map[string]any{"type": "string", "description": "Text to copy to clipboard"},
		}, []string{"text"}),

	mkTool("desktop_window_list", "List all visible windows with their titles and PIDs.", nil, nil),

	mkTool("desktop_window_activate", "Bring a window to the foreground by title or PID.",
		map[string]any{
			"title": map[string]any{"type": "string", "description": "Window title (partial match)"},
			"pid":   map[string]any{"type": "integer", "description": "Window PID"},
		}, nil),

	mkTool("desktop_window_minimize", "Minimize a window by title or PID.",
		map[string]any{
			"title": map[string]any{"type": "string", "description": "Window title (partial match)"},
			"pid":   map[string]any{"type": "integer", "description": "Window PID"},
		}, nil),

	mkTool("desktop_window_maximize", "Maximize a window by title or PID.",
		map[string]any{
			"title": map[string]any{"type": "string", "description": "Window title (partial match)"},
			"pid":   map[string]any{"type": "integer", "description": "Window PID"},
		}, nil),

	mkTool("desktop_window_close", "Close a window by title or PID.",
		map[string]any{
			"title": map[string]any{"type": "string", "description": "Window title (partial match)"},
			"pid":   map[string]any{"type": "integer", "description": "Window PID"},
		}, nil),

	mkTool("desktop_process_list", "List running processes with name and PID.", nil, nil),

	mkTool("desktop_mouse_info", "Get current mouse position and related info.", nil, nil),

	mkTool("desktop_screen_info", "Get screen/monitor layout information.", nil, nil),

	mkTool("desktop_active_window", "Get the currently active/focused window title and PID.", nil, nil),

	mkTool("desktop_sleep", "Pause execution for the specified duration.",
		map[string]any{
			"duration": map[string]any{"type": "number", "description": "Duration in milliseconds. Default: 500"},
		}, nil),
}

func DesktopTools() []openai.Tool { return desktopTools }

func RegisterDesktopTools(mgr *session.Manager) {
	DesktopEnabled = true
	for _, t := range desktopTools {
		name := t.Function.Name
		mgr.RegisterTool(name, func(ctx context.Context, args string) *session.ToolResult {
			return dispatchDesktop(name, args)
		})
	}
}

func normalizeCoord(norm, nativeDim, offset int) int {
	return norm*nativeDim/1000 + offset
}

func needScreenshot() *session.ToolResult {
	if lastNativeW == 0 {
		return &session.ToolResult{Error: "No screenshot taken yet — call desktop_screenshot first to capture the screen, then use the coordinates from that image."}
	}
	return nil
}

func checkJSONKeys(argsStr string, required []string) *session.ToolResult {
	var raw map[string]any
	if err := json.Unmarshal([]byte(argsStr), &raw); err != nil {
		return &session.ToolResult{Error: fmt.Sprintf("参数格式错误: %v", err)}
	}
	var missing []string
	for _, k := range required {
		if _, ok := raw[k]; !ok {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		var have []string
		for k := range raw {
			have = append(have, k)
		}
		return &session.ToolResult{Error: fmt.Sprintf(
			"Missing required parameters: %v. You sent keys: %v. Use exact English parameter names — e.g. \"x\" and \"y\", not translated names.",
			missing, have,
		)}
	}
	return nil
}

// parseAndNormalize unmarshals argsStr, validates required keys, checks for a prior
// screenshot, and normalizes all named coordinate fields from 0-1000 → screen pixels.
func parseAndNormalize(argsStr string, required, normFields []string) (map[string]any, *session.ToolResult) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(argsStr), &raw); err != nil {
		return nil, &session.ToolResult{Error: fmt.Sprintf("参数格式错误: %v", err)}
	}
	if err := checkJSONKeys(argsStr, required); err != nil {
		return nil, err
	}
	if err := needScreenshot(); err != nil {
		return nil, err
	}
	for _, f := range normFields {
		if v, ok := raw[f]; ok {
			n := int(v.(float64))
			switch f {
			case "x", "to_x":
				raw[f] = normalizeCoord(n, lastNativeW, lastOffsetX)
			case "y", "to_y":
				raw[f] = normalizeCoord(n, lastNativeH, lastOffsetY)
			}
		}
	}
	return raw, nil
}

// windowAgentArgs builds agent args for window_activate/minimize/maximize/close.
func windowAgentArgs(cmd string, argsStr string) *session.ToolResult {
	var p struct {
		Title string `json:"title"`
		Pid   int    `json:"pid"`
	}
	if err := json.Unmarshal([]byte(argsStr), &p); err != nil {
		return &session.ToolResult{Error: fmt.Sprintf("参数格式错误: %v", err)}
	}
	args := []string{cmd}
	if p.Title != "" {
		args = append(args, "--title", p.Title)
	}
	if p.Pid != 0 {
		args = append(args, "--pid", itoa(p.Pid))
	}
	return callAgent(args...)
}

func dispatchDesktop(toolName string, argsStr string) *session.ToolResult {
	switch toolName {
	// ── Screenshot (special) ──
	case "desktop_screenshot":
		var p struct {
			Region      string `json:"region"`
			Window      string `json:"window"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal([]byte(argsStr), &p); err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("参数格式错误: %v", err)}
		}
		result := desktopScreenshot(p.Region, p.Window)
		result.ImageLabel = p.Description
		return result

	// ── Coordinate tools (normalized 0-1000 → screen pixels) ──
	case "desktop_click":
		raw, rerr := parseAndNormalize(argsStr, []string{"x", "y"}, []string{"x", "y"})
		if rerr != nil {
			return rerr
		}
		btn := "left"
		if b, ok := raw["button"].(string); ok && b != "" {
			btn = b
		}
		return callAgent("click", "--x", itoa(raw["x"].(int)), "--y", itoa(raw["y"].(int)), "--button", btn)

	case "desktop_double_click":
		raw, rerr := parseAndNormalize(argsStr, []string{"x", "y"}, []string{"x", "y"})
		if rerr != nil {
			return rerr
		}
		return callAgent("click", "--x", itoa(raw["x"].(int)), "--y", itoa(raw["y"].(int)), "--button", "left", "--double")

	case "desktop_move":
		raw, rerr := parseAndNormalize(argsStr, []string{"x", "y"}, []string{"x", "y"})
		if rerr != nil {
			return rerr
		}
		return callAgent("move", "--x", itoa(raw["x"].(int)), "--y", itoa(raw["y"].(int)))

	case "desktop_drag":
		raw, rerr := parseAndNormalize(argsStr, []string{"x", "y", "to_x", "to_y"}, []string{"x", "y", "to_x", "to_y"})
		if rerr != nil {
			return rerr
		}
		return callAgent("drag", "--x", itoa(raw["x"].(int)), "--y", itoa(raw["y"].(int)), "--to-x", itoa(raw["to_x"].(int)), "--to-y", itoa(raw["to_y"].(int)))

	case "desktop_scroll":
		raw, rerr := parseAndNormalize(argsStr, []string{"x", "y"}, []string{"x", "y"})
		if rerr != nil {
			return rerr
		}
		return callAgent("scroll", "--x", itoa(raw["x"].(int)), "--y", itoa(raw["y"].(int)), "--dx", itoa(int(raw["dx"].(float64))), "--dy", itoa(int(raw["dy"].(float64))))

	// ── Text / Keys / Clipboard ──
	case "desktop_type_text":
		var p struct{ Text string `json:"text"` }
		if err := json.Unmarshal([]byte(argsStr), &p); err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("参数格式错误: %v", err)}
		}
		return callAgent("type-text", "--text", p.Text)

	case "desktop_key_tap":
		var p struct{ Keys string `json:"keys"` }
		if err := json.Unmarshal([]byte(argsStr), &p); err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("参数格式错误: %v", err)}
		}
		return callAgent("key-tap", "--keys", p.Keys)

	case "desktop_clipboard_read":
		return callAgent("clipboard-read")

	case "desktop_clipboard_write":
		var p struct{ Text string `json:"text"` }
		if err := json.Unmarshal([]byte(argsStr), &p); err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("参数格式错误: %v", err)}
		}
		return callAgent("clipboard-write", "--text", p.Text)

	// ── Window management ──
	case "desktop_window_activate":
		return windowAgentArgs("window-activate", argsStr)
	case "desktop_window_minimize":
		return windowAgentArgs("window-minimize", argsStr)
	case "desktop_window_maximize":
		return windowAgentArgs("window-maximize", argsStr)
	case "desktop_window_close":
		return windowAgentArgs("window-close", argsStr)

	// ── Info queries (no args) ──
	case "desktop_window_list":
		return callAgent("window-list")
	case "desktop_process_list":
		return callAgent("process-list")
	case "desktop_mouse_info":
		return callAgent("mouse-info")
	case "desktop_screen_info":
		return callAgent("screen-info")
	case "desktop_active_window":
		return callAgent("active-window")

	// ── Sleep ──
	case "desktop_sleep":
		var p struct{ Duration float64 `json:"duration"` }
		if err := json.Unmarshal([]byte(argsStr), &p); err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("参数格式错误: %v", err)}
		}
		if p.Duration <= 0 {
			p.Duration = 500
		}
		time.Sleep(time.Duration(p.Duration) * time.Millisecond)
		return &session.ToolResult{Content: fmt.Sprintf("Slept for %.0fms", p.Duration)}

	default:
		return &session.ToolResult{Error: fmt.Sprintf("unknown tool: %s", toolName)}
	}
}

// ---- agent call helper ----

var agentExe string

func findAgent() string {
	if agentExe != "" {
		return agentExe
	}
	exe, _ := os.Executable()
	dir := filepath.Dir(exe)
	name := "iamhuman-agent"
	if filepath.Ext(name) == "" {
		if _, err := os.Stat(name + ".exe"); err == nil {
			name += ".exe"
		}
	}
	candidates := []string{
		filepath.Join(dir, "iamhuman-agent.exe"),
		filepath.Join(dir, "iamhuman-agent"),
		filepath.Join("robot", "target", "release", "iamhuman-agent.exe"),
		filepath.Join("robot", "target", "release", "iamhuman-agent"),
		filepath.Join("bin", "iamhuman-agent-release"),
		"iamhuman-agent",
		"iamhuman-agent.exe",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			agentExe = c
			return agentExe
		}
	}
	agentExe = "iamhuman-agent"
	return agentExe
}

func callAgent(args ...string) *session.ToolResult {
	cmd := exec.Command(findAgent(), args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return &session.ToolResult{Error: fmt.Sprintf("agent: %s", string(exitErr.Stderr))}
		}
		return &session.ToolResult{Error: fmt.Sprintf("agent: %v", err)}
	}

	var result struct {
		Content string `json:"content"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return &session.ToolResult{Error: fmt.Sprintf("agent output: %v", err)}
	}
	if result.Error != "" {
		return &session.ToolResult{Error: result.Error}
	}
	return &session.ToolResult{Content: result.Content}
}

// ---- screenshot ----

func desktopScreenshot(region, window string) *session.ToolResult {
	dir := filepath.Join(execWorkDir, "screenshots")
	os.MkdirAll(dir, 0755)

	args := []string{"screenshot", "--output", dir}
	if region != "" {
		args = append(args, "--region", region)
	}
	if window != "" {
		args = append(args, "--window", window)
	}

	cmd := exec.Command(findAgent(), args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return &session.ToolResult{Error: fmt.Sprintf("agent: %s", string(exitErr.Stderr))}
		}
		return &session.ToolResult{Error: fmt.Sprintf("agent: %v", err)}
	}

	var parsed struct {
		Path     string `json:"path"`
		Width    int    `json:"width"`
		Height   int    `json:"height"`
		OffsetX  int    `json:"offset_x"`
		OffsetY  int    `json:"offset_y"`
		MouseX   int    `json:"mouse_x"`
		MouseY   int    `json:"mouse_y"`
		Context  string `json:"context"`
		Content  string `json:"content"`
		Error    string `json:"error"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return &session.ToolResult{Error: fmt.Sprintf("agent screenshot output: %v", err)}
	}
	if parsed.Error != "" {
		return &session.ToolResult{Error: parsed.Error}
	}

	lastNativeW = parsed.Width
	lastNativeH = parsed.Height
	lastOffsetX = parsed.OffsetX
	lastOffsetY = parsed.OffsetY

	data, err := os.ReadFile(parsed.Path)
	if err != nil {
		return &session.ToolResult{Content: parsed.Content}
	}

	b64 := base64.StdEncoding.EncodeToString(data)
	uri := fmt.Sprintf("data:image/jpeg;base64,%s", b64)

	imgX := parsed.MouseX - parsed.OffsetX
	imgY := parsed.MouseY - parsed.OffsetY
	normMouseX := 0
	normMouseY := 0
	if parsed.Width > 0 {
		normMouseX = imgX * 1000 / parsed.Width
	}
	if parsed.Height > 0 {
		normMouseY = imgY * 1000 / parsed.Height
	}

	msg := fmt.Sprintf("Screenshot %dx%d %s. Saved to %s. Mouse at screen(%d,%d) = img(%d,%d) = normalized(%d,%d).",
		parsed.Width, parsed.Height, parsed.Context,
		parsed.Path, parsed.MouseX, parsed.MouseY,
		imgX, imgY, normMouseX, normMouseY)
	if parsed.OffsetX != 0 || parsed.OffsetY != 0 {
		msg += fmt.Sprintf(" Image top-left offset: (%d,%d).", parsed.OffsetX, parsed.OffsetY)
	}

	return &session.ToolResult{
		Content: msg,
		Images:  []string{uri},
		Width:   parsed.Width,
		Height:  parsed.Height,
	}
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}
