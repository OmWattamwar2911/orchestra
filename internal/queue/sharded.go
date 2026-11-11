package queue

import (
	"container/heap"
	"sync"
	"sync/atomic"

	pb "orchestra/api/proto/orchestra/v1"
	"orchestra/internal/metrics"
)

var (
	_ TaskQueue = (*PriorityQueue)(nil)
	_ TaskQueue = (*ShardedPriorityQueue)(nil)
)

// queueShard represents an isolated sub-queue with its own mutex and priority heap.
type queueShard struct {
	mu      sync.Mutex
	h       priorityHeap
	nextSeq int64
}

// ShardedPriorityQueue is an N-shard concurrent priority queue that partitions tasks
// across shards by hashing task ID. It uses per-shard mutexes to eliminate central lock contention
// under heavy concurrent producer and consumer loads, backed by atomic counters for lock-free length tracking.
type ShardedPriorityQueue struct {
	shards    []queueShard
	numShards int
	mask      uint64
	totalLen  atomic.Int64
	cursor    atomic.Uint64
}

// fnv64 computes a 64-bit FNV-1a hash of a string.
func fnv64(s string) uint64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

// nextPowerOf2 rounds an integer up to the nearest power of 2.
func nextPowerOf2(n int) int {
	if n <= 1 {
		return 1
	}
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	return n + 1
}

// NewShardedPriorityQueue creates a new ShardedPriorityQueue with N shards.
// If n <= 0, it defaults to 16 shards. n is rounded up to the nearest power of 2.
func NewShardedPriorityQueue(n int) *ShardedPriorityQueue {
	if n <= 0 {
		n = 16
	}
	n = nextPowerOf2(n)

	sq := &ShardedPriorityQueue{
		shards:    make([]queueShard, n),
		numShards: n,
		mask:      uint64(n - 1),
	}

	for i := 0; i < n; i++ {
		sq.shards[i].h = make(priorityHeap, 0)
		heap.Init(&sq.shards[i].h)
	}

	return sq
}

// Enqueue hashes the task ID and inserts the task assignment into the corresponding shard.
func (sq *ShardedPriorityQueue) Enqueue(assignment *pb.TaskAssignment, priority int) {
	if assignment == nil {
		return
	}

	taskID := ""
	if assignment.Task != nil {
		taskID = assignment.Task.Id
	}
	if taskID == "" {
		taskID = assignment.WorkflowId
	}

	h := fnv64(taskID)
	shardIdx := int(h & sq.mask)

	shard := &sq.shards[shardIdx]
	shard.mu.Lock()
	shard.nextSeq++
	it := &item{
		assignment: assignment,
		priority:   priority,
		seq:        shard.nextSeq,
	}
	heap.Push(&shard.h, it)
	shard.mu.Unlock()

	newLen := sq.totalLen.Add(1)
	metrics.QueueDepth.Set(float64(newLen))
}

// Dequeue extracts the highest priority item across probed shards using power-of-two choices
// and striped lock acquisition, returning nil when the queue is empty.
func (sq *ShardedPriorityQueue) Dequeue() *pb.TaskAssignment {
	if sq.totalLen.Load() <= 0 {
		return nil
	}

	start := sq.cursor.Add(1)

	// Phase 1: TryLock power-of-two candidate shards to minimize contention
	s1 := int(start & sq.mask)
	s2 := int((start + 1) & sq.mask)

	// Try non-blocking acquisition of s1
	shard1 := &sq.shards[s1]
	shard2 := &sq.shards[s2]

	var item1, item2 *item
	got1 := shard1.mu.TryLock()
	if got1 {
		if len(shard1.h) > 0 {
			item1 = shard1.h[0]
		} else {
			shard1.mu.Unlock()
			got1 = false
		}
	}

	got2 := shard2.mu.TryLock()
	if got2 {
		if len(shard2.h) > 0 {
			item2 = shard2.h[0]
		} else {
			shard2.mu.Unlock()
			got2 = false
		}
	}

	if got1 && got2 {
		// Pick highest priority
		if item1.priority >= item2.priority {
			shard2.mu.Unlock()
			popped := heap.Pop(&shard1.h).(*item)
			shard1.mu.Unlock()
			newLen := sq.totalLen.Add(-1)
			metrics.QueueDepth.Set(float64(newLen))
			return popped.assignment
		} else {
			shard1.mu.Unlock()
			popped := heap.Pop(&shard2.h).(*item)
			shard2.mu.Unlock()
			newLen := sq.totalLen.Add(-1)
			metrics.QueueDepth.Set(float64(newLen))
			return popped.assignment
		}
	} else if got1 {
		popped := heap.Pop(&shard1.h).(*item)
		shard1.mu.Unlock()
		newLen := sq.totalLen.Add(-1)
		metrics.QueueDepth.Set(float64(newLen))
		return popped.assignment
	} else if got2 {
		popped := heap.Pop(&shard2.h).(*item)
		shard2.mu.Unlock()
		newLen := sq.totalLen.Add(-1)
		metrics.QueueDepth.Set(float64(newLen))
		return popped.assignment
	}

	// Phase 2: Sweep shards in round-robin order with TryLock
	for i := 0; i < sq.numShards; i++ {
		idx := int((start + uint64(i)) & sq.mask)
		shard := &sq.shards[idx]
		if shard.mu.TryLock() {
			if len(shard.h) > 0 {
				popped := heap.Pop(&shard.h).(*item)
				shard.mu.Unlock()
				newLen := sq.totalLen.Add(-1)
				metrics.QueueDepth.Set(float64(newLen))
				return popped.assignment
			}
			shard.mu.Unlock()
		}
	}

	if sq.totalLen.Load() <= 0 {
		return nil
	}

	// Phase 3: Blocking sweep fallback
	for i := 0; i < sq.numShards; i++ {
		idx := int((start + uint64(i)) & sq.mask)
		shard := &sq.shards[idx]
		shard.mu.Lock()
		if len(shard.h) > 0 {
			popped := heap.Pop(&shard.h).(*item)
			shard.mu.Unlock()
			newLen := sq.totalLen.Add(-1)
			metrics.QueueDepth.Set(float64(newLen))
			return popped.assignment
		}
		shard.mu.Unlock()
	}

	return nil
}

// Len returns the atomic total length across all shards in O(1) without locking.
func (sq *ShardedPriorityQueue) Len() int {
	l := sq.totalLen.Load()
	if l < 0 {
		return 0
	}
	return int(l)
}

// Depth returns the queue depth for observability and backpressure.
func (sq *ShardedPriorityQueue) Depth() int {
	return sq.Len()
}

// NumShards returns the number of partitioned shards.
func (sq *ShardedPriorityQueue) NumShards() int {
	return sq.numShards
}
