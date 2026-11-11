package store

import "fmt"

const (
	PrefixRoot        = "/orchestra"
	ElectionKey       = "/orchestra/leader/election"
	PrefixWorkers     = "/orchestra/workers"
	PrefixWorkflows   = "/orchestra/workflows"
	PrefixTasks       = "/orchestra/tasks"
	PrefixEvents      = "/orchestra/events"
)

// WorkflowKey returns the etcd key for a workflow definition and status: /orchestra/workflows/{id}
func WorkflowKey(workflowID string) string {
	return fmt.Sprintf("%s/%s", PrefixWorkflows, workflowID)
}

// TaskKey returns the etcd key for a task within a workflow: /orchestra/tasks/{wfid}/{taskid}
func TaskKey(workflowID, taskID string) string {
	return fmt.Sprintf("%s/%s/%s", PrefixTasks, workflowID, taskID)
}

// WorkflowTasksPrefix returns the key prefix for all tasks of a workflow: /orchestra/tasks/{wfid}/
func WorkflowTasksPrefix(workflowID string) string {
	return fmt.Sprintf("%s/%s/", PrefixTasks, workflowID)
}

// WorkerKey returns the etcd key for worker lease registration: /orchestra/workers/{id}
func WorkerKey(workerID string) string {
	return fmt.Sprintf("%s/%s", PrefixWorkers, workerID)
}

// EventKey returns the append-only etcd key for a state transition event: /orchestra/events/{seq}
// The sequence number is zero-padded to 20 digits to ensure lexicographical sorting matches chronological order.
func EventKey(seq int64) string {
	return fmt.Sprintf("%s/%020d", PrefixEvents, seq)
}

// EventsPrefix returns the key prefix for all append-only state transition events.
func EventsPrefix() string {
	return fmt.Sprintf("%s/", PrefixEvents)
}
