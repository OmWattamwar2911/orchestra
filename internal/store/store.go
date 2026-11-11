package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	pb "orchestra/api/proto/orchestra/v1"
	"orchestra/internal/dag"
	"orchestra/internal/metrics"
)

// Fencer is the interface implemented by leader election candidates to condition
// mutating etcd transactions on active leadership term.
type Fencer interface {
	Fenced(ops ...clientv3.Op) (*clientv3.TxnResponse, error)
	FencedContext(ctx context.Context, extraCmps []clientv3.Cmp, ops ...clientv3.Op) (*clientv3.TxnResponse, error)
	Term() int64
	LeaderKey() string
}

// CommitResult conveys the outcome of a task result submission.
type CommitResult struct {
	Committed bool
	Message   string
	Task      *pb.Task
	Revision  int64
}

// RecoveryResult details recovered workflows, rebuilt in-memory DAGs, and re-queued tasks.
type RecoveryResult struct {
	Workflows     []*pb.Workflow
	Graphs        map[string]*dag.Graph
	RequeuedTasks []*pb.Task
	ReadyTasks    []*pb.Task
}

// Store provides persistence and state machine enforcement for workflows and tasks in etcd.
type Store struct {
	client  *clientv3.Client
	logger  *slog.Logger
	nextSeq atomic.Int64 // Invariant protected: Atomic sequence counter guarantees strictly monotonic append-only event ordering
}

// New initializes a new Store backed by an etcd client.
func New(client *clientv3.Client, logger *slog.Logger) *Store {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Store{
		client: client,
		logger: logger.With(slog.String("component", "store")),
	}
	return s
}

// Client returns the underlying etcd client.
func (s *Store) Client() *clientv3.Client {
	return s.client
}

// InitSequence initializes the event sequence counter by inspecting the latest event in etcd.
func (s *Store) InitSequence(ctx context.Context) error {
	latest, err := s.GetLatestEventSequence(ctx)
	if err != nil {
		return err
	}
	// Invariant protected: Atomic store synchronizes initial event sequence number across leader takeovers
	s.nextSeq.Store(latest)
	return nil
}

// NextSequence allocates the next monotonically increasing event sequence number.
func (s *Store) NextSequence() int64 {
	// Invariant protected: Atomic addition ensures globally unique, strictly increasing sequence IDs
	return s.nextSeq.Add(1)
}

// SubmitWorkflow validates and persists a workflow DAG.
// Idempotency guarantee: If a workflow with the same client-provided ID already exists in etcd,
// the existing workflow is returned without creating duplicates or overwriting tasks.
func (s *Store) SubmitWorkflow(ctx context.Context, fencer Fencer, wf *pb.Workflow) (*pb.Workflow, bool, error) {
	if wf == nil || wf.GetSpec() == nil || wf.GetSpec().GetId() == "" {
		return nil, false, errors.New("invalid workflow: nil or empty workflow spec")
	}

	wfID := wf.GetSpec().GetId()

	// 1. Validate DAG topology using pure domain logic
	domainWf := dag.FromProtoWorkflowSpec(wf.GetSpec())
	if err := dag.Validate(domainWf); err != nil {
		return nil, false, fmt.Errorf("DAG validation failed: %w", err)
	}

	wfKey := WorkflowKey(wfID)

	// Check if workflow already exists in etcd
	getResp, err := s.client.Get(ctx, wfKey)
	if err != nil {
		return nil, false, fmt.Errorf("failed to check existing workflow: %w", err)
	}
	if len(getResp.Kvs) > 0 {
		var existingWf pb.Workflow
		if err := json.Unmarshal(getResp.Kvs[0].Value, &existingWf); err != nil {
			return nil, false, fmt.Errorf("failed to unmarshal existing workflow: %w", err)
		}
		s.logger.Info("idempotent SubmitWorkflow: returned existing workflow", slog.String("workflow_id", wfID))
		return &existingWf, false, nil
	}

	// 2. Prepare workflow and tasks
	now := time.Now().UnixMilli()
	if wf.Status == nil {
		wf.Status = &pb.WorkflowStatus{
			Id:         wfID,
			State:      pb.WorkflowState_WORKFLOW_STATE_RUNNING,
			CreatedAt:  now,
			TotalTasks: int32(len(wf.GetSpec().GetTasks())),
		}
	}

	wfData, err := json.Marshal(wf)
	if err != nil {
		return nil, false, fmt.Errorf("failed to marshal workflow: %w", err)
	}

	ops := make([]clientv3.Op, 0, len(wf.GetSpec().GetTasks())+2)
	ops = append(ops, clientv3.OpPut(wfKey, string(wfData)))

	// Identify root tasks (0 dependencies) which start as READY; all others start as PENDING
	for _, taskSpec := range wf.GetSpec().GetTasks() {
		taskID := taskSpec.GetId()
		initialState := pb.TaskState_TASK_STATE_PENDING
		currentAttempt := int32(0)
		if len(taskSpec.GetDependencies()) == 0 {
			initialState = pb.TaskState_TASK_STATE_READY
			currentAttempt = 1
		}

		t := &pb.Task{
			Spec: taskSpec,
			Status: &pb.TaskStatus{
				Id:             taskID,
				WorkflowId:     wfID,
				State:          initialState,
				CurrentAttempt: currentAttempt,
			},
		}

		taskData, err := json.Marshal(t)
		if err != nil {
			return nil, false, fmt.Errorf("failed to marshal task %s: %w", taskID, err)
		}
		ops = append(ops, clientv3.OpPut(TaskKey(wfID, taskID), string(taskData)))
	}

	// Append initial workflow creation event in the exact same transaction
	seq := s.NextSequence()
	event := NewWorkflowTransitionEvent(seq, now, wfID, pb.WorkflowState_WORKFLOW_STATE_UNSPECIFIED, pb.WorkflowState_WORKFLOW_STATE_RUNNING, fencer.Term(), "")
	eventData, err := json.Marshal(event)
	if err != nil {
		return nil, false, fmt.Errorf("failed to marshal workflow event: %w", err)
	}
	ops = append(ops, clientv3.OpPut(EventKey(seq), string(eventData)))

	// Invariant protected: Compare CreateRevision(wfKey) == 0 ensures atomic creation without overwriting concurrent submissions
	createCmp := clientv3.Compare(clientv3.CreateRevision(wfKey), "=", 0)

	resp, err := fencer.FencedContext(ctx, []clientv3.Cmp{createCmp}, ops...)
	if err != nil {
		return nil, false, fmt.Errorf("fenced workflow creation transaction failed: %w", err)
	}

	if !resp.Succeeded {
		// Concurrent submission won race: load and return existing workflow
		existing, _, err := s.GetWorkflow(ctx, wfID)
		if err != nil {
			return nil, false, fmt.Errorf("concurrent creation race conflict: %w", err)
		}
		return existing, false, nil
	}

	metrics.WorkflowsSubmittedTotal.Inc()

	s.logger.Info("workflow state transition",
		slog.String("workflow_id", wfID),
		slog.Int64("leader_term", fencer.Term()),
		slog.String("from_state", pb.WorkflowState_WORKFLOW_STATE_UNSPECIFIED.String()),
		slog.String("to_state", pb.WorkflowState_WORKFLOW_STATE_RUNNING.String()),
	)

	for _, taskSpec := range wf.GetSpec().GetTasks() {
		taskID := taskSpec.GetId()
		initialState := pb.TaskState_TASK_STATE_PENDING
		currentAttempt := int32(0)
		if len(taskSpec.GetDependencies()) == 0 {
			initialState = pb.TaskState_TASK_STATE_READY
			currentAttempt = 1
		}
		s.logger.Info("task state transition",
			slog.String("workflow_id", wfID),
			slog.String("task_id", taskID),
			slog.Int("attempt_id", int(currentAttempt)),
			slog.String("worker_id", ""),
			slog.Int64("leader_term", fencer.Term()),
			slog.String("from_state", pb.TaskState_TASK_STATE_UNSPECIFIED.String()),
			slog.String("to_state", initialState.String()),
		)
	}

	return wf, true, nil
}

// GetWorkflow retrieves a workflow by ID.
func (s *Store) GetWorkflow(ctx context.Context, workflowID string) (*pb.Workflow, int64, error) {
	key := WorkflowKey(workflowID)
	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get workflow %s: %w", workflowID, err)
	}
	if len(resp.Kvs) == 0 {
		return nil, 0, ErrWorkflowNotFound
	}

	var wf pb.Workflow
	if err := json.Unmarshal(resp.Kvs[0].Value, &wf); err != nil {
		return nil, 0, fmt.Errorf("failed to unmarshal workflow: %w", err)
	}
	return &wf, resp.Kvs[0].ModRevision, nil
}

// ListWorkflows retrieves all workflows from etcd.
func (s *Store) ListWorkflows(ctx context.Context) ([]*pb.Workflow, error) {
	resp, err := s.client.Get(ctx, PrefixWorkflows, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list workflows: %w", err)
	}

	workflows := make([]*pb.Workflow, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var wf pb.Workflow
		if err := json.Unmarshal(kv.Value, &wf); err != nil {
			continue
		}
		workflows = append(workflows, &wf)
	}
	return workflows, nil
}

// PutWorkflow stores or updates workflow metadata through the leader Fencer.
func (s *Store) PutWorkflow(ctx context.Context, fencer Fencer, wf *pb.Workflow) error {
	if wf == nil || wf.GetSpec() == nil || wf.GetSpec().GetId() == "" {
		return errors.New("invalid workflow: nil or empty ID")
	}

	data, err := json.Marshal(wf)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow: %w", err)
	}

	key := WorkflowKey(wf.GetSpec().GetId())
	op := clientv3.OpPut(key, string(data))

	if fencer != nil {
		// Invariant protected: Fencing check prevents zombie ex-leaders from modifying workflow state
		resp, err := fencer.FencedContext(ctx, nil, op)
		if err != nil {
			return err
		}
		if !resp.Succeeded {
			return ErrFencingTokenMismatch
		}
		return nil
	}

	_, err = s.client.Put(ctx, key, string(data))
	return err
}

// TransitionWorkflowState validates and applies a workflow state machine transition conditioned on ModRevision.
func (s *Store) TransitionWorkflowState(ctx context.Context, fencer Fencer, workflowID string, expectedModRev int64, targetState pb.WorkflowState) error {
	wf, currentModRev, err := s.GetWorkflow(ctx, workflowID)
	if err != nil {
		return err
	}

	if expectedModRev > 0 && currentModRev != expectedModRev {
		return ErrCASConflict
	}

	if err := ValidateWorkflowTransition(wf.Status.State, targetState); err != nil {
		return err
	}

	fromState := wf.Status.State
	wf.Status.State = targetState
	now := time.Now().UnixMilli()
	if IsTerminalWorkflowState(targetState) {
		wf.Status.CompletedAt = now
	}

	data, err := json.Marshal(wf)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow: %w", err)
	}

	key := WorkflowKey(workflowID)
	seq := s.NextSequence()
	event := NewWorkflowTransitionEvent(seq, now, workflowID, fromState, targetState, fencer.Term(), wf.Status.ErrorMessage)
	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Invariant protected: Fencing + ModRevision CAS guarantees atomic state transition and event emission
	casCmp := clientv3.Compare(clientv3.ModRevision(key), "=", currentModRev)
	putWf := clientv3.OpPut(key, string(data))
	putEvent := clientv3.OpPut(EventKey(seq), string(eventData))

	resp, err := fencer.FencedContext(ctx, []clientv3.Cmp{casCmp}, putWf, putEvent)
	if err != nil {
		return err
	}
	if !resp.Succeeded {
		return ErrCASConflict
	}

	// Invariant protected: Structured transition logging and metric counters accurately reflect workflow state changes
	s.logger.Info("workflow state transition",
		slog.String("workflow_id", workflowID),
		slog.Int64("leader_term", fencer.Term()),
		slog.String("from_state", fromState.String()),
		slog.String("to_state", targetState.String()),
	)

	if targetState == pb.WorkflowState_WORKFLOW_STATE_SUCCEEDED {
		metrics.WorkflowsSucceededTotal.Inc()
		if wf.Status.CreatedAt > 0 {
			durationSec := float64(now-wf.Status.CreatedAt) / 1000.0
			if durationSec >= 0 {
				metrics.WorkflowDurationSeconds.Observe(durationSec)
			}
		}
	} else if targetState == pb.WorkflowState_WORKFLOW_STATE_FAILED || targetState == pb.WorkflowState_WORKFLOW_STATE_CANCELLED {
		metrics.WorkflowsFailedTotal.Inc()
		if wf.Status.CreatedAt > 0 {
			durationSec := float64(now-wf.Status.CreatedAt) / 1000.0
			if durationSec >= 0 {
				metrics.WorkflowDurationSeconds.Observe(durationSec)
			}
		}
	}

	return nil
}

// GetTask retrieves a single task along with its etcd ModRevision.
func (s *Store) GetTask(ctx context.Context, workflowID, taskID string) (*pb.Task, int64, error) {
	key := TaskKey(workflowID, taskID)
	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get task %s: %w", key, err)
	}
	if len(resp.Kvs) == 0 {
		return nil, 0, ErrTaskNotFound
	}

	var task pb.Task
	if err := json.Unmarshal(resp.Kvs[0].Value, &task); err != nil {
		return nil, 0, fmt.Errorf("failed to unmarshal task: %w", err)
	}

	modRev := resp.Kvs[0].ModRevision
	if task.Status != nil {
		task.Status.ModRevision = modRev
	}
	return &task, modRev, nil
}

// ListTasksForWorkflow retrieves all tasks associated with a workflow.
func (s *Store) ListTasksForWorkflow(ctx context.Context, workflowID string) ([]*pb.Task, error) {
	prefix := WorkflowTasksPrefix(workflowID)
	resp, err := s.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks for workflow %s: %w", workflowID, err)
	}

	tasks := make([]*pb.Task, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var t pb.Task
		if err := json.Unmarshal(kv.Value, &t); err != nil {
			continue
		}
		if t.Status != nil {
			t.Status.ModRevision = kv.ModRevision
		}
		tasks = append(tasks, &t)
	}
	return tasks, nil
}

// PutTask initializes or writes task state to etcd through the leader Fencer.
func (s *Store) PutTask(ctx context.Context, fencer Fencer, task *pb.Task) (int64, error) {
	if task == nil || task.GetSpec() == nil || task.GetSpec().GetId() == "" {
		return 0, errors.New("invalid task: nil or empty ID")
	}

	workflowID := task.GetStatus().GetWorkflowId()
	taskID := task.GetSpec().GetId()
	key := TaskKey(workflowID, taskID)

	data, err := json.Marshal(task)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal task: %w", err)
	}

	op := clientv3.OpPut(key, string(data))

	if fencer != nil {
		// Invariant protected: Leader fencing ensures only the current elected leader can modify task state
		resp, err := fencer.FencedContext(ctx, nil, op)
		if err != nil {
			return 0, fmt.Errorf("fenced put task failed: %w", err)
		}
		if !resp.Succeeded {
			return 0, ErrFencingTokenMismatch
		}
		return resp.Header.Revision, nil
	}

	resp, err := s.client.Put(ctx, key, string(data))
	if err != nil {
		return 0, err
	}
	return resp.Header.Revision, nil
}

// TransitionTaskState validates and applies a task state transition conditioned on ModRevision and active leader fencing.
// It commits the updated task JSON and an append-only event record in the exact same atomic transaction.
func (s *Store) TransitionTaskState(ctx context.Context, fencer Fencer, workflowID, taskID string, expectedModRev int64, targetState pb.TaskState, updateFn func(t *pb.Task)) (*pb.Task, int64, error) {
	task, currentModRev, err := s.GetTask(ctx, workflowID, taskID)
	if err != nil {
		return nil, 0, err
	}

	if expectedModRev > 0 && currentModRev != expectedModRev {
		metrics.StaleResultsRejectedTotal.Inc()
		return nil, 0, ErrCASConflict
	}

	fromState := task.Status.State
	if err := ValidateTaskTransition(fromState, targetState); err != nil {
		metrics.StaleResultsRejectedTotal.Inc()
		return nil, 0, err
	}

	task.Status.State = targetState
	now := time.Now().UnixMilli()
	if IsTerminalTaskState(targetState) {
		task.Status.CompletedAt = now
	}
	if updateFn != nil {
		updateFn(task)
	}

	data, err := json.Marshal(task)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal updated task: %w", err)
	}

	key := TaskKey(workflowID, taskID)
	seq := s.NextSequence()
	workerID := task.Status.AssignedWorker
	errMsg := task.Status.ErrorMessage
	attemptID := task.Status.CurrentAttempt

	event := NewTaskTransitionEvent(seq, now, workflowID, taskID, attemptID, fromState, targetState, fencer.Term(), workerID, errMsg)
	eventData, err := json.Marshal(event)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal event: %w", err)
	}

	// Invariant protected: Fencing + Compare-And-Swap on ModRevision guarantees exactly-one committed transition per task
	casCmp := clientv3.Compare(clientv3.ModRevision(key), "=", currentModRev)
	eventCmp := clientv3.Compare(clientv3.CreateRevision(EventKey(seq)), "=", 0)
	putTask := clientv3.OpPut(key, string(data))
	putEvent := clientv3.OpPut(EventKey(seq), string(eventData))

	resp, err := fencer.FencedContext(ctx, []clientv3.Cmp{casCmp, eventCmp}, putTask, putEvent)
	if err != nil {
		return nil, 0, fmt.Errorf("fenced state transition transaction failed: %w", err)
	}
	if !resp.Succeeded {
		metrics.StaleResultsRejectedTotal.Inc()
		return nil, 0, ErrCASConflict
	}

	task.Status.ModRevision = resp.Header.Revision

	// Invariant protected: Structured logging of state transitions with all required fields
	s.logger.Info("task state transition",
		slog.String("workflow_id", workflowID),
		slog.String("task_id", taskID),
		slog.Int("attempt_id", int(attemptID)),
		slog.String("worker_id", workerID),
		slog.Int64("leader_term", fencer.Term()),
		slog.String("from_state", fromState.String()),
		slog.String("to_state", targetState.String()),
	)

	switch targetState {
	case pb.TaskState_TASK_STATE_COMPLETED:
		metrics.TasksCompletedTotal.Inc()
		if task.Status.StartedAt > 0 {
			latencySec := float64(now-task.Status.StartedAt) / 1000.0
			if latencySec >= 0 {
				metrics.TaskLatencySeconds.Observe(latencySec)
			}
		}
	case pb.TaskState_TASK_STATE_FAILED:
		metrics.TasksFailedTotal.Inc()
	case pb.TaskState_TASK_STATE_DEAD_LETTER:
		metrics.TasksDeadLetterTotal.Inc()
	}

	return task, resp.Header.Revision, nil
}

// CommitTaskResult commits the execution outcome reported by a worker.
// Delivery semantics: At-least-once execution with exactly-one committed result per task.
// Late, duplicate, or mismatched attempt reports are strictly rejected and counted in metrics.
func (s *Store) CommitTaskResult(ctx context.Context, fencer Fencer, req *pb.ReportResultRequest) (*CommitResult, error) {
	if req == nil {
		return &CommitResult{Committed: false, Message: "nil request"}, errors.New("nil report request")
	}

	wfID := req.GetWorkflowId()
	taskID := req.GetTaskId()

	t, currentModRev, err := s.GetTask(ctx, wfID, taskID)
	if err != nil {
		return &CommitResult{Committed: false, Message: fmt.Sprintf("task lookup failed: %v", err)}, err
	}

	// 1. Check if task is already in a terminal state
	if IsTerminalTaskState(t.Status.State) {
		metrics.StaleResultsRejectedTotal.Inc()
		s.logger.Warn("duplicate report rejected: task already in terminal state",
			slog.String("workflow_id", wfID),
			slog.String("task_id", taskID),
			slog.String("current_state", t.Status.State.String()),
		)
		return &CommitResult{Committed: false, Message: "task is already in terminal state"}, nil
	}

	// 2. Enforce attempt ID match: only current attempt results are accepted
	if req.GetAttemptId() != t.Status.CurrentAttempt {
		metrics.StaleResultsRejectedTotal.Inc()
		s.logger.Warn("stale or duplicate report rejected: attempt mismatch",
			slog.String("workflow_id", wfID),
			slog.String("task_id", taskID),
			slog.Int("report_attempt", int(req.GetAttemptId())),
			slog.Int("task_current_attempt", int(t.Status.CurrentAttempt)),
		)
		return &CommitResult{
			Committed: false,
			Message:   fmt.Sprintf("stale report: attempt %d != current attempt %d", req.GetAttemptId(), t.Status.CurrentAttempt),
		}, nil
	}

	// 3. Determine target state based on outcome and retry policy
	var targetState pb.TaskState
	switch req.GetOutcome() {
	case pb.TaskOutcome_TASK_OUTCOME_SUCCESS:
		targetState = pb.TaskState_TASK_STATE_COMPLETED
	case pb.TaskOutcome_TASK_OUTCOME_FAILURE, pb.TaskOutcome_TASK_OUTCOME_TIMEOUT:
		maxRetries := t.Spec.GetMaxRetries()
		if req.GetAttemptId() <= maxRetries {
			targetState = pb.TaskState_TASK_STATE_READY // Auto-retry
		} else {
			targetState = pb.TaskState_TASK_STATE_DEAD_LETTER // Max retries exhausted
		}
	default:
		targetState = pb.TaskState_TASK_STATE_FAILED
	}

	// 4. Execute atomic state transition via Fencer with ModRevision compare-and-swap
	updatedTask, newRev, err := s.TransitionTaskState(ctx, fencer, wfID, taskID, currentModRev, targetState, func(task *pb.Task) {
		task.Status.ErrorMessage = req.GetErrorMessage()
		task.Status.ResultData = req.GetOutput()
		task.Status.AssignedWorker = req.GetWorkerId()
		if targetState == pb.TaskState_TASK_STATE_READY {
			// Increment attempt counter for next dispatch
			task.Status.CurrentAttempt++
			task.Status.AssignedWorker = ""
		}
	})

	if err != nil {
		if errors.Is(err, ErrCASConflict) {
			s.logger.Warn("CAS conflict on task result commit", slog.String("task_id", taskID))
			return &CommitResult{Committed: false, Message: "commit rejected: CAS conflict"}, nil
		}
		return &CommitResult{Committed: false, Message: err.Error()}, err
	}

	return &CommitResult{
		Committed: true,
		Message:   "task result committed",
		Task:      updatedTask,
		Revision:  newRev,
	}, nil
}

// RegisterWorkerLease associates a worker ID and its capacity metadata with an etcd lease.
func (s *Store) RegisterWorkerLease(ctx context.Context, workerID string, metadata []byte, leaseID clientv3.LeaseID) error {
	key := WorkerKey(workerID)
	// Invariant protected: Attaching key to lease ensures automatic expiration when worker fails to heartbeat
	_, err := s.client.Put(ctx, key, string(metadata), clientv3.WithLease(leaseID))
	return err
}

// ListActiveWorkers returns all workers with currently valid, active etcd leases.
func (s *Store) ListActiveWorkers(ctx context.Context) (map[string][]byte, error) {
	resp, err := s.client.Get(ctx, PrefixWorkers, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list active workers: %w", err)
	}

	workers := make(map[string][]byte, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		keyStr := string(kv.Key)
		parts := strings.Split(keyStr, "/")
		workerID := parts[len(parts)-1]
		workers[workerID] = kv.Value
	}
	return workers, nil
}

// Recover is invoked upon leader election takeover.
// It loads all non-terminal workflows, rebuilds in-memory DAG graphs, and re-queues any tasks
// in ASSIGNED or RUNNING state on workers whose lease no longer exists.
func (s *Store) Recover(ctx context.Context, fencer Fencer) (*RecoveryResult, error) {
	s.logger.Info("running leader recovery: rebuilding state and re-queuing orphaned tasks...")

	// 1. Rebuild sequence counter from etcd
	if err := s.InitSequence(ctx); err != nil {
		s.logger.Warn("failed to initialize sequence counter from etcd", slog.Any("error", err))
	}

	// 2. Discover active workers
	activeWorkers, err := s.ListActiveWorkers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect active workers during recovery: %w", err)
	}

	// 3. Load all workflows
	allWorkflows, err := s.ListWorkflows(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflows during recovery: %w", err)
	}

	result := &RecoveryResult{
		Workflows:     make([]*pb.Workflow, 0),
		Graphs:        make(map[string]*dag.Graph),
		RequeuedTasks: make([]*pb.Task, 0),
	}

	for _, wf := range allWorkflows {
		wfID := wf.GetSpec().GetId()
		if wf.GetStatus().GetState() != pb.WorkflowState_WORKFLOW_STATE_RUNNING {
			continue // Skip already terminated workflows
		}

		result.Workflows = append(result.Workflows, wf)

		// Rebuild in-memory DAG
		graph, err := dag.ValidateAndBuild(wf.GetSpec())
		if err != nil {
			s.logger.Error("corrupt workflow DAG encountered during recovery", slog.String("workflow_id", wfID), slog.Any("error", err))
			continue
		}
		result.Graphs[wfID] = graph

		// Inspect tasks for orphaned in-flight states
		tasks, err := s.ListTasksForWorkflow(ctx, wfID)
		if err != nil {
			s.logger.Error("failed to load tasks for workflow", slog.String("workflow_id", wfID), slog.Any("error", err))
			continue
		}

		for _, t := range tasks {
			state := t.GetStatus().GetState()
			taskID := t.GetSpec().GetId()

			// Tasks already in READY state were waiting to be scheduled; re-queue them for dispatch
			if state == pb.TaskState_TASK_STATE_READY {
				s.logger.Info("recovered task in READY state, queuing for dispatch",
					slog.String("workflow_id", wfID),
					slog.String("task_id", taskID),
				)
				result.ReadyTasks = append(result.ReadyTasks, t)
				continue
			}

			// Check for in-flight tasks assigned to dead workers or orphaned across leader transitions
			if state == pb.TaskState_TASK_STATE_ASSIGNED || state == pb.TaskState_TASK_STATE_RUNNING {
				workerID := t.GetStatus().GetAssignedWorker()
				_, isWorkerAlive := activeWorkers[workerID]

				// On leader takeover, in-flight tasks assigned by the prior leader must be re-queued:
				// the prior leader's dispatch streams were severed and its timeout timers lost.
				s.logger.Warn("re-queuing in-flight task on leader takeover",
					slog.String("workflow_id", wfID),
					slog.String("task_id", taskID),
					slog.String("worker_id", workerID),
					slog.Bool("worker_alive", isWorkerAlive),
					slog.String("prev_state", state.String()),
				)

				var nextState pb.TaskState
				if t.Status.CurrentAttempt <= t.Spec.GetMaxRetries() {
					nextState = pb.TaskState_TASK_STATE_READY
				} else {
					nextState = pb.TaskState_TASK_STATE_DEAD_LETTER
				}

				requeued, _, err := s.TransitionTaskState(ctx, fencer, wfID, taskID, t.Status.ModRevision, nextState, func(task *pb.Task) {
					task.Status.CurrentAttempt++
					task.Status.AssignedWorker = ""
					task.Status.StartedAt = 0
				})

				if err != nil {
					s.logger.Error("failed to re-queue orphaned task", slog.String("task_id", taskID), slog.Any("error", err))
				} else if nextState == pb.TaskState_TASK_STATE_READY {
					result.RequeuedTasks = append(result.RequeuedTasks, requeued)
				}
			}
		}
	}

	s.logger.Info("leader recovery completed successfully",
		slog.Int("active_workflows", len(result.Workflows)),
		slog.Int("requeued_tasks", len(result.RequeuedTasks)),
	)
	return result, nil
}

// GetLatestEventSequence inspects the latest event sequence number written to etcd.
func (s *Store) GetLatestEventSequence(ctx context.Context) (int64, error) {
	opts := []clientv3.OpOption{
		clientv3.WithPrefix(),
		clientv3.WithSort(clientv3.SortByKey, clientv3.SortDescend),
		clientv3.WithLimit(1),
	}
	resp, err := s.client.Get(ctx, PrefixEvents, opts...)
	if err != nil {
		return 0, fmt.Errorf("failed to query latest event sequence: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return 0, nil
	}

	keyStr := string(resp.Kvs[0].Key)
	parts := strings.Split(keyStr, "/")
	seqStr := parts[len(parts)-1]
	seq, err := strconv.ParseInt(seqStr, 10, 64)
	if err != nil {
		return 0, nil
	}
	return seq, nil
}

// ListEvents retrieves append-only state transition events starting from a sequence number.
func (s *Store) ListEvents(ctx context.Context, fromSeq int64, limit int64) ([]StateTransitionEvent, error) {
	startKey := EventKey(fromSeq)
	endKey := clientv3.GetPrefixRangeEnd(PrefixEvents)

	opts := []clientv3.OpOption{
		clientv3.WithRange(endKey),
		clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend),
	}
	if limit > 0 {
		opts = append(opts, clientv3.WithLimit(limit))
	}

	resp, err := s.client.Get(ctx, startKey, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	events := make([]StateTransitionEvent, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var e StateTransitionEvent
		if err := json.Unmarshal(kv.Value, &e); err != nil {
			continue
		}
		events = append(events, e)
	}
	return events, nil
}
