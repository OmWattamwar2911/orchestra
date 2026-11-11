package metrics

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var processStartTime = time.Now()

// ReadyCheckFunc defines a readiness inspection hook (e.g. checking etcd connectivity or lease liveness).
type ReadyCheckFunc func(ctx context.Context) error

// NewHandler creates an http.Handler with /metrics, /healthz, and /readyz endpoints.
func NewHandler(processName string, readyCheck ReadyCheckFunc) http.Handler {
	mux := http.NewServeMux()

	// 1. Prometheus metrics scrape endpoint
	mux.Handle("GET /metrics", promhttp.Handler())

	// 2. Health check (liveness probe)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":         "ok",
			"process":        processName,
			"uptime_seconds": time.Since(processStartTime).Seconds(),
			"timestamp":      time.Now().UnixMilli(),
		})
	})

	// 3. Readiness probe
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if readyCheck != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			if err := readyCheck(ctx); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status":  "not ready",
					"process": processName,
					"error":   err.Error(),
				})
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "ready",
			"process": processName,
		})
	})

	return mux
}

// HTTPServer manages the background HTTP metrics and health check server for a daemon process.
type HTTPServer struct {
	server *http.Server
	logger *slog.Logger
}

// StartHTTPServer starts the metrics and health HTTP server on the specified address.
func StartHTTPServer(addr string, processName string, readyCheck ReadyCheckFunc, logger *slog.Logger) (*HTTPServer, error) {
	if logger == nil {
		logger = slog.Default()
	}

	handler := NewHandler(processName, readyCheck)
	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	hs := &HTTPServer{
		server: srv,
		logger: logger.With(slog.String("component", "metrics-server"), slog.String("addr", addr)),
	}

	go func() {
		hs.logger.Info("starting metrics and health HTTP service", slog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			hs.logger.Error("metrics HTTP server exited with error", slog.Any("error", err))
		}
	}()

	return hs, nil
}

// Stop shuts down the metrics and health HTTP server.
func (s *HTTPServer) Stop(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}
