package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"orchestra/internal/election"
	"orchestra/internal/scheduler"
	"orchestra/internal/server"
	"orchestra/internal/store"
)

func main() {
	var (
		nodeID            = flag.String("node-id", "orchestrator-1", "Unique identifier for this orchestrator node")
		etcdEndpoints     = flag.String("etcd-endpoints", "localhost:2379", "Comma-separated list of etcd endpoints")
		grpcPort          = flag.Int("grpc-port", 50051, "Port for gRPC service")
		httpPort          = flag.Int("http-port", 8080, "Port for HTTP / WebSocket dashboard gateway")
		grpcAdvertiseAddr = flag.String("grpc-advertise-addr", "", "Advertised gRPC address for workers and clients to dial (e.g. orchestrator1:50051)")
	)
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})).With(
		slog.String("component", "orchestrator"),
		slog.String("node_id", *nodeID),
	)
	slog.SetDefault(logger)

	endpoints := strings.Split(*etcdEndpoints, ",")
	logger.Info("starting orchestrator daemon",
		slog.String("node_id", *nodeID),
		slog.Any("etcd_endpoints", endpoints),
		slog.Int("grpc_port", *grpcPort),
		slog.Int("http_port", *httpPort),
	)

	// Context cancelled upon SIGINT/SIGTERM
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

	// 2. Initialize etcd storage layer
	storeLayer := store.New(etcdClient, logger)

	// 3. Declare engine and candidate pointers for mutual wiring
	var schedEngine *scheduler.Engine

	advAddr := *grpcAdvertiseAddr
	if advAddr == "" {
		if env := os.Getenv("GRPC_ADVERTISE_ADDR"); env != "" {
			advAddr = env
		} else {
			advAddr = fmt.Sprintf("127.0.0.1:%d", *grpcPort)
		}
	}
	// Invariant protected: Campaign value embeds advertised gRPC address so workers can discover active leader
	campaignVal := election.EncodeLeaderInfo(*nodeID, advAddr)

	cand, err := election.NewCandidate(election.Config{
		NodeID:        *nodeID,
		CampaignValue: campaignVal,
		Client:        etcdClient,
		Logger:        logger,
		SessionTTLSec: 5,
		OnElected: func(leaderCtx context.Context, term int64) error {
			if schedEngine != nil {
				return schedEngine.OnElected(leaderCtx, term)
			}
			return nil
		},
		OnRevoked: func() {
			if schedEngine != nil {
				schedEngine.OnRevoked()
			}
		},
	})
	if err != nil {
		logger.Error("failed to initialize leader candidate", slog.Any("error", err))
		os.Exit(1)
	}

	schedEngine = scheduler.NewEngine(scheduler.Config{
		NodeID:     *nodeID,
		Store:      storeLayer,
		Candidate:  cand,
		EtcdClient: etcdClient,
		Logger:     logger,
	})

	// 4. Start gRPC Server
	grpcSrv := server.NewGRPCServer(schedEngine, *grpcPort, logger)
	go func() {
		if err := grpcSrv.Start(); err != nil {
			logger.Error("gRPC server encountered fatal error", slog.Any("error", err))
		}
	}()
	defer grpcSrv.Stop()

	// 5. Start HTTP Gateway for React Dashboard
	httpGw := server.NewHTTPGateway(*nodeID, *httpPort, schedEngine, storeLayer, cand, logger)
	go func() {
		if err := httpGw.Start(); err != nil {
			logger.Error("HTTP gateway encountered fatal error", slog.Any("error", err))
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = httpGw.Stop(shutdownCtx)
	}()

	// 6. Run Leader Election Campaign in background
	go func() {
		_ = cand.Run(ctx)
	}()

	logger.Info("orchestrator fully initialized and participating in cluster election")

	// Block until shutdown signal received
	<-ctx.Done()
	logger.Info("shutting down orchestrator gracefully...")

	// Graceful step down if currently leader
	if cand.IsLeader() {
		resignCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = cand.Resign(resignCtx)
	}

	logger.Info("orchestrator stopped cleanly")
}
