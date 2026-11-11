package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"orchestra/internal/worker"
)

func main() {
	var (
		nodeID           = flag.String("node-id", "worker-1", "Unique identifier for this worker node")
		etcdEndpoints    = flag.String("etcd-endpoints", "localhost:2379", "Comma-separated list of etcd endpoints")
		orchestratorAddr = flag.String("orchestrator-addr", "localhost:50051", "Default target address for orchestrator gRPC service")
		capacity         = flag.Int("capacity", 4, "Maximum concurrent tasks this worker can execute")
		evidenceLogPath  = flag.String("evidence-log", filepath.Join("results", "evidence.jsonl"), "Path to append-only evidence log")
		httpPort         = flag.Int("http-port", 8081, "Port for HTTP /metrics, /healthz, and /readyz endpoints")
	)
	flag.Parse()

	// Invariant protected: Ensure unique worker identity across scaled container replicas
	workerID := *nodeID
	if envID := os.Getenv("WORKER_ID"); envID != "" {
		workerID = envID
	} else if workerID == "worker-1" || workerID == "" {
		if host, err := os.Hostname(); err == nil && host != "" {
			workerID = "worker-" + host
		}
	}

	evidencePath := *evidenceLogPath
	if envEvidence := os.Getenv("EVIDENCE_LOG"); envEvidence != "" {
		evidencePath = envEvidence
	}
	// If evidencePath points to a directory (such as /results in a shared volume), write evidence-<workerID>.jsonl inside it
	if fi, err := os.Stat(evidencePath); err == nil && fi.IsDir() {
		evidencePath = filepath.Join(evidencePath, fmt.Sprintf("evidence-%s.jsonl", workerID))
	} else if strings.HasSuffix(evidencePath, "/") || strings.HasSuffix(evidencePath, "\\") {
		evidencePath = filepath.Join(evidencePath, fmt.Sprintf("evidence-%s.jsonl", workerID))
	} else if filepath.Base(evidencePath) == "evidence.jsonl" && workerID != "worker-1" {
		evidencePath = filepath.Join(filepath.Dir(evidencePath), fmt.Sprintf("evidence-%s.jsonl", workerID))
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})).With(
		slog.String("component", "worker"),
		slog.String("worker_id", workerID),
	)
	slog.SetDefault(logger)

	endpoints := strings.Split(*etcdEndpoints, ",")
	logger.Info("starting worker daemon",
		slog.String("worker_id", workerID),
		slog.Any("etcd_endpoints", endpoints),
		slog.String("orchestrator_addr", *orchestratorAddr),
		slog.Int("capacity", *capacity),
		slog.String("evidence_log", evidencePath),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 1. Connect to etcd cluster
	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		logger.Error("failed to connect to etcd", slog.Any("error", err))
		os.Exit(1)
	}
	defer etcdClient.Close()

	// 2. Initialize worker instance with leader discovery and bounded execution pool
	w, err := worker.NewWorker(worker.WorkerConfig{
		WorkerID:                workerID,
		Capacity:                *capacity,
		EtcdClient:              etcdClient,
		DefaultOrchestratorAddr: *orchestratorAddr,
		EvidenceLogPath:         evidencePath,
		HTTPPort:                *httpPort,
		Logger:                  logger,
		SupportedTypes:          []string{"echo", "sleep", "cpu", "flaky", "slow_then_crash", "mock", "bash", "cmd"},
	})
	if err != nil {
		logger.Error("failed to initialize worker", slog.Any("error", err))
		os.Exit(1)
	}

	// 3. Start worker in background
	workerErrCh := make(chan error, 1)
	go func() {
		workerErrCh <- w.Start(ctx)
	}()

	logger.Info("worker fully operational and awaiting task dispatch")

	// 4. Block until shutdown signal received or fatal error occurs
	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received, performing graceful teardown...")
		_ = w.Stop()
	case err := <-workerErrCh:
		if err != nil && err != context.Canceled {
			logger.Error("worker terminated with error", slog.Any("error", err))
		}
	}

	logger.Info("worker daemon stopped cleanly")
}
