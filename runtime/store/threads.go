package store

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"

	"beleader/runtime/engine"
)

// ThreadDir returns the directory path for a thread given the data root and ID.
func ThreadDir(dataDir, threadID string) string {
	return filepath.Join(dataDir, "threads", threadID)
}

// EnsureThreadDir creates the thread directory structure.
func EnsureThreadDir(threadDir string) error {
	for _, sub := range []string{threadDir, filepath.Join(threadDir, "workspace"), filepath.Join(threadDir, ".trash")} {
		if err := os.MkdirAll(sub, 0755); err != nil {
			return err
		}
	}
	return nil
}

// Save writes the thread state to thread.json under threadDir.
func SaveThread(threadDir string, thread *engine.Thread) error {
	if err := EnsureThreadDir(threadDir); err != nil {
		return err
	}
	thread.DataDir = threadDir
	thread.WorkspaceDir = filepath.Join(threadDir, "workspace")

	f, err := os.Create(filepath.Join(threadDir, "thread.json"))
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(thread)
}

// Load reads the thread state from thread.json in threadDir.
func LoadThread(threadDir string) (*engine.Thread, error) {
	f, err := os.Open(filepath.Join(threadDir, "thread.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var t engine.Thread
	if err := json.NewDecoder(f).Decode(&t); err != nil {
		return nil, err
	}
	t.DataDir = threadDir
	t.WorkspaceDir = filepath.Join(threadDir, "workspace")
	return &t, nil
}

// Delete removes the entire thread directory.
func DeleteThread(threadDir string) error {
	return os.RemoveAll(threadDir)
}

// ListIDs returns all thread IDs in the data directory.
func ListThreadIDs(dataDir string) ([]string, error) {
	dir := filepath.Join(dataDir, "threads")
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

// AppendMessage appends a single message to messages.jsonl under threadDir.
func AppendMessage(threadDir, threadID string, msg *engine.Message) error {
	if err := os.MkdirAll(threadDir, 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(filepath.Join(threadDir, "messages.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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

// LoadMessages reads messages.jsonl from threadDir line by line.
func LoadMessages(threadDir, threadID string) ([]engine.Message, error) {
	path := filepath.Join(threadDir, "messages.jsonl")

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

// TruncateMessages rewrites messages.jsonl under threadDir with a full snapshot.
func TruncateMessages(threadDir, threadID string, msgs []engine.Message) error {
	path := filepath.Join(threadDir, "messages.jsonl")

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
