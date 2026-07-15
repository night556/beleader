package store

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"

	"beleader/runtime/engine"
)

// Dir returns the threads data directory under the given root.
func Dir(dataDir string) string {
	return filepath.Join(dataDir, "threads")
}

// ThreadDir returns the directory for a specific thread.
func ThreadDir(dataDir, threadID string) string {
	return filepath.Join(Dir(dataDir), threadID)
}

// EnsureThreadDir creates the thread directory structure:
//
//	{dataDir}/threads/{id}/
//	  ├── thread.json
//	  ├── events.jsonl
//	  ├── STATUS.md
//	  ├── .trash/
//	  └── workspace/
func EnsureThreadDir(dataDir, threadID string) (string, error) {
	dir := ThreadDir(dataDir, threadID)
	for _, sub := range []string{dir, filepath.Join(dir, "workspace"), filepath.Join(dir, ".trash")} {
		if err := os.MkdirAll(sub, 0755); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// Save writes the thread state to thread.json.
func SaveThread(dataDir string, thread *engine.Thread) error {
	dir, err := EnsureThreadDir(dataDir, thread.ID)
	if err != nil {
		return err
	}
	thread.DataDir = dir
	thread.WorkspaceDir = filepath.Join(dir, "workspace")

	f, err := os.Create(filepath.Join(dir, "thread.json"))
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(thread)
}

// Load reads the thread state from thread.json.
func LoadThread(dataDir, threadID string) (*engine.Thread, error) {
	f, err := os.Open(filepath.Join(ThreadDir(dataDir, threadID), "thread.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var t engine.Thread
	if err := json.NewDecoder(f).Decode(&t); err != nil {
		return nil, err
	}
	dir := ThreadDir(dataDir, threadID)
	t.DataDir = dir
	t.WorkspaceDir = filepath.Join(dir, "workspace")
	return &t, nil
}

// Delete removes the entire thread directory.
func DeleteThread(dataDir, threadID string) error {
	return os.RemoveAll(ThreadDir(dataDir, threadID))
}

// ListIDs returns all thread IDs in the data directory.
func ListThreadIDs(dataDir string) ([]string, error) {
	dir := Dir(dataDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	return ids, nil
}

// AppendMessage appends a single message to messages.jsonl (append-only).
func AppendMessage(dataDir, threadID string, msg *engine.Message) error {
	dir := ThreadDir(dataDir, threadID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(filepath.Join(dir, "messages.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = f.Write(b)
	return err
}

// LoadMessages reads messages.jsonl line by line and returns all messages.
// If lastKnownSeq is set, it also replays events from events.jsonl after that seq
// to catch any messages written to events but not yet to messages.jsonl (crash recovery).
func LoadMessages(dataDir, threadID string) ([]engine.Message, error) {
	dir := ThreadDir(dataDir, threadID)
	path := filepath.Join(dir, "messages.jsonl")

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var msgs []engine.Message
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)
	for scanner.Scan() {
		var m engine.Message
		if err := json.Unmarshal(scanner.Bytes(), &m); err != nil {
			continue
		}
		msgs = append(msgs, m)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return msgs, nil
}

// TruncateMessages rewrites messages.jsonl with a full snapshot of messages.
// Called after PruneCompressed to sync disk with in-memory state.
func TruncateMessages(dataDir, threadID string, msgs []engine.Message) error {
	dir := ThreadDir(dataDir, threadID)
	path := filepath.Join(dir, "messages.jsonl")

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	for i := range msgs {
		b, err := json.Marshal(&msgs[i])
		if err != nil {
			return err
		}
		b = append(b, '\n')
		if _, err := f.Write(b); err != nil {
			return err
		}
	}
	return nil
}
