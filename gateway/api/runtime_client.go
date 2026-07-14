package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// RuntimeClient is the HTTP client for the Runtime service.
type RuntimeClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewRuntimeClient(baseURL string) *RuntimeClient {
	return &RuntimeClient{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 0},
	}
}

// CreateThreadRequest is the JSON body for POST /v1/threads.
type CreateThreadRequest struct {
	SystemPrompt  string         `json:"system_prompt"`
	Model         map[string]any `json:"model"`
	Tools         []map[string]any `json:"tools"`
	MaxContextPct int            `json:"max_context_pct"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// CreateThreadResponse is the JSON response for POST /v1/threads.
type CreateThreadResponse struct {
	ID string `json:"id"`
}

// TurnRequest is the JSON body for POST /v1/threads/{id}/turns.
type TurnRequest struct {
	Message string   `json:"message"`
	Images  []string `json:"images,omitempty"`
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
func (c *RuntimeClient) SendTurn(threadID string, req TurnRequest) (*http.Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest("POST", c.BaseURL+"/v1/threads/"+threadID+"/turns", bytes.NewReader(body))
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

// ── unused local import guard ──
var _ = time.Now
