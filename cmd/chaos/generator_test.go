package main

import (
	"fmt"
	"math/rand"
	"testing"

	"orchestra/internal/dag"
)

func TestGenerator_RandomWorkflowShapesAndValidity(t *testing.T) {
	// Verify that 100 randomly generated workflows across seeds are 100% valid DAGs
	for seed := int64(1001); seed <= 1100; seed++ {
		rng := rand.New(rand.NewSource(seed))
		wfID := fmt.Sprintf("test-wf-%d", seed)
		spec, shape := GenerateRandomWorkflow(rng, wfID)

		if len(spec.Tasks) < 5 || len(spec.Tasks) > 40 {
			t.Fatalf("seed %d: task count %d out of bounds [5, 40]", seed, len(spec.Tasks))
		}

		domainWf := dag.FromProtoWorkflowSpec(spec)
		if err := dag.Validate(domainWf); err != nil {
			t.Fatalf("seed %d (shape %s): generated invalid DAG: %v", seed, shape, err)
		}

		// Topological ordering must succeed
		order, err := dag.TopologicalOrderFromTasks(domainWf.Tasks)
		if err != nil {
			t.Fatalf("seed %d (shape %s): topological sort failed: %v", seed, shape, err)
		}
		if len(order) != len(spec.Tasks) {
			t.Fatalf("seed %d: expected %d tasks in order, got %d", seed, len(spec.Tasks), len(order))
		}
	}
}
