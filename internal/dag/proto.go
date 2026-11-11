package dag

import (
	pb "orchestra/api/proto/orchestra/v1"
)

// FromProtoWorkflowSpec converts a protobuf WorkflowSpec into a pure domain Workflow.
func FromProtoWorkflowSpec(spec *pb.WorkflowSpec) Workflow {
	if spec == nil {
		return Workflow{}
	}

	tasks := make([]Task, len(spec.GetTasks()))
	for i, t := range spec.GetTasks() {
		tasks[i] = FromProtoTaskSpec(t)
	}

	return Workflow{
		ID:            spec.GetId(),
		Name:          spec.GetName(),
		FailurePolicy: FromProtoFailurePolicy(spec.GetFailurePolicy()),
		Tasks:         tasks,
	}
}

// ToProto converts a domain Workflow into a protobuf WorkflowSpec.
func (w Workflow) ToProto() *pb.WorkflowSpec {
	tasks := make([]*pb.TaskSpec, len(w.Tasks))
	for i, t := range w.Tasks {
		tasks[i] = t.ToProto()
	}

	return &pb.WorkflowSpec{
		Id:            w.ID,
		Name:          w.Name,
		FailurePolicy: w.FailurePolicy.ToProto(),
		Tasks:         tasks,
	}
}

// FromProtoTaskSpec converts a protobuf TaskSpec into a domain Task.
func FromProtoTaskSpec(spec *pb.TaskSpec) Task {
	if spec == nil {
		return Task{}
	}

	deps := make([]string, len(spec.GetDependencies()))
	copy(deps, spec.GetDependencies())

	return Task{
		ID:           spec.GetId(),
		Type:         spec.GetType(),
		Payload:      spec.GetPayload(),
		TimeoutSec:   spec.GetTimeoutSeconds(),
		MaxRetries:   spec.GetMaxRetries(),
		Dependencies: deps,
	}
}

// ToProto converts a domain Task into a protobuf TaskSpec.
func (t Task) ToProto() *pb.TaskSpec {
	deps := make([]string, len(t.Dependencies))
	copy(deps, t.Dependencies)

	return &pb.TaskSpec{
		Id:             t.ID,
		Type:           t.Type,
		Payload:        t.Payload,
		TimeoutSeconds: t.TimeoutSec,
		MaxRetries:     t.MaxRetries,
		Dependencies:   deps,
	}
}

// FromProtoFailurePolicy maps protobuf FailurePolicy to domain FailurePolicy.
func FromProtoFailurePolicy(p pb.FailurePolicy) FailurePolicy {
	switch p {
	case pb.FailurePolicy_FAILURE_POLICY_CONTINUE:
		return FailurePolicyContinue
	case pb.FailurePolicy_FAILURE_POLICY_FAIL_FAST:
		return FailurePolicyFailFast
	default:
		return FailurePolicyFailFast
	}
}

// ToProto maps domain FailurePolicy to protobuf FailurePolicy.
func (p FailurePolicy) ToProto() pb.FailurePolicy {
	switch p {
	case FailurePolicyContinue:
		return pb.FailurePolicy_FAILURE_POLICY_CONTINUE
	case FailurePolicyFailFast:
		return pb.FailurePolicy_FAILURE_POLICY_FAIL_FAST
	default:
		return pb.FailurePolicy_FAILURE_POLICY_FAIL_FAST
	}
}

// FromProtoTaskState maps protobuf TaskState to domain TaskState.
func FromProtoTaskState(s pb.TaskState) TaskState {
	switch s {
	case pb.TaskState_TASK_STATE_PENDING:
		return TaskStatePending
	case pb.TaskState_TASK_STATE_READY:
		return TaskStateReady
	case pb.TaskState_TASK_STATE_ASSIGNED:
		return TaskStateAssigned
	case pb.TaskState_TASK_STATE_RUNNING:
		return TaskStateRunning
	case pb.TaskState_TASK_STATE_COMPLETED:
		return TaskStateCompleted
	case pb.TaskState_TASK_STATE_FAILED:
		return TaskStateFailed
	case pb.TaskState_TASK_STATE_SKIPPED:
		return TaskStateSkipped
	case pb.TaskState_TASK_STATE_CANCELLED:
		return TaskStateCancelled
	case pb.TaskState_TASK_STATE_DEAD_LETTER:
		return TaskStateDeadLetter
	default:
		return TaskStateUnspecified
	}
}

// ToProto maps domain TaskState to protobuf TaskState.
func (s TaskState) ToProto() pb.TaskState {
	switch s {
	case TaskStatePending:
		return pb.TaskState_TASK_STATE_PENDING
	case TaskStateReady:
		return pb.TaskState_TASK_STATE_READY
	case TaskStateAssigned:
		return pb.TaskState_TASK_STATE_ASSIGNED
	case TaskStateRunning:
		return pb.TaskState_TASK_STATE_RUNNING
	case TaskStateCompleted:
		return pb.TaskState_TASK_STATE_COMPLETED
	case TaskStateFailed:
		return pb.TaskState_TASK_STATE_FAILED
	case TaskStateSkipped:
		return pb.TaskState_TASK_STATE_SKIPPED
	case TaskStateCancelled:
		return pb.TaskState_TASK_STATE_CANCELLED
	case TaskStateDeadLetter:
		return pb.TaskState_TASK_STATE_DEAD_LETTER
	default:
		return pb.TaskState_TASK_STATE_UNSPECIFIED
	}
}

// ValidateAndBuild converts a protobuf WorkflowSpec into domain types, validates it, and builds a Graph.
// This preserves backward compatibility for existing scheduler and checker code.
func ValidateAndBuild(spec *pb.WorkflowSpec) (*Graph, error) {
	if spec == nil || len(spec.GetTasks()) == 0 {
		return nil, ErrEmptyWorkflow
	}
	domainWf := FromProtoWorkflowSpec(spec)
	return Build(domainWf)
}

// ResolutionResult describes next recommended state transitions for tasks and workflow.
type ResolutionResult struct {
	WorkflowState  pb.WorkflowState
	ReadyTasks     []string
	SkippedTasks   []string
	CancelledTasks []string
}

// Resolve evaluates current task states and computes the next transitions.
// Preserves backward compatibility for the scheduler engine.
func (d *DAG) Resolve(currentStates map[string]pb.TaskState) ResolutionResult {
	// Invariant protected: Read lock ensures DAG topology remains consistent while evaluating execution status
	d.mu.RLock()
	defer d.mu.RUnlock()

	var result ResolutionResult
	result.WorkflowState = pb.WorkflowState_WORKFLOW_STATE_RUNNING

	domainStates := make(map[string]TaskState, len(currentStates))
	hasDeadLetter := false
	deadLetterTasks := make([]string, 0)

	for id, s := range currentStates {
		ds := FromProtoTaskState(s)
		domainStates[id] = ds
		if ds == TaskStateDeadLetter {
			hasDeadLetter = true
			deadLetterTasks = append(deadLetterTasks, id)
		}
	}

	// Handle FAIL_FAST: Cancel non-terminal tasks if any task hits DEAD_LETTER
	if hasDeadLetter && d.policy == FailurePolicyFailFast {
		result.WorkflowState = pb.WorkflowState_WORKFLOW_STATE_FAILED
		for id := range d.tasks {
			s := domainStates[id]
			if s == TaskStatePending || s == TaskStateReady {
				result.CancelledTasks = append(result.CancelledTasks, id)
			}
		}
		return result
	}

	// Handle CONTINUE: Skip downstream descendants of dead-letter tasks
	skipSet := make(map[string]struct{})
	if hasDeadLetter && d.policy == FailurePolicyContinue {
		for _, dlID := range deadLetterTasks {
			downstream, _ := d.Downstream(dlID)
			for _, descID := range downstream {
				skipSet[descID] = struct{}{}
			}
		}

		for skipID := range skipSet {
			s := domainStates[skipID]
			if s == TaskStatePending || s == TaskStateReady {
				result.SkippedTasks = append(result.SkippedTasks, skipID)
			}
		}
	}

	// ReadyTasks computes all PENDING tasks whose dependencies are COMPLETED
	readyCandidates := d.ReadyTasks(domainStates)
	for _, id := range readyCandidates {
		if _, shouldSkip := skipSet[id]; !shouldSkip {
			result.ReadyTasks = append(result.ReadyTasks, id)
		}
	}

	// Check if workflow has reached a terminal state
	allTerminal := true
	for id := range d.tasks {
		s := domainStates[id]
		isScheduledSkipped := false
		for _, sk := range result.SkippedTasks {
			if sk == id {
				isScheduledSkipped = true
				break
			}
		}
		isScheduledCancelled := false
		for _, c := range result.CancelledTasks {
			if c == id {
				isScheduledCancelled = true
				break
			}
		}

		if isScheduledSkipped || isScheduledCancelled {
			continue
		}

		if !s.IsTerminal() {
			allTerminal = false
			break
		}
	}

	if allTerminal {
		if hasDeadLetter {
			result.WorkflowState = pb.WorkflowState_WORKFLOW_STATE_FAILED
		} else {
			result.WorkflowState = pb.WorkflowState_WORKFLOW_STATE_SUCCEEDED
		}
	}

	return result
}
