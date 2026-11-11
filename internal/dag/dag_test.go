package dag_test

import (
	"errors"
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"testing"

	pb "orchestra/api/proto/orchestra/v1"
	"orchestra/internal/dag"
)

func TestValidate_TableDriven(t *testing.T) {
	tests := []struct {
		name        string
		workflow    dag.Workflow
		wantErr     error
		errContains string
		checkErr    func(t *testing.T, err error)
	}{
		{
			name: "empty workflow",
			workflow: dag.Workflow{
				ID:    "wf-empty",
				Tasks: nil,
			},
			wantErr: dag.ErrEmptyWorkflow,
		},
		{
			name: "empty task id",
			workflow: dag.Workflow{
				ID: "wf-empty-id",
				Tasks: []dag.Task{
					{ID: ""},
				},
			},
			errContains: "task ID cannot be empty",
		},
		{
			name: "duplicate task ids",
			workflow: dag.Workflow{
				ID: "wf-dup",
				Tasks: []dag.Task{
					{ID: "A"},
					{ID: "B"},
					{ID: "A"},
				},
			},
			wantErr: dag.ErrDuplicateTaskID,
			checkErr: func(t *testing.T, err error) {
				var dupErr *dag.DuplicateTaskIDError
				if !errors.As(err, &dupErr) {
					t.Fatalf("expected DuplicateTaskIDError, got %T: %v", err, err)
				}
				if dupErr.TaskID != "A" {
					t.Fatalf("expected duplicate TaskID 'A', got '%s'", dupErr.TaskID)
				}
			},
		},
		{
			name: "unknown dependency id",
			workflow: dag.Workflow{
				ID: "wf-unknown",
				Tasks: []dag.Task{
					{ID: "A"},
					{ID: "B", Dependencies: []string{"C"}}, // C does not exist
				},
			},
			wantErr: dag.ErrUnknownDependency,
			checkErr: func(t *testing.T, err error) {
				var unkErr *dag.UnknownDependencyError
				if !errors.As(err, &unkErr) {
					t.Fatalf("expected UnknownDependencyError, got %T: %v", err, err)
				}
				if unkErr.TaskID != "B" || unkErr.DependencyID != "C" {
					t.Fatalf("expected TaskID='B' DependencyID='C', got '%s', '%s'", unkErr.TaskID, unkErr.DependencyID)
				}
			},
		},
		{
			name: "self dependency",
			workflow: dag.Workflow{
				ID: "wf-self",
				Tasks: []dag.Task{
					{ID: "A", Dependencies: []string{"A"}},
				},
			},
			wantErr: dag.ErrSelfDependency,
			checkErr: func(t *testing.T, err error) {
				var selfErr *dag.SelfDependencyError
				if !errors.As(err, &selfErr) {
					t.Fatalf("expected SelfDependencyError, got %T: %v", err, err)
				}
				if selfErr.TaskID != "A" {
					t.Fatalf("expected TaskID='A', got '%s'", selfErr.TaskID)
				}
				if !slices.Equal(selfErr.Path, []string{"A", "A"}) {
					t.Fatalf("expected cycle path [A A], got %v", selfErr.Path)
				}
			},
		},
		{
			name: "two node cycle A -> B -> A",
			workflow: dag.Workflow{
				ID: "wf-cycle-2",
				Tasks: []dag.Task{
					{ID: "A", Dependencies: []string{"B"}},
					{ID: "B", Dependencies: []string{"A"}},
				},
			},
			wantErr: dag.ErrCycleDetected,
			checkErr: func(t *testing.T, err error) {
				var cycleErr *dag.CycleError
				if !errors.As(err, &cycleErr) {
					t.Fatalf("expected CycleError, got %T: %v", err, err)
				}
				if len(cycleErr.Path) < 3 {
					t.Fatalf("cycle path too short: %v", cycleErr.Path)
				}
				// Verify cycle starts and ends at the same node
				if cycleErr.Path[0] != cycleErr.Path[len(cycleErr.Path)-1] {
					t.Fatalf("cycle path must start and end on same node, got: %v", cycleErr.Path)
				}
			},
		},
		{
			name: "three node cycle A -> B -> C -> A",
			workflow: dag.Workflow{
				ID: "wf-cycle-3",
				Tasks: []dag.Task{
					{ID: "A", Dependencies: []string{"C"}},
					{ID: "B", Dependencies: []string{"A"}},
					{ID: "C", Dependencies: []string{"B"}},
				},
			},
			wantErr: dag.ErrCycleDetected,
			checkErr: func(t *testing.T, err error) {
				var cycleErr *dag.CycleError
				if !errors.As(err, &cycleErr) {
					t.Fatalf("expected CycleError, got %T: %v", err, err)
				}
				if len(cycleErr.Path) != 4 {
					t.Fatalf("expected cycle path length 4, got %v", cycleErr.Path)
				}
				if cycleErr.Path[0] != cycleErr.Path[3] {
					t.Fatalf("cycle path must start and end at same node, got %v", cycleErr.Path)
				}
			},
		},
		{
			name: "cycle inside deep branch",
			workflow: dag.Workflow{
				ID: "wf-sub-cycle",
				Tasks: []dag.Task{
					{ID: "Root"},
					{ID: "Child1", Dependencies: []string{"Root"}},
					{ID: "Child2", Dependencies: []string{"Child1"}},
					{ID: "Loop1", Dependencies: []string{"Child2", "Loop2"}},
					{ID: "Loop2", Dependencies: []string{"Loop1"}},
				},
			},
			wantErr: dag.ErrCycleDetected,
			checkErr: func(t *testing.T, err error) {
				var cycleErr *dag.CycleError
				if !errors.As(err, &cycleErr) {
					t.Fatalf("expected CycleError, got %T: %v", err, err)
				}
				if cycleErr.Path[0] != cycleErr.Path[len(cycleErr.Path)-1] {
					t.Fatalf("cycle path must start and end at same node, got %v", cycleErr.Path)
				}
			},
		},
		{
			name: "valid diamond graph",
			workflow: dag.Workflow{
				ID: "wf-diamond",
				Tasks: []dag.Task{
					{ID: "A"},
					{ID: "B", Dependencies: []string{"A"}},
					{ID: "C", Dependencies: []string{"A"}},
					{ID: "D", Dependencies: []string{"B", "C"}},
				},
			},
			wantErr: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := dag.Validate(tc.workflow)
			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tc.wantErr)
				}
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected error wrapping %v, got: %v", tc.wantErr, err)
				}
			} else if tc.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tc.errContains) {
					t.Fatalf("expected error containing %q, got %v", tc.errContains, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.checkErr != nil && err != nil {
				tc.checkErr(t, err)
			}
		})
	}
}

func TestTopologicalOrder_SpecialTopologies(t *testing.T) {
	// 1. Diamond shape
	t.Run("diamond topology", func(t *testing.T) {
		wf := dag.Workflow{
			ID: "wf-diamond",
			Tasks: []dag.Task{
				{ID: "A"},
				{ID: "B", Dependencies: []string{"A"}},
				{ID: "C", Dependencies: []string{"A"}},
				{ID: "D", Dependencies: []string{"B", "C"}},
			},
		}
		d, err := dag.Build(wf)
		if err != nil {
			t.Fatalf("Build failed: %v", err)
		}

		order := d.TopologicalOrder()
		if len(order) != 4 {
			t.Fatalf("expected 4 tasks, got %d", len(order))
		}
		if order[0] != "A" || order[3] != "D" {
			t.Fatalf("expected order to start with A and end with D, got %v", order)
		}
		// B and C can be in any relative order, but both must come after A and before D
		pos := make(map[string]int)
		for i, id := range order {
			pos[id] = i
		}
		if pos["A"] >= pos["B"] || pos["A"] >= pos["C"] || pos["B"] >= pos["D"] || pos["C"] >= pos["D"] {
			t.Fatalf("topological invariant violated in order: %v", order)
		}
	})

	// 2. Wide fan-out
	t.Run("wide fan-out", func(t *testing.T) {
		tasks := []dag.Task{{ID: "Root"}}
		for i := 1; i <= 100; i++ {
			tasks = append(tasks, dag.Task{
				ID:           fmt.Sprintf("Child-%d", i),
				Dependencies: []string{"Root"},
			})
		}
		d, err := dag.Build(dag.Workflow{ID: "wf-fanout", Tasks: tasks})
		if err != nil {
			t.Fatalf("Build failed: %v", err)
		}

		order := d.TopologicalOrder()
		if len(order) != 101 {
			t.Fatalf("expected 101 tasks, got %d", len(order))
		}
		if order[0] != "Root" {
			t.Fatalf("Root must be first in topological order, got: %s", order[0])
		}
	})

	// 3. Deep chain
	t.Run("deep chain", func(t *testing.T) {
		tasks := []dag.Task{{ID: "Chain-0"}}
		for i := 1; i < 50; i++ {
			tasks = append(tasks, dag.Task{
				ID:           fmt.Sprintf("Chain-%d", i),
				Dependencies: []string{fmt.Sprintf("Chain-%d", i-1)},
			})
		}
		d, err := dag.Build(dag.Workflow{ID: "wf-chain", Tasks: tasks})
		if err != nil {
			t.Fatalf("Build failed: %v", err)
		}

		order := d.TopologicalOrder()
		for i := 0; i < 50; i++ {
			expected := fmt.Sprintf("Chain-%d", i)
			if order[i] != expected {
				t.Fatalf("expected order[%d] == %s, got %s", i, expected, order[i])
			}
		}
	})

	// 4. Disconnected sub-graphs
	t.Run("disconnected sub-graphs", func(t *testing.T) {
		wf := dag.Workflow{
			ID: "wf-disconnected",
			Tasks: []dag.Task{
				{ID: "A1"},
				{ID: "A2", Dependencies: []string{"A1"}},
				{ID: "B1"},
				{ID: "B2", Dependencies: []string{"B1"}},
				{ID: "Isolated"},
			},
		}
		d, err := dag.Build(wf)
		if err != nil {
			t.Fatalf("Build failed: %v", err)
		}

		order := d.TopologicalOrder()
		pos := make(map[string]int)
		for i, id := range order {
			pos[id] = i
		}

		if pos["A1"] >= pos["A2"] {
			t.Fatalf("A1 must come before A2, got %v", order)
		}
		if pos["B1"] >= pos["B2"] {
			t.Fatalf("B1 must come before B2, got %v", order)
		}
		if _, exists := pos["Isolated"]; !exists {
			t.Fatalf("Isolated must be in topo order")
		}
	})
}

func TestReadyTasks(t *testing.T) {
	wf := dag.Workflow{
		ID: "wf-ready-test",
		Tasks: []dag.Task{
			{ID: "Root1"},
			{ID: "Root2"},
			{ID: "Middle", Dependencies: []string{"Root1", "Root2"}},
			{ID: "Leaf", Dependencies: []string{"Middle"}},
		},
	}
	d, err := dag.Build(wf)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	state := map[string]dag.TaskState{
		"Root1":  dag.TaskStatePending,
		"Root2":  dag.TaskStatePending,
		"Middle": dag.TaskStatePending,
		"Leaf":   dag.TaskStatePending,
	}

	// 1. Initially, both roots should be READY
	ready := d.ReadyTasks(state)
	slices.Sort(ready)
	if !slices.Equal(ready, []string{"Root1", "Root2"}) {
		t.Fatalf("expected [Root1 Root2] ready, got %v", ready)
	}

	// 2. Complete Root1, Root2 is RUNNING: Middle should NOT be ready yet
	state["Root1"] = dag.TaskStateCompleted
	state["Root2"] = dag.TaskStateRunning
	ready = d.ReadyTasks(state)
	if len(ready) != 0 {
		t.Fatalf("expected 0 ready tasks, got %v", ready)
	}

	// 3. Complete Root2: Middle should now be READY
	state["Root2"] = dag.TaskStateCompleted
	ready = d.ReadyTasks(state)
	if !slices.Equal(ready, []string{"Middle"}) {
		t.Fatalf("expected [Middle] ready, got %v", ready)
	}

	// 4. Middle transitions to RUNNING: Middle is no longer PENDING, so not ready
	state["Middle"] = dag.TaskStateRunning
	ready = d.ReadyTasks(state)
	if len(ready) != 0 {
		t.Fatalf("expected 0 ready tasks, got %v", ready)
	}

	// 5. Complete Middle: Leaf should become READY
	state["Middle"] = dag.TaskStateCompleted
	ready = d.ReadyTasks(state)
	if !slices.Equal(ready, []string{"Leaf"}) {
		t.Fatalf("expected [Leaf] ready, got %v", ready)
	}

	// 6. Complete Leaf: All tasks completed, 0 ready tasks
	state["Leaf"] = dag.TaskStateCompleted
	ready = d.ReadyTasks(state)
	if len(ready) != 0 {
		t.Fatalf("expected 0 ready tasks when all completed, got %v", ready)
	}
}

func TestDownstream(t *testing.T) {
	wf := dag.Workflow{
		ID: "wf-downstream",
		Tasks: []dag.Task{
			{ID: "A"},
			{ID: "B", Dependencies: []string{"A"}},
			{ID: "C", Dependencies: []string{"A"}},
			{ID: "D", Dependencies: []string{"B", "C"}},
			{ID: "E", Dependencies: []string{"D"}},
			{ID: "Independent"},
		},
	}
	d, err := dag.Build(wf)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Downstream of A: B, C, D, E (excludes Independent)
	downA, err := d.Downstream("A")
	if err != nil {
		t.Fatalf("Downstream(A) failed: %v", err)
	}
	expectedA := []string{"B", "C", "D", "E"}
	if !slices.Equal(downA, expectedA) {
		t.Fatalf("expected downstream(A) == %v, got %v", expectedA, downA)
	}

	// Downstream of B: D, E
	downB, err := d.Downstream("B")
	if err != nil {
		t.Fatalf("Downstream(B) failed: %v", err)
	}
	expectedB := []string{"D", "E"}
	if !slices.Equal(downB, expectedB) {
		t.Fatalf("expected downstream(B) == %v, got %v", expectedB, downB)
	}

	// Downstream of leaf E: empty
	downE, err := d.Downstream("E")
	if err != nil {
		t.Fatalf("Downstream(E) failed: %v", err)
	}
	if len(downE) != 0 {
		t.Fatalf("expected downstream(E) empty, got %v", downE)
	}

	// Unknown task lookup
	_, err = d.Downstream("Missing")
	if !errors.Is(err, dag.ErrTaskNotFound) {
		t.Fatalf("expected ErrTaskNotFound, got: %v", err)
	}
}

func TestProtoConversions(t *testing.T) {
	protoSpec := &pb.WorkflowSpec{
		Id:            "wf-proto",
		Name:          "Proto Pipeline",
		FailurePolicy: pb.FailurePolicy_FAILURE_POLICY_CONTINUE,
		Tasks: []*pb.TaskSpec{
			{
				Id:             "task-1",
				Type:           "extract",
				Payload:        "payload-1",
				TimeoutSeconds: 30,
				MaxRetries:     3,
				Dependencies:   nil,
			},
			{
				Id:             "task-2",
				Type:           "load",
				Payload:        "payload-2",
				TimeoutSeconds: 60,
				MaxRetries:     1,
				Dependencies:   []string{"task-1"},
			},
		},
	}

	// 1. FromProtoWorkflowSpec
	domainWf := dag.FromProtoWorkflowSpec(protoSpec)
	if domainWf.ID != "wf-proto" || domainWf.Name != "Proto Pipeline" {
		t.Fatalf("unexpected domain workflow ID/Name: %s / %s", domainWf.ID, domainWf.Name)
	}
	if domainWf.FailurePolicy != dag.FailurePolicyContinue {
		t.Fatalf("expected FailurePolicyContinue, got %v", domainWf.FailurePolicy)
	}
	if len(domainWf.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(domainWf.Tasks))
	}

	// 2. ToProto roundtrip
	backToProto := domainWf.ToProto()
	if backToProto.GetId() != protoSpec.GetId() || backToProto.GetName() != protoSpec.GetName() {
		t.Fatalf("proto roundtrip ID/Name mismatch")
	}
	if backToProto.GetFailurePolicy() != protoSpec.GetFailurePolicy() {
		t.Fatalf("proto roundtrip failure policy mismatch")
	}
	if len(backToProto.GetTasks()) != len(protoSpec.GetTasks()) {
		t.Fatalf("proto roundtrip task count mismatch")
	}

	// 3. State conversions
	states := []pb.TaskState{
		pb.TaskState_TASK_STATE_PENDING,
		pb.TaskState_TASK_STATE_READY,
		pb.TaskState_TASK_STATE_ASSIGNED,
		pb.TaskState_TASK_STATE_RUNNING,
		pb.TaskState_TASK_STATE_COMPLETED,
		pb.TaskState_TASK_STATE_FAILED,
		pb.TaskState_TASK_STATE_SKIPPED,
		pb.TaskState_TASK_STATE_CANCELLED,
		pb.TaskState_TASK_STATE_DEAD_LETTER,
	}
	for _, ps := range states {
		ds := dag.FromProtoTaskState(ps)
		reconverted := ds.ToProto()
		if reconverted != ps {
			t.Fatalf("state conversion mismatch for %v: got %v", ps, reconverted)
		}
	}
}

// TestPropertyBased_1000RandomDAGs generates 1,000 random DAGs and random completion orders.
// It simulates an execution scheduler and strictly verifies:
// 1. No task is ever started before ALL of its dependencies have reached COMPLETED.
// 2. The simulation always terminates and completes all tasks.
func TestPropertyBased_1000RandomDAGs(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	const totalSimulations = 1000

	for iter := 0; iter < totalSimulations; iter++ {
		// Generate random number of nodes between 5 and 35
		numNodes := 5 + r.Intn(31)

		// Create nodes with strictly ordered IDs (0 to numNodes-1)
		// A task j may only depend on task i if i < j, mathematically guaranteeing acyclicity.
		tasks := make([]dag.Task, numNodes)
		edgeProb := 0.15 + r.Float64()*0.25 // random edge density between 15% and 40%

		for j := 0; j < numNodes; j++ {
			nodeID := fmt.Sprintf("T%d", j)
			var deps []string
			for i := 0; i < j; i++ {
				if r.Float64() < edgeProb {
					deps = append(deps, fmt.Sprintf("T%d", i))
				}
			}
			tasks[j] = dag.Task{
				ID:           nodeID,
				Dependencies: deps,
			}
		}

		wf := dag.Workflow{
			ID:    fmt.Sprintf("rand-wf-%d", iter),
			Tasks: tasks,
		}

		d, err := dag.Build(wf)
		if err != nil {
			t.Fatalf("[iter %d] unexpected validation error for generated DAG: %v", iter, err)
		}

		// Verify TopologicalOrder produces a valid total ordering
		topo := d.TopologicalOrder()
		if len(topo) != numNodes {
			t.Fatalf("[iter %d] topological sort returned %d nodes, expected %d", iter, len(topo), numNodes)
		}
		topoIndex := make(map[string]int, numNodes)
		for idx, id := range topo {
			topoIndex[id] = idx
		}
		for _, task := range tasks {
			for _, depID := range task.Dependencies {
				if topoIndex[depID] >= topoIndex[task.ID] {
					t.Fatalf("[iter %d] topological invariant violated: dependency %s (idx %d) appears after %s (idx %d)",
						iter, depID, topoIndex[depID], task.ID, topoIndex[task.ID])
				}
			}
		}

		// Initialize simulated scheduler
		state := make(map[string]dag.TaskState, numNodes)
		for _, task := range tasks {
			state[task.ID] = dag.TaskStatePending
		}

		completed := make(map[string]bool, numNodes)
		steps := 0
		maxSteps := numNodes * 2 // upper safety bound against infinite loops

		for len(completed) < numNodes && steps < maxSteps {
			steps++

			// Scheduler evaluates ready tasks
			ready := d.ReadyTasks(state)
			if len(ready) == 0 {
				t.Fatalf("[iter %d] deadlock detected at step %d: uncompleted tasks remain (%d/%d) but ReadyTasks is empty! States: %v",
					iter, steps, len(completed), numNodes, state)
			}

			// Invariant check: Assert that EVERY ready task is currently PENDING and ALL its dependencies are COMPLETED
			for _, readyID := range ready {
				if state[readyID] != dag.TaskStatePending {
					t.Fatalf("[iter %d] invariant violation: ReadyTasks returned task %s which is in state %v (must be PENDING)",
						iter, readyID, state[readyID])
				}

				parents, _ := d.Parents(readyID)
				for _, parentID := range parents {
					if !completed[parentID] {
						t.Fatalf("[iter %d] CRITICAL SCHEDULING INVARIANT VIOLATION: task %s was scheduled before dependency %s completed! (Dependency state: %v)",
							iter, readyID, parentID, state[parentID])
					}
				}
			}

			// Simulate random worker execution and completion:
			// Shuffle ready tasks to test arbitrary execution order
			r.Shuffle(len(ready), func(i, j int) {
				ready[i], ready[j] = ready[j], ready[i]
			})

			// Complete a random subset (1 to all) of the currently ready tasks in this step
			numToComplete := 1 + r.Intn(len(ready))
			for i := 0; i < numToComplete; i++ {
				taskID := ready[i]
				state[taskID] = dag.TaskStateCompleted
				completed[taskID] = true
			}
		}

		// Verify termination
		if len(completed) != numNodes {
			t.Fatalf("[iter %d] scheduler failed to terminate within %d steps: completed %d/%d tasks",
				iter, maxSteps, len(completed), numNodes)
		}
	}
}
