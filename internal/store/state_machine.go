package store

import (
	"errors"
	"fmt"

	pb "orchestra/api/proto/orchestra/v1"
)

var (
	ErrIllegalStateTransition  = errors.New("illegal state machine transition")
	ErrTaskInTerminalState     = errors.New("task is already in a terminal state")
	ErrWorkflowInTerminalState = errors.New("workflow is already in a terminal state")
	ErrStaleAttempt            = errors.New("attempt ID mismatch or stale attempt report")
	ErrFencingTokenMismatch    = errors.New("fencing check failed: node is no longer the active leader")
	ErrCASConflict             = errors.New("optimistic concurrency conflict: task mod revision changed")
	ErrWorkflowNotFound        = errors.New("workflow not found in store")
	ErrTaskNotFound            = errors.New("task not found in store")
	ErrWorkflowAlreadyExists   = errors.New("workflow already exists")
)

// IsTerminalTaskState returns true if the task state is terminal (no further transitions allowed).
func IsTerminalTaskState(s pb.TaskState) bool {
	switch s {
	case pb.TaskState_TASK_STATE_COMPLETED,
		pb.TaskState_TASK_STATE_SKIPPED,
		pb.TaskState_TASK_STATE_CANCELLED,
		pb.TaskState_TASK_STATE_DEAD_LETTER:
		return true
	default:
		return false
	}
}

// IsTerminalWorkflowState returns true if the workflow state is terminal.
func IsTerminalWorkflowState(s pb.WorkflowState) bool {
	switch s {
	case pb.WorkflowState_WORKFLOW_STATE_SUCCEEDED,
		pb.WorkflowState_WORKFLOW_STATE_FAILED,
		pb.WorkflowState_WORKFLOW_STATE_CANCELLED:
		return true
	default:
		return false
	}
}

// ValidateTaskTransition enforces legal transitions in the task state machine.
// Returns ErrIllegalStateTransition if the transition violates the state machine invariant.
func ValidateTaskTransition(current pb.TaskState, target pb.TaskState) error {
	if current == target {
		return nil // No-op idempotent transition
	}

	if IsTerminalTaskState(current) {
		return fmt.Errorf("%w: cannot transition from terminal state %s to %s", ErrIllegalStateTransition, current, target)
	}

	switch current {
	case pb.TaskState_TASK_STATE_PENDING:
		switch target {
		case pb.TaskState_TASK_STATE_READY,
			pb.TaskState_TASK_STATE_SKIPPED,
			pb.TaskState_TASK_STATE_CANCELLED:
			return nil
		}

	case pb.TaskState_TASK_STATE_READY:
		switch target {
		case pb.TaskState_TASK_STATE_ASSIGNED,
			pb.TaskState_TASK_STATE_CANCELLED:
			return nil
		}

	case pb.TaskState_TASK_STATE_ASSIGNED:
		switch target {
		case pb.TaskState_TASK_STATE_RUNNING,
			pb.TaskState_TASK_STATE_COMPLETED,
			pb.TaskState_TASK_STATE_FAILED,
			pb.TaskState_TASK_STATE_DEAD_LETTER,
			pb.TaskState_TASK_STATE_READY, // Worker lease expired / re-queue
			pb.TaskState_TASK_STATE_CANCELLED:
			return nil
		}

	case pb.TaskState_TASK_STATE_RUNNING:
		switch target {
		case pb.TaskState_TASK_STATE_COMPLETED,
			pb.TaskState_TASK_STATE_FAILED,
			pb.TaskState_TASK_STATE_DEAD_LETTER,
			pb.TaskState_TASK_STATE_READY, // Worker crash / timeout re-queued
			pb.TaskState_TASK_STATE_CANCELLED:
			return nil
		}

	case pb.TaskState_TASK_STATE_FAILED:
		switch target {
		case pb.TaskState_TASK_STATE_READY, // Auto-retry backoff
			pb.TaskState_TASK_STATE_DEAD_LETTER,
			pb.TaskState_TASK_STATE_CANCELLED:
			return nil
		}
	}

	return fmt.Errorf("%w: invalid transition from %s to %s", ErrIllegalStateTransition, current, target)
}

// ValidateWorkflowTransition enforces legal transitions in the workflow state machine.
func ValidateWorkflowTransition(current pb.WorkflowState, target pb.WorkflowState) error {
	if current == target {
		return nil
	}

	if IsTerminalWorkflowState(current) {
		return fmt.Errorf("%w: cannot transition from terminal workflow state %s to %s", ErrIllegalStateTransition, current, target)
	}

	if current == pb.WorkflowState_WORKFLOW_STATE_RUNNING {
		switch target {
		case pb.WorkflowState_WORKFLOW_STATE_SUCCEEDED,
			pb.WorkflowState_WORKFLOW_STATE_FAILED,
			pb.WorkflowState_WORKFLOW_STATE_CANCELLED:
			return nil
		}
	}

	return fmt.Errorf("%w: invalid workflow transition from %s to %s", ErrIllegalStateTransition, current, target)
}
