package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"beleader/runtime/engine"
)

// ringBuffer is a fixed-size circular buffer for capturing process output.
type ringBuffer struct {
	buf    []byte
	size   int
	write  int
	total  int
	mu     sync.Mutex
	notify chan struct{}
}

func newRingBuffer(size int) *ringBuffer {
	return &ringBuffer{
		buf:    make([]byte, size),
		size:   size,
		notify: make(chan struct{}, 1),
	}
}

func (rb *ringBuffer) Write(p []byte) (int, error) {
	rb.mu.Lock()
	for i := 0; i < len(p); i++ {
		rb.buf[rb.write] = p[i]
		rb.write = (rb.write + 1) % rb.size
	}
	rb.total += len(p)
	rb.mu.Unlock()

	select {
	case rb.notify <- struct{}{}:
	default:
	}
	return len(p), nil
}

func (rb *ringBuffer) read(offset, limit int) (string, int) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	total := rb.total
	if offset >= total {
		return "", total
	}

	available := rb.size
	if total < rb.size {
		available = total
	}
	if offset < total-available {
		offset = total - available
	}

	remaining := total - offset
	if limit <= 0 || limit > remaining {
		limit = remaining
	}

	var out strings.Builder
	for i := offset; i < offset+limit; i++ {
		idx := i % rb.size
		out.WriteByte(rb.buf[idx])
	}
	return out.String(), total
}

func (rb *ringBuffer) resetNotify() {
	select {
	case <-rb.notify:
	default:
	}
}

// execSession tracks a single background process.
type execSession struct {
	id       string
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   *ringBuffer
	stderr   *ringBuffer
	command  string
	started  time.Time
	exitCode int
	exitErr  error
	done     chan struct{}
	mu       sync.Mutex
}

var (
	execSessions = map[string]*execSession{}
	execMu       sync.Mutex
	execSeq      int
	execWorkDir  string
)

func setExecWorkDir(dir string) {
	execWorkDir = dir
}

// SetExecWorkDir sets the working directory for command execution (public API).
func SetExecWorkDir(dir string) {
	setExecWorkDir(dir)
}

func prepareCommandWindows(command string) string {
	if runtime.GOOS == "windows" {
		return "chcp 65001 >nul && " + command
	}
	return command
}

func startBackground(ctx context.Context, command, workDir string) *execSession {
	execMu.Lock()
	execSeq++
	id := fmt.Sprintf("e%d", execSeq)
	execMu.Unlock()

	command = prepareCommandWindows(command)

	var shell, shellFlag string
	if runtime.GOOS == "windows" {
		shell = "cmd"
		shellFlag = "/c"
	} else {
		shell = "bash"
		shellFlag = "-c"
	}

	cmd := exec.CommandContext(ctx, shell, shellFlag, command)
	cmd.Dir = workDir

	stdin, _ := cmd.StdinPipe()
	stdoutBuf := newRingBuffer(100 * 1024)
	stderrBuf := newRingBuffer(100 * 1024)

	cmd.Stdout = io.MultiWriter(stdoutBuf)
	cmd.Stderr = io.MultiWriter(stderrBuf)

	sess := &execSession{
		id:      id,
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdoutBuf,
		stderr:  stderrBuf,
		command: command,
		started: time.Now(),
		done:    make(chan struct{}),
	}

	execMu.Lock()
	execSessions[id] = sess
	execMu.Unlock()

	if err := cmd.Start(); err != nil {
		execMu.Lock()
		delete(execSessions, id)
		execMu.Unlock()
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
		select {
		case stdoutBuf.notify <- struct{}{}:
		default:
		}
	}()

	return sess
}

func sessionSummary(sess *execSession) string {
	sess.mu.Lock()
	exitCode := sess.exitCode
	sess.mu.Unlock()

	select {
	case <-sess.done:
		return fmt.Sprintf("exited (code=%d)", exitCode)
	default:
		return fmt.Sprintf("running (pid=%d, elapsed=%ds)", sess.cmd.Process.Pid, int(time.Since(sess.started).Seconds()))
	}
}

func readCombined(sess *execSession, offset, limit int) (string, int) {
	out, outTotal := sess.stdout.read(offset, limit)
	err, errTotal := sess.stderr.read(offset, limit)

	total := outTotal
	if errTotal > total {
		total = errTotal
	}

	var result string
	if out != "" && err != "" {
		result = out + "\n[stderr]\n" + err
	} else if err != "" {
		result = err
	} else {
		result = out
	}
	return result, total
}

func execHandler(ctx context.Context, args string) *engine.ToolResult {
	var p struct {
		Command    string `json:"command"`
		Timeout    int    `json:"timeout"`
		Background bool   `json:"background"`
		Action     string `json:"action"`
		SessionID  string `json:"session_id"`
		Data       string `json:"data"`
		Offset     int    `json:"offset"`
		Limit      int    `json:"limit"`
	}
	json.Unmarshal([]byte(args), &p)

	if p.Action != "" {
		switch p.Action {
		case "list":
			return listSessions()
		case "poll":
			return pollSession(p.SessionID)
		case "log":
			return logSession(p.SessionID, p.Offset, p.Limit)
		case "write":
			return writeSession(p.SessionID, p.Data)
		case "kill":
			return killSession(p.SessionID)
		default:
			return &engine.ToolResult{Error: fmt.Sprintf("unknown action: %s. Valid: list, poll, log, write, kill", p.Action)}
		}
	}

	if p.Command == "" {
		return &engine.ToolResult{Error: "command required"}
	}

	if p.Background {
		sess := startBackground(ctx, p.Command, execWorkDir)
		if sess == nil {
			return &engine.ToolResult{Error: "failed to start background process"}
		}
		return &engine.ToolResult{Content: fmt.Sprintf("Started background session %s (pid=%d)\nCommand: %s\n\nUse action=poll session_id=%s to check output, action=kill session_id=%s to stop.",
			sess.id, sess.cmd.Process.Pid, sess.command, sess.id, sess.id)}
	}

	if p.Timeout == 0 {
		p.Timeout = 60
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(p.Timeout)*time.Second)
	defer cancel()

	command := prepareCommandWindows(p.Command)

	var shell, shellFlag string
	if runtime.GOOS == "windows" {
		shell = "cmd"
		shellFlag = "/c"
	} else {
		shell = "bash"
		shellFlag = "-c"
	}

	cmd := exec.CommandContext(timeoutCtx, shell, shellFlag, command)
	cmd.Dir = execWorkDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}

	if err := cmd.Start(); err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}

	var outBuf, errBuf strings.Builder
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		buf := make([]byte, 1024)
		for {
			n, readErr := stdout.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				outBuf.WriteString(chunk)
				engine.SendProgress(ctx, p.Command, chunk)
			}
			if readErr != nil {
				break
			}
		}
	}()

	go func() {
		defer wg.Done()
		buf := make([]byte, 1024)
		for {
			n, readErr := stderr.Read(buf)
			if n > 0 {
				errBuf.WriteString(string(buf[:n]))
			}
			if readErr != nil {
				break
			}
		}
	}()

	waitErr := cmd.Wait()
	wg.Wait()

	output := outBuf.String()
	if errOutput := errBuf.String(); errOutput != "" {
		output += "\n[stderr]\n" + errOutput
	}

	if waitErr != nil {
		if timeoutCtx.Err() != nil {
			return &engine.ToolResult{Content: output, Error: fmt.Sprintf("command timed out after %ds", p.Timeout)}
		}
		return &engine.ToolResult{Content: output, Error: waitErr.Error()}
	}
	return &engine.ToolResult{Content: output}
}

func listSessions() *engine.ToolResult {
	execMu.Lock()
	defer execMu.Unlock()

	if len(execSessions) == 0 {
		return &engine.ToolResult{Content: "No background sessions."}
	}

	var out strings.Builder
	for id, sess := range execSessions {
		summary := sessionSummary(sess)
		lastOutput, _ := readCombined(sess, 0, 200)
		lastOutput = strings.ReplaceAll(lastOutput, "\n", "\\n")
		if len(lastOutput) > 300 {
			lastOutput = lastOutput[:300] + "..."
		}
		fmt.Fprintf(&out, "%s | %s | %s | last: %s\n", id, summary, sess.command, lastOutput)
	}
	return &engine.ToolResult{Content: out.String()}
}

func pollSession(id string) *engine.ToolResult {
	execMu.Lock()
	sess := execSessions[id]
	execMu.Unlock()

	if sess == nil {
		return &engine.ToolResult{Error: fmt.Sprintf("session %s not found. Use action=list to see active sessions.", id)}
	}

	sess.stdout.mu.Lock()
	sess.stderr.mu.Lock()
	startOffset := sess.stdout.total
	stderrStart := sess.stderr.total
	sess.stdout.mu.Unlock()
	sess.stderr.mu.Unlock()

	sess.stdout.resetNotify()

	select {
	case <-sess.stdout.notify:
	case <-sess.done:
	case <-time.After(30 * time.Second):
	}

	out, outTotal := sess.stdout.read(startOffset, 0)
	err, _ := sess.stderr.read(stderrStart, 0)

	var result string
	if out != "" && err != "" {
		result = out + "\n[stderr]\n" + err
	} else if err != "" {
		result = err
	} else {
		result = out
	}

	sess.mu.Lock()
	exitCode := sess.exitCode
	sess.mu.Unlock()

	select {
	case <-sess.done:
		return &engine.ToolResult{Content: fmt.Sprintf("[exited code=%d]\n%s\nTotal output: %d bytes", exitCode, result, outTotal)}
	default:
		if result == "" {
			return &engine.ToolResult{Content: fmt.Sprintf("[running pid=%d, elapsed=%ds, no new output in 30s]\nPoll again or check log.", sess.cmd.Process.Pid, int(time.Since(sess.started).Seconds()))}
		}
		return &engine.ToolResult{Content: fmt.Sprintf("[running pid=%d, elapsed=%ds]\n%s", sess.cmd.Process.Pid, int(time.Since(sess.started).Seconds()), result)}
	}
}

func logSession(id string, offset, limit int) *engine.ToolResult {
	execMu.Lock()
	sess := execSessions[id]
	execMu.Unlock()

	if sess == nil {
		return &engine.ToolResult{Error: fmt.Sprintf("session %s not found. Use action=list to see active sessions.", id)}
	}

	if limit <= 0 {
		limit = 5000
	}

	output, total := readCombined(sess, offset, limit)
	summary := sessionSummary(sess)

	if output == "" {
		return &engine.ToolResult{Content: fmt.Sprintf("[%s] (no output yet, %d bytes total)\n", summary, total)}
	}
	return &engine.ToolResult{Content: fmt.Sprintf("[%s, total=%d bytes, offset=%d]\n%s", summary, total, offset, output)}
}

func writeSession(id, data string) *engine.ToolResult {
	execMu.Lock()
	sess := execSessions[id]
	execMu.Unlock()

	if sess == nil {
		return &engine.ToolResult{Error: fmt.Sprintf("session %s not found. Use action=list to see active sessions.", id)}
	}

	select {
	case <-sess.done:
		return &engine.ToolResult{Error: "process already exited"}
	default:
	}

	if sess.stdin == nil {
		return &engine.ToolResult{Error: "no stdin available for this process"}
	}

	_, err := io.WriteString(sess.stdin, data)
	if err != nil {
		return &engine.ToolResult{Error: fmt.Sprintf("write failed: %v", err)}
	}
	return &engine.ToolResult{Content: fmt.Sprintf("Wrote %d bytes to %s", len(data), id)}
}

func killSession(id string) *engine.ToolResult {
	execMu.Lock()
	sess := execSessions[id]
	execMu.Unlock()

	if sess == nil {
		return &engine.ToolResult{Error: fmt.Sprintf("session %s not found", id)}
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

	finalOutput, _ := readCombined(sess, 0, 5000)
	sess.mu.Lock()
	code := sess.exitCode
	sess.mu.Unlock()

	return &engine.ToolResult{Content: fmt.Sprintf("Killed %s (exit code=%d)\n\nFinal output:\n%s", id, code, finalOutput)}
}

// RegisterExecTools registers the run_command tool.
func RegisterExecTools(eng *engine.Engine) {
	eng.RegisterTool("run_command", execHandler)
}
