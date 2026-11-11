package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math/rand"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pb "orchestra/api/proto/orchestra/v1"
	"orchestra/internal/evidence"
	"orchestra/internal/metrics"
)

// Executor manages running tasks assigned to this worker.
type Executor struct {
	workerID       string
	capacity       int32
	activeTasks    atomic.Int32
	evidenceLogger *evidence.Logger
	logger         *slog.Logger

	// mu protects crashHook callback
	// Invariant protected: Synchronizes setting and invoking worker crash hook for chaos testing
	hookMu    sync.RWMutex
	crashHook func()
}

// NewExecutor creates a new task executor.
func NewExecutor(workerID string, capacity int, evidenceLogger *evidence.Logger, logger *slog.Logger) *Executor {
	if capacity <= 0 {
		capacity = 4
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &Executor{
		workerID:       workerID,
		capacity:       int32(capacity),
		evidenceLogger: evidenceLogger,
		logger:         logger.With(slog.String("component", "executor"), slog.String("worker_id", workerID)),
	}
}

// SetCrashHook registers a callback invoked when a "slow_then_crash" task triggers.
func (e *Executor) SetCrashHook(hook func()) {
	// Invariant protected: Lock protects setting chaos crash hook safely across threads
	e.hookMu.Lock()
	defer e.hookMu.Unlock()
	e.crashHook = hook
}

func (e *Executor) invokeCrashHook() {
	// Invariant protected: Read lock protects invoking crash hook safely
	e.hookMu.RLock()
	hook := e.crashHook
	e.hookMu.RUnlock()

	if hook != nil {
		hook()
	}
}

// ActiveTaskCount returns current number of actively executing tasks.
func (e *Executor) ActiveTaskCount() int32 {
	// Invariant protected: Atomic load guarantees thread-safe read of active task concurrency count
	return e.activeTasks.Load()
}

// ExecuteTask runs a single assigned task under context timeout and records audit events.
// Delivery semantics: At-least-once execution with exactly-one committed result per task.
func (e *Executor) ExecuteTask(ctx context.Context, assignment *pb.TaskAssignment) *pb.ReportResultRequest {
	// Invariant protected: Atomic increment tracks active task count, guarding capacity limit
	currentActive := e.activeTasks.Add(1)
	metrics.TasksRunning.Inc()
	defer func() {
		e.activeTasks.Add(-1)
		metrics.TasksRunning.Dec()
	}()

	spec := assignment.GetTask()
	taskID := spec.GetId()
	wfID := assignment.GetWorkflowId()
	attempt := assignment.GetAttempt()
	fencingToken := assignment.GetLeaderFencingToken()

	e.logger.Info("executing task assignment",
		slog.String("workflow_id", wfID),
		slog.String("task_id", taskID),
		slog.Int("attempt", int(attempt)),
		slog.Int64("fencing_token", fencingToken),
		slog.Int("active_tasks", int(currentActive)),
	)

	// 1. Record START event to independent evidence log
	if e.evidenceLogger != nil {
		now := time.Now()
		_ = e.evidenceLogger.Record(evidence.EventRecord{
			TimestampNs: now.UnixNano(),
			WallClockNs: now.UnixNano(),
			MonotonicNs: evidence.MonotonicNowNs(),
			WorkerID:    e.workerID,
			WorkflowID:  wfID,
			TaskID:      taskID,
			Attempt:     attempt,
			AttemptID:   attempt,
			Event:       evidence.EventStart,
		})
	}

	// 2. Set timeout context
	timeout := time.Duration(spec.GetTimeoutSeconds()) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	taskCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 3. Execute payload
	output, outcome, errMsg := e.runPayload(taskCtx, spec.GetType(), spec.GetPayload())

	// 4. Record metrics and FINISH event to independent evidence log
	statusStr := "SUCCESS"
	if outcome == pb.TaskOutcome_TASK_OUTCOME_FAILURE {
		statusStr = "FAILURE"
	} else if outcome == pb.TaskOutcome_TASK_OUTCOME_TIMEOUT {
		statusStr = "TIMEOUT"
	}
	metrics.TasksExecutedTotal.WithLabelValues(statusStr).Inc()

	if e.evidenceLogger != nil {
		now := time.Now()
		_ = e.evidenceLogger.Record(evidence.EventRecord{
			TimestampNs:  now.UnixNano(),
			WallClockNs:  now.UnixNano(),
			MonotonicNs:  evidence.MonotonicNowNs(),
			WorkerID:     e.workerID,
			WorkflowID:   wfID,
			TaskID:       taskID,
			Attempt:      attempt,
			AttemptID:    attempt,
			Event:        evidence.EventFinish,
			Status:       statusStr,
			Output:       string(output),
			ErrorMessage: errMsg,
		})
	}

	return &pb.ReportResultRequest{
		WorkflowId:   wfID,
		TaskId:       taskID,
		AttemptId:    attempt,
		FencingToken: fencingToken,
		WorkerId:     e.workerID,
		Outcome:      outcome,
		Output:       output,
		ErrorMessage: errMsg,
	}
}

func (e *Executor) runPayload(ctx context.Context, taskType, payload string) ([]byte, pb.TaskOutcome, string) {
	lowerType := strings.ToLower(strings.TrimSpace(taskType))

	switch lowerType {
	case "echo":
		return []byte(payload), pb.TaskOutcome_TASK_OUTCOME_SUCCESS, ""

	case "http":
		return ExecuteHTTPTask(ctx, payload)

	case "sleep":
		dur := 100 * time.Millisecond
		trimmed := strings.TrimSpace(payload)
		if trimmed != "" {
			if d, err := time.ParseDuration(trimmed); err == nil {
				dur = d
			} else if ms, err := strconv.Atoi(trimmed); err == nil {
				dur = time.Duration(ms) * time.Millisecond
			}
		}

		select {
		case <-time.After(dur):
			return []byte(fmt.Sprintf("slept %v", dur)), pb.TaskOutcome_TASK_OUTCOME_SUCCESS, ""
		case <-ctx.Done():
			return nil, pb.TaskOutcome_TASK_OUTCOME_TIMEOUT, "task timed out during sleep"
		}

	case "cpu":
		iterations := 50000
		trimmed := strings.TrimSpace(payload)
		if trimmed != "" {
			if n, err := strconv.Atoi(trimmed); err == nil && n > 0 {
				iterations = n
			}
		}

		h := sha256.New()
		data := []byte(payload)
		if len(data) == 0 {
			data = []byte("orchestra-cpu-payload")
		}

		for i := 0; i < iterations; i++ {
			if i%1000 == 0 && ctx.Err() != nil {
				return nil, pb.TaskOutcome_TASK_OUTCOME_TIMEOUT, "cpu task timed out"
			}
			h.Write(data)
			h.Write([]byte(strconv.Itoa(i)))
		}

		hashHex := hex.EncodeToString(h.Sum(nil))
		return []byte(fmt.Sprintf("cpu iterations=%d hash=%s", iterations, hashHex)), pb.TaskOutcome_TASK_OUTCOME_SUCCESS, ""

	case "flaky":
		prob := 0.5
		trimmed := strings.TrimSpace(payload)
		if trimmed != "" {
			trimmed = strings.TrimSuffix(trimmed, "%")
			if p, err := strconv.ParseFloat(trimmed, 64); err == nil {
				if p > 1.0 {
					p = p / 100.0 // Handle percentage (e.g. 80 -> 0.8)
				}
				prob = p
			}
		}

		if rand.Float64() < prob {
			return nil, pb.TaskOutcome_TASK_OUTCOME_FAILURE, fmt.Sprintf("flaky task failed (prob=%.2f)", prob)
		}
		return []byte(fmt.Sprintf("flaky task succeeded (prob=%.2f)", prob)), pb.TaskOutcome_TASK_OUTCOME_SUCCESS, ""

	case "slow_then_crash":
		dur := 100 * time.Millisecond
		trimmed := strings.TrimSpace(payload)
		if trimmed != "" {
			if d, err := time.ParseDuration(trimmed); err == nil {
				dur = d
			} else if ms, err := strconv.Atoi(trimmed); err == nil {
				dur = time.Duration(ms) * time.Millisecond
			}
		}

		select {
		case <-time.After(dur):
			e.logger.Warn("slow_then_crash triggering worker crash hook", slog.Duration("delay", dur))
			e.invokeCrashHook()
			return nil, pb.TaskOutcome_TASK_OUTCOME_FAILURE, "simulated worker crash triggered"
		case <-ctx.Done():
			return nil, pb.TaskOutcome_TASK_OUTCOME_TIMEOUT, "slow_then_crash timed out before crashing"
		}
	}

	// Handle simulated mock / control payloads
	if lowerType == "mock" || strings.HasPrefix(payload, "mock:") {
		trimmed := strings.TrimPrefix(payload, "mock:")
		trimmed = strings.TrimSpace(trimmed)

		if strings.HasPrefix(trimmed, "fail") {
			msg := "simulated failure"
			if idx := strings.Index(trimmed, ":"); idx != -1 {
				msg = strings.TrimSpace(trimmed[idx+1:])
			}
			return nil, pb.TaskOutcome_TASK_OUTCOME_FAILURE, msg
		}

		if strings.HasPrefix(trimmed, "sleep") {
			dur := 100 * time.Millisecond
			if idx := strings.IndexAny(trimmed, ":,"); idx != -1 {
				rawDur := strings.TrimSpace(trimmed[idx+1:])
				if d, err := time.ParseDuration(rawDur); err == nil {
					dur = d
				}
			}
			select {
			case <-time.After(dur):
				return []byte(fmt.Sprintf("slept %v", dur)), pb.TaskOutcome_TASK_OUTCOME_SUCCESS, ""
			case <-ctx.Done():
				return nil, pb.TaskOutcome_TASK_OUTCOME_TIMEOUT, "task timed out during sleep"
			}
		}

		return []byte("mock success: " + payload), pb.TaskOutcome_TASK_OUTCOME_SUCCESS, ""
	}

	// Default: Run via OS shell command
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd.exe", "/c", payload)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", payload)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return out, pb.TaskOutcome_TASK_OUTCOME_TIMEOUT, "execution deadline exceeded"
		}
		return out, pb.TaskOutcome_TASK_OUTCOME_FAILURE, err.Error()
	}

	return out, pb.TaskOutcome_TASK_OUTCOME_SUCCESS, ""
}
