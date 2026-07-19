package store

import (
	"bufio"
	"context"
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

// NewEventWriter opens or creates the events.jsonl file under threadDir.
func NewEventWriter(threadDir, threadID string) (*EventWriter, error) {
	if err := os.MkdirAll(threadDir, 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(threadDir, "events.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	seq := countLines(f)
	return &EventWriter{file: f, seq: seq}, nil
}

// InitEvents creates an empty events.jsonl under threadDir if it doesn't exist.
func InitEvents(threadDir, threadID string) error {
	if err := os.MkdirAll(threadDir, 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(threadDir, "events.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	return f.Close()
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
	f.Seek(0, 2)
	return count
}

func (w *EventWriter) Append(ev *engine.RuntimeEventRecord) error {
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

func (w *EventWriter) Close() error {
	return w.file.Close()
}

// EventSeq returns the current max event sequence number for a thread.
func EventSeq(threadDir, threadID string) (int64, error) {
	path := filepath.Join(threadDir, "events.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer f.Close()
	return countLines(f), nil
}

// ReadEvents reads events from events.jsonl under threadDir starting at sinceSeq.
// It polls for new events until ctx is cancelled.
func ReadEvents(ctx context.Context, threadDir, threadID string, sinceSeq int64, ch chan<- engine.RuntimeEventRecord) error {
	defer close(ch)

	path := filepath.Join(threadDir, "events.jsonl")
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

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(200 * time.Millisecond):
		}

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
