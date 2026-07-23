package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

// ── outputBuf ──

type outputBuf struct {
	mu  sync.Mutex
	buf []byte
}

func newOutputBuf() *outputBuf {
	return &outputBuf{buf: make([]byte, 0, 8192)}
}

func (b *outputBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > 50*1024 {
		keep := len(b.buf) - 50*1024
		b.buf = b.buf[keep:]
	}
	return len(p), nil
}

func (b *outputBuf) StringFrom(pos int) (string, int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if pos >= len(b.buf) {
		return "", len(b.buf)
	}
	s := string(b.buf[pos:])
	return s, len(b.buf)
}

// ── bgSession ──

type bgSession struct {
	id           string
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	output       *outputBuf
	command      string
	started      time.Time
	exitCode     int
	exitErr      error
	done         chan struct{}
	lastCheckPos int
	mu           sync.Mutex
}

var (
	bgSessions = map[string]*bgSession{}
	bgMu       sync.Mutex
	bgSeq      int
)

type shellInfo struct {
	exe    string
	flag   string
	prefix string
}

var cachedShell struct {
	sync.Once
	info shellInfo
}

func detectShell() shellInfo {
	cachedShell.Do(func() {
		if runtime.GOOS == "windows" {
			for _, exe := range []string{"pwsh.exe", "powershell.exe"} {
				if p, err := exec.LookPath(exe); err == nil {
					cachedShell.info = shellInfo{exe: p, flag: "-Command"}
					return
				}
			}
			cachedShell.info = shellInfo{exe: "cmd", flag: "/c", prefix: "chcp 65001 >nul & "}
			return
		}
		for _, exe := range []string{"bash", "zsh", "ash"} {
			if p, err := exec.LookPath(exe); err == nil {
				cachedShell.info = shellInfo{exe: p, flag: "-c"}
				return
			}
		}
		cachedShell.info = shellInfo{exe: "sh", flag: "-c"}
	})
	return cachedShell.info
}

// ── Tool handlers ──

func execHandler(args, workspace, workspaceRoot string, restrict bool, threadID string) *ToolResult {
	var p struct {
		Command    string `json:"command"`
		Timeout    int    `json:"timeout"`
		Background bool   `json:"background"`
	}
	json.Unmarshal([]byte(args), &p)
	if p.Command == "" {
		return &ToolResult{Error: "command is required"}
	}

	workDir := workspace

	if p.Background {
		sess := startBackground(p.Command, workDir)
		if sess == nil {
			return &ToolResult{Error: "failed to start background process"}
		}
		return &ToolResult{
			Content: fmt.Sprintf("Started background session %s (pid=%d)\nCommand: %s\n\nUse task_output(id=\"%s\") to check output, task_output(id=\"%s\", block=true) to wait, task_stop(id=\"%s\") to kill.",
				sess.id, sess.cmd.Process.Pid, sess.command, sess.id, sess.id, sess.id),
		}
	}

	if p.Timeout == 0 {
		p.Timeout = 60
	}
	if p.Timeout > 120 {
		p.Timeout = 120
	}

	sh := detectShell()
	command := sh.prefix + p.Command

	cmd := exec.Command(sh.exe, sh.flag, command)
	cmd.Dir = workDir

	output, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	if err != nil {
		return &ToolResult{Content: string(output), Error: fmt.Sprintf("exit code %d", exitCode)}
	}
	return &ToolResult{Content: string(output)}
}

func taskOutputHandler(args, workspace, workspaceRoot string, restrict bool, threadID string) *ToolResult {
	var p struct {
		ID    string `json:"id"`
		Block bool   `json:"block"`
		Wait  int    `json:"wait"`
	}
	json.Unmarshal([]byte(args), &p)

	bgMu.Lock()
	sess := bgSessions[p.ID]
	bgMu.Unlock()

	if sess == nil {
		return &ToolResult{Error: fmt.Sprintf("session %s not found", p.ID)}
	}

	if p.Block {
		if p.Wait <= 0 {
			p.Wait = 30
		}
		select {
		case <-sess.done:
		case <-time.After(time.Duration(p.Wait) * time.Second):
		}
	}

	sess.mu.Lock()
	pos := sess.lastCheckPos
	sess.mu.Unlock()

	output, endPos := sess.output.StringFrom(pos)

	sess.mu.Lock()
	sess.lastCheckPos = endPos
	exited := false
	select {
	case <-sess.done:
		exited = true
	default:
	}
	code := sess.exitCode
	exitErrStr := ""
	if sess.exitErr != nil {
		exitErrStr = sess.exitErr.Error()
	}
	sess.mu.Unlock()

	if exited {
		if code != 0 || exitErrStr != "" {
			return &ToolResult{Content: fmt.Sprintf("[exited code=%d]\n%s", code, output), Error: exitErrStr}
		}
		return &ToolResult{Content: fmt.Sprintf("[exited code=%d]\n%s", code, output)}
	}

	if output == "" {
		return &ToolResult{Content: fmt.Sprintf("[running pid=%d, elapsed=%ds, no new output]", sess.cmd.Process.Pid, int(time.Since(sess.started).Seconds()))}
	}
	return &ToolResult{Content: fmt.Sprintf("[running pid=%d, elapsed=%ds]\n%s", sess.cmd.Process.Pid, int(time.Since(sess.started).Seconds()), output)}
}

func taskStopHandler(args, workspace, workspaceRoot string, restrict bool, threadID string) *ToolResult {
	var p struct{ ID string `json:"id"` }
	json.Unmarshal([]byte(args), &p)

	bgMu.Lock()
	sess := bgSessions[p.ID]
	bgMu.Unlock()

	if sess == nil {
		return &ToolResult{Error: fmt.Sprintf("session %s not found", p.ID)}
	}

	select {
	case <-sess.done:
	default:
		sess.cmd.Process.Kill()
	}

	select {
	case <-sess.done:
	case <-time.After(5 * time.Second):
	}

	sess.mu.Lock()
	code := sess.exitCode
	exitErrStr := ""
	if sess.exitErr != nil {
		exitErrStr = sess.exitErr.Error()
	}
	sess.mu.Unlock()

	output, _ := sess.output.StringFrom(0)

	if code != 0 || exitErrStr != "" {
		return &ToolResult{Content: fmt.Sprintf("Killed %s (exit code=%d)\n\n%s", p.ID, code, output), Error: exitErrStr}
	}
	return &ToolResult{Content: fmt.Sprintf("Killed %s (exit code=%d)\n\n%s", p.ID, code, output)}
}

// ── Background ──

func startBackground(command, workDir string) *bgSession {
	bgMu.Lock()
	bgSeq++
	id := fmt.Sprintf("e%d", bgSeq)
	bgMu.Unlock()

	sh := detectShell()
	command = sh.prefix + command

	cmd := exec.Command(sh.exe, sh.flag, command)
	cmd.Dir = workDir

	stdin, _ := cmd.StdinPipe()
	outBuf := newOutputBuf()

	cmd.Stdout = outBuf
	cmd.Stderr = outBuf

	sess := &bgSession{
		id:      id,
		cmd:     cmd,
		stdin:   stdin,
		output:  outBuf,
		command: command,
		started: time.Now(),
		done:    make(chan struct{}),
	}

	bgMu.Lock()
	bgSessions[id] = sess
	bgMu.Unlock()

	if err := cmd.Start(); err != nil {
		bgMu.Lock()
		delete(bgSessions, id)
		bgMu.Unlock()
		return nil
	}

	go func() {
		err := cmd.Wait()
		sess.mu.Lock()
		if err != nil {
			sess.exitErr = err
			if exitErr, ok := err.(*exec.ExitError); ok {
				sess.exitCode = exitErr.ExitCode()
			} else {
				sess.exitCode = -1
			}
		}
		sess.mu.Unlock()
		close(sess.done)
	}()

	return sess
}

// Cleanup kills all background processes.
func Cleanup() {
	bgMu.Lock()
	defer bgMu.Unlock()
	for _, sess := range bgSessions {
		select {
		case <-sess.done:
		default:
			if sess.cmd.Process != nil {
				sess.cmd.Process.Kill()
			}
		}
	}
}

func init() {
	register("run_command",
		"Execute a shell command in the workspace directory. Set background=true for long-running commands — returns a session_id. Use task_output to check or wait for results, and task_stop to kill.",
		map[string]any{
			"command":    map[string]any{"type": "string", "description": "Shell command to execute."},
			"timeout":    map[string]any{"type": "integer", "description": "Max seconds for sync mode. Default 60, max 120."},
			"background": map[string]any{"type": "boolean", "description": "Set true for long-running commands. Returns session_id immediately."},
		}, []string{"command"}, execHandler)

	register("task_output",
		"Get output from a background command started with run_command(background=true). Two modes: block=false (check immediately) and block=true (wait up to wait seconds).",
		map[string]any{
			"id":    map[string]any{"type": "string", "description": "Session ID returned by run_command."},
			"block": map[string]any{"type": "boolean", "description": "Whether to block until the command completes. Default false."},
			"wait":  map[string]any{"type": "integer", "description": "Max seconds to wait when block=true. Default 30."},
		}, []string{"id"}, taskOutputHandler)

	register("task_stop",
		"Stop a running background command and return its final output.",
		map[string]any{
			"id": map[string]any{"type": "string", "description": "Session ID to kill."},
		}, []string{"id"}, taskStopHandler)
}
