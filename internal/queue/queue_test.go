package queue_test

import (
	"fmt"
	"sync"
	"testing"

	pb "orchestra/api/proto/orchestra/v1"
	"orchestra/internal/queue"
)

func TestPriorityQueue_OrderingAndFIFO(t *testing.T) {
	pq := queue.NewPriorityQueue()

	if pq.Len() != 0 || pq.Depth() != 0 {
		t.Fatalf("expected empty queue, got len=%d", pq.Len())
	}
	if pq.Dequeue() != nil {
		t.Fatal("expected nil dequeue from empty queue")
	}

	// Insert tasks with varying priorities
	// Low priority (1), High priority (10), Medium priority (5), Another Medium priority (5)
	pq.Enqueue(&pb.TaskAssignment{WorkflowId: "wf", Task: &pb.TaskSpec{Id: "low"}}, 1)
	pq.Enqueue(&pb.TaskAssignment{WorkflowId: "wf", Task: &pb.TaskSpec{Id: "high"}}, 10)
	pq.Enqueue(&pb.TaskAssignment{WorkflowId: "wf", Task: &pb.TaskSpec{Id: "med-1"}}, 5)
	pq.Enqueue(&pb.TaskAssignment{WorkflowId: "wf", Task: &pb.TaskSpec{Id: "med-2"}}, 5)

	if pq.Depth() != 4 {
		t.Fatalf("expected queue depth 4, got %d", pq.Depth())
	}

	// 1. First dequeued must be "high" (priority 10)
	it1 := pq.Dequeue()
	if it1 == nil || it1.GetTask().GetId() != "high" {
		t.Fatalf("expected 'high', got %v", it1)
	}

	// 2. Next two must be "med-1" then "med-2" (FIFO for equal priority 5)
	it2 := pq.Dequeue()
	if it2 == nil || it2.GetTask().GetId() != "med-1" {
		t.Fatalf("expected 'med-1', got %v", it2)
	}

	it3 := pq.Dequeue()
	if it3 == nil || it3.GetTask().GetId() != "med-2" {
		t.Fatalf("expected 'med-2', got %v", it3)
	}

	// 3. Last dequeued must be "low" (priority 1)
	it4 := pq.Dequeue()
	if it4 == nil || it4.GetTask().GetId() != "low" {
		t.Fatalf("expected 'low', got %v", it4)
	}

	if pq.Depth() != 0 {
		t.Fatalf("expected empty queue after all dequeued, got %d", pq.Depth())
	}
}

func TestPriorityQueue_ConcurrentAccess(t *testing.T) {
	pq := queue.NewPriorityQueue()
	const numProducers = 10
	const itemsPerProducer = 100

	var wg sync.WaitGroup
	wg.Add(numProducers)

	for p := 0; p < numProducers; p++ {
		go func(producerID int) {
			defer wg.Done()
			for i := 0; i < itemsPerProducer; i++ {
				pq.Enqueue(&pb.TaskAssignment{
					WorkflowId: "wf-concurrent",
					Task:       &pb.TaskSpec{Id: fmt.Sprintf("p%d-t%d", producerID, i)},
				}, i%5)
			}
		}(p)
	}

	wg.Wait()

	totalExpected := numProducers * itemsPerProducer
	if pq.Depth() != totalExpected {
		t.Fatalf("expected %d items queued, got %d", totalExpected, pq.Depth())
	}

	// Concurrently dequeue all items
	const numConsumers = 5
	dequeuedCount := sync.Map{}
	var consumerWg sync.WaitGroup
	consumerWg.Add(numConsumers)

	for c := 0; c < numConsumers; c++ {
		go func() {
			defer consumerWg.Done()
			for {
				item := pq.Dequeue()
				if item == nil {
					if pq.Depth() == 0 {
						return
					}
					continue
				}
				dequeuedCount.Store(item.GetTask().GetId(), struct{}{})
			}
		}()
	}

	consumerWg.Wait()

	actualCount := 0
	dequeuedCount.Range(func(k, v any) bool {
		actualCount++
		return true
	})

	if actualCount != totalExpected {
		t.Fatalf("expected %d dequeued items, got %d", totalExpected, actualCount)
	}
}
