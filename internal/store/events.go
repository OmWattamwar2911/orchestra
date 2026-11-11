package store

import (
	pb "orchestra/api/proto/orchestra/v1"
)

// StateTransitionEvent represents an immutable, append-only record written to etcd
// for every task or workflow state transition in the same transaction as the state mutation.
type StateTransitionEvent struct {
	SequenceNumber int64  `json:"sequence_number"`
	Timestamp      int64  `json:"timestamp"`
	WorkflowID     string `json:"workflow_id"`
	TaskID         string `json:"task_id,omitempty"`
	AttemptID      int32  `json:"attempt_id,omitempty"`
	FromState      string `json:"from_state"`
	ToState        string `json:"to_state"`
	LeaderTerm     int64  `json:"leader_term"`
	WorkerID       string `json:"worker_id,omitempty"`
	ErrorMessage   string `json:"error_message,omitempty"`
}

// NewTaskTransitionEvent constructs an event record for a task state change.
func NewTaskTransitionEvent(seq int64, now int64, wfID, taskID string, attempt int32, from, to pb.TaskState, term int64, workerID, errMsg string) StateTransitionEvent {
	return StateTransitionEvent{
		SequenceNumber: seq,
		Timestamp:      now,
		WorkflowID:     wfID,
		TaskID:         taskID,
		AttemptID:      attempt,
		FromState:      from.String(),
		ToState:        to.String(),
		LeaderTerm:     term,
		WorkerID:       workerID,
		ErrorMessage:   errMsg,
	}
}

// NewWorkflowTransitionEvent constructs an event record for a workflow state change.
func NewWorkflowTransitionEvent(seq int64, now int64, wfID string, from, to pb.WorkflowState, term int64, errMsg string) StateTransitionEvent {
	return StateTransitionEvent{
		SequenceNumber: seq,
		Timestamp:      now,
		WorkflowID:     wfID,
		FromState:      from.String(),
		ToState:        to.String(),
		LeaderTerm:     term,
		ErrorMessage:   errMsg,
	}
}
