package queue

import (
	"container/heap"
	"sync"

	pb "orchestra/api/proto/orchestra/v1"
	"orchestra/internal/metrics"
)

// TaskQueue is the interface for a thread-safe task priority queue.
// This interface allows swapping in a sharded or persistent queue implementation later.
type TaskQueue interface {
	Enqueue(assignment *pb.TaskAssignment, priority int)
	Dequeue() *pb.TaskAssignment
	Len() int
	Depth() int
}

// Item represents a task assignment stored in the internal heap.
type item struct {
	assignment *pb.TaskAssignment
	priority   int
	seq        int64
	index      int
}

// priorityHeap implements container/heap.Interface.
type priorityHeap []*item

func (h priorityHeap) Len() int { return len(h) }

func (h priorityHeap) Less(i, j int) bool {
	// Higher priority dequeued first; if priorities equal, FIFO based on insertion sequence
	if h[i].priority != h[j].priority {
		return h[i].priority > h[j].priority
	}
	return h[i].seq < h[j].seq
}

func (h priorityHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *priorityHeap) Push(x any) {
	n := len(*h)
	it := x.(*item)
	it.index = n
	*h = append(*h, it)
}

func (h *priorityHeap) Pop() any {
	old := *h
	n := len(old)
	it := old[n-1]
	old[n-1] = nil
	it.index = -1
	*h = old[0 : n-1]
	return it
}

// PriorityQueue is a thread-safe priority queue for tasks awaiting worker dispatch.
type PriorityQueue struct {
	// Invariant protected: Mutex protects the internal binary heap invariants and sequence counter across concurrent workers
	mu      sync.Mutex
	h       priorityHeap
	nextSeq int64
}

// NewPriorityQueue constructs a thread-safe priority queue.
func NewPriorityQueue() *PriorityQueue {
	pq := &PriorityQueue{
		h: make(priorityHeap, 0),
	}
	heap.Init(&pq.h)
	return pq
}

// Enqueue inserts a task assignment with the specified priority into the queue.
func (pq *PriorityQueue) Enqueue(assignment *pb.TaskAssignment, priority int) {
	// Invariant protected: Queue lock guarantees atomic element insertion and heap invariant restoration
	pq.mu.Lock()
	defer pq.mu.Unlock()

	pq.nextSeq++
	it := &item{
		assignment: assignment,
		priority:   priority,
		seq:        pq.nextSeq,
	}
	heap.Push(&pq.h, it)
	metrics.QueueDepth.Set(float64(len(pq.h)))
}

// Dequeue extracts and returns the highest-priority (or earliest FIFO) task assignment, or nil if empty.
func (pq *PriorityQueue) Dequeue() *pb.TaskAssignment {
	// Invariant protected: Queue lock ensures atomic head extraction and bounds safety
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if len(pq.h) == 0 {
		return nil
	}

	it := heap.Pop(&pq.h).(*item)
	metrics.QueueDepth.Set(float64(len(pq.h)))
	return it.assignment
}

// Len returns the current number of queued tasks.
func (pq *PriorityQueue) Len() int {
	// Invariant protected: Queue lock guarantees thread-safe length inspection
	pq.mu.Lock()
	defer pq.mu.Unlock()
	return len(pq.h)
}

// Depth returns the current queue depth (for backpressure and metrics monitoring).
func (pq *PriorityQueue) Depth() int {
	return pq.Len()
}
