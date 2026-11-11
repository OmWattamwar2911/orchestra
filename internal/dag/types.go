package dag

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Workflow and task input limits
const (
	MaxTasksPerWorkflow = 10000
	MaxPayloadSizeBytes = 10 * 1024 * 1024 // 10MB
	MaxTimeoutSeconds   = 86400 * 7        // 7 days
	MaxRetriesLimit     = 50
)

var idRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

// FailurePolicy defines how the scheduler handles task failures within a workflow.
type FailurePolicy int

const (
	FailurePolicyUnspecified FailurePolicy = 0
	FailurePolicyFailFast    FailurePolicy = 1
	FailurePolicyContinue    FailurePolicy = 2
)

func (p FailurePolicy) String() string {
	switch p {
	case FailurePolicyFailFast:
		return "FAIL_FAST"
	case FailurePolicyContinue:
		return "CONTINUE"
	default:
		return "UNSPECIFIED"
	}
}

// TaskState represents the execution lifecycle state of a task.
type TaskState int

const (
	TaskStateUnspecified TaskState = 0
	TaskStatePending     TaskState = 1
	TaskStateReady       TaskState = 2
	TaskStateAssigned    TaskState = 3
	TaskStateRunning     TaskState = 4
	TaskStateCompleted   TaskState = 5
	TaskStateFailed      TaskState = 6
	TaskStateSkipped     TaskState = 7
	TaskStateCancelled   TaskState = 8
	TaskStateDeadLetter  TaskState = 9
)

func (s TaskState) String() string {
	switch s {
	case TaskStatePending:
		return "PENDING"
	case TaskStateReady:
		return "READY"
	case TaskStateAssigned:
		return "ASSIGNED"
	case TaskStateRunning:
		return "RUNNING"
	case TaskStateCompleted:
		return "COMPLETED"
	case TaskStateFailed:
		return "FAILED"
	case TaskStateSkipped:
		return "SKIPPED"
	case TaskStateCancelled:
		return "CANCELLED"
	case TaskStateDeadLetter:
		return "DEAD_LETTER"
	default:
		return "UNSPECIFIED"
	}
}

// IsTerminal returns true if the task has reached a terminal state.
func (s TaskState) IsTerminal() bool {
	switch s {
	case TaskStateCompleted, TaskStateSkipped, TaskStateCancelled, TaskStateDeadLetter, TaskStateFailed:
		return true
	default:
		return false
	}
}

// Task is the domain representation of a task in a workflow DAG.
type Task struct {
	ID           string
	Type         string
	Payload      string
	TimeoutSec   int64
	MaxRetries   int32
	Dependencies []string
}

// Workflow is the domain representation of a directed acyclic graph workflow.
type Workflow struct {
	ID            string
	Name          string
	FailurePolicy FailurePolicy
	Tasks         []Task
}

// Sentinel errors for DAG validation and lookup failures.
var (
	ErrEmptyWorkflow        = errors.New("workflow must contain at least one task")
	ErrTooManyTasks         = errors.New("workflow exceeds maximum task limit of 10000")
	ErrPayloadTooLarge      = errors.New("task payload exceeds maximum limit of 10MB")
	ErrInvalidIDFormat      = errors.New("identifier contains invalid characters (must match ^[a-zA-Z0-9_-]{1,128}$)")
	ErrInvalidTimeout       = errors.New("task timeout is negative or exceeds maximum limit of 604800s")
	ErrInvalidMaxRetries    = errors.New("task max retries is negative or exceeds limit of 50")
	ErrDuplicateTaskID      = errors.New("duplicate task ID detected")
	ErrUnknownDependency    = errors.New("task references unknown dependency ID")
	ErrCycleDetected        = errors.New("cycle detected in workflow DAG")
	ErrSelfDependency       = errors.New("task cannot depend on itself")
	ErrTaskNotFound         = errors.New("task not found in DAG")
	ErrInvalidFailurePolicy = errors.New("invalid or unspecified failure policy")
)

// CycleError conveys the detected cycle path in the workflow DAG.
type CycleError struct {
	Path []string
}

func (e *CycleError) Error() string {
	return fmt.Sprintf("%v: %s", ErrCycleDetected, strings.Join(e.Path, " -> "))
}

func (e *CycleError) Unwrap() error {
	return ErrCycleDetected
}

func (e *CycleError) Is(target error) bool {
	return target == ErrCycleDetected
}

// SelfDependencyError conveys that a task explicitly depends on itself.
type SelfDependencyError struct {
	TaskID string
	Path   []string
}

func (e *SelfDependencyError) Error() string {
	return fmt.Sprintf("%v: task '%s' cannot depend on itself (cycle: %s)", ErrSelfDependency, e.TaskID, strings.Join(e.Path, " -> "))
}

func (e *SelfDependencyError) Unwrap() error {
	return ErrSelfDependency
}

func (e *SelfDependencyError) Is(target error) bool {
	return target == ErrSelfDependency || target == ErrCycleDetected
}

// DuplicateTaskIDError conveys duplicate task IDs within a workflow.
type DuplicateTaskIDError struct {
	TaskID string
}

func (e *DuplicateTaskIDError) Error() string {
	return fmt.Sprintf("%v: %s", ErrDuplicateTaskID, e.TaskID)
}

func (e *DuplicateTaskIDError) Unwrap() error {
	return ErrDuplicateTaskID
}

func (e *DuplicateTaskIDError) Is(target error) bool {
	return target == ErrDuplicateTaskID
}

// UnknownDependencyError conveys a task referencing a non-existent dependency ID.
type UnknownDependencyError struct {
	TaskID       string
	DependencyID string
}

func (e *UnknownDependencyError) Error() string {
	return fmt.Sprintf("%v: task '%s' depends on unknown task '%s'", ErrUnknownDependency, e.TaskID, e.DependencyID)
}

func (e *UnknownDependencyError) Unwrap() error {
	return ErrUnknownDependency
}

func (e *UnknownDependencyError) Is(target error) bool {
	return target == ErrUnknownDependency
}
