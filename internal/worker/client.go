package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	pb "orchestra/api/proto/orchestra/v1"
)

// Client handles gRPC connectivity between a worker node and the orchestrator cluster.
type Client struct {
	// mu protects gRPC connection lifecycle against concurrent reconnects
	// Invariant protected: Ensures only one active gRPC connection and stream loop exist at a time
	mu               sync.Mutex
	workerID         string
	orchestratorAddr string
	capacity         int
	conn             *grpc.ClientConn
	grpcClient       pb.WorkerServiceClient
	executor         *Executor
	logger           *slog.Logger
}

// NewClient creates a new gRPC worker client.
func NewClient(workerID, orchestratorAddr string, capacity int, executor *Executor, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		workerID:         workerID,
		orchestratorAddr: orchestratorAddr,
		capacity:         capacity,
		executor:         executor,
		logger:           logger.With(slog.String("component", "worker-client"), slog.String("worker_id", workerID)),
	}
}

// Connect dials the orchestrator gRPC service.
func (c *Client) Connect(ctx context.Context) error {
	// Invariant protected: Mutex ensures connection initialization is thread-safe
	c.mu.Lock()
	defer c.mu.Unlock()

	conn, err := grpc.DialContext(ctx, c.orchestratorAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("failed to dial orchestrator at %s: %w", c.orchestratorAddr, err)
	}

	c.conn = conn
	c.grpcClient = pb.NewWorkerServiceClient(conn)
	c.logger.Info("connected to orchestrator gRPC service", slog.String("addr", c.orchestratorAddr))
	return nil
}

// Close terminates the gRPC connection.
func (c *Client) Close() error {
	// Invariant protected: Mutex prevents race between closing connection and in-flight stream operations
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Register registers this worker with the orchestrator.
func (c *Client) Register(ctx context.Context) (*pb.RegisterWorkerResponse, error) {
	req := &pb.RegisterWorkerRequest{
		WorkerId:       c.workerID,
		Capacity:       int32(c.capacity),
		SupportedTypes: []string{"bash", "cmd", "mock"},
	}

	resp, err := c.grpcClient.RegisterWorker(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to register worker: %w", err)
	}

	c.logger.Info("registered with orchestrator",
		slog.Bool("registered", resp.GetRegistered()),
		slog.String("leader_id", resp.GetLeaderId()),
	)
	return resp, nil
}

// StartWorkLoop opens the task dispatch stream from the leader and dispatches incoming tasks to the executor.
func (c *Client) StartWorkLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		c.logger.Info("opening task stream to orchestrator leader...")
		stream, err := c.grpcClient.StreamTasks(ctx, &pb.StreamTasksRequest{
			WorkerId: c.workerID,
		})
		if err != nil {
			c.logger.Warn("failed to open task stream, reconnecting in 2s", slog.Any("error", err))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
				continue
			}
		}

		for {
			assignment, err := stream.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) || ctx.Err() != nil {
					c.logger.Info("task stream closed by server or context")
					break
				}
				c.logger.Warn("error receiving from task stream", slog.Any("error", err))
				break
			}

			// Execute assigned task in background goroutine
			go func(asgn *pb.TaskAssignment) {
				resultReq := c.executor.ExecuteTask(ctx, asgn)

				// Report result back to orchestrator
				reportCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				resp, err := c.grpcClient.ReportResult(reportCtx, resultReq)
				if err != nil {
					c.logger.Error("failed to report task result",
						slog.String("task_id", asgn.GetTask().GetId()),
						slog.Any("error", err),
					)
					return
				}

				c.logger.Info("task result acknowledged by leader",
					slog.String("task_id", asgn.GetTask().GetId()),
					slog.Bool("committed", resp.GetCommitted()),
					slog.String("message", resp.GetMessage()),
				)
			}(assignment)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}
