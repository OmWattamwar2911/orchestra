package worker_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pb "orchestra/api/proto/orchestra/v1"
	"orchestra/internal/evidence"
	"orchestra/internal/worker"
)

func TestExecutor_BuiltInTaskTypes(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), fmt.Sprintf("worker-test-%d", time.Now().UnixNano()))
	defer os.RemoveAll(tmpDir)

	evLogger, err := evidence.NewLogger(filepath.Join(tmpDir, "evidence.jsonl"))
	if err != nil {
		t.Fatalf("failed to create evidence logger: %v", err)
	}
	defer evLogger.Close()

	exec := worker.NewExecutor("test-worker", 4, evLogger, nil)
	ctx := context.Background()

	t.Run("echo task returns payload", func(t *testing.T) {
		asgn := &pb.TaskAssignment{
			WorkflowId: "wf-1",
			Task: &pb.TaskSpec{
				Id:             "t-echo",
				Type:           "echo",
				Payload:        "hello orchestra distributed scheduler",
				TimeoutSeconds: 5,
			},
			Attempt:            1,
			LeaderFencingToken: 10,
		}

		res := exec.ExecuteTask(ctx, asgn)
		if res.GetOutcome() != pb.TaskOutcome_TASK_OUTCOME_SUCCESS {
			t.Fatalf("expected SUCCESS, got %v", res.GetOutcome())
		}
		if string(res.GetOutput()) != "hello orchestra distributed scheduler" {
			t.Fatalf("unexpected output: %s", string(res.GetOutput()))
		}
	})

	t.Run("sleep task completes when within timeout", func(t *testing.T) {
		asgn := &pb.TaskAssignment{
			WorkflowId: "wf-1",
			Task: &pb.TaskSpec{
				Id:             "t-sleep",
				Type:           "sleep",
				Payload:        "50ms",
				TimeoutSeconds: 2,
			},
			Attempt:            1,
			LeaderFencingToken: 10,
		}

		start := time.Now()
		res := exec.ExecuteTask(ctx, asgn)
		elapsed := time.Since(start)

		if res.GetOutcome() != pb.TaskOutcome_TASK_OUTCOME_SUCCESS {
			t.Fatalf("expected SUCCESS, got %v", res.GetOutcome())
		}
		if elapsed < 40*time.Millisecond {
			t.Fatalf("sleep finished too quickly: %v", elapsed)
		}
	})

	t.Run("sleep task times out when exceeding timeout", func(t *testing.T) {
		asgn := &pb.TaskAssignment{
			WorkflowId: "wf-1",
			Task: &pb.TaskSpec{
				Id:             "t-sleep-timeout",
				Type:           "sleep",
				Payload:        "5s",
				TimeoutSeconds: 1, // timeout is 1s, payload asks for 5s
			},
			Attempt:            1,
			LeaderFencingToken: 10,
		}

		res := exec.ExecuteTask(ctx, asgn)
		if res.GetOutcome() != pb.TaskOutcome_TASK_OUTCOME_TIMEOUT {
			t.Fatalf("expected TIMEOUT, got %v", res.GetOutcome())
		}
	})

	t.Run("cpu task computes sha256 hash loop", func(t *testing.T) {
		asgn := &pb.TaskAssignment{
			WorkflowId: "wf-1",
			Task: &pb.TaskSpec{
				Id:             "t-cpu",
				Type:           "cpu",
				Payload:        "1000",
				TimeoutSeconds: 5,
			},
			Attempt:            1,
			LeaderFencingToken: 10,
		}

		res := exec.ExecuteTask(ctx, asgn)
		if res.GetOutcome() != pb.TaskOutcome_TASK_OUTCOME_SUCCESS {
			t.Fatalf("expected SUCCESS, got %v", res.GetOutcome())
		}
		outStr := string(res.GetOutput())
		if !strings.Contains(outStr, "cpu iterations=1000") || !strings.Contains(outStr, "hash=") {
			t.Fatalf("unexpected cpu output: %s", outStr)
		}
	})

	t.Run("flaky task failure probability", func(t *testing.T) {
		// 100% failure rate
		asgnFail := &pb.TaskAssignment{
			WorkflowId: "wf-1",
			Task: &pb.TaskSpec{
				Id:             "t-flaky-fail",
				Type:           "flaky",
				Payload:        "1.0",
				TimeoutSeconds: 5,
			},
			Attempt:            1,
			LeaderFencingToken: 10,
		}
		resFail := exec.ExecuteTask(ctx, asgnFail)
		if resFail.GetOutcome() != pb.TaskOutcome_TASK_OUTCOME_FAILURE {
			t.Fatalf("expected FAILURE for prob=1.0, got %v", resFail.GetOutcome())
		}

		// 0% failure rate
		asgnSuccess := &pb.TaskAssignment{
			WorkflowId: "wf-1",
			Task: &pb.TaskSpec{
				Id:             "t-flaky-pass",
				Type:           "flaky",
				Payload:        "0.0",
				TimeoutSeconds: 5,
			},
			Attempt:            1,
			LeaderFencingToken: 10,
		}
		resSuccess := exec.ExecuteTask(ctx, asgnSuccess)
		if resSuccess.GetOutcome() != pb.TaskOutcome_TASK_OUTCOME_SUCCESS {
			t.Fatalf("expected SUCCESS for prob=0.0, got %v", resSuccess.GetOutcome())
		}
	})

	t.Run("slow_then_crash invokes crash hook", func(t *testing.T) {
		var hookTriggered atomic.Bool
		exec.SetCrashHook(func() {
			hookTriggered.Store(true)
		})

		asgn := &pb.TaskAssignment{
			WorkflowId: "wf-1",
			Task: &pb.TaskSpec{
				Id:             "t-crash",
				Type:           "slow_then_crash",
				Payload:        "50ms",
				TimeoutSeconds: 5,
			},
			Attempt:            1,
			LeaderFencingToken: 10,
		}

		res := exec.ExecuteTask(ctx, asgn)
		if res.GetOutcome() != pb.TaskOutcome_TASK_OUTCOME_FAILURE {
			t.Fatalf("expected FAILURE on crash, got %v", res.GetOutcome())
		}
		if !hookTriggered.Load() {
			t.Fatal("expected crash hook to be invoked by slow_then_crash")
		}
	})

	// Verify independent evidence log contents
	t.Run("evidence log contains monotonic and wall clock records", func(t *testing.T) {
		reader := evidence.NewReader(filepath.Join(tmpDir, "evidence.jsonl"))
		records, err := reader.ReadAll()
		if err != nil {
			t.Fatalf("failed to read evidence log: %v", err)
		}
		if len(records) == 0 {
			t.Fatal("expected evidence records to be written, found 0")
		}

		for _, r := range records {
			if r.WorkerID != "test-worker" {
				t.Fatalf("expected worker_id 'test-worker', got '%s'", r.WorkerID)
			}
			if r.WallClockNs <= 0 {
				t.Fatalf("invalid wall_clock_ns: %d", r.WallClockNs)
			}
			if r.MonotonicNs < 0 {
				t.Fatalf("invalid monotonic_ns: %d", r.MonotonicNs)
			}
			if r.Attempt <= 0 {
				t.Fatalf("invalid attempt: %d", r.Attempt)
			}
		}
	})
}
