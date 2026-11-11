package evidence_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"orchestra/internal/evidence"
)

func TestEvidenceLogger_ConcurrentAppendAndRead(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), fmt.Sprintf("evidence-test-%d", time.Now().UnixNano()))
	defer os.RemoveAll(tmpDir)

	logFile := filepath.Join(tmpDir, "evidence.jsonl")
	l, err := evidence.NewLogger(logFile)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	defer l.Close()

	workers := 10
	recordsPerWorker := 50
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			wName := fmt.Sprintf("worker-%d", workerID)
			for i := 0; i < recordsPerWorker; i++ {
				_ = l.Record(evidence.EventRecord{
					WorkerID:   wName,
					WorkflowID: "wf-test",
					TaskID:     fmt.Sprintf("task-%d", i),
					Attempt:    1,
					Event:      evidence.EventStart,
				})
			}
		}(w)
	}

	wg.Wait()
	_ = l.Close()

	reader := evidence.NewReader(logFile)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to read back evidence: %v", err)
	}

	expectedTotal := workers * recordsPerWorker
	if len(records) != expectedTotal {
		t.Fatalf("expected %d records, got %d", expectedTotal, len(records))
	}
}
