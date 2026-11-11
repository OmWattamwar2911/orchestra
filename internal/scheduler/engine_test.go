package scheduler_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
	"google.golang.org/grpc"
	pb "orchestra/api/proto/orchestra/v1"
	"orchestra/internal/election"
	"orchestra/internal/scheduler"
	"orchestra/internal/store"
)

func startTestEtcd(t *testing.T) (*embed.Etcd, *clientv3.Client, func()) {
	t.Helper()
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("etcd-sched-%d", time.Now().UnixNano()))

	cfg := embed.NewConfig()
	cfg.Dir = dir
	cfg.LogLevel = "error"

	clientURL, _ := url.Parse("http://127.0.0.1:0")
	peerURL, _ := url.Parse("http://127.0.0.1:0")
	cfg.ListenClientUrls = []url.URL{*clientURL}
	cfg.AdvertiseClientUrls = []url.URL{*clientURL}
	cfg.ListenPeerUrls = []url.URL{*peerURL}
	cfg.AdvertisePeerUrls = []url.URL{*peerURL}
	cfg.InitialCluster = cfg.InitialClusterFromName(cfg.Name)

	e, err := embed.StartEtcd(cfg)
	if err != nil {
		t.Fatalf("failed to start embedded etcd: %v", err)
	}

	select {
	case <-e.Server.ReadyNotify():
	case <-time.After(10 * time.Second):
		e.Close()
		_ = os.RemoveAll(dir)
		t.Fatal("etcd startup timeout")
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{e.Clients[0].Addr().String()},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		e.Close()
		_ = os.RemoveAll(dir)
		t.Fatalf("client err: %v", err)
	}

	return e, client, func() {
		_ = client.Close()
		e.Close()
		_ = os.RemoveAll(dir)
	}
}

// mockStreamTasksServer implements pb.WorkerService_StreamTasksServer for testing worker streams.
type mockStreamTasksServer struct {
	grpc.ServerStream
	ctx      context.Context
	received chan *pb.TaskAssignment
}

func newMockStreamTasksServer(ctx context.Context) *mockStreamTasksServer {
	return &mockStreamTasksServer{
		ctx:      ctx,
		received: make(chan *pb.TaskAssignment, 32),
	}
}

func (m *mockStreamTasksServer) Context() context.Context {
	return m.ctx
}

func (m *mockStreamTasksServer) Send(asgn *pb.TaskAssignment) error {
	select {
	case <-m.ctx.Done():
		return m.ctx.Err()
	case m.received <- asgn:
		return nil
	}
}

// setupEngineAndLeader creates an active leader engine connected to test etcd.
func setupEngineAndLeader(t *testing.T, client *clientv3.Client, ctx context.Context) (*scheduler.Engine, *store.Store, *election.Candidate) {
	t.Helper()
	s := store.New(client, nil)

	electedCh := make(chan int64, 1)
	cand, err := election.NewCandidate(election.Config{
		NodeID:         "test-leader",
		Client:         client,
		ElectionPrefix: fmt.Sprintf("/orchestra/leader/election/test-%d", time.Now().UnixNano()),
		SessionTTLSec:  5,
		OnElected: func(leaderCtx context.Context, term int64) error {
			electedCh <- term
			return nil
		},
	})
	if err != nil {
		t.Fatalf("failed to create candidate: %v", err)
	}

	engine := scheduler.NewEngine(scheduler.Config{
		NodeID:      "test-leader",
		Store:       s,
		Candidate:   cand,
		EtcdClient:  client,
		BaseBackoff: 10 * time.Millisecond,
		MaxBackoff:  100 * time.Millisecond,
	})

	go func() {
		_ = cand.Campaign(ctx)
	}()

	var term int64
	select {
	case term = <-electedCh:
	case <-time.After(10 * time.Second):
		t.Fatal("leadership takeover timeout")
	}

	if err := engine.OnElected(ctx, term); err != nil {
		t.Fatalf("OnElected failed: %v", err)
	}

	return engine, s, cand
}

// Test 1: Diamond DAG completes in the right order (A -> [B, C] -> D)
func TestScheduler_DiamondDAGCompletesInRightOrder(t *testing.T) {
	_, client, cleanup := startTestEtcd(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	engine, s, _ := setupEngineAndLeader(t, client, ctx)

	// Register fake worker
	workerID := "fake-worker-diamond"
	regResp, err := engine.RegisterWorker(ctx, &pb.RegisterWorkerRequest{
		WorkerId: workerID,
		Capacity: 4,
	})
	if err != nil || !regResp.GetRegistered() {
		t.Fatalf("worker registration failed: %v", err)
	}

	streamSrv := newMockStreamTasksServer(ctx)
	go func() {
		_ = engine.StreamTasks(&pb.StreamTasksRequest{WorkerId: workerID}, streamSrv)
	}()

	// Submit diamond workflow
	submitResp, err := engine.SubmitWorkflow(ctx, &pb.SubmitWorkflowRequest{
		Workflow: &pb.WorkflowSpec{
			Id:            "wf-diamond",
			Name:          "Diamond Pipeline",
			FailurePolicy: pb.FailurePolicy_FAILURE_POLICY_FAIL_FAST,
			Tasks: []*pb.TaskSpec{
				{Id: "A"},
				{Id: "B", Dependencies: []string{"A"}},
				{Id: "C", Dependencies: []string{"A"}},
				{Id: "D", Dependencies: []string{"B", "C"}},
			},
		},
	})
	if err != nil || !submitResp.GetAccepted() {
		t.Fatalf("SubmitWorkflow failed: %v, %s", err, submitResp.GetMessage())
	}

	var executionMu sync.Mutex
	executionLog := make([]string, 0)

	// 1. Worker should receive Task A first
	select {
	case asgn := <-streamSrv.received:
		if asgn.GetTask().GetId() != "A" {
			t.Fatalf("expected task A first, got: %s", asgn.GetTask().GetId())
		}
		executionMu.Lock()
		executionLog = append(executionLog, "A")
		executionMu.Unlock()

		// Complete A
		_, err := engine.ReportResult(ctx, &pb.ReportResultRequest{
			WorkflowId: "wf-diamond",
			TaskId:     "A",
			AttemptId:  asgn.GetAttempt(),
			WorkerId:   workerID,
			Outcome:    pb.TaskOutcome_TASK_OUTCOME_SUCCESS,
		})
		if err != nil {
			t.Fatalf("failed to report result for A: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for task A")
	}

	// 2. Worker should receive Tasks B and C (in parallel)
	receivedParallel := make(map[string]bool)
	for i := 0; i < 2; i++ {
		select {
		case asgn := <-streamSrv.received:
			id := asgn.GetTask().GetId()
			if id != "B" && id != "C" {
				t.Fatalf("expected task B or C, got: %s", id)
			}
			receivedParallel[id] = true
			executionMu.Lock()
			executionLog = append(executionLog, id)
			executionMu.Unlock()

			// Complete task
			_, _ = engine.ReportResult(ctx, &pb.ReportResultRequest{
				WorkflowId: "wf-diamond",
				TaskId:     id,
				AttemptId:  asgn.GetAttempt(),
				WorkerId:   workerID,
				Outcome:    pb.TaskOutcome_TASK_OUTCOME_SUCCESS,
			})
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for parallel tasks (got %d/2)", len(receivedParallel))
		}
	}

	// 3. Worker should receive Task D only after BOTH B and C are completed
	select {
	case asgn := <-streamSrv.received:
		if asgn.GetTask().GetId() != "D" {
			t.Fatalf("expected task D, got: %s", asgn.GetTask().GetId())
		}
		executionMu.Lock()
		executionLog = append(executionLog, "D")
		executionMu.Unlock()

		_, _ = engine.ReportResult(ctx, &pb.ReportResultRequest{
			WorkflowId: "wf-diamond",
			TaskId:     "D",
			AttemptId:  asgn.GetAttempt(),
			WorkerId:   workerID,
			Outcome:    pb.TaskOutcome_TASK_OUTCOME_SUCCESS,
		})
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for task D")
	}

	// Verify workflow reached SUCCEEDED
	wf, _, err := s.GetWorkflow(ctx, "wf-diamond")
	if err != nil {
		t.Fatalf("GetWorkflow failed: %v", err)
	}
	if wf.GetStatus().GetState() != pb.WorkflowState_WORKFLOW_STATE_SUCCEEDED {
		t.Fatalf("expected workflow state SUCCEEDED, got: %s", wf.GetStatus().GetState())
	}

	// Verify ordering invariant: A was first, D was last, B and C in between
	executionMu.Lock()
	defer executionMu.Unlock()
	if len(executionLog) != 4 || executionLog[0] != "A" || executionLog[3] != "D" {
		t.Fatalf("topological execution order violated: %v", executionLog)
	}
}

// Test 2: Failure Policies: FAIL_FAST vs CONTINUE
func TestScheduler_FailurePolicy_FailFastVsContinue(t *testing.T) {
	_, client, cleanup := startTestEtcd(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	engine, s, _ := setupEngineAndLeader(t, client, ctx)

	workerID := "fake-worker-policy"
	_, _ = engine.RegisterWorker(ctx, &pb.RegisterWorkerRequest{WorkerId: workerID, Capacity: 4})
	streamSrv := newMockStreamTasksServer(ctx)
	go func() {
		_ = engine.StreamTasks(&pb.StreamTasksRequest{WorkerId: workerID}, streamSrv)
	}()

	// 1. FAIL_FAST test: A -> B; when A fails with maxRetries 0, B is CANCELLED and workflow FAILS immediately
	t.Run("FAIL_FAST cancels remaining tasks and fails workflow", func(t *testing.T) {
		_, err := engine.SubmitWorkflow(ctx, &pb.SubmitWorkflowRequest{
			Workflow: &pb.WorkflowSpec{
				Id:            "wf-fail-fast",
				FailurePolicy: pb.FailurePolicy_FAILURE_POLICY_FAIL_FAST,
				Tasks: []*pb.TaskSpec{
					{Id: "A", MaxRetries: 0},
					{Id: "B", Dependencies: []string{"A"}},
				},
			},
		})
		if err != nil {
			t.Fatalf("SubmitWorkflow failed: %v", err)
		}

		// Receive task A and fail it
		select {
		case asgn := <-streamSrv.received:
			if asgn.GetTask().GetId() != "A" {
				t.Fatalf("expected task A, got %s", asgn.GetTask().GetId())
			}
			_, _ = engine.ReportResult(ctx, &pb.ReportResultRequest{
				WorkflowId:   "wf-fail-fast",
				TaskId:       "A",
				AttemptId:    asgn.GetAttempt(),
				WorkerId:     workerID,
				Outcome:      pb.TaskOutcome_TASK_OUTCOME_FAILURE,
				ErrorMessage: "unrecoverable task error",
			})
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for task A")
		}

		time.Sleep(200 * time.Millisecond)

		// Assert Task A is in DEAD_LETTER
		taskA, _, _ := s.GetTask(ctx, "wf-fail-fast", "A")
		if taskA.Status.State != pb.TaskState_TASK_STATE_DEAD_LETTER {
			t.Fatalf("expected task A to be DEAD_LETTER, got %s", taskA.Status.State)
		}

		// Assert Task B was CANCELLED
		taskB, _, _ := s.GetTask(ctx, "wf-fail-fast", "B")
		if taskB.Status.State != pb.TaskState_TASK_STATE_CANCELLED {
			t.Fatalf("expected task B to be CANCELLED, got %s", taskB.Status.State)
		}

		// Assert Workflow is FAILED
		wf, _, _ := s.GetWorkflow(ctx, "wf-fail-fast")
		if wf.Status.State != pb.WorkflowState_WORKFLOW_STATE_FAILED {
			t.Fatalf("expected workflow state FAILED, got %s", wf.Status.State)
		}
	})

	// 2. CONTINUE test: Root -> [Branch1, Branch2]; Branch1 -> Downstream1
	// Branch1 fails: Downstream1 is SKIPPED; Branch2 continues executing and finishes!
	t.Run("CONTINUE skips only downstream tasks and allows independent branches to finish", func(t *testing.T) {
		_, err := engine.SubmitWorkflow(ctx, &pb.SubmitWorkflowRequest{
			Workflow: &pb.WorkflowSpec{
				Id:            "wf-continue",
				FailurePolicy: pb.FailurePolicy_FAILURE_POLICY_CONTINUE,
				Tasks: []*pb.TaskSpec{
					{Id: "Branch1", MaxRetries: 0},
					{Id: "Downstream1", Dependencies: []string{"Branch1"}},
					{Id: "Branch2"},
				},
			},
		})
		if err != nil {
			t.Fatalf("SubmitWorkflow failed: %v", err)
		}

		// Both Branch1 and Branch2 are ready roots
		for i := 0; i < 2; i++ {
			select {
			case asgn := <-streamSrv.received:
				id := asgn.GetTask().GetId()
				if id == "Branch1" {
					// Fail Branch1
					_, _ = engine.ReportResult(ctx, &pb.ReportResultRequest{
						WorkflowId:   "wf-continue",
						TaskId:       "Branch1",
						AttemptId:    asgn.GetAttempt(),
						WorkerId:     workerID,
						Outcome:      pb.TaskOutcome_TASK_OUTCOME_FAILURE,
						ErrorMessage: "branch 1 failed",
					})
				} else if id == "Branch2" {
					// Succeed Branch2
					_, _ = engine.ReportResult(ctx, &pb.ReportResultRequest{
						WorkflowId: "wf-continue",
						TaskId:     "Branch2",
						AttemptId:  asgn.GetAttempt(),
						WorkerId:   workerID,
						Outcome:    pb.TaskOutcome_TASK_OUTCOME_SUCCESS,
					})
				}
			case <-time.After(5 * time.Second):
				t.Fatal("timeout waiting for branch tasks")
			}
		}

		time.Sleep(200 * time.Millisecond)

		// Assert Downstream1 was SKIPPED
		down, _, _ := s.GetTask(ctx, "wf-continue", "Downstream1")
		if down.Status.State != pb.TaskState_TASK_STATE_SKIPPED {
			t.Fatalf("expected Downstream1 to be SKIPPED, got %s", down.Status.State)
		}

		// Assert Branch2 is COMPLETED
		b2, _, _ := s.GetTask(ctx, "wf-continue", "Branch2")
		if b2.Status.State != pb.TaskState_TASK_STATE_COMPLETED {
			t.Fatalf("expected Branch2 to be COMPLETED, got %s", b2.Status.State)
		}

		// Workflow reached terminal state
		wf, _, _ := s.GetWorkflow(ctx, "wf-continue")
		if wf.Status.State != pb.WorkflowState_WORKFLOW_STATE_FAILED {
			t.Fatalf("expected workflow state FAILED under continue with failed task, got %s", wf.Status.State)
		}
	})
}

// Test 3: Retry Policy: Exponential backoff up to max retries, then DEAD_LETTER
func TestScheduler_RetryThenDeadLetter(t *testing.T) {
	_, client, cleanup := startTestEtcd(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	engine, s, _ := setupEngineAndLeader(t, client, ctx)

	workerID := "fake-worker-retry"
	_, _ = engine.RegisterWorker(ctx, &pb.RegisterWorkerRequest{WorkerId: workerID, Capacity: 2})
	streamSrv := newMockStreamTasksServer(ctx)
	go func() {
		_ = engine.StreamTasks(&pb.StreamTasksRequest{WorkerId: workerID}, streamSrv)
	}()

	// Submit task with MaxRetries = 2 (attempts: 1, 2, 3)
	_, err := engine.SubmitWorkflow(ctx, &pb.SubmitWorkflowRequest{
		Workflow: &pb.WorkflowSpec{
			Id:            "wf-retry",
			FailurePolicy: pb.FailurePolicy_FAILURE_POLICY_FAIL_FAST,
			Tasks: []*pb.TaskSpec{
				{Id: "RetryTask", MaxRetries: 2},
			},
		},
	})
	if err != nil {
		t.Fatalf("SubmitWorkflow failed: %v", err)
	}

	// Attempt 1: fails
	select {
	case asgn := <-streamSrv.received:
		if asgn.GetAttempt() != 1 {
			t.Fatalf("expected attempt 1, got %d", asgn.GetAttempt())
		}
		_, _ = engine.ReportResult(ctx, &pb.ReportResultRequest{
			WorkflowId: "wf-retry",
			TaskId:     "RetryTask",
			AttemptId:  asgn.GetAttempt(),
			WorkerId:   workerID,
			Outcome:    pb.TaskOutcome_TASK_OUTCOME_FAILURE,
		})
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for attempt 1")
	}

	// Attempt 2: received after backoff, fails
	select {
	case asgn := <-streamSrv.received:
		if asgn.GetAttempt() != 2 {
			t.Fatalf("expected attempt 2, got %d", asgn.GetAttempt())
		}
		_, _ = engine.ReportResult(ctx, &pb.ReportResultRequest{
			WorkflowId: "wf-retry",
			TaskId:     "RetryTask",
			AttemptId:  asgn.GetAttempt(),
			WorkerId:   workerID,
			Outcome:    pb.TaskOutcome_TASK_OUTCOME_FAILURE,
		})
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for attempt 2")
	}

	// Attempt 3 (Final attempt): received, fails
	select {
	case asgn := <-streamSrv.received:
		if asgn.GetAttempt() != 3 {
			t.Fatalf("expected attempt 3, got %d", asgn.GetAttempt())
		}
		_, _ = engine.ReportResult(ctx, &pb.ReportResultRequest{
			WorkflowId: "wf-retry",
			TaskId:     "RetryTask",
			AttemptId:  asgn.GetAttempt(),
			WorkerId:   workerID,
			Outcome:    pb.TaskOutcome_TASK_OUTCOME_FAILURE,
		})
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for attempt 3")
	}

	time.Sleep(200 * time.Millisecond)

	// Max retries reached: task must transition to DEAD_LETTER
	task, _, err := s.GetTask(ctx, "wf-retry", "RetryTask")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if task.Status.State != pb.TaskState_TASK_STATE_DEAD_LETTER {
		t.Fatalf("expected task to be in DEAD_LETTER after exhausting retries, got %s", task.Status.State)
	}
}

// Test 4: Worker death re-queues the task immediately with new attempt ID
func TestScheduler_WorkerDeathRequeuesTask(t *testing.T) {
	_, client, cleanup := startTestEtcd(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	engine, s, _ := setupEngineAndLeader(t, client, ctx)

	// Register Worker 1 with a lease
	lease1, err := client.Grant(ctx, 5)
	if err != nil {
		t.Fatalf("grant lease failed: %v", err)
	}
	_ = s.RegisterWorkerLease(ctx, "worker-dying", []byte(`{"id":"worker-dying"}`), lease1.ID)

	_, _ = engine.RegisterWorker(ctx, &pb.RegisterWorkerRequest{WorkerId: "worker-dying", Capacity: 1})
	stream1 := newMockStreamTasksServer(ctx)
	go func() {
		_ = engine.StreamTasks(&pb.StreamTasksRequest{WorkerId: "worker-dying"}, stream1)
	}()

	// Register healthy Worker 2 to pick up the re-queued task
	_, _ = engine.RegisterWorker(ctx, &pb.RegisterWorkerRequest{WorkerId: "worker-survivor", Capacity: 1})
	stream2 := newMockStreamTasksServer(ctx)
	go func() {
		_ = engine.StreamTasks(&pb.StreamTasksRequest{WorkerId: "worker-survivor"}, stream2)
	}()

	// Submit workflow with 1 task (MaxRetries = 3)
	_, err = engine.SubmitWorkflow(ctx, &pb.SubmitWorkflowRequest{
		Workflow: &pb.WorkflowSpec{
			Id: "wf-worker-kill",
			Tasks: []*pb.TaskSpec{
				{Id: "CriticalTask", MaxRetries: 3},
			},
		},
	})
	if err != nil {
		t.Fatalf("SubmitWorkflow failed: %v", err)
	}

	// Worker 1 receives assignment on attempt 1
	var firstAttempt int32
	select {
	case asgn := <-stream1.received:
		firstAttempt = asgn.GetAttempt()
		if firstAttempt != 1 {
			t.Fatalf("expected attempt 1 on worker-dying, got %d", firstAttempt)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for task on worker-dying")
	}

	// Simulate Worker 1 crashing: revoke its lease on etcd!
	_, err = client.Revoke(ctx, lease1.ID)
	if err != nil {
		t.Fatalf("revoke failed: %v", err)
	}

	// The etcd watch catches lease expiration, immediately re-queuing the task to READY
	// Worker 2 should receive the re-queued task with incremented attempt ID!
	select {
	case asgn := <-stream2.received:
		if asgn.GetTask().GetId() != "CriticalTask" {
			t.Fatalf("expected CriticalTask on worker-survivor, got %s", asgn.GetTask().GetId())
		}
		if asgn.GetAttempt() != firstAttempt+1 {
			t.Fatalf("expected attempt %d on re-queued task, got %d", firstAttempt+1, asgn.GetAttempt())
		}

		// Worker 2 reports success
		res, err := engine.ReportResult(ctx, &pb.ReportResultRequest{
			WorkflowId: "wf-worker-kill",
			TaskId:     "CriticalTask",
			AttemptId:  asgn.GetAttempt(),
			WorkerId:   "worker-survivor",
			Outcome:    pb.TaskOutcome_TASK_OUTCOME_SUCCESS,
		})
		if err != nil || !res.GetCommitted() {
			t.Fatalf("failed to commit task on worker-survivor: %v", err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("timed out waiting for re-queued task on worker-survivor after worker-dying crash")
	}

	// Verify task completed in store
	task, _, _ := s.GetTask(ctx, "wf-worker-kill", "CriticalTask")
	if task.Status.State != pb.TaskState_TASK_STATE_COMPLETED {
		t.Fatalf("expected task COMPLETED, got %s", task.Status.State)
	}
}

// Test 5: Backpressure & Queue Depth
func TestScheduler_BackpressureAndQueueDepth(t *testing.T) {
	_, client, cleanup := startTestEtcd(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	engine, _, _ := setupEngineAndLeader(t, client, ctx)

	// Worker has capacity 1
	workerID := "worker-single-slot"
	_, _ = engine.RegisterWorker(ctx, &pb.RegisterWorkerRequest{WorkerId: workerID, Capacity: 1})
	streamSrv := newMockStreamTasksServer(ctx)
	go func() {
		_ = engine.StreamTasks(&pb.StreamTasksRequest{WorkerId: workerID}, streamSrv)
	}()

	// Submit workflow with 4 independent tasks
	_, err := engine.SubmitWorkflow(ctx, &pb.SubmitWorkflowRequest{
		Workflow: &pb.WorkflowSpec{
			Id: "wf-backpressure",
			Tasks: []*pb.TaskSpec{
				{Id: "T1"},
				{Id: "T2"},
				{Id: "T3"},
				{Id: "T4"},
			},
		},
	})
	if err != nil {
		t.Fatalf("SubmitWorkflow failed: %v", err)
	}

	// Worker receives first task T1
	var firstTask *pb.TaskAssignment
	select {
	case firstTask = <-streamSrv.received:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for first task")
	}

	time.Sleep(100 * time.Millisecond)

	// Backpressure: Since worker has capacity 1, the remaining 3 tasks must be waiting in the queue
	depth := engine.QueueDepth()
	if depth != 3 {
		t.Fatalf("expected queue depth 3 under backpressure, got %d", depth)
	}

	// Worker completes first task -> capacity is freed -> next task is dequeued
	_, _ = engine.ReportResult(ctx, &pb.ReportResultRequest{
		WorkflowId: "wf-backpressure",
		TaskId:     firstTask.GetTask().GetId(),
		AttemptId:  firstTask.GetAttempt(),
		WorkerId:   workerID,
		Outcome:    pb.TaskOutcome_TASK_OUTCOME_SUCCESS,
	})

	select {
	case <-streamSrv.received:
		// Received second task
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for second task after capacity freed")
	}

	time.Sleep(100 * time.Millisecond)
	depthAfter := engine.QueueDepth()
	if depthAfter != 2 {
		t.Fatalf("expected queue depth 2 after 1 task dispatched, got %d", depthAfter)
	}
}

// Test 6: Workflow Cancellation cancels pending and signals running tasks
func TestScheduler_CancelWorkflow(t *testing.T) {
	_, client, cleanup := startTestEtcd(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	engine, s, _ := setupEngineAndLeader(t, client, ctx)

	workerID := "worker-cancel"
	_, _ = engine.RegisterWorker(ctx, &pb.RegisterWorkerRequest{WorkerId: workerID, Capacity: 1})
	streamSrv := newMockStreamTasksServer(ctx)
	go func() {
		_ = engine.StreamTasks(&pb.StreamTasksRequest{WorkerId: workerID}, streamSrv)
	}()

	_, _ = engine.SubmitWorkflow(ctx, &pb.SubmitWorkflowRequest{
		Workflow: &pb.WorkflowSpec{
			Id: "wf-cancel-test",
			Tasks: []*pb.TaskSpec{
				{Id: "RunningTask"},
				{Id: "PendingTask", Dependencies: []string{"RunningTask"}},
			},
		},
	})

	// Wait for RunningTask to be dispatched
	select {
	case <-streamSrv.received:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for RunningTask")
	}

	// Cancel workflow
	cancelResp, err := engine.CancelWorkflow(ctx, &pb.CancelWorkflowRequest{WorkflowId: "wf-cancel-test"})
	if err != nil || !cancelResp.GetSuccess() {
		t.Fatalf("CancelWorkflow failed: %v", err)
	}

	// Verify workflow is CANCELLED
	wf, _, _ := s.GetWorkflow(ctx, "wf-cancel-test")
	if wf.Status.State != pb.WorkflowState_WORKFLOW_STATE_CANCELLED {
		t.Fatalf("expected workflow state CANCELLED, got %s", wf.Status.State)
	}

	// Verify both tasks are CANCELLED
	tRun, _, _ := s.GetTask(ctx, "wf-cancel-test", "RunningTask")
	tPend, _, _ := s.GetTask(ctx, "wf-cancel-test", "PendingTask")
	if tRun.Status.State != pb.TaskState_TASK_STATE_CANCELLED {
		t.Fatalf("expected RunningTask to be CANCELLED, got %s", tRun.Status.State)
	}
	if tPend.Status.State != pb.TaskState_TASK_STATE_CANCELLED {
		t.Fatalf("expected PendingTask to be CANCELLED, got %s", tPend.Status.State)
	}
}
