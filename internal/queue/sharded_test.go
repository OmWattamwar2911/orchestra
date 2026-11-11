package queue_test

import (
	"fmt"
	"sync"
	"testing"

	pb "orchestra/api/proto/orchestra/v1"
	"orchestra/internal/queue"
)

func TestShardedPriorityQueue_BasicAndFIFO(t *testing.T) {
	sq := queue.NewShardedPriorityQueue(4)

	if sq.Len() != 0 || sq.Depth() != 0 {
		t.Fatalf("expected empty queue, got len=%d", sq.Len())
	}
	if sq.Dequeue() != nil {
		t.Fatal("expected nil dequeue from empty queue")
	}

	// Insert tasks with varying priorities
	sq.Enqueue(&pb.TaskAssignment{WorkflowId: "wf", Task: &pb.TaskSpec{Id: "task-low"}}, 1)
	sq.Enqueue(&pb.TaskAssignment{WorkflowId: "wf", Task: &pb.TaskSpec{Id: "task-high"}}, 10)
	sq.Enqueue(&pb.TaskAssignment{WorkflowId: "wf", Task: &pb.TaskSpec{Id: "task-med"}}, 5)

	if sq.Depth() != 3 {
		t.Fatalf("expected queue depth 3, got %d", sq.Depth())
	}

	// High priority task should be dequeued first
	it1 := sq.Dequeue()
	if it1 == nil {
		t.Fatal("expected item, got nil")
	}
	if it1.GetTask().GetId() != "task-high" {
		t.Fatalf("expected task-high, got %s", it1.GetTask().GetId())
	}

	// Next items
	it2 := sq.Dequeue()
	it3 := sq.Dequeue()
	if it2 == nil || it3 == nil {
		t.Fatalf("expected 2 items, got it2=%v, it3=%v", it2, it3)
	}

	if sq.Depth() != 0 {
		t.Fatalf("expected depth 0, got %d", sq.Depth())
	}
	if sq.Dequeue() != nil {
		t.Fatal("expected nil dequeue after empty")
	}
}

func TestShardedPriorityQueue_ConcurrentAccess(t *testing.T) {
	sq := queue.NewShardedPriorityQueue(16)
	const numProducers = 12
	const itemsPerProducer = 200

	var wg sync.WaitGroup
	wg.Add(numProducers)

	for p := 0; p < numProducers; p++ {
		go func(producerID int) {
			defer wg.Done()
			for i := 0; i < itemsPerProducer; i++ {
				sq.Enqueue(&pb.TaskAssignment{
					WorkflowId: "wf-concurrent",
					Task:       &pb.TaskSpec{Id: fmt.Sprintf("p%d-task-%d", producerID, i)},
				}, i%5)
			}
		}(p)
	}

	wg.Wait()

	totalExpected := numProducers * itemsPerProducer
	if sq.Depth() != totalExpected {
		t.Fatalf("expected %d items queued, got %d", totalExpected, sq.Depth())
	}

	// Concurrently dequeue all items
	const numConsumers = 8
	var dequeuedCount sync.Map
	var consumerWg sync.WaitGroup
	consumerWg.Add(numConsumers)

	for c := 0; c < numConsumers; c++ {
		go func() {
			defer consumerWg.Done()
			for {
				item := sq.Dequeue()
				if item == nil {
					if sq.Depth() == 0 {
						return
					}
					continue
				}
				tid := item.GetTask().GetId()
				if _, loaded := dequeuedCount.LoadOrStore(tid, true); loaded {
					t.Errorf("duplicate item dequeued: %s", tid)
				}
			}
		}()
	}

	consumerWg.Wait()

	if sq.Depth() != 0 {
		t.Fatalf("expected queue depth 0, got %d", sq.Depth())
	}

	var count int
	dequeuedCount.Range(func(_, _ any) bool {
		count++
		return true
	})
	if count != totalExpected {
		t.Fatalf("expected exactly %d unique items dequeued, got %d", totalExpected, count)
	}
}
