package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"orchestra/internal/server"
)

func TestHTTPGateway_HealthAndMetrics(t *testing.T) {
	gw := server.NewHTTPGateway("orchestrator-test", 0, nil, nil, nil, nil)
	ts := httptest.NewServer(gw.Handler())
	defer ts.Close()

	// 1. Test /healthz
	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200 from /healthz, got %d", resp.StatusCode)
	}

	var healthzData map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&healthzData); err != nil {
		t.Fatalf("failed to decode /healthz JSON: %v", err)
	}
	if healthzData["status"] != "ok" || healthzData["process"] != "orchestrator" {
		t.Errorf("unexpected /healthz data: %v", healthzData)
	}

	// 2. Test /readyz (no etcd store -> ready by default)
	respReady, err := http.Get(ts.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz failed: %v", err)
	}
	defer respReady.Body.Close()

	if respReady.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200 from /readyz, got %d", respReady.StatusCode)
	}

	var readyzData map[string]any
	if err := json.NewDecoder(respReady.Body).Decode(&readyzData); err != nil {
		t.Fatalf("failed to decode /readyz JSON: %v", err)
	}
	if readyzData["status"] != "ready" {
		t.Errorf("unexpected /readyz status: %v", readyzData["status"])
	}

	// 3. Test /metrics
	respMetrics, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics failed: %v", err)
	}
	defer respMetrics.Body.Close()

	if respMetrics.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200 from /metrics, got %d", respMetrics.StatusCode)
	}

	// 4. Test CORS preflight OPTIONS
	req, _ := http.NewRequest("OPTIONS", ts.URL+"/api/workflows", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	respOpt, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS preflight failed: %v", err)
	}
	defer respOpt.Body.Close()

	if respOpt.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for CORS preflight, got %d", respOpt.StatusCode)
	}
	if respOpt.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("expected Access-Control-Allow-Origin: *")
	}
}
