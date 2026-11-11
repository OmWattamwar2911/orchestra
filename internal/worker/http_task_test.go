package worker_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pb "orchestra/api/proto/orchestra/v1"
	"orchestra/internal/worker"
)

func TestHTTPTask_SuccessGET(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","count":42}`))
	}))
	defer ts.Close()

	payloadJSON, _ := json.Marshal(worker.HTTPTaskPayload{
		URL:            ts.URL,
		Method:         "GET",
		ExpectedStatus: 200,
		AllowPrivate:   true, // httptest.Server binds to 127.0.0.1 loopback
	})

	output, outcome, errMsg := worker.ExecuteHTTPTask(context.Background(), string(payloadJSON))
	if outcome != pb.TaskOutcome_TASK_OUTCOME_SUCCESS {
		t.Fatalf("expected SUCCESS, got %s (err: %s)", outcome, errMsg)
	}
	if !strings.Contains(string(output), `"status":"ok"`) {
		t.Fatalf("unexpected output: %s", string(output))
	}
}

func TestHTTPTask_SuccessPOST_WithHeadersAndBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("X-Custom-Auth") != "secret-token" {
			t.Errorf("missing or invalid header: %s", r.Header.Get("X-Custom-Auth"))
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"action":"start"}` {
			t.Errorf("unexpected body: %s", string(body))
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"created":true}`))
	}))
	defer ts.Close()

	payloadJSON, _ := json.Marshal(worker.HTTPTaskPayload{
		URL:            ts.URL,
		Method:         "POST",
		Headers:        map[string]string{"X-Custom-Auth": "secret-token"},
		Body:           `{"action":"start"}`,
		ExpectedStatus: 201,
		AllowPrivate:   true,
	})

	output, outcome, errMsg := worker.ExecuteHTTPTask(context.Background(), string(payloadJSON))
	if outcome != pb.TaskOutcome_TASK_OUTCOME_SUCCESS {
		t.Fatalf("expected SUCCESS, got %s (err: %s)", outcome, errMsg)
	}
	if !strings.Contains(string(output), `"created":true`) {
		t.Fatalf("unexpected output: %s", string(output))
	}
}

func TestHTTPTask_StatusMismatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("resource not found"))
	}))
	defer ts.Close()

	payloadJSON, _ := json.Marshal(worker.HTTPTaskPayload{
		URL:            ts.URL,
		Method:         "GET",
		ExpectedStatus: 200,
		AllowPrivate:   true,
	})

	_, outcome, errMsg := worker.ExecuteHTTPTask(context.Background(), string(payloadJSON))
	if outcome != pb.TaskOutcome_TASK_OUTCOME_FAILURE {
		t.Fatalf("expected FAILURE for status mismatch, got %s", outcome)
	}
	if !strings.Contains(errMsg, "expected 200, got 404") {
		t.Fatalf("expected status mismatch error message, got: %s", errMsg)
	}
}

func TestHTTPTask_ResponseTooLarge(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Repeat("X", 1024)))
	}))
	defer ts.Close()

	payloadJSON, _ := json.Marshal(worker.HTTPTaskPayload{
		URL:              ts.URL,
		Method:           "GET",
		MaxResponseBytes: 512, // smaller than response
		AllowPrivate:     true,
	})

	_, outcome, errMsg := worker.ExecuteHTTPTask(context.Background(), string(payloadJSON))
	if outcome != pb.TaskOutcome_TASK_OUTCOME_FAILURE {
		t.Fatalf("expected FAILURE for oversized response, got %s", outcome)
	}
	if !strings.Contains(errMsg, "exceeded maximum allowed size") {
		t.Fatalf("expected response too large error, got: %s", errMsg)
	}
}

func TestHTTPTask_SSRF_BlockedPrivateIPs(t *testing.T) {
	restrictedURLs := []string{
		"http://127.0.0.1:8080/metrics",
		"http://localhost:8080/admin",
		"http://10.0.0.1/secret",
		"http://172.16.0.5/api",
		"http://192.168.1.1/setup",
		"http://169.254.169.254/latest/meta-data",
	}

	for _, target := range restrictedURLs {
		payloadJSON, _ := json.Marshal(worker.HTTPTaskPayload{
			URL:          target,
			Method:       "GET",
			AllowPrivate: false, // SSRF guard ACTIVE
		})

		_, outcome, errMsg := worker.ExecuteHTTPTask(context.Background(), string(payloadJSON))
		if outcome != pb.TaskOutcome_TASK_OUTCOME_FAILURE {
			t.Errorf("target %s should have been blocked by SSRF guard, but got %s", target, outcome)
		}
		if !strings.Contains(errMsg, "SSRF") {
			t.Errorf("target %s expected SSRF block error message, got: %s", target, errMsg)
		}
	}
}

func TestHTTPTask_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	payloadJSON, _ := json.Marshal(worker.HTTPTaskPayload{
		URL:          ts.URL,
		Method:       "GET",
		AllowPrivate: true,
	})

	_, outcome, errMsg := worker.ExecuteHTTPTask(ctx, string(payloadJSON))
	if outcome != pb.TaskOutcome_TASK_OUTCOME_TIMEOUT {
		t.Fatalf("expected TIMEOUT, got %s (err: %s)", outcome, errMsg)
	}
}

func TestHTTPTask_PlainURLPayload(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok-plain"))
	}))
	defer ts.Close()

	// When using plain URL without allow_private flag, loopback will be blocked by SSRF
	_, outcome, errMsg := worker.ExecuteHTTPTask(context.Background(), ts.URL)
	if outcome != pb.TaskOutcome_TASK_OUTCOME_FAILURE {
		t.Fatalf("expected SSRF block for plain loopback URL, got %s", outcome)
	}
	if !strings.Contains(errMsg, "SSRF") {
		t.Fatalf("expected SSRF error, got: %s", errMsg)
	}
}
