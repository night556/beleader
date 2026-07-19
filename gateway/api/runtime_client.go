package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// RuntimeClient is the HTTP client for the Runtime service.
type RuntimeClient struct {
	Name     string
	BaseURL  string
	HTTPClient *http.Client

	toolDefs []map[string]any
	toolsMu  sync.RWMutex
}

func NewRuntimeClient(name, baseURL string) *RuntimeClient {
	return &RuntimeClient{
		Name:       name,
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 0},
	}
}

// MCPConfig is the config for a single MCP server passed to Runtime.
type MCPConfig struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Command string `json:"command,omitempty"`
	Args    string `json:"args,omitempty"`
	Env     string `json:"env,omitempty"`
	URL     string `json:"url,omitempty"`
	Headers string `json:"headers,omitempty"`
}

// CreateThreadRequest is the JSON body for POST /v1/threads.
type CreateThreadRequest struct {
	ThreadID          string           `json:"thread_id"`
	ThreadDir         string           `json:"thread_dir"`
	WorkspaceDir      string           `json:"workspace_dir"`
	RestrictWorkspace bool             `json:"restrict_workspace"`
	SystemPrompt      string           `json:"system_prompt"`
	Model             map[string]any   `json:"model"`
	Tools             []map[string]any `json:"tools"`
	MaxContextPct     int              `json:"max_context_pct"`
	MCPServers        []MCPConfig      `json:"mcp_servers,omitempty"`
	Metadata          map[string]any   `json:"metadata,omitempty"`
}

// CreateThreadResponse is the JSON response for POST /v1/threads.
type CreateThreadResponse struct {
	ID string `json:"id"`
}

// TurnRequest is the JSON body for POST /v1/threads/{id}/turns.
type TurnRequest struct {
	Message     string         `json:"message"`
	Images      []string       `json:"images,omitempty"`
	Model       map[string]any `json:"model,omitempty"`
	ThreadDir   string         `json:"thread_dir"`
	WorkspaceDir string        `json:"workspace_dir"`
}

// CreateThread creates a new thread in the Runtime service.
func (c *RuntimeClient) CreateThread(req CreateThreadRequest) (*CreateThreadResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Post(c.BaseURL+"/v1/threads", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create thread: %s — %s", resp.Status, string(b))
	}

	var result CreateThreadResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteThread deletes a thread in the Runtime service.
func (c *RuntimeClient) DeleteThread(id string) error {
	req, err := http.NewRequest("DELETE", c.BaseURL+"/v1/threads/"+id, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// SendTurn sends a user message to a thread and returns the SSE response body.
func (c *RuntimeClient) SendTurn(ctx context.Context, threadID string, req TurnRequest) (*http.Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/v1/threads/"+threadID+"/turns", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("send turn: %s — %s", resp.Status, string(b))
	}

	return resp, nil
}

// FetchEvents fetches events for a thread since a given seq from the Runtime.
func (c *RuntimeClient) FetchEvents(threadID string, sinceSeq int64) (*http.Response, error) {
	url := fmt.Sprintf("%s/v1/threads/%s/events?since_seq=%d", c.BaseURL, threadID, sinceSeq)
	resp, err := c.HTTPClient.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("fetch events: %s — %s", resp.Status, string(b))
	}
	return resp, nil
}

// ParseSSEStream parses an SSE event stream, calling onEvent for each event.
func ParseSSEStream(body io.ReadCloser, onEvent func(eventType string, payload map[string]any)) error {
	defer body.Close()
	scanner := bufio.NewScanner(body)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var eventType string
	var dataBuf bytes.Buffer

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if dataBuf.Len() > 0 {
				var payload map[string]any
				json.Unmarshal(dataBuf.Bytes(), &payload)
				onEvent(eventType, payload)
				dataBuf.Reset()
				eventType = ""
			}
			continue
		}

		if len(line) > 6 && line[:6] == "event:" {
			eventType = line[7:]
		} else if len(line) > 5 && line[:5] == "data:" {
			if dataBuf.Len() > 0 {
				dataBuf.WriteByte('\n')
			}
			dataBuf.WriteString(line[6:])
		}
	}

	// Flush remaining.
	if dataBuf.Len() > 0 {
		var payload map[string]any
		json.Unmarshal(dataBuf.Bytes(), &payload)
		onEvent(eventType, payload)
	}

	return scanner.Err()
}

// ParseAndForwardSSE reads an SSE stream from body, writes each line to w,
// and calls onEvent for each parsed event. Flushes after each event.
func ParseAndForwardSSE(body io.ReadCloser, w io.Writer, flusher http.Flusher, onEvent func(eventType string, payload map[string]any)) error {
	defer body.Close()
	scanner := bufio.NewScanner(body)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var eventType string
	var dataBuf bytes.Buffer

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			// Blank line: end of event — write the event block
			if dataBuf.Len() > 0 {
				fmt.Fprintf(w, "event: %s\n", eventType)
				fmt.Fprintf(w, "data: %s\n\n", dataBuf.String())
				flusher.Flush()

				var payload map[string]any
				json.Unmarshal(dataBuf.Bytes(), &payload)
				onEvent(eventType, payload)
				dataBuf.Reset()
				eventType = ""
			}
			continue
		}

		if len(line) > 6 && line[:6] == "event:" {
			eventType = line[7:]
		} else if len(line) > 5 && line[:5] == "data:" {
			if dataBuf.Len() > 0 {
				dataBuf.WriteByte('\n')
			}
			dataBuf.WriteString(line[6:])
		}
	}

	// Flush remaining.
	if dataBuf.Len() > 0 {
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, dataBuf.String())
		flusher.Flush()
		var payload map[string]any
		json.Unmarshal(dataBuf.Bytes(), &payload)
		onEvent(eventType, payload)
	}

	return scanner.Err()
}

// ToolDefs returns cached tool definitions, fetching from Runtime on first call.
func (c *RuntimeClient) ToolDefs() ([]map[string]any, error) {
	c.toolsMu.RLock()
	if c.toolDefs != nil {
		defer c.toolsMu.RUnlock()
		return c.toolDefs, nil
	}
	c.toolsMu.RUnlock()

	c.toolsMu.Lock()
	defer c.toolsMu.Unlock()
	if c.toolDefs != nil {
		return c.toolDefs, nil
	}

	resp, err := c.HTTPClient.Get(c.BaseURL + "/v1/tools")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("fetch tools: %s", resp.Status)
	}
	var result struct {
		Tools []map[string]any `json:"tools"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	c.toolDefs = result.Tools
	return c.toolDefs, nil
}

// ── Runtime client pool ──

// RuntimeClientPool manages connections to multiple equivalent Runtimes.
type RuntimeClientPool struct {
	clients []*RuntimeClient
	mu      sync.RWMutex
}

func NewRuntimeClientPool() *RuntimeClientPool {
	return &RuntimeClientPool{clients: make([]*RuntimeClient, 0)}
}

func (p *RuntimeClientPool) Set(name, baseURL string) *RuntimeClient {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Replace existing entry with the same name (re-registration).
	for i, c := range p.clients {
		if c.Name == name {
			p.clients[i] = NewRuntimeClient(name, baseURL)
			return p.clients[i]
		}
	}
	c := NewRuntimeClient(name, baseURL)
	p.clients = append(p.clients, c)
	return c
}

// Pick returns any available client (first alive in the pool).
func (p *RuntimeClientPool) Pick() (*RuntimeClient, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if len(p.clients) == 0 {
		return nil, false
	}
	return p.clients[0], true
}

func (p *RuntimeClientPool) Remove(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, c := range p.clients {
		if c.Name == name {
			p.clients = append(p.clients[:i], p.clients[i+1:]...)
			return
		}
	}
}

// ToolDefs returns cached tool definitions fetched from any available Runtime.
func (p *RuntimeClientPool) ToolDefs() ([]map[string]any, error) {
	c, ok := p.Pick()
	if !ok {
		return nil, fmt.Errorf("no runtime available")
	}
	return c.ToolDefs()
}

// HasAny returns whether at least one runtime is registered.
func (p *RuntimeClientPool) HasAny() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.clients) > 0
}

// GetLatestSeq returns the current max event seq for a thread.
func (c *RuntimeClient) GetLatestSeq(threadID string) (int64, error) {
	resp, err := c.HTTPClient.Get(c.BaseURL + "/v1/threads/" + threadID + "/latest-seq")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("get latest seq: %s", resp.Status)
	}
	var result struct {
		Seq int64 `json:"seq"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}
	return result.Seq, nil
}
