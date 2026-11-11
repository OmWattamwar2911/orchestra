package main

import (
	"fmt"
	"math/rand"

	pb "orchestra/api/proto/orchestra/v1"
	"orchestra/internal/dag"
)

type DAGShape string

const (
	ShapeChain   DAGShape = "chain"
	ShapeDiamond DAGShape = "diamond"
	ShapeFanOut  DAGShape = "fan_out"
	ShapeLayered DAGShape = "layered"
)

// GenerateRandomWorkflow builds a randomized, valid DAG workflow with 5 to 40 tasks.
// Driven entirely by the supplied seeded RNG for deterministic replaying.
func GenerateRandomWorkflow(rng *rand.Rand, wfID string) (*pb.WorkflowSpec, DAGShape) {
	// Task count strictly between 5 and 40
	numTasks := rng.Intn(36) + 5

	shapes := []DAGShape{ShapeChain, ShapeDiamond, ShapeFanOut, ShapeLayered}
	shape := shapes[rng.Intn(len(shapes))]

	policy := pb.FailurePolicy_FAILURE_POLICY_FAIL_FAST
	if rng.Float64() < 0.20 {
		policy = pb.FailurePolicy_FAILURE_POLICY_CONTINUE
	}

	var tasks []*pb.TaskSpec

	switch shape {
	case ShapeChain:
		tasks = generateChain(rng, numTasks)
	case ShapeDiamond:
		tasks = generateDiamond(rng, numTasks)
	case ShapeFanOut:
		tasks = generateFanOut(rng, numTasks)
	case ShapeLayered:
		tasks = generateLayered(rng, numTasks)
	}

	spec := &pb.WorkflowSpec{
		Id:            wfID,
		Name:          fmt.Sprintf("Chaos-%s-%s", shape, wfID),
		FailurePolicy: policy,
		Tasks:         tasks,
	}

	// Invariant protected: Verify generated DAG topology using pure Kahn validation logic
	domainWf := dag.FromProtoWorkflowSpec(spec)
	if err := dag.Validate(domainWf); err != nil {
		// Fallback to guaranteed valid chain if edge case generated invalid shape
		spec.Tasks = generateChain(rng, numTasks)
	}

	return spec, shape
}

func generateChain(rng *rand.Rand, n int) []*pb.TaskSpec {
	tasks := make([]*pb.TaskSpec, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("chain-t%d", i)
		var deps []string
		if i > 0 {
			deps = []string{fmt.Sprintf("chain-t%d", i-1)}
		}
		tasks[i] = makeTaskSpec(rng, id, deps)
	}
	return tasks
}

func generateDiamond(rng *rand.Rand, n int) []*pb.TaskSpec {
	tasks := make([]*pb.TaskSpec, 0, n)
	root := makeTaskSpec(rng, "root", nil)
	tasks = append(tasks, root)

	numBranches := n - 2
	if numBranches < 1 {
		numBranches = 1
	}

	branchIDs := make([]string, numBranches)
	for i := 0; i < numBranches; i++ {
		bID := fmt.Sprintf("branch-%d", i)
		branchIDs[i] = bID
		tasks = append(tasks, makeTaskSpec(rng, bID, []string{"root"}))
	}

	sink := makeTaskSpec(rng, "sink", branchIDs)
	tasks = append(tasks, sink)
	return tasks
}

func generateFanOut(rng *rand.Rand, n int) []*pb.TaskSpec {
	tasks := make([]*pb.TaskSpec, 0, n)
	root := makeTaskSpec(rng, "fetch-root", nil)
	tasks = append(tasks, root)

	fanCount := n - 2
	if fanCount < 1 {
		fanCount = 1
	}

	fanIDs := make([]string, fanCount)
	for i := 0; i < fanCount; i++ {
		fID := fmt.Sprintf("fan-%d", i)
		fanIDs[i] = fID
		tasks = append(tasks, makeTaskSpec(rng, fID, []string{"fetch-root"}))
	}

	aggregate := makeTaskSpec(rng, "aggregate-sink", fanIDs)
	tasks = append(tasks, aggregate)
	return tasks
}

func generateLayered(rng *rand.Rand, n int) []*pb.TaskSpec {
	numLayers := 4
	if n < 8 {
		numLayers = 3
	}
	tasksPerLayer := n / numLayers
	if tasksPerLayer < 1 {
		tasksPerLayer = 1
	}

	tasks := make([]*pb.TaskSpec, 0, n)
	layers := make([][]string, numLayers)

	taskIdx := 0
	for l := 0; l < numLayers; l++ {
		count := tasksPerLayer
		if l == numLayers-1 {
			// Include any remaining tasks in final layer
			count = n - taskIdx
			if count < 1 {
				count = 1
			}
		}

		layers[l] = make([]string, 0, count)
		for c := 0; c < count; c++ {
			id := fmt.Sprintf("L%d-t%d", l, c)
			layers[l] = append(layers[l], id)

			var deps []string
			if l > 0 {
				prevLayer := layers[l-1]
				if len(prevLayer) > 0 {
					// Pick 1 to 2 parents from previous layer
					numDeps := 1
					if len(prevLayer) > 1 && rng.Float64() < 0.5 {
						numDeps = 2
					}
					perm := rng.Perm(len(prevLayer))
					for d := 0; d < numDeps; d++ {
						deps = append(deps, prevLayer[perm[d]])
					}
				}
			}

			tasks = append(tasks, makeTaskSpec(rng, id, deps))
			taskIdx++
		}
	}

	return tasks
}

func makeTaskSpec(rng *rand.Rand, id string, deps []string) *pb.TaskSpec {
	roll := rng.Float64()
	var taskType, payload string

	switch {
	case roll < 0.50:
		taskType = "echo"
		payload = fmt.Sprintf("payload for %s", id)
	case roll < 0.80:
		taskType = "sleep"
		// Short sleeps (10ms to 40ms) keep 200 concurrent workflows draining quickly
		payload = fmt.Sprintf("%dms", rng.Intn(31)+10)
	case roll < 0.95:
		taskType = "cpu"
		payload = fmt.Sprintf("%d", rng.Intn(1000)+500)
	default:
		// 5% flaky tasks to exercise retry and dead-letter logic
		taskType = "flaky"
		payload = "0.15"
	}

	return &pb.TaskSpec{
		Id:             id,
		Type:           taskType,
		Payload:        payload,
		TimeoutSeconds: 25,
		MaxRetries:     3,
		Dependencies:   deps,
	}
}
