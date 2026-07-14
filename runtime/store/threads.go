package store

import (
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
