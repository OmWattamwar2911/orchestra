package etcdstore_test

import (
	"context"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	pb "orchestra/api/proto/orchestra/v1"
	"orchestra/internal/etcdstore"
)

type mockFencer struct {
	client    *clientv3.Client
	leaderKey string
	term      int64
}

func (m *mockFencer) Fenced(ops ...clientv3.Op) (*clientv3.TxnResponse, error) {
	return m.FencedContext(context.Background(), nil, ops...)
}

func (m *mockFencer) FencedContext(ctx context.Context, extraCmps []clientv3.Cmp, ops ...clientv3.Op) (*clientv3.TxnResponse, error) {
	cmp := clientv3.Compare(clientv3.CreateRevision(m.leaderKey), "=", m.term)
	allCmps := append([]clientv3.Cmp{cmp}, extraCmps...)
	return m.client.Txn(ctx).If(allCmps...).Then(ops...).Commit()
}

func TestStore_WorkflowCRUD(t *testing.T) {
	_, client, cleanup := StartEmbeddedEtcd(t)
	defer cleanup()

	store := etcdstore.NewStore(client)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wf := &pb.Workflow{
		Spec: &pb.WorkflowSpec{
			Id:            "wf-101",
			Name:          "Batch Pipeline",
			FailurePolicy: pb.FailurePolicy_FAILURE_POLICY_FAIL_FAST,
		},
		Status: &pb.WorkflowStatus{
			Id:    "wf-101",
			State: pb.WorkflowState_WORKFLOW_STATE_RUNNING,
		},
	}

	// 1. PutWorkflow without fencing token
	if err := store.PutWorkflow(ctx, nil, wf); err != nil {
		t.Fatalf("PutWorkflow failed: %v", err)
	}

	// 2. GetWorkflow
	fetched, err := store.GetWorkflow(ctx, "wf-101")
	if err != nil {
		t.Fatalf("GetWorkflow failed: %v", err)
	}
	if fetched.GetSpec().GetName() != "Batch Pipeline" {
		t.Fatalf("expected 'Batch Pipeline', got '%s'", fetched.GetSpec().GetName())
	}

	// 3. ListWorkflows
	list, err := store.ListWorkflows(ctx)
	if err != nil {
		t.Fatalf("ListWorkflows failed: %v", err)
	}
	if len(list) != 1 || list[0].GetSpec().GetId() != "wf-101" {
		t.Fatalf("expected 1 workflow in list, got %d", len(list))
	}
}

func TestStore_FencingProtection(t *testing.T) {
	_, client, cleanup := StartEmbeddedEtcd(t)
	defer cleanup()

	store := etcdstore.NewStore(client)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wf := &pb.Workflow{
		Spec: &pb.WorkflowSpec{
			Id:   "wf-fenced",
			Name: "Fenced Pipeline",
		},
	}

	// Fencing check should fail when using a fabricated or nonexistent election CreateRevision
	fakeFencer := &mockFencer{
		client:    client,
		leaderKey: "/orchestra/leader/election/fake",
		term:      999999,
	}
	err := store.PutWorkflow(ctx, fakeFencer, wf)
	if err == nil {
		t.Fatal("expected ErrFencingTokenMismatch for invalid fencing revision, got nil")
	}
}

func TestStore_CommitTaskResult_ExactlyOneCommitted(t *testing.T) {
	// Invariant verified: At-least-once execution with exactly-one committed result per task.
	_, client, cleanup := StartEmbeddedEtcd(t)
	defer cleanup()

	store := etcdstore.NewStore(client)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Set up simulated election key to yield a valid fencing token
	electionPutResp, err := client.Put(ctx, "/orchestra/leader/election/test-leader", "leader-node-1")
	if err != nil {
		t.Fatalf("failed to setup election key: %v", err)
	}
	// The CreateRevision of the election key acts as the fencing token
	fencer := &mockFencer{
		client:    client,
		leaderKey: "/orchestra/leader/election/test-leader",
		term:      electionPutResp.Header.Revision,
	}

	// 2. Put initial task
	task := &pb.Task{
		Spec: &pb.TaskSpec{
			Id:   "task-1",
			Type: "compute",
		},
		Status: &pb.TaskStatus{
			Id:             "task-1",
			WorkflowId:     "wf-cas",
			State:          pb.TaskState_TASK_STATE_RUNNING,
			CurrentAttempt: 1,
		},
	}

	modRev, err := store.PutTask(ctx, fencer, task)
	if err != nil {
		t.Fatalf("PutTask failed: %v", err)
	}

	// 3. Worker 1 finishes and attempts to commit result
	firstResult := &pb.TaskStatus{
		Id:             "task-1",
		WorkflowId:     "wf-cas",
		State:          pb.TaskState_TASK_STATE_COMPLETED,
		CurrentAttempt: 1,
		ResultData:     []byte("result from attempt 1"),
	}

	newRev, err := store.CommitTaskResult(ctx, fencer, "wf-cas", "task-1", modRev, firstResult)
	if err != nil {
		t.Fatalf("first commit should succeed, got error: %v", err)
	}
	if newRev <= modRev {
		t.Fatalf("expected new mod revision > %d, got %d", modRev, newRev)
	}

	// 4. Duplicate report from another worker (or delayed attempt) with stale modRev
	duplicateResult := &pb.TaskStatus{
		Id:             "task-1",
		WorkflowId:     "wf-cas",
		State:          pb.TaskState_TASK_STATE_COMPLETED,
		CurrentAttempt: 1,
		ResultData:     []byte("stale duplicate result"),
	}

	_, err = store.CommitTaskResult(ctx, fencer, "wf-cas", "task-1", modRev, duplicateResult)
	if err == nil {
		t.Fatal("expected CAS conflict on duplicate/stale result report, got success")
	}

	// Verify that state in store remains the first committed result
	finalTask, _, err := store.GetTask(ctx, "wf-cas", "task-1")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if string(finalTask.GetStatus().GetResultData()) != "result from attempt 1" {
		t.Fatalf("expected 'result from attempt 1', got '%s'", string(finalTask.GetStatus().GetResultData()))
	}
}

func TestStore_WorkerLeaseRegistration(t *testing.T) {
	_, client, cleanup := StartEmbeddedEtcd(t)
	defer cleanup()

	store := etcdstore.NewStore(client)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Grant 5-second lease
	leaseResp, err := client.Grant(ctx, 5)
	if err != nil {
		t.Fatalf("grant lease failed: %v", err)
	}

	metadata := []byte(`{"id":"worker-1","capacity":4}`)
	if err := store.RegisterWorkerLease(ctx, "worker-1", metadata, leaseResp.ID); err != nil {
		t.Fatalf("RegisterWorkerLease failed: %v", err)
	}

	workers, err := store.ListActiveWorkers(ctx)
	if err != nil {
		t.Fatalf("ListActiveWorkers failed: %v", err)
	}

	key := etcdstore.WorkerKey("worker-1")
	if string(workers[key]) != string(metadata) {
		t.Fatalf("expected metadata %s, got %s", string(metadata), string(workers[key]))
	}

	// Revoke lease and ensure worker vanishes
	_, err = client.Revoke(ctx, leaseResp.ID)
	if err != nil {
		t.Fatalf("revoke failed: %v", err)
	}

	workersAfter, err := store.ListActiveWorkers(ctx)
	if err != nil {
		t.Fatalf("ListActiveWorkers failed: %v", err)
	}
	if _, exists := workersAfter[key]; exists {
		t.Fatal("worker still exists after lease revocation")
	}
}
