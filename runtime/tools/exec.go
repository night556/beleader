package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"beleader/runtime/engine"
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
	execWorkDir string
)

func setExecWorkDir(dir string) {
	execWorkDir = dir
}

// SetExecWorkDir sets the working directory for command execution (public API).
func SetExecWorkDir(dir string) {
	setExecWorkDir(dir)
}

// ShellName returns the detected shell executable name (e.g. "pwsh", "bash").
func ShellName() string { return detectShell().exe }

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
			cachedShell.info = shellInfo{exe: "cmd", flag: "/c", prefix: "chcp 65001 >nul && "}
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

// ── Handlers ──

func execHandler(ctx context.Context, args string) *engine.ToolResult {
	var p struct {
		Command    string `json:"command"`
		Timeout    int    `json:"timeout"`
		Background bool   `json:"background"`
	}
	json.Unmarshal([]byte(args), &p)

	if p.Command == "" {
		return &engine.ToolResult{Error: "command is required"}
	}

	if p.Background {
		sess := startBackground(ctx, p.Command, execWorkDir)
		if sess == nil {
			return &engine.ToolResult{Error: "failed to start background process"}
		}
		return &engine.ToolResult{
			Content: fmt.Sprintf("Started background session %s (pid=%d)\nCommand: %s\n\nUse task_output(id=\"%s\") to check output, task_output(id=\"%s\", block=true) to wait, task_stop(id=\"%s\") to kill.",
				sess.id, sess.cmd.Process.Pid, sess.command, sess.id, sess.id, sess.id),
		}
	}

	if p.Timeout == 0 {
		p.Timeout = 60
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(p.Timeout)*time.Second)
	defer cancel()

	sh := detectShell()
	command := sh.prefix + p.Command

	cmd := exec.CommandContext(timeoutCtx, sh.exe, sh.flag, command)
	cmd.Dir = execWorkDir

	engine.SendCommandBegin(ctx, p.Command)

	output, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	engine.SendCommandEnd(ctx, p.Command, exitCode)

	if err != nil {
		if timeoutCtx.Err() != nil {
			return &engine.ToolResult{Content: string(output), Error: fmt.Sprintf("command timed out after %ds", p.Timeout)}
		}
		return &engine.ToolResult{Content: string(output), Error: err.Error()}
	}
	return &engine.ToolResult{Content: string(output)}
}

func taskOutputHandler(ctx context.Context, args string) *engine.ToolResult {
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
		return &engine.ToolResult{Error: fmt.Sprintf("session %s not found. Use task_output(id=\"e1\") with the session ID returned by run_command.", p.ID)}
	}

	if p.Block {
		if p.Wait <= 0 {
			p.Wait = 30
		}
		select {
		case <-sess.done:
		case <-time.After(time.Duration(p.Wait) * time.Second):
			// Return partial output on timeout.
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
			return &engine.ToolResult{
				Content: fmt.Sprintf("[exited code=%d]\n%s", code, output),
				Error:   exitErrStr,
			}
		}
		return &engine.ToolResult{Content: fmt.Sprintf("[exited code=%d]\n%s", code, output)}
	}

	if output == "" {
		return &engine.ToolResult{Content: fmt.Sprintf("[running pid=%d, elapsed=%ds, no new output]", sess.cmd.Process.Pid, int(time.Since(sess.started).Seconds()))}
	}
	return &engine.ToolResult{Content: fmt.Sprintf("[running pid=%d, elapsed=%ds]\n%s", sess.cmd.Process.Pid, int(time.Since(sess.started).Seconds()), output)}
}

func taskStopHandler(ctx context.Context, args string) *engine.ToolResult {
	var p struct {
		ID string `json:"id"`
	}
	json.Unmarshal([]byte(args), &p)

	bgMu.Lock()
	sess := bgSessions[p.ID]
	bgMu.Unlock()

	if sess == nil {
		return &engine.ToolResult{Error: fmt.Sprintf("session %s not found", p.ID)}
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
		return &engine.ToolResult{
			Content: fmt.Sprintf("Killed %s (exit code=%d)\n\n%s", p.ID, code, output),
			Error:   exitErrStr,
		}
	}
	return &engine.ToolResult{Content: fmt.Sprintf("Killed %s (exit code=%d)\n\n%s", p.ID, code, output)}
}

// ── Background ──

func startBackground(ctx context.Context, command, workDir string) *bgSession {
	bgMu.Lock()
	bgSeq++
	id := fmt.Sprintf("e%d", bgSeq)
	bgMu.Unlock()

	sh := detectShell()
	command = sh.prefix + command

	cmd := exec.CommandContext(ctx, sh.exe, sh.flag, command)
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

// GetUndeliveredResults returns results for completed background sessions,
// removes them from the session map, and marks them as delivered.
func GetUndeliveredResults() []engine.BackgroundResult {
	bgMu.Lock()
	defer bgMu.Unlock()

	var results []engine.BackgroundResult
	for id, s := range bgSessions {
		select {
		case <-s.done:
			s.mu.Lock()
			output, _ := s.output.StringFrom(0)
			errStr := ""
			if s.exitErr != nil {
				errStr = s.exitErr.Error()
			}
			r := engine.BackgroundResult{
				ID:       s.id,
				Command:  s.command,
				ExitCode: s.exitCode,
				Output:   output,
				Error:    errStr,
			}
			s.mu.Unlock()
			results = append(results, r)
			delete(bgSessions, id)
		default:
		}
	}
	return results
}

// RegisterExecTools registers run_command, task_output, and task_stop handlers.
func RegisterExecTools(eng *engine.Engine) {
	eng.RegisterTool("run_command", execHandler)
	eng.RegisterTool("task_output", taskOutputHandler)
	eng.RegisterTool("task_stop", taskStopHandler)
}
