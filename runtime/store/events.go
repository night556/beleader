package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"beleader/runtime/engine"
)

type EventWriter struct {
	mu   sync.Mutex
	file *os.File
	seq  int64
}

// NewEventWriter opens or creates the events.jsonl file for appending.
func NewEventWriter(dataDir, threadID string) (*EventWriter, error) {
	dir := ThreadDir(dataDir, threadID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	// Count existing lines to determine starting seq.
	seq := countLines(f)
	return &EventWriter{file: f, seq: seq}, nil
}

func countLines(f *os.File) int64 {
	fi, _ := f.Stat()
	if fi.Size() == 0 {
		return 0
	}
	scanner := bufio.NewScanner(f)
	var count int64
	for scanner.Scan() {
		count++
	}
	f.Seek(0, 2) // seek back to end
	return count
}

// Append writes an event to the JSONL file with an auto-incremented seq and timestamp.
func (w *EventWriter) Append(ev engine.RuntimeEventRecord) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.seq++
	ev.Seq = w.seq
	if ev.Timestamp == "" {
		ev.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}

	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w.file, "%s\n", b)
	return err
}

// Close closes the underlying file.
func (w *EventWriter) Close() error {
	return w.file.Close()
}

// ReadEvents reads events from events.jsonl starting at sinceSeq and sends them to ch.
// When it reaches the end of the file, it continues tailing until ch is closed or ctx is done.
func ReadEvents(dataDir, threadID string, sinceSeq int64, ch chan<- engine.RuntimeEventRecord) error {
	defer close(ch)

	path := filepath.Join(ThreadDir(dataDir, threadID), "events.jsonl")
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var lastSeq int64
	for scanner.Scan() {
		var ev engine.RuntimeEventRecord
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			continue
		}
		lastSeq = ev.Seq
		if ev.Seq > sinceSeq {
			ch <- ev
		}
	}

	// Live tail: watch for new events appended to the file.
	// Re-open and seek to last position for new data.
	for {
		time.Sleep(200 * time.Millisecond)
		f2, err := os.Open(path)
		if err != nil {
			return err
		}
		scanner2 := bufio.NewScanner(f2)
		scanner2.Buffer(buf, 10*1024*1024)
		var lastSeq2 int64
		for scanner2.Scan() {
			var ev engine.RuntimeEventRecord
			if err := json.Unmarshal(scanner2.Bytes(), &ev); err != nil {
				continue
			}
			lastSeq2 = ev.Seq
			if ev.Seq > lastSeq {
				ch <- ev
			}
		}
		if lastSeq2 > lastSeq {
			lastSeq = lastSeq2
		}
		f2.Close()
	}
}
