package checker_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
	pb "orchestra/api/proto/orchestra/v1"
	"orchestra/internal/checker"
	"orchestra/internal/evidence"
	"orchestra/internal/store"
)

func startTestEtcd(t *testing.T) (*embed.Etcd, *clientv3.Client, func()) {
	t.Helper()
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("etcd-chk-%d", time.Now().UnixNano()))

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

func TestInvariantChecker_AllInvariants(t *testing.T) {
	_, client, cleanup := startTestEtcd(t)
	defer cleanup()

	tmpDir := filepath.Join(os.TempDir(), fmt.Sprintf("chk-ev-%d", time.Now().UnixNano()))
	defer os.RemoveAll(tmpDir)

	evPath := filepath.Join(tmpDir, "evidence.jsonl")
	evLogger, err := evidence.NewLogger(evPath)
	if err != nil {
		t.Fatalf("failed to create evidence logger: %v", err)
	}
	defer evLogger.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	s := store.NewStore(client)

	// Set up valid completed workflow
	wf := &pb.Workflow{
		Spec: &pb.WorkflowSpec{
			Id:            "wf-valid",
			Name:          "Valid Linear Pipeline",
			FailurePolicy: pb.FailurePolicy_FAILURE_POLICY_FAIL_FAST,
			Tasks: []*pb.TaskSpec{
				{Id: "taskA"},
				{Id: "taskB", Dependencies: []string{"taskA"}},
			},
		},
		Status: &pb.WorkflowStatus{
			Id:          "wf-valid",
			State:       pb.WorkflowState_WORKFLOW_STATE_SUCCEEDED,
			CompletedAt: 2000,
		},
	}
	_ = s.PutWorkflow(ctx, nil, wf)

	tA := &pb.Task{
		Spec: wf.Spec.Tasks[0],
		Status: &pb.TaskStatus{
			Id:          "taskA",
			WorkflowId:  "wf-valid",
			State:       pb.TaskState_TASK_STATE_COMPLETED,
			CompletedAt: 1000,
		},
	}
	tB := &pb.Task{
		Spec: wf.Spec.Tasks[1],
		Status: &pb.TaskStatus{
			Id:          "taskB",
			WorkflowId:  "wf-valid",
			State:       pb.TaskState_TASK_STATE_COMPLETED,
			CompletedAt: 2000,
		},
	}
	_, _ = s.PutTask(ctx, nil, tA)
	_, _ = s.PutTask(ctx, nil, tB)

	// Record legitimate sequence: taskA starts (100ms), finishes (1000ms), then taskB starts (1100ms), finishes (2000ms)
	_ = evLogger.Record(evidence.EventRecord{
		TimestampNs: 100 * 1e6,
		WorkerID:    "w1",
		WorkflowID:  "wf-valid",
		TaskID:      "taskA",
		Attempt:     1,
		Event:       evidence.EventStart,
	})
	_ = evLogger.Record(evidence.EventRecord{
		TimestampNs: 1000 * 1e6,
		WorkerID:    "w1",
		WorkflowID:  "wf-valid",
		TaskID:      "taskA",
		Attempt:     1,
		Event:       evidence.EventFinish,
		Status:      "SUCCESS",
	})
	_ = evLogger.Record(evidence.EventRecord{
		TimestampNs: 1100 * 1e6,
		WorkerID:    "w2",
		WorkflowID:  "wf-valid",
		TaskID:      "taskB",
		Attempt:     1,
		Event:       evidence.EventStart,
	})
	_ = evLogger.Record(evidence.EventRecord{
		TimestampNs: 2000 * 1e6,
		WorkerID:    "w2",
		WorkflowID:  "wf-valid",
		TaskID:      "taskB",
		Attempt:     1,
		Event:       evidence.EventFinish,
		Status:      "SUCCESS",
	})
	_ = evLogger.Close()

	chk := checker.NewInvariantChecker(s, evPath)
	res, err := chk.VerifyAll(ctx)
	if err != nil {
		t.Fatalf("verification error: %v", err)
	}

	if !res.AllPassed {
		t.Fatalf("expected all invariants to pass, but got violations: %v", res.Reports[0].Violations)
	}
}
