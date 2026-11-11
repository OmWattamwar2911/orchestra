package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "orchestra/api/proto/orchestra/v1"
	"orchestra/internal/dag"
	"orchestra/internal/election"
	"orchestra/internal/scheduler"
	"orchestra/internal/store"
)

// HTTPGateway exposes REST and Server-Sent Event (SSE) streams for the React dashboard.
type HTTPGateway struct {
	nodeID    string
	port      int
	engine    *scheduler.Engine
	store     *store.Store
	candidate *election.Candidate
	logger    *slog.Logger
	server    *http.Server

	// sseMu protects the slice of active in-memory SSE client channels (fallback)
	sseMu      sync.RWMutex
	sseClients map[chan []byte]struct{}
}

// NewHTTPGateway initializes the dashboard HTTP gateway.
func NewHTTPGateway(nodeID string, port int, engine *scheduler.Engine, store *store.Store, candidate *election.Candidate, logger *slog.Logger) *HTTPGateway {
	if logger == nil {
		logger = slog.Default()
	}

	gw := &HTTPGateway{
		nodeID:     nodeID,
		port:       port,
		engine:     engine,
		store:      store,
		candidate:  candidate,
		logger:     logger.With(slog.String("component", "http-gateway"), slog.Int("port", port)),
		sseClients: make(map[chan []byte]struct{}),
	}

	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.HandleFunc("GET /healthz", gw.handleHealthz)
	mux.HandleFunc("GET /readyz", gw.handleReadyz)
	mux.HandleFunc("GET /api/health", gw.handleHealth)
	mux.HandleFunc("GET /api/cluster", gw.handleClusterStatus)
	mux.HandleFunc("GET /api/workflows", gw.handleListWorkflows)
	mux.HandleFunc("GET /api/workflows/", gw.handleGetWorkflow)
	mux.HandleFunc("POST /api/workflows", gw.handleSubmitWorkflow)
	mux.HandleFunc("POST /api/workflows/{id}/cancel", gw.handleCancelWorkflow)
	mux.HandleFunc("POST /api/chaos/stepdown", gw.handleStepdown)
	mux.HandleFunc("POST /api/chaos/kill-worker", gw.handleChaosKillWorker)
	mux.HandleFunc("POST /api/chaos/kill-leader", gw.handleChaosKillLeader)
	mux.HandleFunc("GET /api/events", gw.handleSSEEvents)

	gw.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: corsMiddleware(mux),
	}

	return gw
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Handler returns the underlying http.Handler configured with all routes and CORS middleware.
func (g *HTTPGateway) Handler() http.Handler {
	if g.server != nil {
		return g.server.Handler
	}
	return nil
}

// Start begins serving HTTP traffic.
func (g *HTTPGateway) Start() error {
	g.logger.Info("HTTP gateway listening", slog.String("addr", g.server.Addr))
	if err := g.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Stop gracefully shuts down the HTTP server.
func (g *HTTPGateway) Stop(ctx context.Context) error {
	g.logger.Info("stopping HTTP gateway gracefully...")
	return g.server.Shutdown(ctx)
}

func (g *HTTPGateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":    "UP",
		"node_id":   g.nodeID,
		"is_leader": g.candidate != nil && g.candidate.IsLeader(),
	})
}

func (g *HTTPGateway) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"process": "orchestrator",
		"node_id": g.nodeID,
	})
}

func (g *HTTPGateway) handleReadyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if g.store == nil || g.store.Client() == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "not ready",
			"error":  "etcd client not configured",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	endpoints := g.store.Client().Endpoints()
	if len(endpoints) > 0 {
		_, err := g.store.Client().Status(ctx, endpoints[0])
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "not ready",
				"error":  fmt.Sprintf("etcd endpoint unavailable: %v", err),
			})
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":    "ready",
		"node_id":   g.nodeID,
		"is_leader": g.candidate != nil && g.candidate.IsLeader(),
	})
}

// discoverLeader queries etcd to identify the current active leader and its term.
func (g *HTTPGateway) discoverLeader(ctx context.Context) (*election.LeaderInfo, int64, error) {
	if g.store == nil || g.store.Client() == nil {
		return nil, 0, errors.New("etcd store unavailable")
	}

	opts := []clientv3.OpOption{
		clientv3.WithPrefix(),
		clientv3.WithSort(clientv3.SortByCreateRevision, clientv3.SortAscend),
		clientv3.WithLimit(1),
	}
	resp, err := g.store.Client().Get(ctx, store.ElectionKey, opts...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query leader election: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return nil, 0, errors.New("no active leader key found in etcd")
	}

	info, err := election.ParseLeaderValue(string(resp.Kvs[0].Value))
	if err != nil {
		return nil, 0, err
	}
	return &info, resp.Kvs[0].CreateRevision, nil
}

func (g *HTTPGateway) handleClusterStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var activeWorkers map[string][]byte
	var workflows []*pb.Workflow
	if g.store != nil {
		activeWorkers, _ = g.store.ListActiveWorkers(ctx)
		workflows, _ = g.store.ListWorkflows(ctx)
	}

	workersList := make([]map[string]any, 0, len(activeWorkers))
	for k, v := range activeWorkers {
		var parsed map[string]any
		if err := json.Unmarshal(v, &parsed); err == nil {
			if _, hasID := parsed["worker_id"]; !hasID {
				parsed["worker_id"] = k
			}
			parsed["active"] = true
			workersList = append(workersList, parsed)
		} else {
			workersList = append(workersList, map[string]any{
				"worker_id": k,
				"raw":       string(v),
				"active":    true,
			})
		}
	}

	// Discover leader
	leaderInfo, term, _ := g.discoverLeader(ctx)
	var fencingToken int64
	var leaderKey string
	var electionCount int64
	if g.candidate != nil {
		fencingToken = g.candidate.Term()
		leaderKey = g.candidate.LeaderKey()
		electionCount = g.candidate.ElectionCount()
	}
	if term > fencingToken {
		fencingToken = term
	}

	// Browser-compatible proxy address using incoming host
	reqHost := r.Host
	if reqHost == "" {
		reqHost = fmt.Sprintf("localhost:%d", g.port)
	}

	// Known orchestrator nodes in cluster
	knownNodes := []map[string]any{
		{"node_id": "orchestrator-1", "addr": "orchestrator1:50051", "http_addr": fmt.Sprintf("http://%s", reqHost)},
		{"node_id": "orchestrator-2", "addr": "orchestrator2:50051", "http_addr": fmt.Sprintf("http://%s", reqHost)},
		{"node_id": "orchestrator-3", "addr": "orchestrator3:50051", "http_addr": fmt.Sprintf("http://%s", reqHost)},
	}
	for _, n := range knownNodes {
		id := n["node_id"].(string)
		isLead := leaderInfo != nil && leaderInfo.NodeID == id
		n["is_leader"] = isLead
		if isLead {
			n["role"] = "LEADER"
		} else {
			n["role"] = "STANDBY"
		}
		n["status"] = "UP"
	}

	// Queue depth from engine (or 0 on follower)
	queueDepth := 0
	if g.engine != nil {
		queueDepth = g.engine.QueueDepth()
	}

	// Fetch recent events from etcd
	recentEvents := make([]any, 0)
	if g.store != nil && g.store.Client() != nil {
		opts := []clientv3.OpOption{
			clientv3.WithPrefix(),
			clientv3.WithSort(clientv3.SortByKey, clientv3.SortDescend),
			clientv3.WithLimit(30),
		}
		evResp, err := g.store.Client().Get(ctx, store.PrefixEvents, opts...)
		if err == nil {
			for _, kv := range evResp.Kvs {
				var ev map[string]any
				if err := json.Unmarshal(kv.Value, &ev); err == nil {
					recentEvents = append(recentEvents, ev)
				}
			}
		}
	}

	status := map[string]any{
		"node_id":         g.nodeID,
		"is_leader":       g.candidate != nil && g.candidate.IsLeader(),
		"fencing_token":   fencingToken,
		"leader_key":      leaderKey,
		"leader":          leaderInfo,
		"term":            fencingToken,
		"election_count":  electionCount,
		"nodes":           knownNodes,
		"worker_count":    len(workersList),
		"workers":         workersList,
		"queue_depth":     queueDepth,
		"total_workflows": len(workflows),
		"recent_events":   recentEvents,
		"timestamp":       time.Now().UnixMilli(),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

func (g *HTTPGateway) handleListWorkflows(w http.ResponseWriter, r *http.Request) {
	if g.store == nil {
		http.Error(w, "store unavailable", http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	workflows, err := g.store.ListWorkflows(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type workflowDTO struct {
		Workflow *pb.Workflow `json:"workflow"`
		Tasks    []*pb.Task   `json:"tasks"`
	}

	result := make([]workflowDTO, 0, len(workflows))
	for _, wf := range workflows {
		tasks, _ := g.store.ListTasksForWorkflow(ctx, wf.GetSpec().GetId())
		result = append(result, workflowDTO{
			Workflow: wf,
			Tasks:    tasks,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (g *HTTPGateway) handleGetWorkflow(w http.ResponseWriter, r *http.Request) {
	if g.store == nil {
		http.Error(w, "store unavailable", http.StatusServiceUnavailable)
		return
	}

	wfID := strings.TrimPrefix(r.URL.Path, "/api/workflows/")
	if wfID == "" {
		http.Error(w, "missing workflow ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	wf, _, err := g.store.GetWorkflow(ctx, wfID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	tasks, _ := g.store.ListTasksForWorkflow(ctx, wfID)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"workflow": wf,
		"tasks":    tasks,
	})
}

// parseWorkflowSpec decodes JSON or YAML payload into a protobuf WorkflowSpec.
func parseWorkflowSpec(body []byte) (*pb.WorkflowSpec, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, errors.New("empty request payload")
	}

	// 1. If JSON, parse directly
	if trimmed[0] == '{' {
		var spec pb.WorkflowSpec
		if err := json.Unmarshal(trimmed, &spec); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		return &spec, nil
	}

	// 2. Parse YAML
	spec := &pb.WorkflowSpec{
		FailurePolicy: pb.FailurePolicy_FAILURE_POLICY_FAIL_FAST,
		Tasks:         make([]*pb.TaskSpec, 0),
	}

	scanner := bufio.NewScanner(bytes.NewReader(trimmed))
	var currentTask *pb.TaskSpec
	inTasks := false
	inDeps := false

	for scanner.Scan() {
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "id:") && !inTasks {
			spec.Id = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
			continue
		}
		if strings.HasPrefix(line, "name:") && !inTasks {
			spec.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			continue
		}
		if strings.HasPrefix(line, "failure_policy:") && !inTasks {
			p := strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(line, "failure_policy:")))
			if strings.Contains(p, "CONTINUE") || p == "2" {
				spec.FailurePolicy = pb.FailurePolicy_FAILURE_POLICY_CONTINUE
			} else {
				spec.FailurePolicy = pb.FailurePolicy_FAILURE_POLICY_FAIL_FAST
			}
			continue
		}

		if line == "tasks:" {
			inTasks = true
			continue
		}

		if inTasks {
			if strings.HasPrefix(line, "- id:") {
				inDeps = false
				taskID := strings.TrimSpace(strings.TrimPrefix(line, "- id:"))
				currentTask = &pb.TaskSpec{
					Id:             taskID,
					TimeoutSeconds: 30,
					MaxRetries:     2,
					Dependencies:   make([]string, 0),
				}
				spec.Tasks = append(spec.Tasks, currentTask)
				continue
			}

			if currentTask != nil {
				if strings.HasPrefix(line, "type:") {
					inDeps = false
					currentTask.Type = strings.TrimSpace(strings.TrimPrefix(line, "type:"))
				} else if strings.HasPrefix(line, "payload:") {
					inDeps = false
					p := strings.TrimSpace(strings.TrimPrefix(line, "payload:"))
					p = strings.Trim(p, "\"'")
					currentTask.Payload = p
				} else if strings.HasPrefix(line, "timeout_seconds:") {
					inDeps = false
					val, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "timeout_seconds:")))
					currentTask.TimeoutSeconds = int64(val)
				} else if strings.HasPrefix(line, "max_retries:") {
					inDeps = false
					val, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "max_retries:")))
					currentTask.MaxRetries = int32(val)
				} else if line == "dependencies:" {
					inDeps = true
				} else if inDeps && strings.HasPrefix(line, "-") {
					dep := strings.TrimSpace(strings.TrimPrefix(line, "-"))
					if dep != "" {
						currentTask.Dependencies = append(currentTask.Dependencies, dep)
					}
				}
			}
		}
	}

	if spec.Id == "" {
		return nil, errors.New("missing workflow 'id'")
	}
	if len(spec.Tasks) == 0 {
		return nil, errors.New("workflow must have at least one task")
	}

	return spec, nil
}

func (g *HTTPGateway) handleSubmitWorkflow(w http.ResponseWriter, r *http.Request) {
	var body bytes.Buffer
	if _, err := body.ReadFrom(r.Body); err != nil {
		http.Error(w, fmt.Sprintf("failed to read body: %v", err), http.StatusBadRequest)
		return
	}

	spec, err := parseWorkflowSpec(body.Bytes())
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid workflow definition: %v", err), http.StatusBadRequest)
		return
	}

	// Validate DAG structure before submission
	if _, err := dag.ValidateAndBuild(spec); err != nil {
		http.Error(w, fmt.Sprintf("DAG validation error: %v", err), http.StatusBadRequest)
		return
	}

	// If this node is active leader, process directly
	if g.candidate != nil && g.candidate.IsLeader() && g.engine != nil {
		resp, err := g.engine.SubmitWorkflow(r.Context(), &pb.SubmitWorkflowRequest{
			Workflow: spec,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	// Follower proxy: Discover active leader from etcd and forward via gRPC
	leader, _, err := g.discoverLeader(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("cannot submit: no leader available (%v)", err), http.StatusServiceUnavailable)
		return
	}

	targetAddr := leader.GrpcAddr
	if targetAddr == "" {
		targetAddr = "127.0.0.1:50051"
	}

	conn, err := grpc.NewClient(targetAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to connect to leader at %s: %v", targetAddr, err), http.StatusBadGateway)
		return
	}
	defer conn.Close()

	client := pb.NewOrchestratorServiceClient(conn)
	resp, err := client.SubmitWorkflow(r.Context(), &pb.SubmitWorkflowRequest{Workflow: spec})
	if err != nil {
		http.Error(w, fmt.Sprintf("RPC to leader failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (g *HTTPGateway) handleCancelWorkflow(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	wfID := parts[2]

	// If this node is leader, cancel directly
	if g.candidate != nil && g.candidate.IsLeader() && g.engine != nil {
		resp, err := g.engine.CancelWorkflow(r.Context(), &pb.CancelWorkflowRequest{
			WorkflowId: wfID,
			Reason:     "cancelled via dashboard API",
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	// Follower proxy
	leader, _, err := g.discoverLeader(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("cannot cancel: %v", err), http.StatusServiceUnavailable)
		return
	}
	targetAddr := leader.GrpcAddr
	if targetAddr == "" {
		targetAddr = "127.0.0.1:50051"
	}

	conn, err := grpc.NewClient(targetAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to dial leader: %v", err), http.StatusBadGateway)
		return
	}
	defer conn.Close()

	client := pb.NewOrchestratorServiceClient(conn)
	resp, err := client.CancelWorkflow(r.Context(), &pb.CancelWorkflowRequest{
		WorkflowId: wfID,
		Reason:     "cancelled via dashboard proxy",
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("cancel RPC failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (g *HTTPGateway) checkDevMode(w http.ResponseWriter, r *http.Request) bool {
	devMode := os.Getenv("ORCHESTRA_DEV_MODE") == "true" ||
		os.Getenv("ORCHESTRA_DEV_MODE") == "1" ||
		r.Header.Get("X-Orchestra-Dev-Mode") == "true" ||
		r.URL.Query().Get("dev_mode") == "true"
	if !devMode {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": "chaos endpoints are disabled in production mode (requires ORCHESTRA_DEV_MODE=true)",
		})
		return false
	}
	return true
}

func (g *HTTPGateway) handleStepdown(w http.ResponseWriter, r *http.Request) {
	if !g.checkDevMode(w, r) {
		return
	}

	if g.candidate == nil || !g.candidate.IsLeader() {
		http.Error(w, "node is not leader", http.StatusBadRequest)
		return
	}

	g.logger.Warn("chaos stepdown requested via dashboard API")
	if err := g.candidate.Resign(r.Context()); err != nil {
		http.Error(w, fmt.Sprintf("failed to resign: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "stepped_down"})
}

// handleChaosKillWorker terminates a worker container or revokes its lease in etcd.
func (g *HTTPGateway) handleChaosKillWorker(w http.ResponseWriter, r *http.Request) {
	if !g.checkDevMode(w, r) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var req struct {
		WorkerID string `json:"worker_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	targetWorker := req.WorkerID
	if targetWorker == "" && g.store != nil {
		workers, _ := g.store.ListActiveWorkers(ctx)
		for k := range workers {
			targetWorker = k
			break
		}
	}
	if targetWorker == "" {
		targetWorker = "distributed-scheduler-worker-1"
	}

	g.logger.Warn("chaos kill-worker triggered", slog.String("worker_id", targetWorker))

	// Attempt docker kill on worker container
	containers := []string{
		targetWorker,
		fmt.Sprintf("distributed-scheduler-%s", targetWorker),
		"distributed-scheduler-worker-1",
		"distributed-scheduler-worker-2",
		"distributed-scheduler-worker-3",
		"distributed-scheduler-worker-4",
		"distributed-scheduler-worker-5",
	}

	killed := false
	for _, c := range containers {
		if err := exec.Command("docker", "kill", c).Run(); err == nil {
			killed = true
			g.logger.Info("killed worker container via docker kill", slog.String("container", c))
			break
		}
	}

	// Delete worker key in etcd to ensure immediate scheduler re-queuing
	if g.store != nil && g.store.Client() != nil {
		_, _ = g.store.Client().Delete(ctx, store.WorkerKey(targetWorker))
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":           "worker killed",
		"worker_id":        targetWorker,
		"killed_by_docker": killed,
		"timestamp":        time.Now().UnixMilli(),
	})
}

// handleChaosKillLeader kills the active leader orchestrator container or steps down leadership.
func (g *HTTPGateway) handleChaosKillLeader(w http.ResponseWriter, r *http.Request) {
	if !g.checkDevMode(w, r) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	leader, term, _ := g.discoverLeader(ctx)
	var leaderNodeID string
	if leader != nil {
		leaderNodeID = leader.NodeID
	} else if g.candidate != nil && g.candidate.IsLeader() {
		leaderNodeID = g.nodeID
	}

	g.logger.Warn("chaos kill-leader triggered", slog.String("leader_id", leaderNodeID), slog.Int64("term", term))

	// If this node is the leader, resign immediately
	if g.candidate != nil && g.candidate.IsLeader() {
		_ = g.candidate.Resign(ctx)
	}

	// Delete leader election key in etcd to revoke leadership instantly
	if g.store != nil && g.store.Client() != nil {
		_, _ = g.store.Client().Delete(ctx, store.ElectionKey, clientv3.WithPrefix())
	}

	// Docker kill the leader container
	containerMap := map[string]string{
		"orchestrator-1": "orchestra-node-1",
		"orchestrator-2": "orchestra-node-2",
		"orchestrator-3": "orchestra-node-3",
	}
	cName := containerMap[leaderNodeID]
	if cName == "" {
		cName = "orchestra-node-1"
	}

	err := exec.Command("docker", "kill", cName).Run()
	killedByDocker := err == nil

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":           "leader killed",
		"leader_id":        leaderNodeID,
		"container":        cName,
		"term":             term,
		"killed_by_docker": killedByDocker,
		"timestamp":        time.Now().UnixMilli(),
	})
}

// handleSSEEvents streams workflow, task, worker, and leader changes via Server-Sent Events.
func (g *HTTPGateway) handleSSEEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Emit initial connection event
	initData, _ := json.Marshal(map[string]any{
		"node_id":   g.nodeID,
		"is_leader": g.candidate != nil && g.candidate.IsLeader(),
		"connected": true,
		"timestamp": time.Now().UnixMilli(),
	})
	_, _ = fmt.Fprintf(w, "event: connected\ndata: %s\n\n", initData)
	flusher.Flush()

	// If etcd is not available, fall back to in-memory broadcast
	if g.store == nil || g.store.Client() == nil {
		g.streamInMemorySSE(w, r, flusher)
		return
	}

	// Watch etcd events, workers, and election prefix so ANY node streams live updates
	watchCtx, watchCancel := context.WithCancel(r.Context())
	defer watchCancel()

	eventsCh := g.store.Client().Watch(watchCtx, store.PrefixEvents, clientv3.WithPrefix())
	workersCh := g.store.Client().Watch(watchCtx, store.PrefixWorkers, clientv3.WithPrefix())
	electionCh := g.store.Client().Watch(watchCtx, store.ElectionKey, clientv3.WithPrefix())

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return

		case resp, ok := <-eventsCh:
			if !ok {
				return
			}
			for _, ev := range resp.Events {
				if ev.Type == clientv3.EventTypePut {
					_, _ = fmt.Fprintf(w, "event: task_event\ndata: %s\n\n", string(ev.Kv.Value))
					flusher.Flush()
				}
			}

		case resp, ok := <-workersCh:
			if !ok {
				return
			}
			for _, ev := range resp.Events {
				payload, _ := json.Marshal(map[string]any{
					"key":       string(ev.Kv.Key),
					"value":     string(ev.Kv.Value),
					"type":      ev.Type.String(),
					"timestamp": time.Now().UnixMilli(),
				})
				_, _ = fmt.Fprintf(w, "event: worker_event\ndata: %s\n\n", payload)
				flusher.Flush()
			}

		case resp, ok := <-electionCh:
			if !ok {
				return
			}
			for _, ev := range resp.Events {
				payload, _ := json.Marshal(map[string]any{
					"key":       string(ev.Kv.Key),
					"value":     string(ev.Kv.Value),
					"type":      ev.Type.String(),
					"timestamp": time.Now().UnixMilli(),
				})
				_, _ = fmt.Fprintf(w, "event: leader_event\ndata: %s\n\n", payload)
				flusher.Flush()
			}

		case <-ticker.C:
			_, _ = fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}

func (g *HTTPGateway) streamInMemorySSE(w http.ResponseWriter, r *http.Request, flusher http.Flusher) {
	ch := make(chan []byte, 32)
	g.sseMu.Lock()
	g.sseClients[ch] = struct{}{}
	g.sseMu.Unlock()

	defer func() {
		g.sseMu.Lock()
		delete(g.sseClients, ch)
		g.sseMu.Unlock()
	}()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			_, _ = fmt.Fprintf(w, "event: update\ndata: %s\n\n", msg)
			flusher.Flush()
		case <-ticker.C:
			_, _ = fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}

// Broadcast sends an event payload to in-memory subscriber channels.
func (g *HTTPGateway) Broadcast(eventPayload []byte) {
	g.sseMu.RLock()
	defer g.sseMu.RUnlock()

	for ch := range g.sseClients {
		select {
		case ch <- eventPayload:
		default:
		}
	}
}
