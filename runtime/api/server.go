package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"beleader/runtime/engine"
	"beleader/runtime/llm"
	"beleader/runtime/store"
	"beleader/runtime/tools"
)

// Server is the Runtime HTTP server.
type Server struct {
	eng     *engine.Engine
	threads map[string]*engine.Thread
	mu      sync.RWMutex
	dataDir string
}

// NewServer creates a new Runtime server.
func NewServer(dataDir string) *Server {
	eng := engine.NewEngine()
	tools.RegisterAll(eng)
	s := &Server{
		eng:     eng,
		threads: make(map[string]*engine.Thread),
		dataDir: dataDir,
	}
	tools.SetWorkerGlobals(eng, s.threads)
	return s
}

// CreateThreadRequest is the JSON body for POST /v1/threads.
type CreateThreadRequest struct {
	SystemPrompt  string             `json:"system_prompt"`
	Model         engine.ModelConfig `json:"model"`
	Tools         []engine.ToolDef   `json:"tools"`
	MaxContextPct int                `json:"max_context_pct"`
	Metadata      map[string]any     `json:"metadata,omitempty"`
}

// CreateThreadResponse is the JSON response for POST /v1/threads.
type CreateThreadResponse struct {
	ID string `json:"id"`
}

// TurnRequest is the JSON body for POST /v1/threads/{id}/turns.
type TurnRequest struct {
	Message string             `json:"message"`
	Images  []string           `json:"images,omitempty"`
	Model   *engine.ModelConfig `json:"model,omitempty"`
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(204)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/v1")
	switch {
	case r.Method == "GET" && path == "/threads":
		s.handleListThreads(w, r)
	case r.Method == "POST" && path == "/threads":
		s.handleCreateThread(w, r)
	case r.Method == "GET" && strings.HasPrefix(path, "/threads/") && strings.HasSuffix(path, "/events"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/threads/"), "/events")
		s.handleEvents(w, r, id)
	case r.Method == "GET" && strings.HasPrefix(path, "/threads/") && strings.HasSuffix(path, "/latest-seq"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/threads/"), "/latest-seq")
		s.handleLatestSeq(w, r, id)
	case r.Method == "GET" && strings.HasPrefix(path, "/threads/"):
		id := strings.TrimPrefix(path, "/threads/")
		s.handleGetThread(w, r, id)
	case r.Method == "POST" && strings.HasPrefix(path, "/threads/") && strings.HasSuffix(path, "/turns"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/threads/"), "/turns")
		s.handleTurn(w, r, id)
	case r.Method == "DELETE" && strings.HasPrefix(path, "/threads/"):
		id := strings.TrimPrefix(path, "/threads/")
		s.handleDeleteThread(w, r, id)
	case r.Method == "GET" && path == "/health":
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	default:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
	}
}

func (s *Server) handleListThreads(w http.ResponseWriter, r *http.Request) {
	ids, err := store.ListThreadIDs(s.dataDir)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if ids == nil {
		ids = []string{}
	}
	type threadSummary struct {
		ID        string         `json:"id"`
		Metadata  map[string]any `json:"metadata,omitempty"`
		CreatedAt string         `json:"created_at"`
	}
	var summaries []threadSummary
	for _, id := range ids {
		t, err := store.LoadThread(s.dataDir, id)
		if err != nil {
			continue
		}
		summaries = append(summaries, threadSummary{
			ID:        t.ID,
			Metadata:  t.Metadata,
			CreatedAt: t.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	writeJSON(w, 200, map[string]any{"threads": summaries})
}

func (s *Server) handleGetThread(w http.ResponseWriter, r *http.Request, threadID string) {
	t, err := store.LoadThread(s.dataDir, threadID)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "thread not found"})
		return
	}
	writeJSON(w, 200, t)
}

func (s *Server) handleCreateThread(w http.ResponseWriter, r *http.Request) {
	var req CreateThreadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request: " + err.Error()})
		return
	}

	if req.MaxContextPct <= 0 {
		req.MaxContextPct = 60
	}
	if req.Model.ContextLimit <= 0 {
		req.Model.ContextLimit = 128000
	}
	if req.Metadata == nil {
		req.Metadata = map[string]any{}
	}

	thread := engine.NewThread(req.SystemPrompt, req.Model, req.Tools, "", req.MaxContextPct, req.Metadata, nil)
	thread.OnMessageAppend = func(msg *engine.Message) {
		store.AppendMessage(s.dataDir, thread.ID, msg)
	}

	if err := store.SaveThread(s.dataDir, thread); err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to save thread: " + err.Error()})
		return
	}

	// Ensure events.jsonl exists so SSE connections can open it immediately.
	if err := store.InitEvents(s.dataDir, thread.ID); err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to init events: " + err.Error()})
		return
	}

	s.mu.Lock()
	s.threads[thread.ID] = thread
	s.mu.Unlock()

	log.Printf("[thread] created %s (model=%s)", thread.ID, thread.Model.Model)
	writeJSON(w, 200, CreateThreadResponse{ID: thread.ID})
}

func (s *Server) handleTurn(w http.ResponseWriter, r *http.Request, threadID string) {
	// Try memory first, then disk.
	s.mu.RLock()
	thread, ok := s.threads[threadID]
	s.mu.RUnlock()
	if !ok {
		var err error
		thread, err = store.LoadThread(s.dataDir, threadID)
		if err != nil {
			writeJSON(w, 404, map[string]string{"error": "thread not found"})
			return
		}
		msgs, _ := store.LoadMessages(s.dataDir, threadID)
		thread.Messages = msgs
		s.mu.Lock()
		s.threads[threadID] = thread
		s.mu.Unlock()
	}

	thread.OnMessageAppend = func(msg *engine.Message) {
		store.AppendMessage(s.dataDir, threadID, msg)
	}

	var req TurnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request: " + err.Error()})
		return
	}

	// Update model config if provided (per-turn override).
	if req.Model != nil {
		thread.Model = *req.Model
		store.SaveThread(s.dataDir, thread)
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, 500, map[string]string{"error": "streaming not supported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(200)
	flusher.Flush()

	// Open event writer for persistence.
	ew, err := store.NewEventWriter(s.dataDir, threadID)
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: {\"message\":\"%s\"}\n\n", escapeJSON(err.Error()))
		flusher.Flush()
		return
	}
	defer ew.Close()

	llmClient := llm.New(thread.Model.BaseURL, thread.Model.APIKey, thread.Model.Model)
	tools.SetExecWorkDir(thread.WorkspaceDir)
	toolList := engine.ToolDefsToOpenAI(thread.ToolDefs)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	pauseCh := make(chan struct{})
	interveneCh := make(chan engine.InterveneMsg, 1)

	// Event emission callback: writes SSE + persists to events.jsonl.
	// Injects thread_id and turn_id into every event automatically.
	turnID := engine.NewTurnID()
	emit := func(ev engine.RuntimeEventRecord) {
		if ev.ThreadID == "" {
			ev.ThreadID = threadID
		}
		if ev.TurnID == "" {
			ev.TurnID = turnID
		}
		ew.Append(&ev)
		b, _ := json.Marshal(ev)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Event, string(b))
		flusher.Flush()
	}

	// Emit turn.started.
	emit(engine.RuntimeEventRecord{
		Event: engine.EventTurnStarted,
		Payload: map[string]any{"turn": engine.TurnRecord{
			ID:           turnID,
			ThreadID:     threadID,
			Status:       engine.TurnStatusInProgress,
			InputSummary: truncate(req.Message, 100),
			StartedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		}},
	})

	log.Printf("[turn] %s: %s", threadID, truncate(req.Message, 100))
	result, err := s.eng.RunLoop(ctx, thread, turnID, thread.SystemPrompt, req.Message, toolList, llmClient, thread.Model.ContextLimit, thread.Model.Vision, pauseCh, interveneCh, emit)
	if err != nil {
				emit(engine.FailItem("item_error", turnID, engine.ItemKindError, err.Error()))

		return
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	switch {
	case result.Completed:
		emit(engine.RuntimeEventRecord{
			Event: engine.EventTurnCompleted,
			Payload: map[string]any{"turn": engine.TurnRecord{
				ID:        turnID,
				ThreadID:  threadID,
				Status:    engine.TurnStatusCompleted,
				InputSummary: truncate(req.Message, 100),
				StartedAt:    now,
				EndedAt:      now,
			}},
		})
	case result.Paused:
		emit(engine.RuntimeEventRecord{
			Event: engine.EventTurnCompleted,
			Payload: map[string]any{"turn": engine.TurnRecord{
				ID:        turnID,
				ThreadID:  threadID,
				Status:    engine.TurnStatusInterrupted,
				InputSummary: truncate(req.Message, 100),
				StartedAt:    now,
				EndedAt:      now,
			}},
		})
	case result.Stopped:
		emit(engine.RuntimeEventRecord{
			Event: engine.EventTurnCompleted,
			Payload: map[string]any{"turn": engine.TurnRecord{
				ID:        turnID,
				ThreadID:  threadID,
				Status:    engine.TurnStatusInterrupted,
				InputSummary: truncate(req.Message, 100),
				StartedAt:    now,
				EndedAt:      now,
			}},
		})
	case result.Error != "":
		emit(engine.FailItem("item_error", turnID, engine.ItemKindError, result.Error))
		emit(engine.RuntimeEventRecord{
			Event: engine.EventTurnCompleted,
			Payload: map[string]any{"turn": engine.TurnRecord{
				ID:        turnID,
				ThreadID:  threadID,
				Status:    engine.TurnStatusInterrupted,
				InputSummary: truncate(req.Message, 100),
				StartedAt:    now,
				EndedAt:      now,
			}},
		})
	}

	// Sync messages.jsonl with pruned in-memory state after compression.
	if len(thread.PinnedIDs) > 0 {
		store.TruncateMessages(s.dataDir, threadID, thread.Messages)
	}

	// Save final thread state.
	store.SaveThread(s.dataDir, thread)
	log.Printf("[turn] %s: done (rounds=%d)", threadID, result.Rounds)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request, threadID string) {
	sinceSeq := int64(0)
	if q := r.URL.Query().Get("since_seq"); q != "" {
		sinceSeq, _ = strconv.ParseInt(q, 10, 64)
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, 500, map[string]string{"error": "streaming not supported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(200)
	flusher.Flush()

	ch := make(chan engine.RuntimeEventRecord, 100)
	go store.ReadEvents(s.dataDir, threadID, sinceSeq, ch)

	for ev := range ch {
		b, _ := json.Marshal(ev)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Event, string(b))
		flusher.Flush()
	}
}

func (s *Server) handleLatestSeq(w http.ResponseWriter, r *http.Request, threadID string) {
	seq, err := store.EventSeq(s.dataDir, threadID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]int64{"seq": seq})
}

func (s *Server) handleDeleteThread(w http.ResponseWriter, r *http.Request, threadID string) {
	s.mu.Lock()
	delete(s.threads, threadID)
	s.mu.Unlock()

	if err := store.DeleteThread(s.dataDir, threadID); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	log.Printf("[thread] deleted %s", threadID)
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}