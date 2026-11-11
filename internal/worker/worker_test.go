package worker_test

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
	"google.golang.org/grpc"
	pb "orchestra/api/proto/orchestra/v1"
	"orchestra/internal/election"
	"orchestra/internal/evidence"
	"orchestra/internal/scheduler"
	"orchestra/internal/store"
	"orchestra/internal/worker"
)

func startTestEtcd(t *testing.T) (*embed.Etcd, *clientv3.Client, func()) {
	t.Helper()
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("etcd-wtest-%d", time.Now().UnixNano()))

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

// setupOrchestratorServer spins up a real gRPC server for the scheduler engine and campaigns as leader.
func setupOrchestratorServer(t *testing.T, client *clientv3.Client, ctx context.Context, electionPrefix string) (*scheduler.Engine, *store.Store, string, func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen tcp: %v", err)
	}
	grpcAddr := lis.Addr().String()

	s := store.New(client, nil)

	electedCh := make(chan int64, 1)
	campaignVal := election.EncodeLeaderInfo("orch-leader", grpcAddr)

	cand, err := election.NewCandidate(election.Config{
		NodeID:         "orch-leader",
		CampaignValue:  campaignVal,
		Client:         client,
		ElectionPrefix: electionPrefix,
		SessionTTLSec:  5,
		OnElected: func(leaderCtx context.Context, term int64) error {
			electedCh <- term
			return nil
		},
	})
	if err != nil {
		lis.Close()
		t.Fatalf("failed to create candidate: %v", err)
	}

	engine := scheduler.NewEngine(scheduler.Config{
		NodeID:      "orch-leader",
		Store:       s,
		Candidate:   cand,
		EtcdClient:  client,
		BaseBackoff: 10 * time.Millisecond,
		MaxBackoff:  50 * time.Millisecond,
	})

	grpcServer := grpc.NewServer()
	pb.RegisterOrchestratorServiceServer(grpcServer, engine)
	pb.RegisterWorkerServiceServer(grpcServer, engine)

	go func() {
		_ = grpcServer.Serve(lis)
	}()

	leaderCtx, leaderCancel := context.WithCancel(ctx)
	go func() {
		_ = cand.Campaign(leaderCtx)
	}()

	var term int64
	select {
	case term = <-electedCh:
	case <-time.After(10 * time.Second):
		grpcServer.Stop()
		lis.Close()
		leaderCancel()
		t.Fatal("leadership takeover timeout")
	}

	if err := engine.OnElected(leaderCtx, term); err != nil {
		grpcServer.Stop()
		lis.Close()
		leaderCancel()
		t.Fatalf("OnElected failed: %v", err)
	}

	cleanup := func() {
		_ = cand.Resign(context.Background())
		leaderCancel()
		grpcServer.Stop()
		_ = lis.Close()
	}

	return engine, s, grpcAddr, cleanup
}

// Test: Kill worker mid-task without graceful shutdown (cancel context + revoke lease)
// Assert task is re-queued and completes on survivor worker.
func TestWorker_KillMidTaskRequeuesToSurvivor(t *testing.T) {
	_, client, cleanupEtcd := startTestEtcd(t)
	defer cleanupEtcd()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	electionPrefix := fmt.Sprintf("/orchestra/leader/election/test-%d", time.Now().UnixNano())
	engine, s, grpcAddr, cleanupOrch := setupOrchestratorServer(t, client, ctx, electionPrefix)
	defer cleanupOrch()

	tmpDir := filepath.Join(os.TempDir(), fmt.Sprintf("worker-chaos-%d", time.Now().UnixNano()))
	_ = os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	w1LogPath := filepath.Join(tmpDir, "evidence-w1.jsonl")
	w2LogPath := filepath.Join(tmpDir, "evidence-w2.jsonl")

	// 1. Start Worker 1 (victim worker)
	w1Ctx, cancelW1 := context.WithCancel(ctx)
	w1, err := worker.NewWorker(worker.WorkerConfig{
		WorkerID:                "worker-victim",
		Capacity:                1,
		EtcdClient:              client,
		ElectionPrefix:          electionPrefix,
		DefaultOrchestratorAddr: grpcAddr,
		EvidenceLogPath:         w1LogPath,
	})
	if err != nil {
		t.Fatalf("failed to create worker 1: %v", err)
	}
	go func() {
		_ = w1.Start(w1Ctx)
	}()

	// Allow Worker 1 time to register with etcd and connect gRPC stream
	time.Sleep(500 * time.Millisecond)

	// 2. Submit workflow with a slow sleep task (sleep 2s, timeout 10s, max retries 2)
	// Worker 1 is currently the only live worker, so it is guaranteed to receive Attempt 1!
	submitResp, err := engine.SubmitWorkflow(ctx, &pb.SubmitWorkflowRequest{
		Workflow: &pb.WorkflowSpec{
			Id:            "wf-midtask-kill",
			FailurePolicy: pb.FailurePolicy_FAILURE_POLICY_FAIL_FAST,
			Tasks: []*pb.TaskSpec{
				{
					Id:             "critical-task",
					Type:           "sleep",
					Payload:        "2s",
					TimeoutSeconds: 10,
					MaxRetries:     2,
				},
			},
		},
	})
	if err != nil || !submitResp.GetAccepted() {
		t.Fatalf("failed to submit workflow: %v", err)
	}

	// 3. Wait for Worker 1 to start executing the task (verify start recorded in evidence log)
	w1Reader := evidence.NewReader(w1LogPath)
	var startedOnW1 bool
	for i := 0; i < 40; i++ {
		recs, _ := w1Reader.ReadAll()
		for _, r := range recs {
			if r.TaskID == "critical-task" && r.Event == evidence.EventStart && r.Attempt == 1 {
				startedOnW1 = true
				break
			}
		}
		if startedOnW1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !startedOnW1 {
		t.Fatal("timed out waiting for task to start on worker 1")
	}

	// 4. Start Worker 2 (survivor worker) so it's ready to receive the re-queued task
	w2Ctx, cancelW2 := context.WithCancel(ctx)
	defer cancelW2()
	w2, err := worker.NewWorker(worker.WorkerConfig{
		WorkerID:                "worker-survivor",
		Capacity:                1,
		EtcdClient:              client,
		ElectionPrefix:          electionPrefix,
		DefaultOrchestratorAddr: grpcAddr,
		EvidenceLogPath:         w2LogPath,
	})
	if err != nil {
		t.Fatalf("failed to create worker 2: %v", err)
	}
	go func() {
		_ = w2.Start(w2Ctx)
	}()

	time.Sleep(400 * time.Millisecond)

	// 5. KILL Worker 1 MID-TASK WITHOUT GRACEFUL SHUTDOWN!
	// Abruptly cancel its context and revoke its etcd lease directly.
	cancelW1()
	lease1 := w1.LeaseID()
	if lease1 != 0 {
		_, _ = client.Revoke(context.Background(), lease1)
	}

	// 6. The scheduler's etcd worker watch catches lease loss, immediately re-queues the task (attempt 2),
	// and assigns it to Worker 2 (worker-survivor)!
	w2Reader := evidence.NewReader(w2LogPath)
	var completedOnW2 bool
	for i := 0; i < 80; i++ {
		recs, _ := w2Reader.ReadAll()
		for _, r := range recs {
			if r.TaskID == "critical-task" && r.Event == evidence.EventFinish && r.Status == "SUCCESS" && r.Attempt == 2 {
				completedOnW2 = true
				break
			}
		}
		if completedOnW2 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !completedOnW2 {
		t.Fatal("timed out waiting for task to be re-queued and completed on worker 2")
	}

	// 7. Verify workflow reached terminal state SUCCEEDED in store
	var wf *pb.Workflow
	for i := 0; i < 50; i++ {
		wf, _, err = s.GetWorkflow(ctx, "wf-midtask-kill")
		if err == nil && wf.GetStatus().GetState() == pb.WorkflowState_WORKFLOW_STATE_SUCCEEDED {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GetWorkflow failed: %v", err)
	}
	if wf.GetStatus().GetState() != pb.WorkflowState_WORKFLOW_STATE_SUCCEEDED {
		t.Fatalf("expected workflow state SUCCEEDED, got: %s", wf.GetStatus().GetState())
	}

	// 8. Assert Exactly-One committed result invariant in store
	task, _, err := s.GetTask(ctx, "wf-midtask-kill", "critical-task")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if task.GetStatus().GetState() != pb.TaskState_TASK_STATE_COMPLETED {
		t.Fatalf("expected task state COMPLETED, got: %s", task.GetStatus().GetState())
	}
	if task.GetStatus().GetAssignedWorker() != "worker-survivor" {
		t.Fatalf("expected assigned worker 'worker-survivor', got: %s", task.GetStatus().GetAssignedWorker())
	}

	// 9. Inspect evidence logs:
	// Worker 1 has attempt 1 START (never FINISH SUCCESS)
	w1Recs, _ := w1Reader.ReadAll()
	for _, r := range w1Recs {
		if r.Event == evidence.EventFinish && r.Status == "SUCCESS" {
			t.Fatal("worker 1 should not have recorded a successful FINISH event")
		}
	}
}

// Test: Leader failover and automatic worker reconnection
func TestWorker_LeaderFailoverAndReconnect(t *testing.T) {
	_, client, cleanupEtcd := startTestEtcd(t)
	defer cleanupEtcd()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	electionPrefix := fmt.Sprintf("/orchestra/leader/election/failover-%d", time.Now().UnixNano())

	// Start Leader 1
	engine1, s, grpcAddr1, cleanupOrch1 := setupOrchestratorServer(t, client, ctx, electionPrefix)

	tmpDir := filepath.Join(os.TempDir(), fmt.Sprintf("worker-failover-%d", time.Now().UnixNano()))
	_ = os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	wLogPath := filepath.Join(tmpDir, "evidence-reconnect.jsonl")

	// Start worker connecting via leader discovery
	wCtx, cancelW := context.WithCancel(ctx)
	defer cancelW()

	w, err := worker.NewWorker(worker.WorkerConfig{
		WorkerID:                "worker-reconnect",
		Capacity:                2,
		EtcdClient:              client,
		ElectionPrefix:          electionPrefix,
		DefaultOrchestratorAddr: grpcAddr1,
		EvidenceLogPath:         wLogPath,
	})
	if err != nil {
		t.Fatalf("failed to create worker: %v", err)
	}
	go func() {
		_ = w.Start(wCtx)
	}()

	time.Sleep(500 * time.Millisecond)

	// Submit task on Leader 1 and execute
	_, err = engine1.SubmitWorkflow(ctx, &pb.SubmitWorkflowRequest{
		Workflow: &pb.WorkflowSpec{
			Id: "wf-leader-1",
			Tasks: []*pb.TaskSpec{
				{Id: "task-1", Type: "echo", Payload: "before failover"},
			},
		},
	})
	if err != nil {
		t.Fatalf("SubmitWorkflow on leader 1 failed: %v", err)
	}

	// Wait for task-1 to complete
	time.Sleep(400 * time.Millisecond)
	task1, _, err := s.GetTask(ctx, "wf-leader-1", "task-1")
	if err != nil || task1.GetStatus().GetState() != pb.TaskState_TASK_STATE_COMPLETED {
		t.Fatalf("task-1 did not complete on leader 1: state=%v, err=%v", task1.GetStatus().GetState(), err)
	}

	// Shut down Leader 1
	cleanupOrch1()

	// Start Leader 2 on a new port with the same election prefix!
	engine2, _, grpcAddr2, cleanupOrch2 := setupOrchestratorServer(t, client, ctx, electionPrefix)
	defer cleanupOrch2()

	// Worker should detect new leader via etcd watch and reconnect to grpcAddr2
	time.Sleep(1 * time.Second)

	// Submit task on Leader 2
	_, err = engine2.SubmitWorkflow(ctx, &pb.SubmitWorkflowRequest{
		Workflow: &pb.WorkflowSpec{
			Id: "wf-leader-2",
			Tasks: []*pb.TaskSpec{
				{Id: "task-2", Type: "echo", Payload: "after failover"},
			},
		},
	})
	if err != nil {
		t.Fatalf("SubmitWorkflow on leader 2 failed: %v", err)
	}

	// Wait for task-2 to be received and completed by the reconnected worker
	var task2Completed bool
	for i := 0; i < 40; i++ {
		t2, _, err := s.GetTask(ctx, "wf-leader-2", "task-2")
		if err == nil && t2.GetStatus().GetState() == pb.TaskState_TASK_STATE_COMPLETED {
			task2Completed = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !task2Completed {
		t.Fatalf("worker failed to reconnect and execute task on leader 2 (addr: %s)", grpcAddr2)
	}
}

// Test: Graceful shutdown on Stop()
func TestWorker_GracefulShutdown(t *testing.T) {
	_, client, cleanupEtcd := startTestEtcd(t)
	defer cleanupEtcd()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	electionPrefix := fmt.Sprintf("/orchestra/leader/election/grace-%d", time.Now().UnixNano())
	engine, _, grpcAddr, cleanupOrch := setupOrchestratorServer(t, client, ctx, electionPrefix)
	defer cleanupOrch()

	tmpDir := filepath.Join(os.TempDir(), fmt.Sprintf("worker-grace-%d", time.Now().UnixNano()))
	_ = os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	wLogPath := filepath.Join(tmpDir, "evidence-grace.jsonl")

	wCtx, cancelW := context.WithCancel(ctx)
	w, err := worker.NewWorker(worker.WorkerConfig{
		WorkerID:                "worker-grace",
		Capacity:                2,
		EtcdClient:              client,
		ElectionPrefix:          electionPrefix,
		DefaultOrchestratorAddr: grpcAddr,
		EvidenceLogPath:         wLogPath,
	})
	if err != nil {
		t.Fatalf("failed to create worker: %v", err)
	}

	go func() {
		_ = w.Start(wCtx)
	}()

	time.Sleep(500 * time.Millisecond)

	// Submit a task with 200ms duration
	_, _ = engine.SubmitWorkflow(ctx, &pb.SubmitWorkflowRequest{
		Workflow: &pb.WorkflowSpec{
			Id: "wf-grace",
			Tasks: []*pb.TaskSpec{
				{Id: "grace-task", Type: "sleep", Payload: "100ms", TimeoutSeconds: 5},
			},
		},
	})

	time.Sleep(20 * time.Millisecond)

	// Initiate graceful stop
	cancelW()
	if err := w.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Verify etcd lease was revoked upon clean stop
	leaseID := w.LeaseID()
	if leaseID != 0 {
		ttlResp, err := client.TimeToLive(context.Background(), leaseID)
		if err == nil && ttlResp.TTL > 0 {
			t.Fatalf("expected worker lease to be revoked, but TTL is %d", ttlResp.TTL)
		}
	}
}
