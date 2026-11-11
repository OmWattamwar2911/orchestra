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
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	pb "orchestra/api/proto/orchestra/v1"
	"orchestra/internal/store"
)

// CollectLiveCluster exports etcd events, worker evidence, and workflows for offline verification.
func CollectLiveCluster(ctx context.Context, etcdEndpoints []string, runDir string, workerLogsDir string) error {
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return fmt.Errorf("failed to create run directory: %w", err)
	}

	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   etcdEndpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to etcd at %v: %w", etcdEndpoints, err)
	}
	defer cli.Close()

	// 1. Collect etcd append-only events
	if err := dumpEtcdEvents(ctx, cli, runDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: dumping etcd events failed: %v\n", err)
	}

	// 2. Collect workflow and task states
	if err := dumpEtcdWorkflows(ctx, cli, runDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: dumping etcd workflows failed: %v\n", err)
	}

	// 3. Collect worker evidence logs
	if err := dumpWorkerEvidence(ctx, runDir, workerLogsDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: dumping worker evidence failed: %v\n", err)
	}

	return nil
}

func dumpEtcdEvents(ctx context.Context, cli *clientv3.Client, runDir string) error {
	resp, err := cli.Get(ctx, store.PrefixEvents, clientv3.WithPrefix())
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

	outFile, err := os.Create(filepath.Join(runDir, "events.jsonl"))
	if err != nil {
		return err
	}
	defer outFile.Close()

	for _, it := range items {
		_, _ = outFile.Write(append(it.val, '\n'))
	}
	return nil
}

func dumpEtcdWorkflows(ctx context.Context, cli *clientv3.Client, runDir string) error {
	// Query all workflows
	resp, err := cli.Get(ctx, "/orchestra/workflows/", clientv3.WithPrefix())
	if err != nil {
		return err
	}

	results := make([]WorkflowDumpItem, 0, len(resp.Kvs))

	for _, kv := range resp.Kvs {
		var wf pb.Workflow
		if err := json.Unmarshal(kv.Value, &wf); err != nil {
			continue
		}

		wfID := wf.GetStatus().GetId()
		if wfID == "" {
			wfID = wf.GetSpec().GetId()
		}

		var tasks []*pb.Task
		if wfID != "" {
			tResp, err := cli.Get(ctx, store.WorkflowTasksPrefix(wfID), clientv3.WithPrefix())
			if err == nil {
				for _, tkv := range tResp.Kvs {
					var t pb.Task
					if err := json.Unmarshal(tkv.Value, &t); err == nil {
						tasks = append(tasks, &t)
					}
				}
			}
		}

		results = append(results, WorkflowDumpItem{
			Workflow: &wf,
			Tasks:    tasks,
		})
	}

	outData, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(runDir, "workflows.json"), outData, 0644)
}

func dumpWorkerEvidence(ctx context.Context, runDir string, customDir string) error {
	evidenceRawDir := filepath.Join(runDir, "raw_evidence")
	_ = os.MkdirAll(evidenceRawDir, 0755)

	// Try customDir first
	foundAny := false
	if customDir != "" {
		if entries, err := os.ReadDir(customDir); err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
					data, err := os.ReadFile(filepath.Join(customDir, e.Name()))
					if err == nil {
						_ = os.WriteFile(filepath.Join(evidenceRawDir, e.Name()), data, 0644)
						foundAny = true
					}
				}
			}
		}
	}

	// If not found in customDir, try docker cp
	if !foundAny {
		// Try copying from worker container
		cmd := exec.CommandContext(ctx, "docker", "cp", "distributed-scheduler-worker-1:/results/.", evidenceRawDir)
		_ = cmd.Run()
	}

	// Also check local evidence dir
	if entries, err := os.ReadDir("evidence"); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
				data, err := os.ReadFile(filepath.Join("evidence", e.Name()))
				if err == nil {
					_ = os.WriteFile(filepath.Join(evidenceRawDir, "local_"+e.Name()), data, 0644)
				}
			}
		}
	}

	entries, err := os.ReadDir(evidenceRawDir)
	if err != nil {
		return err
	}

	concatPath := filepath.Join(runDir, "evidence.jsonl")
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
