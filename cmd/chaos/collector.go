package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	clientv3 "go.etcd.io/etcd/client/v3"
	pb "orchestra/api/proto/orchestra/v1"
	"orchestra/internal/store"
)

// RunSummary captures comprehensive statistics and invariant verification for a chaos run.
type RunSummary struct {
	Seed               int64    `json:"seed"`
	WorkflowsSubmitted int      `json:"workflows_submitted"`
	WorkflowsSucceeded int      `json:"workflows_succeeded"`
	WorkflowsFailed    int      `json:"workflows_failed"`
	TotalTasks         int      `json:"total_tasks"`
	TasksCompleted     int      `json:"tasks_completed"`
	TasksFailed        int      `json:"tasks_failed"`
	TasksDeadLetter    int      `json:"tasks_dead_letter"`
	FaultsInjected     int      `json:"faults_injected"`
	DurationMs         int64    `json:"duration_ms"`
	DrainDurationMs    int64    `json:"drain_duration_ms"`
	VerificationPassed bool     `json:"verification_passed"`
	Violations         []string `json:"violations,omitempty"`
	Notes              string   `json:"notes,omitempty"`
}

type Collector struct {
	etcdClient *clientv3.Client
	runDir     string
}

func NewCollector(etcdClient *clientv3.Client, runDir string) *Collector {
	return &Collector{
		etcdClient: etcdClient,
		runDir:     runDir,
	}
}

// CollectAll drains the cluster data and exports etcd events, worker evidence, and workflows.
func (c *Collector) CollectAll(ctx context.Context, submittedIDs []string) error {
	if err := os.MkdirAll(c.runDir, 0755); err != nil {
		return fmt.Errorf("failed to create run directory: %w", err)
	}

	// 1. Collect etcd append-only event log
	if err := c.collectEtcdEvents(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed collecting etcd events: %v\n", err)
	}

	// 2. Collect worker evidence logs from shared volume
	if err := c.collectWorkerEvidence(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed collecting worker evidence: %v\n", err)
	}

	// 3. Collect final workflow and task states
	if err := c.collectWorkflowStates(ctx, submittedIDs); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed collecting workflow states: %v\n", err)
	}

	return nil
}

func (c *Collector) collectEtcdEvents(ctx context.Context) error {
	resp, err := c.etcdClient.Get(ctx, store.PrefixEvents, clientv3.WithPrefix())
	if err != nil {
		return err
	}

	type kvItem struct {
		seq int64
		val []byte
	}
	items := make([]kvItem, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		keyStr := string(kv.Key)
		parts := strings.Split(keyStr, "/")
		seq, err := strconv.ParseInt(parts[len(parts)-1], 10, 64)
		if err == nil {
			items = append(items, kvItem{seq: seq, val: kv.Value})
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].seq < items[j].seq
	})

	outFile, err := os.Create(filepath.Join(c.runDir, "events.jsonl"))
	if err != nil {
		return err
	}
	defer outFile.Close()

	for _, it := range items {
		_, _ = outFile.Write(append(it.val, '\n'))
	}
	return nil
}

func (c *Collector) collectWorkerEvidence(ctx context.Context) error {
	evidenceRawDir := filepath.Join(c.runDir, "worker_logs")
	_ = os.MkdirAll(evidenceRawDir, 0755)

	// Copy from worker container volume
	cmd := exec.CommandContext(ctx, "docker", "cp", "distributed-scheduler-worker-1:/results/.", evidenceRawDir)
	_ = cmd.Run()

	entries, err := os.ReadDir(evidenceRawDir)
	if err != nil {
		return err
	}

	concatPath := filepath.Join(c.runDir, "evidence.jsonl")
	concatFile, err := os.Create(concatPath)
	if err != nil {
		return err
	}
	defer concatFile.Close()

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jsonl") {
			content, err := os.ReadFile(filepath.Join(evidenceRawDir, entry.Name()))
			if err == nil && len(content) > 0 {
				_, _ = concatFile.Write(content)
				if !strings.HasSuffix(string(content), "\n") {
					_, _ = concatFile.WriteString("\n")
				}
			}
		}
	}
	return nil
}

func (c *Collector) collectWorkflowStates(ctx context.Context, submittedIDs []string) error {
	type WorkflowDump struct {
		Workflow *pb.Workflow `json:"workflow"`
		Tasks    []*pb.Task   `json:"tasks"`
	}

	results := make([]WorkflowDump, 0, len(submittedIDs))

	for _, wfID := range submittedIDs {
		wfKey := store.WorkflowKey(wfID)
		resp, err := c.etcdClient.Get(ctx, wfKey)
		if err != nil || len(resp.Kvs) == 0 {
			continue
		}

		var wf pb.Workflow
		if err := json.Unmarshal(resp.Kvs[0].Value, &wf); err != nil {
			continue
		}

		tasksPrefix := store.WorkflowTasksPrefix(wfID)
		tResp, err := c.etcdClient.Get(ctx, tasksPrefix, clientv3.WithPrefix())
		var tasks []*pb.Task
		if err == nil {
			for _, tkv := range tResp.Kvs {
				var t pb.Task
				if err := json.Unmarshal(tkv.Value, &t); err == nil {
					tasks = append(tasks, &t)
				}
			}
		}

		results = append(results, WorkflowDump{
			Workflow: &wf,
			Tasks:    tasks,
		})
	}

	outData, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.runDir, "workflows.json"), outData, 0644)
}
