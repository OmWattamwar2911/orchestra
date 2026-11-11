package metrics_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"orchestra/internal/metrics"
)

func TestMetrics_RegistrationAndEmission(t *testing.T) {
	// Exercise all metrics to confirm registration and operations
	metrics.WorkflowsSubmittedTotal.Inc()
	metrics.WorkflowsSucceededTotal.Inc()
	metrics.WorkflowsFailedTotal.Inc()
	metrics.TasksCompletedTotal.Inc()
	metrics.TasksFailedTotal.Inc()
	metrics.TasksDeadLetterTotal.Inc()
	metrics.StaleResultsRejectedTotal.Inc()
	metrics.FencedWriteRejectionsTotal.Inc()
	metrics.QueueDepth.Set(42)
	metrics.IsLeader.Set(1)
	metrics.LeaderTerm.Set(10)
	metrics.LeaderElectionsTotal.Inc()
	metrics.WorkerCount.Set(5)
	metrics.TaskLatencySeconds.Observe(0.125)
	metrics.WorkflowDurationSeconds.Observe(1.5)
	metrics.TasksRunning.Inc()
	metrics.TasksExecutedTotal.WithLabelValues("success").Inc()

	handler := metrics.NewHandler("test-process", nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 from /metrics, got %d", rec.Code)
	}

	body := rec.Body.String()
	expectedMetrics := []string{
		"workflows_submitted_total",
		"workflows_succeeded_total",
		"workflows_failed_total",
		"tasks_completed_total",
		"tasks_failed_total",
		"tasks_dead_letter_total",
		"stale_results_rejected_total",
		"fenced_write_rejections_total",
		"queue_depth",
		"is_leader",
		"leader_term",
		"leader_elections_total",
		"worker_count",
		"task_latency_seconds",
		"workflow_duration_seconds",
		"tasks_running",
		"tasks_executed_total",
	}

	for _, m := range expectedMetrics {
		if !strings.Contains(body, m) {
			t.Errorf("expected /metrics output to contain metric %q", m)
		}
	}
}

func TestHealthzEndpoint(t *testing.T) {
	handler := metrics.NewHandler("test-node", nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/healthz", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode healthz response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", resp["status"])
	}
	if resp["process"] != "test-node" {
		t.Errorf("expected process 'test-node', got %v", resp["process"])
	}
}

func TestReadyzEndpoint_Healthy(t *testing.T) {
	readyCheck := func(ctx context.Context) error {
		return nil
	}

	handler := metrics.NewHandler("worker-node", readyCheck)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/readyz", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode readyz response: %v", err)
	}

	if resp["status"] != "ready" {
		t.Errorf("expected status 'ready', got %v", resp["status"])
	}
}

func TestReadyzEndpoint_Unhealthy(t *testing.T) {
	readyCheck := func(ctx context.Context) error {
		return errors.New("etcd connection timed out")
	}

	handler := metrics.NewHandler("worker-node", readyCheck)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/readyz", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected HTTP 503, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode readyz response: %v", err)
	}

	if resp["status"] != "not ready" {
		t.Errorf("expected status 'not ready', got %v", resp["status"])
	}
	if !strings.Contains(fmt.Sprint(resp["error"]), "etcd connection timed out") {
		t.Errorf("expected error string to match, got %v", resp["error"])
	}
}

func TestStartHTTPServer_Lifecycle(t *testing.T) {
	port := 19876
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	srv, err := metrics.StartHTTPServer(addr, "test-service", nil, nil)
	if err != nil {
		t.Fatalf("failed to start HTTP server: %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Stop(stopCtx)
	}()

	// Wait briefly for server to bind
	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", port))
	if err != nil {
		t.Fatalf("GET /healthz failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK, got %d: %s", resp.StatusCode, string(body))
	}
}
