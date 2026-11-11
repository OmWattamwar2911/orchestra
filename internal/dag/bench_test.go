package dag_test

import (
	"fmt"
	"testing"

	"orchestra/internal/dag"
)

// generateDeepChain creates a sequential 10,000-task chain: T0 -> T1 -> ... -> T9999.
func generateDeepChain(n int) dag.Workflow {
	tasks := make([]dag.Task, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("T%d", i)
		var deps []string
		if i > 0 {
			deps = []string{fmt.Sprintf("T%d", i-1)}
		}
		tasks[i] = dag.Task{
			ID:           id,
			Dependencies: deps,
		}
	}
	return dag.Workflow{
		ID:    "wf-bench-chain",
		Tasks: tasks,
	}
}

// generateWideFanOut creates a 10,000-task workflow with 1 root and N-1 children.
func generateWideFanOut(n int) dag.Workflow {
	tasks := make([]dag.Task, n)
	tasks[0] = dag.Task{ID: "Root"}
	for i := 1; i < n; i++ {
		tasks[i] = dag.Task{
			ID:           fmt.Sprintf("Child-%d", i),
			Dependencies: []string{"Root"},
		}
	}
	return dag.Workflow{
		ID:    "wf-bench-fanout",
		Tasks: tasks,
	}
}

// generateLayeredDiamond creates 10,000 tasks organized in 100 layers of 100 tasks each.
func generateLayeredDiamond(layers, perLayer int) dag.Workflow {
	total := layers * perLayer
	tasks := make([]dag.Task, 0, total)

	for l := 0; l < layers; l++ {
		for i := 0; i < perLayer; i++ {
			id := fmt.Sprintf("L%d_T%d", l, i)
			var deps []string
			if l > 0 {
				// Each task in layer l depends on 2 tasks from layer l-1
				parent1 := fmt.Sprintf("L%d_T%d", l-1, i)
				parent2 := fmt.Sprintf("L%d_T%d", l-1, (i+1)%perLayer)
				deps = []string{parent1, parent2}
			}
			tasks = append(tasks, dag.Task{
				ID:           id,
				Dependencies: deps,
			})
		}
	}
	return dag.Workflow{
		ID:    "wf-bench-layered",
		Tasks: tasks,
	}
}

func BenchmarkValidate_10000Tasks_DeepChain(b *testing.B) {
	wf := generateDeepChain(10000)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := dag.Validate(wf); err != nil {
			b.Fatalf("validation failed: %v", err)
		}
	}
}

func BenchmarkValidate_10000Tasks_WideFanOut(b *testing.B) {
	wf := generateWideFanOut(10000)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := dag.Validate(wf); err != nil {
			b.Fatalf("validation failed: %v", err)
		}
	}
}

func BenchmarkValidate_10000Tasks_LayeredDiamond(b *testing.B) {
	wf := generateLayeredDiamond(100, 100) // 10,000 tasks
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := dag.Validate(wf); err != nil {
			b.Fatalf("validation failed: %v", err)
		}
	}
}

func BenchmarkBuild_10000Tasks(b *testing.B) {
	wf := generateLayeredDiamond(100, 100)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		d, err := dag.Build(wf)
		if err != nil || d == nil {
			b.Fatalf("build failed: %v", err)
		}
	}
}

func BenchmarkReadyTasks_10000Tasks(b *testing.B) {
	wf := generateLayeredDiamond(100, 100)
	d, err := dag.Build(wf)
	if err != nil {
		b.Fatalf("build failed: %v", err)
	}

	state := make(map[string]dag.TaskState, 10000)
	for _, t := range wf.Tasks {
		state[t.ID] = dag.TaskStatePending
	}
	// Complete layer 0 so layer 1 tasks become ready
	for i := 0; i < 100; i++ {
		state[fmt.Sprintf("L0_T%d", i)] = dag.TaskStateCompleted
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ready := d.ReadyTasks(state)
		if len(ready) == 0 {
			b.Fatal("expected ready tasks")
		}
	}
}
