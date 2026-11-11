package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	pb "orchestra/api/proto/orchestra/v1"
)

func main() {
	var (
		orchestratorAddr = flag.String("addr", "localhost:50051", "Target address for orchestrator gRPC service")
		cmdFlag          = flag.String("cmd", "", "Command to execute: submit, get, list, cancel, watch")
		workflowID       = flag.String("id", "", "Workflow ID for get/cancel/watch commands")
		filePath         = flag.String("file", "", "Path to YAML/JSON workflow definition file for submit command")
		watchAfterSubmit = flag.Bool("watch", false, "Stream and follow workflow execution events after submitting")
	)
	flag.Parse()

	rawArgs := flag.Args()
	var args []string
	for _, a := range rawArgs {
		if a == "-watch" || a == "--watch" {
			*watchAfterSubmit = true
		} else {
			args = append(args, a)
		}
	}

	cmd := *cmdFlag
	if cmd == "" && len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}
	if cmd == "" {
		cmd = "list"
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	})).With(
		slog.String("component", "orchctl"),
	)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Candidates for leader failover dialing (allows host CLI to talk to whichever node holds leadership)
	candidateAddrs := []string{*orchestratorAddr}
	if *orchestratorAddr == "localhost:50051" || *orchestratorAddr == "127.0.0.1:50051" {
		candidateAddrs = []string{"localhost:50051", "localhost:50052", "localhost:50053"}
	}

	conn, client, targetAddr, err := connectToLeader(ctx, candidateAddrs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not connect to orchestrator cluster (%v)\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	switch strings.ToLower(cmd) {
	case "submit":
		targetFile := *filePath
		if targetFile == "" && len(args) > 0 {
			targetFile = args[0]
		}

		var spec *pb.WorkflowSpec
		if targetFile != "" {
			var err error
			spec, err = ParseWorkflowFile(targetFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error parsing workflow file %s: %v\n", targetFile, err)
				os.Exit(1)
			}
		} else {
			// Built-in default diamond if no file provided
			spec = &pb.WorkflowSpec{
				Id:            fmt.Sprintf("diamond-%d", time.Now().Unix()),
				Name:          "Default Diamond Workflow",
				FailurePolicy: pb.FailurePolicy_FAILURE_POLICY_FAIL_FAST,
				Tasks: []*pb.TaskSpec{
					{Id: "fetch", Type: "echo", Payload: "fetched data", TimeoutSeconds: 30, MaxRetries: 2},
					{Id: "transformA", Type: "echo", Payload: "branch A", TimeoutSeconds: 30, MaxRetries: 2, Dependencies: []string{"fetch"}},
					{Id: "transformB", Type: "echo", Payload: "branch B", TimeoutSeconds: 30, MaxRetries: 2, Dependencies: []string{"fetch"}},
					{Id: "merge", Type: "echo", Payload: "merged A and B", TimeoutSeconds: 30, MaxRetries: 2, Dependencies: []string{"transformA", "transformB"}},
					{Id: "publish", Type: "echo", Payload: "published result", TimeoutSeconds: 30, MaxRetries: 2, Dependencies: []string{"merge"}},
				},
			}
		}

		submittedID := submitWorkflow(ctx, client, spec, candidateAddrs, targetAddr)
		if *watchAfterSubmit && submittedID != "" {
			fmt.Printf("\n--- Streaming events for %s ---\n", submittedID)
			watchWorkflow(ctx, client, submittedID)
		}

	case "get":
		id := *workflowID
		if id == "" && len(args) > 0 {
			id = args[0]
		}
		if id == "" {
			fmt.Println("Error: workflow id is required (e.g. `orchctl get <id>`)")
			os.Exit(1)
		}
		getWorkflow(ctx, client, id)

	case "list":
		listWorkflows(ctx, client)

	case "cancel":
		id := *workflowID
		if id == "" && len(args) > 0 {
			id = args[0]
		}
		if id == "" {
			fmt.Println("Error: workflow id is required (e.g. `orchctl cancel <id>`)")
			os.Exit(1)
		}
		cancelWorkflow(ctx, client, id)

	case "watch":
		id := *workflowID
		if id == "" && len(args) > 0 {
			id = args[0]
		}
		if id == "" {
			fmt.Println("Error: workflow id is required (e.g. `orchctl watch <id>`)")
			os.Exit(1)
		}
		watchWorkflow(ctx, client, id)

	default:
		fmt.Printf("Unknown command: %s. Valid commands: submit, get, list, cancel, watch\n", cmd)
		os.Exit(1)
	}
}

func connectToLeader(ctx context.Context, addrs []string) (*grpc.ClientConn, pb.OrchestratorServiceClient, string, error) {
	var lastErr error
	var fallbackConn *grpc.ClientConn
	var fallbackClient pb.OrchestratorServiceClient
	var fallbackAddr string

	for _, addr := range addrs {
		conn, err := grpc.NewClient(addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			lastErr = err
			continue
		}

		client := pb.NewOrchestratorServiceClient(conn)
		// Check if this node is active leader by probing SubmitWorkflow
		probeCtx, probeCancel := context.WithTimeout(ctx, 1*time.Second)
		resp, _ := client.SubmitWorkflow(probeCtx, &pb.SubmitWorkflowRequest{Workflow: nil})
		probeCancel()

		if resp != nil && !strings.Contains(resp.GetMessage(), "not the active cluster leader") {
			// Found active elected leader!
			if fallbackConn != nil {
				_ = fallbackConn.Close()
			}
			return conn, client, addr, nil
		}

		// Save first reachable node as fallback for read queries if no leader found yet
		if fallbackConn == nil {
			fallbackConn = conn
			fallbackClient = client
			fallbackAddr = addr
		} else {
			_ = conn.Close()
		}
	}

	if fallbackConn != nil {
		return fallbackConn, fallbackClient, fallbackAddr, nil
	}
	return nil, nil, "", fmt.Errorf("exhausted endpoints %v: %w", addrs, lastErr)
}

func submitWorkflow(ctx context.Context, client pb.OrchestratorServiceClient, spec *pb.WorkflowSpec, fallbackAddrs []string, currentAddr string) string {
	resp, err := client.SubmitWorkflow(ctx, &pb.SubmitWorkflowRequest{Workflow: spec})
	if err == nil && resp.GetAccepted() {
		fmt.Printf("Workflow submitted successfully!\n")
		fmt.Printf("  ID:       %s\n", resp.GetWorkflowId())
		fmt.Printf("  Accepted: %v\n", resp.GetAccepted())
		fmt.Printf("  Message:  %s\n", resp.GetMessage())
		return resp.GetWorkflowId()
	}

	// If current node rejected because it is not active leader, failover to other cluster nodes
	if resp != nil && strings.Contains(resp.GetMessage(), "not the active cluster leader") {
		for _, altAddr := range fallbackAddrs {
			if altAddr == currentAddr {
				continue
			}
			altConn, err := grpc.NewClient(altAddr,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			)
			if err != nil {
				continue
			}
			altClient := pb.NewOrchestratorServiceClient(altConn)
			altResp, altErr := altClient.SubmitWorkflow(ctx, &pb.SubmitWorkflowRequest{Workflow: spec})
			_ = altConn.Close()
			if altErr == nil && altResp.GetAccepted() {
				fmt.Printf("Workflow submitted successfully (via leader at %s)!\n", altAddr)
				fmt.Printf("  ID:       %s\n", altResp.GetWorkflowId())
				fmt.Printf("  Accepted: %v\n", altResp.GetAccepted())
				fmt.Printf("  Message:  %s\n", altResp.GetMessage())
				return altResp.GetWorkflowId()
			}
		}
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error submitting workflow: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "Workflow submission rejected: %s\n", resp.GetMessage())
	}
	os.Exit(1)
	return ""
}

func getWorkflow(ctx context.Context, client pb.OrchestratorServiceClient, id string) {
	resp, err := client.GetWorkflow(ctx, &pb.GetWorkflowRequest{WorkflowId: id})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error querying workflow %s: %v\n", id, err)
		os.Exit(1)
	}
	wf := resp.GetWorkflow()
	fmt.Printf("Workflow: %s\n", wf.GetSpec().GetId())
	fmt.Printf("  Name:      %s\n", wf.GetSpec().GetName())
	fmt.Printf("  State:     %s\n", wf.GetStatus().GetState().String())
	fmt.Printf("  Policy:    %s\n", wf.GetSpec().GetFailurePolicy().String())
	fmt.Println("  Tasks:")
	for _, t := range wf.GetTasks() {
		fmt.Printf("    - %-14s [%-12s] Attempt: %d  Worker: %-16s Output: %s\n",
			t.GetSpec().GetId(),
			t.GetStatus().GetState().String(),
			t.GetStatus().GetCurrentAttempt(),
			t.GetStatus().GetAssignedWorker(),
			string(t.GetStatus().GetResultData()),
		)
	}
}

func listWorkflows(ctx context.Context, client pb.OrchestratorServiceClient) {
	resp, err := client.ListWorkflows(ctx, &pb.ListWorkflowsRequest{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing workflows: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Active workflows (%d found):\n", len(resp.GetWorkflows()))
	for _, wf := range resp.GetWorkflows() {
		fmt.Printf("  - %-18s %-32s [%s]\n",
			wf.GetSpec().GetId(),
			wf.GetSpec().GetName(),
			wf.GetStatus().GetState().String(),
		)
	}
}

func cancelWorkflow(ctx context.Context, client pb.OrchestratorServiceClient, id string) {
	resp, err := client.CancelWorkflow(ctx, &pb.CancelWorkflowRequest{WorkflowId: id, Reason: "cli cancel"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cancel failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Cancel result: Success=%v, Message=%s\n", resp.GetSuccess(), resp.GetMessage())
}

func watchWorkflow(ctx context.Context, client pb.OrchestratorServiceClient, id string) {
	stream, err := client.WatchWorkflow(ctx, &pb.WatchWorkflowRequest{WorkflowId: id})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening watch stream: %v\n", err)
		os.Exit(1)
	}

	for {
		event, err := stream.Recv()
		if err != nil {
			if err == io.EOF || ctx.Err() != nil {
				break
			}
			fmt.Fprintf(os.Stderr, "Watch stream disconnected: %v\n", err)
			break
		}

		ts := time.UnixMilli(event.GetTimestamp()).Format("15:04:05.000")
		if event.GetTaskUpdate() != nil {
			t := event.GetTaskUpdate()
			fmt.Printf("[%s] Task '%s' -> %s (Attempt %d, Worker: %s)\n",
				ts, t.GetId(), t.GetState().String(), t.GetCurrentAttempt(), t.GetAssignedWorker())
		}
		if event.GetWorkflowState() != pb.WorkflowState_WORKFLOW_STATE_UNSPECIFIED {
			fmt.Printf("[%s] Workflow '%s' -> %s\n", ts, event.GetWorkflowId(), event.GetWorkflowState().String())
			if event.GetWorkflowState() == pb.WorkflowState_WORKFLOW_STATE_SUCCEEDED ||
				event.GetWorkflowState() == pb.WorkflowState_WORKFLOW_STATE_FAILED ||
				event.GetWorkflowState() == pb.WorkflowState_WORKFLOW_STATE_CANCELLED {
				break
			}
		}
	}
}
