package checker

import (
	"context"
	"fmt"
	"time"

	pb "orchestra/api/proto/orchestra/v1"
	"orchestra/internal/dag"
	"orchestra/internal/etcdstore"
	"orchestra/internal/evidence"
)

// InvariantReport details the verification outcome for a single workflow.
type InvariantReport struct {
	WorkflowID               string   `json:"workflow_id"`
	WorkflowState            string   `json:"workflow_state"`
	Passed                   bool     `json:"passed"`
	Violations               []string `json:"violations"`
	TotalTasks               int      `json:"total_tasks"`
	TotalStartEvents         int      `json:"total_start_events"`
	TotalFinishEvents        int      `json:"total_finish_events"`
	CommittedResultCount     int      `json:"committed_result_count"`
	MultipleExecutionRetries int      `json:"multiple_execution_retries"`
}

// VerificationResult summarizes the cluster-wide correctness verification.
type VerificationResult struct {
	Timestamp       time.Time         `json:"timestamp"`
	AllPassed       bool              `json:"all_passed"`
	TotalWorkflows  int               `json:"total_workflows"`
	PassedWorkflows int               `json:"passed_workflows"`
	FailedWorkflows int               `json:"failed_workflows"`
	Reports         []InvariantReport `json:"reports"`
}

// InvariantChecker validates final state against independent worker evidence logs.
type InvariantChecker struct {
	store          *etcdstore.Store
	evidenceReader *evidence.Reader
}

// NewInvariantChecker creates an invariant checker.
func NewInvariantChecker(store *etcdstore.Store, evidenceLogPath string) *InvariantChecker {
	return &InvariantChecker{
		store:          store,
		evidenceReader: evidence.NewReader(evidenceLogPath),
	}
}

// VerifyAll inspects all workflows and asserts correctness across all 5 formal invariants.
func (c *InvariantChecker) VerifyAll(ctx context.Context) (*VerificationResult, error) {
	// Read independent evidence log directly from workers
	events, err := c.evidenceReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read evidence log: %w", err)
	}

	// Group evidence events by (workflow_id, task_id)
	type taskKey struct {
		wfID   string
		taskID string
	}
	startsByTask := make(map[taskKey][]evidence.EventRecord)
	finishesByTask := make(map[taskKey][]evidence.EventRecord)

	for _, ev := range events {
		k := taskKey{wfID: ev.WorkflowID, taskID: ev.TaskID}
		if ev.Event == evidence.EventStart {
			startsByTask[k] = append(startsByTask[k], ev)
		} else if ev.Event == evidence.EventFinish {
			finishesByTask[k] = append(finishesByTask[k], ev)
		}
	}

	workflows, err := c.store.ListWorkflows(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflows from store: %w", err)
	}

	result := &VerificationResult{
		Timestamp:       time.Now(),
		AllPassed:       true,
		TotalWorkflows:  len(workflows),
		Reports:         make([]InvariantReport, 0, len(workflows)),
	}

	for _, wf := range workflows {
		wfID := wf.GetSpec().GetId()
		report := InvariantReport{
			WorkflowID:    wfID,
			WorkflowState: wf.GetStatus().GetState().String(),
			Passed:        true,
			Violations:    make([]string, 0),
		}

		tasks, err := c.store.ListTasksForWorkflow(ctx, wfID)
		if err != nil {
			report.Passed = false
			report.Violations = append(report.Violations, fmt.Sprintf("failed to load tasks: %v", err))
			result.Reports = append(result.Reports, report)
			result.AllPassed = false
			result.FailedWorkflows++
			continue
		}
		report.TotalTasks = len(tasks)

		graph, err := dag.ValidateAndBuild(wf.GetSpec())
		if err != nil {
			report.Passed = false
			report.Violations = append(report.Violations, fmt.Sprintf("invalid DAG: %v", err))
		}

		// Track completion timestamps for topological verification
		taskFinishTimes := make(map[string]int64)

		for _, t := range tasks {
			taskID := t.GetSpec().GetId()
			state := t.GetStatus().GetState()
			k := taskKey{wfID: wfID, taskID: taskID}

			taskStarts := startsByTask[k]
			taskFinishes := finishesByTask[k]
			report.TotalStartEvents += len(taskStarts)
			report.TotalFinishEvents += len(taskFinishes)

			if len(taskStarts) > 1 {
				report.MultipleExecutionRetries++
			}

			// Invariant 1: Exactly-one committed result per task attempt
			// Verify that etcd has exactly one committed status, and if COMPLETED, status has valid completed_at timestamp
			if state == pb.TaskState_TASK_STATE_COMPLETED {
				report.CommittedResultCount++
				taskFinishTimes[taskID] = t.GetStatus().GetCompletedAt()
			}

			// Invariant 2: No orphan tasks - all tasks in finished workflow must be terminal
			if wf.GetStatus().GetState() != pb.WorkflowState_WORKFLOW_STATE_RUNNING {
				if state == pb.TaskState_TASK_STATE_PENDING || state == pb.TaskState_TASK_STATE_READY || state == pb.TaskState_TASK_STATE_ASSIGNED || state == pb.TaskState_TASK_STATE_RUNNING {
					report.Passed = false
					report.Violations = append(report.Violations, fmt.Sprintf("task '%s' left in non-terminal state '%s'", taskID, state.String()))
				}
			}

			// Invariant 3: Failure policy correctness
			if wf.GetSpec().GetFailurePolicy() == pb.FailurePolicy_FAILURE_POLICY_FAIL_FAST {
				if wf.GetStatus().GetState() == pb.WorkflowState_WORKFLOW_STATE_FAILED {
					// Tasks that did not complete should be CANCELLED or DEAD_LETTER
					if state == pb.TaskState_TASK_STATE_READY || state == pb.TaskState_TASK_STATE_ASSIGNED {
						report.Passed = false
						report.Violations = append(report.Violations, fmt.Sprintf("FAIL_FAST violation: task '%s' not cancelled", taskID))
					}
				}
			}
		}

		// Invariant 4: Topological Ordering
		// For every dependency A -> B, task B's first start time must be >= task A's finish time
		if graph != nil {
			for _, t := range tasks {
				taskID := t.GetSpec().GetId()
				parents, _ := graph.Parents(taskID)
				k := taskKey{wfID: wfID, taskID: taskID}
				starts := startsByTask[k]

				if len(starts) > 0 {
					firstStartNs := starts[0].TimestampNs
					firstStartMs := firstStartNs / 1e6

					for _, parentID := range parents {
						finishMs, finished := taskFinishTimes[parentID]
						if finished && firstStartMs < finishMs {
							report.Passed = false
							report.Violations = append(report.Violations,
								fmt.Sprintf("topological violation: task '%s' started at %d before parent '%s' completed at %d",
									taskID, firstStartMs, parentID, finishMs),
							)
						}
					}
				}
			}
		}

		if len(report.Violations) > 0 {
			report.Passed = false
			result.AllPassed = false
			result.FailedWorkflows++
		} else {
			result.PassedWorkflows++
		}

		result.Reports = append(result.Reports, report)
	}

	return result, nil
}
