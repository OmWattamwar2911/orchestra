package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"orchestra/internal/election"
	"orchestra/internal/store"
)

var (
	// ErrNoLeaderFound is returned when no active leader key exists under the election prefix.
	ErrNoLeaderFound = errors.New("no active leader found in etcd")
)

// LeaderDiscovery resolves and monitors the active orchestrator leader from etcd.
type LeaderDiscovery struct {
	client         *clientv3.Client
	electionPrefix string
	logger         *slog.Logger
}

// NewLeaderDiscovery creates a new leader discovery service.
func NewLeaderDiscovery(client *clientv3.Client, electionPrefix string, logger *slog.Logger) *LeaderDiscovery {
	if electionPrefix == "" {
		electionPrefix = store.ElectionKey
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &LeaderDiscovery{
		client:         client,
		electionPrefix: electionPrefix,
		logger:         logger.With(slog.String("component", "leader-discovery")),
	}
}

// DiscoverLeader queries etcd for the active leader with the lowest create revision.
func (d *LeaderDiscovery) DiscoverLeader(ctx context.Context) (*election.LeaderInfo, int64, error) {
	// Invariant protected: Linearizable read of lowest create revision identifies current Raft leader
	opts := []clientv3.OpOption{
		clientv3.WithPrefix(),
		clientv3.WithSort(clientv3.SortByCreateRevision, clientv3.SortAscend),
		clientv3.WithLimit(1),
	}

	resp, err := d.client.Get(ctx, d.electionPrefix, opts...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query leader election prefix: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return nil, 0, ErrNoLeaderFound
	}

	kv := resp.Kvs[0]
	info, err := election.ParseLeaderValue(string(kv.Value))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to parse leader data: %w", err)
	}

	return &info, kv.CreateRevision, nil
}

// WaitForLeader blocks until an active leader is elected or context is cancelled.
func (d *LeaderDiscovery) WaitForLeader(ctx context.Context, pollInterval time.Duration) (*election.LeaderInfo, int64, error) {
	if pollInterval <= 0 {
		pollInterval = 500 * time.Millisecond
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		leader, rev, err := d.DiscoverLeader(ctx)
		if err == nil {
			return leader, rev, nil
		}

		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		case <-ticker.C:
		}
	}
}

// WatchLeader streams updates whenever the leader election state changes.
func (d *LeaderDiscovery) WatchLeader(ctx context.Context) <-chan *election.LeaderInfo {
	out := make(chan *election.LeaderInfo, 4)

	go func() {
		defer close(out)

		rch := d.client.Watch(ctx, d.electionPrefix, clientv3.WithPrefix())
		var lastTerm int64
		var lastNodeID string

		for {
			select {
			case <-ctx.Done():
				return
			case wresp, ok := <-rch:
				if !ok {
					return
				}
				if wresp.Err() != nil {
					d.logger.Warn("leader watch encountered error", slog.Any("error", wresp.Err()))
					time.Sleep(500 * time.Millisecond)
					continue
				}

				// Check current leader
				leader, term, err := d.DiscoverLeader(ctx)
				if err != nil {
					// Leader may be temporarily absent during election failover
					continue
				}

				if term != lastTerm || leader.NodeID != lastNodeID {
					lastTerm = term
					lastNodeID = leader.NodeID
					d.logger.Info("discovered leader change via etcd watch",
						slog.String("leader_id", leader.NodeID),
						slog.String("grpc_addr", leader.GrpcAddr),
						slog.Int64("term", term),
					)

					select {
					case out <- leader:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return out
}
