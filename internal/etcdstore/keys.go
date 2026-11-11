package etcdstore

import "fmt"

const (
	PrefixRoot      = "/orchestra"
	ElectionKey     = "/orchestra/leader/election"
	PrefixWorkers   = "/orchestra/workers"
	PrefixWorkflows = "/orchestra/workflows"
	PrefixTasks     = "/orchestra/tasks"
)

// WorkflowKey returns the etcd key for a workflow definition and status
func WorkflowKey(workflowID string) string {
	return fmt.Sprintf("%s/%s", PrefixWorkflows, workflowID)
}

// TaskKey returns the etcd key for a specific task within a workflow
func TaskKey(workflowID, taskID string) string {
	return fmt.Sprintf("%s/%s/%s", PrefixTasks, workflowID, taskID)
}

// WorkflowTasksPrefix returns the key prefix for all tasks belonging to a workflow
func WorkflowTasksPrefix(workflowID string) string {
	return fmt.Sprintf("%s/%s/", PrefixTasks, workflowID)
}

// WorkerKey returns the etcd key for worker lease registration
func WorkerKey(workerID string) string {
	return fmt.Sprintf("%s/%s", PrefixWorkers, workerID)
}
