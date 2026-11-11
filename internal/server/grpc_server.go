package server

import (
	"fmt"
	"log/slog"
	"net"

	"google.golang.org/grpc"
	pb "orchestra/api/proto/orchestra/v1"
	"orchestra/internal/scheduler"
)

// GRPCServer manages the gRPC network listener and service registration.
type GRPCServer struct {
	server *grpc.Server
	engine *scheduler.Engine
	port   int
	logger *slog.Logger
}

// NewGRPCServer initializes a gRPC server bound to the scheduling engine.
func NewGRPCServer(engine *scheduler.Engine, port int, logger *slog.Logger) *GRPCServer {
	if logger == nil {
		logger = slog.Default()
	}
	s := grpc.NewServer()
	pb.RegisterOrchestratorServiceServer(s, engine)
	pb.RegisterWorkerServiceServer(s, engine)

	return &GRPCServer{
		server: s,
		engine: engine,
		port:   port,
		logger: logger.With(slog.String("component", "grpc-server"), slog.Int("port", port)),
	}
}

// Start begins listening and serving gRPC requests on the configured port.
func (s *GRPCServer) Start() error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", s.port, err)
	}

	s.logger.Info("gRPC server listening", slog.String("addr", lis.Addr().String()))
	return s.server.Serve(lis)
}

// Stop gracefully terminates the gRPC server.
func (s *GRPCServer) Stop() {
	s.logger.Info("stopping gRPC server gracefully...")
	s.server.GracefulStop()
}
