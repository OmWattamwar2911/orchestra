package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	pb "orchestra/api/proto/orchestra/v1"
	"orchestra/internal/election"
	"orchestra/internal/evidence"
	"orchestra/internal/metrics"
	"orchestra/internal/store"
)

// WorkerConfig defines operational parameters for a Worker daemon.
type WorkerConfig struct {
	WorkerID                string
	Capacity                int
	EtcdClient              *clientv3.Client
	ElectionPrefix          string
	DefaultOrchestratorAddr string
	EvidenceLogPath         string
	HTTPPort                int
	Logger                  *slog.Logger
	SupportedTypes          []string
}

// Worker executes workflow tasks assigned by the orchestrator cluster.
// Delivery semantics: At-least-once execution with exactly-one committed result per task.
type Worker struct {
	cfg            WorkerConfig
	etcdClient     *clientv3.Client
	discovery      *LeaderDiscovery
	executor       *Executor
	evidenceLogger *evidence.Logger
	metricsServer  *metrics.HTTPServer
	logger         *slog.Logger

	// Lease management
	leaseID        clientv3.LeaseID
	keepAliveStop  context.CancelFunc
	keepAliveDone  chan struct{}

	// Bounded worker pool & graceful shutdown
	semaphore      chan struct{}
	inFlightWg     sync.WaitGroup
	isShuttingDown atomic.Bool

	// gRPC connection to current cluster leader
	// mu protects connection lifecycle, client instantiation, and active leader metadata
	// Invariant protected: Synchronizes atomic reconnection between leader failovers
	mu             sync.Mutex
	conn           *grpc.ClientConn
	grpcClient     pb.WorkerServiceClient
	activeLeader   *election.LeaderInfo
}

// NewWorker initializes a worker daemon.
func NewWorker(cfg WorkerConfig) (*Worker, error) {
	if cfg.WorkerID == "" {
		return nil, errors.New("worker ID cannot be empty")
	}
	if cfg.Capacity <= 0 {
		cfg.Capacity = 4
	}
	if cfg.EtcdClient == nil {
		return nil, errors.New("etcd client cannot be nil")
	}
	if cfg.EvidenceLogPath == "" {
		cfg.EvidenceLogPath = filepath.Join("results", fmt.Sprintf("evidence-%s.jsonl", cfg.WorkerID))
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With(slog.String("component", "worker"), slog.String("worker_id", cfg.WorkerID))

	evLogger, err := evidence.NewLogger(cfg.EvidenceLogPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize evidence log at %s: %w", cfg.EvidenceLogPath, err)
	}

	exec := NewExecutor(cfg.WorkerID, cfg.Capacity, evLogger, logger)
	disc := NewLeaderDiscovery(cfg.EtcdClient, cfg.ElectionPrefix, logger)

	return &Worker{
		cfg:            cfg,
		etcdClient:     cfg.EtcdClient,
		discovery:      disc,
		executor:       exec,
		evidenceLogger: evLogger,
		logger:         logger,
		semaphore:      make(chan struct{}, cfg.Capacity),
		keepAliveDone:  make(chan struct{}),
	}, nil
}

// Executor returns the underlying task executor.
func (w *Worker) Executor() *Executor {
	return w.executor
}

// LeaseID returns the current etcd lease ID for this worker.
func (w *Worker) LeaseID() clientv3.LeaseID {
	return w.leaseID
}

// Start begins worker lifecycle: establishes etcd liveness lease, discovers leader, and streams tasks.
func (w *Worker) Start(ctx context.Context) error {
	// 1. Create etcd lease (TTL 5s) under /orchestra/workers/{id} and start keepalive
	if err := w.registerLease(ctx); err != nil {
		return fmt.Errorf("failed to register etcd worker lease: %w", err)
	}

	w.logger.Info("registered worker lease in etcd",
		slog.Int64("lease_id", int64(w.leaseID)),
		slog.Int("capacity", w.cfg.Capacity),
	)

	// 2. Start HTTP metrics, healthz, and readyz server
	if w.cfg.HTTPPort >= 0 {
		readyCheck := func(rCtx context.Context) error {
			if w.leaseID == 0 {
				return errors.New("worker lease not active")
			}
			endpoints := w.etcdClient.Endpoints()
			if len(endpoints) > 0 {
				_, err := w.etcdClient.Status(rCtx, endpoints[0])
				return err
			}
			return nil
		}
		srv, err := metrics.StartHTTPServer(fmt.Sprintf(":%d", w.cfg.HTTPPort), "worker", readyCheck, w.logger)
		if err == nil {
			w.metricsServer = srv
		}
	}

	// 3. Start leader supervision and task processing loop
	return w.runLeaderSupervisionLoop(ctx)
}

// registerLease creates a 5-second lease in etcd and attaches worker metadata.
func (w *Worker) registerLease(ctx context.Context) error {
	leaseResp, err := w.etcdClient.Grant(ctx, 5)
	if err != nil {
		return fmt.Errorf("grant etcd lease failed: %w", err)
	}
	w.leaseID = leaseResp.ID

	metadata, err := json.Marshal(map[string]any{
		"worker_id":       w.cfg.WorkerID,
		"capacity":        w.cfg.Capacity,
		"supported_types": w.cfg.SupportedTypes,
		"started_at":      time.Now().UnixMilli(),
	})
	if err != nil {
		return fmt.Errorf("marshal worker metadata failed: %w", err)
	}

	workerKey := store.WorkerKey(w.cfg.WorkerID)
	// Invariant protected: Attaching worker metadata to a 5-second lease ensures immediate
	// task re-queueing by the scheduler if this worker node crashes or disconnects.
	if _, err := w.etcdClient.Put(ctx, workerKey, string(metadata), clientv3.WithLease(leaseResp.ID)); err != nil {
		return fmt.Errorf("put worker metadata failed: %w", err)
	}

	keepAliveCtx, cancelKeepAlive := context.WithCancel(context.Background())
	w.keepAliveStop = cancelKeepAlive

	keepAliveCh, err := w.etcdClient.KeepAlive(keepAliveCtx, leaseResp.ID)
	if err != nil {
		cancelKeepAlive()
		return fmt.Errorf("start keepalive failed: %w", err)
	}

	go func() {
		defer close(w.keepAliveDone)
		for range keepAliveCh {
			// Drain keepalive responses
		}
	}()

	return nil
}

// runLeaderSupervisionLoop dynamically discovers the leader, watches for leadership changes, and maintains task streaming.
func (w *Worker) runLeaderSupervisionLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if w.isShuttingDown.Load() {
			return nil
		}

		w.logger.Info("discovering active orchestrator leader from etcd...")
		leader, _, err := w.discovery.WaitForLeader(ctx, 500*time.Millisecond)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			w.logger.Warn("waiting for leader election...", slog.Any("error", err))
			time.Sleep(1 * time.Second)
			continue
		}

		targetAddr := leader.GrpcAddr
		if targetAddr == "" {
			targetAddr = w.cfg.DefaultOrchestratorAddr
		}
		if targetAddr == "" {
			targetAddr = "127.0.0.1:50051"
		}

		w.logger.Info("connecting to leader orchestrator",
			slog.String("leader_id", leader.NodeID),
			slog.String("target_addr", targetAddr),
		)

		if err := w.connectAndStream(ctx, leader, targetAddr); err != nil {
			if ctx.Err() != nil || w.isShuttingDown.Load() {
				return ctx.Err()
			}
			w.logger.Warn("leader connection or task stream interrupted, reconnecting",
				slog.String("leader_id", leader.NodeID),
				slog.Any("error", err),
			)
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// connectAndStream connects to the specified leader, registers capacity, and streams tasks until disconnect.
func (w *Worker) connectAndStream(ctx context.Context, leader *election.LeaderInfo, targetAddr string) error {
	dialCtx, dialCancel := context.WithTimeout(ctx, 5*time.Second)
	defer dialCancel()

	conn, err := grpc.DialContext(dialCtx, targetAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("failed to dial orchestrator leader at %s: %w", targetAddr, err)
	}

	client := pb.NewWorkerServiceClient(conn)

	// Invariant protected: Lock protects atomic swap of active connection and client handles
	w.mu.Lock()
	if w.conn != nil {
		_ = w.conn.Close()
	}
	w.conn = conn
	w.grpcClient = client
	w.activeLeader = leader
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		if w.conn == conn {
			_ = w.conn.Close()
			w.conn = nil
			w.grpcClient = nil
		}
		w.mu.Unlock()
	}()

	// Register with leader over gRPC
	supported := w.cfg.SupportedTypes
	if len(supported) == 0 {
		supported = []string{"echo", "sleep", "cpu", "flaky", "slow_then_crash", "mock", "bash", "cmd"}
	}

	regCtx, regCancel := context.WithTimeout(ctx, 5*time.Second)
	defer regCancel()

	regResp, err := client.RegisterWorker(regCtx, &pb.RegisterWorkerRequest{
		WorkerId:       w.cfg.WorkerID,
		Capacity:       int32(w.cfg.Capacity),
		SupportedTypes: supported,
	})
	if err != nil {
		return fmt.Errorf("failed to register worker with leader: %w", err)
	}

	w.logger.Info("successfully registered with leader orchestrator",
		slog.String("leader_id", regResp.GetLeaderId()),
		slog.Int("heartbeat_interval", int(regResp.GetHeartbeatIntervalSeconds())),
	)

	// Open task stream from leader
	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()

	// Watch leader key concurrently: if leader changes in etcd, cancel stream to trigger reconnect immediately
	leaderWatcher := w.discovery.WatchLeader(streamCtx)
	go func() {
		for newLeader := range leaderWatcher {
			if newLeader.NodeID != leader.NodeID || newLeader.GrpcAddr != leader.GrpcAddr {
				w.logger.Info("detected leader migration in etcd, reconnecting to new leader",
					slog.String("old_leader", leader.NodeID),
					slog.String("new_leader", newLeader.NodeID),
				)
				streamCancel()
				return
			}
		}
	}()

	stream, err := client.StreamTasks(streamCtx, &pb.StreamTasksRequest{
		WorkerId: w.cfg.WorkerID,
	})
	if err != nil {
		return fmt.Errorf("failed to open StreamTasks: %w", err)
	}

	// Process task assignments from leader
	for {
		assignment, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) || streamCtx.Err() != nil {
				return nil
			}
			return fmt.Errorf("error reading task assignment from stream: %w", err)
		}

		if w.isShuttingDown.Load() {
			return nil
		}

		// Invariant protected: Semaphore bounds execution concurrency to advertised capacity
		select {
		case w.semaphore <- struct{}{}:
		case <-streamCtx.Done():
			return streamCtx.Err()
		}

		w.inFlightWg.Add(1)

		go func(asgn *pb.TaskAssignment) {
			defer func() {
				<-w.semaphore
				w.inFlightWg.Done()
			}()

			w.executeAndReport(ctx, client, asgn)
		}(assignment)
	}
}

// executeAndReport executes the task and sends the outcome to the scheduler leader.
// Delivery semantics: At-least-once execution with exactly-one committed result per task.
func (w *Worker) executeAndReport(ctx context.Context, client pb.WorkerServiceClient, asgn *pb.TaskAssignment) {
	// Execute task in bounded pool with timeout and independent evidence logging
	resultReq := w.executor.ExecuteTask(ctx, asgn)

	// Report result to orchestrator leader
	reportCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.ReportResult(reportCtx, resultReq)
	if err != nil {
		w.mu.Lock()
		curClient := w.grpcClient
		w.mu.Unlock()
		if curClient != nil && curClient != client {
			// Retry once against newly elected leader
			retryCtx, retryCancel := context.WithTimeout(context.Background(), 5*time.Second)
			resp, err = curClient.ReportResult(retryCtx, resultReq)
			retryCancel()
		}
	}
	if err != nil {
		w.logger.Error("failed to report task result to leader",
			slog.String("workflow_id", asgn.GetWorkflowId()),
			slog.String("task_id", asgn.GetTask().GetId()),
			slog.Int("attempt", int(asgn.GetAttempt())),
			slog.Any("error", err),
		)
		return
	}

	if !resp.GetCommitted() {
		// Handled gracefully: "rejected: stale attempt" or terminal task
		w.logger.Warn("task result discarded (uncommitted by scheduler)",
			slog.String("workflow_id", asgn.GetWorkflowId()),
			slog.String("task_id", asgn.GetTask().GetId()),
			slog.Int("attempt", int(asgn.GetAttempt())),
			slog.String("reason", resp.GetMessage()),
		)
		return
	}

	w.logger.Info("task result committed successfully by leader",
		slog.String("workflow_id", asgn.GetWorkflowId()),
		slog.String("task_id", asgn.GetTask().GetId()),
		slog.Int("attempt", int(asgn.GetAttempt())),
		slog.String("message", resp.GetMessage()),
	)
}

// Stop performs graceful shutdown on SIGTERM / SIGINT: stops accepting tasks, waits for in-flight tasks, and revokes lease.
func (w *Worker) Stop() error {
	w.logger.Info("initiating graceful worker shutdown...")
	w.isShuttingDown.Store(true)

	// 1. Close gRPC stream/connection so no new tasks are dispatched
	w.mu.Lock()
	if w.conn != nil {
		_ = w.conn.Close()
		w.conn = nil
	}
	w.mu.Unlock()

	// 2. Wait for in-flight tasks to finish within grace period
	done := make(chan struct{})
	go func() {
		w.inFlightWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		w.logger.Info("all in-flight tasks finished gracefully")
	case <-time.After(5 * time.Second):
		w.logger.Warn("grace period expired, remaining tasks will be re-queued by scheduler on lease revocation")
	}

	// 3. Stop keepalive and revoke etcd lease
	if w.keepAliveStop != nil {
		w.keepAliveStop()
	}
	<-w.keepAliveDone

	if w.leaseID != 0 {
		revokeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = w.etcdClient.Revoke(revokeCtx, w.leaseID)
		w.logger.Info("etcd worker lease revoked")
	}

	// 4. Stop metrics HTTP server
	if w.metricsServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = w.metricsServer.Stop(shutdownCtx)
	}

	// 5. Close evidence logger
	if w.evidenceLogger != nil {
		_ = w.evidenceLogger.Close()
	}

	w.logger.Info("worker shutdown complete")
	return nil
}
