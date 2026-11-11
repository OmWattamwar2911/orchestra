package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	pb "orchestra/api/proto/orchestra/v1"
	"orchestra/internal/dag"
	"orchestra/internal/election"
	"orchestra/internal/metrics"
	"orchestra/internal/queue"
	"orchestra/internal/store"
)

var (
	ErrNotElectedLeader   = errors.New("operation rejected: node is not the active cluster leader")
	ErrWorkflowNotFound   = errors.New("workflow not found")
	ErrTaskNotFound       = errors.New("task not found")
	ErrWorkflowTerminated = errors.New("workflow already reached terminal state")
)

type workerState struct {
	id           string
	capacity     int
	activeTasks  int
	channel      chan *pb.TaskAssignment
	lastSeen     time.Time
	streamActive bool
}

// Engine coordinates DAG resolution, worker dispatch, failure policies, and task timeouts.
type Engine struct {
	pb.UnimplementedOrchestratorServiceServer
	pb.UnimplementedWorkerServiceServer

	nodeID     string
	store      *store.Store
	candidate  *election.Candidate
	etcdClient *clientv3.Client
	logger     *slog.Logger

	baseBackoff time.Duration
	maxBackoff  time.Duration

	// queue holds ready tasks waiting for worker assignment
	queue queue.TaskQueue

	// capacitySignal unblocks the dispatcher when worker capacity becomes available
	capacitySignal chan struct{}

	// graphsMu protects the in-memory cache of active DAG topologies
	// Invariant protected: Guards concurrent graph registration, lookups, and state reconciliation
	graphsMu sync.RWMutex
	graphs   map[string]*dag.Graph

	// workersMu protects active worker dispatch channels and load balancing state
	// Invariant protected: Synchronizes worker registration, capacity tracking, and least-loaded dispatch
	workersMu sync.RWMutex
	workers   map[string]*workerState

	// watchersMu protects workflow subscriber channels
	// Invariant protected: Synchronizes active server-streaming watcher subscriptions
	watchersMu sync.RWMutex
	watchers   map[string][]chan *pb.WatchWorkflowResponse

	// timersMu protects per-task execution timeout timers
	// Invariant protected: Protects concurrent registration and cancellation of task execution timeout timers
	timersMu   sync.Mutex
	taskTimers map[string]*time.Timer

	isReconciling atomic.Bool
}

// Config specifies scheduler engine options.
type Config struct {
	NodeID      string
	Store       *store.Store
	Candidate   *election.Candidate
	EtcdClient  *clientv3.Client
	Queue       queue.TaskQueue
	Logger      *slog.Logger
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
}

// NewEngine creates a new scheduler engine.
func NewEngine(cfg Config) *Engine {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	q := cfg.Queue
	if q == nil {
		q = queue.NewPriorityQueue()
	}

	baseBackoff := cfg.BaseBackoff
	if baseBackoff <= 0 {
		baseBackoff = 50 * time.Millisecond
	}

	maxBackoff := cfg.MaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = 10 * time.Second
	}

	return &Engine{
		nodeID:         cfg.NodeID,
		store:          cfg.Store,
		candidate:      cfg.Candidate,
		etcdClient:     cfg.EtcdClient,
		logger:         logger.With(slog.String("component", "scheduler"), slog.String("node_id", cfg.NodeID)),
		queue:          q,
		capacitySignal: make(chan struct{}, 1),
		graphs:         make(map[string]*dag.Graph),
		workers:        make(map[string]*workerState),
		watchers:       make(map[string][]chan *pb.WatchWorkflowResponse),
		taskTimers:     make(map[string]*time.Timer),
		baseBackoff:    baseBackoff,
		maxBackoff:     maxBackoff,
	}
}

// CheckLeader verifies this node holds active leadership.
func (e *Engine) CheckLeader() error {
	// Invariant protected: Fencing check verifies caller holds active leadership lease revision
	if e.candidate == nil || !e.candidate.IsLeader() {
		return ErrNotElectedLeader
	}
	if e.candidate.Term() <= 0 {
		return ErrNotElectedLeader
	}
	return nil
}

// QueueDepth returns the current number of tasks waiting in the dispatch queue (backpressure metric).
func (e *Engine) QueueDepth() int {
	return e.queue.Depth()
}

// OnElected is invoked when this node becomes the active cluster leader.
// It reconstructs in-flight workflow graphs from etcd, re-queues orphaned tasks, and launches background workers.
func (e *Engine) OnElected(leaderCtx context.Context, term int64) error {
	e.logger.Info("leadership assumed, running state recovery and launching scheduler loops...",
		slog.Int64("term", term),
	)

	// Invariant protected: Atomic flag ensures only one set of scheduler loops runs per leadership term
	e.isReconciling.Store(true)

	// Recover state and re-queue tasks assigned to dead workers
	rec, err := e.store.Recover(leaderCtx, e.candidate)
	if err != nil {
		e.logger.Error("failed to recover state upon leader takeover", slog.Any("error", err))
		return err
	}

	// Cache recovered graphs
	e.graphsMu.Lock()
	for wfID, g := range rec.Graphs {
		e.graphs[wfID] = g
	}
	e.graphsMu.Unlock()

	// Enqueue all recovered tasks that are in READY state
	for _, t := range rec.RequeuedTasks {
		e.queue.Enqueue(&pb.TaskAssignment{
			WorkflowId:         t.Status.WorkflowId,
			Task:               t.Spec,
			Attempt:            t.Status.CurrentAttempt,
			LeaderFencingToken: term,
		}, 1)
	}
	for _, t := range rec.ReadyTasks {
		e.queue.Enqueue(&pb.TaskAssignment{
			WorkflowId:         t.Status.WorkflowId,
			Task:               t.Spec,
			Attempt:            t.Status.CurrentAttempt,
			LeaderFencingToken: term,
		}, 1)
	}
	e.notifyCapacity()

	// Launch background loops
	go e.runDispatcherLoop(leaderCtx)
	go e.runReconciliationLoop(leaderCtx)

	if e.etcdClient != nil {
		go e.watchWorkers(leaderCtx)
	}

	return nil
}

// OnRevoked is invoked when this node loses leadership.
func (e *Engine) OnRevoked() {
	e.logger.Warn("leadership lost, halting scheduling and resetting worker assignments")
	// Invariant protected: Atomic store halts reconciliation loop immediately upon leadership revocation
	e.isReconciling.Store(false)

	// Invalidate task timers
	e.timersMu.Lock()
	for id, timer := range e.taskTimers {
		timer.Stop()
		delete(e.taskTimers, id)
	}
	e.timersMu.Unlock()

	// Invalidate worker references
	// Invariant protected: Workers lock synchronizes cleanup of registered workers during step-down
	e.workersMu.Lock()
	for id, w := range e.workers {
		w.streamActive = false
		w.activeTasks = 0
		delete(e.workers, id)
	}
	metrics.WorkerCount.Set(0)
	metrics.QueueDepth.Set(0)
	e.workersMu.Unlock()
}

// watchWorkers watches /orchestra/workers in etcd. When a worker lease expires or is deleted,
// its in-flight non-terminal tasks are immediately re-queued.
func (e *Engine) watchWorkers(ctx context.Context) {
	rch := e.etcdClient.Watch(ctx, store.PrefixWorkers, clientv3.WithPrefix())
	for {
		select {
		case <-ctx.Done():
			return
		case resp, ok := <-rch:
			if !ok {
				return
			}
			for _, ev := range resp.Events {
				if ev.Type == clientv3.EventTypeDelete {
					keyStr := string(ev.Kv.Key)
					parts := strings.Split(keyStr, "/")
					workerID := parts[len(parts)-1]
					e.logger.Warn("detected worker lease loss via etcd watch", slog.String("worker_id", workerID))
					e.handleWorkerLoss(ctx, workerID)
				}
			}
		}
	}
}

// handleWorkerLoss immediately re-queues all non-terminal tasks assigned to a lost worker.
func (e *Engine) handleWorkerLoss(ctx context.Context, workerID string) {
	// Invariant protected: Workers lock synchronizes cleanup of lost worker metadata
	e.workersMu.Lock()
	if w, exists := e.workers[workerID]; exists {
		w.streamActive = false
		w.activeTasks = 0
		delete(e.workers, workerID)
	}
	metrics.WorkerCount.Set(float64(len(e.workers)))
	e.workersMu.Unlock()

	// Re-queue in-flight tasks for all running workflows
	e.graphsMu.RLock()
	activeWorkflows := make([]string, 0, len(e.graphs))
	for wfID := range e.graphs {
		activeWorkflows = append(activeWorkflows, wfID)
	}
	e.graphsMu.RUnlock()

	for _, wfID := range activeWorkflows {
		tasks, err := e.store.ListTasksForWorkflow(ctx, wfID)
		if err != nil {
			continue
		}

		for _, t := range tasks {
			if t.Status.AssignedWorker != workerID {
				continue
			}

			state := t.Status.State
			if state == pb.TaskState_TASK_STATE_ASSIGNED || state == pb.TaskState_TASK_STATE_RUNNING {
				e.logger.Warn("immediately re-queuing task due to worker crash",
					slog.String("workflow_id", wfID),
					slog.String("task_id", t.Spec.Id),
					slog.String("worker_id", workerID),
				)

				e.stopTaskTimer(wfID, t.Spec.Id)

				var nextState pb.TaskState
				if t.Status.CurrentAttempt <= t.Spec.GetMaxRetries() {
					nextState = pb.TaskState_TASK_STATE_READY
				} else {
					nextState = pb.TaskState_TASK_STATE_DEAD_LETTER
				}

				requeued, _, err := e.store.TransitionTaskState(ctx, e.candidate, wfID, t.Spec.Id, t.Status.ModRevision, nextState, func(task *pb.Task) {
					task.Status.CurrentAttempt++
					task.Status.AssignedWorker = ""
					task.Status.StartedAt = 0
				})

				if err == nil {
					e.broadcastTaskUpdate(wfID, requeued.Status)
					if nextState == pb.TaskState_TASK_STATE_READY {
						e.queue.Enqueue(&pb.TaskAssignment{
							WorkflowId:         wfID,
							Task:               requeued.Spec,
							Attempt:            requeued.Status.CurrentAttempt,
							LeaderFencingToken: e.candidate.Term(),
						}, 1)
						e.notifyCapacity()
					} else {
						// Dead letter: trigger resolution
						e.reconcileWorkflow(ctx, wfID)
					}
				}
			}
		}
	}
}

// notifyCapacity signals the dispatcher loop that worker capacity is available.
func (e *Engine) notifyCapacity() {
	select {
	case e.capacitySignal <- struct{}{}:
	default:
	}
}

// hasAvailableCapacity checks whether any live connected worker has capacity to accept a task.
func (e *Engine) hasAvailableCapacity() bool {
	e.workersMu.Lock()
	defer e.workersMu.Unlock()
	for _, w := range e.workers {
		if w.streamActive && w.activeTasks < w.capacity {
			return true
		}
	}
	return false
}

// computeBackoff calculates exponential backoff with jitter up to maxRetries.
func (e *Engine) computeBackoff(attempt int32) time.Duration {
	base := e.baseBackoff
	maxB := e.maxBackoff

	shift := attempt - 1
	if shift < 0 {
		shift = 0
	}
	if shift > 10 {
		shift = 10
	}

	backoff := base * time.Duration(1<<shift)
	if backoff > maxB {
		backoff = maxB
	}

	// Jitter: random 0-25%
	jitter := time.Duration(rand.Int63n(int64(backoff/4 + 1)))
	return backoff + jitter
}

// startTaskTimer sets an execution timeout timer for an assigned/running task.
func (e *Engine) startTaskTimer(wfID, taskID string, attempt int32, timeout time.Duration) {
	key := fmt.Sprintf("%s/%s", wfID, taskID)

	e.timersMu.Lock()
	defer e.timersMu.Unlock()

	if prev, exists := e.taskTimers[key]; exists {
		prev.Stop()
	}

	// Invariant protected: Task timeout timer marks attempt failed when execution exceeds specified timeout duration
	e.taskTimers[key] = time.AfterFunc(timeout, func() {
		e.handleTaskTimeout(wfID, taskID, attempt)
	})
}

// stopTaskTimer cancels any running timeout timer for a task.
func (e *Engine) stopTaskTimer(wfID, taskID string) {
	key := fmt.Sprintf("%s/%s", wfID, taskID)

	e.timersMu.Lock()
	defer e.timersMu.Unlock()

	if timer, exists := e.taskTimers[key]; exists {
		timer.Stop()
		delete(e.taskTimers, key)
	}
}

// handleTaskTimeout handles execution expiration by failing the attempt and retrying.
func (e *Engine) handleTaskTimeout(wfID, taskID string, attempt int32) {
	e.timersMu.Lock()
	delete(e.taskTimers, fmt.Sprintf("%s/%s", wfID, taskID))
	e.timersMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := e.CheckLeader(); err != nil {
		return
	}

	e.logger.Warn("task execution timed out, reporting failure for retry",
		slog.String("workflow_id", wfID),
		slog.String("task_id", taskID),
		slog.Int("attempt", int(attempt)),
	)

	_, _ = e.ReportResult(ctx, &pb.ReportResultRequest{
		WorkflowId:   wfID,
		TaskId:       taskID,
		AttemptId:    attempt,
		Outcome:      pb.TaskOutcome_TASK_OUTCOME_TIMEOUT,
		ErrorMessage: "task timed out",
	})
}

// runDispatcherLoop continuously matches queued tasks to the least-loaded live workers within advertised capacity.
// Backpressure: If no worker has capacity, tasks wait in the queue.
func (e *Engine) runDispatcherLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if !e.isReconciling.Load() {
			return
		}

		if !e.hasAvailableCapacity() {
			select {
			case <-ctx.Done():
				return
			case <-e.capacitySignal:
			case <-time.After(50 * time.Millisecond):
			}
			continue
		}

		assignment := e.queue.Dequeue()
		if assignment == nil {
			// Queue is empty, wait for items or timeout
			select {
			case <-ctx.Done():
				return
			case <-e.capacitySignal:
			case <-time.After(50 * time.Millisecond):
			}
			continue
		}

		wfID := assignment.GetWorkflowId()
		taskID := assignment.GetTask().GetId()

		// Verify workflow has not been cancelled
		e.graphsMu.RLock()
		_, active := e.graphs[wfID]
		e.graphsMu.RUnlock()
		if !active {
			continue // Workflow already terminated or cancelled
		}

		// Find least-loaded live worker within its advertised capacity
		e.workersMu.Lock()
		var bestWorkerID string
		minLoad := math.MaxInt32

		for id, w := range e.workers {
			if w.streamActive && w.activeTasks < w.capacity {
				if w.activeTasks < minLoad || (w.activeTasks == minLoad && (bestWorkerID == "" || id < bestWorkerID)) {
					minLoad = w.activeTasks
					bestWorkerID = id
				}
			}
		}

		var targetCh chan *pb.TaskAssignment
		if bestWorkerID != "" {
			w := e.workers[bestWorkerID]
			w.activeTasks++
			targetCh = w.channel
		}
		e.workersMu.Unlock()

		if targetCh == nil {
			// Backpressure: no live worker currently has available capacity!
			// Put task back into queue and wait for capacity signal
			e.queue.Enqueue(assignment, 1)
			select {
			case <-ctx.Done():
				return
			case <-e.capacitySignal:
			case <-time.After(50 * time.Millisecond):
			}
			continue
		}

		// Update task state in etcd to ASSIGNED through leader fencer
		t, modRev, err := e.store.GetTask(ctx, wfID, taskID)
		if err != nil {
			// Transient etcd read error during cluster churn: revert worker load and re-queue
			e.workersMu.Lock()
			if w, ok := e.workers[bestWorkerID]; ok && w.activeTasks > 0 {
				w.activeTasks--
			}
			e.workersMu.Unlock()
			e.queue.Enqueue(assignment, 1)
			continue
		}
		if t.Status.State != pb.TaskState_TASK_STATE_READY {
			// Task was already cancelled or transitioned; revert worker active count
			e.workersMu.Lock()
			if w, ok := e.workers[bestWorkerID]; ok && w.activeTasks > 0 {
				w.activeTasks--
			}
			e.workersMu.Unlock()
			continue
		}

		now := time.Now().UnixMilli()
		assignedTask, _, err := e.store.TransitionTaskState(ctx, e.candidate, wfID, taskID, modRev, pb.TaskState_TASK_STATE_ASSIGNED, func(task *pb.Task) {
			task.Status.AssignedWorker = bestWorkerID
			task.Status.StartedAt = now
		})

		if err != nil {
			// CAS or fencing failure: revert worker load and re-queue
			e.workersMu.Lock()
			if w, ok := e.workers[bestWorkerID]; ok && w.activeTasks > 0 {
				w.activeTasks--
			}
			e.workersMu.Unlock()
			e.queue.Enqueue(assignment, 1)
			continue
		}

		e.broadcastTaskUpdate(wfID, assignedTask.Status)

		// Start task timeout timer if configured
		if assignment.GetTask().GetTimeoutSeconds() > 0 {
			timeout := time.Duration(assignment.GetTask().GetTimeoutSeconds()) * time.Second
			e.startTaskTimer(wfID, taskID, assignment.GetAttempt(), timeout)
		}

		// Invariant protected: Workers lock synchronizes pushing task assignment to active worker stream channel
		e.workersMu.Lock()
		w, ok := e.workers[bestWorkerID]
		if ok && w.streamActive && w.channel != nil {
			select {
			case w.channel <- assignment:
			default:
				// If channel buffer is full, revert in etcd to READY and re-queue
				if w.activeTasks > 0 {
					w.activeTasks--
				}
				_, _, _ = e.store.TransitionTaskState(ctx, e.candidate, wfID, taskID, assignedTask.Status.ModRevision, pb.TaskState_TASK_STATE_READY, func(task *pb.Task) {
					task.Status.AssignedWorker = ""
					task.Status.StartedAt = 0
				})
				e.queue.Enqueue(assignment, 1)
			}
		} else {
			// Worker disconnected before send completed: revert in etcd to READY and re-queue
			_, _, _ = e.store.TransitionTaskState(ctx, e.candidate, wfID, taskID, assignedTask.Status.ModRevision, pb.TaskState_TASK_STATE_READY, func(task *pb.Task) {
				task.Status.AssignedWorker = ""
				task.Status.StartedAt = 0
			})
			e.queue.Enqueue(assignment, 1)
		}
		e.workersMu.Unlock()
	}
}

// runReconciliationLoop periodically evaluates active workflows.
func (e *Engine) runReconciliationLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !e.isReconciling.Load() {
				return
			}
			e.reconcileAll(ctx)
		}
	}
}

func (e *Engine) reconcileAll(ctx context.Context) {
	e.graphsMu.RLock()
	activeWorkflows := make([]string, 0, len(e.graphs))
	for wfID := range e.graphs {
		activeWorkflows = append(activeWorkflows, wfID)
	}
	e.graphsMu.RUnlock()

	for _, wfID := range activeWorkflows {
		e.reconcileWorkflow(ctx, wfID)
	}
}

func (e *Engine) reconcileWorkflow(ctx context.Context, wfID string) {
	e.graphsMu.RLock()
	graph, exists := e.graphs[wfID]
	e.graphsMu.RUnlock()

	if !exists {
		return
	}

	tasks, err := e.store.ListTasksForWorkflow(ctx, wfID)
	if err != nil {
		return
	}

	taskStates := make(map[string]pb.TaskState, len(tasks))
	nowMs := time.Now().UnixMilli()
	for _, t := range tasks {
		taskStates[t.GetSpec().GetId()] = t.GetStatus().GetState()

		// Reconciliation safety net: detect tasks stuck in ASSIGNED beyond their timeout
		if t.GetStatus().GetState() == pb.TaskState_TASK_STATE_ASSIGNED {
			timeoutSec := t.GetSpec().GetTimeoutSeconds()
			if timeoutSec <= 0 {
				timeoutSec = 30
			}
			startedAt := t.GetStatus().GetStartedAt()
			if startedAt > 0 && nowMs-startedAt > int64(timeoutSec+5)*1000 {
				e.logger.Warn("reconciliation loop detected stalled ASSIGNED task past timeout",
					slog.String("workflow_id", wfID),
					slog.String("task_id", t.GetSpec().GetId()),
					slog.Int("attempt", int(t.GetStatus().GetCurrentAttempt())),
				)
				go e.handleTaskTimeout(wfID, t.GetSpec().GetId(), t.GetStatus().GetCurrentAttempt())
			}
		}
	}

	res := graph.Resolve(taskStates)
	e.applyResolution(ctx, wfID, res)
}

func (e *Engine) applyResolution(ctx context.Context, wfID string, res dag.ResolutionResult) {
	// 1. Enqueue newly ready tasks
	for _, taskID := range res.ReadyTasks {
		t, modRev, err := e.store.GetTask(ctx, wfID, taskID)
		if err != nil {
			continue
		}
		if t.Status.State == pb.TaskState_TASK_STATE_PENDING {
			readyTask, _, err := e.store.TransitionTaskState(ctx, e.candidate, wfID, taskID, modRev, pb.TaskState_TASK_STATE_READY, func(task *pb.Task) {
				task.Status.CurrentAttempt++
			})
			if err == nil {
				e.queue.Enqueue(&pb.TaskAssignment{
					WorkflowId:         wfID,
					Task:               readyTask.Spec,
					Attempt:            readyTask.Status.CurrentAttempt,
					LeaderFencingToken: e.candidate.Term(),
				}, 1)
				e.broadcastTaskUpdate(wfID, readyTask.Status)
				e.notifyCapacity()
			}
		}
	}

	// 2. Mark skipped tasks (under CONTINUE policy)
	for _, taskID := range res.SkippedTasks {
		t, modRev, err := e.store.GetTask(ctx, wfID, taskID)
		if err != nil {
			continue
		}
		if t.Status.State == pb.TaskState_TASK_STATE_PENDING || t.Status.State == pb.TaskState_TASK_STATE_READY {
			skippedTask, _, _ := e.store.TransitionTaskState(ctx, e.candidate, wfID, taskID, modRev, pb.TaskState_TASK_STATE_SKIPPED, nil)
			if skippedTask != nil {
				e.broadcastTaskUpdate(wfID, skippedTask.Status)
			}
		}
	}

	// 3. Mark cancelled tasks (under FAIL_FAST policy)
	for _, taskID := range res.CancelledTasks {
		t, modRev, err := e.store.GetTask(ctx, wfID, taskID)
		if err != nil {
			continue
		}
		if t.Status.State == pb.TaskState_TASK_STATE_PENDING || t.Status.State == pb.TaskState_TASK_STATE_READY {
			cancelledTask, _, _ := e.store.TransitionTaskState(ctx, e.candidate, wfID, taskID, modRev, pb.TaskState_TASK_STATE_CANCELLED, nil)
			if cancelledTask != nil {
				e.broadcastTaskUpdate(wfID, cancelledTask.Status)
			}
		}
	}

	// 4. Update workflow state if reached terminal state
	if res.WorkflowState != pb.WorkflowState_WORKFLOW_STATE_RUNNING {
		wf, modRev, err := e.store.GetWorkflow(ctx, wfID)
		if err == nil && wf.Status.State == pb.WorkflowState_WORKFLOW_STATE_RUNNING {
			_ = e.store.TransitionWorkflowState(ctx, e.candidate, wfID, modRev, res.WorkflowState)

			e.graphsMu.Lock()
			delete(e.graphs, wfID)
			e.graphsMu.Unlock()

			e.broadcastWorkflowUpdate(wfID, res.WorkflowState)
		}
	}
}

// --- OrchestratorService RPC Implementations ---

func (e *Engine) SubmitWorkflow(ctx context.Context, req *pb.SubmitWorkflowRequest) (*pb.SubmitWorkflowResponse, error) {
	if err := e.CheckLeader(); err != nil {
		return &pb.SubmitWorkflowResponse{Accepted: false, Message: err.Error()}, nil
	}

	spec := req.GetWorkflow()
	if spec == nil {
		return &pb.SubmitWorkflowResponse{Accepted: false, Message: "workflow spec cannot be nil"}, nil
	}

	// Validate DAG
	graph, err := dag.ValidateAndBuild(spec)
	if err != nil {
		return &pb.SubmitWorkflowResponse{Accepted: false, Message: fmt.Sprintf("DAG validation failed: %v", err)}, nil
	}

	wfID := spec.GetId()
	wf := &pb.Workflow{
		Spec: spec,
		Status: &pb.WorkflowStatus{
			Id:         wfID,
			State:      pb.WorkflowState_WORKFLOW_STATE_RUNNING,
			CreatedAt:  time.Now().UnixMilli(),
			TotalTasks: int32(len(spec.GetTasks())),
		},
	}

	// Submit to store with idempotency
	storedWf, isNew, err := e.store.SubmitWorkflow(ctx, e.candidate, wf)
	if err != nil {
		return nil, fmt.Errorf("failed to save workflow: %w", err)
	}

	if !isNew {
		// Idempotent submission: return existing workflow
		return &pb.SubmitWorkflowResponse{
			WorkflowId: wfID,
			Accepted:   true,
			Message:    "workflow already exists",
		}, nil
	}

	// Register DAG in memory
	e.graphsMu.Lock()
	e.graphs[wfID] = graph
	e.graphsMu.Unlock()

	// Enqueue all root tasks (0 dependencies) which are READY
	for _, taskSpec := range storedWf.GetSpec().GetTasks() {
		if len(taskSpec.GetDependencies()) == 0 {
			e.queue.Enqueue(&pb.TaskAssignment{
				WorkflowId:         wfID,
				Task:               taskSpec,
				Attempt:            1,
				LeaderFencingToken: e.candidate.Term(),
			}, 1)
			e.notifyCapacity()
		}
	}

	return &pb.SubmitWorkflowResponse{
		WorkflowId: wfID,
		Accepted:   true,
		Message:    "workflow accepted and running",
	}, nil
}

func (e *Engine) GetWorkflow(ctx context.Context, req *pb.GetWorkflowRequest) (*pb.GetWorkflowResponse, error) {
	wf, _, err := e.store.GetWorkflow(ctx, req.GetWorkflowId())
	if err != nil {
		return nil, err
	}
	tasks, err := e.store.ListTasksForWorkflow(ctx, req.GetWorkflowId())
	if err == nil {
		wf.Tasks = tasks
	}
	return &pb.GetWorkflowResponse{
		Workflow: wf,
	}, nil
}

func (e *Engine) ListWorkflows(ctx context.Context, req *pb.ListWorkflowsRequest) (*pb.ListWorkflowsResponse, error) {
	workflows, err := e.store.ListWorkflows(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.ListWorkflowsResponse{Workflows: workflows}, nil
}

func (e *Engine) CancelWorkflow(ctx context.Context, req *pb.CancelWorkflowRequest) (*pb.CancelWorkflowResponse, error) {
	if err := e.CheckLeader(); err != nil {
		return &pb.CancelWorkflowResponse{Success: false, Message: err.Error()}, nil
	}

	wfID := req.GetWorkflowId()
	wf, modRev, err := e.store.GetWorkflow(ctx, wfID)
	if err != nil {
		return &pb.CancelWorkflowResponse{Success: false, Message: err.Error()}, nil
	}

	if wf.Status.State != pb.WorkflowState_WORKFLOW_STATE_RUNNING {
		return &pb.CancelWorkflowResponse{Success: false, Message: "workflow is not in running state"}, nil
	}

	// Cancel workflow in store
	_ = e.store.TransitionWorkflowState(ctx, e.candidate, wfID, modRev, pb.WorkflowState_WORKFLOW_STATE_CANCELLED)

	// Remove from in-memory graphs
	e.graphsMu.Lock()
	delete(e.graphs, wfID)
	e.graphsMu.Unlock()

	// Cancel all pending/ready/assigned/running tasks
	tasks, _ := e.store.ListTasksForWorkflow(ctx, wfID)
	for _, t := range tasks {
		e.stopTaskTimer(wfID, t.Spec.Id)
		if !store.IsTerminalTaskState(t.Status.State) {
			cancelledTask, _, _ := e.store.TransitionTaskState(ctx, e.candidate, wfID, t.Spec.Id, t.Status.ModRevision, pb.TaskState_TASK_STATE_CANCELLED, nil)
			if cancelledTask != nil {
				e.broadcastTaskUpdate(wfID, cancelledTask.Status)
			}
		}
	}

	e.broadcastWorkflowUpdate(wfID, pb.WorkflowState_WORKFLOW_STATE_CANCELLED)
	return &pb.CancelWorkflowResponse{Success: true, Message: "workflow cancelled"}, nil
}

func (e *Engine) WatchWorkflow(req *pb.WatchWorkflowRequest, stream pb.OrchestratorService_WatchWorkflowServer) error {
	wfID := req.GetWorkflowId()
	ch := make(chan *pb.WatchWorkflowResponse, 64)

	e.watchersMu.Lock()
	e.watchers[wfID] = append(e.watchers[wfID], ch)
	e.watchersMu.Unlock()

	defer func() {
		e.watchersMu.Lock()
		subs := e.watchers[wfID]
		for i, c := range subs {
			if c == ch {
				e.watchers[wfID] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		e.watchersMu.Unlock()
	}()

	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case update := <-ch:
			if err := stream.Send(update); err != nil {
				return err
			}
		}
	}
}

// --- WorkerService RPC Implementations ---

func (e *Engine) RegisterWorker(ctx context.Context, req *pb.RegisterWorkerRequest) (*pb.RegisterWorkerResponse, error) {
	workerID := req.GetWorkerId()
	capVal := int(req.GetCapacity())
	if capVal <= 0 {
		capVal = 1
	}

	e.workersMu.Lock()
	w, exists := e.workers[workerID]
	if !exists {
		w = &workerState{
			id:           workerID,
			capacity:     capVal,
			channel:      make(chan *pb.TaskAssignment, capVal*2),
			lastSeen:     time.Now(),
			streamActive: true,
		}
		e.workers[workerID] = w
	} else {
		w.capacity = capVal
		w.lastSeen = time.Now()
		w.streamActive = true
	}
	metrics.WorkerCount.Set(float64(len(e.workers)))
	e.workersMu.Unlock()

	e.logger.Info("registered worker node", slog.String("worker_id", workerID), slog.Int("capacity", capVal))
	e.notifyCapacity()

	return &pb.RegisterWorkerResponse{
		Registered:               true,
		HeartbeatIntervalSeconds: 3,
		LeaderId:                 e.nodeID,
	}, nil
}

func (e *Engine) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	workerID := req.GetWorkerId()

	e.workersMu.Lock()
	if w, exists := e.workers[workerID]; exists {
		w.lastSeen = time.Now()
	}
	e.workersMu.Unlock()

	return &pb.HeartbeatResponse{Acknowledged: true}, nil
}

func (e *Engine) StreamTasks(req *pb.StreamTasksRequest, stream pb.WorkerService_StreamTasksServer) error {
	workerID := req.GetWorkerId()

	e.workersMu.Lock()
	w, exists := e.workers[workerID]
	if !exists {
		w = &workerState{
			id:           workerID,
			capacity:     4,
			channel:      make(chan *pb.TaskAssignment, 8),
			lastSeen:     time.Now(),
			streamActive: true,
		}
		e.workers[workerID] = w
	} else {
		w.streamActive = true
		w.lastSeen = time.Now()
		if w.channel == nil {
			w.channel = make(chan *pb.TaskAssignment, w.capacity*2)
		}
	}
	ch := w.channel
	e.workersMu.Unlock()

	e.logger.Info("worker connected task dispatch stream", slog.String("worker_id", workerID))
	e.notifyCapacity()

	defer func() {
		e.workersMu.Lock()
		if curr, ok := e.workers[workerID]; ok {
			curr.streamActive = false
		}
		e.workersMu.Unlock()
		e.logger.Info("worker disconnected task dispatch stream", slog.String("worker_id", workerID))
	}()

	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case asgn, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(asgn); err != nil {
				// Re-enqueue assignment if send fails
				e.queue.Enqueue(asgn, 1)
				return err
			}
		}
	}
}

// ReportResult processes task outcomes reported by workers.
// Delivery semantics: At-least-once execution with exactly-one committed result per task.
func (e *Engine) ReportResult(ctx context.Context, req *pb.ReportResultRequest) (*pb.ReportResultResponse, error) {
	if err := e.CheckLeader(); err != nil {
		return &pb.ReportResultResponse{Committed: false, Message: err.Error()}, nil
	}

	wfID := req.GetWorkflowId()
	taskID := req.GetTaskId()
	workerID := req.GetWorkerId()

	// Cancel timeout timer if running
	e.stopTaskTimer(wfID, taskID)

	// Free worker capacity
	if workerID != "" {
		e.workersMu.Lock()
		if w, ok := e.workers[workerID]; ok && w.activeTasks > 0 {
			w.activeTasks--
		}
		e.workersMu.Unlock()
		e.notifyCapacity()
	}

	// Commit result through store
	res, err := e.store.CommitTaskResult(ctx, e.candidate, req)
	if err != nil {
		return &pb.ReportResultResponse{Committed: false, Message: err.Error()}, nil
	}

	if !res.Committed {
		return &pb.ReportResultResponse{Committed: false, Message: res.Message}, nil
	}

	// Broadcast task update
	if res.Task != nil && res.Task.Status != nil {
		e.broadcastTaskUpdate(wfID, res.Task.Status)

		// Check if task needs retry with exponential backoff
		if res.Task.Status.State == pb.TaskState_TASK_STATE_READY {
			attempt := res.Task.Status.CurrentAttempt
			backoff := e.computeBackoff(attempt)
			e.logger.Info("scheduling task retry with exponential backoff",
				slog.String("workflow_id", wfID),
				slog.String("task_id", taskID),
				slog.Int("next_attempt", int(attempt)),
				slog.Duration("backoff", backoff),
			)

			// Re-enqueue task after backoff delay
			time.AfterFunc(backoff, func() {
				if e.isReconciling.Load() {
					e.queue.Enqueue(&pb.TaskAssignment{
						WorkflowId:         wfID,
						Task:               res.Task.Spec,
						Attempt:            attempt,
						LeaderFencingToken: e.candidate.Term(),
					}, 1)
					e.notifyCapacity()
				}
			})
		}
	}

	// Reconcile dependent tasks
	e.reconcileWorkflow(ctx, wfID)

	return &pb.ReportResultResponse{Committed: true, Message: "task result committed"}, nil
}

func (e *Engine) broadcastTaskUpdate(wfID string, status *pb.TaskStatus) {
	e.watchersMu.RLock()
	subs := e.watchers[wfID]
	e.watchersMu.RUnlock()

	for _, ch := range subs {
		select {
		case ch <- &pb.WatchWorkflowResponse{
			WorkflowId: wfID,
			TaskUpdate: status,
			Timestamp:  time.Now().UnixMilli(),
		}:
		default:
		}
	}
}

func (e *Engine) broadcastWorkflowUpdate(wfID string, state pb.WorkflowState) {
	e.watchersMu.RLock()
	subs := e.watchers[wfID]
	e.watchersMu.RUnlock()

	for _, ch := range subs {
		select {
		case ch <- &pb.WatchWorkflowResponse{
			WorkflowId:    wfID,
			WorkflowState: state,
			Timestamp:     time.Now().UnixMilli(),
		}:
		default:
		}
	}
}
