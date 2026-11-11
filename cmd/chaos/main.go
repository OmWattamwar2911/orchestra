package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	pb "orchestra/api/proto/orchestra/v1"
	"orchestra/internal/checker"
	"orchestra/internal/etcdstore"
	"orchestra/internal/store"
)

type MultiRunSummary struct {
	Timestamp      time.Time    `json:"timestamp"`
	TotalRuns      int          `json:"total_runs"`
	RunsPassed     int          `json:"runs_passed"`
	RunsFailed     int          `json:"runs_failed"`
	TotalWorkflows int          `json:"total_workflows"`
	TotalTasks     int          `json:"total_tasks"`
	TotalFaults    int          `json:"total_faults"`
	RunSummaries   []RunSummary `json:"run_summaries"`
}

func main() {
	var (
		seedFlag      = flag.Int64("seed", 1001, "Initial pseudo-random seed for deterministic reproduction")
		runsFlag      = flag.Int("runs", 10, "Total number of consecutive chaos runs to execute")
		workflowsFlag = flag.Int("workflows", 200, "Number of workflows to submit per run (default 200)")
		rateFlag      = flag.Int("rate", 15, "Submission rate in workflows per second")
		injectFaults  = flag.Bool("inject-faults", true, "Whether to inject worker/leader/etcd faults during load")
		resultsDir    = flag.String("results-dir", "results", "Root directory to save run artifacts")
		etcdEndpoints = flag.String("etcd-endpoints", "localhost:2379", "Comma-separated etcd endpoints")
		orchAddrs     = flag.String("orchestrator-addrs", "localhost:50051,localhost:50052,localhost:50053", "Comma-separated orchestrator gRPC endpoints")
		drainTimeout  = flag.Duration("drain-timeout", 180*time.Second, "Maximum duration to wait for cluster drain per run")
	)
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})).With(slog.String("component", "chaos-runner"))
	slog.SetDefault(logger)

	fmt.Println("================================================================================")
	fmt.Printf(" ORCHESTRA CHAOS TEST SUITE (Runs: %d, Workflows/Run: %d, Rate: %d wf/s)\n", *runsFlag, *workflowsFlag, *rateFlag)
	fmt.Printf(" Base Seed: %d | Faults Enabled: %v | Results Dir: %s\n", *seedFlag, *injectFaults, *resultsDir)
	fmt.Println("================================================================================")

	candidateAddrs := strings.Split(*orchAddrs, ",")
	etcdNodes := strings.Split(*etcdEndpoints, ",")

	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints:   etcdNodes,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		logger.Error("failed to connect to etcd", slog.Any("error", err))
		os.Exit(1)
	}
	defer etcdClient.Close()

	overallSummary := MultiRunSummary{
		Timestamp:    time.Now(),
		TotalRuns:    *runsFlag,
		RunSummaries: make([]RunSummary, 0, *runsFlag),
	}

	for r := 0; r < *runsFlag; r++ {
		currentSeed := *seedFlag + int64(r)
		fmt.Printf("\n>>> STARTING RUN %d/%d (Seed: %d) <<<\n", r+1, *runsFlag, currentSeed)

		summary, err := executeSingleRun(currentSeed, *workflowsFlag, *rateFlag, *injectFaults, *drainTimeout, *resultsDir, candidateAddrs, etcdClient, logger)
		if err != nil {
			fmt.Printf("Run %d failed with error: %v\n", r+1, err)
			summary.Notes = fmt.Sprintf("run error: %v", err)
			summary.VerificationPassed = false
		}

		overallSummary.RunSummaries = append(overallSummary.RunSummaries, summary)
		overallSummary.TotalWorkflows += summary.WorkflowsSubmitted
		overallSummary.TotalTasks += summary.TotalTasks
		overallSummary.TotalFaults += summary.FaultsInjected

		if summary.VerificationPassed {
			overallSummary.RunsPassed++
			fmt.Printf(">>> RUN %d/%d (Seed: %d) PASSED: %d workflows, %d tasks, %d faults in %d ms <<<\n",
				r+1, *runsFlag, currentSeed, summary.WorkflowsSubmitted, summary.TotalTasks, summary.FaultsInjected, summary.DurationMs)
		} else {
			overallSummary.RunsFailed++
			fmt.Printf(">>> RUN %d/%d (Seed: %d) FAILED INVARIANT CHECK! Violations: %v <<<\n",
				r+1, *runsFlag, currentSeed, summary.Violations)
		}
	}

	// Persist overall multi-run summary
	_ = os.MkdirAll(*resultsDir, 0755)
	overallJSON, _ := json.MarshalIndent(overallSummary, "", "  ")
	summaryFile := filepath.Join(*resultsDir, "chaos_summary.json")
	_ = os.WriteFile(summaryFile, overallJSON, 0644)

	fmt.Println("\n================================================================================")
	fmt.Printf("CHAOS EXPERIMENT COMPLETE: %d/%d runs passed (%d failed)\n",
		overallSummary.RunsPassed, overallSummary.TotalRuns, overallSummary.RunsFailed)
	fmt.Printf("Total Workflows Tested: %d | Total Tasks: %d | Total Faults: %d\n",
		overallSummary.TotalWorkflows, overallSummary.TotalTasks, overallSummary.TotalFaults)
	fmt.Printf("Comprehensive summary written to: %s\n", summaryFile)
	fmt.Println("================================================================================")

	if overallSummary.RunsFailed > 0 {
		os.Exit(1)
	}
}

func executeSingleRun(
	seed int64,
	numWorkflows int,
	rate int,
	injectFaults bool,
	drainTimeout time.Duration,
	resultsBaseDir string,
	orchAddrs []string,
	etcdClient *clientv3.Client,
	logger *slog.Logger,
) (RunSummary, error) {
	runDir := filepath.Join(resultsBaseDir, fmt.Sprintf("run-%d", seed))
	_ = os.MkdirAll(runDir, 0755)

	summary := RunSummary{
		Seed:               seed,
		WorkflowsSubmitted: numWorkflows,
	}

	startTime := time.Now()
	rng := rand.New(rand.NewSource(seed))

	// 1. Initialize Fault Injector
	faultsLogPath := filepath.Join(runDir, "faults.jsonl")
	injector, err := NewFaultInjector(rng, etcdClient, faultsLogPath, logger)
	if err != nil {
		return summary, fmt.Errorf("failed to initialize fault injector: %w", err)
	}
	defer injector.Close()

	faultCtx, stopFaults := context.WithCancel(context.Background())
	defer stopFaults()

	// 2. Launch fault injection in background during workload generation
	var faultsInjectedCount int
	if injectFaults {
		go func() {
			ticker := time.NewTicker(3500 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-faultCtx.Done():
					return
				case <-ticker.C:
					injector.InjectRandomFault(faultCtx)
					faultsInjectedCount++
				}
			}
		}()
	}

	// 3. Workload generator: submit W workflows
	submittedIDs := make([]string, 0, numWorkflows)
	delayBetweenSubmissions := time.Duration(1000/rate) * time.Millisecond
	if delayBetweenSubmissions <= 0 {
		delayBetweenSubmissions = 50 * time.Millisecond
	}

	for i := 0; i < numWorkflows; i++ {
		wfID := fmt.Sprintf("wf-s%d-%04d", seed, i)
		spec, _ := GenerateRandomWorkflow(rng, wfID)
		summary.TotalTasks += len(spec.GetTasks())

		submitCtx, submitCancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := submitWithRetry(submitCtx, orchAddrs, spec)
		submitCancel()

		if err != nil {
			logger.Warn("workflow submission failed", slog.String("wf_id", wfID), slog.Any("error", err))
		} else {
			submittedIDs = append(submittedIDs, wfID)
		}

		time.Sleep(delayBetweenSubmissions)
	}

	// Workload submitted; allow any active fault to finish and stop injector
	time.Sleep(1 * time.Second)
	stopFaults()
	summary.FaultsInjected = faultsInjectedCount

	// 4. Cluster Drain: wait for all submitted workflows to reach terminal state
	drainStart := time.Now()
	fmt.Printf("Awaiting cluster drain (%d workflows submitted)...\n", len(submittedIDs))

	drainCtx, cancelDrain := context.WithTimeout(context.Background(), drainTimeout)
	defer cancelDrain()

	allTerminal := awaitClusterDrain(drainCtx, etcdClient, submittedIDs)
	summary.DrainDurationMs = time.Since(drainStart).Milliseconds()

	if !allTerminal {
		summary.Notes = "cluster drain timed out before all workflows reached terminal state"
		fmt.Println("Warning: drain timed out; proceeding with data collection...")
	}

	// 5. Collect artifacts
	collector := NewCollector(etcdClient, runDir)
	collectCtx, cancelCollect := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelCollect()
	_ = collector.CollectAll(collectCtx, submittedIDs)

	// 6. Calculate final state breakdown
	for _, wfID := range submittedIDs {
		resp, err := etcdClient.Get(context.Background(), store.WorkflowKey(wfID))
		if err == nil && len(resp.Kvs) > 0 {
			var wf pb.Workflow
			if err := json.Unmarshal(resp.Kvs[0].Value, &wf); err == nil {
				if wf.GetStatus().GetState() == pb.WorkflowState_WORKFLOW_STATE_SUCCEEDED {
					summary.WorkflowsSucceeded++
				} else {
					summary.WorkflowsFailed++
				}
			}
		}

		tResp, err := etcdClient.Get(context.Background(), store.WorkflowTasksPrefix(wfID), clientv3.WithPrefix())
		if err == nil {
			for _, tkv := range tResp.Kvs {
				var t pb.Task
				if err := json.Unmarshal(tkv.Value, &t); err == nil {
					switch t.GetStatus().GetState() {
					case pb.TaskState_TASK_STATE_COMPLETED:
						summary.TasksCompleted++
					case pb.TaskState_TASK_STATE_FAILED:
						summary.TasksFailed++
					case pb.TaskState_TASK_STATE_DEAD_LETTER:
						summary.TasksDeadLetter++
					}
				}
			}
		}
	}

	summary.DurationMs = time.Since(startTime).Milliseconds()

	// 7. Run Invariant Verification
	evidencePath := filepath.Join(runDir, "evidence.jsonl")
	etcdStore := etcdstore.NewStore(etcdClient)
	chk := checker.NewInvariantChecker(etcdStore, evidencePath)

	vResult, err := chk.VerifyAll(context.Background())
	if err != nil {
		summary.VerificationPassed = false
		summary.Violations = append(summary.Violations, fmt.Sprintf("checker error: %v", err))
	} else {
		summary.VerificationPassed = vResult.AllPassed
		if !vResult.AllPassed {
			for _, r := range vResult.Reports {
				if !r.Passed {
					summary.Violations = append(summary.Violations, r.Violations...)
				}
			}
		}
	}

	// Save single-run summary
	summaryJSON, _ := json.MarshalIndent(summary, "", "  ")
	_ = os.WriteFile(filepath.Join(runDir, "summary.json"), summaryJSON, 0644)

	return summary, nil
}

func submitWithRetry(ctx context.Context, addrs []string, spec *pb.WorkflowSpec) error {
	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		conn, client, _, err := connectToLeader(ctx, addrs)
		if err == nil {
			resp, submitErr := client.SubmitWorkflow(ctx, &pb.SubmitWorkflowRequest{Workflow: spec})
			_ = conn.Close()
			if submitErr == nil && resp.GetAccepted() {
				return nil
			}
			if resp != nil && !resp.GetAccepted() {
				lastErr = fmt.Errorf("submission rejected: %s", resp.GetMessage())
			} else {
				lastErr = submitErr
			}
		} else {
			lastErr = err
		}

		select {
		case <-time.After(500 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return fmt.Errorf("failed to submit %s: %w", spec.GetId(), lastErr)
}

func awaitClusterDrain(ctx context.Context, etcdClient *clientv3.Client, submittedIDs []string) bool {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			allDone := true
			for _, wfID := range submittedIDs {
				resp, err := etcdClient.Get(ctx, store.WorkflowKey(wfID))
				if err != nil || len(resp.Kvs) == 0 {
					allDone = false
					break
				}
				var wf pb.Workflow
				if err := json.Unmarshal(resp.Kvs[0].Value, &wf); err != nil {
					allDone = false
					break
				}
				state := wf.GetStatus().GetState()
				if state == pb.WorkflowState_WORKFLOW_STATE_RUNNING || state == pb.WorkflowState_WORKFLOW_STATE_UNSPECIFIED {
					allDone = false
					break
				}
			}
			if allDone {
				return true
			}
		}
	}
}

func connectToLeader(ctx context.Context, addrs []string) (*grpc.ClientConn, pb.OrchestratorServiceClient, string, error) {
	var lastErr error
	for _, addr := range addrs {
		dialCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
		conn, err := grpc.DialContext(dialCtx, addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
		)
		cancel()
		if err != nil {
			lastErr = err
			continue
		}

		client := pb.NewOrchestratorServiceClient(conn)
		// Check leader via probe
		probeCtx, probeCancel := context.WithTimeout(ctx, 1*time.Second)
		resp, _ := client.SubmitWorkflow(probeCtx, &pb.SubmitWorkflowRequest{Workflow: nil})
		probeCancel()

		if resp != nil && !strings.Contains(resp.GetMessage(), "not the active cluster leader") {
			return conn, client, addr, nil
		}
		_ = conn.Close()
	}
	return nil, nil, "", fmt.Errorf("no leader reachable at %v: %w", addrs, lastErr)
}
