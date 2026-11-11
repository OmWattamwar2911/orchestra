package election_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
	"orchestra/internal/election"
)

// startTestEtcd boots a hermetic embedded etcd cluster node for integration testing.
func startTestEtcd(t *testing.T) (*embed.Etcd, *clientv3.Client, func()) {
	t.Helper()
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("etcd-elect-test-%d", time.Now().UnixNano()))

	cfg := embed.NewConfig()
	cfg.Dir = dir
	cfg.LogLevel = "error"

	clientURL, _ := url.Parse("http://127.0.0.1:0")
	peerURL, _ := url.Parse("http://127.0.0.1:0")
	cfg.ListenClientUrls = []url.URL{*clientURL}
	cfg.AdvertiseClientUrls = []url.URL{*clientURL}
	cfg.ListenPeerUrls = []url.URL{*peerURL}
	cfg.AdvertisePeerUrls = []url.URL{*peerURL}
	cfg.InitialCluster = cfg.InitialClusterFromName(cfg.Name)

	e, err := embed.StartEtcd(cfg)
	if err != nil {
		t.Fatalf("failed to start embedded etcd: %v", err)
	}

	select {
	case <-e.Server.ReadyNotify():
	case <-time.After(10 * time.Second):
		e.Close()
		_ = os.RemoveAll(dir)
		t.Fatal("embedded etcd startup timeout")
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{e.Clients[0].Addr().String()},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		e.Close()
		_ = os.RemoveAll(dir)
		t.Fatalf("failed to initialize etcd client: %v", err)
	}

	return e, client, func() {
		_ = client.Close()
		e.Close()
		_ = os.RemoveAll(dir)
	}
}

// Test 1: 3 candidates, exactly one leader.
func TestLeaderElection_ThreeCandidates_ExactlyOneLeader(t *testing.T) {
	_, client, cleanup := startTestEtcd(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var mu sync.Mutex
	electedCount := 0
	leaderElectedCh := make(chan string, 3)

	cands := make([]*election.Candidate, 3)
	for i := 0; i < 3; i++ {
		nodeID := fmt.Sprintf("candidate-%d", i+1)
		c, err := election.NewCandidate(election.Config{
			NodeID:         nodeID,
			Client:         client,
			ElectionPrefix: "/test/election/three-nodes",
			SessionTTLSec:  5,
			OnElected: func(leaderCtx context.Context, term int64) error {
				mu.Lock()
				electedCount++
				mu.Unlock()
				leaderElectedCh <- nodeID
				return nil
			},
		})
		if err != nil {
			t.Fatalf("failed to create candidate %s: %v", nodeID, err)
		}
		cands[i] = c

		go func(cand *election.Candidate) {
			_ = cand.Campaign(ctx)
		}(c)
	}

	// Wait for election
	var winningNode string
	select {
	case winningNode = <-leaderElectedCh:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for a leader to be elected")
	}

	// Let the cluster settle briefly
	time.Sleep(500 * time.Millisecond)

	// Invariant: Exactly one candidate must report IsLeader() == true
	activeLeaders := 0
	for _, c := range cands {
		if c.IsLeader() {
			activeLeaders++
			if c.Term() <= 0 {
				t.Fatalf("leader term must be positive, got %d", c.Term())
			}
			if c.LeaderKey() == "" {
				t.Fatal("leader key must not be empty")
			}

			// Verify the leader can perform a fenced write
			resp, err := c.Fenced(clientv3.OpPut("/test/fenced/key", "leader-data"))
			if err != nil || !resp.Succeeded {
				t.Fatalf("fenced write by active leader failed: %v", err)
			}
		}
	}

	if activeLeaders != 1 {
		t.Fatalf("expected exactly 1 active leader among 3 candidates, got %d (elected node: %s)", activeLeaders, winningNode)
	}
}

// Test 2: Kill the leader, a different candidate becomes leader within 10 seconds.
func TestLeaderElection_KillLeader_FailoverWithin10Seconds(t *testing.T) {
	_, client, cleanup := startTestEtcd(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	var mu sync.Mutex
	electedHistory := make([]struct {
		nodeID string
		term   int64
	}, 0)
	electedCh := make(chan struct {
		nodeID string
		term   int64
	}, 5)

	candCtxs := make([]context.Context, 3)
	candCancels := make([]context.CancelFunc, 3)
	cands := make([]*election.Candidate, 3)

	for i := 0; i < 3; i++ {
		nodeID := fmt.Sprintf("node-%d", i+1)
		candCtxs[i], candCancels[i] = context.WithCancel(ctx)
		c, err := election.NewCandidate(election.Config{
			NodeID:         nodeID,
			Client:         client,
			ElectionPrefix: "/test/election/failover",
			SessionTTLSec:  5,
			OnElected: func(leaderCtx context.Context, term int64) error {
				mu.Lock()
				electedHistory = append(electedHistory, struct {
					nodeID string
					term   int64
				}{nodeID, term})
				mu.Unlock()

				electedCh <- struct {
					nodeID string
					term   int64
				}{nodeID, term}
				return nil
			},
		})
		if err != nil {
			t.Fatalf("failed to create candidate %s: %v", nodeID, err)
		}
		cands[i] = c

		go func(idx int) {
			_ = cands[idx].Campaign(candCtxs[idx])
		}(i)
	}

	// 1. Wait for initial leader
	var firstLeader struct {
		nodeID string
		term   int64
	}
	select {
	case firstLeader = <-electedCh:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for initial leader")
	}

	// Identify leader index
	leaderIdx := -1
	for i, c := range cands {
		if c.IsLeader() {
			leaderIdx = i
			break
		}
	}
	if leaderIdx == -1 {
		t.Fatalf("could not find candidate matching first leader %s", firstLeader.nodeID)
	}

	firstLeaderCandidate := cands[leaderIdx]
	firstLeaderCtx := firstLeaderCandidate.LeaderContext()

	// 2. Kill the leader by cancelling its campaign context
	killTime := time.Now()
	candCancels[leaderIdx]()

	// Assert leaderCtx is cancelled immediately
	select {
	case <-firstLeaderCtx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("first leader's leaderCtx was not cancelled upon death/stepdown")
	}

	// 3. Assert a different candidate becomes leader within 10 seconds
	var successor struct {
		nodeID string
		term   int64
	}
	select {
	case successor = <-electedCh:
		elapsed := time.Since(killTime)
		if elapsed > 10*time.Second {
			t.Fatalf("failover took longer than 10 seconds: %v", elapsed)
		}
		if successor.nodeID == firstLeader.nodeID {
			t.Fatalf("successor should be a different candidate, got same: %s", successor.nodeID)
		}
		if successor.term <= firstLeader.term {
			t.Fatalf("successor term (%d) must be greater than previous term (%d)", successor.term, firstLeader.term)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("failover timeout: no new leader elected within 10 seconds")
	}

	// Verify successor can perform fenced write
	for _, c := range cands {
		if c.IsLeader() {
			resp, err := c.Fenced(clientv3.OpPut("/test/fenced/after-failover", "new-leader-data"))
			if err != nil || !resp.Succeeded {
				t.Fatalf("successor fenced write failed: %v", err)
			}
		}
	}
}

// Test 3: Zombie test: simulate an old leader that is paused past its lease expiry,
// then resumes and attempts a Fenced write after a new leader exists. Assert the write is rejected.
func TestLeaderElection_ZombieLeader_FencedWriteRejected(t *testing.T) {
	_, client, cleanup := startTestEtcd(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	leader1Elected := make(chan int64, 1)
	leader2Elected := make(chan int64, 1)

	// Candidate 1 (will become the zombie)
	cand1, err := election.NewCandidate(election.Config{
		NodeID:         "zombie-node-1",
		Client:         client,
		ElectionPrefix: "/test/election/zombie-protection",
		SessionTTLSec:  5,
		OnElected: func(leaderCtx context.Context, term int64) error {
			leader1Elected <- term
			return nil
		},
	})
	if err != nil {
		t.Fatalf("failed to create cand1: %v", err)
	}

	// Candidate 2 (will become the legitimate successor)
	cand2, err := election.NewCandidate(election.Config{
		NodeID:         "successor-node-2",
		Client:         client,
		ElectionPrefix: "/test/election/zombie-protection",
		SessionTTLSec:  5,
		OnElected: func(leaderCtx context.Context, term int64) error {
			leader2Elected <- term
			return nil
		},
	})
	if err != nil {
		t.Fatalf("failed to create cand2: %v", err)
	}

	cand1Ctx, cancelCand1 := context.WithCancel(ctx)
	defer cancelCand1()

	go func() {
		_ = cand1.Campaign(cand1Ctx)
	}()

	// 1. Wait for cand1 to win election
	var term1 int64
	select {
	case term1 = <-leader1Elected:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for cand1 to become leader")
	}

	targetKey := "/orchestra/test/critical-data"

	// 2. Candidate 1 performs a legitimate fenced write during its term
	resp, err := cand1.Fenced(clientv3.OpPut(targetKey, "term1-legitimate-data"))
	if err != nil || !resp.Succeeded {
		t.Fatalf("initial fenced write by leader 1 failed: %v", err)
	}

	// Verify etcd contains term1 data
	getResp, err := client.Get(ctx, targetKey)
	if err != nil || len(getResp.Kvs) == 0 || string(getResp.Kvs[0].Value) != "term1-legitimate-data" {
		t.Fatalf("expected term1 data in etcd")
	}

	// 3. Simulate pausing Candidate 1 (e.g. stop-the-world GC pause, network partition):
	// While Candidate 1's goroutine is "paused", its etcd lease expires.
	// We simulate this by revoking Candidate 1's session lease directly via etcd server.
	zombieLeaseID := cand1.SessionLeaseID()
	if zombieLeaseID == clientv3.NoLease {
		t.Fatal("cand1 session lease ID is missing")
	}

	// Directly revoke cand1's lease on etcd to simulate lease expiry during pause
	_, err = client.Revoke(ctx, zombieLeaseID)
	if err != nil {
		t.Fatalf("failed to revoke cand1 lease: %v", err)
	}

	// 4. Start Candidate 2. With cand1's lease expired, cand2 campaigns and wins leadership
	go func() {
		_ = cand2.Campaign(ctx)
	}()

	var term2 int64
	select {
	case term2 = <-leader2Elected:
		if term2 <= term1 {
			t.Fatalf("expected successor term %d > zombie term %d", term2, term1)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for cand2 to win election after cand1 lease revocation")
	}

	// Successor commits new valid state
	resp2, err := cand2.Fenced(clientv3.OpPut(targetKey, "term2-legitimate-data"))
	if err != nil || !resp2.Succeeded {
		t.Fatalf("successor fenced write failed: %v", err)
	}

	// 5. Now Candidate 1 (the Zombie) "resumes" from its pause.
	// Candidate 1 still has its old term1 and attempts a Fenced write to overwrite the critical key.
	zombieResp, zombieErr := cand1.Fenced(clientv3.OpPut(targetKey, "zombie-corrupting-write"))

	// Invariant protected: Assert that the zombie write is strictly REJECTED
	if zombieErr == nil && (zombieResp != nil && zombieResp.Succeeded) {
		t.Fatal("CRITICAL INVARIANT VIOLATION: zombie ex-leader write succeeded! Fencing check failed.")
	}

	if zombieErr != election.ErrNotLeader {
		t.Logf("zombie error received: %v (expected ErrNotLeader)", zombieErr)
	}

	// Verify that Candidate 1's leaderCtx was cancelled as a consequence of the failed fencing check
	select {
	case <-cand1.LeaderContext().Done():
		// Success: leader context was cancelled immediately
	default:
		t.Fatal("expected zombie leader context to be cancelled after failed fencing check")
	}

	// 6. Assert that etcd state was NOT corrupted and still retains the legitimate successor's data
	finalGet, err := client.Get(ctx, targetKey)
	if err != nil || len(finalGet.Kvs) == 0 {
		t.Fatalf("failed to read back target key from etcd: %v", err)
	}

	finalVal := string(finalGet.Kvs[0].Value)
	if finalVal != "term2-legitimate-data" {
		t.Fatalf("DATA CORRUPTION DETECTED: expected 'term2-legitimate-data', got '%s'", finalVal)
	}
}
