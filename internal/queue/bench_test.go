package queue_test

import (
	"fmt"
	"math/rand"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	pb "orchestra/api/proto/orchestra/v1"
	"orchestra/internal/queue"
)

func runParallelEnqueue(b *testing.B, q queue.TaskQueue, goroutines int) {
	origGOMAXPROCS := runtime.GOMAXPROCS(goroutines)
	defer runtime.GOMAXPROCS(origGOMAXPROCS)

	b.SetParallelism(1) // with GOMAXPROCS=goroutines, this creates exactly `goroutines` workers
	b.ResetTimer()

	var counter atomic.Int64
	b.RunParallel(func(pbState *testing.PB) {
		for pbState.Next() {
			n := counter.Add(1)
			q.Enqueue(&pb.TaskAssignment{
				WorkflowId: "bench-wf",
				Task: &pb.TaskSpec{
					Id: fmt.Sprintf("task-%d", n%10000),
				},
			}, int(n%10))
		}
	})
}

func runParallelMixed(b *testing.B, q queue.TaskQueue, goroutines int) {
	origGOMAXPROCS := runtime.GOMAXPROCS(goroutines)
	defer runtime.GOMAXPROCS(origGOMAXPROCS)

	// Pre-fill with items
	for i := 0; i < 5000; i++ {
		q.Enqueue(&pb.TaskAssignment{
			WorkflowId: "bench-wf",
			Task: &pb.TaskSpec{
				Id: fmt.Sprintf("prefill-%d", i),
			},
		}, i%10)
	}

	b.SetParallelism(1)
	b.ResetTimer()

	var counter atomic.Int64
	b.RunParallel(func(pbState *testing.PB) {
		for pbState.Next() {
			n := counter.Add(1)
			if n%2 == 0 {
				q.Enqueue(&pb.TaskAssignment{
					WorkflowId: "bench-wf",
					Task: &pb.TaskSpec{
						Id: fmt.Sprintf("task-%d", n%10000),
					},
				}, int(n%10))
			} else {
				q.Dequeue()
			}
		}
	})
}

// BenchmarkEnqueue runs parallel enqueue benchmarks across 1, 8, 32, 128 goroutines
func BenchmarkEnqueue(b *testing.B) {
	goroutinesList := []int{1, 8, 32, 128}

	for _, g := range goroutinesList {
		b.Run(fmt.Sprintf("Coarse/Goroutines=%d", g), func(b *testing.B) {
			q := queue.NewPriorityQueue()
			runParallelEnqueue(b, q, g)
		})

		b.Run(fmt.Sprintf("Sharded/Goroutines=%d", g), func(b *testing.B) {
			q := queue.NewShardedPriorityQueue(32)
			runParallelEnqueue(b, q, g)
		})
	}
}

// BenchmarkMixed runs 50% Enqueue / 50% Dequeue across 1, 8, 32, 128 goroutines
func BenchmarkMixed(b *testing.B) {
	goroutinesList := []int{1, 8, 32, 128}

	for _, g := range goroutinesList {
		b.Run(fmt.Sprintf("Coarse/Goroutines=%d", g), func(b *testing.B) {
			q := queue.NewPriorityQueue()
			runParallelMixed(b, q, g)
		})

		b.Run(fmt.Sprintf("Sharded/Goroutines=%d", g), func(b *testing.B) {
			q := queue.NewShardedPriorityQueue(32)
			runParallelMixed(b, q, g)
		})
	}
}

// BenchmarkChaosLoad tests heavy burst contention:
// Rapid randomized operations (60% enqueue bursts, 35% dequeues, 5% depth inspections)
// with randomly distributed task IDs, priority spikes, and concurrent contention.
func BenchmarkChaosLoad(b *testing.B) {
	b.Run("Coarse", func(b *testing.B) {
		q := queue.NewPriorityQueue()
		runChaosBenchmark(b, q)
	})

	b.Run("Sharded", func(b *testing.B) {
		q := queue.NewShardedPriorityQueue(32)
		runChaosBenchmark(b, q)
	})
}

func runChaosBenchmark(b *testing.B, q queue.TaskQueue) {
	origGOMAXPROCS := runtime.GOMAXPROCS(32)
	defer runtime.GOMAXPROCS(origGOMAXPROCS)

	for i := 0; i < 2000; i++ {
		q.Enqueue(&pb.TaskAssignment{
			WorkflowId: "chaos-prefill",
			Task:       &pb.TaskSpec{Id: fmt.Sprintf("init-%d", i)},
		}, i%10)
	}

	b.SetParallelism(1)
	b.ResetTimer()

	var counter atomic.Int64
	b.RunParallel(func(pbState *testing.PB) {
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		for pbState.Next() {
			c := counter.Add(1)
			dice := r.Intn(100)
			if dice < 60 {
				// 60%: Random task insertion with priority spike
				p := r.Intn(20)
				tid := fmt.Sprintf("chaos-task-%d-%d", c%500, r.Intn(1000))
				q.Enqueue(&pb.TaskAssignment{
					WorkflowId: "chaos-wf",
					Task:       &pb.TaskSpec{Id: tid},
				}, p)
			} else if dice < 95 {
				// 35%: Dequeue
				q.Dequeue()
			} else {
				// 5%: Depth check
				_ = q.Depth()
			}
		}
	})
}
