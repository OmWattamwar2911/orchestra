package evidence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// EventType defines discrete lifecycle points recorded directly by workers.
type EventType string

const (
	EventStart     EventType = "START"
	EventHeartbeat EventType = "HEARTBEAT"
	EventFinish    EventType = "FINISH"
)

var monotonicBase = time.Now()

// MonotonicNowNs returns nanoseconds elapsed since process initiation using the monotonic clock.
func MonotonicNowNs() int64 {
	return time.Since(monotonicBase).Nanoseconds()
}

// EventRecord captures an independent audit observation recorded directly by workers.
type EventRecord struct {
	EventID      string    `json:"event_id"`
	TimestampNs  int64     `json:"timestamp_ns"`  // Wall-clock timestamp in nanoseconds
	WallClockNs  int64     `json:"wall_clock_ns"` // Wall-clock nanoseconds
	MonotonicNs  int64     `json:"monotonic_ns"`  // Monotonic nanoseconds
	WorkerID     string    `json:"worker_id"`
	WorkflowID   string    `json:"workflow_id"`
	TaskID       string    `json:"task_id"`
	Attempt      int32     `json:"attempt"`
	AttemptID    int32     `json:"attempt_id"` // Canonical attempt id alias
	Event        EventType `json:"event"`
	Status       string    `json:"status,omitempty"` // SUCCESS, FAILURE, TIMEOUT
	Output       string    `json:"output,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
}

// Logger writes immutable, append-only verification events for the independent invariant checker.
type Logger struct {
	// mu protects the append-only evidence log file from interleaved partial writes across worker threads
	// Invariant protected: Guarantees atomic line-delimited JSON serialization of concurrent evidence events
	mu   sync.Mutex
	file *os.File
	path string
}

// NewLogger initializes an evidence logger targeting a specified file path.
func NewLogger(filePath string) (*Logger, error) {
	if filePath == "" {
		filePath = filepath.Join("results", "evidence.jsonl")
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create evidence log directory: %w", err)
	}

	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open evidence log file: %w", err)
	}

	return &Logger{
		file: f,
		path: filePath,
	}, nil
}

// Record appends an audit event to the evidence log.
func (l *Logger) Record(rec EventRecord) error {
	now := time.Now()
	if rec.WallClockNs == 0 {
		rec.WallClockNs = now.UnixNano()
	}
	if rec.TimestampNs == 0 {
		rec.TimestampNs = rec.WallClockNs
	}
	if rec.MonotonicNs == 0 {
		rec.MonotonicNs = time.Since(monotonicBase).Nanoseconds()
	}
	if rec.AttemptID == 0 && rec.Attempt != 0 {
		rec.AttemptID = rec.Attempt
	}
	if rec.Attempt == 0 && rec.AttemptID != 0 {
		rec.Attempt = rec.AttemptID
	}
	if rec.EventID == "" {
		rec.EventID = fmt.Sprintf("ev-%d", rec.TimestampNs)
	}

	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("failed to marshal evidence record: %w", err)
	}
	data = append(data, '\n')

	// Invariant protected: File write lock guarantees that log entries are never interleaved or corrupted
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, err := l.file.Write(data); err != nil {
		return fmt.Errorf("failed to write evidence record: %w", err)
	}
	return nil
}

// Close flushes and closes the evidence log file.
func (l *Logger) Close() error {
	// Invariant protected: Mutex ensures no in-flight write executes while the file descriptor is closed
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		return l.file.Close()
	}
	return nil
}
