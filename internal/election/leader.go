package election

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	"orchestra/internal/metrics"
	"orchestra/internal/store"
)

var (
	// ErrNotLeader is returned when an attempted operation fails because this node
	// is not the leader or its fencing token check failed.
	ErrNotLeader = errors.New("fencing check failed: node is not the active leader")

	// ErrTxnFailed is returned when compare conditions other than fencing fail in a fenced transaction.
	ErrTxnFailed = errors.New("fenced transaction failed: compare condition not met")
)

/*
Distributed Systems Invariants & Theoretical Foundations:

1. What etcd guarantees:
   - Linearizable consistency for reads and writes backed by the Raft consensus algorithm.
   - Atomicity and serializability via multi-key compare-and-swap transactions (Txn).
   - Lease-based time-to-live (TTL) mechanisms: keys attached to a lease are automatically purged
     when the lease expires or is revoked after heartbeat failures.
   - Strictly monotonic 64-bit revisions across all state changes (Header.Revision and CreateRevision).

2. What a fencing problem is:
   - In distributed scheduling, a node may continue executing as if it were the leader when it has
     in fact lost leadership (a "zombie leader").
   - Common causes include stop-the-world garbage collection pauses, CPU starvation, OS thread
     suspension, or asymmetric network partitions where the leader cannot renew its lease with etcd.
   - While the ex-leader is paused, its etcd lease expires. The remaining cluster members detect
     this expiry and elect a new leader, which obtains a higher term and begins reconciling state.
   - When the zombie ex-leader wakes up, if it were to write blindly to storage without checking its
     status, it would clobber or corrupt state changes committed by the legitimate successor leader.

3. Why the check must be inside the same transaction as the write:
   - Checking leadership before writing (e.g., `if candidate.IsLeader() { etcd.Put(...) }`) introduces
     a fatal Time-of-Check to Time-of-Use (TOCTOU) race condition.
   - The node could lose leadership (via GC pause or partition) immediately after the check succeeds
     but before the write is executed.
   - By placing the fencing condition `Compare(CreateRevision(electionKey) == term)` inside the
     exact same atomic etcd transaction (`Txn`) as the mutation operations, etcd verifies that the
     leader's lease key is still active at the exact serialization instant the write is committed
     to the Raft log. If the node lost leadership, the transaction evaluates to false and is rejected
     atomically with zero side effects.
*/

// LeaderInfo represents identity and connectivity endpoints advertised by the elected leader.
type LeaderInfo struct {
	NodeID   string `json:"node_id"`
	GrpcAddr string `json:"grpc_addr"`
}

// EncodeLeaderInfo serializes leader identity and address for election campaign.
func EncodeLeaderInfo(nodeID, grpcAddr string) string {
	data, _ := json.Marshal(LeaderInfo{
		NodeID:   nodeID,
		GrpcAddr: grpcAddr,
	})
	return string(data)
}

// ParseLeaderValue extracts LeaderInfo from etcd election key value.
// It handles JSON structures, raw host:port strings, or bare node IDs.
func ParseLeaderValue(val string) (LeaderInfo, error) {
	var info LeaderInfo
	if err := json.Unmarshal([]byte(val), &info); err == nil && info.NodeID != "" {
		return info, nil
	}
	if strings.Contains(val, ":") {
		return LeaderInfo{NodeID: val, GrpcAddr: val}, nil
	}
	return LeaderInfo{NodeID: val, GrpcAddr: ""}, nil
}

// Candidate represents an orchestrator node participating in leader election.
type Candidate struct {
	nodeID         string
	campaignValue  string
	client         *clientv3.Client
	electionPrefix string
	sessionTTLSec  int
	logger         *slog.Logger

	onElected func(leaderCtx context.Context, term int64) error
	onRevoked func()

	// mu protects all leadership lifecycle state, session, and leader context cancellation
	// Invariant protected: Synchronizes leadership state transitions, session handles, and leader context cancellation
	mu           sync.RWMutex
	session      *concurrency.Session
	election     *concurrency.Election
	leaderKey    string
	term         int64
	isLeader     bool
	leaderCancel context.CancelFunc
	leaderCtx    context.Context
}

// Config specifies election parameters for Candidate initialization.
type Config struct {
	NodeID         string
	CampaignValue  string
	Client         *clientv3.Client
	ElectionPrefix string
	SessionTTLSec  int
	Logger         *slog.Logger
	OnElected      func(leaderCtx context.Context, term int64) error
	OnRevoked      func()
}

// NewCandidate constructs a Candidate instance for participating in leader election.
func NewCandidate(cfg Config) (*Candidate, error) {
	if cfg.Client == nil {
		return nil, errors.New("etcd client cannot be nil")
	}
	if cfg.NodeID == "" {
		return nil, errors.New("node ID cannot be empty")
	}
	ttl := cfg.SessionTTLSec
	if ttl <= 0 {
		ttl = 5 // Default session TTL 5s
	}
	prefix := cfg.ElectionPrefix
	if prefix == "" {
		prefix = store.ElectionKey
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	campaignVal := cfg.CampaignValue
	if campaignVal == "" {
		campaignVal = cfg.NodeID
	}

	return &Candidate{
		nodeID:         cfg.NodeID,
		campaignValue:  campaignVal,
		client:         cfg.Client,
		electionPrefix: prefix,
		sessionTTLSec:  ttl,
		logger:         logger.With(slog.String("component", "election"), slog.String("node_id", cfg.NodeID)),
		onElected:      cfg.OnElected,
		onRevoked:      cfg.OnRevoked,
	}, nil
}

// IsLeader reports whether this candidate currently holds active cluster leadership.
func (c *Candidate) IsLeader() bool {
	// Invariant protected: Read lock ensures consistent snapshot of leadership status
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isLeader
}

// Term returns the active fencing token (the election key's CreateRevision), or 0 if not leader.
func (c *Candidate) Term() int64 {
	// Invariant protected: Read lock ensures atomic access to the active term/fencing token
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.term
}

// LeaderKey returns the etcd lease key for the current leadership term.
func (c *Candidate) LeaderKey() string {
	// Invariant protected: Read lock ensures safe access to the active election key string
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.leaderKey
}

// LeaderContext returns the active leadership context if leader, or context.Background().
func (c *Candidate) LeaderContext() context.Context {
	// Invariant protected: Read lock ensures safe access to active leader context
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.leaderCtx != nil {
		return c.leaderCtx
	}
	return context.Background()
}

// Campaign blocks campaigning for leadership until the parent context is cancelled.
// It automatically re-campaigns after network disconnects or lease loss.
func (c *Candidate) Campaign(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			c.logger.Info("campaign loop stopping on parent context cancellation")
			c.stepDown()
			return ctx.Err()
		default:
		}

		session, err := concurrency.NewSession(c.client,
			concurrency.WithTTL(c.sessionTTLSec),
			concurrency.WithContext(ctx),
		)
		if err != nil {
			c.logger.Error("failed to create etcd election session, retrying in 1s", slog.Any("error", err))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1 * time.Second):
				continue
			}
		}

		election := concurrency.NewElection(session, c.electionPrefix)

		// Invariant protected: Mutex synchronizes assignment of session and election handle
		c.mu.Lock()
		c.session = session
		c.election = election
		c.mu.Unlock()

		c.logger.Info("campaigning for leader election", slog.String("prefix", c.electionPrefix))

		// Campaign blocks until this candidate is elected or ctx is cancelled
		if err := election.Campaign(ctx, c.campaignValue); err != nil {
			if errors.Is(err, context.Canceled) {
				_ = session.Close()
				return err
			}
			c.logger.Warn("election campaign interrupted, re-campaigning", slog.Any("error", err))
			_ = session.Close()
			continue
		}

		// Election succeeded! Retrieve election key details to establish fencing token
		leaderKey := election.Key()
		term := election.Rev()

		// If election.Rev() is 0 (or not populated in some versions), read create revision directly
		if term == 0 {
			resp, err := c.client.Get(ctx, leaderKey)
			if err != nil || len(resp.Kvs) == 0 {
				c.logger.Error("failed to inspect election key revision", slog.Any("error", err))
				_ = election.Resign(ctx)
				_ = session.Close()
				continue
			}
			term = resp.Kvs[0].CreateRevision
		}

		leaderCtx, cancelLeader := context.WithCancel(ctx)

		// Invariant protected: Mutex protects publication of leadership status, term, and leaderCtx
		c.mu.Lock()
		c.isLeader = true
		c.term = term
		c.leaderKey = leaderKey
		c.leaderCtx = leaderCtx
		c.leaderCancel = cancelLeader
		c.mu.Unlock()

		// Update Prometheus leadership metrics
		metrics.IsLeader.Set(1)
		metrics.LeaderTerm.Set(float64(term))
		metrics.LeaderElectionsTotal.Inc()

		c.logger.Info("elected as active cluster leader",
			slog.Int64("term", term),
			slog.String("election_key", leaderKey),
		)

		// Monitor session heartbeat in background; cancel leaderCtx immediately if session is lost
		sessionLost := make(chan struct{})
		go func(s *concurrency.Session, cancelFn context.CancelFunc) {
			select {
			case <-s.Done():
				c.logger.Warn("etcd election lease expired or lost - cancelling leader context immediately")
				cancelFn()
				close(sessionLost)
			case <-leaderCtx.Done():
				return
			}
		}(session, cancelLeader)

		// Invoke onElected callback with leaderCtx and fencing term
		if c.onElected != nil {
			if err := c.onElected(leaderCtx, term); err != nil {
				c.logger.Error("onElected callback failed, resigning leadership", slog.Any("error", err))
				cancelLeader()
				_ = election.Resign(ctx)
				_ = session.Close()
				c.stepDown()
				continue
			}
		}

		// Wait until session is lost or parent context cancelled
		select {
		case <-sessionLost:
			c.logger.Warn("leadership revoked due to session loss")
		case <-ctx.Done():
			c.logger.Info("stepping down leadership due to shutdown")
		}

		cancelLeader()
		c.stepDown()
		_ = session.Close()
	}
}

// Run is an alias for Campaign to maintain backward compatibility.
func (c *Candidate) Run(ctx context.Context) error {
	return c.Campaign(ctx)
}

// Fenced executes ops in an etcd transaction conditioned on this candidate still being leader.
// Condition: The election key still exists and its CreateRevision matches this candidate's term.
// If the condition fails, it cancels leaderCtx immediately and returns ErrNotLeader.
func (c *Candidate) Fenced(ops ...clientv3.Op) (*clientv3.TxnResponse, error) {
	return c.FencedContext(c.LeaderContext(), nil, ops...)
}

// FencedContext executes ops in an etcd transaction conditioned on leadership and optional extra compare predicates.
// Invariant protected: Fencing check prevents zombie ex-leaders from mutating etcd state after leadership loss.
func (c *Candidate) FencedContext(ctx context.Context, extraCmps []clientv3.Cmp, ops ...clientv3.Op) (*clientv3.TxnResponse, error) {
	c.mu.RLock()
	isLeader := c.isLeader
	term := c.term
	leaderKey := c.leaderKey
	cancelFn := c.leaderCancel
	c.mu.RUnlock()

	if !isLeader || term <= 0 || leaderKey == "" {
		if cancelFn != nil {
			cancelFn()
		}
		return nil, ErrNotLeader
	}

	// Invariant protected: The election key CreateRevision check is executed inside the exact same
	// transaction as the mutating ops, preventing Time-of-Check to Time-of-Use (TOCTOU) split-brain writes.
	fencingCmp := clientv3.Compare(clientv3.CreateRevision(leaderKey), "=", term)
	cmps := make([]clientv3.Cmp, 0, len(extraCmps)+1)
	cmps = append(cmps, fencingCmp)
	cmps = append(cmps, extraCmps...)

	txn := c.client.Txn(ctx).If(cmps...).Then(ops...)
	resp, err := txn.Commit()
	if err != nil {
		return nil, fmt.Errorf("fenced txn commit failed: %w", err)
	}

	if !resp.Succeeded {
		// Fencing check or extra comparison failed. Check if leaderKey still matches our term.
		checkResp, checkErr := c.client.Get(ctx, leaderKey)
		if checkErr != nil || len(checkResp.Kvs) == 0 || checkResp.Kvs[0].CreateRevision != term {
			// Leadership was lost! Cancel leaderCtx immediately so all leader-only work halts.
			metrics.FencedWriteRejectionsTotal.Inc()
			c.logger.Error("fencing check failed in txn - canceling leader context",
				slog.Int64("expected_term", term),
				slog.String("leader_key", leaderKey),
			)
			if cancelFn != nil {
				cancelFn()
			}
			c.stepDown()
			return resp, ErrNotLeader
		}

		// Extra compare conditions (such as CAS ModRevision) failed while still leader
		return resp, ErrTxnFailed
	}

	return resp, nil
}

// stepDown resets leadership flags, cancels leaderCtx, and invokes onRevoked.
func (c *Candidate) stepDown() {
	// Invariant protected: Mutex synchronizes leadership teardown, term invalidation, and context cancellation
	c.mu.Lock()
	wasLeader := c.isLeader
	cancelFn := c.leaderCancel
	c.isLeader = false
	c.term = 0
	c.leaderKey = ""
	c.leaderCancel = nil
	c.mu.Unlock()

	// Update Prometheus leadership metrics
	metrics.IsLeader.Set(0)
	metrics.LeaderTerm.Set(0)

	if cancelFn != nil {
		cancelFn()
	}

	if wasLeader && c.onRevoked != nil {
		c.onRevoked()
	}
}

// Resign voluntarily steps down if currently leader.
func (c *Candidate) Resign(ctx context.Context) error {
	// Invariant protected: Mutex protects access to active election handle during voluntary step-down
	c.mu.Lock()
	election := c.election
	c.mu.Unlock()

	c.stepDown()
	if election != nil {
		return election.Resign(ctx)
	}
	return nil
}

// SessionLeaseID returns the current lease ID of the candidate's session, if active.
func (c *Candidate) SessionLeaseID() clientv3.LeaseID {
	// Invariant protected: Read lock ensures safe concurrent access to session lease handle
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.session != nil {
		return c.session.Lease()
	}
	return clientv3.NoLease
}
