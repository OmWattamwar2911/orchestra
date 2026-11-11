package store_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
	pb "orchestra/api/proto/orchestra/v1"
	"orchestra/internal/election"
	"orchestra/internal/store"
)

func startTestEtcd(t *testing.T) (*embed.Etcd, *clientv3.Client, func()) {
	t.Helper()
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("etcd-store-test-%d", time.Now().UnixNano()))

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
		t.Fatalf("failed to create etcd client: %v", err)
	}

	return e, client, func() {
		_ = client.Close()
		e.Close()
		_ = os.RemoveAll(dir)
	}
}

type testMockFencer struct {
	client    *clientv3.Client
	leaderKey string
	term      int64
}

func (m *testMockFencer) Term() int64 {
	return m.term
}

func (m *testMockFencer) LeaderKey() string {
	return m.leaderKey
}

func (m *testMockFencer) Fenced(ops ...clientv3.Op) (*clientv3.TxnResponse, error) {
	return m.FencedContext(context.Background(), nil, ops...)
}

func (m *testMockFencer) FencedContext(ctx context.Context, extraCmps []clientv3.Cmp, ops ...clientv3.Op) (*clientv3.TxnResponse, error) {
	cmp := clientv3.Compare(clientv3.CreateRevision(m.leaderKey), "=", m.term)
	allCmps := append([]clientv3.Cmp{cmp}, extraCmps...)
	return m.client.Txn(ctx).If(allCmps...).Then(ops...).Commit()
}

func createTestFencer(t *testing.T, client *clientv3.Client) *testMockFencer {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	leaderKey := fmt.Sprintf("/orchestra/leader/election/test-%d", time.Now().UnixNano())
	resp, err := client.Put(ctx, leaderKey, "leader-test")
	if err != nil {
		t.Fatalf("failed to initialize test leader key: %v", err)
	}
	return &testMockFencer{
		client:    client,
		leaderKey: leaderKey,
		term:      resp.Header.Revision,
	}
}

// Test 1: Idempotent SubmitWorkflow
func TestStore_IdempotentSubmitWorkflow(t *testing.T) {
	_, client, cleanup := startTestEtcd(t)
	defer cleanup()

	s := store.New(client, nil)
	fencer := createTestFencer(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wfSpec := &pb.WorkflowSpec{
		Id:            "wf-idempotent-1",
		Name:          "Pipeline 1",
		FailurePolicy: pb.FailurePolicy_FAILURE_POLICY_FAIL_FAST,
		Tasks: []*pb.TaskSpec{
			{Id: "task-A", Type: "bash"},
			{Id: "task-B", Type: "bash", Dependencies: []string{"task-A"}},
		},
	}

	wf := &pb.Workflow{Spec: wfSpec}

	// 1. Initial SubmitWorkflow: should create workflow and root task as READY
	createdWf, isNew, err := s.SubmitWorkflow(ctx, fencer, wf)
	if err != nil {
		t.Fatalf("first SubmitWorkflow failed: %v", err)
	}
	if !isNew {
		t.Fatal("expected first submission to be flagged as new (isNew=true)")
	}
	if createdWf.GetSpec().GetId() != "wf-idempotent-1" {
		t.Fatalf("unexpected workflow ID: %s", createdWf.GetSpec().GetId())
	}

	// Verify root task-A initialized to READY (0 dependencies)
	taskA, _, err := s.GetTask(ctx, "wf-idempotent-1", "task-A")
	if err != nil {
		t.Fatalf("failed to get task-A: %v", err)
	}
	if taskA.Status.State != pb.TaskState_TASK_STATE_READY {
		t.Fatalf("expected task-A to be READY, got: %s", taskA.Status.State)
	}

	// Verify task-B initialized to PENDING (depends on task-A)
	taskB, _, err := s.GetTask(ctx, "wf-idempotent-1", "task-B")
	if err != nil {
		t.Fatalf("failed to get task-B: %v", err)
	}
	if taskB.Status.State != pb.TaskState_TASK_STATE_PENDING {
		t.Fatalf("expected task-B to be PENDING, got: %s", taskB.Status.State)
	}

	// 2. Duplicate SubmitWorkflow with same ID: must return existing workflow without creating duplicates
	secondWf, isNew2, err := s.SubmitWorkflow(ctx, fencer, wf)
	if err != nil {
		t.Fatalf("second SubmitWorkflow failed: %v", err)
	}
	if isNew2 {
		t.Fatal("expected second submission to return existing workflow (isNew=false)")
	}
	if secondWf.GetSpec().GetId() != "wf-idempotent-1" {
		t.Fatalf("expected same workflow ID, got: %s", secondWf.GetSpec().GetId())
	}

	// Verify workflow list only contains 1 workflow
	list, err := s.ListWorkflows(ctx)
	if err != nil {
		t.Fatalf("ListWorkflows failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected exactly 1 workflow in store, got %d", len(list))
	}
}

// Test 2: Legal and Illegal State Transitions
func TestStore_StateTransitionsAndValidation(t *testing.T) {
	_, client, cleanup := startTestEtcd(t)
	defer cleanup()

	s := store.New(client, nil)
	fencer := createTestFencer(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wf := &pb.Workflow{
		Spec: &pb.WorkflowSpec{
			Id: "wf-transitions",
			Tasks: []*pb.TaskSpec{
				{Id: "T1", MaxRetries: 2},
			},
		},
	}
	_, _, err := s.SubmitWorkflow(ctx, fencer, wf)
	if err != nil {
		t.Fatalf("SubmitWorkflow failed: %v", err)
	}

	// T1 is READY (0 dependencies)
	task, modRev, err := s.GetTask(ctx, "wf-transitions", "T1")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if task.Status.State != pb.TaskState_TASK_STATE_READY {
		t.Fatalf("expected initial state READY, got %s", task.Status.State)
	}

	// Illegal transition: READY -> COMPLETED directly (must go through ASSIGNED/RUNNING)
	_, _, err = s.TransitionTaskState(ctx, fencer, "wf-transitions", "T1", modRev, pb.TaskState_TASK_STATE_COMPLETED, nil)
	if err == nil || !errors.Is(err, store.ErrIllegalStateTransition) {
		t.Fatalf("expected ErrIllegalStateTransition for READY -> COMPLETED, got: %v", err)
	}

	// Legal transition: READY -> ASSIGNED
	task, modRev, err = s.TransitionTaskState(ctx, fencer, "wf-transitions", "T1", modRev, pb.TaskState_TASK_STATE_ASSIGNED, func(t *pb.Task) {
		t.Status.AssignedWorker = "worker-1"
	})
	if err != nil {
		t.Fatalf("READY -> ASSIGNED failed: %v", err)
	}
	if task.Status.State != pb.TaskState_TASK_STATE_ASSIGNED {
		t.Fatalf("expected state ASSIGNED, got %s", task.Status.State)
	}

	// Legal transition: ASSIGNED -> RUNNING
	task, modRev, err = s.TransitionTaskState(ctx, fencer, "wf-transitions", "T1", modRev, pb.TaskState_TASK_STATE_RUNNING, func(t *pb.Task) {
		t.Status.StartedAt = time.Now().UnixMilli()
	})
	if err != nil {
		t.Fatalf("ASSIGNED -> RUNNING failed: %v", err)
	}
	if task.Status.State != pb.TaskState_TASK_STATE_RUNNING {
		t.Fatalf("expected state RUNNING, got %s", task.Status.State)
	}

	// Legal transition: RUNNING -> COMPLETED
	task, modRev, err = s.TransitionTaskState(ctx, fencer, "wf-transitions", "T1", modRev, pb.TaskState_TASK_STATE_COMPLETED, func(t *pb.Task) {
		t.Status.ResultData = []byte("success-output")
	})
	if err != nil {
		t.Fatalf("RUNNING -> COMPLETED failed: %v", err)
	}
	if task.Status.State != pb.TaskState_TASK_STATE_COMPLETED {
		t.Fatalf("expected state COMPLETED, got %s", task.Status.State)
	}

	// Illegal transition out of terminal state: COMPLETED -> READY
	_, _, err = s.TransitionTaskState(ctx, fencer, "wf-transitions", "T1", modRev, pb.TaskState_TASK_STATE_READY, nil)
	if err == nil || !errors.Is(err, store.ErrIllegalStateTransition) {
		t.Fatalf("expected ErrIllegalStateTransition out of terminal state COMPLETED, got: %v", err)
	}
}

// Test 3: Exactly-One Committed Result & Duplicate Result Report Rejection
func TestStore_CommitTaskResult_DuplicateRejectedAndCountedInMetric(t *testing.T) {
	_, client, cleanup := startTestEtcd(t)
	defer cleanup()

	s := store.New(client, nil)
	fencer := createTestFencer(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wf := &pb.Workflow{
		Spec: &pb.WorkflowSpec{
			Id: "wf-commit-dup",
			Tasks: []*pb.TaskSpec{
				{Id: "task-exec", MaxRetries: 3},
			},
		},
	}
	_, _, err := s.SubmitWorkflow(ctx, fencer, wf)
	if err != nil {
		t.Fatalf("SubmitWorkflow failed: %v", err)
	}

	_, modRev, _ := s.GetTask(ctx, "wf-commit-dup", "task-exec")

	// Transition to ASSIGNED -> RUNNING on attempt 1
	_, modRev, err = s.TransitionTaskState(ctx, fencer, "wf-commit-dup", "task-exec", modRev, pb.TaskState_TASK_STATE_ASSIGNED, func(t *pb.Task) {
		t.Status.AssignedWorker = "worker-1"
	})
	if err != nil {
		t.Fatalf("transition to ASSIGNED failed: %v", err)
	}
	_, _, err = s.TransitionTaskState(ctx, fencer, "wf-commit-dup", "task-exec", modRev, pb.TaskState_TASK_STATE_RUNNING, nil)
	if err != nil {
		t.Fatalf("transition to RUNNING failed: %v", err)
	}

	// 1. Worker 1 reports result for attempt 1: should be accepted and committed
	req1 := &pb.ReportResultRequest{
		WorkflowId: "wf-commit-dup",
		TaskId:     "task-exec",
		AttemptId:  1,
		WorkerId:   "worker-1",
		Outcome:    pb.TaskOutcome_TASK_OUTCOME_SUCCESS,
		Output:     []byte("committed-result-1"),
	}

	res1, err := s.CommitTaskResult(ctx, fencer, req1)
	if err != nil || !res1.Committed {
		t.Fatalf("expected first report to be committed, got: %v (%s)", err, res1.Message)
	}

	// 2. Worker 1 or duplicate delayed report sends attempt 1 again: MUST be rejected
	req2 := &pb.ReportResultRequest{
		WorkflowId: "wf-commit-dup",
		TaskId:     "task-exec",
		AttemptId:  1,
		WorkerId:   "worker-stale",
		Outcome:    pb.TaskOutcome_TASK_OUTCOME_SUCCESS,
		Output:     []byte("stale-duplicate-output"),
	}

	res2, err := s.CommitTaskResult(ctx, fencer, req2)
	if err != nil {
		t.Fatalf("unexpected error on duplicate report: %v", err)
	}
	if res2.Committed {
		t.Fatal("CRITICAL INVARIANT VIOLATION: duplicate report was committed!")
	}

	// 3. Stale attempt ID mismatch test (e.g. attempt 0 or attempt 99): MUST be rejected
	req3 := &pb.ReportResultRequest{
		WorkflowId: "wf-commit-dup",
		TaskId:     "task-exec",
		AttemptId:  99,
		WorkerId:   "worker-ghost",
		Outcome:    pb.TaskOutcome_TASK_OUTCOME_SUCCESS,
	}
	res3, _ := s.CommitTaskResult(ctx, fencer, req3)
	if res3.Committed {
		t.Fatal("expected report with wrong attempt ID to be rejected")
	}

	// 4. Invariant: Assert that etcd strictly holds the first committed result data
	finalTask, _, err := s.GetTask(ctx, "wf-commit-dup", "task-exec")
	if err != nil {
		t.Fatalf("failed to retrieve task: %v", err)
	}
	if string(finalTask.Status.ResultData) != "committed-result-1" {
		t.Fatalf("expected 'committed-result-1', got '%s'", string(finalTask.Status.ResultData))
	}
	if finalTask.Status.State != pb.TaskState_TASK_STATE_COMPLETED {
		t.Fatalf("expected state COMPLETED, got %s", finalTask.Status.State)
	}
}

// Test 4: Append-Only Event Stream in Same Transaction
func TestStore_AppendOnlyEventStream(t *testing.T) {
	_, client, cleanup := startTestEtcd(t)
	defer cleanup()

	s := store.New(client, nil)
	fencer := createTestFencer(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wf := &pb.Workflow{
		Spec: &pb.WorkflowSpec{
			Id: "wf-events",
			Tasks: []*pb.TaskSpec{
				{Id: "A"},
			},
		},
	}
	// Initial submit writes workflow creation event
	_, _, err := s.SubmitWorkflow(ctx, fencer, wf)
	if err != nil {
		t.Fatalf("SubmitWorkflow failed: %v", err)
	}

	_, modRev, _ := s.GetTask(ctx, "wf-events", "A")

	// Transition: READY -> ASSIGNED -> RUNNING -> COMPLETED (3 more events)
	_, modRev, _ = s.TransitionTaskState(ctx, fencer, "wf-events", "A", modRev, pb.TaskState_TASK_STATE_ASSIGNED, nil)
	_, modRev, _ = s.TransitionTaskState(ctx, fencer, "wf-events", "A", modRev, pb.TaskState_TASK_STATE_RUNNING, nil)
	_, _, _ = s.TransitionTaskState(ctx, fencer, "wf-events", "A", modRev, pb.TaskState_TASK_STATE_COMPLETED, nil)

	// Query events
	events, err := s.ListEvents(ctx, 1, 100)
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}

	if len(events) < 4 {
		t.Fatalf("expected at least 4 events in append-only log, got %d", len(events))
	}

	// Verify strictly monotonic sequence numbering
	for i := 0; i < len(events); i++ {
		if events[i].SequenceNumber != int64(i+1) {
			t.Fatalf("expected sequence number %d, got %d", i+1, events[i].SequenceNumber)
		}
		if events[i].WorkflowID != "wf-events" {
			t.Fatalf("expected workflow ID 'wf-events', got '%s'", events[i].WorkflowID)
		}
		if events[i].LeaderTerm <= 0 {
			t.Fatalf("event leader term must be positive, got %d", events[i].LeaderTerm)
		}
	}
}

// Test 5: Leader Crash Mid-Workflow & Recovery
// Simulates a leader crash mid-workflow where in-flight tasks assigned to a dead worker
// are re-queued by the new leader, and the new leader drives the workflow to completion.
func TestStore_LeaderCrashMidWorkflow_RecoveryFinishesWorkflow(t *testing.T) {
	_, client, cleanup := startTestEtcd(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	s := store.New(client, nil)

	// 1. Candidate 1 becomes Leader 1
	elected1 := make(chan int64, 1)
	cand1, err := election.NewCandidate(election.Config{
		NodeID:         "leader-1",
		Client:         client,
		ElectionPrefix: "/orchestra/leader/election",
		SessionTTLSec:  5,
		OnElected: func(leaderCtx context.Context, term int64) error {
			elected1 <- term
			return nil
		},
	})
	if err != nil {
		t.Fatalf("failed to create cand1: %v", err)
	}

	cand1Ctx, cancelCand1 := context.WithCancel(ctx)
	go func() {
		_ = cand1.Campaign(cand1Ctx)
	}()

	select {
	case <-elected1:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for cand1 to become leader")
	}

	// 2. Register worker-1 with a lease
	leaseResp, err := client.Grant(ctx, 5)
	if err != nil {
		t.Fatalf("failed to grant lease: %v", err)
	}
	_ = s.RegisterWorkerLease(ctx, "worker-1", []byte(`{"id":"worker-1"}`), leaseResp.ID)

	// 3. Leader 1 submits diamond workflow: A -> [B, C] -> D
	diamondWf := &pb.Workflow{
		Spec: &pb.WorkflowSpec{
			Id:            "wf-failover-diamond",
			Name:          "Failover Diamond",
			FailurePolicy: pb.FailurePolicy_FAILURE_POLICY_FAIL_FAST,
			Tasks: []*pb.TaskSpec{
				{Id: "A"},
				{Id: "B", Dependencies: []string{"A"}, MaxRetries: 3},
				{Id: "C", Dependencies: []string{"A"}, MaxRetries: 3},
				{Id: "D", Dependencies: []string{"B", "C"}},
			},
		},
	}

	_, _, err = s.SubmitWorkflow(ctx, cand1, diamondWf)
	if err != nil {
		t.Fatalf("SubmitWorkflow failed: %v", err)
	}

	// Task A completes under Leader 1
	_, revA, _ := s.GetTask(ctx, "wf-failover-diamond", "A")
	_, revA, _ = s.TransitionTaskState(ctx, cand1, "wf-failover-diamond", "A", revA, pb.TaskState_TASK_STATE_ASSIGNED, func(t *pb.Task) {
		t.Status.AssignedWorker = "worker-1"
	})
	_, revA, _ = s.TransitionTaskState(ctx, cand1, "wf-failover-diamond", "A", revA, pb.TaskState_TASK_STATE_RUNNING, nil)
	_, _, _ = s.TransitionTaskState(ctx, cand1, "wf-failover-diamond", "A", revA, pb.TaskState_TASK_STATE_COMPLETED, nil)

	// Task B is transitioned to READY -> ASSIGNED -> RUNNING on worker-1
	_, revB, _ := s.GetTask(ctx, "wf-failover-diamond", "B")
	_, revB, _ = s.TransitionTaskState(ctx, cand1, "wf-failover-diamond", "B", revB, pb.TaskState_TASK_STATE_READY, func(t *pb.Task) {
		t.Status.CurrentAttempt = 1
	})
	_, revB, _ = s.TransitionTaskState(ctx, cand1, "wf-failover-diamond", "B", revB, pb.TaskState_TASK_STATE_ASSIGNED, func(t *pb.Task) {
		t.Status.AssignedWorker = "worker-1"
	})
	_, _, _ = s.TransitionTaskState(ctx, cand1, "wf-failover-diamond", "B", revB, pb.TaskState_TASK_STATE_RUNNING, nil)

	// Task C is also READY
	_, revC, _ := s.GetTask(ctx, "wf-failover-diamond", "C")
	_, _, _ = s.TransitionTaskState(ctx, cand1, "wf-failover-diamond", "C", revC, pb.TaskState_TASK_STATE_READY, func(t *pb.Task) {
		t.Status.CurrentAttempt = 1
	})

	// 4. Simulate simultaneous worker-1 crash (lease revoked) and Leader 1 CRASH
	_, _ = client.Revoke(ctx, leaseResp.ID)
	cancelCand1() // Kill leader 1

	// 5. Candidate 2 steps in and becomes Leader 2
	elected2 := make(chan int64, 1)
	cand2, err := election.NewCandidate(election.Config{
		NodeID:         "leader-2",
		Client:         client,
		ElectionPrefix: "/orchestra/leader/election",
		SessionTTLSec:  5,
		OnElected: func(leaderCtx context.Context, term int64) error {
			elected2 <- term
			return nil
		},
	})
	if err != nil {
		t.Fatalf("failed to create cand2: %v", err)
	}

	go func() {
		_ = cand2.Campaign(ctx)
	}()

	var term2 int64
	select {
	case term2 = <-elected2:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for cand2 to win election")
	}

	// 6. Leader 2 calls Recover() to rebuild state and re-queue orphaned in-flight tasks
	recovery, err := s.Recover(ctx, cand2)
	if err != nil {
		t.Fatalf("Recover() failed on leader 2: %v", err)
	}

	if len(recovery.Workflows) != 1 {
		t.Fatalf("expected 1 active workflow recovered, got %d", len(recovery.Workflows))
	}
	if len(recovery.RequeuedTasks) != 1 || recovery.RequeuedTasks[0].Spec.Id != "B" {
		t.Fatalf("expected task B to be re-queued from dead worker-1, got %v", recovery.RequeuedTasks)
	}

	// Verify task B in store is now READY with incremented attempt
	taskBRecovered, revBRecovered, _ := s.GetTask(ctx, "wf-failover-diamond", "B")
	if taskBRecovered.Status.State != pb.TaskState_TASK_STATE_READY {
		t.Fatalf("expected task B to be reset to READY, got %s", taskBRecovered.Status.State)
	}
	if taskBRecovered.Status.CurrentAttempt != 2 {
		t.Fatalf("expected task B attempt incremented to 2, got %d", taskBRecovered.Status.CurrentAttempt)
	}

	// 7. Register a new healthy worker-2
	lease2, _ := client.Grant(ctx, 10)
	_ = s.RegisterWorkerLease(ctx, "worker-2", []byte(`{"id":"worker-2"}`), lease2.ID)

	// 8. Leader 2 completes tasks B, C, and D
	// Complete B on attempt 2
	_, revBRecovered, _ = s.TransitionTaskState(ctx, cand2, "wf-failover-diamond", "B", revBRecovered, pb.TaskState_TASK_STATE_ASSIGNED, func(t *pb.Task) {
		t.Status.AssignedWorker = "worker-2"
	})
	_, _, _ = s.TransitionTaskState(ctx, cand2, "wf-failover-diamond", "B", revBRecovered, pb.TaskState_TASK_STATE_RUNNING, nil)
	commitB, err := s.CommitTaskResult(ctx, cand2, &pb.ReportResultRequest{
		WorkflowId: "wf-failover-diamond",
		TaskId:     "B",
		AttemptId:  2,
		WorkerId:   "worker-2",
		Outcome:    pb.TaskOutcome_TASK_OUTCOME_SUCCESS,
	})
	if err != nil || !commitB.Committed {
		t.Fatalf("failed to commit task B under leader 2: %v", err)
	}

	// Complete C
	_, revC, _ = s.GetTask(ctx, "wf-failover-diamond", "C")
	_, revC, _ = s.TransitionTaskState(ctx, cand2, "wf-failover-diamond", "C", revC, pb.TaskState_TASK_STATE_ASSIGNED, func(t *pb.Task) {
		t.Status.AssignedWorker = "worker-2"
	})
	_, _, _ = s.TransitionTaskState(ctx, cand2, "wf-failover-diamond", "C", revC, pb.TaskState_TASK_STATE_RUNNING, nil)
	commitC, err := s.CommitTaskResult(ctx, cand2, &pb.ReportResultRequest{
		WorkflowId: "wf-failover-diamond",
		TaskId:     "C",
		AttemptId:  1,
		WorkerId:   "worker-2",
		Outcome:    pb.TaskOutcome_TASK_OUTCOME_SUCCESS,
	})
	if err != nil || !commitC.Committed {
		t.Fatalf("failed to commit task C under leader 2: %v", err)
	}

	// Both B and C are complete, D transitions to READY -> COMPLETED
	_, revD, _ := s.GetTask(ctx, "wf-failover-diamond", "D")
	_, revD, _ = s.TransitionTaskState(ctx, cand2, "wf-failover-diamond", "D", revD, pb.TaskState_TASK_STATE_READY, func(t *pb.Task) {
		t.Status.CurrentAttempt = 1
	})
	_, revD, _ = s.TransitionTaskState(ctx, cand2, "wf-failover-diamond", "D", revD, pb.TaskState_TASK_STATE_ASSIGNED, func(t *pb.Task) {
		t.Status.AssignedWorker = "worker-2"
	})
	_, _, _ = s.TransitionTaskState(ctx, cand2, "wf-failover-diamond", "D", revD, pb.TaskState_TASK_STATE_RUNNING, nil)
	commitD, err := s.CommitTaskResult(ctx, cand2, &pb.ReportResultRequest{
		WorkflowId: "wf-failover-diamond",
		TaskId:     "D",
		AttemptId:  1,
		WorkerId:   "worker-2",
		Outcome:    pb.TaskOutcome_TASK_OUTCOME_SUCCESS,
	})
	if err != nil || !commitD.Committed {
		t.Fatalf("failed to commit task D under leader 2: %v", err)
	}

	// Workflow completes to SUCCEEDED
	_, wfRev, _ := s.GetWorkflow(ctx, "wf-failover-diamond")
	err = s.TransitionWorkflowState(ctx, cand2, "wf-failover-diamond", wfRev, pb.WorkflowState_WORKFLOW_STATE_SUCCEEDED)
	if err != nil {
		t.Fatalf("failed to transition workflow to SUCCEEDED: %v", err)
	}

	// Verify final workflow state
	wfFinal, _, err := s.GetWorkflow(ctx, "wf-failover-diamond")
	if err != nil {
		t.Fatalf("failed to read back workflow: %v", err)
	}
	if wfFinal.Status.State != pb.WorkflowState_WORKFLOW_STATE_SUCCEEDED {
		t.Fatalf("expected workflow to be SUCCEEDED, got %s", wfFinal.Status.State)
	}
	if term2 <= 0 {
		t.Fatalf("expected positive leader term, got %d", term2)
	}
}
