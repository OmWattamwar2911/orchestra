package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"orchestra/internal/election"
	"orchestra/internal/store"
)

// FaultRecord represents an audit log entry for an injected failure.
type FaultRecord struct {
	TimestampNs int64     `json:"timestamp_ns"`
	Timestamp   time.Time `json:"timestamp"`
	Type        string    `json:"type"`
	Target      string    `json:"target"`
	Details     string    `json:"details"`
	Error       string    `json:"error,omitempty"`
}

type FaultInjector struct {
	rng           *rand.Rand
	etcdClient    *clientv3.Client
	faultsLogPath string
	logger        *slog.Logger
	// mu protects the append-only faults log file
	// Invariant protected: Synchronizes atomic serialization of concurrent fault events
	mu   sync.Mutex
	file *os.File

	workers       []string
	orchestrators []string
	etcdNodes     []string
	networkName   string
}

func NewFaultInjector(rng *rand.Rand, etcdClient *clientv3.Client, faultsLogPath string, logger *slog.Logger) (*FaultInjector, error) {
	if logger == nil {
		logger = slog.Default()
	}

	f, err := os.OpenFile(faultsLogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open faults log: %w", err)
	}

	return &FaultInjector{
		rng:           rng,
		etcdClient:    etcdClient,
		faultsLogPath: faultsLogPath,
		logger:        logger.With(slog.String("component", "fault-injector")),
		file:          f,
		workers: []string{
			"distributed-scheduler-worker-1",
			"distributed-scheduler-worker-2",
			"distributed-scheduler-worker-3",
			"distributed-scheduler-worker-4",
			"distributed-scheduler-worker-5",
		},
		orchestrators: []string{
			"orchestra-node-1",
			"orchestra-node-2",
			"orchestra-node-3",
		},
		etcdNodes: []string{
			"orchestra-etcd1",
			"orchestra-etcd2",
			"orchestra-etcd3",
		},
		networkName: "distributed-scheduler_orchestra-net",
	}, nil
}

func (fi *FaultInjector) Close() error {
	fi.mu.Lock()
	defer fi.mu.Unlock()
	if fi.file != nil {
		return fi.file.Close()
	}
	return nil
}

func (fi *FaultInjector) logFault(record FaultRecord) {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	data, err := json.Marshal(record)
	if err == nil && fi.file != nil {
		_, _ = fi.file.Write(append(data, '\n'))
	}
	fi.logger.Info("injected chaos fault",
		slog.String("type", record.Type),
		slog.String("target", record.Target),
		slog.String("details", record.Details),
		slog.String("error", record.Error),
	)
}

func (fi *FaultInjector) findLeaderContainer(ctx context.Context) string {
	resp, err := fi.etcdClient.Get(ctx, store.ElectionKey,
		clientv3.WithPrefix(),
		clientv3.WithSort(clientv3.SortByCreateRevision, clientv3.SortAscend),
		clientv3.WithLimit(1),
	)
	if err == nil && len(resp.Kvs) > 0 {
		info, err := election.ParseLeaderValue(string(resp.Kvs[0].Value))
		if err == nil && info.NodeID != "" {
			switch info.NodeID {
			case "orchestrator-1":
				return "orchestra-node-1"
			case "orchestrator-2":
				return "orchestra-node-2"
			case "orchestrator-3":
				return "orchestra-node-3"
			}
		}
	}
	// Fallback to random orchestrator if etcd read fails
	return fi.orchestrators[fi.rng.Intn(len(fi.orchestrators))]
}

// InjectRandomFault selects and applies one of the 5 required failure scenarios.
func (fi *FaultInjector) InjectRandomFault(ctx context.Context) {
	faultRoll := fi.rng.Intn(5)

	switch faultRoll {
	case 0:
		fi.KillRandomWorker(ctx)
	case 1:
		fi.KillCurrentLeader(ctx)
	case 2:
		fi.RestartEtcdNode(ctx)
	case 3:
		fi.PauseLeaderZombie(ctx)
	case 4:
		fi.DisconnectWorkerBriefly(ctx)
	}
}

// 1. Kill a random worker (docker kill) then resurrect after 1s
func (fi *FaultInjector) KillRandomWorker(ctx context.Context) {
	worker := fi.workers[fi.rng.Intn(len(fi.workers))]
	now := time.Now()

	err := exec.CommandContext(ctx, "docker", "kill", worker).Run()
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}

	fi.logFault(FaultRecord{
		TimestampNs: now.UnixNano(),
		Timestamp:   now,
		Type:        "KILL_WORKER",
		Target:      worker,
		Details:     "killed worker container via docker kill",
		Error:       errStr,
	})

	// Restart container after 1s to restore execution capacity
	go func() {
		time.Sleep(1 * time.Second)
		_ = exec.Command("docker", "start", worker).Run()
	}()
}

// 2. Kill the current leader container (docker kill)
func (fi *FaultInjector) KillCurrentLeader(ctx context.Context) {
	leader := fi.findLeaderContainer(ctx)
	now := time.Now()

	err := exec.CommandContext(ctx, "docker", "kill", leader).Run()
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}

	fi.logFault(FaultRecord{
		TimestampNs: now.UnixNano(),
		Timestamp:   now,
		Type:        "KILL_LEADER",
		Target:      leader,
		Details:     "killed active orchestrator leader via docker kill",
		Error:       errStr,
	})

	// Restart container after 2s so it rejoins cluster as candidate
	go func() {
		time.Sleep(2 * time.Second)
		_ = exec.Command("docker", "start", leader).Run()
	}()
}

// 3. Restart a single etcd node
func (fi *FaultInjector) RestartEtcdNode(ctx context.Context) {
	etcdNode := fi.etcdNodes[fi.rng.Intn(len(fi.etcdNodes))]
	now := time.Now()

	err := exec.CommandContext(ctx, "docker", "restart", etcdNode).Run()
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}

	fi.logFault(FaultRecord{
		TimestampNs: now.UnixNano(),
		Timestamp:   now,
		Type:        "RESTART_ETCD",
		Target:      etcdNode,
		Details:     "restarted etcd consensus node via docker restart",
		Error:       errStr,
	})
}

// 4. Pause a leader process past its 5s lease expiry (the zombie scenario), then unpause
func (fi *FaultInjector) PauseLeaderZombie(ctx context.Context) {
	leader := fi.findLeaderContainer(ctx)
	now := time.Now()

	pauseErr := exec.CommandContext(ctx, "docker", "pause", leader).Run()
	errStr := ""
	if pauseErr != nil {
		errStr = pauseErr.Error()
	}

	fi.logFault(FaultRecord{
		TimestampNs: now.UnixNano(),
		Timestamp:   now,
		Type:        "PAUSE_LEADER_ZOMBIE_START",
		Target:      leader,
		Details:     "paused leader container for 7.5s (lease TTL=5s) to trigger zombie fencing check",
		Error:       errStr,
	})

	if pauseErr == nil {
		// Sleep 7.5s to ensure etcd session TTL (5s) expires and standby assumes leadership
		select {
		case <-time.After(7500 * time.Millisecond):
		case <-ctx.Done():
		}

		unpauseNow := time.Now()
		unpauseErr := exec.Command("docker", "unpause", leader).Run()
		unpauseErrStr := ""
		if unpauseErr != nil {
			unpauseErrStr = unpauseErr.Error()
		}

		fi.logFault(FaultRecord{
			TimestampNs: unpauseNow.UnixNano(),
			Timestamp:   unpauseNow,
			Type:        "PAUSE_LEADER_ZOMBIE_END",
			Target:      leader,
			Details:     "unpaused zombie ex-leader; subsequent mutating writes must be rejected by fencing CAS",
			Error:       unpauseErrStr,
		})
	}
}

// 5. Brief network disconnect between a worker and the leader
func (fi *FaultInjector) DisconnectWorkerBriefly(ctx context.Context) {
	worker := fi.workers[fi.rng.Intn(len(fi.workers))]
	now := time.Now()

	disconnectErr := exec.CommandContext(ctx, "docker", "network", "disconnect", fi.networkName, worker).Run()
	errStr := ""
	if disconnectErr != nil {
		errStr = strings.TrimSpace(disconnectErr.Error())
	}

	fi.logFault(FaultRecord{
		TimestampNs: now.UnixNano(),
		Timestamp:   now,
		Type:        "DISCONNECT_WORKER_START",
		Target:      worker,
		Details:     fmt.Sprintf("disconnected %s from %s for 2.5s", worker, fi.networkName),
		Error:       errStr,
	})

	if disconnectErr == nil {
		select {
		case <-time.After(2500 * time.Millisecond):
		case <-ctx.Done():
		}

		reconnNow := time.Now()
		connErr := exec.Command("docker", "network", "connect", fi.networkName, worker).Run()
		connErrStr := ""
		if connErr != nil {
			connErrStr = strings.TrimSpace(connErr.Error())
		}

		fi.logFault(FaultRecord{
			TimestampNs: reconnNow.UnixNano(),
			Timestamp:   reconnNow,
			Type:        "DISCONNECT_WORKER_END",
			Target:      worker,
			Details:     fmt.Sprintf("reconnected %s to %s", worker, fi.networkName),
			Error:       connErrStr,
		})
	}
}
