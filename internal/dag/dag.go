package dag

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

// DAG represents a validated Directed Acyclic Graph of workflow tasks.
type DAG struct {
	// Invariant protected: RWMutex ensures consistent snapshot of graph adjacency and task definitions across concurrent lookups
	mu         sync.RWMutex
	workflowID string
	policy     FailurePolicy
	tasks      map[string]Task
	parents    map[string][]string // taskID -> list of direct dependencies (parents)
	children   map[string][]string // taskID -> list of direct dependents (children)
	topoOrder  []string            // Topologically sorted task IDs via Kahn's algorithm
}

// Graph is an alias for DAG to maintain backward compatibility.
type Graph = DAG

// Validate verifies that a workflow forms a valid DAG:
// - Rejects empty workflows.
// - Rejects duplicate task IDs.
// - Rejects self-dependencies.
// - Rejects unknown dependency IDs.
// - Detects cycles using Kahn's algorithm and extracts the cycle path.
func Validate(w Workflow) error {
	if len(w.Tasks) == 0 {
		return ErrEmptyWorkflow
	}
	if len(w.Tasks) > MaxTasksPerWorkflow {
		return fmt.Errorf("%w: %d tasks", ErrTooManyTasks, len(w.Tasks))
	}
	if w.ID != "" && !idRegex.MatchString(w.ID) {
		return fmt.Errorf("%w: workflow ID '%s'", ErrInvalidIDFormat, w.ID)
	}

	taskMap := make(map[string]Task, len(w.Tasks))
	for _, t := range w.Tasks {
		if t.ID == "" {
			return errors.New("task ID cannot be empty")
		}
		if !idRegex.MatchString(t.ID) {
			return fmt.Errorf("%w: task ID '%s'", ErrInvalidIDFormat, t.ID)
		}
		if len(t.Payload) > MaxPayloadSizeBytes {
			return fmt.Errorf("%w: task '%s' payload size %d bytes", ErrPayloadTooLarge, t.ID, len(t.Payload))
		}
		if t.TimeoutSec < 0 || t.TimeoutSec > MaxTimeoutSeconds {
			return fmt.Errorf("%w: task '%s' timeout %d", ErrInvalidTimeout, t.ID, t.TimeoutSec)
		}
		if t.MaxRetries < 0 || t.MaxRetries > MaxRetriesLimit {
			return fmt.Errorf("%w: task '%s' max retries %d", ErrInvalidMaxRetries, t.ID, t.MaxRetries)
		}
		if _, exists := taskMap[t.ID]; exists {
			return &DuplicateTaskIDError{TaskID: t.ID}
		}
		taskMap[t.ID] = t
	}

	// Validate dependencies and self-loops
	children := make(map[string][]string, len(w.Tasks))
	inDegree := make(map[string]int, len(w.Tasks))
	nodeIDs := make([]string, 0, len(w.Tasks))

	for id := range taskMap {
		children[id] = make([]string, 0)
		inDegree[id] = 0
		nodeIDs = append(nodeIDs, id)
	}

	for id, task := range taskMap {
		seenDeps := make(map[string]struct{}, len(task.Dependencies))
		for _, depID := range task.Dependencies {
			if depID == id {
				return &SelfDependencyError{
					TaskID: id,
					Path:   []string{id, id},
				}
			}
			if _, exists := taskMap[depID]; !exists {
				return &UnknownDependencyError{
					TaskID:       id,
					DependencyID: depID,
				}
			}
			if _, seen := seenDeps[depID]; seen {
				continue // deduplicate redundant dependency declarations
			}
			seenDeps[depID] = struct{}{}

			children[depID] = append(children[depID], id)
			inDegree[id]++
		}
	}

	// Kahn's algorithm for cycle detection
	queue := make([]string, 0, len(taskMap))
	for _, id := range nodeIDs {
		if inDegree[id] == 0 {
			queue = append(queue, id)
		}
	}

	visitedCount := 0
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		visitedCount++

		for _, child := range children[curr] {
			inDegree[child]--
			if inDegree[child] == 0 {
				queue = append(queue, child)
			}
		}
	}

	if visitedCount != len(taskMap) {
		cyclePath := findCyclePath(nodeIDs, children)
		return &CycleError{Path: cyclePath}
	}

	return nil
}

// Validate calls Validate on the workflow receiver.
func (w Workflow) Validate() error {
	return Validate(w)
}

// Build constructs and validates a DAG from a Workflow domain model.
func Build(w Workflow) (*DAG, error) {
	if len(w.Tasks) == 0 {
		return nil, ErrEmptyWorkflow
	}

	taskMap := make(map[string]Task, len(w.Tasks))
	parents := make(map[string][]string, len(w.Tasks))
	children := make(map[string][]string, len(w.Tasks))
	inDegree := make(map[string]int, len(w.Tasks))
	nodeIDs := make([]string, 0, len(w.Tasks))

	for _, t := range w.Tasks {
		if t.ID == "" {
			return nil, errors.New("task ID cannot be empty")
		}
		if _, exists := taskMap[t.ID]; exists {
			return nil, &DuplicateTaskIDError{TaskID: t.ID}
		}
		taskMap[t.ID] = t
		parents[t.ID] = make([]string, 0)
		children[t.ID] = make([]string, 0)
		inDegree[t.ID] = 0
		nodeIDs = append(nodeIDs, t.ID)
	}

	for id, task := range taskMap {
		seenDeps := make(map[string]struct{}, len(task.Dependencies))
		for _, depID := range task.Dependencies {
			if depID == id {
				return nil, &SelfDependencyError{
					TaskID: id,
					Path:   []string{id, id},
				}
			}
			if _, exists := taskMap[depID]; !exists {
				return nil, &UnknownDependencyError{
					TaskID:       id,
					DependencyID: depID,
				}
			}
			if _, seen := seenDeps[depID]; seen {
				continue
			}
			seenDeps[depID] = struct{}{}

			parents[id] = append(parents[id], depID)
			children[depID] = append(children[depID], id)
			inDegree[id]++
		}
	}

	// Kahn's algorithm for deterministic topological order
	// Sort initial nodes with in-degree 0 for determinism
	initialZeroNodes := make([]string, 0, len(taskMap))
	for _, id := range nodeIDs {
		if inDegree[id] == 0 {
			initialZeroNodes = append(initialZeroNodes, id)
		}
	}
	sort.Strings(initialZeroNodes)

	queue := initialZeroNodes
	topoOrder := make([]string, 0, len(taskMap))

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		topoOrder = append(topoOrder, curr)

		// Sort children when multiple become ready to ensure deterministic order
		readyChildren := make([]string, 0, len(children[curr]))
		for _, child := range children[curr] {
			inDegree[child]--
			if inDegree[child] == 0 {
				readyChildren = append(readyChildren, child)
			}
		}
		sort.Strings(readyChildren)
		queue = append(queue, readyChildren...)
	}

	if len(topoOrder) != len(taskMap) {
		cyclePath := findCyclePath(nodeIDs, children)
		return nil, &CycleError{Path: cyclePath}
	}

	policy := w.FailurePolicy
	if policy == FailurePolicyUnspecified {
		policy = FailurePolicyFailFast
	}

	return &DAG{
		workflowID: w.ID,
		policy:     policy,
		tasks:      taskMap,
		parents:    parents,
		children:   children,
		topoOrder:  topoOrder,
	}, nil
}

// New is an alias for Build.
func New(w Workflow) (*DAG, error) {
	return Build(w)
}

// TopologicalOrder returns the topologically sorted task IDs computed via Kahn's algorithm.
func (d *DAG) TopologicalOrder() []string {
	// Invariant protected: Read lock ensures immutable access to the computed topological order
	d.mu.RLock()
	defer d.mu.RUnlock()

	out := make([]string, len(d.topoOrder))
	copy(out, d.topoOrder)
	return out
}

// TopoOrder is an alias for TopologicalOrder for backward compatibility.
func (d *DAG) TopoOrder() []string {
	return d.TopologicalOrder()
}

// ReadyTasks evaluates task states and returns all tasks that are currently PENDING
// and whose dependencies are ALL in the COMPLETED state.
func (d *DAG) ReadyTasks(state map[string]TaskState) []string {
	// Invariant protected: Read lock protects graph dependency structure during readiness evaluation
	d.mu.RLock()
	defer d.mu.RUnlock()

	var ready []string
	for _, id := range d.topoOrder {
		// Only consider tasks currently PENDING
		if state[id] != TaskStatePending {
			continue
		}

		// Verify every dependency is COMPLETED
		allDepsCompleted := true
		for _, parentID := range d.parents[id] {
			if state[parentID] != TaskStateCompleted {
				allDepsCompleted = false
				break
			}
		}

		if allDepsCompleted {
			ready = append(ready, id)
		}
	}
	return ready
}

// Downstream returns all transitive descendants of the given task ID.
// This is used for marking tasks as SKIPPED under the CONTINUE failure policy.
func (d *DAG) Downstream(taskID string) ([]string, error) {
	// Invariant protected: Read lock protects traversal of adjacency list
	d.mu.RLock()
	defer d.mu.RUnlock()

	if _, exists := d.tasks[taskID]; !exists {
		return nil, fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}

	visited := make(map[string]bool)
	var queue []string
	for _, child := range d.children[taskID] {
		visited[child] = true
		queue = append(queue, child)
	}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		for _, child := range d.children[curr] {
			if !visited[child] {
				visited[child] = true
				queue = append(queue, child)
			}
		}
	}

	descendants := make([]string, 0, len(visited))
	for id := range visited {
		descendants = append(descendants, id)
	}
	sort.Strings(descendants)
	return descendants, nil
}

// Descendants returns downstream transitive descendants as a set for backward compatibility.
func (d *DAG) Descendants(taskID string) (map[string]struct{}, error) {
	list, err := d.Downstream(taskID)
	if err != nil {
		return nil, err
	}
	res := make(map[string]struct{}, len(list))
	for _, id := range list {
		res[id] = struct{}{}
	}
	return res, nil
}

// Parents returns direct dependencies of the specified task ID.
func (d *DAG) Parents(taskID string) ([]string, error) {
	// Invariant protected: Read lock ensures safe concurrent access to parent dependency mapping
	d.mu.RLock()
	defer d.mu.RUnlock()

	p, exists := d.parents[taskID]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}
	out := make([]string, len(p))
	copy(out, p)
	return out, nil
}

// Children returns direct dependents of the specified task ID.
func (d *DAG) Children(taskID string) ([]string, error) {
	// Invariant protected: Read lock ensures safe concurrent access to child adjacency mapping
	d.mu.RLock()
	defer d.mu.RUnlock()

	c, exists := d.children[taskID]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}
	out := make([]string, len(c))
	copy(out, c)
	return out, nil
}

// Tasks returns a shallow copy of all registered tasks in the DAG.
func (d *DAG) Tasks() map[string]Task {
	// Invariant protected: Read lock ensures safe snapshot copy of registered tasks
	d.mu.RLock()
	defer d.mu.RUnlock()

	res := make(map[string]Task, len(d.tasks))
	for k, v := range d.tasks {
		res[k] = v
	}
	return res
}

// WorkflowID returns the ID of the workflow.
func (d *DAG) WorkflowID() string {
	// Invariant protected: Read lock ensures safe concurrent access to workflow ID
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.workflowID
}

// FailurePolicy returns the configured failure policy.
func (d *DAG) FailurePolicy() FailurePolicy {
	// Invariant protected: Read lock ensures safe concurrent access to failure policy
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.policy
}

// TopologicalOrderFromTasks is a standalone helper executing Kahn's algorithm on a slice of Tasks.
func TopologicalOrderFromTasks(tasks []Task) ([]string, error) {
	dag, err := Build(Workflow{Tasks: tasks})
	if err != nil {
		return nil, err
	}
	return dag.TopologicalOrder(), nil
}

// findCyclePath performs a depth-first search using 3-color graph coloring
// (0: unvisited, 1: currently visiting, 2: visited) to detect and extract an exact directed cycle path.
func findCyclePath(nodeIDs []string, children map[string][]string) []string {
	color := make(map[string]int, len(nodeIDs))
	var cycle []string

	var dfs func(u string, stack []string) bool
	dfs = func(u string, stack []string) bool {
		color[u] = 1
		stack = append(stack, u)

		// Sort child list for deterministic cycle extraction
		childList := make([]string, len(children[u]))
		copy(childList, children[u])
		sort.Strings(childList)

		for _, v := range childList {
			if color[v] == 1 {
				// Back-edge found! Extract cycle from stack
				for i, node := range stack {
					if node == v {
						cycle = make([]string, len(stack)-i+1)
						copy(cycle, stack[i:])
						cycle[len(cycle)-1] = v
						return true
					}
				}
			}
			if color[v] == 0 {
				if dfs(v, stack) {
					return true
				}
			}
		}

		color[u] = 2
		return false
	}

	sortedNodes := make([]string, len(nodeIDs))
	copy(sortedNodes, nodeIDs)
	sort.Strings(sortedNodes)

	for _, u := range sortedNodes {
		if color[u] == 0 {
			if dfs(u, nil) {
				return cycle
			}
		}
	}
	return nil
}
